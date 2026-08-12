#!/usr/bin/env bash
# One-shot in-cluster deploy of a service from the WORKING TREE (ADR-0200,
# ADR-0205) — the occasional "I need my uncommitted service in the cluster for
# edge/auth/e2e testing" case. No watch loop (that was Skaffold's job; the daily
# loop is native execution). Builds the image(s), imports them into k3d, and helm-
# upgrades the same chart prod uses with the local values overlay.
#
#   mise run service:deploy -- <svc>
#
# If the full tier (ArgoCD) manages this service, its auto-sync is paused first so
# self-heal does not revert your local image; re-enable with:
#   argocd app set local-service-<svc> --sync-policy automated   (or just cluster:full)
set -euo pipefail

CLUSTER="${CLUSTER:-platform}"
NS="platform"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

SVC="${1:?usage: mise run service:deploy -- <svc>}"
SVC_DIR="services/${SVC}"
VALUES="infra/gitops/services/local/values/${SVC}.yaml"
[ -d "$SVC_DIR" ] || {
  echo "✗ no such service: ${SVC_DIR}" >&2
  exit 1
}
[ -f "$VALUES" ] || {
  echo "✗ missing local values: ${VALUES}" >&2
  exit 1
}

k() { kubectl --context "k3d-${CLUSTER}" -n "$NS" "$@"; }
h() { helm --kube-context "k3d-${CLUSTER}" "$@"; }

# Bring up what this service declares it needs, before deploying it (ADR-0205,
# ADR-0600). Without this a deploy onto a bare base silently CrashLoops: orders
# wants Temporal and OpenFGA, catalog's values mount openfga-creds and point at an
# in-cluster OpenFGA URL, and every service chart mounts a <svc>-db secret. The
# dependency list is the service's own — the same declaration its native tasks use
# — so the two paths cannot disagree about what the service needs.
#
# This is the failure mode we explicitly chose not to inherit: Tilt lets you enable
# a subset and documents that it does NOT consider dependencies, which turns "why
# is my service CrashLooping" into a scavenger hunt.
# Read the declaration straight out of the service's own config rather than asking
# mise to resolve it — `mise tasks deps` only resolves from inside the service
# directory, and grepping the source of truth keeps this script explicit.
deps="$(grep -o 'dep:[a-z-]*' "${SVC_DIR}/.mise.toml" | sort -u || true)"
# db-secrets is an in-cluster-only need: a natively-run service reads .env instead,
# so it is not in the service's own task graph and is added on this path alone.
for dep in $deps dep:db-secrets; do
  bash scripts/dep-apply.sh "${dep#dep:}"
done

# The sibling services this one calls, from the same declaration (svc:*). In-cluster
# they resolve by DNS, so they must be PRESENT — but they must not be port-forwarded
# the way the native path forwards them, so this recurses into deploy rather than
# calling svc-apply.sh. A deployed orders whose catalog is missing is the same
# scavenger hunt as one whose Temporal is missing; only the symptom moves (a failing
# checkout instead of a CrashLoop).
#
# DEPLOYING_SERVICES carries the visited set through the recursion. Service call
# graphs are supposed to be acyclic, but nothing enforces that, and an accidental
# cycle here would fork-bomb rather than fail — so it is cut off explicitly.
svcs="$(grep -o 'svc:[a-z-]*' "${SVC_DIR}/.mise.toml" | sort -u || true)"
export DEPLOYING_SERVICES="${DEPLOYING_SERVICES:-} ${SVC}"
for s in $svcs; do
  name="${s#svc:}"
  case " ${DEPLOYING_SERVICES} " in
  *" ${name} "*)
    echo "→ ${name} already in this deploy chain — skipping (cycle guard)"
    continue
    ;;
  esac
  if k rollout status "deploy/${name}-server" --timeout=0 >/dev/null 2>&1; then
    echo "  ${name} already deployed — skipping"
  else
    echo "→ ${SVC} calls ${name}; deploying it first"
    bash scripts/service-deploy.sh "$name"
  fi
done

TAG="local-$(date +%s)" # unique tag forces a re-pull of the imported image
SET=(--set "image.repository=${SVC}-server" --set "image.tag=${TAG}")

# Build identity baked into the image (ADR-0103): the working-tree SHA (+ -dirty
# for uncommitted edits — the norm for this local path), so /version and the
# X-App-Version header report exactly what you deployed.
REV="$(git rev-parse --short=12 HEAD 2>/dev/null || echo unknown)"
git diff --quiet 2>/dev/null || REV="${REV}-dirty"
BUILD_ARGS=(--build-arg "GIT_SHA=${REV}" --build-arg BUILD_VERSION=local
  --build-arg "BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)")

echo "→ building ${SVC}-server"
docker build -t "${SVC}-server:${TAG}" \
  --build-arg SERVICE="${SVC}" --build-arg APP_CMD=server "${BUILD_ARGS[@]}" \
  -f "${SVC_DIR}/Dockerfile" .
k3d image import "${SVC}-server:${TAG}" -c "$CLUSTER"

# Build the worker too when this service declares one (orders, payment).
if grep -qE '^\s*enabled:\s*true' <(awk '/^worker:/{f=1} f' "$VALUES"); then
  echo "→ building ${SVC}-worker"
  docker build -t "${SVC}-worker:${TAG}" \
    --build-arg SERVICE="${SVC}" --build-arg APP_CMD=worker "${BUILD_ARGS[@]}" \
    -f "${SVC_DIR}/Dockerfile" .
  k3d image import "${SVC}-worker:${TAG}" -c "$CLUSTER"
  SET+=(--set "worker.image.repository=${SVC}-worker" --set "worker.image.tag=${TAG}")
fi

# Pause Argo auto-sync on this service if the full tier manages it.
if k -n argocd get application.argoproj.io "local-service-${SVC}" >/dev/null 2>&1; then
  echo "→ pausing ArgoCD auto-sync on local-service-${SVC}"
  k -n argocd patch application.argoproj.io "local-service-${SVC}" --type merge \
    -p '{"spec":{"syncPolicy":{"automated":null}}}'
fi

echo "→ helm upgrade ${SVC} (working-tree image ${TAG})"
# --take-ownership: when the full tier normally manages this service, its resources
# are owned by ArgoCD (Server-Side Apply), not a Helm release. Helm 4 refuses to
# adopt them without this flag. Auto-sync is already paused above, so taking
# ownership for the local override is safe; a re-run of cluster:full restores GitOps.
h upgrade --install "$SVC" infra/helm/service -n "$NS" -f "$VALUES" \
  --take-ownership --force-conflicts --set image.pullPolicy=IfNotPresent "${SET[@]}" --timeout 5m
k rollout restart "deploy/${SVC}-server"
k rollout status "deploy/${SVC}-server" --timeout=180s
echo "✓ ${SVC} deployed from working tree (tag ${TAG})"

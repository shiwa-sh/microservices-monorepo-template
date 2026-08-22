#!/usr/bin/env bash
# One-shot in-cluster deploy from the WORKING TREE (ADR-0200, ADR-0205) — the
# occasional "I need my uncommitted code in the cluster for edge/auth/e2e testing"
# case. No watch loop: the daily loop is native execution.
# Builds the image(s), loads them into the kind node, and helm-upgrades the same
# chart prod uses with the local values overlay.
#
#   mise run service:deploy -- <svc>
#
# Two kinds of deployable, one chart. A Go SERVICE lives in services/<name> and
# builds server (and worker) images from one multi-stage Dockerfile parameterised by
# SERVICE/APP_CMD. An APP lives in apps/<name>, builds a single image from its own
# Dockerfile, and is a deployable exactly when it has a local values file — which is
# what keeps apps/admin out of this path (it ships inside the lowdefy chart).
# Everything after the build is identical, because both are the same Helm chart with
# the same values layout, so the split is confined to resolving the source and the
# build recipe.
#
# If the full tier (ArgoCD) manages this service, its auto-sync is paused first so
# self-heal does not revert your local image; re-enable with:
#   argocd app set local-service-<svc> --sync-policy automated   (or just cluster:full)
set -euo pipefail

source "$(dirname "$0")/lib/argo.sh"

CLUSTER="${CLUSTER:-platform}"
source "$(dirname "$0")/lib/cluster-ctx.sh"
NS="platform"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

SVC="${1:?usage: mise run service:deploy -- <svc>}"
VALUES="infra/gitops/services/local/values/${SVC}.yaml"
[ -f "$VALUES" ] || {
  echo "✗ missing local values: ${VALUES}" >&2
  exit 1
}

if [ -d "services/${SVC}" ]; then
  KIND=service
  SVC_DIR="services/${SVC}"
  # The chart's deployment is always <name>-server; only the image name differs.
  IMAGE="${SVC}-server"
elif [ -d "apps/${SVC}" ]; then
  KIND=app
  SVC_DIR="apps/${SVC}"
  IMAGE="${SVC}"
else
  echo "✗ no such deployable: neither services/${SVC} nor apps/${SVC}" >&2
  exit 1
fi

k() { kubectl --context "$(cluster_ctx)" -n "$NS" "$@"; }
h() { helm --kube-context "$(cluster_ctx)" "$@"; }

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
# Comment lines are excluded and the name must start with a letter: a prose
# reference to `dep:*` in a comment otherwise matches with an EMPTY component name,
# and dep-apply.sh then fails its usage guard on an argument nobody wrote.
deps="$(grep -v '^[[:space:]]*#' "${SVC_DIR}/.mise.toml" | grep -o 'dep:[a-z][a-z-]*' | sort -u || true)"
# db-secrets is an in-cluster-only need: a natively-run service reads .env instead,
# so it is not in the service's own task graph and is added on this path alone. An
# app has no database, and the chart mounts it no <name>-db secret.
if [ "$KIND" = service ]; then
  deps="${deps} dep:db-secrets"
fi
# ONLY ON THE INNER LOOP. A `dep:*` is a lightweight STAND-IN — a plain Postgres,
# `temporal server start-dev`, an in-memory OpenFGA (ADR-0600) — and the full tier
# already runs the real component, deployed by Argo from the same charts production
# uses. Applying a stand-in there does not add a missing dependency: it adds a
# SECOND database beside CNPG's, in the same namespace, for a service whose values
# point at the real one. Measured: `cluster:add -- analytics` on the full tier
# created `deployment/postgres` next to a healthy CNPG cluster and then failed
# waiting for it to roll out, so the service it was asked to deploy never got built.
if [ "$(cluster_tier)" = "full" ]; then
  echo "→ full tier: dependencies come from the platform charts, not stand-ins"
  deps=""
fi
for dep in $deps; do
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
svcs="$(grep -v '^[[:space:]]*#' "${SVC_DIR}/.mise.toml" | grep -o 'svc:[a-z][a-z-]*' | sort -u || true)"
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

# publish_image — get a locally built image onto the ACTIVE TIER's nodes.
#
# One mechanism for both tiers, because both are kind (ADR-0600): `kind load
# docker-image` copies from the host's image store straight into every node of the
# named cluster, so a three-node tier needs no registry round trip and no pull
# credential for an image that already exists on this machine.
#
# This is the imperative path, where there is nothing to reconcile from git. What
# Argo deploys still comes from the local registry by a `registry.localhost:5000`
# reference, exactly as a deployed environment pulls from its own registry — that
# path is cluster:full's build_push, not this one.
publish_image() {
  kind load docker-image "$1" --name "$(cluster_name)" >/dev/null
}

TAG="local-$(date +%s)" # unique tag forces a re-pull of the imported image
REPO="${IMAGE}"
WORKER_REPO="${SVC}-worker"
SET=(--set "image.repository=${REPO}" --set "image.tag=${TAG}")

# Build identity baked into the image (ADR-0103): the working-tree SHA (+ -dirty
# for uncommitted edits — the norm for this local path), so /version and the
# X-App-Version header report exactly what you deployed.
REV="$(git rev-parse --short=12 HEAD 2>/dev/null || echo unknown)"
git diff --quiet 2>/dev/null || REV="${REV}-dirty"

echo "→ building ${IMAGE}"
if [ "$KIND" = service ]; then
  docker build -t "${IMAGE}:${TAG}" \
    --build-arg SERVICE="${SVC}" --build-arg APP_CMD=server \
    --build-arg "GIT_SHA=${REV}" --build-arg BUILD_VERSION=local \
    --build-arg "BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -f "${SVC_DIR}/Dockerfile" .
else
  # The app's Dockerfile takes its own build identity (SERVICE_VERSION) and, unlike
  # a Go binary, bakes one env-specific value: `output: "standalone"` freezes
  # next.config into server.js, so the server-action CSRF allowlist is decided at
  # BUILD time. Prod builds leave it empty and rely on same-origin (ADR-0306); this
  # image is local-only and pinned to one host anyway, so it is passed here from the
  # same values file the pod reads it from at runtime — one origin, one source.
  docker build -t "${IMAGE}:${TAG}" \
    --build-arg "SERVICE_VERSION=${REV}" \
    --build-arg "EDGE_PUBLIC_ORIGIN=$(yq -r '.env.EDGE_PUBLIC_ORIGIN // ""' "$VALUES")" \
    -f "${SVC_DIR}/Dockerfile" .
fi
publish_image "${IMAGE}:${TAG}"

# Build the worker too when this service declares one (orders, payment).
if grep -qE '^\s*enabled:\s*true' <(awk '/^worker:/{f=1} f' "$VALUES"); then
  echo "→ building ${SVC}-worker"
  docker build -t "${SVC}-worker:${TAG}" \
    --build-arg SERVICE="${SVC}" --build-arg APP_CMD=worker \
    --build-arg "GIT_SHA=${REV}" --build-arg BUILD_VERSION=local \
    --build-arg "BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -f "${SVC_DIR}/Dockerfile" .
  publish_image "${SVC}-worker:${TAG}"
  SET+=(--set "worker.image.repository=${WORKER_REPO}" --set "worker.image.tag=${TAG}")
fi

# Pause Argo auto-sync on this service if the full tier manages it.
APP="$(argo_service_app "$SVC")"
if [ -n "$APP" ]; then
  echo "→ pausing ArgoCD auto-sync on ${APP}"
  k -n argocd patch application.argoproj.io "$APP" --type merge \
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

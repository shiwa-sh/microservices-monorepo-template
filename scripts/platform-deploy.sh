#!/usr/bin/env bash
# One-shot working-tree overlay of a platform chart (ADR-0205) — the rare local
# infra-iteration case (e.g. changing Ory or the observability chart and testing
# before pushing). Pauses ArgoCD auto-sync on that one app so self-heal does not
# revert you, then helm-upgrades the chart from the working tree with the local
# values overlay. Re-enable sync when done (or just re-run cluster:full).
#
#   mise run platform:deploy -- <chart>     # e.g. ory, observability, openfga
#
# For GitOps-wiring changes (sync-waves, ApplicationSets, App defs) helm cannot
# exercise the delivery path — push a branch and point the local root-app
# targetRevision at it instead. For CNI/CRD changes (Cilium) prefer cluster:delete
# + a fresh cluster:full over an in-place upgrade.
set -euo pipefail

CLUSTER="${CLUSTER:-platform}"
source "$(dirname "$0")/lib/cluster-ctx.sh"
NS="platform"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

CHART="${1:?usage: mise run platform:deploy -- <chart>}"
CHART_DIR="infra/helm/platform/${CHART}"
[ -d "$CHART_DIR" ] || {
  echo "✗ no such platform chart: ${CHART_DIR}" >&2
  exit 1
}

k() { kubectl --context "$(cluster_ctx)" "$@"; }
h() { helm --kube-context "$(cluster_ctx)" "$@"; }

# The lowdefy admin console (ADR-0401) is the one platform chart whose image we
# build: `lowdefy build` bakes the YAML pages (apps/admin, incl. _generated/) into
# the image, so a chart/values change alone is not enough — rebuild + push the
# image, then roll the pod (the local overlay pins :local, pullPolicy Always, so a
# restart re-pulls). Same one-command story as any other platform chart, plus the
# image step this one needs.
if [ "$CHART" = "lowdefy" ]; then
  REG="registry.localhost:5000"
  echo "→ regenerating admin pages + rebuilding the admin image (${REG}/admin:local)"
  bash scripts/gen-admin.sh
  docker build -t "${REG}/admin:local" -f apps/admin/Dockerfile apps/admin
  docker push "${REG}/admin:local"
fi

APP="local-platform-${CHART}"
if k -n argocd get application.argoproj.io "$APP" >/dev/null 2>&1; then
  echo "→ pausing ArgoCD auto-sync on ${APP}"
  k -n argocd patch application.argoproj.io "$APP" --type merge \
    -p '{"spec":{"syncPolicy":{"automated":null}}}'
fi

# Mirror what the platform ApplicationSet supplies, or the working-tree overlay
# installs a chart missing config ArgoCD would have provided. Keep in sync with the
# `valueFiles` + `fileParameters` blocks in
# infra/gitops/{bootstrap,local-bootstrap}/appset-platform.yaml.
#
# The appset applies the two auth value files to EVERY chart in the tier (charts
# that don't consume them ignore the extra keys), so do the same rather than
# special-casing: the failure mode of omitting them is silent and severe. Ory is
# the chart that actually needs them — without kratos.config.identity.schemas
# Kratos CrashLoops on `missing properties: "schemas"`, and an Oathkeeper with
# empty accessRules stops gating every ops origin while still reporting Ready.
extra_args=(
  -f infra/auth/kratos/values.yaml
  -f infra/auth/oathkeeper/values.yaml
)
case "$CHART" in
openfga)
  # Without this seed.model is empty and the seed Job is disabled (no store is
  # created) — ADR-0304.
  extra_args+=(--set-file "seed.model=infra/auth/openfga/model.json")
  ;;
ory)
  extra_args+=(
    --set-file 'kratos.kratos.identitySchemas.user\.v1\.json=infra/auth/kratos/identity-schemas/user.v1.json'
    --set-file 'oathkeeper.oathkeeper.accessRules=infra/auth/oathkeeper/access-rules.json'
  )
  ;;
esac

echo "→ helm upgrade ${CHART} from the working tree"
h dependency update "$CHART_DIR" >/dev/null
# --take-ownership: platform charts are normally owned by ArgoCD (Server-Side
# Apply), not a Helm release; Helm 4 refuses to adopt them without this flag. Sync
# is paused above, so this is safe for the local override; cluster:full restores it.
# Value-file order matches the ApplicationSet: the auth overlays first, the
# per-env overlay last so it wins. extra_args therefore precedes the local values.
h upgrade --install "$CHART" "$CHART_DIR" -n "$NS" \
  --take-ownership --force-conflicts "${extra_args[@]}" \
  -f infra/gitops/platform/local/values.yaml --timeout 8m

# lowdefy's image tag is stable (:local), so helm sees no change to trigger a
# rollout; restart explicitly to re-pull the image just rebuilt above.
if [ "$CHART" = "lowdefy" ]; then
  echo "→ restarting lowdefy to re-pull the rebuilt image"
  k -n "$NS" rollout restart deploy/lowdefy
  k -n "$NS" rollout status deploy/lowdefy --timeout=180s
fi
echo "✓ ${CHART} overlaid from working tree."
echo "  Re-enable GitOps when done:"
echo "    kubectl -n argocd patch application.argoproj.io ${APP} --type merge \\"
echo "      -p '{\"spec\":{\"syncPolicy\":{\"automated\":{\"prune\":true,\"selfHeal\":true}}}}'"

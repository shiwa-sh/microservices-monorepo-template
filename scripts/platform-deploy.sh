#!/usr/bin/env bash
# One-shot working-tree overlay of a platform chart (ADR-0016) — the rare local
# infra-iteration case (e.g. changing Ory or the observability chart and testing
# before pushing). Pauses ArgoCD auto-sync on that one app so self-heal does not
# revert you, then helm-upgrades the chart from the working tree with the local
# values overlay. Re-enable sync when done (or just re-run cluster:full).
#
#   mise run platform:deploy -- <chart>     # e.g. ory, observability, spicedb
#
# For GitOps-wiring changes (sync-waves, ApplicationSets, App defs) helm cannot
# exercise the delivery path — push a branch and point the local root-app
# targetRevision at it instead. For CNI/CRD changes (Cilium) prefer cluster:delete
# + a fresh cluster:full over an in-place upgrade.
set -euo pipefail

CLUSTER="${CLUSTER:-platform}"
NS="platform"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

CHART="${1:?usage: mise run platform:deploy -- <chart>}"
CHART_DIR="infra/helm/platform/${CHART}"
[ -d "$CHART_DIR" ] || { echo "✗ no such platform chart: ${CHART_DIR}" >&2; exit 1; }

k() { kubectl --context "k3d-${CLUSTER}" "$@"; }
h() { helm --kube-context "k3d-${CLUSTER}" "$@"; }

APP="local-platform-${CHART}"
if k -n argocd get application.argoproj.io "$APP" >/dev/null 2>&1; then
  echo "→ pausing ArgoCD auto-sync on ${APP}"
  k -n argocd patch application.argoproj.io "$APP" --type merge \
    -p '{"spec":{"syncPolicy":{"automated":null}}}'
fi

echo "→ helm upgrade ${CHART} from the working tree"
h dependency update "$CHART_DIR" >/dev/null
h upgrade --install "$CHART" "$CHART_DIR" -n "$NS" \
  -f infra/gitops/platform/local/values.yaml --timeout 8m
echo "✓ ${CHART} overlaid from working tree."
echo "  Re-enable GitOps when done:"
echo "    kubectl -n argocd patch application.argoproj.io ${APP} --type merge \\"
echo "      -p '{\"spec\":{\"syncPolicy\":{\"automated\":{\"prune\":true,\"selfHeal\":true}}}}'"

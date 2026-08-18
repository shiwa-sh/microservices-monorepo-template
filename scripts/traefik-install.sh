#!/usr/bin/env bash
# Install Traefik — the edge controller every environment runs (ADR-0305) — from
# the committed chart infra/helm/platform/traefik. kind ships no ingress
# controller, so both local tiers install this chart: it is the one edge
# configuration in the repo. The IngressRoute/Middleware CRDs it ships must exist
# before the gateway Application applies any route, which is why this runs
# imperatively (cluster:base, and cluster:full before the root app) rather than
# waiting on Argo. Idempotent (helm upgrade --install).
set -euo pipefail

source "$(dirname "$0")/lib/cluster-ctx.sh"

CLUSTER="${CLUSTER:-platform}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

k() { kubectl --context "$(cluster_ctx)" "$@"; }
h() { helm --kube-context "$(cluster_ctx)" "$@"; }

# Already up? (helm release present and the controller ready) → nothing to do.
if h -n kube-system status traefik >/dev/null 2>&1 &&
  k -n kube-system rollout status deploy/traefik --timeout=0 >/dev/null 2>&1; then
  echo "✓ Traefik already installed and ready"
  exit 0
fi

echo "→ installing Traefik (edge controller) from infra/helm/platform/traefik"
helm dependency update infra/helm/platform/traefik >/dev/null
# The chart's own values.yaml carries the local NodePort mapping. No gitops overlay
# applies: traefik is imperative-only, absent from the platform ApplicationSet, so
# there is no per-environment values file for it to coalesce with.
h upgrade --install traefik infra/helm/platform/traefik -n kube-system --timeout 5m
k -n kube-system rollout status deploy/traefik --timeout=300s
echo "✓ Traefik installed; CRDs registered"

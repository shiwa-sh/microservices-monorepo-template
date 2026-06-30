#!/usr/bin/env bash
# Install Cilium as the CNI on the local cluster (ADR-0003). A CNI must exist
# before any pod (including ArgoCD) can schedule, so this runs imperatively for
# both local tiers and is excluded from the local platform ApplicationSet. k3d
# keeps kube-proxy, so kubeProxyReplacement is off; one operator replica (the
# default 2 needs 2 nodes for anti-affinity). Idempotent (helm upgrade --install).
set -euo pipefail

CLUSTER="${CLUSTER:-platform}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

k() { kubectl --context "k3d-${CLUSTER}" "$@"; }
h() { helm --kube-context "k3d-${CLUSTER}" "$@"; }

# Already up? (helm release present and node Ready) → nothing to do.
if h -n kube-system status cilium >/dev/null 2>&1 \
   && k get node -o jsonpath='{.items[0].status.conditions[?(@.type=="Ready")].status}' 2>/dev/null | grep -q True; then
  echo "✓ Cilium already installed and node Ready"
  exit 0
fi

SERVER_IP="$(k get node -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')"
echo "→ installing Cilium (apiserver ${SERVER_IP}:6443)"
helm dependency update infra/helm/platform/cilium >/dev/null

h upgrade --install cilium infra/helm/platform/cilium -n kube-system \
  --set cilium.kubeProxyReplacement=false \
  --set cilium.k8sServiceHost="${SERVER_IP}" \
  --set cilium.k8sServicePort=6443 \
  --set cilium.operator.replicas=1 \
  --timeout 5m

echo "→ waiting for the node to go Ready (Cilium up)…"
k wait --for=condition=Ready node --all --timeout=600s
echo "✓ Cilium installed; node Ready"

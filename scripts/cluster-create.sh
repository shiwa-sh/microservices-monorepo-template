#!/usr/bin/env bash
# Create (or resume) the single local k3d cluster (ADR-0003, ADR-0016). One
# cluster serves both local tiers; what differs is what you bring up on it:
#   mise run cluster:up     → inner loop (lightweight deps, run services natively)
#   mise run cluster:full   → full platform via ArgoCD
#
# Flannel + the built-in network policy are disabled because the full tier runs
# Cilium as the CNI (NetworkPolicy + Hubble, ADR-0003); the inner loop installs a
# minimal Cilium too so there is always a CNI. Traefik stays (it provides the
# IngressRoute/Middleware CRDs the edge uses). Ports 8080/8443 map the loadbalancer.
set -euo pipefail

CLUSTER="${CLUSTER:-platform}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

# Behind a corporate/loopback HTTP proxy, configure Docker (and thus the k3d nodes)
# at the environment level — not here. See docs/dev-loop.md ("HTTP proxies").

if ! k3d cluster list 2>/dev/null | awk '{print $1}' | grep -qx "$CLUSTER"; then
  echo "→ creating k3d cluster '$CLUSTER'"
  # Traefik (k3s-bundled) stays: the full tier's edge needs its IngressRoute /
  # Middleware CRDs (the inner loop simply ignores it). Flannel + built-in netpol
  # are disabled because Cilium is the CNI.
  k3d cluster create "$CLUSTER" \
    --servers 1 --agents 0 \
    --port "8080:80@loadbalancer" --port "8443:443@loadbalancer" \
    --k3s-arg '--flannel-backend=none@server:*' \
    --k3s-arg '--disable-network-policy@server:*' \
    --k3s-arg '--kubelet-arg=eviction-hard=imagefs.available<5%,nodefs.available<5%@server:*' \
    "${PROXY_ARGS[@]}"
else
  # `cluster:down` only STOPS the cluster (keeping the image cache + volumes), so
  # resume it here — a no-op if already running. `cluster:purge` deletes it.
  echo "→ cluster '$CLUSTER' exists; ensuring it is started"
  k3d cluster start "$CLUSTER" || true
fi
kubectl config use-context "k3d-${CLUSTER}"

echo "✓ cluster '$CLUSTER' ready (context k3d-${CLUSTER})"

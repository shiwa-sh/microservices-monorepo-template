#!/usr/bin/env bash
# Ensure the single local k3d cluster exists and is running (ADR-0003, ADR-0016).
# Convergent: create it if absent, start it if stopped (cluster:stop keeps the
# image cache + volumes; cluster:delete deletes it). One cluster serves both local
# tiers; what differs is what you bring up on it:
#   mise run cluster:lite     → inner loop (lightweight deps, run services natively)
#   mise run cluster:full   → full platform via ArgoCD
#
# Flannel + the built-in network policy are disabled because Cilium is the CNI
# (NetworkPolicy + Hubble, ADR-0003). Traefik stays (it provides the IngressRoute/
# Middleware CRDs the edge uses). Ports 8080/8443 map the loadbalancer.
#
# Behind a corporate/loopback HTTP proxy, configure Docker (and thus the k3d nodes)
# at the environment level — not here. See docs/dev-loop.md ("HTTP proxies").
set -euo pipefail

CLUSTER="${CLUSTER:-platform}"

if ! k3d cluster list | awk '{print $1}' | grep -qx "$CLUSTER"; then
  echo "→ creating k3d cluster '$CLUSTER'"
  k3d cluster create "$CLUSTER" \
    --servers 1 --agents 0 \
    --port "8080:80@loadbalancer" --port "8443:443@loadbalancer" \
    --k3s-arg '--flannel-backend=none@server:*' \
    --k3s-arg '--disable-network-policy@server:*'
else
  echo "→ cluster '$CLUSTER' exists; starting it (no-op if already running)"
  k3d cluster start "$CLUSTER"
fi

kubectl config use-context "k3d-${CLUSTER}"
echo "✓ cluster '$CLUSTER' ready (context k3d-${CLUSTER})"

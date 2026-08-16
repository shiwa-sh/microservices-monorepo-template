#!/usr/bin/env bash
# Heal a local kind cluster after a host reboot (ADR-0200).
#
# Docker's restart policy on the kind node is `unless-stopped`, so on daemon start
# (after a reboot) Docker replays the node container raw — the container comes back
# and its processes resume, but two things can be stale:
#
#   1. The docker-bridge gateway IP can change across a daemon restart. The host
#      edge glue (scripts/cluster-edge-glue.sh) points the frontend-dev
#      EndpointSlice at that address — the only host IP a pod can reach — so a
#      changed bridge gives a 502 at / until it is re-stamped.
#   2. Cilium's per-node datapath can be half-restored: pods then cannot egress
#      to the node/API, which cascades to CoreDNS (its `kubernetes` plugin never
#      goes ready) and every controller. A restart of the CNI + DNS rebuilds it.
#
# kind has no stop/start command of its own; docker stop+start on the node is the
# restart that re-runs the node's entrypoint cleanly. Idempotent and safe to run
# anytime the cluster looks wedged after a reboot. See docs/dev-loop.md
# ("After a reboot").
set -euo pipefail

source "$(dirname "$0")/lib/cluster-ctx.sh"

CLUSTER="${CLUSTER:-platform}"
node="${CLUSTER}-control-plane"
k() { kubectl --context "$(cluster_ctx)" "$@"; }

if ! docker inspect "$node" >/dev/null 2>&1; then
  echo "cluster '$CLUSTER' not found — nothing to heal (run 'mise run cluster:full')"
  exit 0
fi

echo "→ healing '$CLUSTER': restart the node container"
docker restart "$node" >/dev/null

# The API server takes a while to come back after the container restarts; wait on
# the node before touching anything.
kubectl --context "$(cluster_ctx)" wait --for=condition=Ready node --all --timeout=300s

# Rebuild the CNI + DNS datapath, then PROVE recovery via CoreDNS readiness —
# CoreDNS is only Ready once a pod can actually reach the API, so it is the honest
# datapath probe. Fail loudly if it does not come back (no silent half-heal).
echo "→ rebuilding Cilium + CoreDNS datapath"
k -n kube-system rollout restart ds/cilium >/dev/null 2>&1 || true
k -n kube-system rollout status ds/cilium --timeout=180s || true
k -n kube-system rollout restart deploy/coredns >/dev/null 2>&1 || true

echo "→ verifying pod→API datapath (CoreDNS readiness)…"
if k -n kube-system rollout status deploy/coredns --timeout=180s; then
  # A reboot's raw node restart can change the docker-bridge gateway IP, so re-stamp
  # the host-only frontend edge glue (route + fresh host address) as part of healing.
  CLUSTER="$CLUSTER" bash scripts/cluster-edge-glue.sh
  echo "✓ heal complete: CoreDNS ready — pods can reach the API again"
else
  echo "✗ CoreDNS still not ready after heal — the Cilium pod-egress datapath is" >&2
  echo "  wedged deeper than a restart clears (pods cannot reach the API). This" >&2
  echo "  cluster's CNI state is corrupt; recreate it: mise run cluster:delete && \\" >&2
  echo "  CLUSTER_PRELOAD=1 mise run cluster:full" >&2
  exit 1
fi

#!/usr/bin/env bash
# Stop the full local platform created by scripts/cluster-full.sh — WITHOUT
# destroying it. `k3d cluster stop` halts the node container but keeps it, so the
# node's containerd image cache and volumes survive. `mise run cluster:full:up`
# then resumes it (via `k3d cluster start`) with no cold image re-pulls — which
# matters a lot behind a slow/restricted egress proxy.
#
# To fully delete the cluster (free disk, force a clean from-scratch rebuild,
# re-derive the node proxy env), use: mise run cluster:full:purge
set -euo pipefail
CLUSTER="platform-full"

if k3d cluster list 2>/dev/null | awk '{print $1}' | grep -qx "$CLUSTER"; then
  echo "→ stopping k3d cluster '$CLUSTER' (image cache preserved)"
  k3d cluster stop "$CLUSTER"
  echo "✓ cluster:full:down complete — resume with 'mise run cluster:full:up',"
  echo "  or delete entirely with 'mise run cluster:full:purge'."
else
  echo "cluster '$CLUSTER' not found — nothing to do"
fi

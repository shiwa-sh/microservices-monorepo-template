#!/usr/bin/env bash
# Stop the local k3d cluster (ADR-0003) — WITHOUT destroying it. `k3d cluster stop`
# halts the node container but keeps it, so the node's containerd image cache and
# volumes survive; the next `cluster:up`/`cluster:full` resumes it with no cold
# image re-pulls — which matters a lot behind a slow/restricted egress proxy. To
# delete the cluster entirely (reclaim disk, clean recreate), use `cluster:purge`.
set -euo pipefail
CLUSTER="${CLUSTER:-platform}"

if k3d cluster list 2>/dev/null | awk '{print $1}' | grep -qx "$CLUSTER"; then
  echo "→ stopping k3d cluster '$CLUSTER' (image cache preserved)"
  k3d cluster stop "$CLUSTER"
  echo "✓ cluster:down complete — resume with 'mise run cluster:up' or 'cluster:full'."
else
  echo "cluster '$CLUSTER' not found — nothing to do"
fi

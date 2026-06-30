#!/usr/bin/env bash
# Stop the local k3d cluster (ADR-0003) — WITHOUT destroying it. `k3d cluster stop`
# halts the node container but keeps it, so the node's containerd image cache and
# volumes survive; the next `cluster:lite`/`cluster:full` resumes it with no cold
# image re-pulls — which matters a lot behind a slow/restricted egress proxy. To
# delete the cluster entirely (reclaim disk, clean recreate), use `cluster:delete`.
set -euo pipefail
CLUSTER="${CLUSTER:-platform}"

if k3d cluster list | awk '{print $1}' | grep -qx "$CLUSTER"; then
  echo "→ stopping k3d cluster '$CLUSTER' (image cache preserved)"
  k3d cluster stop "$CLUSTER"
  echo "✓ cluster:stop complete — resume with 'mise run cluster:lite' or 'cluster:full'."
else
  echo "cluster '$CLUSTER' not found — nothing to do"
fi

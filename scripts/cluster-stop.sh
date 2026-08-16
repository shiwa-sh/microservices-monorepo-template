#!/usr/bin/env bash
# Stop the local kind cluster (ADR-0200) — WITHOUT destroying it. `docker stop` on
# the node container halts it but keeps it, so the node's containerd image cache
# and volumes survive; the next `cluster:base`/`cluster:full` resumes it with no
# cold image re-pulls — which matters a lot behind a slow/restricted egress proxy.
# kind has no stop command of its own, so this talks to docker directly. To delete
# the cluster entirely (reclaim disk, clean recreate), use `cluster:delete`.
set -euo pipefail
CLUSTER="${CLUSTER:-platform}"
node="${CLUSTER}-control-plane"

if docker inspect "$node" >/dev/null 2>&1; then
  echo "→ stopping kind cluster '$CLUSTER' (image cache preserved)"
  docker stop "$node" >/dev/null
  echo "✓ cluster:stop complete — resume with 'mise run cluster:base' or 'cluster:full'."
else
  echo "cluster '$CLUSTER' not found — nothing to do"
fi

#!/usr/bin/env bash
# Stop the ACTIVE TIER's kind cluster (ADR-0600) — WITHOUT destroying it.
#
# `docker stop` on the node containers halts them but keeps them, so each node's
# containerd image cache and its volumes survive; the next `cluster:base` /
# `cluster:full` resumes them with no cold image re-pulls — which matters a lot
# behind a slow or restricted egress proxy. kind has no stop command of its own, so
# this talks to docker directly. To delete the cluster entirely, use `cluster:delete`.
#
# EVERY node, not just the control plane: the full tier is three, and stopping one
# of them leaves two containers holding memory for a cluster nobody is using.
set -euo pipefail

source "$(dirname "$0")/lib/log.sh"
source "$(dirname "$0")/lib/cluster-ctx.sh"

NAME="$(cluster_name)"

if ! kind get clusters 2>/dev/null | grep -qx "$NAME"; then
  echo "cluster '${NAME}' not found — nothing to do"
  exit 0
fi

mapfile -t nodes < <(kind get nodes --name "$NAME")
step "stopping kind cluster '${NAME}' — ${#nodes[@]} node(s), image cache preserved"
printf '%s\n' "${nodes[@]}" | xargs -r docker stop >/dev/null
ok "cluster:stop complete — resume with 'mise run cluster:base' or 'cluster:full'."

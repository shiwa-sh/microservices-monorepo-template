#!/usr/bin/env bash
# DELETE the full local platform created by scripts/cluster-full.sh. Unlike
# `cluster:full:down` (which only stops the cluster, keeping its image cache),
# this destroys the node container and its volumes — reclaiming disk and forcing
# the next `cluster:full:up` to recreate everything and re-pull every image cold.
# Use this when you want a clean from-scratch rebuild or to free space; otherwise
# prefer `cluster:full:down` so you don't re-pull the platform over the proxy.
set -euo pipefail
CLUSTER="platform-full"

if k3d cluster list 2>/dev/null | awk '{print $1}' | grep -qx "$CLUSTER"; then
  echo "→ deleting k3d cluster '$CLUSTER' (image cache will be wiped)"
  k3d cluster delete "$CLUSTER"
  echo "✓ cluster:full:purge complete"
else
  echo "cluster '$CLUSTER' not found — nothing to do"
fi

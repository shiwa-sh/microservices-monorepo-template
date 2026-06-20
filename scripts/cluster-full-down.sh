#!/usr/bin/env bash
# Tear down the full local platform created by scripts/cluster-full.sh.
set -euo pipefail
CLUSTER="platform-full"

if k3d cluster list 2>/dev/null | awk '{print $1}' | grep -qx "$CLUSTER"; then
  echo "→ deleting k3d cluster '$CLUSTER'"
  k3d cluster delete "$CLUSTER"
else
  echo "cluster '$CLUSTER' not found — nothing to do"
fi

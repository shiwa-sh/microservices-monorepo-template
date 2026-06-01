#!/usr/bin/env bash
# Tear down the local k3d cluster (ADR-0003). Stop `skaffold dev` first (Ctrl-C)
# to remove the deployed services; this deletes the whole cluster regardless.
set -euo pipefail

k3d cluster delete platform-dev || true
echo "✓ dev:down complete"

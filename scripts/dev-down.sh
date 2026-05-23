#!/usr/bin/env bash
# Tear down local k3d cluster and port-forwards.
set -euo pipefail

bash scripts/dev-portforward.sh down || true
k3d cluster delete platform-dev || true
echo "✓ dev:down complete"

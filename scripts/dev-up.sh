#!/usr/bin/env bash
# Bring up the local k3d cluster and reconcile platform charts (ADR-0003).
# Two profiles: full (default) and --minimal.
set -euo pipefail

PROFILE="full"
for arg in "$@"; do
  if [[ "$arg" == "--minimal" ]]; then PROFILE="minimal"; fi
done

CLUSTER="platform-dev"
VALUES="infra/helm/values/local-${PROFILE}.yaml"

if ! k3d cluster list | awk '{print $1}' | grep -qx "$CLUSTER"; then
  echo "→ creating k3d cluster '$CLUSTER'"
  k3d cluster create "$CLUSTER" \
    --servers 1 --agents 0 \
    --port "80:80@loadbalancer" --port "443:443@loadbalancer" \
    --k3s-arg "--disable=traefik@server:0"
fi

kubectl config use-context "k3d-${CLUSTER}"

echo "→ installing platform charts (profile: $PROFILE)"
helm dependency build infra/helm/platform/_umbrella 2>/dev/null || true
helm upgrade --install platform infra/helm/platform/_umbrella \
  --values "$VALUES" \
  --namespace platform --create-namespace \
  --wait --timeout 5m

echo "→ establishing port-forwards"
bash scripts/dev-portforward.sh up

echo "✓ dev:up complete ($PROFILE)"

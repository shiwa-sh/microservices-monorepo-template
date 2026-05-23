#!/usr/bin/env bash
# Build a service image and load it into the local k3d cluster (ADR-0003).
# Usage: mise run dev:build <service>
set -euo pipefail

SERVICE="${1:?usage: dev-build.sh <service>}"
DOCKERFILE="services/${SERVICE}/Dockerfile"

if [[ ! -f "$DOCKERFILE" ]]; then
  echo "no Dockerfile at $DOCKERFILE" >&2
  exit 1
fi

SHA=$(git rev-parse --short HEAD)

for cmd in server worker; do
  TAG="${SERVICE}-${cmd}:${SHA}"
  echo "→ building $TAG"
  docker build -f "$DOCKERFILE" --build-arg "CMD=${cmd}" -t "$TAG" .
  k3d image import "$TAG" -c platform-dev
done

kubectl rollout restart "deployment/${SERVICE}-server" 2>/dev/null || true
kubectl rollout restart "deployment/${SERVICE}-worker" 2>/dev/null || true
echo "✓ ${SERVICE} rolled out"

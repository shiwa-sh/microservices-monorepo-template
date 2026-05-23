#!/usr/bin/env bash
# Scaffold a new service from services/_template/ (ADR-0002).
# Usage: scripts/new-service.sh <name>
set -euo pipefail

NAME="${1:?usage: $0 <service-name>}"
DEST="services/${NAME}"

if [[ -d "$DEST" ]]; then
  echo "✗ ${DEST} already exists" >&2
  exit 1
fi

cp -r services/_template "$DEST"

# Strip the //go:build _template constraint from every Go file.
find "$DEST" -name "*.go" -print0 | while IFS= read -r -d '' f; do
  # Remove the build constraint line and the blank line that follows it.
  sed -i '/^\/\/go:build _template$/{N;d;}' "$f"
done

# Substitute the service name in obvious places.
find "$DEST" -type f \( -name "*.go" -o -name "*.yaml" -o -name "*.md" -o -name "Dockerfile" -o -name "*.toml" \) \
  -exec sed -i "s/_template/${NAME}/g" {} +

# Stamp the init migration with the current timestamp.
ts=$(date -u +%Y%m%d%H%M%S)
mv "${DEST}/migrations/"*_init.sql "${DEST}/migrations/${ts}_init.sql"

echo "✓ created ${DEST}. Next:"
echo "    1. Edit ${DEST}/openapi.yaml — define your routes"
echo "    2. mise run gen:all"
echo "    3. Implement handlers/ and wire them in cmd/server/main.go"
echo "    4. Add infra/gitops/services/dev/values/${NAME}.yaml so ArgoCD picks it up"

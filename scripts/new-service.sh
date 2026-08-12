#!/usr/bin/env bash
# Scaffold a new service from services/_template/ (ADR-0101).
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
echo "    1. Register a local port in scripts/lib/ports.sh, and set the same PORT in"
echo "       ${DEST}/.mise.toml (it ships 80XX and will fail lint until you do)"
echo "    2. Edit ${DEST}/openapi.yaml — define your routes"
echo "    3. mise run gen"
echo "    4. Implement handlers/ and wire them in cmd/server/main.go"
echo "    5. Trim dep:* to what you actually read, and add svc:* for every service"
echo "       you call over HTTP — an undeclared callee fails at runtime, not startup"
echo "    6. Add infra/gitops/services/<env>/values/${NAME}.yaml for EVERY env —"
echo "       the ApplicationSet generates one Argo app per values file, so a missing"
echo "       one means you are silently absent from that environment"
echo ""
echo "  Then: mise run lint:service-contract   # checks all of the above (ADR-0205)"

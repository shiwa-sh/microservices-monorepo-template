#!/usr/bin/env bash
# Lint every OpenAPI spec under services/ and api/shared/ (ADR-0008).
set -euo pipefail

shopt -s nullglob globstar

specs=()
for f in services/*/openapi.yaml api/shared/**/*.yaml; do
  [[ -f "$f" ]] && specs+=("$f")
done

if [[ ${#specs[@]} -eq 0 ]]; then
  echo "no OpenAPI specs yet"
  exit 0
fi

exec spectral lint --ruleset tools/codegen/spectral.yaml "${specs[@]}"

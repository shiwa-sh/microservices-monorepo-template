#!/usr/bin/env bash
# Lint every OpenAPI spec under services/ (ADR-0008). Each spec is fully
# self-contained: shared shapes (the Error response, the workflow handle) are
# declared in each spec's own components rather than via cross-file $refs, to
# keep specs portable across the codegen (ogen) and linting (vacuum) tools.
set -euo pipefail

shopt -s nullglob globstar

specs=()
for f in services/*/openapi.yaml; do
  [[ -f "$f" ]] && specs+=("$f")
done

if [[ ${#specs[@]} -eq 0 ]]; then
  echo "no OpenAPI specs yet"
  exit 0
fi

exec vacuum lint --ruleset tools/codegen/openapi-ruleset.yaml --fail-severity error "${specs[@]}"

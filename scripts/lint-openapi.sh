#!/usr/bin/env bash
# Lint every OpenAPI spec under services/ (ADR-0303). Each spec is fully
# self-contained: shared shapes (the Error response, the workflow handle) are
# declared in each spec's own components rather than via cross-file $refs, to
# keep specs portable across the codegen (ogen) and linting (vacuum) tools.
set -euo pipefail
source "$(dirname "$0")/lib/log.sh"

shopt -s nullglob globstar

specs=()
for f in services/*/openapi.yaml; do
  [[ -f "$f" ]] && specs+=("$f")
done

if [[ ${#specs[@]} -eq 0 ]]; then
  ok "no OpenAPI specs yet"
  exit 0
fi

step "linting ${#specs[@]} OpenAPI spec(s) with vacuum"
vacuum lint --ruleset tools/codegen/openapi-ruleset.yaml --fail-severity error "${specs[@]}"

# Resource-prefix ownership is the one rule vacuum cannot express: it is a property
# of the SET of specs, and vacuum lints one document at a time (ADR-0303).
step "checking resource-prefix ownership across the /api namespace"
go run ./tools/lint-api-prefixes

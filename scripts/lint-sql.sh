#!/usr/bin/env bash
# Run sqruff across all service migrations and sqlc queries (ADR-0300).
set -euo pipefail
source "$(dirname "$0")/lib/log.sh"

shopt -s nullglob globstar

targets=()
for d in services/*/migrations services/*/internal/store/queries; do
  [[ -d "$d" ]] && targets+=("$d")
done

if [[ ${#targets[@]} -eq 0 ]]; then
  ok "no SQL targets to lint (yet)"
  exit 0
fi

step "linting ${#targets[@]} SQL target(s) with sqruff"
sqruff lint "${targets[@]}"

# PII tagging (ADR-0301): a column holding personal data carries its pii:<class>
# comment in the DDL, so erasure, export, and redaction enumerate their targets by
# query rather than by memory.
step "checking personal-data columns are tagged"
go run ./tools/lint-pii

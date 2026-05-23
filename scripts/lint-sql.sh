#!/usr/bin/env bash
# Run sqlfluff across all service migrations and sqlc queries (ADR-0007).
set -euo pipefail

shopt -s nullglob globstar

targets=()
for d in services/*/migrations services/*/internal/store/queries; do
  [[ -d "$d" ]] && targets+=("$d")
done

if [[ ${#targets[@]} -eq 0 ]]; then
  echo "no SQL targets to lint (yet)"
  exit 0
fi

exec sqlfluff lint "${targets[@]}"

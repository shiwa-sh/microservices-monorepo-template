#!/usr/bin/env bash
# Breaking-change DETECTION for the API contracts (ADR-0303). This labels; it does
# not gate. A break is allowed — the point is that it is intentional and visible in
# review rather than discovered by a consumer, so this exits 0 with findings and
# writes them where the pull request can show them.
#
# BASE_REF names what the specs are compared against; it defaults to the
# integration branch, as lint-commits.sh does.
set -euo pipefail
# shellcheck source=lib/log.sh
source "$(dirname "$0")/lib/log.sh"

base="${BASE_REF:-master}"
base="${base#refs/heads/}"

ref=""
for candidate in "origin/${base}" "$base"; do
  if git rev-parse --verify --quiet "${candidate}^{commit}" >/dev/null; then
    ref="$candidate"
    break
  fi
done

if [[ -z "$ref" ]]; then
  warn "no such base ref: ${base} — skipping breaking-change detection"
  exit 0
fi

shopt -s nullglob
specs=(services/*/openapi.yaml)
if [[ ${#specs[@]} -eq 0 ]]; then
  ok "no OpenAPI specs yet"
  exit 0
fi

step "comparing ${#specs[@]} spec(s) against ${ref}"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

breaking=0
for spec in "${specs[@]}"; do
  # A spec that does not exist on the base is new: nothing to break.
  if ! git cat-file -e "${ref}:${spec}" 2>/dev/null; then
    detail "$(dirname "$spec" | xargs basename): new spec, nothing to compare"
    continue
  fi
  old="${work}/$(echo "$spec" | tr / _)"
  git show "${ref}:${spec}" >"$old"

  # JSON rather than the text renderer: "no findings" is an empty array, which is
  # unambiguous, where the text output varies ("No changes detected" when the specs
  # are identical, an empty body when they differ but nothing breaks).
  findings="$(oasdiff breaking "$old" "$spec" --format json 2>/dev/null || true)"
  count="$(printf '%s' "${findings:-[]}" | jq 'length' 2>/dev/null || echo 0)"
  if [[ "$count" -gt 0 ]]; then
    breaking=1
    warn "$(basename "$(dirname "$spec")"): ${count} breaking change(s)"
    # level 3 is oasdiff's ERR; anything lower is a warning it still classifies.
    printf '%s' "$findings" |
      jq -r '.[] | "    \(.operation) \(.path) — \(.text) [\(.id)]"'
  fi
done

if [[ "$breaking" -eq 1 ]]; then
  warn "breaking API changes above — intentional is fine, unnoticed is not (ADR-0303)"
  if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
    printf 'breaking=true\n' >>"$GITHUB_OUTPUT"
  fi
else
  ok "no breaking API changes against ${ref}"
fi

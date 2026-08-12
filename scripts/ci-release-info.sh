#!/usr/bin/env bash
# Resolve a CalVer release tag to its version and commit, as forge step-output
# assignments (ADR-0102, ADR-0103). GITHUB_REF names the tag that triggered the
# run; the `^{}` peel is what turns an annotated tag into the commit it points at.
set -euo pipefail
# shellcheck source=lib/log.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib/log.sh"

ref="${GITHUB_REF:-}"
[[ "$ref" == refs/tags/* ]] || fail "GITHUB_REF is not a tag: ${ref:-<unset>}"

printf 'version=%s\n' "${ref#refs/tags/}"
printf 'sha=%s\n' "$(git rev-parse "${ref}^{}")"

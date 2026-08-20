#!/usr/bin/env bash
# Shell lint gate (ADR-0101). All glue in this repo is bash, so scripts get the
# same static analysis Go/TS do: shellcheck over every tracked *.sh. `-x` follows
# the `source lib/log.sh` includes so the shared vocabulary helpers resolve. This
# is a lint only — formatting lives in `format:shell` (shfmt).
set -euo pipefail
source "$(dirname "$0")/lib/log.sh"
source "$(dirname "$0")/lib/repo-files.sh"
cd "$(cd "$(dirname "$0")/.." && pwd)"

step "shellcheck: linting shell scripts"

# Shared with format:shell (scripts/lib/repo-files.sh), so the linter and the
# formatter cannot act on different sets.
#
# An empty result is a FAILURE, not a clean run. It used to be `ok "no shell
# scripts to lint"`, which is how `act` — running outside a git work tree — passed
# this gate having checked none of the scripts in it.
mapfile -d '' -t files < <(sh_files)
if [[ ${#files[@]} -eq 0 ]]; then
  fail "no shell scripts found via $(sh_source) — this repository has dozens, so the enumeration is broken rather than the tree empty"
fi

# --severity=warning gates on bugs (unquoted expansions, bad tests, unset vars)
# without failing on style/info chatter such as yq single-quote DSL (SC2016) or
# sed-vs-parameter-expansion (SC2001); silence those inline where they are wrong.
shellcheck -x --severity=warning "${files[@]}"

ok "shellcheck clean (${#files[@]} scripts)"

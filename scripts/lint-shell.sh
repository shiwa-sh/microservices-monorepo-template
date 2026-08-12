#!/usr/bin/env bash
# Shell lint gate (ADR-0101). All glue in this repo is bash, so scripts get the
# same static analysis Go/TS do: shellcheck over every tracked *.sh. `-x` follows
# the `source lib/log.sh` includes so the shared vocabulary helpers resolve. This
# is a lint only — formatting lives in `format:shell` (shfmt).
set -euo pipefail
source "$(dirname "$0")/lib/log.sh"
cd "$(cd "$(dirname "$0")/.." && pwd)"

step "shellcheck: linting shell scripts"

# NUL-delimited so paths with spaces survive; git ls-files is the source of truth
# for what is tracked, mapfile keeps it to the exact set.
mapfile -d '' -t files < <(git ls-files -z '*.sh')
if [[ ${#files[@]} -eq 0 ]]; then
  ok "no shell scripts to lint"
  exit 0
fi

# --severity=warning gates on bugs (unquoted expansions, bad tests, unset vars)
# without failing on style/info chatter such as yq single-quote DSL (SC2016) or
# sed-vs-parameter-expansion (SC2001); silence those inline where they are wrong.
shellcheck -x --severity=warning "${files[@]}"

ok "shellcheck clean (${#files[@]} scripts)"

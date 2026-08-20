#!/usr/bin/env bash
# Shell formatter (ADR-0101). All glue in this repo is bash, so scripts get the
# same formatting Go/TS do: shfmt over every shell script, in place. `-i 2` matches
# the repo's 2-space style.
#
# A script rather than a one-line mise task, so it shares its file enumeration with
# `lint:shell` (scripts/lib/sh-files.sh). The two disagreeing is how a repository
# ends up formatted but unlinted.
set -euo pipefail
source "$(dirname "$0")/lib/log.sh"
source "$(dirname "$0")/lib/sh-files.sh"
cd "$(cd "$(dirname "$0")/.." && pwd)"

mapfile -d '' -t files < <(sh_files)
if [[ ${#files[@]} -eq 0 ]]; then
  fail "no shell scripts found via $(sh_source) — this repository has dozens, so the enumeration is broken rather than the tree empty"
fi

shfmt -w -i 2 "${files[@]}"
ok "shfmt formatted ${#files[@]} scripts"

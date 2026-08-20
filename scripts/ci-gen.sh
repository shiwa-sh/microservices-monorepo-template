#!/usr/bin/env bash
# Drift check (ADR-0101, ADR-0303): regenerate AND reformat everything, then fail
# if anything changed.
#
#   mise run ci:gen
#
# Formatting belongs here rather than in `lint` because the linters are analysers
# and none of them rewrites code — a correctly-analysed-but-misformatted file
# passes `mise run lint` cleanly. Only the formatters can tell, and they tell by
# rewriting, which is the shape this check already handles for generated code.
#
# # Why a script, and why content hashing
#
# Drift is "regenerating CHANGES something", which is not the same question as "the
# working tree is dirty", and three earlier versions of this check confused them.
#
#   `git diff --exit-code` compares the worktree against the INDEX, so a partially
#   staged commit reported every unstaged file as drift.
#
#   A before/after snapshot of `git status` plus `git diff HEAD` fixed that and
#   left a subtler version: both read the index and the stash, and lefthook stashes
#   unstaged changes for the duration of a pre-commit hook — so any commit made
#   with unrelated work in the tree failed here, and failed by telling the
#   developer to run the command that, run by hand, reports no drift.
#
#   Hashing file content fixed that, and depended on `git ls-files` — which prints
#   nothing outside a work tree, which is exactly where `act` runs CI. It also used
#   bash process substitution in a task mise runs under `sh`.
#
# So: content, enumerated through scripts/lib/repo-files.sh, in a bash script that
# is not at the mercy of which shell the task runner picks.
set -euo pipefail
source "$(dirname "$0")/lib/log.sh"
source "$(dirname "$0")/lib/repo-files.sh"
cd "$(cd "$(dirname "$0")/.." && pwd)"

snapshot() {
  repo_files | xargs -0 sha256sum 2>/dev/null | LC_ALL=C sort
}

before="$(snapshot)"
if [[ -z "$before" ]]; then
  fail "no files found via $(repo_source) — the enumeration is broken rather than the tree empty"
fi

mise run gen
mise run format

after="$(snapshot)"
if [[ "$after" == "$before" ]]; then
  ok "no drift: generated and formatted artifacts are current"
  exit 0
fi

echo "::error::Generated or formatted artifacts are out of date. Run 'mise run ci:gen' and commit the result."
# The changed paths, which is the actionable half. Printing a full diff here buried
# the drift in every other uncommitted change in the tree.
comm -3 <(printf '%s\n' "$before") <(printf '%s\n' "$after") |
  sed -n 's/^[[:space:]]*[0-9a-f]\{64\}[[:space:]]*//p' | sort -u
exit 1

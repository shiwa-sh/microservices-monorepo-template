#!/usr/bin/env bash
# Decide whether tonight's cluster:up full suite should run, as a forge step-output
# assignment (ADR-0102, ADR-0601). Both nightlies cost a full-platform bring-up,
# and re-running one against a commit that already passed buys nothing: the code
# is identical, so a red is flake or upstream drift and a green is a result already
# held. On a repository that pauses for a few weeks that is every night.
#
# EVENT is the forge event name; anything but the cron is someone explicitly asking
# for the suite. Progress goes to $GITHUB_STEP_SUMMARY when the forge provides it.
set -euo pipefail

summary() {
  [[ -n "${GITHUB_STEP_SUMMARY:-}" ]] && printf '%s\n' "$*" >>"$GITHUB_STEP_SUMMARY"
  :
}
decide() {
  summary "$2"
  printf 'run=%s\n' "$1"
  exit 0
}

[[ "${EVENT:-}" == schedule ]] || decide true "→ ${EVENT:-manual} is an explicit request; running the suite."

# 26h, not 24: cron runs are queued on a shared pool and start late by a variable
# amount (this repo has seen 03:00 fire at 05:52 and 06:09). A 24h window silently
# drops commits into the gap between a late run and the next one.
[[ -z "$(git log --since='26 hours ago' --oneline)" ]] || decide true "→ new commits since the last nightly; running the suite."

# Monthly floor. Code standing still does not mean the world does: base images,
# Helm charts, and registry contents move underneath an unchanged commit, and the
# 1st-of-the-month run is what catches that on a dormant repository. Day-of-month
# rather than a weekday so it stays one run per month however the calendar falls.
[[ "$(date -u +%d)" != 01 ]] || decide true "→ no new commits, but it is the 1st; running the monthly floor."

decide false "✓ no commits in the last 26h — skipping tonight's bring-up."

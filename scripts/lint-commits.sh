#!/usr/bin/env bash
# Conventional Commit gate over a commit RANGE (ADR-0103). The lefthook `commit-msg`
# hook checks one message as it is written; this checks the range a pull request
# proposes, which is the only place a rebase or a cherry-pick can smuggle a bad
# message past the hook.
#
# BASE_REF names the branch the range is measured against. CI sets it to the pull
# request's base; locally it defaults to the integration branch, so the range is
# "what this working branch adds". An empty range passes, which is what a checkout
# sitting on the base itself should do.
set -euo pipefail
# shellcheck source=lib/log.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib/log.sh"

base="${BASE_REF:-master}"
base="${base#refs/heads/}"

# Prefer the remote-tracking ref: in CI the base branch is fetched but not checked
# out, so `master` alone may not resolve.
for candidate in "origin/${base}" "$base"; do
  if git rev-parse --verify --quiet "${candidate}^{commit}" >/dev/null; then
    ref="$candidate"
    break
  fi
done

if [[ -z "${ref:-}" ]]; then
  warn "no such base ref: ${base} — skipping the commit-range check"
  exit 0
fi

range="${ref}..HEAD"
count="$(git rev-list --count "$range")"
if [[ "$count" -eq 0 ]]; then
  ok "no commits ahead of ${ref}"
  exit 0
fi

step "checking ${count} commit message(s) in ${range}"
cog check --ignore-merge-commits "$range"
ok "commit messages conform to Conventional Commits"

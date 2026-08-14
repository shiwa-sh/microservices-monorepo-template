#!/usr/bin/env bash
# Adversarially test affected-detection (ADR-0101, ADR-0601).
#
#   mise run ci:affected-adversarial
#
# `tools/affected` decides which services CI builds, tests and publishes. Everything
# downstream trusts it, and its failure mode is the quiet one: a service it does not
# name is a service nothing ran against, and the pull request goes green. ADR-0101
# calls it the most load-bearing tooling in the repo and mitigates it with unit
# tests — which check the classifier against a list of paths someone wrote down,
# which is the same list the classifier was written from.
#
# So this asks the question from the other side. For every service, in a scratch
# worktree, break that service and assert the manifest names it. The break is a real
# edit to a real file in a real git tree, and the manifest comes from the real
# command CI runs.
#
# The scratch worktree is what makes it safe to run anywhere: nothing touches the
# working tree, and the worktree is removed on the way out, including on failure.
set -euo pipefail

source "$(dirname "$0")/lib/log.sh"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

BASE="${BASE_REF:-HEAD}"

work="$(mktemp -d)"
cleanup() {
  git worktree remove --force "$work/tree" >/dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT

step "creating a scratch worktree at ${BASE}"
git worktree add --quiet --detach "$work/tree" "$BASE"
base_sha="$(git -C "$work/tree" rev-parse HEAD)"

# The classifier under test is the WORKING TREE's, built to a BINARY outside the
# worktree.
#
# Both halves matter. Building from the working tree means the version under test is
# the one being edited, which in CI is the same file and locally is the change you
# are making to the classifier itself. And building it OUT of the worktree means it
# does not appear in the diff being classified — `tools/` is a global trigger, so a
# copy of the tool inside the tree makes every probe come back `global: true`, which
# this check accepts as "selected". It passed a deliberately sabotaged classifier
# exactly once before that was noticed.
step "building the classifier under test"
go build -o "$work/affected" ./tools/affected

fail=0
checked=0

for dir in services/*/; do
  svc="$(basename "$dir")"
  [ "${svc#_}" = "$svc" ] || continue # the scaffold deploys nowhere

  # A file the service actually owns, and a change that is unmistakably its own: a
  # comment at the end of its README. Not a Go file — the point is to test the
  # PATH classifier, and a broken build would confuse the two failures.
  target="${dir}README.md"
  [ -f "$work/tree/$target" ] || {
    warn "${svc}: no README.md to perturb"
    fail=1
    continue
  }

  printf '\n<!-- affected-detection probe -->\n' >>"$work/tree/$target"
  git -C "$work/tree" add "$target" >/dev/null
  # --no-verify: lefthook's commit-msg hook enforces conventional commits, and this
  # commit exists for one `git diff` inside a scratch worktree that is deleted
  # seconds later. Asking it to be a well-formed changelog entry is asking the
  # probe to satisfy a rule about history it never joins.
  git -C "$work/tree" -c user.email=ci@local -c user.name=ci \
    commit --quiet --no-verify -m "chore: perturb ${svc}"

  manifest="$(cd "$work/tree" && "$work/affected" --base "$base_sha")"
  named="$(printf '%s' "$manifest" | jq -r --arg s "$svc" '(.services // []) | index($s) != null')"
  global="$(printf '%s' "$manifest" | jq -r '.global')"

  # `global` counts as naming it: a change classified global rebuilds everything,
  # which is over-selection rather than the under-selection this checks for.
  if [ "$named" = true ] || [ "$global" = true ]; then
    detail "${svc}: selected"
  else
    warn "${svc}: changed and NOT selected — CI would skip it"
    printf '%s\n' "$manifest" >&2
    fail=1
  fi
  checked=$((checked + 1))

  # Back to the base for the next service, so each is tested alone.
  git -C "$work/tree" reset --hard --quiet "$base_sha"
done

if [ "$fail" -ne 0 ]; then
  fail "affected-detection missed a changed service"
fi

ok "affected-detection selected all ${checked} services when each was changed alone"

#!/usr/bin/env bash
# Decide whether a pull request pays for the smoke suite, as a forge step-output
# assignment (ADR-0102, ADR-0601).
#
# The suite was opt-in, by the `e2e-smoke` label. That is the right default for a
# one-service change — the unit and handler tests already cover it, and a cluster
# bring-up is ~17 minutes — and the wrong one for a change that spans services,
# which is precisely the class no test below the e2e level can see. A cross-service
# regression that reaches master then waits for the nightly, which is the largest
# row in docs/reference/detection-latency.md.
#
# So the label still forces the suite on, and a PR touching MORE THAN ONE service
# now gets it without anyone remembering to ask. `tools/affected` already computes
# that set for the publish matrices, so this is a policy over an existing signal
# rather than new detection.
#
# Inputs, both from the workflow: LABELED is whether the `e2e-smoke` label is
# present; BASE_REF is the branch the PR merges into.
set -euo pipefail

cd "$(cd "$(dirname "$0")/.." && pwd)"

summary() {
  [[ -n "${GITHUB_STEP_SUMMARY:-}" ]] && printf '%s\n' "$*" >>"$GITHUB_STEP_SUMMARY"
  :
}
decide() {
  summary "$2"
  printf 'run=%s\n' "$1"
  exit 0
}

[[ "${LABELED:-false}" != true ]] || decide true "→ the \`e2e-smoke\` label is set; running the suite."

manifest="$(mise run ci:affected -- --base "origin/${BASE_REF:-master}")"

# A global change (a shared library, the toolchain, the platform charts) reaches
# every service by definition, so it is the multi-service case at its widest.
[[ "$(jq -r '.global' <<<"$manifest")" != true ]] ||
  decide true "→ a global change; running the suite."

services="$(jq -r '.services | length' <<<"$manifest")"
[[ "$services" -le 1 ]] ||
  decide true "→ ${services} services affected; a cross-service change is what only an e2e can see."

decide false "✓ ${services} service(s) affected and no label — the smoke suite is skipped."

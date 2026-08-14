#!/usr/bin/env bash
# The vulnerability merge gate (ADR-0104, ADR-0106).
#
#   mise run ci:scan
#
# Where this can live. Neither the registry nor the cluster scans (ADR-0105), and
# a registry-side scanner runs after the push, which is after the merge this gates.
# So the gate is here, on the source, before anything is built or promoted.
#
# What it scans, and what it does not:
#
#   vuln      Go modules and Bun packages, from the lockfiles. This is the half
#             that finds a dependency CVE the day it is published.
#   secret    OFF. gitleaks owns that question (lint:secrets, ADR-0202). Two gates
#             answering the same one is how a finding is triaged twice and fixed
#             never.
#   misconfig OFF, for now, and this is a debt rather than a decision. ADR-0104
#             bought one binary for this surface too, and turning it on today
#             fails on ~40 findings that are all the same finding: the pod
#             security posture D11 exists to fix, across every third-party chart.
#             A gate that ships red is a gate people learn to bypass, so it is
#             enabled with D11 rather than before it. The vendored upstream charts
#             under **/charts are excluded either way — they are someone else's
#             manifests, unpacked from a .tgz, and nothing here can fix them.
#
# Why --ignore-unfixed. ADR-0106 pairs this gate with Renovate: a CVE with a fix
# available produces a bump, which a human reviews and merges. A CVE with no fix
# available produces nothing anyone can act on, and a gate that stays red until an
# upstream maintainer publishes something is a gate people learn to bypass. The
# unfixed ones are not invisible: `mise run scan:report` prints everything,
# including what has no fix and what sits below the gate's severity.
set -euo pipefail

source "$(dirname "$0")/lib/log.sh"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

# HIGH and CRITICAL only. MEDIUM on a transitive dependency is a weekly Renovate
# bump, not a merge blocker, and a gate that blocks on everything is one that gets
# an --exit-code 0 within a month.
SEVERITY="${TRIVY_SEVERITY:-HIGH,CRITICAL}"

# Directories with nothing of ours in them. node_modules is the e2e island's npm
# tree (its own lockfile is scanned); .next and dist are build output; test-results
# holds Playwright traces.
SKIP=(
  # Vendored upstream Helm charts, unpacked from their .tgz. Someone else's
  # manifests; a finding here is a finding for them.
  --skip-dirs "**/charts"
  --skip-dirs "**/node_modules"
  --skip-dirs "**/.next"
  --skip-dirs "**/dist"
  --skip-dirs "**/test-results"
  --skip-dirs "**/playwright-report"
)

# `report` is the triage view: everything, gated on nothing. It is what you run
# when the question is "what do we carry", not "does this merge".
if [ "${1:-gate}" = "report" ]; then
  step "every dependency finding, fixed or not, at every severity"
  trivy fs --scanners vuln --no-progress "${SKIP[@]}" .
  exit 0
fi

step "scanning dependencies (severity ${SEVERITY})"
trivy fs \
  --scanners vuln \
  --severity "$SEVERITY" \
  --ignore-unfixed \
  --exit-code 1 \
  --no-progress \
  "${SKIP[@]}" \
  .

ok "no fixable ${SEVERITY} findings"

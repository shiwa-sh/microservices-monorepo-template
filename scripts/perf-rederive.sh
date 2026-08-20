#!/usr/bin/env bash
# Re-derive the measured numbers the ADR set asserts (ADR-0001, ADR-0204).
#
#   mise run perf:re-derive
#
# ADR-0001 rules that a drifted measurement invalidates the value it set. Several
# ADRs carry numbers tagged *(measured)* — resource sizings in ADR-0204, the
# structural-failure figures in ADR-0501 — and a tag is the strongest claim the
# convention offers and the cheapest to fake. This is what makes it checkable.
#
# It PRINTS rather than rewrites. The comparison is a person's job: a number that
# moved may mean the platform changed, or that the machine did, and a script that
# edited the ADR would erase exactly that distinction. What it removes is the
# excuse that re-deriving is laborious.
set -euo pipefail
source "$(dirname "$0")/lib/log.sh"
cd "$(cd "$(dirname "$0")/.." && pwd)"

step "re-deriving the measured numbers"

detail "committed values, as the ADR set states them:"
grep -rn '(measured)' docs/adr/*.md | sed 's/^/    /' | head -20 || true

echo
detail "resource governance, computed from the charts as they stand now:"
mise run lint:resource-governance 2>&1 | grep -E 'requests|containers' | sed 's/^/    /' || true

echo
detail "load suite — this needs a running cluster (ADR-0601):"
if mise run perf:smoke 2>&1 | tail -20 | sed 's/^/    /'; then
  ok "load suite ran"
else
  warn "load suite did not run — a cluster is required, and its absence is not drift"
fi

echo
warn "compare the two by hand, and change the ADR only when the platform changed"
warn "a number measured on different hardware is a different measurement, not a regression"

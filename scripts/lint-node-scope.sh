#!/usr/bin/env bash
# Node-confinement lint (ADR-0001 §runtime, ADR-0018 §Node escape hatch).
#
# Node is a sanctioned test-only runtime: it drives the Playwright e2e/visual
# runner and nothing else. The whole npm world therefore lives exactly once, in
# the repo-root e2e/ workspace. This guard fails CI if it leaks anywhere else —
# an npm lockfile outside e2e/, or e2e/ being pulled into the Bun workspace
# (which would make `bun install` manage it). Bun (bun.lock*) stays the runtime
# for apps/ and libs/.
set -euo pipefail
cd "$(cd "$(dirname "$0")/.." && pwd)"

fail=0

# 1. No npm lockfile outside e2e/. (Bun lockfiles are fine — that is the app runtime.)
stray=$(find . \
  -path ./e2e -prune -o \
  -path '*/node_modules' -prune -o \
  -name package-lock.json -print 2>/dev/null || true)
if [ -n "$stray" ]; then
  echo "✗ npm lockfile(s) outside e2e/ — Node is e2e-only (ADR-0001/0018):" >&2
  echo "$stray" >&2
  fail=1
fi

# 2. e2e/ must NOT be a Bun workspace member (it is an npm/Node island).
if grep -qE '"e2e"|"e2e/' package.json 2>/dev/null; then
  echo "✗ e2e/ is listed in the root Bun workspace; it must stay a Node island." >&2
  fail=1
fi

# 3. The e2e workspace must actually exist and be npm-managed.
if [ ! -f e2e/package.json ]; then
  echo "✗ missing e2e/package.json — the sanctioned Node workspace." >&2
  fail=1
fi

if [ "$fail" -eq 0 ]; then
  echo "✓ node-scope: npm confined to e2e/"
fi
exit "$fail"

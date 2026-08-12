#!/usr/bin/env bash
# Human-output vocabulary gate (ADR-0001). Scripts speak the fixed vocabulary from
# scripts/lib/log.sh — → step, ✓ ok, ✗ fail, ⚠ warn — not bare status prose. This
# flags `echo`/`printf` of a bare leading WARN/WARNING/ERROR/FAIL(ED)/OK token,
# which should be `warn`/`fail`/`ok` instead. It is a formatting lint, not a
# behaviour check.
set -euo pipefail
source "$(dirname "$0")/lib/log.sh"
cd "$(cd "$(dirname "$0")/.." && pwd)"

step "checking scripts use the log vocabulary (ADR-0001)"

# echo/printf whose message starts with a bare status word. `lib/log.sh` defines
# the vocabulary, so it is exempt.
pattern='(echo|printf)[[:space:]]+(-[a-zA-Z]+[[:space:]]+)?["'\''](WARN(ING)?|ERROR|FAIL(ED)?|OK)\b'

hits=$(grep -rnE "$pattern" scripts --include='*.sh' | grep -v 'scripts/lib/log.sh' || true)

if [[ -n "$hits" ]]; then
  warn "bare status prose found — use warn/fail/ok from scripts/lib/log.sh:"
  printf '%s\n' "$hits" | sed 's/^/  /'
  fail "log-vocabulary lint failed"
fi

ok "all scripts use the log vocabulary"

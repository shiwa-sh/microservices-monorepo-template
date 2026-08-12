# shellcheck shell=bash
# Shared human-output vocabulary for scripts (ADR-0001). Source it, don't execute:
#   source "$(dirname "$0")/lib/log.sh"
# Then: step "doing X"; ok "done"; warn "heads up"; fail "fatal" (fail exits 1).
#
# This is formatting only — it swallows no errors and hides no failures, so it is
# compatible with the "explicit scripts, no magic" rule. The four symbols are the
# fixed vocabulary; nothing here alters control flow except `fail`, which exits.

# Avoid re-defining if sourced twice.
if [[ -n "${__LOG_SH_LOADED:-}" ]]; then return 0 2>/dev/null || true; fi
__LOG_SH_LOADED=1

# → a step is starting.
step() { printf '→ %s\n' "$*"; }

# ✓ a step succeeded.
ok() { printf '✓ %s\n' "$*"; }

# ⚠ a warning (recoverable); goes to stderr so it stands out but does not pollute
# a piped stdout.
warn() { printf '⚠ %s\n' "$*" >&2; }

# ✗ a fatal error to stderr, then exit. Optional second arg is the exit code.
fail() {
  printf '✗ %s\n' "$1" >&2
  exit "${2:-1}"
}

# Two-space-indented sub-detail under a step.
detail() { printf '  %s\n' "$*"; }

#!/usr/bin/env bash
# Diagnose a developer environment.
# Reports on prerequisites, mise health, pinned tool versions, and common
# foot-guns (stale generated code, missing hooks). Read-only; never mutates.

set -uo pipefail

PASS=0
FAIL=0
WARN=0

ok()   { printf "  \033[32m✓\033[0m %s\n" "$1"; PASS=$((PASS+1)); }
bad()  { printf "  \033[31m✗\033[0m %s\n" "$1"; FAIL=$((FAIL+1)); }
warn() { printf "  \033[33m!\033[0m %s\n" "$1"; WARN=$((WARN+1)); }
hdr()  { printf "\n\033[1m%s\033[0m\n" "$1"; }

have() { command -v "$1" >/dev/null 2>&1; }

hdr "Prerequisites"
for cmd in curl git bash; do
  if have "$cmd"; then ok "$cmd present"; else bad "$cmd missing"; fi
done

hdr "mise"
if have mise; then
  ok "mise on PATH ($(mise --version))"
  if mise doctor >/dev/null 2>&1; then
    ok "mise doctor clean"
  else
    warn "mise doctor reported issues — run 'mise doctor' for detail"
  fi
  missing=$(mise ls --missing 2>/dev/null || true)
  if [[ -z "$missing" ]]; then
    ok "all pinned tools installed"
  else
    bad "missing tools — run 'mise install':"
    printf "%s\n" "$missing" | sed 's/^/      /'
  fi
else
  bad "mise not on PATH — run scripts/install.sh"
fi

hdr "Repo state"
if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  ok "inside a git work tree"
  if [[ -f .git/hooks/pre-commit ]]; then
    ok "lefthook pre-commit hook installed"
  else
    warn "no pre-commit hook — run 'mise run setup'"
  fi
  if ! git diff --quiet --exit-code 2>/dev/null; then
    warn "uncommitted changes present"
  fi
else
  bad "not inside a git work tree"
fi

hdr "Summary"
printf "  %d passed, %d warnings, %d failed\n" "$PASS" "$WARN" "$FAIL"

[[ "$FAIL" -eq 0 ]] || exit 1

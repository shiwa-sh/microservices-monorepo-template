#!/usr/bin/env bash
# Plaintext-secret scan (ADR-0202). The Rule — plaintext secret values do not appear
# in any committed file — rested on review alone, and it is the one defect class
# that survives its own fix: reverting the file leaves the value in history, so the
# remediation is rotation (risk-register row 13).
#
# Two scopes, because they answer different questions:
#   working tree  the default, and what a pre-commit hook wants: is the change I am
#                 about to make clean?
#   full history  GITLEAKS_HISTORY=1, for CI on the default branch: is anything
#                 already in the history that has not been rotated?
set -euo pipefail
# shellcheck source=lib/log.sh
source "$(dirname "$0")/lib/log.sh"

args=(--config .gitleaks.toml --redact --no-banner)

if [[ -n "${GITLEAKS_HISTORY:-}" ]]; then
  step "scanning full git history for plaintext secrets"
  gitleaks git "${args[@]}"
else
  step "scanning the working tree for plaintext secrets"
  gitleaks dir . "${args[@]}"
fi

ok "no plaintext secrets found"

#!/usr/bin/env bash
# Reconcile infra/forge/branch-protection.yaml into the forge (ADR-0102).
#
#   FORGE_URL=https://forge.example.com FORGE_REPO=owner/repo \
#   FORGE_TOKEN=… mise run forge:apply
#
# Idempotent: the branch protection is created on the first run and patched on
# every one after, so running it twice changes nothing and running it after a UI
# edit puts the committed value back — which is the point of the file existing.
#
# What the API cannot set is REPORTED rather than skipped silently. Forgejo's
# fork-run approval is a repository setting with no API surface in every release,
# and a script that quietly leaves it unset produces a repo that reads as
# protected and executes a stranger's code on the runner.
set -euo pipefail
# shellcheck source=lib/log.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib/log.sh"

CONFIG="$(dirname "${BASH_SOURCE[0]}")/../infra/forge/branch-protection.yaml"

url="${FORGE_URL:-}"
repo="${FORGE_REPO:-}"
token="${FORGE_TOKEN:-}"

if [[ -z "$url" || -z "$repo" || -z "$token" ]]; then
  warn "FORGE_URL/FORGE_REPO/FORGE_TOKEN unset — branch protection is NOT applied"
  detail "the committed rules are in ${CONFIG}"
  exit 0
fi

branch="$(yq -r '.branch' "$CONFIG")"
# The protection block is the API body: the file's field names are Forgejo's, so
# the payload is the block itself plus the branch it applies to. A mapping table
# here would be a second place for a field name to be wrong.
body="$(yq -o=json '.protection + {"rule_name": .branch}' "$CONFIG")"

api() { # <method> <path> [body]
  local method="$1" path="$2" data="${3:-}"
  curl -sS -o /dev/null -w '%{http_code}' -X "$method" \
    -H "Authorization: token ${token}" \
    -H "Content-Type: application/json" \
    ${data:+--data "$data"} \
    "${url}/api/v1/repos/${repo}/${path}"
}

step "applying branch protection to ${repo}@${branch}"
code="$(api PATCH "branch_protections/${branch}" "$body")"
if [[ "$code" == "404" ]]; then
  code="$(api POST "branch_protections" "$body")"
fi
[[ "$code" =~ ^2 ]] || fail "forge refused the branch protection (HTTP ${code})"
detail "$(yq -r '.protection.status_check_contexts | join(", ")' "$CONFIG") required"

# ── What this script cannot reconcile ───────────────────────────────────────
if [[ "$(yq -r '.actions.approval_for_outside_collaborators' "$CONFIG")" == "true" ]]; then
  warn "fork-run approval is a repository setting with no API: confirm it is on"
  detail "${url}/${repo}/settings/actions — approval required for outside collaborators"
fi

ok "branch protection applied to ${repo}@${branch}"

#!/usr/bin/env bash
# Mint a Kratos session token for a registered identity so you can hit
# authenticated endpoints locally (ADR-0009, ADR-0010). Requires the full tier
# (Ory) up (mise run cluster:full). Drives the native (API) login flow against the
# Kratos public API via a port-forward and prints the session token.
#
#   mise run auth:token -- <email>      # password read from KRATOS_PASSWORD or prompt
#
# This is dev glue, not a product flow; browser logins go through the edge UI.
set -euo pipefail

CLUSTER="${CLUSTER:-platform}"
NS="platform"
k() { kubectl --context "k3d-${CLUSTER}" -n "$NS" "$@"; }

email="${1:?usage: mise run auth:token -- <email>}"
password="${KRATOS_PASSWORD:-}"
if [ -z "$password" ]; then
  read -rsp "password for ${email}: " password; echo
fi

k port-forward svc/ory-kratos-public 4433:80 >/dev/null &
pf=$!
trap 'kill "$pf" 2>/dev/null || true' EXIT
sleep 3

base="http://localhost:4433"
flow="$(curl -fsS -H 'Accept: application/json' "${base}/self-service/login/api")"
action="$(printf '%s' "$flow" | jq -r '.ui.action')"
resp="$(curl -fsS -H 'Accept: application/json' -H 'Content-Type: application/json' \
  -X POST "$action" \
  -d "$(jq -n --arg id "$email" --arg pw "$password" \
        '{method:"password", identifier:$id, password:$pw}')")"

token="$(printf '%s' "$resp" | jq -r '.session_token // empty')"
if [ -z "$token" ]; then
  echo "✗ login failed:" >&2
  printf '%s\n' "$resp" | jq -r '.ui.messages[]?.text // .error.message // .' >&2 || true
  exit 1
fi
echo "$token"

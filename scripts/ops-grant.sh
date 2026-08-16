#!/usr/bin/env bash
# Operator onboarding (ADR-0304, ADR-0306): grant a human the ops tier by adding
# them to group:operator in OpenFGA. Resolves the Kratos identity id by email so
# operators are referenced by who they are, not an opaque id. Idempotent.
#
#   mise run ops:grant -- alice@example.com            # add to group:operator
#   mise run ops:grant -- alice@example.com --revoke   # remove
#
# The per-tool dashboard grants (dashboard:<tool>#viewer@group:operator#member)
# are platform policy seeded once per env by the Argo-synced OpenFGA seed Job
# (infra/helm/platform/openfga); this only manages individual membership. The
# new operator must still enrol a second factor
# (AAL2) before any ops dashboard renders. Run against the target cluster
# (KUBE_CONTEXT overrides the current kubectl context).
set -euo pipefail
cd "$(cd "$(dirname "$0")/.." && pwd)"

NS="${NS:-platform}"
KCTX="${KUBE_CONTEXT:-}"
ctx_args=()
[ -n "$KCTX" ] && ctx_args=(--context "$KCTX")

email="${1:-}"
action="write"
[ "${2:-}" = "--revoke" ] && action="delete"
if [ -z "$email" ]; then
  echo "usage: mise run ops:grant -- <email> [--revoke]" >&2
  exit 2
fi

k() { kubectl "${ctx_args[@]}" -n "$NS" "$@"; }

# Cleanup must be armed before backgrounding anything, or a failure in between
# (e.g. the secret read) would orphan a port-forward.
kpf=""
opf=""
trap 'kill "$kpf" "$opf" 2>/dev/null || true' EXIT

# Reap stale port-forwards from a prior run whose cleanup trap didn't fire (e.g.
# mise/bash killed abruptly), else the new ones fail to bind 4434/18080.
pkill -f 'kubectl.*port-forward svc/(ory-kratos-admin|openfga)' 2>/dev/null || true

# 1. Resolve the Kratos identity id from the email via the admin API.
k port-forward svc/ory-kratos-admin 4434:80 >/dev/null &
kpf=$!
# 2. Open the OpenFGA HTTP API with its preshared key. Local 18080, not 8080: the local edge
#    maps host 8080, so binding 8080 would collide with it.
API="http://localhost:18080"
sk="$(k get secret openfga-creds -o jsonpath='{.data.preshared_key}' | base64 -d)"
k port-forward svc/openfga 18080:8080 >/dev/null &
opf=$!
sleep 4

id="$(curl -fsS "http://localhost:4434/admin/identities?credentials_identifier=${email}" |
  jq -r '.[0].id // ""')"
if [ -z "$id" ]; then
  echo "no Kratos identity for ${email} — they must register first" >&2
  exit 1
fi

# Coarse ops gate (ADR-0306) is a CLAIM check on the `operator` identity trait, NOT
# an OpenFGA call — and it is ALWAYS enforced (the OpenFGA group:operator membership
# below only feeds the optional fine gate, OPS_FINE_GRAINED). Setting group:operator
# without the trait grants nothing, so set the trait here too. The gate additionally
# requires AAL2, which the operator enrols themselves.
op_val=true
[ "$action" = "delete" ] && op_val=false
curl -fsS -X PATCH "http://localhost:4434/admin/identities/${id}" \
  -H 'Content-Type: application/json' \
  -d "[{\"op\":\"add\",\"path\":\"/traits/operator\",\"value\":${op_val}}]" >/dev/null

# Find the platform store by name (same discovery the services do), then write or
# delete the membership tuple. Idempotent: writing an existing tuple or deleting an
# absent one is a no-op (OpenFGA errors on both, so tolerate ONLY those two).
sid="$(fga store list --api-url "$API" --api-token "$sk" |
  jq -r '.stores[] | select(.name=="platform") | .id' | head -n1)"
if [ -z "$sid" ]; then
  echo "no OpenFGA store 'platform' — has the seed Job run?" >&2
  exit 1
fi

set +e
out="$(fga tuple "$action" --store-id "$sid" --api-url "$API" --api-token "$sk" \
  "user:${id}" member group:operator 2>&1)"
rc=$?
set -e
if [ "$rc" -ne 0 ]; then
  if echo "$out" | grep -qE 'already existed|did not exist'; then
    : # already in the desired state
  else
    echo "$out" >&2
    exit 1
  fi
fi

verb="granted"
[ "$action" = "delete" ] && verb="revoked"
echo "✓ ${verb} operator trait + group:operator for ${email} (user:${id})"
[ "$action" = "write" ] && echo "  → they must have AAL2 (a second factor) enrolled; re-login if already signed in."

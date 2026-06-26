#!/usr/bin/env bash
# Operator onboarding (ADR-0010, ADR-0017): grant a human the ops tier by adding
# them to group:operator in SpiceDB. Resolves the Kratos identity id by email so
# operators are referenced by who they are, not an opaque id. Idempotent.
#
#   mise run ops:grant -- alice@example.com            # add to group:operator
#   mise run ops:grant -- alice@example.com --revoke   # remove
#
# The per-tool dashboard grants (dashboard:<tool>#viewer@group:operator#member)
# are platform policy seeded once per env (scripts/seed-spicedb.sh); this only
# manages individual membership. The new operator must still enrol a second factor
# (AAL2) before any ops dashboard renders. Run against the target cluster
# (KUBE_CONTEXT overrides the current kubectl context).
set -euo pipefail
cd "$(cd "$(dirname "$0")/.." && pwd)"

NS="${NS:-platform}"
KCTX="${KUBE_CONTEXT:-}"
ctx_args=()
[ -n "$KCTX" ] && ctx_args=(--context "$KCTX")

email="${1:-}"
action="touch"
[ "${2:-}" = "--revoke" ] && action="delete"
if [ -z "$email" ]; then
  echo "usage: mise run ops:grant -- <email> [--revoke]" >&2
  exit 2
fi

k() { kubectl "${ctx_args[@]}" -n "$NS" "$@"; }

# 1. Resolve the Kratos identity id from the email via the admin API.
k port-forward svc/ory-kratos-admin 4434:80 >/dev/null 2>&1 &
kpf=$!
# 2. Open the SpiceDB gRPC port with its preshared key.
sk="$(k get secret spicedb-creds -o jsonpath='{.data.preshared_key}' | base64 -d)"
k port-forward svc/spicedb 50051:50051 >/dev/null 2>&1 &
spf=$!
trap 'kill "$kpf" "$spf" 2>/dev/null || true' EXIT
sleep 4

id="$(curl -fsS "http://localhost:4434/admin/identities?credentials_identifier=${email}" \
  | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d[0]["id"] if d else "")')"
if [ -z "$id" ]; then
  echo "no Kratos identity for ${email} — they must register first" >&2
  exit 1
fi

zed relationship "$action" group:operator member "user:${id}" \
  --endpoint 127.0.0.1:50051 --insecure --token "$sk"

verb="granted"; [ "$action" = "delete" ] && verb="revoked"
echo "✓ ${verb} group:operator for ${email} (user:${id})"

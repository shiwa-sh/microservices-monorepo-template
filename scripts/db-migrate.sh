#!/usr/bin/env bash
# Apply each service's migrations to the local Postgres (ADR-0007).
# Run after `cluster:lite`; this opens its own port-forward, so it is independent of
# whether `dev:forward` is running and of the native inner loop. Schema migrations
# are run separately here, not by the service in the inner loop (the service runs
# natively against these deps and does not apply migrations itself).
set -euo pipefail

kubectl -n platform port-forward svc/postgres 5432:5432 >/dev/null &
pf=$!
trap 'kill "$pf" 2>/dev/null || true' EXIT
sleep 2

for svc in orders catalog orgs payment; do
  dir="services/$svc/migrations"
  [[ -d "$dir" ]] || continue
  echo "→ migrating $svc"
  DATABASE_URL="postgres://dev:dev@localhost:5432/${svc}?sslmode=disable" \
    DBMATE_MIGRATIONS_DIR="$dir" DBMATE_NO_DUMP_SCHEMA=true \
    dbmate up
done

echo "✓ migrations applied"

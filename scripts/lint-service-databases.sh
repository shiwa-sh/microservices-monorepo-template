#!/usr/bin/env bash
# Every service that owns a schema has a database (ADR-0300).
#
#   mise run lint:service-databases
#
# CNPG creates one database per Cluster and the rest come from
# `postInitApplicationSQL` in infra/helm/platform/postgres/values.yaml. That list is
# hand-written, it runs ONCE at bootstrap, and nothing else references it — so a new
# service can be built, tested locally against a database its own migrations
# created, and reach a deployed environment where the database does not exist.
#
# The failure is not subtle at the pod, but it is invisible at review: the service
# crashloops on connect while every chart, policy and route around it is correct.
# The analytics service shipped exactly this way.
set -euo pipefail

source "$(dirname "$0")/lib/log.sh"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

VALUES="infra/helm/platform/postgres/values.yaml"
[ -f "$VALUES" ] || fail "$VALUES not found"

# The bootstrap database plus every one created afterwards.
declared="$(
  {
    yq -r '.cluster.initdb.database // ""' "$VALUES"
    yq -r '.cluster.initdb.postInitApplicationSQL // [] | .[]' "$VALUES" |
      sed -nE 's/^[[:space:]]*CREATE DATABASE[[:space:]]+([a-z0-9_]+).*/\1/p'
  } | grep -vE '^\s*$' | sort -u
)"

[ -n "$declared" ] || fail "no databases parsed from ${VALUES} — the check cannot run"

rc=0
found=0
for dir in services/*/; do
  svc="$(basename "$dir")"
  [ "${svc#_}" = "$svc" ] || continue # skip _template

  # A service owns a schema when it ships migrations. A worker-only deployable owns
  # none and must NOT have a database — see services/platform.
  [ -d "${dir}migrations" ] || continue
  # An empty migrations directory is not ownership either.
  compgen -G "${dir}migrations/*.sql" >/dev/null || continue

  found=$((found + 1))
  if ! printf '%s\n' "$declared" | grep -qx "$svc"; then
    warn "${svc} ships migrations but no database named '${svc}' is created in ${VALUES}"
    rc=1
  fi
done

[ "$found" -gt 0 ] || fail "no services with migrations found — the check would pass vacuously"

# Migration VERSIONS must be unique within a service.
#
# dbmate keys `schema_migrations` by the numeric prefix, so two files sharing one
# makes the second insert violate the primary key — and the chain cannot be applied
# to a FRESH database at all. It is invisible on an existing one, because the
# version is already recorded, which is why this reached the repository: payment
# carried two files at `20260814000001` and its migrations had never been run from
# empty.
for dir in services/*/migrations/; do
  svc="$(basename "$(dirname "$dir")")"
  [ "${svc#_}" = "$svc" ] || continue
  dupes="$(find "$dir" -maxdepth 1 -name '*.sql' -printf '%f\n' 2>/dev/null |
    sed -E 's/_.*//' | sort | uniq -d)"
  if [ -n "$dupes" ]; then
    for v in $dupes; do
      warn "${svc}: two migrations share version ${v} — dbmate cannot apply this to a fresh database"
    done
    rc=1
  fi
done

if [ "$rc" -eq 0 ]; then
  ok "every schema-owning service has a database (${found} checked)"
fi
exit "$rc"

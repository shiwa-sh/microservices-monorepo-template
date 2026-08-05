#!/usr/bin/env bash
# Guard the inner loop's one silent failure (ADR-0016). Run from a service
# directory, as a dependency of that service's `server`/`worker`/`migrate` tasks.
#
# mise TOLERATES a missing `_.file = ".env"` — it does not warn, it just resolves
# no environment. So on a fresh clone `mise run server` used to start with an unset
# DATABASE_URL and die somewhere inside the pgx driver, which tells you nothing
# about the actual problem ("you never copied .env.example").
#
# Seeding alone is not enough either: `_.file` is read when mise LOADS the config,
# before any task runs, so a copy made here cannot be picked up by the run that
# made it. Hence seed-then-stop — the next invocation has a real environment, and
# the message says so rather than leaving you to guess.
set -euo pipefail

source "$(dirname "$0")/lib/log.sh"

[ -f .env ] && exit 0

[ -f .env.example ] || fail "no .env or .env.example in $(pwd)"

cp .env.example .env
step "seeded .env from .env.example"
detail "review it, then re-run — mise reads .env when it loads the config, so this"
detail "run has no environment yet."
exit 1

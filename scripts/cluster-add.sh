#!/usr/bin/env bash
# One verb for "put this in the cluster" (ADR-0030), over the two kinds of thing
# you can add to `cluster:base`:
#
#   mise run cluster:add -- orders     # a service      → scripts/service-deploy.sh
#   mise run cluster:add -- ory        # a platform chart → scripts/platform-deploy.sh
#
# ── Why a dispatcher and not one merged script ────────────────────────────────
# The two paths share only a spine (pause Argo, then `helm upgrade
# --take-ownership` with the local values). Everything else genuinely differs: the
# argument selects a values file for a service but a chart directory for the
# platform; services always build and import an image, charts never do (except
# lowdefy); charts need the auth overlays and per-chart special cases that mirror
# the ApplicationSet, services need none of it. Merging the bodies would produce
# two scripts in a trenchcoat, so this unifies the interface and leaves the
# implementations where the knowledge lives. Both stay runnable under their own
# names — `platform:deploy` especially, because it prints the "re-enable GitOps"
# instructions you must act on.
#
# ── Why dependency components are not nouns here ──────────────────────────────
# `postgres` is BOTH a platform chart (infra/helm/platform/postgres) and a slice of
# infra/local/deps.yaml. Making dependency components addable by name would make
# `cluster:add -- postgres` ambiguous on day one. They are graph-internal instead:
# services declare `dep:*` in their own .mise.toml and mise resolves them. So this
# dispatcher only ever sees two namespaces, and the ambiguity cannot arise.
set -euo pipefail

source "$(dirname "$0")/lib/log.sh"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

NAME="${1:?usage: mise run cluster:add -- <service|chart>}"

is_service=false
is_chart=false
[ -d "services/${NAME}" ] && [ "${NAME#_}" = "${NAME}" ] && is_service=true
[ -d "infra/helm/platform/${NAME}" ] && is_chart=true

# A name that is both is a repo bug, not a user error — say so plainly rather than
# silently picking one. Keeping services and platform charts disjoint is the rule
# that makes this dispatcher unambiguous (ADR-0030).
if [ "$is_service" = true ] && [ "$is_chart" = true ]; then
  fail "'${NAME}' is both a service and a platform chart — rename one; the two namespaces must stay disjoint"
fi

if [ "$is_service" = true ]; then
  exec bash scripts/service-deploy.sh "$NAME"
elif [ "$is_chart" = true ]; then
  exec bash scripts/platform-deploy.sh "$NAME"
fi

detail "services:        $(find services -mindepth 1 -maxdepth 1 -type d -not -name '_*' -printf '%f ' 2>/dev/null)"
detail "platform charts: $(find infra/helm/platform -mindepth 1 -maxdepth 1 -type d -printf '%f ' 2>/dev/null)"
fail "unknown: '${NAME}' is neither a service nor a platform chart"

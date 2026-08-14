#!/usr/bin/env bash
# Add ONE opt-in local dependency component on top of `cluster:base` (ADR-0205,
# ADR-0600). Backs the `dep:*` mise tasks, which services declare for themselves:
#
#   # services/orders/.mise.toml
#   [tasks.server]
#   depends = ["dep:postgres", "dep:temporal", "dep:openfga"]
#
# Components are label slices of the shared `infra/local/deps.yaml`, never forks of
# it — `kubectl apply` honours `--selector`, so a component is a SELECTION.
#
# ── Why one script and not one per component ──────────────────────────────────
# THE GUARD IS THE WHOLE POINT. mise resolves the dependency graph — it dedupes
# diamonds and parallelises independent edges — but it has no cluster-state
# awareness. Its `sources`/`outputs` staleness skipping is keyed on FILES, and "is
# Temporal already Ready?" is not a file. So without a fast-exit, every
# `mise run server` re-pays a full apply + rollout wait for every dependency, and
# the model ends up slower than the one it replaced.
#
# That guard is the one piece of Garden this repo does not get for free (ADR-0600:
# Garden does status checks natively). Hand-rolling it per component would mean one
# missing check silently costing the inner loop its speed, so it lives here, once,
# and every component inherits it.
#
#   mise run dep:temporal          # via the graph, the normal path
#   bash scripts/dep-apply.sh temporal   # directly, same thing
set -euo pipefail

source "$(dirname "$0")/lib/log.sh"
source "$(dirname "$0")/lib/preflight.sh"

CLUSTER="${CLUSTER:-platform}"
NS="platform"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

COMPONENT="${1:?usage: bash scripts/dep-apply.sh <component>}"

# The guard below reads a failed probe as "not up". Assert the probe can actually
# run first, so an unreachable cluster fails loudly instead of re-applying blindly.
require_cluster "$CLUSTER"

k() { kubectl --context "k3d-${CLUSTER}" "$@"; }

# Each component names the resource whose existence-and-readiness means "already
# up". Deployments get a rollout wait; Secrets are either there or not.
case "$COMPONENT" in
postgres)
  kind=deploy
  probe=postgres
  ;;
temporal)
  kind=deploy
  probe=temporal
  ;;
openfga)
  kind=deploy
  probe=openfga
  ;;
db-secrets)
  kind=secret
  probe=orders-db
  ;;
*)
  fail "unknown component: ${COMPONENT} (known: postgres, temporal, openfga, db-secrets)"
  ;;
esac

# ── The guard ─────────────────────────────────────────────────────────────────
# `rollout status --timeout=0` returns non-zero rather than blocking when the
# Deployment is not yet complete, which is exactly the "up or not?" question.
if [ "$kind" = deploy ]; then
  if k -n "$NS" rollout status "deploy/${probe}" --timeout=0 >/dev/null 2>&1; then
    detail "dep:${COMPONENT} already up — skipping"
    exit 0
  fi
elif k -n "$NS" get "secret/${probe}" >/dev/null 2>&1; then
  detail "dep:${COMPONENT} already up — skipping"
  exit 0
fi

step "adding dep:${COMPONENT}"
# The namespace is not in this file — it comes from the namespaces chart, with its
# pod-security profile (ADR-0200). What is selected here is the component itself.
k apply -f infra/local/deps.yaml -l "local.platform/component=${COMPONENT}"

if [ "$kind" = deploy ]; then
  k -n "$NS" rollout status "deploy/${probe}" --timeout=180s
fi

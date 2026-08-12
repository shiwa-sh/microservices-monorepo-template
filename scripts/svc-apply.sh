#!/usr/bin/env bash
# Make ONE sibling service reachable from a natively-run process (ADR-0205,
# ADR-0600). Backs the `svc:*` mise tasks, which services declare for themselves
# exactly as they declare `dep:*`:
#
#   # services/orders/.mise.toml
#   [tasks.worker]
#   depends = ["env", "dep:postgres", "dep:temporal", "svc:catalog", "svc:payment"]
#
# ── Why this exists ───────────────────────────────────────────────────────────
# `dep:*` covers the INFRASTRUCTURE a service needs (Postgres, Temporal, OpenFGA).
# It does not cover the other services it calls, and in a microservices template
# that is the half that matters more: the orders checkout saga dials catalog and
# payment, so `mise run worker` used to hand you a process that started cleanly and
# died on the first checkout. Closing that gap by hand meant knowing the callee
# list, deploying each one, port-forwarding each one, and editing .env — four steps
# a named profile can't help with and the service itself already knows.
#
# This is the precise failure mode ADR-0600 rejected Tilt for: enabling a subset
# without considering its dependencies turns "why is my service failing" into a
# scavenger hunt. Declaring callees here makes the graph tell the truth.
#
# ── What "reachable" means ────────────────────────────────────────────────────
# The callee answers on its registered local port (scripts/lib/ports.sh), so the
# caller's CATALOG_URL/PAYMENT_URL default just works. Two different situations
# satisfy that, and BOTH are legitimate:
#
#   1. The callee runs natively too — you are debugging across services and
#      started it yourself. Nothing to do; this must not stomp on it.
#   2. The callee is in the cluster, port-forwarded to that same port. The default
#      (ADR-0205): you are working on the CALLER, and the callees should be opaque.
#
# So the guard probes the PORT, not the deployment. Case 1 then falls out for free
# rather than needing a flag — start catalog natively and orders stops managing it.
set -euo pipefail

source "$(dirname "$0")/lib/log.sh"
source "$(dirname "$0")/lib/ports.sh"
source "$(dirname "$0")/lib/preflight.sh"

CLUSTER="${CLUSTER:-platform}"
NS="platform"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

SVC="${1:?usage: bash scripts/svc-apply.sh <service>}"
[ -d "services/${SVC}" ] || fail "no such service: services/${SVC}"

PORT="$(service_port "$SVC")"
RUNDIR="${XDG_RUNTIME_DIR:-${TMPDIR:-/tmp}}/platform-local"
PIDFILE="${RUNDIR}/portforward-${SVC}.pid"

# Before any guard: a probe that CANNOT run must not be read as "not up", or the
# fall-through starts building images against an unreachable cluster.
require_cluster "$CLUSTER"

k() { kubectl --context "k3d-${CLUSTER}" -n "$NS" "$@"; }

# Probe the local port with bash's /dev/tcp rather than curl: it asks the only
# question that matters (is something listening?) without caring whether the
# service has a health endpoint, needs auth, or is mid-startup.
port_open() { (exec 3<>"/dev/tcp/127.0.0.1/${PORT}") >/dev/null 2>&1; }

# ── The guard ─────────────────────────────────────────────────────────────────
# Same contract as dep-apply.sh: fast-exit when already satisfied, so re-entering
# the graph on every `mise run worker` stays cheap. Without it each run would
# re-deploy a service that is already up.
if port_open; then
  detail "svc:${SVC} already reachable on :${PORT} — skipping"
  exit 0
fi

# A stale pidfile means a forward died (cluster restart, laptop sleep). Clear it
# before starting a new one so the pidfile never outlives its process.
if [ -f "$PIDFILE" ]; then
  kill "$(cat "$PIDFILE")" 2>/dev/null || true
  rm -f "$PIDFILE"
fi

# Deploy only if it isn't there. `cluster:add -- <svc>` is the same code path, so a
# service you deployed by hand earlier is picked up rather than rebuilt.
if k rollout status "deploy/${SVC}-server" --timeout=0 >/dev/null 2>&1; then
  detail "svc:${SVC} deployed already — forwarding only"
else
  step "svc:${SVC}: deploying (the caller needs it; you are not working on it)"
  bash scripts/service-deploy.sh "$SVC"
fi

# :80 is the SERVICE port, not the container's :8080 — infra/helm/service/templates/
# service.yaml maps 80 → targetPort http. Forwarding to 8080 fails outright, since a
# svc/ port-forward resolves against the Service's declared ports.
step "svc:${SVC}: forwarding :${PORT} → ${SVC}-server:80"
mkdir -p "$RUNDIR"

# Detached, because a mise task must return for the graph to continue but the
# forward has to outlive it. This is the one piece of process management in the dev
# loop; it is confined here, keyed by a pidfile, and torn down by cluster:remove.
nohup kubectl --context "k3d-${CLUSTER}" -n "$NS" \
  port-forward "svc/${SVC}-server" "${PORT}:80" \
  >"${RUNDIR}/portforward-${SVC}.log" 2>&1 &
echo $! >"$PIDFILE"

for _ in $(seq 1 50); do
  port_open && break
  sleep 0.2
done

port_open || fail "svc:${SVC}: port-forward did not come up — see ${RUNDIR}/portforward-${SVC}.log"
ok "svc:${SVC} reachable on :${PORT}"

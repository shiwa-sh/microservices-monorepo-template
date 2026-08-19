# shellcheck shell=bash
# The local port registry (ADR-0205). Source it, don't execute:
#   source "$(dirname "$0")/lib/ports.sh"
#   port="$(service_port catalog)"
#
# ── Why a registry and not a convention ───────────────────────────────────────
# Every server binds :8080 in-cluster, and that is fine when exactly one service
# runs natively. It stops being fine the moment a second one does — debugging the
# checkout saga means orders, catalog and payment on one machine at once — because
# they collide, and because orders then has to be TOLD where catalog is.
#
# Picking ports ad hoc pushes that choice into each engineer's gitignored .env,
# where it cannot be shared, defaulted, or checked. So the assignment is committed
# here instead: one stable port per service, the same on every machine. That is
# what lets services/orders/.env.example ship a WORKING CATALOG_URL rather than a
# commented-out suggestion to invent one.
#
# These apply to natively-run processes only. In-cluster nothing changes: the chart
# sets no PORT, so every pod still binds the :8080 the containerPort, the edge
# IngressRoutes and the NetworkPolicies all assume.
#
# ── Adding a service ──────────────────────────────────────────────────────────
# Add a line. Keep it unique — `mise run lint:ports` fails the build on a
# collision, because two services quietly sharing a port is a bind race that only
# shows up when someone runs both.
#
# 8080 is deliberately UNASSIGNED: the local edge maps host 8080, and leaving it
# free keeps "the port I bound" and "the port the edge answers on" distinct.

if [[ -n "${__PORTS_SH_LOADED:-}" ]]; then return 0 2>/dev/null || true; fi
__PORTS_SH_LOADED=1

# service:port — the registry itself.
__LOCAL_PORTS="
analytics:8086
authz:8085
catalog:8081
orders:8082
orgs:8083
payment:8084
"

# service_port <svc> — prints the registered local port, or fails.
service_port() {
  local svc="${1:?service_port: missing service}" line
  line="$(printf '%s\n' "$__LOCAL_PORTS" | grep "^${svc}:" || true)"
  if [ -z "$line" ]; then
    printf '✗ no local port registered for %s — add it to scripts/lib/ports.sh\n' "$svc" >&2
    return 1
  fi
  printf '%s' "${line#*:}"
}

# all_port_entries — prints every "service:port" line. Used by lint:ports.
all_port_entries() { printf '%s\n' "$__LOCAL_PORTS" | grep ':'; }

#!/usr/bin/env bash
# Port-forward the inner-loop dependencies (ADR-0200, ADR-0205) so a service run
# NATIVELY on the host (in any editor/IDE, or `go run ./services/<svc>/...`) can
# reach them. Long-running
# — run it in a separate terminal (or background) and leave it up while you iterate.
#
#   mise run cluster:base      # once: the local floor (a service's tasks do this for you)
#   mise run dev:forward     # this script, in its own terminal
#   DATABASE_URL=... TEMPORAL_HOST_PORT=localhost:7233 OPENFGA_API_URL=http://localhost:18080 \
#     go run ./services/orders/cmd/server   # run the service natively
set -euo pipefail

CLUSTER="${CLUSTER:-platform}"
source "$(dirname "$0")/lib/cluster-ctx.sh"
NS="platform"
k() { kubectl --context "$(cluster_ctx)" -n "$NS" "$@"; }

echo "→ forwarding deps: postgres 5432, temporal 7233 + 8233 (Ctrl-C to stop)"
pids=()
cleanup() { kill "${pids[@]}" 2>/dev/null || true; }
trap cleanup EXIT INT TERM

k port-forward svc/postgres 5432:5432 &
pids+=($!)
k port-forward svc/temporal 7233:7233 &
pids+=($!)
k port-forward svc/temporal 8233:8233 &
pids+=($!)
# Local 18080, not 8080: the local edge maps host 8080, so the OpenFGA forward uses
# 18080 (matches OPENFGA_API_URL in services/*/.env.example). Services themselves
# bind their own registered ports (scripts/lib/ports.sh) and never collide here.
k port-forward svc/openfga 18080:8080 &
pids+=($!)

# If observability is up (full tier / obs profile), forward Grafana + the OTel
# Collector's Faro receiver too. The frontend's dev RUM shim forwards beacons to
# FARO_COLLECT_URL=http://localhost:12347/collect (apps/frontend/src/app/api/rum).
# The collector is in its own namespace (ADR-0200 — hostPath and hostPorts), so it
# takes its own kubectl invocation rather than the `k` helper's.
agent() { kubectl --context "$(cluster_ctx)" -n otel-agent "$@"; }
if agent get svc otel-collector >/dev/null 2>&1; then
  echo "→ observability detected: grafana 3001, faro 12347"
  k port-forward svc/grafana 3001:80 &
  pids+=($!)
  agent port-forward svc/otel-collector 12347:8027 &
  pids+=($!)
fi

wait

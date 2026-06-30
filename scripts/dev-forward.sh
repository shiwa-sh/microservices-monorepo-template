#!/usr/bin/env bash
# Port-forward the inner-loop dependencies (ADR-0003, ADR-0016) so a service run
# NATIVELY on the host (in any editor/IDE, or `go run ./services/<svc>/...`) can
# reach them. Replaces the Skaffold `platform` module's port-forwards. Long-running
# — run it in a separate terminal (or background) and leave it up while you iterate.
#
#   mise run cluster:lite      # once: cluster + lightweight deps
#   mise run dev:forward     # this script, in its own terminal
#   DATABASE_URL=... TEMPORAL_HOST_PORT=localhost:7233 SPICEDB_ENDPOINT=localhost:50051 \
#     go run ./services/orders/cmd/server   # run the service natively
set -euo pipefail

CLUSTER="${CLUSTER:-platform}"
NS="platform"
k() { kubectl --context "k3d-${CLUSTER}" -n "$NS" "$@"; }

echo "→ forwarding deps: postgres 5432, temporal 7233 + 8233 (Ctrl-C to stop)"
pids=()
cleanup() { kill "${pids[@]}" 2>/dev/null || true; }
trap cleanup EXIT INT TERM

k port-forward svc/postgres 5432:5432 &  pids+=($!)
k port-forward svc/temporal 7233:7233 &  pids+=($!)
k port-forward svc/temporal 8233:8233 &  pids+=($!)
k port-forward svc/spicedb  50051:50051 & pids+=($!)

# If observability is up (full tier / obs profile), forward Grafana + Faro too.
if k get svc otel-lgtm >/dev/null 2>&1; then
  echo "→ observability detected: grafana 3001, faro 12347"
  k port-forward svc/otel-lgtm 3001:3000 & pids+=($!)
  k port-forward svc/faro 12347:12347 &     pids+=($!)
fi

wait

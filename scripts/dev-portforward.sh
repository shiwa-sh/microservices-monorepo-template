#!/usr/bin/env bash
# Manage declarative port-forwards for local development (ADR-0003).
# Ports listed here are the canonical local addresses services on the host use.
set -euo pipefail

PIDFILE=".k3d/portforward.pids"
mkdir -p .k3d

# name|namespace|svc|local:remote
FORWARDS=(
  "postgres|platform|svc/postgres-rw|5432:5432"
  "temporal|platform|svc/temporal-frontend|7233:7233"
  "otel|platform|svc/otel-collector|4317:4317"
  "grafana|platform|svc/grafana|3000:3000"
  "minio|platform|svc/minio|9000:9000"
  "minio-console|platform|svc/minio|9001:9001"
)

case "${1:-up}" in
  up)
    : > "$PIDFILE"
    for entry in "${FORWARDS[@]}"; do
      IFS='|' read -r name ns svc ports <<<"$entry"
      nohup kubectl -n "$ns" port-forward "$svc" "$ports" >/tmp/pf-"$name".log 2>&1 &
      pid=$!
      disown "$pid" 2>/dev/null || true
      echo "$pid|$name" >> "$PIDFILE"
    done
    echo "✓ port-forwards started"
    ;;
  down)
    if [[ -f "$PIDFILE" ]]; then
      while IFS='|' read -r pid name; do
        kill "$pid" 2>/dev/null || true
      done < "$PIDFILE"
      rm -f "$PIDFILE"
    fi
    echo "✓ port-forwards stopped"
    ;;
  *)
    echo "usage: $0 {up|down}" >&2; exit 1 ;;
esac

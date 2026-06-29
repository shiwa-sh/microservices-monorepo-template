#!/usr/bin/env bash
# Inner loop (ADR-0003, ADR-0016): the single k3d cluster + a CNI + the lightweight
# dependency stand-ins (Postgres, Temporal, SpiceDB). The service you are changing
# runs NATIVELY on the host — no image build, no in-cluster redeploy, no watch on
# the hot path. After this, run `mise run dev:forward` and start the service in any
# editor/IDE (or `go run`). The full platform is `mise run cluster:full` instead.
set -euo pipefail

CLUSTER="${CLUSTER:-platform}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

bash scripts/cluster-create.sh
bash scripts/cilium-install.sh

echo "→ applying lightweight dev dependencies (Postgres, Temporal, SpiceDB)"
kubectl --context "k3d-${CLUSTER}" apply -f infra/local/deps.yaml

cat <<'EOF'
✓ cluster:up complete (inner loop).
  Next:
    mise run dev:forward          # port-forward the deps (separate terminal)
    mise run db:migrate           # apply migrations to the local DB
    # then run the service natively, e.g.:
    DATABASE_URL=postgres://dev:dev@localhost:5432/orders?sslmode=disable \
      TEMPORAL_HOST_PORT=localhost:7233 SPICEDB_ENDPOINT=localhost:50051 \
      go run ./services/orders/cmd/server
EOF

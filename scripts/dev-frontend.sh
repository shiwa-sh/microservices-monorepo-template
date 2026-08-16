#!/usr/bin/env bash
# The host-run `next dev` the local edge routes `/` to (ADR-0400, ADR-0205).
# Long-running — use a separate terminal.
#
# Its whole job beyond `next dev` is TLS trust. The local wildcard used to be a
# self-signed LEAF, which cannot be a trust anchor, so this task set
# NODE_TLS_REJECT_UNAUTHORIZED=0 — a blanket "verify nothing" for the process. The
# local issuer is now a real CA (infra/helm/platform/cert-manager), so the honest
# thing is available: pull the CA out of the cluster and trust exactly it.
#
# That also keeps the host loop and the in-cluster image on the same mechanism —
# the pod mounts the same secret at NODE_EXTRA_CA_CERTS
# (infra/gitops/services/local/values/frontend.yaml). Neither disables verification.
set -euo pipefail

source "$(dirname "$0")/lib/log.sh"

CLUSTER="${CLUSTER:-platform}"
source "$(dirname "$0")/lib/cluster-ctx.sh"
NS="platform"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

CA_DIR="${XDG_CACHE_HOME:-${HOME}/.cache}/platform-local"
CA_FILE="${CA_DIR}/local-ca.crt"

k() { kubectl --context "$(cluster_ctx)" "$@"; }

# Re-extract every run: a cluster:delete mints a new CA, and a stale file would
# fail verification in a way that looks like a code problem.
if k -n "$NS" get secret local-ca-tls >/dev/null 2>&1; then
  mkdir -p "$CA_DIR"
  k -n "$NS" get secret local-ca-tls -o jsonpath='{.data.ca\.crt}' | base64 -d >"$CA_FILE"
  export NODE_EXTRA_CA_CERTS="$CA_FILE"
  detail "trusting the local CA from ${CA_FILE}"
else
  warn "local-ca-tls not found — is cluster:base up? TLS calls to the edge will fail"
fi

# Run through the island's own mise config (apps/frontend/.mise.toml) so the
# pinned node is on PATH: `next`'s CLI and Turbopack's workers are
# `#!/usr/bin/env node` binaries, and bun spawns node for them (ADR-0100).
cd apps/frontend
exec mise run dev

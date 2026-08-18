#!/usr/bin/env bash
# Rewrite the env host to the edge FROM INSIDE the cluster (ADR-0205).
#
# `dev.localtest.me` is real public DNS that resolves to 127.0.0.1. That is exactly
# right on the host and exactly wrong in a pod, where 127.0.0.1 is the pod's own
# loopback — so an in-cluster workload dialling EDGE_INTERNAL_ORIGIN would connect
# to itself instead of Traefik.
#
# Rewriting the NAME (rather than pointing the workload at a Service DNS name) is
# what keeps the rest of the stack honest: the /api IngressRoutes match on
# Host(dev.localtest.me) and the wildcard cert is issued for it, so the request must
# genuinely carry that host and that SNI. A cluster-internal name would fail both.
#
# kind runs a stock CoreDNS Deployment with no custom-record mechanism, so this
# patches its Corefile directly.
# The rewrite stanza is inserted once into the default `.:53` server block
# (idempotent — it is skipped when already present) and CoreDNS restarts to load
# it. Local-only: deployed environments resolve their real host through real DNS.
set -euo pipefail

source "$(dirname "$0")/lib/cluster-ctx.sh"

CLUSTER="${CLUSTER:-platform}"
k() { kubectl --context "$(cluster_ctx)" "$@"; }

STANZA='rewrite stop {
    name exact dev.localtest.me traefik.kube-system.svc.cluster.local
    answer auto
}
rewrite stop {
    name regex (.*)\.dev\.localtest\.me traefik.kube-system.svc.cluster.local
    answer auto
}'

k -n kube-system get configmap coredns >/dev/null 2>&1 || {
  echo "✗ CoreDNS ConfigMap not found — is the cluster up?" >&2
  exit 1
}

corefile="$(k -n kube-system get configmap coredns -o jsonpath='{.data.Corefile}')"
if printf '%s' "$corefile" | grep -q 'name exact dev.localtest.me'; then
  echo "✓ dev.localtest.me rewrite already present in CoreDNS"
  exit 0
fi

# The Corefile's first line opens the default server block; the stanza goes
# directly after it. Kind's Corefile always starts with `.:53 {`.
patched="$(printf '%s\n' "$corefile" | awk -v stanza="$STANZA" '
  NR == 1 { print; print stanza; next }
  { print }
')"

k -n kube-system create configmap coredns \
  --from-literal="Corefile=$patched" \
  --dry-run=client -o yaml | k apply -f - >/dev/null
k -n kube-system rollout restart deploy/coredns >/dev/null
k -n kube-system rollout status deploy/coredns --timeout=120s
echo "✓ dev.localtest.me → traefik.kube-system rewrite installed in CoreDNS"

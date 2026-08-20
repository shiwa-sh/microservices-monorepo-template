#!/usr/bin/env bash
# Ensure the single local kind cluster exists and is running (ADR-0200, ADR-0205).
# Convergent: create it if absent, start it if stopped (cluster:stop keeps the
# image cache + volumes; cluster:delete deletes it). One cluster serves both local
# tiers; what differs is what you bring up on it:
#   mise run cluster:base     → the local floor (a service's own tasks bring it up)
#   mise run cluster:full   → full platform via ArgoCD
#
# Cilium is the CNI (NetworkPolicy + Hubble, ADR-0200): disableDefaultCNI and no
# kube-proxy are set at create time in infra/local/kind.yaml (kind declines
# kube-proxy itself). Traefik comes from the committed
# chart (ADR-0305), installed by cluster:base / cluster:full — kind ships no
# ingress controller. Ports 8080/8443 map the edge.
#
# Local image registry (ADR-0205 parity): repo-built images (services, lowdefy)
# have no CI/ghcr locally, so Argo has nothing to pull. A plain registry container
# on the kind docker network — registry.localhost:5000 — is the local stand-in for
# CI: cluster:full builds+pushes to it, the local values overlays point at it, and
# Argo pulls exactly as prod pulls from ghcr. The container is host-level, not a
# kind workload: it survives cluster delete/recreate, and this script re-attaches
# it to the (recreated) kind network on every ensure.
#
# Proxied networks: kind cannot inject proxy environment into the node, so the
# proxied path is host-pull + load instead —
# CLUSTER_PRELOAD=1 / cluster:unwedge. See infra/local/kind.yaml and
# docs/dev-loop.md ("HTTP proxies").
set -euo pipefail

source "$(dirname "$0")/lib/log.sh"
source "$(dirname "$0")/lib/cluster-ctx.sh"

CLUSTER="${CLUSTER:-platform}"
REGISTRY="registry.localhost" # container name = the cluster-facing pull name

# The registry container: exists once, survives kind cluster recreation. Published
# on loopback:5000 because docker treats loopback as an insecure registry and that
# sidesteps per-machine daemon.json for pushes (cluster:full pushes to
# 127.0.0.1:5000; the node pulls the same registry over the docker network, and
# blobs are keyed by repo path, so the two names never need to agree).
if ! docker inspect "$REGISTRY" >/dev/null 2>&1; then
  echo "→ creating local registry container '${REGISTRY}:5000'"
  docker run -d --restart=always --name "$REGISTRY" \
    -p 127.0.0.1:5000:5000 registry:2 >/dev/null
fi

# The full tier holds the same edge ports (8080/8443) — the two tiers are
# alternatives rather than peers. Name the conflict rather than letting docker
# report a bind failure from inside a half-created cluster.
if docker inspect -f '{{.State.Running}}' "${CLUSTER}-full-controlplane-1" 2>/dev/null | grep -qx true; then
  echo "✗ the full tier is running and holds the edge ports 8080/8443." >&2
  echo "  The two tiers are alternatives — run one at a time:" >&2
  echo "    TIER=full mise run cluster:stop" >&2
  exit 1
fi

if ! kind get clusters 2>/dev/null | grep -qx "$CLUSTER"; then
  echo "→ creating kind cluster '$CLUSTER'"
  kind create cluster --name "$CLUSTER" --config infra/local/kind.yaml
else
  node="${CLUSTER}-control-plane"
  if [ "$(docker inspect -f '{{.State.Running}}' "$node" 2>/dev/null)" = "false" ]; then
    echo "→ cluster '$CLUSTER' exists but is stopped; starting it"
    docker start "$node" >/dev/null
    # The API server comes back as the node's processes resume; wait on the node
    # before anything probes it.
    kubectl --context "$(cluster_ctx)" wait --for=condition=Ready node --all --timeout=180s
  else
    echo "→ cluster '$CLUSTER' exists and is running"
  fi
fi

# The node resolves registry.localhost through docker's embedded DNS on the kind
# network, so the container must be on it. The network is recreated with the
# cluster, so re-attach on every ensure.
if docker network inspect kind >/dev/null 2>&1; then
  if ! docker inspect "$REGISTRY" --format \
    '{{range $k, $v := .NetworkSettings.Networks}}{{$k}} {{end}}' | grep -qw kind; then
    echo "→ attaching ${REGISTRY} to the kind docker network"
    docker network connect kind "$REGISTRY"
  fi
fi

kubectl config use-context "$(cluster_ctx)" >/dev/null

# Re-stamp the host-only frontend edge glue on every start. It is not GitOps-managed,
# so a stop/start otherwise comes back without the catch-all `frontend` route (404 at
# /) or with a stale docker-bridge address. No-op on a brand-new cluster whose Traefik
# CRDs aren't up yet — cluster:full re-runs it once the platform is synced.
CLUSTER="$CLUSTER" bash scripts/cluster-edge-glue.sh

echo "✓ cluster '$CLUSTER' ready (context $(cluster_ctx))"

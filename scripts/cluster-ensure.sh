#!/usr/bin/env bash
# Ensure the ACTIVE TIER's kind cluster exists and is running (ADR-0200, ADR-0600).
# Convergent: create it if absent, start it if stopped (cluster:stop keeps the image
# cache + volumes; cluster:delete deletes it).
#
#   mise run cluster:base            → the inner loop, one node
#   TIER=full mise run cluster:full  → the platform on three nodes, via Argo CD
#
# BOTH TIERS ARE KIND. A tier is a node count and a set of workloads, not a
# distribution — one provisioner, one image path, one bring-up to debug (ADR-0600).
# The two are separate clusters so neither bring-up disturbs the other, and they
# share the edge's host ports, which makes them alternatives rather than peers.
#
# Cilium is the CNI (NetworkPolicy + Hubble, ADR-0206): disableDefaultCNI and
# kubeProxyMode: none are set at create time in the tier's config, and the committed
# chart is installed by cluster:base / cluster:full. kind ships no ingress
# controller, so Traefik comes from its committed chart too (ADR-0305).
#
# Local image registry (ADR-0105): repo-built images have no CI or ghcr locally, so
# Argo has nothing to pull. zot — the same registry every deployed environment runs —
# is a host container at registry.localhost:5000, and it also mirrors the upstream
# registries as a pull-through cache, so a proxied network needs no preloading. The
# container is host-level rather than a cluster workload: it survives delete and
# recreate, and this script re-attaches it to the kind network on every ensure.
set -euo pipefail

source "$(dirname "$0")/lib/log.sh"
source "$(dirname "$0")/lib/cluster-ctx.sh"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

NAME="$(cluster_name)"
CTX="$(cluster_ctx)"
CONFIG="$(cluster_kind_config)"
REGISTRY="registry.localhost" # container name = the cluster-facing pull name
ZOT_IMAGE="ghcr.io/project-zot/zot-linux-amd64:v2.1.20"

# The registry container: exists once, survives cluster recreation. Published on
# loopback:5000 because docker treats loopback as an insecure registry, which
# sidesteps a per-machine daemon.json for pushes — cluster:full pushes to
# 127.0.0.1:5000 while the nodes pull the same registry over the docker network, and
# blobs are keyed by repo path, so the two names never have to agree.
if ! docker inspect "$REGISTRY" >/dev/null 2>&1; then
  step "creating local registry container '${REGISTRY}:5000' (zot)"
  # The config path is passed explicitly: the image's default command names a
  # config.json, and this one is YAML so that its reasons can be read next to it.
  docker run -d --restart=always --name "$REGISTRY" \
    -p 127.0.0.1:5000:5000 \
    -v "${ROOT}/infra/local/zot-config.yaml:/etc/zot/config.yaml:ro" \
    -v "${REGISTRY}-data:/var/lib/zot" \
    "$ZOT_IMAGE" serve /etc/zot/config.yaml >/dev/null
fi

# A registry that is restarting is a registry every pull will miss, and the failure
# lands later on a pod rather than here — so it is checked, not assumed.
for _ in $(seq 1 30); do
  [ "$(docker inspect -f '{{.State.Running}}' "$REGISTRY" 2>/dev/null)" = "true" ] && break
  sleep 1
done
if [ "$(docker inspect -f '{{.State.Running}}' "$REGISTRY" 2>/dev/null)" != "true" ]; then
  fail "the local registry '${REGISTRY}' is not running:
$(docker logs --tail 5 "$REGISTRY" 2>&1 | sed 's/^/    /')"
fi

# The OTHER tier holds the same edge ports. Name the conflict rather than letting
# docker report a bind failure from inside a half-created cluster.
other="$NAME"
[ "$(cluster_tier)" = "full" ] && other="${CLUSTER:-platform}" || other="${CLUSTER:-platform}-full"
if kind get clusters 2>/dev/null | grep -qx "$other" &&
  docker inspect -f '{{.State.Running}}' "${other}-control-plane" 2>/dev/null | grep -qx true; then
  fail "cluster '${other}' is running and holds the edge ports 8080/8443.
  The two tiers are alternatives — run one at a time:
    CLUSTER=${CLUSTER:-platform} TIER=$([ "$(cluster_tier)" = "full" ] && echo base || echo full) mise run cluster:stop
  then re-run this."
fi

if ! kind get clusters 2>/dev/null | grep -qx "$NAME"; then
  step "creating kind cluster '${NAME}' from ${CONFIG}"
  kind create cluster --name "$NAME" --config "$CONFIG"
else
  # Every node, not just the control plane: a multi-node cluster that was stopped
  # comes back with its workers down, and a control plane alone reports Ready while
  # nothing schedules.
  mapfile -t nodes < <(kind get nodes --name "$NAME")
  stopped=0
  for n in "${nodes[@]}"; do
    [ "$(docker inspect -f '{{.State.Running}}' "$n" 2>/dev/null)" = "false" ] && stopped=1
  done
  if [ "$stopped" = 1 ]; then
    step "cluster '${NAME}' exists but is stopped; starting ${#nodes[@]} node(s)"
    for n in "${nodes[@]}"; do docker start "$n" >/dev/null; done
    kubectl --context "$CTX" wait --for=condition=Ready node --all --timeout=300s
  else
    step "cluster '${NAME}' exists and is running"
  fi
fi

# The nodes resolve registry.localhost through docker's embedded DNS on the kind
# network, so the container has to be on it. kind puts every cluster on one network
# named `kind`, which is recreated with the first cluster — so re-attach every time.
if docker network inspect kind >/dev/null 2>&1; then
  if ! docker inspect "$REGISTRY" --format \
    '{{range $k, $v := .NetworkSettings.Networks}}{{$k}} {{end}}' | grep -qw kind; then
    step "attaching ${REGISTRY} to the kind docker network"
    docker network connect kind "$REGISTRY"
  fi
fi

kubectl config use-context "$CTX" >/dev/null

# Re-stamp the host-only frontend edge glue on every start. It is not GitOps-managed,
# so a stop/start otherwise comes back without the catch-all `frontend` route (404 at
# /) or with a stale docker-bridge address. No-op on a brand-new cluster whose Traefik
# CRDs aren't up yet — cluster:full re-runs it once the platform is synced.
CLUSTER="$NAME" bash scripts/cluster-edge-glue.sh

ok "cluster '${NAME}' ready (context ${CTX})"

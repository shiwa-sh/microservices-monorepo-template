#!/usr/bin/env bash
# Ensure the FULL tier's Talos cluster exists and is running (ADR-0600, ADR-0200).
#
#   TIER=full mise run cluster:ensure
#
# The full tier runs Talos in Docker because that is the only local option that
# takes a machine config, so the mechanism ADR-0206 uses to deliver the CNI and to
# disable kube-proxy is exercised rather than approximated.
#
# One control plane and two workers. The docker provisioner takes `--workers` and
# has no `--controlplanes` flag, so etcd has one member here; ADR-0600 says why that
# is the right trade and what the workers are actually for — a real network hop
# between pods, meaningful anti-affinity, and node-pinned volumes.
set -euo pipefail

source "$(dirname "$0")/lib/log.sh"
source "$(dirname "$0")/lib/cluster-ctx.sh"

NAME="$(cluster_name)"
CTX="$(cluster_ctx)"
REGISTRY="registry.localhost"
WORKERS="${TALOS_WORKERS:-2}"
# The subnet is pinned rather than left to the provisioner's default, because the
# proxy patch below needs the gateway's address before the network exists.
SUBNET="${TALOS_SUBNET:-10.5.0.0/24}"
GATEWAY="${SUBNET%.*/*}.1"
CP_IP="${SUBNET%.*/*}.2"
# Pinned, and pinned to the same version the deployed machine configs are written
# against (infra/talos/). A local tier on a different Talos than production is a
# tier that cannot answer the question it exists to answer.
TALOS_VERSION="1.13.5"
K8S_VERSION="1.36.2"

# The two tiers are ALTERNATIVES, not peers, and they share the edge's host ports —
# 8080 and 8443 are what every doc, the e2e default host, the perf runner and the
# frontend's own server-side fetcher all name. Keeping one address is what keeps
# those true for whichever tier is up.
#
# So the other tier holding those ports is a conflict worth naming. Without this,
# docker reports `Bind for 0.0.0.0:8080 failed: port is already allocated` from
# inside a half-created cluster, and the state directory is left behind for the next
# run to trip over.
other_tier_running() {
  local other="${CLUSTER:-platform}"
  docker inspect -f '{{.State.Running}}' "${other}-control-plane" 2>/dev/null | grep -qx true
}

if other_tier_running; then
  fail "the inner-loop cluster is running and holds the edge ports 8080/8443.
  The two tiers are alternatives — run one at a time:
    mise run cluster:stop     # keeps its images and volumes
  then re-run this."
fi

# HOST DISK. The nodes are containers, so they share this filesystem and each
# kubelet measures ITS OWN image filesystem as the host's. Cross the eviction
# threshold and every node takes a `disk-pressure` taint at once.
#
# What makes that worth a preflight rather than a surprise is the failure
# signature, which looks nothing like the cause: the Cilium OPERATOR cannot
# schedule (it does not tolerate disk-pressure), so it never registers the Cilium
# CRDs, so every agent waits five minutes and then fatals with "Unable to find all
# Cilium CRDs". Nothing in that chain mentions disk. Measured on this machine at
# 89% full.
#
# The threshold is the kubelet's default of 10% free, with headroom: the platform's
# images are several GB and are pulled after this check passes.
avail_pct="$(df --output=pcent / | tail -1 | tr -dc '0-9')"
if [ "$avail_pct" -ge 85 ]; then
  fail "the host filesystem is ${avail_pct}% full; Talos nodes share it and will take
  disk-pressure taints, which stops the Cilium operator scheduling and makes every
  agent fatal on missing CRDs. Free space first:
    docker image prune -af && docker volume prune -af
    mise run cluster:delete          # the inner-loop cluster, if it is not in use"
fi

# The registry container is host-level and shared with the inner loop: it survives
# either cluster being deleted, which is what makes `cluster:delete` cheap.
if ! docker inspect "$REGISTRY" >/dev/null 2>&1; then
  step "creating local registry container '${REGISTRY}:5000'"
  docker run -d --restart=always --name "$REGISTRY" \
    -p 127.0.0.1:5000:5000 registry:2 >/dev/null
fi

# A container is not a cluster. A create that failed part way leaves containers and
# a state directory behind with no kubeconfig entry, and treating that as "exists,
# just stopped" starts the corpse and then fails on a missing context — which reads
# as a broken script rather than as a half-finished create. Both have to be there.
if docker inspect "${NAME}-controlplane-1" >/dev/null 2>&1 &&
  kubectl config get-contexts -o name 2>/dev/null | grep -qx "$CTX"; then
  if [ "$(docker inspect -f '{{.State.Running}}' "${NAME}-controlplane-1")" = "false" ]; then
    step "cluster '${NAME}' exists but is stopped; starting it"
    for c in $(docker ps -a --filter "name=^${NAME}-" --format '{{.Names}}'); do
      docker start "$c" >/dev/null
    done
  else
    step "cluster '${NAME}' exists and is running"
  fi
else
  # Clear any half-created remains before creating, so a retry after a failed create
  # is just a retry rather than a manual cleanup someone has to work out.
  if docker inspect "${NAME}-controlplane-1" >/dev/null 2>&1; then
    step "clearing a half-created '${NAME}' before recreating it"
    talosctl cluster destroy --name "$NAME" >/dev/null 2>&1 || true
    docker ps -a --filter "name=^${NAME}-" --format '{{.Names}}' | xargs -r docker rm -f >/dev/null 2>&1 || true
    docker network rm "$NAME" >/dev/null 2>&1 || true
    rm -rf "${HOME}/.talos/clusters/${NAME}"
  fi

  # A PROXIED HOST (ADR-0600, and the Talos half of docs/guide/http-proxy.md).
  #
  # Talos nodes are containers and inherit nothing from the host's shell, so a
  # laptop behind a proxy produces a cluster that cannot pull. The failure does not
  # mention the proxy: the node reports `403 Forbidden` fetching
  # registry.k8s.io/etcd, etcd never starts, and `cluster create` times out on
  # "waiting for etcd to be healthy" — which reads as a broken cluster rather than
  # as a blocked network. Measured on exactly that host.
  #
  # The proxy is reached at the docker GATEWAY, never at 127.0.0.1: loopback inside
  # a node container is the node itself. NO_PROXY has to carry the pod and service
  # CIDRs and the node subnet, or the nodes proxy their own east-west traffic.
  #
  # Generated rather than committed: the address is per machine, and a committed
  # patch naming someone's proxy is wrong for everyone else.
  proxy_patch=""
  host_proxy="${HTTPS_PROXY:-${https_proxy:-}}"
  if [ -n "$host_proxy" ]; then
    proxy_url="$(printf '%s' "$host_proxy" | sed -E "s#//(127\\.0\\.0\\.1|localhost)#//${GATEWAY}#")"
    proxy_patch="$(mktemp)"
    trap 'rm -f "$proxy_patch"' EXIT
    cat >"$proxy_patch" <<PATCH
machine:
  env:
    http_proxy: ${proxy_url}
    https_proxy: ${proxy_url}
    no_proxy: localhost,127.0.0.1,10.96.0.0/12,10.244.0.0/16,${SUBNET},registry.localhost,.svc,.cluster.local
PATCH
    step "host proxy detected; nodes will reach it at ${proxy_url}"
  fi

  step "creating Talos cluster '${NAME}' (1 control plane, ${WORKERS} workers)"
  # Runs in the FOREGROUND, and that is worth stating because the obvious worry is
  # wrong: `talosctl cluster create` blocks on cluster health, and health normally
  # includes every node being Ready — which needs a CNI this tier deliberately does
  # not ship. It does not deadlock. Talos SKIPS the node-readiness, kube-proxy and
  # coredns checks when `cni: name: none` is set, and the create returns with the
  # nodes still NotReady. Measured: the run prints
  # `waiting for all k8s nodes to report ready: SKIP` and exits 0.
  talosctl cluster create docker \
    --name "$NAME" \
    --workers "$WORKERS" \
    --image "ghcr.io/siderolabs/talos:v${TALOS_VERSION}" \
    --kubernetes-version "$K8S_VERSION" \
    --subnet "$SUBNET" \
    --config-patch "@infra/talos/local/patch.yaml" \
    ${proxy_patch:+--config-patch "@${proxy_patch}"} \
    --exposed-ports "8080:30080/tcp,8443:30443/tcp" \
    --talosconfig-destination "$HOME/.talos/config"

  # etcd's own precondition is the ETCD volume, which is provisioned a few seconds
  # after the containers exist. Bootstrapping before it is ready returns success and
  # does nothing — measured: etcd sat in `Preparing / waiting for … etcd spec`
  # indefinitely, because the spec bootstrap produces was never written. So the
  # bootstrap is retried until the spec appears rather than issued once.
  step "waiting for the control plane's ETCD volume, then bootstrapping"
  for _ in $(seq 1 60); do
    if talosctl -n "$CP_IP" --talosconfig "$HOME/.talos/config" get etcdspec >/dev/null 2>&1; then
      break
    fi
    talosctl -n "$CP_IP" --talosconfig "$HOME/.talos/config" bootstrap >/dev/null 2>&1 || true
    sleep 5
  done
  talosctl -n "$CP_IP" --talosconfig "$HOME/.talos/config" get etcdspec >/dev/null 2>&1 ||
    fail "etcd never accepted the bootstrap; 'talosctl cluster destroy --name ${NAME}' and retry"
fi

# The nodes resolve registry.localhost through docker's embedded DNS on the
# cluster's own network, so the registry container has to be on it. The network is
# recreated with the cluster, so re-attach every time.
if docker network inspect "$NAME" >/dev/null 2>&1; then
  if ! docker inspect "$REGISTRY" --format \
    '{{range $k, $v := .NetworkSettings.Networks}}{{$k}} {{end}}' | grep -qw "$NAME"; then
    step "attaching ${REGISTRY} to the ${NAME} docker network"
    docker network connect "$NAME" "$REGISTRY"
  fi
fi

# Refresh the kubeconfig entry from the CLUSTER rather than trusting what is there.
# A recreated cluster reuses the name and mints a NEW CA, so a stale entry from the
# previous one fails with `x509: certificate signed by unknown authority` — which
# reads as a broken cluster rather than as a stale credential. `--force` replaces
# the entry instead of merging beside it.
step "refreshing the kubeconfig entry for ${CTX}"
talosctl --talosconfig "$HOME/.talos/config" -n "$CP_IP" kubeconfig --force >/dev/null

kubectl config use-context "$CTX" >/dev/null

# Nodes come up NotReady and stay there until a CNI exists — `cni: name: none` is
# the point of this tier — so Cilium is installed here rather than by the caller.
# Until it is, this cluster has no pod network and nothing can be deployed to it.
step "waiting for the API server"
for _ in $(seq 1 60); do
  if kubectl --context "$CTX" get nodes >/dev/null 2>&1; then break; fi
  sleep 5
done
kubectl --context "$CTX" get nodes >/dev/null 2>&1 ||
  fail "the API server did not answer; try 'talosctl cluster destroy --name ${NAME}' and re-run"

# Nodes register a little after the API answers, so this waits rather than asserting
# immediately — an empty `get nodes` right after bootstrap is normal, not a failure.
expected=$((WORKERS + 1))
step "waiting for ${expected} nodes to register"
for _ in $(seq 1 60); do
  actual="$(kubectl --context "$CTX" get nodes --no-headers 2>/dev/null | wc -l)"
  [ "$actual" -lt "$expected" ] || break
  sleep 5
done
actual="$(kubectl --context "$CTX" get nodes --no-headers 2>/dev/null | wc -l)"
[ "$actual" -eq "$expected" ] ||
  fail "expected ${expected} nodes, found ${actual}"

# Cilium, from the same committed chart the deployed environments use (ADR-0206).
# No value overrides: the chart's defaults ARE the Talos ones — KubePrism at
# localhost:7445 and the reduced agent capability set — so this tier runs the chart
# as written, which is the parity the tier exists for. It is the INNER loop that
# overrides them, in scripts/cilium-install.sh, because kind has no KubePrism.
step "installing Cilium (the CNI this tier's machine config deliberately omits)"
helm dependency update infra/helm/platform/cilium >/dev/null 2>&1 || true
helm --kube-context "$CTX" upgrade --install cilium infra/helm/platform/cilium \
  -n kube-system --set cilium.operator.replicas=1 --timeout 10m

step "waiting for the nodes to go Ready"
kubectl --context "$CTX" wait --for=condition=Ready node --all --timeout=600s

ok "cluster '${NAME}' ready (context ${CTX}, ${actual} nodes, Cilium up)"

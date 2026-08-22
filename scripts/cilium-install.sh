#!/usr/bin/env bash
# Install Cilium as the CNI on the local cluster (ADR-0206). A CNI must exist
# before any pod (including ArgoCD) can schedule, so this runs imperatively for
# both local tiers and is excluded from the local platform ApplicationSet. kind
# declines kube-proxy at create time (kubeProxyMode: none in the tier's kind config),
# so the chart's default kubeProxyReplacement=true applies. One operator replica (the default 2 needs 2 nodes for anti-affinity).
# Idempotent (helm upgrade --install).
set -euo pipefail

source "$(dirname "$0")/lib/cluster-ctx.sh"

CLUSTER="${CLUSTER:-platform}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

k() { kubectl --context "$(cluster_ctx)" "$@"; }
h() { helm --kube-context "$(cluster_ctx)" "$@"; }

# Already up? (helm release present and node Ready) → nothing to do.
if h -n kube-system status cilium >/dev/null 2>&1 &&
  k get node -o jsonpath='{.items[0].status.conditions[?(@.type=="Ready")].status}' 2>/dev/null | grep -q True; then
  echo "✓ Cilium already installed and node Ready"
  exit 0
fi

# The API server address is the ONE value this tier overrides. The chart points the
# agent at KubePrism (localhost:7445), which is Talos's per-node control-plane load
# balancer and exists only in a deployed environment (ADR-0206).
#
# The address here is the CONTROL PLANE NODE'S CONTAINER NAME, and both halves of
# that matter:
#
#   - not `127.0.0.1`, because loopback is the API server on the control-plane node
#     ONLY. On the full tier's workers nothing listens there, so their agents never
#     connect, those nodes never go Ready, and the failure reads as a broken CNI
#     rather than as a wrong address.
#   - not the node's docker IP, because docker reshuffles those across restarts and
#     a pinned IP then points the agent at whatever container took the address.
#
# The name resolves through docker's embedded DNS on the kind network, from the host
# network namespace the agent runs in — the same resolution path that lets a node
# reach `registry.localhost`, and it needs no CNI to work.
APISERVER="$(cluster_name)-control-plane"
echo "→ installing Cilium (apiserver ${APISERVER}:6443)"
helm dependency update infra/helm/platform/cilium >/dev/null

# Otherwise local runs the chart as-is, including WireGuard transparent encryption
# (ADR-0206) for east-west PII posture and the reduced agent capability set — same
# datapath as prod, so no local/prod parity gap.
h upgrade --install cilium infra/helm/platform/cilium -n kube-system \
  --set cilium.operator.replicas=1 \
  --set "cilium.k8sServiceHost=${APISERVER}" \
  --set cilium.k8sServicePort=6443 \
  --timeout 5m

echo "→ waiting for the node to go Ready (Cilium up)…"
k wait --for=condition=Ready node --all --timeout=600s

# hubble-relay CrashLoopBackOff after a stop/start survivor: hubble-peer is backed
# by the cilium-agent pod itself (a hostPort :4244). Cilium's eBPF LB owns that
# ClusterIP and health-gates the backend from the EndpointSlice's ready condition.
# On a stop/start the agent goes briefly NotReady → endpoint ready:false → Cilium
# marks the sole backend Unhealthy (quarantined) and, because the LB state is
# restored from the PINNED bpf map, never re-reconciles it —
# `service-no-backend-response: reject` then turns every relay peer-notify into an
# instant ECONNREFUSED, wedging hubble-relay permanently (a fresh build never hits
# the race, so it only bites after the first stop/start). The chart hard-codes
# hubble-peer with no publishNotReadyAddresses and exposes no knob, so patch it:
# keeping the peer endpoint always-ready across agent restarts stops the quarantine
# at the source. (Persists across stop/start; re-applied on every cluster:full when
# this script re-runs.)
k -n kube-system patch svc hubble-peer --type merge -p '{"spec":{"publishNotReadyAddresses":true}}'
echo "✓ Cilium installed; node Ready"

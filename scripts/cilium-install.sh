#!/usr/bin/env bash
# Install Cilium as the CNI on the local cluster (ADR-0206). A CNI must exist
# before any pod (including ArgoCD) can schedule, so this runs imperatively for
# both local tiers and is excluded from the local platform ApplicationSet. k3d
# keeps kube-proxy, so kubeProxyReplacement is off; one operator replica (the
# default 2 needs 2 nodes for anti-affinity). Idempotent (helm upgrade --install).
set -euo pipefail

CLUSTER="${CLUSTER:-platform}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

k() { kubectl --context "k3d-${CLUSTER}" "$@"; }
h() { helm --kube-context "k3d-${CLUSTER}" "$@"; }

# Already up? (helm release present and node Ready) → nothing to do.
if h -n kube-system status cilium >/dev/null 2>&1 &&
  k get node -o jsonpath='{.items[0].status.conditions[?(@.type=="Ready")].status}' 2>/dev/null | grep -q True; then
  echo "✓ Cilium already installed and node Ready"
  exit 0
fi

# k8sServiceHost stays 127.0.0.1 (chart default in values.yaml): the agent runs
# hostNetwork on the node, and k3d serves the API at loopback there. Do NOT pin it
# to the node's Docker InternalIP — k3d reshuffles that IP between the serverlb and
# server-0 containers on every stop/start, so a pinned InternalIP points the agent at
# the wrong container and the CNI never starts (connection refused to :6443).
echo "→ installing Cilium (apiserver 127.0.0.1:6443)"
helm dependency update infra/helm/platform/cilium >/dev/null

# Local runs the chart as-is, including WireGuard transparent encryption (ADR-0206)
# for east-west PII posture — same datapath as prod, so no local/prod parity gap.
h upgrade --install cilium infra/helm/platform/cilium -n kube-system \
  --set cilium.kubeProxyReplacement=false \
  --set cilium.operator.replicas=1 \
  --timeout 5m

echo "→ waiting for the node to go Ready (Cilium up)…"
k wait --for=condition=Ready node --all --timeout=600s

# hubble-relay CrashLoopBackOff after a stop/start survivor: hubble-peer is backed
# by the cilium-agent pod itself (a hostPort :4244). With kubeProxyReplacement off,
# Cilium's eBPF LB still owns that ClusterIP and health-gates the backend from the
# EndpointSlice's ready condition. On a stop/start the agent goes briefly NotReady →
# endpoint ready:false → Cilium marks the sole backend Unhealthy (quarantined) and,
# because the LB state is restored from the PINNED bpf map, never re-reconciles it —
# `service-no-backend-response: reject` then turns every relay peer-notify into an
# instant ECONNREFUSED, wedging hubble-relay permanently (a fresh build never hits the
# race, so it only bites after the first stop/start). The chart hard-codes hubble-peer
# with no publishNotReadyAddresses and exposes no knob, so patch it: keeping the peer
# endpoint always-ready across agent restarts stops the quarantine at the source.
# (Persists across stop/start; re-applied on every cluster:full when this script re-runs.)
k -n kube-system patch svc hubble-peer --type merge -p '{"spec":{"publishNotReadyAddresses":true}}'
echo "✓ Cilium installed; node Ready"

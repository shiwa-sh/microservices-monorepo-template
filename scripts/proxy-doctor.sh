#!/usr/bin/env bash
# Diagnose (and where it is safe, apply) this machine's proxy setup for the cluster
# tasks — the four steps of docs/guide/http-proxy.md, checked against what is
# actually configured rather than against what was intended.
#
# Why this exists: proxy configuration is a property of YOUR MACHINE, not of this
# template, and the repo deliberately carries no proxy values. That leaves a setup
# whose steps are read from four different places — the host, a build container, the
# cluster node, a cluster pod — and a loopback proxy has a DIFFERENT ADDRESS in each
# of them. Getting one wrong does not produce an error that names the proxy; it
# produces a hung image pull, a build that cannot reach a package registry, or an
# Argo CD app stuck at `Unknown`. This script does the address arithmetic and says
# which step is wrong.
#
#   mise run proxy:doctor          diagnose only; prints the exact fix for each gap
#   mise run proxy:doctor -- --fix apply every fix that does not need root, and
#                                  print a ready-to-paste block for the ones that do
#
# On a direct network it exits 0 and tells you none of this applies.
set -euo pipefail

source "$(dirname "$0")/lib/log.sh"

FIX=0
[[ "${1:-}" == "--fix" ]] && FIX=1

GAPS=0
SUDO_STEPS=""
gap() {
  GAPS=$((GAPS + 1))
  printf '✗ %s\n' "$1" >&2
}

# The NO_PROXY every layer needs. Cluster-internal traffic must stay direct, and
# fc00::/7 is not optional: docker's embedded DNS answers the kind node with its
# IPv6 ULA first, and without this the node's own kubelet dials the API server
# THROUGH the proxy and `kind create` aborts before the cluster is usable.
NO_PROXY_VALUE='10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,fc00::/7,.svc,.svc.cluster.local,.cluster.local,127.0.0.1,localhost,.localtest.me'

# ── Step 0: is there a proxy at all, and where does it live ───────────────────
PROXY="${HTTPS_PROXY:-${https_proxy:-}}"
if [[ -z "$PROXY" ]]; then
  # Not exported here, but it may still be configured for docker. Ask docker
  # before concluding the network is direct.
  PROXY="$(docker info --format '{{.HTTPSProxy}}' 2>/dev/null || true)"
fi
if [[ -z "$PROXY" ]]; then
  ok "no proxy configured anywhere this script can see"
  detail "If your network reaches ghcr.io, github.com and the chart repos directly,"
  detail "nothing in docs/guide/http-proxy.md applies and the cluster tasks work as-is."
  detail "If it does NOT, set HTTPS_PROXY to your proxy and run this again."
  exit 0
fi

PROXY_HOST="${PROXY#*://}"
PROXY_PORT="${PROXY_HOST##*:}"
PROXY_HOST="${PROXY_HOST%%:*}"
LOOPBACK=0
[[ "$PROXY_HOST" == "127.0.0.1" || "$PROXY_HOST" == "localhost" || "$PROXY_HOST" == "::1" ]] && LOOPBACK=1

step "proxy: $PROXY"
if ((LOOPBACK)); then
  detail "loopback — so each layer below needs a DIFFERENT address for the same proxy."
else
  detail "routable — the same address works from every layer."
fi

# Address of the proxy as seen from a container on a given docker network.
addr_from_network() {
  local net="$1" gw
  ((LOOPBACK)) || {
    printf '%s' "$PROXY"
    return
  }
  gw="$(docker network inspect "$net" -f '{{range .IPAM.Config}}{{.Gateway}} {{end}}' 2>/dev/null |
    tr ' ' '\n' | grep -E '^[0-9]+\.' | head -1)"
  [[ -n "$gw" ]] || return 1
  printf 'http://%s:%s' "$gw" "$PROXY_PORT"
}

# ── Step 1: the docker daemon, for image pulls ────────────────────────────────
step "step 1 — docker daemon (image pulls)"
if ! command -v docker >/dev/null 2>&1; then
  gap "docker not found; steps 1-3 cannot be checked"
else
  daemon_https="$(docker info --format '{{.HTTPSProxy}}' 2>/dev/null || true)"
  daemon_no="$(docker info --format '{{.NoProxy}}' 2>/dev/null || true)"
  if [[ -z "$daemon_https" ]]; then
    gap "the daemon has no HTTPS proxy — image pulls go direct and will hang"
    SUDO_STEPS="$SUDO_STEPS daemon"
  elif [[ "$daemon_no" != *"fc00::/7"* ]]; then
    gap "daemon NO_PROXY is missing fc00::/7 — 'kind create' will abort with an EOF"
    detail "currently: ${daemon_no:-<empty>}"
    SUDO_STEPS="$SUDO_STEPS daemon"
  else
    ok "daemon proxied, NO_PROXY covers fc00::/7"
  fi
fi

# ── Step 2: docker build ──────────────────────────────────────────────────────
step "step 2 — docker build (RUN steps inside a build container)"
DOCKER_CFG="${DOCKER_CONFIG:-$HOME/.docker}/config.json"
build_want="$(addr_from_network bridge || true)"
if [[ -z "$build_want" ]]; then
  gap "cannot read the docker bridge gateway; is the docker daemon running?"
else
  build_have="$(jq -r '.proxies.default.httpsProxy // ""' "$DOCKER_CFG" 2>/dev/null || echo "")"
  if [[ "$build_have" == "$build_want" ]]; then
    ok "build proxy set to $build_want"
  elif ((FIX)); then
    mkdir -p "$(dirname "$DOCKER_CFG")"
    [[ -f "$DOCKER_CFG" ]] || echo '{}' >"$DOCKER_CFG"
    tmp="$(mktemp)"
    jq --arg p "$build_want" --arg np "$NO_PROXY_VALUE" \
      '.proxies.default = {httpProxy: $p, httpsProxy: $p, noProxy: $np}' \
      "$DOCKER_CFG" >"$tmp" && mv "$tmp" "$DOCKER_CFG"
    ok "wrote proxies.default = $build_want to $DOCKER_CFG"
  else
    gap "build proxy is '${build_have:-<unset>}', should be $build_want"
    detail "a loopback value here is the classic build failure: inside the build"
    detail "container 127.0.0.1 is the container itself, so the package manager"
    detail "goes direct and the firewall blocks it. Fix: mise run proxy:doctor -- --fix"
  fi
fi

# ── Step 2b: the daemon's embedded resolver ───────────────────────────────────
step "step 2b — docker's embedded DNS"
NODE="${CLUSTER:-platform}-control-plane"
if ! docker inspect "$NODE" >/dev/null 2>&1; then
  detail "no '$NODE' container; skipped (nothing to check until a cluster exists)"
elif docker exec "$NODE" getent hosts github.com >/dev/null 2>&1; then
  ok "the node resolves github.com"
else
  gap "the node cannot resolve github.com while the host can — stale daemon resolver"
  detail "the daemon cached the host's nameservers at start; restart it once:"
  detail "  sudo systemctl restart docker"
fi

# ── Step 3: the shell that brings a cluster up ────────────────────────────────
step "step 3 — the shell you run cluster:full from"
if [[ -n "${HTTPS_PROXY:-${https_proxy:-}}" ]]; then
  ok "HTTPS_PROXY is exported here"
  detail "bring a cluster up with: mise run cluster:full"
else
  gap "HTTPS_PROXY is not exported in this shell"
  detail "  export HTTPS_PROXY=$PROXY"
fi

# ── Step 4: Argo CD's repo-server ─────────────────────────────────────────────
step "step 4 — Argo CD repo-server (chart repositories)"
repo_want="$(addr_from_network kind || true)"
if ! kubectl get deploy argocd-repo-server -n argocd >/dev/null 2>&1; then
  detail "no repo-server in this cluster; skipped"
elif [[ -z "$repo_want" ]]; then
  gap "cannot read the kind network gateway"
else
  repo_have="$(kubectl get deploy argocd-repo-server -n argocd \
    -o jsonpath='{range .spec.template.spec.containers[0].env[?(@.name=="HTTPS_PROXY")]}{.value}{end}' 2>/dev/null || true)"
  if [[ "$repo_have" == "$repo_want" ]]; then
    ok "repo-server proxied at $repo_want"
  elif ((FIX)); then
    step "applying repo-server proxy via helm"
    helm upgrade argocd infra/helm/platform/argocd -n argocd --reuse-values --timeout 8m \
      --set "argo-cd.repoServer.env[0].name=HTTPS_PROXY" \
      --set "argo-cd.repoServer.env[0].value=$repo_want" \
      --set "argo-cd.repoServer.env[1].name=HTTP_PROXY" \
      --set "argo-cd.repoServer.env[1].value=$repo_want" \
      --set "argo-cd.repoServer.env[2].name=NO_PROXY" \
      --set-string "argo-cd.repoServer.env[2].value=${NO_PROXY_VALUE//,/\\,}" >/dev/null
    kubectl rollout status deploy/argocd-repo-server -n argocd --timeout=180s >/dev/null
    ok "repo-server proxied at $repo_want"
    detail "Argo caches the generation error with the manifests, so a stuck app needs"
    detail "  argocd app get <app> --core --hard-refresh"
    detail "a plain --refresh replays the old ComparisonError and reads as a failed fix."
  else
    gap "repo-server has ${repo_have:-no proxy}, should be $repo_want"
    detail "without it, any chart with external dependencies: leaves its app at"
    detail "'Unknown' with a helm repo add timeout, while every other app syncs."
    detail "Fix: mise run proxy:doctor -- --fix"
  fi
fi

# ── The one thing that needs root ─────────────────────────────────────────────
if [[ "$SUDO_STEPS" == *daemon* ]]; then
  DROPIN="$(mktemp -d)/http-proxy.conf"
  cat >"$DROPIN" <<EOF
[Service]
Environment="HTTP_PROXY=$PROXY"
Environment="HTTPS_PROXY=$PROXY"
Environment="NO_PROXY=$NO_PROXY_VALUE"
EOF
  printf '\n'
  step "step 1 needs root. Written for you at $DROPIN — paste this:"
  detail "sudo install -Dm644 $DROPIN /etc/systemd/system/docker.service.d/http-proxy.conf"
  detail "sudo systemctl daemon-reload && sudo systemctl restart docker"
  detail "then re-run: mise run proxy:doctor -- --fix"
fi

printf '\n'
if ((GAPS == 0)); then
  ok "proxy setup complete — the cluster tasks should work unchanged"
  exit 0
fi
fail "$GAPS proxy gap(s) above; details in docs/guide/http-proxy.md"

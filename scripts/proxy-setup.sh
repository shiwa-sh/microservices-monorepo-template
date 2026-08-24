#!/usr/bin/env bash
# Converge THIS MACHINE for a firewalled network, once, before any cluster command
# (docs/guide/http-proxy.md). Nothing in scripts/cluster.sh knows this ran: on a
# direct network it exits 0 saying so, and the cluster commands are the same either way.
#
#   mise run proxy:setup            apply every fix that does not need root
#   mise run proxy:setup -- --check diagnose only
#
# Proxy values are a property of your machine, so the repo carries none. What makes
# this worth a script is the address arithmetic: the settings are read from four
# places — host shell, build container, cluster node, cluster pod — and a loopback
# proxy has a DIFFERENT address in each. Getting one wrong never names the proxy; it
# produces a hung pull, a build that cannot reach a package registry, or an Argo app
# stuck at Unknown.
#
# The node's containerd is deliberately absent from that list. It pulls only from the
# local zot, which reaches the proxy through the docker daemon (step 1), so one
# component holds image egress (ADR-0105). Step 1 is therefore the one that matters
# for images: get it wrong and `cluster:up` fails while warming the registry, before
# it creates a cluster.
set -euo pipefail

source "$(dirname "$0")/lib/log.sh"

APPLY=1
[[ "${1:-}" == "--check" ]] && APPLY=0

GAPS=0
NEEDS_ROOT=0
gap() {
  GAPS=$((GAPS + 1))
  printf '✗ %s\n' "$1" >&2
}

# Cluster-internal traffic must stay direct. fc00::/7 is not optional: docker's
# embedded DNS answers the node with its IPv6 ULA first, and without it the node's
# kubelet dials the API server THROUGH the proxy and `kind create` aborts.
NO_PROXY_VALUE='10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,fc00::/7,.svc,.svc.cluster.local,.cluster.local,127.0.0.1,localhost,.localtest.me'

PROXY="${HTTPS_PROXY:-${https_proxy:-}}"
# Not exported here does not mean absent — ask docker before concluding.
[[ -n "$PROXY" ]] || PROXY="$(docker info --format '{{.HTTPSProxy}}' 2>/dev/null || true)"
if [[ -z "$PROXY" ]]; then
  ok "no proxy configured anywhere this script can see"
  detail "If your network reaches ghcr.io, github.com and the chart repos directly,"
  detail "none of this applies: run 'mise run cluster:up' as-is."
  detail "If it does not, export HTTPS_PROXY and run this again."
  exit 0
fi

PROXY_HOSTPORT="${PROXY#*://}"
PROXY_PORT="${PROXY_HOSTPORT##*:}"
PROXY_HOST="${PROXY_HOSTPORT%%:*}"
LOOPBACK=0
[[ "$PROXY_HOST" =~ ^(127\.0\.0\.1|localhost|::1)$ ]] && LOOPBACK=1

step "proxy: ${PROXY}"
if ((LOOPBACK)); then
  detail "loopback — each layer below needs a different address for the same proxy"
else
  detail "routable — the same address works from every layer"
fi

# The proxy as seen from a container on a given docker network.
addr_from_network() {
  local gw
  ((LOOPBACK)) || {
    printf '%s' "$PROXY"
    return
  }
  gw="$(docker network inspect "$1" -f '{{range .IPAM.Config}}{{.Gateway}} {{end}}' 2>/dev/null |
    tr ' ' '\n' | grep -E '^[0-9]+\.' | head -1)"
  [[ -n "$gw" ]] || return 1
  printf 'http://%s:%s' "$gw" "$PROXY_PORT"
}

step "step 1 — the docker daemon (image pulls, and zot's own egress)"
if ! command -v docker >/dev/null 2>&1; then
  gap "docker not found; steps 1-4 cannot be checked"
else
  daemon_proxy="$(docker info --format '{{.HTTPSProxy}}' 2>/dev/null || true)"
  daemon_no="$(docker info --format '{{.NoProxy}}' 2>/dev/null || true)"
  if [[ -z "$daemon_proxy" ]]; then
    gap "the daemon has no HTTPS proxy — image pulls go direct and hang"
    NEEDS_ROOT=1
  elif [[ "$daemon_no" != *"fc00::/7"* ]]; then
    gap "daemon NO_PROXY is missing fc00::/7 — 'kind create' aborts with an EOF"
    detail "currently: ${daemon_no:-<empty>}"
    NEEDS_ROOT=1
  else
    ok "daemon proxied, NO_PROXY covers fc00::/7"
  fi
fi

step "step 2 — docker build (RUN steps inside a build container)"
DOCKER_CFG="${DOCKER_CONFIG:-$HOME/.docker}/config.json"
if ! build_want="$(addr_from_network bridge)"; then
  gap "cannot read the docker bridge gateway; is the daemon running?"
else
  build_have="$(jq -r '.proxies.default.httpsProxy // ""' "$DOCKER_CFG" 2>/dev/null || true)"
  if [[ "$build_have" == "$build_want" ]]; then
    ok "build proxy set to ${build_want}"
  elif ((APPLY)); then
    mkdir -p "$(dirname "$DOCKER_CFG")"
    [[ -f "$DOCKER_CFG" ]] || echo '{}' >"$DOCKER_CFG"
    tmp="$(mktemp)"
    jq --arg p "$build_want" --arg np "$NO_PROXY_VALUE" \
      '.proxies.default = {httpProxy: $p, httpsProxy: $p, noProxy: $np}' \
      "$DOCKER_CFG" >"$tmp" && mv "$tmp" "$DOCKER_CFG"
    ok "wrote proxies.default = ${build_want} to ${DOCKER_CFG}"
  else
    # A loopback value here is the classic build failure: inside the build container
    # 127.0.0.1 is the container, so the package manager goes direct and is blocked.
    gap "build proxy is '${build_have:-<unset>}', should be ${build_want}"
  fi
fi

step "step 3 — docker's embedded resolver"
NODE="${CLUSTER:-platform}-control-plane"
if ! docker inspect "$NODE" >/dev/null 2>&1; then
  detail "no '${NODE}' container — nothing to check until a cluster exists"
elif docker exec "$NODE" getent hosts github.com >/dev/null 2>&1; then
  ok "the node resolves github.com"
else
  # The daemon cached the host's nameservers at start.
  gap "the node cannot resolve github.com while the host can — stale daemon resolver"
  detail "sudo systemctl restart docker"
fi

step "step 4 — ArgoCD's repo-server (chart repositories, full tier only)"
if ! kubectl get deploy argocd-repo-server -n argocd >/dev/null 2>&1; then
  detail "no repo-server in this cluster — skipped"
elif ! repo_want="$(addr_from_network kind)"; then
  gap "cannot read the kind network gateway"
else
  repo_have="$(kubectl -n argocd get deploy argocd-repo-server \
    -o jsonpath='{range .spec.template.spec.containers[0].env[?(@.name=="HTTPS_PROXY")]}{.value}{end}' 2>/dev/null || true)"
  if [[ "$repo_have" == "$repo_want" ]]; then
    ok "repo-server proxied at ${repo_want}"
  elif ((APPLY)); then
    step "applying the repo-server proxy"
    helm upgrade argocd infra/helm/platform/argocd -n argocd --reuse-values --timeout 8m \
      --set "argo-cd.repoServer.env[0].name=HTTPS_PROXY" \
      --set "argo-cd.repoServer.env[0].value=${repo_want}" \
      --set "argo-cd.repoServer.env[1].name=HTTP_PROXY" \
      --set "argo-cd.repoServer.env[1].value=${repo_want}" \
      --set "argo-cd.repoServer.env[2].name=NO_PROXY" \
      --set-string "argo-cd.repoServer.env[2].value=${NO_PROXY_VALUE//,/\\,}" >/dev/null
    kubectl -n argocd rollout status deploy/argocd-repo-server --timeout=180s >/dev/null
    ok "repo-server proxied at ${repo_want}"
    # Argo caches the generation error with the manifests, so a plain --refresh
    # replays it and reads as a failed fix.
    detail "un-stick an app with: argocd app get <app> --core --hard-refresh"
  else
    # Without it, any chart with external dependencies: leaves its app at Unknown on
    # a helm-repo timeout while every other app syncs.
    gap "repo-server has ${repo_have:-no proxy}, should be ${repo_want}"
  fi
fi

if ((NEEDS_ROOT)); then
  DROPIN="$(mktemp -d)/http-proxy.conf"
  cat >"$DROPIN" <<EOF
[Service]
Environment="HTTP_PROXY=${PROXY}"
Environment="HTTPS_PROXY=${PROXY}"
Environment="NO_PROXY=${NO_PROXY_VALUE}"
EOF
  printf '\n'
  step "step 1 needs root — written for you, paste this:"
  detail "sudo install -Dm644 ${DROPIN} /etc/systemd/system/docker.service.d/http-proxy.conf"
  detail "sudo systemctl daemon-reload && sudo systemctl restart docker"
  detail "then re-run: mise run proxy:setup"
fi

printf '\n'
((GAPS == 0)) || fail "${GAPS} gap(s) above; details in docs/guide/http-proxy.md"
ok "proxy setup complete — 'mise run cluster:up' works unchanged from here"

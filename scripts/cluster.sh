#!/usr/bin/env bash
# The local cluster, one entrypoint (ADR-0600). Stages live in lib/cluster.sh.
#
#   cluster.sh up [base|full]     create, resume or repair the tier
#   cluster.sh stop [base|full]   halt the nodes, keep the image cache
#   cluster.sh down [base|full]   delete the cluster
#   cluster.sh heal [base|full]   restart the nodes and rebuild the datapath
#   cluster.sh status [base|full] what is running, and where to reach it
#   cluster.sh add <name>         one service, app or platform chart, from the working tree
#   cluster.sh remove <name>      the same, uninstalled and handed back to GitOps
#   cluster.sh glue [name port]   re-stamp the host edge glue
#
# The tier is a stage list, not a branch. Both are kind, one node each; what
# separates them is which workloads run and what drives them — cluster:up base is
# imperative, cluster:up full is ArgoCD syncing committed master (ADR-0205).
#
# Nothing here knows about HTTP proxies. A firewalled machine converges its
# environment once with proxy:setup and then runs these commands unchanged.
set -euo pipefail

source "$(dirname "$0")/lib/cluster.sh"
cd "$ROOT"

require_tools docker kind kubectl helm

# Every bring-up shares this floor; the tier adds to it.
PRELUDE=(registry warm cluster cni coredns namespaces edge glue)
STAGES_base=(postgres certs priority middlewares secrets ory identities)
STAGES_full=(sopskey images argocd rootapp populatezot identities)

run_stages() {
  local stage
  for stage in "$@"; do "stage_${stage}"; done
}

epilogue_base() {
  cat <<EOF

✓ cluster:up base — real edge, real identity, Postgres.

  Add what you need on top; nothing else is running:
    cd services/<svc> && mise run server    native, pulls its own deps in
    mise run cluster:add -- <svc>           in-cluster, from the working tree
    mise run mock:start                     /api from the committed OpenAPI

  Open:       https://${DOMAIN}:8443/
  Log in:     $(login_hint)
  Teardown:   mise run cluster:stop (keep cache) / cluster:down (delete)

  Persistence is ephemeral here and no workflow runs without dep:temporal. Assert
  persistence, Temporal behaviour and authorization decisions on the full tier.
EOF
}

epilogue_full() {
  cat <<EOF

✓ cluster:up full — ArgoCD-driven from master.
  Product:      https://${DOMAIN}:8443/api/<resource>/
  Log in:       https://${DOMAIN}:8443/auth/login — $(login_hint)
  Ops tier:     https://<name>.ops.${DOMAIN}:8443/ — argocd, grafana, hubble,
                temporal, seaweedfs, lowdefy, headlamp, pgweb (ADR-0306)
  Frontend:     run it natively on :3000; the edge glue routes /auth and the
                landing page to the host.
  Diagnose:     mise run cluster:status
  Break-glass:  kubectl port-forward with your kubeconfig (docs/guide/break-glass.md)
  Teardown:     mise run cluster:stop (keep cache) / cluster:down (delete)
EOF
}

# The credentials are committed once, in the e2e fixtures; read them rather than
# repeating them here.
login_hint() {
  bun --silent -e \
    "const {ADMIN} = await import('${ROOT}/test/e2e/fixtures/identities.ts'); console.log(ADMIN.email + ' / ' + ADMIN.password)" \
    2>/dev/null || echo '<see test/e2e/fixtures/identities.ts>'
}

# `add` and `remove` resolve the same three namespaces, and have to agree: an `add`
# that succeeds and a `remove` that refuses to undo it is worse than neither.
classify() { # <name> → service | app | chart
  local name="$1" kinds=()
  [ -d "services/${name}" ] && [ "${name#_}" = "$name" ] && kinds+=(service)
  # An app is a deployable only when it has a local values file — which is what
  # tells apps/frontend (the service chart deploys it) from apps/admin (baked into
  # the lowdefy chart).
  [ -d "apps/${name}" ] && [ -f "infra/gitops/services/local/values/${name}.yaml" ] && kinds+=(app)
  [ -d "infra/helm/platform/${name}" ] && kinds+=(chart)

  case "${#kinds[@]}" in
  1) printf '%s' "${kinds[0]}" ;;
  0)
    detail "services:        $(find services -mindepth 1 -maxdepth 1 -type d -not -name '_*' -printf '%f ')"
    detail "platform charts: $(find infra/helm/platform -mindepth 1 -maxdepth 1 -type d -printf '%f ')"
    fail "'${name}' is not a service, an app, or a platform chart"
    ;;
  # A name in two namespaces is a repo bug, not a user error. Keeping them disjoint
  # is what makes this dispatch unambiguous.
  *) fail "'${name}' is a ${kinds[*]} at once — rename one; the namespaces must stay disjoint" ;;
  esac
}

VERB="${1:-up}"
shift || true

# The tier names the cluster, so every verb that acts on one takes it the same way.
# `add`, `remove` and `glue` are excluded: their first argument is a name, and a
# service called `full` would otherwise be read as a tier. TIER stays the fallback.
case "$VERB" in
up | stop | down | heal | status)
  case "${1:-}" in
  base | full)
    TIER="$1"
    shift
    ;;
  "") ;;
  *) fail "'$1' is not a tier — use \"base\" or \"full\"" ;;
  esac
  ;;
esac

case "$VERB" in
up)
  step "bringing up the ${TIER} tier as cluster '$(cluster_name)'"
  run_stages "${PRELUDE[@]}"
  case "$TIER" in
  base) run_stages "${STAGES_base[@]}" && epilogue_base ;;
  full) run_stages "${STAGES_full[@]}" && epilogue_full ;;
  esac
  ;;

stop)
  # `docker stop` keeps the containers, so each node's containerd image cache and
  # its volumes survive and the next `up` resumes cold-pull-free. kind has no stop
  # of its own. Every node: stopping one of a multi-node cluster leaves the rest
  # holding memory for a cluster nobody is using.
  name="$(cluster_name)"
  if ! cluster_exists "$name"; then
    ok "cluster '${name}' does not exist — nothing to stop"
    other_tier_hint stop
    exit 0
  fi
  mapfile -t nodes < <(kind get nodes --name "$name")
  step "stopping '${name}' — ${#nodes[@]} node(s), image cache pr eserved"
  printf '%s\n' "${nodes[@]}" | xargs -r docker stop >/dev/null
  ok "stopped; resume with '$(up_hint)'"
  ;;

down)
  name="$(cluster_name)"
  # kind exits 0 deleting a cluster that does not exist, so the check is here or the
  # verb reports a deletion it never performed.
  if ! cluster_exists "$name"; then
    ok "cluster '${name}' does not exist — nothing to delete"
    other_tier_hint down
    exit 0
  fi
  step "deleting kind cluster '${name}'"
  kind delete cluster --name "$name"
  ok "deleted '${name}' (the registry container survives — it is host-level)"
  ;;

heal)
  # A host reboot replays the node container raw: Cilium's per-node datapath can be
  # half-restored (pods then cannot reach the API, which cascades to CoreDNS and
  # every controller), and the docker-bridge gateway can have moved under the edge
  # glue. Restarting the node re-runs its entrypoint cleanly; forcing those stages
  # rebuilds what the restart alone leaves stale.
  name="$(cluster_name)"
  if ! docker inspect "${name}-control-plane" >/dev/null 2>&1; then
    other_tier_hint heal
    fail "cluster '${name}' does not exist — nothing to heal"
  fi
  step "restarting the nodes of '${name}'"
  kind get nodes --name "$name" | xargs -r docker restart >/dev/null
  # Poll before any RBAC-gated call: during the cold-start window the API server can
  # answer Forbidden for list/watch, which is fatal under set -e.
  wait_for "the API server" 180 kubectl --context "$(cluster_ctx)" get --raw /healthz
  k wait --for=condition=Ready node --all --timeout=300s
  k -n kube-system rollout restart ds/cilium >/dev/null
  k -n kube-system rollout status ds/cilium --timeout=180s
  # CoreDNS is Ready only once a pod can actually reach the API, so it is the honest
  # datapath probe rather than a component that happens to be restarted.
  k -n kube-system rollout restart deploy/coredns >/dev/null
  k -n kube-system rollout status deploy/coredns --timeout=180s ||
    fail "CoreDNS is still not Ready — the Cilium datapath is wedged deeper than a
  restart clears. Recreate the cluster: mise run cluster:down -- ${TIER} && $(up_hint)"
  run_stages glue
  ok "heal complete — pods can reach the API again"
  ;;

status)
  name="$(cluster_name)"
  step "tier ${TIER} · cluster '${name}' · context $(cluster_ctx)"
  if ! cluster_exists "$name"; then
    warn "not created — run '$(up_hint)'"
    other_tier_hint status
    exit 0
  fi
  detail "registry: $(docker inspect -f '{{.State.Status}}' "$REGISTRY" 2>/dev/null || echo absent) (${REGISTRY}:5000)"
  k get nodes
  if k get ns argocd >/dev/null 2>&1; then
    k -n argocd get applications.argoproj.io \
      -o custom-columns='APP:.metadata.name,SYNC:.status.sync.status,HEALTH:.status.health.status'
  else
    h -n "$NS" list --short
  fi
  k get pods -A --field-selector=status.phase!=Running,status.phase!=Succeeded 2>/dev/null | grep . ||
    ok "every pod is Running or Succeeded"
  ;;

add)
  NAME="${1:?usage: mise run cluster:add -- <service|app|chart>}"
  # The two paths share only a spine (pause Argo, then helm upgrade --take-ownership
  # with the local values). A service resolves to a values file and always builds an
  # image; a chart resolves to a directory and needs the auth overlays. Unify the
  # interface, leave the implementations where the knowledge lives.
  case "$(classify "$NAME")" in
  service | app) exec bash scripts/service-deploy.sh "$NAME" ;;
  chart) exec bash scripts/platform-deploy.sh "$NAME" ;;
  esac
  ;;

remove)
  NAME="${1:?usage: mise run cluster:remove -- <service|app|chart>}"
  require_cluster
  KIND="$(classify "$NAME")"

  if [ "$KIND" = service ]; then
    # The native edge glue is labelled, so it comes out by selector — including when
    # the service was never deployed at all.
    if k -n "$NS" get service,ingressroute,middleware,endpointslice \
      -l "local.platform/glue=${NAME}" -o name 2>/dev/null | grep -q .; then
      step "removing the native edge glue for ${NAME}"
      k -n "$NS" delete service,ingressroute,middleware,endpointslice \
        -l "local.platform/glue=${NAME}" --ignore-not-found >/dev/null
    fi
    # A svc:* port-forward outlives the mise task that started it, so removal reaps
    # it too; otherwise a later svc:* guard probes an open port backed by nothing.
    PIDFILE="${XDG_RUNTIME_DIR:-${TMPDIR:-/tmp}}/platform-local/portforward-${NAME}.pid"
    if [ -f "$PIDFILE" ]; then
      step "stopping the ${NAME} port-forward"
      kill "$(cat "$PIDFILE")" 2>/dev/null || true
      rm -f "$PIDFILE"
    fi
  fi

  if h -n "$NS" status "$NAME" >/dev/null 2>&1; then
    step "uninstalling ${NAME}"
    h -n "$NS" uninstall "$NAME"
  else
    detail "no helm release '${NAME}' in ${NS} — nothing to uninstall"
  fi

  # Hand it back to GitOps, mirroring the pause in the deploy scripts. Removal must
  # be symmetric with add, or the cluster drifts out of Argo's control one service
  # at a time with nothing saying why.
  case "$KIND" in
  chart) APP="local-platform-${NAME}" ;;
  *) APP="$(argo_service_app "$NAME")" ;;
  esac
  if [ -n "$APP" ] && k -n argocd get application.argoproj.io "$APP" >/dev/null 2>&1; then
    step "restoring ArgoCD auto-sync on ${APP}"
    k -n argocd patch application.argoproj.io "$APP" --type merge \
      -p '{"spec":{"syncPolicy":{"automated":{"prune":true,"selfHeal":true}}}}' >/dev/null
    detail "Argo will re-sync ${NAME} from committed master"
  fi
  ok "${NAME} removed"
  ;;

glue)
  require_cluster
  stage_glue "$@"
  ;;

*)
  fail "unknown verb '${VERB}' — up | stop | down | heal | status | add | remove | glue"
  ;;
esac

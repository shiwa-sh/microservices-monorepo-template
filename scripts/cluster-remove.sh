#!/usr/bin/env bash
# The counterpart to cluster:add (ADR-0600):
#
#   mise run cluster:remove -- orders   # a service, or its native edge glue
#   mise run cluster:remove -- ory      # a platform chart
#
# ── Why this is not just `helm uninstall` ─────────────────────────────────────
# The add path PAUSES ArgoCD auto-sync on the matching app so self-heal cannot
# revert a working-tree image. A bare uninstall therefore leaves the thing gone AND
# unmanaged: a later cluster:full will not bring it back until someone re-patches
# the sync policy by hand, and nothing tells you that is why. So removal restores
# GitOps as part of the same command — the add/remove pair has to be symmetric or
# the cluster quietly drifts out of Argo's control one service at a time.
set -euo pipefail

source "$(dirname "$0")/lib/log.sh"
source "$(dirname "$0")/lib/argo.sh"

CLUSTER="${CLUSTER:-platform}"
source "$(dirname "$0")/lib/cluster-ctx.sh"
NS="platform"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

NAME="${1:?usage: mise run cluster:remove -- <service|chart>}"

k() { kubectl --context "$(cluster_ctx)" "$@"; }
h() { helm --kube-context "$(cluster_ctx)" "$@"; }

is_service=false
is_app=false
is_chart=false
[ -d "services/${NAME}" ] && [ "${NAME#_}" = "${NAME}" ] && is_service=true
# Same rule cluster-add.sh uses: an app is a deployable when it has local values.
# The two scripts have to agree, or `cluster:add -- frontend` succeeds and
# `cluster:remove -- frontend` refuses to undo it.
[ -d "apps/${NAME}" ] && [ -f "infra/gitops/services/local/values/${NAME}.yaml" ] && is_app=true
[ -d "infra/helm/platform/${NAME}" ] && is_chart=true

if [ "$is_chart" = true ] && { [ "$is_service" = true ] || [ "$is_app" = true ]; }; then
  fail "'${NAME}' is both a platform chart and a service/app — rename one; the namespaces must stay disjoint"
fi
[ "$is_service" = true ] || [ "$is_app" = true ] || [ "$is_chart" = true ] ||
  fail "unknown: '${NAME}' is not a service, an app, or a platform chart"

# From here on, a service and an app are the same thing: one chart, one release,
# one Argo application. Only the glue and port-forward reaping below is
# service-only, because only a service has a native run mode.
is_workload=false
{ [ "$is_service" = true ] || [ "$is_app" = true ]; } && is_workload=true

# The native edge glue (scripts/service-dev.sh) is labelled, so it comes out by
# selector — including the case where the service was never deployed at all.
if [ "$is_service" = true ]; then
  if k -n "$NS" get service,ingressroute,middleware,endpointslice \
    -l "local.platform/glue=${NAME}" -o name 2>/dev/null | grep -q .; then
    step "removing the native edge glue for ${NAME}"
    k -n "$NS" delete service,ingressroute,middleware,endpointslice \
      -l "local.platform/glue=${NAME}" --ignore-not-found
  fi
fi

# A svc:* port-forward outlives the mise task that started it (see svc-apply.sh),
# so removal has to reap it too — otherwise the forward survives the uninstall and
# a later svc:* guard probes an open port backed by nothing.
if [ "$is_service" = true ]; then
  RUNDIR="${XDG_RUNTIME_DIR:-${TMPDIR:-/tmp}}/platform-local"
  PIDFILE="${RUNDIR}/portforward-${NAME}.pid"
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

# Hand it back to GitOps. Mirrors the pause in service-deploy.sh / platform-deploy.sh.
if [ "$is_workload" = true ]; then
  APP="$(argo_service_app "$NAME")"
else
  APP="local-platform-${NAME}"
fi
if k -n argocd get application.argoproj.io "$APP" >/dev/null 2>&1; then
  step "restoring ArgoCD auto-sync on ${APP}"
  k -n argocd patch application.argoproj.io "$APP" --type merge \
    -p '{"spec":{"syncPolicy":{"automated":{"prune":true,"selfHeal":true}}}}'
  detail "Argo will re-sync ${NAME} from committed master"
fi

ok "${NAME} removed"

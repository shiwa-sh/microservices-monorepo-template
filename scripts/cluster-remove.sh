#!/usr/bin/env bash
# The counterpart to cluster:add (ADR-0030):
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

CLUSTER="${CLUSTER:-platform}"
NS="platform"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

NAME="${1:?usage: mise run cluster:remove -- <service|chart>}"

k() { kubectl --context "k3d-${CLUSTER}" "$@"; }
h() { helm --kube-context "k3d-${CLUSTER}" "$@"; }

is_service=false
is_chart=false
[ -d "services/${NAME}" ] && [ "${NAME#_}" = "${NAME}" ] && is_service=true
[ -d "infra/helm/platform/${NAME}" ] && is_chart=true

if [ "$is_service" = true ] && [ "$is_chart" = true ]; then
  fail "'${NAME}' is both a service and a platform chart — rename one; the two namespaces must stay disjoint"
fi
[ "$is_service" = true ] || [ "$is_chart" = true ] ||
  fail "unknown: '${NAME}' is neither a service nor a platform chart"

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
if [ "$is_service" = true ]; then
  APP="local-service-${NAME}"
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

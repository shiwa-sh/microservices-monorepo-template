#!/usr/bin/env bash
# Route a cluster lifecycle verb to the ACTIVE TIER's provisioner (ADR-0600).
#
#   bash scripts/cluster-dispatch.sh ensure|delete|stop
#
# The two tiers run different distributions, so they have different provisioners
# and different lifecycle commands. Every caller goes through here rather than
# naming `kind` or `talosctl` directly, which is what keeps `TIER=full mise run
# cluster:delete` from deleting the inner loop's cluster.
set -euo pipefail

source "$(dirname "$0")/lib/log.sh"
source "$(dirname "$0")/lib/cluster-ctx.sh"

VERB="${1:?usage: cluster-dispatch.sh ensure|delete|stop}"
NAME="$(cluster_name)"
TIER_NAME="$(cluster_tier)"

case "$VERB" in
ensure)
  if [ "$TIER_NAME" = "full" ]; then
    exec bash "$(dirname "$0")/cluster-talos-ensure.sh"
  fi
  exec bash "$(dirname "$0")/cluster-ensure.sh"
  ;;
delete)
  if [ "$TIER_NAME" = "full" ]; then
    step "deleting Talos cluster '${NAME}'"
    talosctl cluster destroy --name "$NAME" || true
    # The provisioner leaves the state directory behind when destroy fails part
    # way, and a stale one makes the next create look like an existing cluster.
    rm -rf "${HOME}/.talos/clusters/${NAME}"
    ok "deleted '${NAME}'"
    exit 0
  fi
  step "deleting kind cluster '${NAME}'"
  kind delete cluster --name "$NAME"
  ;;
stop)
  if [ "$TIER_NAME" = "full" ]; then
    step "stopping Talos cluster '${NAME}'"
    docker ps --filter "name=^${NAME}-" --format '{{.Names}}' | xargs -r docker stop >/dev/null
    ok "stopped '${NAME}' (containers kept)"
    exit 0
  fi
  exec bash "$(dirname "$0")/cluster-stop.sh"
  ;;
*)
  fail "unknown verb '${VERB}' — use ensure, delete or stop"
  ;;
esac

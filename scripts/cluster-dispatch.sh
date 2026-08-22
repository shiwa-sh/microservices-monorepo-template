#!/usr/bin/env bash
# Route a cluster lifecycle verb to the ACTIVE TIER's cluster (ADR-0600).
#
#   bash scripts/cluster-dispatch.sh ensure|delete|stop
#
# Both tiers are kind, so this is not a provisioner switch — it is the one place
# that resolves WHICH cluster a verb acts on. Every caller goes through it rather
# than naming a cluster directly, which is what keeps `TIER=full mise run
# cluster:delete` from deleting the inner loop's.
set -euo pipefail

source "$(dirname "$0")/lib/log.sh"
source "$(dirname "$0")/lib/cluster-ctx.sh"

VERB="${1:?usage: cluster-dispatch.sh ensure|delete|stop}"
NAME="$(cluster_name)"

case "$VERB" in
ensure)
  exec bash "$(dirname "$0")/cluster-ensure.sh"
  ;;
delete)
  step "deleting kind cluster '${NAME}'"
  kind delete cluster --name "$NAME"
  ok "deleted '${NAME}'"
  ;;
stop)
  exec bash "$(dirname "$0")/cluster-stop.sh"
  ;;
*)
  fail "unknown verb '${VERB}' — use ensure, delete or stop"
  ;;
esac

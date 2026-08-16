# shellcheck shell=bash
source "$(dirname "${BASH_SOURCE[0]}")/cluster-ctx.sh"
# Preflight assertions for scripts that probe the cluster. Source it, don't execute:
#   source "$(dirname "$0")/lib/preflight.sh"
#   require_cluster "$CLUSTER"
#
# ── Why this exists ───────────────────────────────────────────────────────────
# The idempotence guards in dep-apply.sh and svc-apply.sh ask "is this already up?"
# by running a kubectl probe and treating a non-zero exit as NO. That is correct
# when the answer really is no, and dangerous when the probe could not run at all:
# a missing kubectl or an unreachable cluster looks identical to "not deployed", so
# the guard falls through to the install path and starts building images against
# nothing.
#
# Found the honest way — by running a script outside `mise exec`, where kubectl is
# not on PATH, and watching it announce it was deploying a service that was already
# running. Asserting the probe CAN run separates the two answers.
if [[ -n "${__PREFLIGHT_SH_LOADED:-}" ]]; then return 0 2>/dev/null || true; fi
__PREFLIGHT_SH_LOADED=1

# require_cluster — fail unless kubectl exists and the local cluster answers. The
# cluster name comes from CLUSTER (the same env cluster_ctx resolves); the probe
# asserts the probe CAN run, which is what separates "not deployed" from "cannot
# check". Cheap enough to call on every guarded script.
require_cluster() {
  command -v kubectl >/dev/null 2>&1 ||
    fail "kubectl not found on PATH — run this through mise (\`mise run …\`), which puts the pinned toolchain there"

  kubectl --context "$(cluster_ctx)" cluster-info >/dev/null 2>&1 ||
    fail "cluster $(cluster_ctx) is not reachable — run 'mise run cluster:base' first"
}

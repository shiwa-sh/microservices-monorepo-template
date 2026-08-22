# shellcheck shell=bash
# Resolve the local cluster's identity (ADR-0600). Source it, don't execute:
#   source "$(dirname "$0")/lib/cluster-ctx.sh"
#
# There are TWO local clusters, both on kind (ADR-0600):
#
#   base  the inner loop      cluster `<name>`
#   full  the whole platform  cluster `<name>-full`
#
# A tier is a set of workloads, not a distribution and not a node count: both are a
# single kind node. They are two
# clusters rather than one so neither bring-up disturbs the other, and every script
# resolves through these functions rather than hardcoding a name — which is what
# stops `mise run cluster:delete` in one tier taking the other with it.
#
# The tier comes from TIER, defaulting to the inner loop: the common case is a
# developer running one service, and the full tier is entered deliberately.
# shellcheck disable=SC2317  # the guard's `return` is reached when re-sourced.
if [[ -n "${__CLUSTER_CTX_SH_LOADED:-}" ]]; then return 0 2>/dev/null || true; fi
__CLUSTER_CTX_SH_LOADED=1

# The tier is validated HERE, at source time, and not inside the functions below.
#
# A check inside `cluster_ctx` cannot stop anything: the functions are called as
# `$(cluster_ctx)`, and an `exit` inside a command substitution kills only the
# subshell — the caller carries on with an empty context and kubectl then talks to
# whatever the current-context happens to be. Failing while the library loads is the
# only point where refusing actually refuses.
case "${TIER:=base}" in
base | full) ;;
*)
  printf '✗ TIER=%s is not a tier — use "base" or "full"\n' "$TIER" >&2
  exit 1
  ;;
esac

# cluster_tier — prints the active tier, "base" or "full".
cluster_tier() { printf '%s' "$TIER"; }

# cluster_name — prints the cluster's name for the active tier.
#
# The full tier's name carries the tier, so `docker ps` and `kubectl config
# get-contexts` both say which cluster is which without anyone having to remember.
cluster_name() {
  if [ "$TIER" = "full" ]; then
    printf '%s-full' "${CLUSTER:-platform}"
  else
    printf '%s' "${CLUSTER:-platform}"
  fi
}

# cluster_ctx — prints the kube context name for the active tier.
#
# kind prefixes its context with `kind-` and that prefix is not configurable, so the
# context is never the cluster name. Callers ask for it here rather than assembling
# it, which is the whole reason this function survives now that one provisioner
# serves both tiers.
cluster_ctx() { printf 'kind-%s' "$(cluster_name)"; }

# cluster_kind_config — prints the kind config both tiers are created from.
#
# ONE file, because the tiers no longer differ in node shape (ADR-0600): each is a
# single node, and what separates them is which workloads run there. A second config
# would be two files to keep identical.
cluster_kind_config() { printf 'infra/local/kind.yaml'; }

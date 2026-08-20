# shellcheck shell=bash
# Resolve the local cluster's identity (ADR-0600). Source it, don't execute:
#   source "$(dirname "$0")/lib/cluster-ctx.sh"
#
# There are TWO local clusters, because the two tiers run different distributions:
#
#   base  the inner loop, on kind          context `kind-<name>`
#   full  the platform, on Talos in Docker context `admin@<name>`
#
# One cluster cannot serve both — a tier is a distribution here, not a set of
# workloads — so each owns its own cluster, its own context and its own lifecycle.
# Every script resolves through these functions rather than hardcoding a prefix,
# which is what stops `mise run cluster:delete` in one tier taking the other with it.
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
# The two provisioners name their contexts differently and neither is configurable:
# kind prefixes `kind-`, and talosctl writes `admin@<cluster>`. That asymmetry is
# the reason this function exists.
cluster_ctx() {
  if [ "$TIER" = "full" ]; then
    printf 'admin@%s' "$(cluster_name)"
  else
    printf 'kind-%s' "$(cluster_name)"
  fi
}

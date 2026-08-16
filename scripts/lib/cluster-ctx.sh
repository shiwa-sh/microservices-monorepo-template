# shellcheck shell=bash
# Resolve the local cluster's kube context (ADR-0600). The inner loop runs on kind
# (context `kind-<cluster>`); the full tier's Talos provisioner will add a second
# context shape. Every script resolves through here rather than hardcoding the
# prefix, so a new provisioner changes one function, not nineteen literals.
if [[ -n "${__CLUSTER_CTX_SH_LOADED:-}" ]]; then return 0 2>/dev/null || true; fi
__CLUSTER_CTX_SH_LOADED=1

# cluster_ctx — prints the kube context name for the local cluster.
cluster_ctx() {
  printf 'kind-%s' "${CLUSTER:-platform}"
}

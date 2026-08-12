#!/usr/bin/env bash
# Gate a release on prod actually converging on the release commit (ADR-0201,
# ADR-0103). "Argo Healthy" already means the new ReplicaSet is available, but a
# release must not report success before prod runs the release commit.
#
# Scaffolded: template CI has no prod kubeconfig, as with any cluster-touching
# step. Set ARGOCD_SERVER and ARGOCD_AUTH_TOKEN to arm it.
set -euo pipefail
# shellcheck source=lib/log.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib/log.sh"

sha="${SHA:-}"
[[ -n "$sha" ]] || fail "SHA is unset"

if [[ -z "${ARGOCD_SERVER:-}" || -z "${ARGOCD_AUTH_TOKEN:-}" ]]; then
  warn "ARGOCD_SERVER/ARGOCD_AUTH_TOKEN unset — prod convergence is NOT verified"
  detail "argocd app wait -l env=prod --revision \"${sha}\" --sync --health --timeout 600"
  exit 0
fi

step "waiting for prod to converge on ${sha}"
argocd app wait -l env=prod --revision "$sha" --sync --health --timeout 600
ok "prod converged on ${sha}"

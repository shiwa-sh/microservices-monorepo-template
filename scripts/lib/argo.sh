#!/usr/bin/env bash
# ArgoCD helpers shared by the working-tree deploy paths (ADR-0600).

# argo_service_app prints the Application name that manages a service, or nothing
# when Argo does not manage it.
#
# There are two names in play. The committed ApplicationSet names each app after
# the values FILE it generated from, which for a while meant `local-service-
# orders.yaml`; it now trims the extension, so a cluster bootstrapped from a newer
# commit has `local-service-orders`. A cluster is bootstrapped from the remote
# branch rather than the working tree, so both spellings are live in the wild and a
# script that guesses one silently fails against the other — the failure being that
# auto-sync is never paused and Argo reverts the deploy that just reported success.
argo_service_app() {
  local svc="$1" cluster="${CLUSTER:-platform}" name
  for name in "local-service-${svc}" "local-service-${svc}.yaml"; do
    if kubectl --context "k3d-${cluster}" -n argocd get application.argoproj.io "$name" >/dev/null 2>&1; then
      echo "$name"
      return 0
    fi
  done
  return 0
}

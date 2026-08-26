#!/usr/bin/env bash
# The live-patch window (ADR-0201, ADR-0600).
#
#   mise run argo:pause     stop reconciliation; the cluster keeps running
#   mise run argo:resume    hand the cluster back to GitOps
#
# Argo CD's selfHeal reverts a direct edit to a synced resource within seconds, so
# validating an uncommitted values or NetworkPolicy fix in-cluster means scaling the
# application-controller to zero first. Anything patched live and not committed to
# master is reverted on the first sync after resume — push first, then resume.
#
# A paused controller is a cluster that has silently stopped tracking master, which
# looks identical to a healthy one until a deploy goes missing. Both verbs therefore
# report the state they left behind rather than exiting mute.
#
# Argo CD is the engine for the full tier only, so the tier is not an argument here:
# there is exactly one cluster that has an application-controller to scale, and
# lib/cluster.sh resolves it from what is running.
set -euo pipefail

source "$(dirname "$0")/lib/cluster.sh"
cd "$ROOT"

STS=statefulset/argocd-application-controller

verb="${1:?usage: argo.sh <pause|resume>}"
case "$verb" in
pause | resume) ;;
*) fail "'${verb}' is not a verb — use \"pause\" or \"resume\"" ;;
esac
# The window is cluster-wide, so there is no second operand to accept. An ignored
# argument reads as an applied one.
[ "$#" -eq 1 ] || fail "argo.sh takes one verb — got: $*"

require_cluster
[ "$(cluster_tier)" = full ] ||
  fail "Argo CD runs on the full tier only, and the ${TIER} tier is what is up.
  Nothing reconciles the inner loop, so there is no window to open or close."

k -n argocd get "$STS" >/dev/null 2>&1 ||
  fail "no application-controller in $(cluster_ctx) — the full tier is up but its
  GitOps bootstrap is not. Re-run '$(up_hint)'."

if [ "$verb" = pause ]; then
  step "pausing Argo CD on $(cluster_ctx) — the cluster stops tracking master"
  k -n argocd scale "$STS" --replicas=0 >/dev/null
  ok "paused; resume with 'mise run argo:resume' when the patch is committed"
  exit 0
fi

step "resuming Argo CD on $(cluster_ctx)"
k -n argocd scale "$STS" --replicas=1 >/dev/null
# Rollout status, not the scale call's return: scale reports the intent was recorded,
# and "resumed" is a claim about a controller that is actually running.
k -n argocd rollout status "$STS" --timeout=180s >/dev/null ||
  fail "the application-controller did not come back — 'kubectl -n argocd describe ${STS}'"

# Auto-sync paused per app by service:deploy is a second, narrower pause, and a
# controller that is running again does not clear it. Name the apps still holding a
# working-tree image, because the cluster now looks reconciled and is not.
# custom-columns and awk rather than a jsonpath filter: kubectl's jsonpath has no
# negation, so `?(!@.spec.syncPolicy.automated)` matches nothing and the warning
# silently never fires — which is the failure this whole block exists to prevent.
mapfile -t manual < <(
  k -n argocd get application.argoproj.io --no-headers \
    -o custom-columns='NAME:.metadata.name,AUTO:.spec.syncPolicy.automated' 2>/dev/null |
    awk '$2 == "<none>" { print $1 }'
)
if [ "${#manual[@]}" -gt 0 ]; then
  warn "${#manual[@]} application(s) still have auto-sync off — service:deploy pauses the app it deploys:"
  printf '    · %s\n' "${manual[@]}" >&2
  detail "hand one back with: argocd app set <name> --sync-policy automated"
fi
ok "resumed — the cluster tracks master again"

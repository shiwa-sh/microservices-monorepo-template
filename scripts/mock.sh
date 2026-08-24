#!/usr/bin/env bash
# The API mock for the UI development loop (ADR-0600).
#
#   mise run mock:start    # stamp the spec, apply the manifests, take over /api
#   mise run mock:stop     # remove it; /api goes back to whatever is deployed
#   mise run mock:logs     # follow it — unmatched routes and validation failures
#
# Why a script rather than three `kubectl apply` one-liners: the ConfigMap holding
# the spec is STAMPED, not committed. infra/local/mock.yaml carries the Deployment,
# Service and IngressRoute; the document Prism serves comes from the committed
# projection at build time of the command, so the mock can never serve a contract
# older than the working tree. That is the whole reason its only input is
# `internal.json` (ADR-0600) — no globbing of services/*/openapi.yaml, no route
# files, no fixture bodies to drift.
set -euo pipefail

source "$(dirname "$0")/lib/log.sh"

CLUSTER="${CLUSTER:-platform}"
source "$(dirname "$0")/lib/cluster.sh"
NS="platform"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

SPEC="apps/frontend/public/devportal/openapi/internal.json"
MANIFEST="infra/local/mock.yaml"

k() { kubectl --context "$(cluster_ctx)" -n "$NS" "$@"; }

case "${1:-}" in
start)
  [ -f "$SPEC" ] || fail "missing ${SPEC} — run \`mise run gen\` first"

  step "stamping the committed projection into the mock's spec ConfigMap"
  # --dry-run | apply, so a re-run replaces the data rather than failing on an
  # existing object. The hash below is what makes the pod notice.
  k create configmap mock-spec --from-file="internal.json=${SPEC}" \
    --dry-run=client -o yaml | k apply -f - >/dev/null
  detail "$(wc -c <"$SPEC" | tr -d ' ') bytes from ${SPEC}"

  step "applying the mock"
  k apply -f "$MANIFEST" >/dev/null

  # Prism reads the document once at startup, so a changed spec needs a new pod.
  # Annotating with the content hash makes that automatic and idempotent: an
  # unchanged spec leaves the pod alone, so `mock:start` is safe to re-run.
  hash="$(sha256sum "$SPEC" | cut -c1-16)"
  k patch deployment mock --type merge \
    -p "{\"spec\":{\"template\":{\"metadata\":{\"annotations\":{\"local.platform/spec-hash\":\"${hash}\"}}}}}" >/dev/null

  k rollout status deployment/mock --timeout=120s
  ok "mock serving /api — https://dev.localtest.me:8443/api/products"
  detail "it answers from the spec's examples; it has no session, no 401, no state"
  ;;

stop)
  step "removing the mock"
  # The IngressRoute goes first: /api returns to the real services (or to nothing)
  # before the pod disappears, rather than after.
  k delete -f "$MANIFEST" --ignore-not-found >/dev/null
  k delete configmap mock-spec --ignore-not-found >/dev/null
  ok "mock removed — /api is back to whatever is deployed"
  ;;

logs)
  k logs -f deployment/mock
  ;;

*)
  fail "usage: mise run mock:{start,stop,logs}"
  ;;
esac

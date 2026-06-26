#!/usr/bin/env bash
# e2e readiness gate (ADR-0018). Runs before the browser suite as a fast failure
# *localiser*, not an acceptance test: if this is red, cluster:full is not up, so a
# red e2e reads "infra down" rather than "app broken". Two cheap classes of check:
#   1. the dashboards' backing deploys report an available replica, and
#   2. the edge auth layer is live — an ops origin answers with a deny (401/403) or
#      a login redirect (302/303), never a connection error or 5xx.
#
# curl hits the local edge directly (--noproxy '*'): *.localtest.me must never go
# through a dev HTTPS_PROXY. kubectl is on PATH via mise.
set -uo pipefail

HOST="${E2E_HOST:-dev.localtest.me:8443}"
failed=0

ok()   { printf '✓ %s\n' "$1"; }
bad()  { printf '✗ %s: %s\n' "$1" "$2" >&2; failed=$((failed + 1)); }

deploy_ready() { # ns name
  local reps
  reps=$(kubectl -n "$1" get deploy "$2" -o jsonpath='{.status.availableReplicas}' 2>/dev/null)
  if [ -n "$reps" ] && [ "$reps" != "0" ]; then
    ok "$2 ready ($1)"
  else
    bad "$2 ready ($1)" "no available replicas"
  fi
}

edge_gates() { # tool
  local code
  code=$(curl -sk --noproxy '*' -o /dev/null -w '%{http_code}' \
    "https://$1.ops.${HOST}/" --max-time 8 2>/dev/null)
  case "$code" in
    401 | 403 | 302 | 303) ok "edge gates $1 (HTTP $code)" ;;
    *) bad "edge gates $1" "unexpected HTTP $code (edge/oathkeeper not gating)" ;;
  esac
}

deploy_ready platform    ory-oathkeeper
deploy_ready platform    ory-kratos
deploy_ready platform    authz-server
deploy_ready platform    spicedb
deploy_ready platform    grafana
deploy_ready platform    temporal-web
deploy_ready kube-system hubble-ui
deploy_ready kube-system hubble-relay
edge_gates grafana
edge_gates temporal
edge_gates hubble

if [ "$failed" -gt 0 ]; then
  printf '\npreflight: %d check(s) failed — cluster:full is not ready (infra down, not app broken)\n' "$failed" >&2
  exit 1
fi
printf '\npreflight: all checks passed\n'

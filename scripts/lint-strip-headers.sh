#!/usr/bin/env bash
# Anti-spoofing gate (ADR-0305). Every IngressRoute that authenticates
# via the Oathkeeper forwardAuth middleware MUST also apply strip-identity-headers
# BEFORE it, so a client cannot inject X-User-* / X-Org-Id / X-Roles on any route
# (anonymous routes especially). Fails CI if a forward-auth route is missing the
# strip, or applies it after forwardAuth.
set -euo pipefail
source "$(dirname "$0")/lib/log.sh"
cd "$(cd "$(dirname "$0")/.." && pwd)"

step "checking every forward-auth route strips identity headers first"
# Render the gateway + per-service /api route, then feed the manifests to the
# single-binary Go gate (no ambient Python). The Go tool reads YAML from stdin.
{
  kubectl kustomize infra/gateway
  echo '---'
  # The per-resource /api route (chart template) — render with ingress on and a
  # resource declared, as real services are configured (flat /api/<resource>, ADR-0306).
  helm template svc infra/helm/service \
    --set name=svc --set image.repository=svc --set image.tag=dev \
    --set ingress.enabled=true --set ingress.host=example.com \
    --set 'ingress.resources={svc}'
} | go run ./tools/lint-strip-headers

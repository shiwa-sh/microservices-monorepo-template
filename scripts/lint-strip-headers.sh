#!/usr/bin/env bash
# Anti-spoofing gate (ADR-0009, Phase 8). Every IngressRoute that authenticates
# via the Oathkeeper forwardAuth middleware MUST also apply strip-identity-headers
# BEFORE it, so a client cannot inject X-User-* / X-Org-Id / X-Roles on any route
# (anonymous routes especially). Fails CI if a forward-auth route is missing the
# strip, or applies it after forwardAuth.
set -euo pipefail
cd "$(cd "$(dirname "$0")/.." && pwd)"

tmp="$(mktemp --suffix=.yaml)"
trap 'rm -f "$tmp"' EXIT
{
  kubectl kustomize infra/gateway
  echo '---'
  # The per-service /api route (chart template) — render with ingress on.
  helm template svc infra/helm/service \
    --set name=svc --set image.repository=svc --set image.tag=dev \
    --set ingress.enabled=true --set ingress.host=example.com
} > "$tmp"

python3 - "$tmp" <<'PY'
import sys, yaml
bad = []
checked = 0
for d in yaml.safe_load_all(open(sys.argv[1])):
    if not d or d.get("kind") != "IngressRoute":
        continue
    name = d.get("metadata", {}).get("name", "?")
    for route in d.get("spec", {}).get("routes", []):
        mws = [m.get("name") for m in route.get("middlewares", [])]
        if "oathkeeper-forward-auth" not in mws:
            continue
        checked += 1
        if "strip-identity-headers" not in mws:
            bad.append(f"{name}: forward-auth route without strip-identity-headers")
        elif mws.index("strip-identity-headers") > mws.index("oathkeeper-forward-auth"):
            bad.append(f"{name}: strip-identity-headers must come BEFORE forward-auth")
if bad:
    print("✗ anti-spoofing gate failed:", file=sys.stderr)
    for b in bad:
        print("  " + b, file=sys.stderr)
    sys.exit(1)
print(f"✓ all {checked} forward-auth routes strip identity headers first")
PY

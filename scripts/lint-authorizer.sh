#!/usr/bin/env bash
# Ops-tier authorizer policy (ADR-0017, Phase 8). Every operator-dashboard access
# rule must authorize per-tool via the remote_json authorizer (→ SpiceDB Checker),
# never `allow`. A re-introduced `"authorizer": {"handler": "allow"}` on an ops
# route is exactly the gap this whole effort closes, so CI fails on it.
#
# Product /api routes legitimately keep `allow` (services authorize in-process via
# libs/go/authz), so only ops-* rules are checked.
set -euo pipefail
cd "$(cd "$(dirname "$0")/.." && pwd)"

RULES="infra/auth/oathkeeper/access-rules.json"

python3 - "$RULES" <<'PY'
import json, sys
rules = json.load(open(sys.argv[1]))
bad = []
for r in rules:
    rid = r.get("id", "")
    handler = r.get("authorizer", {}).get("handler")
    # Ops dashboards are identified by the `ops-` rule-id prefix (one origin per
    # tool under *.ops.<host>). They must use remote_json, not allow.
    if rid.startswith("ops-") and handler != "remote_json":
        bad.append(f"{rid}: authorizer is {handler!r}, expected 'remote_json'")
if bad:
    print("✗ ops dashboard rules must authorize via remote_json, never allow:", file=sys.stderr)
    for b in bad:
        print("  " + b, file=sys.stderr)
    sys.exit(1)
print(f"✓ all ops dashboard rules use remote_json ({sum(r.get('id','').startswith('ops-') for r in rules)} rules)")
PY

#!/usr/bin/env bash
# Auth-config single-source lint (Phase 0.4 of the auth plan).
#
# The auth config lives exactly once, in the canonical infra/auth tree, and is
# injected into the Ory umbrella chart at install time (Helm `-f` overlays +
# `--set-file`; ArgoCD valueFiles + fileParameters). This guard fails CI if any
# of it is re-inlined back into the chart values, which is how the copies silently
# diverged before.
set -euo pipefail
cd "$(cd "$(dirname "$0")/.." && pwd)"

VALUES="infra/helm/platform/ory/values.yaml"
fail=0

# Markers that only appear when kratos/oathkeeper config or the string artefacts
# have been inlined into the chart values. The legitimate file carries only the
# `enabled` flags and the (deferred) Hydra block.
PATTERNS='accessRules|identitySchemas|default_schema_id|authenticators:|access_rules:|selfservice:|cookie_session|ory_kratos_session'
# Match with original line numbers, then drop comment lines (the header doc-pointers
# legitimately name these keys) — a comment is `<n>:<spaces>#…`.
if hits=$(grep -nEi "$PATTERNS" "$VALUES" 2>/dev/null | grep -vE '^[0-9]+:\s*#'); then
  echo "✗ auth config re-inlined into $VALUES — it must live only in infra/auth/*:" >&2
  echo "$hits" >&2
  fail=1
fi

# The canonical artefacts must exist (the injection points reference them).
for f in \
  infra/auth/kratos/values.yaml \
  infra/auth/oathkeeper/values.yaml \
  infra/auth/oathkeeper/access-rules.json \
  infra/auth/kratos/identity-schemas/user.v1.json; do
  if [ ! -f "$f" ]; then
    echo "✗ missing canonical auth artefact: $f" >&2
    fail=1
  fi
done

if [ "$fail" -eq 0 ]; then
  echo "✓ auth config single-source: no inline copies in $VALUES"
fi
exit "$fail"

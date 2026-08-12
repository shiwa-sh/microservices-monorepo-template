#!/usr/bin/env bash
# Emit the affected manifest as forge step-output assignments (ADR-0102). The
# matrix jobs in publish.yml need `services` and `apps` as separate JSON arrays;
# splitting the manifest is pipeline logic, so it lives here and the workflow step
# only redirects into $GITHUB_OUTPUT.
set -euo pipefail

manifest="$(mise run ci:affected)"
printf 'services=%s\n' "$(printf '%s' "$manifest" | jq -c '.services')"
printf 'apps=%s\n' "$(printf '%s' "$manifest" | jq -c '.apps')"

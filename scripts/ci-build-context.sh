#!/usr/bin/env bash
# Emit an app's Docker build context as a forge step-output assignment (ADR-0102).
# The context differs per app: the frontend builds from the repo root scoped by its
# Dockerfile.dockerignore, while admin builds from apps/admin because the root
# .dockerignore excludes apps/.
#   mise run ci:build-context -- admin
set -euo pipefail
# shellcheck source=lib/log.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib/log.sh"

app="${1:-}"
[[ -n "$app" ]] || fail "usage: mise run ci:build-context -- <app>"

case "$app" in
admin) printf 'context=apps/admin\n' ;;
*) printf 'context=.\n' ;;
esac

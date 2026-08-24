#!/usr/bin/env bash
# Helm dependency resolution must not depend on the caller's machine (ADR-0101).
#
# `helm dependency build` reads Chart.lock and requires the dependency's repository
# to be REGISTERED in the caller's helm config. `helm dependency update` resolves it
# from the URL already in Chart.yaml and needs nothing ambient.
#
# The distinction is invisible locally, which is the whole problem: a developer
# registers a repo once, every `build` after that works forever, and the failure
# appears on the first clean runner — "Error: no repository definition for
# https://charts.jetstack.io". That is what happened the first time CI ran
# cluster:up, on a script that had worked on every machine that ever ran it.
#
# So: a bare `build` is forbidden. Either use `update`, or fall back to it. This is a
# static check on purpose — proving it dynamically means downloading charts on every
# lint run, and the shape of the call is enough to know the answer.
set -euo pipefail

source "$(dirname "$0")/lib/log.sh"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

step "checking helm dependency calls resolve without pre-registered repos"

# A line calling `dependency build` is fine only if it also names `dependency
# update` — on the same line, or on the next one as a `||` fallback.
offenders=""
while IFS= read -r hit; do
  file="${hit%%:*}"
  rest="${hit#*:}"
  line="${rest%%:*}"
  # The call plus the line after it, which is where a `|| … update` fallback lives.
  window="$(sed -n "${line},$((line + 1))p" "$file")"
  case "$window" in
  *"dependency update"*) ;;
  *) offenders="${offenders}  ${file}:${line}"$'\n' ;;
  esac
  # This file is excluded because it necessarily names the thing it forbids — in
  # the pattern and in the explanation of why the pattern exists.
done < <(grep -rn 'dependency build' --include='*.sh' --exclude="$(basename "$0")" scripts/ || true)

if [ -n "$offenders" ]; then
  printf '✗ `helm dependency build` with no `update` fallback:\n%s\n' "$offenders" >&2
  echo "  build needs the repo already registered; a clean runner has none." >&2
  echo "  Use \`dependency update\`, or \`build … || update …\`." >&2
  exit 1
fi

ok "helm dependency calls resolve from Chart.yaml alone"

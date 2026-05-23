#!/usr/bin/env bash
# Fail on floating container image / tool tags (ADR-0002).
# Looks at Dockerfiles, Helm values, GitHub workflows, and .mise.toml.

set -euo pipefail

FOUND=0

scan() {
  local label="$1"
  shift
  local pattern="$1"
  shift
  # shellcheck disable=SC2068
  local hits
  hits=$(grep -RInE "$pattern" $@ 2>/dev/null || true)
  if [[ -n "$hits" ]]; then
    echo "✗ $label:"
    echo "$hits" | sed 's/^/    /'
    FOUND=1
  fi
}

paths=(Dockerfile infra/helm .github/workflows .mise.toml services apps)

# Floating image tags in Helm values / Dockerfiles / workflows.
scan "floating image tags" \
  '(:latest|:stable|:main)([^A-Za-z0-9_.-]|$)' \
  "${paths[@]}"

# Unpinned action versions in workflows (uses: foo/bar@main, @master, @vN without a SHA).
scan "unpinned GitHub Action references" \
  'uses: [^ ]+@(main|master|develop)\b' \
  .github/workflows 2>/dev/null || true

if [[ "$FOUND" -ne 0 ]]; then
  echo
  echo "Floating tags forbidden by ADR-0002. Pin to a concrete version/SHA."
  exit 1
fi

echo "✓ no floating tags"

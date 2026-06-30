#!/usr/bin/env bash
# Checks .mise.toml tools against latest available versions and optionally upgrades them.
# Uses --bump to cross major/minor boundaries and write new versions back to .mise.toml.
set -euo pipefail

cd "$(dirname "$0")/.."

DRY_RUN=false
YES=false

for arg in "$@"; do
  case "$arg" in
    -n|--dry-run) DRY_RUN=true ;;
    -y|--yes)     YES=true ;;
    -h|--help)
      echo "Usage: $(basename "$0") [-n|--dry-run] [-y|--yes]"
      echo ""
      echo "  -n, --dry-run   Show what would change without upgrading"
      echo "  -y, --yes       Skip confirmation prompt"
      exit 0
      ;;
  esac
done

if [[ -z "${GITHUB_TOKEN:-}" ]]; then
  # Try to source one from gh CLI if available, to avoid unauthenticated rate limits
  if command -v gh &>/dev/null && gh auth status &>/dev/null 2>&1; then
    GITHUB_TOKEN=$(gh auth token 2>/dev/null || true)
    export GITHUB_TOKEN
  else
    echo "WARN: GITHUB_TOKEN is not set. mise makes unauthenticated GitHub API calls,"
    echo "      which are rate-limited (60 req/hr). Set a token to avoid 403 errors:"
    echo ""
    echo "  export GITHUB_TOKEN=<your-token>   # https://github.com/settings/tokens"
    echo "  # or: log in with 'gh auth login' and this script will use it automatically"
    echo ""
  fi
fi

echo "Checking for outdated tools (latest across all versions)..."
echo ""

outdated=$(mise outdated --bump --no-header)

if [[ -z "$outdated" ]]; then
  echo "All tools are up to date."
  exit 0
fi

# Print as a proper table with header
echo "Tool                    Current    Latest"
echo "----------------------  ---------  ---------"
echo "$outdated"
echo ""

if $DRY_RUN; then
  echo "Dry run — no changes made."
  exit 0
fi

# `mise run upgrade` runs tasks in parallel, so this script has no TTY on stdin.
# Only prompt when interactive; otherwise the explicit task invocation is consent.
if ! $YES && [[ -t 0 ]]; then
  read -rp "Upgrade all outdated tools and update .mise.toml? [y/N] " confirm
  [[ "$confirm" =~ ^[Yy]$ ]] || { echo "Aborted."; exit 0; }
fi

mise upgrade --bump
echo ""
echo "Done. Review version bumps with: git diff .mise.toml"

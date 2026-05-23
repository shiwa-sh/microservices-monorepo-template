#!/usr/bin/env bash
# Minimal mise bootstrap for CI runners.
# Installs mise, activates it for the current job, and installs the toolchain
# pinned in .mise.toml. CI workflows source this before invoking `mise run …`.

set -euo pipefail

curl https://mise.run | sh

MISE_BIN="${HOME}/.local/bin/mise"

# Make mise available to subsequent steps in GitHub Actions.
if [[ -n "${GITHUB_PATH:-}" ]]; then
  echo "${HOME}/.local/bin" >>"${GITHUB_PATH}"
fi

eval "$("${MISE_BIN}" activate bash)"

"${MISE_BIN}" install

#!/usr/bin/env bash
# Print the environment corrections a non-GitHub forge needs, for $GITHUB_ENV.
#
#   mise run ci:forge-env >> "$GITHUB_ENV"
#
# One correction so far. `GITHUB_TOKEN` is minted by whichever forge runs the
# workflow, and mise presents it to api.github.com whenever it resolves a
# `github:` or `ubi:` tool. On GitHub the forge and the API are the same host, so
# the token is right. On Forgejo they are not: the token is valid, for somewhere
# else, and GitHub answers `401 Bad credentials` — which reads as an expired
# secret rather than as a token offered to the wrong host.
#
# It has to be corrected for the whole JOB rather than for the step that installs
# the toolchain, because every later step runs `mise run ci:*` and mise resolves
# tools there too. That is why this writes to $GITHUB_ENV instead of being a
# step-level `env:`.
#
# `FORGEJO_ACTIONS` is the discriminator: the runner sets it and GitHub never
# does. Comparing `GITHUB_SERVER_URL` does not work — a forge configured to
# resolve actions from github.com reports github.com there too.
#
# The fallback is `MISE_GITHUB_TOKEN`, which an adopter sets as a forge secret to
# keep authenticated rate limits; empty means unauthenticated, whose limit is per
# IP and low.
set -euo pipefail

[ -n "${FORGEJO_ACTIONS:-}" ] || exit 0

echo "GITHUB_TOKEN=${MISE_GITHUB_TOKEN:-}"

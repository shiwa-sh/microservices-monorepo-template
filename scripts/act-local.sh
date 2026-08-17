#!/usr/bin/env bash
# Local act runner that bakes the mise toolchain into one base image.
#
#   mise run act:local               # build base image if stale, then run the PR gate
#   mise run act:local -- --rebuild  # force a fresh image build
#   mise run act:local -- -j build   # run a single job against the base image
#
# act runs every pull_request job in its own container, and every job used to
# re-install the full ~30-tool mise toolchain from scratch: 8 containers × ~2.8 GB,
# plus a long cold start, on every run. This bakes the toolchain into one image
# once and runs act with -P ubuntu-latest=<image>, so every job shares a single
# read-only layer and starts with the tools already on PATH.
#
# The image is rebuilt only when its inputs change: the root .mise.toml, or the
# mise version CI pins in .github/actions/setup/action.yml. The proxy and the
# GitHub token are build args, needed to fetch and verify the tools during the
# bake, and are never persisted into the image.
set -euo pipefail

source "$(dirname "$0")/lib/log.sh"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

PROXY="${ACT_PROXY:-http://127.0.0.1:8118}"
IMAGE="${ACT_IMAGE:-act-local:latest}"
EVENT="${ACT_EVENT:-pull_request}"
ENV_FILE="${ACT_ENV_FILE:-.env}"

NO_PROXY="127.0.0.1,localhost"
HOST_IP="$(ip -4 route get 1.1.1.1 2>/dev/null | awk '{for (i = 1; i <= NF; i++) if ($i == "src") print $(i + 1)}' || true)"
[[ -n "$HOST_IP" ]] && NO_PROXY+=",$HOST_IP"

REBUILD=0
ACT_ARGS=()
while [[ $# -gt 0 ]]; do
  case "$1" in
  --rebuild) REBUILD=1 ;;
  --)
    shift
    ACT_ARGS+=("$@")
    break
    ;;
  *) ACT_ARGS+=("$1") ;;
  esac
  shift
done

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# The mise version CI pins, single-sourced from the setup action (renovate-managed).
MISE_VERSION="$(sed -n '/depName=jdx\/mise/,+1p' .github/actions/setup/action.yml | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1)"
[[ -n "$MISE_VERSION" ]] || fail "cannot read the mise version from .github/actions/setup/action.yml"

# Rebuild whenever a tool or the mise binary changes.
SOURCE_HASH="$({
  cat .mise.toml
  sed -n '/depName=jdx\/mise/,+1p' .github/actions/setup/action.yml
} | cksum | awk '{print $1}')"

TOKEN=""
if [[ -f "$ENV_FILE" ]]; then
  TOKEN="$(grep -E '^GITHUB_TOKEN=' "$ENV_FILE" | head -1 | cut -d= -f2- | tr -d ' \r\n"' || true)"
fi

step "checking proxy ${PROXY}"
if ! curl -sS -o /dev/null -m 10 -x "$PROXY" https://api.github.com/rate_limit; then
  fail "proxy ${PROXY} is unreachable — the bake needs it to download and verify tools"
fi

needs_build() {
  [[ "$REBUILD" == 1 ]] && return 0
  local baked
  baked="$(docker image inspect --format '{{ index .Config.Labels "act-local.source-hash" }}' "$IMAGE" 2>/dev/null || true)"
  [[ "$baked" == "$SOURCE_HASH" ]]
}

build_image() {
  step "building ${IMAGE} — baking the full mise toolchain"
  cp .mise.toml "$WORK/"
  cat >"$WORK/Dockerfile" <<'DOCKERFILE'
FROM catthehacker/ubuntu:act-latest

ARG MISE_VERSION
ARG SOURCE_HASH
ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG NO_PROXY
ARG GITHUB_TOKEN

LABEL act-local.source-hash="${SOURCE_HASH}"

# mise at the version CI pins (.github/actions/setup/action.yml).
RUN set -eux; \
  curl -fsSL "https://github.com/jdx/mise/releases/download/v${MISE_VERSION}/mise-v${MISE_VERSION}-linux-x64.tar.zst" | tar --zstd -xf - -C /tmp; \
  mv /tmp/mise/bin/mise /usr/local/bin/mise; \
  rm -rf /tmp/mise

# The toolchain from the root .mise.toml. The proxy and token are passed on the
# RUN line (not ENV), so neither persists into the image.
COPY .mise.toml /opt/act-local/.mise.toml
RUN set -eux; \
  cd /opt/act-local; \
  mise trust -y .mise.toml; \
  HTTP_PROXY="${HTTP_PROXY}" HTTPS_PROXY="${HTTPS_PROXY}" NO_PROXY="${NO_PROXY}" \
  GITHUB_TOKEN="${GITHUB_TOKEN}" \
  mise install

# The install is done; drop mise's download cache to keep the layer lean.
RUN rm -rf /root/.cache/mise
DOCKERFILE

  docker build --network host -t "$IMAGE" \
    --build-arg "MISE_VERSION=$MISE_VERSION" \
    --build-arg "SOURCE_HASH=$SOURCE_HASH" \
    --build-arg "HTTP_PROXY=$PROXY" \
    --build-arg "HTTPS_PROXY=$PROXY" \
    --build-arg "NO_PROXY=$NO_PROXY" \
    --build-arg "GITHUB_TOKEN=$TOKEN" \
    "$WORK"
  ok "built ${IMAGE}"
}

if needs_build; then
  [[ -n "$TOKEN" ]] || warn "no GITHUB_TOKEN in ${ENV_FILE} — the bake may hit the unauthenticated GitHub rate limit"
  build_image
else
  step "reusing ${IMAGE} (toolchain unchanged — pass --rebuild to force)"
fi

if command -v mise >/dev/null 2>&1; then
  ACT_RUN=(mise x act -- act)
else
  ACT_RUN=(act)
fi

step "running act ${EVENT} on ${IMAGE}"
"${ACT_RUN[@]}" "$EVENT" \
  -P "ubuntu-latest=${IMAGE}" \
  --network host \
  --pull=false \
  --env "HTTP_PROXY=$PROXY" --env "HTTPS_PROXY=$PROXY" --env "NO_PROXY=$NO_PROXY" \
  --secret-file "$ENV_FILE" \
  -e ".github/act/$EVENT.json" \
  "${ACT_ARGS[@]}"

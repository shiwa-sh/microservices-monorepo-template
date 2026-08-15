# syntax=docker/dockerfile:1.7
# Built from this directory as context (apps/admin), independent of the repo-root
# .dockerignore the Go service images rely on (it excludes apps/).
#
# Lowdefy 5 requires Node + pnpm, so this image — unlike the Go/bun runtime images
# — is Node-based. One of the two Node exceptions ADR-0100 sanctions, and the only
# one that ships as an image. No repo runtime code is Node: Lowdefy is a
# third-party app we install and run, so the runtime is upstream's choice, not
# ours. Concretely, bun cannot displace it: the CLI aborts unless pnpm is on PATH
# (its startUp checks), and `lowdefy start` spawns `pnpm run start` → `next start`,
# whose bin is `#!/usr/bin/env node`. Running it under bun would mean aliasing node
# and still installing pnpm — an unsupported hack (nobody upstream has attempted
# it) for no gain, since the runtime is sealed in this image either way.
ARG NODE_VERSION=22
ARG LOWDEFY_VERSION=5.4.0
FROM node:${NODE_VERSION}-alpine
ARG LOWDEFY_VERSION
# Build identity (ADR-0103): the commit SHA, recorded as the standard OCI revision
# label so the image reports exactly what was built (the same SERVICE_VERSION build
# arg the frontend takes, so one apps matrix builds both).
ARG SERVICE_VERSION=dev
LABEL org.opencontainers.image.revision="${SERVICE_VERSION}"
WORKDIR /app
# pnpm is required by Lowdefy at BUILD and at START — `lowdefy start` refuses to
# run without it on PATH (its checkPnpmIsInstalled), and then spawns it.
#
# Installed globally rather than through `corepack enable`, which is what this line
# used to do. corepack only installs a SHIM: the real pnpm is downloaded on first
# use, into the invoking user's own cache. That is invisible while the container
# runs as root during the build, and fails at start once it runs as `node`
# (ADR-0200) — a different, unwritable cache, and a registry fetch that this
# cluster's default-deny egress drops anyway. `npm install -g` puts the real thing
# in /usr/local/lib/node_modules at build time, readable by every user.
#
# pnpm 10+ hard-fails (ERR_PNPM_IGNORED_BUILDS) when a dependency's build script is
# blocked; the Lowdefy server's deps (sharp, @sentry/cli) need theirs, so allow them.
ARG PNPM_VERSION=10.18.2
RUN npm install -g pnpm@${PNPM_VERSION} && pnpm config set dangerously-allow-all-builds true
COPY . .
# Lowdefy's own `@lowdefy/server` fetch (a raw axios GET to registry.npmjs.org,
# not npm's client) reliably gets "socket hang up"/400 through this project's
# egress proxy; npm's own fetcher (pacote) does not. Pre-populate the server
# directory via `npm pack` so `lowdefy build` finds it already present and
# skips its own fetch (see lowdefy's getServer.js: skips fetch if
# .lowdefy/server/package.json name+version already match).
RUN npm install -g lowdefy@${LOWDEFY_VERSION} \
  && mkdir -p .lowdefy/server \
  && npm pack "@lowdefy/server@${LOWDEFY_VERSION}" --pack-destination /tmp \
  && tar xzf "/tmp/lowdefy-server-${LOWDEFY_VERSION}.tgz" --strip-components=1 -C .lowdefy/server \
  && rm "/tmp/lowdefy-server-${LOWDEFY_VERSION}.tgz" \
  && lowdefy build
EXPOSE 3000
# Run as the `node` user (uid 1000, in the base image) rather than root, which is
# what `restricted` requires of the namespace this lands in (ADR-0200). The build
# above runs as root and leaves /app root-owned, and `lowdefy start` writes into
# .lowdefy/ and .next/ at startup — so the tree is handed over explicitly rather
# than relying on world-writable defaults.
RUN chown -R node:node /app
USER node
# `lowdefy start` serves the built app on :3000; auth is enforced upstream at the
# edge (ADR-0401), this process trusts the forwarded X-User-Email header.
CMD ["lowdefy", "start"]

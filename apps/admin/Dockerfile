# syntax=docker/dockerfile:1.7
# Built from this directory as context (apps/admin), independent of the repo-root
# .dockerignore the Go service images rely on (it excludes apps/).
#
# Lowdefy 5 requires Node + pnpm (it shells out to pnpm and serves a Next.js app),
# so this image — unlike the Go/bun runtime images — is Node-based. It's a
# sanctioned, container-confined island, the same exception ADR-0018 makes for the
# Playwright e2e runner: a third-party tool that cannot run on bun. No repo runtime
# code is Node; this is just how the upstream admin tool ships.
ARG NODE_VERSION=22
ARG LOWDEFY_VERSION=5.4.0

FROM node:${NODE_VERSION}-alpine
ARG LOWDEFY_VERSION
WORKDIR /app
# corepack provides the pnpm that Lowdefy requires at build and start. pnpm 10+
# hard-fails (ERR_PNPM_IGNORED_BUILDS) when a dependency's build script is blocked;
# the Lowdefy server's deps (sharp, @sentry/cli) need theirs, so allow them.
RUN corepack enable && pnpm config set dangerously-allow-all-builds true
COPY . .
RUN npm install -g lowdefy@${LOWDEFY_VERSION} && lowdefy build
EXPOSE 3000
# `lowdefy start` serves the built app on :3000; auth is enforced upstream at the
# edge (ADR-0012), this process trusts the forwarded X-User-Email header.
CMD ["lowdefy", "start"]

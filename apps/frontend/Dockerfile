# syntax=docker/dockerfile:1.7
# Bun is the only JS runtime in this image — install, build, and the
# production server.js all run under Bun. Next's standalone output is
# officially Node-targeted, but Bun runs it in practice and removing Node
# keeps the platform on a single runtime (ADR-0000 §"one way to do things").
ARG BUN_VERSION=1.3.14

FROM oven/bun:${BUN_VERSION}-alpine AS deps
WORKDIR /app
COPY package.json bun.lock ./
# Every workspace member's manifest, because `bun install` resolves the workspace
# list before it installs anything: a member named in the root package.json but
# absent from the build context fails the install outright ("Workspace not found").
# A new member needs a line here — the root list and this stage are one decision.
COPY apps/frontend/package.json apps/frontend/
COPY libs/ts/id/package.json libs/ts/id/
COPY libs/ts/money/package.json libs/ts/money/
RUN --mount=type=cache,target=/root/.bun bun install --frozen-lockfile

FROM oven/bun:${BUN_VERSION}-alpine AS build
WORKDIR /app
# BOTH trees. In a Bun workspace a member's executables land in its own
# node_modules/.bin, not the root's, so carrying only /app/node_modules forward
# leaves `next` unresolvable and the build dies with "next: not found". The build
# context excludes node_modules (Dockerfile.dockerignore), which is what makes this
# the only source of them — and what stops a developer's host install from hiding
# the omission locally while CI builds from a clean checkout.
COPY --from=deps /app/node_modules /app/node_modules
COPY --from=deps /app/apps/frontend/node_modules /app/apps/frontend/node_modules
COPY . .
# Build identity baked into the bundle (ADR-0103): next.config inlines
# NEXT_PUBLIC_SERVICE_VERSION from SERVICE_VERSION at build, so the shipped JS and
# the X-App-Version response header both report the git SHA / release.
ARG SERVICE_VERSION=dev
ENV SERVICE_VERSION=$SERVICE_VERSION
# Optional, and deliberately EMPTY by default. `output: "standalone"` freezes
# next.config into server.js, so passing this here bakes the server-action CSRF
# allowlist — and with it one env's host — into the image, which forfeits the
# build-once / promote-by-digest flow (ADR-0103). Unset, the allowlist is empty and
# the check falls back to Origin == (X-Forwarded-)Host, which ADR-0306's same-origin
# layout already satisfies. Pass it only for a build you intend to pin to one env.
#
# The name is EDGE_PUBLIC_ORIGIN, which is what next.config.mjs reads: the origin
# split into public/internal when the frontend gained an in-cluster deployment, and
# a build arg that no longer matches the variable it feeds is one nothing reports —
# the allowlist just stays empty.
#
# The RUNTIME origins that server components fetch with are separate and come from
# the pod env (infra/gitops/services/<env>/values/frontend.yaml).
ARG EDGE_PUBLIC_ORIGIN=
ENV EDGE_PUBLIC_ORIGIN=$EDGE_PUBLIC_ORIGIN
RUN cd apps/frontend && bun run build

FROM oven/bun:${BUN_VERSION}-alpine AS runtime
WORKDIR /app
ENV NODE_ENV=production
COPY --from=build /app/apps/frontend/.next/standalone ./
COPY --from=build /app/apps/frontend/.next/static    ./apps/frontend/.next/static
COPY --from=build /app/apps/frontend/public          ./apps/frontend/public
EXPOSE 3000
CMD ["bun", "apps/frontend/server.js"]

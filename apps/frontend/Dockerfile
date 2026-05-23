# syntax=docker/dockerfile:1.7
# Bun is the only JS runtime in this image — install, build, and the
# production server.js all run under Bun. Next 15's standalone output is
# officially Node-targeted, but Bun runs it in practice and removing Node
# keeps the platform on a single runtime (ADR-0000 §"one way to do things").
ARG BUN_VERSION=1.3.14

FROM oven/bun:${BUN_VERSION}-alpine AS deps
WORKDIR /app
COPY package.json bun.lockb* ./
COPY apps/frontend/package.json apps/frontend/
RUN --mount=type=cache,target=/root/.bun bun install --frozen-lockfile

FROM oven/bun:${BUN_VERSION}-alpine AS build
WORKDIR /app
COPY --from=deps /app/node_modules /app/node_modules
COPY . .
RUN cd apps/frontend && bun run build

FROM oven/bun:${BUN_VERSION}-alpine AS runtime
WORKDIR /app
ENV NODE_ENV=production
COPY --from=build /app/apps/frontend/.next/standalone ./
COPY --from=build /app/apps/frontend/.next/static    ./apps/frontend/.next/static
COPY --from=build /app/apps/frontend/public          ./apps/frontend/public
EXPOSE 3000
CMD ["bun", "apps/frontend/server.js"]

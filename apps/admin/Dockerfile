# syntax=docker/dockerfile:1.7
ARG BUN_VERSION=1.2.4
FROM oven/bun:${BUN_VERSION}-alpine AS build
WORKDIR /app
COPY apps/admin/ .
RUN bun add -g lowdefy@4.5.7 && lowdefy build

FROM oven/bun:${BUN_VERSION}-alpine AS runtime
WORKDIR /app
COPY --from=build /app/.lowdefy/build /app/build
EXPOSE 3000
ENV LOWDEFY_BUILD_OUT_DIR=/app/build
CMD ["bun", "x", "lowdefy", "start"]

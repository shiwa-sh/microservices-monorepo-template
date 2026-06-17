# Multi-stage build (ADR-0002). The CMD build arg selects which cmd/{server,worker}
# binary the build stage compiles, producing -server or -worker images.
# syntax=docker/dockerfile:1.7
ARG GO_VERSION=1.26.3

# ─── stage 1: build ──────────────────────────────────────────────────────────
FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src

# Cached module download.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/root/go/pkg/mod go mod download

# Source: shared libs + only this service's tree, so editing one service doesn't
# invalidate the others' build/watch sets (skaffold rebuilds per artifact).
COPY libs/go ./libs/go

ARG CMD=server
ARG SERVICE=_template
COPY services/${SERVICE} ./services/${SERVICE}
RUN --mount=type=cache,target=/root/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
      -ldflags="-s -w" \
      -o /out/app ./services/${SERVICE}/cmd/${CMD}

# Migrations are baked in so the dbmate init container can read them at /db/migrations.
RUN mkdir -p /out/migrations && cp -r services/${SERVICE}/migrations/* /out/migrations/ 2>/dev/null || true

# ─── stage 2: runtime ────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
COPY --from=build /out/migrations /db/migrations
USER nonroot:nonroot
ENTRYPOINT ["/app"]

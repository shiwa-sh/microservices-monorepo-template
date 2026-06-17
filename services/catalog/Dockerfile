# syntax=docker/dockerfile:1.7
ARG GO_VERSION=1.26.3
FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/root/go/pkg/mod go mod download
# Scoped copy: shared libs + only this service's source, so editing one service
# doesn't invalidate the others' build/watch sets (skaffold rebuilds per artifact).
COPY libs/go ./libs/go
ARG APP_CMD=server
ARG SERVICE=catalog
COPY services/${SERVICE} ./services/${SERVICE}
RUN --mount=type=cache,target=/root/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/app ./services/${SERVICE}/cmd/${APP_CMD}
RUN mkdir -p /out/migrations && cp -r services/${SERVICE}/migrations/* /out/migrations/ 2>/dev/null || true

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
COPY --from=build /out/migrations /db/migrations
USER nonroot:nonroot
ENTRYPOINT ["/app"]

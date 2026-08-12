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
ARG SERVICE=_template
COPY services/${SERVICE} ./services/${SERVICE}
# Build identity baked into the binary (ADR-0103) so the running pod self-reports
# its git SHA / release; passed by the build scripts + CI, VCS-stamp fallback when absent.
ARG GIT_SHA=""
ARG BUILD_VERSION=dev
ARG BUILD_TIME=""
RUN --mount=type=cache,target=/root/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
      -ldflags="-s -w \
        -X github.com/tabmadi/microservices-monorepo-template/libs/go/buildinfo.Version=${BUILD_VERSION} \
        -X github.com/tabmadi/microservices-monorepo-template/libs/go/buildinfo.SHA=${GIT_SHA} \
        -X github.com/tabmadi/microservices-monorepo-template/libs/go/buildinfo.BuiltAt=${BUILD_TIME}" \
      -o /out/app ./services/${SERVICE}/cmd/${APP_CMD}
RUN mkdir -p /out/migrations && cp -r services/${SERVICE}/migrations/* /out/migrations/ 2>/dev/null || true

FROM gcr.io/distroless/static-debian13:nonroot@sha256:f7f8f729987ad0fdf6b05eeeae94b26e6a0f613bdf46feea7fc40f7bd72953e6
COPY --from=build /out/app /app
COPY --from=build /out/migrations /db/migrations
# dbmate (static binary) co-located with the baked migrations so the chart's
# migration initContainer runs them from this image — no separate dbmate image.
COPY --from=amacneil/dbmate:2.27.0 /usr/local/bin/dbmate /usr/local/bin/dbmate
USER nonroot:nonroot
ENTRYPOINT ["/app"]

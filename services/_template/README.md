# `_template` service

The starting skeleton for every backend service. **Do not edit this directory
to add features** — copy it to `services/<your-name>/` and strip the build
tags. Use `scripts/new-service.sh <name>` to do this automatically.

## What's inside

| Path                          | Purpose                                                     |
|-------------------------------|-------------------------------------------------------------|
| `openapi.yaml`                | The service contract (ADR-0008). Source of truth.           |
| `cmd/server/main.go`          | HTTP server entry point (calls `obs.Init`, `dbmw`, etc.)    |
| `cmd/worker/main.go`          | Temporal worker entry point (ADR-0006)                      |
| `internal/handlers/`          | Generated-server-binding glue                               |
| `internal/workflows/`         | Owned Temporal workflows (ADR-0006)                         |
| `internal/activities/`        | Owned Temporal activities                                   |
| `internal/domain/`            | Pure types — no DB/HTTP/Temporal imports                    |
| `internal/store/queries/`     | sqlc input — SQL files (ADR-0007)                           |
| `internal/store/` (generated) | sqlc output — typed Go                                      |
| `migrations/`                 | dbmate migrations (ADR-0007)                                |
| `sqlc.yaml`                   | sqlc config for this service                                |
| `.mise.toml`                  | Service-local tasks (`run`, `worker`, `test`, `migrate`, …) |
| `Dockerfile`                  | Multi-stage build with `CMD` build arg (ADR-0002)           |

## Standard tasks

```sh
mise run run        # HTTP server
mise run worker     # Temporal worker
mise run test
mise run lint
mise run migrate    # dbmate up
mise run generate   # sqlc + openapi codegen
mise run build      # go build
```

## Build tags

Every `*.go` file in this directory has `//go:build _template`. This excludes
the template from the repo-wide `go build ./...`. When you scaffold a new
service, `scripts/new-service.sh` strips these tags.

# `_template` service

The starting skeleton for every backend service. **Do not edit this directory to add features** — copy it to `services/<your-name>/` and strip the build tags. Use `scripts/new-service.sh <name>` to do this automatically.

## What's inside

| Path | Purpose |
| --- | --- |
| `openapi.yaml` | The service contract (ADR-0303). Source of truth. |
| `cmd/server/main.go` | HTTP server entry point (calls `obs.Init`, `dbmw`, etc.) |
| `cmd/worker/main.go` | Temporal worker entry point (ADR-0302) |
| `internal/handlers/` | Implements the ogen-generated `Handler`; reads/writes via the sqlc store |
| `internal/workflows/` | Owned Temporal workflows (ADR-0302) |
| `internal/activities/` | Owned Temporal activities |
| `internal/store/queries/` | sqlc input — SQL files (ADR-0300) |
| `internal/store/` (generated) | sqlc output — typed Go |
| `migrations/` | dbmate migrations (ADR-0300) |
| `sqlc.yaml` | sqlc config for this service |
| `.mise.toml` | Service-local tasks (`run`, `worker`, `test`, `migrate`, …) |
| `Dockerfile` | Multi-stage build with `CMD` build arg (ADR-0101) |

## Checklist for a new service

`scripts/new-service.sh` copies the skeleton; these are the things it cannot decide for you. The full rationale is the service contract in [ADR-0205](../../docs/adr/0205-environment-parity.md); `mise run lint:service-contract` fails on anything missed here.

1. **Register a local port** in `scripts/lib/ports.sh`, and set the same `PORT` in your `.mise.toml` (the skeleton ships `80XX` so it fails loudly until you do). Ports must be unique; `:8080` is reserved for the local edge mapping.
2. **Trim `dep:*`** to what the service actually reads — drop `dep:temporal` with no worker, `dep:postgres` with no database.
3. **Add `svc:*`** for every other service you call over HTTP. Miss one and the service starts cleanly, then fails on the first call that leaves it.
4. **Write `.env.example`** covering every variable you read. It is what seeds a fresh clone's `.env`, so an unlisted variable becomes a silent default.
5. **Add a values file per environment** under `infra/gitops/services/<env>/values/`. This is the step that actually deploys you: the ApplicationSet generates one Argo Application *per values file*, so skipping an environment means you are absent there and nothing says so. If that is deliberate, declare `# platform/not-deployed: <env>` in your `.mise.toml` instead.
6. **Decide your edge exposure** in those values: `ingress.enabled` plus `ingress.resources`, and `networkPolicy.allowFrom` for east-west callers. An internal service must set `networkPolicy.allowFromGateway: false` — see `services/authz` for a worked example.
7. **Replace this README** with one describing what the service is for.

## Standard tasks

```sh
mise run server     # HTTP server
mise run worker     # Temporal worker
mise run test
mise run lint
mise run migrate    # dbmate up
mise run generate   # sqlc + openapi codegen
mise run build      # go build
```

## Build tags

Every `*.go` file in this directory has `//go:build _template`. This excludes the template from the repo-wide `go build ./...`. When you scaffold a new service, `scripts/new-service.sh` strips these tags.

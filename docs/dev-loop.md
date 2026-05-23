# Local development loop

Per [ADR-0003](adr/0003-cluster-topology.md), k3d is the only local runtime. The
**minimal profile** boots just the dependencies a single service needs (Postgres,
Temporal, OTel-LGTM, MinIO) and exposes them on canonical `localhost` ports. The
service under test runs on the host — `go run`, an IDE debugger, dlv, whatever.

This file is editor-agnostic. Any IDE that can load a `.env` file and run a Go
`main.go` works the same way.

## One-time setup

```sh
mise run setup                       # lefthook hooks
cp services/catalog/.env.example services/catalog/.env
# Fill DATABASE_URL's password (see comment in .env.example).
```

## Inner loop

```sh
mise run dev:up -- --minimal         # k3d + port-forwards (5432, 7233, 4317, 3000, 9000/9001)
mise run -C services/catalog migrate # dbmate up
mise run -C services/catalog run     # go run ./cmd/server  → http://localhost:8080
```

To debug, point your editor's Go run configuration at
`services/catalog/cmd/server/main.go` with the working directory set to the
service folder so `.env` is picked up. Breakpoints, evaluate-expression, and
hot-restart all work — the service is a plain host process.

Observability UI: <http://localhost:3000> (Grafana, no auth in dev).

## Teardown

```sh
mise run dev:down                    # stops port-forwards + deletes the k3d cluster
```

## When to use the **full** profile instead

`mise run dev:up` (no flag) brings up the gateway, auth stack, ArgoCD, and every
other chart. Use it when the bug only reproduces with Tyk, Kratos, SpiceDB, or
GitOps in the path. The inner-loop pattern above does not apply — services run
in-cluster via `mise run dev:build <service>` (build image + `k3d image import`
+ rollout restart).

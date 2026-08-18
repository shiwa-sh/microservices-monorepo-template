# Local environment reference

Lookup tables for the local cluster: ports, URLs, and the environment contract. How to *use* them is [dev-loop](../dev-loop.md); the decisions behind them are [ADR-0600](../adr/0600-local-development-loop.md) and [ADR-0306](../adr/0306-trust-tiers-and-urls.md).

## Service ports on the host

Assigned in `scripts/lib/ports.sh` and set as `PORT` in each service's `.mise.toml`. `lint:ports` enforces that the two agree and that no two services collide.

| Service | Host port |
| --- | --- |
| catalog | 8081 |
| orders | 8082 |
| orgs | 8083 |
| payment | 8084 |
| authz | 8085 |

`:8080` is unassigned, because the host maps it to the edge. In-cluster this changes nothing: pods still bind `:8080`.

Override for a single run with `PORT=` in that service's `.env`, which wins over the registry. Prefer editing the registry so callers' `<SVC>_URL` defaults stay correct.

## Forwarded dependencies

`mise run dev:forward` exposes the in-cluster dependencies to the host process and to tools like `psql`.

| Dependency | Host address |
| --- | --- |
| Postgres | `localhost:5432` |
| Temporal gRPC | `localhost:7233` |
| Temporal UI | `localhost:8233` |
| OpenFGA | `localhost:18080` |

OpenFGA is forwarded to `18080` rather than its own `8080` because the host maps `8080` to the edge. `OPENFGA_API_URL` in every `.env.example` points at `18080` to match.

## Product URLs

Everything is served from one origin, `https://dev.localtest.me:8443`. The hostname is real public DNS resolving to `127.0.0.1`, with a locally-issued wildcard certificate. The edge matches longest-prefix, so the specific routes win over the `/` catch-all.

Deployed environments terminate on `443` and omit the port.

| Path | Serves | Auth | Defined in |
| --- | --- | --- | --- |
| `/` | landing page, from the host dev server | public | `infra/local/edge-auth.yaml` |
| `/panel`, `/devportal` | authenticated frontend areas | session | `apps/frontend/src/proxy.ts` |
| `/auth/login`, `/auth/registration`, … | Kratos UI pages, from the host dev server | public | `infra/local/edge-auth.yaml` |
| `/auth/self-service`, `/auth/.well-known`, `/auth/sessions` | Kratos public API | public | `infra/local/edge-auth.yaml` |
| `/api/<resource>` | service APIs, flat namespace ([ADR-0306](../adr/0306-trust-tiers-and-urls.md)) | Oathkeeper | `infra/helm/service/templates/ingressroute.yaml` |
| `/api/rum` | browser-telemetry ingest | public | `infra/gateway/frontend-observability.yaml` |

## Ops URLs

One origin per tool under `*.ops.<host>`, never a product path. Every route is defined in `infra/gateway/ingressroutes.yaml`; an opt-in tool's route resolves to a backend only once its chart is enabled.

The **Auth** column is the always-on coarse gate: the `operator` claim plus an AAL2 session, with no authz call ([ADR-0304](../adr/0304-identity-and-authorization.md)). The optional per-tool `dashboard:<tool>#view` layer is off by default.

| URL | Tool | Extra login |
| --- | --- | --- |
| `grafana.ops.dev.localtest.me:8443` | Grafana — metrics, logs, traces, RUM, policy drops | none |
| `hubble.ops.dev.localtest.me:8443` | Hubble UI — live service map, flows, drop verdicts | none |
| `temporal.ops.dev.localtest.me:8443` | Temporal Web UI | none |
| `argocd.ops.dev.localtest.me:8443` | Argo CD | none |
| `lowdefy.ops.dev.localtest.me:8443` | Lowdefy admin console | none |
| `headlamp.ops.dev.localtest.me:8443` | Headlamp — Kubernetes debug UI, read-only | none |
| `pgweb.ops.dev.localtest.me:8443` | pgweb — read-only database inspector | none |
| `mailpit.ops.dev.localtest.me:8443` | Mailpit — the mail sink's viewer, non-prod only | none |
| `seaweedfs.ops.dev.localtest.me:8443` | SeaweedFS admin UI, non-prod only | **yes** — see below |

Grafana and Argo CD trust the edge and serve anonymously, so an operator who clears the gate lands straight on the tool. **The SeaweedFS admin UI is the exception**: it carries its own credential rather than trusting the edge, so it prompts for the pre-seeded local one after the edge gate. Its master and filer UIs are not routed ([ADR-0306](../adr/0306-trust-tiers-and-urls.md)); reach them by port-forward.

Mailpit holds every message the Kratos courier submits, so it is where a verification or recovery mail is read; production keeps no such store ([ADR-0307](../adr/0307-outbound-email.md)).

Without the edge, Grafana is still reachable by `kubectl -n platform port-forward svc/grafana 3000:80`. Mailpit likewise by `kubectl -n platform port-forward svc/mailpit 8025:8025`.

## Frontend environment contract

`mise run dev:frontend` sets these. Starting the dev server another way — an IDE run configuration, a debugger — needs the same values: copy `apps/frontend/.env.example` to `.env.local`, which Next loads before evaluating its config.

| Variable | Why it exists |
| --- | --- |
| `EDGE_PUBLIC_ORIGIN` | The **browser's** edge origin, so it carries the port. The server-action CSRF allowlist is derived from it. Server components fall back to it when the internal origin is unset, which is correct on the host where the two are the same. Unset, the first server-side fetch throws |
| `EDGE_INTERNAL_ORIGIN` | Where **this process** dials the edge. In-cluster that is the edge's `:443`, not the browser's `:8443` — one variable cannot be both, which is why there are two. Set only in-cluster |
| `NODE_TLS_REJECT_UNAUTHORIZED` | Local only. See the note below |

Browser-side calls need none of this: the client uses a relative `/api`, which is same-origin by construction. A server-side fetch has no document to resolve a relative URL against, so the origin is named once — and it must be configuration rather than the request's `Host` header, because that header is client-controlled and this fetcher forwards the user's session cookie.

`NODE_TLS_REJECT_UNAUTHORIZED` exists only while the local issuer mints a self-signed **leaf**. Under the local **CA** ([ADR-0600](../adr/0600-local-development-loop.md)) the certificate is trusted properly and the variable is not set; deployed environments never set either TLS variable.

## Task index

| Task | Does |
| --- | --- |
| `mise run setup` | install the git hooks. Once per clone |
| `mise run server` / `worker` | run a service natively; resolves the floor plus its declared dependencies |
| `mise run dev:forward` | port-forward the dependencies to the host. Leave running |
| `mise run dev:frontend` | run the frontend dev server against the edge |
| `mise run db:migrate` | apply every service's migrations to the local database |
| `mise run auth:seed` | seed the committed test identities |
| `mise run cluster:base` | the local floor |
| `mise run cluster:full` | the real charts at single replica, via Argo CD |
| `mise run cluster:add -- <name>` | build, import, and deploy one service or platform chart |
| `mise run cluster:remove -- <name>` | uninstall **and restore Argo auto-sync** |
| `mise run cluster:stop` / `cluster:delete` | stop, keeping the image cache; or delete |
| `mise run cluster:heal` | recover a cluster wedged after a host reboot |
| `mise run cluster:unwedge` | recover stalled image pulls ([http-proxy](../guide/http-proxy.md)) |
| `mise run e2e` / `e2e:smoke` | browser acceptance suites ([ADR-0601](../adr/0601-testing-strategy.md)) |
| `mise run perf` / `perf:smoke` / `perf:stress` / `perf:seed` | load suites ([perf runbook](../guide/performance-runbook.md)) |
| `mise run lint` / `format` | every language, including Markdown |

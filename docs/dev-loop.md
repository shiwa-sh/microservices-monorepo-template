# Local development loop

Per [ADR-0003](adr/0003-cluster-topology.md), k3d is the only local runtime.
`mise run cluster:up` creates the cluster and applies the lightweight dev
dependencies (Postgres, Temporal, SpiceDB) from `infra/local/deps.yaml`. The
inner loop itself is `skaffold dev` (`mise run dev`), which builds, deploys, and
live-reloads the services in-cluster.

This file is editor-agnostic. Any IDE that can load a `.env` file and run a Go
`main.go` works the same way.

## One-time setup

```sh
mise run setup                       # lefthook hooks
cp services/catalog/.env.example services/catalog/.env  # only for host-process debugging
```

## Inner loop (in-cluster)

```sh
mise run cluster:up          # k3d + deps (Postgres, Temporal, SpiceDB)
mise run dev                 # skaffold dev: build + deploy + live-reload all services
mise run dev -m catalog      # …or scope to a single service (others keep their last deploy)
```

`skaffold dev` port-forwards the service servers (e.g. orders → `localhost:8080`)
plus Postgres (`localhost:5432`) and the Temporal UI (`localhost:8233`) so local
tools like `psql` can reach them.

## Host-process debugging (optional)

To run one service on the host instead of in-cluster — for a debugger, dlv, or
faster iteration — use the deps' port-forwards. `mise run dev -m platform` brings
up Postgres on `localhost:5432` without deploying any service, then:

```sh
mise run -C services/catalog migrate   # dbmate up against localhost:5432
mise run -C services/catalog run       # go run ./cmd/server  → http://localhost:8080
```

To debug, point your editor's Go run configuration at
`services/catalog/cmd/server/main.go` with the working directory set to the
service folder so `.env` is picked up. Breakpoints, evaluate-expression, and
hot-restart all work — the service is a plain host process.

## Teardown

```sh
mise run cluster:down                    # stops port-forwards + deletes the k3d cluster
```

## Formatting & linting

`mise run format` / `mise run lint` cover every language, including Markdown.
Generated code is never linted or formatted: Go SDKs (ogen) and sqlc store code
are skipped via `exclusions.generated` in `.golangci.yml` (both `golangci-lint
run` and `golangci-lint fmt`), the TS SDKs and admin `_generated/` via
`biome.json`, and rumdl via `.rumdl.toml`.
Markdown is governed by **rumdl** (`.rumdl.toml`), the single source of truth for
both linting and formatting. `mise run format:md` (`rumdl fmt`) auto-fixes most
rules and runs on staged `.md` files via the lefthook pre-commit hook. For inline
editor warnings that match CI exactly, point your editor at rumdl's LSP
(`rumdl server`) — the repo stays IDE-neutral and ships no editor config.

**Tables** are the one thing `--fix` can't repair. `MD060` enforces *aligned*
tables (whitespace-padded columns) — the exact format the JetBrains
"Incorrect table formatting" inspection wants, so CI and the IDE agree. When a
table is flagged, align it with **Alt+Enter → "Reformat table"** in JetBrains
(note: plain `Ctrl+Alt+L` does *not* align Markdown tables — only that quick-fix
does). Outside JetBrains, align the columns by hand to satisfy CI.

## The full platform is not local

The edge (Traefik + Ory Oathkeeper), auth stack (Kratos), and GitOps (ArgoCD) are
**not** brought up by `cluster:up` — it only applies the lightweight deps above.
The full platform is delivered by ArgoCD in staging/prod (per
[ADR-0003](adr/0003-cluster-topology.md)). If a bug only reproduces with the edge,
auth, or GitOps in the path, reproduce it in a staging environment rather than locally.

### Heavier local option: `mise run cluster:full`

For testing the edge / NetworkPolicy / observability on a laptop without a staging
cluster, `mise run cluster:full:up` (scripts/cluster-full.sh) stands up the **same
charts production runs**, at a single replica ([ADR-0016](adr/0016-environment-parity.md)):
**Cilium** as the CNI (real NetworkPolicy + Hubble), **CNPG**, the **Temporal**
chart, the **SpiceDB** chart, in-cluster **MinIO**, the **observability** chart,
and Traefik + Ory (Kratos + Oathkeeper). It is installed directly with `helm` (not
ArgoCD — that reconciles from git@master, not your tree; for the ArgoCD path see
[cluster/gitops-local.md](cluster/gitops-local.md)).

Local diverges from prod **only** through one values overlay,
`infra/gitops/platform/local/values.yaml`, consumed the same way the ArgoCD
ApplicationSet consumes the dev/staging/prod overlays. The only genuine local
substitutions are: in-cluster MinIO instead of the off-cluster bucket (S3 API both
sides), a self-signed `*.dev.localtest.me` wildcard cert, and a **committed
throwaway age key** so SOPS decrypts locally exactly as it does in prod (the
`sops-operator` materialises every credential from
`infra/gitops/platform/local/secrets/platform.enc.yaml` — nothing is `kubectl
create secret`'d). Plan for ~16GB free RAM; a measured `full`-profile bring-up
settles around **5GB resident / <0.5 core idle** on the single k3d node. Tear down
with `mise run cluster:full:down`.

#### Profiles

`cluster:full:up` takes an optional **profile** that selects the component subset —
thin overlays on the same charts, for the different roles that need a partial
platform:

```sh
mise run cluster:full:up            # full (default): everything incl. edge + services
mise run cluster:full:up min        # Postgres only — a service author iterating on a DB
mise run cluster:full:up backend    # + Temporal + SpiceDB (workflows + authz)
mise run cluster:full:up obs        # observability + its MinIO backend (the LGTM/Faro slice)
```

The cluster, Cilium, namespace, TLS, and the SOPS secrets are the always-on
baseline; the profile gates everything else.

#### Endpoints (full profile)

Everything is served from one origin, **`https://dev.localtest.me:8443`** (real DNS
→ 127.0.0.1, self-signed wildcard TLS — accept the cert once). The edge (Traefik)
matches longest-prefix, so the specific routes below win over the `/` catch-all.

| URL | What it gives you | Auth | Defined in |
| --- | --- | --- | --- |
| `/` | Landing page (host-run `next dev`) | public | `infra/local/edge-auth.yaml` |
| `/panel`, `/admin`, `/devportal` | Frontend authenticated areas | Kratos session | `apps/frontend/src/proxy.ts` |
| `/auth/login`, `/auth/registration`, … | Kratos UI pages (host-run `next dev`) | public | `infra/local/edge-auth.yaml` |
| `/auth/self-service`, `/auth/.well-known`, `/auth/sessions` | Kratos public API | public | `infra/local/edge-auth.yaml` |
| `/api/catalog/`, `/api/orders/`, `/api/orgs/`, `/api/payment/` | Service APIs through the edge | Oathkeeper | `infra/helm/service/templates/ingressroute.yaml` |
| `/api/observability/faro` | Faro/RUM browser-telemetry ingest | public | `infra/gateway/frontend-observability.yaml` |

The **ops tier** (ADR-0017) is a separate origin per operator dashboard under
`*.ops.<host>` — never a product path. Each requires an authorized (AAL2 operator)
session; a bare login does not grant tool access.

| Ops URL | Tool | Auth | Defined in |
| --- | --- | --- | --- |
| `grafana.ops.dev.localtest.me/` | **Grafana** — LGTM dashboards | Oathkeeper (`dashboard:grafana#view`) | `infra/gateway/ingressroutes.yaml` |
| `hubble.ops.dev.localtest.me/` | Cilium **Hubble UI** — network-flow map | Oathkeeper (`dashboard:hubble#view`) | `infra/gateway/ingressroutes.yaml` |
| `temporal.ops.dev.localtest.me/` | **Temporal Web UI** | Oathkeeper (`dashboard:temporal#view`) | `infra/gateway/ingressroutes.yaml` |
| `minio.ops.dev.localtest.me/` | **MinIO console** (non-prod) | Oathkeeper (`dashboard:minio#view`) | `infra/gateway/ingressroutes.yaml` |
| `console.ops.<host>/` | **Lowdefy** admin console (deployed envs) | Oathkeeper (`dashboard:console#view`) | `infra/gateway/ingressroutes.yaml` |
| `argo.ops.<host>/` | **Argo CD** (deployed envs) | Oathkeeper (`dashboard:argo#view`) | `infra/gateway/ingressroutes.yaml` |

Grafana has its own login behind the Kratos gate — sign in with `admin` / `admin`
(the local `grafana.adminPassword`). Without the edge you can still reach it by
port-forward: `kubectl -n platform port-forward svc/grafana 3000:80`, then
<http://localhost:3000/> (it now serves at root, not a sub-path).

The `/api/*`, `/api/observability/*` and the `*.ops.<host>` dashboard routes only
exist with the `gateway`/`services`/`observability` components up (the `full`
profile); `min`/`backend`/`obs` bring up a subset (see [Profiles](#profiles)).
Argo CD and the Lowdefy console are deployed-env only (not in the local profile).

#### Login flow (full profile)

The edge serves `*.dev.localtest.me` on `:8443` (real DNS → 127.0.0.1, no
`/etc/hosts` edits). Auth-gated routes (e.g. the Hubble UI at
`https://hubble.dev.localtest.me:8443/`) redirect an unauthenticated browser to
Kratos at `…/auth/login`; register/login there and the redirect returns you to the
gated page. The Kratos session cookie is scoped to `dev.localtest.me` (parent
domain), so one login covers the edge and every `*.dev.localtest.me` subdomain. The landing page and `/auth` UI are served by a host-run `next dev`
(run `next dev -H 0.0.0.0` on the host — the dev server is not in-cluster), wired
through `infra/local/edge-auth.yaml`.

**There is no seeded user** — Kratos starts with an empty identity store. Create
one at <https://dev.localtest.me:8443/auth/register> with any email and a password
that clears Kratos' defaults (≥ 8 chars and not a known-breached password — it runs
a HaveIBeenPwned check, so `password123` is rejected); then log in with it. Email
verification is configured but the local SMTP sink isn't wired up, so verification
mail isn't delivered — login doesn't require it.

Start the host `next dev` with **`APP_ORIGIN=dev.localtest.me`** so the login and
registration **server actions** pass Next's Origin/CSRF check (it feeds
`serverActions.allowedOrigins` in `next.config.mjs`). Without it, form submits from
the edge origin are rejected as cross-origin:

```sh
APP_ORIGIN=dev.localtest.me next dev -H 0.0.0.0
```

The full Kratos self-service set is served under `/auth/` — `login`, `register`,
`recovery`, and `settings` (these are frontend pages, identical in every env, not
local-only).

> On a restricted network whose registry blocks **digest** pulls (only tags
> resolve), pre-pull the platform images by tag and `k3d image import` them; the
> upstream charts pin images by digest. A normal connection pulls them directly.

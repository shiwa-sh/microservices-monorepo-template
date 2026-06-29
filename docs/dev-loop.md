# Local development loop

Per [ADR-0003](adr/0003-cluster-topology.md), k3d is the only local runtime.
`mise run cluster:up` creates the cluster and applies the lightweight dev
dependencies (Postgres, Temporal, SpiceDB) from `infra/local/deps.yaml`. The
inner loop is **native execution**: you run the service you are changing directly
on the host against those dependencies — no image build, no in-cluster redeploy,
no file-watch on the hot path.

This file is editor-agnostic. Any IDE that can load a `.env` file and run a Go
`main.go` works the same way.

## One-time setup

```sh
mise run setup                       # lefthook hooks
cp services/catalog/.env.example services/catalog/.env  # only for host-process debugging
```

## Inner loop (native)

```sh
mise run cluster:up          # k3d + a CNI + deps (Postgres, Temporal, SpiceDB)
mise run dev:forward         # port-forward the deps to localhost (leave running in its own terminal)
mise run db:migrate          # apply each service's migrations to the local Postgres
# then run the service natively (any editor/IDE, or go run):
DATABASE_URL=postgres://dev:dev@localhost:5432/catalog?sslmode=disable \
  TEMPORAL_HOST_PORT=localhost:7233 SPICEDB_ENDPOINT=localhost:50051 \
  go run ./services/catalog/cmd/server      # → http://localhost:8080
```

`dev:forward` exposes Postgres (`localhost:5432`), Temporal (`7233` gRPC / `8233`
UI), and SpiceDB (`50051`) so the host process — and tools like `psql` — can reach
them. Re-running the service is just re-running the binary; there is nothing to
rebuild or redeploy. To debug, point your editor's Go run configuration at
`services/<svc>/cmd/server/main.go` with those env vars set; breakpoints and
hot-restart work because the service is a plain host process.

### Putting a service *in* the cluster (edge/auth/e2e)

When you need the service behind the edge (not the native hot path), do a one-shot
build-import-deploy — no watch loop:

```sh
mise run service:deploy -- catalog       # build → k3d image import → helm upgrade
mise run service:undeploy -- catalog     # helm uninstall
```

## Teardown

```sh
mise run cluster:down        # stops the cluster, keeps the image cache + volumes
mise run cluster:purge       # deletes the cluster (reclaims disk, forces a clean recreate)
```

## End-to-end & visual tests

End-to-end and visual-regression tests are owned by [ADR-0018](adr/0018-testing-strategy.md):
**Playwright** drives them from the repo-root `e2e/` workspace against the full platform.

```sh
mise run cluster:full         # the environment e2e runs against (ArgoCD-driven)
mise run e2e:smoke            # product golden path + a key dashboard render
mise run e2e                  # full suite: every journey, every dashboard, all visual baselines
```

The browser test is the acceptance gauge — a rendered, authenticated dashboard (Grafana,
Hubble, Temporal) is the proof the whole stack underneath is wired. A Go/shell **preflight
readiness** check runs first so a red e2e reads "infra down" vs "app broken". The suite ships a
committed deterministic test identity (an AAL1 user + an AAL2 operator); there is nothing to seed
by hand. Playwright's runner is Node — the **one** sanctioned Node tool in the repo
([ADR-0001](adr/0001-language-and-runtime.md)), scoped to `e2e/` and CI; everything else stays on Bun.

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

## The full platform: `mise run cluster:full`

The edge (Traefik + Ory Oathkeeper), auth stack (Kratos), and the data tier are
**not** brought up by `cluster:up` — it only applies the lightweight deps above.
For end-to-end work, the edge, auth, NetworkPolicy, or observability on a laptop,
`mise run cluster:full` (scripts/platform-up.sh) stands up the **same charts
production runs**, at a single replica ([ADR-0016](adr/0016-environment-parity.md)),
**delivered by ArgoCD** — the same engine staging/prod use: **Cilium** as the CNI
(real NetworkPolicy + Hubble), **CNPG**, the **Temporal** chart, the **SpiceDB**
chart, in-cluster **MinIO**, the **observability** chart, Traefik + Ory (Kratos +
Oathkeeper), and the Lowdefy console.

`cluster:full` creates the cluster, installs the two components ArgoCD cannot
bootstrap (the CNI and ArgoCD itself), plants the SOPS age key, then applies a
local root-app (`infra/gitops/bootstrap-local/`) that syncs committed **`master`**
from the remote. Ordering, readiness, and secret materialisation are ArgoCD's job
(sync waves), not a shell script's. Because it syncs committed `master`, services
run **CI-built images**; to put uncommitted service code in the cluster use
`service:deploy`, and for uncommitted infra see
[cluster/gitops-local.md](cluster/gitops-local.md).

Local diverges from prod **only** through one values overlay,
`infra/gitops/platform/local/values.yaml`, consumed the same way the ArgoCD
ApplicationSet consumes the dev/staging/prod overlays. The only genuine local
substitutions are: in-cluster MinIO instead of the off-cluster bucket (S3 API both
sides), cert-manager with a **self-signed** `*.dev.localtest.me` wildcard issuer
(same mechanism as prod's Let's Encrypt), and a **committed throwaway age key** so
SOPS decrypts locally exactly as it does in prod (the `sops-operator` materialises
every credential from `infra/gitops/platform/local/secrets/platform.enc.yaml` —
only the age key itself is created imperatively). Plan for ~16GB free RAM. Tear
down with `mise run cluster:down` (keep the cache) or `cluster:purge` (delete).

### Endpoints

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

`cluster:full` brings up the whole platform (edge, services, observability,
console); Argo CD itself is installed imperatively for the local full tier and is
reachable at `argo.ops.<host>` like the other dashboards.

### Login flow

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

## HTTP proxies

If your host routes egress through an HTTP proxy, configure it at the **environment
level**, not in the repo — `cluster-create.sh` carries no proxy logic. Docker
propagates its proxy settings into every container it starts, including the k3d
nodes, so in-cluster image pulls (and ArgoCD-synced workloads in `cluster:full`)
inherit it automatically.

Set it once in `~/.docker/config.json`:

```json
{
  "proxies": {
    "default": {
      "httpProxy": "http://proxy.example.com:8080",
      "httpsProxy": "http://proxy.example.com:8080",
      "noProxy": "10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,.svc,.svc.cluster.local,127.0.0.1,localhost,.localtest.me"
    }
  }
}
```

For the Docker **daemon** itself (so the daemon's own pulls are proxied too), use a
systemd drop-in — `/etc/systemd/system/docker.service.d/http-proxy.conf`:

```ini
[Service]
Environment="HTTP_PROXY=http://proxy.example.com:8080"
Environment="HTTPS_PROXY=http://proxy.example.com:8080"
Environment="NO_PROXY=10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,.svc,.svc.cluster.local,127.0.0.1,localhost,.localtest.me"
```

then `sudo systemctl daemon-reload && sudo systemctl restart docker`.

> **Loopback proxies** (e.g. `127.0.0.1:8080`, as some sandboxes use) are not
> reachable from inside a container as `127.0.0.1`. Point the k3d nodes at the host
> instead — substitute `host.k3d.internal` for `127.0.0.1`/`localhost` in the
> values above, and make sure that name resolves on the node (Docker normally adds
> it; if a restart drops it, re-add `<gateway-ip> host.k3d.internal` to the node's
> `/etc/hosts`).

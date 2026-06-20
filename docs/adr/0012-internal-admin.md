# ADR-0012: Internal Admin Tool

- **Status:** Accepted
- **Date:** 2026-05-19
- **Deciders:** Platform team
- **Related:** [ADR-0002](0002-monorepo.md), [ADR-0007](0007-data.md), [ADR-0008](0008-api-contracts.md), [ADR-0009](0009-api-gateway.md), [ADR-0010](0010-auth.md)

## Context

Operating ~100 services means a steady demand for back-office screens: list and edit rows in a
service's database, trigger one-off endpoint calls (resets, retries, recomputes), and occasionally
submit a small form to POST a body to a service. Today this is done with `psql`, `curl`, and Temporal
workflows manually started — uneven, error-prone, and gated on engineers who know the right
incantations.

The workload is concretely:

- **~90% CRUD over Postgres-backed resources** exposed via service REST APIs.
- **Buttons that call a service endpoint** (`POST /reset`, `POST /requeue/:id`, etc.).
- **Occasional small forms** to construct a request body.

Each service owns its own Postgres database per [ADR-0007](0007-data.md). The OpenAPI spec per
service ([ADR-0008](0008-api-contracts.md)) already describes the resource shapes and operations,
and clients are committed to `libs/{go,ts}/sdks/`.

We need an internal admin surface that meets these constraints:

- **OSS, free, self-hosted** ([ADR-0000](0000-platform-foundations.md)).
- **Configuration in Git, not in a UI.** Admin pages are reviewed and blame-able like any other code.
- **LLM-authorable.** The bulk of pages should be generated; humans (or an LLM) make small edits.
- **Approachable by backend Go engineers and DevOps** — people who read YAML fluently and React
  reluctantly. A "small change" must be a small change.
- **No second auth integration.** Reuse Kratos sessions from [ADR-0010](0010-auth.md).
- **Operational cost bounded to one stateless container.** No new database, no new operator.

## Decision drivers

1. **Declarative over imperative.** The admin pages live as YAML files in the repo. Editing a page
   is editing a file.
2. **REST through services, not direct SQL.** Admin actions go through the same REST endpoints that
   the frontend uses. Service boundaries from [ADR-0002](0002-monorepo.md) are not bypassed by
   admin convenience.
3. **Hand-written first, codegen when it pays off.** Day one, admin pages are hand-written (or
   LLM-written) YAML in `custom/`. The OpenAPI specs already committed per
   [ADR-0008](0008-api-contracts.md) make a generator possible, but it is added only when the admin
   surface is large enough that hand-writing CRUD pages hurts — not as day-one machinery.
4. **Zero new auth surface.** Lowdefy's built-in auth/sessions are not used. The edge
   authenticates; Lowdefy trusts the upstream identity header.
5. **Stateless deployment.** No MongoDB, no admin-tool database. Treat the admin tool like a
   static-site server: config in, traffic out.

## Decisions

### Tool: Lowdefy

[Lowdefy](https://lowdefy.com) (Apache 2.0, self-hosted) is the internal admin tool.

Rationale:

- **Pure YAML configuration.** Every page is a file of ~30 lines. Backend engineers and DevOps read
  YAML daily for Helm, Kubernetes, mise, and GitOps; the cognitive surface is familiar.
- **First-class REST connector (`AxiosHttp`).** Most pages are REST against service endpoints,
  matching the boundary discipline from [ADR-0002](0002-monorepo.md).
- **First-class Postgres connector (`Knex`)** as an escape hatch for read-only "show me the raw
  table" cases the REST API genuinely cannot serve.
- **LLM-friendly.** YAML is dense, declarative, and well-represented in training data.
- **Stateless when external auth is used.** No MongoDB needed in this deployment shape (see below).

Alternatives considered and rejected:

- **Appsmith** — config is positional widget-tree JSON, not Git-reviewable or LLM-editable in
  practice; OSS auth is limited.
- **ToolJet, Budibase, Retool** — AGPL/GPL licenses or SaaS-first.
- **Refine / react-admin** — capable and type-safe against our generated TS clients, but the
  authoring surface is React. Backend engineers and DevOps making "small changes" land in hooks
  and providers, which is the failure mode this ADR exists to avoid.
- **NocoDB, Directus** — UI-first authoring; AGPL or BSL.

### Deployment: stateless, behind the edge, no MongoDB

Lowdefy runs in-cluster:

- Helm chart under `infra/helm/platform/lowdefy/`, deployed by ArgoCD per [ADR-0004](0004-gitops.md).
- A single `lowdefy-server` container. Stateless. No persistent volume.
- **No MongoDB.** Lowdefy's built-in auth providers require a session store; we do not configure
  them, so the session store is not needed.
- Image tag pinned by SHA per [ADR-0002](0002-monorepo.md).

Auth is enforced at the edge:

- Traefik routes `/internal/admin/*` through Oathkeeper to the Lowdefy service ([ADR-0009](0009-api-gateway.md)).
- The route requires a valid Kratos session per [ADR-0010](0010-auth.md). Unauthenticated requests
  never reach Lowdefy.
- Oathkeeper forwards the authenticated identity as an `X-User-Email` (or equivalent) header. Lowdefy
  reads it through a request operator and exposes it to pages as `_request.headers.x-user-email`.
- Authorization (which users see which pages) is enforced by SpiceDB checks issued from Lowdefy
  pages via the same REST surface services use. The admin tool does not own its own RBAC.

### Repository layout

```text
apps/admin/
├── lowdefy.yaml                    # root config: connections, menu, global theme
├── custom/                         # hand-written / LLM-written pages (the day-one surface)
│   └── <service>/
│       └── *.yaml                  # CRUD pages, button-per-endpoint pages, joined views, forms
└── _generated/                     # codegen output, only once tools/admin-gen/ is added (deferred)
    └── <service>/
        ├── tables.yaml             # CRUD pages, one per resource
        └── actions.yaml            # button-per-endpoint pages
```

`apps/admin/` is the second application under `apps/` and is therefore the kind of decision
[ADR-0002](0002-monorepo.md) requires its own ADR for — this document is that ADR. Lowdefy server
itself is deployed from `infra/helm/platform/lowdefy/`; `apps/admin/` contains only the YAML configuration
mounted into it.

### Day-one authoring: hand-written / LLM-written pages in `apps/admin/custom/`

Every admin page is a YAML file under `custom/<service>/`:

- CRUD pages (list / detail / create / edit) over a service's resources, via its REST connector.
- A button (and a form, when the operation has a request body) per `POST`-style action endpoint.
- Joined views that aggregate across services, and one-off operational dashboards.

A backend engineer adding a button that calls a service endpoint edits one YAML file under
`custom/<service>/`. An LLM does the same. Neither needs to learn React. Because the OpenAPI spec
([ADR-0008](0008-api-contracts.md)) already describes every resource and operation, an LLM has
enough to scaffold a page directly from the spec on request — which is the leverage that makes a
standing generator unnecessary day one.

### Deferred: the `tools/admin-gen/` generator

When the admin surface grows large enough that hand-writing CRUD pages across many services becomes
repetitive, a Go program at `tools/admin-gen/` is added to generate `apps/admin/_generated/<service>/`
from `services/*/openapi.yaml`:

- For every OpenAPI path group tagged `admin:crud`, generate list / detail / create / edit pages in
  `tables.yaml`; for every operation tagged `admin:action`, a button/form page in `actions.yaml`.
- It emits **only** REST-connector pages; direct-DB pages are never generated.
- Invocation matches [ADR-0008](0008-api-contracts.md): `mise run gen:admin`, included in `mise run
  gen`, drift-checked by `ci-drift.yml`.

This is a latent capability — the `admin:crud` / `admin:action` tag convention and the `_generated/`
path are reserved for it — but the generator is not built until the page count justifies it. Until
then, `_generated/` does not exist and `custom/` is the whole surface.

### Connections: REST first, Postgres as escape hatch

`lowdefy.yaml` declares two kinds of connections:

- **REST connections** (`AxiosHttp`), one per service, baseURL pointing at the in-cluster service
  address. Auth is the user's Kratos session, forwarded by the edge as identity headers.
- **Postgres connections** (`Knex`), one per service database. **Read-only credentials only.** Used
  only for views the REST API cannot serve (cross-table joins for diagnostics, raw inspection).
  Mutations go through REST, never through the Postgres connection.

Connection credentials are sourced from External Secrets ([ADR-0005](0005-secrets.md)).

### What Lowdefy does not own

- **Dashboards, logs, traces** — Grafana ([ADR-0011](0011-observability.md)). Lowdefy pages link out.
- **External API consumers** — frontend devportal route group ([ADR-0009](0009-api-gateway.md)).
- **Long-running operations** — Temporal workflows ([ADR-0006](0006-temporal.md)). Admin pages start
  workflows via REST; they do not orchestrate them.

## Consequences

### Positive

- Admin pages are YAML files in the same repo, reviewed in PRs, blame-able, hand- or LLM-written day
  one, with a generator reserved for when the surface grows.
- LLM-driven authoring is the path of least resistance, not a workaround.
- Backend engineers and DevOps can make small admin changes without touching React.
- Single stateless container in the cluster — no MongoDB, no new operator, no new auth integration.
- The OpenAPI investment from [ADR-0008](0008-api-contracts.md) powers admin pages for free; new
  services get admin coverage automatically when they ship a spec.
- Service boundaries from [ADR-0002](0002-monorepo.md) survive: admin actions go through REST, not
  around it.

### Negative / Risks

- Lowdefy's community is smaller than React-based alternatives. If upstream maintenance stalls, we
  own a Node server with YAML configs. Mitigated by the fact that the generated artifacts are
  declarative and portable to a successor tool; the lock-in is the runtime, not the data.
- No compile-time type safety between an OpenAPI spec change and a Lowdefy page. Day one this is
  caught by review and by runtime errors surfacing on first page load post-deploy; once
  `tools/admin-gen/` exists, `mise run gen:admin` plus CI drift-check catches generated pages too.
- Direct-Postgres connections, even read-only, are a discipline risk: an engineer can add a
  mutation behind the read-only credential's back. Mitigated by enforcing read-only Postgres roles
  at the database level (the user literally cannot `UPDATE`), not by convention.
- Hand-writing CRUD pages does not scale to a large fleet. Accepted day one (LLM scaffolding from the
  spec keeps the per-page cost low); the `tools/admin-gen/` generator is added when the page count
  makes this bite. Its `_generated/` directory will then inflate PR diffs on spec changes, mitigated
  by the same conventions applied to other generated artifacts in [ADR-0002](0002-monorepo.md).

### Follow-ups

- `infra/helm/platform/lowdefy/` chart values (image tag, REST/Postgres connection config from secrets).
- `apps/admin/lowdefy.yaml` root config, with menu structure and global theme.
- **(Deferred)** `tools/admin-gen/` Go program with unit tests, generating `_generated/` from
  `services/*/openapi.yaml`; the `mise run gen:admin` task and its inclusion in `mise run gen` and
  `ci-drift.yml`; and a vacuum ruleset rule enforcing valid `admin:crud` / `admin:action` tags. Built
  when the admin surface justifies a generator, not day one.
- Postgres read-only role provisioning template in `infra/helm/platform/postgres/`, referenced by per-service
  Helm values.
- Traefik route for `/internal/admin/*` in `infra/gateway/` plus the Oathkeeper rule in
  `infra/auth/oathkeeper/`, including the Kratos session check and identity header forwarding.
- SpiceDB schema for admin authorization (which users see which services / actions).

## Rules

- The internal admin tool is Lowdefy, self-hosted, deployed via Helm + ArgoCD under
  `infra/helm/platform/lowdefy/`.
- Admin pages live as YAML in `apps/admin/`. Day one they are hand- or LLM-written under `custom/`.
  `_generated/` is reserved for the deferred `tools/admin-gen/` output (committed and drift-checked
  when that generator exists); it is not present until then.
- Admin actions go through service REST APIs by default. Direct Postgres connections are
  read-only, used only for views the REST API cannot serve, and enforced read-only at the database
  role level — not by convention.
- Lowdefy's built-in auth/sessions are not used. MongoDB is not deployed for Lowdefy.
- Authentication for `/internal/admin/*` is enforced at the edge by Oathkeeper's Kratos session check per
  [ADR-0010](0010-auth.md). Authorization is enforced via SpiceDB calls from Lowdefy pages.
- Once the `tools/admin-gen/` generator exists, the set of admin pages generated for a service is
  determined by `admin:crud` and `admin:action` tags on its OpenAPI operations; a service without
  these tags gets no generated admin pages. Until then, a service's admin pages are whatever is
  hand-written under `custom/<service>/`.
- `apps/admin/` is the only first-party application under `apps/` besides `apps/frontend/`. Adding
  a third application requires its own ADR per [ADR-0002](0002-monorepo.md).
- Lowdefy is pinned to a specific release tag in Helm values. Floating tags are forbidden per
  [ADR-0002](0002-monorepo.md).

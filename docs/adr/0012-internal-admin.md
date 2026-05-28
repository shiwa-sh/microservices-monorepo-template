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
3. **Codegen-first, hand-edit second.** Most pages are generated from the OpenAPI specs already
   committed per [ADR-0008](0008-api-contracts.md). The generator is the bulk of the leverage; the
   custom directory is the escape hatch.
4. **Zero new auth surface.** Lowdefy's built-in auth/sessions are not used. The gateway
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

### Deployment: stateless, behind the gateway, no MongoDB

Lowdefy runs in-cluster:

- Helm chart under `infra/helm/lowdefy/`, deployed by ArgoCD per [ADR-0004](0004-gitops.md).
- A single `lowdefy-server` container. Stateless. No persistent volume.
- **No MongoDB.** Lowdefy's built-in auth providers require a session store; we do not configure
  them, so the session store is not needed.
- Image tag pinned by SHA per [ADR-0002](0002-monorepo.md).

Auth is enforced at the gateway:

- Tyk routes `/internal/admin/*` to the Lowdefy service ([ADR-0009](0009-api-gateway.md)).
- The route requires a valid Kratos session per [ADR-0010](0010-auth.md). Unauthenticated requests
  never reach Lowdefy.
- Tyk forwards the authenticated identity as an `X-User-Email` (or equivalent) header. Lowdefy
  reads it through a request operator and exposes it to pages as `_request.headers.x-user-email`.
- Authorization (which users see which pages) is enforced by SpiceDB checks issued from Lowdefy
  pages via the same REST surface services use. The admin tool does not own its own RBAC.

### Repository layout

```text
apps/admin/
├── lowdefy.yaml                    # root config: connections, menu, global theme
├── _generated/                     # committed codegen output (drift-checked)
│   └── <service>/
│       ├── tables.yaml             # CRUD pages, one per resource
│       └── actions.yaml            # button-per-endpoint pages
└── custom/                         # hand-written / LLM-written
    └── <service>/
        └── *.yaml                  # overrides, joined views, custom forms
```

`apps/admin/` is the second application under `apps/` and is therefore the kind of decision
[ADR-0002](0002-monorepo.md) requires its own ADR for — this document is that ADR. Lowdefy server
itself is deployed from `infra/helm/lowdefy/`; `apps/admin/` contains only the YAML configuration
mounted into it.

### Codegen: `tools/admin-gen/`

A Go program at `tools/admin-gen/` reads `services/*/openapi.yaml` and produces
`apps/admin/_generated/<service>/`:

- For every OpenAPI path group tagged `admin:crud`, generate list / detail / create / edit pages
  in `tables.yaml`.
- For every operation tagged `admin:action`, generate a page with a button (and a form, if the
  operation has a request body) in `actions.yaml`.
- The generator emits **only** REST-connector pages. Direct-DB pages are never generated.

Invocation matches [ADR-0008](0008-api-contracts.md): `mise run gen:admin`, included in
`mise run gen:all`, drift-checked by `ci-drift.yml`.

### Hand-written and LLM-written pages: `apps/admin/custom/`

Anything not derivable from the OpenAPI spec lives in `custom/`:

- Joined views that aggregate across services.
- Custom forms for actions that need shaping before the request body is sent.
- One-off operational dashboards.

A backend engineer adding a button that calls a service endpoint edits one YAML file under
`custom/<service>/`. An LLM does the same. Neither needs to learn React.

### Connections: REST first, Postgres as escape hatch

`lowdefy.yaml` declares two kinds of connections:

- **REST connections** (`AxiosHttp`), one per service, baseURL pointing at the in-cluster service
  address. Auth is the user's Kratos session, forwarded by the gateway.
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

- Admin pages are YAML files in the same repo, reviewed in PRs, blame-able, generated where
  possible, hand-edited where not.
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
- No compile-time type safety between an OpenAPI spec change and a Lowdefy page. Mitigated by
  `mise run gen:admin` regenerating on spec change and CI failing on drift; runtime errors surface
  immediately on first page load post-deploy.
- Direct-Postgres connections, even read-only, are a discipline risk: an engineer can add a
  mutation behind the read-only credential's back. Mitigated by enforcing read-only Postgres roles
  at the database level (the user literally cannot `UPDATE`), not by convention.
- The `_generated/` directory inflates PR diffs on spec changes. Mitigated by the same conventions
  applied to other generated artifacts in [ADR-0002](0002-monorepo.md).

### Follow-ups

- `infra/helm/lowdefy/` chart values (image tag, REST/Postgres connection config from secrets).
- `apps/admin/lowdefy.yaml` root config, with menu structure and global theme.
- `tools/admin-gen/` Go program with unit tests. Generates `_generated/` from
  `services/*/openapi.yaml`.
- `mise run gen:admin` task; inclusion in `mise run gen:all` and `ci-drift.yml`.
- Spectral rule (or `tools/admin-gen/` lint pass) enforcing valid `admin:crud` / `admin:action`
  tags on OpenAPI operations.
- Postgres read-only role provisioning template in `infra/helm/cnpg/`, referenced by per-service
  Helm values.
- Tyk route definition for `/internal/admin/*` in `infra/gateway/`, including the Kratos session
  check and identity header forwarding.
- SpiceDB schema for admin authorization (which users see which services / actions).

## Rules

- The internal admin tool is Lowdefy, self-hosted, deployed via Helm + ArgoCD under
  `infra/helm/lowdefy/`.
- Admin pages live as YAML in `apps/admin/`. `_generated/` is codegen output, committed and
  drift-checked. `custom/` is hand-written or LLM-written.
- Admin actions go through service REST APIs by default. Direct Postgres connections are
  read-only, used only for views the REST API cannot serve, and enforced read-only at the database
  role level — not by convention.
- Lowdefy's built-in auth/sessions are not used. MongoDB is not deployed for Lowdefy.
- Authentication for `/internal/admin/*` is enforced at the gateway by Kratos session check per
  [ADR-0010](0010-auth.md). Authorization is enforced via SpiceDB calls from Lowdefy pages.
- The set of admin pages generated for a service is determined by `admin:crud` and `admin:action`
  tags on its OpenAPI operations. A service without these tags gets no generated admin pages.
- `apps/admin/` is the only first-party application under `apps/` besides `apps/frontend/`. Adding
  a third application requires its own ADR per [ADR-0002](0002-monorepo.md).
- Lowdefy is pinned to a specific release tag in Helm values. Floating tags are forbidden per
  [ADR-0002](0002-monorepo.md).

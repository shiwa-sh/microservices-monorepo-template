# ADR-0401: Internal Admin Tool

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0100](0100-language-and-runtime.md), [ADR-0101](0101-monorepo.md), [ADR-0201](0201-gitops.md), [ADR-0202](0202-secrets.md), [ADR-0300](0300-data.md), [ADR-0302](0302-temporal.md), [ADR-0303](0303-api-contracts-and-lifecycle.md), [ADR-0304](0304-identity-and-authorization.md), [ADR-0306](0306-trust-tiers-and-urls.md), [ADR-0500](0500-observability.md)
- **Decides:** Lowdefy pages over the service APIs are the admin surface, and every admin mutation goes through the Go API rather than raw SQL.

## Context

Operating a service fleet creates a steady demand for back-office screens. Absent a tool for it, that work is done with `psql`, `curl`, and hand-started workflows: uneven, error-prone, and gated on whoever knows the right incantations.

The workload is concretely:

| Share | Need |
| --- | --- |
| ~90% | CRUD over Postgres-backed resources exposed by service REST APIs |
| | Buttons that call a service endpoint — reset, requeue, recompute |
| | Occasional small forms constructing a request body |

Each service owns its own database ([ADR-0300](0300-data.md)), and each spec already describes the resource shapes and operations ([ADR-0303](0303-api-contracts-and-lifecycle.md)).

## Decision drivers

1. **Declarative over imperative.** Admin pages are files in the repo, so editing a page is editing a file ([ADR-0000](0000-platform-foundations.md), principle 1).
2. **REST through services, never direct SQL.** Admin convenience does not bypass service boundaries.
3. **Generated from the spec, custom for the rest.** The specs describe every resource, so the bulk is generated.
4. **A small change costs a small amount of work for whoever makes it.** Admin screens are edited by backend and platform engineers between other work, not by a frontend team. The cost of an edit is what it obliges the editor to re-enter — a build, a component library, a data-fetching layer, a review from another discipline — rather than the language it is written in.
5. **Zero new auth surface.** The edge authenticates; the tool trusts the upstream identity header.
6. **One stateless container.** No new database, no new operator.

## Considered options

| Option | Config format | Licence | Verdict |
| --- | --- | --- | --- |
| **Lowdefy** | **pure YAML**, roughly 30 lines per page | [Apache-2.0](https://github.com/lowdefy/lowdefy/blob/main/LICENSE) | **Chosen.** YAML is what these engineers already read daily for Helm, Kubernetes, mise, and GitOps, and it is dense and well-represented for LLM authoring. First-class REST and Postgres connectors, and stateless when external auth is used *(documented)* |
| **An `(admin)` route group in the existing frontend** | **React, in the app already being built** | n/a — first-party | **The baseline, and the real contender.** It adds no component and reuses the design system, session, and generated clients. It loses on driver 4: every CRUD screen is hand-written React by the people driver 4 describes as reluctant to write it, and the generated bulk in the chosen option is exactly that work. It also puts an operator-facing surface inside the product origin, against [ADR-0306](0306-trust-tiers-and-urls.md) |
| Appsmith | positional widget-tree JSON | Apache-2.0 | Not reviewable in a diff and not LLM-editable. Its OSS authentication is limited |
| Windmill | scripts plus a YAML app definition | [AGPL-3.0](https://github.com/windmill-labs/windmill/blob/main/LICENSE-AGPL) | The closest competitor on driver 1 — a real config-as-files story — and it is a workflow and job platform first, so adopting it puts a second execution engine beside Temporal ([ADR-0302](0302-temporal.md)) |
| ToolJet | UI-authored | AGPL-3.0 | Excluded on the authoring model before the licence matters |
| Budibase | UI-authored | GPL-3.0 | As above |
| Retool | UI-authored | proprietary, SaaS-first | Fails principle 3 outright |
| Refine, react-admin | **React** | MIT | Capable and type-safe against the generated clients, and the authoring surface is React. A "small change" lands in hooks and providers, which is the failure mode driver 4 exists to avoid |
| NocoDB | UI-first | AGPL-3.0 | A database UI rather than an application builder, and driver 2 forbids the direct-SQL shape it is best at |
| Directus | UI-first | [BUSL 1.1](https://directus.io/bsl) | As above, and the licence is not OSI-approved |

### The read-only database inspector

| Option | Shape | Read-only enforcement | Verdict |
| --- | --- | --- | --- |
| **pgweb** | a single Go binary | `--readonly` flag, plus a read-only database role | **Chosen.** The same one-stateless-container shape as Lowdefy, and two independent layers of read-only *(reasoned)* |
| pgAdmin | a stateful Python application | none — it is a full client | Against a single-binary toolchain ([ADR-0100](0100-language-and-runtime.md)), and it offers write access that the role must then take back |
| CloudBeaver | a Java server | connection-level | A heavier runtime again, for a superset of features break-glass does not use |
| `psql` through `talosctl` and a pod | none | the role only | Always available and the actual fallback. It is not a UI, and the point of this row is that the inspector exists to save an operator from needing it during an incident |

## Decision

### The write-path invariant

**Every admin mutation goes through the service's Go API, never through raw SQL.** This is load-bearing; the tool choice is secondary to it.

The API is where domain invariants are enforced, where the **dual write to OpenFGA** happens ([ADR-0304](0304-identity-and-authorization.md)), and where a workflow can be triggered ([ADR-0302](0302-temporal.md)). A raw-SQL write bypasses all three: it desynchronises the authorization store from the application database and skips every invariant the service guarantees.

It cuts two ways:

- **Lowdefy is correct only when pointed at the API.** Direct Postgres connections exist as a **read-only** escape hatch.
- **pgweb and `psql` can only speak raw SQL**, so they can never be the write path. They are sanctioned as break-glass **inspectors** — SELECTs during an incident — and are enforced read-only **at the database-role level**, not by convention. The user literally cannot `UPDATE`.

### Deployment

| Property | Value |
| --- | --- |
| Chart | `infra/helm/platform/lowdefy/`, reconciled by Argo CD |
| Shape | one stateless container, no persistent volume |
| Datastore | **none.** Lowdefy's built-in auth providers would require a session store; they are not configured, so it is not needed |
| Origin | `lowdefy.ops.<host>` ([ADR-0306](0306-trust-tiers-and-urls.md)), its own ops-tier origin behind the forward-auth, never a product path |
| Coarse gate | the `operator` claim plus AAL2, with **no authz call** ([ADR-0304](0304-identity-and-authorization.md)), so the admin console does not share fate with the product authorization plane |
| Identity | Oathkeeper forwards the authenticated identity as a header, which Lowdefy exposes to pages |
| Fine-grained authorization | enforced by `Checker` calls inside **the service APIs the pages call**. The admin tool owns no RBAC of its own |

### Repository layout

```text
apps/admin/
├── lowdefy.yaml       # root config: connections, page index, theme
├── pages/             # hand-written pages not tied to one service
├── custom/<service>/  # hand-written pages the generator cannot express
└── _generated/        # tools/admin-gen output — committed, drift-checked, never edited
    ├── pages.yaml     # page index: hand-written first, then generated
    └── <service>/
        ├── <resource>.yaml    # one CRUD page per x-admin: crud tag
        └── <operationId>.yaml # one action page per x-admin: action operation
```

`apps/admin/` is the second application under `apps/`, which [ADR-0101](0101-monorepo.md) requires an ADR for. This is that ADR. The server itself is deployed from its platform chart; `apps/admin/` holds only configuration mounted into it.

### Generation from the spec

`tools/admin-gen/` generates from the service specs, driven by two markers:

| Marker | Placed on | Produces |
| --- | --- | --- |
| `x-admin: crud` | a tag | one CRUD page: a list table, plus a create form where a collection `POST` returns **201**, an edit form where `PUT` exists, and a delete control where `DELETE` exists |
| `x-admin: action` | an operation | one action page: inputs for path parameters and request body, and a button that calls the endpoint |

An async `202` create is a workflow trigger rather than a scaffoldable form, so a resource whose create is async stays list-only.

The generator emits **only REST-connector pages**, calling each service in-cluster. Direct-database pages are never generated. Pages use raw spec paths, so output is independent of the edge prefix. Output is deterministic, committed, and drift-checked, invoked as `mise run gen:admin` inside `mise run gen`.

The `x-admin` markers are specification extensions, so they are stripped from the rendered developer portals and never leak into public docs.

Because the spec already describes every resource, a page needing shaping the generator does not do can be hand-scaffolded under `custom/` directly from the spec. Neither a human nor an LLM needs to learn React.

### Connections

| Kind | Purpose | Credentials |
| --- | --- | --- |
| REST, one per service | **the write path.** Auth is the user's session, forwarded by the edge as identity headers | — |
| Postgres, one per service database | **read-only.** Views the REST API cannot serve: cross-table joins for diagnostics, raw inspection | read-only roles, materialised from SOPS-encrypted files by the operator ([ADR-0202](0202-secrets.md)) |

### What would change this decision

| Change | Effect |
| --- | --- |
| The admin surface is edited by a frontend team | **Decisive.** Driver 4 is a claim about who makes the edit. Where that is people who write React daily, the `(admin)` route group wins on every remaining driver |
| A page needs interaction the config format cannot express | **None, once.** That is what `custom/` is for. **Decisive if it recurs**, because a config tool carrying a growing hand-written region is a framework with extra steps |
| The design system becomes a requirement for admin screens | **Decisive.** Operator tooling is deliberately outside the design contract ([ADR-0400](0400-frontend.md)); making it inside means the surface belongs in the frontend app |
| Upstream stalls | **Decisive**, on the trigger recorded in *Negative / Risks* below |
| An admin action needs to bypass a service API | **None.** The write-path invariant outranks the tool, and a mutation the API cannot express is a missing endpoint |

### What this tool does not own

| Concern | Owner |
| --- | --- |
| Dashboards, logs, traces | Grafana ([ADR-0500](0500-observability.md)). Admin pages link out |
| External API consumers | the devportal route group ([ADR-0400](0400-frontend.md)) |
| Long-running operations | Temporal ([ADR-0302](0302-temporal.md)). Admin pages start workflows over REST; they do not orchestrate them |

## Consequences

### Positive

- Admin pages are files in the same repo, reviewed in PRs and blame-able.
- LLM-driven authoring is the path of least resistance rather than a workaround.
- Backend and platform engineers make small admin changes without touching React.
- One stateless container: no datastore, no operator, no new auth integration.
- The contract investment powers admin pages for free — a new service gets coverage when it ships a spec.
- Service boundaries survive, because admin actions go through REST rather than around it.

### Negative / Risks

- **Lowdefy's community is the smallest of any component on the floor.** If upstream stalls, the platform owns a server with YAML configs. The configuration is declarative and portable, so the lock-in is the runtime rather than the data — but the pages are the smaller half of what is lost. The larger half is the operator habits built on them, and those transfer to nothing.

**The fallback is named, and it is the runner-up.**

| Field | Value |
| --- | --- |
| **Trigger** | upstream releases stop for two quarters, or a security advisory against the runtime goes unanswered for one |
| **Seam** | ✓ the `(admin)` route group in the existing frontend, costed in *Considered options* above. The write path is unchanged, because both authoring surfaces call the same service APIs, and the generated CRUD pages are regenerated rather than ported |
| **Cost if adopted late** | every hand-written custom page is rewritten in React once, and the surface moves from an ops-tier origin into the product app, which [ADR-0306](0306-trust-tiers-and-urls.md) treats as a tier change rather than a relocation |

This makes the weakest dependency on the floor a **deferral rather than a bet**: the alternative exists, it is first-party, and it is already compared.

- **No compile-time type safety between a spec change and a page.** The generator and drift check keep generated pages in step; hand-written pages are caught only by review and by runtime errors on first load.
- **Read-only Postgres connections are a discipline risk**, since an engineer could try to add a mutation behind them. Mitigated by enforcement at the database-role level rather than by convention.
- **The generated directory inflates PR diffs** on spec changes. Mitigated by the same committed-and-drift-checked convention applied to every other generated artifact. Generated pages are deliberately plain; anything richer is a custom page.
- **It is a Node island** ([ADR-0100](0100-language-and-runtime.md)), installed and run rather than built, so the runtime is upstream's choice. Confined to this app's image and its optional local tasks.

## Rules

- The internal admin tool is Lowdefy, self-hosted and deployed via Helm and Argo CD.
- Admin pages live as YAML in `apps/admin/`. The generated directory is never hand-edited. `(CI: ci:gen)`
- **Every admin mutation goes through the service Go API, never raw SQL.**
- Direct Postgres connections — Lowdefy's and pgweb's — are read-only, used only for inspection the REST API cannot serve, and enforced read-only at the database-role level.
- Lowdefy's built-in auth and sessions are not used, and no datastore is deployed for it.
- The admin console is served on its own ops-tier origin behind the forward-auth. Fine-grained authorization is the `Checker` call inside the service APIs the pages call.
- Generated pages are determined by the spec markers. A service with neither marker gets no generated pages. `(CI: lint:openapi)`
- `apps/admin/` is the only first-party application under `apps/` besides the frontend. A third requires its own ADR.
- Lowdefy is pinned to a specific release tag. `(CI: lint:floating-tags)`

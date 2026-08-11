# Your first change

[`reading-path`](../reading-path.md) takes you to comprehension. This takes you to a merged pull request.

One worked change — **adding a field to a service** — walked through every gate it passes. The change is deliberately small, because the point is not the field. The point is that a two-line edit to an OpenAPI file touches eight mechanisms, and knowing which one is talking to you is most of what makes this platform fast to work in rather than slow.

**What you need first:** a local floor. `mise run cluster:base`, and [`../dev-loop.md`](../dev-loop.md) if it does not come up.

## The change

`catalog` serves products. You are adding a `restock_at` timestamp to the product resource — readable on `GET`, writable on `PUT`, and stored.

## 1. The contract, first and always

Edit `services/catalog/openapi.yaml`. Nothing else, yet.

```yaml
    restock_at:
      type: string
      format: date-time
      description: When the next restock is expected.
```

**The spec is the source, not a description of the code.** Handlers, clients, and validators are generated from this file, so a field that is not here does not exist ([ADR-0303](../adr/0303-api-contracts-and-lifecycle.md)).

Three rules bind that field, and each has a gate waiting:

| Rule | Why | Gate |
| --- | --- | --- |
| Timestamps are RFC 3339, UTC, with a literal `Z` | one wire format, and a non-`Z` offset is rejected rather than converted ([ADR-0003](../adr/0003-naming-and-identifiers.md)) | `lint:openapi` |
| Property names are `snake_case` | the wire casing is fixed per surface | `lint:openapi` |
| Shared shapes come from `tools/codegen/shared-components.yaml` | timestamps, identifiers, and the error envelope are identical across specs because they are one file, not five copies | `lint:openapi` |

**If the field were money, stop and read [ADR-0003](../adr/0003-naming-and-identifiers.md) before typing.** A monetary field typed `number` is a contract defect, because the generated TypeScript client parses it as a double. It is a `string` carrying a decimal, backed by Postgres `numeric`.

## 2. Generate

```sh
mise run gen
```

This rewrites four trees from the edited spec:

- `services/catalog/internal/api/` — the ogen server interface, with validation compiled into the decoder
- `libs/go/sdks/catalog/` — the Go client other services call through
- `libs/ts/sdks/catalog/` — the TypeScript client the frontend calls through
- `apps/admin/_generated/` — the Lowdefy CRUD pages

**Generated code is committed** ([ADR-0000](../adr/0000-platform-foundations.md), principle 9). Commit it in the same pull request as the spec change. It is never hand-edited, never linted, and never formatted — if the output is wrong, the input is wrong.

Your build now fails, and that is the mechanism working: ogen widened the server interface, so the compiler is telling you which handler is missing the field.

## 3. Migration

```sh
cd services/catalog
mise run db:new -- add_restock_at
```

Write the `up` and the `down`. The authored SQL is what runs — there is no schema DSL between you and the database ([ADR-0300](../adr/0300-data.md)).

```sql
ALTER TABLE products ADD COLUMN restock_at timestamptz;
```

`timestamptz`, never `timestamp`. And if the column held personal data it would need its class recorded in this same migration, because the creating migration is the only place that fact is authored:

```sql
COMMENT ON COLUMN products.owner_email IS 'pii:contact';
```

`lint:sql` checks both.

## 4. Query and handler

Add the column to the sqlc query, regenerate, then wire the handler. Two rules apply here and neither is obvious from the code around them:

- **No cross-service import.** If `restock_at` needs something from `orders`, it arrives over the generated client, never as a Go import ([ADR-0101](../adr/0101-monorepo.md)). `depguard` fails the build if you try.
- **Errors use the platform envelope**, not a bespoke shape. Return the RFC 9457 problem type; `trace_id` is filled from the active span by `libs/go/observability` rather than by your handler ([ADR-0303](../adr/0303-api-contracts-and-lifecycle.md)).

If the change were authz-relevant — a field that decides who can see a resource — it would belong in a Temporal workflow rather than a bare store method, because the database write and the OpenFGA tuple write have to succeed or fail together ([ADR-0302](../adr/0302-temporal.md), [ADR-0304](../adr/0304-identity-and-authorization.md)).

## 5. Run it

```sh
mise run db:migrate
mise run server                     # terminal 1
mise run service:dev -- catalog     # terminal 2, once
```

Then exercise it through `https://dev.localtest.me:8443/api/products` rather than against the port directly. **Curling the port hands your service forged identity headers**, so a request that works there and fails through the edge is the single most common local surprise ([`../dev-loop.md`](../dev-loop.md)).

## 6. What the pull request runs

Nine gates, in the order they will fail you:

| Gate | Rejects | Where |
| --- | --- | --- |
| `ci:gen` | your generated trees not matching your spec — the drift check | before anything else is worth reading |
| `lint:openapi` | casing, timestamp format, a divergent shared component, a missing `example` on a 2xx response | the spec |
| `lint:service-contract` | a resource prefix colliding with another service's | the spec |
| `oasdiff` | a breaking change, which **labels** the pull request rather than blocking it | the spec, against `master` |
| `lint:sql` | a `timestamp` where `timestamptz` belongs, an untagged PII column | the migration |
| `lint:api-audience` | a route exposed at the wrong audience | the spec and the edge config |
| `ci:lint` | everything else — Go, TypeScript, Markdown, shell | the whole diff |
| `ci:affected` | nothing. It **selects** what the rest of CI runs | the diff |
| `ci:test` | unit and integration tests for the selected services | the selection |

**`ci:affected` is the one to understand.** It reads your diff and decides which services CI exercises. A change it does not select is a change nothing tested, and that is a known accepted risk rather than a bug ([`../reference/risk-register.md`](../reference/risk-register.md), row 8). If your change is subtle — a shared library, a chart value, a generated tree — check that the services you expect appear in the affected output.

**Add the `e2e` label if your change touches more than one service.** The full browser suite is nightly, so without the label a cross-service regression sits in `master` until the morning ([`../reference/build-path.md`](../reference/build-path.md)).

## 7. What review is for

Most rules here are enforced by a machine, and the ones that are not are the ones review exists for. A reviewer is reading for the four classes nothing checks:

- **A secret in a committed file.** This is the one defect that survives its own fix — reverting the file leaves it in history, so remediation is a rotation.
- **An authorization check omitted**, or an authz-relevant write outside a workflow.
- **A rule enforced at the wrong layer** — something asserted in a CI lint that a `kubectl apply` would route around ([ADR-0203](../adr/0203-policy-enforcement.md)).
- **A comparison a decision needed and does not have.** If your change adopts a tool, [ADR-0002](../adr/0002-tool-adoption.md) says what it owes.

Everything else, a machine already said.

## What you did not have to do

Worth noticing, because it is the payment for the density of the rest:

- No version to bump. Releases are CalVer over the repository, computed at release time ([ADR-0103](../adr/0103-release-and-versioning.md)).
- No client to hand-write, in either language, and no admin page to build.
- No chart to edit. The service chart is shared and takes values ([ADR-0201](../adr/0201-gitops.md)).
- No environment to configure. Local, dev, staging, and production take the same chart and differ in values ([ADR-0205](../adr/0205-environment-parity.md)).
- No deploy step. Merging to `master` is the deploy; Argo CD reconciles it.

## When the change is bigger than a field

| The change | Read first |
| --- | --- |
| A new service | [ADR-0000](../adr/0000-platform-foundations.md)'s six decomposition forces. A new service names the one that justifies it |
| A new platform component | [`../operational-surface.md`](../operational-surface.md)'s budget rule, then [ADR-0002](../adr/0002-tool-adoption.md) for the comparison it owes |
| A breaking API change | [ADR-0303](../adr/0303-api-contracts-and-lifecycle.md). `oasdiff` labels it; the ADR says what you do about the label |
| Anything binding more than one service | an ADR. [`../adr/_template.md`](../adr/_template.md) is the start |

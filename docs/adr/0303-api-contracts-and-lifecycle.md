# ADR-0303: API Contracts, Codegen & Lifecycle

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0100](0100-language-and-runtime.md), [ADR-0101](0101-monorepo.md), [ADR-0103](0103-release-and-versioning.md), [ADR-0302](0302-temporal.md), [ADR-0305](0305-edge-auth-and-traffic-policy.md), [ADR-0306](0306-trust-tiers-and-urls.md), [ADR-0400](0400-frontend.md)

## Context

Services expose APIs to three consumer groups:

| Consumer | Transport | Upgrade coupling |
| --- | --- | --- |
| The Next.js frontend ([ADR-0400](0400-frontend.md)) | browser → edge | ships in the same commit and release |
| Other services | in-cluster HTTP | ships in the same release |
| Third parties | edge, when a public API is flagged on | cannot be forced to upgrade |

The platform needs one source of truth per API surface, generated code in Go and TypeScript, generated request validation, a workflow in which contract drift cannot survive CI, coverage for streaming, and a rule for how a live contract evolves.

Wire efficiency for internal calls is not a priority. JSON over HTTP everywhere: operational simplicity, browser-friendliness, gateway-friendliness, and human-debuggability outrank binary-protocol throughput.

## Decision drivers

1. **One contract, two languages.** Go server and TS client from the same artifact.
2. **Single validation surface.** One generated validator in the service; no parallel edge-validation config.
3. **Public API readiness.** Third-party consumers expect OpenAPI docs and SDKs.
4. **Browser fit.** No proxies, no Envoy sidecars, no `grpc-web`.
5. **Spec-first, enforced.** CI fails on stale generated code or hand-written shadow types.
6. **Version machinery matches consumer reality.** A co-shipped consumer needs none; an uncontrolled one needs it.

## Considered options

### Contract format

| Option | Browser | Public consumers | Cost |
| --- | --- | --- | --- |
| **OpenAPI 3.1 + ogen + openapi-typescript** | native `fetch` | docs and SDKs generated | **Chosen** — one spec drives server, both clients, docs, SDKs |
| gRPC + `grpc-web` | needs an Envoy/Connect proxy, loses streaming semantics | expect OpenAPI anyway | a proxy tier plus a second IDL |
| Connect-RPC (Buf) | speaks HTTP/JSON | workable | an RPC framework where OpenAPI/JSON already satisfies every consumer. Revisit only if a binary protocol is needed |
| GraphQL | good | poor fit for service-to-service | a query planner is a platform component the budget does not hold ([ADR-0000](0000-platform-foundations.md)) |
| tRPC | good | none | TypeScript-only; the backend is Go |

### Version scheme, for when online versioning is flagged on

| Option | URL stability | Why not |
| --- | --- | --- |
| **Date in a request header** (`Api-Version: 2026-07-01`) | flat resource URL preserved | **Chosen** — the dominant convention for continuously-evolving REST (Stripe, GitHub, Azure), and one calendar spans release and contract |
| SemVer major in path (`/api/v2/...`) | forks the URL | Fights the topology-hidden flat URL ([ADR-0306](0306-trust-tiers-and-urls.md)). Its value is the breaking signal to an independent pinner, which pin-and-sunset already delivers |
| Google AIP path-major | forks the URL | Exposes only the major and serves both from one backend — the same idea with a coarser label |

## Decision

The contract source of truth is **OpenAPI 3.1**, one spec per HTTP service at `services/<service>/openapi.yaml`.

### Scope: mandatory, no per-service exemption

Every service that serves HTTP ships a spec and implements the ogen-generated `Handler`, including east-west control-plane services. Only a service with no HTTP surface — a pure worker — ships no spec. `mise run gen` discovers specs by glob.

The `authz` service ([ADR-0304](0304-identity-and-authorization.md)) owns no database and sits behind Oathkeeper rather than the `/api` edge, and is spec-first regardless: its `/authorize` decision endpoint and `/operators` action are ogen operations. OpenFGA is the source of truth for the authorization *model*; the spec is the source of truth for authz's *HTTP contract*. The two describe different things.

Uniformity buys one mental model, one toolchain, and tooling (admin-gen, linters, drift-check) that assumes a spec always exists. It costs a few unused artifacts — ogen emits an authz client no caller imports. The trade favours uniformity.

### Specs are self-contained

Cross-service shapes — the error envelope, common ID and time types, the workflow handle ([ADR-0302](0302-temporal.md)) — are declared in each spec's own `components`, not imported by cross-file `$ref`. No external file reference means no resolution step, and every spec stays portable across the codegen and lint tools. The shapes are duplicated by convention and kept identical.

### URL shape: flat resource namespace

| Property | Value |
| --- | --- |
| `servers` url, edge-exposed | `/api` |
| `servers` url, east-west only | `/` |
| Paths | globally-unique resource nouns — `/products`, `/orders`, `/charges` |
| Exposed URL | `<host>/api/<resource>`, no service segment ([ADR-0306](0306-trust-tiers-and-urls.md)) |
| Version segment | none |

The service owning a resource is a hidden edge-routing detail. Because all specs share one `/api` namespace, two edge-exposed specs must not claim the same top-level resource prefix; vacuum lint fails on collision.

### Codegen

| Output | Tool | Location |
| --- | --- | --- |
| Go server, client, types | `ogen` — type-safe, OpenTelemetry-instrumented | `libs/go/sdks/<service>/` |
| TS client | `openapi-typescript` + `openapi-fetch`, ~6 KB runtime | `libs/ts/sdks/<service>/` |
| Public SDKs | OpenAPI Generator | published per language as third-party consumers arrive |

All generated artifacts are committed and drift-checked in CI ([ADR-0101](0101-monorepo.md)).

OpenAPI YAML is hand-written. TypeSpec and equivalent authoring layers are not used. A spec that grows unwieldy is a signal to split the service or factor shapes into more `components`, not to add a second authoring tool.

### Workflow

1. An API change is a PR to `services/<service>/openapi.yaml`.
2. CI runs **vacuum** with the repo ruleset at `tools/codegen/openapi-ruleset.yaml` — style and breaking-change detection.
3. `mise run gen` regenerates the Go server, Go client, and TS client.
4. CI fails if generated files are stale (`git diff --exit-code`).
5. Hand-written code imports generated types and declares no parallel ones.

### Validation

Schema validation is **service-side only**. `ogen`'s generated server decodes and validates every request into typed Go values; the validator is generated from the same spec and costs nothing to maintain. The service also owns all business-rule validation — ownership, limits, state transitions, idempotency.

The edge does not validate request bodies. Internal service-to-service calls bypass it, so the service is the only place that sees every request; a second edge validator would be redundant rather than defence in depth.

### Streaming

| Mechanism | When | Declaration |
| --- | --- | --- |
| Server-Sent Events | default for server→client push | `text/event-stream` response; Traefik passes through |
| Server-streaming over HTTP/2 | binary frames or high throughput | chunked-transfer response, documented per endpoint |
| WebSockets | bidirectional | `services/<service>/README.md` with a JSON Schema for message envelopes, plus a one-line justification per endpoint |

Traefik handles WS upgrades. gRPC and Connect are not introduced for streaming.

### Audience and visibility

`x-audience` classifies each surface so the developer portals ([ADR-0400](0400-frontend.md)) render filtered projections of the same specs rather than separate hand-maintained documents. It is **one ordered ladder** — a widening audience boundary.

| Value | Meaning | Edge-reachable |
| --- | --- | --- |
| `cluster` | east-west, in-cluster only, gated by NetworkPolicy | no |
| `internal` | first-party edge surface; curated out of public docs | yes, `/api` |
| `public` | third-party edge surface | yes, `/api` |

Resolution order: the operation's `x-audience`, else the service default on `info`, else `cluster`. The fail-closed default means a spec is never treated as edge-reachable unless it says so. A mostly-`public` service can mark one write operation `internal`; an otherwise-edge service can mark an east-west webhook `cluster`.

The field follows Zalando's `x-audience` convention. The three values simplify Zalando's enum, and folding operation-level visibility onto the same axis replaces a separate Redocly-style `x-internal` flag — one ladder, no second label to reconcile.

**`x-audience` is documentation scoping, not access control.** Exposure is decided by the edge route and Oathkeeper ([ADR-0305](0305-edge-auth-and-traffic-policy.md)); a service with `ingress.enabled: false` is unreachable whatever its spec says. A CI check ties the ladder to reality: a service is edge-exposed **iff** it has at least one `internal`/`public` operation, and a `cluster`-only service has no `/api` route. That pair defines an east-west endpoint ([ADR-0306](0306-trust-tiers-and-urls.md)); such endpoints appear in no portal.

### Lifecycle default: one live version

The live API is whatever the current production release ([ADR-0103](0103-release-and-versioning.md)) serves. There is no version in the path, no version header, and no support window.

- **A breaking change is a normal PR.** The only consumer is the co-shipped frontend, so a breaking change updates its caller in the same commit and they deploy together. The previous shape is not kept alive.
- **The contract diff is a review signal, not a gate.** `oasdiff` labels a change breaking so the break is intentional and visible in review. It does not require a version bump or a second live surface.
- **No `Deprecation`/`Sunset` machinery, no transformation layer, no N-1.** Those belong to the deferred path.

Versioning schemes protect consumers who cannot be forced to upgrade. With one in-tree consumer, that cost buys nothing.

### Lifecycle upgrade: online versioning, flagged off

**Trigger:** a consumer the project cannot deploy in lockstep — a third party, a partner, or a first-party mobile app on its own cadence. **Seam:** the header is read at the edge and the compat layer wraps the handler, so adoption is additive. **Cost if adopted late:** the first out-of-lockstep consumer is unsupported until the flag flips.

Nothing below is built or operated until then.

| Element | Rule |
| --- | --- |
| Version label | date header, `Api-Version: 2026-07-01` |
| Where dates come from | the subsequence of release dates on which the public contract changed visibly ([ADR-0103](0103-release-and-versioning.md)). Most releases mint no API version |
| Missing header | resolves to latest; the resolved date is echoed in the response. Clients are advised to pin |
| Support window | N-1. At most two live versions; the previous is sunset on a documented date |
| Deprecation signalling | `Deprecation` (RFC 9745) + `Sunset` (RFC 8594) + `Link` to migration notes |
| Compatibility | by transformation, bounded to N-1. Handlers produce only the latest shape; a thin response layer maps it back one version |

## Consequences

### Positive

- One artifact powers server, internal client, frontend client, docs, and public SDKs. Per-service contract cost is fixed regardless of consumer count.
- Every consumer sees the same API shape; no protocol-translation layer.
- Schema validation is generated, not written.
- Public API readiness is a CI artifact, not a project.
- The default runs zero version machinery, and the upgrade path is designed rather than improvised under pressure.

### Negative / Risks

- OpenAPI is awkward for discriminated unions and conditional schemas. Mitigated by vacuum rules enforcing flat schemas; complex polymorphism signals an over-coupled API surface.
- The streaming story is pragmatic rather than unified. Mitigated by per-WebSocket justification.
- Cross-service shapes are duplicated across self-contained specs. Mitigated by their small, stable surface; a bundler step restores a single source if drift appears.
- The default cannot serve an out-of-lockstep consumer. Accepted — that consumer is the documented trigger.

### Follow-ups

- The `mise run gen:*` task family wrapping `scripts/gen-*.sh`.
- `mise run gen:openapi-public`, emitting the merged bundles the Scalar portals consume: the dev-portal bundle keeps operations at audience `>= internal`, the public bundle keeps only `public`.
- `oasdiff` wired as a labelling step in CI, not a version gate.
- When the flag is added: `Api-Version` negotiation, the N-1 response-compat layer, `Deprecation`/`Sunset`/`Link` emission, and `docs/api-versions.md` recording which release dates are contract boundaries.

## Rules

- The contract source of truth is OpenAPI 3.1, one file per HTTP service at `services/<service>/openapi.yaml`, and every HTTP service generates its server with ogen. East-west control-plane services are included. Only a service with no HTTP surface ships no spec. `(CI: lint:openapi)`
- Each spec is self-contained: cross-service shapes are declared inline in `components` and kept identical across services. Cross-file `$ref` is not used. `(CI: lint:openapi)`
- An edge-exposed spec's `servers` url is `/api` and its paths are globally-unique resource nouns; the exposed URL carries no service segment and no version segment. An all-`cluster` service uses `servers: /`. `(CI: lint:api-resources)`
- Every spec declares `info.x-audience` on the ladder `cluster` | `internal` | `public`, default `cluster`; an operation may override it. It is documentation scoping, not access control. `(CI: lint:api-audience)`
- A service is edge-exposed iff it has an `internal` or `public` operation; a `cluster`-only service has no `/api` route. `(CI: lint:api-audience)`
- All clients and server stubs are generated from the spec and committed. `(CI: ci-drift)`
- Hand-written code imports generated types. Parallel hand-written request and response types are not declared. `(CI: lint:api-wildcard)`
- The service validates request schemas through the generated ogen server. The edge performs no schema validation and no business-rule validation. `(review-only)`
- Server-Sent Events is the default streaming mechanism. A WebSocket endpoint carries a justification in the service README. `(review-only)`
- gRPC, Connect-RPC, GraphQL, and tRPC are not used. `(review-only)`
- OpenAPI YAML is hand-written. TypeSpec and equivalent authoring layers are not used. `(review-only)`
- A spec change is a PR; merge is blocked on vacuum passing and on `mise run gen` producing no diff. `(CI: ci-contract, ci-drift)`
- The API serves a single live version — the current production release. There is no version in the path or a header, and no support window. `(review-only)`
- A breaking contract change ships in one PR with its co-shipped caller; the previous shape is not kept alive. `(review-only)`
- `oasdiff` labels breaking changes for review. It is not a version gate. `(CI: ci-contract)`
- Online versioning — date `Api-Version` header, N-1 window, `Deprecation`/`Sunset`/`Link`, and an N-1-bounded response-transformation layer — is deferred behind the out-of-lockstep-consumer trigger and is not operated in the default. `(review-only)`

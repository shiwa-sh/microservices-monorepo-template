# ADR-0008: API Contracts & Codegen

- **Status:** Accepted
- **Date:** 2026-05-19
- **Deciders:** Platform team
- **Related:** [ADR-0001](0001-language-and-runtime.md), [ADR-0009](0009-api-gateway.md)

## Context

Services expose APIs to three consumer groups:

1. **The Next.js frontend** ([ADR-0002](0002-monorepo.md)) — browser-side.
2. **Other internal services** — service-to-service via HTTP ([ADR-0006](0006-temporal.md)).
3. **Third-party / public API consumers** — external developers who require stable, documented contracts.

We use Tyk as the API gateway ([ADR-0009](0009-api-gateway.md)) and Temporal for workflow orchestration ([ADR-0006](0006-temporal.md)).

We need:

- A single **source of truth** for every API surface.
- **Generated code** in both Go (server stubs and internal clients) and TypeScript (frontend client). No hand-written request/response types on either side.
- **Schema validation at the gateway and in the service** — both from the same artifact, no duplicated definitions.
- A workflow strict enough that contract drift cannot survive code review or CI.
- Coverage for streaming use cases (SSE common; bidirectional rare but possible).

Wire efficiency for internal calls is explicitly **not** a priority. JSON over HTTP everywhere. Operational simplicity, browser-friendliness, gateway-friendliness, and human-debuggability win over binary-protocol throughput at our scale.

## Decision drivers

1. **One contract, two languages.** Go server + TS client from the same artifact.
2. **Gateway-first validation.** Tyk consumes OpenAPI natively; this is non-negotiable.
3. **Public API readiness.** Third-party consumers expect OpenAPI docs and SDKs.
4. **Browser fit.** No special proxies, no Envoy sidecars, no `grpc-web`.
5. **Spec-first, enforced.** The spec leads. CI fails if generated code is stale or hand-written types shadow generated ones.

## Considered options

- **OpenAPI 3.1 + `ogen` + `openapi-typescript` + `openapi-fetch`** — one spec drives Go server, Go client, TS client, Tyk config, public docs, downloadable SDKs.
- **gRPC + `grpc-web`** — Tyk's gRPC support is shallow; browsers need an Envoy/Connect proxy and lose streaming semantics; public consumers expect OpenAPI anyway.
- **Connect-RPC (Buf)** — speaks HTTP/JSON and gRPC simultaneously, but Tyk does not understand Connect natively. Revisit only if we ever replace Tyk with a Connect-aware proxy.
- **GraphQL** — wrong fit for service-to-service and public-API consumers; gateway features are harder; adds a query planner an 8-person team cannot afford.
- **tRPC** — TS-only; our backend is Go.

## Decision

**The API contract source of truth is OpenAPI 3.1.** One spec per service at `services/<service>/openapi.yaml`. Each spec is **fully self-contained**: cross-service shapes (the error envelope, common ID/time types, the workflow handle from [ADR-0006](0006-temporal.md)) are declared in the spec's own `components` rather than imported from a shared `api/shared/` namespace by cross-file `$ref`. Keeping each spec self-contained (no external file references) makes specs portable across the codegen and linting tools and removes any cross-file resolution step, so these shapes are duplicated by convention and kept identical across services.

### Codegen

| Output                      | Tool                                                   | Location                                               |
|-----------------------------|--------------------------------------------------------|--------------------------------------------------------|
| Go server + client + types  | `ogen` (type-safe, OpenTelemetry-instrumented)         | `libs/go/sdks/<service>/`                              |
| TS client                   | `openapi-typescript` + `openapi-fetch` (~6 KB runtime) | `libs/ts/sdks/<service>/`                              |
| Tyk API definitions         | `tools/codegen/tyk-gen`                                | `infra/gateway/apis/`                                  |
| Public SDKs                 | OpenAPI Generator                                      | published per-language as third-party consumers arrive |

All generated artifacts are committed to the repo and drift-checked in CI per [ADR-0002](0002-monorepo.md).

### Workflow

1. API change is a PR to `services/<service>/openapi.yaml`.
2. CI runs **vacuum** lint with the repo ruleset at `tools/codegen/openapi-ruleset.yaml` (style + breaking-change detection).
3. `mise run gen:all` regenerates Go server, Go client, TS client, Tyk definitions.
4. CI fails if generated files are out of date (`git diff --exit-code`).
5. Tyk picks up the same spec for gateway-level validation and docs.
6. Hand-written code imports generated types and never declares parallel ones.

### Validation

- **Tyk** validates request schemas at the edge using the OpenAPI spec ([ADR-0009](0009-api-gateway.md)).
- **The service** re-validates request schemas via `ogen`'s generated server, which decodes and validates every request into typed Go values. The validator is generated from the same spec; it costs nothing to maintain.
- **The service** owns all **business-rule** validation (ownership, limits, state transitions, idempotency). These cannot live at the gateway.

The reason the gateway and service both validate schemas: internal service-to-service calls bypass the gateway entirely ([ADR-0006](0006-temporal.md)), and a gateway misconfiguration that lets bad input through must not turn into a correctness incident in the service.

### Streaming

- **Server-Sent Events (SSE)** is the default for server→client push. Declared in OpenAPI as `text/event-stream` responses; Tyk passes through.
- **Server-streaming over HTTP/2** is used only when SSE is awkward (binary frames, very high throughput). Declared with a chunked-transfer response, documented per-endpoint.
- **WebSockets** for bidirectional streaming. Documented in `services/<service>/README.md` with a JSON-Schema for message envelopes. Treated as a deliberate exception: each WS endpoint needs a one-line justification in the README.
- Tyk handles WS upgrades. gRPC and Connect are not introduced for streaming.

### Authoring layer

OpenAPI YAML is hand-written. **TypeSpec is not used.** If a service's spec grows unwieldy, the response is to split the service or factor shapes into more `components` within the same file (possibly splitting the service), not to introduce a second authoring tool.

## Consequences

### Positive

- One artifact powers server, internal client, frontend client, gateway, docs, and public SDKs. Per-service contract cost is roughly fixed regardless of how many consumers exist.
- Browser, third-party, and service-to-service consumers all see the same API shape; no protocol-translation layer.
- Tyk's strongest feature (OpenAPI-native validation) is fully used.
- Schema-level validation is generated, not written — zero hand-maintained duplication between gateway and service.
- Public API readiness is a CI artifact, not a project.

### Negative / Risks

- OpenAPI is awkward for complex discriminated unions and conditional schemas. Mitigated by vacuum ruleset rules enforcing flat schemas; complex polymorphism is a hint that the API surface is too coupled.
- Streaming story is pragmatic, not unified. Mitigated by per-WS-endpoint justification and acceptance that gRPC/Connect are not adopted for this alone.
- Double schema validation (gateway + service) has a CPU cost. Acceptable at target throughput.
- Cross-service shapes (error envelope, workflow handle) are duplicated across specs because each spec is kept self-contained (no external `$ref`s). Mitigated by their small, stable surface; a future bundler step could restore a single source if drift becomes a problem.

### Follow-ups

- `tools/codegen/generate.sh` (and the `mise run gen:*` task family).
- `tools/codegen/openapi-ruleset.yaml` ruleset.
- `tools/codegen/tyk-gen` for Tyk API definition emission.
- Shared shapes (error envelope, workflow handle) declared inline in each `services/<service>/openapi.yaml` `components` block.
- Lefthook pre-commit hook running the affected generator slice.
- CI drift-check job per [ADR-0002](0002-monorepo.md).

## Rules

- The contract source of truth is OpenAPI 3.1, one file per service at `services/<service>/openapi.yaml`.
- Each spec is self-contained: cross-service shapes (error envelope, workflow handle) are declared inline in the spec's `components` and kept identical across services. Cross-file `$ref` is avoided to keep specs portable across the codegen and linting tools.
- All clients, server stubs, and Tyk API definitions are generated from the spec and committed. CI fails on drift.
- Hand-written code imports generated types. Parallel hand-written request/response types are forbidden.
- Both the gateway and the service validate request schemas, both from the same OpenAPI artifact.
- Business-rule validation lives only in the service, never in the gateway.
- Server-Sent Events is the default streaming mechanism. WebSockets require a per-endpoint justification in the service README.
- gRPC, Connect-RPC, GraphQL, and tRPC are not used.
- OpenAPI YAML is hand-written. TypeSpec and equivalent authoring layers are not used.
- A spec change is a PR; merging is blocked on vacuum lint passing and on `mise run gen:all` producing no diff.

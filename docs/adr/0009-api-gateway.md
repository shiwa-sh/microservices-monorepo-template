# ADR-0009: Edge Authentication & Traffic Policy

- **Status:** Accepted
- **Date:** 2026-05-19
- **Deciders:** Platform team
- **Related:** [ADR-0003](0003-cluster-topology.md), [ADR-0008](0008-api-contracts.md), [ADR-0010](0010-auth.md)

## Context

Traefik is the cluster ingress ([ADR-0003](0003-cluster-topology.md)): TLS termination, hostname/path routing, load
balancing, static assets. This ADR covers what happens to a request *after* Traefik and *before* it reaches a service:
who validates the caller's identity, how that identity is carried onward, and what traffic policy is applied at the edge.

Service-to-service calls inside the cluster bypass the edge entirely ([ADR-0006](0006-temporal.md)) and are gated by
Cilium NetworkPolicy ([ADR-0003](0003-cluster-topology.md)), not by a token.

The edge owns:

- **Identity validation** — validate the Kratos session / Hydra JWT once, at the edge.
- **Identity propagation** — forward identity to services as trusted headers, in **one shape for every request**.
- **Rate limiting** — protect auth-sensitive endpoints (login, signup, recovery) and, where present, public APIs.

It does **not** own: cluster ingress / TLS (Traefik does), permission decisions (services do, via [ADR-0010](0010-auth.md)'s
`Checker`), request-schema validation (services do, via `ogen` — see [ADR-0008](0008-api-contracts.md)).

## Decision drivers

1. **One identity shape for every request.** A service handler reads identity the same way whether the call came from
   the browser or another service. No "JWT here, headers there."
2. **Services are auth-free in their handlers.** Identity arrives pre-validated; services read it, they do not parse
   tokens.
3. **Open source, single-binary, Ory-consistent.** The edge validator should fit the Kratos/Hydra stack already deployed
   ([ADR-0010](0010-auth.md)), not add a parallel control plane.
4. **Operational simplicity.** No second datastore, no plugin runtime, no gateway-specific config codegen.

## Considered options

- **Ory Oathkeeper (ForwardAuth)** — Ory's identity & access proxy. Validates Kratos sessions and Hydra JWTs, mutates
  requests (strip + inject identity headers), declarative access rules in YAML. Single Go binary, no datastore.
  Consolidates into the Ory stack already run for [ADR-0010](0010-auth.md).
- **Tyk / Kong (full API-management gateway)** — rich API management (per-key quotas, dev portal, plugins), but each
  adds a control plane, a datastore (Redis/Postgres), a plugin runtime, and gateway-specific config codegen. Their one
  unique value at our scale — edge OpenAPI validation — is redundant with `ogen`'s in-service validation
  ([ADR-0008](0008-api-contracts.md)); the rest goes unused. Reserved as a per-project add-on for a project that ships a
  monetised public API.
- **Traefik Enterprise / Hub OIDC middleware** — closes the JWT-validation gap in Traefik OSS, but is a paid tier
  ([ADR-0000](0000-platform-foundations.md): OSS only).
- **DIY ForwardAuth service** — a small Go service reusing `libs/go/authmw/`. Viable, but Oathkeeper is hardened,
  declarative, and keeps auth-critical validation code out of our ownership.

## Decision

**The edge is Traefik ([ADR-0003](0003-cluster-topology.md)) fronting Ory Oathkeeper.** Oathkeeper does identity
validation and identity-header injection. **No full API-management gateway (Tyk, Kong) is deployed in the template
default.**

### Identity: validate once at the edge, headers everywhere after

```text
Internet
  → Traefik       (TLS, routing, load balancing, rate limiting)
  → Oathkeeper    (validate Kratos session / Hydra JWT → strip → inject X-User-Id / X-Org-Id / X-Roles)
  → service       (reads identity headers only; authmw is a header reader; Checker authorises the user)

service → service (forwards the same identity headers; Cilium NetworkPolicy gates reachability; no token on the path)
```

- Oathkeeper validates the caller's Kratos session cookie (browser) or Hydra-issued JWT (third-party / machine clients).
- It **strips any client-supplied identity headers** and sets authoritative `X-User-Id`, `X-Org-Id`, `X-Roles`.
- Services read identity **only** from these headers. They never parse a JWT. `authmw` ([ADR-0010](0010-auth.md)) is a
  trusted-header reader.
- Internal service-to-service calls **forward the same headers** and are gated by Cilium NetworkPolicy. There is no
  token on the internal path.

This is the single request shape: every service, edge-origin or internal, reads identity from the same headers.

### Rate limiting

Traefik's rate-limit middleware throttles auth-sensitive routes (login, signup, password reset) per source from day
one — a security control independent of any public API. Per-API-key *tiered quotas* are a full-API-management feature; a
project that ships a monetised public API adds a gateway (Tyk/Kong) for its own routes at that point, behind a
per-project flag. The template default does not.

### Security headers & Origin policy

The edge is the one place every route passes through, so blanket browser-security headers and the CSRF Origin check live
here, complementing the per-request CSP nonce the frontend sets ([ADR-0014](0014-frontend.md)).

- **Static security headers** are a Traefik `Middleware` applied to all responses: `frame-ancestors 'none'`,
  `X-Content-Type-Options: nosniff`, `Referrer-Policy: strict-origin-when-cross-origin`, and HSTS. The nonce-bearing
  `script-src` directive stays in the frontend ([ADR-0014](0014-frontend.md)); the edge sets only the directives that
  never vary per request.
- **CSRF Origin check.** For cookie-authenticated state-changing requests (the Kratos session cookie), an Oathkeeper
  rule rejects requests whose `Origin` is not the project's own domain. Bearer-token traffic (Hydra/Oathkeeper) is not
  browser-attached and is exempt. This backstops the `SameSite=Lax` cookie ([ADR-0010](0010-auth.md)) and the Next.js
  Server-Actions `Origin` check ([ADR-0014](0014-frontend.md)).

### Configuration

- Oathkeeper access rules live in `infra/auth/oathkeeper/` as declarative YAML, deployed via Helm at
  `infra/helm/platform/ory/` alongside Kratos and Hydra.
- Traefik routing and rate-limit middleware live in `infra/gateway/` as Traefik CRDs (`Middleware`, `IngressRoute`),
  committed per [ADR-0002](0002-monorepo.md).
- There is **no gateway-specific API-definition codegen.** The OpenAPI spec drives service codegen
  ([ADR-0008](0008-api-contracts.md)); the edge does not consume it.

### Developer portal

The developer portal is a route group in the frontend application: `apps/frontend/src/app/(devportal)/`. It renders
OpenAPI specs (Scalar or Redoc) and, when a public API exists, calls Hydra to issue and rotate third-party OAuth2 client
credentials. Until the first third-party consumer, it serves a "documentation coming soon" placeholder — no separate
component to operate.

### Hydra is a public-API flag

Hydra ([ADR-0010](0010-auth.md)) issues OAuth2 tokens for third-party / external machine clients. **Internal-only
projects do not deploy it:** Kratos (login) + Oathkeeper (edge) + SpiceDB (authz) is the internal stack. A project
exposing a public API or external machine clients sets `hydra_thirdparty: on`; Oathkeeper then validates those JWTs at
the edge and converts them to the same identity headers, so the internal request shape is unchanged.

### Authz boundary

Settled here and inherited by [ADR-0010](0010-auth.md):

- The edge validates identity and injects identity headers. It does **not** call the authz engine.
- Services receive identity headers and use the shared Go authz client (`libs/go/authz/`) to make permission decisions.
- Service-to-service calls bypass the edge; they authorise the forwarded user identity the same way, and Cilium
  NetworkPolicy decides which services may reach which.

## Consequences

### Positive

- One identity shape across the whole platform; service handlers are auth-free.
- The edge validator is one Go binary in the Ory family — no Redis, no plugin runtime, no gateway codegen.
- Removing Tyk removes Redis, the gRPC plugin server, `infra/gateway/apis/` codegen, and `tools/codegen/tyk-gen`.
- JWT validation lives at exactly one hardened chokepoint, not duplicated into every service.
- The dev portal as a frontend route group keeps visual consistency and removes a separate stateful component.

### Negative / Risks

- **Header-trust internally rests on Cilium NetworkPolicy.** A misconfigured policy could let a pod spoof `X-User-Id`.
  Mitigated by default-deny policies in the service template and by Hubble flow visibility
  ([ADR-0003](0003-cluster-topology.md)) as the audit surface.
- **No edge schema validation.** Mitigated by `ogen`'s generated in-service validation from the same spec
  ([ADR-0008](0008-api-contracts.md)); internal calls bypass the edge anyway, so the service is the only point that sees
  every request.
- **Per-API-key quotas are not available day one.** Accepted; reintroduced per-project when a monetised public API is
  real.
- **Oathkeeper is another component to operate.** Accepted; it is lighter than the Tyk + Redis + plugin stack it
  replaces and shares the Ory operational model.

### Follow-ups

- `infra/helm/platform/ory/` extended with Oathkeeper alongside Kratos and Hydra.
- `infra/auth/oathkeeper/` access rules (validate session/JWT → strip → inject identity headers).
- `infra/gateway/` Traefik `Middleware` + `IngressRoute` for `/api` routing, rate limiting, and static security headers.
- `infra/auth/oathkeeper/` CSRF Origin-check rule for cookie-authenticated state-changing requests.
- `apps/frontend/src/app/(devportal)/` placeholder route group.
- `docs/gateway/runbook.md` covering edge rules, rate-limit changes, and the public-API gateway add-on.

## Rules

- The edge is Traefik fronting Ory Oathkeeper. No full API-management gateway is deployed in the template default.
- Oathkeeper validates the Kratos session or Hydra JWT, strips client-supplied identity headers, and injects
  `X-User-Id`, `X-Org-Id`, `X-Roles`. It does not call the authz engine.
- Every request carries identity in the same header shape. Services read identity from headers and never parse a token.
- Service-to-service calls bypass the edge, forward the identity headers, and are gated by Cilium NetworkPolicy
  ([ADR-0003](0003-cluster-topology.md)). No token is on the internal path.
- Request-schema validation is service-side via `ogen` ([ADR-0008](0008-api-contracts.md)). There is no edge schema
  validation.
- Rate limiting on auth-sensitive routes is configured as Traefik middleware in `infra/gateway/`.
- Static browser-security headers (`frame-ancestors`, `nosniff`, `Referrer-Policy`, HSTS) are a Traefik middleware applied to all responses; the per-request CSP nonce is set by the frontend ([ADR-0014](0014-frontend.md)).
- Cookie-authenticated state-changing requests are Origin-checked by an Oathkeeper rule. Bearer-token traffic is exempt.
- Hydra is deployed only for projects exposing a public API or external machine clients (`hydra_thirdparty` flag).
  Internal-only projects run Kratos + Oathkeeper + SpiceDB.
- The developer portal is a route group in `apps/frontend/`, not a separate application or a gateway feature.
- A project that needs tiered per-API-key quotas adds a full gateway (Tyk/Kong) for its own routes via its own decision;
  it is not the platform default.

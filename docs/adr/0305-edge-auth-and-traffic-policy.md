# ADR-0305: Edge Authentication & Traffic Policy

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0101](0101-monorepo.md), [ADR-0200](0200-cluster-topology.md), [ADR-0303](0303-api-contracts-and-lifecycle.md), [ADR-0304](0304-identity-and-authorization.md), [ADR-0306](0306-trust-tiers-and-urls.md), [ADR-0400](0400-frontend.md)

## Context

Traefik is the cluster ingress ([ADR-0200](0200-cluster-topology.md)), handling TLS, routing, load balancing, and static assets. This ADR covers what happens **after Traefik and before a service**: who validates the caller's identity, how that identity is carried onward, and what traffic policy applies at the edge.

Service-to-service calls bypass the edge entirely and are gated by Cilium NetworkPolicy, not by a token.

| The edge owns | The edge does not own |
| --- | --- |
| Identity validation — the Kratos session or Hydra JWT, once | Cluster ingress and TLS — Traefik's |
| Identity propagation as trusted headers, in one shape for every request | Permission decisions — services', through `Checker` ([ADR-0304](0304-identity-and-authorization.md)) |
| Rate limiting on auth-sensitive endpoints | Request-schema validation — services', through ogen ([ADR-0303](0303-api-contracts-and-lifecycle.md)) |
| Static browser-security headers and the CSRF Origin check | The per-request CSP nonce — the frontend's ([ADR-0400](0400-frontend.md)) |

## Decision drivers

1. **One identity shape for every request.** A handler reads identity the same way whether the call came from a browser or another service. No "JWT here, headers there".
2. **Services are auth-free in their handlers.** Identity arrives pre-validated.
3. **Consistent with the identity stack already deployed** ([ADR-0304](0304-identity-and-authorization.md)), rather than a parallel control plane.
4. **Operational simplicity.** No second datastore, no plugin runtime, no gateway-specific config codegen.

## Considered options

| Option | Extra components | Config source | Verdict |
| --- | --- | --- | --- |
| **Ory Oathkeeper as ForwardAuth** | **none** — a single Go binary with no datastore | declarative YAML access rules in git | **Chosen.** Validates Kratos sessions and Hydra JWTs, and mutates requests to strip and inject identity headers. Consolidates into the Ory stack already run |
| Tyk or Kong | a control plane, a datastore, a plugin runtime | gateway-specific codegen | Rich API management whose one unique value here — edge OpenAPI validation — is redundant with in-service validation. The rest goes unused. Reserved as a per-project add-on for a monetised public API |
| Traefik Enterprise OIDC middleware | none | Traefik CRDs | Closes the JWT gap in Traefik OSS, and is a paid tier ([ADR-0000](0000-platform-foundations.md), principle 3) |
| A DIY ForwardAuth service | one first-party service | our code | Viable, and it puts auth-critical validation code under our ownership. Oathkeeper is hardened and declarative |

## Decision

The edge is **Traefik fronting Ory Oathkeeper**. No full API-management gateway is deployed by default.

### Validate once, headers everywhere after

```text
Internet
  → Traefik       TLS, routing, load balancing, rate limiting
  → Oathkeeper    validate Kratos session or Hydra JWT → strip → inject X-User-Id / X-Org-Id / X-Roles
  → service       reads identity headers only; Checker authorises

service → service forwards the same headers; NetworkPolicy gates reachability; no token on the path
```

Oathkeeper **strips any client-supplied identity headers** before setting authoritative ones. Services read identity only from those headers and never parse a JWT — `authmw` is a trusted-header reader ([ADR-0304](0304-identity-and-authorization.md)).

This is the single request shape: every service, edge-origin or internal, reads identity identically.

### Rate limiting

Traefik's middleware throttles auth-sensitive routes — login, signup, password reset — per source from day one, as a security control independent of any public API.

Per-API-key tiered quotas are a full-API-management feature. A project shipping a monetised public API adds a gateway for its own routes at that point, behind a per-project flag.

### Security headers and Origin policy

The edge is the one place every route passes through, so blanket headers live here.

| Control | Where |
| --- | --- |
| `frame-ancestors 'none'`, `X-Content-Type-Options: nosniff`, `Referrer-Policy`, HSTS | a Traefik `Middleware` on all responses — these directives never vary per request |
| The nonce-bearing `script-src` directive | the frontend ([ADR-0400](0400-frontend.md)) |
| **CSRF Origin check** | an Oathkeeper rule rejecting cookie-authenticated state-changing requests whose `Origin` is not the project's own domain. Bearer-token traffic is not browser-attached and is exempt |

The Origin check backstops the `SameSite=Lax` cookie ([ADR-0304](0304-identity-and-authorization.md)) and the Next.js Server-Actions check ([ADR-0400](0400-frontend.md)).

### Configuration and routing

| Concern | Location |
| --- | --- |
| Oathkeeper access rules | `infra/auth/oathkeeper/` as declarative YAML, deployed alongside Kratos and Hydra |
| Traefik routing, rate limits, middleware | `infra/gateway/` as Traefik CRDs, committed ([ADR-0101](0101-monorepo.md)) |

**Flat resource routing.** The API is a flat namespace ([ADR-0306](0306-trust-tiers-and-urls.md)), so the edge routes each resource prefix to its backing service through a per-resource `PathPrefix` rule. Service topology stays hidden and a resource can move between services without a URL change.

**The route table is the single registry of resource ownership**, and a CI lint fails if two edge-exposed specs claim the same prefix ([ADR-0303](0303-api-contracts-and-lifecycle.md)). Oathkeeper matches one rule on the whole `/api/` prefix and does not enumerate services.

There is no gateway-specific API-definition codegen. The spec drives service codegen; the edge does not consume it.

### Hydra is a public-API flag

Hydra issues OAuth2 tokens for third-party and external machine clients. **Internal-only projects do not deploy it** — Kratos, Oathkeeper, and OpenFGA are the internal stack. A project exposing a public API sets `hydra_thirdparty: on`, and Oathkeeper then validates those JWTs and converts them to the same identity headers, so **the internal request shape is unchanged**.

### Authz boundary

- The edge validates identity and injects headers. It does **not** call the authz engine.
- Services receive the headers and decide permissions through the shared client.
- Service-to-service calls bypass the edge, authorise the forwarded identity the same way, and NetworkPolicy decides which services may reach which.

The one sanctioned exception is operator tooling, which is third-party and cannot call `Checker`. That gate is [ADR-0304](0304-identity-and-authorization.md)'s.

### Developer portals

Both portals render the specs as **filtered projections on the `x-audience` ladder**, never separately maintained documents, and both render the edge `/api` surface. East-west operations appear in neither and are documented in service READMEs.

Scalar's request console is why the dev portal is same-origin with `/api` ([ADR-0306](0306-trust-tiers-and-urls.md)): "try it" hits the real edge with the caller's session and needs no CORS. The renderer, the route group, and the rejected alternatives are [ADR-0400](0400-frontend.md)'s; the projections are [ADR-0303](0303-api-contracts-and-lifecycle.md)'s.

## Consequences

### Positive

- One identity shape across the platform, and service handlers are auth-free.
- The edge validator is one Go binary in a family already operated: no Redis, no plugin runtime, no gateway codegen.
- JWT validation lives at exactly one hardened chokepoint rather than being duplicated into every service.

### Negative / Risks

- **Header trust internally rests on NetworkPolicy.** A misconfigured policy would let a pod spoof `X-User-Id`. Mitigated by default-deny in the service template and by Hubble flows as the audit surface ([ADR-0200](0200-cluster-topology.md)).
- **No edge schema validation.** Mitigated by generated in-service validation from the same spec. Internal calls bypass the edge anyway, so the service is the only point that sees every request.
- **Per-API-key quotas are unavailable on day one.** Accepted, and reintroduced per project when a monetised public API is real.
- **Oathkeeper is one more component to operate.** Accepted; it shares the Ory operational model.

### Follow-ups

- `infra/helm/platform/ory/` extended with Oathkeeper.
- `infra/auth/oathkeeper/` access rules and the CSRF Origin-check rule.
- `infra/gateway/` middleware and routes for `/api`, rate limiting, and static security headers.
- `docs/gateway/runbook.md` covering edge rules, rate-limit changes, and the public-API gateway add-on.

## Rules

- The edge is Traefik fronting Ory Oathkeeper. No full API-management gateway is deployed by default. `(review-only)`
- Oathkeeper validates the Kratos session or Hydra JWT, strips client-supplied identity headers, and injects the authoritative ones. It does not call the authz engine. `(CI: lint:strip-headers)`
- Every request carries identity in the same header shape. Services read identity from headers and never parse a token. `(CI: lint:auth-inline)`
- Service-to-service calls bypass the edge, forward the identity headers, and are gated by NetworkPolicy. No token is on the internal path. `(review-only)`
- Request-schema validation is service-side. There is no edge schema validation. `(review-only)`
- Rate limiting on auth-sensitive routes is Traefik middleware in `infra/gateway/`. `(review-only)`
- Static browser-security headers are a Traefik middleware on all responses; the per-request CSP nonce is the frontend's. `(review-only)`
- Cookie-authenticated state-changing requests are Origin-checked by an Oathkeeper rule. Bearer-token traffic is exempt. `(review-only)`
- The edge routes each resource prefix to its backing service, and the route table is the single registry of resource ownership. `(CI: lint:api-resources)`
- Hydra is deployed only for projects exposing a public API or external machine clients. `(review-only)`
- A project needing tiered per-API-key quotas adds a full gateway for its own routes by its own decision. It is not the platform default. `(review-only)`

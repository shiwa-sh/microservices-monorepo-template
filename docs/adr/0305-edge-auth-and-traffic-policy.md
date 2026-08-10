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
3. **The edge reads the credential the identity provider issues**, with no translation layer standing between them. [ADR-0304](0304-identity-and-authorization.md) settles what that credential is.
4. **The edge adds no state.** Whatever sits here is on the path of every request, so a datastore behind it is a datastore in front of everything.
5. **Authorization lives in one place.** A policy language at the edge is a second place a permission decision can be written.

## Considered options

| Option | Added components | Reads a Kratos browser session | Identity passed onward as | Verdict |
| --- | --- | --- | --- | --- |
| **Ory Oathkeeper as ForwardAuth** | **none beyond one Go binary**, no datastore | **yes**, and Hydra JWTs through the same rule set | mutated trusted headers, one shape for every route | **Chosen.** Declarative YAML access rules in git, and the only option that reads both credential formats this platform issues without a hop in between |
| Plain Traefik `ForwardAuth` to Kratos `/sessions/whoami` | **none at all** | yes | nothing — `whoami` authenticates but does not mutate the request | The zero-component baseline, and the row that prices the chosen one. Without mutation every service must strip client-supplied identity headers itself, and there is no rule language to mark a route public |
| oauth2-proxy | one | **no — it speaks OIDC**, and Kratos issues browser sessions rather than OIDC tokens | OIDC claims as headers | The reflexive answer to this problem, and it needs Hydra in front of Kratos before it has anything to validate. That adds an OAuth2 round trip to first-party browser traffic to satisfy a proxy rather than a requirement |
| Authelia | one, plus a session store | no — it carries its own identity model | headers | An identity provider with a forward-auth front-end. Adopting it re-decides [ADR-0304](0304-identity-and-authorization.md) rather than serving it |
| Pomerium | one, plus its own state | no — OIDC | headers, with per-route policy | The strongest alternative on expressiveness. Same OIDC mismatch, and its policy language is a second home for authorization, against driver 5 |
| Envoy `ext_authz` | Envoy at the edge, beside or instead of Traefik | through a service we write | whatever that service sets | A mechanism rather than a product: it re-poses the question as *what runs behind it*, and swapping the ingress is a bigger change than this decision |
| Tyk or Kong | a control plane, a datastore, a plugin runtime | through a plugin | plugin-defined | Rich API management whose one unique value here — edge OpenAPI validation — is redundant with in-service validation ([ADR-0303](0303-api-contracts-and-lifecycle.md)) |
| Traefik's commercial OIDC middleware | none | no — OIDC | headers | Closes the JWT gap in Traefik's open distribution, and is a paid tier ([ADR-0000](0000-platform-foundations.md), principle 3) |
| A DIY ForwardAuth service | one first-party service | yes, once written | ours | Viable, and it puts auth-critical validation under our ownership. Oathkeeper is the same shape, declarative, and maintained by someone else |

**The OIDC mismatch is what eliminates most of the field.** Kratos issues browser sessions, not OIDC tokens; Hydra issues OAuth2 tokens for third-party clients ([ADR-0304](0304-identity-and-authorization.md)). Every proxy above that expects an OIDC provider therefore needs Hydra inserted into the first-party browser path, which is an OAuth2 flow added to serve the proxy rather than any consumer.

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

**A denial at the edge looks like a denial from a service.** Oathkeeper rejects before any handler runs, so it is the platform's other error producer, and its `401` and `403` responses carry `application/problem+json` in the shape [ADR-0303](0303-api-contracts-and-lifecycle.md) fixes — same members, same `trace_id` extension. A generated client therefore has one error branch, not one per producer. Oathkeeper's error handlers are configured for this rather than left at their default body.

### Where this sits against zero trust

A reader who knows [NIST SP 800-207](https://csrc.nist.gov/pubs/sp/800/207/final) will stop at the diagram above, because a service that trusts a header is the thing zero trust exists to eliminate. The position is deliberate: **this platform takes 800-207's authorization model and declines its transport model.**

| Tenet | Here |
| --- | --- |
| Access is granted per-request and per-resource, evaluated dynamically | **held.** Every protected call goes through `Checker` against OpenFGA ([ADR-0304](0304-identity-and-authorization.md)). Nothing is authorised merely by having reached the service |
| All communication is secured regardless of network location | **held.** WireGuard encrypts all east-west pod traffic ([ADR-0200](0200-cluster-topology.md)) |
| No implicit trust is derived from network position | **declined.** A service trusts `X-User-Id` because Cilium default-deny guarantees only sanctioned callers reach its port and Oathkeeper strips client-supplied headers at the edge. That is positional trust, which is precisely the assumption 800-207 removes |

The concession buys handlers with no auth code, one identity shape for every caller, no per-hop token minting or verification, and no sidecar on the hot path. It costs this: code executing inside a sanctioned caller can forge identity to a downstream service. The blast radius is that caller's own egress allowances, bounded further by the `restricted` profile limiting what a compromised pod can do at all ([ADR-0200](0200-cluster-topology.md)).

**The seam is built and its trigger is already recorded.** [ADR-0200](0200-cluster-topology.md) makes per-workload certificate identity a Cilium mutual-auth and SPIFFE upgrade, on the trigger of compliance requiring an auditable CA chain. Taking it converts positional trust into cryptographic trust and changes the transport only — the authorization half is 800-207's already, so nothing above it moves.

### Rate limiting

Traefik's middleware throttles auth-sensitive routes — login, signup, password reset — per source from day one, as a security control independent of any public API.

**Per-API-key tiered quotas are deferred.** They are a full-API-management feature, and nothing else in the gateway is wanted.

| Field | Value |
| --- | --- |
| **Trigger** | a project bills for API access, so a caller's quota differs by contract rather than by route |
| **Seam** | ✓ the public API is already a distinct audience with its own routes ([ADR-0303](0303-api-contracts-and-lifecycle.md)), so a gateway is placed in front of those routes only, behind a per-project flag. First-party browser traffic keeps the path above |
| **Cost if adopted late** | the API has already been billed against counters kept somewhere else — usually in a service — and those become the migration, not the gateway |

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

- **Header trust internally rests on NetworkPolicy.** A misconfigured policy would let a pod spoof `X-User-Id`. Mitigated by default-deny in the service template and by Hubble flows as the audit surface ([ADR-0200](0200-cluster-topology.md)). This is the recorded deviation from NIST SP 800-207 above, not an unexamined gap, and it is the one property the SPIFFE seam exists to convert.
- **No edge schema validation.** Mitigated by generated in-service validation from the same spec. Internal calls bypass the edge anyway, so the service is the only point that sees every request.
- **Per-API-key quotas are unavailable on day one.** Accepted, and reintroduced per project when a monetised public API is real.
- **Oathkeeper is one more component to operate.** Accepted; it shares the Ory operational model.

## Rules

- The edge is Traefik fronting Ory Oathkeeper. No full API-management gateway is deployed by default.
- Oathkeeper validates the Kratos session or Hydra JWT, strips client-supplied identity headers, and injects the authoritative ones. It does not call the authz engine. `(CI: lint:authz)`
- Every request carries identity in the same header shape. Services read identity from headers and never parse a token. `(CI: lint:auth-inline)`
- Service-to-service calls bypass the edge, forward the identity headers, and are gated by NetworkPolicy. No token is on the internal path.
- Request-schema validation is service-side. There is no edge schema validation.
- Edge denials return `application/problem+json` in the shape [ADR-0303](0303-api-contracts-and-lifecycle.md) fixes, carrying `trace_id`. Oathkeeper's default error body is not shipped. `(ref: RFC 9457)`
- Rate limiting on auth-sensitive routes is Traefik middleware in `infra/gateway/`.
- Static browser-security headers are a Traefik middleware on all responses; the per-request CSP nonce is the frontend's.
- Cookie-authenticated state-changing requests are Origin-checked by an Oathkeeper rule. Bearer-token traffic is exempt.
- The edge routes each resource prefix to its backing service, and the route table is the single registry of resource ownership. `(CI: lint:service-contract)`
- Hydra is deployed only for projects exposing a public API or external machine clients.
- A project needing tiered per-API-key quotas adds a full gateway for its own routes by its own decision. It is not the platform default.

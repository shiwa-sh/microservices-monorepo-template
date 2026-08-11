# Gateway & edge runbook

How-to for operating the Traefik + Oathkeeper edge. The decision (Traefik ingress, Oathkeeper identity filter, no API-management gateway) is [ADR-0305](../adr/0305-edge-auth-and-traffic-policy.md); trust tiers and hostnames are [ADR-0306](../adr/0306-trust-tiers-and-urls.md).

## Model

- **Traefik** is the only ingress: TLS (cert-manager), host/path routing, load balancing, rate limiting.
- **Oathkeeper** sits behind it as the identity filter — it authenticates and injects `X-User-Id`, `X-Org-Id`, `X-Roles`, and strips any client-supplied identity headers first ([ADR-0305](../adr/0305-edge-auth-and-traffic-policy.md), [ADR-0304](../adr/0304-identity-and-authorization.md)).
- Edge config lives in `infra/gateway/` and `infra/auth/oathkeeper/`; it is not inlined into chart values (guarded by `mise run lint:auth-inline`).

## Add a route

**For a product API resource:**

1. Declare the resource in the service's `ingress.resources`. The edge routes `/api/<resource>` on the apex host and strips the `/api` prefix.
2. Check no other service already owns that resource name. `mise run lint:openapi` rejects the collision, and finding it yourself is faster ([ADR-0303](../adr/0303-api-contracts-and-lifecycle.md)).
3. Apply `strip-identity-headers` **before** forward-auth in the middleware chain. Get this backwards and a client can inject its own `X-User-Id`; `mise run lint:authz` fails the build if you do.

**For an ops tool:**

1. Add a `Host({tool}.ops.<host>)` IngressRoute behind the ops forward-auth. Its coarse gate is the `operator` claim plus AAL2 ([ADR-0306](../adr/0306-trust-tiers-and-urls.md)).
2. Add the Oathkeeper access rule in `infra/auth/oathkeeper/access-rules.json`. Use `remote_json` — `mise run lint:authz` rejects `allow`, because an `allow` rule on the ops tier is an unauthenticated origin on the cluster's control surface.

## Certificates & DNS

- Two wildcard certs per environment: `*.<host>` and `*.ops.<host>`, via cert-manager DNS-01 ([ADR-0202](../adr/0202-secrets.md), [ADR-0306](../adr/0306-trust-tiers-and-urls.md)).
- `*.<host>` and `*.ops.<host>` resolve to the edge; locally `*.localtest.me` → 127.0.0.1.

## Diagnose

- A 401 at the edge is Oathkeeper rejecting the session/JWT ([jwt-validation](../reference/jwt-validation.md)).
- A 403 on an ops origin is the coarse claim gate (not `operator` / not AAL2) or the optional per-tool OpenFGA check ([break-glass](break-glass.md) if the auth plane itself is down).
- Inspect live routing with the Traefik dashboard / `kubectl -n platform logs` for the edge and Oathkeeper.

# Gateway & edge runbook

How-to for operating the Traefik + Oathkeeper edge. The decision (Traefik ingress, Oathkeeper identity filter, no API-management gateway) is [ADR-0305](../adr/0305-edge-auth-and-traffic-policy.md); trust tiers and hostnames are [ADR-0306](../adr/0306-trust-tiers-and-urls.md).

## Model

- **Traefik** is the only ingress: TLS (cert-manager), host/path routing, load balancing, rate limiting.
- **Oathkeeper** sits behind it as the identity filter — it authenticates and injects `X-User-Id`, `X-Org-Id`, `X-Roles`, and strips any client-supplied identity headers first ([ADR-0305](../adr/0305-edge-auth-and-traffic-policy.md), [ADR-0304](../adr/0304-identity-and-authorization.md)).
- Edge config lives in `infra/gateway/` and `infra/auth/oathkeeper/`; it is not inlined into chart values (guarded by `mise run lint:auth-inline`).

## Add a route

- Product API: a flat `/api/<resource>` IngressRoute on the apex `<host>` behind the Oathkeeper forward-auth ([ADR-0306](../adr/0306-trust-tiers-and-urls.md)). The service declares the resources it owns in `ingress.resources`; the edge routes each `/api/<resource>` and strips `/api`. No two services may own the same resource (CI-linted, [ADR-0303](../adr/0303-api-contracts-and-lifecycle.md)). Every forward-auth route MUST apply `strip-identity-headers` before forward-auth — enforced by `mise run lint:authz`.
- Ops tool: a `Host({tool}.ops.<host>)` IngressRoute behind the ops forward-auth, whose coarse gate is the `operator` claim + AAL2 ([ADR-0306](../adr/0306-trust-tiers-and-urls.md)). Add the Oathkeeper access rule in `infra/auth/oathkeeper/access-rules.json` (`mise run lint:authz` checks it uses `remote_json`, never `allow`).

## Certificates & DNS

- Two wildcard certs per environment: `*.<host>` and `*.ops.<host>`, via cert-manager DNS-01 ([ADR-0202](../adr/0202-secrets.md), [ADR-0306](../adr/0306-trust-tiers-and-urls.md)).
- `*.<host>` and `*.ops.<host>` resolve to the edge; locally `*.localtest.me` → 127.0.0.1.

## Diagnose

- A 401 at the edge is Oathkeeper rejecting the session/JWT ([jwt-validation](../reference/jwt-validation.md)).
- A 403 on an ops origin is the coarse claim gate (not `operator` / not AAL2) or the optional per-tool OpenFGA check ([break-glass](break-glass.md) if the auth plane itself is down).
- Inspect live routing with the Traefik dashboard / `kubectl -n platform logs` for the edge and Oathkeeper.

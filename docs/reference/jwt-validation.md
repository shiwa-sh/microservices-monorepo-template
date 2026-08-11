# JWT validation

The single definition of how the edge validates a Hydra-issued JWT, referenced by [ADR-0304](../adr/0304-identity-and-authorization.md) and [ADR-0305](../adr/0305-edge-auth-and-traffic-policy.md). Browser callers use the Kratos session cookie instead; this document covers the third-party / machine-client JWT path (present only when Hydra is enabled, `hydra_thirdparty`).

## Validation contract

Ory Oathkeeper validates every JWT at the edge before any identity header is injected. A token is accepted only if all of the following hold:

| Check | Requirement |
| --- | --- |
| Algorithm | RS256 (asymmetric); `none` and HMAC are rejected |
| Signature | verifies against the issuer's JWKS |
| `iss` (issuer) | matches the configured Hydra issuer URL for the environment |
| `aud` (audience) | contains the expected API audience |
| `exp` (expiry) | in the future |
| `nbf` (not-before) | in the past |
| Clock skew | 30 s tolerance on `exp` / `nbf` |

Keys are fetched from the issuer's JWKS endpoint and cached; there is no per-service JWKS fetch — only the edge validates ([ADR-0304](../adr/0304-identity-and-authorization.md)).

## After validation

On success, Oathkeeper strips any client-supplied identity headers and injects the authoritative `X-User-Id`, `X-Org-Id`, `X-Roles` from the token claims (`sub`, `org_id`, `roles`). Past the edge there are no tokens — services read identity only from these headers via `libs/go/authmw/` ([ADR-0304](../adr/0304-identity-and-authorization.md)).

On failure the request is rejected at the edge with `401`; it never reaches a backend.

# ADR-0017: URL & Domain Structure (Trust Tiers)

- **Status:** Proposed
- **Date:** 2026-06-25
- **Deciders:** Platform team
- **Related:** [ADR-0003](0003-cluster-topology.md), [ADR-0009](0009-api-gateway.md), [ADR-0010](0010-auth.md), [ADR-0011](0011-observability.md), [ADR-0012](0012-internal-admin.md), [ADR-0014](0014-frontend.md), [ADR-0015](0015-naming-and-identifiers.md)

## Context

Every environment exposes two very different kinds of HTTP surface behind the same Traefik edge
([ADR-0003](0003-cluster-topology.md), [ADR-0009](0009-api-gateway.md)):

1. **Product** — the user-facing Next.js app (landing, auth UI, the `panel`/`admin`/`devportal` route groups,
   [ADR-0014](0014-frontend.md)), the service APIs (`/api/<svc>`, [ADR-0008](0008-api-contracts.md)), and browser
   telemetry ingest.
2. **Operations tooling** — third-party operator dashboards we deploy but do not author: Hubble
   ([ADR-0003](0003-cluster-topology.md)), Grafana ([ADR-0011](0011-observability.md)), the Lowdefy internal-admin
   console ([ADR-0012](0012-internal-admin.md)), Argo CD ([ADR-0004](0004-gitops.md)), the Temporal Web UI
   ([ADR-0006](0006-temporal.md)), and the MinIO console (non-prod).

Until now these were addressed ad hoc: most product and ops surfaces shared **one origin** (`<env-host>`) and were
separated only by URL path (`/grafana`, `/internal/admin`, `/api/*`). That has two problems the team hit in practice:

- **No browser-level isolation between tiers.** Path segments on one origin share cookies, `localStorage`, and the DOM.
  A flaw in code we do not control (a Hubble/Grafana XSS, a dangling-subdomain takeover) executes in the **same origin**
  as the product app and its session. The browser's same-origin policy provides *zero* separation between a path and its
  siblings.
- **Some tools cannot be served under a path at all.** Hubble UI's router is hardwired to basename `/` and 404s under
  any path prefix; it must be served at the root of its own origin. Grafana needs `serve_from_sub_path`, Argo CD and the
  Temporal UI have their own base-path quirks. Path hosting is a per-tool fight.

The web consensus is that **separate origins (subdomains) are the right boundary for internal/higher-risk tooling**:
browsers treat each subdomain as a distinct origin, so cookies, storage, CSP, and rate-limit/WAF scope are isolated by
construction. Subpaths are the default only when everything on the origin is one trusted application. Trust-tier
segmentation by subdomain (`admin.api.example.com` vs `public.api.example.com`) is an established pattern.

We need one deliberate URL/origin scheme that (a) isolates the product tier from the ops tier at the browser level,
(b) still lets a single operator login flow cover all the ops tools, and (c) gives every tool a predictable hostname
that fits a wildcard certificate.

## Decision drivers

1. **Browser-enforced origin isolation between tiers.** A flaw in one origin must not be able to read or script another
   (DOM, storage) — the boundary is the same-origin policy, not path discipline. Credential-level isolation is layered on
   top via per-tool authorization (and, optionally, OIDC), not assumed from the cookie alone.
2. **One operator session across ops tools.** Operators use several dashboards in a sitting; they should log in once for
   the whole ops tier (the tools are the same trust level), with that login carrying a second factor (AAL2).
3. **Least authority, not least cookie.** A logged-in session must not by itself grant any ops tool; access is an
   explicit per-tool grant ([ADR-0010](0010-auth.md)).
4. **Predictable, wildcard-friendly names** ([ADR-0015](0015-naming-and-identifiers.md)): lowercase, hyphen-within-
   segment, every tier under a single parent label so one wildcard cert covers it.
5. **Env parity** ([ADR-0016](0016-environment-parity.md)). The same scheme, host-parameterised, in local and every
   deployed environment.

## Decisions

### Two trust tiers under one registrable host

Let `<host>` be the environment host (e.g. `dev.localtest.me` locally, `dev.example.com` in a deployed env). Surfaces
split into exactly two tiers:

| Tier        | Origin                 | What lives there                                                                 |
|-------------|------------------------|----------------------------------------------------------------------------------|
| **Product** | `<host>` (apex)        | Next.js app — landing, `/auth/*`, `panel`/`admin`/`devportal`; `/api/<svc>/*`; `/api/observability/faro` |
| **Ops**     | `*.ops.<host>`         | one origin per operator tool (table below)                                       |

The Next.js app is the **whole product origin**: it serves the public landing page and the authenticated route groups,
and the service APIs stay **same-origin** under `<host>/api/<svc>/*` (the browser app is their only client, so
same-origin avoids CORS and keeps the session cookie naturally scoped). The ops tier nests every tool one level under a
shared `ops.` label.

### Ops-tier hostnames

| Tool                       | Hostname                  | Notes                                              |
|----------------------------|---------------------------|----------------------------------------------------|
| Hubble UI                  | `hubble.ops.<host>`       | served at root (router can't run under a path)     |
| Grafana                    | `grafana.ops.<host>`      | drop `serve_from_sub_path`; served at root         |
| Argo CD                    | `argo.ops.<host>`         | replaces port-forward access                       |
| Temporal Web UI            | `temporal.ops.<host>`     | replaces port-forward access                       |
| Lowdefy internal admin     | `console.ops.<host>`      | named `console` to disambiguate from the product `/admin` route group |
| MinIO console              | `minio.ops.<host>`        | **non-prod only** ([ADR-0016](0016-environment-parity.md))         |

Names follow [ADR-0015](0015-naming-and-identifiers.md)'s charset (`^[a-z][a-z0-9-]*$`, hyphen within a segment, never
underscore). The grammar is `{tool}.{tier}.{env-host}`; the product tier carries **no** tier label (it is the apex).

### Why the `ops.` label is load-bearing, not cosmetic

Cookies are sent to a domain and its **descendants** only, never to siblings or a higher ancestor's other children.
That single rule forces the nesting:

- If ops tools were flat (`hubble.<host>`, `grafana.<host>`), the only domain that covers all of them is the common
  parent `<host>` — which **is the product origin**. A cookie shared across flat ops tools would therefore also reach the
  product, re-merging the tiers.
- Nesting under `ops.<host>` lets the ops session cookie be scoped `Domain=ops.<host>`: it covers every `*.ops.<host>`
  tool but **not** the apex `<host>`. Shared across ops, invisible to product.

So the `ops.` segment is the mechanism that makes "one operator login, isolated from product" expressible on a single
registrable host.

### Cookie & session mechanics

**Default model — one parent-scoped session cookie.** A single Kratos `ory_kratos_session` cookie scoped to the parent
`<host>` (`Domain=<host>`) is shared across the product apex and every `*.ops.<host>` origin. This is accepted because
every origin under `<host>` is **first-party and edge-gated**: tier isolation is enforced by **per-tool authorization**
(next section) and by requiring **AAL2 — operator MFA ([ADR-0010](0010-auth.md))** on the ops tier, *not* by cookie
scope. A compromised non-operator product session that reaches an ops origin is still denied by the dashboard
authorizer. The cookie keeps `Secure` + `HttpOnly` + `SameSite=Lax` ([ADR-0010](0010-auth.md)).

The one property deliberately traded away is *token*-level isolation between tiers: the session token is sent across the
whole `<host>` subtree, so an XSS on the high-surface product origin could ride an **operator's** session into the ops
tools. This is acceptable while every subtree origin is first-party; product-origin CSP ([ADR-0014](0014-frontend.md)) is
the compensating control.

**Hardening upgrade (optional).** To also isolate the token, split the cookie: keep the product cookie host-only on the
apex, and have the ops tier mint an `ops.<host>`-scoped session via **OIDC against the central IdP** — Hydra
(`hydra_thirdparty`, [ADR-0009](0009-api-gateway.md)) behind one ops-tier auth proxy (per-tier, not per-tool). One Kratos
instance cannot issue two differently-scoped cookies, and the apex cannot set a cookie on the sibling `ops.` subtree, so
OIDC is the mechanism if token isolation is wanted. This upgrade becomes **mandatory** if any non-first-party origin is
ever hosted under `<host>`.

### Authorization: who may reach which surface

Authentication only proves *who* the operator is; it does not entitle them to every tool. Access is authorized per
surface, at one of two enforcement points split by **who owns the code**:

- **Product surfaces are our code → the app/service decides.** The `/admin`, `/panel`, `/devportal` route groups and the
  `/api/<svc>` endpoints authorize with SpiceDB through `libs/go/authz`'s `Checker` ([ADR-0010](0010-auth.md)); the edge
  only authenticates. Page-level access to `/admin` is a `Checker.Allowed` call in the RSC layer, not a bare session
  check.
- **Ops dashboards are third-party → the edge decides.** Hubble, Grafana, Argo CD, Temporal, the MinIO console, and the
  Lowdefy console cannot run a permission check themselves, so authorization moves to the ops-tier Oathkeeper. Its
  authorizer is **not** `allow`: each ops route uses the `remote_json` authorizer to call the same SpiceDB `Checker`,
  modelling each tool as a resource:

  ```zed
  definition dashboard {
    relation viewer: user | group#member
    permission view = viewer
  }
  ```

  A request to `grafana.ops.<host>` checks `view` on `dashboard:grafana`; `hubble.ops.<host>` checks `dashboard:hubble`.
  Granting `dashboard:console#viewer@user:alice` **without** a `dashboard:hubble` tuple gives Alice the admin console but
  not Hubble — per-tool, per-user, revocable, inheritable through `group`/`org` relations.

Coarse-then-fine: an `operator` group gates the **whole** ops tier (a non-operator gets nothing), and per-`dashboard`
grants refine within it. The ops tier additionally requires an **AAL2 session** (operator MFA,
[ADR-0010](0010-auth.md)) — the operator's second factor is enforced at the ops-tier forward-auth, independent of the
product tier (where B2C MFA stays optional). The current `"authorizer": { "handler": "allow" }` on every dashboard rule
is the gap this closes. The edge decision still flows through the SpiceDB `Checker`, so [ADR-0010](0010-auth.md)'s "every
permission decision goes through `Checker`" holds.

### Certificates & DNS

- Two wildcard certs per environment: `*.<host>` (covers the apex's siblings and `ops.<host>` itself) and
  `*.ops.<host>` (covers the ops tools, which are two labels deep). Both via cert-manager DNS-01 in deployed envs
  ([ADR-0005](0005-secrets.md)); both as SANs on the locally-generated wildcard for local.
- DNS: `*.<host>` and `*.ops.<host>` resolve to the edge. Locally this is free (`*.localtest.me` → 127.0.0.1).

### Routing

- Product: Traefik `Host(\<host>\)` routes (the per-service `/api/<svc>` IngressRoutes and the frontend catch-all
  already match on `Host`, [ADR-0009](0009-api-gateway.md)).
- Ops: one `Host(\{tool}.ops.<host>\)` IngressRoute per tool, each behind the ops forward-auth middleware. Host-
  parameterised so local and deployed envs share the manifests ([ADR-0016](0016-environment-parity.md)).

## Consequences

### Positive

- Each tier is a **separate origin**: an ops dashboard cannot read or script the product app's DOM/storage, and a
  product-side XSS cannot read or script an ops tool — browser-enforced same-origin policy, not convention. (Credential-
  level isolation is handled by per-tool authz + operator AAL2 in the default model, or fully by the OIDC upgrade.)
- Each ops tool is also isolated from the *other* ops tools (separate origins): per-origin CSP, security headers, rate
  limits, and storage.
- Tools that resist path hosting (Hubble, Grafana, Argo, Temporal) are each served at a clean root — no `base-path`
  fights.
- A logged-in session no longer implies tool access: every ops surface is per-tool authorized and AAL2-gated.
- Argo CD, Temporal, and the MinIO console get first-class auth-gated URLs instead of port-forwarding.

### Negative / Risks

- **A second wildcard cert and deeper DNS** (`*.ops.<host>`, four labels deep in prod). cert-manager handles it, but it
  is more moving parts.
- **No token-level tier isolation in the default model.** The parent-scoped cookie sends the session token across the
  whole `<host>` subtree, so a product-origin XSS could ride an *operator's* session into ops. Accepted while every
  origin is first-party; per-tool authz + operator AAL2 + product CSP are the compensating controls, and the OIDC
  upgrade closes it if needed.
- **Subdomain-takeover hygiene** matters more: dangling `*.ops.<host>` DNS must not be left claimable.
- **Migration churn:** existing `/grafana`, `/internal/admin`, and the recently-added `hubble.<host>` URLs all move;
  docs, bookmarks, and the redirect handler change.

### Follow-ups

- Implement the product/ops split in `infra/gateway` (host-parameterised ops IngressRoutes), the per-tool chart values
  (Grafana/Argo/Temporal base-path off; Hubble already root), and the cert-manager Certificate (two wildcards).
- Switch every ops dashboard authorizer from `allow` to `remote_json` → SpiceDB `Checker`; add the `operator` group and
  `dashboard` resource to `infra/auth/spicedb/schema.zed`; enforce **AAL2** on the ops-tier forward-auth.
- Keep the default parent-scoped cookie. *Optional hardening:* stand up the ops-tier OIDC proxy (Hydra) for token
  isolation — required only if a non-first-party origin is ever hosted under `<host>`.
- Move `hubble.<host>` → `hubble.ops.<host>` (supersedes the [ADR-0003](0003-cluster-topology.md) hubble-subdomain note
  and the work recorded in this session).
- Update `docs/dev-loop.md`, ADR-0003/0009/0011/0012 endpoint references, and the `scripts/cluster-full.sh` banner.

## Rules

- Surfaces belong to exactly one tier: **product** on the apex `<host>`, **ops tooling** on `*.ops.<host>`. No operator
  dashboard is served from a product path, and no product surface is served from an `ops.` subdomain.
- Service APIs stay same-origin under `<host>/api/<svc>/*`; they are not given their own origin unless a non-browser
  client requires it.
- Ops-tier hostnames are `{tool}.ops.<host>`, lowercase, matching `^[a-z][a-z0-9-]*$`
  ([ADR-0015](0015-naming-and-identifiers.md)).
- The default is one session cookie scoped to the parent `<host>`, shared across tiers; tier isolation is enforced by
  per-tool authorization and an **AAL2 (operator MFA)** requirement on the ops tier, not by cookie scope. This is
  permitted **only** while every origin under `<host>` is first-party and edge-gated.
- Splitting the cookie (product host-only on the apex + an `ops.<host>` cookie minted via OIDC) is the optional token-
  isolation upgrade, and is **mandatory** if any non-first-party origin is hosted under `<host>`.
- Each environment provisions both `*.<host>` and `*.ops.<host>` certificates.
- Third-party operator dashboards are authorized **per-tool at the edge** by Oathkeeper's `remote_json` authorizer
  against the SpiceDB `Checker` (`dashboard:<tool>#view`), never `allow`. Product surfaces authorize in-app/in-service
  through `libs/go/authz` ([ADR-0010](0010-auth.md)); a bare authenticated session never grants tool access.

# ADR-0304: Identity & Authorization

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0100](0100-language-and-runtime.md), [ADR-0104](0104-supply-chain-security.md), [ADR-0200](0200-cluster-topology.md), [ADR-0202](0202-secrets.md), [ADR-0205](0205-environment-parity.md), [ADR-0301](0301-data-lifecycle-privacy.md), [ADR-0302](0302-temporal.md), [ADR-0303](0303-api-contracts-and-lifecycle.md), [ADR-0305](0305-edge-auth-and-traffic-policy.md), [ADR-0306](0306-trust-tiers-and-urls.md), [ADR-0400](0400-frontend.md), [ADR-0401](0401-internal-admin.md), [ADR-0500](0500-observability.md), [ADR-0501](0501-operator-uis-and-dashboards.md)
- **Decides:** Ory Kratos authenticates, OpenFGA authorizes every protected call through `Checker`, and organisations are relationships rather than roles.

## Context

The platform serves four kinds of principal:

| Principal | Needs |
| --- | --- |
| B2C users | public sign-up, email and password, social login, MFA, recovery |
| B2B organisations | multi-tenant orgs, SSO, per-org roles, SCIM provisioning |
| Services calling services | identity forwarded as headers, trusted by network policy ([ADR-0200](0200-cluster-topology.md)). No per-call token |
| Third-party API consumers | tokens to call public APIs |

Constraints inherited from earlier ADRs: self-host only, per-service footprint matters ([ADR-0100](0100-language-and-runtime.md)), and the edge validates tokens while services decide permissions ([ADR-0305](0305-edge-auth-and-traffic-policy.md)).

**Hard requirement on the UI.** The login UI is a custom Next.js implementation of a Figma design. It is not the provider's hosted pages, themed templates, or a forked vendor UI. The provider must expose a headless, API-first identity flow that Next.js drives end to end.

**Authorization shape.** Day-one features already exceed flat RBAC: resource ownership across orgs, shared resources, role per org per resource. Temporal is available ([ADR-0302](0302-temporal.md)), which makes the dual write between application database and authz store a routine workflow.

The decision splits into three: the identity provider, the OAuth2 authorization server, and the authorization engine.

## Decision drivers

1. **Configuration lives in the repository** ([ADR-0000](0000-platform-foundations.md), principle 1). Identity schemas, OAuth2 clients, and authz models are files, not database state.
2. **Headless identity provider**, required by the UI constraint.
3. **Polyglot-tolerant integration contracts.** Auth is a *protocol* contract — token format, header names, validation rules — expressible in any language.
4. **No new runtime and no new datastore** on the always-on floor for this concern ([ADR-0100](0100-language-and-runtime.md), [ADR-0300](0300-data.md)).

## Considered options

### Identity provider

| Option | Headless flow | Config source of truth | Verdict |
| --- | --- | --- | --- |
| **Ory Kratos** | fully headless self-service flows | **JSON Schema identity files and YAML in git** | **Chosen.** Go, Apache-2.0, the reference implementation of the headless pattern |
| Zitadel | a genuine headless Session API covering password, MFA, passkeys, and external IdPs | **its own event-sourced database**, despite its Terraform provider | Rejected on driver 1 — the same driver that rejected Coroot ([ADR-0501](0501-operator-uis-and-dashboards.md)). Secondary: its own guide makes hosted login the default and states it was "designed with security in mind, which limits the customization capabilities", and its headline customization story is forking a beta Next.js app, which the UI constraint excludes by name |
| Keycloak | themes, or the Admin and Account REST APIs — not a first-class headless self-service flow | realm import/export, with drift back to the database | The most mature OSS provider, at a JVM footprint incompatible with [ADR-0100](0100-language-and-runtime.md), and extension work in Java |
| Authentik | flow-based | database | Python, Django, Celery, and Redis is a third runtime and three components. B2C self-service flows are less mature |
| SuperTokens | fully headless — the recipe model is API-first, with the UI entirely the caller's | **its own core service and database**, with configuration partly in code and partly in that store | The closest competitor to Kratos on driver 2, and it splits configuration between the SDK's initialisation code and the core's state, so no single committed file describes the identity setup |
| Logto | headless Management and Experience APIs | its own database, with a Terraform provider over the management API | Fails driver 1 the same way Zitadel does: the API is the source of truth and git holds a script that talks to it |
| Casdoor | headless API, though the UI is the primary surface | database, with configuration objects | Broad protocol coverage across a wide feature surface, and the same database-as-truth problem |

### OAuth2 and OIDC server

Needed only when a third-party client obtains its own credentials; first-party browsers use Kratos sessions.

| Option | Added runtime and datastore | Consent flow integrates with the chosen IdP | Config source of truth | Verdict |
| --- | --- | --- | --- | --- |
| **Ory Hydra** | none — Go on the existing Postgres | **yes, first-party with Kratos** | YAML plus committed client definitions | **Chosen.** The only option that adds an OAuth2 server without also adding a second identity system |
| Keycloak | a JVM, and its own store | it *is* the IdP, so the question dissolves — and so does Kratos | realm import, drifting to the database | Adopting it here re-decides the row above, and [ADR-0100](0100-language-and-runtime.md) bars the runtime |
| Zitadel | its own event-sourced store | as above, it replaces Kratos | its own database | Same: it is an IdP that also does OAuth2, not an OAuth2 server for someone else's IdP |
| Authentik | Python, Django, Celery, Redis | as above | database | Same shape, heavier |
| Dex | none — Go, and it can run storage-less | **no** — Dex federates *upstream* identity providers; it issues tokens for users it does not own | committed YAML, the best of the field on driver 1 | The thinnest by far, and it is a federating proxy rather than an authorization server with its own consent and client management |
| Authelia | Go, plus a session store | no — its own identity model | committed YAML | An access-control portal that gained an OIDC provider, and its user model is not Kratos's |
| Build our own | none | n/a | ours | Excluded by [ADR-0000](0000-platform-foundations.md): security-critical infrastructure is not hand-rolled |
| Ship no OAuth2 server — the honest baseline | none | n/a | n/a | Correct until a third party needs credentials, which is why Hydra is flag-gated Opt-in rather than Core ([`operational-surface.md`](../operational-surface.md)) |

### Authorization engine

Day-one requirements are ReBAC, not flat RBAC: cross-org sharing, role per org per resource, and reverse-index questions such as "which resources can this user see". That narrows the field to Zanzibar-style engines.

| Engine | Model and capability | Governance | Fit | Verdict |
| --- | --- | --- | --- | --- |
| **OpenFGA** | Zanzibar ReBAC; ABAC through CEL conditions and contextual tuples; reverse index via `ListObjects` and `ListUsers` | **CNCF** — vendor-neutral, multi-vendor contributors | single Go binary on Postgres; `.fga` DSL, `fga model test` in CI, official Go SDK behind a seam | **Chosen.** Matches SpiceDB on capability and operational shape, and wins on governance. Contextual tuples also let a check read data at request time, easing the sync the dual write carries |
| SpiceDB | Zanzibar ReBAC; ABAC through CEL caveats; strongest reverse-index and `Watch` APIs; `ZedToken` consistency | single-vendor | single Go binary on Postgres | The credible fallback, equal on capability. Loses only on governance, and its consistency edge bites only at large scale |
| Ory Keto | Zanzibar ReBAC; ABAC only through OPL; no conditions, `Watch`, or batch | single-vendor, some features source-available | fits the existing Ory stack | The thinnest feature set of the mature engines, with licence-gated extras and no offsetting advantage |
| Permify | ReBAC plus first-class ABAC; native multi-tenancy | single-vendor, under corporate ownership; smaller community | Go on Postgres | Capable, and narrower adoption against a vendor-neutral option of equal capability makes it the weaker long-term bet |
| Topaz | ReBAC directory **plus full OPA/Rego** — the most expressive; weak reverse-index and large-scale query APIs | community-maintained, without a sponsoring vendor; pre-1.0 | edge or sidecar with an embedded directory, so authz data must be replicated to every instance | The richest policy language, and the worst data-sync story of the field |
| Casbin | PERM-model **library** with role inheritance and implicit-permission queries; **no relationship-graph traversal and no reverse index over resources** | Apache, vendor-neutral | in-process; polyglot ports must agree on matcher semantics | Not a Zanzibar engine. It answers "what may this user do" well and "which resources can this user see" only by enumeration, which is the day-one requirement |
| In-process RBAC per service | hand-rolled | ours | diverges per service | The complex cases exist from day one, and migrating to a Zanzibar engine later is a per-feature data migration |

Any two Zanzibar engines are interchangeable at the architecture level: the `Checker` seam in `libs/go/authz/` is engine-agnostic, so reopening this decision touches one library, the model file, and the chart — not the services.

### Why ReBAC from day one, rather than RBAC and migrate later

The template spawns many products and only some need ReBAC, so shipping flat RBAC and migrating later is the obvious alternative. It is rejected, because the switching cost is not one number but three, and they differ sharply.

| Cost | Difficulty | Why |
| --- | --- | --- |
| **Check sites** | ≈ free | `Checker.Allowed(ctx, action, resource)` is already the sole authz seam, and inline role checks are already forbidden. Bought regardless of engine |
| **Data model** | hard, and it grows with accumulated data | RBAC stores subject→role; ReBAC stores subject→relation→object. Switching means backfilling a tuple per existing relationship — trivial while the store holds only "alice is org-admin", brutal once per-resource ACL tables exist |
| **Dual-write discipline** | hard and invasive | RBAC in one database is one transaction; ReBAC keeps two stores in sync. Retrofitting that into every authz-relevant mutation path late is the genuinely difficult part |

The deciding distinction is the **shape of the question**. RBAC answers a *subject-shaped* one — what role has this user. ReBAC also answers an *object-shaped* one — may this user act on *this specific* resource, which is sharing, per-resource roles, and cross-org ownership. So:

- A role-shaped product **never needs to switch**; RBAC would serve it forever.
- A per-resource product **cannot be answered by RBAC at all**; it needed ReBAC from the start, not later.
- "Start RBAC, migrate later" therefore serves only a population that either never migrates or should never have started on RBAC — and the rare true middle case hits the **maximum** accumulated cost of both hard columns. It optimises for the worst path.

**The honest tax:** even at L1 below, this imposes the dual-write habit plus one extra Postgres-backed binary that RBAC-in-the-database would not need. Accepted for one tool, one mental model, and no worst-path migration.

## Decision

### Identity provider: Ory Kratos

Kratos handles registration, login, MFA, recovery, settings, and social login. Identity schemas are JSON Schema files at `infra/auth/kratos/identity-schemas/`, and all other configuration is YAML at `infra/auth/kratos/`.

The Next.js login UI lives at `apps/frontend/src/app/(landing)/auth/` and drives Kratos self-service flows directly.

The session cookie is `SameSite=Lax`, `Secure`, and `HttpOnly`, in the sense [RFC 6265bis](https://datatracker.ietf.org/doc/html/draft-ietf-httpbis-rfc6265bis) gives those attributes. That is the first line of CSRF defence; Kratos's built-in anti-CSRF token covers the self-service flows, and the edge Origin check ([ADR-0305](0305-edge-auth-and-traffic-policy.md)) backstops other cookie-authenticated mutations. Browser-side CSP and Server-Actions handling are [ADR-0400](0400-frontend.md)'s.

### Password policy

Two local controls are on everywhere, with no dependency outside the pod: **`min_password_length: 12`** and **`identifier_similarity_check_enabled: true`**, which rejects a password resembling the email or username.

The shape follows [NIST SP 800-63B](https://pages.nist.gov/800-63-3/sp800-63b.html): length carries the strength, composition rules and forced rotation are absent because they push users toward predictable patterns, and the breach check below is 800-63B's compromised-credential screening. That document is also where the **AAL** levels used throughout this ADR are defined.

**The HaveIBeenPwned breach check is off by default.** It is a genuine control — it catches the one class a length rule cannot, a long passphrase already in a breach corpus — and its cost is not the network policy:

- **It needs world egress on every password operation.** Each registration, password login, and password change does a k-anonymity range lookup over HTTPS. Some deployments run on a closed network with no route to the internet, so the control cannot function there whatever the manifest says.
- **It fails open, silently.** `ignore_network_errors` defaults to true, so an unreachable endpoint neither fails the flow nor logs an error. Measured on a default-deny cluster: about 160 dropped SYNs over 20 seconds of retries per registration, a 20-second latency tax, and the breached password accepted anyway.

A control that reads as enabled in the manifest and is inert on the wire is worse than one that is honestly off, because it invites the reviewer to stop looking.

Where it earns its keep is the B2C tier, whose users are AAL1 with no forced second factor; operators carry [TOTP](https://www.rfc-editor.org/rfc/rfc6238) or [WebAuthn](https://www.w3.org/TR/webauthn-3/) regardless.

An environment with the egress enables it by setting `haveibeenpwned_enabled` with `max_breaches: 0` and restoring the `toFQDNs` allow and its companion L7 DNS rule, which the network policy keeps as a comment. A closed network can instead point `haveibeenpwned_host` at a self-hosted k-anonymity API. Kratos has no setting for a custom CA chain, so that host must serve a certificate the container already trusts.

This is a per-environment security posture, not a parity violation: the policy is identical in every environment that can enforce it. The pattern [ADR-0205](0205-environment-parity.md) forbids is the opposite one — enabling it in production and quietly disabling it locally, so local enforces a weaker policy while looking the same.

### OAuth2 server: Ory Hydra, when a public API exists

Hydra is deployed when a project exposes a public API or external machine clients, behind the `hydra_thirdparty` flag. It issues authorization-code and client-credentials tokens for third parties and external machines. Those tokens are validated at the edge and converted to the standard identity headers, so **the internal request shape is identical whether or not Hydra is deployed**.

Hydra is not used for internal service-to-service calls, which carry no token. When deployed it is the only issuer of external tokens; no service mints its own. Configuration lives at `infra/auth/hydra/`, and third-party clients are declared in YAML applied by a post-sync hook.

**Token shape and validation are the specifications', not ours.** A third-party integrator brings a library, not a reading of our documentation, and every local variation here is a bug someone else has to discover.

| Concern | Decision |
| --- | --- |
| Access-token format | JWT per [RFC 9068](https://www.rfc-editor.org/rfc/rfc9068) — `typ: at+jwt` and the profile's required claims. Opaque tokens with introspection are not used, because the edge would then call Hydra on every request |
| Validation | [RFC 8725](https://www.rfc-editor.org/rfc/rfc8725) JWT best practice. The signing algorithm is pinned by the edge rule, so `alg: none` and algorithm-confusion attacks fail by construction, and `iss`, `aud`, and `exp` are all checked. A token minted for one audience is not accepted for another |
| Grants | authorization code with [PKCE](https://www.rfc-editor.org/rfc/rfc7636) for **every** client, public and confidential alike, plus client credentials for machine callers. Implicit and resource-owner-password grants are disabled. This is the OAuth 2.1 posture, and it is a configuration Hydra does not impose on its own |
| Discovery | the [OpenID Connect Core](https://openid.net/specs/openid-connect-core-1_0.html) provider metadata document. A third party configures its client from that endpoint rather than from prose we maintain |

### B2B organisations: an `orgs` service on top of Kratos

Kratos stores identities, not organisations. Multi-tenancy is a first-party `services/orgs/` owning organisations and memberships, the active organisation for a session, and org-level invitations and SSO connection records. The service exists whether or not a B2B feature does, because retrofitting an organisation boundary through every table and tuple is the migration this avoids.

**The organisation is the unit of authorization.** Every protected resource in the model belongs to an `org`, and a user only ever acts through a role in some org. A user with zero orgs is an account that can own nothing.

So membership is a day-one invariant: **the moment a Kratos identity is created it gets a personal organisation in which it is `admin`.** The post-registration webhook starts the `RegisterUser` workflow, whose two activities are the same dual write as any authz mutation — the `orgs` rows and the OpenFGA tuple. Because membership is many-to-many, a user can create further orgs and be invited into others; the personal org is a first tenant, not a ceiling.

| Model | Tenant created | Gain over eager | Cost |
| --- | --- | --- | --- |
| **Personal org, eager** | automatically, at registration | **Chosen.** Zero org-less branching, and a superset of the others — a sales-led flow layers on unchanged | one-person-org clutter |
| Lazy tenant | on the first action needing one | less clutter | every caller must handle the org-less user, so "identities with no org" becomes normal rather than an edge case |
| Owner-creates-first | manually, as a deliberate step | cleanest for pure B2B | a freshly signed-up user sits in limbo |
| Invite-only or SCIM | by an admin or an external IdP | fits regulated environments | no self-serve growth, and the provisioning path must exist up front |
| No org, pure B2C | never — the user is the boundary | simplest if teams never arrive | re-adding tenancy later is a data migration and reworks the model's ownership relations |
| A single shared default org | at bootstrap | trivially few orgs | **Rejected** — it puts unrelated users in one tenant and breaks isolation |

The personal org's default name is a **generic constant**, not the user's email. An org may later hold a whole team and its name is shown to every member invited into it, so an email there would leak PII into a shared display field ([ADR-0301](0301-data-lifecycle-privacy.md)) and would go stale when the mutable email changes. The stable identity is the org id.

Switching model means editing the registration webhook path and, for the B2C case, the model's ownership relations. The `Checker` seam and every service's authz call are unaffected.

### Directory provisioning is deferred

[SCIM](https://www.rfc-editor.org/rfc/rfc7644) lets an enterprise customer's IdP drive joiners and leavers directly. It is per-customer plumbing, so it waits.

| Field | Value |
| --- | --- |
| **Trigger** | the first contract making directory-driven provisioning a condition of purchase |
| **Seam** | ✓ mostly. `orgs` already owns memberships, invitations, and SSO connection records, and `RegisterUser` already performs the membership dual write. SCIM is an authenticated endpoint driving that path. The gap is deprovisioning: a Kratos identity state change must also revoke sessions and tuples, which no flow does today |
| **Cost if adopted late** | joiner and leaver stay manual, and a leaver retaining access is the audit finding that surfaces it. Adoption then reconciles existing memberships against the directory once per customer |

### Authorization engine: OpenFGA

Accessed only through the `authz.Checker` interface in `libs/go/authz/`. A depguard rule confines the SDK to that library.

One engine serves every project at whatever complexity it needs. **What grows is the schema, never the tool.**

| Level | Model | Typical instance |
| --- | --- | --- |
| **L1 — role per org** | members and admins of an organisation. This *is* flat RBAC | roughly 15 lines of DSL, and most instances stay here |
| **L2 — resource ownership** | `owner`, `editor`, `viewer` on individual resources | adds relations to the same file |
| **L3 — sharing and hierarchy** | groups, folders, inheritance, cross-org sharing | adds relations to the same file |

The `Checker` interface is identical at every level, so **L1 is the first-class default, not a fallback**:

```fga
type user

type organization
  relations
    define admin: [user]
    define member: [user]
    define view: admin or member
    define manage: admin
```

That is flat RBAC, running on the engine that also does L2 and L3, so nobody ever migrates tools. Role-shaped usage keeps the tuple set small — membership changes only, already an `orgs` operation — so the dual-write surface scales down with it.

| Concern | Decision |
| --- | --- |
| Model authoring | the OpenFGA **DSL** at `infra/auth/openfga/model.fga` is the single source of truth, transformed to JSON for the API by `mise run gen:authz-model` |
| Scope | **one global model**, not per service, because authz relationships routinely cross service boundaries |
| Tests | check assertions at `infra/auth/openfga/fga.yaml`; CI runs `fga model test` and asserts the JSON is in sync with the DSL |
| Deployment | Helm, sharing the platform Postgres, as a plain Deployment and Service with an `openfga migrate` initContainer — the same pattern our own services use. OpenFGA ships no first-party operator |
| Bootstrap | OpenFGA needs a **store** and an immutable, versioned **authorization model** created in-cluster. A seed Job ensures a store named `platform` exists, writes the model, and applies the static ops grants. Services resolve the store by name at first use, so no store-ID plumbing is needed; the ids are opaque deploy-time values, not secrets |

### Accepted limitation: no consistency token

SpiceDB's `ZedToken` gives per-request read-after-write consistency. OpenFGA has no equivalent ([openfga/roadmap#67]), offering only a coarse `HIGHER_CONSISTENCY` flag.

This is not a day-one concern: it bites only at large, heavily replicated scale, and nothing here depends on it — the `Checker` runs at default consistency and the dual write threads no token.

| Field | Value |
| --- | --- |
| **Trigger** | a read-after-write staleness bug is observed in production — a user completing a mutation and then being denied the resource it granted |
| **Seam** | ✓ every read goes through `Checker`, so raising consistency is a per-call-site argument inside one library, not a change in any service |
| **Cost if adopted late** | the failure has already reached users as an intermittent permission error, which is the hardest class of authz bug to attribute |

[openfga/roadmap#67]: https://github.com/openfga/roadmap/issues/67

### Dual-write discipline

Every authz-relevant mutation — create, share, transfer, delete on a protected resource — is wrapped in a Temporal workflow ([ADR-0302](0302-temporal.md)) containing both the database write and the OpenFGA write as activities, so the pair cannot half-apply.

Direct database writes to authz-relevant tables outside a workflow are a review-blocker, enforced by a depguard-style rule against the relevant store methods.

### Identity is validated at the edge and carried as headers

Tokens are validated once, at the edge. **Past the edge there are no tokens** — identity travels as a fixed set of trusted headers, the same shape for every request.

| Stage | Behaviour |
| --- | --- |
| Edge validation | Oathkeeper validates the Kratos session cookie or the Hydra-issued JWT. JWTs are RS256 with keys at a JWKS endpoint, and validation requires issuer, audience, expiry, signature, and `nbf` with 30s skew tolerance, defined once in `docs/reference/jwt-validation.md` |
| Header injection | Oathkeeper **strips any client-supplied identity headers** and injects authoritative `X-User-Id`, `X-Org-Id`, and `X-Roles` |
| Service read | `libs/go/authmw/` reads the headers into a typed principal. Services never fetch JWKS or parse a JWT |
| Service to service | no token. The same headers are forwarded, gated by Cilium NetworkPolicy — network identity, not a per-call machine token |

A conformance suite at `tools/auth-conformance/` ships with the repo: identity-header fixtures with expected principals and authorization outcomes. Any non-Go service passes it.

### Authorizing operator tooling at the edge

"The edge validates, services decide" assumes a first-party service exists to decide. Operator dashboards are third-party and cannot call `Checker`, so their authorization happens at the edge in two layers ([ADR-0306](0306-trust-tiers-and-urls.md)).

| Layer | Mechanism |
| --- | --- |
| **Coarse gate, mandatory** | The ops-tier Oathkeeper requires `X-Roles` to contain `operator` plus an **AAL2** session. It reads only the session and its claims and makes **no OpenFGA call** — deliberately, so the debugging surface does not share fate with the product authorization plane. An OpenFGA outage must not lock every operator out of the dashboards needed to diagnose it ([`docs/guide/break-glass.md`](../guide/break-glass.md)) |
| **Fine gate, optional** | The route adds the `remote_json` authorizer calling `Checker`, modelling each tool as a `dashboard` resource. Deferred until an operator should reach some tools and not others — until then the coarse gate expresses the same policy. The seam is the authorizer field on one route; the cost of waiting is that every operator has held access to every tool up to that point |

This is the only sanctioned edge-side permission decision. Product surfaces decide in-service through `Checker`.

### Security verification: ASVS Level 2

**The application security bar is [OWASP ASVS](https://owasp.org/www-project-application-security-verification-standard/) Level 2**, pinned to version 5.0. L2 is the level ASVS describes as appropriate for an application handling significant transactions and personal data, which is this platform's default posture.

The neighbouring levels do not fit. L1 is a floor reachable without design support and asserts little. L3 targets systems where a breach is a safety event, and its demands — per-transaction reauthentication, full segregation of duties, exhaustive access audit — would reshape decisions taken deliberately elsewhere in this set.

The version is pinned so an upstream revision is a reviewed bump rather than a silently moved goalpost, the same discipline as the Pod Security Standards version in [ADR-0200](0200-cluster-topology.md).

**The claim is met across the set, not here.** ASVS is far broader than identity, so this ADR states the bar and names who holds each part of it:

| Concern | Owner |
| --- | --- |
| Authentication, session management | this ADR — Kratos sessions, the NIST 800-63B password policy, AAL levels |
| Access control | this ADR — OpenFGA through `Checker`, never inline |
| Input validation and encoding | [ADR-0303](0303-api-contracts-and-lifecycle.md) — validators generated from the spec, so coverage is not a matter of diligence |
| Error handling and logging | [ADR-0303](0303-api-contracts-and-lifecycle.md)'s error envelope, [ADR-0500](0500-observability.md)'s structured logs and PII rule |
| Data protection and privacy | [ADR-0301](0301-data-lifecycle-privacy.md) |
| Communications security | [ADR-0200](0200-cluster-topology.md) WireGuard east-west, [ADR-0205](0205-environment-parity.md) verified TLS everywhere |
| Configuration and secrets | [ADR-0202](0202-secrets.md) |
| Malicious-code and supply chain | [ADR-0104](0104-supply-chain-security.md) |

**The bar covers first-party surfaces.** Vendored operator tooling — Lowdefy, Grafana, pgweb ([ADR-0401](0401-internal-admin.md), [ADR-0501](0501-operator-uis-and-dashboards.md)) — is outside it, the same boundary [ADR-0400](0400-frontend.md) draws for accessibility, and for the same reason: a claim over software we do not write is a claim we cannot keep.

**The claim is verified on a cadence, by a named holder.** L2 is a design claim rather than a test result, and an unexamined claim ages into a false one — so the examination is scheduled rather than intended:

| Field | Value |
| --- | --- |
| Owner | the platform team, as a whole. Each row of the table above is verified by whoever owns the ADR it names |
| Cadence | once per ASVS release, and on any change to a row's owning ADR |
| Artefact | a per-row verdict — met, met with a stated exception, or unmet — in [`docs/reference/asvs-verification.md`](../reference/asvs-verification.md), which is live state and carries the date each row was last examined |
| Failure | a row unexamined for a full cadence is stated as unverified rather than as met. The bar remains the target and stops being a claim |

### Day-one component cost

| Component | Role |
| --- | --- |
| Kratos | human login |
| Oathkeeper | edge validation and identity-header injection ([ADR-0305](0305-edge-auth-and-traffic-policy.md)) |
| OpenFGA | authorization |
| `services/orgs/` | organisations and memberships |
| `libs/go/authmw/` and `libs/go/authz/` | header reader and `Checker` |

Hydra is deployed only when a project exposes a public API. There is no service-to-service token. All components share the platform Postgres and observability stack.

## Consequences

### Positive

- Login UI freedom: the Figma design is implemented exactly, with no fight against vendor templates.
- One way to get a token for human or machine identity.
- Authz is a single coherent model from day one — no deferred migration, no per-service RBAC tables.
- Schema in files, tests in files, a CLI for debugging, GitOps for everything.
- Polyglot-safe: auth integration is a protocol contract, so escape-hatch services participate on equal terms.
- Dual-write risk is structurally contained by routing authz-relevant mutations through Temporal.

### Negative / Risks

- **Three platform components on day one** for internal-only projects, plus the `orgs` service. Accepted: the constraints — headless login and ReBAC, both from day one — make consolidation impossible without compromise. Dropping the service-to-service token in favour of NetworkPolicy removes a validation path from every service.
- **Building B2B organisations on Kratos is real work.** Accepted; the alternative costs the UI requirement.
- **Dual-write discipline must be enforced.** A direct write that skips the workflow is a silent authz bug. Mitigated by lint, a review checklist, and integration tests asserting OpenFGA state after every workflow.
- **ASVS L2 is a design claim, not a test result.** No job proves it; it is asserted by construction and checked by review. Its worth is that a reviewer has a named checklist instead of a private sense of what secure means, and its risk is that an unexamined claim ages into a false one. A conformance review belongs to whoever needs the assurance, and this document does not schedule one.
- **L2 is not the bar every deployment needs.** A regulated project raises it and records the delta in its own ADR rather than editing this one.
- **The coarse ops gate is a claim, not a policy check.** An operator whose access should have been revoked keeps it until their session expires or the trait is removed. Accepted deliberately, so the dashboards survive an authz outage.

## Rules

- The application security bar is OWASP ASVS 5.0 Level 2 for first-party surfaces. Vendored operator tooling is out of scope, and the exclusion is stated rather than assumed. `(ref: OWASP ASVS 5.0 L2)`
- Human identity is owned by Ory Kratos. There is no alternate user store.
- External tokens are issued by Ory Hydra, deployed only for projects exposing a public API. No service issues its own tokens. Access tokens are `at+jwt`, and the authorization-code grant requires PKCE for every client; implicit and resource-owner-password grants are disabled. `(ref: RFC 9068, RFC 7636, OIDC Core)`
- The login UI is the Next.js app driving Kratos self-service flows. Hosted pages are not used.
- The password policy is `min_password_length: 12` plus `identifier_similarity_check_enabled: true` in every environment. The breach check is off by default and enabled per environment where the egress exists. Enabling it where the egress does not exist is not done, because it fails open silently. `(ref: NIST SP 800-63B)`
- The session cookie is `SameSite=Lax`, `Secure`, `HttpOnly`. `(ref: RFC 6265bis)`
- B2B organisations are owned by `services/orgs/`; other services consult its HTTP API. `(CI: ci:lint)`
- Every protected resource belongs to an `org`, and a user acts only through a role in an org. Every identity gets a personal org at registration through the `RegisterUser` dual write. A single shared default org is not used.
- Changing the tenancy model is confined to the registration webhook path and is a deliberate per-project decision.
- Directory provisioning ships only after the documented contract trigger fires. Until then, membership changes go through invitations.
- Authorization is OpenFGA, accessed only through the `Checker` interface. Direct SDK use elsewhere is not permitted. `(CI: ci:lint)`
- The OpenFGA schema is one global file. Per-service schemas are not used. `(CI: lint:authz)`
- Authz-relevant mutations run inside a Temporal workflow with the database write and the OpenFGA write as separate activities. `(CI: lint:authz)`
- Inline role checks in handlers are not used. Every permission decision goes through `Checker`. `(CI: lint:authz)`
- Operator dashboards are gated at the edge by the coarse claim plus AAL2, with no OpenFGA call. Optional per-tool refinement adds the `remote_json` authorizer.
- A simple instance uses an L1 schema, which is the first-class default. L2 and L3 grow the same schema on the same engine.
- Tokens are validated once at the edge with the algorithm pinned and `iss`, `aud`, and `exp` checked; services do not validate tokens. `(CI: lint:auth-inline; ref: RFC 8725)`
- Identity is carried as `X-User-Id`, `X-Org-Id`, and `X-Roles`, injected at the edge and forwarded unchanged internally. Services read identity only from these headers. `(CI: lint:authz)`
- Service-to-service calls carry no token. Shared secrets, HMAC schemes, and per-call machine tokens are not used.
- A non-Go service reads the identity headers through the same contract and passes `tools/auth-conformance/` before merging.
- Auth configuration is canonical only at `infra/auth/*` and is delivered to charts by file injection, never hand-copied inline into a chart's values. `(CI: ci:gen)`

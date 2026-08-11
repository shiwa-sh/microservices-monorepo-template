# ADR-0306: Trust Tiers & URL Structure

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0003](0003-naming-and-identifiers.md), [ADR-0200](0200-cluster-topology.md), [ADR-0201](0201-gitops.md), [ADR-0202](0202-secrets.md), [ADR-0205](0205-environment-parity.md), [ADR-0302](0302-temporal.md), [ADR-0303](0303-api-contracts-and-lifecycle.md), [ADR-0304](0304-identity-and-authorization.md), [ADR-0305](0305-edge-auth-and-traffic-policy.md), [ADR-0400](0400-frontend.md), [ADR-0401](0401-internal-admin.md), [ADR-0500](0500-observability.md), [ADR-0501](0501-operator-uis-and-dashboards.md)
- **Decides:** Product is served from the apex and operator tooling from one origin per tool under `*.ops.<host>`.

## Context

Every environment exposes two kinds of HTTP surface behind one Traefik edge:

| Surface | Contents | Code ownership |
| --- | --- | --- |
| **Product** | the Next.js app, the service APIs, browser telemetry ingest | first-party |
| **Operations tooling** | Hubble UI, Grafana, the Lowdefy console, Argo CD, the Temporal UI, Headlamp, pgweb, the SeaweedFS admin UI | **third-party — deployed, not authored** |

This ADR fixes where each lives, how one operator login covers the ops tier, and what the API path looks like.

## Decision drivers

1. **Browser-enforced origin isolation between tiers.** A flaw in one origin must not read or script another. The boundary is the same-origin policy, not path discipline.
2. **Logins are paid during an incident.** An investigation crosses several dashboards, so the number of authentications it costs is a property of the layout, not a preference.
3. **Least authority, not least cookie.** A logged-in session grants no ops tool by itself.
4. **Predictable, wildcard-friendly names** ([ADR-0003](0003-naming-and-identifiers.md)).
5. **Parity** ([ADR-0205](0205-environment-parity.md)). The same scheme, host-parameterised, everywhere.

## Considered options

### Where ops tooling lives

| Option | Browser isolation | Per-tool hosting cost | Verdict |
| --- | --- | --- | --- |
| **A subdomain per tool under a shared `ops.` label** | **each origin is distinct**: cookies, storage, CSP, and rate-limit scope isolated by construction | none — every tool is served at a root | **Chosen** |
| Paths on the product origin | **none** — path segments share cookies, `localStorage`, and the DOM, so a flaw in code we do not control executes in the same origin as the product session | a per-tool fight: Hubble UI's router is hardwired to basename `/` and 404s under any prefix, Grafana needs sub-path serving, Argo CD and the Temporal UI have their own quirks | Both problems are disqualifying |
| A flat subdomain per tool, no `ops.` label | per-tool isolation, and see below | none | The only domain covering all of them is the product origin itself, so a shared ops cookie would reach product and re-merge the tiers |
| Paths on the product origin with `__Host-` cookies per tool | **none between tools** — the prefix binds a cookie to the host, not to a path, so every tool still shares one origin | the same per-tool fight | The cheap middle option, and it does not exist: `__Host-` hardens a cookie against subdomain injection and grants no path isolation, because the same-origin policy has no path dimension |
| A separate registrable domain for ops | strongest — no shared parent | none, plus a second certificate authority chain and DNS zone | Warranted only where the parent-scoped cookie is itself the threat. Recorded as the hardening path |

### How ops origins are named

| Option | Survives swapping the tool | Naming decision per tool | Verdict |
| --- | --- | --- | --- |
| **After the tool, lowercased** — `grafana`, `hubble`, `temporal` | no — the URL moves with the tool | none; the name is already chosen | **Chosen.** One sentence, no judgement calls, and the rename happens only on an event that is already a migration |
| After the concept — `o11y`, `map`, `workflows`, `s3` | **claimed, but no** | one debate per tool | The theory is that a concept name outlives the tool. It does not hold: swapping a network-flow UI for an APM suite changes the concept too, and a tool's concept drifts as it grows features. Concept naming produces renames on tool changes *and* naming debates in between. It also lets a hostname stake a territorial claim — one asserting ownership of "observability" frames a complementary second tool as a rival for the name rather than as a different question-answerer |

Two accepted costs: `pgweb.ops` is less self-evident than `db.ops` to a newcomer, and the URL moves when the tool does. The first is answered by the table below naming the concern next to every host; the second would have happened under concept naming anyway.

The objection that tool names leak the stack is real and weak here: the wildcard certificate keeps individual subdomains out of Certificate Transparency logs, every origin is gated on an operator session with a second factor, and the `ops.` label already announces that operator tooling exists.

### Where the service API lives

| Option | CORS | Isolation gained | Verdict |
| --- | --- | --- | --- |
| **A path on the product origin, `<host>/api/<resource>`** | **none needed** | n/a | **Chosen.** A JSON API has no DOM, JS, or browser storage to isolate, so a separate origin protects nothing it has |
| `api.<host>` | manufactured for our own frontend | **none against the cookie** — a `Domain=<host>` cookie reaches `api.<host>` too | The reasons it sounds right evaporate: WAF and rate limits attach per path router already, and the edge already routes `/api/*` to services and `/` to the frontend |
| A separate registrable domain | required | **hard credential isolation** — the app session cookie genuinely cannot reach the API | Warranted only for that guarantee, or for separate edge and CDN infrastructure at scale. Not the default |
| Per-service paths, `/api/<svc>/...` | none needed | n/a | Leaks topology into every URL, so a resource cannot move between services without breaking callers |

## Decision

### Two tiers under one registrable host

`<host>` is the environment host.

| Tier | Origin | Contents |
| --- | --- | --- |
| **Product** | `<host>` (apex) | the Next.js app — landing, auth, `panel`, `devportal`; `/api/<resource>/*`; the browser telemetry ingest path |
| **Ops** | `*.ops.<host>` | one origin per operator tool |

The service APIs stay **same-origin** under the flat path, because the browser app is their only first-party client, so same-origin avoids CORS and keeps the session cookie naturally scoped.

### Ops-tier hostnames

The grammar is `{tool}.{tier}.{env-host}`. The product tier carries no tier label.

| Tool | Hostname | Note |
| --- | --- | --- |
| Hubble UI | `hubble.ops.<host>` | served at root; its SPA cannot run under a path |
| Grafana | `grafana.ops.<host>` | sub-path serving off |
| Argo CD | `argocd.ops.<host>` | replaces port-forward access |
| Temporal UI | `temporal.ops.<host>` | replaces port-forward access |
| Lowdefy admin | `lowdefy.ops.<host>` | the sole admin surface ([ADR-0401](0401-internal-admin.md)) |
| Headlamp | `headlamp.ops.<host>` | read-only by default ([ADR-0501](0501-operator-uis-and-dashboards.md)) |
| pgweb | `pgweb.ops.<host>` | read-only break-glass |
| SeaweedFS admin | `seaweedfs.ops.<host>` | **non-prod only**, and the sole exposed surface of that component |

**A component exposing several UIs gets one origin, not several.** SeaweedFS ships a master UI, a filer UI, and an admin UI; only the admin UI is routed. The others are diagnostic surfaces reached the way any unrouted surface is reached, because an origin per internal view multiplies CSP, rate-limit, and session surface for no operator capability that the admin UI lacks. The production instance runs outside the cluster ([ADR-0200](0200-cluster-topology.md)), so it has no `ops.<host>` origin at all and its administration is not an edge concern.

### Why the `ops.` label is load-bearing

Cookies are sent to a domain and its **descendants** only, never to siblings or to a parent's other children.

- Flat ops hostnames would have `<host>` — **the product origin** — as their only common parent, so any cookie shared across ops tools would also reach product.
- Nesting under `ops.<host>` lets an ops cookie be scoped to that label: it covers every tool and **not** the apex.

The `ops.` segment is the mechanism that makes "one operator login, isolated from product" expressible on a single registrable host.

### Cookie and session model

**Default: one parent-scoped session cookie**, shared across the apex and every ops origin. This is accepted because every origin under `<host>` is first-party and edge-gated: **tier isolation is enforced by per-tool authorization and by requiring a second factor on the ops tier, not by cookie scope.** A compromised non-operator product session reaching an ops origin is still denied by the dashboard gate.

**The property deliberately traded away is token-level isolation.** The session token is sent across the whole subtree, so an XSS on the high-surface product origin could ride an *operator's* session into the ops tools. The product-origin CSP ([ADR-0400](0400-frontend.md)) is the compensating control.

**This is the highest-severity accepted risk in the set, and it is recorded as one.** The impact is every ops tool an operator can reach, which is the cluster's control surface; the likelihood is whatever the product origin's XSS posture is; and the compensating control is a header, not a boundary. It carries its severity in [`docs/reference/risk-register.md`](../reference/risk-register.md) rather than only in this list, because a risk of this size read once in a Consequences section is a risk nobody re-reads.

**Isolating the token too is a deferred hardening.** Keep the product cookie host-only on the apex and have the ops tier mint its own scoped session via OIDC behind one ops-tier auth proxy — per tier, not per tool. One Kratos instance cannot issue two differently-scoped cookies, and the apex cannot set a cookie on the sibling subtree, so OIDC is the mechanism.

| Field | Value |
| --- | --- |
| **Trigger** | any non-first-party origin is hosted under `<host>`, or the product surface renders content one user supplies to another. The first makes the shared cookie unsound by construction; the second raises the XSS likelihood the CSP is holding alone |
| **Seam** | ✓ every ops route already passes one forward-auth middleware ([ADR-0305](0305-edge-auth-and-traffic-policy.md)), so the change is what that middleware validates. No tool is reconfigured, and no URL moves |
| **Cost if adopted late** | the shared parent-scoped cookie has already been readable by the newly added origin for as long as it has been hosted, and no later scoping retracts a token that has already been sent |

### Authorization is split by who owns the code

| Surface | Enforcement point |
| --- | --- |
| Product — our code | the app or service decides, through `Checker` ([ADR-0304](0304-identity-and-authorization.md)). The edge only authenticates, and page-level access is a `Checker` call in the render layer, not a bare session check |
| Ops dashboards — third-party | the edge decides, because the tool cannot run a permission check itself |

The ops gate's two layers, and why the coarse one deliberately makes no authz call, are [ADR-0304](0304-identity-and-authorization.md)'s. The consequence for this ADR: losing the authz plane degrades the ops tier to "any operator reaches any tool", never "nobody reaches anything".

### Certificates, DNS, and routing

- Two wildcard certificates per environment: one for the apex's siblings, which also covers `ops.<host>` itself, and one for the ops tools, which sit two labels deep. Both issued by cert-manager.
- Both wildcards resolve to the edge.
- Product routes match on host. Ops routes are one host-matched IngressRoute per tool behind the ops forward-auth middleware, host-parameterised so every environment shares the manifests.

### Content placement: the SEO axis against the trust axis

Two independent axes decide where a surface lives, and conflating them is the mistake this rule prevents.

- **Discoverability favours an apex subdirectory.** Google states it handles subdomains and subdirectories equivalently for ranking, so this is not a ranking claim: a subdirectory keeps one host, one certificate, and one analytics property, and it is same-origin with `/api`, which is what makes a "try it" console work without CORS.
- **Trust isolation favours a separate origin**, and that is a browser guarantee rather than a preference.

They conflict only for anonymous content, and one rule resolves it: **the first axis applies only to surfaces that are both anonymous and indexable. Anything behind a login is `noindex` and is placed on trust grounds alone.**

| Surface | Placement | Deciding axis |
| --- | --- | --- |
| Product docs, blog, guides, changelog | apex subdirectory | anonymous and indexable |
| Public API reference — `public` operations only | apex subdirectory | anonymous and indexable; first-party and read-only |
| Dev portal — audience `>= internal` | apex subdirectory | behind the session and `Checker`; same-origin with `/api`, so "try it" needs no CORS |
| Partner credential dashboard | its own subdomain, separate auth realm | behind login, so `noindex`; a non-Kratos realm must not share the apex session cookie |

**The external third tier is deferred.** An internal-only project has exactly the product and ops tiers, and the rows above describe where the third one lands when it exists.

| Field | Value |
| --- | --- |
| **Trigger** | a party outside the organisation is issued credentials against the API |
| **Seam** | ✓ the tier is a third host under the same wildcard and the same edge, and its auth realm is a second Oathkeeper rule set rather than a change to the first ([ADR-0305](0305-edge-auth-and-traffic-policy.md)). Nothing in the product or ops tiers moves |
| **Cost if adopted late** | partner credentials have already been issued through whatever surface existed, usually the product origin, so the separation begins by migrating live credentials and the sessions holding them |

### The API path

The service API is a **flat resource namespace** — the URL names the resource, not the service that owns it. This is the Stripe and GitHub facade: a caller sees one coherent surface, and which service serves a route is an internal detail that can change without breaking a URL.

**Collision governance.** One flat namespace means two services cannot both own a resource prefix. That is a feature — a name collision is a genuine domain-modelling conflict — and it is enforced by CI rather than left to chance. The edge route table is the ownership registry ([ADR-0305](0305-edge-auth-and-traffic-policy.md)).

**East-west endpoints are not on this surface.** They bypass the edge, are gated by NetworkPolicy, and appear in neither docs portal.

**One reserved path sits outside both tiers.** `/.well-known/` on the apex is neither product nor ops surface; it is where the web's own conventions live, so nothing else claims that prefix. The apex publishes [`security.txt`](https://www.rfc-editor.org/rfc/rfc9116) there — a contact address and a disclosure policy — because a researcher who finds something looks in exactly one place, and its absence routes the report to whatever public inbox they can find instead.

**The path holds for a public or partner API too.** It is distinguished by its `x-audience: public` contract and its JWT auth, not by its origin. Versioning never enters the URL: the default is single-live-version, and online versioning rides a header ([ADR-0303](0303-api-contracts-and-lifecycle.md)), which is precisely why the flat resource URL stays stable across versions.

## Consequences

### Positive

- Each tier is a separate origin, so an ops dashboard cannot read or script the product app, and a product-side XSS cannot script an ops tool. Browser-enforced, not conventional.
- Each ops tool is isolated from the others too: per-origin CSP, headers, rate limits, and storage.
- Tools that resist path hosting are each served at a clean root, with no base-path fights.
- A logged-in session does not imply tool access.
- Argo CD, Temporal, and the SeaweedFS admin UI get auth-gated URLs instead of port-forwarding.

### Negative / Risks

- **A second wildcard certificate and deeper DNS.** cert-manager handles it, and it is more moving parts.
- **No token-level tier isolation in the default model.** Accepted while every origin is first-party, with per-tool authorization, operator MFA, and the product CSP as compensating controls, and the OIDC upgrade as the closer.
- **Subdomain-takeover hygiene matters more.** Dangling ops DNS must not be left claimable.
- **The ops tier depends on one edge auth path.** Its failure locks operators out of every dashboard at once, which is why break-glass is a written procedure ([`docs/ops/break-glass.md`](../ops/break-glass.md)) rather than improvisation.

## Rules

- Surfaces belong to exactly one tier: product on the apex, ops tooling on `*.ops.<host>`. No operator dashboard is served from a product path, and no product surface from an `ops.` subdomain.
- A project shipping a public API adds a deferred external tier: public docs on an apex subdirectory and the partner credential dashboard on its own subdomain.
- Anonymous, indexable, first-party content is served from an apex subdirectory. A subdomain is used only for a distinct trust boundary, never merely because a surface is public.
- The public API docs portal is anonymous; only credential management is authenticated.
- The service API is a flat resource namespace at `<host>/api/<resource>`, never per-service. `(CI: lint:service-contract)`
- `/.well-known/` on the apex is reserved for web conventions and claimed by no product route. The apex serves `security.txt` with a contact address and a disclosure policy. `(ref: RFC 9116, RFC 8615)`
- The path holds for the public API too. A distinct origin is used only for hard credential isolation — which needs a separate registrable domain, since a parent-scoped cookie reaches a subdomain — or separate edge infrastructure at scale.
- Ops-tier hostnames are `{tool}.ops.<host>`, named after the **upstream project**, lowercase, matching the naming charset ([ADR-0003](0003-naming-and-identifiers.md)). Not the concept it serves, not its binary, and not an abbreviation of either.
- The default is one session cookie scoped to the parent host, with tier isolation enforced by per-tool authorization and an AAL2 requirement on the ops tier. This is permitted only while every origin under `<host>` is first-party and edge-gated.
- Splitting the cookie is the optional token-isolation upgrade and is mandatory if any non-first-party origin is hosted under `<host>`.
- Each environment provisions both wildcard certificates.
- A bare authenticated session never grants ops-tool access.

# ADR-0700: Marketing & Product Analytics

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0104](0104-supply-chain-security.md), [ADR-0200](0200-cluster-topology.md), [ADR-0204](0204-resource-management.md), [ADR-0301](0301-data-lifecycle-privacy.md), [ADR-0303](0303-api-contracts-and-lifecycle.md), [ADR-0304](0304-identity-and-authorization.md), [ADR-0306](0306-trust-tiers-and-urls.md), [ADR-0400](0400-frontend.md), [ADR-0500](0500-observability.md)

## Context

The platform has no instrument for the people who acquire its users. Marketing needs three things, which are usually conflated and are not equally cheap:

| Need | Question |
| --- | --- |
| Acquisition and journey | where a visitor came from, which page they entered on, what they navigated through |
| Engagement and conversion | time on page, clicks, custom events, and the funnel and retention questions built on them |
| Session replay | watching a recorded session to see why someone dropped off |

One requirement dominates all three: **an error must be connectable to the journey that produced it.**

The platform already collects browser signals. Faro POSTs to the edge, through the collector, into Loki and Tempo alongside service telemetry ([ADR-0400](0400-frontend.md), [ADR-0500](0500-observability.md)). That apparatus answers "is it broken or slow" and was never built to answer "did the campaign convert": Loki does counts adequately and funnels badly — no step joins, weak distinct-over-window — and marketing wants months of retention where ops logs want days.

**The correlation requirement eliminates most of the market before features are compared.** Every standalone analytics product mints its own session identity in its own store with its own retention, and Faro mints another. With no join key, "this user hit an error, show me their journey" degrades to correlating on timestamp and IP, which [ADR-0301](0301-data-lifecycle-privacy.md) makes worse rather than better.

So the decision is not which analytics tool. It is **which store holds Faro's events**.

## Decision drivers

1. **An error is connectable to the journey that produced it**, without correlating on timestamp and IP.
2. **A platform component costs more than a service here.** A component pays the full resource, supply-chain, network-policy, and backup tax; a service inherits all of it from the template.
3. **PII discipline is already decided.** An analytics store satisfies [ADR-0301](0301-data-lifecycle-privacy.md) on day one rather than being retrofitted.
4. **Marketing must self-serve without ops-tier access.** Marketing are not operators, and Grafana is not their tool ([ADR-0306](0306-trust-tiers-and-urls.md)).
5. **Collection stays vendor-neutral.** The backend is the replaceable part.

## Considered options

| Option | Session identity | Components added | Verdict |
| --- | --- | --- | --- |
| **Faro events into a first-party service on the existing Postgres** | **shared with ops telemetry** | **none** | **Chosen** |
| Umami | **its own, separate from Faro's** | **none — it runs on PostgreSQL**, which the platform already has | The cheapest self-hosted product here on component weight, and it still fails the correlation requirement: its session identity is its own, so a funnel step cannot join to a trace or an error. Privacy-first page and event analytics, without the product-analytics questions this ADR exists to answer |
| Plausible CE | its own | **ClickHouse in addition to PostgreSQL** | As Umami on correlation, and it brings the datastore Umami does not |
| Matomo | its own | MariaDB | As above, plus a second SQL engine to operate. Its funnel, cohort, and heatmap features are paid Marketplace plugins rather than free core, which is most of what would justify it |
| Rybbit, OpenPanel | its own | ClickHouse | Newer entrants in the same shape: a ClickHouse-backed product-analytics server with its own identity, so they lose the same way with less operating history |
| PostHog self-hosted | its own | a fleet of mostly-stateful systems — comparable to the entire Core floor in [`operational-surface.md`](../operational-surface.md) — plus a self-authored and permanently self-maintained chart | Kubernetes self-host is sunset upstream, leaving an unversioned image shipping continuously from master — irreconcilable with [ADR-0104](0104-supply-chain-security.md)'s digest-pin and signature admission, which exists precisely to forbid floating dependencies |
| PostHog Cloud | native OpenTelemetry ingest, ids auto-attached, a direct jump from exception to replay | none | **The closest product fit.** Rejected as the default because the data leaves the estate and it means operating two RUM agents on one frontend. **Retained as the documented escape hatch** |
| OpenReplay self-hosted | its own | a 2 vCPU / 8 GB / 50 GB floor plus bundled Postgres, Redis, and ClickHouse | Maintained charts, versioned releases, and the best replay in the category. Correlation to our traces is integration work rather than a native join. Reconsidered only if the replay trigger fires |
| Loki as the analytics store | shared | none | Viable for a first week and not as an endpoint: no funnel step joins, and routing identity-bearing events into the log store would breach [ADR-0500](0500-observability.md)'s PII rule |

## Decision

### One browser agent, one session identity

Faro remains the **only** browser agent. Marketing events are emitted through its event API under a reserved `marketing.*` namespace. No second SDK is added.

Correlation is therefore not something this ADR builds; it is a consequence of refusing to add a second agent. An exception in Tempo, a RUM log in Loki, and a funnel step in the analytics store all carry the same session id and the same trace context.

### The split happens at the collector

A `routing` connector in the existing DaemonSet collector evaluates a condition on the event name and diverts `marketing.*` events out of the logs pipeline into an exporter aimed at the analytics service. Ops signals are untouched.

This placement does three things at once:

- Marketing events **never reach Loki**, so [ADR-0500](0500-observability.md)'s PII rule holds for the log store.
- Marketing data gets its own retention class instead of inheriting the ops-log one.
- The browser stays ignorant of the topology, so the backend can be replaced without a frontend release.

It is additive configuration on a component already running: no gateway tier, no new component.

### The store is a service, not a platform component

`services/analytics/` is an ordinary first-party service built from the template: a partitioned events table on the existing cluster, generated queries, migrations, a spec at `x-audience: internal`, deployed by the shared chart. Funnels, retention, and cohorts are SQL window functions over that table, with pre-aggregated rollups refreshed by a Temporal `Schedule`.

**Zero platform components are added.** Backups, HA, network policy, resource governance, signing, and admission are already solved for this shape of thing, which is the entire point of putting the capability here rather than in a vendored chart.

### The panel is a route group on the product origin

Served under the product origin and `noindex`. This is the dev-portal pattern applied unchanged: a route group rather than a separate app, behind the app session, and **page-gated by `Checker` rather than by a bare session check**.

The proxy proves only that a session exists; the route-group root performs the authoritative check in the render layer.

The authorization model gains one type mirroring the existing ops-tier `dashboard` shape, so there is one pattern rather than two, granted through a marketing group. **This is what resolves the placement problem:** marketing never receives an ops-tier account, never needs operator MFA, and never touches Grafana, while the ops tier's least-authority rule stays intact.

### Identity, PII, and retention are declared up front

Events carry the session id and, when the visitor is authenticated, the identity id. That makes this table a **PII store by definition**, and it is treated as one from the first migration: a declared data class with a retention period, schema-level PII tags, reachable by the erasure and DSAR workflows, pruned by a retention `Schedule` ([ADR-0301](0301-data-lifecycle-privacy.md)).

Raw IP addresses are never stored, and user-agent is reduced to a parsed device class at ingest.

Doing this at design time is most of why the analytics store is a service we own rather than a vendored product: **an erasure workflow cannot reach into a third-party component's schema.**

### Consent, and where its boundary falls

[ePrivacy](https://eur-lex.europa.eu/eli/dir/2002/58/oj) Art. 5(3) attaches to **storing or reading information on the visitor's device**, not to processing in general. The boundary is therefore a technical line rather than a legal judgement, and the split is designed to sit on it:

| Path | Client-side storage | Basis |
| --- | --- | --- |
| Ops RUM — errors, web vitals, traces | **none.** The session id lives in memory for the page's lifetime and is never persisted | legitimate interest (GDPR Art. 6(1)(f)). No Art. 5(3) trigger, so no consent gate |
| `marketing.*` — journey, funnel, retention | persistent, because the questions span visits | **consent**, recorded before the first event |

Making the ops path storage-less is the load-bearing part: it is why a visitor who refuses analytics still gets working error reporting, and why refusing does not degrade the platform's ability to see that something is broken.

| Concern | Decision |
| --- | --- |
| Consent record | first-party, written through the analytics service. It carries a timestamp, the version of the purpose text shown, and the signal's source, because GDPR Art. 7(1) requires consent to be demonstrable after the fact |
| Withdrawal | through the same control that grants it (Art. 7(3)). Withdrawal stops emission on the next event, not on a support request |
| Prior signals | [Global Privacy Control](https://globalprivacycontrol.org/) is honoured as a refusal. A visitor sending it is never prompted, because prompting someone who already answered is the dark pattern the rule exists to stop |
| Enforcement | **two points.** The browser wrapper emits no `marketing.*` event without a recorded grant, and the analytics service drops any that arrive without one. The second exists because the first runs on a client the platform does not control |
| The consent record itself | PII. Declared class, declared retention, reachable by the erasure and DSAR workflows like every other row ([ADR-0301](0301-data-lifecycle-privacy.md)) |
| Purpose mapping | the single thing that varies by project and by counsel: which events fall under which purpose. It is configuration, reviewed at instantiation. **The mechanism above does not change with the answer** |

This is why consent is not deferred. The platform ships the mechanism; a project's legal review sets the purpose mapping it enforces. A regime with no consent requirement configures the mapping accordingly and the code path is identical.

### Session replay is deferred

Capture is open source and self-hosted transport works. **The player is not** — rendering a recorded session is a vendor cloud feature, and the OSS distribution has no documented player. Self-hosting replay therefore means either building one or adopting OpenReplay. Both are real projects, and replay is the worst PII surface here because it captures the DOM, which is unbounded by construction.

| Field | Value |
| --- | --- |
| **Trigger** | three distinct incidents within one quarter where a reported bug could not be reproduced from traces plus RUM logs alone |
| **Seam** | the capture instrumentation attaches to the agent already running, and payloads key by the same session id |
| **Cost if adopted late** | the incidents that motivated it are already unreproducible |

On that trigger, the choice between building a player and adopting OpenReplay is a separate ADR.

### Scale seam: ClickHouse

Postgres as a row store is the floor, not a ceiling claim.

| Field | Value |
| --- | --- |
| **Trigger** | p95 of the funnel query exceeding 2s against the declared rollup window, or the events table sustaining more than about 10M rows per month |
| **Seam** | collection, the routing connector, and the panel are unchanged. Only the service's store and queries move — which is exactly what the collector split buys |

## Consequences

### Positive

- **Zero new platform components.** The Core floor and the operational surface are unchanged.
- **Correlation is free**, because there is one agent and one session identity. No other option achieves this without either a second agent or a vendor.
- **Analytics data is under the same privacy discipline as everything else** from the first migration, rather than being the one store the erasure workflow cannot reach.
- **Marketing is served without expanding the ops tier.**
- **The backend stays swappable**, because the browser never learns where events are stored.

### Negative / Risks

- **Marketing gets a panel we build, not exploration they drive.** Every new question is a PR. This is the real cost, and it is a people cost rather than a component cost. If the question rate outgrows the platform team's capacity, the escape hatch is **PostHog Cloud**, not a self-hosted rebuild — and that switch is made deliberately.
- **Faro de-duplicates consecutive identical events**, so click counts undercount until emitters are shaped to defeat it. Proven by a test rather than assumed.
- **Consented-only analytics is not a census.** A funnel measures the consenting population, and the refusal rate is itself a confounder — refusers are not a random sample. Marketing must read these numbers as a directional panel rather than as traffic truth, and the ops RUM path is the only one that sees everyone.
- **Two enforcement points can disagree.** The service's view of a withdrawal lags the browser's by at most one event, so a withdrawn visitor's in-flight event is dropped server-side rather than never sent. That is the correct failure direction and it is still a discarded write.
- **Storage-lessness constrains the ops path permanently.** Any future ops feature wanting a cross-visit identifier — a returning-user metric, a durable device id — crosses the Art. 5(3) line and moves that feature behind consent. The boundary is cheap to hold now and expensive to move later.
- **Funnel SQL is work a free hosted tier would have given away.** Accepted in exchange for the correlation requirement and zero operational surface.
- **A routing-connector mistake sends identity-bearing events into Loki.** This is one line of configuration away at all times and needs a standing test, not review vigilance.

## Rules

- The frontend has exactly one browser telemetry agent. A second analytics SDK is not added — it mints a second session identity and breaks error-to-journey correlation.
- Marketing events are emitted under the reserved `marketing.*` namespace and routed out of the logs pipeline at the collector. A `marketing.*` event reaching Loki is a defect.
- Analytics storage is a first-party service on the existing cluster. A dedicated analytics datastore joins the platform only on the documented ClickHouse trigger.
- The marketing panel is a route group on the product origin, page-gated by `Checker` in addition to the session gate. Marketing surfaces on the ops tier are not created.
- The events table carries a declared data class and schema-level PII tags, and is reached by the erasure and DSAR workflows. Raw IP addresses are never stored.
- Session replay ships only after the documented incident trigger fires, and through its own ADR.
- The ops RUM path writes nothing to client-side storage; its session id is in-memory and per-page. A persistent identifier on that path is a defect. `(CI: e2e)`
- A `marketing.*` event is emitted only against a recorded consent grant, and the analytics service drops any that arrives without one. Both gates exist; neither is sufficient alone.
- Global Privacy Control is honoured as a refusal, and a visitor sending it is not prompted. `(ref: Global Privacy Control)`
- The consent record carries a timestamp, the purpose-text version, and the signal source, and is reachable by the erasure and DSAR workflows ([ADR-0301](0301-data-lifecycle-privacy.md)).
- The purpose mapping is configuration reviewed at project instantiation. Changing it never changes the enforcement path.

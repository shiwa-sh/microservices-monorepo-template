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

1. **Correlation by construction, not by join.** One browser agent means one session identity.
2. **This platform makes services cheap and components expensive.** A platform component pays the full resource, supply-chain, network-policy, and backup tax. Capability belongs in a service.
3. **PII discipline is already decided.** An analytics store satisfies [ADR-0301](0301-data-lifecycle-privacy.md) on day one rather than being retrofitted.
4. **Marketing must self-serve without ops-tier access.** Marketing are not operators, and Grafana is not their tool ([ADR-0306](0306-trust-tiers-and-urls.md)).
5. **Collection stays vendor-neutral.** The backend is the replaceable part.

## Considered options

| Option | Session identity | Components added | Verdict |
| --- | --- | --- | --- |
| **Faro events into a first-party service on the existing Postgres** | **shared with ops telemetry** | **none** | **Chosen** |
| Umami, Plausible CE, Matomo, Rybbit, OpenPanel | **its own, separate from Faro's** | a datastore the platform does not have — ClickHouse or MariaDB | The correlation requirement fails regardless of feature set. Matomo's headline features are paid plugins rather than free core |
| PostHog self-hosted | its own | roughly eight mostly-stateful systems against a Core floor of about 23, plus a self-authored and permanently self-maintained chart | Kubernetes self-host is sunset upstream, leaving an unversioned image shipping continuously from master — irreconcilable with [ADR-0104](0104-supply-chain-security.md)'s digest-pin and signature admission, which exists precisely to forbid floating dependencies |
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
- **Consent is an open legal question.** The session tracker uses web storage, and the analytics purpose is generally consent-requiring even though this is first-party and carries no advertising cookie. The proposed split — consent gates `marketing.*` events only, while ops RUM continues on legitimate-interest grounds — **requires legal sign-off and is not the platform team's decision.** Until signed off, the emitters ship disabled.
- **Funnel SQL is work a free hosted tier would have given away.** Accepted in exchange for the correlation requirement and zero operational surface.
- **A routing-connector mistake sends identity-bearing events into Loki.** This is one line of configuration away at all times and needs a standing test, not review vigilance.

### Follow-ups

- `services/analytics/` scaffold: event schema, partitioning, rollup `Schedule`, spec.
- The data-class registry entry, schema-level PII tags, and erasure and DSAR activities for the new store.
- The routing connector, and a test asserting no `marketing.*` event reaches Loki.
- The route group, its session entry, the authorization type, and assertions covering grant and denial.
- The consent gate in the frontend, and legal sign-off on the purpose split.
- A test proving event de-duplication does not corrupt click counts.
- The ClickHouse Scale row in [`docs/operational-surface.md`](../operational-surface.md).
- `docs/marketing/analytics.md`: the event taxonomy and a query cookbook.

## Rules

- The frontend has exactly one browser telemetry agent. A second analytics SDK is not added — it mints a second session identity and breaks error-to-journey correlation. `(review-only)`
- Marketing events are emitted under the reserved `marketing.*` namespace and routed out of the logs pipeline at the collector. A `marketing.*` event reaching Loki is a defect. `(CI: to be added with the routing connector)`
- Analytics storage is a first-party service on the existing cluster. A dedicated analytics datastore joins the platform only on the documented ClickHouse trigger. `(review-only)`
- The marketing panel is a route group on the product origin, page-gated by `Checker` in addition to the session gate. Marketing surfaces on the ops tier are not created. `(review-only)`
- The events table carries a declared data class and schema-level PII tags, and is reached by the erasure and DSAR workflows. Raw IP addresses are never stored. `(review-only)`
- Session replay ships only after the documented incident trigger fires, and through its own ADR. `(review-only)`
- The `marketing.*` emitters stay disabled until the consent purpose split has legal sign-off. `(review-only)`

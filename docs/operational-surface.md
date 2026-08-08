# Operational Surface & Component Tiers

This document is the running inventory of every platform component the template operates, sorted into three tiers, plus the budget rule that governs adding to the always-on floor. It makes [ADR-0000](adr/0000-platform-foundations.md)'s *thinnest viable platform* principle (2) checkable rather than aspirational.

The unit that matters, given the platform-engineering budget in [ADR-0000](adr/0000-platform-foundations.md), is not the raw component count — most of these components are the honest production floor for a multi-tenant application at meaningful load. The unit that matters is **operational surface the platform team must understand at 3am**. Tiering makes that surface explicit and bounded.

## The three tiers

### Core — always on

The real floor. Every environment runs these; a project removes one only by writing an ADR that shows the floor already covers its concern.

| Component | Concern | ADR |
| --- | --- | --- |
| k3s | Kubernetes runtime | [0200](adr/0200-cluster-topology.md) |
| Cilium (+ WireGuard, default-deny) | CNI, east-west encryption, network policy | [0200](adr/0200-cluster-topology.md) |
| Traefik | Ingress / edge routing | [0305](adr/0305-edge-auth-and-traffic-policy.md) |
| cert-manager | TLS certificate lifecycle | [0200](adr/0200-cluster-topology.md) |
| CloudNativePG (CNPG) | Postgres | [0300](adr/0300-data.md) |
| ArgoCD | GitOps reconciliation | [0201](adr/0201-gitops.md) |
| Kratos | Identity / authentication | [0304](adr/0304-identity-and-authorization.md) |
| Oathkeeper | Edge authorization (forward-auth) | [0305](adr/0305-edge-auth-and-traffic-policy.md), [0304](adr/0304-identity-and-authorization.md) |
| sops-operator | Secret decryption | [0202](adr/0202-secrets.md) |
| Kyverno | Admission policy (image signatures, digest pins) — **declared Core, not yet deployed**; see [ADR-0204](adr/0204-resource-management.md) *Risks* | [0104](adr/0104-supply-chain-security.md) |
| OpenFGA | Authorization (ReBAC) | [0304](adr/0304-identity-and-authorization.md) |
| Temporal | Durable execution | [0302](adr/0302-temporal.md) |
| Temporal Worker Controller | Versioned (rainbow) worker deploys — one Deployment per Build ID, retained until drained | [0302](adr/0302-temporal.md) |
| Loki | Log storage | [0500](adr/0500-observability.md) |
| Tempo | Trace storage | [0500](adr/0500-observability.md) |
| Prometheus | Metrics storage | [0500](adr/0500-observability.md) |
| Pyroscope | Continuous profile storage | [0500](adr/0500-observability.md) |
| Grafana | One UI for all four signals, with cross-signal navigation | [0500](adr/0500-observability.md), [0501](adr/0501-operator-uis-and-dashboards.md) |
| OTel Collector | Telemetry collection (DaemonSet) | [0500](adr/0500-observability.md) |
| Alloy | Scrapes `pprof` endpoints and pushes to Pyroscope | [0500](adr/0500-observability.md) |
| MinIO | Object storage (non-prod; a real bucket in prod) | [0205](adr/0205-environment-parity.md) |
| Lowdefy admin | Internal ops CRUD over the Go API | [0401](adr/0401-internal-admin.md) |
| Headlamp | Read-mostly Kubernetes debug UI | [0501](adr/0501-operator-uis-and-dashboards.md) |
| Hubble UI | Live service map, network flows, drop verdicts (bundled with Cilium) | [0501](adr/0501-operator-uis-and-dashboards.md) |
| pgweb | Read-only break-glass DB inspector | [0401](adr/0401-internal-admin.md) |

### Core, decided but not yet deployed

Decided in an ADR, with no chart in `infra/` yet. Each is counted in [ADR-0000](adr/0000-platform-foundations.md)'s always-on floor, and axis B at maximum makes each a first-class decision rather than a signup.

| Component | Concern | ADR |
| --- | --- | --- |
| Forgejo | Source control, review, and pipelines | [0102](adr/0102-source-control-and-ci.md) |
| zot | Image registry, and the store for signatures and attestations | [0105](adr/0105-image-registry.md) |
| maddy | Outbound mail submission and DKIM signing | [0307](adr/0307-outbound-email.md) |
| Alertmanager | Alert routing, grouping, and silences | [0502](adr/0502-alerting-and-on-call.md) |

Outbound email is the sharpest of the four. It is the platform's **only** component whose principal cost — deliverability reputation — engineering effort cannot retire, which is why [ADR-0000](adr/0000-platform-foundations.md) ranks it first to concede on a move down axis B. Until it is deployed, identity verification and recovery have no production path.

### Scale — documented swap-in when a real signal demands it

Not shipped by default. Each is a variant the Core floor grows into when a measured signal appears. **A Scale row is only legitimate if its seam already exists** — otherwise it is a bet, not a deferral ([ADR-0000](adr/0000-platform-foundations.md)).

| Swap | From (Core) | Trigger | Seam |
| --- | --- | --- | --- |
| **Mimir** | Prometheus | metrics need multi-tenant isolation, HA, or long-retention object-storage durability | ✅ Prometheus-compatible: same query API, dashboards, and alert rules, so the swap touches storage config, not instrumentation ([ADR-0500](adr/0500-observability.md)) |
| **Temporal for a given trivial job** | outbox plus a small worker | a best-effort job grows a second step, a cross-service call, compensation, or a durability guarantee | ✅ Temporal is already Core; this seam governs only whether one job earns a workflow ([ADR-0302](adr/0302-temporal.md)) |
| **Collector gateway tier** | DaemonSet only | trace volume justifies tail sampling | ✅ services always emit to `localhost:4317`, so the gateway is a deploy, not a code change ([ADR-0500](adr/0500-observability.md)) |
| **Loki / Tempo microservices mode** | monolithic single binary | one backend's ingest volume outgrows a single Deployment | ✅ object storage already holds the data, so the split is a values change, not a data migration |
| **k6-operator** | the k6 binary on a developer or CI machine | k6 reports `dropped_iterations` > 0 at the target rate, or absolute capacity figures are needed rather than relative regression signals | ✅ scenarios are unchanged; only where the VUs run changes ([ADR-0601](adr/0601-testing-strategy.md)) |
| **ClickHouse** | the analytics service on the shared Postgres | funnel-query p95 above 2s against the declared rollup window, or the events table sustaining more than ~10M rows per month | ✅ collection, the routing connector, and the panel are unchanged; only the service's store and queries move ([ADR-0700](adr/0700-analytics.md)) |
| **Longhorn** | `local-path` volumes | any volume exceeding 50% of node disk | ⚠️ a **bet** — existing volumes migrate per workload, so this is a data move rather than a values change ([ADR-0200](adr/0200-cluster-topology.md)) |
| **GlitchTip** | `exception.*` records grouped by fingerprint in Loki and Grafana | fingerprint triage becomes routine work rather than incident work, or the novelty window misses a fault that reached a customer | ✅ it ingests the Sentry envelope protocol, so the collector gains an exporter and services are not re-instrumented ([ADR-0503](adr/0503-error-tracking.md)) |

### Opt-in — flag-gated, off unless a project asks

Real components that most instances never need. Gated behind a project flag, the same pattern as `hydra_thirdparty`.

| Component | Concern | Flag / ADR |
| --- | --- | --- |
| Hydra | OAuth2 provider for third-party API clients | [0304](adr/0304-identity-and-authorization.md) |

## Budget rule

The floor is not free. Every Core component is a system the platform team must be able to reason about during an incident. So:

- **Nothing joins Core without answering "does the floor already cover this?"** A new always-on component must show that no existing Core part serves its concern. If an existing part covers 90% of the need, that is the answer.
- **Prefer the Core floor over a Scale variant until a measured signal appears.** Picking the scale-tier variant of a component before scale exists is the template's characteristic over-commitment. Ship the floor; document the seam.
- **A Scale swap needs a written trigger**, not a preference. The trigger is a measurable condition (ingest volume, tenant count, retention window), stated in the table above and in the owning ADR.
- **An Opt-in component ships off.** It carries no operational surface until a project flips its flag, at which point that project owns its runbook.

This budget is what makes [ADR-0000](adr/0000-platform-foundations.md) principle 3 (*operational sovereignty*) affordable. That principle is a **hard constraint** — self-hosting is not traded away to reclaim operational budget — so the floor is the only lever left: when operating it consistently crowds out feature work, the response is to *remove or consolidate* a Core component per its ADR, or to add capacity. It is never to keep adding.

A project whose sovereignty requirement is weaker sits at a different point on axis B and says so in its own ADR ([ADR-0000](adr/0000-platform-foundations.md), *Moving down axis B*). That is a different position, not an escape hatch inside this one.

# Operational Surface & Component Tiers

This document is the running inventory of every platform component the template operates, sorted into three tiers, plus the budget rule that governs adding to the always-on floor. [`adoption-path.md`](adoption-path.md) runs the other way, and governs *removing* from it. It makes [ADR-0000](adr/0000-platform-foundations.md)'s *thinnest viable platform* principle (2) checkable rather than aspirational.

The unit that matters, given the platform-engineering budget in [ADR-0000](adr/0000-platform-foundations.md), is not the raw component count — most of these components are the honest production floor for a multi-tenant application at meaningful load. The unit that matters is **operational surface the platform team must understand at 3am**. Tiering makes that surface explicit and bounded.

## The three tiers

### Core — always on

The real floor. Every environment runs these; a project removes one only by writing an ADR that shows the floor already covers its concern, or by taking a row from [`adoption-path.md`](adoption-path.md) and recording the restore trigger it is watching.

**Recurring obligation** is the work that arrives whether or not anything is wrong — the work a busy quarter silently defers. **Failure needs** is what the responder must already have or already know when the component is the reason something is broken at 3am. Together they are the demand side of [ADR-0000](adr/0000-platform-foundations.md)'s capacity question, and no headcount is stated for them: a project sums this column against its own coverage obligations and reaches its own number.

| Component | Concern | Recurring obligation | Failure needs | ADR |
| --- | --- | --- | --- | --- |
| Talos Linux | node OS and Kubernetes runtime, as one artefact | `talosctl upgrade` per node as an A/B image swap, and `talosctl upgrade-k8s` on its own cadence | the machine-config document, node API access, and a rebuild path that has been walked before | [0200](adr/0200-cluster-topology.md) |
| Cilium (+ WireGuard, default-deny) | CNI, east-west encryption, network policy | chart upgrades once Argo CD adopts the bootstrap release; policy review as services are added | eBPF-level debugging — `cilium status`, the Hubble CLI. Harder than flannel, and the ADR says so. **Cannot be hot-swapped on a live cluster** | [0200](adr/0200-cluster-topology.md) |
| Traefik | Ingress / edge routing | chart upgrades; route and middleware review as the edge surface grows | reading the edge config together with the Oathkeeper filter behind it | [0305](adr/0305-edge-auth-and-traffic-policy.md) |
| cert-manager | TLS certificate lifecycle | one wildcard certificate per environment renewing over ACME DNS-01, and the provider API credential behind it | DNS provider access, and the ability to tell a renewal failure from a propagation delay | [0200](adr/0200-cluster-topology.md) |
| CloudNativePG (CNPG) | Postgres | minor upgrades; backup verification; a restore rehearsed **quarterly**; a major-version upgrade once per Postgres release cadence in a planned window | someone who has performed a failover and a PITR before, not read about one. The single highest-stakes row here | [0300](adr/0300-data.md) |
| ArgoCD | GitOps reconciliation | chart upgrades; triage of drift notifications, since the production platform runs `selfHeal=false` and reverts nothing on its own | knowing that a manual sync is the break-glass once retries exhaust, and that a cluster rebuild is never the remedy | [0201](adr/0201-gitops.md) |
| Kratos | Identity / authentication | config and schema migrations on upgrade; the identity data is the most painful in the platform to migrate | reading the self-service flows, AAL levels, and session semantics under pressure | [0304](adr/0304-identity-and-authorization.md) |
| Oathkeeper | Edge authorization (forward-auth) | keeping the rule set in step with routes as services are added | JWKS, session-cookie validation, and the header contract every service downstream trusts | [0305](adr/0305-edge-auth-and-traffic-policy.md), [0304](adr/0304-identity-and-authorization.md) |
| sops-operator | Secret decryption | recipient churn on every onboarding and offboarding — `sops updatekeys` across every encrypted file, plus rotation of every secret a departing engineer could read; cluster-key rotation with an overlap window | the ops-recovery key, held offline on more than one person's hardware token | [0202](adr/0202-secrets.md) |
| Kyverno | Admission policy (image signatures, digest pins) — **declared Core, not yet deployed**; see [ADR-0204](adr/0204-resource-management.md) *Risks* | policy upkeep; carrying current and previous public keys through a signing-key rotation window | recognising that a failing admission webhook blocks deploys cluster-wide, and knowing the intended break-glass | [0104](adr/0104-supply-chain-security.md) |
| OpenFGA | Authorization (ReBAC) | model changes shipped through the idempotent seeding Job; tuple data grows with the domain | reading the authorization model, and distinguishing a missing tuple from a wrong model | [0304](adr/0304-identity-and-authorization.md) |
| Temporal | Durable execution | server upgrades; namespace, retention, and archival configuration; it is another tenant on the Postgres cluster | workflow history, replay semantics, and telling a non-determinism error from an application fault | [0302](adr/0302-temporal.md) |
| Temporal Worker Controller | Versioned (rainbow) worker deploys — one Deployment per Build ID, retained until drained | watching versions drain, and reaping those that have | the failure that surprises people: deleting a `WorkerDeployment` **blocks on a finalizer** while pollers remain, so the CR sits `Terminating` and Argo cannot recreate it | [0302](adr/0302-temporal.md) |
| Loki | Log storage | retention and compaction against the bucket; chart upgrades | LogQL, and where the bucket lifecycle rules are set | [0500](adr/0500-observability.md) |
| Tempo | Trace storage | retention against the bucket; chart upgrades | reading a trace across services, and knowing sampling is set at the collector | [0500](adr/0500-observability.md) |
| Prometheus | Metrics storage | local TSDB sizing as active series grow; alert-rule upkeep, since a rule without a `severity` label is a defect | PromQL, and the active-series threshold that signals the Mimir swap | [0500](adr/0500-observability.md) |
| Pyroscope | Continuous profile storage | retention against the bucket; chart upgrades | reading a flame graph well enough to act on it | [0500](adr/0500-observability.md) |
| Grafana | One UI for all four signals, with cross-signal navigation | dashboards are committed JSON, so panels are maintained in the repository as services change | knowing it trusts the edge and serves anonymously — an edge fault takes the debugging surface with it | [0500](adr/0500-observability.md), [0501](adr/0501-operator-uis-and-dashboards.md) |
| OTel Collector | Telemetry collection (DaemonSet) | pipeline configuration and version bumps across a fast-moving upstream | that every service emits to `localhost:4317`, so a DaemonSet fault is a per-node telemetry blackout | [0500](adr/0500-observability.md) |
| Alloy | Scrapes `pprof` endpoints and pushes to Pyroscope | scrape configuration as services are added | little — its failure degrades profiling only | [0500](adr/0500-observability.md) |
| MinIO | Object storage (non-prod; a real bucket in prod) | credentials and upgrades in non-prod; production points at an external bucket instead | knowing the console is the one surface that does not accept the edge's proxy-trust and prompts for root credentials | [0205](adr/0205-environment-parity.md) |
| Lowdefy admin | Internal ops CRUD over the Go API | page YAML tracks the API as it changes; a stale page is a broken ops task | reading the YAML, which is the point of choosing it | [0401](adr/0401-internal-admin.md) |
| Headlamp | Read-mostly Kubernetes debug UI | chart upgrades | none beyond `kubectl`, which is also its replacement | [0501](adr/0501-operator-uis-and-dashboards.md) |
| Hubble UI | Live service map, network flows, drop verdicts (bundled with Cilium) | none of its own — it upgrades with Cilium | none beyond the Hubble CLI, which is also its replacement | [0501](adr/0501-operator-uis-and-dashboards.md) |
| pgweb | Read-only break-glass DB inspector | chart upgrades; the read-only role is enforced at the database level and must stay that way | knowing it is read-only by database role, not by convention | [0401](adr/0401-internal-admin.md) |

### Core, decided but not yet deployed

Decided in an ADR, with no chart in `infra/` yet. Each is counted in [ADR-0000](adr/0000-platform-foundations.md)'s always-on floor, and axis B at maximum makes each a first-class decision rather than a signup.

| Component | Concern | Recurring obligation | Failure needs | ADR |
| --- | --- | --- | --- | --- |
| Forgejo | Source control, review, and pipelines | upgrades, runner capacity, and its tenancy on the Postgres cluster | knowing the forge is also where CI runs, so its outage stops delivery as well as review | [0102](adr/0102-source-control-and-ci.md) |
| zot | Image registry, and the store for signatures and attestations | retention as a committed config file; upgrades | knowing that pods pull from it, so an outage is a cluster-wide inability to start new workloads | [0105](adr/0105-image-registry.md) |
| maddy | Outbound mail submission and DKIM signing | **the heaviest recurring row in the floor**: IP warmup, DMARC aggregate-report review, and blocklist monitoring, none of which engineering effort retires | DNS access for SPF, DKIM, and DMARC, plus the understanding that a delivery fault shows up first as a drop in identity-flow completion | [0307](adr/0307-outbound-email.md) |
| Alertmanager | Alert routing, grouping, and silences | route and silence upkeep as rules change | knowing that **nothing pages anyone** — no escalation receiver is attached, so an overnight incident is found in the morning | [0502](adr/0502-alerting-and-on-call.md) |

Outbound email is the sharpest of the four. It is the platform's **only** component whose principal cost — deliverability reputation — engineering effort cannot retire, which is why [ADR-0000](adr/0000-platform-foundations.md) ranks it first to concede on a move down axis B. Until it is deployed, identity verification and recovery have no production path.

### Scale — documented swap-in when a real signal demands it

Not shipped by default. Each is a variant the Core floor grows into when a measured signal appears. **A Scale row is only legitimate if its seam already exists** — otherwise it is a bet, not a deferral ([ADR-0000](adr/0000-platform-foundations.md)).

**The seam marker describes the forward direction only.** A swap that is cheap to adopt is not automatically cheap to abandon, and the two costs are recorded separately for the same reason [`adoption-path.md`](adoption-path.md) ranks reductions by cost-to-reverse rather than by saving. A row whose reversal is a data move is one-way in practice: adopt it on the trigger, not in anticipation of it.

| Swap | From (Core) | Trigger to adopt | Seam (forward) | Cost to reverse |
| --- | --- | --- | --- | --- |
| **Mimir** | Prometheus | metrics need multi-tenant isolation, HA, or long-retention object-storage durability | ✅ Prometheus-compatible: same query API, dashboards, and alert rules, so the swap touches storage config, not instrumentation ([ADR-0500](adr/0500-observability.md)) | **moderate, and asymmetric.** Dashboards and rules return untouched, but history written to the bucket does not come back into a local TSDB — either Mimir stays for reads or the retention obligation that justified it is dropped first |
| **Temporal for a given trivial job** | outbox plus a small worker | a best-effort job grows a second step, a cross-service call, compensation, or a durability guarantee | ✅ Temporal is already Core; this seam governs only whether one job earns a workflow ([ADR-0302](adr/0302-temporal.md)) | **low**, and scoped to one job. Reversal is rare in practice because the trigger is a step that was added, and steps are seldom removed |
| **Collector gateway tier** | DaemonSet only | trace volume justifies tail sampling | ✅ services always emit to `localhost:4317`, so the gateway is a deploy, not a code change ([ADR-0500](adr/0500-observability.md)) | **low.** Remove the deploy; sampling policy returns to the DaemonSet. Genuinely symmetric, because services never learned the gateway existed |
| **Loki / Tempo microservices mode** | monolithic single binary | one backend's ingest volume outgrows a single Deployment | ✅ object storage already holds the data, so the split is a values change, not a data migration | **low.** Object storage holds the data in both shapes, so collapsing back is the same values change run the other way. The most reversible row here |
| **k6-operator** | the k6 binary on a developer or CI machine | k6 reports `dropped_iterations` > 0 at the target rate, or absolute capacity figures are needed rather than relative regression signals | ✅ scenarios are unchanged; only where the VUs run changes ([ADR-0601](adr/0601-testing-strategy.md)) | **low.** Scenarios are the artefact and they do not change; only the runner does |
| **ClickHouse** | the analytics service on the shared Postgres | funnel-query p95 above 2s against the declared rollup window, or the events table sustaining more than ~10M rows per month | ✅ collection, the routing connector, and the panel are unchanged; only the service's store and queries move ([ADR-0700](adr/0700-analytics.md)) | **high — one-way in practice.** The queries were rewritten for a column store, and the volume that triggered the move is the volume Postgres refused. Reversal means a data move backwards into the store that could not hold it |
| **Longhorn** | `local-path` volumes | any volume exceeding 50% of node disk | ⚠️ a **bet** — existing volumes migrate per workload, so this is a data move rather than a values change ([ADR-0200](adr/0200-cluster-topology.md)) | **high, in both directions.** A per-workload data move each way. The only row here that is expensive to adopt *and* expensive to abandon, which is why it is marked a bet rather than a deferral |
| **GlitchTip** | `exception.*` records grouped by fingerprint in Loki and Grafana | fingerprint triage becomes routine work rather than incident work, or the novelty window misses a fault that reached a customer | ✅ it ingests the Sentry envelope protocol, so the collector gains an exporter and services are not re-instrumented ([ADR-0503](adr/0503-error-tracking.md)) | **low**, minus history. Drop the exporter and the chart; grouping returns to Loki, but the accumulated fingerprint history goes with the component ([ADR-0503](adr/0503-error-tracking.md)) |

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

This budget is what makes [ADR-0000](adr/0000-platform-foundations.md) principle 3 (*operational sovereignty*) affordable. That principle is a **hard constraint** — self-hosting is not traded away to reclaim operational budget — so the floor is the only lever left: when operating it consistently crowds out feature work, the response is to *remove or consolidate* a Core component per [`adoption-path.md`](adoption-path.md), or to add capacity. It is never to keep adding.

A project whose sovereignty requirement is weaker sits at a different point on axis B and says so in its own ADR ([ADR-0000](adr/0000-platform-foundations.md), *Moving down axis B*). That is a different position, not an escape hatch inside this one.

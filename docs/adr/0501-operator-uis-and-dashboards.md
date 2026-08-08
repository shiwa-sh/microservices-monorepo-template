# ADR-0501: Operator UIs & Dashboard Hierarchy

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0200](0200-cluster-topology.md), [ADR-0201](0201-gitops.md), [ADR-0306](0306-trust-tiers-and-urls.md), [ADR-0500](0500-observability.md), [ADR-0601](0601-testing-strategy.md)

## Context

[ADR-0500](0500-observability.md) pins the Grafana/LGTM backend for all four signals. Above that backend, an operator under incident asks four questions, and each needs an owning surface:

| Question | Owning surface |
| --- | --- |
| Is anything wrong right now, and where? | Grafana |
| What talks to what, and is the network denying a flow? | Hubble UI |
| What pods exist, describe them, tail their logs | Headlamp |
| What is deployed, and did it sync? | Argo CD ([ADR-0201](0201-gitops.md)) |

Cilium already runs Hubble agent and relay as the audit surface for the default-deny NetworkPolicy posture ([ADR-0200](0200-cluster-topology.md)), so a live flow graph exists in the cluster and only its UI is open.

Without a stated dashboard structure, a growing dashboard set invites two failure modes: one mega-dashboard nobody reads, and unowned sprawl.

## Decision drivers

1. **One pane of glass.** Two tools that appear to own the same signals is an operational cost, not redundancy.
2. **Dashboards as code** ([ADR-0000](0000-platform-foundations.md), principle 1). The observability surface lives in git.
3. **Bounded footprint** ([ADR-0000](0000-platform-foundations.md), principle 2). The template pays for signal, not for stacks.
4. **The landing page answers one question in seconds**, without scanning a table.
5. **No UI mutates desired state.** A debug surface is not a control surface.

## Considered options

### Application-observability suite

| Option | Service map | Dashboards as code | Footprint | Verdict |
| --- | --- | --- | --- | --- |
| **Grafana stack + bundled Hubble UI** | Hubble UI, live | JSON in git | zero marginal — both already run | **Chosen** |
| Coroot Community Edition (Apache-2.0) | keyed by Deployment, no name collapse | **UI-only click-ops state in ClickHouse** — not in git, not reviewable, invisible to Argo | five always-on components plus ClickHouse | Rejected. Fails driver 2 outright, and duplicates most of the Grafana stack. CE also has no SSO/RBAC, ~7-day retention, requires tracefs/debugfs mounts, and exposes no stable query API for the acceptance gauge ([ADR-0601](0601-testing-strategy.md)). Its eBPF extras are real, but instrumented services already ship richer OTLP |
| Isovalent/Cisco Enterprise Hubble | maintained, no collapse defect | n/a | n/a | Commercial ([ADR-0000](0000-platform-foundations.md), principle 3) |
| No map at all | `hubble` CLI answers point queries only | n/a | zero | The UI is free once Hubble runs |

### Service-map data source inside Grafana

Both candidate feeds fail structurally, so no Grafana service-map dashboard is built.

| Feed | Failure | Consequence |
| --- | --- | --- |
| Tempo `metrics-generator` `service_graphs` | Edges appear only when both sides of a CLIENT/SERVER span pair land in one instance — measured: ~1 edge | The generator, Prometheus's `--web.enable-remote-write-receiver`, and a tempo→prometheus NetworkPolicy rule stay undeployed |
| Hubble `flow` metric | With `sourceContext=workload` the client side of most intra-cluster flows resolves to an empty label — measured: 82/391 series with an empty source, zero platform→platform edges | A Node Graph collapses into one unnamed hub. The `flow` metric stays off |

### Kubernetes debug UI

| Option | Shared web origin | Verdict |
| --- | --- | --- |
| **Headlamp** (CNCF Sandbox, Apache-2.0) | yes, OIDC/header aware | **Chosen** — one in-cluster deployment, drops into the existing ops-origin pattern |
| k9s | no — a TUI | No shared origin, no edge gating |
| Kubernetes Dashboard | yes | Token-handling design and past-CVE history, unwanted on an ops origin |

## Decision

### Grafana owns application observability

Dashboards form a three-level funnel. Each level answers one question and links down. All are JSON under `infra/observability/dashboards/` ([ADR-0500](0500-observability.md)).

| Level | Dashboard | Question | Home |
| --- | --- | --- | --- |
| L1 | **Overview** (`overview.json`) | Is anything wrong right now, and where? | **yes** |
| L1.5 | **Applications** (`applications.json`) | What is running, and how big or busy is each workload? | |
| L2 | **Service detail** (`service-detail.json`) | What symptom does this service show? SLO, RED, resources, logs, traces, profiles | |
| L3 | **Platform components** (`platform-components.json`) | Is a stateful dependency the cause? | |
| L3 | **Network: policy denials** (`hubble-drops.json`) | Is a NetworkPolicy blocking a flow? | |

**Overview's contract: every panel either turns red or links down a level.** No deep-dive timeseries. Row order:

1. **Cluster capacity** — node CPU, memory, disk free; CPU and memory requests committed; pod slots used.
2. Firing Prometheus alerts — count and table, from `ALERTS`.
3. Cluster-wide golden signals — total req/s, 5xx %, p95.
4. Per-service SLO table — the same expressions as the Service detail SLO tiles, rows linking to it.
5. One-line platform-dependency stats — Postgres, Temporal, policy drops — linking to their L3 dashboards.

Capacity sits above the alert count, the one departure from verdict-before-everything. It earns the position by being the only *leading* row: exhausting disk, memory, or schedulable capacity is still actionable, whereas a firing alert is already the incident. It also has no other home, because every other panel is per-service or per-dependency. Both ceilings are shown because they are different failures — requests-committed predicts "the scheduler refuses the next pod", real usage predicts "the kernel kills a container", and either can occur while the other looks healthy.

**Applications** is the workload directory one click below Overview: kubeletstats-driven, so idle and non-HTTP pods appear, which the HTTP-traffic-driven SLO table does not guarantee.

A new panel must place itself: does it change the L1 verdict, describe one service (L2), or one dependency (L3)? A question fitting none of the three is a Grafana Explore query, not a committed dashboard.

### Hubble UI owns the service map

Served at `hubble.ops.<host>` behind the ops forward-auth — the operator claim plus AAL2, with `dashboard:hubble#view` as the optional per-tool refinement ([ADR-0304](0304-identity-and-authorization.md)). Set by `hubble.ui.enabled=true` in the Cilium values, at the origin root, because its React Router cannot run under a path prefix. It reads the relay's live flow stream and owns interactive drop investigation (`verdict=DROPPED` plus reason, live).

Two accepted defects:

- **It collapses workloads sharing `app.kubernetes.io/name` into one card** — all Temporal roles render as one "temporal" node. Cosmetic: the map serves topology, and per-workload detail is Grafana's, where `k8s_deployment_name` distinguishes roles.
- **Upstream is in maintenance mode** (v0.13.5, April 2024). Accepted against the liveness criterion because it is a read-only viewer over data already collected — abandonment costs the view, not the data, which makes it a control-plane risk under [ADR-0000](0000-platform-foundations.md) principle 4. If it breaks outright, this ADR is revisited.

### Headlamp owns pod-level debugging

Served at `headlamp.ops.<host>` behind the same ops forward-auth, with the optional per-tool grant `dashboard:headlamp#view`.

Headlamp is bound to the built-in **`view` ClusterRole**: list, get, watch, describe, and pod-log tailing. `view` grants no `create` verbs, so there is **no exec and no port-forward**, and it **cannot read `Secret` objects**. It is a debugging surface, not a control surface — it does not create, edit, or delete reconciled resources. Desired-state changes go through git and Argo CD.

A project wanting interactive exec or port-forward swaps `view` for a custom role that adds those verbs, and takes the wider surface knowingly. A project wanting tighter limits narrows the role further, for example by dropping `pods/log`.

### Metric split between live and historical

`hubble_drop_total` is enabled for what a live UI cannot do: **history** (the `hubble-drops` dashboard) and **alerting** (`PolicyDropsDetected` on `rate(hubble_drop_total{reason="POLICY_DENIED"}[5m])`, `infra/observability/alerts/policy-drops.yaml`). No per-service drop panels — live investigation is Hubble UI's. The `flow`, `tcp`, `http`, and `dns` Hubble metrics stay off.

### Triage path

1. **Grafana `overview`** — is anything wrong, and which service or component?
2. **Service detail** — what symptom?
3. **Platform components**, **policy denials**, logs, traces — why?
4. **Hubble UI** — what talks to what, and is the network denying a flow right now?
5. **Headlamp** — what pods exist, and what do they say?

There is no signal overlap. Every application signal is Grafana's; the live flow map and drop inspection are Hubble UI's; drop history and alerting are Grafana's, because they need Prometheus retention the UI lacks.

## Consequences

### Positive

- One stack to operate and one place to look first; no second observability suite to patch, store for, or reconcile.
- Every dashboard is a git artifact — reviewable, reproducible, Argo-visible.
- Operators land on a verdict rather than a table.
- The map and the debug UI cost nothing extra: Hubble already runs for the audit surface, and Headlamp is one deployment on the existing ops-origin pattern.
- Read-only-by-default means no UI becomes a GitOps-bypassing control plane.

### Negative / Risks

- The map collapses multi-role workloads by name. Accepted as cosmetic.
- Hubble UI upstream is stagnant. Accepted, with the exception recorded above.
- Idle event-driven services show blank RED panels until traffic arrives; kubeletstats rows are always present.
- `view` blocks secrets, exec, and port-forward, but log tailing still exposes any sensitive value an application logs, and pod specs and ConfigMaps expose inline env literals. Gated behind operator plus AAL2.
- Headlamp is one more always-on component to patch in every environment. Accepted as Core operational surface; a project removes it with `enabled: false` in the env overlay.

## Rules

- Application observability — overview, SLO/RED, resources, logs, traces, profiling — is owned by the Grafana stack, and dashboards live as JSON under `infra/observability/dashboards/`. `(review-only)`
- Dashboards occupy the L1 → L1.5 → L2 → L3 funnel; `overview.json` is the Grafana home dashboard and every one of its panels either turns red or links down a level. `(review-only)`
- A question that fits no level in the funnel is a Grafana Explore query, not a committed dashboard. `(review-only)`
- The service map is the bundled Hubble UI at `hubble.ops.<host>` behind the ops forward-auth. No separate APM suite is deployed. `(review-only)`
- No Grafana service-map dashboard is built from Tempo `service_graphs` or the Hubble `flow` metric; both fail structurally. Re-adding one requires revisiting this ADR. `(review-only)`
- The Hubble `drop` metric stays enabled for history and alerting; the `flow`, `tcp`, `http`, and `dns` metrics stay off. `(review-only)`
- The Kubernetes debug UI is Headlamp, a Core component deployed via Helm and Argo CD in every environment at `headlamp.ops.<host>` behind the ops forward-auth. `(review-only)`
- Headlamp runs with a read-only ClusterRole. It is a debugging surface, not a control surface; desired-state changes go through git and Argo CD, never a UI. `(review-only)`

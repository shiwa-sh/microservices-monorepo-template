# Adoption Path — Reducing and Growing the Floor

[`operational-surface.md`](operational-surface.md) is the inventory: what the template runs, and the budget rule governing what may join it. This document runs in the opposite direction. It answers **what a project gives up, in what order, and what it costs to take back.**

The Core floor is a **position on a path, not the origin of one.** A project arriving with less capacity than the floor demands is not disqualified from the template; it enters below the floor and records which rungs it skipped. A project that outgrows the floor moves up into the Scale tier. Both directions use the same machinery, and this document holds both ends of it.

## Three ways to get smaller, and they are not interchangeable

| Move | What changes | What is kept | Where it lives |
| --- | --- | --- | --- |
| **Defer a capability** | the function is absent | the seam that lets it return as a chart | Band 1, below |
| **Concede the operator** | someone else runs it; axis B moves down | the capability, entire | Band 2, below |
| **Concede the capability** | the function goes, and the application is shaped by its absence | nothing — this is a bet | Band 3, below |
| **Defer scale** | the thin variant runs until a measured signal | the seam to the heavier one | [`operational-surface.md`](operational-surface.md), *Scale* |

Only the first three reduce the floor. The fourth is the growth direction, and it lives in the inventory beside the components it applies to — but it obeys the same rule as the three above: [`operational-surface.md`](operational-surface.md)'s Scale table records **cost to reverse** per row, because a swap that is cheap to adopt is not automatically cheap to abandon.

## The ordering rule

The instinct is to rank by savings — drop the heaviest component first. That is wrong here, because cost-to-reverse does not correlate with cost-to-run, and a reduction taken in the wrong order is paid for twice.

> **A removal with a seam is a deferral. A removal without a seam is a bet.**

That is [`operational-surface.md`](operational-surface.md)'s Scale-tier rule read backwards, and it yields the ordering:

1. **Take every deferral before any bet.** Band 1 is exhausted first, in full, regardless of how little each row saves.
2. **Prefer conceding the operator over conceding the capability.** Band 2 costs money and vendor coupling. Band 3 costs a rewrite, later, under pressure.
3. **Never buy capacity with a bet that a deferral would have bought.** If Band 1 and Band 2 together do not close the gap, the position on axis B is wrong — see [ADR-0000](adr/0000-platform-foundations.md), *Moving down axis B* — and that is a decision to take deliberately rather than a floor to keep shaving.

Every row below carries a **restore trigger**: the observable condition under which the thing comes back. A reduction without one is not a reduction, it is an omission.

## Band 1 — deferrals below the floor

The seam exists. Each row is removed by not deploying a chart, and restored by deploying it. Application code is unchanged in both directions, which is what makes these safe to take first and safe to take all of.

| Removed | What covers the concern instead | Restore trigger | Cost to reverse |
| --- | --- | --- | --- |
| **Hubble UI** | `hubble observe` on the CLI — the agent and relay run either way as the audit surface for default-deny ([ADR-0206](adr/0206-cluster-networking.md)) | flow debugging becomes routine rather than incidental, or a non-author needs to read drop verdicts | a values field ([ADR-0501](adr/0501-operator-uis-and-dashboards.md)) |
| **pgweb** | `psql` over a port-forward against the same read-only role | break-glass DB inspection happens under time pressure often enough that the port-forward is the delay | a chart ([ADR-0401](adr/0401-internal-admin.md)) |
| **Headlamp** | `kubectl`, which every operator of this platform must have anyway | someone who needs to read cluster state is not a `kubectl` user | a chart ([ADR-0501](adr/0501-operator-uis-and-dashboards.md)) |
| **Pyroscope + Alloy** | metrics and traces for *what* is slow; no answer for *where* in the code | a CPU or memory regression that metrics locate to a service but not to a function, twice | two charts — services already expose `pprof`, so nothing is re-instrumented ([ADR-0500](adr/0500-observability.md)) |
| **Lowdefy admin** | the Go API directly, plus whatever DB inspector survives above | an operational task is performed by someone who does not write API calls, or a support workflow becomes recurring | a chart and its YAML pages ([ADR-0401](adr/0401-internal-admin.md)) |
| **Tempo** | logs correlated by trace ID, which is present in the log line either way | a fault crosses more than two services and log correlation stops being sufficient | a chart and one collector exporter — OpenTelemetry instrumentation is unchanged, which is the point of [ADR-0500](adr/0500-observability.md)'s OTel-first rule |

**Take this band top to bottom.** The upper rows lose a convenience; the lower rows lose a diagnostic. A project at axis C high that has taken every row in this band has meaningfully lengthened its time-to-diagnose, and should say so in the same place it records its detection posture.

**Lowdefy and pgweb interact.** Dropping both leaves no non-`psql` path to the data at all. That is a legitimate position for a team where everyone is an engineer, and an untenable one the moment support work is done by anyone else.

## Band 2 — conceding the operator

The capability is unchanged; axis B moves down. Nothing in the architecture moves: the ADR set, OpenAPI contracts and codegen, the service template, the Helm and GitOps trees, policy-as-admission, repo layout, release and versioning, and the local dev loop are all unaffected by a managed swap.

Ranked by **capacity returned per unit of sovereignty conceded**, rather than by ease:

| # | Self-hosted here | Managed equivalent | Why this rank |
| --- | --- | --- | --- |
| 1 | Outbound email (maddy) | Any transactional email provider | Deliverability is reputational, not technical — the one cost engineering effort cannot retire ([ADR-0307](adr/0307-outbound-email.md)). Concede this first even at high sovereignty |
| 2 | Alert routing and on-call (Alertmanager) | A hosted paging service | No mature self-hosted escalation layer exists; the alternative is a rota and a phone ([ADR-0502](adr/0502-alerting-and-on-call.md)) |
| 3 | PostgreSQL (CNPG) | Managed Postgres | Highest operational risk per unit of engineering time. Backups, PITR, failover, and major upgrades all become someone else's rota |
| 4 | Object storage (SeaweedFS) | Any S3-compatible service with Object Lock | Retires a stateful production component and the off-cluster host it runs on. Object Lock is a requirement of the row it replaces ([ADR-0200](adr/0200-cluster-topology.md)), not an optional extra, and non-prod keeps the in-cluster instance so the local loop stays offline |
| 5 | Observability (the Grafana stack) | A hosted observability backend | A backend family to one vendor. OpenTelemetry instrumentation is unchanged ([ADR-0500](adr/0500-observability.md)) |
| 6 | Temporal | Temporal Cloud | Workflow code is identical; only the connection target changes ([ADR-0302](adr/0302-temporal.md)) |
| 7 | Forge and CI (Forgejo) | A hosted forge | Cheap to move either way, because `mise run ci:*` keeps workflows thin ([ADR-0102](adr/0102-source-control-and-ci.md)) |
| 8 | Image registry (zot) | A hosted registry | Verify cosign policy and scanning parity before swapping ([ADR-0105](adr/0105-image-registry.md)) |
| 9 | Identity (Ory) | A hosted IdP | Late, not first: [ADR-0304](adr/0304-identity-and-authorization.md)'s headless requirement narrows the field, and identity data is the most painful to migrate twice |
| 10 | Kubernetes (Talos) | Managed Kubernetes | Keep the API, drop the substrate. Removes [ADR-0200](adr/0200-cluster-topology.md)'s node provisioning entirely |

A **hybrid** position is legitimate and common: self-host the orchestrator and the stateless platform, take managed Postgres, object storage, email, and paging. That removes most of the pager burden while keeping the parts whose sovereignty usually motivated the choice.

Reversal is a migration per component in either direction, so each row is chosen deliberately. Rows 1, 2, 7, and 8 are close to free to reverse; rows 3 and 9 carry a data migration and are the ones to think hardest about.

## Band 3 — conceding the capability

These are **bets**. There is no seam: the application is written differently in their absence, and restoring the component later means changing code that was shaped by not having it. Each row states what is being wagered.

| Removed | What the application does instead | Cost if the bet is wrong | Cost to reverse |
| --- | --- | --- | --- |
| **Temporal** | an outbox table and a small worker per job, with retries and compensation written by hand | every multi-step process that acquires a second call, a compensation, or a durability guarantee grows a bespoke state machine, and each is a separate correctness surface at axis C high | a rewrite of every such process, plus the platform components. [`operational-surface.md`](operational-surface.md) already names outbox-plus-worker as the pre-Temporal shape for a *single trivial job*; this row generalises it to the whole platform, which is where it stops being cheap |
| **OpenFGA** | Postgres RLS ([ADR-0300](adr/0300-data.md)) plus role claims from the session | relationship-shaped rules — sharing, delegation, nested org structures — become bespoke queries spread across services, and the authorization model stops being inspectable in one place | re-model the authorization domain and migrate every relationship into tuples. Cheap while the model stays flat, and it never stays flat |
| **Kyverno** | CI-only checks: signature verification and digest pinning are enforced before merge, not at admission | every `(enforced: Kyverno)` annotation in the ADR set becomes an unenforced convention. Anything reaching the cluster by another path — a manual `kubectl apply`, a break-glass action, a compromised pipeline — is unchecked ([ADR-0104](adr/0104-supply-chain-security.md)) | a chart, and an audit of what was admitted meanwhile. The component is cheap to restore; the trust gap it leaves is not recoverable retroactively |
| **Cilium's default-deny and WireGuard posture** | a simpler CNI with no network policy and no east-west encryption | lateral movement is unbounded inside the cluster, and the multi-tenant isolation argument rests on RLS alone | **the highest in this document.** [ADR-0200](adr/0200-cluster-topology.md) records that CNI cannot be hot-swapped on a live cluster — the security posture is set at bootstrap. Reversing this is a cluster rebuild and a migration |

### The line

**A project that takes rows from this band is no longer at A-high / B-maximal / C-high.** It has changed its position on axes A or C, not merely its operating budget — and the machinery in this repository is calibrated for the position it left.

That is a legitimate place to be. It is not a legitimate place to *drift* into. A project taking any Band 3 row records it in **an ADR of its own**, stating the new position on the axes and superseding the affected decisions explicitly, in the same way [ADR-0000](adr/0000-platform-foundations.md) requires for a move down axis B. Deleting a component and leaving the ADR that mandates it in place produces a documented system nobody is running, which is the failure this repository's conventions exist to prevent.

If more than one row here looks necessary, the honest reading is that this template is not the right starting point for that project, and a smaller one will cost less than this one held together by exceptions.

## Irreducible

**This section is about functions, not components.** Every row below has a perfectly good managed form, and most of them are already ranked in Band 2. What is irreducible is that *something* performs the function — not that this repository's choice of component performs it, and emphatically not that it is self-hosted.

Read a row as: *you may stop operating this; you may not stop having it.*

| Function | Self-hosted here | Managed form | Why the function does not reduce |
| --- | --- | --- | --- |
| An edge with TLS | Traefik + cert-manager | a cloud load balancer with managed certificates | Traffic has to arrive somewhere, terminated. `(ref: RFC 8555)` |
| A relational store | PostgreSQL on CNPG | any managed Postgres — **Band 2, rank 3** | Every stateful component here is a Postgres tenant ([ADR-0300](adr/0300-data.md)) |
| Authentication | Kratos + Oathkeeper | a hosted IdP — **Band 2, rank 9**, and the field is narrow because [ADR-0304](adr/0304-identity-and-authorization.md) requires headless | The edge identity contract is what every service consumes ([ADR-0305](adr/0305-edge-auth-and-traffic-policy.md)) |
| Reconciliation from git | Argo CD | a hosted GitOps control plane | Principle 1. Conceding the *tool* is possible; conceding reconciliation means cluster state stops being derivable from the repository ([ADR-0201](adr/0201-gitops.md)) |
| Secret encryption at rest in git | SOPS + operator | a cloud KMS as the SOPS key backend — the file format and the workflow are unchanged | Without it, secrets leave the repository and the deploy stops being reproducible ([ADR-0202](adr/0202-secrets.md)) |
| Metrics, logs, and one place to read them | Prometheus + Loki + Grafana + OTel Collector | a hosted observability backend — **Band 2, rank 5** | Below this there is no detection at all. Band 1 removes the *third* and *fourth* signals; it does not touch the first two |
| A container runtime with an API | Kubernetes, shipped by Talos | any managed Kubernetes — **Band 2, rank 10** | The deploy target of every artefact ([ADR-0200](adr/0200-cluster-topology.md)) |
| An image registry | zot | GHCR, ECR, Artifact Registry, or any OCI registry — **Band 2, rank 8**. Verify that it stores referrers, since signatures and attestations live beside the image ([ADR-0105](adr/0105-image-registry.md)) | Pods pull from somewhere |
| A forge | Forgejo | GitHub, GitLab, or any hosted forge — **Band 2, rank 7**. Workflows are a portable subset of `mise run` calls precisely so this stays cheap ([ADR-0102](adr/0102-source-control-and-ci.md)) | Principle 1 again: the repository is the source of truth, so it has to be hosted |
| An outbound **transactional** mail path | maddy | a transactional provider — Postmark, SES, Mailgun — **Band 2, rank 1**, the first thing to concede at any sovereignty level | Identity verification and recovery have no production path without one ([ADR-0307](adr/0307-outbound-email.md)) |

**A note on mail, because the substitution is easy to get wrong.** Google Workspace, Microsoft 365, and their peers sell **human mailboxes**, which [ADR-0307](adr/0307-outbound-email.md) treats as a separate system from transactional sending — a different egress IP, a different DKIM selector, and deliberately never the same sender. Buying mailboxes does not replace maddy, and routing platform mail through a mailbox suite forfeits the reputation separation the ADR is built on. The managed form of *this row* is a transactional provider. Both can be bought, and they are two purchases.

**No row here is a reason to stay self-hosted.** If capacity is the binding constraint, Band 2 is the intended response and the whole of it is available.

## Hardenings a project takes on its own

The reverse of a reduction: positions the template holds at a deliberate default, where a project's own risk profile may justify paying more than the floor does. Each is additive, so none of them moves a band.

| Hardening | The default, and why | Take it when |
| --- | --- | --- |
| **Ops tooling on its own registrable domain** | One session cookie is scoped to the parent host, so it reaches the apex and every `*.ops.<host>` origin. Tier isolation rests on per-tool authorization and a second factor, not on cookie scope, which means a product-origin XSS can ride an operator's session into the ops tools ([ADR-0306](adr/0306-trust-tiers-and-urls.md)) | The product surface renders content one user supplies to another, or anything not first-party is hosted under the apex. Costs a second DNS zone and a second certificate chain per environment, and it is the strongest available answer |
| **An ops-tier OIDC session** | The same risk, at lower cost: one auth proxy in front of the ops tier minting its own scoped session, keeping the product cookie host-only on the apex | The separate domain is not worth its DNS and certificate cost, but the shared token is still the risk you want gone. Adds one component to the floor |
| **Per-workload certificate identity** | Services trust `X-User-Id` because default-deny guarantees only sanctioned callers reach the port. Code inside any sanctioned caller can forge it ([ADR-0305](adr/0305-edge-auth-and-traffic-policy.md)) | A service performs a monetary mutation, or a second team owns a service in the cluster. Cilium mutual auth and SPIFFE, no sidecars |
| **A paging receiver** | Alerts route to email and an unwired webhook. Nothing pages, so overnight detection is next working day ([ADR-0502](adr/0502-alerting-and-on-call.md)) | The project states an availability objective tighter than next-working-day. The objective and the receiver are one decision, not two |

**A hardening taken is a hardening recorded.** Each of these changes what an ADR states is true of the platform, so it amends the owning ADR's Rules in the project's copy — the same obligation as a Band 3 concession, in the opposite direction.

## Growing back

Every restore trigger above points upward, and the same seam machinery continues past the floor into [`operational-surface.md`](operational-surface.md)'s **Scale** tier, where each row carries a trigger to adopt and a cost to reverse in the same shape used here.

**The ladder is not equally reversible at every rung**, and the asymmetry does not follow the direction of travel. Going up, ClickHouse and Longhorn are the expensive rungs to undo. Going down, Cilium's posture is the expensive one, because the CNI is set at bootstrap. Everything else in both directions is a chart or a values change. The full ladder, from smallest to largest:

| Position | What it is |
| --- | --- |
| Band 3 taken | a different platform, at a different point on the axes, recorded in its own ADR |
| Bands 1–2 taken | the floor's capabilities, thinner and partly operated by others |
| Band 1 taken | the floor without its convenience and third-signal surfaces |
| **The Core floor** | what this template ships |
| Scale rows swapped in | the floor's heavier variants, each on a measured trigger |

There is no rung on this ladder that is a *maturity level*, and moving up is not progress. [ADR-0000](adr/0000-platform-foundations.md) makes that point about the axes and it holds here: a position is a more expensive answer to a pressure a system may not have.

## How to use this

1. Read [`operational-surface.md`](operational-surface.md)'s Core table, including the **recurring obligation** and **failure needs** columns. That is the demand side, and it is the same for every adopter.
2. Set it against your own coverage obligations, existing operational skill, and tolerance for detection latency. The gap between those is the risk ([ADR-0000](adr/0000-platform-foundations.md), *Capacity is a separate question from need*).
3. If a gap exists, close it with Band 1 in full, then Band 2 in order.
4. If a gap remains after both, do not proceed into Band 3 to close it. Re-read *Moving down axis B*: the position, not the floor, is what is mispriced.

Each row taken removes its obligation from the demand side. **That is the only honest way to lower the capacity this platform requires** — the alternative, holding the floor and hoping, is the failure mode [ADR-0000](adr/0000-platform-foundations.md) names.

## Where a reduction is written down

Every reduction is recorded in the project's own repository beside the restore trigger it is watching. A reduction nobody wrote down is indistinguishable from a component that was forgotten. What differs is the weight of the record, and it differs by band:

| Band | Recorded as | Why that weight |
| --- | --- | --- |
| **1 — deferral** | a values change, plus a row in the project's own register of what is deferred and what would restore it | The seam is untouched and no ADR stops being true. The chart is off, and the record is the trigger |
| **2 — managed swap** | a values change, plus an amendment to the owning ADR's *Rules* naming the managed operator | The capability is unchanged and the operator is not. The ADR states who runs it, so the ADR is what stops being true — an amendment, not a new document |
| **3 — capability concession** | an ADR of its own, stating the new position on the axes and superseding the affected decisions explicitly | The application is shaped by the absence. That is a different system, and it needs the argument written down |

**Band 2 is the row that decides whether this path stays cheap.** An amendment is a paragraph; treating it as a full re-litigation is what makes teams skip the record and drift instead. Name the managed service, mark the rule it changes, and keep the trigger that would bring it back in-house.

## A worked example

One project, walked end to end, because the doctrine above is complete and abstract, and a first adopter reasons by analogy whether or not an analogy is supplied.

**The project.** A B2B product handling money. Axis A high — three teams shipping independently. Axis C high — a wrong answer is a transaction. Axis B is where it does not match: self-hosting is preferred rather than binding, and platform work is a real part of several jobs rather than the whole of anyone's.

**Step 1 — the demand side.** Reading [`operational-surface.md`](operational-surface.md)'s Core table against their own coverage, three rows are immediately unaffordable: CNPG's *someone who has performed a failover and a PITR before*, maddy's deliverability work, and Alertmanager's *nothing pages anyone* against a product that moves money.

**Step 2 — Band 1, in full.** They take every row: no Hubble UI, no pgweb, no Headlamp, no Pyroscope or Alloy, no Lowdefy, no Tempo. Each is a chart not deployed, application code unchanged. They record that time-to-diagnose is now longer and that the restore trigger for Tempo — a fault crossing more than two services — is the one they expect to fire first.

**Step 3 — Band 2, in order.** Rank 1, maddy to a transactional provider: the deliverability obligation leaves entirely. Rank 2, a hosted paging service, which is also [ADR-0502](adr/0502-alerting-and-on-call.md)'s escalation trigger firing — at axis C high with money in flight, next-working-day detection is not a posture they can hold ([`reference/detection-latency.md`](reference/detection-latency.md)). Rank 3, managed Postgres: the largest capacity return in the document, and the row that removes the highest-stakes obligation on the floor.

**Step 4 — stop.** Three Band 2 rows close the gap, so Band 3 is never opened. Axis B has moved from maximal to hybrid, which is a deliberate position rather than a drift, and the architecture is untouched: the ADRs, the contracts, the service template, the Helm and GitOps trees, and the local loop are all unchanged.

**What they now carry.** Three rules amended to name a managed operator, six deferral triggers to watch, one paging subscription, and a written record that their SLO is defensible because a receiver exists to defend it. What they no longer carry is the pretence that part-time operators could hold the full floor — which is the outcome this document exists to produce.

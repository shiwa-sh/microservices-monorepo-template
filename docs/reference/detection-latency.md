# Detection Latency

Every decision below is defensible on its own, and no ADR composes them. This document does: per failure class, what detects it, and how long that takes. It is the number an availability objective is bounded by ([ADR-0500](../adr/0500-observability.md)) and the number [ADR-0502](../adr/0502-alerting-and-on-call.md)'s escalation trigger buys down.

**Detection is not response.** Every row states time-to-notice. What happens next is [`incident-management`](../guide/incident-management.md).

## The four positions that set these numbers

| Position | Effect on detection | Owning decision |
| --- | --- | --- |
| Nothing pages | out-of-hours detection is next working day, for every class | [ADR-0502](../adr/0502-alerting-and-on-call.md) |
| Per-PR e2e is label-gated, full e2e is nightly | a cross-service regression can sit in `master` until the nightly run | [ADR-0601](../adr/0601-testing-strategy.md) |
| Production platform runs `selfHeal=false` | drift persists until a human reads the notification | [ADR-0201](../adr/0201-gitops.md) |
| Backup restore is rehearsed quarterly | a backup that cannot be restored is discovered at the rehearsal, or at the restore | [ADR-0207](../adr/0207-cluster-storage.md) |

## Per failure class

**In hours** assumes someone is reading alert mail and dashboards. **Out of hours** assumes nobody is, because nothing rings.

| Failure | What detects it | In hours | Out of hours |
| --- | --- | --- | --- |
| Pod crashloop, one service | Alertmanager rule → email | minutes | next working day |
| Node lost | Alertmanager rule → email | minutes | next working day |
| Error-rate spike in production | `errors_total` rate alert ([ADR-0503](../adr/0503-error-tracking.md)) → email | minutes | next working day |
| Latency regression inside the SLO | the burn-rate rule at `ticket` severity ([ADR-0502](../adr/0502-alerting-and-on-call.md)) | hours | next working day |
| Cross-service regression merged to `master` | the nightly full e2e suite, unless the PR carried the e2e label | **up to a day** | **up to a day** |
| Operator dashboard broken | the nightly suite only — no per-PR coverage | **up to a day** | **up to a day** |
| Cluster drift from the repository | Argo CD notification, since production does not self-heal | hours | next working day |
| Certificate approaching expiry | cert-manager renewal failure alert | minutes to hours | next working day, and the renewal window is weeks — the slowest failure to become urgent |
| Backup failing | the backup job's own alert, and the quarterly restore rehearsal for a backup that *runs* and cannot be restored | minutes for a failed job; **up to a quarter** for an unrestorable one | same |
| Silent data corruption | nothing systematic. It surfaces as an application fault or at a restore | **unbounded** | **unbounded** |
| Alertmanager itself down, cluster up | the `alertmanager-watchdog-check` CronJob asks Alertmanager for a firing `Watchdog` every five minutes and fails when there is none ([ADR-0502](../adr/0502-alerting-and-on-call.md)) | within five minutes | until the failed job is noticed — next working day |
| Alertmanager down with the cluster | nothing. The check runs in the cluster it watches, so it stops with it | **unbounded** | **unbounded** |
| A `marketing.*` event reaching the log store | the standing routing assertion in the e2e suite ([ADR-0700](../adr/0700-analytics.md)) | at the next suite run | same |
| Recovery objectives missed | the quarterly restore rehearsal measures RTO and RPO against [ADR-0200](../adr/0200-cluster-topology.md)'s stated values | **up to a quarter** | same |

## Reading the table

**Two rows are unbounded, and they are the ones to change first.** An unbounded row is not a long detection time; it is the absence of detection, and the fault is found by its consequence rather than by a signal. Silent data corruption has no systematic detector at any budget this platform accepts.

Alertmanager's own failure splits in two, and the split is the honest part. The common shape — routing broken while the cluster runs — is now detected, because the `Watchdog` has a consumer that does not travel through the pipeline it checks. The other shape is not, because that consumer runs in the cluster it watches and goes quiet with it. Closing it needs something outside the cluster, which is the paging concession [ADR-0502](../adr/0502-alerting-and-on-call.md) defers.

**This table is also the RTO bound.** [ADR-0200](../adr/0200-cluster-topology.md) states recovery in under 30 minutes measured from the start of recovery. Out of hours, the row that detects the failure is added to that figure before a user sees service restored, which is why the recovery objective and the detection objective are stated separately rather than summed into one number that would be wrong half the day.

**The dominant term is not the alerting stack.** Rules evaluate in seconds and route in seconds. What sets every row whose out-of-hours figure is *next working day* is that the receiver reaches an inbox rather than a person, so the honest summary is: **this platform detects at the speed of someone looking.**

**The two day-long rows are a test-scheduling choice**, not an observability one. They are bought down by running e2e on the pull requests that touch more than one service, which affected-detection already identifies, rather than by any new component.

**An availability objective is bounded by this table.** A target that tolerates less downtime than the detection time in the row that will break first is a target nothing here can keep. Raising it is [ADR-0502](../adr/0502-alerting-and-on-call.md)'s escalation trigger, and its price is a hosted paging service and a rota.

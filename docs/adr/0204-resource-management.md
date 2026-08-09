# ADR-0204: Resource Management & Scheduling

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0104](0104-supply-chain-security.md), [ADR-0200](0200-cluster-topology.md), [ADR-0201](0201-gitops.md), [ADR-0302](0302-temporal.md), [ADR-0500](0500-observability.md), [ADR-0601](0601-testing-strategy.md)

## Context

Application services, stateful platform components, and batch Jobs share the same node set ([ADR-0200](0200-cluster-topology.md)). Without resource governance one noisy service starves Temporal, Postgres, or the observability stack, and the scheduler has no basis for eviction order.

Against the platform-engineering budget in [ADR-0000](0000-platform-foundations.md), nobody can babysit per-pod tuning. The policy must be defaulted rather than per-service.

## Decision drivers

1. **No workload can starve a Core component** ([`docs/operational-surface.md`](../operational-surface.md)).
2. **Defaults over per-service tuning.** The policy applies to every namespace without per-service work.
3. **Predictable eviction order** under pressure.
4. **Boring in-tree Kubernetes primitives** ([ADR-0000](0000-platform-foundations.md), principle 5).

## Considered options

### Guardrails

| Option | Applies without per-service work | Bypassable | Added components | Verdict |
| --- | --- | --- | --- | --- |
| **`LimitRange` + `ResourceQuota` + `PriorityClass` + `PodDisruptionBudget`** | yes — namespace-scoped | no, they are API-server objects | **none — in-tree** | **Chosen.** Four primitives the API server already enforces |
| Per-service requests only, reviewed by hand | no | trivially, by omission | none | The omission is the failure mode, and review does not catch what is absent |
| A mutating admission policy that stamps defaults | yes | no | an admission controller | It reaches one case the chosen option cannot — a chart with no field to set — and buying a controller for defaulting alone inverts driver 4. Where [ADR-0104](0104-supply-chain-security.md) brings Kyverno for signature verification, that case is covered as a by-product |
| Vertical Pod Autoscaler | yes | no | one controller | Rewrites requests from observed usage, which conflicts with values derived from a recorded measurement and reviewed in git |

### CPU limits

| Option | Behaviour under contention | Failure mode | Verdict |
| --- | --- | --- | --- |
| **Requests and memory limits, no CPU limit** | CPU is shared in proportion to requests, and a burst may use idle capacity | a busy neighbour slows a burst | **Chosen.** Memory is incompressible, so its limit is the only thing between one leak and a node-wide OOM. CPU is compressible, so a limit buys nothing the requests do not already give |
| A CPU limit on every container | CFS throttles at the quota even when the node is idle | a burst becomes tail latency, which reads as a code defect | Measured: `temporal-history` bursts to 2121m from near-idle, so a plausible-looking 500m limit quarters it and surfaces only as slow checkouts |
| CPU limits equal to requests | no bursting at all; Guaranteed QoS class | capacity sits idle while a pod throttles | Buys eviction-order predictability that the `PriorityClass` tiers already provide |
| No limits of any kind | nothing bounds a leak | one leak takes the node | The asymmetry above is the whole point: memory needs the limit, CPU does not |

### Autoscaling

| Option | Scales on | Added components | Verdict |
| --- | --- | --- | --- |
| **No HPA by default, opt-in per service** | a documented sustained-load signal | none | **Chosen.** The one place driver 2 cuts the other way: a default that guesses is worse than an explicit opt-in |
| An HPA on CPU for every service | CPU utilisation against request | none — in-tree | With no CPU limit the utilisation ratio is a weak signal, and a service with no load history oscillates against it |
| Vertical Pod Autoscaler | observed usage, written back into requests | one controller | As above: it overwrites reviewed, measured values |
| VPA in recommendation-only mode | the same, as advice | one controller and a dashboard | The advice is what the [ADR-0601](0601-testing-strategy.md) load suite already produces, into a dashboard that already exists |
| KEDA | queue depth and external metrics | one controller and its CRDs | The right answer for event-driven scaling, and nothing here scales on a queue — Temporal workers pull their own work |

## Decision

Every container declares resources, and each namespace carries guardrails.

| Guardrail | Role |
| --- | --- |
| **Requests and memory limits, mandatory** | Every container sets CPU and memory requests and a memory limit. A container with neither its own values nor a namespace default is a defect |
| **`LimitRange` per namespace** | Supplies default requests and limits so a missing value fails safe rather than scheduling unbounded |
| **`ResourceQuota` per namespace** | Caps aggregate CPU and memory so one namespace cannot consume the cluster |
| **`PriorityClass` tiers** | `platform-critical` > `product` > `batch`. Under pressure, batch dies first and Core last |
| **`PodDisruptionBudget`** | On every multi-replica and stateful component, so a drain or upgrade never takes quorum below one |

**No container sets a CPU limit; every container sets a memory limit.** Contention is handled by requests plus the priority tiers.

**Autoscaling is opt-in per service**, on a documented sustained-load signal — the same grow-on-a-trigger discipline as [ADR-0200](0200-cluster-topology.md)'s node growth. It is not template default machinery.

### Ordering

All four guardrails live in `infra/helm/platform/resource-governance/`, ordered against each other by sync wave inside that chart — PriorityClass -5, LimitRange -4, ResourceQuota and PDB -3 — and placed in the **base** tier ([ADR-0201](0201-gitops.md)). A `LimitRange` defaults only the pods created after it, and a pod naming an absent `PriorityClass` is rejected outright.

### Sizings are measured, not estimated

Every number is taken from `cluster:full` under the [ADR-0601](0601-testing-strategy.md) load suite and is re-derivable from the `load-test` dashboard. The measurements that changed decisions:

| Workload | Measured peak | Consequence |
| --- | --- | --- |
| Tempo | **1858Mi**, about 400Mi at rest | The heaviest pod in the cluster. The 512Mi namespace default would have OOM-killed it during exactly the runs meant to observe it. Explicit 3Gi limit |
| temporal-history | **2121m CPU**, 647Mi | The strongest evidence for the no-CPU-limits rule: a 500m limit throttles it to a quarter of measured need |
| Oathkeeper / Traefik | 192m / 184m CPU | The edge costs roughly 3× catalog on the read path. Size the edge first |
| Loki | 453Mi | 88% of the namespace default. Explicit 1Gi |
| Postgres | 373Mi, 267m CPU | — |

### Memory sizing uses the working set

`k8s_pod_memory_limit_utilization_ratio` divides memory **usage**, which counts reclaimable page cache, by the limit. It overstates real pressure by two to three times across every pod measured here — `otel-cluster` reported 97.9% against a true 58.0%.

The OOM killer acts on the **working set**, so limits are sized against `k8s_pod_memory_working_set_bytes / k8s_container_memory_limit_bytes`, which is what the `load-test` dashboard computes.

### Deliberate gaps

Recorded so the documentation and the cluster agree.

| Gap | Reason |
| --- | --- |
| `kube-system` has a `LimitRange` and **no `ResourceQuota`** | A quota rejects pod creation, and in `kube-system` the rejected pods are the CNI DaemonSet, CoreDNS, and the ingress controller. The recourse for a rejected system pod is a working cluster. The line is drawn at objects that can only add defaults |
| **No `min` or `max` on any `LimitRange`** | Both reject. A `min.memory: 16Mi` floor would have rejected Cilium's `install-cni-binaries` initContainer, which legitimately requests 10Mi, on every node. Caught by rendering every chart before applying |
| **Only one `PodDisruptionBudget` is enabled** | A PDB with `minAvailable: 1` in front of a single-replica workload permits zero disruptions and hangs `kubectl drain` forever. On a single-node tier only `postgres-pgbouncer` runs two replicas. The rest are declared and disabled in values, as the list a multi-node environment enables as it scales those components out |
| **Temporal pods cannot declare `priorityClassName`** | The upstream chart exposes per-role `resources` and no priority field, so they inherit the `globalDefault`. This is why `platform-critical` is the default rather than `product`: the workloads that *cannot* declare a priority are exactly the ones needing the high one. Workloads we control declare `product` explicitly to sit below the platform |
| **One CPU limit survives** | `temporal-worker-controller-manager` inherits `limits.cpu: 500m` from its subchart. Removing a nested subchart key from a parent values file is a [long-standing Helm coalescing limitation](https://github.com/helm/helm/issues/9136) rather than a supported operation. Accepted because 500m is about 80× the measured 6m peak of a leader-elected reconciler. Allow-listed with a reason, as is Cilium's init-container limit |

## Consequences

### Positive

- A noisy service cannot starve a Core component, and eviction order is deterministic.
- Guardrails are namespace-level defaults rather than per-service toil.
- Capacity pressure surfaces as quota rejections in CI-reviewed manifests rather than as OOMs at 3am.
- Every committed number traces to a recorded measurement, so re-deriving it after a traffic change is mechanical.

### Negative / Risks

- **Requests set too low still overcommit; too high waste capacity.** Mitigated by observability ([ADR-0500](0500-observability.md)) feeding right-sizing, and by the load suite producing the number.
- **`ResourceQuota` can block a deploy when a namespace is full.** Intended back-pressure, and it must be a legible failure rather than a silently pending pod. `lint:resource-governance` prints per-namespace utilisation on every CI run so the cap is approached knowingly.
- **A chart with no field for a value cannot be governed by a values file.** `platform-critical` is the `globalDefault` precisely so those charts inherit the priority they need, which is a workaround rather than a control: it defaults every un-annotated workload upward, including ones that should sit below the platform. Admission-time mutation ([ADR-0104](0104-supply-chain-security.md)) is the mechanism that replaces it.

## Rules

- Every container declares CPU and memory requests and a memory limit, or lands in a namespace whose `LimitRange` supplies them. A container with neither is a defect. `(CI: lint:resource-governance)`
- No container sets a CPU limit unless throttling is the intent. An inherited limit that cannot be removed is allow-listed with a reason. `(CI: lint:resource-governance)`
- A `LimitRange` may only add defaults, never reject: no `min`, no `max`. Rejecting a third-party pod on a floor we invented is a self-inflicted outage. `(CI: lint:resource-governance)`
- Every namespace we own has a `LimitRange` and a `ResourceQuota`. `kube-system` is the documented exception. `(CI: lint:resource-governance)`
- The summed requests and memory limits of a namespace fit inside its `ResourceQuota`, with utilisation reported on every run. `(CI: lint:resource-governance)`
- Workloads carry a `PriorityClass`. `platform-critical` is the `globalDefault`, so charts that cannot express a priority inherit the right one, and product workloads opt down explicitly.
- Every multi-replica or stateful component has a `PodDisruptionBudget` preserving quorum. A single-replica workload gets none.
- Requests and limits are derived from measurement ([ADR-0601](0601-testing-strategy.md)), and the measurement is recorded next to the value.
- Memory sizing uses the working set, never the limit-utilisation ratio.
- HPA is opt-in per service on a documented sustained-load signal, never a template default.

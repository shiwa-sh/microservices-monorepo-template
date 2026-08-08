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

| Option | Applies without per-service work | Bypassable | Verdict |
| --- | --- | --- | --- |
| **`LimitRange` + `ResourceQuota` + `PriorityClass` + `PodDisruptionBudget`** | yes — namespace-scoped | no, they are API-server objects | **Chosen.** Four in-tree primitives, no new component |
| Per-service requests only, reviewed by hand | no | trivially, by omission | The omission is the failure mode, and review does not catch what is absent |
| A mutating admission policy that stamps defaults | yes | no | Needs an admission controller ([ADR-0104](0104-supply-chain-security.md)) that is not deployed. It remains the right answer for charts that cannot express a field — see *Risks* |
| Vertical Pod Autoscaler | yes | no | Adds a component that rewrites requests from observed usage, which conflicts with values derived from a recorded measurement and reviewed in git |

## Decision

Every container declares resources, and each namespace carries guardrails.

| Guardrail | Role |
| --- | --- |
| **Requests and memory limits, mandatory** | Every container sets CPU and memory requests and a memory limit. A container with neither its own values nor a namespace default is a defect |
| **`LimitRange` per namespace** | Supplies default requests and limits so a missing value fails safe rather than scheduling unbounded |
| **`ResourceQuota` per namespace** | Caps aggregate CPU and memory so one namespace cannot consume the cluster |
| **`PriorityClass` tiers** | `platform-critical` > `product` > `batch`. Under pressure, batch dies first and Core last |
| **`PodDisruptionBudget`** | On every multi-replica and stateful component, so a drain or upgrade never takes quorum below one |

### CPU limits are not set; memory limits are

The asymmetry is deliberate and measured. Memory is incompressible, so a limit is the only thing between one leak and a node-wide OOM. A CPU limit throttles through CFS and converts a burst into tail latency.

`temporal-history` bursts to **2.1 cores** from near-idle. A plausible-looking 500m limit would have quartered it and surfaced only as slow checkouts. Contention is handled by requests plus the priority tiers.

### No HPA by default

Autoscaling is opt-in per service when a measured, sustained load signal justifies it — the same grow-on-a-trigger discipline as [ADR-0200](0200-cluster-topology.md)'s node growth. A service adds an HPA with a documented signal. It is not template default machinery.

### Ordering

All four guardrails live in `infra/helm/platform/resource-governance/`, ordered against each other by sync wave inside that chart — PriorityClass -5, LimitRange -4, ResourceQuota and PDB -3 — and placed in the **base** tier ([ADR-0201](0201-gitops.md)). A `LimitRange` only defaults pods created after it exists, and a pod naming a `PriorityClass` that does not yet exist is rejected outright.

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
| **One CPU limit survives** | `temporal-worker-controller-manager` inherits `limits.cpu: 500m` from its subchart, and a parent values file cannot delete it — Helm coalesces maps and skips parent nulls. Accepted because 500m is about 80× the measured 6m peak of a leader-elected reconciler. Allow-listed with a reason, as is Cilium's init-container limit |

## Consequences

### Positive

- A noisy service cannot starve a Core component, and eviction order is deterministic.
- Guardrails are namespace-level defaults rather than per-service toil.
- Capacity pressure surfaces as quota rejections in CI-reviewed manifests rather than as OOMs at 3am.
- Every committed number traces to a recorded measurement, so re-deriving it after a traffic change is mechanical.

### Negative / Risks

- **Requests set too low still overcommit; too high waste capacity.** Mitigated by observability ([ADR-0500](0500-observability.md)) feeding right-sizing, and by the load suite producing the number.
- **`ResourceQuota` can block a deploy when a namespace is full.** Intended back-pressure, and it must be a legible failure rather than a silently pending pod. `lint:resource-governance` prints per-namespace utilisation on every CI run so the cap is approached knowingly.
- **Kyverno is listed as a Core component in [`docs/operational-surface.md`](../operational-surface.md) and [ADR-0104](0104-supply-chain-security.md) but is not deployed**, and appears nowhere in `infra/`. That removes the admission-time mutation that would otherwise stamp `priorityClassName` onto charts which cannot express it. This ADR depends on the gap; closing it belongs to [ADR-0104](0104-supply-chain-security.md).

## Rules

- Every container declares CPU and memory requests and a memory limit, or lands in a namespace whose `LimitRange` supplies them. A container with neither is a defect. `(CI: lint:resource-governance)`
- No container sets a CPU limit unless throttling is genuinely wanted. An inherited limit that cannot be removed is allow-listed with a reason. `(CI: lint:resource-governance)`
- A `LimitRange` may only add defaults, never reject: no `min`, no `max`. Rejecting a third-party pod on a floor we invented is a self-inflicted outage. `(CI: lint:resource-governance)`
- Every namespace we own has a `LimitRange` and a `ResourceQuota`. `kube-system` is the documented exception. `(CI: lint:resource-governance)`
- The summed requests and memory limits of a namespace fit inside its `ResourceQuota`, with utilisation reported on every run. `(CI: lint:resource-governance)`
- Workloads carry a `PriorityClass`. `platform-critical` is the `globalDefault`, so charts that cannot express a priority inherit the right one, and product workloads opt down explicitly. `(review-only)`
- Every multi-replica or stateful component has a `PodDisruptionBudget` preserving quorum. A single-replica workload gets none. `(review-only)`
- Requests and limits are derived from measurement ([ADR-0601](0601-testing-strategy.md)), and the measurement is recorded next to the value. `(review-only)`
- Memory sizing uses the working set, never the limit-utilisation ratio. `(review-only)`
- HPA is opt-in per service on a documented sustained-load signal, never a template default. `(review-only)`

# Cost Model

Sovereignty is justified partly on **fee predictability** ([ADR-0000](../adr/0000-platform-foundations.md), principle 3), and nothing states what that claim means. This document does, without prices: a price list is wrong within a year and wrong for every adopter who is not the one who wrote it.

What the platform knows is the **shape of the bill** and what appears on it. What a project knows is its provider and its scale. The two together produce a number; neither produces one alone. This is the same division [`../operational-surface.md`](../operational-surface.md) uses for capacity, and for the same reason.

## The claim, stated precisely

The claim is not that self-hosting is cheaper. It is that its cost curve has a different shape, and that the shape is the thing being bought.

| | Self-hosted here | Managed equivalents |
| --- | --- | --- |
| **Floor** | high, and paid from day one — the platform runs before any service does | low, often near zero |
| **Slope against usage** | flat between node boundaries. Requests, tenants, and services add nothing until capacity does | tracks usage, and per-seat or per-service fees track the organisation |
| **Steps** | at node boundaries, and each step is a known quantity | none visible, which is the appeal and the exposure |
| **Predictability** | a change in the bill is a change made deliberately | a change in the bill can be a change in traffic, in a vendor's pricing, or in a tier's definition |
| **The bad surprise** | capacity runs out under load, which is an incident | a bill arrives that nobody decided, which is a negotiation |

**The trade is a known cost against an unknown one.** At axis B maximal, that trade is the point: a fixed floor is a number to plan against, and a usage-tracking bill is a number to react to. Where the floor is unaffordable, the answer is [`../adoption-path.md`](../adoption-path.md) rather than a cheaper interpretation of this document.

## What appears on the bill

The inventory to price against a provider. Multiply by environments — the template assumes dev, staging, and production ([ADR-0205](../adr/0205-environment-parity.md)) — except where a row says otherwise.

| Line item | Driven by | Notes |
| --- | --- | --- |
| Compute instances | node count per cluster | the control plane's minimum is set by etcd quorum, which is three, and that is a threshold rather than a price ([ADR-0200](../adr/0200-cluster-topology.md)) |
| Node disk | the working set of every stateful component, plus local volumes | volumes are node-local, so disk is sized per node rather than pooled ([ADR-0200](../adr/0200-cluster-topology.md)) |
| Load balancer | one per cluster | |
| Object storage capacity | backups, logs, traces, profiles, and images, each against its retention | the largest variable row, and the one retention policy controls directly ([ADR-0500](../adr/0500-observability.md)) |
| Object storage requests and egress | telemetry write rate, image pulls, restore reads | usage-tracking even here, and the row most often forgotten |
| Object storage, production | run outside the cluster | a host or a service, and not optional: it holds the backups a rebuild reads |
| Static IP for mail | one, dedicated, with a matching `PTR` | separate from the cluster's ingress address ([ADR-0307](../adr/0307-outbound-email.md)) |
| DNS zone | one per environment domain | |
| Certificates | none — ACME | a cost avoided, and the reason cert-manager is on the floor |
| Forge and CI runners | pipeline minutes become instance hours on infrastructure you own | self-hosted CI moves this from a per-minute fee to capacity ([ADR-0102](../adr/0102-source-control-and-ci.md)) |
| Backup storage and retention | the recovery objective, and Object Lock retention no shorter than it | |
| Paging service | the escalation concession, when the trigger fires | the one managed subscription the template anticipates ([ADR-0502](../adr/0502-alerting-and-on-call.md)) |

**What is not on it.** Every per-seat fee for the tools this floor replaces — the forge, the registry, the observability backend, the error tracker, the workflow engine, the identity provider. Those are the fees sovereignty retires, and they are the honest other side of the floor. A comparison that prices the floor and omits them is not a comparison.

## The row that is not infrastructure

**Operator attention is the scarce resource, and it is the largest line item.** [`../operational-surface.md`](../operational-surface.md) carries the recurring obligation and the failure-response requirement of every component; those columns are the labour half of this bill. A cost model that prices instances and omits them will make sovereignty look cheapest exactly where it is most expensive — a small team, few services, the same fixed floor.

The ratio worth computing is not cost per month. It is **platform work against product work**: at many services the ratio is comfortable, and at a handful it approaches one to one ([ADR-0000](../adr/0000-platform-foundations.md)). Fewer services makes the platform proportionally heavier, and no infrastructure line item shows that.

## Using this

1. Price the inventory above with your provider, per environment.
2. Add the retired per-seat fees as a credit, honestly — including the ones a smaller team would not have bought.
3. Sum the obligation columns in [`../operational-surface.md`](../operational-surface.md) against your own coverage hours.
4. Compare against [`../adoption-path.md`](../adoption-path.md) Band 2, which is ranked by **capacity returned per unit of sovereignty conceded** — the same currency as step 3 rather than step 1.

Steps 3 and 4 are where the answer usually is. A project that cannot afford the floor rarely cannot afford the instances.

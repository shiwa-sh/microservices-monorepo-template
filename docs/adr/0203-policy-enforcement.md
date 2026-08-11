# ADR-0203: Policy Enforcement Strategy

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0001](0001-documentation-and-output-conventions.md), [ADR-0101](0101-monorepo.md), [ADR-0104](0104-supply-chain-security.md), [ADR-0200](0200-cluster-topology.md), [ADR-0201](0201-gitops.md), [ADR-0202](0202-secrets.md), [ADR-0204](0204-resource-management.md), [ADR-0303](0303-api-contracts-and-lifecycle.md), [ADR-0304](0304-identity-and-authorization.md), [ADR-0305](0305-edge-auth-and-traffic-policy.md)
- **Decides:** Every invariant is enforced at the earliest layer the constrained party cannot bypass, one layer per class of invariant.

## Context

Four mechanisms enforce invariants on this platform: CI lints before merge, Pod Security Admission in the API server, Kyverno at admission, and CiliumNetworkPolicy in the datapath. Each is decided in the ADR that needed it, and each of those ADRs answers *how do I enforce this rule*. None answers *which layer is entitled to enforce which class of rule*.

That gap has two failure modes. An invariant lands in the wrong layer — a runtime property checked only before merge, so anything reaching the cluster by another path is unchecked. Or it lands in two layers, and the second copy drifts from the first until they disagree and neither is trusted.

This ADR decides the assignment and the rule that produces it. It adds no mechanism and re-decides none: what each mechanism does is owned by the ADR that introduced it.

## Decision drivers

1. **One enforcement point per invariant** ([ADR-0000](0000-platform-foundations.md), principle 5). A rule checked in two places is two rules that agree until they do not.
2. **Enforcement sits where the constrained party cannot route around it** ([ADR-0104](0104-supply-chain-security.md)). A gate that a `kubectl apply` bypasses does not constrain the path an incident takes.
3. **The gate's own failure is survivable and understood.** Every layer here fails differently, and a layer whose failure blocks all deploys is priced accordingly.
4. **No component is added for enforcement alone** ([ADR-0000](0000-platform-foundations.md), principle 2). An in-tree mechanism beats a controller expressing the same rule.

## Considered options

| Option | Reaches state not created by CI | Failure mode | Added components | Verdict |
| --- | --- | --- | --- | --- |
| **Layered by class of invariant, one layer each** | yes, for the classes that need it | per layer, and stated per layer below | none — every mechanism is already decided | **Chosen.** It is the only option that lets driver 2 and driver 3 be answered separately per rule instead of once for all rules *(reasoned)* |
| Everything in CI | **no** — a manual apply, a break-glass action, or a compromised pipeline is unchecked | fails open: an unrun check is a passed check | none | Sound for repository properties, unsound for cluster properties, and the distinction is exactly what needs deciding |
| Everything through one policy engine | yes | **one [webhook whose outage blocks every deploy](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/#failure-policy)**, including the deploy that fixes it | none beyond Kyverno | Concentrates driver 3's risk into a single component to buy uniformity nobody reads. It would also move rules the API server enforces in-tree onto a controller, against driver 4 |
| Per-ADR choice, unstated | inconsistently | unknown until an incident | none | The honest baseline: what exists without this document. It produces both failure modes in the Context above, and neither is visible until a rule is tested |

## Decision

**An invariant is enforced at the earliest layer that the constrained party cannot bypass.** Earliest, because feedback before merge is cheaper than feedback at deploy; cannot-bypass, because that is what makes it enforcement rather than advice. The two are in tension exactly once, and driver 2 wins that case.

| Class of invariant | Layer | Why this layer owns it |
| --- | --- | --- |
| Repository properties — layout, naming, generated-code drift, contract shape, prose | **CI lints** ([ADR-0101](0101-monorepo.md), [ADR-0303](0303-api-contracts-and-lifecycle.md)) | The subject is a file in a pull request. There is no cluster state to check and nothing to bypass: an unmerged branch cannot violate a rule about `master` |
| Workload privilege — capabilities, host namespaces, root, volume types | **Pod Security Admission** ([ADR-0200](0200-cluster-topology.md)) | In-tree, so driver 4 is decisive. Namespace labels with a pinned `enforce-version`, no component to run |
| Image provenance — signatures, attestations, digest pins | **Kyverno** ([ADR-0104](0104-supply-chain-security.md)) | The property is about the artefact a pod *runs*, which only the cluster can check at the moment it runs it. CI proves the image was signed; admission proves this pod uses one that was |
| Reachability — which workload may open a connection to which | **CiliumNetworkPolicy** ([ADR-0206](0206-cluster-networking.md)) | Enforced in the datapath, so it constrains a compromised pod rather than a cooperating one. Nothing above the network can express it |
| Resource governance — requests, limits, quota, eviction order | **In-tree API objects** ([ADR-0204](0204-resource-management.md)) | `LimitRange`, `ResourceQuota`, `PriorityClass`, and `PodDisruptionBudget` are enforced by the API server itself. Kyverno could express the same rules and would add a controller to the path for no reach it does not already have |
| Request authorization — who may call what | **The edge and the application** ([ADR-0305](0305-edge-auth-and-traffic-policy.md), [ADR-0304](0304-identity-and-authorization.md)) | The subject is a request, not an object, so no admission layer sees it |

**A rule appears in one row.** Where two layers could hold a rule, the table above decides it, and the losing layer does not carry a weaker copy "for feedback" — that copy is the drift this ADR exists to prevent.

**One rule is deliberately enforced twice, and it is the exception that shows the test.** Digest pinning is linted in CI and enforced at admission ([ADR-0104](0104-supply-chain-security.md)). The two checks have different subjects: the lint reads the values file a human edits, and admission reads the pod spec a chart rendered. Neither is a copy of the other, because a rendered pod spec can carry a floating tag no values file contains.

### Each layer fails differently, and the difference is the price

| Layer | On failure | Blast radius |
| --- | --- | --- |
| CI lints | **open** — an unrun check is a passed check | one merge, and everything downstream of it |
| Pod Security Admission | with the API server | nothing separable: if it is down, nothing is being admitted anyway |
| Kyverno | **closed, cluster-wide** — a failing webhook blocks admission for everything it matches | every deploy, including the one that would fix it. This is the largest single-component blast radius on the floor ([`operational-surface.md`](../operational-surface.md)), and the break-glass is [ADR-0202](0202-secrets.md)'s kubeconfig path plus removing the webhook configuration |
| CiliumNetworkPolicy | **last-known-good** — policy already programmed stays in the eBPF maps when the agent restarts | new pods do not get policy until the agent is back, so scheduling under an agent outage is what to avoid |

Kyverno's row is why driver 3 exists as a separate driver: it is the only layer here whose own outage is an incident on its own, and the reason its scope stays at image provenance rather than growing to whatever a policy engine can express.

### The annotation is the index

[ADR-0001](0001-documentation-and-output-conventions.md)'s Rules annotations name the layer: `(CI: <task>)` is a lint, `(enforced: <policy>)` is admission or datapath, `(ref: <standard>)` is an adopted external standard. An unannotated rule is enforced by review, which is a stated position rather than an omission — and a rule whose annotation names a layer the table above does not assign to its class is a defect in one of the two.

## Consequences

### Positive

- A reviewer reading a new rule has one question to answer — what class of invariant is this — and the layer follows.
- The four mechanisms stop being four independent policies and become one statement with four implementations.
- The blast-radius table makes the Kyverno concentration visible where it is decided, rather than discoverable during an outage.
- Scope creep in the policy engine has a stated argument against it, so refusing the next rule Kyverno could express costs a link rather than a debate.

### Negative / Risks

- **The layer assignment is review-enforced.** A rule can be written at the wrong layer and nothing rejects it; the annotation makes the mistake visible in a diff and does not prevent it.
- **Earliest-and-unbypassable is a rule with a genuine tension in it**, and it is resolved in favour of unbypassable every time. That costs feedback latency on the image-provenance class, where a violation is found at deploy rather than at merge.
- **CI lints fail open by construction.** Nothing here changes that; the class assigned to them is the class where failing open is acceptable because the subject never reaches the cluster.

## Rules

- Every invariant is enforced at exactly one layer, chosen by its class in the assignment table. A second copy at another layer is not added for feedback.
- Repository properties are enforced in CI, workload privilege by Pod Security Admission, image provenance by Kyverno, reachability by CiliumNetworkPolicy, and resource governance by in-tree API objects.
- Kyverno's scope is image provenance. Extending it to a class this ADR assigns elsewhere requires amending this ADR. `(enforced: Kyverno)`
- A rule carries the annotation of the layer that enforces it, and an unannotated rule is enforced by review. `(ref: ADR-0001)`
- No component is added whose only purpose is enforcing a rule an in-tree mechanism already enforces.

# The Build Path

[`detection-latency.md`](detection-latency.md) composes the run-time path: per failure class, how long a fault runs before anyone notices. This document composes the other one. **Per class of defect, what stops it between a keystroke and production, and what reaches production unchecked.**

Every gate below is decided in an ADR, and each ADR is complete about its own gate. What no ADR can state is the total, because the total is a property of the sequence rather than of any layer in it.

**A defect class with no row here is one nobody has classified**, which is the same failure the run-time table's unbounded rows name.

## The four positions that set these numbers

| Position | Effect on the merge path | Owning decision |
| --- | --- | --- |
| CI lints fail open by construction | an unrun check is a passed check, so every repository-property gate is advisory against a path that skips CI | [ADR-0203](../adr/0203-policy-enforcement.md) |
| Affected-detection selects what CI runs | a selection bug produces a green run against something it did not select | [ADR-0101](../adr/0101-monorepo.md), register row 8 |
| Per-PR e2e is label-gated; the full suite is nightly | a cross-service defect passes the merge gate and is caught by a scheduled run | [ADR-0601](../adr/0601-testing-strategy.md), register row 6 |
| Admission is the only layer a manual apply cannot route around | anything checked solely in CI is unenforced against the incident path | [ADR-0203](../adr/0203-policy-enforcement.md) |

## The path

A change passes through six positions. The first four are advisory in the strict sense — they constrain a cooperating author — and the last two constrain the cluster.

| # | Position | Constrains | Bypassed by |
| --- | --- | --- | --- |
| 1 | Local hooks | the author, before the push | not pushing through them |
| 2 | CI lints and codegen drift | the pull request | a CI outage, a skipped run, affected-detection not selecting the change |
| 3 | Tests — unit, integration, e2e | the pull request | the same, plus e2e being label-gated |
| 4 | Review | the pull request | approval |
| 5 | Argo CD reconciliation | what the cluster converges to | a manual `kubectl apply` |
| 6 | Admission — PSA and Kyverno | what the cluster runs | nothing available to a deploying party |

**Positions 1 to 4 share one bypass**, and it is not malice: a CI outage, a misconfigured path filter, or a change affected-detection did not select produces the same result as skipping them deliberately. That is why [ADR-0203](../adr/0203-policy-enforcement.md) assigns runtime properties to positions 5 and 6 rather than to 2.

## Per class of defect

**Caught by** names the earliest position that rejects the defect. **Reaches production if** names what has to be true for it to get past every position.

| Defect class | Caught by | Reaches production if | Residual |
| --- | --- | --- | --- |
| Layout, naming, prose, generated-code drift | 2 — CI lints ([ADR-0101](../adr/0101-monorepo.md), [ADR-0001](../adr/0001-documentation-and-output-conventions.md)) | CI does not run, or affected-detection does not select the change | **bounded.** The subject is a file. A repository property that reaches production is a cosmetic defect, which is why this class is assigned to a layer that fails open |
| Contract shape — spec validity, audience, breaking change | 2 — vacuum and oasdiff ([ADR-0303](../adr/0303-api-contracts-and-lifecycle.md)) | as above | **bounded.** The generated client is committed and drift-checked, so a spec change that skipped the gate still has to pass the drift check to merge |
| A single service's logic | 3 — unit and integration tests | the service was not selected by affected-detection | **register row 8.** This is the class that row is about, and its reach is one merge plus everything downstream |
| A cross-service regression | 3 — e2e, **label-gated per PR**, nightly in full | the pull request carried no e2e label | **register row 6, up to a day.** The largest routine hole in this table, and the one bought down by a policy change rather than a component |
| An operator dashboard broken | 3 — the nightly suite only | always, until the nightly run | **up to a day**, and it is a day of exposure on the surface used *during* an incident |
| A known-vulnerable dependency or base image | 2 — Trivy at the merge gate ([ADR-0104](../adr/0104-supply-chain-security.md)) | the CVE is published after the merge | **the pull side is unbounded.** Trivy is push-shaped: it sees what is being built, not what is already running. Nothing on the floor notices that a CVE published today affects an image built in March, which is what [`../operational-surface.md`](../operational-surface.md)'s Dependency-Track row would answer |
| An unsigned or floating-tag image | **6 — Kyverno at admission** ([ADR-0104](../adr/0104-supply-chain-security.md)) | it does not. Admission is not bypassable by a deploying party | **none, by construction.** This class is why position 6 exists |
| A privileged or non-conforming workload | **6 — Pod Security Admission** ([ADR-0200](../adr/0200-cluster-topology.md)) | it does not | **none.** In-tree, so it fails with the API server rather than separately |
| An unreachable or over-reachable workload | **6 — CiliumNetworkPolicy** in the datapath ([ADR-0206](../adr/0206-cluster-networking.md)) | it does not, for traffic. A missing policy is a different defect | **none for enforcement; the gap is a policy nobody wrote**, which position 4 is the only check on |
| A resource-governance defect — no request, wrong priority | **6 — in-tree API objects** ([ADR-0204](../adr/0204-resource-management.md)) | it does not | **none.** `LimitRange` defaults it or `ResourceQuota` rejects it |
| A plaintext secret in a committed file | 4 — review ([ADR-0202](../adr/0202-secrets.md)) | a reviewer does not see it | **unbounded, and this is the sharpest row here.** The rule is stated and no position enforces it. Unlike the classes above, the defect is permanent once merged: git history holds it after the file is fixed |
| An authorization defect — a check omitted, or a dual write outside a workflow | 4 — review ([ADR-0304](../adr/0304-identity-and-authorization.md), [ADR-0302](../adr/0302-temporal.md)) | a reviewer does not see it | **unbounded.** Statically detectable in principle; enforced by attention in fact |
| Cluster state diverging from the repository | 5 — Argo CD, with `selfHeal=false` in production ([ADR-0201](../adr/0201-gitops.md)) | a human applies it and nobody reads the drift notification | **hours to next working day.** Detection, not enforcement — production reverts nothing on its own, deliberately |

## Reading the table

**Three residuals are unbounded, and they are not the same kind of thing.**

- A **plaintext secret** and an **authorization defect** are unbounded because review is the only position holding them. Both are mechanically detectable in principle, so the residual is a gate nobody has written rather than a property nobody can check.
- The **CVE pull side** is unbounded because the gate is push-shaped by design. No amount of CI closes it; it is closed by a component, and the component is priced out of Core by the operational budget ([`../operational-surface.md`](../operational-surface.md)).

**The plaintext-secret row is the one to read first.** It is the only class here where the defect survives the fix: reverting the file does not remove it from history, so the remediation is a credential rotation rather than a revert. Every other row on this table is repaired by shipping a correction.

**The path is sound where it is unbypassable and advisory where it is not, and the assignment is deliberate.** Reading the *Caught by* column, every class assigned to position 6 has no residual, and every class with an unbounded residual is assigned to position 4. That is [ADR-0203](../adr/0203-policy-enforcement.md)'s rule producing exactly what it promises — and the cost of the rule stated in one place: **enforcement strength on this platform tracks whether a machine can see the property, not how much the defect costs.** A plaintext secret is more expensive than a floating tag and is checked by a person, because one is a string a linter must guess at and the other is a field.

**Two rows are a scheduling choice, not a capability gap.** The cross-service regression and the broken operator dashboard both have a suite that catches them; the suite runs nightly rather than on the pull request. Affected-detection already knows which pull requests touch more than one service, so moving the first is a policy change in the workflow.

**Compare the two composites.** The run-time path's dominant term is human attention ([`detection-latency.md`](detection-latency.md)); the build path's is whether a property is machine-visible. They fail differently, and a project reducing the floor should read both: [`../adoption-path.md`](../adoption-path.md)'s managed swaps change the first and leave the second untouched.

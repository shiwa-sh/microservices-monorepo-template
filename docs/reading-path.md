# Reading Path

The ADR set is optimised for correct application by someone who has already read it. That is right for whoever maintains this platform and wrong for a first week, so this document is the ordered entry: what to read, in what order, and what each role can skip.

**Start with a running cluster, not with ADR-0000.** [`dev-loop.md`](dev-loop.md) ends with a served request. Reading the reasoning behind a system you have seen respond is a different exercise from reading it cold.

## The first ninety minutes

In this order. Each one is a prerequisite for the next.

| # | Read | Why here |
| --- | --- | --- |
| 1 | [`dev-loop.md`](dev-loop.md) | A cluster comes up and a request is served. Everything below is about a thing you have now watched work |
| 2 | [`reference/system-view.md`](reference/system-view.md) | The shape: what runs, what calls what, and where a request goes. One page |
| 3 | [ADR-0000](adr/0000-platform-foundations.md), *Thesis* and *Principles* | The three axes and this platform's position, then the ten principles every later decision cites. Stop before *Prior art* |
| 4 | [ADR-0001](adr/0001-documentation-and-output-conventions.md), *ADR structure* and *Banned constructs* | How to read the documents, and how to write one. It is the shortest path to reading the rest quickly |
| 5 | [`operational-surface.md`](operational-surface.md) | What is always on, what it obliges, and what stops working when each part does |
| 6 | The **Decides** line of every ADR, from [the index](adr/README.md) | One sentence each. The whole set's decisions in a single pass |

After these, the set is navigable. **The estimate assumes a skim of steps 3 and 5 rather than a close read** — they are dense, and the ninety minutes buys orientation, not retention. What remains is depth, and depth is read on demand.

## Then make a change

Navigable is not productive. [`guide/first-change.md`](guide/first-change.md) walks one small change — adding a field to a service — through every gate it passes: the spec, `mise run gen`, the migration, the drift check, the nine CI gates in the order they fail, and the four classes review exists for. It is the how-to counterpart of this document, and the fastest way to convert a read of the set into a working model of it.

## By role

Every role reads the ninety minutes above. What follows is what to read next, and what to leave until it matters.

| Role | Read next | Safe to defer |
| --- | --- | --- |
| **Anyone, before their first pull request** | [`guide/first-change.md`](guide/first-change.md) | nothing. It is one walkthrough, and it is shorter than the time lost to the mistakes it prevents |
| **Backend** | [0303](adr/0303-api-contracts-and-lifecycle.md) contracts, [0300](adr/0300-data.md) data, [0302](adr/0302-temporal.md) workflows, [0304](adr/0304-identity-and-authorization.md) authorization, [0003](adr/0003-naming-and-identifiers.md) identifiers, [0500](adr/0500-observability.md) instrumentation | the cluster block (02xx), the frontend block (04xx) |
| **Frontend** | [0400](adr/0400-frontend.md) stack, [0306](adr/0306-trust-tiers-and-urls.md) tiers and origins, [0303](adr/0303-api-contracts-and-lifecycle.md) generated clients, [0700](adr/0700-analytics.md) consent and events | data (03xx beyond contracts), infrastructure (02xx) |
| **Platform** | the whole 02xx block, then [0104](adr/0104-supply-chain-security.md), [0201](adr/0201-gitops.md), [0203](adr/0203-policy-enforcement.md), [0500](adr/0500-observability.md)–[0502](adr/0502-alerting-and-on-call.md) | the frontend block, analytics |
| **Security reviewer** | [`security-baseline.md`](security-baseline.md), then [0304](adr/0304-identity-and-authorization.md), [0305](adr/0305-edge-auth-and-traffic-policy.md), [0306](adr/0306-trust-tiers-and-urls.md), [0203](adr/0203-policy-enforcement.md), [0202](adr/0202-secrets.md), [0301](adr/0301-data-lifecycle-privacy.md), and [`reference/risk-register.md`](reference/risk-register.md) | everything else. The register is the fastest route to what this platform accepts |
| **Deciding whether to adopt** | the root README, then [`adoption-path.md`](adoption-path.md) and [`operational-surface.md`](operational-surface.md) | the ADRs. They answer *why*, and adoption is a question about *cost* |
| **An agent** | [`../AGENTS.md`](../AGENTS.md), then the Rules sections named there for the task class at hand | full ADR bodies, until a rule's rationale is the actual question |

## Reading an ADR quickly

The sections are in a fixed order, so a partial read is a predictable read.

| Want | Read |
| --- | --- |
| What is true | the **Decides** line, then **Rules** |
| Whether a rule is enforced | the annotation on that rule — a task, a policy, a standard, or nothing, which means review |
| Why, in one pass | **Decision drivers**, then the verdict column of **Considered options** |
| Why not the obvious alternative | that option's row. Losers are described in their best form, which is what makes the row worth reading |
| What it would take to change it | **Consequences**, and any Trigger/Seam/Cost table |

## Where the set is not the answer

| Question | Not an ADR |
| --- | --- |
| How do I run it? | [`dev-loop.md`](dev-loop.md), and the how-to documents indexed in [`README.md`](README.md) |
| How do I change it? | [`guide/first-change.md`](guide/first-change.md) |
| What can reach production unchecked? | [`reference/build-path.md`](reference/build-path.md) |
| What is deployed, and what does it cost to keep? | [`operational-surface.md`](operational-surface.md) |
| Can we run less than this? | [`adoption-path.md`](adoption-path.md) |
| What does this platform accept going wrong? | [`reference/risk-register.md`](reference/risk-register.md) |
| How long until we notice? | [`reference/detection-latency.md`](reference/detection-latency.md) |
| What are we waiting for, and who is watching? | [`reference/deferral-register.md`](reference/deferral-register.md) |

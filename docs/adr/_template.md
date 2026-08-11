# ADR-NNNN: Title

- **Status:** Accepted
- **Date:** YYYY-MM-DD
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md)
- **Decides:** one sentence, declarative, naming what is true of the platform because of this ADR. It is the index entry and the skim line, not a summary of the reasoning.

<!--
Written to ADR-0001. Before merging, check:
  - Every word is load-bearing. No intensifiers, hedges, meta-commentary, or chronology.
  - Three or more items sharing two or more attributes are a table.
  - A fact appears in exactly one ADR; everywhere else is a link.
  - Standing law only. No Follow-ups, no roadmap, no TODO, no note on whether the artefact exists yet. Planned work is not part of the committed record.
  - Every tool named in the Decision has a tool-register row, and a comparison at the depth its tier owes (ADR-0002).
  - Every decisive comparison cell is graded (measured), (documented), or (reasoned).
  - Every number survives the doubling test. No illustrative figures, no counting live state that lives elsewhere.
  - No tool release versions. Spec versions, licence identifiers, and a capability boundary stated as a floor are fine; the pin lives in the lockfile or the chart. Prefer /latest/ in a documentation link.
  - Drivers are properties. Swap the winner for a loser and re-read them: they must be unmet, not wrong.
  - Written for whoever maintains this platform, not for a prospective adopter. Selection guidance goes in README.md.
  - A section with nothing to say is omitted, not padded.
  - The number comes from the block your layer owns — see README.md.
-->

## Context

The problem, and the constraints inherited from earlier ADRs. What is being decided, and what is out of scope.

## Decision drivers

In priority order. Anchor each to an ADR-0000 principle where one applies.

1. **Driver.** Why it matters here.
2. **Driver.** Why it matters here.

## Considered options

**Mandatory** ([ADR-0000](0000-platform-foundations.md), principle 7). A table with prose cells, where every option answers the same questions. Not a scoring grid: the trade-offs are qualitative and numeric scores manufacture false precision.

Depth is set by the tool's exit-cost tier ([ADR-0002](0002-tool-adoption.md)) — a full table at Tier 1 with a named runner-up, a short one at Tier 2, a register line at Tier 3. A long deep-dive belongs in the PR discussion, not here.

The cell that produces each verdict is graded *(measured)*, *(documented)*, or *(reasoned)*. Cells that do not decide the row are not graded.

| Option | <the question that decides it> | <the second question> | Verdict |
| --- | --- | --- | --- |
| **Chosen option** | | | **Chosen.** Why *(grade)* |
| Alternative | | | Why it lost *(grade)*. The runner-up says so |
| Alternative | | | Why it lost *(grade)* |
| Do nothing | | | The honest baseline, and why it does not survive |

## Decision

Declarative. "We use X. We do not use Y." Tables for anything with structure.

For anything deferred, state all three fields, or it is not a deferral:

| Field | Value |
| --- | --- |
| **Trigger** | an *observable* condition. "When we grow" is not a trigger |
| **Seam** | whether the slot already exists. Without one this is a **bet**, and must be labelled as one |
| **Cost if adopted late** | what the delay buys, and what it risks |

## Consequences

### Positive

- What this buys.

### Negative / Risks

- **The cost, stated plainly.** How it is mitigated, or that it is accepted and why.

## Rules

Flat, greppable, normative. A rule that a task, a policy, or a standard enforces names it; one that nothing enforces carries no annotation and is not thereby weaker.

- The rule, stated as a standing fact. `(CI: <task>)`
- The rule, stated as a standing fact. `(enforced: <admission policy>)`
- The rule, stated as a standing fact. `(ref: <standard>)`
- The rule, stated as a standing fact.

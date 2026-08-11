# ADR-0002: Tool Adoption & Comparison Requirement

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0001](0001-documentation-and-output-conventions.md)
- **Decides:** Exit cost sorts every tool into three tiers, each owing a fixed depth of recorded comparison, and one register lists them all.

## Context

[ADR-0000](0000-platform-foundations.md) principle 7 states that a tool with no recorded comparison is an assumption rather than a decision, and sets depth by exit cost. Principle 7 is the rule; it does not say what "recorded" means for a linter versus a database, and it does not say where the record lives.

Without that, the rule fails in both directions. Applied uniformly at ADR depth, every transitive dependency demands a page and the set becomes unreadable. Applied by taste, depth tracks how interesting the choice felt at the time, which is uncorrelated with what it costs to undo.

This ADR sets the tiers, the depth each owes, the single register, and the grading a comparison cell carries. It decides no tool.

## Decision drivers

1. **Depth tracks exit cost, not interest.** [ADR-0000](0000-platform-foundations.md) principle 4 already measures adoption by what abandonment costs. The record is that measurement applied to itself: the effort spent writing a comparison is proportional to the effort of acting on it later.
2. **The record is mechanically checkable.** A rule a reviewer applies from memory decays. Whether a tool has a register row, and whether that row's ADR holds a comparison table, is a parse.
3. **A comparison is evidence that an evaluation happened.** Its function is to let a later reader re-litigate the decision correctly, which requires the losing options and what each was judged on — not a justification of the winner.
4. **One home per fact.** [ADR-0001](0001-documentation-and-output-conventions.md) puts a fact in exactly one place. A tool's tier, licence, governing body, and maturity are one fact each, and they belong together in one registry rather than scattered across the ADRs that use them.

## Considered options

### How depth is set

| Option | What decides depth | Failure mode | Verdict |
| --- | --- | --- | --- |
| **Tiers by exit cost** | how long removal takes and what it touches | a tier assignment can be argued, and the argument is the useful part | **Chosen.** It is principle 4 applied to the record, so the same judgement produces both the adoption and its documentation *(reasoned)* |
| Principle 7 alone, no tiers | the author | depth tracks novelty and enthusiasm, and the boring expensive choice gets the thinnest record *(reasoned)* | The observed failure of the unqualified rule |
| One depth for every tool | nothing | either every linter gets an ADR, or the database gets a line | Uniformity at the cost of the property being measured |
| Tiers by component tier — Core / Scale / Opt-in | operational obligation | orthogonal to exit cost. An Opt-in library can be a rewrite to remove, and a Core component can be a chart swap | Measures the wrong axis. [`docs/operational-surface.md`](../operational-surface.md) keeps that tiering for what it is for |
| Weighted scoring matrix | a numeric total | manufactures precision from qualitative trade-offs, and the weights are chosen after the answer is known | Rejected on the same grounds [ADR-0001](0001-documentation-and-output-conventions.md) rejects scoring grids |

### Where the record lives

| Option | Discoverable by | Why not |
| --- | --- | --- |
| **Comparison in the owning ADR, one register row per tool** | reading the decision, or the register | **Chosen.** The reasoning sits beside what it decides, and the register answers "what is in here and why" in one file *(reasoned)* |
| Register only, with no ADR table | the register | A row cannot carry what each option was judged on. The comparison degrades to a list of names *(reasoned)* |
| ADR only, no register | reading all of the ADRs | A tool nobody wrote an ADR for is invisible, which is the exact failure principle 7 exists to catch |
| A separate document per comparison | a directory listing | Splits the decision from its reasoning, and [ADR-0001](0001-documentation-and-output-conventions.md) gives an ADR both |

## Decision

### The three tiers

A tool's tier is set by one question: **what does removing it cost, and what does it touch.**

| Tier | Exit cost | Touches | Owes |
| --- | --- | --- | --- |
| **1 — structural** | months, and possibly customer data | the architecture, or state that must be migrated | a full comparison table in the owning ADR, a **named runner-up**, and a register row |
| **2 — substitutable** | weeks | one interface, one chart, or one generator's output | a short comparison table in the owning ADR — the decisive question and the options that answered it differently — and a register row |
| **3 — library** | days | code inside one package or one build step | a register row naming what it was picked over |

**The tier is a property of the seam, not of the component's size.** PostgreSQL is Tier 1 because the data is in it. Argo CD is Tier 1 because every environment's reconciliation runs through it. `zod` is Tier 3 because replacing it is a mechanical edit inside the packages that import it, however many of those there are.

**A Tier 1 tool names its runner-up.** The runner-up is the fallback [ADR-0000](0000-platform-foundations.md) principle 4 requires of every young component, and stating it converts a novel choice from a bet into a deferral. A Tier 1 table whose losing options are all disqualified on a hard constraint has no runner-up, and says so.

### Evidence grading

A comparison cell that produces a verdict states where its claim comes from. Three grades, and one of them is marked on every decisive cell:

| Grade | Means | Obligation |
| --- | --- | --- |
| *(measured)* | a number this platform produced | states its conditions and how to re-derive it ([ADR-0001](0001-documentation-and-output-conventions.md), *Numbers*) |
| *(documented)* | read from the option's own documentation, licence, or governing body | carries a citation, because it can silently become false. Load-bearing instances are dated in [`reference/upstream-status.md`](../reference/upstream-status.md) |
| *(reasoned)* | follows from a property already established in this set | carries no citation. There is nothing external to check, which is the point of the mark |

**Only the decisive cell is graded** — the one the verdict rests on. Grading every cell restores the density the tables exist to avoid, and a reader who cannot tell which cell decided the row is reading a table that has not finished deciding.

The grade is what lets a reader re-litigate correctly. A *(documented)* claim is re-checked against upstream; a *(measured)* one is re-run; a *(reasoned)* one is argued with. Conflating the three is how a table that has gone stale continues to read as rigorous.

### The register

[`docs/tool-register.md`](../tool-register.md) lists every tool this platform runs, builds with, or generates from, at every tier. It is the canonical inventory in the sense [ADR-0001](0001-documentation-and-output-conventions.md) means: the one place a tool's tier, licence, governing body, maturity signal, and owning ADR are recorded.

It is not a second decision surface. A row states facts about a tool and points at the ADR that chose it; the reasoning stays in the ADR.

**Licence, governing body, and maturity are recorded and do not veto** ([ADR-0000](0000-platform-foundations.md) principle 4). They are evidence about exit cost — a single-vendor project under a source-available licence has a different abandonment profile from a foundation-governed one — and the register carries them so that evidence is available without re-researching it.

### What is out of scope

**Transitive dependencies are not tools.** A package pulled in by something in the register is governed by the register row above it. Vendoring, forking, or depending on a transitive package directly makes it a Tier 3 adoption and gives it a row.

**A specification is not a tool.** OpenAPI, OCI, RFC 9457, and SLSA are contracts the platform conforms to. Where one was chosen over an alternative, the comparison lives in the ADR that adopted it and no register row follows.

## Consequences

### Positive

- Depth is derivable rather than negotiated. A reviewer asks what removal costs, and the required record follows.
- The register makes a missing comparison visible without reading the set, which is the failure principle 7 names and could not previously see.
- Evidence grading gives a stale table a way to be caught: a *(documented)* cell has a source that can be re-read, and re-reading it is a bounded task.
- A Tier 1 runner-up is a fallback recorded before it is needed, so a novel structural choice is a deferral rather than a bet.

### Negative / Risks

- **Tier assignment is review-enforced.** A tool can be filed at Tier 3 to avoid owing a table, and no parse detects a wrong tier — only a missing row. Mitigated by the tier test being one question with a checkable answer, and by the register showing the claimed exit cost beside the tier.
- **The register duplicates the tool's name into a second file.** Accepted: the duplication is one row, it is drift-checked, and the alternative is a fact with no home.
- **Grading adds a token to decisive cells.** Accepted at three characters per row for the ability to tell a measurement from an inference.

## Rules

- Every tool this platform runs, builds with, or generates from carries a row in [`docs/tool-register.md`](../tool-register.md) stating its tier, owning ADR, licence, governing body, maturity signal, and exit cost. `(CI: lint:tool-register)`
- A tool's tier is set by exit cost: **1** structural, months and possibly customer data; **2** substitutable, weeks behind a stable interface; **3** library, days inside one package.
- A Tier 1 tool has a full comparison table in its owning ADR and a **named runner-up**, or states that no option survived the hard constraints. `(CI: lint:tool-register)`
- A Tier 2 tool has a short comparison table in its owning ADR, carrying the decisive question and the options that answered it differently. `(CI: lint:tool-register)`
- A Tier 3 tool's register row names what it was picked over. No ADR table is owed.
- Every alternative named in a register row appears in the owning ADR's *Considered options*. A rejection nobody can see is indistinguishable from an option nobody considered. `(CI: lint:tool-register)`
- A comparison cell that produces a verdict is graded *(measured)*, *(documented)*, or *(reasoned)*. Cells that do not decide the row are not graded.
- A *(documented)* claim carries a citation. A load-bearing one is dated in [`docs/reference/upstream-status.md`](../reference/upstream-status.md).
- A *(measured)* claim states the conditions it was taken under and how to re-derive it. `(ref: ADR-0001 Numbers)`
- Licence, governing body, and maturity are recorded for every tool and do not veto a choice.
- A transitive dependency is governed by the register row above it. Depending on one directly makes it a Tier 3 adoption and gives it a row.
- A comparison is a table with prose cells, never a weighted scoring matrix.

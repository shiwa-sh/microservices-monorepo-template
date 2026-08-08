# ADR-0001: Documentation & Output Conventions

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0500](0500-observability.md)

## Context

Four surfaces carry written language in this repository: ADRs and docs, structured logs, human CLI output, and code comments. Each has an audience that is half human and half LLM, and neither audience tolerates drift. Without a written style, scripts invent symbol vocabularies, logs interpolate context into message strings, docs accrete a dialect nobody recorded, and comments restate the code beside them.

Most of what a house style would say is already published. Re-deriving it produces a longer, unaudited rulebook.

## Decision drivers

1. **One style for humans and LLMs.** The rules must be greppable and mechanically checkable, not matters of taste.
2. **Adopt, don't restate.** A citation is shorter than a paraphrase, and a borrowed rule survives re-litigation where a house rule does not.
3. **Density is a correctness property.** A reader who skims a wordy ADR applies it wrongly. Length is the tax every future reader pays.
4. **Symbols are for humans, never for machines.** This line runs through every surface below.

## Considered options

| Option | Coverage | Maintenance | Why not |
| --- | --- | --- | --- |
| No written style | none | none | The default outcome is four dialects and no way to call a PR wrong |
| Write a complete house style guide | total | high — a second standards body to run | Duplicates Google, OTel, and clig.dev at lower quality, and drifts from them silently |
| **Adopt standards by reference; make only the deltas normative** | total | low — deltas are short | **Chosen** |
| Adopt [RFC 2119](https://www.rfc-editor.org/rfc/rfc2119)/[8174](https://www.rfc-editor.org/rfc/rfc8174) keywords for Rules | normative strength | low | Its `SHOULD`/`MAY` tiers invite negotiation. Every Rule here is binding; strength is carried by the enforcement annotation instead |

## Decision

### Adopted standards

| Surface | Adopted standard | Local delta (normative) |
| --- | --- | --- |
| Prose / docs | [ISO 24495-1:2023](https://www.iso.org/standard/78907.html) plain language; [Google Developer Documentation Style Guide](https://developers.google.com/style) (second person, present tense, active voice); [Diátaxis](https://diataxis.fr) genres | the density rules and banned constructs below; `ADR-XXXX` citation form; final-state facts |
| ADR structure | [MADR](https://adr.github.io/madr/) section set | the fixed section order below; mandatory comparison table ([ADR-0000](0000-platform-foundations.md), principle 7) |
| Structured logs | [OTel Semantic Conventions](https://opentelemetry.io/docs/specs/semconv/); [RFC 5424](https://www.rfc-editor.org/rfc/rfc5424) severity; [12-Factor XI](https://12factor.net/logs) | no symbols; context as attributes, never interpolated |
| CLI / human stdout | [clig.dev](https://clig.dev); POSIX Utility Conventions (exit codes, `--help`, `--version`) | the fixed `→ ✓ ✗ ⚠` vocabulary and 2-space sub-detail indent |
| Code comments | [Effective Go](https://go.dev/doc/effective_go), [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments); [Google Style Guides](https://google.github.io/styleguide/) for TypeScript | the comment rules below |

Diátaxis classifies an ADR as **explanation plus reference**. It is not a tutorial and not a how-to: it records a decision and its constraints, and never walks a reader through a task.

### Density

An ADR is a high-density technical document. Every word carries meaning or is deleted.

| Rule | Test a reviewer applies |
| --- | --- |
| One idea per sentence | The sentence has one main clause |
| Delete what survives deletion | Remove a clause; if the meaning is unchanged, it stays removed |
| Table over prose | Three or more items sharing two or more attributes become a table |
| Bullets over prose | Three or more parallel items become a list |
| Paragraphs are short | Over roughly 60 words, the paragraph becomes a list or a table |
| No restatement | A fact appears in exactly one ADR. Elsewhere it is a link |
| Figurative language is limited to the [ADR-0000](0000-platform-foundations.md) vocabulary | Any other metaphor, analogy, or wordplay is cut |

### Numbers

A number is stated when the number **is** the decision. One test settles it:

> **If this number doubled, would the decision change?**

| Answer | Kind | Example | Treatment |
| --- | --- | --- | --- |
| Yes | **Threshold** | "any volume exceeding 50% of node disk", "funnel-query p95 above 2s" | Stated. Changing it is a reviewed act |
| Yes | **Measurement that set a value** | Tempo's measured peak, which sets its explicit limit ([ADR-0204](0204-resource-management.md)) | Stated **with its conditions and how to re-derive it**. If it drifts, the value it set is wrong and must change |
| Yes | **Count of what is on the page** | "the six forces below" | Stated. The reader checks it against the table beside it |
| No | **Illustrative figure** | a footprint quoted to show the platform dominates | **Not stated.** Say the shape: "the platform dominates the footprint" |
| No | **Count of live state elsewhere** | "~25 always-on components", "ten workflows under `.github/workflows/`" | **Not stated.** Name the thing and link to its registry |

The last two rot. Nothing forces their update, and a figure without a date reads as current, so a reader draws a conclusion that stopped being true. A count of components, files, ADRs, or services belongs to the registry that owns them — [`docs/operational-surface.md`](../operational-surface.md) for platform components, [`README.md`](README.md) for the ADR set.

The argument almost never needs the figure. "A floor of a couple of dozen components, fixed regardless of service count" carries the same weight as an exact tally and cannot go stale. Where a figure is load-bearing, it is a threshold, or it states the conditions it was taken under and where to take it again.

### Line breaks

**Markdown is not hard-wrapped.** One line per paragraph, list item, or table row; a line ends only where the content does. `MD013` is disabled in `.rumdl.toml` for this reason.

Hard wrapping costs more than it looks. Editing one word reflows the paragraph, so the diff shows every line after it and review cannot see what changed. The wrap column is also a per-author choice, which is how a repo ends up with three of them.

The reader's line length is the reader's business: an editor soft-wraps, and so does every renderer.

### Tables

Tables are **compact**, never column-padded: one space inside each pipe, and a bare `---` delimiter regardless of column width. `MD060` enforces it and `mise run format:md` applies it.

Padding is the same trap as hard wrapping. Widening one cell reflows every row, so a one-word edit arrives as a whole-table diff. It also has to be redone by hand on every subsequent edit, which is why the padded tables in this repo were a minority that drifted.

### Banned constructs

| Banned | Example found in this repo | Write instead |
| --- | --- | --- |
| **Chronology** — how the decision came to be | "It has since grown into a good workload directory" | The standing fact: what it is now |
| **The former state** | "Today this is done with `psql`, `curl`, and Temporal" | The problem, stated without a date |
| **Discovery narrative** | "it turned out to be exactly the floor plus the mock" | "It is the floor plus the mock" |
| **Retrospection about the ADR set** | "ADR-NNNN did not previously cover" | Cover it, silently |
| **Intensifiers** | `very`, `really`, `quite`, `actually`, `simply`, `just`, `of course`, `obviously`, `clearly` | Delete. If the claim needs an intensifier it is not established |
| **Hedges** | `arguably`, `essentially`, `basically`, `more or less`, `in practice` | Decide. A hedge is an unfinished decision ([ADR-0000](0000-platform-foundations.md)) |
| **Meta-commentary** | "It is worth knowing", "Note that", "It should be noted" | State the thing |
| **Rhetorical questions** | "But is the pod actually running the new code?" | The answer, as a statement |
| **Dated section headings** | `## Implementation (2026-07-26)` | An undated heading |
| **Planned work** — a `Follow-ups` list, a `TODO`, a roadmap | "`tools/lint-prose` for the intensifier and hedge lists" | Nothing. State the law and stop. The gap belongs in a working file |
| **The implementation's status** | "tracked in", "not yet wired", "lands in a later phase", "the status quo", a count of what exists so far | The rule, unqualified. Whether the artefact exists is not the ADR's subject, and an option loses on its merits rather than on being what happens to run |
| **A link to an untracked file** | ``[`plan.md`](plan.md)`` | Nothing. The file is absent from every other clone |

Deleting a word is always in scope for a documentation PR and never needs its own justification.

### An ADR is law, not a plan

An ADR states what is true of this platform. It never states what someone intends to do about it.

The two are easy to confuse because both describe something absent from the repo, and the difference is where the obligation sits. **"Deployments reference images by digest" binds every future deploy whether or not one exists today.** "Wire the digest check into CI" binds an engineer, on a date, and is stale the moment the work lands or is dropped.

| An ADR's Rule | A working-file task |
| --- | --- |
| Holds indefinitely | Completes, and is then deleted |
| Is violated by code | Is *unmet* by an empty repo, which is not a violation |
| Binds every reader, including a generated project's | Binds one person in one repo |
| A reviewer cites it to reject a PR | A reviewer cannot cite it at all |

**A gap between an ADR and the repo does not weaken the ADR.** The decision is binding from the day it is accepted; an unbuilt artefact is unfinished work, not an unfinished decision. Recording the gap inside the ADR converts a standing rule into a status report, which is the one thing that guarantees it goes stale.

Gaps are recorded in a **local working file, never committed** — `*.local.md`, ignored by `.gitignore`, described in [`AGENTS.md`](../../AGENTS.md). Two reasons it stays out of the repo. A tracked plan is inherited by every generated project, which then carries a backlog belonging to someone else. And a plan is per-engineer working state, so committing it makes one person's queue everyone's merge conflict.

No file under `docs/` links to a local working file. A link that resolves in one clone and dangles in every other is worse than no link.

### A driver is a property, not an answer

A driver states what the decision is being optimised for. It is written so that someone who does not yet know the outcome could apply it and reach one.

Two failures make a driver useless, and both read as reasoning:

| Failure | Example | Instead |
| --- | --- | --- |
| **The driver names its own answer** | "One repo-wide release version, because the repo ships as a unit" | "The unit of versioning is the unit of shipping" — the property, which the version line then follows from |
| **The driver caricatures the loser** | "Keyless over key management. A team this size should not operate a signing-key HSM" — when the alternative was a key in the secret store already running | State the property, and let the option lose on what it actually is |

**A driver that only makes sense once you know the decision was written backwards.** Both failures produce a Considered options table whose verdicts cite a driver written to eliminate them, which is circular and reads as rigour.

The test: swap the chosen option for a rejected one and re-read the drivers. If they now sound wrong rather than unmet, they are describing the answer instead of the problem.

### ADR structure

Sections appear in this order. A section with nothing to say is omitted, not filled.

| # | Section | Contains |
| --- | --- | --- |
| 1 | Header | Status, Date, Deciders, Related |
| 2 | Context | The problem and the constraints inherited from earlier ADRs |
| 3 | Decision drivers | What is being optimised for, in priority order — see *A driver is a property, not an answer* |
| 4 | Considered options | A comparison table against the alternatives. Mandatory ([ADR-0000](0000-platform-foundations.md), principle 7) |
| 5 | Decision | Declarative. "We use X. We do not use Y." |
| 6 | Consequences | Positive, Negative / Risks. No `Follow-ups` — see *An ADR is law, not a plan* |
| 7 | Rules | Flat, greppable, normative bullets derived from the decision |

The comparison table uses prose cells where every option answers the same questions. It is not a scoring grid: the trade-offs are qualitative, and numeric scores manufacture false precision.

[ADR-0000](0000-platform-foundations.md) is the one exemption. It chooses no technology, so it has no options to compare and no single decision to state; it carries the thesis, principles, process, prior art, and vocabulary instead. Every other ADR uses the order above.

### Structured logs

The message is a short lowercase phrase with no trailing punctuation and no symbols. All context is key-value attributes following OTel semantic conventions, never interpolated into the message string. This restates [ADR-0500](0500-observability.md) as a Rule; it is not a second decision.

### Human CLI output

Scripts speak a fixed four-symbol vocabulary:

- `→` a step is starting
- `✓` a step succeeded
- `✗` a fatal error
- `⚠` a warning — never bare `WARN`
- two-space indent for sub-detail under a step

No written standard fixes TUI symbols; the checkmark/arrow idiom is convention by imitation of npm, cargo, and kubectl. clig.dev is cited for the principle and the vocabulary is fixed here. `scripts/lib/log.sh` (`step`/`ok`/`fail`/`warn`) implements it — formatting only, no error swallowing.

### Code comments

The density and banned-construct rules above apply unchanged to comments.

| Rule | Rationale |
| --- | --- |
| Comments explain **why**, not what | The code states what. A comment that restates it is a second source of truth that rots |
| A comment that explains confusing code is a defect | Rewrite the code ([Kernighan & Plauger](https://en.wikipedia.org/wiki/The_Elements_of_Programming_Style), *"Don't comment bad code — rewrite it"*) |
| Load-bearing constraints cite `ADR-XXXX` | The reader can reach the reasoning without archaeology |
| Present tense, full sentences for doc comments | Google / Effective Go |
| Go doc comments begin with the identifier name | Effective Go |
| No commented-out code | Git holds it |
| No changelog, author, or date comments | Git holds them |
| No decorative banners or section dividers | The file structure is the structure |
| `TODO` cites an issue or ADR, or it is not merged | An uncited `TODO` is the "temporary" that [ADR-0000](0000-platform-foundations.md) forbids |

### Template docs are final-state facts

**Scope: this template repository's own `docs/` only.** A project generated from it is a living system where ADR history, `Supersedes`/`Amends`, `Proposed → Accepted`, and real authored dates are legitimate.

**The ADR set is inherited wholesale.** A generated project clones it and it becomes that project's own record, so an ADR is written for the engineer maintaining the system, never for someone deciding whether to adopt it. Two consequences:

- **The subject is "this platform", not "this template".** Anything that reads correctly only in this repo — *we build a template*, *when not to use this*, *who should adopt this* — is selection guidance for a reader who has not decided yet. It belongs in the root `README.md`. The ADR states the position that was chosen.
- **`template` names an artefact, not the audience.** `services/_template/` and *the template default* are correct in any repo; *this template targets…* is not.
- **No doc under `docs/` links to the root `README.md`.** A generated project rewrites or deletes that file, so any reference into it is a link that rots and a fact that vanishes. Every term, test, and table an ADR relies on is stated in the ADR set itself.
- **The resulting overlap is expected**, and is not a violation of one-fact-one-place, which governs the ADR set. The README restates for a reader who has not decided yet; the ADR states for the engineer who now owns the result. Removing the duplication by deleting from the ADR is the wrong direction.

The template's snapshot obeys:

- **No change-history or evolution narrative.** Each decision is a standing fact plus its rationale, never its chronology.
- **No `Supersedes` / `Amends` chains.** Every ADR is current. One that must change is rewritten in place.
- **No `Proposed → Accepted` progression.** A shipped ADR reads `Accepted`.
- **A uniform date across the set**, so it reads as one design rather than accretion.
- **Full rewrite over patch.** A patched-in paragraph that clashes in voice is a defect even when factually correct.

### Numbering

ADR numbers are allocated in blocks of a hundred, one block per layer, sequential within the block. The first two digits carry the layer, so a new ADR lands in its block without renumbering the set. [`README.md`](README.md) holds the block map.

### Enforcement annotations

Every Rule ends with how it is enforced, so a reader distinguishes a hard invariant from an aspiration.

| Annotation | Means |
| --- | --- |
| `(CI: <task>)` | a named `mise` task or workflow rejects the violation |
| `(enforced: <policy>)` | admission control rejects it in the cluster, which CI cannot bypass |
| `(review-only)` | a human applies it |
| `(ref: <standard>)` | the adopted external standard is the rule |

The annotation names an enforcement that **exists**. An unbuilt check is `(review-only)` until the day it runs, because a rule advertising a gate nobody wrote is weaker than one that admits it is reviewed.

## Consequences

### Positive

- The conventions are mostly links, so they are short and hard to argue with.
- "Follow OTel semconv, clig.dev, Google" is one instruction an LLM acts on without a bespoke rulebook.
- The banned-constructs table gives a reviewer a mechanical basis for rejecting prose, replacing taste with a citation.
- The density rules bound ADR length, which bounds the cost every future reader pays.

### Negative / Risks

- Adopted standards evolve upstream and a citation can drift. Mitigated by making only the deltas normative.
- Density and comment rules are review-only until a linter exists. The banned-constructs list is written to be grep-shaped so that lint is a later addition, not a redesign.
- Terse prose loses nuance a longer version would carry. Accepted: nuance that matters becomes a table row, and nuance that does not becomes deletion.

## Rules

- Prose follows the Google developer-docs voice (second person, present tense, active) and ISO 24495-1 plain language, organised by Diátaxis genre. An ADR is explanation plus reference, never a tutorial. `(ref: Google dev-docs, ISO 24495-1, Diátaxis)`
- Every word is load-bearing. A clause whose deletion does not change the meaning is deleted. `(review-only)`
- Three or more items sharing two or more attributes are a table; three or more parallel items are a list. `(review-only)`
- Intensifiers, hedges, meta-commentary, rhetorical questions, and chronology are not used in template docs. `(review-only)`
- Figurative language is limited to the [ADR-0000](0000-platform-foundations.md) vocabulary. `(review-only)`
- A number is stated only if doubling it would change the decision: a threshold, a measurement that set a value, or a count of what is on the same page. An illustrative figure becomes the shape it demonstrates, and a count of live state elsewhere becomes a link to its registry. `(review-only)`
- A fact appears in exactly one ADR. Every other mention is a link. `(review-only)`
- Markdown is not hard-wrapped: one line per paragraph, list item, or table row. `(CI: lint:md)`
- Tables are compact — one space inside each pipe, a bare `---` delimiter — never column-padded. `(CI: lint:md)`
- An ADR uses the section order Context → Decision drivers → Considered options → Decision → Consequences → Rules, and omits rather than pads an empty section. `(review-only)`
- A decision driver states a property being optimised for, never the chosen option restated and never a rejected option's worst form. Swapping the winner for a loser must leave the drivers unmet, not wrong. `(review-only)`
- An ADR states standing law, never planned work. No `Follow-ups` section, no roadmap, no `TODO`, and no note on whether an artefact exists yet. A gap between an ADR and the repo is unfinished work, not an unfinished decision, and does not weaken the rule. `(review-only)`
- Gaps between the decided platform and the built one are recorded in a local `*.local.md` working file, which is ignored by `.gitignore` and never committed. `(review-only)`
- No committed file links to a local working file. `(CI: lint:md)`
- ADR numbers are allocated in blocks of a hundred by layer, per [`docs/adr/README.md`](README.md). `(review-only)`
- Structured logs carry a lowercase message with no trailing punctuation and no symbols; context is OTel-conventioned attributes, never string-interpolated. `(review-only; ref: OTel semconv)`
- Human CLI output uses `→` step, `✓` success, `✗` fatal, `⚠` warning, with two-space sub-detail indent. Bare `WARN`/`ERROR` prose and ad-hoc symbols are not used. `(review-only; ref: clig.dev)`
- Code comments explain why, not what, in present tense, and cite `ADR-XXXX` when load-bearing. `(review-only)`
- Commented-out code, changelog/author/date comments, and decorative banners are not committed. `(review-only)`
- A `TODO` cites an issue or an ADR. `(review-only)`
- A comment that exists to explain confusing code is a defect; the code is rewritten. `(review-only)`
- Template docs are final-state facts: no change-history, no `Supersedes`/`Amends` chains, no `Proposed → Accepted` narrative, a uniform date, and full-rewrite-over-patch. **`(scope: template repo only)`** — a generated project keeps honest ADR history. `(review-only)`
- An ADR addresses the engineer maintaining the platform, never a prospective adopter. Selection guidance — who should use this, when not to, what to swap before adopting — lives in the root `README.md`. `(review-only)`
- No file under `docs/` links to the root `README.md`, and the ADR set defines every term, test, and table it uses. A generated project rewrites that README, so duplication there is expected and correct. `(review-only)`
- A term enters the [ADR-0000](0000-platform-foundations.md) vocabulary only when the ADR set uses it, and only if the repo does not already use that word in another sense. `(review-only)`
- Every ADR Rule is annotated with its enforcement: `(CI: <task>)`, `(enforced: <policy>)`, `(review-only)`, or `(ref: <standard>)`. The annotation names an enforcement that exists; an unbuilt check is `(review-only)`. `(review-only)`

# Agent guide

Tool-agnostic guide for any coding agent (Codex, Cursor, Claude Code, or another) working in this repo. `AGENTS.md` is the one standard: an agent either reads it or it does not — the repo carries no per-tool shim files (`CLAUDE.md`, `.cursor/rules/`, etc.). A tool that ignores `AGENTS.md` is a limitation of that tool, not something the repo works around.

## The one rule that outranks this file

**Humans are the first developers. ADRs and `docs/` outrank this file.** The canonical home for any decision, convention, or rationale is an [ADR](docs/adr) or a `docs/` file — human-first, tool-neutral, reviewed.

This file holds only agent-specific operational hints: how to navigate, build, and run the repo, and what to read first. Everything else is a link into the ADRs and docs.

- A line a new human developer would also need belongs in an ADR or a doc. This file links it.
- A line one tool needs belongs in that tool's own dotfile, not here.

## Read first

- [ADR-0000](docs/adr/0000-platform-foundations.md) — the thesis, principles, vocabulary, and ADR process. Read it before anything else.
- [docs/adr/README.md](docs/adr/README.md) — the block map and the full ADR index, one line each.
- The [ADR index](docs/adr) — every load-bearing decision. Each ADR ends with a flat **Rules** section.

## How the docs are organised

- **`Rules`** at the bottom of each ADR are normative and greppable. Each is annotated with its enforcement: `(CI: <lint>)` = enforced by a linter, `(review-only)` = human-reviewed, `(ref: <standard>)` = an adopted external standard. Treat a `(CI: …)` rule as a hard invariant. To check a convention, grep the Rules sections first; read the full ADR only when you need the rationale behind a rule.
- **House style** for prose, logging, CLI output, and code comments is [ADR-0001](docs/adr/0001-documentation-and-output-conventions.md). It adopts ISO 24495-1 plain language, Google developer-docs voice, OTel semantic conventions for logs, and clig.dev for CLI output, and makes only the deltas normative. Its **banned-constructs table** governs every doc and every comment you write: no chronology, no intensifiers, no hedges, no meta-commentary. Every word is load-bearing, and three or more items sharing two or more attributes are a table.
- **Runbooks and reference** are indexed in [docs/README.md](docs/README.md). A doc holds a procedure or live state; a decision lives only in its ADR.
- **An ADR is law, not a plan.** It states what is true of this platform, never what someone intends to do about it. Do not add a `Follow-ups` section, a roadmap, a `TODO`, or a note that something is "not yet wired" — a gap between an ADR and the repo is unfinished work, not an unfinished decision, and the rule binds regardless ([ADR-0001](docs/adr/0001-documentation-and-output-conventions.md)).
- **Planned work goes in a local `*.local.md` file**, which `.gitignore` excludes and nothing committed links to. `PLAN.local.md` is this repo's, and it is where you record any gap you find between a decision and the code. It may be absent — that is normal, since it is per-engineer and untracked. Never create a committed roadmap, backlog, or status file to replace it: a tracked plan is inherited by every generated project, which then carries a backlog belonging to someone else.
- **Audience.** The ADR set is inherited wholesale by every project generated from this repo, so write an ADR for the engineer maintaining the platform, not for someone deciding whether to adopt it. Never write "this template targets…" or "when not to use this" in an ADR — that is selection guidance and belongs in the root `README.md`. Nothing under `docs/` links to that file, because a generated project rewrites it ([ADR-0001](docs/adr/0001-documentation-and-output-conventions.md)).
- **Component tiers and the operational budget** are [docs/operational-surface.md](docs/operational-surface.md) (Core / Scale / Opt-in). It is also the **only** place platform components are counted.
- **Before writing a number, ask whether doubling it would change the decision.** If yes it is a threshold or a sizing measurement — state it, with the conditions it was taken under. If no it is decoration that goes stale: write the shape ("the platform dominates the footprint") not the figure, and never count live state that lives elsewhere — "the four observability components", "ten workflows in `.github/workflows/`", "~25 components". Link to the registry instead ([ADR-0001](docs/adr/0001-documentation-and-output-conventions.md)).

## Working in the repo

- The task runner is `mise` (root `.mise.toml`); commands are `mise run <task>`. `mise run cluster:base` / `cluster:full` bring up the local cluster ([ADR-0600](docs/adr/0600-local-development-loop.md)).
- Generated code is committed and drift-checked in CI ([ADR-0000](docs/adr/0000-platform-foundations.md), [ADR-0303](docs/adr/0303-api-contracts-and-lifecycle.md)); regenerate with `mise run gen`, do not hand-edit generated files.
- ArgoCD reconciles the cluster from `master`; a working-tree change is invisible in-cluster until pushed ([ADR-0201](docs/adr/0201-gitops.md)).
- Before finishing a change, run `mise run check` (or `mise run pre-commit`) to lint, test, and format; `mise run test` / `lint` / `format` run each individually.

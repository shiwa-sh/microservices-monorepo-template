# Agent guide

Tool-agnostic guide for any coding agent (Codex, Cursor, Claude Code, or another) working in this repo. `AGENTS.md` is
the one standard: an agent either reads it or it does not — the repo carries no per-tool shim files (`CLAUDE.md`,
`.cursor/rules/`, etc.). A tool that ignores `AGENTS.md` is a limitation of that tool, not something the repo works
around.

## The one rule that outranks this file

**Humans are the first developers. ADRs and `docs/` outrank this file.** The canonical home for any decision,
convention, or rationale is an [ADR](docs/adr) or a `docs/` file — human-first, tool-neutral, reviewed. This file holds
**only genuinely agent-specific operational hints** (how to navigate, build, and run the repo; what to read first) and
otherwise points into the ADRs and docs. Before adding a line here, ask: *would a new human developer also need this?*
If yes, it belongs in an ADR or doc, and this file merely links it. There is no tool-specific instruction here — anything
a single tool needs lives in that tool's own dotfile.

## Read first

- [ADR-0000](docs/adr/0000-platform-foundations.md) — the thesis, principles, vocabulary, and ADR process. Read it
  before anything else.
- [README.md](README.md) — the stack-at-a-glance table mapping each concern to its tool and ADR.
- The [ADR index](docs/adr) — every load-bearing decision. Each ADR ends with a flat **Rules** section.

## How the docs are organised

- **`Rules`** at the bottom of each ADR are normative and greppable. Each is annotated with its enforcement:
  `(CI: <lint>)` = enforced by a linter, `(review-only)` = human-reviewed, `(ref: <standard>)` = an adopted external
  standard. Treat a `(CI: …)` rule as a hard invariant. To check a convention, grep the Rules sections first; read the
  full ADR only when you need the rationale behind a rule.
- **House style** (prose, logging, CLI output, comments) is [ADR-0019](docs/adr/0019-prose-logging-output-conventions.md):
  it adopts Google developer-docs prose, OpenTelemetry semantic conventions for logs, and clig.dev for CLI output, and
  lists only the project-specific deltas. Follow the standards, not a bespoke rulebook.
- **Conventions and runbooks** live in `docs/<area>/…` (for example `docs/observability/conventions.md`,
  `docs/ops/break-glass.md`).
- **Component tiers and the operational budget** are [docs/operational-surface.md](docs/operational-surface.md)
  (Core / Scale / Opt-in).

## Working in the repo

- The task runner is `mise` (root `.mise.toml`); commands are `mise run <task>`. `mise run cluster:base` /
  `cluster:full` bring up the local cluster ([ADR-0003](docs/adr/0003-cluster-topology.md),
  [ADR-0016](docs/adr/0016-environment-parity.md)).
- Generated code is committed and drift-checked in CI ([ADR-0000](docs/adr/0000-platform-foundations.md),
  [ADR-0008](docs/adr/0008-api-contracts.md)); regenerate with `mise run gen`, do not hand-edit generated files.
- ArgoCD reconciles the cluster from `master`; a working-tree change is invisible in-cluster until pushed
  ([ADR-0004](docs/adr/0004-gitops.md)).
- Before finishing a change, run `mise run check` (or `mise run pre-commit`) to lint, test, and format;
  `mise run test` / `lint` / `format` run each individually.

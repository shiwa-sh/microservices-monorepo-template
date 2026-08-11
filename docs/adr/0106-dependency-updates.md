# ADR-0106: Dependency Updates & Template Propagation

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0101](0101-monorepo.md), [ADR-0102](0102-source-control-and-ci.md), [ADR-0104](0104-supply-chain-security.md)
- **Decides:** Renovate proposes every pin bump as a batched pull request the normal gates judge, and Copier propagates template changes by 3-way merge.

## Context

Every version in this repository is pinned. Developer and CI tools are pinned in `.mise.toml`, runtime images carry an explicit tag in Helm values, production values carry a digest, and floating tags are rejected in CI and at admission ([ADR-0101](0101-monorepo.md), [ADR-0104](0104-supply-chain-security.md)).

Pinning fixes what runs and transfers the currency problem to whoever moves the pins. Three decided positions depend on something moving them on a cadence:

| Position | What it assumes | Owning ADR |
| --- | --- | --- |
| Digest-pinned production images | a digest is refreshed when its tag is rebuilt, or production runs a base image nobody patches | [ADR-0104](0104-supply-chain-security.md) |
| Floating tags rejected everywhere | an upgrade is an explicit, reviewed act that someone performs | [ADR-0101](0101-monorepo.md) |
| Trivy as a merge gate | a CVE with a fix available produces a bump, rather than a permanently red gate | [ADR-0104](0104-supply-chain-security.md) |

A second currency problem runs in the other direction. This repository is a template, so a fix made here has to reach the projects generated from it, and the mechanism that carries it is the same class of decision: an automated pull request against a repository whose gates decide whether it lands.

Both are decided here. What a CVE means once it is found — triage, severity, and whether a build is blocked — belongs to [ADR-0104](0104-supply-chain-security.md).

## Decision drivers

1. **An upgrade is a reviewed change, not an event.** [ADR-0101](0101-monorepo.md) makes a version bump a normal PR so that any SHA reproduces its own toolchain. Whatever moves a pin produces a PR and passes the gates, or it defeats the pinning.
2. **The cadence is the platform's, not the dependency's.** A tool that pushes changes when upstream releases hands the release schedule to whoever publishes most often. The recurring obligation is a fixed slot ([`operational-surface.md`](../operational-surface.md)), which means batching.
3. **One repository, one dependency graph** ([ADR-0101](0101-monorepo.md)). The monorepo already forces every service onto one version of a dependency, so the update mechanism has one PR to open, not one per consumer.
4. **No ambient runtime** ([ADR-0000](0000-platform-foundations.md) principle 6). The mechanism runs from a pinned toolchain like everything else.
5. **Propagation is a merge, not a regeneration.** A generated project has diverged deliberately. A mechanism that overwrites its divergence is a mechanism nobody runs twice.

## Considered options

### Dependency updates

| Option | Ecosystems it covers | Grouping and scheduling | Digest pinning | Self-hosted | Verdict |
| --- | --- | --- | --- | --- | --- |
| **Renovate** | Go modules, npm/Bun, Dockerfiles, Helm charts and values, GitHub/Forgejo Actions, `.mise.toml`, Terraform, and regex-matched pins anywhere else | grouped presets, arbitrary schedules, and a dependency dashboard issue | [resolves and writes the digest](https://docs.renovatebot.com/configuration-options/#pindigests) beside the tag it pins *(documented)* | a container, run from CI or as a service | **Chosen.** It is the only option covering the whole pin surface, and it is the only one that writes digests rather than tags *(documented)* |
| Dependabot | Go modules, npm, Docker, Actions | limited grouping, daily or weekly | tags only | forge-native, and the forge here is not GitHub | Loses on coverage: `.mise.toml`, Helm values, and Terraform are outside it, so a second mechanism covers the rest *(documented)* |
| `renovate` in "pin everything, merge nothing" mode plus manual sweeps | as Renovate | a person | as Renovate | — | The honest baseline. It is what happens when nothing is configured, and driver 2's fixed slot is the thing that is skipped in a busy quarter |
| A scheduled job running each ecosystem's own updater | whatever is scripted | bespoke | bespoke | fully | Rebuilds Renovate at lower quality, and each ecosystem's updater has different opinions about lockfiles |
| Nothing — bump on demand | — | — | — | — | Makes the digest pin a permanent freeze, which converts [ADR-0104](0104-supply-chain-security.md)'s immutability property into an unpatched base image *(reasoned)* |

### Template propagation

| Option | Update model | Divergence in the generated project | Runtime | Verdict |
| --- | --- | --- | --- | --- |
| **Copier** | `copier update` performs a [**3-way merge**](https://copier.readthedocs.io/en/stable/updating/) between the old template render, the new one, and the project's current state | preserved, and conflicts surface as conflicts | Python, run as a pinned tool | **Chosen.** The 3-way merge is the property; it is the only option here that treats the generated project as authoritative over its own divergence *(documented)* |
| Cookiecutter | regeneration only | overwritten, or manually reconciled | Python | No update path. A second tool would carry the merge, which is the whole problem |
| A git remote and periodic merges from the template | git merge | preserved | none | Works, and it drags the template's whole history into every generated project, including files the project deleted at generation time |
| Copy once, never propagate | — | total | none | The honest baseline, and the position this template would take if a fix here never had to reach a generated project. It does |

**Copier is a Python tool, and [ADR-0000](0000-platform-foundations.md) principle 6 bans ambient runtimes rather than Python.** It is pinned in `.mise.toml` like every other tool, it runs on a developer or CI machine, and no service, image, or cluster workload depends on it. That is the same position [ADR-0100](0100-language-and-runtime.md) takes on developer tooling generally: the two-language cap governs authored code, not the tools that generate it.

## Decision

### Renovate opens the pull request; the gates decide

Renovate runs on a schedule from CI and opens pull requests. It merges nothing. Every update PR passes the same gates a human PR passes — lint, generated-code drift, tests, and the affected-service selection ([ADR-0101](0101-monorepo.md)) — because a bump that skips them is a pin that was never load-bearing.

| Class | Grouping | Schedule |
| --- | --- | --- |
| Go modules, and Bun packages | one PR per ecosystem, minor and patch together | weekly |
| Major versions, any ecosystem | one PR each, alone | weekly, and never grouped — a major is a decision |
| Container base images and platform chart images | one PR per image, tag and digest together | weekly |
| Pinned tools in `.mise.toml` | one PR for the file | weekly |
| Anything with a fixed CVE that Trivy gates on | one PR each, immediately | on detection |
| Terraform providers and modules | one PR | monthly |

**A digest is updated with its tag, never separately.** A digest that moves while its tag does not is a rebuild of the same version, which is the case base-image patching produces and the case a human reviewer cannot evaluate from the diff alone. The PR body carries both.

**Renovate's dashboard issue is the queue.** It lists what is pending, what is rate-limited, and what is failing to build, in the forge where work is tracked ([ADR-0102](0102-source-control-and-ci.md)).

### Update review is a standing obligation

The weekly batch is recurring work in the sense [`operational-surface.md`](../operational-surface.md) means: it arrives whether or not anything is wrong, and deferring it is the quiet failure. Two rules bound it.

**A dependency PR that fails its gates is not merged and is not disabled.** The failure is the information the gate exists to produce. Renovate reopening it every week is the correct behaviour.

**An update held back carries the reason in the Renovate configuration**, as a package rule with a comment, rather than in a person's memory. A pin held for a reason nobody recorded is indistinguishable from a pin nobody looked at.

### Copier propagates the template

A generated project tracks this repository through Copier's answers file and updates by 3-way merge. The template's own guarantee is bounded and stated:

| Guarantee | Value |
| --- | --- |
| **What propagates** | files the template renders and the project has not rewritten |
| **What does not** | anything the project edited, which arrives as a conflict rather than an overwrite |
| **Compatibility promise** | none. A template update may conflict, and resolving it is the project's work |
| **Cadence** | the project's, on its own schedule |

**Renovate updates the template pin.** The Copier answers file records the template's commit, which makes it a dependency like any other, so the same weekly mechanism proposes the template bump and the same gates judge it.

## Consequences

### Positive

- Digest pinning stays an immutability property rather than becoming a freeze, because something refreshes the digests on a cadence.
- One PR per ecosystem per week is a bounded review surface, and the monorepo's single dependency graph is what makes it one PR rather than one per service.
- A held-back dependency carries its reason in a reviewed file.
- A fix made in the template reaches generated projects through a mechanism that respects their divergence, so the second update is no more expensive than the first.

### Negative / Risks

- **A grouped PR fails as a unit.** One bad minor version blocks the batch behind it, and splitting the group is manual work. Accepted: the alternative is one PR per dependency, which is the review load driver 2 exists to bound.
- **Renovate is a large configuration surface**, and a misconfigured package rule silently stops updating something. Mitigated by the dashboard issue listing what is rate-limited or disabled, which makes silence visible.
- **The template's compatibility promise is none**, so a project that skips updates for long enough faces one large conflicted merge rather than several small ones. Stated rather than mitigated: the template is a starting point that a project owns, and pretending otherwise would make it a framework.
- **Copier is a Python tool in a two-language platform.** Accepted under the developer-tooling scope above, and pinned like every other tool.

## Rules

- Renovate opens every dependency, image, chart, and tool-version update as a pull request. It merges nothing, and no update bypasses the gates a human pull request passes. `(CI: ci:lint, ci:gen, ci:test)`
- Updates are grouped by ecosystem and batched on a schedule. A major version is proposed alone.
- A container image update moves the tag and the digest in the same pull request. A digest is never updated on its own. `(CI: lint:floating-tags)`
- A dependency held back carries its reason as a comment on the package rule that holds it. A pin with no recorded reason is treated as unreviewed.
- A failing update pull request is left open. Disabling the update to clear the queue is not done.
- A CVE that Trivy gates on and that has a fixed version available is proposed immediately, outside the batch. `(CI: ci:scan)`
- A generated project tracks this template through Copier's answers file and updates by 3-way merge. The template makes no compatibility promise, and a conflict is the project's to resolve.
- Copier and Renovate are pinned in `.mise.toml` like every other tool, and neither runs inside a cluster workload. `(CI: lint:node-scope)`

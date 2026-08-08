# ADR-0102: Source Control & CI Platform

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0101](0101-monorepo.md), [ADR-0103](0103-release-and-versioning.md), [ADR-0104](0104-supply-chain-security.md)

## Context

[ADR-0000](0000-platform-foundations.md) principle 3 makes source control and CI a first-class decision rather than a signup. A forge bundles four concerns a managed provider hides: the repository, the review record, the pipeline, and the identity that signs artefacts.

That last one makes the forge the platform's **trust root**. [ADR-0104](0104-supply-chain-security.md) signs every image with the CI workflow's OIDC identity, so the forge defines what "provably built by us" means.

Every workflow under `.github/workflows/` runs on provider-hosted runners. That is the largest standing distance between the stated axis-B position and what executes.

## Decision drivers

1. **Operational sovereignty** ([ADR-0000](0000-platform-foundations.md), principle 3). Source, review, and build run on infrastructure the organisation controls.
2. **Thinnest viable platform** (principle 2). A forge is one component. A suite that arrives with its own datastore fleet is a different purchase.
3. **The forge stays cheap to leave** (principle 4). CI logic lives in `mise run ci:*` ([ADR-0101](0101-monorepo.md)), so workflow YAML is a thin caller and portability is structural.
4. **One primitive per concern** (principle 5). The forge runs the pipelines. A separate CI engine is a second system.
5. **A CI identity keyless signing can use** ([ADR-0104](0104-supply-chain-security.md)).

## Considered options

| Option | Component weight | Pipelines | Governance | Verdict |
| --- | --- | --- | --- | --- |
| **Forgejo + Forgejo Actions** | one Go binary, plus a database on the existing CNPG cluster | built in, GitHub-Actions workflow syntax, runners are self-hosted by definition | non-profit (Codeberg e.V.), AGPL | **Chosen.** The only option that satisfies drivers 1–4 together |
| Gitea + Gitea Actions | identical | identical | company-controlled | Functionally equivalent. Provenance is *recorded evidence about exit cost* (principle 4), and the governing body is the only discriminator between two otherwise equal choices |
| GitLab CE | a suite — Gitaly, Redis, Sidekiq, its own Postgres, several web services | mature and complete | company-controlled, open-core | Rejected by principle 2, as [ADR-0000](0000-platform-foundations.md) already records. It replaces one component with a platform |
| Managed forge (the status quo) | none | hosted runners | third-party | Fails driver 1. It is the position this ADR exists to close |
| Self-hosted forge + separate CI engine | two components | Woodpecker, Tekton, or Argo Workflows | varies | Rejected by principle 5, as [ADR-0000](0000-platform-foundations.md) already records: a second system beside the forge |
| Do nothing | none | hosted | third-party | The honest baseline. It leaves axis B asserted rather than held, and leaves the signing identity in a third party's control |

Forgejo and Gitea are the same software lineage and score identically on every technical row. Principle 4 says provenance does not veto a choice but is recorded; here it is the only distinguishing evidence, so it decides.

## Decision

| Concern | Decision |
| --- | --- |
| Forge | **Forgejo**, self-hosted, backed by a database on the existing CNPG cluster ([ADR-0300](0300-data.md)) rather than its own |
| Pipelines | **Forgejo Actions**, with runners on infrastructure we control |
| Workflow content | checkout, toolchain setup, then `mise run ci:*`. Logic does not live in YAML |
| Review record | pull requests on the forge. Branch protection and required checks are configuration in the repository ([ADR-0000](0000-platform-foundations.md), principle 1) |
| Registry | **not** the forge's package registry. [ADR-0105](0105-image-registry.md) decides it separately |

**Workflow YAML is a portable subset.** Steps are `actions/checkout`, the repository's own composite setup action, and `mise run` calls. That subset runs on both Forgejo Actions and the provider-hosted workflows the template ships, which is what makes the forge cheap to leave in either direction.

### The template ships provider-hosted workflows

A generated project needs CI from its first commit, before any cluster exists to run a forge on. The workflows under `.github/workflows/` are therefore the bootstrap path, not a competing decision.

| Field | Value |
| --- | --- |
| **Trigger** | the platform cluster serves its first environment |
| **Seam** | ✅ the thin-YAML rule above. Moving is a re-targeting of the same `mise run ci:*` calls, not a rewrite of pipeline logic |
| **Cost if adopted late** | source, review history, and the signing identity stay with a third party. Migration cost does not grow with the delay, so waiting is cheap — but the sovereignty claim is not true until it lands |

### Signing identity

[ADR-0104](0104-supply-chain-security.md) chooses cosign keyless, which needs the runner to mint an OIDC token a public Fulcio instance will accept. That is a property of the pipeline platform, not of cosign.

| Case | Signing |
| --- | --- |
| The runner mints an accepted OIDC token | keyless, unchanged. [ADR-0104](0104-supply-chain-security.md) holds as written |
| It does not | a cosign key pair held in SOPS ([ADR-0202](0202-secrets.md)), which reintroduces the key custody [ADR-0104](0104-supply-chain-security.md) driver 2 removes |

The second case is a **real cost of this decision**, not a footnote: the fallback trades a hosted trust root for a key the platform team must rotate. It is verified before the migration trigger fires, never after.

## Consequences

### Positive

- Source, review, pipelines, and the build identity run on controlled infrastructure. Axis B is held rather than asserted.
- Forgejo Actions reuses the workflow syntax already in the repository, so the migration is re-targeting rather than rewriting.
- No second CI engine, and no second datastore: the forge uses the Postgres already on the floor.

### Negative / Risks

- **The forge joins Core**, and a forge outage blocks merges and builds. Argo CD keeps reconciling from the last synced commit, so a forge outage does not stop the running system.
- **Self-hosted runners are compute the platform team owns**, including their cache and their isolation. Untrusted-PR execution is a policy question a public repository must answer before enabling it.
- **The keyless-signing fallback may reintroduce key custody**, reversing an [ADR-0104](0104-supply-chain-security.md) driver.
- **Forgejo's pipeline feature set trails the provider it imitates.** The thin-YAML rule bounds the exposure: the surface used is checkout, setup, and a task call.

### Follow-ups

- `infra/helm/platform/forgejo/` with its chart, values per environment, and its CNPG database.
- Runner provisioning, and the isolation policy for pull requests from outside the organisation.
- Verify the runner's OIDC token against Fulcio, and record the outcome in [ADR-0104](0104-supply-chain-security.md).
- Migrate `.github/workflows/` to the forge, and retire whichever set does not survive.

## Rules

- The forge is Forgejo, self-hosted, with its database on the existing CNPG cluster. `(review-only)`
- Pipelines run on Forgejo Actions with runners on controlled infrastructure. No second CI engine is introduced ([ADR-0000](0000-platform-foundations.md), principle 5). `(review-only)`
- Workflow YAML checks out, sets up the toolchain, and calls `mise run ci:*`. Pipeline logic is not written in YAML. `(review-only)`
- Branch protection and required checks are configuration in the repository, never set through the forge UI ([ADR-0000](0000-platform-foundations.md), principle 1). `(review-only)`
- Container images are published to the registry in [ADR-0105](0105-image-registry.md), not to the forge's package registry. `(review-only)`

# Upstream Status

Third-party facts the ADR set rests on that can become false without anything in this repository changing: a project's maintenance status, a licence, a governance body, a capability's availability tier. They are live state, so they live here rather than in the ADRs that cite them, and each carries the date it was last verified.

**A stale row is a decision resting on something nobody has checked.** A row unverified for a year is restated as unverified rather than left to imply currency.

**The cadence has an owner, because a cadence without one is a signal with no reader** ([ADR-0000](../adr/0000-platform-foundations.md)). A quarterly Temporal `Schedule` opens a tracking issue in the forge, assigned to the platform component owner named in [`../operational-surface.md`](../operational-surface.md) for the ADR each row is load-bearing for. The same `Schedule` carries [`asvs-verification.md`](asvs-verification.md)'s rows and [`deferral-register.md`](deferral-register.md)'s **query** rows, which are the other two places this set asks someone to go and look. One issue, three tables, one owner per row.

Walking a row is three steps: read the upstream source, set the *Verified* date to today whether or not the status changed, and — where the status did change — open a second issue against the owning ADR rather than editing the ADR here. The date moving is the evidence the check happened; the status moving is a decision to reopen.

| Fact | Status | Verified | Load-bearing for |
| --- | --- | --- | --- |
| Grafana OnCall's open-source distribution | entered maintenance in March 2025 and was archived in March 2026; the repository is read-only, and cloud-connected phone, SMS, and push delivery are withdrawn from OSS | 2026-08-11 | [ADR-0502](../adr/0502-alerting-and-on-call.md) — the direct evidence that no credible self-hosted escalation layer exists, and [ADR-0000](../adr/0000-platform-foundations.md)'s ranking of on-call as second to concede |
| Temporal's Rust SDK | public preview, not generally available | 2026-08-11 | [ADR-0100](../adr/0100-language-and-runtime.md) — driver 4 excludes Rust on SDK maturity, and this is the fact that does it |
| OpenTelemetry Rust | beta across all three signals | 2026-08-11 | [ADR-0100](../adr/0100-language-and-runtime.md), same driver |
| zot's OCI 1.1 referrers support | native, so signatures and attestations store beside the image without a workaround | 2026-08-11 | [ADR-0105](../adr/0105-image-registry.md), [ADR-0104](../adr/0104-supply-chain-security.md) — admission verification is a registry read |
| MinIO's community edition | the repository was archived in April 2026; distribution is source-only and the maintained build is a proprietary-licensed product | 2026-08-11 | [ADR-0207](../adr/0207-cluster-storage.md) — abandonment, not novelty, is why the object store is not MinIO |
| Forgejo's governance | a non-profit umbrella, GPLv3-or-later | 2026-08-11 | [ADR-0102](../adr/0102-source-control-and-ci.md) — the only row distinguishing it from Gitea, so it carries the decision alone |
| Calico's open distribution | FQDN egress policy and per-flow logs remain in the paid tier | 2026-08-11 | [ADR-0206](../adr/0206-cluster-networking.md) — both capabilities are load-bearing, which is what makes Calico the runner-up rather than the choice |
| Untitled UI React | the React library is MIT; the Figma kit and the PRO component tiers are commercial | 2026-08-11 | [ADR-0400](../adr/0400-frontend.md) — only the MIT part is vendored |
| Distroless base images | non-current tags stop receiving updates | 2026-08-11 | [ADR-0101](../adr/0101-monorepo.md) — a base image left on an old tag is a security decision made by not deciding |

## What belongs here

A fact belongs in this table when **someone else can change it**. A fact belongs in an ADR when this platform decides it.

| Kind | Where |
| --- | --- |
| Maintenance status, archival, abandonment | here |
| Licence, governance body, ownership change | here |
| A capability moving between free and paid tiers | here |
| A capability's availability tier — preview, beta, GA | here |
| What this platform does about any of the above | the owning ADR |

Reasoning is never cited to this document. A decision cites the fact; the fact is verified here; and if the fact turns over, the decision is reopened by its own trigger rather than by an edit to this table.

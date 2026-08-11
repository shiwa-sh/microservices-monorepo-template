# ASVS Verification

[ADR-0304](../adr/0304-identity-and-authorization.md) sets the application security bar at OWASP ASVS Level 2 and states plainly that this is a design claim rather than a test result. This table is where that claim is examined: one row per concern, the ADR that owns it, and the verdict from the last examination.

**A verdict is one of three.** *Met* means someone read the requirements against the implementation and found them held. *Met with exception* means a requirement is deliberately unmet and the reason is recorded in the owning ADR. *Unverified* means nobody has looked, which is the honest starting state of every row and the state a row returns to when it goes a full cadence without examination.

**Cadence.** Once per ASVS release, and on any change to a row's owning ADR. The verdict is a property of a document version and an implementation, so both are named.

| Concern | Owner | Verdict | Examined |
| --- | --- | --- | --- |
| Authentication | [ADR-0304](../adr/0304-identity-and-authorization.md) — Kratos sessions, the NIST 800-63B-shaped password policy, AAL levels | unverified | — |
| Session management | [ADR-0304](../adr/0304-identity-and-authorization.md), [ADR-0306](../adr/0306-trust-tiers-and-urls.md) — lifetimes, step-up, cookie scope | unverified | — |
| Access control | [ADR-0304](../adr/0304-identity-and-authorization.md) — OpenFGA through `Checker`, never inline | unverified | — |
| Input validation and encoding | [ADR-0303](../adr/0303-api-contracts-and-lifecycle.md) — validators compiled into the decoder from the spec | unverified | — |
| Error handling and logging | [ADR-0303](../adr/0303-api-contracts-and-lifecycle.md)'s error envelope, [ADR-0500](../adr/0500-observability.md)'s structured logs and PII rule | unverified | — |
| Data protection and privacy | [ADR-0301](../adr/0301-data-lifecycle-privacy.md) | unverified | — |
| Communications security | [ADR-0200](../adr/0200-cluster-topology.md) east-west encryption, [ADR-0205](../adr/0205-environment-parity.md) verified TLS in every environment | unverified | — |
| Configuration and secrets | [ADR-0202](../adr/0202-secrets.md) | unverified | — |
| Malicious code and supply chain | [ADR-0104](../adr/0104-supply-chain-security.md) | unverified | — |

## Two things this table is not

**It is not a compliance artefact.** ASVS is the bar this platform designs to; a framework a project must demonstrably meet is [`per-instance-hardening.md`](per-instance-hardening.md).

**It does not cover vendored surfaces.** Lowdefy, Grafana, and pgweb are outside the bar ([ADR-0304](../adr/0304-identity-and-authorization.md)), the same boundary [ADR-0400](../adr/0400-frontend.md) draws for accessibility: a claim over software this platform does not write is a claim it cannot keep.

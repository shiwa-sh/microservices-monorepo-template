# ADR-0104: Supply-Chain Security

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0102](0102-source-control-and-ci.md), [ADR-0103](0103-release-and-versioning.md), [ADR-0105](0105-image-registry.md), [ADR-0201](0201-gitops.md)

## Context

The platform runs first-party images built in CI plus third-party charts and images. [`security-baseline.md`](../security-baseline.md) defers supply-chain controls per instance, which leaves the provenance of what runs unverified.

The platform team is small against a whole fleet ([ADR-0000](0000-platform-foundations.md)), and grows far more slowly than the fleet does. The controls must therefore be CI-defaulted and enforced at admission: a manual audit neither scales nor blocks.

## Decision drivers

1. **What runs is provably what we built** — signed, from our pipeline.
2. **Keyless over key management.** A team this size should not operate a signing-key HSM.
3. **Enforced at admission, not merely produced at build.** An unsigned image must not schedule.
4. **In-tree with GitOps** ([ADR-0201](0201-gitops.md)). Admission policy is files in the repo ([ADR-0000](0000-platform-foundations.md), principle 1).

## Considered options

### Signing

| Option | Key custody | Verdict |
| --- | --- | --- |
| **cosign keyless (Fulcio/Rekor OIDC)** | **none** — short-lived certificates bound to the CI workflow identity | **Chosen.** Nothing to hold, nothing to rotate |
| cosign with a long-lived key pair | a key to store, rotate, and protect | Recreates the custody problem driver 2 removes |
| Notary v2 / notation | a key or a hosted trust store | Same custody cost, narrower ecosystem |
| No signing, digest pins only | none | Digest pins prove immutability, not origin. A pinned digest from a compromised builder is still pinned |

### Admission enforcement

| Option | Policy as files | Verifies cosign signatures natively | Verdict |
| --- | --- | --- | --- |
| **Kyverno** | YAML in the repo | yes, including attestations | **Chosen** — the policy language is the same YAML the rest of the platform is written in |
| OPA Gatekeeper | Rego in the repo | through an external data provider | Rego is a second language for one concern |
| Admission in CI only | n/a | n/a | CI can be bypassed; admission cannot. Driver 3 rules it out |

## Decision

| Concern | Decision |
| --- | --- |
| Image signing | **cosign keyless**, using the CI workflow's OIDC identity. Every first-party image is signed |
| SBOM | generated with **syft** (SPDX) and attached as a cosign attestation, so the bill of materials travels with the image |
| Provenance | **SLSA build provenance** — what source, what builder — emitted as an attestation |
| Admission | **Kyverno** verifies the signature and required attestations on first-party images, and requires digest-pinned references |
| Third-party images | pinned by digest and allow-listed. Upstream signatures are verified where the publisher provides them, and a pinned digest is accepted where they do not |

Signatures and attestations are OCI referrers stored beside the image in the registry ([ADR-0105](0105-image-registry.md)), so admission verification is a registry read.

**Scope boundary.** Signing, SBOM, and provenance are build-time concerns in CI; Kyverno is the runtime gate. Vulnerability *scanning* is complementary and tracked in [`security-baseline.md`](../security-baseline.md), not here.

**Keyless signing depends on the pipeline platform.** The runner must mint an OIDC token a public Fulcio instance accepts. [ADR-0102](0102-source-control-and-ci.md) records the forge decision this rests on, and the key-based fallback if that token is unavailable.

The digest-pin rule reinforces [ADR-0103](0103-release-and-versioning.md): production already pins by digest, and Kyverno makes that structural rather than conventional.

## Consequences

### Positive

- The cluster runs only images it can prove came from our pipeline, and the digest-pin rule closes tag drift.
- Keyless signing removes key custody entirely.
- SBOM and provenance make incident response and CVE triage a lookup rather than an investigation.

### Negative / Risks

- **Kyverno is a new Core component** — admission-controller operational surface. Accepted: it is the enforcement point that makes the rest non-optional.
- **Keyless signing depends on the CI OIDC provider and a transparency log**, which is a third-party trust root at maximal sovereignty ([ADR-0000](0000-platform-foundations.md), *Where axis B is not maximal*). An outage blocks signing, not running. Mitigated by signing on release rather than on every reconcile.
- **An admission gate can block a deploy during an incident.** The break-glass path is documented in [`docs/ops/break-glass.md`](../ops/break-glass.md) rather than left to improvisation.

## Rules

- Every first-party image is cosign-signed keyless in CI and carries an SBOM and a provenance attestation. `(CI: image-workflow)`
- Kyverno rejects at admission any image lacking a valid signature or referenced by a floating tag. `(enforced: Kyverno)`
- All images are digest-pinned. `(CI: lint:floating-tags; enforced: Kyverno)`
- Third-party images are pinned by digest and allow-listed, with upstream signatures verified where published. `(review-only)`
- Admission policy is committed YAML reconciled by Argo CD, never applied by hand. `(review-only)`

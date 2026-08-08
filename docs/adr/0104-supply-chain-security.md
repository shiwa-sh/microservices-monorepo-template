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
2. **No hardware custody and no human-held credential.** A team this size cannot operate an HSM, and a key a person can copy is not a signing identity. Whatever holds the key must be machinery the platform already runs.
3. **Enforced at admission, not merely produced at build.** An unsigned image must not schedule.
4. **In-tree with GitOps** ([ADR-0201](0201-gitops.md)). Admission policy is files in the repo ([ADR-0000](0000-platform-foundations.md), principle 1).

## Considered options

### Signing

| Option | Where trust is rooted | Key custody | Verdict |
| --- | --- | --- | --- |
| **cosign with a key pair in SOPS** | **the cluster's own age key** ([ADR-0202](0202-secrets.md)) | one key pair, in the secret machinery that already holds every other secret | **Chosen.** The only option whose trust root is inside the boundary principle 3 draws |
| cosign keyless against the public Fulcio and Rekor | a third party's certificate authority and transparency log | none | Its value is verification by parties who do not trust us, and the only verifier here is our own admission controller. It also outsources the trust root, which is the dependency [ADR-0102](0102-source-control-and-ci.md) rejects a managed forge over. **Available only to issuers on [Sigstore's configured list](https://docs.sigstore.dev/certificate_authority/oidc-in-fulcio/)**, which a self-hosted forge is not |
| cosign keyless against a self-hosted Fulcio and Rekor | a certificate authority we operate | **a CA root key** — strictly more consequential than a signing key | Adds Fulcio, Rekor, TUF root metadata, and a timestamp authority to the floor, and its [documented production path](https://github.com/sigstore/fulcio/blob/main/docs/setup.md) expects a cloud KMS. It fails principle 2 without satisfying driver 2 |
| Notary v2 / notation | a key or a hosted trust store | the same as the chosen option | Equivalent custody, narrower ecosystem and tooling |
| No signing, digest pins only | nothing | none | Digest pins prove immutability, not origin. A pinned digest from a compromised builder is still pinned |

**Keyless is an axis-B-low technology.** It works by trusting somebody else to attest who you are, and its payoff — a public, tamper-evident log a stranger can check without your cooperation — is addressed to an audience this platform does not have. That is the same reasoning [ADR-0103](0103-release-and-versioning.md) applies to SemVer: a signal with no reader is cost without benefit.

### Admission enforcement

| Option | Policy as files | Verifies cosign signatures natively | Verdict |
| --- | --- | --- | --- |
| **Kyverno** | YAML in the repo | yes, including attestations | **Chosen** — the policy language is the same YAML the rest of the platform is written in |
| OPA Gatekeeper | Rego in the repo | through an external data provider | Rego is a second language for one concern |
| Admission in CI only | n/a | n/a | CI can be bypassed; admission cannot. Driver 3 rules it out |

## Decision

| Concern | Decision |
| --- | --- |
| Image signing | **cosign with a key pair**, generated at bootstrap and held in SOPS ([ADR-0202](0202-secrets.md)). Every first-party image is signed |
| SBOM | generated with **syft** ([SPDX](https://spdx.dev/)) and attached as a cosign attestation, so the bill of materials travels with the image |
| Provenance | **[SLSA](https://slsa.dev/) build provenance** — what source, what builder — emitted as an attestation |
| Admission | **Kyverno** verifies the signature and required attestations on first-party images, and requires digest-pinned references |
| Third-party images | pinned by digest and allow-listed. Upstream signatures are verified where the publisher provides them, and a pinned digest is accepted where they do not |

Signatures and attestations are OCI referrers stored beside the image in the registry ([ADR-0105](0105-image-registry.md)), so admission verification is a registry read.

**Scope boundary.** Signing, SBOM, and provenance are build-time concerns in CI; Kyverno is the runtime gate. Vulnerability *scanning* is complementary and tracked in [`security-baseline.md`](../security-baseline.md), not here.

**One signing identity, unchanged by the forge migration.** The private key is a SOPS-encrypted secret the CI job decrypts, and the public key is committed and referenced by the Kyverno policy. Nothing about it depends on which forge runs the pipeline, so moving the forge ([ADR-0102](0102-source-control-and-ci.md)) re-targets the workflow without touching the trust root. A trust-root change mid-life would mean re-signing every image or carrying two verification paths permanently.

**Escape hatch.** If first-party images are ever published for consumers outside this organisation to pull and verify, public verifiability acquires a reader and keyless earns its cost.

| Field | Value |
| --- | --- |
| **Trigger** | a first-party image is published for an external party to verify |
| **Seam** | present: cosign signs and Kyverno verifies either way, so the change is which key material the policy names |
| **Cost if adopted late** | images already published carry a signature verifiable only against our key, so external verification begins at the switch rather than covering history |

The digest-pin rule reinforces [ADR-0103](0103-release-and-versioning.md): production already pins by digest, and Kyverno makes that structural rather than conventional.

## Consequences

### Positive

- The cluster runs only images it can prove came from our pipeline, and the digest-pin rule closes tag drift.
- The trust root is inside the sovereignty boundary: nothing outside the organisation is asked to vouch for what we built.
- SBOM and provenance make incident response and CVE triage a lookup rather than an investigation.

### Negative / Risks

- **Kyverno is a new Core component** — admission-controller operational surface. Accepted: it is the enforcement point that makes the rest non-optional.
- **There is a key, and a key can be stolen.** A compromised signing key signs anything, and there is no independent log to contradict it — the property keyless buys and this does not. Bounded by the key never leaving SOPS and the cluster, by rotation being a documented procedure rather than an improvisation, and by Kyverno pinning the public key so a substituted key fails admission rather than passing quietly.
- **Signature history is only as good as the key's history.** Rotation invalidates nothing already signed, so the policy carries the current and previous public keys through a rotation window.
- **An admission gate can block a deploy during an incident.** The break-glass path is documented in [`docs/ops/break-glass.md`](../ops/break-glass.md) rather than left to improvisation.

## Rules

- Every first-party image is cosign-signed in CI with the platform key pair, and carries an SBOM and a provenance attestation. `(CI: image-workflow)`
- The signing private key exists only as a SOPS-encrypted secret; the public key is committed and named by the Kyverno policy. It is never held by a person and never stored unencrypted. `(review-only)`
- Kyverno rejects at admission any image lacking a valid signature or referenced by a floating tag. `(enforced: Kyverno)`
- All images are digest-pinned. `(CI: lint:floating-tags; enforced: Kyverno)`
- Third-party images are pinned by digest and allow-listed, with upstream signatures verified where published. `(review-only)`
- Admission policy is committed YAML reconciled by Argo CD, never applied by hand. `(review-only)`

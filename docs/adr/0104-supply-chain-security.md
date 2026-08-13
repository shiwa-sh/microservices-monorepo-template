# ADR-0104: Supply-Chain Security

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0102](0102-source-control-and-ci.md), [ADR-0103](0103-release-and-versioning.md), [ADR-0105](0105-image-registry.md), [ADR-0201](0201-gitops.md), [ADR-0203](0203-policy-enforcement.md)
- **Decides:** Images are signed by a cosign key pair held in SOPS, carry syft SBOM and SLSA provenance attestations, and are verified at admission.

## Context

The platform runs first-party images built in CI plus third-party charts and images. Absent a control, the provenance of a running image is inferred from the registry it was pulled from, which records where a thing was found rather than who made it.

The platform team is small against a whole fleet ([ADR-0000](0000-platform-foundations.md)), and grows far more slowly than the fleet does. The controls must therefore be CI-defaulted and enforced at admission: a manual audit neither scales nor blocks.

## Decision drivers

1. **The origin of a running image is verifiable**, not inferred from where it was found.
2. **No hardware custody and no human-held credential.** A team this size cannot operate an HSM, and a key a person can copy is not a signing identity. Whatever holds the key must be machinery the platform already runs.
3. **A control the constrained party can bypass is not a control.** Whatever gates an image has to sit where the deploying party cannot route around it.
4. **In-tree with GitOps** ([ADR-0201](0201-gitops.md)). Admission policy is files in the repo ([ADR-0000](0000-platform-foundations.md), principle 1).
5. **The trust root survives a forge migration** ([ADR-0102](0102-source-control-and-ci.md)). Changing it mid-life means re-signing every image or carrying two verification paths permanently.

## Considered options

### Signing

| Option | Where trust is rooted | Key custody | Verdict |
| --- | --- | --- | --- |
| **cosign with a key pair in SOPS** | **the cluster's own age key** ([ADR-0202](0202-secrets.md)) | one key pair, in the secret machinery that already holds every other secret | **Chosen.** The only option whose trust root is inside the boundary principle 3 draws *(reasoned)* |
| cosign keyless against the public Fulcio and Rekor | a third party's certificate authority and transparency log | none | Its value is verification by parties who do not trust us, and the only verifier here is our own admission controller. It also outsources the trust root, the dependency [ADR-0102](0102-source-control-and-ci.md) rejects a managed forge over. The public instance signs only for issuers named in [its own configuration](https://docs.sigstore.dev/certificate_authority/oidc-in-fulcio/), so a self-hosted forge's issuer has to be internet-reachable and accepted upstream before it can sign at all |
| cosign keyless against a self-hosted Fulcio and Rekor | a certificate authority we operate | **a CA root key** — strictly more consequential than a signing key, though [Fulcio's backends](https://github.com/sigstore/fulcio/blob/main/docs/setup.md) include an on-disk encrypted key, so it can sit in the same machinery | Custody is solvable; the floor is not. Fulcio, Rekor, TUF root metadata, and a timestamp authority join the always-on floor to serve a single verifier that already trusts us. Principle 2 refuses the purchase |
| notation, from the Notary Project | a key or a hosted trust store | the same as the chosen option | Equivalent custody, narrower ecosystem and tooling |
| A detached GPG signature beside the image | a key we hold | the same as the chosen option | Not an OCI artefact: nothing publishes it as a referrer and no admission controller on this floor reads one, so the verification step this table exists to enable has nothing to call |
| No signing, digest pins only | nothing | none | Digest pins prove immutability, not origin. A pinned digest from a compromised builder is still pinned |

**Keyless is an axis-B-low technology.** It works by trusting somebody else to attest who the signer is, and its payoff — a public, tamper-evident log a stranger can check without the signer's cooperation — is addressed to an audience this platform does not have. That is [ADR-0000](0000-platform-foundations.md)'s *signal with no reader* applied to a signature, as [ADR-0103](0103-release-and-versioning.md) applies it to a version number: the reader is named, and here nobody is standing in that position.

### Admission enforcement

| Option | Added components | Policy as files | Verifies signatures at admission | Verdict |
| --- | --- | --- | --- | --- |
| **Kyverno** | one controller | YAML in the repo | yes, natively, including attestations | **Chosen** — the policy language is the same YAML the rest of the platform is written in *(documented)* |
| OPA Gatekeeper | one controller | Rego in the repo | through an external data provider | Rego is a second language for one concern |
| Ratify with Gatekeeper | two | Rego plus verifier CRDs | yes, as an external verifier | Purpose-built for exactly this, and it costs Gatekeeper's second language *and* a second component |
| Kubernetes `ValidatingAdmissionPolicy` | **none — in-tree** | CEL in the repo | no — a CEL expression cannot read a registry | The option that expands no floor, and signature verification is the one thing in-process CEL cannot do |
| Admission in CI only | none | n/a | no — the gate is the pipeline | The deploying party owns the pipeline, so driver 3 rules it out |

### Scanning and SBOM generation

Both Tier 2 ([ADR-0002](0002-tool-adoption.md)): each reads an image and writes a report, so a swap changes a task and leaves the artefacts alone.

| Concern | Chosen | Picked over | Why |
| --- | --- | --- | --- |
| Vulnerability scanning | **Trivy** | Grype, Clair, a registry-side scanner, Snyk | One binary covering OS packages, language dependencies, infrastructure-as-code, and secrets, so one gate covers surfaces that would otherwise be several. Grype is the closest and is package-scanning only; a registry-side scanner runs after the push, which is after the merge this gates *(reasoned)* |
| SBOM generation | **syft** | Trivy's own SBOM output, cdxgen, the build system's | It produces the SPDX document the attestation carries, and it is the implementation the scanner also reads. Using the scanner for both would tie the SBOM's fidelity to a scanner's release cadence |

**Trivy is push-shaped, and that is a property rather than a defect.** It sees what is being built. Nothing here notices that a CVE published today affects an image built months ago — the pull-shaped half — which is [`../operational-surface.md`](../operational-surface.md)'s Dependency-Track row, deferred on the operational budget with its triggers stated there.

## Decision

| Concern | Decision |
| --- | --- |
| Image signing | **cosign with a key pair**, generated at bootstrap and held in SOPS ([ADR-0202](0202-secrets.md)). Every first-party image is signed |
| SBOM | generated with **syft** ([SPDX](https://spdx.dev/)) and attached as a cosign attestation, so the bill of materials travels with the image |
| Provenance | **[SLSA](https://slsa.dev/) build provenance** — what source, what builder — emitted as an [in-toto](https://github.com/in-toto/attestation) attestation, which is the envelope SLSA and cosign both speak |
| Admission | **Kyverno** verifies the signature and required attestations on first-party images, and requires digest-pinned references |
| Third-party images | pinned by digest and allow-listed. Upstream signatures are verified where the publisher provides them, and a pinned digest is accepted where they do not |
| Vulnerability scanning | **in CI, where a finding blocks a merge.** Neither the registry ([ADR-0105](0105-image-registry.md)) nor the cluster scans |

Signatures and attestations are OCI referrers stored beside the image in the registry ([ADR-0105](0105-image-registry.md)), so admission verification is a registry read.

**Signing, SBOM, and provenance are build-time; Kyverno is the runtime gate.** Scanning sits with the build-time half for the same reason: a scan after admission reports on what already shipped, which is a dashboard rather than a gate. Which scanner an instance runs is a per-instance choice recorded in [`per-instance-hardening.md`](../reference/per-instance-hardening.md). That it runs before merge is not.

**One signing identity, unchanged by the forge migration.** The private key is a SOPS-encrypted secret the CI job decrypts, and the public key is committed and named by the Kyverno policy. Nothing about it depends on which forge runs the pipeline, so moving the forge re-targets the workflow and leaves the trust root untouched (driver 5).

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
- **An admission gate can block a deploy during an incident.** The break-glass path is documented in [`docs/guide/break-glass.md`](../guide/break-glass.md) rather than left to improvisation.

## Rules

- Every first-party image is cosign-signed in CI with the platform key pair, and carries an SBOM and a provenance attestation. `(CI: ci:publish)`
- Vulnerability scanning is a merge gate in CI. Neither the registry nor the cluster scans ([ADR-0105](0105-image-registry.md)).
- The signing private key exists only as a SOPS-encrypted secret; the public key is committed and named by the Kyverno policy. It is never held by a person and never stored unencrypted.
- Kyverno rejects at admission any image lacking a valid signature or referenced by a floating tag. `(enforced: Kyverno)`
- All images are digest-pinned. `(CI: lint:floating-tags; enforced: Kyverno)`
- Third-party images are pinned by digest and allow-listed, with upstream signatures verified where published.
- Admission policy is committed YAML reconciled by Argo CD, never applied by hand.

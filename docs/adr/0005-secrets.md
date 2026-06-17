# ADR-0005: Secrets Management

- **Status:** Accepted
- **Date:** 2026-05-19
- **Deciders:** Platform team
- **Related:** [ADR-0002](0002-monorepo.md), [ADR-0004](0004-gitops.md), [ADR-0015](0015-naming-and-identifiers.md)

## Context

Every environment needs secrets: database passwords, JWT signing keys, OAuth client secrets, API tokens for external providers. They must be:

- **Versioned alongside the rest of the configuration**, so a deploy is reproducible from git.
- **Never readable in plaintext from the repo** by anyone outside the recipient list.
- **Decryptable in-cluster** by the GitOps controller at sync time, without humans being in the loop.
- **Decryptable locally** by engineers who need to run services against local infrastructure.
- **Rotatable** with a known procedure for key compromise and offboarding.

## Decision drivers

1. **Open source only.** No paid SaaS in the secret path ([ADR-0000](0000-platform-foundations.md)).
2. **GitOps-native.** Secrets reach the cluster through the same mechanism as everything else — a git commit reconciled by ArgoCD.
3. **One mechanism, every environment.** Local, dev, staging, prod all decrypt the same way.
4. **No additional stateful component** if it can be avoided.

## Decisions

### Tool: SOPS + age

[`sops`](https://github.com/getsops/sops) with [`age`](https://github.com/FiloSottile/age) recipients is the encryption tool.

- **`age`, not GPG.** Age has no key-server / web-of-trust ceremony and a single-line public key format.
- **`sops`, not sealed-secrets.** SOPS encrypts files at the key level (not the manifest level), so values are visible in diffs as encrypted blobs; structure stays reviewable.

### Recipient model

Every encrypted file in the repo has exactly three classes of recipient, listed in `.sops.yaml` at the repo root:

1. **Per-engineer keys** — one age public key per engineer with current access. Engineers generate their own key pairs with `age-keygen`; the private key lives at `~/.config/sops/age/keys.txt` and never leaves the laptop.
2. **Per-cluster key** — one age public key per environment (`dev`, `staging`, `prod`), named in `.sops.yaml` by the `{project}-{env}` form from [ADR-0015](0015-naming-and-identifiers.md). The matching private key lives only in that cluster, as a Kubernetes Secret in the `sops` namespace, materialised at cluster bootstrap by Ansible. The in-cluster operator (below) reads it.
3. **Ops-recovery key** — a single age public key whose private half is held offline by 2–3 senior engineers. It exists so that a lost cluster key can be recovered without re-encrypting every secret. Used only in disaster recovery.

`.sops.yaml` declares creation rules per path so files under `infra/gitops/<env>/` are encrypted to that environment's cluster key plus engineers plus ops-recovery; files outside an env path are encrypted to engineers plus ops-recovery only.

### In-cluster decryption

The cluster-side operator is the **sops-operator** (a small controller that watches CRDs referencing SOPS-encrypted files and produces native Kubernetes `Secret` objects). The Helm chart for the operator lives at `infra/helm/platform/sops-operator/`.

ArgoCD reconciles the encrypted file along with the rest of the manifests; the operator decrypts and creates the corresponding `Secret`; the service pod consumes the `Secret` via standard `envFrom` or `volumeMounts`.

Service authors reference secrets by name in the service's Helm values, exactly as they would any other Kubernetes Secret. The encryption layer is invisible to the service.

### Local decryption

`age-keygen` once, after which `mise run dev:up` and any `sops decrypt` invocation works without further configuration.

Engineers run services locally against decrypted secrets via:

```sh
sops exec-env infra/gitops/dev/secrets/<svc>.enc.yaml -- mise run -C services/<svc> run
```

The wrapper task in each service's `.mise.toml` makes this a single `mise run run` invocation.

### Key lifecycle

- **Onboarding:** the new engineer runs `age-keygen`, opens a PR adding the public key to `.sops.yaml`, and runs `sops updatekeys` on all encrypted files. The PR diff is the audit trail.
- **Offboarding:** the leaving engineer's public key is removed from `.sops.yaml`, `sops updatekeys` runs on all files, and every secret that engineer had access to is rotated. The rotation is non-optional regardless of departure circumstances; it is the standing policy.
- **Cluster-key rotation:** a new cluster key pair is generated, the public key is added as an additional recipient to env-scoped files via `sops updatekeys`, the in-cluster Secret is updated, and after one full sync cycle the old key is removed.
- **Ops-recovery key rotation:** generated fresh annually as part of the security review; old key destroyed.

### Backups

The encrypted files are in git and inherit git's distribution. The age private keys are not:

- Engineer private keys are personal; loss requires the onboarding flow to re-add a new key.
- Cluster private keys are stored in the cluster *and* backed up encrypted-by-ops-recovery to the same off-cluster bucket used in [ADR-0003](0003-cluster-topology.md).
- Ops-recovery private key copies are held offline by 2–3 senior engineers (laptops + hardware tokens).

## Consequences

### Positive

- Secrets are versioned in git like everything else; no separate state store to operate or back up.
- Local–prod parity is exact: the same SOPS-encrypted file is decrypted by the engineer and by the cluster.
- Onboarding is a PR. Offboarding is a PR plus a rotation runbook.
- No additional stateful service to operate (no Vault, no Infisical, no Bitwarden).
- The recipient model — engineer, cluster, ops-recovery — is small enough to keep in one's head and fits naturally onto access reviews.

### Negative / Risks

- **Offboarding requires re-encryption of every file and rotation of every secret.** Mitigated by automating both with `scripts/secrets-rotate.sh`.
- **Lost engineer private key without backup means lost decrypt access for that engineer.** Acceptable; the engineer regenerates a key, PRs the new public key, gets re-onboarded.
- **A compromised cluster key exposes that environment's secrets in any subsequent git access.** Mitigated by the cluster-key-rotation procedure and by limiting cluster keys' recipient scope to env-scoped files.
- **The ops-recovery key is a high-value target.** Mitigated by offline storage with hardware tokens and an annual rotation.

### Follow-ups

- `infra/helm/platform/sops-operator/` Helm chart.
- `.sops.yaml` at repo root with creation rules and the initial recipient list.
- `scripts/secrets-rotate.sh` for offboarding and bulk rotation.
- `scripts/bootstrap-cluster-sops-key.sh` (called from the Ansible bootstrap role).
- `docs/secrets/runbook.md` covering onboarding, offboarding, cluster-key rotation, and ops-recovery procedures.

## Rules

- Plaintext secret values do not appear in any file committed to the repo. CI fails on patterns that look like high-entropy values.
- All committed secrets are SOPS-encrypted with age recipients listed in `.sops.yaml`.
- Every encrypted file has exactly three recipient classes: per-engineer keys, the matching env's cluster key, the ops-recovery key.
- Age private keys are not stored in shared services. Engineer keys live on engineer laptops; cluster keys live only in the cluster they belong to.
- Service Helm values reference secrets by Kubernetes Secret name. Services do not call SOPS or age at runtime.
- Onboarding adds the engineer's public key via PR plus `sops updatekeys`. Offboarding removes it via PR plus `sops updatekeys` plus rotation of every secret the engineer had access to.
- Rotation on offboarding is mandatory, not discretionary, regardless of the circumstances of departure.
- The ops-recovery private key is never online and never on a single machine; it is held offline by 2–3 senior engineers and rotated annually.
- Engineers do not run `kubectl create secret` ad-hoc. Every cluster Secret is produced by the sops-operator from an encrypted file in the repo.

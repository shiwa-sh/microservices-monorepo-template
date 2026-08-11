# ADR-0202: Secrets Management

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0000](0000-platform-foundations.md), [ADR-0003](0003-naming-and-identifiers.md), [ADR-0101](0101-monorepo.md), [ADR-0200](0200-cluster-topology.md), [ADR-0201](0201-gitops.md), [ADR-0205](0205-environment-parity.md)

## Context

Every environment needs secrets: database passwords, JWT signing keys, OAuth client secrets, API tokens. They must be versioned alongside the rest of the configuration so a deploy is reproducible from git, never readable in plaintext by anyone outside the recipient list, decryptable in-cluster by the GitOps controller at sync time without a human in the loop, decryptable locally by engineers running against real infrastructure, and rotatable with a known procedure for compromise and offboarding.

## Decision drivers

1. **Self-hosted and open source.** No managed service in the secret path ([ADR-0000](0000-platform-foundations.md), principle 3).
2. **GitOps-native.** Secrets reach the cluster the same way as everything else: a git commit reconciled by Argo CD.
3. **One mechanism, every environment**, local included ([ADR-0205](0205-environment-parity.md)).
4. **No additional stateful component** if it can be avoided ([ADR-0000](0000-platform-foundations.md), principle 2).

## Considered options

| Option | Added stateful component | An engineer can decrypt locally | Diff granularity | Verdict |
| --- | --- | --- | --- | --- |
| **SOPS + age** | none — the operator is stateless | **yes**, with that engineer's own key | per-value: structure stays readable, values are encrypted blobs | **Chosen.** The only option that holds drivers 3 and 4 together |
| SOPS + GPG | none | yes | per-value | Key-server and web-of-trust ceremony, and a multi-line key format. age has a single-line public key and no ceremony |
| sealed-secrets | a controller holding a key it generates, backs up, and rotates | **no — the encryption is one-way by design** | per-value: `encryptedData` is a per-key map. Ciphertext is non-deterministic, so re-sealing rewrites untouched values too | Fails driver 3. An engineer cannot read a committed secret or run against one, so local and cluster need different mechanisms |
| Vault or OpenBao | **a service to run, unseal, back up, and upgrade** | through the service | n/a | Fails driver 4. Its real prize is dynamic short-lived credentials, which is a separate question from storing static config |
| External Secrets Operator | a controller **plus a store to back it** | through that store | n/a | Driver 1 is satisfiable — OpenBao and Infisical self-host. It fails driver 4 harder than the row above: the same stateful store, and a sync controller on top |
| A provider secret manager, synced in | none in-cluster beyond a controller | through the provider | n/a | Fails driver 1 outright: the secret path leaves infrastructure we control, and the provider becomes a bootstrap dependency |
| Uncommitted `kubectl create secret` — the honest baseline | none | n/a — nothing is committed | n/a — the values are not in git | An environment stops being reproducible from the repo, and each value survives only in the shell history of whoever created it |

## Decision

### SOPS with age recipients

[`sops`](https://github.com/getsops/sops) encrypts at the key level, so values appear in diffs as encrypted blobs while structure stays reviewable. [`age`](https://github.com/FiloSottile/age) supplies the recipients.

### Three recipient classes

Every encrypted file has exactly three, declared in `.sops.yaml` at the repo root.

| Class | Naming | Private key lives | Purpose |
| --- | --- | --- | --- |
| **Per-engineer** | the engineer's `{handle}` ([ADR-0003](0003-naming-and-identifiers.md)), e.g. `eng_alice`, so every key traces to a person | `~/.config/sops/age/keys.txt` on that laptop, never leaving it | Day-to-day access. Scoped by project and handle, not by environment |
| **Per-cluster** | `{project}-{env}` | only in that cluster, as a Secret in the `sops` namespace, materialised at bootstrap | In-cluster decryption |
| **Ops-recovery** | one key | offline, on the hardware tokens of more than one senior engineer | Recovering a lost cluster key without re-encrypting every secret. Disaster recovery only |

`.sops.yaml` declares creation rules per path: files under `infra/gitops/platform/<env>/secrets/` are encrypted to that environment's cluster key plus engineers plus ops-recovery, and files outside an env path to engineers plus ops-recovery only.

Those per-env files reach the cluster through the `secrets` ApplicationSet at sync-wave 1 — after the base-tier operator, before the data tier that consumes the Secrets ([ADR-0201](0201-gitops.md)).

### In-cluster decryption

[**sops-secrets-operator**](https://github.com/isindir/sops-secrets-operator) watches `SopsSecret` custom resources holding SOPS-encrypted values and produces native Kubernetes `Secret` objects. Argo CD reconciles the encrypted file with the rest of the manifests, the operator decrypts and creates the `Secret`, and the pod consumes it through standard `envFrom` or `volumeMounts`.

Service authors reference secrets by Kubernetes Secret name in Helm values, exactly as they would any other Secret. The encryption layer is invisible to the service.

### Local decryption

`mise run secrets:age` once, after which `cluster:base` and any `sops decrypt` work without further configuration. Running against a real environment's secrets:

```sh
sops exec-env infra/gitops/platform/dev/secrets/platform.enc.yaml -- mise run -C services/<svc> server
```

The inner loop needs no decryption at all: each service's `.env.example`, copied to `.env` and loaded by its `.mise.toml`, carries local development credentials, so `mise run server` stands alone.

### Key lifecycle

| Event | Procedure |
| --- | --- |
| Onboarding | the engineer runs `mise run secrets:age`, opens a PR adding the public key to `.sops.yaml`, and runs `sops updatekeys` on all encrypted files. The PR diff is the audit trail |
| Offboarding | remove the public key, run `sops updatekeys`, and **rotate every secret that engineer could read**. Rotation is standing policy regardless of the circumstances of departure |
| Cluster-key rotation | generate a new pair, add the public key as an additional recipient on env-scoped files, update the in-cluster Secret, and remove the old key after one full sync cycle |
| Ops-recovery rotation | generated fresh annually as part of the security review; the old key is destroyed |

### Backups

Encrypted files live in git and inherit git's distribution. The private keys do not.

| Key | Backup |
| --- | --- |
| Engineer | none — personal. Loss means re-running the onboarding flow with a new key |
| Cluster | backed up encrypted-to-ops-recovery in the same off-cluster bucket [ADR-0200](0200-cluster-topology.md) uses |
| Ops-recovery | offline copies on the hardware tokens of more than one senior engineer, so a single departure does not lose recovery |

## Consequences

### Positive

- Secrets are versioned in git like everything else. There is no separate state store to operate, back up, or upgrade.
- Local and production parity is exact: the same encrypted file is decrypted by an engineer and by the cluster.
- Onboarding is a PR; offboarding is a PR plus a rotation runbook.
- Three recipient classes is small enough to hold in one's head and maps directly onto an access review.

### Negative / Risks

- **Offboarding requires re-encrypting every file and rotating every secret.** The scope of the work is set by how many secrets one engineer could read, which is every secret in the repo. The [secrets runbook](../secrets/runbook.md) carries the steps.
- **A lost engineer key loses that engineer's access.** Acceptable: regenerate, PR the new public key, re-onboard.
- **A compromised cluster key exposes that environment's secrets on any later git access.** Mitigated by the rotation procedure and by limiting cluster keys to env-scoped files.
- **The ops-recovery key is a high-value target.** Mitigated by offline storage on hardware tokens and annual rotation.
- **Credentials are static.** Rotation is a procedure rather than an expiry. Dynamic short-lived credentials are the one thing the rejected options buy, and adopting them is a separate decision with its own operational cost.

## Rules

- Plaintext secret values do not appear in any committed file.
- All committed secrets are SOPS-encrypted to age recipients listed in `.sops.yaml`.
- Every encrypted file has exactly three recipient classes: per-engineer keys, the matching environment's cluster key, and the ops-recovery key.
- Age private keys are not stored in shared services. Engineer keys live on laptops; cluster keys live only in the cluster they belong to.
- Service Helm values reference secrets by Kubernetes Secret name. Services do not call SOPS or age at runtime.
- Onboarding adds a public key by PR plus `sops updatekeys`. Offboarding removes it by PR plus `sops updatekeys` plus rotation of every secret that engineer could read.
- Rotation on offboarding is mandatory regardless of the circumstances of departure.
- The ops-recovery private key is never online and never on a single machine, and is rotated annually.
- Every cluster Secret is produced by the sops-operator from an encrypted file in the repo. `kubectl create secret` is not used.

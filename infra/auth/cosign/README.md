# The image-signing key pair (ADR-0104)

`cosign.pub` is committed and **empty in the template**. `signing-key.enc.yaml` does
not exist yet. Both are created by one command, once, at bootstrap:

```sh
mise run secrets:cosign
```

It refuses while `.sops.yaml` still carries placeholder recipients, because a
signing key encrypted to a placeholder is a signing key nobody can decrypt.

## Why the public key is a committed file rather than a values field

Every environment's Kyverno policy names this one file, delivered as a Helm
`fileParameter`. One platform key pair, one copy of it, one thing to review — a
public key pasted into three environment values files is three things that can
disagree, and the one that disagrees is the one that stops verifying.

It is committed **empty** rather than absent because the `fileParameter` is
declared for every application in its tier: a path that does not exist fails them
all, not just this one. Empty renders no policy, which is the honest state of a
platform that has published nothing yet.

## Why the private half is not here

It is at `signing-key.enc.yaml`, SOPS-encrypted to the engineers, ops-recovery, and
the CI identity — and to no cluster key. Kyverno verifies with the public half, so
no cluster ever needs the private one, and a secret delivered to a cluster is a
secret that cluster can be made to read.

CI decrypts it with an age key held as a forge secret (`SOPS_AGE_KEY_CI`), signs,
and the key exists as a file only inside a directory `scripts/ci-sign.sh` removes on
the way out.

## Rotation is not this command

Rotation has an ordering constraint: the policy must trust both public keys through
the window, or the cluster loses the ability to restart images it is already
running. The six steps are in [`../../../docs/guide/secrets-runbook.md`](../../../docs/guide/secrets-runbook.md).

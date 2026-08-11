# Secrets runbook

How-to for managing secrets. The decision (SOPS + age, sops-operator, secrets in Git encrypted) is [ADR-0202](../adr/0202-secrets.md); this is the operational procedure.

## Model

- Secrets are committed **encrypted** with SOPS + age; the private age key never enters Git. The in-cluster sops-operator decrypts them into Kubernetes Secrets ([ADR-0202](../adr/0202-secrets.md)).
- No secret is ever committed in plaintext, and no secret is set by clicking in a UI ([ADR-0000](../adr/0000-platform-foundations.md) principle 3).

## Generate / rotate the age key

```sh
mise run secrets:age            # generates the local age key material
```

Locally the key is a throwaway planted at bootstrap ([ADR-0205](../adr/0205-environment-parity.md)). In a deployed environment the age private key is provisioned to the cluster out-of-band and is the root of trust for decryption — treat its loss as a full secret-rotation event.

## Edit a secret

1. Decrypt in place with SOPS, edit, re-encrypt (SOPS does this transactionally on save).
2. Commit the encrypted file. ArgoCD + sops-operator reconcile it into a Kubernetes Secret ([ADR-0201](../adr/0201-gitops.md)).
3. Never paste the decrypted value into a chart values file — the auth-config single-source lint (`mise run lint:auth-inline`) and review guard against inlined secrets.

## Rotate a leaked secret

1. Change the underlying credential at its source (DB password, bucket key, etc.).
2. Update and re-encrypt the SOPS file; commit.
3. Roll the consuming workloads so they pick up the new Kubernetes Secret.
4. Rotate the age key too if the private key itself may be exposed.

## Break-glass

Recovering the cluster when the auth plane is down is [break-glass](break-glass.md); sealing local-admin creds in SOPS is the optional secondary break-glass described there.

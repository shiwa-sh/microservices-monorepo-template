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

## Rotate the image-signing key

The cosign key pair signs every first-party image, and Kyverno verifies it at admission ([ADR-0104](../adr/0104-supply-chain-security.md)). Rotating it is the one rotation with an ordering constraint, because the policy that trusts the key is also the policy that blocks every deploy when it is wrong.

**The policy carries both public keys through the window.** Images signed with the old key are already running and will be re-admitted on any reschedule, so removing the old key before those images are gone locks the cluster out of restarting its own workloads.

1. **Generate the new pair** and SOPS-encrypt the private half to the cluster age key. Commit both halves — the public one in plaintext, the private one encrypted.
2. **Add the new public key to the `ClusterPolicy` as a second attestor**, and merge. The policy now accepts either. Confirm the Application is synced before going further.
3. **Point CI at the new private key** and merge. Every image built from here is signed with the new key.
4. **Rebuild and roll every running image.** This is the step with the wall clock in it — the window stays open until nothing signed by the old key can be scheduled, including anything a node replacement would pull.
5. **Remove the old public key from the policy** and merge.
6. **Delete the old private key** from the SOPS file.

**Do not compress steps 2 and 5.** A policy that trusts only the new key while old-signed images are still schedulable turns a rotation into a cluster-wide admission failure, which is [`../reference/risk-register.md`](../reference/risk-register.md) row 5 arriving on purpose. The break-glass, if it happens anyway, is the kubeconfig path below plus removing the webhook configuration.

**If the private key may be exposed**, the window is not negotiable and steps 4 and 5 are the incident. Until step 5 lands, an attacker holding the old key can sign an image the cluster admits.

## Break-glass

Recovering the cluster when the auth plane is down is [break-glass](break-glass.md); sealing local-admin creds in SOPS is the optional secondary break-glass described there.

# prod platform secrets

This directory is synced by the `prod-secrets` Application (the `secrets` ApplicationSet, [ADR-0201](../../../../../docs/adr/0201-gitops.md), sync-wave 1). The base-tier sops-operator reconciles the `SopsSecret` CR here into native Kubernetes Secrets that the data + core platform tiers consume ([ADR-0202](../../../../../docs/adr/0202-secrets.md)).

**The template ships this README, not the secret.** A template repo cannot carry real cluster secrets, and the `prod` cluster key in `.sops.yaml` is a placeholder. Until you add `platform.enc.yaml` below, the Application syncs to zero resources (Healthy, empty) and the platform charts that reference these Secrets stay in `CreateContainerConfigError`.

## Adopt

1. Generate the `prod` cluster age key and replace the `cluster_prod` placeholder in `.sops.yaml` with its public half; plant the private half in-cluster (see `docs/secrets/runbook.md` / the Ansible bootstrap role).
2. Copy the skeleton below to `platform.enc.yaml`, fill in real values, and encrypt it in place: `sops --encrypt --in-place infra/gitops/platform/prod/secrets/platform.enc.yaml`. The `.sops.yaml` rule for this path encrypts only `data`/`stringData` values (the CR structure stays readable) to `cluster_prod` + engineers + ops-recovery.
3. Commit. Argo delivers it and the operator materialises the Secrets.

## `platform.enc.yaml` skeleton

```yaml
apiVersion: isindir.github.com/v1alpha3
kind: SopsSecret
metadata:
  name: platform
  namespace: platform
spec:
  secretTemplates:
    - name: observability-bucket
      stringData:
        AWS_ACCESS_KEY_ID: ""
        AWS_SECRET_ACCESS_KEY: ""
    - name: postgres-superuser
      type: kubernetes.io/basic-auth
      stringData:
        username: ""
        password: ""
    - name: temporal-db-creds
      stringData:
        password: ""
    - name: openfga-creds
      stringData:
        preshared_key: ""
        datastore_uri: ""
    - name: catalog-db
      stringData: { DATABASE_URL: "" }
    - name: orders-db
      stringData: { DATABASE_URL: "" }
    - name: orgs-db
      stringData: { DATABASE_URL: "" }
    - name: payment-db
      stringData: { DATABASE_URL: "" }
    - name: kratos-secrets
      stringData:
        secretsDefault: ""
        secretsCookie: ""
        secretsCipher: ""
        dsn: ""
        smtpConnectionURI: ""
```

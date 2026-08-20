# staging platform secrets

This directory is synced by the `staging-secrets` Application (the `secrets` ApplicationSet, [ADR-0201](../../../../../docs/adr/0201-gitops.md), sync-wave 1). The base-tier sops-operator reconciles the `SopsSecret` CR here into native Kubernetes Secrets that the data + core platform tiers consume ([ADR-0202](../../../../../docs/adr/0202-secrets.md)).

**The template ships this README, not the secret.** A template repo cannot carry real cluster secrets, and the `staging` cluster key in `.sops.yaml` is a placeholder. Until you add `platform.enc.yaml` below, the Application syncs to zero resources (Healthy, empty) and the platform charts that reference these Secrets stay in `CreateContainerConfigError`.

## Adopt

1. Generate the `staging` cluster age key and replace the `cluster_staging` placeholder in `.sops.yaml` with its public half; plant the private half in-cluster (see `docs/guide/secrets-runbook.md` and `infra/talos/README.md` — on Talos it rides in the SOPS-encrypted machine config as an inline manifest, because there is no host filesystem to place it on).
2. **Replace the other two recipients of that path as well.** `.sops.yaml` gives it `eng_placeholder` and `ops_recovery` beside the cluster key, and sops encrypts to every recipient of a rule or to none — one remaining placeholder fails the encrypt with `malformed recipient`, naming the placeholder rather than the rule that pulled it in.
3. Copy the skeleton below to `platform.enc.yaml`, fill in real values, and encrypt it in place: `sops --encrypt --in-place infra/gitops/platform/staging/secrets/platform.enc.yaml`. The `.sops.yaml` rule for this path encrypts only `data`/`stringData` values (the CR structure stays readable) to `cluster_staging` + engineers + ops-recovery.
4. Commit. Argo delivers it and the operator materialises the Secrets.

## `platform.enc.yaml` skeleton

```yaml
apiVersion: isindir.github.com/v1alpha3
kind: SopsSecret
metadata:
  name: platform
  namespace: platform
spec:
  secretTemplates:
    # The DNS-01 solver's credential (ADR-0205). Without it the public issuer
    # never becomes Ready and no wildcard certificate is ever issued.
    - name: cloudflare-api-token
      stringData:
        token: ""
    - name: observability-bucket
      stringData:
        AWS_ACCESS_KEY_ID: ""
        AWS_SECRET_ACCESS_KEY: ""
    # `username` has to be the role `cluster.initdb.owner` names in the postgres
    # chart, and the same string every DSN below uses. Three places, one role.
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
        # Non-production delivers to the sink, never to a recipient (ADR-0307):
        #   smtp://mailpit.platform.svc.cluster.local:1025/?disable_starttls=true
        # Leaving this empty falls back to the chart placeholder, which is a real
        # relay host — the one outcome the Rule forbids.
        smtpConnectionURI: ""
```

# prod platform secrets

This directory is synced by the `prod-secrets` Application (the `secrets` ApplicationSet, [ADR-0201](../../../../../docs/adr/0201-gitops.md), sync-wave 1). The base-tier sops-operator reconciles the `SopsSecret` CR here into native Kubernetes Secrets that the data + core platform tiers consume ([ADR-0202](../../../../../docs/adr/0202-secrets.md)).

**The template ships this README, not the secret.** A template repo cannot carry real cluster secrets, and the `prod` cluster key in `.sops.yaml` is a placeholder. Until you add `platform.enc.yaml` below, the Application syncs to zero resources (Healthy, empty) and the platform charts that reference these Secrets stay in `CreateContainerConfigError`.

## Adopt

1. Generate the `prod` cluster age key and replace the `cluster_prod` placeholder in `.sops.yaml` with its public half; plant the private half in-cluster (see `docs/guide/secrets-runbook.md` and `infra/talos/README.md` — on Talos it rides in the SOPS-encrypted machine config as an inline manifest, because there is no host filesystem to place it on).
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
    # The read-only inspector role (ADR-0401). CNPG reconciles this password onto
    # the `readonly` role the cluster creates at bootstrap; pgweb connects as it,
    # so the inspector cannot write even if its own flag is lost.
    - name: postgres-readonly
      type: kubernetes.io/basic-auth
      stringData:
        username: readonly
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
        # Production submits through maddy (ADR-0307), so this is a real
        # submission endpoint. Mailpit is not deployed here.
        smtpConnectionURI: ""
    # The registry's push and pull credentials, and the store credential it reads
    # and writes S3 with (ADR-0105). `htpasswd` holds one BCRYPT line per identity
    # — an apr1 file loads without complaint and then refuses every password.
    #
    #   htpasswd -nbB ci <push-password>
    #   htpasswd -nbB cluster <pull-password>
    - name: zot-credentials
      stringData:
        AWS_ACCESS_KEY_ID: ""
        AWS_SECRET_ACCESS_KEY: ""
        htpasswd: ""
    # maddy's DKIM private key (ADR-0307). Its public half goes in the TXT record
    # at `<selector>._domainkey.mail.example.com`, and the two must be generated
    # together: maddy MINTS a key when this is absent, which yields an agent that
    # signs every message with a key no DNS record matches.
    - name: maddy-dkim
      stringData:
        private.key: ""
    # The submission credential Kratos, the services and Alertmanager authenticate
    # with. The hash, not the password — this value reaches maddy's config file.
    #
    #   maddy hash --password <submission-password>
    - name: maddy-submission
      stringData:
        password_hash: ""
```

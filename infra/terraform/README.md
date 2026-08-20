# Terraform — per-project, not day one

The template's day-one path assumes **pre-provided nodes** (ADR-0200): the machine configs under `infra/talos/` are applied to hosts already running Talos, named in a committed inventory (`infra/talos/inventory/<env>/nodes.yml`). There is **no default Terraform run**, and no bucket is Terraform-created — Loki/Tempo durability and CNPG backups point at an existing S3-compatible bucket by endpoint + credentials (via SOPS-decrypted Secrets, ADR-0202).

`terraform` stays available as a latent tool in `.mise.toml`. A project that provisions its **own** infrastructure adds a provider module here and wires it in:

```text
infra/terraform/
  modules/<provider>/   # e.g. hetzner, aws, gcp — the machines + network + DNS
  environments/<env>/   # backend config + a module block per environment
```

A project that provisions its own infrastructure also gains the first-party
[`siderolabs/talos`](https://registry.terraform.io/providers/siderolabs/talos/latest)
provider, which applies the machine configs and bootstraps the cluster as part of
the plan. That is the difference the two modes actually make: not a different
cluster, but machine-config apply as a planned resource rather than a command
someone runs.

Either way the node addresses end up in `infra/talos/inventory/<env>/nodes.yml`,
which is what `talosctl` and `lint:naming` both read.

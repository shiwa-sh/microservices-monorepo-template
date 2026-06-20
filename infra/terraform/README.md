# Terraform — per-project, not day one

The template's day-one path is **Ansible-first** (ADR-0003): `infra/ansible/bootstrap.yml`
runs against a committed inventory of pre-provided hosts
(`infra/ansible/inventory/<env>/hosts.yml`). There is **no default Terraform run**, and
no bucket is Terraform-created — Loki/Mimir/Tempo durability and CNPG backups point at an
existing S3-compatible bucket by endpoint + credentials (via SOPS-decrypted Secrets,
ADR-0005).

`terraform` stays available as a latent tool in `.mise.toml`. A project that provisions
its **own** infrastructure adds a provider module here and wires it in:

```text
infra/terraform/
  modules/<provider>/   # e.g. hetzner, aws, gcp — the machines + network + DNS
  environments/<env>/   # backend config + a module block per environment
```

After `terraform apply`, copy the node IPs into the matching
`infra/ansible/inventory/<env>/hosts.yml` and run `bootstrap.yml` as usual.

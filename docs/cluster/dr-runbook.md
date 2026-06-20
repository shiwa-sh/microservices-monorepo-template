# Disaster recovery runbook (ADR-0003)

Target RTO: 30 min. RPO ≈ WAL archive interval (minutes).

## Detection

Uptime Kuma pages on-call within 1-2 minutes of total cluster loss.

## Recovery

```sh
# 1. Provision a new node set — ONLY if the project provisions its own infra.
#    On pre-provided infrastructure the hosts already exist; skip to step 2.
cd infra/terraform/environments/<env>
terraform apply

# 2. Bootstrap k3s + the in-cluster age key (against the Terraform-produced
#    hosts, or the committed inventory of pre-provided hosts).
cd ../../../ansible
ansible-playbook -i inventory/<env> bootstrap.yml

# 3. Re-install ArgoCD's root Application; ArgoCD reconciles everything else.
kubectl apply -f infra/gitops/bootstrap/root-application.yaml

# 4. CNPG restores Postgres from PITR in the external bucket. Wait for the
#    `Cluster` CR to report `Phase: Cluster in healthy state`.
kubectl -n platform get cluster postgres -w
```

Rehearsed quarterly against a staging rebuild, tracked as a Temporal `Schedule`.

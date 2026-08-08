# GitOps runbook

How to operate the Argo CD deploy path. The decision is [ADR-0201](../adr/0201-gitops.md); this is the procedure.

## Model

- Argo CD reconciles every environment from `master`. **A working-tree change is invisible in-cluster until it is pushed.**
- `selfHeal` reverts direct `kubectl` and `helm` edits, so a change must land on `master` to persist. The two exceptions — `platform:deploy` and a branch `targetRevision` — are in [ADR-0600](../adr/0600-local-development-loop.md).
- Configuration is files in this repo; nothing is set by clicking in the Argo UI ([ADR-0000](../adr/0000-platform-foundations.md), principle 1).

## Deploy a change

1. Merge to `master`.
2. Argo CD detects and syncs it. Watch at `argocd.ops.<host>` ([ADR-0306](../adr/0306-trust-tiers-and-urls.md)) or with `kubectl get applications -n argocd`.

## Add an alert or a dashboard

Both are files reconciled by their own Application ([ADR-0500](../adr/0500-observability.md)).

| Artefact | Location | Extra step |
| --- | --- | --- |
| Prometheus alert rule | `infra/observability/alerts/*.yaml` | **add the file to that directory's `kustomization.yaml`**, or it is not shipped and nothing reports it |
| Grafana dashboard | `infra/observability/dashboards/*.json` | add the file to the dashboards `kustomization.yaml` |

A firing alert appears on the Prometheus alerts page, as the `ALERTS` series, and at whichever Alertmanager receiver its `severity` selects ([ADR-0502](../adr/0502-alerting-and-on-call.md)). A rule without a `severity` label is a defect. Nothing pages anyone: no escalation receiver is attached.

## Bring up the full platform locally

```sh
mise run cluster:full     # the real charts at single replica, via Argo CD from master
```

This is the mechanism the deployed clusters use ([ADR-0600](../adr/0600-local-development-loop.md)). To exercise **uncommitted** GitOps wiring, see [gitops-local](../cluster/gitops-local.md).

## Diagnose a stuck sync

- `kubectl get applications -n argocd` — find the app that is OutOfSync or Degraded.
- `kubectl describe application <name> -n argocd` — read its events and conditions.
- CRD → operator → instance ordering is handled by sync waves. A first-run platform sync is slow by design.
- Every Application carries a `retry` policy, so a transient error self-heals. A manual `argocd app sync` is the break-glass once retries exhaust; a cluster rebuild is never the remedy.
- If a node's containerd wedges on a large image pull, recover with `mise run cluster:heal` ([dev-loop](../dev-loop.md)) rather than by restarting the node.

## Break-glass

When the auth plane gating the Argo UI is down, reach it by `kubectl port-forward` ([break-glass](../ops/break-glass.md)).

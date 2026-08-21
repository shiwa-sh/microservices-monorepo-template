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

| Artefact | Location | What to check before merging |
| --- | --- | --- |
| Prometheus alert rule | `infra/observability/alerts/*.yaml` | the rule carries a `severity` of `page` or `ticket` ([ADR-0502](../adr/0502-alerting-and-on-call.md)) |
| Grafana dashboard | `infra/observability/dashboards/*.json` | the `uid` is stable, because links and the funnel depend on it ([ADR-0501](../adr/0501-operator-uis-and-dashboards.md)) |

Each directory is a chart reconciled by its own Application, and its template globs the directory into the ConfigMap the consumer mounts, so **adding the file is the whole step** — there is no index to update. A chart rather than a plain directory source because neither artefact is a Kubernetes manifest: a dashboard is Grafana JSON and a rule file is native Prometheus format, so both have to be generated into a ConfigMap rather than applied. Kustomize is not used anywhere ([ADR-0201](../adr/0201-gitops.md)). The failure mode this shape has instead: one malformed file fails the render for the whole directory, and the Application's condition names the chart.

A firing alert appears on the Prometheus alerts page, as the `ALERTS` series, and at whichever Alertmanager receiver its `severity` selects. A rule without a `severity` label is a defect. Nothing pages anyone: no escalation receiver is attached.

## Silence an alert

Silences are configuration, so they live in the repository like everything else ([ADR-0000](../adr/0000-platform-foundations.md), principle 1). A silence created in the Alertmanager UI is invisible to review and vanishes on the next reconcile. The `alertmanager-silence-sync` CronJob is what makes that last clause true: it reconciles the live silences to the committed file every ten minutes, so a UI silence lasts until the next run and no longer.

1. Add the matcher to `infra/observability/alertmanager/silences.yaml`.
2. **Set `endsAt` to an explicit timestamp.** A silence with no end is how an alert stops existing without anyone deciding it should.
3. Put the reason in the comment above the matcher — what is being suppressed, and what would make it safe to remove.
4. Merge. Argo CD reconciles it like any other file.

An expired silence is deleted from the file rather than extended. Extending one twice is the signal that the rule underneath it is wrong: either the threshold is set where it fires without a human action, in which case demote it to `ticket`, or the condition is genuinely acceptable, in which case the rule is deleted and the reason recorded in the owning ADR.

## Bring up the full platform locally

```sh
mise run cluster:full     # the real charts at single replica, via Argo CD from master
```

This is the mechanism the deployed clusters use ([ADR-0600](../adr/0600-local-development-loop.md)). To exercise **uncommitted** GitOps wiring, see [gitops-local](gitops-local.md).

## Diagnose a stuck sync

- `kubectl get applications -n argocd` — find the app that is OutOfSync or Degraded.
- `kubectl describe application <name> -n argocd` — read its events and conditions.
- CRD → operator → instance ordering is handled by sync waves. A first-run platform sync is slow by design.
- Every Application carries a `retry` policy, so a transient error self-heals. A manual `argocd app sync` is the break-glass once retries exhaust; a cluster rebuild is never the remedy.
- If a node's containerd wedges on a large image pull, recover with `mise run cluster:heal` ([dev-loop](../dev-loop.md)) rather than by restarting the node.

## When the symptom names the wrong layer

Cluster faults in this platform have a habit of announcing themselves as something
unrelated, because the component that *reports* the error is rarely the one that is
broken. Each of these was diagnosed the long way at least once.

| What you see | What it usually is |
| --- | --- |
| Cilium agents fatal on `Unable to find all Cilium CRDs` | The **operator cannot schedule**. Check node taints — `disk-pressure` is the common one, and on a laptop the node's "image filesystem" is your whole disk |
| Helm hangs in pre-install, Events list empty | **Nothing can schedule at all.** An all-control-plane cluster taints every node; `allowSchedulingOnControlPlanes` is the fix |
| Pods time out on `10.96.0.1:443`, Cilium reports healthy | **RBAC or the dataplane**, not the API server. `cilium-dbg` reports the agent's own view and will say `KubeProxyReplacement: True` and `Cluster health: 3/3` while no pod can reach a Service |
| etcd stuck `Preparing`, waiting on a service that is Running | The **etcd spec is missing**. The waiting list is static and names every precondition, not the unmet one. A bootstrap issued before a post-apply reboot writes a spec that dies with the old instance |
| Pod `ContainerCreating` with **no pull events at all** | A **volume**, not an image. A missing secret or a hostPath the kubelet was never given mounts forever without an error |
| `403 Forbidden` pulling `registry.k8s.io/etcd`, cluster never bootstraps | A **proxy**, and the machine config does not carry it ([`http-proxy`](http-proxy.md)) |
| An admission webhook times out, and other webhooks work | The webhook pod **predates the current CNI**. Replacing the CNI leaves the endpoints of already-running pods stale, and only the pods nothing has restarted since are affected. Roll the deployment |
| A replica join retries forever with `no route to host` | **Network policy**, not routing. `no route to host` is what a Cilium drop looks like to the caller; `hubble observe --verdict DROPPED` names the rule's absence in one line |
| An Application reports **OutOfSync while `argocd app diff` prints nothing** | A **mutating webhook** and a client-side diff. The live object carries fields no chart rendered, so the diff never converges — and because a tier gates on every Application being Synced, one such resource holds every later wave shut. `controller.diff.server.side` ([ADR-0201](../adr/0201-gitops.md)) |
| Applications look Synced but are hours **stale**, and a refresh annotation is never consumed | The **application controller is restarting**, usually OOM. `status.reconciledAt` falling behind is the tell; every Application keeps its last status while nothing reconciles |
| An admission webhook times out **intermittently**, and works after a restart | The API server reaches a pod from its own node's address, which Cilium classifies as `host` or `remote-node` — **never** `kube-apiserver`. A policy admitting only that entity works exactly when the webhook shares a node with the caller |
| Every ops panel answers **404** while the routes exist | Traefik **drops a router whose middleware cannot be built**. The reason appears only in Traefik's log — `kubernetes service not found: platform/edge-errors` — and the panel looks unrouted rather than unbacked |
| A registry push retries layers forever with `unexpected EOF` | A **timeout on one of the hops**. An image push is the longest request the edge carries; whichever of Traefik or the registry cuts first decides, so both are set together ([ADR-0105](../adr/0105-image-registry.md)) |

**The habit that saves the most time: verify the dataplane with a pod.**

```sh
kubectl run t --rm -i --restart=Never --image=docker.io/library/busybox:1.37 -- \
  sh -c 'wget -T10 -O/dev/null https://10.96.0.1:443/version
         nslookup kubernetes.default.svc.cluster.local'
```

A 401 from the API means the connection worked — that is success, not failure.
Resolution failing is the signal that Service routing is dead, whatever every
component's own health endpoint claims.

Both halves of that image reference are load-bearing, and admission rejects the
short forms this command is usually written with. `busybox` is denied because the
allow-list holds repositories in registry-qualified form, and an untagged
`docker.io/library/busybox` is denied because every entry demands a tag or a
digest — an implicit `latest` is exactly what ADR-0104 exists to reject. Query the
**fully qualified** Service name: a short name is resolved through the pod's search
list, so a working cluster still prints one `NXDOMAIN` per unmatched suffix first,
which reads as a DNS failure and is not one.

A namespace under the `restricted` Pod Security profile needs the four fields
`kubectl run` omits (`runAsNonRoot`, `allowPrivilegeEscalation: false`,
`capabilities.drop: [ALL]`, `seccompProfile`), so there apply a Pod manifest that
carries them instead.

## Break-glass

When the auth plane gating the Argo UI is down, reach it by `kubectl port-forward` ([break-glass](break-glass.md)).

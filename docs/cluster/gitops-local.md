# The full tier runs through ArgoCD locally (ADR-0004, ADR-0016)

`mise run cluster:full` **is** the ArgoCD path. It creates the cluster, installs
the two components ArgoCD cannot bootstrap (the CNI and ArgoCD itself), plants the
SOPS age key, then applies a local root-app (`infra/gitops/bootstrap-local/`) that
reconciles the same app-of-apps prod uses — sync waves, `ApplicationSet`
generators, secret materialisation and all — against committed **`master`** on the
remote. CI's end-to-end job and per-PR previews reuse this exact path.

So the everyday full tier already exercises ArgoCD; there is nothing to opt into.
What needs a little ceremony is testing **uncommitted** changes, because ArgoCD
reconciles a git ref, not your working tree.

## Local bootstrap layout

`infra/gitops/bootstrap-local/` is a sibling of `bootstrap/` — deliberately, so
the prod root-app's `directory.recurse` over `bootstrap/` never picks up the
local-only appsets. It contains a local root-app plus `local`-scoped copies of the
platform/services `ApplicationSet`s and the gateway/secrets apps. Cilium and ArgoCD
are excluded from the local platform appset (they are installed imperatively first).

## Testing uncommitted changes

### Service code → `service:deploy`

The fast path. Build your working-tree service into the cluster and let it override
the Argo-synced (CI-image) copy:

```sh
mise run service:deploy -- catalog     # build → k3d image import → helm upgrade (Argo auto-sync paused)
```

### Platform chart / values → `platform:deploy`

```sh
mise run platform:deploy -- ory        # working-tree helm upgrade, Argo auto-sync paused on that app
```

Re-enable GitOps for that app when done (the command prints the exact `kubectl
patch`), or just re-run `cluster:full`.

### GitOps wiring (sync waves, ApplicationSets, app defs) → a branch

`helm` cannot exercise the delivery path, so push a branch and point the local
root-app at it. This is the only change that genuinely needs a git round-trip:

```sh
git switch -c my-gitops-change
# edit infra/gitops/** ; commit ; push
git push -u origin my-gitops-change

# Point the local bootstrap at the branch (root-app + the appsets' revision/
# targetRevision all default to master), then bring the full tier up:
sed -i -E 's/(revision|targetRevision): master/\1: my-gitops-change/' \
  infra/gitops/bootstrap-local/*.yaml
mise run cluster:full
```

Revert the `sed` before merging — `master` is the committed default (choice a).
ArgoCD must be able to clone the repo; this template's repo is public, so no
credentials are needed locally.

## CNI / CRD-operator changes (e.g. Cilium)

Prefer `mise run cluster:delete` + a fresh `cluster:full` **with** the change over
an in-place upgrade — hot-swapping a CNI on a live cluster blips networking. This
is inherent to the component, not a tooling gap.

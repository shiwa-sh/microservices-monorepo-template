# Local development loop

How to run and debug services on your machine. The model is [ADR-0600](adr/0600-local-development-loop.md); ports, URLs, and the environment contract are [local-environment](reference/local-environment.md). Making your first change end to end, and the gates it passes, is [first-change](guide/first-change.md).

## Pick a mode first

Three. Every mode runs the real Cilium, Traefik, Kratos, and Oathkeeper on upstream Kubernetes over etcd, so the edge, the network policy, and the datapath are the same in all of them. What the top two modes trade away is a single node and the machine config: `cluster:full` boots the one a deployed environment boots.

| You are | Run | Costs | Cannot tell you |
| --- | --- | --- | --- |
| Changing one service's code | `mise run cluster:base`, then `mise run server` in the service | the floor, and seconds to restart the binary | how the service behaves under the charts CI builds, and anything needing more than one node — etcd has a single member here ([ADR-0600](adr/0600-local-development-loop.md)) |
| Building UI | `mise run cluster:base`, then `mise run dev:frontend` | the floor plus the mock | anything stateful — see *What this tier cannot tell you* |
| Verifying the whole system, or running e2e | `mise run cluster:full` | around 16 GB of RAM, and minutes | nothing above the host kernel. It is the deployed distribution and the deployed shape, at one replica |

Start at the top and move down only when the row you are on cannot answer the question. **Every mode logs in for real**: there is no bypass, no fake session, and no environment branch in the session path in any of them.

The two tiers are **two clusters**, each with its own `kubectl` context, so bringing one up leaves the other alone and `cluster:full` never costs you the inner loop you had running.

`mise run cluster:base` brings up the local **floor** — Cilium, Traefik, cert-manager, Postgres, Kratos, Oathkeeper — and everything above it is opt-in, declared by the service that needs it. The inner loop is **native execution**: you run the service you are changing directly on the host, with no image build, no in-cluster redeploy, and no file watch on the hot path.

**There is no cluster step to remember.** A service's own tasks declare what they need and mise resolves the graph, bringing up the floor plus exactly those components and skipping whatever is already Ready.

This file is editor-agnostic. Any IDE that loads a `.env` file and runs a Go `main.go` works the same way.

## One-time setup

```sh
mise run setup      # git hooks
```

Every service ships a `.env.example` listing the variables it reads, already pointing at the forwarded dependencies. Its `.mise.toml` loads `.env`, so `mise run server` needs no inline environment. `.env` is gitignored; `.env.example` is the tracked contract.

You do not copy it by hand. The first `mise run server` seeds `.env` from `.env.example` and stops, telling you to re-run — it stops because mise reads the file when it *loads* the config, before any task runs, so the run that creates `.env` cannot see it.

## Run one service

```sh
cd services/catalog
mise run server         # floor + this service's deps, then serves :8081
```

In their own terminals:

```sh
mise run dev:forward    # port-forward the deps to localhost — leave running
mise run db:migrate     # apply migrations to the local database
cd services/orders && mise run worker
```

Re-running the service is re-running the binary; there is nothing to rebuild. To debug, point your editor's Go run configuration at `services/<svc>/cmd/server/main.go` and have it load that service's `.env`. Breakpoints and hot-restart work because the service is a plain host process.

## When your service calls other services

Nothing to arrange. A service declares its callees the same way it declares Postgres, so the graph brings them up:

```sh
cd services/orders && mise run worker    # also deploys catalog and payment, forwarded
```

You are working on orders; catalog and payment are opaque.

## Debug a flow across several services

Run the ones you want breakpoints in **first**, natively:

```sh
cd services/catalog && mise run server   # terminal 1 — :8081
cd services/payment && mise run server   # terminal 2 — :8084
cd services/orders  && mise run worker   # terminal 3 — sees both answered, deploys neither
```

Same commands, same ports, no flags. A `svc:*` edge guards on **whether the port answers**, not on whether a deployment exists, so a callee you started yourself satisfies the dependency and the graph leaves it alone. Anything you did not start is deployed and forwarded as usual.

To go back to opaque callees, stop the native process and run `mise run cluster:remove -- catalog` to clear the leftover forward.

## Run a service behind the real edge

A service curled directly on its port is being handed **forged** identity headers. To run it natively behind the real Oathkeeper chain:

```sh
mise run service:dev -- catalog   # stamps the per-service edge glue
cd services/catalog && mise run server
```

That routes `https://dev.localtest.me:8443/api/products` to your host process through the middlewares the deployed service gets, so the identity headers are genuine. This is the class of bug that otherwise appears only after a deploy.

## Put a service in the cluster

When you need it running but are not editing it — a one-shot build, import, and deploy, with no watch loop:

```sh
mise run cluster:add -- catalog      # build → import → upgrade
mise run cluster:remove -- catalog   # uninstall AND restore Argo auto-sync
```

`cluster:add` also takes a platform chart, and brings up the service's declared dependency components first, so a deploy onto a bare floor does not CrashLoop on a missing Temporal. It follows `svc:*` edges too, since in-cluster callees resolve by DNS and must exist.

**Always prefer `cluster:remove` to a bare `helm uninstall`.** The add path pauses Argo auto-sync and removal is what restores it. Skip it and the service is both gone and unmanaged, so a later `cluster:full` will not bring it back.

## Build UI

The inner loop runs one service natively, and a frontend is nobody's single client — one panel screen fans out across four services. [ADR-0600](adr/0600-local-development-loop.md) closes that gap by inverting the usual split: **the edge and identity stack stay real, and the application services are replaced by their own OpenAPI contract.**

Both are already in the floor, so UI work is the floor plus the mock rather than a separate tier:

```sh
mise run cluster:base
mise run dev:frontend    # host :3000, reached through the edge
```

The mock is a vendored Prism container fed the committed `internal.json` projection, started with the floor. `mise run mock:start`, `mock:stop`, and `mock:logs` control it directly when you need to.

`dev:frontend` extracts the local CA from the cluster and trusts exactly it, so nothing disables TLS verification — the same mechanism the in-cluster image uses.

Open <https://dev.localtest.me:8443/> and **log in for real**. The task seeds the committed test identities, and a session lasts 7 days, so this is a weekly event rather than a per-run one. What you get is the production authentication path: real cookie, real CSRF, real expiry, real redirect on a gated route, and every `/api` call decided by the same access rules production runs.

The frontend carries **no** development-only auth code — no bypass, no fake session, no environment branch in the session path — and `lint:auth-inline` fails the build if one appears.

**What this tier cannot tell you.** The mock is stateless: a create is not reflected by the next read, a workflow handle polls nothing, and no authorization decision is real. Persistence, workflow behaviour, and authorization are asserted against real services on `cluster:full`, where the e2e suite runs. The mock is forbidden in `mise run test`, `ci:affected`, and the e2e and visual suites ([ADR-0601](adr/0601-testing-strategy.md)).

### The frontend as its production container

The daily loop is the host dev server above. To verify the Dockerfile, the standalone output, or a production-only rendering difference:

```sh
mise run cluster:add -- frontend
```

This runs the image CI builds, in production mode and with no TLS bypass. The glue catch-all route holds a lower priority, so the in-cluster frontend wins while it exists and the host dev server takes over again the moment you `cluster:remove` it.

## The full platform

```sh
mise run cluster:full
```

Stands up the **same charts production runs** at a single replica, **delivered by Argo CD** — the engine the deployed clusters use. It creates the cluster, installs the two components Argo cannot bootstrap (the CNI and Argo itself), plants the age key, builds and pushes the repo images to a local registry, then applies a local root app that syncs committed `master`. Ordering, readiness, and secret materialisation are Argo's job, not a shell script's.

Plan for around 16 GB of free RAM.

Because it syncs committed `master`, uncommitted **infra** needs a push — see [cluster/gitops-local](guide/gitops-local.md). For uncommitted **service** code use `cluster:add`.

**The local registry is the CI stand-in.** Production GitOps works because CI builds each image, pushes it, and Argo pulls it. Locally there is no CI, so `cluster:full` pushes to a local registry at a stable tag and the local overlay points at it — Argo then deploys exactly as production does, the only difference being the registry host. A registry is the only way an image reaches this tier: its nodes hold no image the cluster did not pull, which is also true of a deployed node.

One-time host setup: add `127.0.0.1 registry.localhost` to `/etc/hosts`, so the host `docker push` resolves to IPv4. A bare `*.localhost` resolves to IPv6 on some systems, which the registry does not listen on. This is machine setup, never in the repo — the same rule as [http-proxy](guide/http-proxy.md).

What may differ from production, and what may not, is [ADR-0205](adr/0205-environment-parity.md).

## Teardown

```sh
mise run cluster:stop     # stop, keeping the image cache and volumes
mise run cluster:delete   # delete, reclaiming disk and forcing a clean recreate
```

## Troubleshooting

| Symptom | Cause | Fix |
| --- | --- | --- |
| Every pod `ContainerCreating` and the CNI down after a host reboot | Docker's restart policy replays the node container raw, skipping the step that injects the node's host alias, so image pulls fail | `mise run cluster:heal` — idempotent, safe to run whenever the cluster looks wedged |
| Pods in `ImagePullBackOff` behind a proxy | some egress proxies truncate large layers on containerd's single-stream pull | `mise run cluster:unwedge` ([http-proxy](guide/http-proxy.md)) |
| A service starts cleanly then fails on its first cross-service call | a missing `svc:*` edge in its `.mise.toml` — nothing detects this ([ADR-0600](adr/0600-local-development-loop.md)) | add the edge |
| A gated route returns `403` locally but works when curled directly | curling the port bypasses the edge and forges identity headers | run it behind the edge, above |

## Tests

Both suites run against `cluster:full` and neither is part of `mise run test` or gates a merge ([ADR-0601](adr/0601-testing-strategy.md)).

```sh
mise run e2e:smoke      # product golden path + a key dashboard render
mise run e2e            # every journey, every dashboard, all visual baselines
mise run perf:seed      # bulk rows, so the read path meets a realistic table
mise run perf:smoke     # ~30s — are the scenarios still wired to the API?
mise run perf           # the steady baseline, nightly and pre-release
mise run perf:stress    # ramp to saturation; thresholds are meant to break here
```

The browser test is the acceptance gauge: a rendered, authenticated dashboard proves the whole stack underneath is wired. A preflight readiness check runs first, so a red e2e reads "infra down" rather than "app broken".

Load metrics leave k6 over OTLP into the cluster's collector, so a run appears in Grafana on the `load-test` dashboard next to the pod CPU and memory it caused — that correlation is the point. How to read the shapes is [test/perf/runbook](guide/performance-runbook.md).

## Formatting and linting

`mise run format` and `mise run lint` cover every language, including Markdown. Generated code is never linted or formatted; the exclusions live in each tool's config ([ADR-0101](adr/0101-monorepo.md)).

Markdown is governed by **rumdl**, the single source of truth for both linting and formatting. `mise run format:md` auto-fixes most rules and runs on staged files via the pre-commit hook. For inline editor warnings that match CI exactly, point your editor at rumdl's LSP — the repo stays IDE-neutral and ships no editor config.

**Tables are auto-fixed too.** `MD060` enforces the compact form — one space inside each pipe, a bare `---` delimiter, never column padding — and `format:md` applies it. Do not use an IDE's reformat-table action: most of them pad columns, which is the opposite.

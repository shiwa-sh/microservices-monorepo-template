# The local development loop (ADR-0600)

Two tiers, and a service is native or in-cluster within either. This is which one to
pick, how to be logged in, and what each tier deliberately cannot tell you.

## Pick a tier

| You are | Run | Cost |
| --- | --- | --- |
| writing service code | `mise run cluster:base`, then the service's own task | seconds to start, no image build |
| working on the UI | `cluster:base` + `mise run mock:start` | same, plus a mock serving the committed contract |
| changing charts, sync waves, or anything Argo delivers | `mise run cluster:full` | minutes; it is the whole platform |
| about to open a PR | `mise run cluster:full`, then `mise run e2e` | the configuration CI runs |

`cluster:base` is the floor: Cilium, Traefik, cert-manager, a stand-in Postgres,
Kratos and Oathkeeper, plus the seeded test identities. It is unconditional and it
is cheap. Everything else is opt-in and **declared by the service that needs it** —
there are no profiles to pick from, and no table to keep honest.

```sh
mise run cluster:base                 # the floor
mise run -C services/orders server    # brings up dep:postgres, dep:openfga, dep:temporal first
mise run -C services/orders worker    # …and svc:catalog, svc:payment, which the saga calls
```

Those `depends` lists are checked against the code by `lint:service-deps`, so a
dependency the code has and the list does not is a CI failure rather than a
confusing one at run time.

## Native or in-cluster

| Mode | Command | Use it when | It cannot |
| --- | --- | --- | --- |
| **native** | `mise run service:dev -- <svc>`, then `mise run -C services/<svc> server` | you want a debugger, or a change every second | see the pod's env or files, and NetworkPolicy does not apply to it |
| **in-cluster** | `mise run cluster:add -- <svc>` | you need the real pod: its env, its mounts, its policy | attach a debugger, and it costs an image build per change |

Both sit behind the real edge. Native mode stamps an IngressRoute and an
EndpointSlice pointing at the host, so Traefik routes to your process and it
receives **real Oathkeeper identity headers** — the same ones the pod would get.

Any number of services can be native at once. Each binds its own registered port
from `scripts/lib/ports.sh`; `:8080` is unassigned because the host maps it to the
edge. Debugging a flow across three services is three native services with
breakpoints in all three, and everything they call still in the cluster.

`cluster:add` pauses that app's Argo auto-sync so the working-tree image is not
reverted. `mise run cluster:remove -- <svc>` puts it back — leaving it paused means
the cluster has silently stopped tracking `master`.

## Be logged in

There is no bypass, and adding one is a review-blocker (`lint:auth-inline`). Both
tiers run the real Kratos, and both seed the committed test identities:

| Identity | Is | Gets you |
| --- | --- | --- |
| operator | AAL2 (password + TOTP) | the ops hosts — Grafana, the admin console, Temporal, Hubble, pgweb, headlamp |
| user | AAL1 (password) | the product surface |

Their credentials are `test/e2e/fixtures/identities.ts`, which is also what the e2e
suite logs in with. Log in at `https://dev.localtest.me:8443/auth/login`; the ops
hosts are `https://<name>.ops.dev.localtest.me:8443` and answer `401` until you do.

The operator's second factor is enrolled at run time rather than imported, because
Kratos cannot import a TOTP credential. `mise run auth:seed` re-runs the whole
provisioning if a login stops working.

## HTTP proxies

Behind a proxy, `NO_PROXY` must include `.localtest.me`, `localhost` and `127.0.0.1`
— otherwise every local request is sent to the proxy, which cannot resolve any of
it, and the symptom is a timeout rather than a refusal. The node's containerd has
the same problem in reverse: it needs the proxy to pull public images, and a pull
that wedges shows up as `ImagePullBackOff` on a pod that was fine yesterday. That is
what `mise run cluster:unwedge` is for. On a proxied machine a FRESH cluster cannot
pull its bootstrap images at all — kind cannot inject the proxy into the node — so
first bring-up is `CLUSTER_PRELOAD=1 mise run cluster:full`, which host-pulls and
`kind load`s the images before the imperative head blocks. The full setup, including
what to put in the Docker daemon's environment, is
[guide/http-proxy.md](guide/http-proxy.md).

## After a reboot

The inner loop routes to natively-run services through an EndpointSlice pointing at
the docker-bridge gateway — the only host address a pod can reach. It is stamped at
bring-up and is wrong after the bridge is recreated, which a reboot does: the
cluster comes back, the pods are Ready, and every native route 502s. `mise run
cluster:heal` re-starts the node (clearing a half-restored Cilium datapath) and
re-stamps the edge glue. It is idempotent; run it whenever the cluster survived
something the host did.

## UI work: the mock

```sh
mise run mock:start     # Prism, serving apps/frontend/public/devportal/openapi/internal.json
mise run mock:logs      # the interesting half: unmatched routes, and bodies that fail the spec
mise run mock:stop
```

The mock takes over `/api` at a priority no service route can reach, so it works
whether or not any service is running. It sits **behind the real edge** and the real
middleware chain, so the request path — TLS, identity-header stripping, forward-auth,
security headers — is the production one.

What it will not do, by design:

- **serve identity.** No `401`, no session awareness, no identity headers. Auth
  behaviour comes from Kratos and Oathkeeper, which are already in the floor.
- **have state.** Two POSTs return the same thing; nothing persists, nothing
  progresses. A workflow's `status` stays at whatever the example says.
- **be a test fixture.** It is forbidden in `test`, `ci:affected`, and the e2e and
  visual suites — a suite that passes against a mock has asserted the mock.

Its only input is the committed projection, so a response you disagree with is fixed
in `services/<svc>/openapi.yaml` and regenerated. That is the point: the fidelity is
bought in the contract, where the whole team gets it.

## What each tier cannot tell you

**`cluster:base` cannot tell you:**

- whether it works on the real datastores. Its Postgres is a plain ephemeral image,
  not CNPG: no pooler, no failover, no backups, and the database is empty again on
  every restart. Its Temporal and OpenFGA stand-ins are the same trade.
- whether the delivery works. Nothing here reconciles from git, so sync waves,
  ApplicationSet generators and secret materialisation are all untested.
- whether a native service obeys its NetworkPolicy. The host process is not in the
  mesh, so nothing enforces one against it.

**`cluster:full` cannot tell you:**

- whether your uncommitted change works, unless you put it there. Argo reconciles
  committed `master`, not your working tree — use `cluster:add`, `platform:deploy`,
  or a branch `targetRevision` ([guide/gitops-local.md](guide/gitops-local.md)).
- whether it survives scale. Everything runs at one replica through the `local`
  values overlay, so a bug that needs two of something will not appear.

**Neither tells you about node count or the multi-node failure modes**, and neither
is a performance measurement: everything shares one machine's CPU with your
compiler. Load testing is `mise run perf` against the full tier, and even that is a
relative number.

## When it breaks

| Symptom | Try |
| --- | --- |
| a pod stuck pulling, after a laptop sleep or a proxy hiccup | `mise run cluster:unwedge` |
| native routing dead (502 at /) after a reboot — the bridge IP changed | `mise run cluster:heal` |
| everything is odd and you want the time back | `mise run cluster:delete`, then bring the tier up again |
| the edge answers 404 for a service you deployed | check Argo did not revert it: `kubectl -n argocd get app` |

Disk is the usual culprit for a full tier that half-works: the node evicts pods and
garbage-collects images it cannot re-pull, because the inner loop's images exist
only on the node. `docker system prune` and a rebuild, in that order.

## Where the rules are

[ADR-0600](adr/0600-local-development-loop.md) is the decision and its
consequences; this page is how to use it. [guide/gitops-local.md](guide/gitops-local.md) covers
testing uncommitted infrastructure, and [guide/first-change.md](guide/first-change.md) is the
end-to-end walk through adding something.

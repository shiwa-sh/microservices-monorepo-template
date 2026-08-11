# System View

The ADR set is decision-complete and description-empty: it records why every part is what it is, and nothing shows the shape. This document is the description. It decides nothing, and every element traces to the ADR that chose it.

Three views, in the order a newcomer needs them: what runs, how a request moves, and how identity reaches a handler.

## Containers

Everything inside the cluster boundary is reconciled from this repository by Argo CD ([ADR-0201](../adr/0201-gitops.md)). Everything outside it is deliberate: the two rows that must survive the cluster.

```text
                     ┌─────────────────────────────────────────────────────────┐
   browser ── TLS ──▶ │ Traefik            edge: routing, TLS, rate limiting    │
                     │   └─▶ Oathkeeper   forward-auth: validate, strip, inject │
                     └───────────────┬─────────────────────────────────────────┘
                                     │  X-User-Id / X-Org-Id / X-Roles
        ┌────────────────────────────┼────────────────────────────┐
        ▼                            ▼                            ▼
  ┌───────────┐             ┌─────────────────┐          ┌─────────────────┐
  │ frontend  │             │ services/*      │◀────────▶│ authz + OpenFGA │
  │ Next.js   │────HTTP────▶│ Go: server      │  Checker └─────────────────┘
  │ one app   │             │     worker      │
  └───────────┘             └────────┬────────┘
                                     │
        ┌────────────────┬───────────┴───────────┬──────────────────┐
        ▼                ▼                       ▼                  ▼
  ┌───────────┐   ┌─────────────┐        ┌──────────────┐   ┌──────────────┐
  │ Postgres  │   │ Temporal    │        │ OTel         │   │ Kratos       │
  │ CNPG      │   │ server      │        │ Collector    │   │ identity     │
  │ one DB    │   │ + workers   │        │ DaemonSet    │   └──────────────┘
  │ per svc   │   └─────────────┘        └──────┬───────┘
  └───────────┘                                 │
                                    ┌───────────┴────────────┐
                                    ▼                        ▼
                          ┌──────────────────┐     ┌──────────────────┐
                          │ Loki  Tempo      │     │ Prometheus       │
                          │ Pyroscope        │     │ + Alertmanager   │
                          └────────┬─────────┘     └──────────────────┘
                                   │                        │
                                   └────────┬───────────────┘
                                            ▼
                                      ┌──────────┐
                                      │ Grafana  │  one UI, four signals
                                      └──────────┘
  ═══════════════════════════ cluster boundary ═══════════════════════════
        ▼                                              ▼
  ┌───────────────────┐                        ┌──────────────────┐
  │ object storage    │  backups, log/trace/   │ forge + registry │  images,
  │ off-cluster       │  profile data, images  │ Forgejo + zot    │  pipelines
  └───────────────────┘                        └──────────────────┘
```

**Two things sit outside on purpose.** Production object storage holds the backups a cluster rebuild reads, so it cannot live in the failure domain it protects ([ADR-0200](../adr/0200-cluster-topology.md)). The registry is what pods pull from, so a cluster that cannot start pods cannot be the thing serving its own images ([ADR-0105](../adr/0105-image-registry.md)).

**What is not here.** Kyverno and Pod Security Admission sit in the API server's admission path rather than in any request path, and CiliumNetworkPolicy is in the datapath between every pair of boxes above. Neither appears as a box because neither is a hop ([ADR-0203](../adr/0203-policy-enforcement.md)).

## A request, end to end

The checkout saga, which is the one path exercising every mechanism ([ADR-0302](../adr/0302-temporal.md)).

```text
browser         Traefik      Oathkeeper     orders svc     Temporal      catalog/payment
   │                │             │              │             │                │
   │──POST /api/orders───────────▶│              │             │                │
   │                │─forward-auth▶│             │             │                │
   │                │             │─validate session (Kratos)  │                │
   │                │             │─strip client headers       │                │
   │                │◀─200 + X-User-Id / X-Org-Id / X-Roles    │                │
   │                │────────────────────────────▶│            │                │
   │                │             │              │─Checker ▶ OpenFGA             │
   │                │             │              │─start workflow ─────▶│        │
   │◀────────────────202 + workflow handle ───────│             │                │
   │                │             │              │             │─activity ──────▶│
   │                │             │              │             │  HTTP + same headers
   │                │             │              │             │◀─result ────────│
   │                │             │              │             │─compensate on failure
   │──GET /api/orders/{id} ──────▶│──────────────▶│            │                │
   │◀───────────────── state, read from Postgres ─│             │                │
```

**Three properties this view makes visible.** Identity is established once and forwarded, never re-validated per hop. The synchronous request returns a handle rather than waiting for the saga. And a cross-service call is HTTP through a generated client, so the workflow's steps cross the same boundary any caller does ([ADR-0000](../adr/0000-platform-foundations.md), principle 10).

## Identity and authorization

Where a decision is made about *who* and a separate decision about *what they may do*.

```text
  ┌─────────┐   session cookie    ┌────────────┐    who is this?    ┌──────────┐
  │ browser │────────────────────▶│ Oathkeeper │───────────────────▶│  Kratos  │
  └─────────┘                     └─────┬──────┘                    └──────────┘
                                        │ authoritative headers
                                        ▼
                                  ┌────────────┐   may they do it?  ┌──────────┐
                                  │  service   │───────────────────▶│ OpenFGA  │
                                  │  handler   │      Checker       │  tuples  │
                                  └─────┬──────┘                    └──────────┘
                                        │ authorized mutation
                                        ▼
                                  ┌────────────┐  dual write, in one workflow
                                  │  Postgres  │  ── authz-relevant change ──▶ OpenFGA
                                  └────────────┘
```

| Question | Answered by | Where |
| --- | --- | --- |
| Who is this? | Kratos, through Oathkeeper | once, at the edge ([ADR-0305](../adr/0305-edge-auth-and-traffic-policy.md)) |
| May they do this? | OpenFGA, through `Checker` | in the handler, per request and per resource ([ADR-0304](../adr/0304-identity-and-authorization.md)) |
| May they reach this port? | CiliumNetworkPolicy | in the datapath, independent of both ([ADR-0206](../adr/0206-cluster-networking.md)) |
| May this operator open a dashboard? | the ops coarse gate — the `operator` claim plus AAL2, deliberately with **no** OpenFGA call | at the edge, so an authorization outage cannot lock operators out of the dashboards that diagnose it |

**The load-bearing asymmetry.** Between the edge and a service, identity is a header trusted because of network position ([ADR-0305](../adr/0305-edge-auth-and-traffic-policy.md)). Authorization is never positional: a request that has arrived is authorized as though it came from anywhere. The seam that makes the first cryptographic is recorded with its trigger, and the second needs no seam because it never made the assumption.

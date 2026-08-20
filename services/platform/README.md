# platform

The worker that owns work belonging to the platform rather than to any one service ([ADR-0302](../../docs/adr/0302-temporal.md), [ADR-0301](../../docs/adr/0301-data-lifecycle-privacy.md)).

```sh
cd services/platform
mise run worker
```

## Why this exists

A quarterly DR drill ([ADR-0207](../../docs/adr/0207-cluster-storage.md)), a retention pass ([ADR-0301](../../docs/adr/0301-data-lifecycle-privacy.md)), a cardinality audit ([ADR-0500](../../docs/adr/0500-observability.md)) and a quarterly trigger review ([ADR-0000](../../docs/adr/0000-platform-foundations.md)) are each decided by an ADR that names no owner. None belongs to catalog, orders, orgs, payment or authz.

Hanging them off one of those services would put it in charge of work outside its domain, which is what the process-owner rule exists to prevent. A Kubernetes `CronJob` is forbidden for business-meaningful work ([ADR-0302](../../docs/adr/0302-temporal.md)), and three of the four are that. So they get a worker.

It is **worker-only** — no HTTP surface, because it answers no requests. That is why the shared chart gained `server.enabled`.

## What it does not do

[ADR-0301](../../docs/adr/0301-data-lifecycle-privacy.md) rejects "a dedicated erasure service calling each owning service's API". This is not that. The objection there was to a service reimplementing retries, timers and state against a Temporal that is already Core; everything here gets all three from Temporal. What lives in this process is the workflow, not a second orchestrator.

## Erasure ordering

`EraseSubject` removes application data per service, then the Kratos identity, then the OpenFGA tuples — in that order, deliberately. While the tuples exist the services can still answer questions about the subject, which is what makes a failed run safe to retry. Removing them first would leave rows nothing can reach: erased in effect, and invisible to the retry that was supposed to erase them.

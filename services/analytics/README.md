# analytics

The marketing event store and the consent record ([ADR-0700](../../docs/adr/0700-analytics.md)).

An ordinary first-party service, and that is the decision rather than a convenience: a platform component pays the full resource, supply-chain, network-policy and backup tax, while a service inherits all of it from the template. It is also what makes an erasure workflow able to reach these rows, which it could not do inside a vendored product.

```sh
cd services/analytics
mise run server     # http://localhost:8086
```

## Who calls it

Nobody in a browser. The browser emits `marketing.*` events through Faro — the only browser agent — and the collector's routing connector splits them out of the logs pipeline and delivers them here. That indirection is why marketing events never reach Loki, and why this store can be replaced without a frontend release.

The consent control writes decisions to `/analytics/consent`; the panel reads funnels through queries over `events`.

## Consent is enforced twice

The browser wrapper emits no `marketing.*` event without a recorded grant, and `RecordEvents` drops any batch whose session has none. The second exists because the first runs on a client the platform does not control — either alone is a policy, and both together are a control.

A dropped batch is not an error. The caller is the collector, which cannot fix a missing grant and must not retry, so the response reports `stored` and `dropped` and a non-zero `dropped` is a client worth looking at.

## PII

Every column is classified from the first migration ([ADR-0301](../../docs/adr/0301-data-lifecycle-privacy.md)). The table carries a session id always and an identity id when the visitor is authenticated, which makes it a PII store by definition. Raw IP addresses are never stored and the user agent is reduced to a device class at ingest.

`events` is partitioned by month on `occurred_at`, so retention is dropping a partition rather than deleting rows — a retention job that rewrites a table is a retention job that gets postponed.

# ADR-0301: Data Lifecycle & Privacy

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0200](0200-cluster-topology.md), [ADR-0300](0300-data.md), [ADR-0302](0302-temporal.md), [ADR-0303](0303-api-contracts-and-lifecycle.md), [ADR-0304](0304-identity-and-authorization.md), [ADR-0500](0500-observability.md)

## Context

[ADR-0500](0500-observability.md) forbids PII in telemetry and provides redaction. Nothing decides what happens to the data the platform **deliberately** stores: how long it is kept, how it is erased, and how a subject access request is served.

A multi-tenant application at meaningful monthly actives has retention, right-to-erasure, and portability obligations that cannot be retrofitted cheaply once data has accreted across services.

Those obligations are [GDPR](https://eur-lex.europa.eu/eli/reg/2016/679/oj) Art. 15 (access), Art. 17 (erasure), Art. 20 (portability), and Art. 5(1)(e) (storage limitation). They are named here because they set the deadline and the scope this ADR builds machinery for, not because the platform is a compliance product. A project under a different regime substitutes its own articles; the mechanism below does not change.

## Decision drivers

1. **Erasure and export must be correct across stores** — the application database *and* OpenFGA ([ADR-0304](0304-identity-and-authorization.md)) — or a subject is deleted while still appearing in authz tuples.
2. **A half-completed erasure is worse than an unstarted one.** The deadline is one month from the request (GDPR Art. 12(3)), so the process must survive a partial failure and resume, not restart from an unknown point.
3. **One execution answers "is this subject erased".** Whatever runs it must be able to say so without correlating several services' logs.
4. **Retention is declared, not incidental.** Every stored category has a class and a lifespan.
5. **PII is identifiable by a machine**, so retention, export, and redaction target it mechanically rather than by memory.

## Considered options

| Option | Survives a partial failure | Spans stores outside one database | Answers "is it done" | Verdict |
| --- | --- | --- | --- | --- |
| **Temporal workflows for erasure, DSAR, and retention** | yes — each activity retries, and the execution resumes where it stopped | yes, including OpenFGA | one execution, and its event history is the audit trail | **Chosen.** The obligation is a long-running, multi-store, deadline-bearing mutation, which is the shape [ADR-0302](0302-temporal.md) already buys a primitive for |
| Outbox events, consumed per service | yes, per hop | yes, eventually | no — by correlating every consumer | No single execution owns the request. [ADR-0302](0302-temporal.md) keeps the outbox for fire-and-forget work, and a statutory deadline is not fire-and-forget |
| A dedicated erasure service calling each owning service's API | only once it implements its own retries, timers, and state | yes | its own store, once built | This is Temporal's job description rebuilt inside one service, against a Temporal that is already Core |
| Change-data-capture, such as Debezium | yes | yes | no — the same correlation problem, plus a connector | Adds a component and a second delivery path to answer a question the platform can already ask directly |
| A scheduled script per service | no | no — nothing coordinates OpenFGA | logs only | Produces exactly the failure driver 1 names |
| Database-level cascade deletes | within one transaction | no | none | OpenFGA is not in the database, and neither is another service's data |
| A manual runbook on request — the honest baseline | no | only as far as the operator is thorough | a ticket | Unbounded latency, and correctness varies by who runs it |

## Decision

| Concern | Mechanism |
| --- | --- |
| **Data classes** | Every stored category is classified — operational, PII, audit, telemetry — with a declared retention period. Telemetry retention belongs to [ADR-0500](0500-observability.md), backups to [ADR-0200](0200-cluster-topology.md); this ADR owns application data |
| **PII tagging** | A column holding personal data carries `COMMENT ON COLUMN … IS 'pii:<class>'` in the migration that creates it ([ADR-0300](0300-data.md)). The tag lives in the DDL, travels with the schema, and is readable from `pg_description`, so erasure, export, and redaction enumerate their targets by query rather than by memory |
| **Right to erasure** | A Temporal workflow erases or anonymises the subject's rows across every owning service **and** removes the corresponding OpenFGA tuples, as dual-write activities |
| **Subject access and portability** | A Temporal workflow gathers the subject's data across services through their APIs ([ADR-0303](0303-api-contracts-and-lifecycle.md)) and produces an export |
| **Retention enforcement** | A Temporal `Schedule` prunes or anonymises data past its class's retention. Each run's event history is the record that it happened, and to what |

Anonymise-versus-hard-delete is a per-category decision, recorded with the data class. Audit data may carry a retention obligation that an erasure request cannot override.

## Consequences

### Positive

- Erasure and export are correct across the application database and OpenFGA by construction, rather than depending on an engineer remembering to also run something against authz.
- Retention is a declared property of each class, enforced on a schedule.
- PII tagging makes redaction and erasure target the right fields.

### Negative / Risks

- **Every service must implement erasure and DSAR activities for the data it owns.** Accepted: it is the same per-service dual-write discipline authz already requires.
- **A new service can silently omit them**, and the omission surfaces only on the first request that needs it. Mitigated by the data-class registry being the checklist a new service is reviewed against.

## Rules

- Every stored data category has a declared class and retention period, enforced by a Temporal `Schedule`.
- A column holding personal data carries a `pii:<class>` column comment in the migration that creates it, so erasure, export, and redaction enumerate their targets by query.
- Right-to-erasure and DSAR run as Temporal workflows acting across every owning service **and** OpenFGA. A raw multi-store delete script is not used.
- Anonymise-versus-delete is recorded per data class, never decided per request.

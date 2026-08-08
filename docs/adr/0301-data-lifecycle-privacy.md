# ADR-0301: Data Lifecycle & Privacy

- **Status:** Accepted
- **Date:** 2026-08-06
- **Deciders:** Platform team
- **Related:** [ADR-0200](0200-cluster-topology.md), [ADR-0300](0300-data.md), [ADR-0302](0302-temporal.md), [ADR-0303](0303-api-contracts-and-lifecycle.md), [ADR-0304](0304-identity-and-authorization.md), [ADR-0500](0500-observability.md)

## Context

[ADR-0500](0500-observability.md) forbids PII in telemetry and provides redaction. Nothing decides what happens to the data the platform **deliberately** stores: how long it is kept, how it is erased, and how a subject access request is served.

A multi-tenant application at meaningful monthly actives has retention, right-to-erasure, and portability obligations that cannot be retrofitted cheaply once data has accreted across services.

## Decision drivers

1. **Erasure and export must be correct across stores** — the application database *and* OpenFGA ([ADR-0304](0304-identity-and-authorization.md)) — or a user is deleted while still appearing in authz tuples.
2. **A durable, auditable process**, not a hand-run script. The same reliability primitive as every other cross-store mutation ([ADR-0302](0302-temporal.md)).
3. **Retention is declared, not incidental.** Data has a class and a lifespan.
4. **PII is identifiable**, so retention and redaction can target it mechanically.

## Considered options

| Option | Cross-store correctness | Auditable | Verdict |
| --- | --- | --- | --- |
| **Temporal workflows for erasure, DSAR, and retention** | activities span every owning service and OpenFGA in one execution | the event history is the audit trail | **Chosen** |
| A scheduled script per service | each service deletes its own rows; nothing coordinates OpenFGA | logs only | Produces exactly the failure driver 1 names |
| Database-level cascade deletes | within one database only | none | OpenFGA is not in the database, and neither is another service's data |
| Manual runbook on request | depends on the operator | a ticket | Unbounded latency, and correctness varies by who runs it |

## Decision

| Concern | Mechanism |
| --- | --- |
| **Data classes** | Every stored category is classified — operational, PII, audit, telemetry — with a declared retention period. Telemetry retention belongs to [ADR-0500](0500-observability.md), backups to [ADR-0200](0200-cluster-topology.md); this ADR owns application data |
| **PII tagging** | Columns holding personal data are annotated at the schema level, so erasure, export, and redaction target them mechanically rather than by memory |
| **Right to erasure** | A Temporal workflow erases or anonymises the subject's rows across every owning service **and** removes the corresponding OpenFGA tuples, as dual-write activities |
| **Subject access and portability** | A Temporal workflow gathers the subject's data across services through their APIs ([ADR-0303](0303-api-contracts-and-lifecycle.md)) and produces an export |
| **Retention enforcement** | A Temporal `Schedule` prunes or anonymises data past its class's retention and opens a tracking record |

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

- Every stored data category has a declared class and retention period, enforced by a Temporal `Schedule`. `(review-only)`
- Personal data columns are tagged at the schema level so erasure, export, and redaction target them mechanically. `(review-only)`
- Right-to-erasure and DSAR run as Temporal workflows acting across every owning service **and** OpenFGA. A raw multi-store delete script is not used. `(review-only)`
- Anonymise-versus-delete is recorded per data class, never decided per request. `(review-only)`

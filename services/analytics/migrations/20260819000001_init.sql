-- migrate:up
-- The analytics store (ADR-0700). It is a PII store by definition — every row
-- carries a session id, and an authenticated visitor's rows carry an identity id —
-- so it is treated as one from this first migration rather than retrofitted.
--
-- Partitioned by month on `occurred_at`. Retention here is dropping a partition
-- rather than deleting rows (ADR-0301): a delete of a month's events from a single
-- table rewrites the table, and a retention job that is expensive is a retention
-- job that gets postponed.
create table events (
  id uuid not null,
  -- The Faro session id, which is what joins a funnel step to the exception and
  -- the RUM log from the same visit. Pseudonymous, and it still resolves to a
  -- person through the consent record below, so erasure must reach it.
  session_id text not null,
  -- Present only when the visitor is authenticated. Null is the common case and is
  -- not a defect: most of a funnel happens before anyone signs in.
  identity_id text,
  -- The reserved namespace ADR-0700 gives marketing events. Stored without the
  -- prefix; the prefix is how the collector ROUTES them, not what they are.
  name text not null,
  -- The event's own fields. Kept as jsonb because a funnel gains steps faster than
  -- a migration can be written, and constrained by the emitting wrapper rather
  -- than by the column.
  properties jsonb not null default '{}'::jsonb,
  -- A parsed class — `mobile`, `desktop`, `tablet` — and never the user-agent
  -- string, which is a fingerprinting surface. Reduced at ingest, so the raw value
  -- never lands here.
  device_class text not null default 'unknown',
  occurred_at timestamptz not null,
  created_at timestamptz not null default now()
) partition by range (occurred_at);

-- The primary key includes the partition key, which Postgres requires, and the
-- ordering puts `occurred_at` first so a range scan over a window is the index's
-- natural direction.
create unique index events_pkey on events (occurred_at, id);
create index events_session on events (session_id, occurred_at);
create index events_name on events (name, occurred_at);

-- The consent record (ADR-0700, GDPR Art. 7(1)). It exists to make consent
-- DEMONSTRABLE after the fact, which is why it carries the version of the purpose
-- text shown and where the signal came from rather than a bare boolean.
create table consent (
  session_id text primary key,
  identity_id text,
  -- `granted`, `withdrawn`, or `refused`. Withdrawal is a state rather than a
  -- deletion: erasing the record would erase the evidence that consent was once
  -- given, which is the opposite of demonstrable.
  state text not null check (state in ('granted', 'withdrawn', 'refused')),
  -- The version of the purpose text the visitor was shown. Consent is to a stated
  -- purpose, so a changed purpose is a new consent rather than a continuing one.
  purpose_version text not null,
  -- How the signal arrived: `control` for the on-page control, `gpc` for a Global
  -- Privacy Control header. A `gpc` row is a refusal nobody was prompted for.
  source text not null check (source in ('control', 'gpc')),
  decided_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

-- migrate:down
drop table consent;
drop table events;

-- migrate:up
-- The funnel rollup (ADR-0700, ADR-0302).
--
-- A funnel question — "how many sessions that saw the product page reached
-- checkout" — is answered by scanning every event in the window and ordering each
-- session's steps. That scan is cheap on a week of events and untenable on a year,
-- and the panel asking it is interactive. So the answer is computed on a schedule
-- and stored here, and the panel reads a table whose row count grows with
-- FUNNELS × STEPS × BUCKETS rather than with traffic.
--
-- This is deliberately NOT partitioned, unlike `events`. A rollup row per funnel
-- step per day is a few thousand rows a year; partitioning it would add the
-- machinery that makes retention cheap to a table where retention costs nothing.
create table funnel_rollup (
  -- The funnel's id from infra/analytics/funnels.yaml. Not a foreign key: the
  -- definitions are committed configuration rather than a table, so the database
  -- cannot enforce that this names a real one. A rollup for a funnel that was
  -- deleted is history, and dropping it on the next pass would erase the record
  -- that the funnel ever ran.
  funnel text not null,
  -- The step's position, zero-based. Ordering is the whole point of a funnel, and
  -- an index is what survives a step being renamed.
  step_index integer not null,
  -- The event name this step counts, denormalised so a rollup row is readable
  -- without the definition file beside it.
  step_name text not null,
  -- The bucket this row covers: its start, half-open to the next bucket.
  bucket_start timestamptz not null,
  bucket_end timestamptz not null,
  -- Sessions that reached this step IN ORDER — having reached every earlier step
  -- first, each at or before this one. A session that lands on checkout from a
  -- bookmark has not traversed the funnel, and counting it would make every funnel
  -- look flat.
  sessions bigint not null,
  computed_at timestamptz not null default now(),

  -- One row per funnel, step and bucket. Recomputing a bucket REPLACES it, which
  -- is what makes the pass safe to re-run over a window that is still filling.
  primary key (funnel, bucket_start, step_index)
);

-- The panel's read: one funnel over a range of buckets, in step order.
create index funnel_rollup_lookup on funnel_rollup (funnel, bucket_start desc, step_index);

-- Every column, classified (ADR-0301). A rollup is an AGGREGATE — the smallest
-- thing it counts is a session count, never a session — so every column here is
-- `pii:none`, and saying so explicitly is what distinguishes "no personal data"
-- from "nobody classified it".
--
-- This is also what makes the rollup outlive an erasure. `EraseSubject` removes a
-- subject's events; it does not revisit these counts, and it does not need to,
-- because a count of sessions is not a record about any of them. A rollup that
-- carried session ids would have to be walked, and would make erasure a rewrite of
-- every bucket the subject ever appeared in.
comment on column funnel_rollup.funnel is 'pii:none';
comment on column funnel_rollup.step_index is 'pii:none';
comment on column funnel_rollup.step_name is 'pii:none';
comment on column funnel_rollup.bucket_start is 'pii:none';
comment on column funnel_rollup.bucket_end is 'pii:none';
comment on column funnel_rollup.sessions is 'pii:none';
comment on column funnel_rollup.computed_at is 'pii:none';

-- migrate:down
drop table funnel_rollup;

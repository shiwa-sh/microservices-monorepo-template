-- name: GetConsent :one
select
  session_id,
  identity_id,
  state,
  purpose_version,
  source,
  decided_at
from consent
where session_id = $1;

-- name: UpsertConsent :one
-- A decision replaces the previous one for the session and keeps the row, because
-- withdrawal is a state rather than a deletion: erasing it would erase the evidence
-- that consent was once given, which is what GDPR Art. 7(1) asks to be demonstrable.
insert into consent (session_id, identity_id, state, purpose_version, source)
values ($1, $2, $3, $4, $5)
on conflict (session_id) do update set
identity_id = excluded.identity_id,
state = excluded.state,
purpose_version = excluded.purpose_version,
source = excluded.source,
updated_at = now()
returning session_id, identity_id, state, purpose_version, source, decided_at;

-- name: InsertEvent :exec
insert into events (
  id, session_id, identity_id, name, properties, device_class, occurred_at
)
values ($1, $2, $3, $4, $5, $6, $7);

-- name: SummariseEvents :many
-- One row per event name over a window, with the distinct sessions that produced
-- it. This is the shape every funnel question starts from, and it is a plain
-- aggregate rather than a window function because the first question a panel
-- answers is "what is happening at all".
select
  name,
  count(*) as occurrences,
  count(distinct session_id) as sessions
from events
where occurred_at >= $1
group by name
order by occurrences desc
limit 50;

-- name: FunnelStepFirstSeen :many
-- Each session's FIRST occurrence of each named step within the window.
--
-- The ordering that makes a funnel a funnel is not done here. This returns one row
-- per session and step, and the caller walks a session's steps in the definition's
-- order, keeping only those whose first occurrence is at or after the previous
-- step's. Doing it in SQL would mean a query whose shape depends on the number of
-- steps — which sqlc cannot generate and a reviewer cannot read — and this shape
-- serves a funnel of any length.
--
-- `name = any($3::text[])` rather than a join against the definitions: the funnels
-- are committed configuration, not a table (infra/analytics/funnels.yaml).
select
  session_id,
  name,
  min(occurred_at)::timestamptz as first_seen
from events
where
  occurred_at >= $1
  and occurred_at < $2
  and name = any($3::text [])
group by session_id, name;

-- name: UpsertFunnelRollup :exec
-- Recomputing a bucket REPLACES it. A pass over a window that is still filling is
-- therefore safe to re-run, and re-running is the normal case: the most recent
-- bucket is always incomplete.
insert into funnel_rollup (
  funnel, step_index, step_name, bucket_start, bucket_end, sessions
)
values ($1, $2, $3, $4, $5, $6)
on conflict (funnel, bucket_start, step_index) do update set
step_name = excluded.step_name,
bucket_end = excluded.bucket_end,
sessions = excluded.sessions,
computed_at = now();

-- name: GetFunnelRollup :many
-- The panel's read: one funnel over a range of buckets, in bucket then step order,
-- which is the order it renders.
select
  funnel,
  step_index,
  step_name,
  bucket_start,
  bucket_end,
  sessions
from funnel_rollup
where
  funnel = $1
  and bucket_start >= $2
  and bucket_start < $3
order by bucket_start desc, step_index asc;

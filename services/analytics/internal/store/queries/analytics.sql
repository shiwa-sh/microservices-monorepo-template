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

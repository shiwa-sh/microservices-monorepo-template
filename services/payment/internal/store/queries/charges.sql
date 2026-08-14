-- name: CreateCharge :one
-- amount_cents is written from the money columns until the follow-up migration
-- drops it: the column is still NOT NULL, and nothing reads it any more.
insert into charges (id, order_id, amount, currency, status, idempotency_key, amount_cents)
values ($1, $2, $3, $4, 'pending', $5, ($3::numeric * 100)::integer)
returning id, order_id, amount, currency, status;

-- name: ListCharges :many
select
  id,
  order_id,
  amount,
  currency,
  status
from charges
order by created_at desc limit 100;

-- name: GetCharge :one
select
  id,
  order_id,
  amount,
  currency,
  status
from charges
where id = $1;

-- name: GetByIdempotencyKey :one
select
  id,
  order_id,
  amount,
  currency,
  status
from charges
where idempotency_key = $1;

-- name: UpdateChargeStatus :exec
update charges set status = $2
where id = $1;

-- name: CreateCharge :one
insert into charges (order_id, amount_cents, status, idempotency_key)
values ($1, $2, 'pending', $3)
returning id, order_id, amount_cents, status;

-- name: GetCharge :one
select id, order_id, amount_cents, status from charges where id = $1;

-- name: GetByIdempotencyKey :one
select id, order_id, amount_cents, status from charges where idempotency_key = $1;

-- name: UpdateChargeStatus :exec
update charges set status = $2 where id = $1;

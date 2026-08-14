-- name: ListOrders :many
select
  id,
  product_id,
  quantity,
  total,
  currency,
  status
from orders
order by created_at desc limit 100;

-- name: CreateOrder :one
-- total_cents is still written because the column is NOT NULL until the follow-up
-- migration drops it; the order starts at zero either way.
insert into orders (id, product_id, quantity, total, currency, status, owner_id, org_id, idempotency_key, total_cents)
values ($1, $2, $3, 0, $4, 'pending', $5, $6, $7, 0)
on conflict (id) do nothing
returning id, product_id, quantity, total, currency, status;

-- name: GetOrderByIdempotencyKey :one
select
  id,
  product_id,
  quantity,
  total,
  currency,
  status
from orders
where idempotency_key = $1;

-- name: SetOrderTotal :exec
update orders set total = $2, currency = $3, total_cents = ($2::numeric * 100)::integer
where id = $1;

-- name: GetOrder :one
select
  id,
  product_id,
  quantity,
  total,
  currency,
  status
from orders
where id = $1;

-- name: UpdateOrderStatus :exec
update orders set status = $2
where id = $1;

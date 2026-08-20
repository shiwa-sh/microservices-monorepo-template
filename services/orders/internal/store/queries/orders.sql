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
-- The order starts at a zero total; the saga sets it once the price is known.
insert into orders (id, product_id, quantity, total, currency, status, owner_id, org_id, idempotency_key)
values ($1, $2, $3, 0, $4, 'pending', $5, $6, $7)
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
update orders set total = $2, currency = $3
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

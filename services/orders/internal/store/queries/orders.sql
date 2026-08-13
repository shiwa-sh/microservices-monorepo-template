-- name: ListOrders :many
select
  id,
  product_id,
  quantity,
  total_cents,
  status
from orders
order by created_at desc limit 100;

-- name: CreateOrder :one
insert into orders (id, product_id, quantity, total_cents, status, owner_id, org_id)
values ($1, $2, $3, $4, 'pending', $5, $6)
on conflict (id) do nothing
returning id, product_id, quantity, total_cents, status;

-- name: GetOrder :one
select
  id,
  product_id,
  quantity,
  total_cents,
  status
from orders
where id = $1;

-- name: UpdateOrderStatus :exec
update orders set status = $2
where id = $1;

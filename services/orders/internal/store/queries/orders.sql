-- name: CreateOrder :one
insert into orders (product_id, quantity, total_cents, status)
values ($1, $2, $3, 'pending')
returning id, product_id, quantity, total_cents, status;

-- name: GetOrder :one
select id, product_id, quantity, total_cents, status from orders where id = $1;

-- name: UpdateOrderStatus :exec
update orders set status = $2 where id = $1;

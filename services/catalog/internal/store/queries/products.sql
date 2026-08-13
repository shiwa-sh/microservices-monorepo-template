-- name: ListProducts :many
select
  id,
  name,
  price_cents
from products
order by created_at desc limit 100;

-- name: GetProduct :one
select
  id,
  name,
  price_cents
from products
where id = $1;

-- name: CreateProduct :one
insert into products (id, name, price_cents) values ($1, $2, $3) returning id, name, price_cents;

-- name: UpdateProduct :one
update products set name = $2, price_cents = $3
where id = $1
returning id, name, price_cents;

-- name: DeleteProduct :exec
delete from products where id = $1;

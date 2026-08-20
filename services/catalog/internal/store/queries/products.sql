-- name: ListProducts :many
select
  id,
  name,
  price,
  currency
from products
order by created_at desc limit 100;

-- name: GetProduct :one
select
  id,
  name,
  price,
  currency
from products
where id = $1;

-- name: CreateProduct :one
insert into products (id, name, price, currency)
values ($1, $2, $3, $4)
returning id, name, price, currency;

-- name: UpdateProduct :one
update products set name = $2, price = $3, currency = $4
where id = $1
returning id, name, price, currency;

-- name: DeleteProduct :exec
delete from products where id = $1;

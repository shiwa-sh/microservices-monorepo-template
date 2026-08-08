# catalog

The simplest shop service. Pure CRUD over a single `products` table — no workflows.

Demonstrates the generated-code path: OpenAPI spec → ogen server (`libs/go/sdks/catalog`) → `internal/handlers` (implements the generated `Handler`) → sqlc queries → dbmate migrations → observability middleware.

```sh
cd services/catalog
mise run migrate    # apply migrations to $DATABASE_URL
mise run server     # http://localhost:8081 (its registered port; :8080 in-cluster)
```

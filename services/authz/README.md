# authz

The edge authorizer. Oathkeeper's `remote_json` authorizer calls it on every gated request (`infra/auth/oathkeeper/values.yaml`), and it answers by checking the relationship tuples in OpenFGA via `libs/go/authz`.

Unlike the shop services it is **internal**: no `/api` edge route, no database, no OpenAPI spec (it is not a public API, which is why `lint:api-audience` exempts it). Its two callers both reach it east-west on the server port — Oathkeeper, and the Lowdefy admin console's `authz-admin` connection (`apps/admin/lowdefy.yaml`), which POSTs to `/operators` backend-to-backend rather than back through the edge.

```sh
cd services/authz
mise run server     # http://localhost:8085
```

`:8085` is this service's registered local port (`scripts/lib/ports.sh`); in-cluster it binds `:8080` like every other service.

It also owns one workflow. Creating an operator is a dual write — a Kratos identity, then a `group:operator` tuple — and ADR-0304 puts an authz-relevant pair in a Temporal workflow, so `POST /operators` returns `202` and a handle rather than the operator. The worker (`cmd/worker`) serves the `authz-queue`; it is the only worker on the platform that opens no database pool, because this service owns no schema.

Because it has no database, it declares only `dep:openfga`. It needs an OpenFGA preshared key at startup even though the client dials lazily — locally that is the fixed dev key in `.env.example`, and in-cluster it comes from the SOPS-managed `openfga-creds` secret.

# admin (Lowdefy)

Internal admin tool (ADR-0012). Pure YAML pages.

| Path                    | Source                                                  |
|-------------------------|---------------------------------------------------------|
| `lowdefy.yaml`          | Top-level Lowdefy config (connections, pages, auth)     |
| `pages/`                | Hand-written pages that don't map to a single service   |
| `custom/<service>/`     | Hand- or LLM-written pages, per-service                 |

Day one, every page is hand- or LLM-written under `custom/`. The OpenAPI-driven
generator (`tools/admin-gen`, emitting `_generated/<service>/`) is added only
when the page count justifies it (ADR-0012); `_generated/` does not exist until
then.

Auth is enforced at the edge. Lowdefy reads the authenticated identity from the
`X-User-Email` header forwarded by Oathkeeper (ADR-0009, ADR-0012).

## Standard tasks

```sh
mise run build   # produces .lowdefy/build
mise run run     # lowdefy dev — local hot-reload
mise run lint    # lowdefy validate
```

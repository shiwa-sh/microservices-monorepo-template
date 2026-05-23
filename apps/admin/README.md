# admin (Lowdefy)

Internal admin tool (ADR-0012). Pure YAML pages.

| Path                       | Source                                                |
|----------------------------|--------------------------------------------------------|
| `lowdefy.yaml`             | Top-level Lowdefy config (connections, pages, auth)    |
| `pages/`                   | Hand-written pages that don't map to a single service  |
| `_generated/<service>/`    | Generated from `services/<service>/openapi.yaml`       |
| `custom/<service>/`        | Hand- or LLM-written pages, per-service                |

Auth is enforced at the gateway. Lowdefy reads the authenticated identity
from the `X-User-Email` header (ADR-0012).

## Standard tasks

```sh
mise run build   # produces .lowdefy/build
mise run run     # lowdefy dev — local hot-reload
mise run lint    # lowdefy validate
```

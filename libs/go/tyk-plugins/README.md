# Tyk plugins

Plugins are written in **Go** via Tyk's gRPC plugin server (ADR-0009).
Each plugin lives in its own subdirectory with a `README.md` explaining why
the work belongs at the gateway and not in the service.

No plugins yet. Add one as `libs/go/tyk-plugins/<name>/` when the first
gateway-scope concern (e.g., per-request rewrite, third-party signature
verification) appears.

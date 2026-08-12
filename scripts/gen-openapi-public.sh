#!/usr/bin/env bash
# Emit the developer-portal spec projections the Scalar-rendered portals consume
# (ADR-0303, ADR-0305, ADR-0400). Each projection is ONE merged OpenAPI document,
# not a file per service: the flat /api/<resource> namespace (ADR-0306) hides
# service topology, so the portal is a single unified reference grouped by resource
# tag — no per-service document switcher.
#
# The audience ladder (ADR-0303) is a single per-operation label — cluster →
# internal → public — resolved from the operation's own `x-audience`, else the
# service default (`info.x-audience`), else the fail-closed `cluster`. Projections
# are a threshold on that ladder:
#   - internal.json  audience >= internal (edge surface). The dev portal (behind the
#                    /devportal session gate) renders this — first-party + public ops.
#   - public.json    audience == public only — the anonymous public docs portal's
#                    data (ships with a public API). `cluster` (east-west) ops are in
#                    neither: they bypass the edge and are documented in the READMEs.
#
# Merge is safe because the flat namespace makes paths globally unique and the
# duplicated shared components (Problem, WorkflowHandle, Error) are identical across
# specs, so a deep merge collapses them. `tags` are rebuilt from the operations that
# remain, which prunes tags orphaned by the filtering.
#
# `x-*` specification extensions (incl. the resolved `x-audience`) are projection-time
# inputs only — the renderer never reads them — so they are stripped from the emitted
# specs. That keeps the artifacts lean and avoids editor JSON-schema false-positives
# ("Property is not allowed"). The source specs keep their extensions.
#
# Outputs are generated artifacts: git-committed, Biome-ignored, drift-checked by
# ci:gen. YAML → JSON so the browser renderer needs no YAML parser.
set -euo pipefail

shopt -s nullglob

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

out_dir="apps/frontend/public/devportal/openapi"

# Resolve each operation's effective audience (op override → service default →
# cluster) onto the operation, so the merged document filters on one per-op label.
resolve='.info.x-audience as $svc | (.paths.[].[] | select(tag == "!!map")) |= (.x-audience = (.x-audience // $svc // "cluster"))'
# Deep-merge all documents into one (single emit; `*` collapses the identical shared
# components, replaces arrays last-wins).
merge='(. as $item ireduce ({}; . * $item))'
# Recursively drop every `x-` extension key at any depth (info, operations, schemas).
strip_ext='(.. | select(tag == "!!map")) |= with_entries(select(.key | test("^x-") | not))'
# Unify the merged document: one title, the flat server, and a tag list derived from
# the operations that remain (auto-pruning tags orphaned by the audience filter).
envelope='.openapi = "3.1.0"
  | .info = {"title": "API reference", "version": "1.0.0"}
  | .servers = [{"url": "/api"}]
  | .tags = ([.paths[][].tags // [] | .[]] | unique | map({"name": .}))'
drop_empty_paths='del(.paths.* | select(tag == "!!map" and length == 0))'
# Drop component schemas no longer referenced by any surviving path once the
# audience filter has removed operations. Without this, an all-`cluster` service
# (e.g. authz) whose operations are all filtered out would still leak its request/
# response schemas into the merged components — its paths are gone but its schemas
# orphan. `$refs` is every `$ref` remaining in the document (paths and the schemas
# themselves, so schema-to-schema references keep their targets); a schema absent
# from that set is unreachable and pruned. Shared schemas (Problem, WorkflowHandle)
# stay because surviving responses still reference them.
prune_orphan_schemas='([.. | select(tag == "!!map" and has("$ref")) | .["$ref"]]) as $refs
  | .components.schemas |= with_entries(.key as $k | select($refs | any_c(. == "#/components/schemas/" + $k)))'

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

resolved=()
for spec in services/*/openapi.yaml; do
  service=$(basename "$(dirname "$spec")")
  # _template is scaffolding, not a live service — never documented.
  [ "$service" = "_template" ] && continue
  yq "$resolve" "$spec" >"$tmp/${service}.yaml"
  resolved+=("$tmp/${service}.yaml")
done

rm -rf "$out_dir"
mkdir -p "$out_dir"

echo "→ dev portal projection (audience >= internal)"
yq ea -o=json "${merge}
  | del(.paths.*.* | select(tag == \"!!map\" and .x-audience == \"cluster\"))
  | ${drop_empty_paths} | ${prune_orphan_schemas} | ${envelope} | ${strip_ext}" \
  "${resolved[@]}" >"$out_dir/internal.json"

echo "→ public docs projection (audience == public)"
yq ea -o=json "${merge}
  | del(.paths.*.* | select(tag == \"!!map\" and .x-audience != \"public\"))
  | ${drop_empty_paths} | ${prune_orphan_schemas} | ${envelope} | ${strip_ext}" \
  "${resolved[@]}" >"$out_dir/public.json"

# Belt-and-suspenders: fail loudly if any x- extension survived into the output, so
# a future yq change can never silently reintroduce the editor warnings.
for out in "$out_dir"/*.json; do
  n=$(yq -o=json '[.. | select(tag == "!!map") | keys.[] | select(test("^x-"))] | length' "$out")
  if [ "$n" != "0" ]; then
    echo "✗ x- extension key leaked into ${out} (strip_ext failed)" >&2
    exit 1
  fi
done

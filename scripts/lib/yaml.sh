# shellcheck shell=bash
# In-place edit of a single YAML scalar, preserving the rest of the file byte for
# byte. Source it, don't execute:
#   source "$(dirname "$0")/lib/yaml.sh"
#
# Why not `yq -i`: it rewrites the whole document from its own parse tree, so
# flow-style spacing collapses, blank lines vanish, and trailing comments are
# re-spaced. On the committed GitOps values files that turns every promotion into a
# formatting diff a human has to read past, and it fights the drift check.
#
# Why not `yq '… | line'` to locate the scalar: it miscounts in the presence of
# comment and blank lines — on infra/gitops/platform/dev/values.yaml it reports the
# admin image tag two to three lines above where it is. So the line is found by
# walking block indentation, and yq is used only to answer whether the path exists,
# which is the question it answers reliably.

if [[ -n "${__YAML_SH_LOADED:-}" ]]; then return 0 2>/dev/null || true; fi
__YAML_SH_LOADED=1

# yaml_scalar_line <file> <dotted.path>
# Echoes the 1-based line of the scalar, or nothing when the path is not a
# block-style key in the file.
yaml_scalar_line() {
  awk -v target="${2#.}" '
    # Track the block path by indentation: pop every frame at or below this
    # line s indent, then push this key.
    match($0, /^[[:space:]]*[A-Za-z0-9_.-]+:/) {
      line = $0
      indent = match(line, /[^ ]/) - 1
      key = line; sub(/^[[:space:]]*/, "", key); sub(/:.*/, "", key)
      while (depth > 0 && indents[depth] >= indent) depth--
      depth++
      indents[depth] = indent
      keys[depth] = key
      path = keys[1]
      for (i = 2; i <= depth; i++) path = path "." keys[i]
      if (path == target) { print NR; exit }
    }
  ' "$1"
}

# yaml_set_scalar <file> <dotted.path> <value>
# Returns 1 without touching the file when the path is absent, so a caller can
# treat "this values file has no worker image" as a skip rather than an error.
# A path yq can see but the walk cannot reach is a hard failure, never a silent
# skip: that is the case where a promotion would quietly not happen.
yaml_set_scalar() {
  local file="$1" path="$2" value="$3" line

  [[ -f "$file" ]] || return 1
  [[ "$(yq "${path} // \"\"" "$file" 2>/dev/null)" != "" ]] || return 1

  line="$(yaml_scalar_line "$file" "$path")"
  if [[ ! "$line" =~ ^[0-9]+$ ]]; then
    printf '✗ %s: %s exists but is not a block-style key — cannot edit it in place\n' \
      "$file" "$path" >&2
    exit 1
  fi

  # Rewrite the value between the key and any trailing comment, leaving the
  # indentation, the key, and the comment exactly as they were. Writing through a
  # temp file means a failure cannot truncate a committed values file.
  local tmp
  tmp="$(mktemp)"
  awk -v n="$line" -v v="$value" '
    NR != n { print; next }
    {
      key = $0; sub(/:.*/, ":", key)
      rest = substr($0, length(key) + 1)
      comment = (match(rest, /#/)) ? substr(rest, RSTART) : ""
      printf "%s \"%s\"%s%s\n", key, v, (comment == "" ? "" : "   "), comment
    }
  ' "$file" >"$tmp"
  mv "$tmp" "$file"
}

#!/usr/bin/env bash
# Rename a freshly generated project (ADR-0106, ADR-0003).
#
#   scripts/project-rename.sh <project-slug> <module-path> <apex-host> <image-registry>
#
# Run once, by Copier, in the generated project — never in the template. Copier
# renders only `*.jinja` files (see copier.yml for why), so the strings a project
# must own are replaced here instead, literally: the Go module path, the apex
# host and its per-environment names, and the registry namespace.
#
# Literal replacement rather than templating is also what keeps the template
# runnable: the tree a contributor clones is the tree CI builds, with no
# placeholder that only resolves after generation.
set -euo pipefail
# shellcheck source=lib/log.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib/log.sh"

[ "$#" -eq 4 ] || fail "usage: project-rename.sh <project-slug> <module-path> <apex-host> <image-registry>"
slug="$1" module="$2" apex="$3" registry="$4"

cd "$(dirname "${BASH_SOURCE[0]}")/.."

# The guard against running this in the template itself. Copier writes the
# answers file before it runs its tasks, so its presence is what distinguishes a
# generated project from a clone of the template.
[ -f .copier-answers.yml ] ||
  fail "no .copier-answers.yml — this runs in a generated project, not in the template"

# What the template calls itself. Read from go.mod rather than hard-coded, so a
# template that is itself renamed does not leave this script pointing at a name
# nothing uses.
old_module="$(awk '/^module /{print $2; exit}' go.mod)"
old_registry="ghcr.io/tabmadi/microservices-monorepo-template"
old_apex="example.com"

[ "$old_module" != "$module" ] || fail "the module path is already ${module}"

# The file list is a text search over the working tree. A generated project is
# not a git repository yet — Copier runs this before the first commit — so
# `git ls-files` has nothing to answer with, and grep must therefore be told
# which directories are not source.
step "renaming to ${slug}"
PRUNE=(--exclude-dir=.git --exclude-dir=node_modules --exclude-dir=.rumdl_cache --binary-files=without-match)

replace() { # <from> <to>
  # `|` is the sed delimiter, so it is the one character an argument may not
  # carry unescaped. Nothing here can contain one — a module path, a host and a
  # registry namespace are all URL-shaped — but escaping it costs one expansion.
  local from="${1//|/\\|}" to="${2//|/\\|}"
  grep -rlZ -F "${PRUNE[@]}" -- "$1" . 2>/dev/null |
    xargs -0 -r sed -i "s|${from}|${to}|g"
}

replace "$old_module" "$module"
detail "module path → ${module}"
replace "$old_registry" "$registry"
detail "images → ${registry}"
# The environment hosts before the apex, so `dev.example.com` does not become
# `dev.<apex>.com` by way of the shorter match.
for env in dev staging prod; do
  replace "${env}.${old_apex}" "${env}.${apex}"
done
replace "mail.${old_apex}" "mail.${apex}"
replace "$old_apex" "$apex"
detail "hosts → ${apex}"

ok "renamed to ${slug} — run 'mise run gen' and 'mise run check' before the first commit"

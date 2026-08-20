#!/usr/bin/env bash
# The files this repository's gates act on, enumerated one way for all of them.
#
# `repo_files` prints every file, `sh_files` the shell scripts, both NUL-delimited.
# A shared helper rather than a line repeated per gate, because the gates must
# agree: a formatter and a linter that enumerate differently produce a repository
# that is formatted and unlinted, or the reverse, and neither reports anything.
#
# # Why this is not one `git ls-files`
#
# It was, and the failure it produced is the one ADR-0000's standing findings warn
# about. Outside a git work tree — which is exactly where `act` runs the CI jobs,
# because it copies the workspace without `.git` — `git ls-files` prints nothing and
# exits 0. `format:shell` then died confusingly (`-w cannot be used on standard
# input`), and `lint:shell` did something worse: it reported `✓ no shell scripts to
# lint` and PASSED, having checked none of 78 scripts. A gate that cannot resolve
# its input must fail, never skip.
#
# So git remains the source of truth where there is one, `find` covers the case
# where there is not, and an empty result is an error in every caller.
# `repo_source` names which path was taken, so a run that checked the wrong set is
# legible afterwards.

if [[ -n "${__REPO_FILES_LOADED:-}" ]]; then return 0 2>/dev/null || true; fi
__REPO_FILES_LOADED=1

# repo_source — prints `git` or `find`, whichever the enumerators will use.
repo_source() {
  if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    printf 'git'
  else
    printf 'find'
  fi
}

# sh_files — prints every shell script, NUL-delimited, from the repository root.
#
# The `find` branch mirrors what `git ls-files` would return closely enough for a
# gate: it excludes the directories .gitignore excludes and nothing else. It can
# include an untracked script, which is the safe direction — checking one file too
# many costs a second, and checking one too few is how a broken script ships.
sh_files() {
  if [[ "$(repo_source)" == "git" ]]; then
    git ls-files -z '*.sh'
    return
  fi
  prune_find -name '*.sh'
}

# repo_files — every file a gate should consider, NUL-delimited.
#
# Tracked AND untracked-but-not-ignored under git, because a generated file that
# is new is still drift; everything but the pruned directories otherwise.
repo_files() {
  if [[ "$(repo_source)" == "git" ]]; then
    git ls-files -z
    git ls-files -z --others --exclude-standard
    return
  fi
  prune_find
}

# prune_find — `find` over the repository with the vendored and generated trees
# excluded, matching what .gitignore keeps out of `git ls-files`.
prune_find() {
  find . \
    -type d \( -name .git -o -name node_modules -o -name .next -o -name dist -o -name vendor \) -prune \
    -o -type f "$@" -print0
}

# sh_source — the historical name, kept because the shell gates read well with it.
sh_source() { repo_source; }

#!/usr/bin/env bash
# Branch facts for the readiness gate, read-only git only. Sourced by
# scripts/branch-ready.sh (and tested by branch_test.sh); run with the working
# directory at go/. `next` names the backlog file and defaults to
# NEXT-ITERATIONS.md.

# archived_files <base> — the .md files ADDED under .claude/archive/go between
# <base> and HEAD, one per line. Only added files count: a later touch of an
# archive file is not a "done" event.
archived_files() {
  git diff --name-only --diff-filter=A "$1" HEAD -- ':(top).claude/archive/go' | grep '\.md$' || true
}

# backlog_state <base> — `changed` when the backlog differs from <base>, else
# `delivered <n>` when n > 0 entries were archived on the branch (an entry
# planned and archived on one branch leaves the backlog identical to the base),
# else nothing.
backlog_state() {
  local base="$1" n
  if ! git diff --quiet "$base" HEAD -- "${next:-NEXT-ITERATIONS.md}"; then
    echo "changed"
    return 0
  fi
  n=$(archived_files "$base" | grep -c . || true)
  if [[ "$n" -gt 0 ]]; then
    echo "delivered $n"
  fi
}

# dependency_only <base> — succeeds when the branch changed something under go/
# and every changed path under go/ between <base> and HEAD is go.mod or go.sum
# (a Dependabot or manual dependency update). Paths outside go/ count neither
# way. --relative prints the paths relative to go/; without it git prints
# repository-root paths.
dependency_only() {
  local changed
  changed=$(git diff --name-only --relative "$1" HEAD -- .)
  [[ -n "$changed" ]] || return 1
  ! grep -qvxE 'go\.(mod|sum)' <<<"$changed"
}

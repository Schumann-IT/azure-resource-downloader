#!/usr/bin/env bash
# Tests for the branch facts in lib/branch.sh: whether a branch touched the
# backlog, delivered it by archiving an entry, or changed only go.mod / go.sum
# (dependency-only). They run against throw-away
# git repositories under a temp directory (removed on exit), never this checkout.
#
# Usage: scripts/lib/branch_test.sh   (normally via `make test-scripts`)
set -uo pipefail

if ! command -v git >/dev/null 2>&1; then
  echo "ℹ️  git not found — skipping the branch tests"
  exit 0
fi

lib="$(cd "$(dirname "$0")" && pwd)/branch.sh"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
next="NEXT-ITERATIONS.md"
# shellcheck source=branch.sh
source "$lib"

passed=0
failed=0
assert_eq() {
  local name="$1" expected="$2" actual="$3"
  if [[ "$expected" == "$actual" ]]; then
    passed=$((passed + 1))
  else
    failed=$((failed + 1))
    echo "❌ $name" >&2
    echo "   expected: $(printf '%q' "$expected")" >&2
    echo "   actual:   $(printf '%q' "$actual")" >&2
  fi
}

g() {
  git -c user.name=test -c user.email=test@example.invalid -c commit.gpgsign=false "$@" >/dev/null
}

# new_repo <name> — a repository on main with a committed go/ and archive layout;
# leaves the working directory at go/ on a new branch `work`.
new_repo() {
  local dir="$tmp/$1"
  mkdir -p "$dir/go" "$dir/.claude/archive/go" "$dir/.claude/archive/web"
  git -C "$dir" init -q -b main
  printf '# Next iterations\n' >"$dir/go/NEXT-ITERATIONS.md"
  printf '# Changelog\n' >"$dir/go/CHANGELOG.md"
  : >"$dir/.claude/archive/go/.gitkeep"
  : >"$dir/.claude/archive/web/.gitkeep"
  cd "$dir" || exit 1
  g add -A
  g commit -q -m "chore: seed"
  g checkout -q -b work
  cd go || exit 1
}

commit_all() {
  g -C "$(git rev-parse --show-toplevel)" add -A
  g commit -q -m "$1"
}

# 1. Only another go/ file changed.
new_repo other
echo x >other.txt
commit_all "feat(go): other"
base=$(git merge-base HEAD main)
assert_eq "other file only: state empty" "" "$(backlog_state "$base")"
assert_eq "other file only: no archived files" "" "$(archived_files "$base")"

# 2. Entry added, then archived: net-zero backlog diff, one new archive file.
new_repo netzero
printf '# Next iterations\n\n## 1. Thing\n' >NEXT-ITERATIONS.md
commit_all "docs(go): plan thing"
printf '# Next iterations\n' >NEXT-ITERATIONS.md
printf -- '---\nstatus: done\n---\n' >../.claude/archive/go/2026-10-01-thing.md
echo note >../.claude/archive/go/notes.txt
commit_all "docs(go): close thing"
base=$(git merge-base HEAD main)
assert_eq "net-zero backlog: delivered 1" "delivered 1" "$(backlog_state "$base")"
assert_eq "archived_files lists exactly the added go file" \
  ".claude/archive/go/2026-10-01-thing.md" "$(archived_files "$base")"

# 3. Backlog edited.
new_repo edited
printf '# Next iterations\n\n## 1. Thing\n' >NEXT-ITERATIONS.md
commit_all "docs(go): plan thing"
base=$(git merge-base HEAD main)
assert_eq "backlog edited: changed" "changed" "$(backlog_state "$base")"

# 4. Backlog edited and an entry archived: changed wins.
new_repo both
printf '# Next iterations\n\n## 1. Other\n' >NEXT-ITERATIONS.md
printf -- '---\nstatus: dropped\n---\n' >../.claude/archive/go/2026-10-01-gone.md
commit_all "docs(go): plan and drop"
base=$(git merge-base HEAD main)
assert_eq "edited and archived: changed" "changed" "$(backlog_state "$base")"

# 5. Only the other project's archive grew.
new_repo webonly
printf -- '---\nstatus: done\n---\n' >../.claude/archive/web/2026-10-01-thing.md
echo x >other.txt
commit_all "feat(go): other"
base=$(git merge-base HEAD main)
assert_eq "web archive only: state empty" "" "$(backlog_state "$base")"
assert_eq "web archive only: no go archived files" "" "$(archived_files "$base")"

# dep_only <base> — dependency_only as a word, for assert_eq.
dep_only() {
  if dependency_only "$1"; then echo yes; else echo no; fi
}

# 6. go.mod and go.sum changed: a dependency-only branch.
new_repo depboth
printf 'module x\n' >go.mod
printf 'sum\n' >go.sum
commit_all "build(go): bump modules"
base=$(git merge-base HEAD main)
assert_eq "go.mod + go.sum: dependency-only" "yes" "$(dep_only "$base")"
assert_eq "go.mod + go.sum: backlog state empty" "" "$(backlog_state "$base")"

# 7. go.sum alone: a dependency-only branch.
new_repo depsum
printf 'sum\n' >go.sum
commit_all "build(go): bump a transitive module"
base=$(git merge-base HEAD main)
assert_eq "go.sum alone: dependency-only" "yes" "$(dep_only "$base")"

# 8. go.mod and a .go file: not dependency-only, and the backlog is untouched,
#    so the gate fails as before.
new_repo depcode
printf 'module x\n' >go.mod
printf 'package main\n' >main.go
commit_all "build(go): bump and fix a call site"
base=$(git merge-base HEAD main)
assert_eq "go.mod + .go file: not dependency-only" "no" "$(dep_only "$base")"
assert_eq "go.mod + .go file: backlog state empty" "" "$(backlog_state "$base")"

# 9. go.mod and the backlog: not dependency-only; the backlog changed.
new_repo depbacklog
printf 'module x\n' >go.mod
printf '# Next iterations\n\n## 1. Thing\n' >NEXT-ITERATIONS.md
commit_all "build(go): bump and plan"
base=$(git merge-base HEAD main)
assert_eq "go.mod + backlog: not dependency-only" "no" "$(dep_only "$base")"
assert_eq "go.mod + backlog: changed" "changed" "$(backlog_state "$base")"

# 10. A change only outside go/: not dependency-only (nothing under go/ changed).
new_repo depoutside
mkdir -p ../.github
printf 'version: 2\n' >../.github/dependabot.yml
commit_all "ci: add dependabot"
base=$(git merge-base HEAD main)
assert_eq "outside go/ only: not dependency-only" "no" "$(dep_only "$base")"

# 11. go.mod plus a change outside go/: dependency-only (outside paths count
#     neither way).
new_repo depmixed
printf 'module x\n' >go.mod
mkdir -p ../.github
printf 'version: 2\n' >../.github/dependabot.yml
commit_all "build(go): bump with dependabot config"
base=$(git merge-base HEAD main)
assert_eq "go.mod + outside go/: dependency-only" "yes" "$(dep_only "$base")"

if [[ "$failed" -gt 0 ]]; then
  echo "❌ branch_test: $failed of $((passed + failed)) assertions failed" >&2
  exit 1
fi
echo "✅ branch_test: $passed assertions passed"

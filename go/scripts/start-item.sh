#!/usr/bin/env bash
# Start gate for go/: may implementation of entry N begin? It refuses on the
# release branch, on a dirty go/ tree, when entry N is not in HEAD's
# NEXT-ITERATIONS.md (the backlog must be committed before it is implemented),
# and when the entry has no outstanding plan item. On success it prints the
# entry's title, Goal and Plan. It changes nothing; the only git it runs is
# read-only (`rev-parse`, `status`, `show`). Outside a clone it reads the
# working copy and says so, so the gate stays usable there.
#
# Exit codes: 0 ready, 1 a check failed, 2 usage.
# Usage: scripts/start-item.sh N   (normally via `make start-item N=<n>`)
set -uo pipefail

cd "$(dirname "$0")/.."
changelog="CHANGELOG.md"
next="NEXT-ITERATIONS.md"
# shellcheck source=lib/changelog.sh
source scripts/lib/changelog.sh

n="${1:-}"
if [[ ! "$n" =~ ^[1-9][0-9]*$ ]]; then
  echo "usage: start-item N (a positive entry number) — e.g. make start-item N=1" >&2
  exit 2
fi
rb="${RELEASE_BRANCH:-main}"

fail() {
  echo "❌ $1" >&2
  exit 1
}

content=""
if ! command -v git >/dev/null 2>&1 || ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  echo "ℹ️  git not found or not a repository — reading the working copy of $next; branch and clean-tree checks skipped"
  content=$(cat "$next")
else
  # 1. Not on the release branch, and not detached.
  branch=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo HEAD)
  if [[ "$branch" == "HEAD" ]]; then
    fail "detached HEAD — check out a branch first"
  elif [[ "$branch" == "$rb" ]]; then
    fail "on branch '$branch' — implementation happens on a branch off $rb: git switch -c <type>/<slug>"
  fi
  echo "✅ on branch '$branch' (not $rb)"

  # 2. go/ has no uncommitted changes (its own message on failure).
  scripts/working-tree-clean.sh || exit 1

  # 3. The backlog is committed: read entry N from HEAD, never from the editor.
  if ! content=$(git show "HEAD:./$next" 2>/dev/null); then
    fail "$next is not committed — commit the backlog before implementing"
  fi
fi

# 4. Entry N exists in that content.
section=$(printf '%s\n' "$content" | entry_section_in "$n")
if [[ -z "$section" ]]; then
  numbers=$(printf '%s\n' "$content" | entry_numbers_in | paste -sd, - | sed 's/,/, /g')
  fail "entry $n is not in HEAD's $next — committed entries: ${numbers:-none} (edit and commit the backlog first)"
fi
title=$(printf '%s\n' "$section" | entry_title_in)

# 5. It has a Plan with at least one outstanding item.
plan=$(printf '%s\n' "$section" | entry_plan_in)
if [[ -z "$plan" ]]; then
  fail "entry $n has no **Plan.** block — add concrete work items before starting"
fi
open=$(printf '%s\n' "$section" | plan_open_items_in)
if printf '%s\n' "$section" | entry_title_struck_in; then
  if [[ -n "$open" ]]; then
    fail "entry $n's title is struck but plan items are open — unstrike the title or strike the items"
  fi
  fail "entry $n is fully delivered — say \"item $n is done\" to archive it"
fi
if [[ -z "$open" ]]; then
  fail "entry $n has no outstanding plan items — say \"item $n is done\" to archive it"
fi

echo ""
echo "## $n. $title"
echo ""
printf '%s\n' "$section" | entry_goal_in
echo ""
printf '%s\n' "$plan"
echo ""
echo "✅ go/: item $n is ready to start — $title ($(printf '%s\n' "$open" | wc -l | tr -d ' ') open plan item(s))"

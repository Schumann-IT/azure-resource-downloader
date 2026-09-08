#!/usr/bin/env bash
# Preflight for `make branch-ready`: refuse to report on a dirty tree, so the
# verdict describes the commit that will be merged and not whatever happens to
# be open in the editor. It runs before `make ci`, so a dirty tree costs nothing
# to discover.
#
# This is the one git command in go/'s readiness tooling (the Makefile's
# `git describe` for the version stamp aside), it is read-only
# (`git status --porcelain`), and it is scoped to go/ so an unrelated edit in
# web/ cannot block this project's gate. `release-ready` still runs no git at
# all, and the repository-wide branch and working-tree checks still live only in
# the root release script.
#
# A missing git or a checkout that is not a repository is reported and waved
# through rather than failed: the gate must stay usable outside a clone.
#
# Usage: scripts/working-tree-clean.sh   (normally via `make branch-ready`)
set -uo pipefail

cd "$(dirname "$0")/.."
max_listed=20

if ! command -v git >/dev/null 2>&1; then
  echo "ℹ️  git not found — skipping the clean-tree check"
  exit 0
fi
if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  echo "ℹ️  not a git repository — skipping the clean-tree check"
  exit 0
fi

dirty=$(git status --porcelain -- . | grep -v '^[[:space:]]*$' || true)
if [[ -z "$dirty" ]]; then
  echo "✅ go/ has no uncommitted changes"
  exit 0
fi

count=$(printf '%s\n' "$dirty" | wc -l | tr -d ' ')
echo "❌ go/ has uncommitted changes — commit or stash them, then run this again:" >&2
printf '%s\n' "$dirty" | head -n "$max_listed" >&2
if [[ "$count" -gt "$max_listed" ]]; then
  echo "… and $((count - max_listed)) more" >&2
fi
exit 1

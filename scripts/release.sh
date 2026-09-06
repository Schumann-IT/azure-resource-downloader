#!/usr/bin/env bash
# Publishes releases. A project has a new version when the newest version
# section of its CHANGELOG.md is a bare, undated `## [X.Y.Z]` heading — the
# state a hand-closed [Unreleased] section leaves behind. The per-project
# readiness reports (`make -C go release-ready` / `npm --prefix web run
# release-ready`) run before this script as prerequisites of the root Makefile
# targets, so they are not repeated here. Publishing checks that the checkout is
# on the release branch with a clean working tree (these two checks live ONLY
# here), then stamps today's date onto each pending heading
# (`## [X.Y.Z] - YYYY-MM-DD`), commits, tags that commit, pushes the branch and
# the tags, and creates one GitHub release per project with that changelog
# section as notes. Projects whose newest version is already dated are skipped,
# so the script is safe to run when only one project changed.
# See README.md#releasing.
#
# Usage: scripts/release.sh [status|publish]   (normally via `make release-status` / `make release`)
# Env:   RELEASE_BRANCH  branch releases are cut from (default: main)
set -uo pipefail
cd "$(dirname "$0")/.."

mode="${1:-publish}"
projects="go web"
branch_expected="${RELEASE_BRANCH:-main}"

# Newest version heading in a project's changelog (sections are newest-first).
newest_heading() {
  grep -m 1 -E '^## \[[0-9]+\.[0-9]+\.[0-9]+\]' "$1/CHANGELOG.md" || true
}

# Version number of a heading such as `## [1.2.3]` or `## [1.2.3] - 2026-09-07`.
heading_version() {
  printf '%s' "$1" | sed -n 's/^## \[\([0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*\)\].*/\1/p'
}

# Body of one released section: everything between its heading and the next `## `.
release_notes() {
  awk -v v="$2" '$0 ~ "^## \\[" v "\\] - " {f=1; next} /^## / {f=0} f' "$1/CHANGELOG.md"
}

pending=""
for p in $projects; do
  heading=$(newest_heading "$p")
  v=$(heading_version "$heading")
  if [[ -z "$v" ]]; then
    echo "⏭  $p: no version section in $p/CHANGELOG.md"
    continue
  fi
  tag="$p/v$v"
  if [[ "$heading" != "## [$v]" ]]; then
    echo "✅ $p: v$v is already released ($heading)"
  elif git rev-parse -q --verify "refs/tags/$tag" >/dev/null; then
    echo "❌ $p: v$v is undated in $p/CHANGELOG.md but $tag already exists — pick a new version" >&2
    conflict=1
  else
    echo "🚀 $p: v$v is closed and undated → would publish $tag"
    pending="$pending $p:$v"
  fi
done
if [[ -n "${conflict:-}" ]]; then
  exit 1
fi

if [[ "$mode" == "status" ]]; then
  exit 0
fi
if [[ "$mode" != "publish" ]]; then
  echo "usage: $0 [status|publish]" >&2
  exit 2
fi
if [[ -z "$pending" ]]; then
  echo "Nothing to release."
  exit 0
fi

# Repository state: release branch, clean tree. Checked here and nowhere else.
branch=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "?")
if [[ "$branch" != "$branch_expected" ]]; then
  echo "❌ on branch '$branch', but releases are cut from '$branch_expected' (override with RELEASE_BRANCH)" >&2
  exit 1
fi
if [[ -n "$(git status --porcelain)" ]]; then
  echo "❌ the working tree has uncommitted changes — commit or stash them first:" >&2
  git status --short >&2
  exit 1
fi

# Publisher preflight.
if ! command -v gh >/dev/null 2>&1; then
  echo "❌ gh CLI not found — install it from https://cli.github.com and run 'gh auth login'" >&2
  exit 1
fi
if ! gh auth status >/dev/null 2>&1; then
  echo "❌ gh is not authenticated; run 'gh auth login'" >&2
  exit 1
fi

set -e

# Stamp the release date onto each pending heading and commit — the only edit
# this workflow makes to a changelog.
today=$(date +%Y-%m-%d)
subject=""
for entry in $pending; do
  p=${entry%%:*}
  v=${entry#*:}
  sed -i.bak "s/^## \[$v\]\$/## [$v] - $today/" "$p/CHANGELOG.md" && rm -f "$p/CHANGELOG.md.bak"
  git add "$p/CHANGELOG.md"
  subject="$subject${subject:+, }$p v$v"
done
git commit -q -m "release: $subject"
echo "📝 stamped $today into $(git diff-tree --no-commit-id --name-only -r HEAD | tr '\n' ' ')"

echo "⬆️  Pushing $(git rev-parse --abbrev-ref HEAD)..."
git push -q origin HEAD

for entry in $pending; do
  p=${entry%%:*}
  v=${entry#*:}
  tag="$p/v$v"
  git tag -a "$tag" -m "$p v$v"
  git push -q origin "$tag"
  release_notes "$p" "$v" | gh release create "$tag" --title "$p v$v" --notes-file -
  echo "✅ published $tag"
done

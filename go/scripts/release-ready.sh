#!/usr/bin/env bash
# Release-readiness report for go/. It changes nothing — no file edits, no
# commits, no tags — and runs no git command at all. Closing the changelog
# (`## [Unreleased]` emptied into a new, undated `## [X.Y.Z]` section) is done by
# hand; this script only reports whether that state has been reached. The
# release date is stamped onto that heading by the root release script.
# With no undated heading there is nothing to release and the script reports
# success ("no release needed"). Otherwise every check is run and reported, and
# the script exits non-zero only when ALL checks fail. Branch and working-tree
# checks, tagging and the GitHub release live at the repository root
# (`make release`). See ../../README.md#development-workflow (step 4, Release).
#
# Usage: scripts/release-ready.sh          (normally via `make release-ready`)
set -uo pipefail

cd "$(dirname "$0")/.."
changelog="CHANGELOG.md"
next="NEXT-ITERATIONS.md"
# shellcheck source=lib/changelog.sh
source scripts/lib/changelog.sh
passed=0
failed=0

ok() {
  echo "✅ $1"
  passed=$((passed + 1))
}

fail() {
  echo "❌ $1" >&2
  failed=$((failed + 1))
}

# Is a release pending? An undated newest heading means "closed, awaiting
# release"; a dated or missing one means nothing to do (see lib/changelog.sh).
if [[ ! -f "$changelog" ]]; then
  echo "❌ $changelog not found" >&2
  exit 1
fi
heading=$(newest_heading)
version=$(newest_version)
if ! awaiting_release; then
  if [[ -z "$version" ]]; then
    echo "ℹ️  $changelog has no version section yet"
  else
    echo "ℹ️  newest changelog version $version is already released ($heading)"
  fi
  pending_count=$(unreleased_items | wc -l | tr -d ' ')
  if [[ "$pending_count" -gt 0 ]]; then
    echo "ℹ️  [Unreleased] has $pending_count line(s) waiting — close it as a new '## [X.Y.Z]' section when you want to release"
  fi
  echo ""
  echo "✅ go/: no release needed"
  exit 0
fi
echo "🚀 newest changelog version $version is closed and awaiting release (go/v$version)"

# 1. No struck-out entries in NEXT-ITERATIONS.md. A strikeout marks work that
#    shipped and is still waiting to be cleared out; `make branch-ready` gates
#    that at branch close, this is the release-time backstop.
if [[ ! -f "$next" ]]; then
  fail "$next not found"
else
  struck=$(struck_lines)
  if [[ -n "$struck" ]]; then
    fail "$next has struck-out entries — delete them (and check $changelog records the work):"
    echo "$struck" >&2
  else
    ok "$next has no struck-out entries"
  fi
fi

# 2. CHANGELOG.md has an empty `## [Unreleased]` section: everything pending has
#    already been moved into the version section that is about to be released.
if ! has_unreleased; then
  fail "$changelog has no '## [Unreleased]' section"
else
  unreleased=$(unreleased_items)
  if [[ -n "$unreleased" ]]; then
    fail "$changelog still has items under [Unreleased] — move them into the [$version] section first:"
    echo "$unreleased" | head -n 5 >&2
  else
    ok "$changelog has an empty [Unreleased] section"
  fi
fi

echo ""
if [[ "$passed" -eq 0 ]]; then
  echo "❌ go/ is not ready to release: all $failed checks failed" >&2
  exit 1
fi
if [[ "$failed" -gt 0 ]]; then
  echo "⚠️  go/: $passed of $((passed + failed)) checks passed — resolve the ❌ items above before running 'make release' at the repository root"
else
  echo "✅ go/ is ready to release v$version — run 'make release' from the repository root to tag and publish go/v$version"
fi

#!/usr/bin/env bash
# Branch-readiness report for go/: is this feature or fix branch ready to ship?
# It changes nothing — no file edits, no commits, no tags — and runs no git
# command, exactly like the release report it complements. (The clean-tree
# preflight that `make branch-ready` runs before it is a separate script.)
#
# It asks the opposite questions to `release-ready`. A finished branch has
# recorded its work under `## [Unreleased]` and has cleared the entries it
# delivered out of NEXT-ITERATIONS.md (they are struck through while the work is
# in progress). Closing the changelog into a new `## [X.Y.Z]` is the release
# step, which `release-ready` reports on. There is no version file to keep
# untouched: this project's version is the `go/vX.Y.Z` tag.
#
# Unlike that report, this one is a gate: it exits non-zero if a single check
# fails.
#
# Usage: scripts/branch-ready.sh   (normally via `make branch-ready`)
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

info() {
  echo "ℹ️  $1"
}

if [[ ! -f "$changelog" ]]; then
  echo "❌ $changelog not found" >&2
  exit 1
fi

# 1. Nothing struck out in NEXT-ITERATIONS.md. A strikeout marks work that
#    shipped and is waiting to be cleared out; clearing it is part of closing
#    the branch, so the entry must be gone before the branch is merged.
if [[ ! -f "$next" ]]; then
  fail "$next not found"
else
  struck=$(struck_lines)
  if [[ -n "$struck" ]]; then
    fail "$next still has struck-out entries — delete them now that the work is done (their $changelog entry is the record):"
    echo "$struck" >&2
  else
    ok "$next has no struck-out entries left"
  fi

  # 2. Numbering is contiguous. Clearing entries out shifts the numbers, so a
  #    gap or a repeat means the renumbering was forgotten.
  numbers=$(entry_numbers | paste -sd, -)
  count=$(entry_numbers | wc -l | tr -d ' ')
  expected=$(seq -s, 1 "$count" 2>/dev/null || true)
  if [[ "$numbers" != "$expected" ]]; then
    fail "$next entries are numbered ${numbers//,/, } — renumber them 1..$count after clearing entries out"
  elif [[ "$count" -eq 0 ]]; then
    ok "$next has no numbered entries (parked ideas only)"
  else
    ok "$next entries are numbered 1..$count"
  fi
fi

# 3. The branch recorded its work under an open `## [Unreleased]`. An empty one
#    is legitimate for a branch with no user- or operator-visible effect, so it
#    is reported rather than failed; a closed changelog means this is a release
#    branch and `release-ready` is the report to run instead.
if ! has_unreleased; then
  fail "$changelog has no '## [Unreleased]' section — add one above the newest version"
elif awaiting_release; then
  info "$changelog is closed for release ($(newest_heading)) — run 'make release-ready' for that flow"
else
  unreleased_count=$(unreleased_items | wc -l | tr -d ' ')
  if [[ "$unreleased_count" -eq 0 ]]; then
    info "$changelog has an empty [Unreleased] section — fine only if this branch changed nothing a user or operator can notice"
  else
    ok "$changelog records $unreleased_count line(s) under [Unreleased]"
  fi
fi

echo ""
if [[ "$failed" -gt 0 ]]; then
  echo "❌ go/: $failed of $((passed + failed)) checks failed — resolve the ❌ items above before closing the branch" >&2
  exit 1
fi
echo "✅ go/: branch is ready to ship ($passed checks passed)"

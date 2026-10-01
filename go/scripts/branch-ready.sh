#!/usr/bin/env bash
# Branch-readiness report for go/: is this feature or fix branch ready to ship?
# It changes nothing — no file edits, no commits, no tags. The only git it runs
# is read-only (`rev-parse`, `merge-base`, `diff`, `show`), and outside a clone
# those checks are skipped with a note. (The clean-tree preflight that
# `make branch-ready` runs before it is a separate script.)
#
# It asks the opposite questions to `release-ready`. A finished branch has
# recorded its work under `## [Unreleased]`, has archived the entries it
# delivered out of NEXT-ITERATIONS.md (they are struck through while the work is
# in progress; "item N is done" moves them to ../.claude/archive/go/), touched
# the backlog at all (every change starts as an entry), and was not made on the
# release branch. Closing the changelog into a new `## [X.Y.Z]` is the release
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
# shellcheck source=lib/branch.sh
source scripts/lib/branch.sh
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
    fail "$next still has struck-out entries — archive them (say \"item N is done\") now that the work is done; their $changelog entry is the record:"
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

# 4–7. Branch checks, read-only git. Skipped outside a clone so the gate stays
#      usable there.
rb="${RELEASE_BRANCH:-main}"
if ! command -v git >/dev/null 2>&1 || ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  info "git not found or not a repository — skipping the branch checks"
else
  # 4. Not on the release branch: implementation ships from a branch off it.
  branch=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo HEAD)
  if [[ "$branch" == "$rb" || "$branch" == "HEAD" ]]; then
    fail "on '$branch' — implementation ships from a branch off $rb, not from $rb itself"
  else
    ok "on branch '$branch' (not $rb)"
  fi

  # Where this branch left the release branch. Without it the diff-based checks
  # have nothing to compare against.
  base=$(git merge-base HEAD "$rb" 2>/dev/null || git merge-base HEAD "origin/$rb" 2>/dev/null || true)
  if [[ -z "$base" ]]; then
    info "neither '$rb' nor 'origin/$rb' exists — skipping the backlog and archive checks"
  elif git diff --quiet "$base" HEAD -- .; then
    info "no changes in go/ on this branch — skipping the backlog and archive checks"
  else
    # 5. Every branch that changes go/ touches its backlog: it delivers, refines
    #    or adds an entry. A small fix still gets a small entry. An entry planned
    #    and archived on the same branch leaves the backlog identical to the
    #    base, so an archived entry (the archive file is the evidence) counts too.
    state=$(backlog_state "$base")
    case "$state" in
      changed) ok "$next changed on this branch" ;;
      delivered\ *) ok "$next delivered on this branch (${state#delivered } archived entry(ies))" ;;
      *) fail "$next is unchanged on this branch — every branch that changes go/ delivers, refines or adds an entry (a small fix still gets a small entry)" ;;
    esac

    # 6. Every commit on the branch follows Conventional Commits (the squash
    #    merge takes the pull request title, but the branch history is what a
    #    reviewer reads). Merge commits are skipped.
    bad_subjects=$(git log --no-merges --format=%s "$base"..HEAD | unconventional_subjects_in)
    if [[ -n "$bad_subjects" ]]; then
      fail "commit subject(s) on this branch do not follow Conventional Commits (type(go|web|release)!: lowercase description, no trailing period):"
      printf '%s\n' "$bad_subjects" | sed 's|^|   |' >&2
    else
      ok "every commit subject on this branch follows Conventional Commits"
    fi

    # 7. An entry archived as done must be recorded under [Unreleased]. Only
    #    files ADDED on the branch count: a later touch of an archive file is not
    #    a "done" event. Archive file names are <date>-<slug>.md, never with
    #    spaces, so the unquoted loop is safe.
    archived=$(archived_files "$base")
    done_count=0
    bad=""
    for f in $archived; do
      status=$(git show "HEAD:$f" | frontmatter_value_in status)
      case "$status" in
        done) done_count=$((done_count + 1)) ;;
        dropped) ;;
        *) bad="$bad $f" ;;
      esac
    done
    if [[ -n "$bad" ]]; then
      fail "archive file(s) without 'status: done' or 'status: dropped' frontmatter:$bad"
    fi
    if [[ "$done_count" -eq 0 ]]; then
      info "no entries archived as done on this branch"
    elif ! has_unreleased || awaiting_release; then
      info "$done_count entry(ies) archived and the changelog is closed — 'make release-ready' is the report for that"
    else
      base_count=$(git show "$base:./$changelog" 2>/dev/null | unreleased_items_in | wc -l | tr -d ' ')
      head_count=$(unreleased_items | wc -l | tr -d ' ')
      if [[ "$head_count" -gt "$base_count" ]]; then
        ok "$done_count archived entry(ies) are recorded under [Unreleased] ($base_count → $head_count line(s))"
      else
        fail "$done_count entry(ies) archived as done but [Unreleased] in $changelog did not grow ($base_count → $head_count line(s)) — record the work"
      fi
    fi
  fi
fi

echo ""
if [[ "$failed" -gt 0 ]]; then
  echo "❌ go/: $failed of $((passed + failed)) checks failed — resolve the ❌ items above before closing the branch" >&2
  exit 1
fi
echo "✅ go/: branch is ready to ship ($passed checks passed)"

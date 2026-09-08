# Readers shared by the two readiness reports (branch-ready.sh, release-ready.sh)
# so both agree on what "closed", "[Unreleased] is empty" and "struck out" mean.
# Read-only by design: no file edits and no git commands in either report.
#
# Source this file from a script whose cwd is go/; it expects $changelog and
# $next to name CHANGELOG.md and NEXT-ITERATIONS.md.

# Non-blank lines under `## [Unreleased]`, up to the next `## ` heading.
unreleased_items() {
  awk '/^## \[Unreleased\]$/ {f=1; next} /^## / {f=0} f' "$changelog" | grep -v '^[[:space:]]*$' || true
}

# Does the changelog have a `## [Unreleased]` heading at all?
has_unreleased() {
  grep -q '^## \[Unreleased\]$' "$changelog"
}

# The newest `## [X.Y.Z]` heading line (with or without a date), or empty.
newest_heading() {
  grep -m 1 -E '^## \[[0-9]+\.[0-9]+\.[0-9]+\]' "$changelog" || true
}

# The X.Y.Z of the newest heading, or empty.
newest_version() {
  newest_heading | sed -n 's/^## \[\([0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*\)\].*/\1/p'
}

# Is a release pending? Closing [Unreleased] writes a bare `## [X.Y.Z]`; the root
# release script stamps the date when it publishes. So an undated newest heading
# means "closed, awaiting release"; a dated or missing one means it is history.
awaiting_release() {
  local heading version
  heading=$(newest_heading)
  version=$(newest_version)
  [[ -n "$version" && "$heading" == "## [$version]" ]]
}

# Struck-out lines in NEXT-ITERATIONS.md as `N:line`: work that shipped and is
# still waiting to be cleared out. The boundary checks keep a `~~~` fence or
# rule from being read as a strikeout.
struck_lines() {
  grep -nE '(^|[^~])~~[^~]' "$next" || true
}

# The `## N. Title` / `### N. Title` numbers in file order, one per line.
# Numbering is presentational and is made contiguous again whenever entries are
# cleared out, so a gap or a repeat means that renumbering was missed. Matches
# struck titles too (`## ~~5. …~~`).
entry_numbers() {
  grep -E '^#{2,3} ~*[0-9]+\.' "$next" | sed -E 's/^#{2,3} ~*([0-9]+)\..*/\1/' || true
}

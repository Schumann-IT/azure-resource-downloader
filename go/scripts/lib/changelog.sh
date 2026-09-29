# Readers shared by the readiness reports (branch-ready.sh, release-ready.sh)
# and the start gate (start-item.sh), so all of them agree on what "closed",
# "[Unreleased] is empty", "struck out" and "entry N" mean. Read-only by design:
# no file edits and no git commands in here.
#
# Every reader exists in two forms: `<name>_in` reads the content from stdin,
# which is what lets a caller feed it `git show HEAD:./FILE` or
# `git show <base>:./FILE` without a temp file; `<name>` is the wrapper over the
# working copy, using $changelog / $next, which the sourcing script sets. Source
# this file from a script whose cwd is go/.
#
# Portability: BSD awk and bash 3.2 (macOS) — no interval expressions inside
# awk regexes, no mapfile.

# Non-blank lines under `## [Unreleased]`, up to the next `## ` heading.
unreleased_items_in() {
  awk '/^## \[Unreleased\]$/ {f=1; next} /^## / {f=0} f' | grep -v '^[[:space:]]*$' || true
}
unreleased_items() { unreleased_items_in < "$changelog"; }

# Does the changelog have a `## [Unreleased]` heading at all?
has_unreleased() {
  grep -q '^## \[Unreleased\]$' "$changelog"
}

# The newest `## [X.Y.Z]` heading line (with or without a date), or empty.
newest_heading_in() {
  grep -m 1 -E '^## \[[0-9]+\.[0-9]+\.[0-9]+\]' || true
}
newest_heading() { newest_heading_in < "$changelog"; }

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
# still waiting to be archived. The boundary checks keep a `~~~` fence or rule
# from being read as a strikeout.
struck_lines_in() {
  grep -nE '(^|[^~])~~[^~]' || true
}
struck_lines() { struck_lines_in < "$next"; }

# The `## N. Title` / `### N. Title` numbers in file order, one per line.
# Numbering is presentational and is made contiguous again whenever entries are
# archived, so a gap or a repeat means that renumbering was missed. Matches
# struck titles too (`## ~~5. …~~` and `## 5. ~~…~~`).
entry_numbers_in() {
  grep -E '^#{2,3} ~*[0-9]+\.' | sed -E 's/^#{2,3} ~*([0-9]+)\..*/\1/' || true
}
entry_numbers() { entry_numbers_in < "$next"; }

# Entry N from stdin: its `## N.` / `### N.` heading line (struck or not) up to,
# not including, the next heading of the same or a higher level. Fenced code
# (``` or ~~~) is never scanned for headings, so a `#` inside a code block cannot
# end a section. Empty output when N is absent. `n=1` cannot match `## 11.`,
# because the number must be followed by `. `.
entry_section_in() {
  awk -v n="$1" '
    /^(```|~~~)/ { fence = !fence }
    !fence && /^#+ / {
      lvl = index($0, " ") - 1
      if (f && lvl <= start) exit
      if (!f && lvl >= 2 && lvl <= 3 && $0 ~ ("^#+ ~*" n "\\. ")) { f = 1; start = lvl }
    }
    f'
}
entry_section() { entry_section_in "$1" < "$next"; }

# The title of a section on stdin (its first line), with the heading marks, the
# number and any `~~` removed.
entry_title_in() {
  sed -n -E '1{s/~~//g;s/^#+ [0-9]+\. *//p;}'
}

# Is the section's title struck (`## ~~N. …~~` or `## N. ~~…~~`)?
entry_title_struck_in() {
  sed -n 1p | grep -q '~~'
}

# The Goal paragraph: from the `**Goal.**` line to the first blank line.
entry_goal_in() {
  awk '/^\*\*Goal\.\*\*/ {f=1} f && /^[[:space:]]*$/ {exit} f'
}

# The Plan block: from the `**Plan.**` line to the end of the section.
entry_plan_in() {
  awk '/^\*\*Plan\.\*\*/ {f=1} f'
}

# Top-level plan bullets (`- …`) of a section on stdin that are not struck
# (`- ~~…`), one per line. Nested bullets are continuation of their parent and
# are not counted.
plan_open_items_in() {
  entry_plan_in | grep -E '^- ' | grep -vE '^- ~~' || true
}

# Commit subjects on stdin that do not follow Conventional Commits 1.0.0 as this
# repository applies it: `type(scope)!: description` with type in feat | fix |
# docs | refactor | test | build | ci | chore | revert, scope `go`, `web` or
# `release` (or none), a lowercase description and no trailing period. One
# offending subject per output line; empty when all conform.
CONVENTIONAL_SUBJECT='^(feat|fix|docs|refactor|test|build|ci|chore|revert)(\((go|web|release)\))?!?: [a-z0-9](.*[^. ])?$'
unconventional_subjects_in() {
  grep -vE "$CONVENTIONAL_SUBJECT" || true
}

# The value of `key:` in a leading `---` frontmatter block on stdin; empty when
# there is no such block or key. Surrounding double quotes are removed.
frontmatter_value_in() {
  awk -v k="$1" '
    NR == 1 { if ($0 != "---") exit; next }
    $0 == "---" { exit }
    index($0, k ":") == 1 { sub("^" k ":[ \t]*", ""); gsub(/^"|"$/, ""); print; exit }'
}

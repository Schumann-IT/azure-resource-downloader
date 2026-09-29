#!/usr/bin/env bash
# Tests for the readers in lib/changelog.sh. They are what make the backlog
# conventions enforceable — a regression here would silently pass a branch that
# still has shipped-but-unarchived entries, or start an entry that is not
# committed — so they are covered directly. Fixtures are heredocs piped into the
# `*_in` readers; nothing is written to disk and no git runs.
#
# Usage: scripts/lib/changelog_test.sh   (normally via `make test-scripts`)
set -uo pipefail

cd "$(dirname "$0")/../.."
changelog="/dev/null"
next="/dev/null"
# shellcheck source=changelog.sh
source scripts/lib/changelog.sh

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

backlog=$(cat <<'MD'
# Next iterations

## 1. First entry

**Goal.** Do the first thing,
across two lines.

> **Note.** A note.

**Plan.**

- ~~done item~~
- open item
  - nested item does not count
- another open item

## 2. ~~Second entry~~

**Goal.** Second.

**Plan.**

- ~~all~~
- ~~done~~

## 11. Eleventh entry

**Goal.** Eleven.

**Plan.**

```
## not a heading
```

- open

## Parked ideas

### Idea: something

Text.
MD
)

nested=$(cat <<'MD'
## Features

### 1. Web entry

**Goal.** Web.

**Plan.**

- one

### 2. Another

**Goal.** Two.

## Fixes

None.
MD
)

# entry_section_in
assert_eq "section 1 starts at its heading" "## 1. First entry" "$(printf '%s\n' "$backlog" | entry_section_in 1 | head -1)"
assert_eq "section 1 ends before entry 2" "- another open item" "$(printf '%s\n' "$backlog" | entry_section_in 1 | grep -v '^$' | tail -1)"
assert_eq "section 2 matches a struck title" "## 2. ~~Second entry~~" "$(printf '%s\n' "$backlog" | entry_section_in 2 | head -1)"
assert_eq "n=1 does not match 11" "0" "$(printf '%s\n' "$backlog" | entry_section_in 1 | grep -c 'Eleven')"
assert_eq "a heading inside a fence does not end a section" "- open" "$(printf '%s\n' "$backlog" | entry_section_in 11 | grep -v '^$' | tail -1)"
assert_eq "absent entry yields nothing" "" "$(printf '%s\n' "$backlog" | entry_section_in 3)"
assert_eq "### entries under ## Features" "### 1. Web entry" "$(printf '%s\n' "$nested" | entry_section_in 1 | head -1)"
assert_eq "### section stops at the next ###" "- one" "$(printf '%s\n' "$nested" | entry_section_in 1 | grep -v '^$' | tail -1)"
assert_eq "### section stops at a ##" "**Goal.** Two." "$(printf '%s\n' "$nested" | entry_section_in 2 | grep -v '^$' | tail -1)"

# title / struck / goal / plan
s1=$(printf '%s\n' "$backlog" | entry_section_in 1)
s2=$(printf '%s\n' "$backlog" | entry_section_in 2)
assert_eq "title strips number" "First entry" "$(printf '%s\n' "$s1" | entry_title_in)"
assert_eq "title strips ~~" "Second entry" "$(printf '%s\n' "$s2" | entry_title_in)"
assert_eq "unstruck title" "no" "$(printf '%s\n' "$s1" | entry_title_struck_in && echo yes || echo no)"
assert_eq "struck title" "yes" "$(printf '%s\n' "$s2" | entry_title_struck_in && echo yes || echo no)"
assert_eq "goal paragraph" $'**Goal.** Do the first thing,\nacross two lines.' "$(printf '%s\n' "$s1" | entry_goal_in)"
assert_eq "plan block starts at **Plan.**" "**Plan.**" "$(printf '%s\n' "$s1" | entry_plan_in | head -1)"
assert_eq "open items exclude struck and nested" $'- open item\n- another open item' "$(printf '%s\n' "$s1" | plan_open_items_in)"
assert_eq "no open items when all struck" "" "$(printf '%s\n' "$s2" | plan_open_items_in)"
assert_eq "no plan block yields no plan" "" "$(printf '## 3. X\n\n**Goal.** G.\n' | entry_plan_in)"

# numbers / strikeouts
assert_eq "entry numbers in order" "1,2,11" "$(printf '%s\n' "$backlog" | entry_numbers_in | paste -sd, -)"
assert_eq "entry numbers ignore ideas" "1,2" "$(printf '%s\n' "$nested" | entry_numbers_in | paste -sd, -)"
assert_eq "struck lines, fence ignored" "4" "$(printf '%s\n' "$backlog" | struck_lines_in | wc -l | tr -d ' ')"
assert_eq "struck lines ignore ~~~ fences" "0" "$(printf '~~~\ncode\n~~~\n' | struck_lines_in | wc -l | tr -d ' ')"

# changelog readers
cl=$(cat <<'MD'
# Changelog

## [Unreleased]

### Added

- **One.** Text.

## [0.1.0] - 2026-09-06

- old
MD
)
assert_eq "unreleased items" $'### Added\n- **One.** Text.' "$(printf '%s\n' "$cl" | unreleased_items_in)"
assert_eq "newest heading" "## [0.1.0] - 2026-09-06" "$(printf '%s\n' "$cl" | newest_heading_in)"
assert_eq "empty unreleased" "" "$(printf '## [Unreleased]\n\n## [0.1.0]\n- x\n' | unreleased_items_in)"

# frontmatter
fm=$(printf -- '---\ntitle: "Quoted title"\nstatus: done\nchangelog: Unreleased\n---\n\nstatus: body\n')
assert_eq "frontmatter status" "done" "$(printf '%s\n' "$fm" | frontmatter_value_in status)"
assert_eq "frontmatter unquotes" "Quoted title" "$(printf '%s\n' "$fm" | frontmatter_value_in title)"
assert_eq "frontmatter missing key" "" "$(printf '%s\n' "$fm" | frontmatter_value_in branch)"
assert_eq "no frontmatter block" "" "$(printf 'status: done\n' | frontmatter_value_in status)"

# commit subjects
subjects=$(printf '%s\n' 'feat(go): add a thing' 'fix(web): repair a thing' 'chore: tidy' 'docs(go): plan the audit entry' 'chore(release): go v0.4.0, web v0.4.0' 'feat(go)!: drop the flag' 'Feat(go): capitalised type' 'feat(api): unknown scope' 'feat(go): Trailing period.' 'update stuff' 'feat: ')
assert_eq "unconventional subjects" $'Feat(go): capitalised type\nfeat(api): unknown scope\nfeat(go): Trailing period.\nupdate stuff\nfeat: ' "$(printf '%s\n' "$subjects" | unconventional_subjects_in)"
assert_eq "all conventional" "" "$(printf 'feat(web): x\nrevert: y\n' | unconventional_subjects_in)"

echo ""
if [[ "$failed" -gt 0 ]]; then
  echo "❌ scripts/lib/changelog.sh: $failed of $((passed + failed)) assertions failed" >&2
  exit 1
fi
echo "✅ scripts/lib/changelog.sh: $passed assertions passed"

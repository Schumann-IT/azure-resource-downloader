'use strict';
// Readers shared by the readiness reports (branch-ready, release-ready) and the
// start gate (start-item), so all of them agree on what "closed", "[Unreleased]
// is empty", "struck out" and "entry N" mean. Read-only by design: no file edits
// and no git commands in here.
//
// Every reader works on a string, which is what lets a caller feed it the
// content of `git show HEAD:./FILE` or `git show <base>:./FILE`; the path-based
// `read*` functions are wrappers over the working copy.

const fs = require('fs');

const HEADING = /^(#+) /;
const ENTRY_HEADING = /^#{2,3} ~*(\d+)\./;
const FENCE = /^(```|~~~)/;

// Parses a changelog into the facts a report needs. `awaitingRelease` is the
// pivot both scripts turn on: closing the changelog writes a bare `## [X.Y.Z]`
// and the root release script stamps the date when it publishes, so an undated
// newest heading means "closed, awaiting release" while a dated one is history.
function parseChangelog(text) {
  const lines = text.split('\n');
  const unreleasedIndex = lines.findIndex((line) => line === '## [Unreleased]');
  let unreleasedItems = [];
  if (unreleasedIndex !== -1) {
    let nextHeading = lines.findIndex((line, i) => i > unreleasedIndex && line.startsWith('## '));
    if (nextHeading === -1) nextHeading = lines.length;
    unreleasedItems = lines.slice(unreleasedIndex + 1, nextHeading).filter((line) => line.trim() !== '');
  }
  const heading = lines.find((line) => /^## \[\d+\.\d+\.\d+\]/.test(line)) ?? '';
  const versionMatch = /^## \[(\d+\.\d+\.\d+)\]/.exec(heading);
  const version = versionMatch ? versionMatch[1] : '';
  return {
    unreleasedIndex,
    unreleasedItems,
    heading,
    version,
    awaitingRelease: version !== '' && heading === `## [${version}]`,
  };
}

function readChangelog(changelogPath) {
  return parseChangelog(fs.readFileSync(changelogPath, 'utf8'));
}

// Struck-out lines: work that shipped and is still waiting to be archived. The
// boundary checks keep a `~~~` fence or rule from being read as a strikeout.
function struckLines(text) {
  return text
    .split('\n')
    .map((line, i) => ({ line, n: i + 1 }))
    .filter(({ line }) => /(^|[^~])~~[^~]/.test(line));
}

function readStruckLines(nextPath) {
  return struckLines(fs.readFileSync(nextPath, 'utf8'));
}

// The `## N. Title` / `### N. Title` numbers in file order. Numbering is
// presentational and is made contiguous again whenever entries are archived, so
// a gap or a repeat means that renumbering was missed. Matches struck titles too.
function entryNumbers(text) {
  return text
    .split('\n')
    .map((line) => ENTRY_HEADING.exec(line))
    .filter((match) => match !== null)
    .map((match) => Number(match[1]));
}

function readEntryNumbers(nextPath) {
  return entryNumbers(fs.readFileSync(nextPath, 'utf8'));
}

// Entry N: its `## N.` / `### N.` heading line (struck or not) up to, not
// including, the next heading of the same or a higher level. Fenced code is
// never scanned for headings. `[]` when N is absent; `n = 1` cannot match
// `## 11.` because the number must be followed by `. `.
function entrySection(text, n) {
  const start = new RegExp(`^#{2,3} ~*${n}\\. `);
  const out = [];
  let fence = false;
  let level = 0;
  for (const line of text.split('\n')) {
    if (FENCE.test(line)) fence = !fence;
    const lvl = fence ? 0 : headingLevel(line);
    if (level > 0 && lvl > 0 && lvl <= level) break;
    if (level === 0 && (lvl === 2 || lvl === 3) && start.test(line)) level = lvl;
    if (level > 0) out.push(line);
  }
  return out;
}

// `#` count of a heading line, 0 for any other line.
function headingLevel(line) {
  const heading = HEADING.exec(line);
  return heading ? heading[1].length : 0;
}

// The title of a section (its first line) with the heading marks, the number and
// any `~~` removed.
function entryTitle(section) {
  return (section[0] ?? '').replaceAll('~~', '').replace(/^#+ \d+\. */, '');
}

// Is the section's title struck (`## ~~N. …~~` or `## N. ~~…~~`)?
function entryTitleStruck(section) {
  return (section[0] ?? '').includes('~~');
}

// The Goal paragraph: from the `**Goal.**` line to the first blank line.
function entryGoal(section) {
  const out = [];
  let inGoal = false;
  for (const line of section) {
    if (line.startsWith('**Goal.**')) inGoal = true;
    if (inGoal && line.trim() === '') break;
    if (inGoal) out.push(line);
  }
  return out;
}

// The Plan block: from the `**Plan.**` line to the end of the section.
function entryPlan(section) {
  const i = section.findIndex((line) => line.startsWith('**Plan.**'));
  return i === -1 ? [] : section.slice(i);
}

// Top-level plan bullets (`- …`) that are not struck (`- ~~…`). Nested bullets
// are continuation of their parent and are not counted.
function planOpenItems(section) {
  return entryPlan(section).filter((line) => line.startsWith('- ') && !line.startsWith('- ~~'));
}

// Conventional Commits 1.0.0 as this repository applies it: `type(scope)!:
// description` with a fixed type set, scope `go`, `web` or `release` (or none),
// a lowercase description and no trailing period.
const CONVENTIONAL_SUBJECT = /^(feat|fix|docs|refactor|test|build|ci|chore|revert)(\((go|web|release)\))?!?: [a-z0-9](.*[^. ])?$/;

// The commit subjects that do not conform; `[]` when all do.
function unconventionalSubjects(subjects) {
  return subjects.filter((subject) => !CONVENTIONAL_SUBJECT.test(subject));
}

// `key: value` pairs of a leading `---` frontmatter block; `{}` when there is no
// such block or it is unterminated. Surrounding double quotes are removed.
function parseFrontmatter(text) {
  const lines = text.split('\n');
  if (lines[0] !== '---') return {};
  const end = lines.findIndex((line, i) => i > 0 && line === '---');
  if (end === -1) return {};
  const out = {};
  for (const line of lines.slice(1, end)) {
    const colon = line.indexOf(':');
    if (colon <= 0) continue;
    const key = line.slice(0, colon);
    if (!/^[A-Za-z][\w-]*$/.test(key)) continue;
    out[key] = line.slice(colon + 1).trim().replace(/^"(.*)"$/, '$1');
  }
  return out;
}

module.exports = {
  parseChangelog,
  readChangelog,
  struckLines,
  readStruckLines,
  entryNumbers,
  readEntryNumbers,
  entrySection,
  entryTitle,
  entryTitleStruck,
  entryGoal,
  entryPlan,
  planOpenItems,
  parseFrontmatter,
  unconventionalSubjects,
};

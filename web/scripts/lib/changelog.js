'use strict';
// Readers shared by the two readiness reports (branch-ready, release-ready) so
// both agree on what "closed", "[Unreleased] is empty" and "struck out" mean.
// Read-only by design: no file edits and no git commands in either report.

const fs = require('fs');

// Parses CHANGELOG.md into the facts a report needs. `awaitingRelease` is the
// pivot both scripts turn on: closing the changelog writes a bare `## [X.Y.Z]`
// and the root release script stamps the date when it publishes, so an undated
// newest heading means "closed, awaiting release" while a dated one is history.
function readChangelog(changelogPath) {
  const lines = fs.readFileSync(changelogPath, 'utf8').split('\n');
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

// Struck-out lines in NEXT-ITERATIONS.md: work that shipped and is still waiting
// to be cleared out. The boundary checks keep a `~~~` fence or rule from being
// read as a strikeout.
function readStruckLines(nextPath) {
  return fs
    .readFileSync(nextPath, 'utf8')
    .split('\n')
    .map((line, i) => ({ line, n: i + 1 }))
    .filter(({ line }) => /(^|[^~])~~[^~]/.test(line));
}

// The `### N. Title` numbers in file order. Numbering is presentational and is
// made contiguous again whenever entries are cleared out, so a gap or a repeat
// means that renumbering was missed. Matches struck titles too (`### ~~5. …`).
function readEntryNumbers(nextPath) {
  return fs
    .readFileSync(nextPath, 'utf8')
    .split('\n')
    .map((line) => /^#{2,3} ~*(\d+)\./.exec(line))
    .filter((match) => match !== null)
    .map((match) => Number(match[1]));
}

module.exports = { readChangelog, readStruckLines, readEntryNumbers };

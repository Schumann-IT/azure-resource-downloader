#!/usr/bin/env node
// Branch-readiness report for web/: is this feature or fix branch ready to ship?
// It changes nothing — no file edits, no commits, no tags — and runs no git
// command, exactly like the release report it complements.
//
// It asks the opposite questions to `release-ready`. A finished branch has
// recorded its work under `## [Unreleased]`, has cleared the entries it
// delivered out of NEXT-ITERATIONS.md (they are struck through while the work is
// in progress) and has left `version` alone: bumping it and closing the
// changelog are the release step, which `release-ready` reports on.
//
// Unlike that report, this one is a gate: it exits non-zero if a single check
// fails.
//
// Usage: npm run branch-ready
'use strict';

const fs = require('fs');
const path = require('path');
const { readChangelog, readStruckLines, readEntryNumbers } = require('./lib/changelog');

const root = path.resolve(__dirname, '..');
const changelogPath = path.join(root, 'CHANGELOG.md');
const nextPath = path.join(root, 'NEXT-ITERATIONS.md');
let passed = 0;
let failed = 0;

function ok(message) {
  console.log(`✅ ${message}`);
  passed += 1;
}

function fail(message, details) {
  console.error(`❌ ${message}`);
  if (details) console.error(details);
  failed += 1;
}

function info(message) {
  console.log(`ℹ️  ${message}`);
}

if (!fs.existsSync(changelogPath)) {
  console.error('❌ CHANGELOG.md not found');
  process.exit(1);
}
const { unreleasedIndex, unreleasedItems, heading, version, awaitingRelease } =
  readChangelog(changelogPath);

// 1. Nothing struck out in NEXT-ITERATIONS.md. A strikeout marks work that
//    shipped and is waiting to be cleared out; clearing it is part of closing
//    the branch, so the entry must be gone before the branch is merged.
if (!fs.existsSync(nextPath)) {
  fail('NEXT-ITERATIONS.md not found');
} else {
  const struck = readStruckLines(nextPath);
  if (struck.length > 0) {
    fail(
      'NEXT-ITERATIONS.md still has struck-out entries — delete them now that the work is done (their CHANGELOG.md entry is the record):',
      struck.map(({ line, n }) => `${n}:${line}`).join('\n'),
    );
  } else {
    ok('NEXT-ITERATIONS.md has no struck-out entries left');
  }

  // 2. Numbering is contiguous. Clearing entries out shifts the numbers, so a
  //    gap or a repeat means the renumbering was forgotten.
  const numbers = readEntryNumbers(nextPath);
  const expected = numbers.map((_, i) => i + 1);
  if (numbers.join(',') !== expected.join(',')) {
    fail(
      `NEXT-ITERATIONS.md entries are numbered ${numbers.join(', ')} — renumber them 1..${numbers.length} after clearing entries out`,
    );
  } else {
    ok(`NEXT-ITERATIONS.md entries are numbered 1..${numbers.length}`);
  }
}

// 3. The branch recorded its work under an open `## [Unreleased]`. An empty one
//    is legitimate for a branch with no user- or operator-visible effect, so it
//    is reported rather than failed; a closed changelog means this is a release
//    branch and `release-ready` is the report to run instead.
if (unreleasedIndex === -1) {
  fail("CHANGELOG.md has no '## [Unreleased]' section — add one above the newest version");
} else if (awaitingRelease) {
  info(`CHANGELOG.md is closed for release (${heading}) — run 'npm run release-ready' for that flow`);
} else if (unreleasedItems.length === 0) {
  info('CHANGELOG.md has an empty [Unreleased] section — fine only if this branch changed nothing a user or operator can notice');
} else {
  ok(`CHANGELOG.md records ${unreleasedItems.length} line(s) under [Unreleased]`);
}

// 4. The version was left alone: bumping `package.json` and closing the
//    changelog belong to the release, not to a feature branch. Skipped once the
//    changelog is closed, because then the version is *expected* to have moved
//    and checking it is release-ready's job.
if (!awaitingRelease) {
  const pkgVersion = JSON.parse(fs.readFileSync(path.join(root, 'package.json'), 'utf8')).version;
  if (version === '') {
    info('CHANGELOG.md has no version section yet — nothing to compare package.json against');
  } else if (pkgVersion !== version) {
    fail(
      `package.json says ${pkgVersion} but the newest released version is ${version} — bumping the version is part of cutting the release, not of a branch`,
    );
  } else {
    ok(`package.json version ${pkgVersion} matches the newest released version`);
  }
}

console.log('');
if (failed > 0) {
  console.error(`❌ web/: ${failed} of ${passed + failed} checks failed — resolve the ❌ items above before closing the branch`);
  process.exit(1);
}
console.log(`✅ web/: branch is ready to ship (${passed} checks passed)`);

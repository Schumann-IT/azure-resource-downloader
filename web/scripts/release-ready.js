#!/usr/bin/env node
// Release-readiness report for web/. It changes nothing — no file edits, no
// commits, no tags — and runs no git command at all. Closing the changelog
// (`## [Unreleased]` emptied into a new, undated `## [X.Y.Z]` section) and
// bumping `version` in package.json are done by hand; this script only reports
// whether that state has been reached. The release date is stamped onto that
// heading by the root release script. With no undated heading there is nothing
// to release and the script reports success ("no release needed"). Otherwise
// every check is run and reported, and the script exits non-zero only when ALL
// checks fail. Branch and working-tree checks, tagging and the GitHub release
// live at the repository root (`make release`). See ../../README.md#releasing.
//
// Usage: npm run release-ready
'use strict';

const fs = require('fs');
const path = require('path');

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

// Is a release pending? Closing [Unreleased] writes a bare `## [X.Y.Z]`; the root
// release script stamps the date when it publishes. So an undated newest heading
// means "closed, awaiting release"; a dated or missing one means nothing to do.
if (!fs.existsSync(changelogPath)) {
  console.error('❌ CHANGELOG.md not found');
  process.exit(1);
}
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
if (version === '' || heading !== `## [${version}]`) {
  if (version === '') {
    console.log('ℹ️  CHANGELOG.md has no version section yet');
  } else {
    console.log(`ℹ️  newest changelog version ${version} is already released (${heading})`);
  }
  if (unreleasedItems.length > 0) {
    console.log(`ℹ️  [Unreleased] has ${unreleasedItems.length} line(s) waiting — close it as a new '## [X.Y.Z]' section when you want to release`);
  }
  console.log('');
  console.log('✅ web/: no release needed');
  process.exit(0);
}
console.log(`🚀 newest changelog version ${version} is closed and awaiting release (web/v${version})`);

// 1. No struck-out entries in NEXT-ITERATIONS.md. A strikeout marks work that
//    shipped but has not been moved into CHANGELOG.md yet.
if (!fs.existsSync(nextPath)) {
  fail('NEXT-ITERATIONS.md not found');
} else {
  const struck = fs
    .readFileSync(nextPath, 'utf8')
    .split('\n')
    .map((line, i) => ({ line, n: i + 1 }))
    .filter(({ line }) => /(^|[^~])~~[^~]/.test(line));
  if (struck.length > 0) {
    fail(
      'NEXT-ITERATIONS.md has struck-out entries — move them into CHANGELOG.md and delete them:',
      struck.map(({ line, n }) => `${n}:${line}`).join('\n'),
    );
  } else {
    ok('NEXT-ITERATIONS.md has no struck-out entries');
  }
}

// 2. CHANGELOG.md has an empty `## [Unreleased]` section: everything pending has
//    already been moved into the version section that is about to be released.
if (unreleasedIndex === -1) {
  fail("CHANGELOG.md has no '## [Unreleased]' section");
} else if (unreleasedItems.length > 0) {
  fail(
    `CHANGELOG.md still has items under [Unreleased] — move them into the [${version}] section first:`,
    unreleasedItems.slice(0, 5).join('\n'),
  );
} else {
  ok('CHANGELOG.md has an empty [Unreleased] section');
}

// 3. package.json carries the version that is about to be released.
const pkgVersion = JSON.parse(fs.readFileSync(path.join(root, 'package.json'), 'utf8')).version;
if (pkgVersion !== version) {
  fail(`package.json says ${pkgVersion} but CHANGELOG.md says ${version} — run 'npm version ${version} --no-git-tag-version' and commit`);
} else {
  ok(`package.json version ${pkgVersion} matches CHANGELOG.md`);
}

console.log('');
if (passed === 0) {
  console.error(`❌ web/ is not ready to release: all ${failed} checks failed`);
  process.exit(1);
}
if (failed > 0) {
  console.log(`⚠️  web/: ${passed} of ${passed + failed} checks passed — resolve the ❌ items above before running 'make release' at the repository root`);
} else {
  console.log(`✅ web/ is ready to release v${version} — run 'make release' from the repository root to tag and publish web/v${version}`);
}

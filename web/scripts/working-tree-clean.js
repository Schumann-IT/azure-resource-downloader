#!/usr/bin/env node
// Preflight for `npm run branch-ready`: refuse to report on a dirty tree, so the
// verdict describes the commit that will be merged and not whatever happens to
// be open in the editor. It runs before the tests and the build, so a dirty tree
// costs nothing to discover.
//
// It is read-only (`git status --porcelain`) and scoped to web/, so an unrelated
// edit in go/ cannot block this project's gate. The branch report and the start
// gate run further read-only git (`rev-parse`, `merge-base`, `diff`, `show`),
// all through lib/git.js; `release-ready` still runs no git at all, and the
// repository-wide branch and working-tree checks still live only in the root
// release script.
//
// A missing git or a checkout that is not a repository is reported and waved
// through rather than failed: the gate must stay usable outside a clone.
//
// Usage: npm run branch-ready (runs this first)
'use strict';

const path = require('path');
const { isRepo, dirtyFiles } = require('./lib/git');

const root = path.resolve(__dirname, '..');
const MAX_LISTED = 20;

if (!isRepo(root)) {
  console.log('ℹ️  git not found or not a repository — skipping the clean-tree check');
  process.exit(0);
}

const dirty = dirtyFiles(root);
if (dirty.length === 0) {
  console.log('✅ web/ has no uncommitted changes');
  process.exit(0);
}

console.error('❌ web/ has uncommitted changes — commit or stash them, then run this again:');
console.error(dirty.slice(0, MAX_LISTED).join('\n'));
if (dirty.length > MAX_LISTED) {
  console.error(`… and ${dirty.length - MAX_LISTED} more`);
}
process.exit(1);

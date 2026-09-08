#!/usr/bin/env node
// Preflight for `npm run branch-ready`: refuse to report on a dirty tree, so the
// verdict describes the commit that will be merged and not whatever happens to
// be open in the editor. It runs before the tests and the build, so a dirty tree
// costs nothing to discover.
//
// This is the one git command in web/'s tooling, it is read-only
// (`git status --porcelain`), and it is scoped to web/ so an unrelated edit in
// go/ cannot block this project's gate. `release-ready` still runs no git at
// all, and the repository-wide branch and working-tree checks still live only in
// the root release script.
//
// A missing git or a checkout that is not a repository is reported and waved
// through rather than failed: the gate must stay usable outside a clone.
//
// Usage: npm run branch-ready (runs this first)
'use strict';

const { execFileSync } = require('child_process');
const path = require('path');

const root = path.resolve(__dirname, '..');
const MAX_LISTED = 20;

let status;
try {
  status = execFileSync('git', ['status', '--porcelain', '--', root], {
    cwd: root,
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'pipe'],
  });
} catch (error) {
  const reason = error.code === 'ENOENT' ? 'git not found' : 'not a git repository';
  console.log(`ℹ️  ${reason} — skipping the clean-tree check`);
  process.exit(0);
}

const dirty = status.split('\n').filter((line) => line.trim() !== '');
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

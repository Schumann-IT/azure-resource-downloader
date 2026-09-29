#!/usr/bin/env node
// Start gate for web/: may implementation of entry N begin? It refuses on the
// release branch, on a dirty web/ tree, when entry N is not in HEAD's
// NEXT-ITERATIONS.md (the backlog must be committed before it is implemented),
// and when the entry has no outstanding plan item. On success it prints the
// entry's title, Goal and Plan. It changes nothing; the only git it runs is
// read-only (`rev-parse`, `status`, `show`). Outside a clone it reads the
// working copy and says so, so the gate stays usable there.
//
// Exit codes: 0 ready, 1 a check failed, 2 usage.
// Usage: npm run start-item -- N
'use strict';

const fs = require('fs');
const path = require('path');
const { isRepo, currentBranch, dirtyFiles, showFile } = require('./lib/git');
const {
  entrySection,
  entryTitle,
  entryTitleStruck,
  entryGoal,
  entryPlan,
  planOpenItems,
  entryNumbers,
} = require('./lib/changelog');

const NEXT = 'NEXT-ITERATIONS.md';
const MAX_LISTED = 20;

// Checks 1–3: the committed backlog, or null after reporting why it cannot be
// read (release branch, detached HEAD, dirty tree, uncommitted file). Outside a
// clone the working copy is read and the skip is announced.
function committedBacklog({ root, releaseBranch, out, err }) {
  if (!isRepo(root)) {
    out(`ℹ️  git not found or not a repository — reading the working copy of ${NEXT}; branch and clean-tree checks skipped`);
    return fs.readFileSync(path.join(root, NEXT), 'utf8');
  }
  // 1. Not on the release branch, and not detached.
  const branch = currentBranch(root);
  if (branch === 'HEAD') {
    err('❌ detached HEAD — check out a branch first');
    return null;
  }
  if (branch === releaseBranch) {
    err(`❌ on branch '${branch}' — implementation happens on a branch off ${releaseBranch}: git switch -c <type>/<slug>`);
    return null;
  }
  out(`✅ on branch '${branch}' (not ${releaseBranch})`);

  // 2. web/ has no uncommitted changes.
  const dirty = dirtyFiles(root);
  if (dirty.length > 0) {
    err('❌ web/ has uncommitted changes — commit or stash them, then run this again:');
    err(dirty.slice(0, MAX_LISTED).join('\n'));
    if (dirty.length > MAX_LISTED) err(`… and ${dirty.length - MAX_LISTED} more`);
    return null;
  }
  out('✅ web/ has no uncommitted changes');

  // 3. The backlog is committed: read entry N from HEAD, never from the editor.
  const content = showFile(root, 'HEAD', NEXT);
  if (content === null) err(`❌ ${NEXT} is not committed — commit the backlog before implementing`);
  return content;
}

// Checks 4–5 on one entry: `{ error }` naming the refusal, or the parts to print.
function entryVerdict(content, n) {
  const section = entrySection(content, Number(n));
  if (section.length === 0) {
    const numbers = entryNumbers(content);
    const listed = numbers.length > 0 ? numbers.join(', ') : 'none';
    return { error: `entry ${n} is not in HEAD's ${NEXT} — committed entries: ${listed} (edit and commit the backlog first)` };
  }
  const plan = entryPlan(section);
  if (plan.length === 0) return { error: `entry ${n} has no **Plan.** block — add concrete work items before starting` };
  const open = planOpenItems(section);
  if (entryTitleStruck(section)) {
    if (open.length > 0) return { error: `entry ${n}'s title is struck but plan items are open — unstrike the title or strike the items` };
    return { error: `entry ${n} is fully delivered — say "item ${n} is done" to archive it` };
  }
  if (open.length === 0) return { error: `entry ${n} has no outstanding plan items — say "item ${n} is done" to archive it` };
  return { title: entryTitle(section), goal: entryGoal(section), plan, open };
}

// Exported for the spec; `root` is the project folder and `n` the raw argument.
function startItem({ root, n, releaseBranch = 'main', out = console.log, err = console.error }) {
  if (!/^[1-9]\d*$/.test(String(n ?? ''))) {
    err('usage: start-item N (a positive entry number) — e.g. npm run start-item -- 1');
    return 2;
  }
  const content = committedBacklog({ root, releaseBranch, out, err });
  if (content === null) return 1;
  const verdict = entryVerdict(content, n);
  if (verdict.error) {
    err(`❌ ${verdict.error}`);
    return 1;
  }
  out('');
  out(`## ${n}. ${verdict.title}`);
  out('');
  out(verdict.goal.join('\n'));
  out('');
  out(verdict.plan.join('\n'));
  out('');
  out(`✅ web/: item ${n} is ready to start — ${verdict.title} (${verdict.open.length} open plan item(s))`);
  return 0;
}

module.exports = { startItem };

if (require.main === module) {
  process.exit(
    startItem({
      root: path.resolve(__dirname, '..'),
      n: process.argv[2],
      releaseBranch: process.env.RELEASE_BRANCH ?? 'main',
    }),
  );
}

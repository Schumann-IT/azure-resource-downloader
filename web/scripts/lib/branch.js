'use strict';
// The git facts behind the branch checks of `npm run branch-ready`, gathered in
// one place so the report only turns them into lines and the spec can assert
// them against a fixture repository. Read-only git only.
//
// `backlogChanged` is the net diff of NEXT-ITERATIONS.md against the merge-base;
// `backlogTouched` also counts an entry added and archived on the same branch
// (net-zero backlog diff, but a new archive file), which delivers the backlog too.
// `dependencyOnly` is true when the branch changed something under web/ and every
// changed path there is exactly package.json or package-lock.json (a Dependabot or
// manual dependency update); `backlogCheck` turns the facts into the check-6 line.

const { git, isRepo, currentBranch, mergeBase, showFile } = require('./git');
const { parseChangelog, parseFrontmatter } = require('./changelog');

// `project` names the archive folder (`.claude/archive/<project>` at the
// repository root); `root` is the project folder the diffs are scoped to.
function readBranchFacts({ root, releaseBranch = 'main', project }) {
  if (!isRepo(root)) return { available: false, reason: 'git not found or not a repository' };
  const facts = {
    available: true,
    branch: currentBranch(root),
    base: mergeBase(root, releaseBranch),
    folderChanged: null,
    backlogChanged: null,
    backlogTouched: null,
    dependencyOnly: null,
    archived: [],
    baseUnreleasedCount: null,
    subjects: [],
  };
  if (facts.base === null) return facts;
  const changed = (pathspec) => git(['diff', '--quiet', facts.base, 'HEAD', '--', pathspec], { cwd: root }).status !== 0;
  facts.folderChanged = changed('.');
  facts.backlogChanged = changed('NEXT-ITERATIONS.md');
  // Only files ADDED on the branch count as "done" events; `:(top)` makes the
  // pathspec repository-relative whatever the folder depth.
  const added = git(['diff', '--name-only', '--diff-filter=A', facts.base, 'HEAD', '--', `:(top).claude/archive/${project}`], {
    cwd: root,
  });
  facts.archived = added.stdout
    .split('\n')
    .filter((line) => line.endsWith('.md'))
    .map((file) => ({
      path: file,
      status: parseFrontmatter(git(['show', `HEAD:${file}`], { cwd: root }).stdout).status ?? '',
    }));
  facts.backlogTouched = facts.backlogChanged || facts.archived.length > 0;
  // --relative prints paths relative to web/ and drops paths outside it, so only
  // a root package.json / package-lock.json qualifies (a nested one does not).
  const changedPaths = git(['diff', '--name-only', '--relative', facts.base, 'HEAD', '--', '.'], { cwd: root })
    .stdout.split('\n')
    .filter((line) => line !== '');
  facts.dependencyOnly = changedPaths.length > 0 && changedPaths.every((p) => p === 'package.json' || p === 'package-lock.json');
  facts.subjects = git(['log', '--no-merges', '--format=%s', `${facts.base}..HEAD`], { cwd: root })
    .stdout.split('\n')
    .filter((line) => line !== '');
  const baseChangelog = showFile(root, facts.base, 'CHANGELOG.md');
  facts.baseUnreleasedCount = baseChangelog === null ? 0 : parseChangelog(baseChangelog).unreleasedItems.length;
  return facts;
}

// The backlog check (check 6) as data: a changed backlog, an archived entry and a
// dependency-only branch pass, in that order; anything else fails.
function backlogCheck(facts) {
  if (facts.backlogChanged) return { ok: true, message: 'NEXT-ITERATIONS.md changed on this branch' };
  if (facts.backlogTouched) {
    return { ok: true, message: `NEXT-ITERATIONS.md delivered on this branch (${facts.archived.length} archived entry(ies))` };
  }
  if (facts.dependencyOnly) return { ok: true, message: 'dependency-only branch: backlog check not required' };
  return {
    ok: false,
    message:
      'NEXT-ITERATIONS.md is unchanged on this branch — every branch that changes web/ delivers, refines or adds an entry (a small fix still gets a small entry)',
  };
}

module.exports = { readBranchFacts, backlogCheck };

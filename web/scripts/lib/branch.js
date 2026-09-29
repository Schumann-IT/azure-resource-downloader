'use strict';
// The git facts behind the branch checks of `npm run branch-ready`, gathered in
// one place so the report only turns them into lines and the spec can assert
// them against a fixture repository. Read-only git only.

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
  facts.subjects = git(['log', '--no-merges', '--format=%s', `${facts.base}..HEAD`], { cwd: root })
    .stdout.split('\n')
    .filter((line) => line !== '');
  const baseChangelog = showFile(root, facts.base, 'CHANGELOG.md');
  facts.baseUnreleasedCount = baseChangelog === null ? 0 : parseChangelog(baseChangelog).unreleasedItems.length;
  return facts;
}

module.exports = { readBranchFacts };

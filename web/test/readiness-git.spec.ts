import { promises as fsp } from 'fs';
import * as os from 'os';
import * as path from 'path';

// The start gate and the branch facts behind `npm run branch-ready` need a
// repository to look at, so they are exercised against a throwaway one built
// with `git init` in a temp directory — never against this checkout. The whole
// suite is skipped when git is not available, so the tests stay runnable
// without it, exactly as the gates themselves degrade.
/* eslint-disable @typescript-eslint/no-require-imports */
const { git, isRepo } = require('../scripts/lib/git');
const { startItem } = require('../scripts/start-item');
const { readBranchFacts, backlogCheck } = require('../scripts/lib/branch');
/* eslint-enable @typescript-eslint/no-require-imports */

const BACKLOG = ['# Next iterations', '', '## 1. First entry', '', '**Goal.** G.', '', '**Plan.**', '', '- open item', '', '## 2. ~~Done entry~~', '', '**Goal.** D.', '', '**Plan.**', '', '- ~~done~~', ''].join(
  '\n',
);
const CHANGELOG = ['# Changelog', '', '## [Unreleased]', '', '## [0.1.0] - 2026-09-01', '', '- old', ''].join('\n');

function hasGit(): boolean {
  try {
    return git(['--version'], { cwd: os.tmpdir() }).status === 0;
  } catch {
    return false;
  }
}

const describeWithGit = hasGit() ? describe : describe.skip;

describeWithGit('start gate and branch facts against a fixture repository', () => {
  let repo: string;
  let root: string;
  const out: string[] = [];
  const err: string[] = [];
  const io = { out: (line: string) => out.push(line), err: (line: string) => err.push(line) };

  function run(args: string[]): void {
    const r = git(args, { cwd: repo });
    if (r.status !== 0) throw new Error(`git ${args.join(' ')} failed: ${r.stderr}`);
  }

  beforeAll(async () => {
    repo = await fsp.mkdtemp(path.join(os.tmpdir(), 'readiness-git-'));
    root = path.join(repo, 'web');
    await fsp.mkdir(root);
    run(['init', '-q', '-b', 'main']);
    run(['config', 'user.email', 'test@example.invalid']);
    run(['config', 'user.name', 'Readiness Spec']);
    run(['config', 'commit.gpgsign', 'false']);
    await fsp.writeFile(path.join(root, 'NEXT-ITERATIONS.md'), BACKLOG);
    await fsp.writeFile(path.join(root, 'CHANGELOG.md'), CHANGELOG);
    run(['add', '.']);
    run(['commit', '-q', '-m', 'plan']);
  });

  afterAll(async () => {
    await fsp.rm(repo, { recursive: true, force: true });
  });

  beforeEach(() => {
    out.length = 0;
    err.length = 0;
  });

  it('is a repository', () => {
    expect(isRepo(root)).toBe(true);
  });

  it('refuses on the release branch and on a bad argument', () => {
    expect(startItem({ root, n: '1', releaseBranch: 'main', ...io })).toBe(1);
    expect(err[0]).toContain("on branch 'main'");
    expect(startItem({ root, n: 'x', releaseBranch: 'main', ...io })).toBe(2);
  });

  it('starts an open entry on a branch, refuses a done, an unknown and an uncommitted one', async () => {
    run(['switch', '-q', '-c', 'feat/one']);
    expect(startItem({ root, n: '1', releaseBranch: 'main', ...io })).toBe(0);
    expect(out.join('\n')).toContain('## 1. First entry');
    expect(out.join('\n')).toContain('- open item');
    expect(out.at(-1)).toContain('item 1 is ready to start');

    expect(startItem({ root, n: '2', releaseBranch: 'main', ...io })).toBe(1);
    expect(err.at(-1)).toContain('item 2 is done');
    expect(startItem({ root, n: '3', releaseBranch: 'main', ...io })).toBe(1);
    expect(err.at(-1)).toContain('committed entries: 1, 2');

    // An entry that exists only in the working copy is not committed, and the
    // dirty tree is what the gate reports first.
    await fsp.appendFile(path.join(root, 'NEXT-ITERATIONS.md'), '\n## 3. New\n\n**Goal.** N.\n\n**Plan.**\n\n- x\n');
    err.length = 0;
    expect(startItem({ root, n: '3', releaseBranch: 'main', ...io })).toBe(1);
    expect(err[0]).toContain('uncommitted changes');
    run(['checkout', '-q', '--', 'web/NEXT-ITERATIONS.md']);
  });

  it('reports the branch facts before and after archiving an entry', async () => {
    let facts = readBranchFacts({ root, releaseBranch: 'main', project: 'web' });
    expect(facts.available).toBe(true);
    expect(facts.branch).toBe('feat/one');
    expect(facts.base).not.toBeNull();
    expect(facts.folderChanged).toBe(false);
    expect(facts.archived).toEqual([]);

    await fsp.mkdir(path.join(repo, '.claude', 'archive', 'web'), { recursive: true });
    await fsp.writeFile(path.join(repo, '.claude', 'archive', 'web', '2026-09-29-done-entry.md'), '---\ntitle: Done entry\nstatus: done\n---\n## Done entry\n');
    await fsp.writeFile(path.join(root, 'NEXT-ITERATIONS.md'), BACKLOG.replace(/## 2\. ~~Done entry~~[\s\S]*$/, ''));
    await fsp.writeFile(path.join(root, 'CHANGELOG.md'), CHANGELOG.replace('## [Unreleased]\n', '## [Unreleased]\n\n- **Done entry.** Shipped.\n'));
    run(['add', '.']);
    run(['commit', '-q', '-m', 'docs(web): close done entry']);

    facts = readBranchFacts({ root, releaseBranch: 'main', project: 'web' });
    expect(facts.folderChanged).toBe(true);
    expect(facts.backlogChanged).toBe(true);
    expect(facts.backlogTouched).toBe(true);
    expect(facts.archived).toEqual([{ path: '.claude/archive/web/2026-09-29-done-entry.md', status: 'done' }]);
    expect(facts.baseUnreleasedCount).toBe(0);
    expect(facts.subjects).toEqual(['docs(web): close done entry']);
  });

  it('reports no merge base when the release branch does not exist', () => {
    const facts = readBranchFacts({ root, releaseBranch: 'nope', project: 'web' });
    expect(facts.available).toBe(true);
    expect(facts.base).toBeNull();
    expect(facts.backlogChanged).toBeNull();
    expect(facts.backlogTouched).toBeNull();
    expect(facts.dependencyOnly).toBeNull();
  });

  it('counts only a net backlog change or a web archive file as touching the backlog', async () => {
    run(['switch', '-q', 'main']);

    run(['switch', '-q', '-c', 'fix/other-file']);
    await fsp.writeFile(path.join(root, 'other.txt'), 'x\n');
    run(['add', '.']);
    run(['commit', '-q', '-m', 'fix(web): other file']);
    let facts = readBranchFacts({ root, releaseBranch: 'main', project: 'web' });
    expect(facts.folderChanged).toBe(true);
    expect(facts.backlogChanged).toBe(false);
    expect(facts.backlogTouched).toBe(false);
    expect(facts.dependencyOnly).toBe(false);

    run(['switch', '-q', 'main']);
    run(['switch', '-q', '-c', 'fix/added-and-archived']);
    const backlogPath = path.join(root, 'NEXT-ITERATIONS.md');
    await fsp.appendFile(backlogPath, '\n## 3. Added\n\n**Goal.** A.\n\n**Plan.**\n\n- ~~x~~\n');
    run(['add', '.']);
    run(['commit', '-q', '-m', 'docs(web): plan added']);
    await fsp.writeFile(backlogPath, BACKLOG);
    await fsp.mkdir(path.join(repo, '.claude', 'archive', 'web'), { recursive: true });
    await fsp.writeFile(path.join(repo, '.claude', 'archive', 'web', 'added.md'), '---\ntitle: Added\nstatus: done\n---\n');
    await fsp.writeFile(path.join(root, 'other.txt'), 'y\n');
    run(['add', '.']);
    run(['commit', '-q', '-m', 'docs(web): close added']);
    facts = readBranchFacts({ root, releaseBranch: 'main', project: 'web' });
    expect(facts.backlogChanged).toBe(false);
    expect(facts.archived).toEqual([{ path: '.claude/archive/web/added.md', status: 'done' }]);
    expect(facts.backlogTouched).toBe(true);

    run(['switch', '-q', 'main']);
    run(['switch', '-q', '-c', 'fix/go-archive']);
    await fsp.mkdir(path.join(repo, '.claude', 'archive', 'go'), { recursive: true });
    await fsp.writeFile(path.join(repo, '.claude', 'archive', 'go', 'g.md'), '---\ntitle: G\nstatus: done\n---\n');
    await fsp.writeFile(path.join(root, 'other.txt'), 'z\n');
    run(['add', '.']);
    run(['commit', '-q', '-m', 'fix(web): go archive only']);
    facts = readBranchFacts({ root, releaseBranch: 'main', project: 'web' });
    expect(facts.backlogChanged).toBe(false);
    expect(facts.archived).toEqual([]);
    expect(facts.backlogTouched).toBe(false);
    expect(facts.dependencyOnly).toBe(false);
  });

  describe('dependency-only branches', () => {
    const DEP_OK = { ok: true, message: 'dependency-only branch: backlog check not required' };

    async function branchWith(name: string, files: Record<string, string>): Promise<void> {
      run(['switch', '-q', 'main']);
      run(['switch', '-q', '-c', name]);
      for (const [rel, content] of Object.entries(files)) {
        const abs = path.join(repo, rel);
        await fsp.mkdir(path.dirname(abs), { recursive: true });
        await fsp.writeFile(abs, content);
      }
      run(['add', '.']);
      run(['commit', '-q', '-m', 'build(web): bump']);
    }

    const facts = () => readBranchFacts({ root, releaseBranch: 'main', project: 'web' });

    it('accepts package.json and package-lock.json only', async () => {
      await branchWith('build/deps', { 'web/package.json': '{}\n', 'web/package-lock.json': '{}\n' });
      const f = facts();
      expect(f.dependencyOnly).toBe(true);
      expect(f.backlogTouched).toBe(false);
      expect(backlogCheck(f)).toEqual(DEP_OK);
    });

    it('accepts package-lock.json alone', async () => {
      await branchWith('build/lock', { 'web/package-lock.json': '{"a":1}\n' });
      expect(facts().dependencyOnly).toBe(true);
    });

    it('rejects package.json plus a source file', async () => {
      await branchWith('build/with-src', { 'web/package.json': '{"a":2}\n', 'web/src/x.ts': 'x\n' });
      const f = facts();
      expect(f.dependencyOnly).toBe(false);
      const check = backlogCheck(f);
      expect(check.ok).toBe(false);
      expect(check.message).toContain('NEXT-ITERATIONS.md is unchanged on this branch');
    });

    it('rejects a nested package.json', async () => {
      await branchWith('build/nested', { 'web/sub/package.json': '{}\n' });
      expect(facts().dependencyOnly).toBe(false);
    });

    it('lets a changed backlog win over the exemption', async () => {
      await branchWith('build/with-backlog', {
        'web/package.json': '{"a":3}\n',
        'web/NEXT-ITERATIONS.md': `${BACKLOG}\n- extra\n`,
      });
      const f = facts();
      expect(f.dependencyOnly).toBe(false);
      expect(backlogCheck(f)).toEqual({ ok: true, message: 'NEXT-ITERATIONS.md changed on this branch' });
    });

    it('ignores paths outside web/', async () => {
      await branchWith('build/with-github', {
        'web/package-lock.json': '{"a":4}\n',
        '.github/dependabot.yml': 'version: 2\n',
      });
      expect(facts().dependencyOnly).toBe(true);
    });

    it('does not treat a change outside web/ as a web change', async () => {
      await branchWith('build/github-only', { '.github/dependabot.yml': 'version: 3\n' });
      const f = facts();
      expect(f.folderChanged).toBe(false);
      expect(f.dependencyOnly).toBe(false);
    });
  });

  describe('backlogCheck', () => {
    it('reports a changed backlog', () => {
      expect(backlogCheck({ backlogChanged: true, backlogTouched: true, archived: [], dependencyOnly: false })).toEqual({
        ok: true,
        message: 'NEXT-ITERATIONS.md changed on this branch',
      });
    });

    it('reports a delivered backlog', () => {
      const archived = [{ path: 'a.md', status: 'done' }];
      expect(backlogCheck({ backlogChanged: false, backlogTouched: true, archived, dependencyOnly: false })).toEqual({
        ok: true,
        message: 'NEXT-ITERATIONS.md delivered on this branch (1 archived entry(ies))',
      });
    });
  });
});

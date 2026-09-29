import { promises as fsp } from 'fs';
import * as os from 'os';
import * as path from 'path';

// The readers behind `npm run branch-ready`, `npm run release-ready` and
// `npm run start-item`. They are what makes the backlog conventions enforceable
// — a regression here would silently pass a branch that still has
// shipped-but-unarchived entries, or start an entry that is not committed — so
// they are covered directly rather than through the reports, which resolve
// their paths from the project root and cannot be pointed at a fixture. (The
// git-backed start gate and branch facts have their own spec against a fixture
// repository: readiness-git.spec.ts.)
// eslint-disable-next-line @typescript-eslint/no-require-imports
const changelog = require('../scripts/lib/changelog');
const {
  readChangelog,
  readStruckLines,
  readEntryNumbers,
  parseChangelog,
  entrySection,
  entryTitle,
  entryTitleStruck,
  entryGoal,
  entryPlan,
  planOpenItems,
  entryNumbers,
  parseFrontmatter,
  unconventionalSubjects,
} = changelog;

const BACKLOG = [
  '# Next iterations',
  '',
  '## 1. First entry',
  '',
  '**Goal.** Do the first thing,',
  'across two lines.',
  '',
  '> **Note.** A note.',
  '',
  '**Plan.**',
  '',
  '- ~~done item~~',
  '- open item',
  '  - nested item does not count',
  '- another open item',
  '',
  '## 2. ~~Second entry~~',
  '',
  '**Goal.** Second.',
  '',
  '**Plan.**',
  '',
  '- ~~all~~',
  '',
  '## 11. Eleventh entry',
  '',
  '**Goal.** Eleven.',
  '',
  '**Plan.**',
  '',
  '```',
  '## not a heading',
  '```',
  '',
  '- open',
  '',
  '## Parked ideas',
  '',
  '### Idea: something',
  '',
  'Text.',
  '',
].join('\n');

const NESTED = ['## Features', '', '### 1. Web entry', '', '**Goal.** Web.', '', '**Plan.**', '', '- one', '', '### 2. Another', '', '**Goal.** Two.', '', '## Fixes', '', 'None.', ''].join(
  '\n',
);

describe('readiness readers', () => {
  let dir: string;

  beforeAll(async () => {
    dir = await fsp.mkdtemp(path.join(os.tmpdir(), 'readiness-'));
  });

  afterAll(async () => {
    await fsp.rm(dir, { recursive: true, force: true });
  });

  async function changelog(body: string): Promise<string> {
    const file = path.join(dir, `changelog-${Math.random().toString(36).slice(2)}.md`);
    await fsp.writeFile(file, body);
    return file;
  }

  async function nextIterations(body: string): Promise<string> {
    const file = path.join(dir, `next-${Math.random().toString(36).slice(2)}.md`);
    await fsp.writeFile(file, body);
    return file;
  }

  it('reads an open [Unreleased] with its items and the newest released version', async () => {
    const file = await changelog(
      '# Changelog\n\n## [Unreleased]\n\n### Added\n\n- **Something.** Shipped.\n\n## [0.1.1] - 2026-09-07\n\n- old\n',
    );
    const result = readChangelog(file);
    expect(result.unreleasedIndex).toBeGreaterThan(-1);
    expect(result.unreleasedItems).toHaveLength(2);
    expect(result.version).toBe('0.1.1');
    // A dated heading is history, so nothing is awaiting release.
    expect(result.awaitingRelease).toBe(false);
  });

  it('treats an undated newest heading as closed and awaiting release', async () => {
    const file = await changelog('# Changelog\n\n## [Unreleased]\n\n## [0.2.0]\n\n- new\n');
    const result = readChangelog(file);
    expect(result.awaitingRelease).toBe(true);
    expect(result.version).toBe('0.2.0');
    expect(result.unreleasedItems).toEqual([]);
  });

  it('reports a missing [Unreleased] section and a changelog without any version', async () => {
    const file = await changelog('# Changelog\n\nnothing here yet\n');
    const result = readChangelog(file);
    expect(result.unreleasedIndex).toBe(-1);
    expect(result.version).toBe('');
    expect(result.awaitingRelease).toBe(false);
  });

  it('finds struck-out lines without mistaking a ~~~ fence for one', async () => {
    const file = await nextIterations(
      '### ~~5. Shipped entry~~\n\n- ~~a delivered item~~\n- an outstanding item\n\n~~~\nnot a strikeout\n~~~\n',
    );
    const struck = readStruckLines(file);
    expect(struck.map((s: { n: number }) => s.n)).toEqual([1, 3]);
  });

  it('reads entry numbers from struck and unstruck titles alike', async () => {
    const file = await nextIterations(
      '## Fixes\n\n### 1. One\n\n### ~~2. Two~~\n\n### 3. Three\n\n## Parked ideas\n\n### Idea: not numbered\n',
    );
    expect(readEntryNumbers(file)).toEqual([1, 2, 3]);
  });

  it('exposes a gap in the numbering rather than smoothing it over', async () => {
    // What catches a clear-out that forgot to renumber the survivors.
    const file = await nextIterations('### 1. One\n\n### 3. Three\n');
    expect(readEntryNumbers(file)).toEqual([1, 3]);
  });

  describe('string readers', () => {
    it('extracts entry 1 up to the next heading of the same level', () => {
      const section = entrySection(BACKLOG, 1);
      expect(section[0]).toBe('## 1. First entry');
      expect(section.filter((l) => l !== '').at(-1)).toBe('- another open item');
      expect(section.join('\n')).not.toContain('Second');
    });

    it('matches a struck title, and 1 does not match 11', () => {
      expect(entrySection(BACKLOG, 2)[0]).toBe('## 2. ~~Second entry~~');
      expect(entrySection(BACKLOG, 1).join('\n')).not.toContain('Eleven');
      expect(entrySection(BACKLOG, 11)[0]).toBe('## 11. Eleventh entry');
    });

    it('does not end a section at a heading inside a fence, and yields [] for an absent entry', () => {
      expect(entrySection(BACKLOG, 11).filter((l) => l !== '').at(-1)).toBe('- open');
      expect(entrySection(BACKLOG, 3)).toEqual([]);
    });

    it('reads ### entries under ## Features and stops at the next ### or ##', () => {
      const one = entrySection(NESTED, 1);
      expect(one[0]).toBe('### 1. Web entry');
      expect(one.filter((l) => l !== '').at(-1)).toBe('- one');
      expect(entrySection(NESTED, 2).filter((l) => l !== '').at(-1)).toBe('**Goal.** Two.');
    });

    it('reads title, struck state, goal, plan and open items', () => {
      const one = entrySection(BACKLOG, 1);
      const two = entrySection(BACKLOG, 2);
      expect(entryTitle(one)).toBe('First entry');
      expect(entryTitle(two)).toBe('Second entry');
      expect(entryTitleStruck(one)).toBe(false);
      expect(entryTitleStruck(two)).toBe(true);
      expect(entryGoal(one)).toEqual(['**Goal.** Do the first thing,', 'across two lines.']);
      expect(entryPlan(one)[0]).toBe('**Plan.**');
      expect(planOpenItems(one)).toEqual(['- open item', '- another open item']);
      expect(planOpenItems(two)).toEqual([]);
      expect(entryPlan(['## 3. X', '', '**Goal.** G.'])).toEqual([]);
    });

    it('lists entry numbers and ignores ideas', () => {
      expect(entryNumbers(BACKLOG)).toEqual([1, 2, 11]);
      expect(entryNumbers(NESTED)).toEqual([1, 2]);
    });

    it('parses a changelog from a string the same way as from a file', () => {
      const text = '# Changelog\n\n## [Unreleased]\n\n### Added\n\n- **One.** Text.\n\n## [0.1.0] - 2026-09-06\n\n- old\n';
      const parsed = parseChangelog(text);
      expect(parsed.unreleasedItems).toEqual(['### Added', '- **One.** Text.']);
      expect(parsed.version).toBe('0.1.0');
      expect(parsed.awaitingRelease).toBe(false);
      expect(parseChangelog('## [Unreleased]\n\n## [0.1.0]\n- x\n').unreleasedItems).toEqual([]);
    });

    it('reports commit subjects that are not Conventional Commits', () => {
      expect(
        unconventionalSubjects([
          'feat(go): add a thing',
          'fix(web): repair a thing',
          'chore: tidy',
          'chore(release): go v0.4.0, web v0.4.0',
          'feat(go)!: drop the flag',
          'Feat(go): capitalised type',
          'feat(api): unknown scope',
          'feat(go): Trailing period.',
          'update stuff',
          'feat: ',
          'feat(web): x',
        ]),
      ).toEqual(['Feat(go): capitalised type', 'feat(api): unknown scope', 'feat(go): Trailing period.', 'update stuff', 'feat: ']);
    });

    it('parses a leading frontmatter block and nothing else', () => {
      const text = '---\ntitle: "Quoted title"\nstatus: done\nchangelog: Unreleased\n---\n\nstatus: body\n';
      expect(parseFrontmatter(text)).toEqual({ title: 'Quoted title', status: 'done', changelog: 'Unreleased' });
      expect(parseFrontmatter('status: done\n')).toEqual({});
      expect(parseFrontmatter('---\nstatus: done\n')).toEqual({});
    });
  });
});

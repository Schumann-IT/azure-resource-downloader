import { promises as fsp } from 'fs';
import * as os from 'os';
import * as path from 'path';

// The readers behind `npm run branch-ready` and `npm run release-ready`. They
// are what makes the strikeout convention enforceable — a regression here would
// silently pass a branch that still has shipped-but-uncleared entries — so they
// are covered directly rather than through the scripts, which resolve their
// paths from the project root and cannot be pointed at a fixture.
// eslint-disable-next-line @typescript-eslint/no-var-requires
const {
  readChangelog,
  readStruckLines,
  readEntryNumbers,
} = require('../scripts/lib/changelog');

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
});

import {
  charParts,
  DiffLine,
  diffYaml,
  MAX_DIFF_BYTES,
  MAX_FULL_LINES,
  MAX_PART_LINE,
  pairRows,
  WHOLE_FILE,
} from '../src/docs/yaml-diff';

const lines = (n: number, prefix = 'line') =>
  Array.from({ length: n }, (_, i) => `${prefix}${i + 1}`).join('\n') + '\n';

describe('diffYaml', () => {
  it('numbers removed, added and context lines against their own side', () => {
    const diff = diffYaml('a: 1\nb: 2\nc: 3\n', 'a: 1\nb: 20\nc: 3\nd: 4\n');
    expect(diff.added).toBe(2);
    expect(diff.removed).toBe(1);
    expect(diff.hunks).toHaveLength(1);
    expect(diff.hunks[0].header).toBe('@@ -1,3 +1,4 @@');
    expect(diff.hunks[0].lines).toEqual([
      { text: 'a: 1', added: false, removed: false, oldNo: 1, newNo: 1 },
      { text: 'b: 2', added: false, removed: true, oldNo: 2, newNo: null },
      { text: 'b: 20', added: true, removed: false, oldNo: null, newNo: 2 },
      { text: 'c: 3', added: false, removed: false, oldNo: 3, newNo: 3 },
      { text: 'd: 4', added: true, removed: false, oldNo: null, newNo: 4 },
    ]);
  });

  it('collapses long unchanged runs into separate hunks with context', () => {
    const before = lines(30);
    const after = before.replace('line2\n', 'LINE2\n').replace('line28\n', 'LINE28\n');
    const diff = diffYaml(before, after);
    expect(diff.hunks.map((h) => h.header)).toEqual([
      '@@ -1,5 +1,5 @@',
      '@@ -25,6 +25,6 @@',
    ]);
    expect(diff.hunks[0].lines.map((l) => l.text)).not.toContain('line10');
  });

  it('yields no hunks for identical files', () => {
    const diff = diffYaml('a: 1\n', 'a: 1\n');
    expect(diff).toEqual({
      hunks: [],
      added: 0,
      removed: 0,
      changes: 0,
      changesLabel: '0 differences',
      wholeFile: false,
      capped: false,
      tooLarge: false,
    });
  });

  it('keeps the text verbatim — escaping is the template\'s job', () => {
    const diff = diffYaml('x: 1\n', 'x: "<script>"\n');
    expect(diff.hunks[0].lines.map((l) => l.text)).toContain('x: "<script>"');
  });

  it('refuses inputs above the size cap instead of stalling the request', () => {
    const big = 'a'.repeat(MAX_DIFF_BYTES);
    expect(diffYaml(big, 'b\n')).toEqual({
      hunks: [],
      added: 0,
      removed: 0,
      changes: 0,
      changesLabel: '0 differences',
      wholeFile: false,
      capped: false,
      tooLarge: true,
    });
  });
});

describe('diffYaml (whole file)', () => {
  it('keeps every line in one hunk with its line numbers', () => {
    const before = lines(30);
    const after = before.replace('line2\n', 'LINE2\n').replace('line28\n', 'LINE28\n');
    const diff = diffYaml(before, after, WHOLE_FILE);
    expect(diff.wholeFile).toBe(true);
    expect(diff.capped).toBe(false);
    expect(diff.hunks).toHaveLength(1);
    const texts = diff.hunks[0].lines.map((l) => l.text);
    expect(texts).toContain('line10');
    expect(diff.hunks[0].lines[0]).toMatchObject({ text: 'line1', oldNo: 1, newNo: 1 });
    const all = diff.hunks[0].lines;
    expect(all[all.length - 1]).toMatchObject({ text: 'line30', oldNo: 30, newNo: 30 });
    expect(diff.changes).toBe(2);
  });

  it('yields no hunk at all for equal texts', () => {
    const diff = diffYaml('a: 1\n', 'a: 1\n', WHOLE_FILE);
    expect(diff.hunks).toEqual([]);
    expect(diff.wholeFile).toBe(true);
  });

  it('falls back to hunks above the line cap, and says so', () => {
    const before = lines(MAX_FULL_LINES + 1);
    const after = before.replace('line2\n', 'LINE2\n');
    const diff = diffYaml(before, after, WHOLE_FILE);
    expect(diff.capped).toBe(true);
    expect(diff.wholeFile).toBe(false);
    expect(diff.hunks[0].header).toBe('@@ -1,5 +1,5 @@');
  });
});

describe('charParts', () => {
  it('marks only the characters that differ', () => {
    const parts = charParts('at: "2026-09-03T23:31:35Z"', 'at: "2026-09-03T23:29:37Z"');
    expect(parts).not.toBeNull();
    const changed = (side: Array<{ text: string; changed: boolean }>) =>
      side.filter((p) => p.changed).map((p) => p.text).join('');
    expect(parts?.left.map((p) => p.text).join('')).toBe('at: "2026-09-03T23:31:35Z"');
    expect(parts?.right.map((p) => p.text).join('')).toBe('at: "2026-09-03T23:29:37Z"');
    expect(changed(parts?.left ?? []).length).toBeLessThanOrEqual(3);
    expect(changed(parts?.right ?? []).length).toBeLessThanOrEqual(3);
    expect(parts?.left[0].changed).toBe(false);
    expect(parts?.left[0].text.startsWith('at: "2026-09-03T23:')).toBe(true);
  });

  it('gives up on two unrelated lines instead of producing confetti', () => {
    expect(charParts('displayName: Alpha', 'id: 9f3c')).toBeNull();
  });

  it('skips lines over the length cap', () => {
    const long = 'x'.repeat(MAX_PART_LINE + 1);
    expect(charParts(long, `${long}y`)).toBeNull();
  });

  it('keeps the text verbatim, markup included', () => {
    const parts = charParts('x: "<a>"', 'x: "<b>"');
    expect(parts?.right.map((p) => p.text).join('')).toBe('x: "<b>"');
  });
});

describe('diffYaml rows', () => {
  it('marks a modified row and leaves one-sided rows unmarked', () => {
    const diff = diffYaml('a\nb: 1\nc\nd\n', 'a\nb: 2\n');
    const rows = diff.hunks[0].rows.filter((r) => !r.context);
    expect(rows.map((r) => [r.left?.text ?? null, r.right?.text ?? null, r.modified])).toEqual([
      ['b: 1', 'b: 2', true],
      ['c', null, false],
      ['d', null, false],
    ]);
    expect(rows[0].leftParts).toEqual([
      { text: 'b: ', changed: false },
      { text: '1', changed: true },
    ]);
    expect(rows[1].leftParts).toBeNull();
  });

  it('numbers each run of changed rows once and links it to the next', () => {
    const before = lines(20);
    const after = before
      .replace('line2\nline3\n', 'LINE2\nLINE3\n')
      .replace('line10\n', 'LINE10\n')
      .replace('line18\n', 'LINE18\n');
    const diff = diffYaml(before, after, WHOLE_FILE);
    expect(diff.changes).toBe(3);
    expect(diff.changesLabel).toBe('3 differences');
    const starts = diff.hunks[0].rows.filter((r) => r.change !== null);
    expect(starts.map((r) => [r.left?.text, r.change, r.next?.n, r.next?.first])).toEqual([
      ['line2', 1, 2, false],
      ['line10', 2, 3, false],
      ['line18', 3, 1, true],
    ]);
    expect(starts.map((r) => r.next?.label)).toEqual([
      'next difference',
      'next difference',
      'first difference',
    ]);
  });

  it('numbers differences across hunks, and gives a single one no link', () => {
    const before = lines(30);
    const twice = before.replace('line2\n', 'LINE2\n').replace('line28\n', 'LINE28\n');
    const diff = diffYaml(before, twice);
    expect(diff.hunks.map((h) => h.rows.find((r) => r.change !== null)?.change)).toEqual([1, 2]);
    const once = diffYaml(before, before.replace('line2\n', 'LINE2\n'));
    expect(once.changesLabel).toBe('1 difference');
    expect(once.hunks[0].rows.find((r) => r.change === 1)?.next).toBeNull();
  });
});

describe('pairRows', () => {
  const ctx = (text: string, n: number): DiffLine => ({ text, added: false, removed: false, oldNo: n, newNo: n });
  const del = (text: string, n: number): DiffLine => ({ text, added: false, removed: true, oldNo: n, newNo: null });
  const add = (text: string, n: number): DiffLine => ({ text, added: true, removed: false, oldNo: null, newNo: n });

  it('puts a modified line on one row, removed left and added right', () => {
    const rows = pairRows([ctx('a: 1', 1), del('b: 2', 2), add('b: 20', 2), ctx('c: 3', 3)]);
    expect(rows).toEqual([
      { context: true, left: ctx('a: 1', 1), right: ctx('a: 1', 1) },
      { context: false, left: del('b: 2', 2), right: add('b: 20', 2) },
      { context: true, left: ctx('c: 3', 3), right: ctx('c: 3', 3) },
    ]);
  });

  it('leaves the overhang of an uneven change one-sided', () => {
    const rows = pairRows([del('x', 1), del('y', 2), del('z', 3), add('x2', 1)]);
    expect(rows.map((r) => [r.left?.text ?? null, r.right?.text ?? null])).toEqual([
      ['x', 'x2'],
      ['y', null],
      ['z', null],
    ]);
  });

  it('pairs an added run followed by a removed run the same way', () => {
    const rows = pairRows([add('new', 1), del('old', 1)]);
    expect(rows).toEqual([{ context: false, left: del('old', 1), right: add('new', 1) }]);
  });

  it('does not pair changes separated by context', () => {
    const rows = pairRows([del('a', 1), ctx('b', 2), add('c', 2)]);
    expect(rows.map((r) => [r.left?.text ?? null, r.right?.text ?? null])).toEqual([
      ['a', null],
      ['b', 'b'],
      [null, 'c'],
    ]);
  });

  it('is what diffYaml attaches to every hunk', () => {
    const diff = diffYaml('a: 1\nb: 2\n', 'a: 1\nb: 20\n');
    const paired = diff.hunks[0].rows.map(({ context, left, right }) => ({ context, left, right }));
    expect(paired).toEqual(pairRows(diff.hunks[0].lines));
    expect(diff.hunks[0].rows).toHaveLength(2);
  });
});

import { DiffLine, diffYaml, MAX_DIFF_BYTES, pairRows } from '../src/docs/yaml-diff';

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
    expect(diff).toEqual({ hunks: [], added: 0, removed: 0, tooLarge: false });
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
      tooLarge: true,
    });
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
    expect(diff.hunks[0].rows).toEqual(pairRows(diff.hunks[0].lines));
    expect(diff.hunks[0].rows).toHaveLength(2);
  });
});

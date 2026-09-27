import { diffYaml, MAX_DIFF_BYTES } from '../src/docs/yaml-diff';

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

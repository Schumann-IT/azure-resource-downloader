import { describe, expect, it } from 'vitest';
import {
  deepEqual,
  diffFileSets,
  diffValues,
  summarizeValueDiff,
} from '../src/diff';
import type { YamlValue } from '../src/types';

describe('deepEqual', () => {
  it('treats objects with reordered keys as equal', () => {
    expect(deepEqual({ a: 1, b: 2 }, { b: 2, a: 1 })).toBe(true);
  });
  it('detects differing arrays', () => {
    expect(deepEqual([1, 2, 3], [1, 2, 4])).toBe(false);
  });
  it('handles dotted/@ keys', () => {
    expect(
      deepEqual({ '@odata.type': 'x' }, { '@odata.type': 'x' }),
    ).toBe(true);
  });
});

describe('diffValues', () => {
  it('marks identical documents unchanged', () => {
    const d = diffValues({ a: 1, b: { c: 2 } }, { a: 1, b: { c: 2 } });
    expect(d.status).toBe('unchanged');
  });

  it('detects a changed scalar', () => {
    const d = diffValues({ a: 1 }, { a: 2 });
    expect(d.status).toBe('changed');
    const leaf = d.children?.[0];
    expect(leaf?.status).toBe('changed');
    expect(leaf?.left).toBe(1);
    expect(leaf?.right).toBe(2);
  });

  it('detects added and removed keys', () => {
    const d = diffValues({ a: 1, gone: true }, { a: 1, fresh: true });
    const byKey = Object.fromEntries(
      (d.children ?? []).map((c) => [c.key, c.status]),
    );
    expect(byKey.gone).toBe('removed');
    expect(byKey.fresh).toBe('added');
  });

  it('diffs arrays by index including added elements', () => {
    const d = diffValues({ list: [1, 2] }, { list: [1, 2, 3] });
    const list = d.children?.find((c) => c.key === 'list');
    const added = list?.children?.find((c) => c.key === '2');
    expect(added?.status).toBe('added');
    expect(added?.right).toBe(3);
  });

  it('honors ignoreFields by key name', () => {
    const left: YamlValue = { id: 'A', displayName: 'X' };
    const right: YamlValue = { id: 'B', displayName: 'X' };
    expect(diffValues(left, right).status).toBe('changed');
    expect(diffValues(left, right, { ignoreFields: ['id'] }).status).toBe(
      'unchanged',
    );
  });

  it('honors ignoreFields by full dotted path', () => {
    const left: YamlValue = { meta: { lastModifiedDateTime: '2026-01-01' }, v: 1 };
    const right: YamlValue = { meta: { lastModifiedDateTime: '2026-02-02' }, v: 1 };
    expect(
      diffValues(left, right, { ignoreFields: ['meta.lastModifiedDateTime'] })
        .status,
    ).toBe('unchanged');
  });
});

describe('summarizeValueDiff', () => {
  it('counts leaf changes by status', () => {
    const d = diffValues(
      { a: 1, b: 2, gone: 1, same: 'z' },
      { a: 9, b: 2, fresh: 1, same: 'z' },
    );
    const s = summarizeValueDiff(d);
    expect(s.changed).toBe(1); // a
    expect(s.removed).toBe(1); // gone
    expect(s.added).toBe(1); // fresh
    expect(s.unchanged).toBe(2); // b, same
  });
});

describe('diffFileSets', () => {
  const left: Record<string, YamlValue> = {
    keep: { v: 1 },
    change: { v: 1 },
    onlyLeft: { v: 1 },
  };
  const right: Record<string, YamlValue> = {
    keep: { v: 1 },
    change: { v: 2 },
    onlyRight: { v: 1 },
  };

  it('classifies added/removed/changed/unchanged at file level', () => {
    const { entries, summary } = diffFileSets(left, right);
    const bySlug = Object.fromEntries(entries.map((e) => [e.slug, e.status]));
    expect(bySlug.keep).toBe('unchanged');
    expect(bySlug.change).toBe('changed');
    expect(bySlug.onlyLeft).toBe('removed');
    expect(bySlug.onlyRight).toBe('added');
    expect(summary).toEqual({ added: 1, removed: 1, changed: 1, unchanged: 1 });
  });

  it('attaches per-field change counts for changed files', () => {
    const { entries } = diffFileSets(left, right);
    const changed = entries.find((e) => e.slug === 'change');
    expect(changed?.changes?.changed).toBe(1);
  });
});

import {
  compareListing,
  comparePane,
  FileDigest,
  fileDigest,
  formatSize,
  interleaveRows,
  ListingCell,
  pairComparison,
  pairStatus,
} from '../src/docs/compare-view';
import { MetadataEntry, ResourcesMetadata } from '../src/docs/resources-metadata';
import { MAX_DIFF_BYTES } from '../src/docs/yaml-diff';

const cell = (name: string): ListingCell => ({ key: name, name, href: `/${name}` });

const meta = (entries: Array<[string, string]>): ResourcesMetadata => ({
  entries: entries.map(
    ([key, displayName]): MetadataEntry => ({
      key,
      type: key.slice(0, key.lastIndexOf('/')),
      displayName,
      presentInTenant: true,
    }),
  ),
  nameCounts: new Map(),
  lookup: new Map(),
});

const policy = (group: string, value: number, created = '2026-01-01T00:00:00Z') =>
  [
    `createdDateTime: "${created}"`,
    'name: Policy',
    'assignments:',
    '    - target:',
    `        groupId: ${group}`,
    'settings:',
    `    - value: ${value}`,
    '',
  ].join('\n');

describe('fileDigest', () => {
  const m = meta([]);

  it('hashes a parsable file as exported, normalised and without assignments', () => {
    const d = fileDigest(policy('g1', 1), m);
    expect(d.size).toBe(Buffer.byteLength(policy('g1', 1)));
    expect(d.raw).toMatch(/^[0-9a-f]{64}$/);
    expect(d.normalised).toMatch(/^[0-9a-f]{64}$/);
    expect(d.withoutAssignments).toMatch(/^[0-9a-f]{64}$/);
    expect(d.normalised).not.toBe(d.withoutAssignments);
    expect(d.reason).toBe('');
  });

  it('digests two files equal after normalisation equally', () => {
    const a = fileDigest(policy('g1', 1, '2026-01-01T00:00:00Z'), m);
    const b = fileDigest(policy('g1', 1, '2026-05-05T00:00:00Z'), m);
    expect(a.raw).not.toBe(b.raw);
    expect(a.normalised).toBe(b.normalised);
  });

  it('keeps only the raw hash of a file that does not parse', () => {
    const d = fileDigest('a: [', m);
    expect(d.raw).toMatch(/^[0-9a-f]{64}$/);
    expect(d.normalised).toBeNull();
    expect(d.reason).toBe('does not parse');
  });

  it('keeps only the raw hash of a file over the size cap', () => {
    const d = fileDigest(`a: "${'x'.repeat(MAX_DIFF_BYTES)}"\n`, m);
    expect(d.normalised).toBeNull();
    expect(d.withoutAssignments).toBeNull();
    expect(d.reason).toBe('too large to normalise');
  });
});

describe('pairStatus', () => {
  const m = meta([]);
  const digest = (raw: string) => fileDigest(raw, m);
  const large = (raw: string): FileDigest => ({
    size: MAX_DIFF_BYTES,
    raw,
    normalised: null,
    withoutAssignments: null,
    reason: 'too large to normalise',
  });

  it('calls equal files identical', () => {
    expect(pairStatus(digest(policy('g1', 1)), digest(policy('g1', 1)))).toEqual({
      state: 'identical',
      reason: '',
    });
  });

  it('calls files equal after normalisation identical', () => {
    const left = digest(policy('g1', 1, '2026-01-01T00:00:00Z'));
    const right = digest(policy('g1', 1, '2026-05-05T00:00:00Z'));
    expect(pairStatus(left, right).state).toBe('identical');
  });

  it('tells a difference only in audience from a real one', () => {
    expect(pairStatus(digest(policy('g1', 1)), digest(policy('g2', 1))).state).toBe('audience');
    expect(pairStatus(digest(policy('g1', 1)), digest(policy('g1', 2))).state).toBe('different');
    expect(pairStatus(digest(policy('g1', 1)), digest(policy('g2', 2))).state).toBe('different');
  });

  it('cannot judge a file that does not parse or could not be read', () => {
    expect(pairStatus(digest(policy('g1', 1)), digest('a: ['))).toEqual({
      state: 'unknown',
      reason: 'does not parse',
    });
    expect(pairStatus(null, digest(policy('g1', 1)))).toEqual({
      state: 'unknown',
      reason: 'a file could not be read',
    });
  });

  it('compares a too-large pair as exported', () => {
    expect(pairStatus(large('same'), large('same')).state).toBe('identical');
    expect(pairStatus(large('one'), large('two'))).toEqual({
      state: 'unknown',
      reason: 'too large to normalise',
    });
  });

  it('agrees with the pair diff on identical and audience only', () => {
    const cases: Array<[string, string]> = [
      [policy('g1', 1), policy('g1', 1, '2026-05-05T00:00:00Z')],
      [policy('g1', 1), policy('g2', 1)],
      [policy('g1', 1), policy('g1', 2)],
    ];
    for (const [l, r] of cases) {
      const status = pairStatus(digest(l), digest(r));
      const diff = pairComparison(l, r, m, m, false);
      expect(status.state === 'identical').toBe(diff.identical);
      expect(status.state === 'audience').toBe(diff.audienceOnly);
    }
  });
});

describe('comparePane', () => {
  const P = 'Microsoft.Graph/deviceCompliancePolicies';
  const G = 'Microsoft.Graph/groups';
  const listing = compareListing(
    {
      id: 'stage',
      metadata: meta([
        [`${P}/diff`, 'Differs'],
        [`${P}/same`, 'Same'],
        [`${P}/stage_only`, 'Stage only'],
        [`${G}/admins`, 'Admins'],
        ['Microsoft.Graph/windowsAutopilotDeviceIdentities/dev1', 'Device one'],
      ]),
    },
    {
      id: 'prod',
      metadata: meta([
        [`${P}/diff`, 'Differs (prod)'],
        [`${P}/same`, 'Same'],
        [`${P}/prod_only`, 'Prod only'],
        [`${G}/admins`, 'Admins'],
      ]),
    },
    new Set(['Microsoft.Graph/windowsAutopilotDeviceIdentities']),
    '_resource',
  );
  const d = (raw: string, normalised: string): FileDigest => ({
    size: 2048,
    raw,
    normalised,
    withoutAssignments: normalised,
    reason: '',
  });
  const left = new Map<string, FileDigest | null>([
    [`${P}/diff`, d('l', 'one')],
    [`${P}/same`, d('s', 'same')],
    [`${G}/admins`, d('a', 'admins')],
  ]);
  const right = new Map<string, FileDigest | null>([
    [`${P}/diff`, d('r', 'two')],
    [`${P}/same`, d('s', 'same')],
    [`${G}/admins`, d('a', 'admins')],
  ]);
  const pane = (selectedKey = '', same = false) =>
    comparePane(listing, left, right, { a: 'stage', b: 'prod', selectedKey, same });
  const rows = (p: ReturnType<typeof pane>) =>
    p.groups.flatMap((g) => g.rows.map((r) => [r.n, r.key.slice(r.key.lastIndexOf('/') + 1), r.state]));

  it('shows what needs attention and counts everything', () => {
    const p = pane();
    expect(p.totals).toEqual({ different: 1, onlyA: 2, onlyB: 1, identical: 2, unknown: 0 });
    expect(p.groups.map((g) => [g.label, g.excluded])).toEqual([
      ['Device Compliance Policies', false],
      ['Windows Autopilot Device Identities', true],
    ]);
    expect(rows(p)).toEqual([
      [1, 'diff', 'different'],
      [3, 'stage_only', 'onlyLeft'],
      [4, 'prod_only', 'onlyRight'],
      [6, 'dev1', 'onlyLeft'],
    ]);
  });

  it('shows identical pairs too with same, numbered as in the default view', () => {
    const p = pane('', true);
    expect(rows(p)).toEqual([
      [1, 'diff', 'different'],
      [2, 'same', 'identical'],
      [3, 'stage_only', 'onlyLeft'],
      [4, 'prod_only', 'onlyRight'],
      [5, 'admins', 'identical'],
      [6, 'dev1', 'onlyLeft'],
    ]);
  });

  it('keeps the selected pair shown even when it is identical', () => {
    const p = pane(`${P}/same`);
    const shown = p.groups.flatMap((g) => g.rows);
    expect(shown.filter((r) => r.selected).map((r) => r.n)).toEqual([2]);
    expect(shown.some((r) => r.key === `${G}/admins`)).toBe(false);
  });

  it('builds one href per row that keeps the view and drops the key', () => {
    const [diff, stageOnly] = pane('', true).groups[0].rows.filter((r) => r.key !== `${P}/same`);
    expect(diff.href).toBe(`/_compare/${P}/diff?a=stage&b=prod&same#row-1`);
    expect(stageOnly.href).toBe(`/stage/_resource/${P}/stage_only`);
    expect(pane().groups[0].rows[0].href).toBe(`/_compare/${P}/diff?a=stage&b=prod#row-1`);
  });

  it('labels every marker in words', () => {
    const labels = pane().groups.flatMap((g) => g.rows.map((r) => [r.marker, r.label]));
    expect(labels).toEqual([
      ['≠', 'different'],
      ['→', 'only in stage'],
      ['←', 'only in prod'],
      ['→', 'only in stage'],
    ]);
  });

  it('links the view toggles, swap and first difference', () => {
    const listingPane = pane();
    expect(listingPane.sameHref).toBe('/_compare?a=stage&b=prod&same');
    expect(listingPane.swapHref).toBe('/_compare?a=prod&b=stage');
    expect(listingPane.firstDifferenceHref).toBe(`/_compare/${P}/diff?a=stage&b=prod#row-1`);
    const pairPane = pane(`${P}/diff`, true);
    expect(pairPane.sameHref).toBe(`/_compare/${P}/diff?a=stage&b=prod`);
    expect(pairPane.swapHref).toBe(`/_compare/${P}/diff?a=prod&b=stage&same`);
    expect(pairPane.listingHref).toBe('/_compare?a=stage&b=prod&same');
  });

  it('says why it is empty', () => {
    const allSame = compareListing(
      { id: 'stage', metadata: meta([[`${P}/same`, 'Same']]) },
      { id: 'prod', metadata: meta([[`${P}/same`, 'Same']]) },
      new Set(),
      '_resource',
    );
    const p = comparePane(allSame, left, right, { a: 'stage', b: 'prod', selectedKey: '', same: false });
    expect(p.groups).toEqual([]);
    expect(p.emptyNote).toContain('Nothing needs attention');
    expect(pane().emptyNote).toBe('');
  });
});

describe('formatSize', () => {
  it('shows bytes, kilobytes and megabytes, and nothing when unread', () => {
    expect(formatSize(undefined)).toBe('');
    expect(formatSize(512)).toBe('512 B');
    expect(formatSize(2048)).toBe('2.0 KB');
    expect(formatSize(3 * 1024 * 1024)).toBe('3.0 MB');
  });
});

describe('interleaveRows', () => {
  it('orders pairs first, then left-only, then right-only, each alphabetical', () => {
    const rows = interleaveRows(
      [
        [cell('Zeta'), cell('Zeta (prod)')],
        [cell('Alpha'), cell('Alpha')],
      ],
      [cell('Only left B'), cell('Only left A')],
      [cell('Only right')],
    );
    expect(rows.map((r) => [r.left?.name ?? null, r.right?.name ?? null])).toEqual([
      ['Alpha', 'Alpha'],
      ['Zeta', 'Zeta (prod)'],
      ['Only left A', null],
      ['Only left B', null],
      [null, 'Only right'],
    ]);
  });

  it('does not reorder its inputs in place', () => {
    const onlyA = [cell('b'), cell('a')];
    interleaveRows([], onlyA, []);
    expect(onlyA.map((c) => c.name)).toEqual(['b', 'a']);
  });
});

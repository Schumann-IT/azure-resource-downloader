import {
  compareListing,
  compareNavigation,
  interleaveRows,
  ListingCell,
} from '../src/docs/compare-view';
import { MetadataEntry, ResourcesMetadata } from '../src/docs/resources-metadata';

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

describe('compareNavigation', () => {
  const P = 'Microsoft.Graph/deviceCompliancePolicies';
  const listing = compareListing(
    {
      id: 'stage',
      metadata: meta([
        [`${P}/win`, 'Windows'],
        [`${P}/stage_only`, 'Stage only'],
        ['Microsoft.Graph/windowsAutopilotDeviceIdentities/dev1', 'Device one'],
      ]),
    },
    { id: 'prod', metadata: meta([[`${P}/win`, 'Windows (prod)'], [`${P}/prod_only`, 'Prod only']]) },
    new Set(['Microsoft.Graph/windowsAutopilotDeviceIdentities']),
    '_resource',
  );

  it('mirrors the listing: its order, labelled types, notes and hrefs', () => {
    const nav = compareNavigation(listing, 'stage', 'prod');
    expect(nav.map((s) => [s.label, s.note, s.active])).toEqual([
      ['Device Compliance Policies', '', false],
      ['Windows Autopilot Device Identities', 'not documented', false],
    ]);
    expect(nav[0].items.map((i) => [i.label, i.note, i.href, i.active])).toEqual([
      ['Windows', '', `/_compare/${P}/win?a=stage&b=prod`, false],
      ['Stage only', 'only in stage', `/stage/_resource/${P}/stage_only`, false],
      ['Prod only', 'only in prod', `/prod/_resource/${P}/prod_only`, false],
    ]);
  });

  it('marks the pair being viewed and opens its section', () => {
    const nav = compareNavigation(listing, 'stage', 'prod', `${P}/win`);
    expect(nav[0].active).toBe(true);
    expect(nav[0].items.filter((i) => i.active).map((i) => i.label)).toEqual(['Windows']);
    expect(nav[1].active).toBe(false);
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

import * as yaml from 'js-yaml';
import {
  carriesIdentity,
  normaliseResource,
} from '../src/docs/compare-normalise';
import {
  parseResourcesMetadata,
  ReferenceLookup,
} from '../src/docs/resources-metadata';

const G1 = '8964516b-c223-4f58-a866-232d3690c9b4';
const G2 = '7a73b11f-e242-4ffe-ab1a-c4a8a70d5f64';
const P1 = 'c036fdcb-4fad-4596-94cb-365d2b23a016';
const P2 = '866b2e5d-a549-472c-b618-3df33778ba05';
const FILTER = '3b0c88b6-a688-4387-aa7d-09e9923ed020';
const TEMPLATE = 'fd9922eb-42b0-448a-92ef-dc01a51f1aa1';
const ZERO = '00000000-0000-0000-0000-000000000000';
const ONE = '00000000-0000-0000-0000-000000000001';

const lookupOf = (pairs: Array<[string, string[]]>): ReferenceLookup =>
  new Map(pairs.map(([id, names]) => [id, new Set(names)]));

const policy = (policyId: string, groupId: string, value = 1) => `'@odata.context': https://graph.microsoft.com/beta/$metadata#x('${policyId}')
id: ${policyId}
createdDateTime: "2026-03-28T16:56:52Z"
lastModifiedDateTime: "2026-03-28T16:56:52Z"
name: Same policy
assignments:
    - id: ${policyId}_${groupId}
      source: direct
      sourceId: ${policyId}
      target:
        groupId: ${groupId}
settings:
    - id: "0"
      value: ${value}
settings@odata.context: https://graph.microsoft.com/beta/$metadata#x('${policyId}')/settings
`;

describe('normaliseResource', () => {
  it('makes the same configuration under different ids compare equal', () => {
    const a = normaliseResource(policy(P1, G1), lookupOf([[G1, ['Admins']]]), new Map());
    const b = normaliseResource(policy(P2, G2), lookupOf([[G2, ['Admins']]]), new Map());
    expect(a?.text).toBe(b?.text);
    expect(a?.text).toContain('groupId: Admins');
    expect(a?.text).not.toMatch(/[0-9a-f]{8}-[0-9a-f]{4}-/);
  });

  it('keeps a real setting difference', () => {
    const a = normaliseResource(policy(P1, G1, 1), lookupOf([[G1, ['Admins']]]), new Map());
    const b = normaliseResource(policy(P2, G2, 15), lookupOf([[G2, ['Admins']]]), new Map());
    expect(a?.text).not.toBe(b?.text);
  });

  it('reports what it dropped, including nested ids and nested @odata.context', () => {
    const out = normaliseResource(policy(P1, G1), lookupOf([[G1, ['Admins']]]), new Map());
    expect(out?.report.droppedKeys).toEqual([
      '*@odata.context',
      'createdDateTime',
      'id',
      'lastModifiedDateTime',
      'sourceId',
    ]);
    // top-level id, assignments[].id, sourceId, two contexts, two timestamps
    expect(out?.report.dropped).toBe(7);
    expect(out?.report.resolved).toBe(1);
  });

  it('drops GUID-carrying id shapes and keeps ids that are content', () => {
    const raw = yaml.dump({
      items: [
        { id: P1 },
        { id: `${P1}_${G1}` },
        { id: `${P1}:${G1}` },
        { id: `${P1}_en-us` },
        { id: '0' },
        { id: 'all_users' },
        { id: 'Fido2' },
        { id: ZERO },
        { id: ONE },
      ],
    });
    const out = normaliseResource(raw, new Map(), new Map());
    const doc = yaml.load(out?.text ?? '') as { items: Array<{ id?: string }> };
    expect(doc.items.map((i) => i.id ?? null)).toEqual([
      null,
      null,
      null,
      null,
      '0',
      'all_users',
      'Fido2',
      ZERO,
      ONE,
    ]);
  });

  it('drops the group identity fields that differ between tenants', () => {
    const group = (mail: string) => `displayName: All Company
mail: ${mail}
mailNickname: ${mail}
proxyAddresses:
    - SMTP:${mail}
securityIdentifier: S-1-12-1-1
renewedDateTime: "2024-05-03T12:20:49Z"
version: 3
`;
    const a = normaliseResource(group('a@x'), new Map(), new Map());
    const b = normaliseResource(group('b@y'), new Map(), new Map());
    expect(a?.text).toBe(b?.text);
    expect(a?.text).toBe('displayName: All Company\n');
  });

  it('resolves a group, a filter and a notification template id', () => {
    const raw = `assignments:
    - target:
        groupId: ${G1}
        deviceAndAppManagementAssignmentFilterId: ${FILTER}
rules:
    - notificationTemplateId: ${TEMPLATE}
`;
    const out = normaliseResource(
      raw,
      lookupOf([
        [G1, ['Admins']],
        [FILTER, ['Mac filter']],
        [TEMPLATE, ['Warn users']],
      ]),
      new Map(),
    );
    expect(out?.text).toContain('groupId: Admins');
    expect(out?.text).toContain('deviceAndAppManagementAssignmentFilterId: Mac filter');
    expect(out?.text).toContain('notificationTemplateId: Warn users');
    expect(out?.report.resolved).toBe(3);
  });

  it('flags an ambiguous and an unresolved reference and keeps their GUIDs', () => {
    const raw = `a:\n    groupId: ${G1}\nb:\n    groupId: ${G2}\n`;
    const out = normaliseResource(raw, lookupOf([[G1, ['One', 'Two']]]), new Map());
    expect(out?.text).toContain(G1);
    expect(out?.text).toContain(G2);
    expect(out?.report).toMatchObject({ resolved: 0, ambiguous: 1, unresolved: 1 });
  });

  it('passes a zero-sentinel reference through unflagged', () => {
    const raw = `notificationTemplateId: ${ZERO}\n`;
    const out = normaliseResource(raw, new Map(), new Map());
    expect(out?.text).toContain(ZERO);
    expect(out?.report).toMatchObject({ resolved: 0, ambiguous: 0, unresolved: 0 });
  });

  it('counts a resolved name shared by several resources', () => {
    const out = normaliseResource(
      `groupId: ${G1}\n`,
      lookupOf([[G1, ['IT-Admins']]]),
      new Map([['IT-Admins', 2]]),
    );
    expect(out?.report.sharedNames).toBe(1);
  });

  it('separates an audience-only difference', () => {
    const lookupA = lookupOf([[G1, ['Stage audience']]]);
    const lookupB = lookupOf([[G2, ['Prod audience']]]);
    const a = normaliseResource(policy(P1, G1), lookupA, new Map());
    const b = normaliseResource(policy(P2, G2), lookupB, new Map());
    expect(a?.text).not.toBe(b?.text);
    const opts = { withoutAssignments: true };
    expect(normaliseResource(policy(P1, G1), lookupA, new Map(), opts)?.text).toBe(
      normaliseResource(policy(P2, G2), lookupB, new Map(), opts)?.text,
    );
    // The same cut, from the one parse: equal to normalising without assignments.
    expect(a?.textWithoutAssignments).toBe(
      normaliseResource(policy(P1, G1), lookupA, new Map(), opts)?.text,
    );
    expect(a?.textWithoutAssignments).toBe(b?.textWithoutAssignments);
    expect(normaliseResource('name: x\n', new Map(), new Map())?.textWithoutAssignments).toBe('name: x\n');
  });

  it('keeps an unquoted date-like scalar as the same text', () => {
    const out = normaliseResource('startDate: 2026-03-28\nflag: yes\n', new Map(), new Map());
    expect(out?.text).toContain('startDate: 2026-03-28\n');
    const reloaded = yaml.load(out?.text ?? '', { schema: yaml.CORE_SCHEMA });
    expect(reloaded).toEqual({ flag: 'yes', startDate: '2026-03-28' });
  });

  it('returns undefined for YAML that is not an object', () => {
    expect(normaliseResource('- a\n- b\n', new Map(), new Map())).toBeUndefined();
    expect(normaliseResource('a: [oops\n', new Map(), new Map())).toBeUndefined();
  });

  it('knows which values carry an identity', () => {
    expect(carriesIdentity(P1)).toBe(true);
    expect(carriesIdentity(ZERO)).toBe(false);
    expect(carriesIdentity('all_users')).toBe(false);
    expect(carriesIdentity(42)).toBe(false);
  });
});

describe('parseResourcesMetadata', () => {
  const RAW = `generatedAt: "2026-09-03T23:29:37Z"
resources:
    Microsoft.Graph/groups/admins.yaml:
        resourceId: ${G1}
        displayName: Admins
        odataType: '#microsoft.graph.group'
        presentInTenant: true
    Microsoft.Graph/groups/admins_2.yaml:
        resourceId: ${G2}
        displayName: Admins
        presentInTenant: true
    Microsoft.Graph/deviceConfigurations/gone.yaml:
        resourceId: ${P1}
        displayName: Gone
        presentInTenant: false
    Microsoft.Graph/deviceManagement/singleton.yaml:
        displayName: Singleton
    ../escape.yaml:
        displayName: Nope
    toplevel.yaml:
        displayName: Nope
`;

  it('strips the .yaml suffix, derives the type and keeps optional fields optional', () => {
    const meta = parseResourcesMetadata(RAW);
    expect(meta?.entries.map((e) => e.key)).toEqual([
      'Microsoft.Graph/deviceConfigurations/gone',
      'Microsoft.Graph/deviceManagement/singleton',
      'Microsoft.Graph/groups/admins',
      'Microsoft.Graph/groups/admins_2',
    ]);
    const singleton = meta?.entries.find((e) => e.displayName === 'Singleton');
    expect(singleton).toMatchObject({
      type: 'Microsoft.Graph/deviceManagement',
      presentInTenant: true,
    });
    expect(singleton?.resourceId).toBeUndefined();
    expect(meta?.entries.find((e) => e.displayName === 'Gone')?.presentInTenant).toBe(false);
  });

  it('builds the set-valued lookup and the name counts', () => {
    const meta = parseResourcesMetadata(RAW);
    expect(meta?.lookup.get(G1)).toEqual(new Set(['Admins']));
    expect(meta?.lookup.get(P1)).toEqual(new Set(['Gone']));
    expect(meta?.nameCounts.get('Admins')).toBe(2);
  });

  it('degrades to undefined on anything that is not a resources map', () => {
    expect(parseResourcesMetadata('generatedAt: x\n')).toBeUndefined();
    expect(parseResourcesMetadata('resources: [a]\n')).toBeUndefined();
    expect(parseResourcesMetadata('resources: {oops\n')).toBeUndefined();
  });
});

import {
  buildNavigation,
  buildProgrammeFilters,
  isKnownProgramme,
  parseTenantIndex,
  UNCATEGORISED,
} from '../src/docs/tenant-index';

const INDEX_YAML = `version: 1
tenant: contoso.com
generatedAt: "2026-01-01T00:00:00Z"
complete: false
incompleteReason: 4 resource types could not be listed
counts:
    documented: 2
    pending: 1
    excluded:
        Microsoft.Graph/windowsAutopilotDeviceIdentities: 4
        Microsoft.Graph/groups: 29
resources:
    - type: Microsoft.Graph/deviceCompliancePolicies
      doc: Microsoft.Graph/deviceCompliancePolicies/win_os.md
      displayName: Windows OS validation
      summary: Requires a minimum Windows build.
      documented: true
      scope: device
      assignments:
        groups: 2
    - type: Microsoft.Graph/assignmentFilters
      doc: Microsoft.Graph/assignmentFilters/mac_intel.md
      displayName: Intel Macs
      documented: true
      scope: device
    - type: Microsoft.Graph/deviceCompliancePolicies
      doc: Microsoft.Graph/deviceCompliancePolicies/mac_os.md
      displayName: macOS validation
      documented: false
`;

describe('parseTenantIndex', () => {
  it('parses the index emitted by `azure-rd docs generate-index`', () => {
    const index = parseTenantIndex(INDEX_YAML);
    expect(index).toBeDefined();
    expect(index!.tenant).toBe('contoso.com');
    expect(index!.generatedAt).toBe('2026-01-01T00:00:00Z');
    expect(index!.complete).toBe(false);
    expect(index!.incompleteReason).toBe(
      '4 resource types could not be listed',
    );
    expect(index!.counts).toEqual({
      documented: 2,
      pending: 1,
      // Sorted so the rendered page is stable across runs.
      excluded: [
        { type: 'Microsoft.Graph/groups', count: 29 },
        {
          type: 'Microsoft.Graph/windowsAutopilotDeviceIdentities',
          count: 4,
        },
      ],
    });
    expect(index!.resources).toHaveLength(3);
    expect(index!.resources[0].assignments).toEqual({
      groups: 2,
      allUsers: false,
      allDevices: false,
      targetedBy: 0,
    });
  });

  it('rejects anything that is not an index object (folder is then not a tenant)', () => {
    expect(parseTenantIndex('version: [oops\n')).toBeUndefined();
    expect(parseTenantIndex('')).toBeUndefined();
    expect(parseTenantIndex('- a\n- b\n')).toBeUndefined();
    expect(parseTenantIndex('version: 1\n')).toBeUndefined();
    expect(parseTenantIndex('version: 0\ntenant: x\n')).toBeUndefined();
    expect(parseTenantIndex('version: one\ntenant: x\n')).toBeUndefined();
  });

  it('accepts a later schema version and ignores fields it does not know', () => {
    // The index is also the tenant marker, so refusing a newer schema would make
    // the tenant disappear rather than degrade.
    const index = parseTenantIndex(
      'version: 3\ntenant: x\nsomethingNew: [a]\n' +
        'resources:\n  - type: T\n    doc: T/a.md\n    alsoNew: true\n',
    );
    expect(index).toBeDefined();
    expect(index!.version).toBe(3);
    expect(index!.resources).toHaveLength(1);
  });

  it('tolerates a missing resources list and unexpected field types', () => {
    const index = parseTenantIndex('version: 1\ntenant: x\nresources: nope\n');
    expect(index).toBeDefined();
    expect(index!.resources).toEqual([]);
    expect(index!.counts).toEqual({ documented: 0, pending: 0, excluded: [] });
  });
});

// A version-2 index as `docs generate-index` writes it with a `taxonomy:`
// section: the header programme registry (zero-count entries kept) plus
// many-to-many per-resource membership.
const PROGRAMME_YAML = `version: 2
tenant: contoso.com
vocabularies:
    platform: [Windows, macOS, n/a]
    function: [Compliance, Security]
programmes:
    - id: cis-hardening
      label: CIS hardening
      count: 2
    - id: vpn
      label: VPN
      count: 0
resources:
    - type: Microsoft.Graph/deviceCompliancePolicies
      doc: Microsoft.Graph/deviceCompliancePolicies/win_os.md
      displayName: Windows OS validation
      documented: true
      groups:
        - id: cis-hardening
          label: CIS hardening
        - id: defender
          label: Defender / MDE
    - type: Microsoft.Graph/deviceShellScripts
      doc: Microsoft.Graph/deviceShellScripts/mac_cis.md
      displayName: macOS CIS script
      documented: true
      groups:
        - id: cis-hardening
          label: CIS hardening
    - type: Microsoft.Graph/assignmentFilters
      doc: Microsoft.Graph/assignmentFilters/mac_intel.md
      displayName: Intel Macs
      documented: true
`;

describe('parseTenantIndex (version 2 grouping surface)', () => {
  const index = parseTenantIndex(PROGRAMME_YAML)!;

  it('reads the programme registry in the index order, keeping zero counts', () => {
    expect(index.programmes).toEqual([
      { id: 'cis-hardening', label: 'CIS hardening', count: 2 },
      { id: 'vpn', label: 'VPN', count: 0 },
    ]);
  });

  it('reads the axis vocabularies in their declared display order', () => {
    expect(index.vocabularies.platform).toEqual(['Windows', 'macOS', 'n/a']);
    expect(index.vocabularies.function).toEqual(['Compliance', 'Security']);
  });

  it('reads many-to-many membership as stable id plus display label', () => {
    expect(index.resources[0].groups).toEqual([
      { id: 'cis-hardening', label: 'CIS hardening' },
      { id: 'defender', label: 'Defender / MDE' },
    ]);
    expect(index.resources[2].groups).toEqual([]);
  });

  it('leaves an index without a taxonomy with no programmes and no groups', () => {
    const plain = parseTenantIndex(INDEX_YAML)!;
    expect(plain.programmes).toEqual([]);
    expect(plain.vocabularies).toEqual({ platform: [], function: [] });
    expect(plain.resources.every((r) => r.groups.length === 0)).toBe(true);
  });
});

describe('programme filter', () => {
  const index = parseTenantIndex(PROGRAMME_YAML)!;

  it('narrows the tree to the programme, reading membership and deriving none', () => {
    const sections = buildNavigation(index, 'contoso.com', '', 'cis-hardening');
    // Sections keep their type ordering; only the membership decides who is in.
    expect(sections.flatMap((s) => s.items).map((i) => i.label)).toEqual([
      'Windows OS validation',
      'macOS CIS script',
    ]);
  });

  it('keeps the active programme in every document href so it survives a click', () => {
    const sections = buildNavigation(index, 'contoso.com', '', 'cis-hardening');
    expect(sections[0].items[0].href).toBe(
      '/contoso.com/Microsoft.Graph/deviceCompliancePolicies/win_os?programme=cis-hardening',
    );
  });

  it('collects the resources no programme matched under the uncategorised bucket', () => {
    const sections = buildNavigation(index, 'contoso.com', '', UNCATEGORISED);
    expect(sections.flatMap((s) => s.items).map((i) => i.label)).toEqual([
      'Intel Macs',
    ]);
  });

  it('keeps the document being viewed even when the filter excludes it', () => {
    const sections = buildNavigation(
      index,
      'contoso.com',
      'Microsoft.Graph/assignmentFilters/mac_intel',
      'cis-hardening',
    );
    const filters = sections.find(
      (s) => s.key === 'Microsoft.Graph/assignmentFilters',
    )!;
    expect(filters.active).toBe(true);
    expect(filters.items.map((i) => i.label)).toEqual(['Intel Macs']);
  });

  it('badges each resource with the programmes it belongs to', () => {
    const sections = buildNavigation(index, 'contoso.com');
    const policy = sections
      .flatMap((s) => s.items)
      .find((i) => i.label === 'Windows OS validation')!;
    expect(policy.badges.slice(0, 2)).toEqual([
      'CIS hardening',
      'Defender / MDE',
    ]);
  });

  it('offers All, every declared programme in order, and uncategorised', () => {
    const filters = buildProgrammeFilters(index, '/contoso.com');
    expect(filters.map((f) => [f.id, f.count])).toEqual([
      ['', 3],
      ['cis-hardening', 2],
      ['vpn', 0],
      [UNCATEGORISED, 1],
    ]);
    expect(filters[0].active).toBe(true);
    expect(filters[0].href).toBe('/contoso.com');
    expect(filters[1].href).toBe('/contoso.com?programme=cis-hardening');
    expect(filters[3].href).toBe(`/contoso.com?programme=${UNCATEGORISED}`);
  });

  it('marks the active choice and hangs the filter off the current page', () => {
    const base = '/contoso.com/Microsoft.Graph/deviceShellScripts/mac_cis';
    const filters = buildProgrammeFilters(index, base, 'cis-hardening');
    expect(filters.filter((f) => f.active).map((f) => f.id)).toEqual([
      'cis-hardening',
    ]);
    expect(filters[1].href).toBe(`${base}?programme=cis-hardening`);
  });

  it('offers no filter at all for an index without a taxonomy', () => {
    const plain = parseTenantIndex(INDEX_YAML)!;
    expect(buildProgrammeFilters(plain, '/contoso.com')).toEqual([]);
    expect(buildNavigation(plain, 'contoso.com', '', 'cis-hardening')).toEqual(
      buildNavigation(plain, 'contoso.com'),
    );
  });

  it('recognises only the values this index can serve', () => {
    expect(isKnownProgramme(index, '')).toBe(true);
    expect(isKnownProgramme(index, 'vpn')).toBe(true);
    expect(isKnownProgramme(index, UNCATEGORISED)).toBe(true);
    expect(isKnownProgramme(index, 'nope')).toBe(false);
    // Membership a resource carries but the registry does not declare is not a
    // filter value: the registry is the source of truth for what exists.
    expect(isKnownProgramme(index, 'defender')).toBe(false);
    const plain = parseTenantIndex(INDEX_YAML)!;
    expect(isKnownProgramme(plain, UNCATEGORISED)).toBe(false);
  });
});

describe('buildNavigation', () => {
  const index = parseTenantIndex(INDEX_YAML)!;
  const sections = buildNavigation(index, 'contoso.com');

  it('groups by resource type, sorted, with sorted items', () => {
    expect(sections.map((s) => s.key)).toEqual([
      'Microsoft.Graph/assignmentFilters',
      'Microsoft.Graph/deviceCompliancePolicies',
    ]);
    expect(sections[0].label).toBe('Assignment Filters');
    expect(sections[1].items.map((i) => i.label)).toEqual([
      'macOS validation',
      'Windows OS validation',
    ]);
  });

  it('turns each doc path into an extensionless tenant route', () => {
    expect(sections[0].items[0].href).toBe(
      '/contoso.com/Microsoft.Graph/assignmentFilters/mac_intel',
    );
  });

  it('keeps pending resources visible so counts stay honest', () => {
    const pending = sections[1].items.find((i) => i.label === 'macOS validation');
    expect(pending).toBeDefined();
    expect(pending!.documented).toBe(false);
  });

  it('marks nothing active without a current document (the landing page)', () => {
    expect(sections.every((s) => !s.active)).toBe(true);
    expect(sections.every((s) => s.items.every((i) => !i.active))).toBe(true);
  });

  it('marks the current document and its section, with or without the .md suffix', () => {
    for (const active of [
      'Microsoft.Graph/deviceCompliancePolicies/win_os',
      'Microsoft.Graph/deviceCompliancePolicies/win_os.md',
    ]) {
      const withActive = buildNavigation(index, 'contoso.com', active);
      const compliance = withActive.find(
        (s) => s.key === 'Microsoft.Graph/deviceCompliancePolicies',
      )!;
      expect(compliance.active).toBe(true);
      expect(
        compliance.items.filter((i) => i.active).map((i) => i.label),
      ).toEqual(['Windows OS validation']);
      const filters = withActive.find(
        (s) => s.key === 'Microsoft.Graph/assignmentFilters',
      )!;
      expect(filters.active).toBe(false);
    }
  });

  it('summarises assignments as counts only', () => {
    const policy = sections[1].items.find(
      (i) => i.label === 'Windows OS validation',
    )!;
    expect(policy.badges).toContain('2 groups');
    expect(policy.badges).toContain('device');
  });
});

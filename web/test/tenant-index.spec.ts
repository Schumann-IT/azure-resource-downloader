import {
  buildFacetFilters,
  buildNavigation,
  countMatching,
  hasSelection,
  parseFacetSelection,
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

// A version-3 index as `docs generate-index` writes it with a multi-axis
// `taxonomy:`: the header `facets` registry (axes and values in display order,
// zero-count values kept) and a per-resource `facets` map carrying value ids
// only. The map's keys are serialised sorted, which is deliberately *not* the
// display order the header declares.
const FACETS_YAML = `version: 3
tenant: contoso.com
facets:
    - id: programme
      label: Programme
      values:
        - id: cis-hardening
          label: CIS hardening
          count: 2
        - id: defender
          label: Defender / MDE
          count: 1
        - id: vpn
          label: VPN
          count: 0
    - id: platform
      label: Platform
      values:
        - id: windows
          label: Windows
          count: 1
        - id: macos
          label: macOS
          count: 1
resources:
    - type: Microsoft.Graph/deviceCompliancePolicies
      doc: Microsoft.Graph/deviceCompliancePolicies/win_os.md
      displayName: Windows OS validation
      documented: true
      facets:
        platform:
            - windows
        programme:
            - cis-hardening
            - defender
    - type: Microsoft.Graph/deviceShellScripts
      doc: Microsoft.Graph/deviceShellScripts/mac_cis.md
      displayName: macOS CIS script
      documented: true
      facets:
        platform:
            - macos
        programme:
            - cis-hardening
    - type: Microsoft.Graph/assignmentFilters
      doc: Microsoft.Graph/assignmentFilters/mac_intel.md
      displayName: Intel Macs
      documented: true
`;

describe('parseTenantIndex (version 3 facet surface)', () => {
  const index = parseTenantIndex(FACETS_YAML)!;

  it('reads the axis registry in the header order, keeping zero counts', () => {
    expect(index.facets.map((f) => [f.id, f.label])).toEqual([
      ['programme', 'Programme'],
      ['platform', 'Platform'],
    ]);
    expect(index.facets[0].values.map((v) => [v.id, v.count])).toEqual([
      ['cis-hardening', 2],
      ['defender', 1],
      ['vpn', 0],
    ]);
  });

  it('reads per-resource membership as value ids only', () => {
    expect(index.resources[0].facets).toEqual({
      platform: ['windows'],
      programme: ['cis-hardening', 'defender'],
    });
    expect(index.resources[2].facets).toEqual({});
  });

  it('synthesises the programme axis from a version-2 index', () => {
    const legacy = parseTenantIndex(PROGRAMME_YAML)!;
    expect(legacy.facets.map((f) => f.id)).toEqual(['programme']);
    expect(legacy.facets[0].values.map((v) => [v.id, v.count])).toEqual([
      ['cis-hardening', 2],
      ['vpn', 0],
    ]);
    expect(legacy.resources[0].facets).toEqual({
      programme: ['cis-hardening', 'defender'],
    });
  });

  it('leaves an index with neither facets nor programmes unfilterable', () => {
    const plain = parseTenantIndex(INDEX_YAML)!;
    expect(plain.facets).toEqual([]);
    expect(plain.resources.every((r) => r.facets.programme === undefined)).toBe(
      true,
    );
  });
});

describe('facet filtering', () => {
  const index = parseTenantIndex(FACETS_YAML)!;
  const legacy = parseTenantIndex(PROGRAMME_YAML)!;

  it('narrows the tree to the selection, reading membership and deriving none', () => {
    const sections = buildNavigation(index, 'contoso.com', '', {
      programme: ['cis-hardening'],
    });
    // Sections keep their type ordering; only the membership decides who is in.
    expect(sections.flatMap((s) => s.items).map((i) => i.label)).toEqual([
      'Windows OS validation',
      'macOS CIS script',
    ]);
  });

  it('ORs several values on one axis', () => {
    const sections = buildNavigation(index, 'contoso.com', '', {
      programme: ['defender', 'vpn'],
    });
    expect(sections.flatMap((s) => s.items).map((i) => i.label)).toEqual([
      'Windows OS validation',
    ]);
  });

  it('ANDs across axes', () => {
    expect(
      buildNavigation(index, 'contoso.com', '', {
        programme: ['cis-hardening'],
        platform: ['macos'],
      }).flatMap((s) => s.items.map((i) => i.label)),
    ).toEqual(['macOS CIS script']);
    // A combination nothing satisfies is empty rather than falling back to one
    // of the axes.
    expect(
      buildNavigation(index, 'contoso.com', '', {
        programme: ['defender'],
        platform: ['macos'],
      }),
    ).toEqual([]);
  });

  it('keeps the whole selection in every document href so a click loses nothing', () => {
    const sections = buildNavigation(index, 'contoso.com', '', {
      platform: ['windows'],
      programme: ['cis-hardening'],
    });
    // Axes in the header's order, values sorted: one selection, one URL.
    expect(sections[0].items[0].href).toBe(
      '/contoso.com/Microsoft.Graph/deviceCompliancePolicies/win_os' +
        '?programme=cis-hardening&platform=windows',
    );
  });

  it('collects the resources an axis matched to nothing under its uncategorised bucket', () => {
    const sections = buildNavigation(index, 'contoso.com', '', {
      programme: [UNCATEGORISED],
    });
    expect(sections.flatMap((s) => s.items).map((i) => i.label)).toEqual([
      'Intel Macs',
    ]);
  });

  it('keeps the document being viewed even when the filter excludes it', () => {
    const sections = buildNavigation(
      index,
      'contoso.com',
      'Microsoft.Graph/assignmentFilters/mac_intel',
      { programme: ['cis-hardening'] },
    );
    const filters = sections.find(
      (s) => s.key === 'Microsoft.Graph/assignmentFilters',
    )!;
    expect(filters.active).toBe(true);
    expect(filters.items.map((i) => i.label)).toEqual(['Intel Macs']);
    // ... but it is not counted into the selection, or the number would stop
    // describing the filter and start describing the page. It is flagged
    // instead, so the view can explain why the tree is one longer than the
    // count rather than leaving the reader to reconcile the two.
    expect(countMatching(index, { programme: ['cis-hardening'] })).toBe(2);
    expect(filters.items[0].exempt).toBe(true);
    expect(
      sections
        .flatMap((s) => s.items)
        .filter((i) => i.exempt)
        .map((i) => i.label),
    ).toEqual(['Intel Macs']);
  });

  it('flags nothing as exempt when the document being viewed matches', () => {
    const sections = buildNavigation(
      index,
      'contoso.com',
      'Microsoft.Graph/deviceShellScripts/mac_cis',
      { programme: ['cis-hardening'] },
    );
    const items = sections.flatMap((s) => s.items);
    expect(items.map((i) => i.label)).toEqual([
      'Windows OS validation',
      'macOS CIS script',
    ]);
    expect(items.every((i) => i.exempt === false)).toBe(true);
    expect(countMatching(index, { programme: ['cis-hardening'] })).toBe(
      items.length,
    );
  });

  it('badges each resource with its membership, labels resolved from the header', () => {
    const sections = buildNavigation(index, 'contoso.com');
    const policy = sections
      .flatMap((s) => s.items)
      .find((i) => i.label === 'Windows OS validation')!;
    expect(policy.badges).toEqual([
      'CIS hardening',
      'Defender / MDE',
      'Windows',
    ]);
  });

  it('falls back to a version-2 resource label for a value the registry omits', () => {
    const policy = buildNavigation(legacy, 'contoso.com')
      .flatMap((s) => s.items)
      .find((i) => i.label === 'Windows OS validation')!;
    expect(policy.badges.slice(0, 2)).toEqual([
      'CIS hardening',
      'Defender / MDE',
    ]);
  });

  it('offers every axis, with All, the declared values in order, and uncategorised', () => {
    const filters = buildFacetFilters(index, '/contoso.com');
    expect(filters.map((f) => [f.id, f.label])).toEqual([
      ['programme', 'Programme'],
      ['platform', 'Platform'],
    ]);
    expect(filters[0].values.map((v) => [v.id, v.count])).toEqual([
      ['', 3],
      ['cis-hardening', 2],
      ['defender', 1],
      // Unfiltered, a value that matched nothing here is still offered: "empty
      // in this tenant" is information the registry carries on purpose.
      ['vpn', 0],
      [UNCATEGORISED, 1],
    ]);
    expect(filters[0].values[0].active).toBe(true);
    expect(filters[0].values[0].href).toBe('/contoso.com');
    expect(filters[0].values[1].href).toBe(
      '/contoso.com?programme=cis-hardening',
    );
    expect(filters[1].values[1].href).toBe('/contoso.com?platform=windows');
  });

  it('counts each axis with its own selection removed', () => {
    const filters = buildFacetFilters(index, '/contoso.com', {
      programme: ['defender'],
    });
    // Sibling values still show what picking them instead would give ...
    expect(filters[0].values.map((v) => [v.id, v.count])).toEqual([
      ['', 3],
      ['cis-hardening', 2],
      ['defender', 1],
      ['vpn', 0],
      [UNCATEGORISED, 1],
    ]);
    // ... while the other axis reacts to the selection.
    expect(filters[1].values.map((v) => [v.id, v.count])).toEqual([
      ['', 1],
      ['windows', 1],
    ]);
  });

  it('drops a value another axis has emptied, keeping the selected one visible', () => {
    const filters = buildFacetFilters(index, '/contoso.com', {
      platform: ['macos'],
      programme: ['defender'],
    });
    // `defender` is a dead end under platform=macos, but it is the active
    // choice, so it stays selectable to be undone; `vpn` and the uncategorised
    // bucket are dropped as dead ends.
    expect(filters[0].values.map((v) => [v.id, v.count])).toEqual([
      ['', 1],
      ['cis-hardening', 1],
      ['defender', 0],
    ]);
    expect(filters[0].values[2].active).toBe(true);
  });

  it('toggles one value per chip and keeps the rest of the selection', () => {
    const selection = { programme: ['cis-hardening'], platform: ['windows'] };
    const filters = buildFacetFilters(index, '/contoso.com', selection);
    const chip = (axis: number, id: string) =>
      filters[axis].values.find((v) => v.id === id)!;
    // An active chip links to the selection without it, the other axis intact.
    expect(chip(0, 'cis-hardening').href).toBe('/contoso.com?platform=windows');
    // An inactive chip adds itself to the selection instead of replacing it.
    expect(chip(0, 'defender').href).toBe(
      '/contoso.com?programme=cis-hardening&programme=defender&platform=windows',
    );
    // The per-axis "All" chip clears just that axis.
    expect(chip(1, '').href).toBe('/contoso.com?programme=cis-hardening');
  });

  it('hangs the chooser off the current page', () => {
    const base = '/contoso.com/Microsoft.Graph/deviceShellScripts/mac_cis';
    const filters = buildFacetFilters(index, base, {
      programme: ['cis-hardening'],
    });
    expect(
      filters[0].values.filter((v) => v.active).map((v) => v.id),
    ).toEqual(['cis-hardening']);
    expect(filters[0].values[2].href).toBe(
      `${base}?programme=cis-hardening&programme=defender`,
    );
  });

  it('counts distinct resources, never the sum of the value counts', () => {
    // Membership is many-to-many: the programme values sum to 3 across 2
    // resources, so a total must be counted over the resource list.
    const summed = index.facets[0].values.reduce((n, v) => n + v.count, 0);
    expect(summed).toBe(3);
    expect(countMatching(index, { programme: ['cis-hardening', 'defender'] })).toBe(
      2,
    );
    expect(countMatching(index)).toBe(3);
  });

  it('offers no filter at all for an index without a taxonomy', () => {
    const plain = parseTenantIndex(INDEX_YAML)!;
    expect(buildFacetFilters(plain, '/contoso.com')).toEqual([]);
    expect(
      buildNavigation(plain, 'contoso.com', '', { programme: ['cis-hardening'] }),
    ).toEqual(buildNavigation(plain, 'contoso.com'));
  });

  it('keeps a version-2 index working through the synthesised axis', () => {
    const filters = buildFacetFilters(legacy, '/contoso.com');
    expect(filters.map((f) => f.label)).toEqual(['Programme']);
    expect(filters[0].values.map((v) => [v.id, v.count])).toEqual([
      ['', 3],
      ['cis-hardening', 2],
      ['vpn', 0],
      [UNCATEGORISED, 1],
    ]);
    expect(filters[0].values[1].href).toBe('/contoso.com?programme=cis-hardening');
  });

  it('reads a repeated query parameter, dropping values it cannot serve', () => {
    expect(
      parseFacetSelection(index, {
        programme: ['defender', 'cis-hardening', 'defender', 'nope'],
        platform: 'windows',
      }),
    ).toEqual({
      // De-duplicated and sorted, so one selection has one URL.
      programme: ['cis-hardening', 'defender'],
      platform: ['windows'],
    });
    // Membership a resource carries but the header does not declare is not a
    // filter value: the registry is the source of truth for what exists.
    expect(parseFacetSelection(legacy, { programme: 'defender' })).toEqual({});
    expect(parseFacetSelection(index, { programme: '' })).toEqual({});
    expect(parseFacetSelection(index, { nosuchaxis: 'x' })).toEqual({});
    expect(parseFacetSelection(index, { programme: UNCATEGORISED })).toEqual({
      programme: [UNCATEGORISED],
    });
    const plain = parseTenantIndex(INDEX_YAML)!;
    expect(parseFacetSelection(plain, { programme: UNCATEGORISED })).toEqual({});
  });

  it('reports whether anything is filtering at all', () => {
    expect(hasSelection({})).toBe(false);
    expect(hasSelection({ programme: [] })).toBe(false);
    expect(hasSelection({ programme: ['vpn'] })).toBe(true);
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

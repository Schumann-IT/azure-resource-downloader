import { buildNavigation, parseTenantIndex } from '../src/docs/tenant-index';

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

  it('rejects anything that is not a version-1 index (folder is then not a tenant)', () => {
    expect(parseTenantIndex('version: [oops\n')).toBeUndefined();
    expect(parseTenantIndex('')).toBeUndefined();
    expect(parseTenantIndex('- a\n- b\n')).toBeUndefined();
    expect(parseTenantIndex('version: 2\ntenant: x\n')).toBeUndefined();
    expect(parseTenantIndex('version: 1\n')).toBeUndefined();
  });

  it('tolerates a missing resources list and unexpected field types', () => {
    const index = parseTenantIndex('version: 1\ntenant: x\nresources: nope\n');
    expect(index).toBeDefined();
    expect(index!.resources).toEqual([]);
    expect(index!.counts).toEqual({ documented: 0, pending: 0, excluded: [] });
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

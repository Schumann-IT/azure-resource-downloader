import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { buildOutputIndex, buildTenantIndexFromDisk } from '../src/index-builder';
import { listTenants, walkTenant } from '../src/walk';

let root: string;

beforeAll(() => {
  root = mkdtempSync(join(tmpdir(), 'ard-walk-'));
  const typeDir = join(root, 'cb-gmbh.com', 'Microsoft.Graph', 'deviceCompliancePolicies');
  mkdirSync(typeDir, { recursive: true });
  writeFileSync(join(typeDir, 'doc-prompt.md'), '# Documentation prompt\n');
  writeFileSync(
    join(typeDir, 'gbl_c_prd_d_mac_os.yaml'),
    [
      "'@odata.type': '#microsoft.graph.macOSCompliancePolicy'",
      'id: 11111111-1111-1111-1111-111111111111',
      'displayName: GBL Mac OS',
      "lastModifiedDateTime: '2026-05-01T00:00:00Z'",
      'version: 3',
    ].join('\n'),
  );
  // A second resource that also has a generated doc.
  writeFileSync(join(typeDir, 'gbl_c_prd_d_win.yaml'), 'id: 22222222\ndisplayName: GBL Win\n');
  writeFileSync(join(typeDir, 'gbl_c_prd_d_win.doc.md'), '# Generated doc\n');

  // An ARM provider folder too.
  const arm = join(root, 'cb-gmbh.com', 'Microsoft.Resources', 'resourceGroups');
  mkdirSync(arm, { recursive: true });
  writeFileSync(join(arm, 'rg_prod.yaml'), 'id: /subscriptions/x/rg_prod\nname: rg-prod\n');
});

afterAll(() => {
  rmSync(root, { recursive: true, force: true });
});

describe('walk', () => {
  it('lists tenants', () => {
    expect(listTenants(root)).toEqual(['cb-gmbh.com']);
  });

  it('walks a tenant into provider/type/resource nodes', () => {
    const tree = walkTenant(root, 'cb-gmbh.com');
    expect(tree.providers.map((p) => p.provider)).toEqual([
      'Microsoft.Graph',
      'Microsoft.Resources',
    ]);
    const graph = tree.providers[0];
    const type = graph.resourceTypes[0];
    expect(type.resourceType).toBe('deviceCompliancePolicies');
    expect(type.docPromptPath).toMatch(/doc-prompt\.md$/);
    expect(type.resources).toHaveLength(2);
  });

  it('extracts identity metadata and excludes doc-prompt.md as a resource', () => {
    const tree = walkTenant(root, 'cb-gmbh.com');
    const resources = tree.providers[0].resourceTypes[0].resources;
    const mac = resources.find((r) => r.slug === 'gbl_c_prd_d_mac_os');
    expect(mac?.displayName).toBe('GBL Mac OS');
    expect(mac?.version).toBe(3);
    expect(mac?.hasDoc).toBe(false);
    const win = resources.find((r) => r.slug === 'gbl_c_prd_d_win');
    expect(win?.hasDoc).toBe(true);
    expect(win?.docPath).toMatch(/gbl_c_prd_d_win\.doc\.md$/);
  });
});

describe('index builder', () => {
  it('builds a per-tenant index with counts', () => {
    const idx = buildTenantIndexFromDisk(root, 'cb-gmbh.com');
    expect(idx.totalResources).toBe(3);
    const graph = idx.providers.find((p) => p.provider === 'Microsoft.Graph');
    expect(graph?.resourceTypes[0].count).toBe(2);
  });

  it('builds a top-level index over all tenants', () => {
    const out = buildOutputIndex(root);
    expect(out.tenants).toHaveLength(1);
    expect(out.tenants[0].tenant).toBe('cb-gmbh.com');
    expect(typeof out.generatedAt).toBe('string');
  });
});

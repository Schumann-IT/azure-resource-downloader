import { promises as fsp } from 'fs';
import * as os from 'os';
import * as path from 'path';
import { Test } from '@nestjs/testing';
import { NestExpressApplication } from '@nestjs/platform-express';
import request from 'supertest';
import { AppModule } from '../src/app.module';
import { configureViews } from '../src/configure-app';

// Reproduces, as automated tests, the manual endpoint checks: discovery via
// docs/index.yaml, picker, the index-driven tenant landing page, a nested
// settings-catalog doc with <details> + cross-type ../groups link rewrite,
// group page, 404, path traversal, the agent prompt not being served, and
// no-restart refresh. Runs against a self-contained fixture tenant so it does
// not depend on the real output/ export.

const INDEX_YAML = `version: 1
tenant: My Tenant
generatedAt: "2026-01-01T00:00:00Z"
complete: true
counts:
    documented: 2
    pending: 1
    excluded:
        Microsoft.Graph/windowsAutopilotDeviceIdentities: 4
resources:
    - type: Microsoft.Graph/deviceManagementConfigurationPolicies
      doc: Microsoft.Graph/deviceManagementConfigurationPolicies/p1.md
      displayName: Policy One
      summary: A firewall policy.
      documented: true
      assignments:
        groups: 1
    - type: Microsoft.Graph/groups
      doc: Microsoft.Graph/groups/g1.md
      displayName: Admins
      documented: true
    - type: Microsoft.Graph/deviceCompliancePolicies
      doc: Microsoft.Graph/deviceCompliancePolicies/c1.md
      displayName: Compliance One
      documented: false
`;

const POLICY_MD = `---
source: p1.yaml
sourceSha256: 845ddb
promptSha256: 04cbf6
generatedAt: 2026-01-01T00:00:00Z
---

# Policy One

A firewall policy.

<details>
<summary><code>firewall/enabled</code></summary>

value: true

<details>
<summary>nested child</summary>

deep value

</details>

</details>

Assigned to [Admins](../groups/g1.md).
`;

const GROUP_MD = `---
source: g1.yaml
sourceSha256: ee11
promptSha256: 04cbf6
generatedAt: 2026-01-01T00:00:00Z
---

# Admins

An assigned security group.
`;

describe('Docs browser (e2e)', () => {
  let app: NestExpressApplication;
  let root: string;
  let exportDir: string;
  let tenantDir: string;
  let policyFile: string;

  beforeAll(async () => {
    root = await fsp.mkdtemp(path.join(os.tmpdir(), 'docsroot-'));
    exportDir = path.join(root, 'mytenant');
    tenantDir = path.join(exportDir, 'docs');

    const policyDir = path.join(
      tenantDir,
      'Microsoft.Graph',
      'deviceManagementConfigurationPolicies',
    );
    const groupsDir = path.join(tenantDir, 'Microsoft.Graph', 'groups');
    await fsp.mkdir(policyDir, { recursive: true });
    await fsp.mkdir(groupsDir, { recursive: true });

    await fsp.writeFile(path.join(tenantDir, 'index.yaml'), INDEX_YAML);
    // The agent prompt lives next to the index and must never be served.
    await fsp.writeFile(path.join(tenantDir, 'generate.md'), '# agent prompt');
    policyFile = path.join(policyDir, 'p1.md');
    await fsp.writeFile(policyFile, POLICY_MD);
    await fsp.writeFile(path.join(groupsDir, 'g1.md'), GROUP_MD);

    // A housekeeping directory that itself looks like an export: it must NOT be
    // surfaced as a second tenant (discovery stops at the matched tenant and
    // skips `_`-prefixed dirs).
    const trash = path.join(exportDir, '_to_delete', 'docs');
    await fsp.mkdir(trash, { recursive: true });
    await fsp.writeFile(path.join(trash, 'index.yaml'), INDEX_YAML);

    // An export whose docs/ folder has no index.yaml is not a tenant.
    const halfDir = path.join(root, 'half-baked', 'docs');
    await fsp.mkdir(halfDir, { recursive: true });
    await fsp.writeFile(path.join(halfDir, 'stray.md'), '# half');

    // A docs/index.yaml the parser rejects makes the folder *not* a tenant
    // instead of crashing discovery.
    const brokenDir = path.join(root, 'broken', 'docs');
    await fsp.mkdir(brokenDir, { recursive: true });
    await fsp.writeFile(path.join(brokenDir, 'index.yaml'), 'version: [oops\n');

    process.env.DOCS_ROOT = root;

    const moduleRef = await Test.createTestingModule({
      imports: [AppModule],
    }).compile();
    app = moduleRef.createNestApplication<NestExpressApplication>();
    configureViews(app);
    await app.init();
  });

  afterAll(async () => {
    await app?.close();
    await fsp.rm(root, { recursive: true, force: true });
  });

  it('GET /healthz reports one tenant with the index counts', async () => {
    const res = await request(app.getHttpServer()).get('/healthz').expect(200);
    expect(res.body).toEqual({
      status: 'ok',
      tenants: 1,
      documents: 2,
      pending: 1,
    });
  });

  it('GET / lists exactly the real tenant with its index counts', async () => {
    const res = await request(app.getHttpServer()).get('/').expect(200);
    expect(res.text).toContain('mytenant');
    expect(res.text).toContain('2 documented');
    expect(res.text).toContain('1 pending');
    // The housekeeping / marker-less / malformed folders are not tenants.
    expect(res.text).not.toContain('_to_delete');
    expect(res.text).not.toContain('half-baked');
    expect(res.text).not.toContain('broken');
  });

  it('GET /:tenant renders the index-driven landing page with routes per document', async () => {
    const res = await request(app.getHttpServer())
      .get('/mytenant')
      .expect(200);
    expect(res.text).toContain('My Tenant');
    expect(res.text).toContain('Policy One');
    expect(res.text).toContain('A firewall policy.');
    expect(res.text).toContain(
      'href="/mytenant/Microsoft.Graph/deviceManagementConfigurationPolicies/p1"',
    );
    // A not-yet-documented resource is listed as pending, honestly.
    expect(res.text).toContain('Compliance One');
    expect(res.text).toContain('pending');
    // Excluded bulk types are reported as counts only, never listed.
    expect(res.text).toContain(
      'Microsoft.Graph/windowsAutopilotDeviceIdentities',
    );
  });

  it('does not serve the agent prompt (docs/generate.md)', async () => {
    await request(app.getHttpServer()).get('/mytenant/generate').expect(404);
    await request(app.getHttpServer()).get('/mytenant/generate.md').expect(404);
  });

  it('reflects a regenerated index.yaml on the next request without a restart', async () => {
    await fsp.writeFile(
      path.join(tenantDir, 'index.yaml'),
      INDEX_YAML.replace('displayName: Policy One', 'displayName: Policy Renamed'),
    );
    const res = await request(app.getHttpServer())
      .get('/mytenant')
      .expect(200);
    expect(res.text).toContain('Policy Renamed');
    await fsp.writeFile(path.join(tenantDir, 'index.yaml'), INDEX_YAML);
  });

  it('renders a settings-catalog doc: <details> pass through, ../groups link rewritten, frontmatter stripped', async () => {
    const res = await request(app.getHttpServer())
      .get(
        '/mytenant/Microsoft.Graph/deviceManagementConfigurationPolicies/p1',
      )
      .expect(200);
    // Raw HTML <details> blocks survive (html: true), nested included.
    expect((res.text.match(/<details>/g) || []).length).toBeGreaterThanOrEqual(
      2,
    );
    // Cross-type ../groups/g1.md resolved to an absolute app route.
    expect(res.text).toContain(
      'href="/mytenant/Microsoft.Graph/groups/g1"',
    );
    // Frontmatter is surfaced as metadata (source) but never rendered as body.
    expect(res.text).toContain('p1.yaml');
    expect(res.text).not.toContain('promptSha256');
  });

  it('the rewritten cross-type link target resolves (group page loads)', async () => {
    const res = await request(app.getHttpServer())
      .get('/mytenant/Microsoft.Graph/groups/g1')
      .expect(200);
    expect(res.text).toContain('Admins');
  });

  it('GET an unknown document returns 404 without leaking a filesystem path', async () => {
    const res = await request(app.getHttpServer())
      .get('/mytenant/does/not/exist')
      .expect(404);
    expect(res.text).toContain('404');
    expect(res.text).not.toContain(root); // no absolute path leaked
  });

  it('GET an unknown tenant returns 404', async () => {
    await request(app.getHttpServer()).get('/nope-tenant').expect(404);
    // The skipped housekeeping dir is not routable as a tenant either.
    await request(app.getHttpServer()).get('/_to_delete').expect(404);
  });

  it('rejects path traversal (encoded) with a 404', async () => {
    await request(app.getHttpServer())
      .get('/mytenant/..%2f..%2f..%2fetc%2fpasswd')
      .expect(404);
  });

  it('reflects an edited document on the next request without a restart', async () => {
    const marker = `PROBE_${Date.now()}`;
    await fsp.appendFile(policyFile, `\n\n${marker}\n`);
    const res = await request(app.getHttpServer())
      .get(
        '/mytenant/Microsoft.Graph/deviceManagementConfigurationPolicies/p1',
      )
      .expect(200);
    expect(res.text).toContain(marker);
  });
});

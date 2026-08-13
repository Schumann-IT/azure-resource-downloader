import { promises as fsp } from 'fs';
import * as os from 'os';
import * as path from 'path';
import { Test } from '@nestjs/testing';
import { NestExpressApplication } from '@nestjs/platform-express';
import request from 'supertest';
import { AppModule } from '../src/app.module';
import { configureViews } from '../src/configure-app';

// Reproduces, as automated tests, the manual endpoint checks: discovery +
// computed count, picker, index render + link rewrite, nested settings-catalog
// doc with <details> + cross-type ../groups link rewrite, group page, 404,
// path traversal, and no-restart refresh. Runs against a self-contained fixture
// tenant so it does not depend on the real output/ export.

const MANIFEST = JSON.stringify({
  version: 2,
  tenant: 'My Tenant',
  generatedAt: '2026-01-01T00:00:00Z',
  types: {
    'Microsoft.Graph/deviceManagementConfigurationPolicies': {
      promptSha256: 'p',
      resources: { 'p1.yaml': { sha256: 'a', doc: 'p1.md' } },
    },
    'Microsoft.Graph/groups': {
      promptSha256: 'p',
      resources: { 'g1.yaml': { sha256: 'b', doc: 'g1.md' } },
    },
    'Microsoft.Graph/deviceCompliancePolicies': {
      promptSha256: 'p',
      resources: { 'c1.yaml': { sha256: 'c', doc: 'c1.md' } },
    },
  },
});

const INDEX_MD = `# My Tenant — configuration

Intro paragraph.

- [Policy One](Microsoft.Graph/deviceManagementConfigurationPolicies/p1.md) — a policy
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
  let tenantDir: string;
  let policyFile: string;

  beforeAll(async () => {
    root = await fsp.mkdtemp(path.join(os.tmpdir(), 'docsroot-'));
    tenantDir = path.join(root, 'mytenant');

    const policyDir = path.join(
      tenantDir,
      'Microsoft.Graph',
      'deviceManagementConfigurationPolicies',
    );
    const groupsDir = path.join(tenantDir, 'Microsoft.Graph', 'groups');
    await fsp.mkdir(policyDir, { recursive: true });
    await fsp.mkdir(groupsDir, { recursive: true });

    await fsp.writeFile(path.join(tenantDir, '.doc-manifest.json'), MANIFEST);
    await fsp.writeFile(path.join(tenantDir, 'index.md'), INDEX_MD);
    policyFile = path.join(policyDir, 'p1.md');
    await fsp.writeFile(policyFile, POLICY_MD);
    await fsp.writeFile(path.join(groupsDir, 'g1.md'), GROUP_MD);

    // A housekeeping directory that itself contains marker files: it must NOT
    // be surfaced as a second tenant (discovery stops at the matched tenant and
    // skips `_`-prefixed dirs).
    const trash = path.join(tenantDir, '_to_delete');
    await fsp.mkdir(trash, { recursive: true });
    await fsp.writeFile(path.join(trash, '.doc-manifest.json'), MANIFEST);
    await fsp.writeFile(path.join(trash, 'index.md'), '# trash');

    // A folder with only index.md (no manifest) is not a tenant.
    const halfDir = path.join(root, 'half-baked');
    await fsp.mkdir(halfDir, { recursive: true });
    await fsp.writeFile(path.join(halfDir, 'index.md'), '# half');

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

  it('GET /healthz reports one tenant and the computed document count (3)', async () => {
    const res = await request(app.getHttpServer()).get('/healthz').expect(200);
    expect(res.body).toEqual({ status: 'ok', tenants: 1, documents: 3 });
  });

  it('GET / lists exactly the real tenant with its resource count', async () => {
    const res = await request(app.getHttpServer()).get('/').expect(200);
    expect(res.text).toContain('mytenant');
    expect(res.text).toContain('3 resources');
    // The housekeeping / half-baked folders are not tenants.
    expect(res.text).not.toContain('_to_delete');
    expect(res.text).not.toContain('half-baked');
  });

  it('GET /:tenant renders index.md and rewrites relative .md links to routes', async () => {
    const res = await request(app.getHttpServer())
      .get('/mytenant')
      .expect(200);
    expect(res.text).toContain('My Tenant — configuration');
    expect(res.text).toContain(
      'href="/mytenant/Microsoft.Graph/deviceManagementConfigurationPolicies/p1"',
    );
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

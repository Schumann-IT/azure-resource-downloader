import { createHash } from 'crypto';
import { promises as fsp } from 'fs';
import * as os from 'os';
import * as path from 'path';
import { Test } from '@nestjs/testing';
import { NestExpressApplication } from '@nestjs/platform-express';
import request from 'supertest';
import * as yauzl from 'yauzl';
import { AppModule } from '../src/app.module';
import { configureViews } from '../src/configure-app';
import { OVERVIEW_FILE } from '../src/docs/export/confluence';

// Reproduces, as automated tests, the manual endpoint checks: discovery via
// docs/index.yaml, picker, the summary-driven tenant landing page and its
// index fallback, the sidebar navigation, a nested settings-catalog doc with
// <details> + cross-type ../groups link rewrite, group page, 404, path
// traversal, the agent prompt not being served, and no-restart refresh. Runs
// against self-contained fixture tenants so it does not depend on the real
// output/ export.

// A version-3 index, as `docs generate-index` writes it with a multi-axis
// `taxonomy:`: the header `facets` registry (two axes, a zero-count value kept)
// and a per-resource `facets` map of value ids, with the transitional
// `programmes`/`groups` mirrors the CLI still emits beside them.
const INDEX_YAML = `version: 3
tenant: My Tenant
generatedAt: "2026-01-01T00:00:00Z"
complete: true
vocabularies:
    platform: [Windows, macOS, n/a]
    function: [Compliance, Security]
programmes:
    - id: firewall
      label: Firewall
      count: 1
    - id: vpn
      label: VPN
      count: 0
    - id: hardening
      label: Hardening
      count: 1
facets:
    - id: programme
      label: Programme
      values:
        - id: firewall
          label: Firewall
          count: 1
        - id: vpn
          label: VPN
          count: 0
        - id: hardening
          label: Hardening
          count: 1
    - id: platform
      label: Platform
      values:
        - id: windows
          label: Windows
          count: 1
        - id: macos
          label: macOS
          count: 1
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
      groups:
        - id: firewall
          label: Firewall
        - id: hardening
          label: Hardening
      facets:
        platform:
            - windows
        programme:
            - firewall
            - hardening
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
      facets:
        platform:
            - macos
`;

// The same tenant as an older CLI wrote it: version 2, the single programme axis
// expressed through `programmes` + per-resource `groups` and no `facets` at all.
// The filter must still work, from the synthesised axis.
const LEGACY_INDEX_YAML = `version: 2
tenant: My Tenant
generatedAt: "2026-01-01T00:00:00Z"
complete: true
programmes:
    - id: firewall
      label: Firewall
      count: 1
    - id: vpn
      label: VPN
      count: 0
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
      groups:
        - id: firewall
          label: Firewall
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
source: resources/Microsoft.Graph/deviceManagementConfigurationPolicies/p1.yaml
sourceSha256: 845ddb
promptSha256: 04cbf6
generatedAt: 2026-01-01T00:00:00Z
---

# Policy One

\`p1.yaml\`

A firewall policy, unlike \`other_policy.yaml\` which is stricter.

| Field | Value |
|---|---|
| Resource type | Microsoft.Graph/deviceManagementConfigurationPolicies |

<!-- assignments:start -->

| Direction | Target |
|---|---|
| include | 11111111-2222-3333-4444-555555555555 |

<!-- assignments:end -->

## References

Assigned to [Admins](../groups/g1.md).

<!-- used-by:start -->

## Used by

Spliced into the middle of a section by the CLI.

<!-- used-by:end -->

## Lifecycle & operations

The older spelling of the heading, as it still sits on disk.

## Metadata

A heading the current contract does not declare.

## Settings

<details data-setting="settings[0].value" data-note="security">
<summary><code>firewall/enabled</code></summary>

value: true

<details data-setting="settings[0].children[0].value" data-note="inert">
<summary>nested child</summary>

deep value

</details>

</details>
`;

// The tenant-wide management summary the generation agent writes at the docs
// root: its own H1, no frontmatter, links relative to docs/.
const SUMMARY_MD = `# My Tenant — Intune and Entra configuration

## Management summary

A large, consistently named Intune estate.

The firewall baseline is
[Policy One](Microsoft.Graph/deviceManagementConfigurationPolicies/p1.md).

### Findings

| Severity | Finding | Affected | Documents |
|---|---|---|---|
| critical | Two credentials sit in the configuration in cleartext. | 2 | [Policy One](Microsoft.Graph/deviceManagementConfigurationPolicies/p1.md) |
| medium | Six resources are configured but targeted at nothing. | 6 | — |
| nonsense | An unrecognised severity must stay plain text. | 1 | — |

### Recommendations

1. Rotate both credentials.
`;

// A document the generation agent wrote without frontmatter: it has no known
// source, so the top-bar switcher must not offer a YAML view for it.
const COMPLIANCE_MD = `# Compliance One

No frontmatter, so no source YAML is claimed.
`;

// The exported source YAML behind p1.md, mirroring docs/<type>/<name>.md as
// resources/<type>/<name>.yaml.
const POLICY_YAML = `id: 11111111-2222-3333-4444-555555555555\nname: Policy One\nsettings:\n  firewall:\n    enabled: true\n`;

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
  let policyYamlFile: string;
  let summaryFile: string;

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
    summaryFile = path.join(tenantDir, 'summary.md');
    await fsp.writeFile(summaryFile, SUMMARY_MD);
    // The agent prompt lives next to the index and must never be served.
    await fsp.writeFile(path.join(tenantDir, 'generate.md'), '# agent prompt');
    policyFile = path.join(policyDir, 'p1.md');
    await fsp.writeFile(policyFile, POLICY_MD);
    await fsp.writeFile(path.join(groupsDir, 'g1.md'), GROUP_MD);

    const complianceDir = path.join(
      tenantDir,
      'Microsoft.Graph',
      'deviceCompliancePolicies',
    );
    await fsp.mkdir(complianceDir, { recursive: true });
    await fsp.writeFile(path.join(complianceDir, 'c1.md'), COMPLIANCE_MD);

    // The second served root: <export>/resources, a sibling of docs/.
    const policyResourceDir = path.join(
      exportDir,
      'resources',
      'Microsoft.Graph',
      'deviceManagementConfigurationPolicies',
    );
    await fsp.mkdir(policyResourceDir, { recursive: true });
    policyYamlFile = path.join(policyResourceDir, 'p1.yaml');
    await fsp.writeFile(policyYamlFile, POLICY_YAML);
    // A Markdown file inside resources/ must not become reachable.
    await fsp.writeFile(
      path.join(policyResourceDir, 'p1.md'),
      '# not a resource',
    );

    // A housekeeping directory that itself looks like an export: it must NOT be
    // surfaced as a second tenant (discovery stops at the matched tenant and
    // skips `_`-prefixed dirs).
    const trash = path.join(exportDir, '_to_delete', 'docs');
    await fsp.mkdir(trash, { recursive: true });
    await fsp.writeFile(path.join(trash, 'index.yaml'), INDEX_YAML);

    // A tenant whose export carries no summary.md: the landing page must fall
    // back to the index listing rather than 404. Its index is also the version-2
    // one, so the same tenant doubles as the degradation case.
    const bareDir = path.join(root, 'nosummary', 'docs');
    await fsp.mkdir(bareDir, { recursive: true });
    await fsp.writeFile(path.join(bareDir, 'index.yaml'), LEGACY_INDEX_YAML);

    // An export whose docs/ folder has no index.yaml is not a tenant.
    const halfDir = path.join(root, 'half-baked', 'docs');
    await fsp.mkdir(halfDir, { recursive: true });
    await fsp.writeFile(path.join(halfDir, 'stray.md'), '# half');

    // An index from a newer CLI: a later schema version and fields this build
    // knows nothing about must still discover as a tenant, because the index is
    // the tenant marker — refusing it would hide the export entirely.
    const futureDir = path.join(root, 'future', 'docs');
    await fsp.mkdir(futureDir, { recursive: true });
    await fsp.writeFile(
      path.join(futureDir, 'index.yaml'),
      'version: 3\ntenant: Future Tenant\nsomethingNew: [a]\nresources: []\n',
    );

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

  it('GET /healthz reports the discovered tenants with the index counts', async () => {
    const res = await request(app.getHttpServer()).get('/healthz').expect(200);
    expect(res.body).toEqual({
      status: 'ok',
      rootReadable: true,
      tenants: 3,
      documents: 4,
      pending: 2,
    });
  });

  it('every response carries the baseline security headers', async () => {
    const expectSecurityHeaders = (res: request.Response): void => {
      expect(res.headers['content-security-policy']).toBe(
        "default-src 'none'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; frame-ancestors 'none'; base-uri 'none'",
      );
      expect(res.headers['x-content-type-options']).toBe('nosniff');
      expect(res.headers['referrer-policy']).toBe('same-origin');
      expect(res.headers['x-frame-options']).toBe('DENY');
    };

    expectSecurityHeaders(await request(app.getHttpServer()).get('/').expect(200));
    expectSecurityHeaders(
      await request(app.getHttpServer()).get('/healthz').expect(200),
    );
    // Static assets: proves the middleware runs before express.static.
    expectSecurityHeaders(
      await request(app.getHttpServer()).get('/favicon.svg').expect(200),
    );
    expectSecurityHeaders(
      await request(app.getHttpServer())
        .get('/mytenant/Microsoft.Graph/groups/g1')
        .expect(200),
    );
    expectSecurityHeaders(
      await request(app.getHttpServer())
        .get(
          '/mytenant/_resource/Microsoft.Graph/deviceManagementConfigurationPolicies/p1',
        )
        .expect(200),
    );
    expectSecurityHeaders(
      await request(app.getHttpServer())
        .get(
          '/mytenant/_resource/Microsoft.Graph/deviceManagementConfigurationPolicies/p1?raw',
        )
        .expect(200),
    );
    expectSecurityHeaders(
      await request(app.getHttpServer())
        .get('/mytenant/_export/confluence')
        .buffer()
        .parse(binaryParser)
        .expect(200),
    );
    expectSecurityHeaders(
      await request(app.getHttpServer()).get('/nope-tenant').expect(404),
    );
  });

  it('serves a favicon and keeps /favicon.ico out of the tenant route', async () => {
    // The static icon every HTML page links to.
    const svg = await request(app.getHttpServer())
      .get('/favicon.svg')
      .expect(200);
    expect(svg.headers['content-type']).toMatch(/image\/svg\+xml/);

    // The browser's own /favicon.ico probe must not be treated as a tenant
    // named "favicon.ico" and answered with the 404 view.
    const ico = await request(app.getHttpServer())
      .get('/favicon.ico')
      .expect(301);
    expect(ico.headers['location']).toBe('/favicon.svg');
    expect(ico.text).not.toContain('Document not found');

    // Every rendered page declares the icon so the probe is not made at all.
    for (const route of [
      '/',
      '/mytenant',
      '/mytenant/Microsoft.Graph/groups/g1',
      '/mytenant/_resource/Microsoft.Graph/deviceManagementConfigurationPolicies/p1',
      '/no-such-tenant',
    ]) {
      const res = await request(app.getHttpServer()).get(route);
      expect(res.text).toContain(
        '<link rel="icon" type="image/svg+xml" href="/favicon.svg" />',
      );
      // The shared <head> declares both schemes so the UA chrome follows the
      // page instead of staying light around a dark one.
      expect(res.text).toContain(
        '<meta name="color-scheme" content="light dark" />',
      );
    }
  });

  it('names the tenant in the page title, and the YAML view apart from the document', async () => {
    const doc = await request(app.getHttpServer())
      .get('/mytenant/Microsoft.Graph/deviceManagementConfigurationPolicies/p1')
      .expect(200);
    // The document's own H1, then the tenant as the index names it.
    expect(doc.text).toContain('<title>Policy One · My Tenant</title>');

    // Both representations of one resource are routinely open at once, so their
    // tabs must not read the same.
    const yaml = await request(app.getHttpServer())
      .get(
        '/mytenant/_resource/Microsoft.Graph/deviceManagementConfigurationPolicies/p1',
      )
      .expect(200);
    expect(yaml.text).toContain('<title>p1.yaml · My Tenant</title>');

    // The picker is not inside a tenant, so it keeps its own title.
    const picker = await request(app.getHttpServer()).get('/').expect(200);
    expect(picker.text).toContain('<title>Documentation</title>');
  });

  it('GET / lists exactly the real tenants with their index counts', async () => {
    const res = await request(app.getHttpServer()).get('/').expect(200);
    expect(res.text).toContain('mytenant');
    expect(res.text).toContain('2 documented');
    expect(res.text).toContain('1 pending');
    // A newer index schema still lists; only unparseable ones drop out.
    expect(res.text).toContain('future');
    // The housekeeping / marker-less / malformed folders are not tenants.
    expect(res.text).not.toContain('_to_delete');
    expect(res.text).not.toContain('half-baked');
    expect(res.text).not.toContain('broken');
  });

  it('GET /:tenant renders docs/summary.md as the landing body, links rewritten', async () => {
    const res = await request(app.getHttpServer())
      .get('/mytenant')
      .expect(200);
    expect(res.text).toContain('A large, consistently named Intune estate.');
    // Summary links are relative to docs/ and become tenant routes.
    expect(res.text).toContain(
      'href="/mytenant/Microsoft.Graph/deviceManagementConfigurationPolicies/p1"',
    );
    // The summary owns the page heading — the view adds none of its own.
    expect(res.text).not.toContain('listing the index instead');
    // Title first, then the export facts, then the summary's prose.
    const title = res.text.indexOf('My Tenant — Intune and Entra configuration</a></h1>');
    const facts = res.text.indexOf('class="export-summary');
    const body = res.text.indexOf('A large, consistently named Intune estate.');
    expect(title).toBeGreaterThan(-1);
    expect(facts).toBeGreaterThan(title);
    expect(body).toBeGreaterThan(facts);
    expect(res.text).toContain('Exported <strong>2026-01-01T00:00:00Z</strong>');
    expect(res.text).toContain('<span>2 documented</span>');
    expect(res.text).toContain('<span>1 pending</span>');
    expect(res.text).toContain('<span>4 excluded</span>');
    expect(res.text).not.toContain('Incomplete export');
  });

  it('GET /:tenant falls back to the index listing when there is no summary.md', async () => {
    const res = await request(app.getHttpServer())
      .get('/nosummary')
      .expect(200);
    expect(res.text).toContain('listing the index instead');
    expect(res.text).toContain('class="export-summary');
    expect(res.text).toContain('A firewall policy.');
    expect(res.text).toContain(
      'href="/nosummary/Microsoft.Graph/deviceManagementConfigurationPolicies/p1"',
    );
  });

  // Handlebars escapes `=` inside an attribute as `&#x3D;`, which the HTML parser
  // decodes back to `=`, so the rendered links work; the assertions have to match
  // the source as escaped.
  const q = (base: string, query: string) =>
    `href="${base}?${query.replace(/=/g, '&#x3D;').replace(/&(?!#x3D;)/g, '&amp;')}"`;

  const programme = (base: string, id: string) => q(base, `programme=${id}`);

  // The filter narrows the navigation, not the page body: the tenant summary and
  // a document's own text still say whatever they say. Exclusion is therefore
  // asserted against the sidebar only.
  const sidebarOf = (html: string) =>
    html.slice(html.indexOf('<aside'), html.indexOf('</aside>'));

  it('badges a resource with its programmes in the index listing', async () => {
    const res = await request(app.getHttpServer())
      .get('/nosummary')
      .expect(200);
    expect(res.text).toContain('Firewall');
  });

  it('offers every axis the index declares, zero-count values included', async () => {
    const res = await request(app.getHttpServer())
      .get('/mytenant')
      .expect(200);
    // Axis headings are the labels from the index, not names baked in here.
    expect(res.text).toContain('Programme');
    expect(res.text).toContain('Platform');
    expect(res.text).toContain(programme('/mytenant', 'firewall'));
    expect(res.text).toContain(q('/mytenant', 'platform=windows'));
    // A value that matched nothing here is still offered while nothing is
    // filtering: "empty in this tenant" is information the registry carries.
    expect(res.text).toContain(programme('/mytenant', 'vpn'));
    expect(res.text).toContain(programme('/mytenant', '_uncategorised'));
    // Nothing selected, so no reset and no "showing" line.
    expect(res.text).not.toContain('Clear filters');
  });

  it('GET /:tenant?programme= narrows the tree to that programme', async () => {
    const res = await request(app.getHttpServer())
      .get('/mytenant?programme=firewall')
      .expect(200);
    expect(sidebarOf(res.text)).toContain('Policy One');
    expect(sidebarOf(res.text)).not.toContain('Admins');
    // The choice rides along in every document link, so it survives a click.
    expect(res.text).toContain(
      programme(
        '/mytenant/Microsoft.Graph/deviceManagementConfigurationPolicies/p1',
        'firewall',
      ),
    );
    // The selection is stated, and resettable, without any client-side state.
    expect(sidebarOf(res.text)).toContain('Showing 1 of 3');
    expect(sidebarOf(res.text)).toContain('Clear filters');
    expect(sidebarOf(res.text)).toContain('href="/mytenant"');
  });

  it('combines two axes: OR within an axis, AND across axes', async () => {
    const both = await request(app.getHttpServer())
      .get('/mytenant?programme=firewall&platform=windows')
      .expect(200);
    expect(sidebarOf(both.text)).toContain('Policy One');
    expect(sidebarOf(both.text)).toContain('Showing 1 of 3');

    // The same programme with the other platform is a dead end, not a fallback
    // to one of the two axes.
    const dead = await request(app.getHttpServer())
      .get('/mytenant?programme=firewall&platform=macos')
      .expect(200);
    expect(sidebarOf(dead.text)).not.toContain('Policy One');
    expect(sidebarOf(dead.text)).toContain('No documents match these filters');

    // Two values on one axis are OR-ed: the zero-count one adds nothing but
    // does not remove the other's matches either.
    const ored = await request(app.getHttpServer())
      .get('/mytenant?programme=firewall&programme=vpn')
      .expect(200);
    expect(sidebarOf(ored.text)).toContain('Policy One');
    expect(sidebarOf(ored.text)).not.toContain('Admins');
  });

  it('toggles one value per chip without losing the other axis', async () => {
    const res = await request(app.getHttpServer())
      .get('/mytenant?programme=firewall&platform=windows')
      .expect(200);
    const sidebar = sidebarOf(res.text);
    // The active programme chip switches itself off and keeps the platform.
    expect(sidebar).toContain(q('/mytenant', 'platform=windows'));
    // An inactive chip adds itself to the selection instead of replacing it.
    expect(sidebar).toContain(
      q('/mytenant', 'programme=firewall&programme=hardening&platform=windows'),
    );
  });

  it('stops offering a value another filter has emptied', async () => {
    const unfiltered = await request(app.getHttpServer())
      .get('/mytenant')
      .expect(200);
    expect(unfiltered.text).toContain(programme('/mytenant', 'vpn'));

    // Under platform=windows, `vpn` leads nowhere, so it is not offered ...
    const filtered = await request(app.getHttpServer())
      .get('/mytenant?platform=windows')
      .expect(200);
    const sidebar = sidebarOf(filtered.text);
    expect(sidebar).not.toContain('programme&#x3D;vpn');
    // ... while a value that still leads somewhere is.
    expect(sidebar).toContain(
      q('/mytenant', 'programme=firewall&platform=windows'),
    );
  });

  it('GET /:tenant?programme=_uncategorised shows exactly what the taxonomy missed', async () => {
    const res = await request(app.getHttpServer())
      .get('/mytenant?programme=_uncategorised')
      .expect(200);
    const sidebar = sidebarOf(res.text);
    expect(sidebar).toContain('Admins');
    expect(sidebar).toContain('Compliance One');
    expect(sidebar).not.toContain('Policy One');
  });

  it('says so plainly when a selection matches nothing in this tenant', async () => {
    const res = await request(app.getHttpServer())
      .get('/mytenant?programme=vpn')
      .expect(200);
    const sidebar = sidebarOf(res.text);
    expect(sidebar).toContain('No documents match these filters');
    expect(sidebar).toContain('Showing 0 of 3');
    expect(sidebar).not.toContain('Policy One');
    // The selected value stays offered even at 0, or the choice that emptied
    // the tree could not be undone.
    expect(sidebar).toContain('programme&#x3D;vpn');
  });

  it('ignores an unknown value instead of rendering an empty tenant', async () => {
    const res = await request(app.getHttpServer())
      .get('/mytenant?programme=nope&platform=nope&nosuchaxis=x')
      .expect(200);
    const sidebar = sidebarOf(res.text);
    expect(sidebar).toContain('Policy One');
    expect(sidebar).toContain('Admins');
    expect(sidebar).not.toContain('Clear filters');
  });

  it('still filters a version-2 index, from the synthesised programme axis', async () => {
    const offered = await request(app.getHttpServer())
      .get('/nosummary')
      .expect(200);
    expect(offered.text).toContain('Programme');
    expect(offered.text).toContain(programme('/nosummary', 'firewall'));
    // That index declares no second axis, so none is invented for it.
    expect(offered.text).not.toContain('platform&#x3D;');

    const res = await request(app.getHttpServer())
      .get('/nosummary?programme=firewall')
      .expect(200);
    expect(sidebarOf(res.text)).toContain('Policy One');
    expect(sidebarOf(res.text)).not.toContain('Admins');
  });

  it('keeps the document you are on in its sidebar even when filtered out', async () => {
    const res = await request(app.getHttpServer())
      .get('/mytenant/Microsoft.Graph/groups/g1?programme=firewall')
      .expect(200);
    const sidebar = sidebarOf(res.text);
    expect(sidebar).toContain('Admins');
    expect(sidebar).toContain(
      programme('/mytenant/Microsoft.Graph/groups/g1', 'firewall'),
    );
    // The exemption does not inflate the count: it describes the selection. The
    // extra row is labelled and announced instead, so the tree being one longer
    // than the count is explained rather than left to be reconciled.
    expect(sidebar).toContain('Showing 1 of 3');
    expect(sidebar).toContain('plus the document you are viewing');
    expect(sidebar).toContain('outside the filter');
    // The filter is still the active one, not silently reset by the visit.
    expect(sidebar).toContain('aria-current="true"');
  });

  it('says nothing about an exemption when the document you are on matches', async () => {
    const res = await request(app.getHttpServer())
      .get(
        '/mytenant/Microsoft.Graph/deviceManagementConfigurationPolicies/p1' +
          '?programme=firewall',
      )
      .expect(200);
    const sidebar = sidebarOf(res.text);
    expect(sidebar).toContain('Showing 1 of 3');
    expect(sidebar).not.toContain('plus the document you are viewing');
    expect(sidebar).not.toContain('outside the filter');
  });

  it('GET /:tenant/summary redirects to the landing page (the summary is its body)', async () => {
    await request(app.getHttpServer())
      .get('/mytenant/summary')
      .expect(302)
      .expect('Location', '/mytenant');
    await request(app.getHttpServer())
      .get('/mytenant/summary.md')
      .expect(302)
      .expect('Location', '/mytenant');
  });

  it('renders the sidebar navigation with the tenant metadata on the landing page', async () => {
    const res = await request(app.getHttpServer())
      .get('/mytenant')
      .expect(200);
    expect(res.text).toContain('My Tenant');
    expect(res.text).toContain('2 documented');
    expect(res.text).toContain('Policy One');
    expect(res.text).toContain(
      'href="/mytenant/Microsoft.Graph/groups/g1"',
    );
    // A not-yet-documented resource stays visible as pending, honestly.
    expect(res.text).toContain('Compliance One');
    expect(res.text).toContain('pending');
    // Excluded bulk types are reported as counts only, never listed.
    expect(res.text).toContain(
      'Microsoft.Graph/windowsAutopilotDeviceIdentities',
    );
  });

  it('marks the current document in the sidebar and opens its section', async () => {
    const res = await request(app.getHttpServer())
      .get('/mytenant/Microsoft.Graph/groups/g1')
      .expect(200);
    expect(res.text).toContain('nav-tree');
    expect(res.text).toContain('<details open>');
    expect(res.text).toMatch(
      /href="\/mytenant\/Microsoft\.Graph\/groups\/g1"[^>]*aria-current="page"/,
    );
  });

  it('names the navigation sidebar as a landmark', async () => {
    // Assistive technology needs a name for the <aside>, the way the view
    // switcher and the facet filters already have one.
    const res = await request(app.getHttpServer())
      .get('/mytenant/Microsoft.Graph/groups/g1')
      .expect(200);
    expect(res.text).toMatch(
      /<aside aria-label="Tenant navigation"[^>]*class="nav-tree/,
    );
  });

  it('gives the top bar the same width as the page under it', async () => {
    const wide = await request(app.getHttpServer())
      .get('/mytenant/Microsoft.Graph/groups/g1')
      .expect(200);
    // The document layout is max-w-7xl, so an inset max-w-5xl header row would
    // leave the breadcrumb out of line with the sidebar and the document.
    expect(wide.text).toMatch(/<div class="mx-auto max-w-7xl px-4 py-3/);

    const narrow = await request(app.getHttpServer()).get('/').expect(200);
    expect(narrow.text).toMatch(/<div class="mx-auto max-w-5xl px-4 py-3/);
  });

  it('tags the summary Findings table and its severities for the stylesheet', async () => {
    const res = await request(app.getHttpServer())
      .get('/mytenant')
      .expect(200);

    // The table is found by its Severity header, and each body row carries the
    // severity so CSS can draw an icon instead of the word.
    expect(res.text).toMatch(/<table class="findings">/);
    expect(res.text).toMatch(/<tr data-severity="critical">/);
    expect(res.text).toMatch(/<tr data-severity="medium">/);
    expect(res.text).toMatch(
      /<td data-severity="critical" title="critical">critical<\/td>/,
    );
    // The word stays in the DOM: the icon is an image replacement, not a swap.
    expect(res.text).toContain('>critical</td>');

    // A value outside the closed set is left alone rather than mislabelled.
    expect(res.text).not.toContain('data-severity="nonsense"');
    expect(res.text).toContain('<td>nonsense</td>');
  });

  it('tags declared document sections and leaves undeclared headings unstyled', async () => {
    const res = await request(app.getHttpServer())
      .get('/mytenant/Microsoft.Graph/deviceManagementConfigurationPolicies/p1')
      .expect(200);

    // A heading in the CLI's closed set is addressable *and* styled.
    expect(res.text).toMatch(
      /<h2 id="settings"[^>]*data-section="settings"[^>]*class="doc-section-heading"/,
    );
    expect(res.text).toMatch(/data-section="references"/);
    // `Lifecycle & operations` and `Lifecycle and operations` are one section,
    // so the styling survives the pending regeneration either way.
    expect(res.text).toMatch(
      /<h2 id="lifecycle-and-operations"[^>]*class="doc-section-heading"/,
    );
    // ...and no percent-encoded id is emitted for it any more.
    expect(res.text).not.toContain('lifecycle-%26-operations');

    // A heading outside the vocabulary stays addressable but borrows no
    // section's identity.
    expect(res.text).toMatch(/<h2 id="metadata"[^>]*data-section="metadata"/);
    expect(res.text).not.toMatch(
      /<h2 id="metadata"[^>]*class="doc-section-heading"/,
    );
  });

  it('tags the summary sections on the tenant landing page', async () => {
    const res = await request(app.getHttpServer())
      .get('/mytenant')
      .expect(200);
    expect(res.text).toMatch(
      /<h2 id="management-summary"[^>]*data-section="management-summary"/,
    );
    // The findings/recommendations H3 vocabulary is declared too.
    expect(res.text).toMatch(
      /<h3 id="findings"[^>]*data-section="findings"[^>]*class="doc-section-heading"/,
    );
    expect(res.text).toMatch(/data-section="recommendations"/);
    // The em dash in the H1 no longer percent-encodes into the anchor.
    expect(res.text).not.toContain('%E2%80%94');
  });

  it('wraps each H2 run in a section carrying the same slug', async () => {
    const res = await request(app.getHttpServer())
      .get('/mytenant/Microsoft.Graph/deviceManagementConfigurationPolicies/p1')
      .expect(200);

    expect(res.text).toContain(
      '<section class="doc-section" data-section="settings">',
    );
    expect(res.text).toContain(
      '<section class="doc-section" data-section="lifecycle-and-operations">',
    );
    // An undeclared heading is still a section — it just gets no styling.
    expect(res.text).toContain(
      '<section class="doc-section" data-section="metadata">',
    );
    // Every section closes: one wrapper per H2 that opened one.
    const opens = (res.text.match(/<section class="doc-section"/g) || []).length;
    const closes = (res.text.match(/<\/section>/g) || []).length;
    expect(opens).toBe(closes);

    // The wrapper emits no newline of its own. The Confluence exporter unwraps
    // `<section>` but keeps the text between the tags, so a block token would
    // put a blank line into every exported page for a browser-only wrapper.
    expect(res.text).toMatch(/<section class="doc-section"[^>]*><h2/);
    expect(res.text).not.toMatch(/<section class="doc-section"[^>]*>\n/);

    // The H1, the metadata table and the assignments block stay in the
    // pre-section prelude, before the first wrapper.
    expect(res.text.indexOf('class="doc-metadata"')).toBeLessThan(
      res.text.indexOf('<section class="doc-section"'),
    );
  });

  it('does not let a spliced block straddle a section boundary', async () => {
    const res = await request(app.getHttpServer())
      .get('/mytenant/Microsoft.Graph/deviceManagementConfigurationPolicies/p1')
      .expect(200);

    // `## Used by` is spliced inside the References section, so it must not open
    // a section of its own — that would close References inside the block's div
    // and emit mis-nested HTML.
    expect(res.text).toContain('<div class="doc-used-by">');
    expect(res.text).not.toMatch(
      /<section class="doc-section" data-section="used-by"/,
    );
    // The heading itself is still styled and addressable.
    expect(res.text).toMatch(
      /<h2 id="used-by"[^>]*data-section="used-by"[^>]*class="doc-section-heading"/,
    );
    // The block opens and closes inside one section.
    const body = res.text;
    const div = body.indexOf('<div class="doc-used-by">');
    const divEnd = body.indexOf('</div>', div);
    expect(body.slice(div, divEnd)).not.toContain('</section>');
  });

  it('turns the assignments marker pair into a selectable element', async () => {
    const res = await request(app.getHttpServer())
      .get('/mytenant/Microsoft.Graph/deviceManagementConfigurationPolicies/p1')
      .expect(200);
    expect(res.text).toContain('<div class="doc-assignments">');
    // The comments are gone, so nothing selectable is left behind.
    expect(res.text).not.toContain('assignments:start');
    expect(res.text).not.toContain('assignments:end');
  });

  it('classes the metadata table but not the assignments table', async () => {
    const res = await request(app.getHttpServer())
      .get('/mytenant/Microsoft.Graph/deviceManagementConfigurationPolicies/p1')
      .expect(200);
    expect((res.text.match(/class="doc-metadata"/g) || []).length).toBe(1);
    // The metadata table is the first one; the assignments table is untagged.
    expect(res.text.indexOf('class="doc-metadata"')).toBeLessThan(
      res.text.indexOf('<div class="doc-assignments">'),
    );
    expect(res.text).toContain('<table>');
  });

  it('does not class the summary findings table as metadata', async () => {
    const res = await request(app.getHttpServer())
      .get('/mytenant')
      .expect(200);
    expect(res.text).toContain('<table class="findings">');
    expect(res.text).not.toContain('doc-metadata');
  });

  it('leaves tables without a Severity column untagged', async () => {
    const res = await request(app.getHttpServer())
      .get('/mytenant/Microsoft.Graph/deviceManagementConfigurationPolicies/p1')
      .expect(200);
    expect(res.text).toContain('<table>');
    expect(res.text).not.toContain('class="findings"');
  });

  it('reflects an edited summary.md on the next request without a restart', async () => {
    const marker = `SUMMARY_PROBE_${Date.now()}`;
    await fsp.writeFile(summaryFile, `${SUMMARY_MD}\n${marker}\n`);
    const res = await request(app.getHttpServer())
      .get('/mytenant')
      .expect(200);
    expect(res.text).toContain(marker);
    await fsp.writeFile(summaryFile, SUMMARY_MD);
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
    // The index drives the sidebar, which is on the landing page too.
    expect(res.text).toContain('Policy Renamed');
    await fsp.writeFile(path.join(tenantDir, 'index.yaml'), INDEX_YAML);
  });

  it('reflects a regenerated index.yaml in the picker and /healthz counts, within the discovery TTL', async () => {
    await fsp.writeFile(
      path.join(tenantDir, 'index.yaml'),
      INDEX_YAML.replace('documented: 2', 'documented: 12')
        .replace('pending: 1', 'pending: 0')
        .replace(
          'generatedAt: "2026-01-01T00:00:00Z"',
          'generatedAt: "2026-02-02T00:00:00Z"',
        ),
    );

    const picker = await request(app.getHttpServer()).get('/').expect(200);
    expect(picker.text).toContain('12 documented');
    expect(picker.text).toContain('exported 2026-02-02T00:00:00Z');

    const health = await request(app.getHttpServer())
      .get('/healthz')
      .expect(200);
    expect(health.body).toEqual({
      status: 'ok',
      rootReadable: true,
      tenants: 3,
      documents: 14,
      pending: 1,
    });

    await fsp.writeFile(path.join(tenantDir, 'index.yaml'), INDEX_YAML);
  });

  it('drops the source echo under the H1 but keeps prose mentions of other resources', async () => {
    const res = await request(app.getHttpServer())
      .get(
        '/mytenant/Microsoft.Graph/deviceManagementConfigurationPolicies/p1',
      )
      .expect(200);
    // The redundant `<source>.yaml` paragraph is gone from the body...
    expect(res.text).not.toContain('<p><code>p1.yaml</code></p>');
    // ...while a mention of another resource inside a sentence survives.
    expect(res.text).toContain('<code>other_policy.yaml</code>');
    // The H1 is untouched.
    expect(res.text).toContain('Policy One');
  });

  it('renders a settings-catalog doc: <details> pass through, ../groups link rewritten, frontmatter stripped', async () => {
    const res = await request(app.getHttpServer())
      .get(
        '/mytenant/Microsoft.Graph/deviceManagementConfigurationPolicies/p1',
      )
      .expect(200);
    // Raw HTML <details> blocks survive (html: true), nested included, with the
    // generator's own data-setting/data-note attributes untouched.
    expect((res.text.match(/<details /g) || []).length).toBeGreaterThanOrEqual(
      2,
    );
    expect(res.text).toContain('data-setting="settings[0].value"');
    expect(res.text).toContain('data-note="security"');
    expect(res.text).toContain('data-note="inert"');
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
    expect(res.text).toContain('Document not found');
    expect(res.text).not.toContain(root); // no absolute path leaked
  });

  it('GET an unknown tenant returns 404', async () => {
    const res = await request(app.getHttpServer())
      .get('/nope-tenant')
      .expect(404);
    expect(res.text).toContain('Tenant not found');
    // The skipped housekeeping dir is not routable as a tenant either.
    await request(app.getHttpServer()).get('/_to_delete').expect(404);
  });

  it('rejects path traversal (encoded) with a 404', async () => {
    await request(app.getHttpServer())
      .get('/mytenant/..%2f..%2f..%2fetc%2fpasswd')
      .expect(404);
  });

  it('GET /:tenant/_resource/* renders the source YAML with line anchors', async () => {
    const res = await request(app.getHttpServer())
      .get(
        '/mytenant/_resource/Microsoft.Graph/deviceManagementConfigurationPolicies/p1',
      )
      .expect(200);
    expect(res.text).toContain('yaml-view');
    // Highlighted, with a deep-linkable anchor and gutter link per line.
    expect(res.text).toContain('id="L1"');
    expect(res.text).toContain('href="#L1"');
    expect(res.text).toContain('firewall');
    // The breadcrumb shows the document path, never the _resource prefix.
    expect(res.text).not.toContain('>_resource<');
    // The sidebar still travels with the page.
    expect(res.text).toContain('nav-tree');
  });

  it('serves ?raw as plain text with nosniff', async () => {
    const res = await request(app.getHttpServer())
      .get(
        '/mytenant/_resource/Microsoft.Graph/deviceManagementConfigurationPolicies/p1?raw',
      )
      .expect(200)
      .expect('Content-Type', 'text/plain; charset=utf-8')
      .expect('X-Content-Type-Options', 'nosniff');
    expect(res.text).toBe(POLICY_YAML);
  });

  it('offers the Documentation/YAML switcher on both representations', async () => {
    const doc = await request(app.getHttpServer())
      .get(
        '/mytenant/Microsoft.Graph/deviceManagementConfigurationPolicies/p1',
      )
      .expect(200);
    expect(doc.text).toContain(
      'href="/mytenant/_resource/Microsoft.Graph/deviceManagementConfigurationPolicies/p1"',
    );

    const yaml = await request(app.getHttpServer())
      .get(
        '/mytenant/_resource/Microsoft.Graph/deviceManagementConfigurationPolicies/p1',
      )
      .expect(200);
    expect(yaml.text).toMatch(
      /href="\/mytenant\/_resource\/Microsoft\.Graph\/deviceManagementConfigurationPolicies\/p1"[^>]*aria-current="page"/,
    );
    // ...and back to the document.
    expect(yaml.text).toContain(
      'href="/mytenant/Microsoft.Graph/deviceManagementConfigurationPolicies/p1"',
    );
  });

  it('omits the YAML switcher entry for a document without a source', async () => {
    const res = await request(app.getHttpServer())
      .get('/mytenant/Microsoft.Graph/deviceCompliancePolicies/c1')
      .expect(200);
    expect(res.text).toContain('No frontmatter');
    expect(res.text).not.toContain(
      '/mytenant/_resource/Microsoft.Graph/deviceCompliancePolicies/c1',
    );
  });

  it('shows an inert Drift entry, stating why, for an export with no drift observation', async () => {
    const inert =
      '<span aria-disabled="true" title="No drift observation. Run azure-rd resource drift."';
    const doc = await request(app.getHttpServer())
      .get('/mytenant/Microsoft.Graph/deviceManagementConfigurationPolicies/p1')
      .expect(200);
    expect(doc.text).toContain(inert);
    expect(doc.text).not.toContain('/mytenant/_drift/');

    const yaml = await request(app.getHttpServer())
      .get('/mytenant/_resource/Microsoft.Graph/deviceManagementConfigurationPolicies/p1')
      .expect(200);
    expect(yaml.text).toContain(inert);

    // The landing page gains Summary | Drift, with Drift inert for the same reason.
    const landing = await request(app.getHttpServer()).get('/mytenant').expect(200);
    expect(landing.text).toMatch(/href="\/mytenant"[^>]*aria-current="page"/);
    expect(landing.text).toContain(inert);
  });

  it('says so on the drift routes of an export with no drift observation', async () => {
    const tenantPage = await request(app.getHttpServer())
      .get('/mytenant/_drift')
      .expect(200);
    expect(tenantPage.text).toContain('No drift observation for this export');
    const res = await request(app.getHttpServer())
      .get('/mytenant/_drift/Microsoft.Graph/deviceManagementConfigurationPolicies/p1')
      .expect(404);
    expect(res.text).toContain('No drift observation');
    expect(res.text).not.toContain(root);
  });

  it('404s for a missing resource, without leaking a filesystem path', async () => {
    const res = await request(app.getHttpServer())
      .get('/mytenant/_resource/Microsoft.Graph/groups/g1')
      .expect(404);
    expect(res.text).toContain('Source YAML not found');
    expect(res.text).not.toContain(root);
  });

  it('does not serve a Markdown file from the resources root', async () => {
    await request(app.getHttpServer())
      .get(
        '/mytenant/_resource/Microsoft.Graph/deviceManagementConfigurationPolicies/p1.md',
      )
      .expect(404);
  });

  it('rejects traversal out of the resources root (into docs/)', async () => {
    await request(app.getHttpServer())
      .get('/mytenant/_resource/..%2fdocs%2fsummary')
      .expect(404);
    await request(app.getHttpServer())
      .get('/mytenant/_resource/..%2f..%2f..%2fetc%2fpasswd')
      .expect(404);
  });

  it('reflects a re-downloaded resource on the next request without a restart', async () => {
    const marker = `yaml_probe_${Date.now()}`;
    await fsp.writeFile(policyYamlFile, `${POLICY_YAML}${marker}: true\n`);
    const res = await request(app.getHttpServer())
      .get(
        '/mytenant/_resource/Microsoft.Graph/deviceManagementConfigurationPolicies/p1',
      )
      .expect(200);
    expect(res.text).toContain(marker);
    await fsp.writeFile(policyYamlFile, POLICY_YAML);
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

  it('offers the export as a plain download link on the tenant picker', async () => {
    const res = await request(app.getHttpServer()).get('/').expect(200);
    expect(res.text).toContain('href="/mytenant/_export/confluence"');
    expect(res.text).toContain('href="/nosummary/_export/confluence"');
    expect(res.text).toContain('One-way publish');

    // ...and not on the tenant landing page, which is documentation only.
    const landing = await request(app.getHttpServer())
      .get('/mytenant')
      .expect(200);
    expect(landing.text).not.toContain('_export');
  });

  it('exports the tenant as a Confluence-importable zip', async () => {
    const res = await request(app.getHttpServer())
      .get('/mytenant/_export/confluence')
      .buffer()
      .parse(binaryParser)
      .expect(200)
      .expect('Content-Type', 'application/zip')
      .expect('Content-Disposition', 'attachment; filename="mytenant.zip"');

    // Zip entry names are stored uncompressed in the local file headers, so the
    // archive can be inspected without an unzip dependency.
    const entries = (res.body as Buffer).toString('utf8');
    // One folder, whose name becomes the space name.
    expect(entries).toContain(
      'My Tenant documentation/deviceManagementConfigurationPolicies — Policy One.html',
    );
    expect(entries).toContain('My Tenant documentation/groups — Admins.html');
    // A pending document is exported too, so the space is not silently partial.
    expect(entries).toContain(
      'My Tenant documentation/deviceCompliancePolicies — Compliance One.html',
    );
    expect(entries).toContain('My Tenant documentation/Overview.html');
  });

  it('leaves the browser render cache and the docs root untouched', async () => {
    const before = await request(app.getHttpServer())
      .get('/mytenant/Microsoft.Graph/groups/g1')
      .expect(200);
    const tree = await snapshot(root);

    await request(app.getHttpServer())
      .get('/mytenant/_export/confluence')
      .buffer()
      .parse(binaryParser)
      .expect(200);

    // The export renders with the same env as the browser, so it can neither
    // poison nor bypass the mtime-keyed cache.
    const after = await request(app.getHttpServer())
      .get('/mytenant/Microsoft.Graph/groups/g1')
      .expect(200);
    expect(after.text).toBe(before.text);
    // Read-only: the archive is built in memory and streamed.
    expect(await snapshot(root)).toEqual(tree);
  });

  it('follows EXPORT_INDEX, which is read per request', async () => {
    const previous = process.env.EXPORT_INDEX;
    try {
      // Unset: the by-type list alone, however rich this tenant's taxonomy is.
      delete process.env.EXPORT_INDEX;
      const byType = await overviewOf('/mytenant/_export/confluence');
      expect(byType).toContain('<h2>Pages</h2>');
      expect(byType).not.toContain('<h2>Platform</h2>');

      // `both` adds one collapsible section per axis the index declares, after
      // that list — no restart, because the variable is read at its point of use.
      process.env.EXPORT_INDEX = 'both';
      const both = await overviewOf('/mytenant/_export/confluence');
      expect(both.indexOf('<h2>Pages</h2>')).toBeLessThan(
        both.indexOf('<h2>Programme</h2>'),
      );
      expect(both).toContain('<summary>Windows (1)</summary>');
      // The export classifies through the sidebar's own rule, so a page with no
      // value on an axis lands in the bucket the filter would put it in.
      expect(both).toContain('<summary>Uncategorised (1)</summary>');

      // `axis` drops the spine; a typo falls back to the default.
      process.env.EXPORT_INDEX = 'axis';
      expect(await overviewOf('/mytenant/_export/confluence')).not.toContain(
        '<h2>Pages</h2>',
      );
      process.env.EXPORT_INDEX = 'nonsense';
      expect(await overviewOf('/mytenant/_export/confluence')).toContain(
        '<h2>Pages</h2>',
      );
    } finally {
      if (previous === undefined) delete process.env.EXPORT_INDEX;
      else process.env.EXPORT_INDEX = previous;
    }
  });

  it('404s an unknown export format and an unknown tenant', async () => {
    const format = await request(app.getHttpServer())
      .get('/mytenant/_export/docx')
      .expect(404);
    expect(format.text).toContain('Export format not found');

    const tenant = await request(app.getHttpServer())
      .get('/nosuchtenant/_export/confluence')
      .expect(404);
    expect(tenant.text).toContain('Tenant not found');
  });

  // The overview page out of the streamed archive. The index mode changes no
  // file name, so the assertion needs the entry's *contents*, which `yazl`
  // deflates.
  async function overviewOf(route: string): Promise<string> {
    const res = await request(app.getHttpServer())
      .get(route)
      .buffer()
      .parse(binaryParser)
      .expect(200);
    return readZipEntry(res.body as Buffer, OVERVIEW_FILE);
  }
});

// A DOCS_ROOT that does not exist (unmounted volume, typo). Discovery already
// degrades to "no tenants" instead of crashing; the health endpoint has to say
// which of the two it is, without echoing the path.
describe('Docs browser with a missing docs root (e2e)', () => {
  let app: NestExpressApplication;
  let missingRoot: string;

  beforeAll(async () => {
    const parent = await fsp.mkdtemp(path.join(os.tmpdir(), 'docsroot-'));
    missingRoot = path.join(parent, 'does-not-exist');
    await fsp.rm(parent, { recursive: true, force: true });
    process.env.DOCS_ROOT = missingRoot;

    const moduleRef = await Test.createTestingModule({
      imports: [AppModule],
    }).compile();
    app = moduleRef.createNestApplication<NestExpressApplication>();
    configureViews(app);
    await app.init();
  });

  afterAll(async () => {
    await app?.close();
  });

  it('GET /healthz stays 200 but reports the root as unreadable', async () => {
    const res = await request(app.getHttpServer()).get('/healthz').expect(200);
    expect(res.body).toEqual({
      status: 'degraded',
      rootReadable: false,
      tenants: 0,
      documents: 0,
      pending: 0,
    });
    expect(res.text).not.toContain(missingRoot);
  });

  it('GET / still renders the empty picker without leaking the path', async () => {
    const res = await request(app.getHttpServer()).get('/').expect(200);
    expect(res.text).toContain('No tenants found');
    expect(res.text).not.toContain(missingRoot);
  });
});

// An export with a drift/ tree beside docs/ and resources/, as
// `azure-rd resource drift` and the analysis agent leave it. Its own app and
// docs root, so the main suite's counts are untouched.
describe('Docs browser drift view (e2e)', () => {
  const BASELINE = '2026-01-01T00:00:00Z';
  const OBSERVED = '2026-02-01T10:00:00Z';
  const T = 'Microsoft.Graph/deviceConfigurations';

  const BASE: Record<string, string> = {
    changed1: 'id: changed1\nsettings:\n  enabled: true\n',
    same1: 'id: same1\n',
    old_name: 'id: renamed1\ndisplayName: Old name\n',
    unattested1: 'id: unattested1\n',
    tampered1: 'id: tampered1\nsecret: a\n',
  };
  const CHANGED_PAYLOAD = 'id: changed1\nsettings:\n  enabled: false\n';
  const RENAMED_PAYLOAD = 'id: renamed1\ndisplayName: New name\n';
  const TAMPERED_PAYLOAD = 'id: tampered1\nsecret: b\n';
  const ADDED_PAYLOAD = 'id: loc1\ndisplayName: kali-vpn-location\n';

  const sha = (s: string) => createHash('sha256').update(s).digest('hex');

  const INDEX = `version: 3
tenant: drifted.example
generatedAt: "${BASELINE}"
complete: true
resources:
${[...Object.keys(BASE)]
  .map(
    (n) =>
      `    - type: ${T}\n      doc: ${T}/${n}.md\n      displayName: ${n}\n      documented: true\n`,
  )
  .join('')}    - type: Microsoft.Graph/organizationalBranding
      doc: Microsoft.Graph/organizationalBranding/brand1.md
      displayName: brand1
      documented: true
`;

  const OBSERVATION = `observedAt: "${OBSERVED}"
tenant: drifted.example
toolVersion: azure-rd test
baseline:
    generatedAt: "${BASELINE}"
    toolVersion: azure-rd test
run:
    complete: false
    incompleteReason: 1 resource types could not be listed
counts:
    compared: 5
    unchanged: 1
    changed: 2
    renamed: 1
    added: 1
    removed: 0
    unattested: 1
unknownTypes:
    - Microsoft.Graph/organizationalBranding
removalsSuppressed: true
notComparable:
    - key: ${T}/unattested1.yaml
      reason: baseline entry predates per-entry config attestation
findings:
    ${T}/changed1.yaml:
        verdict: changed
        displayName: Changed one
        baselineKey: ${T}/changed1.yaml
        baselineSha256: ${sha(BASE.changed1)}
        payloadSha256: ${sha(CHANGED_PAYLOAD)}
        deltas:
            - path: settings.enabled
              old: "true"
              new: "false"
    ${T}/new_name.yaml:
        verdict: renamed
        displayName: New name
        previousDisplayName: Old name
        baselineKey: ${T}/old_name.yaml
        baselineSha256: ${sha(BASE.old_name)}
        payloadSha256: ${sha(RENAMED_PAYLOAD)}
        deltas:
            - path: displayName
              old: Old name
              new: New name
    ${T}/tampered1.yaml:
        verdict: changed
        displayName: Tampered one
        baselineKey: ${T}/tampered1.yaml
        baselineSha256: ${sha(BASE.tampered1)}
        payloadSha256: ${sha(TAMPERED_PAYLOAD)}
        deltas:
            - path: secretDelta
              old: a
              new: b
    Microsoft.Graph/namedLocations/new_loc.yaml:
        verdict: added
        displayName: Kali VPN location
        payloadSha256: ${sha(ADDED_PAYLOAD)}
payloads:
    - ${T}/changed1.yaml
    - ${T}/new_name.yaml
    - ${T}/tampered1.yaml
    - Microsoft.Graph/namedLocations/new_loc.yaml
`;

  const ANALYSIS = `---
observedAt: ${OBSERVED}
verdict: changed
severity: high
---

# Drift: Changed one

See [the rename](new_name.md) and [its documentation](../../../docs/${T}/changed1.md).
`;

  const DRIFT_INDEX = `---
findings: 4
---

# Drift analysis summary

| Severity | Verdict | Resource | Judgment |
|---|---|---|---|
| high | changed | [Changed one](${T}/changed1.md) | Tightened. |
| medium | shifted | [Changed one](${T}/changed1.md) | Unknown verdict. |
`;

  let app: NestExpressApplication;
  let root: string;
  let exportDir: string;
  let driftDir: string;

  const write = async (rel: string, content: string) => {
    const file = path.join(exportDir, rel);
    await fsp.mkdir(path.dirname(file), { recursive: true });
    await fsp.writeFile(file, content);
  };

  // The drift tree exactly as the tests expect it, so a case that rewrites or
  // deletes it can put it back.
  const writeDrift = async () => {
    await fsp.rm(driftDir, { recursive: true, force: true });
    await write('drift/metadata.yaml', OBSERVATION);
    await write('drift/index.md', DRIFT_INDEX);
    await write('drift/analyze.md', '# The analysis prompt, never served\n');
    await write(`drift/${T}/changed1.yaml`, CHANGED_PAYLOAD);
    await write(`drift/${T}/changed1.md`, ANALYSIS);
    await write(`drift/${T}/new_name.yaml`, RENAMED_PAYLOAD);
    // Not what the observation recorded: the file was edited after the run.
    await write(`drift/${T}/tampered1.yaml`, 'id: tampered1\nsecret: edited later\n');
    await write('drift/Microsoft.Graph/namedLocations/new_loc.yaml', ADDED_PAYLOAD);
  };

  const get = (url: string) => request(app.getHttpServer()).get(url);

  beforeAll(async () => {
    root = await fsp.mkdtemp(path.join(os.tmpdir(), 'docsroot-drift-'));
    exportDir = path.join(root, 'drifted');
    driftDir = path.join(exportDir, 'drift');
    await write('docs/index.yaml', INDEX);
    for (const [name, yaml] of Object.entries(BASE)) {
      await write(`docs/${T}/${name}.md`, `---\nsource: ${T}/${name}.yaml\n---\n\n# ${name}\n`);
      await write(`resources/${T}/${name}.yaml`, yaml);
    }
    await write(
      'docs/Microsoft.Graph/organizationalBranding/brand1.md',
      '---\nsource: Microsoft.Graph/organizationalBranding/brand1.yaml\n---\n\n# brand1\n',
    );
    await write('resources/Microsoft.Graph/organizationalBranding/brand1.yaml', 'id: brand1\n');
    await write('resources/metadata.yaml', `generatedAt: "${BASELINE}"\ntenant: drifted.example\n`);
    await writeDrift();

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

  it('links a finding from the documentation and YAML views, verdict in the label', async () => {
    const href = `href="/drifted/_drift/${T}/changed1"`;
    const doc = await get(`/drifted/${T}/changed1`).expect(200);
    expect(doc.text).toContain(href);
    expect(doc.text).toContain('Drift · changed</a>');
    const yaml = await get(`/drifted/_resource/${T}/changed1`).expect(200);
    expect(yaml.text).toContain(href);
  });

  it('shows the Drift entry inert with its reason for an unchanged resource', async () => {
    const doc = await get(`/drifted/${T}/same1`).expect(200);
    expect(doc.text).toContain(
      `<span aria-disabled="true" title="Unchanged as of ${OBSERVED}"`,
    );
    expect(doc.text).not.toContain(`/drifted/_drift/${T}/same1`);
  });

  it('renders a changed finding: deltas, severity, links and the analysis', async () => {
    const res = await get(`/drifted/_drift/${T}/changed1`).expect(200);
    expect(res.text).toContain('Changed one');
    expect(res.text).toContain('What changed');
    expect(res.text).toContain('settings.enabled');
    expect(res.text).toMatch(/class="badge[^"]*">changed</);
    expect(res.text).toMatch(/class="badge[^"]*">high</);
    // Both sides verified: one diff link instead of the two YAML views.
    expect(res.text).toContain(`href="/drifted/_drift/${T}/changed1?diff"`);
    expect(res.text).not.toContain('>Baseline YAML</a>');
    expect(res.text).not.toContain(`href="/drifted/_drift/${T}/changed1?yaml"`);
    // The analysis's links stay in the drift view, or reach the documentation.
    expect(res.text).toContain(`href="/drifted/_drift/${T}/new_name"`);
    expect(res.text).toContain(`href="/drifted/${T}/changed1"`);
    // The frontmatter is not in the body.
    expect(res.text).not.toContain('severity: high');
    // `_drift` is a representation, not a breadcrumb segment.
    expect(res.text).not.toMatch(/<span class="text-slate-500[^"]*">_drift<\/span>/);
    // The sidebar marks the resource's own item.
    expect(res.text).toMatch(
      new RegExp(`href="/drifted/${T}/changed1"[^>]*aria-current="page"`),
    );
    // The switcher's Drift entry is the current page.
    expect(res.text).toMatch(
      new RegExp(`href="/drifted/_drift/${T}/changed1"[^>]*aria-current="page"`),
    );
  });

  it('serves the verified payload highlighted and raw', async () => {
    const yaml = await get(`/drifted/_drift/${T}/changed1?yaml`).expect(200);
    expect(yaml.text).toContain('observed payload');
    const raw = await get(`/drifted/_drift/${T}/changed1?raw`)
      .expect(200)
      .expect('Content-Type', 'text/plain; charset=utf-8')
      .expect('X-Content-Type-Options', 'nosniff');
    expect(raw.text).toBe(CHANGED_PAYLOAD);
  });

  it('shows a changed finding as a line diff of baseline against observed', async () => {
    const res = await get(`/drifted/_drift/${T}/changed1?diff`).expect(200);
    expect(res.text).toContain(`resources/${T}/changed1.yaml`);
    expect(res.text).toContain(`drift/${T}/changed1.yaml`);
    expect(res.text).toContain('@@ -1,3 +1,3 @@');
    expect(res.text).toMatch(/class="diff-removed[\s\S]*? {2}enabled: true</);
    expect(res.text).toMatch(/class="diff-added[\s\S]*? {2}enabled: false</);
    expect(res.text).toMatch(/class="diff-context[\s\S]*?id: changed1</);
    expect(res.text).toContain(`href="/drifted/_resource/${T}/changed1?raw"`);
    expect(res.text).toContain(`href="/drifted/_drift/${T}/changed1?raw"`);
  });

  it('lays the drift diff out side by side, baseline left and observed right', async () => {
    const res = await get(`/drifted/_drift/${T}/changed1?diff`).expect(200);
    // One grid that reflows on its own width — a container query, not a second copy.
    expect(res.text).toContain('class="drift-diff @container');
    expect(res.text.match(/class="diff-hunk"/g)).toHaveLength(1);
    expect(res.text).toMatch(/class="diff-pane-label[^"]*">baseline</);
    expect(res.text).toMatch(/class="diff-pane-label[^"]*">observed</);
    // A modified value is one row: the removed line left, the added line right.
    expect(res.text).toMatch(
      /class="diff-row[^"]*">\s*<div class="diff-removed diff-left[\s\S]*? {2}enabled: true<[\s\S]*?<div class="diff-added diff-right[\s\S]*? {2}enabled: false</,
    );
    expect(res.text).not.toMatch(/class="diff-removed diff-right/);
    expect(res.text).not.toMatch(/class="diff-added diff-left/);
    // A context line is present on both sides, the right copy only when wide.
    expect(res.text).toMatch(/class="diff-context diff-left[\s\S]*?id: changed1<[\s\S]*?class="diff-context diff-right hidden @5xl:grid/);
  });

  it('withholds the diff when either side is missing or no longer verified', async () => {
    await get(`/drifted/_drift/${T}/tampered1?diff`).expect(404);
    // An addition has no baseline to diff against.
    const added = await get('/drifted/_drift/Microsoft.Graph/namedLocations/new_loc?diff').expect(404);
    expect(added.text).toContain('YAML diff not available');
    expect(added.text).not.toContain(root);
  });

  it("reaches a rename from the old name's page and renders it", async () => {
    const doc = await get(`/drifted/${T}/old_name`).expect(200);
    expect(doc.text).toContain(`href="/drifted/_drift/${T}/old_name"`);
    expect(doc.text).toContain('Drift · renamed</a>');
    const res = await get(`/drifted/_drift/${T}/old_name`).expect(200);
    expect(res.text).toContain('New name');
    expect(res.text).toContain('was <strong>Old name</strong>');
    expect(res.text).toContain(`href="/drifted/_drift/${T}/new_name?diff"`);
    const diff = await get(`/drifted/_drift/${T}/old_name?diff`).expect(200);
    expect(diff.text).toContain(`resources/${T}/old_name.yaml`);
    expect(diff.text).toContain(`drift/${T}/new_name.yaml`);
    expect(diff.text).toMatch(/class="diff-added[\s\S]*?displayName: New name</);
  });

  it('renders an addition with its payload, and no documentation link', async () => {
    const res = await get('/drifted/_drift/Microsoft.Graph/namedLocations/new_loc').expect(200);
    expect(res.text).toContain('Kali VPN location');
    expect(res.text).toContain('Observed payload');
    expect(res.text).toContain('kali-vpn-location');
    expect(res.text).not.toContain('>Documentation</a>');
    // No baseline, so no diff: the observed YAML is linked on its own.
    expect(res.text).toContain('href="/drifted/_drift/Microsoft.Graph/namedLocations/new_loc?yaml"');
    expect(res.text).not.toContain('?diff"');
  });

  it('withholds the comparison and the payload when a file no longer matches', async () => {
    const res = await get(`/drifted/_drift/${T}/tampered1`).expect(200);
    expect(res.text).toContain('The files on disk no longer match this observation');
    expect(res.text).not.toContain('What changed');
    expect(res.text).not.toContain('secretDelta');
    expect(res.text).not.toContain(`/drifted/_drift/${T}/tampered1?yaml`);
    await get(`/drifted/_drift/${T}/tampered1?raw`).expect(404);
    await get(`/drifted/_drift/${T}/tampered1?yaml`).expect(404);
  });

  it('links a type the run could not list and an entry it could not compare', async () => {
    const brand = await get('/drifted/Microsoft.Graph/organizationalBranding/brand1').expect(200);
    expect(brand.text).toContain(
      'href="/drifted/_drift/Microsoft.Graph/organizationalBranding/brand1"',
    );
    const brandDrift = await get(
      '/drifted/_drift/Microsoft.Graph/organizationalBranding/brand1',
    ).expect(200);
    expect(brandDrift.text).toContain('could not list');

    const unattested = await get(`/drifted/_resource/${T}/unattested1`).expect(200);
    expect(unattested.text).toContain(`href="/drifted/_drift/${T}/unattested1"`);
    const unattestedDrift = await get(`/drifted/_drift/${T}/unattested1`).expect(200);
    expect(unattestedDrift.text).toContain('per-entry config attestation');
  });

  it('adds the drift finding count and observation time to the picker line', async () => {
    const res = await get('/').expect(200);
    expect(res.text).toMatch(
      new RegExp(`exported ${BASELINE} · <span class="picker-drift[^"]*">drift detected on 4 resources ${OBSERVED}</span>`),
    );
  });

  it('renders the tenant drift page: header, analysis summary and every finding', async () => {
    const res = await get('/drifted/_drift').expect(200);
    expect(res.text).toContain(`Observed <strong>${OBSERVED}</strong>`);
    expect(res.text).toContain('Incomplete run');
    expect(res.text).toContain('Removals were suppressed');
    expect(res.text).toContain('Microsoft.Graph/organizationalBranding');
    expect(res.text).toContain('Drift analysis summary');
    // The analysis title comes first, then the observation header.
    expect(res.text.indexOf('Drift analysis summary</a></h1>')).toBeGreaterThan(-1);
    expect(res.text.indexOf('class="drift-observation')).toBeGreaterThan(
      res.text.indexOf('Drift analysis summary</a></h1>'),
    );
    expect(res.text).toContain(`href="/drifted/_drift/${T}/changed1"`);
    // A finding with no document of its own is still reachable here.
    expect(res.text).toContain('href="/drifted/_drift/Microsoft.Graph/namedLocations/new_loc"');
    expect(res.text).not.toContain('findings: 4');
    // The report's Findings table: verdicts from the closed set become icons,
    // every cell names its column, anything else stays plain text.
    expect(res.text).toContain('<table class="findings findings-drift">');
    expect(res.text).toContain(
      '<td data-column="verdict" data-verdict="changed" title="changed">changed</td>',
    );
    expect(res.text).toContain('<td data-column="verdict">shifted</td>');
    expect(res.text).toContain('<th data-column="resource">Resource</th>');
    expect(res.text).not.toContain('data-verdict="shifted"');

    const landing = await get('/drifted').expect(200);
    expect(landing.text).toContain('href="/drifted/_drift"');
  });

  it('keeps the drift tree root unreachable, and redirects index to the tenant page', async () => {
    for (const url of [
      '/drifted/_drift/metadata',
      '/drifted/_drift/metadata.yaml',
      '/drifted/_drift/metadata?raw',
      '/drifted/_drift/analyze',
      '/drifted/_drift/analyze.md',
    ]) {
      const res = await get(url).expect(404);
      expect(res.text).not.toContain(root);
    }
    await get('/drifted/_drift/index').expect(302).expect('Location', '/drifted/_drift');
    await get('/drifted/_drift/index.md').expect(302).expect('Location', '/drifted/_drift');
  });

  it('never serves a .md as a payload, and rejects traversal out of the drift root', async () => {
    for (const url of [
      `/drifted/_drift/${T}/changed1.md?raw`,
      '/drifted/_drift/..%2fresources%2fmetadata?raw',
      `/drifted/_drift/${T}/..%2f..%2f..%2fresources%2f${encodeURIComponent(T)}%2fchanged1?raw`,
    ]) {
      const res = await get(url).expect(404);
      expect(res.text).not.toContain(CHANGED_PAYLOAD);
      expect(res.text).not.toContain(root);
    }
  });

  it('writes nothing under the export while serving the drift views', async () => {
    const before = await snapshot(exportDir);
    await get('/drifted/_drift').expect(200);
    await get(`/drifted/_drift/${T}/changed1`).expect(200);
    await get(`/drifted/_drift/${T}/changed1?raw`).expect(200);
    expect(await snapshot(exportDir)).toEqual(before);
  });

  it('keeps the drift tree out of the Confluence export', async () => {
    // A summary that links into both drift routes and the drift tree itself.
    const summary = path.join(exportDir, 'docs', 'summary.md');
    await fsp.writeFile(
      summary,
      `# Summary\n\nSee [drift](/drifted/_drift), [the diff](/drifted/_drift/${T}/changed1?diff) and [the analysis](../drift/${T}/changed1.md).\n`,
    );
    try {
      const res = await get('/drifted/_export/confluence')
        .buffer()
        .parse(binaryParser)
        .expect(200);
      const entries = await readAllZipEntries(res.body as Buffer);
      // One page per indexed document plus the overview, nothing else.
      const indexed = INDEX.split('- type:').length - 1;
      expect(entries.size).toBe(indexed + 1);
      const all = [...entries.values()].join('\n');
      expect(all).toContain('See drift, the diff and the analysis.');
      expect(all).not.toContain('_drift');
      expect(all).not.toContain('drift/');
      expect(all).not.toContain('Drift analysis summary');
      expect(all).not.toContain('Drift: Changed one');
      expect(all).not.toContain('The analysis prompt');
      expect(all).not.toContain('enabled: false');
      expect(all).not.toContain('Kali VPN location');
    } finally {
      await fsp.rm(summary, { force: true });
    }
  });

  it('gates an observation whose baseline the export no longer holds, at both scopes', async () => {
    const metadata = path.join(exportDir, 'resources', 'metadata.yaml');
    await fsp.writeFile(metadata, 'generatedAt: 2026-03-03T00:00:00.5Z\n');
    try {
      const tenantPage = await get('/drifted/_drift').expect(200);
      expect(tenantPage.text).toContain('This drift observation is outdated');
      expect(tenantPage.text).not.toContain(`/drifted/_drift/${T}/changed1"`);
      // The picker dates an outdated observation but never counts it.
      const picker = await get('/').expect(200);
      expect(picker.text).toContain(`drift observation outdated ${OBSERVED}`);
      expect(picker.text).not.toContain('drift detected on');
      const page = await get(`/drifted/_drift/${T}/changed1`).expect(200);
      expect(page.text).toContain('This drift observation is outdated');
      expect(page.text).not.toContain('What changed');
      await get(`/drifted/_drift/${T}/changed1?raw`).expect(404);
    } finally {
      await fsp.writeFile(metadata, `generatedAt: "${BASELINE}"\ntenant: drifted.example\n`);
    }
    const restored = await get(`/drifted/_drift/${T}/changed1`).expect(200);
    expect(restored.text).toContain('What changed');
  });

  it('does not gate on a stale index alone', async () => {
    const indexFile = path.join(exportDir, 'docs', 'index.yaml');
    await fsp.writeFile(indexFile, INDEX.replace(`"${BASELINE}"`, '"2026-05-05T00:00:00.5Z"'));
    try {
      const page = await get(`/drifted/_drift/${T}/changed1`).expect(200);
      expect(page.text).toContain('What changed');
    } finally {
      await fsp.writeFile(indexFile, INDEX);
    }
  });

  it('reflects a rewritten or deleted drift tree on the next request', async () => {
    try {
      // An empty observation, no analysis yet: the page still links, and says so.
      await fsp.writeFile(
        path.join(driftDir, 'metadata.yaml'),
        `observedAt: "2026-02-02T00:00:00Z"\nbaseline:\n    generatedAt: "${BASELINE}"\nrun:\n    complete: true\n`,
      );
      await fsp.rm(path.join(driftDir, 'index.md'));
      const landing = await get('/drifted').expect(200);
      expect(landing.text).toContain('href="/drifted/_drift"');
      const tenantPage = await get('/drifted/_drift').expect(200);
      expect(tenantPage.text).toContain('recorded no findings');
      const picker = await get('/').expect(200);
      expect(picker.text).toContain('no drift detected 2026-02-02T00:00:00Z');

      // The next drift run or download deletes the tree wholesale.
      await fsp.rm(driftDir, { recursive: true, force: true });
      const gone = await get(`/drifted/${T}/changed1`).expect(200);
      expect(gone.text).toContain('<span aria-disabled="true" title="No drift observation.');
      const goneLanding = await get('/drifted').expect(200);
      expect(goneLanding.text).not.toContain('href="/drifted/_drift"');
      const gonePicker = await get('/').expect(200);
      expect(gonePicker.text).not.toMatch(/drift detected|no drift|drift observation/);
    } finally {
      await writeDrift();
    }
  });
});

describe('Docs browser tenant compare (e2e)', () => {
  const P = 'Microsoft.Graph/deviceManagementConfigurationPolicies';
  const GUIDS = {
    stageAdmins: '8964516b-c223-4f58-a866-232d3690c9b4',
    stageAudience: '11111111-c223-4f58-a866-232d3690c9b4',
    prodAdmins: '7a73b11f-e242-4ffe-ab1a-c4a8a70d5f64',
    prodAudience: '22222222-e242-4ffe-ab1a-c4a8a70d5f64',
    stageP1: 'c036fdcb-4fad-4596-94cb-365d2b23a016',
    stageP2: 'c036fdcb-4fad-4596-94cb-365d2b23a017',
    stageP3: 'c036fdcb-4fad-4596-94cb-365d2b23a018',
    prodP1: '866b2e5d-a549-472c-b618-3df33778ba05',
    prodP2: '866b2e5d-a549-472c-b618-3df33778ba06',
    prodP3: '866b2e5d-a549-472c-b618-3df33778ba07',
  };

  const policy = (policyId: string, groupId: string, value: number) => `'@odata.context': https://graph.microsoft.com/beta/$metadata#x('${policyId}')
id: ${policyId}
createdDateTime: "2026-03-28T16:56:52Z"
lastModifiedDateTime: "2026-03-28T16:56:52Z"
name: Same policy
assignments:
    - id: ${policyId}_${groupId}
      sourceId: ${policyId}
      target:
        groupId: ${groupId}
settings:
    - id: "0"
      value: ${value}
`;

  const index = (tenant: string, excluded = '') =>
    [
      'version: 3',
      `tenant: ${tenant}`,
      'generatedAt: "2026-01-01T00:00:00Z"',
      'complete: true',
      'counts:',
      '    documented: 0',
      '    pending: 0',
      ...(excluded ? ['    excluded:', `        ${excluded}: 1`] : []),
      'resources: []',
      '',
    ].join('\n');

  const entry = (key: string, name: string, id: string | null, present = true) =>
    [
      `    ${key}.yaml:`,
      ...(id ? [`        resourceId: ${id}`] : []),
      `        displayName: ${name}`,
      `        presentInTenant: ${present}`,
      '',
    ].join('\n');

  const metadata = (entries: string[]) =>
    ['generatedAt: "2026-01-01T00:00:00Z"', 'resources:', ...entries].join('\n');

  const STAGE_METADATA = metadata([
    entry('Microsoft.Graph/groups/admins', 'Admins', GUIDS.stageAdmins),
    entry('Microsoft.Graph/groups/stage_audience', 'Stage audience', GUIDS.stageAudience),
    entry(P + '/p1', 'Policy One', GUIDS.stageP1),
    entry(P + '/p2', 'Policy Two', GUIDS.stageP2),
    entry(P + '/p3', 'Policy Three', GUIDS.stageP3),
    entry('Microsoft.Graph/deviceConfigurations/dc1', 'Stage only config', null),
    entry('Microsoft.Graph/deviceConfigurations/gone', 'Gone config', null, false),
    entry('Microsoft.Graph/windowsAutopilotDeviceIdentities/dev1', 'Device one', null),
  ]);

  const PROD_METADATA = metadata([
    entry('Microsoft.Graph/groups/admins', 'Admins', GUIDS.prodAdmins),
    entry('Microsoft.Graph/groups/prod_audience', 'Prod audience', GUIDS.prodAudience),
    entry(P + '/p1', 'Policy One', GUIDS.prodP1),
    entry(P + '/p2', 'Policy Two', GUIDS.prodP2),
    entry(P + '/p3', 'Policy Three (prod)', GUIDS.prodP3),
    entry('Microsoft.Graph/namedLocations/loc1', 'Prod only location', null),
  ]);

  let app: NestExpressApplication;
  let root: string;

  const write = async (rel: string, content: string) => {
    const file = path.join(root, rel);
    await fsp.mkdir(path.dirname(file), { recursive: true });
    await fsp.writeFile(file, content);
  };
  const get = (url: string) => request(app.getHttpServer()).get(url);
  const q = '?a=stage&b=prod';
  // An href as Handlebars writes it into an attribute: `&` and `=` escaped.
  const href = (url: string) =>
    `href="${url.replace(/&/g, '&amp;').replace(/=/g, '&#x3D;')}"`;

  beforeAll(async () => {
    root = await fsp.mkdtemp(path.join(os.tmpdir(), 'docsroot-compare-'));
    await write('stage/docs/index.yaml', index('Stage', 'Microsoft.Graph/windowsAutopilotDeviceIdentities'));
    await write('stage/resources/metadata.yaml', STAGE_METADATA);
    await write('stage/resources/Microsoft.Graph/groups/admins.yaml', 'displayName: Admins\nmail: a@stage\n');
    await write('stage/resources/Microsoft.Graph/groups/stage_audience.yaml', 'displayName: Stage audience\n');
    await write(`stage/resources/${P}/p1.yaml`, policy(GUIDS.stageP1, GUIDS.stageAdmins, 1));
    await write(`stage/resources/${P}/p2.yaml`, policy(GUIDS.stageP2, GUIDS.stageAudience, 1));
    await write(`stage/resources/${P}/p3.yaml`, policy(GUIDS.stageP3, GUIDS.stageAdmins, 1));
    await write('stage/resources/Microsoft.Graph/deviceConfigurations/dc1.yaml', 'displayName: Stage only config\n');
    await write('stage/resources/Microsoft.Graph/deviceConfigurations/gone.yaml', 'displayName: Gone config\n');

    await write('prod/docs/index.yaml', index('Prod'));
    await write('prod/resources/metadata.yaml', PROD_METADATA);
    await write('prod/resources/Microsoft.Graph/groups/admins.yaml', 'displayName: Admins\nmail: a@prod\n');
    await write('prod/resources/Microsoft.Graph/groups/prod_audience.yaml', 'displayName: Prod audience\n');
    await write(`prod/resources/${P}/p1.yaml`, policy(GUIDS.prodP1, GUIDS.prodAdmins, 15));
    await write(`prod/resources/${P}/p2.yaml`, policy(GUIDS.prodP2, GUIDS.prodAudience, 1));
    await write(`prod/resources/${P}/p3.yaml`, policy(GUIDS.prodP3, GUIDS.prodAdmins, 1));

    // A docs-only copy: a tenant, but not comparable.
    await write('docsonly/docs/index.yaml', index('Docs only'));
    // A housekeeping folder that looks like an export: never a tenant, so
    // never comparable either.
    await write('_hidden/docs/index.yaml', index('Hidden'));
    await write('_hidden/resources/metadata.yaml', PROD_METADATA);

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

  it('offers Compare with… only on cards whose export has resources/metadata.yaml', async () => {
    const res = await get('/').expect(200);
    expect(res.text).toContain(href(`/_compare?a=stage`));
    expect(res.text).toContain(href(`/_compare?a=prod`));
    expect(res.text).not.toContain('a&#x3D;docsonly');
    expect(res.text).not.toContain('compare-selecting');
  });

  it('renders the picker in its selecting state for a alone', async () => {
    const res = await get('/_compare?a=stage').expect(200);
    expect(res.text).toContain('Comparing <strong>Stage</strong>');
    expect(res.text).toContain('class="compare-cancel');
    expect(res.text).toContain('compare-selected');
    expect(res.text).toContain(href(`/_compare?a=stage&b=prod`));
    expect(res.text).toContain('Compare with Stage</a>');
    expect(res.text).not.toContain('b&#x3D;docsonly');
    expect(res.text).not.toContain('b&#x3D;stage');
    expect(res.text).not.toContain('compare-start');
  });

  it('redirects a bare /_compare to the picker', async () => {
    const res = await get('/_compare').expect(302);
    expect(res.headers.location).toBe('/');
  });

  it.each([
    ['the same tenant twice', '/_compare?a=stage&b=stage'],
    ['an unknown tenant', '/_compare?a=nobody&b=prod'],
    ['an unknown first tenant alone', '/_compare?a=nobody'],
    ['a tenant without resources/metadata.yaml', '/_compare?a=docsonly&b=prod'],
    ['a _-prefixed folder', '/_compare?a=_hidden&b=prod'],
    ['a repeated parameter', '/_compare?a=stage&a=prod&b=prod'],
  ])('refuses %s without leaking a path', async (_label, url) => {
    const res = await get(url).expect(404);
    expect(res.text).toContain('Tenant comparison not available');
    expect(res.text).not.toContain(root);
  });

  it('lists the three-way split from the two metadata files', async () => {
    const res = await get(`/_compare${q}`).expect(200);
    expect(res.text).toContain('4 in both · 3 only in stage · 2 only in prod');
    // A resource the export kept after it left the tenant is not listed.
    expect(res.text).not.toContain('Gone config');
    // Paired rows link to their diff, single-side rows to that tenant's YAML.
    expect(res.text).toContain(href(`/_compare/${P}/p1?a=stage&b=prod`));
    expect(res.text).toContain('href="/stage/_resource/Microsoft.Graph/deviceConfigurations/dc1"');
    expect(res.text).toContain('href="/prod/_resource/Microsoft.Graph/namedLocations/loc1"');
    // An excluded type comes last and is marked.
    expect(res.text.indexOf('Microsoft.Graph/deviceConfigurations')).toBeLessThan(
      res.text.indexOf('Microsoft.Graph/windowsAutopilotDeviceIdentities'),
    );
    expect(res.text).toMatch(/compare-excluded[\s\S]*?Microsoft\.Graph\/windowsAutopilotDeviceIdentities/);
    expect(res.text).toContain('0 in both · 1 only left · 0 only right');
    expect(res.text).toContain(href(`/_compare?a=prod&b=stage`));
    // `_compare` is a representation, not a breadcrumb segment.
    expect(res.text).not.toMatch(/<span class="text-slate-500[^"]*">_compare<\/span>/);
  });

  it('lists the two exports side by side, one row per key', async () => {
    const res = await get(`/_compare${q}`).expect(200);
    const rows = res.text.split('<tr class="compare-row').slice(1).map((r) => r.slice(0, r.indexOf('</tr>')));
    // A pair is one row: each side's own display name, both linking to the pair diff.
    const three = rows.find((r) => r.includes('Policy Three (prod)'));
    expect(three).toBeDefined();
    expect(three).toContain(' compare-paired');
    expect(three).toMatch(/>Policy Three<\/a>/);
    expect(three!.split(href(`/_compare/${P}/p3?a=stage&b=prod`)).length - 1).toBe(2);
    // A single-side key has one cell and an empty one opposite.
    const onlyLeft = rows.find((r) => r.includes('Stage only config'));
    expect(onlyLeft).toContain(' compare-only-left');
    expect(onlyLeft).toMatch(/Stage only config[\s\S]*class="compare-empty/);
    const onlyRight = rows.find((r) => r.includes('Prod only location'));
    expect(onlyRight).toContain(' compare-only-right');
    expect(onlyRight).toMatch(/class="compare-empty[\s\S]*Prod only location/);
    // Pairs lead each type, and the columns are headed by the tenant ids.
    expect(res.text).toMatch(/<th[^>]*>stage<\/th>\s*<th[^>]*>prod<\/th>/);
    expect(res.text).not.toContain('id="compare-only-a"');
  });

  it('diffs a pair with ids and timestamps normalised away and references resolved', async () => {
    const res = await get(`/_compare/${P}/p1${q}`).expect(200);
    expect(res.text).toMatch(/class="diff-removed[\s\S]*?value: 1</);
    expect(res.text).toMatch(/class="diff-added[\s\S]*?value: 15</);
    // The same audience on both sides: resolved, so not part of the diff.
    expect(res.text).toContain('references resolved to names\n          1 left, 1 right');
    expect(res.text).not.toContain('groupId:');
    for (const guid of [GUIDS.stageP1, GUIDS.prodP1, GUIDS.stageAdmins, GUIDS.prodAdmins]) {
      expect(res.text).not.toContain(guid);
    }
    expect(res.text).not.toContain('2026-03-28T16:56:52Z');
    expect(res.text).toContain('class="compare-report');
    expect(res.text).toContain('createdDateTime');
    expect(res.text).not.toContain('Differs only in audience');
    expect(res.text).toContain(href(`/_compare/${P}/p1?a=stage&b=prod&raw`));
    expect(res.text).toContain(`href="/stage/_resource/${P}/p1"`);
    // Side by side, headed by the two tenant ids; the modified value is one row.
    expect(res.text).toMatch(/class="diff-pane-label[^"]*">stage</);
    expect(res.text).toMatch(/class="diff-pane-label[^"]*">prod</);
    expect(res.text).toMatch(/class="diff-row[^"]*">\s*<div class="diff-removed diff-left[\s\S]*?value: 1<[\s\S]*?<div class="diff-added diff-right[\s\S]*?value: 15</);
  });

  it('shows the raw diff with the identities back', async () => {
    const res = await get(`/_compare/${P}/p1${q}&raw`).expect(200);
    expect(res.text).toContain(GUIDS.stageP1);
    expect(res.text).toContain(GUIDS.prodP1);
    expect(res.text).toContain('The files as exported, without normalisation.');
    expect(res.text).toContain(href(`/_compare/${P}/p1?a=stage&b=prod`));
  });

  it('says when a pair differs only in audience', async () => {
    const res = await get(`/_compare/${P}/p2${q}`).expect(200);
    expect(res.text).toContain('Differs only in audience');
    expect(res.text).toContain('groupId: Stage audience');
    expect(res.text).toContain('groupId: Prod audience');
  });

  it('says when a pair is identical after normalisation', async () => {
    const res = await get(`/_compare/${P}/p3.yaml${q}`).expect(200);
    expect(res.text).toContain('No differences after normalisation.');
    expect(res.text).not.toContain('class="drift-diff ');
  });

  it.each([
    ['a traversal', `/_compare/..%2F..%2Fstage%2Fdocs%2Findex${q}`],
    ['a key only one export lists', `/_compare/Microsoft.Graph/deviceConfigurations/dc1${q}`],
    ['a key no longer present in the tenant', `/_compare/Microsoft.Graph/deviceConfigurations/gone${q}`],
    ['the metadata file itself', `/_compare/metadata${q}`],
  ])('refuses %s as a resource comparison without leaking a path', async (_label, url) => {
    const res = await get(url).expect(404);
    expect(res.text).toContain('Resource comparison not available');
    expect(res.text).not.toContain(root);
  });

  it('refuses a pair diff for an ineligible tenant', async () => {
    const res = await get(`/_compare/${P}/p1?a=stage&b=docsonly`).expect(404);
    expect(res.text).toContain('Tenant comparison not available');
  });

  it('reflects an edited resource and an edited metadata file on the next request', async () => {
    const p3 = path.join(root, `prod/resources/${P}/p3.yaml`);
    const metadata = path.join(root, 'prod/resources/metadata.yaml');
    try {
      await fsp.writeFile(p3, policy(GUIDS.prodP3, GUIDS.prodAdmins, 20));
      const diff = await get(`/_compare/${P}/p3${q}`).expect(200);
      expect(diff.text).not.toContain('No differences after normalisation.');
      expect(diff.text).toMatch(/class="diff-added[\s\S]*?value: 20</);

      await write('prod/resources/Microsoft.Graph/deviceConfigurations/dc1.yaml', 'displayName: Stage only config\n');
      await fsp.writeFile(
        metadata,
        PROD_METADATA + entry('Microsoft.Graph/deviceConfigurations/dc1', 'Now in prod', null),
      );
      const listing = await get(`/_compare${q}`).expect(200);
      expect(listing.text).toContain('5 in both · 2 only in stage · 2 only in prod');
    } finally {
      await fsp.writeFile(p3, policy(GUIDS.prodP3, GUIDS.prodAdmins, 1));
      await fsp.writeFile(metadata, PROD_METADATA);
    }
  });

  it('writes nothing under the docs root', async () => {
    const before = await snapshot(root);
    await get('/').expect(200);
    await get('/_compare?a=stage').expect(200);
    await get(`/_compare${q}`).expect(200);
    await get(`/_compare/${P}/p1${q}`).expect(200);
    await get(`/_compare/${P}/p1${q}&raw`).expect(200);
    expect(await snapshot(root)).toEqual(before);
  });
});

// Reads one entry, matched by the tail of its name, out of a zip in memory.
function readZipEntry(zip: Buffer, suffix: string): Promise<string> {
  return new Promise((resolve, reject) => {
    yauzl.fromBuffer(zip, { lazyEntries: true }, (err, file) => {
      if (err || !file) return reject(err ?? new Error('unreadable zip'));
      file.on('entry', (entry) => {
        if (!entry.fileName.endsWith(suffix)) return file.readEntry();
        file.openReadStream(entry, (streamErr, stream) => {
          if (streamErr || !stream) {
            return reject(streamErr ?? new Error('unreadable entry'));
          }
          const chunks: Buffer[] = [];
          stream.on('data', (chunk: Buffer) => chunks.push(chunk));
          stream.on('end', () =>
            resolve(Buffer.concat(chunks).toString('utf8')),
          );
          stream.on('error', reject);
        });
      });
      file.on('end', () => reject(new Error(`no ${suffix} in the archive`)));
      file.readEntry();
    });
  });
}

// Every entry of a zip in memory, by name.
function readAllZipEntries(zip: Buffer): Promise<Map<string, string>> {
  return new Promise((resolve, reject) => {
    const out = new Map<string, string>();
    yauzl.fromBuffer(zip, { lazyEntries: true }, (err, file) => {
      if (err || !file) return reject(err ?? new Error('unreadable zip'));
      file.on('entry', (entry) => {
        file.openReadStream(entry, (streamErr, stream) => {
          if (streamErr || !stream) {
            return reject(streamErr ?? new Error('unreadable entry'));
          }
          const chunks: Buffer[] = [];
          stream.on('data', (chunk: Buffer) => chunks.push(chunk));
          stream.on('end', () => {
            out.set(entry.fileName, Buffer.concat(chunks).toString('utf8'));
            file.readEntry();
          });
          stream.on('error', reject);
        });
      });
      file.on('end', () => resolve(out));
      file.readEntry();
    });
  });
}

// superagent has no parser for application/zip, so collect the raw bytes.
function binaryParser(res: any, cb: any): void {
  const chunks: Buffer[] = [];
  res.on('data', (chunk: Buffer) => chunks.push(Buffer.from(chunk)));
  res.on('end', () => cb(null, Buffer.concat(chunks)));
}

// Every file under `dir` with its size and mtime, for asserting that a request
// wrote nothing.
async function snapshot(dir: string): Promise<string[]> {
  const out: string[] = [];
  const walk = async (current: string): Promise<void> => {
    const entries = await fsp.readdir(current, { withFileTypes: true });
    for (const entry of entries.sort((a, b) => a.name.localeCompare(b.name))) {
      const full = path.join(current, entry.name);
      if (entry.isDirectory()) {
        await walk(full);
      } else {
        const stat = await fsp.stat(full);
        out.push(`${path.relative(dir, full)}:${stat.size}:${stat.mtimeMs}`);
      }
    }
  };
  await walk(dir);
  return out;
}

import { promises as fsp } from 'fs';
import * as os from 'os';
import * as path from 'path';
import { Test } from '@nestjs/testing';
import { NestExpressApplication } from '@nestjs/platform-express';
import request from 'supertest';
import { AppModule } from '../src/app.module';
import { configureViews } from '../src/configure-app';

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
      tenants: 3,
      documents: 4,
      pending: 2,
    });
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
  });

  it('GET /:tenant falls back to the index listing when there is no summary.md', async () => {
    const res = await request(app.getHttpServer())
      .get('/nosummary')
      .expect(200);
    expect(res.text).toContain('listing the index instead');
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

  it('404s for a missing resource, without leaking a filesystem path', async () => {
    const res = await request(app.getHttpServer())
      .get('/mytenant/_resource/Microsoft.Graph/groups/g1')
      .expect(404);
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

  it('404s an unknown export format and an unknown tenant', async () => {
    await request(app.getHttpServer())
      .get('/mytenant/_export/docx')
      .expect(404);
    await request(app.getHttpServer())
      .get('/nosuchtenant/_export/confluence')
      .expect(404);
  });
});

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

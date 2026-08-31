import {
  buildPageNames,
  pageTitle,
  sanitizeTitle,
  typeLeaf,
} from '../src/docs/export/page-name';
import {
  rewriteExportHref,
  toConfluenceHtml,
} from '../src/docs/export/html-allowlist';
import {
  buildExportPlan,
  documentPage,
  overviewPage,
  spaceName,
} from '../src/docs/export/confluence';
import {
  DEFAULT_DETAILS_STRATEGY,
  parseDetailsStrategy,
} from '../src/docs/export/details-strategy';
import { parseTenantIndex, TenantIndex } from '../src/docs/tenant-index';

// The Confluence exporter's pure modules: page naming (which decides what every
// link in the export points at), the allowlist serialiser (the only thing
// standing between operator-supplied prose and the importer) and the format.

const INDEX_YAML = `version: 1
tenant: contoso.onmicrosoft.com
generatedAt: "2026-01-01T00:00:00Z"
complete: true
counts:
    documented: 2
    pending: 0
    excluded:
        Microsoft.Graph/windowsAutopilotDeviceIdentities: 4
resources:
    - type: Microsoft.Graph/deviceManagementConfigurationPolicies
      doc: Microsoft.Graph/deviceManagementConfigurationPolicies/p1.md
      displayName: Policy One
      summary: A firewall policy.
      documented: true
    - type: Microsoft.Graph/groups
      doc: Microsoft.Graph/groups/g1.md
      displayName: Admins
      documented: true
`;

function index(): TenantIndex {
  const parsed = parseTenantIndex(INDEX_YAML);
  if (!parsed) throw new Error('fixture index must parse');
  return parsed;
}

function options(pages: Record<string, string> = {}) {
  return {
    tenant: 'mytenant',
    pageFileByDoc: new Map(Object.entries(pages)),
    details: DEFAULT_DETAILS_STRATEGY,
  };
}

describe('page names', () => {
  it('takes the last segment of a resource type as the title prefix', () => {
    expect(typeLeaf('Microsoft.Graph/groups')).toBe('groups');
    expect(typeLeaf('')).toBe('');
  });

  it('replaces characters that are illegal in a file name or a page title', () => {
    expect(sanitizeTitle('Win10/11: baseline *v2*?')).toBe(
      'Win10-11- baseline -v2-',
    );
    // A run of illegal characters collapses to one dash.
    expect(sanitizeTitle('a<<>>b')).toBe('a-b');
  });

  it('strips leading and trailing dots and collapses whitespace', () => {
    expect(sanitizeTitle('  .hidden   name.  ')).toBe('hidden name');
  });

  it('truncates to stay under the Confluence title limit', () => {
    expect(sanitizeTitle('x'.repeat(400)).length).toBe(200);
  });

  it('prefers the index display name, then the H1, then the file name', () => {
    const source = {
      doc: 'Microsoft.Graph/groups/g1-a1b2.md',
      type: 'Microsoft.Graph/groups',
      displayName: 'Admins',
      h1: 'Ignored',
    };
    expect(pageTitle(source)).toBe('groups — Admins');
    expect(pageTitle({ ...source, displayName: '' })).toBe('groups — Ignored');
    expect(pageTitle({ ...source, displayName: '', h1: '' })).toBe(
      'groups — g1-a1b2',
    );
  });

  it('deduplicates colliding titles deterministically instead of overwriting', () => {
    const names = buildPageNames([
      {
        doc: 'Microsoft.Graph/groups/b.md',
        type: 'Microsoft.Graph/groups',
        displayName: 'Windows:baseline',
      },
      {
        doc: 'Microsoft.Graph/groups/a.md',
        type: 'Microsoft.Graph/groups',
        displayName: 'Windows/baseline',
      },
      {
        doc: 'Microsoft.Graph/groups/c.md',
        type: 'Microsoft.Graph/groups',
        displayName: 'windows-baseline',
      },
    ]);
    // All three sanitise to the same title; document path order decides.
    expect(names.get('Microsoft.Graph/groups/a')?.file).toBe(
      'groups — Windows-baseline.html',
    );
    expect(names.get('Microsoft.Graph/groups/b')?.file).toBe(
      'groups — Windows-baseline (2).html',
    );
    // The collision check ignores case, because Confluence titles and
    // case-insensitive filesystems do.
    expect(names.get('Microsoft.Graph/groups/c')?.title).toBe(
      'groups — windows-baseline (3)',
    );
  });

  it('falls back to a placeholder rather than producing an empty file name', () => {
    const names = buildPageNames([{ doc: '?.md', type: '', displayName: '' }]);
    expect([...names.values()][0].file).toBe('Untitled.html');
  });
});

describe('the export href rewrite', () => {
  it('maps an app route to the page file it became', () => {
    const opts = options({ 'Microsoft.Graph/groups/g1': 'groups — Admins.html' });
    expect(
      rewriteExportHref('/mytenant/Microsoft.Graph/groups/g1', opts),
    ).toBe('groups — Admins.html');
  });

  it('drops in-document anchors, other representations and unknown targets', () => {
    const opts = options({ 'Microsoft.Graph/groups/g1': 'g.html' });
    expect(rewriteExportHref('#security', opts)).toBeNull();
    expect(
      rewriteExportHref('/mytenant/_resource/Microsoft.Graph/groups/g1', opts),
    ).toBeNull();
    expect(rewriteExportHref('/othertenant/x', opts)).toBeNull();
    expect(rewriteExportHref('/mytenant/Microsoft.Graph/groups/gone', opts)).toBeNull();
  });

  it('keeps external links verbatim', () => {
    const opts = options();
    expect(rewriteExportHref('https://learn.microsoft.com/x', opts)).toBe(
      'https://learn.microsoft.com/x',
    );
    expect(rewriteExportHref('//example.com/x', opts)).toBe('//example.com/x');
  });
});

describe('the allowlist serialiser', () => {
  it('escapes a bare angle bracket in prose instead of emitting an element', () => {
    // macOS plist payloads are quoted with bare angle brackets, which
    // `html: true` already turns into phantom elements in the browser.
    const html = toConfluenceHtml('<p>The <key> element holds it.</p>', options());
    expect(html).toBe('<p>The &lt;key&gt; element holds it.</p>');
  });

  it('escapes a closing pseudo-element it actually saw, and invents none', () => {
    expect(toConfluenceHtml('<p><string>x</string></p>', options())).toBe(
      '<p>&lt;string&gt;x&lt;/string&gt;</p>',
    );
    expect(toConfluenceHtml('<p><key>x</p>', options())).toBe(
      '<p>&lt;key&gt;x</p>',
    );
  });

  it('keeps the author casing of an escaped pseudo-element', () => {
    expect(toConfluenceHtml('<p><PayloadUUID></p>', options())).toBe(
      '<p>&lt;PayloadUUID&gt;</p>',
    );
  });

  it('unwraps real HTML the importer does not preserve but keeps its text', () => {
    expect(
      toConfluenceHtml('<div class="x"><p>kept</p></div>', options()),
    ).toBe('<p>kept</p>');
  });

  it('unwraps the heading permalink markdown-it-anchor adds', () => {
    const html = toConfluenceHtml(
      '<h2 id="security"><a class="header-anchor" href="#security">Security</a></h2>',
      options(),
    );
    expect(html).toBe('<h2>Security</h2>');
  });

  it('drops elements whose content must not travel', () => {
    expect(
      toConfluenceHtml('<p>a</p><script>alert(1)</script><p>b</p>', options()),
    ).toBe('<p>a</p><p>b</p>');
  });

  it('drops attributes that are not on an element allowlist', () => {
    expect(
      toConfluenceHtml('<td colspan="2" class="x" id="y">v</td>', options()),
    ).toBe('<td colspan="2">v</td>');
  });

  it('replaces an image with its alt text, because media is not exported', () => {
    expect(
      toConfluenceHtml('<p><img src="a.png" alt="A diagram" /></p>', options()),
    ).toBe('<p>A diagram</p>');
  });

  it('rewrites a document link and degrades one that has no page', () => {
    const opts = options({ 'Microsoft.Graph/groups/g1': 'groups — Admins.html' });
    expect(
      toConfluenceHtml(
        '<p>See <a href="/mytenant/Microsoft.Graph/groups/g1">Admins</a>.</p>',
        opts,
      ),
    ).toBe('<p>See <a href="groups — Admins.html">Admins</a>.</p>');
    expect(
      toConfluenceHtml(
        '<p>See <a href="/mytenant/Microsoft.Graph/groups/gone">Gone</a>.</p>',
        opts,
      ),
    ).toBe('<p>See Gone.</p>');
  });

  it('escapes text exactly once', () => {
    expect(toConfluenceHtml('<p>a &amp; b &lt;c&gt;</p>', options())).toBe(
      '<p>a &amp; b &lt;c&gt;</p>',
    );
  });
});

// One shared fixture for the `<details>` question: a nested block, a group-label
// block with no value, a body with a link, and a value that itself contains
// ` = `. Passthrough must leave the structure alone; a transform lands here.
const DETAILS_FIXTURE =
  '<details>\n' +
  '<summary><code>firewall/enabled = true</code></summary>\n' +
  '<p>Blocks inbound traffic. See <a href="/mytenant/Microsoft.Graph/groups/g1">Admins</a>.</p>\n' +
  '<details>\n' +
  '<summary>Sub-options (1)</summary>\n' +
  '<details>\n' +
  '<summary><code>rule/name = allow = deny</code></summary>\n' +
  '<p>deep value</p>\n' +
  '</details>\n' +
  '</details>\n' +
  '</details>';

describe('the details strategy', () => {
  it('defaults to passthrough and rejects a strategy that does not exist', () => {
    expect(parseDetailsStrategy(undefined)).toBe('passthrough');
    expect(parseDetailsStrategy('')).toBe('passthrough');
    expect(parseDetailsStrategy('headings')).toBeNull();
  });

  it('passes the block through, nesting and summaries verbatim', () => {
    const html = toConfluenceHtml(
      DETAILS_FIXTURE,
      options({ 'Microsoft.Graph/groups/g1': 'groups — Admins.html' }),
    );
    // Three blocks in, three blocks out, at the same depth.
    expect(html.match(/<details>/g)).toHaveLength(3);
    expect(html.match(/<\/details>/g)).toHaveLength(3);
    // The `path = value` summary is not parsed, so a value containing ` = `
    // cannot be mangled.
    expect(html).toContain('<code>rule/name = allow = deny</code>');
    // A group-label block with no value keeps its label.
    expect(html).toContain('<summary>Sub-options (1)</summary>');
    // Links inside a block are rewritten like any other.
    expect(html).toContain('<a href="groups — Admins.html">Admins</a>');
  });
});

describe('the Confluence format', () => {
  it('names the space after the tenant domain', () => {
    expect(spaceName('contoso.onmicrosoft.com')).toBe(
      'contoso.onmicrosoft.com documentation',
    );
    expect(spaceName('')).toBe('Tenant documentation');
  });

  it('plans one page per indexed resource, in document path order', () => {
    const plan = buildExportPlan(index());
    expect(plan.space).toBe('contoso.onmicrosoft.com documentation');
    expect(plan.pages.map((p) => p.file)).toEqual([
      'deviceManagementConfigurationPolicies — Policy One.html',
      'groups — Admins.html',
    ]);
    expect(
      plan.pageFileByDoc.get('Microsoft.Graph/groups/g1'),
    ).toBe('groups — Admins.html');
  });

  it('uses the document H1 when the index has no display name', () => {
    const src = index();
    src.resources[1].displayName = '';
    const plan = buildExportPlan(
      src,
      new Map([['Microsoft.Graph/groups/g1', 'All admins']]),
    );
    expect(plan.pages.map((p) => p.title)).toContain('groups — All admins');
  });

  it('emits the provenance the stripped frontmatter carried', () => {
    const page = documentPage({
      title: 'groups — Admins',
      bodyHtml: '<h1>Admins</h1>',
      meta: { source: 'g1.yaml', sourceSha256: 'ee11' },
      docPath: 'Microsoft.Graph/groups/g1',
    });
    expect(page).toContain('<code>g1.yaml</code>');
    expect(page).toContain('<code>ee11</code>');
    expect(page).toContain('docs/Microsoft.Graph/groups/g1.md');
    expect(page).toContain('edits made in Confluence are lost');
    expect(page).toContain('<h1>Admins</h1>');
  });

  it('replaces the sidebar with a grouped link list and reports what was skipped', () => {
    const plan = buildExportPlan(index());
    const html = overviewPage({
      tenantName: 'contoso.onmicrosoft.com',
      index: index(),
      pages: plan.pages.filter((p) => p.type === 'Microsoft.Graph/groups'),
      summaryHtml: '<p>A large estate.</p>',
      skipped: plan.pages.filter((p) => p.type !== 'Microsoft.Graph/groups'),
    });
    expect(html).toContain('<h3>groups</h3>');
    expect(html).toContain('<a href="groups — Admins.html">groups — Admins</a>');
    expect(html).toContain('<p>A large estate.</p>');
    expect(html).toContain('one-way publish');
    expect(html).toContain('Not exported');
    expect(html).toContain(
      'Microsoft.Graph/deviceManagementConfigurationPolicies/p1.md',
    );
    // Excluded bulk types stay counts, never a page.
    expect(html).toContain(
      'Microsoft.Graph/windowsAutopilotDeviceIdentities (4)',
    );
  });
});

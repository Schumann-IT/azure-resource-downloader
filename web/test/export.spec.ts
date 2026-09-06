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
  countMatching,
  parseTenantIndex,
  TenantIndex,
} from '../src/docs/tenant-index';
import {
  ExportIndexMode,
  parseExportIndexMode,
} from '../src/docs/export/export-index-mode';

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

// The same two resources with a taxonomy: two axes in header order, a declared
// value this tenant matched to nothing (`vpn`), a multi-valued resource and a
// resource carrying no `platform` at all, so the uncategorised bucket is real.
const FACETED_INDEX_YAML = `version: 3
tenant: contoso.onmicrosoft.com
generatedAt: "2026-01-01T00:00:00Z"
complete: true
counts:
    documented: 2
    pending: 0
facets:
    - id: programme
      label: Programme
      values:
        - id: firewall
          label: Firewall
          count: 1
        - id: hardening
          label: Hardening
          count: 2
        - id: vpn
          label: VPN
          count: 0
    - id: platform
      label: Platform
      values:
        - id: windows
          label: Windows
          count: 1
resources:
    - type: Microsoft.Graph/deviceManagementConfigurationPolicies
      doc: Microsoft.Graph/deviceManagementConfigurationPolicies/p1.md
      displayName: Policy One
      documented: true
      facets:
        platform:
            - windows
        programme:
            - firewall
            - hardening
    - type: Microsoft.Graph/groups
      doc: Microsoft.Graph/groups/g1.md
      displayName: Admins
      documented: true
      facets:
        programme:
            - hardening
`;

function facetedIndex(): TenantIndex {
  const parsed = parseTenantIndex(FACETED_INDEX_YAML);
  if (!parsed) throw new Error('fixture index must parse');
  return parsed;
}

// The overview page of a whole tenant, under one index mode.
function overview(src: TenantIndex, indexMode?: ExportIndexMode): string {
  const plan = buildExportPlan(src);
  return overviewPage({
    tenantName: src.tenant,
    index: src,
    pages: plan.pages,
    summaryHtml: null,
    skipped: [],
    indexMode,
  });
}

function options(pages: Record<string, string> = {}) {
  return {
    tenant: 'mytenant',
    pageFileByDoc: new Map(Object.entries(pages)),
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

  it('carries the section styling hooks through as plain structure', () => {
    // The browser's section hooks are a stylesheet concern only: the marker
    // wrapper unwraps and the heading's class/data-section are not on any
    // element allowlist, so an export is unaffected by them.
    expect(
      toConfluenceHtml(
        '<div class="doc-assignments"><p>Assigned.</p></div>',
        options(),
      ),
    ).toBe('<p>Assigned.</p>');
    expect(
      toConfluenceHtml(
        '<h2 id="security" class="doc-section-heading" data-section="security">Security</h2>',
        options(),
      ),
    ).toBe('<h2>Security</h2>');
    expect(
      toConfluenceHtml('<table class="doc-metadata"><tr><td>v</td></tr></table>', options()),
    ).toContain('<table>');
    // The section wrapper unwraps too, so the export is flat as before.
    expect(
      toConfluenceHtml(
        '<section class="doc-section" data-section="settings"><p>kept</p></section>',
        options(),
      ),
    ).toBe('<p>kept</p>');
    // The generator's own setting attributes are not on the <details> allowlist.
    expect(
      toConfluenceHtml(
        '<details data-setting="a.b" data-note="security"><summary>s</summary></details>',
        options(),
      ),
    ).toBe('<details><summary>s</summary></details>');
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

// One shared fixture for the settings block: a nested block, a group-label block
// with no value, a body with a link, and a value that itself contains ` = `.
// Confluence's importer turns the block into a native collapsible expand, so the
// serialiser must leave the structure alone — including the summary text, which
// is never parsed into key/value and so cannot be mangled.
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

describe('settings blocks', () => {
  it('are exported verbatim, nesting and summaries included', () => {
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

describe('the export index mode', () => {
  it('maps each id, and anything else, to a mode', () => {
    expect(parseExportIndexMode('type')).toBe('type');
    expect(parseExportIndexMode('both')).toBe('both');
    expect(parseExportIndexMode('axis')).toBe('axis');
    expect(parseExportIndexMode(' AXIS ')).toBe('axis');
    // Unset, empty or a typo lands on the mode that changes nothing rather than
    // failing the export.
    expect(parseExportIndexMode(undefined)).toBe('type');
    expect(parseExportIndexMode('')).toBe('type');
    expect(parseExportIndexMode('by-axis')).toBe('type');
    expect(parseExportIndexMode(3)).toBe('type');
  });
});

describe('the axis index on the overview page', () => {
  it('emits no axis section by default, however rich the taxonomy', () => {
    const html = overview(facetedIndex());
    expect(html).toContain('<h2>Pages</h2>');
    expect(html).not.toContain('<h2>Programme</h2>');
    expect(html).not.toContain('<details>');
    // The default is today's export, so the mode passed explicitly agrees.
    expect(html).toBe(overview(facetedIndex(), 'type'));
  });

  it('adds axis sections after the by-type list under `both`', () => {
    const html = overview(facetedIndex(), 'both');
    expect(html.indexOf('<h2>Pages</h2>')).toBeLessThan(
      html.indexOf('<h2>Programme</h2>'),
    );
    // Axes and values in header order, labels read from the header.
    expect(html.indexOf('<h2>Programme</h2>')).toBeLessThan(
      html.indexOf('<h2>Platform</h2>'),
    );
    expect(
      [...html.matchAll(/<summary>([^<]+)<\/summary>/g)].map((m) => m[1]),
    ).toEqual([
      'Firewall (1)',
      'Hardening (2)',
      'VPN (0)',
      'Uncategorised (0)',
      'Windows (1)',
      'Uncategorised (1)',
    ]);
    // Said once per axis, not once per page.
    expect(
      html.split('A page can appear under more than one value').length - 1,
    ).toBe(2);
  });

  it('drops the by-type list under `axis`', () => {
    const html = overview(facetedIndex(), 'axis');
    expect(html).not.toContain('<h2>Pages</h2>');
    expect(html).not.toContain('<h3>groups</h3>');
    expect(html).toContain('<h2>Programme</h2>');
  });

  it('lists a multi-valued resource under each of its values', () => {
    const html = overview(facetedIndex(), 'axis');
    const firewall = section(html, 'Firewall (1)');
    const hardening = section(html, 'Hardening (2)');
    expect(firewall).toContain('deviceManagementConfigurationPolicies — Policy One');
    expect(hardening).toContain('deviceManagementConfigurationPolicies — Policy One');
    expect(hardening).toContain('groups — Admins');
  });

  it('counts the links it renders, which is what the sidebar counts too', () => {
    const src = facetedIndex();
    const html = overview(src, 'axis');
    for (const [summary, axis, value] of [
      ['Firewall (1)', 'programme', 'firewall'],
      ['Hardening (2)', 'programme', 'hardening'],
      ['VPN (0)', 'programme', 'vpn'],
      ['Windows (1)', 'platform', 'windows'],
    ] as const) {
      const links = section(html, summary).split('<li>').length - 1;
      // The number in the summary is the number of links beneath it, so a
      // collapsed section never promises more than it holds ...
      expect(links).toBe(Number(summary.match(/\((\d+)\)/)![1]));
      // ... and every resource in this fixture became a page, so it also equals
      // the sidebar's own count of the same single-value selection.
      expect(links).toBe(countMatching(src, { [axis]: [value] }));
    }
  });

  it('omits an axis nothing matches, in every mode', () => {
    const src = facetedIndex();
    // A declared axis no resource is a member of is not offered in the sidebar,
    // so it is not a section here either.
    src.facets.push({
      id: 'lifecycle',
      label: 'Lifecycle',
      values: [{ id: 'pilot', label: 'Pilot', count: 0 }],
    });
    expect(overview(src, 'both')).not.toContain('<h2>Lifecycle</h2>');
    expect(overview(src, 'axis')).not.toContain('<h2>Lifecycle</h2>');
  });

  it('falls back to the by-type list when the index has no usable axis', () => {
    for (const mode of ['type', 'both', 'axis'] as const) {
      const html = overview(index(), mode);
      // Byte for byte today's overview, whatever the mode asked for: an export
      // can never come out with no index at all.
      expect(html).toBe(overview(index(), 'type'));
      expect(html).toContain('<h2>Pages</h2>');
      expect(html).not.toContain('<details>');
    }
  });

  it('says so when the index lists no resources, in every mode', () => {
    const empty = parseTenantIndex(
      FACETED_INDEX_YAML.slice(0, FACETED_INDEX_YAML.indexOf('resources:')),
    )!;
    for (const mode of ['type', 'both', 'axis'] as const) {
      expect(overview(empty, mode)).toContain(
        "This tenant's index lists no resources.",
      );
    }
  });
});

// The `<li>` rows one collapsed axis section holds.
function section(html: string, summary: string): string {
  const at = html.indexOf(`<summary>${summary}</summary>`);
  if (at < 0) throw new Error(`no section ${summary}`);
  const end = html.indexOf('</details>', at);
  return html.slice(at, end);
}

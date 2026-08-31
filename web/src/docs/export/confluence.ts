import { IndexResource, TenantIndex } from '../tenant-index';
import { escapeHtml } from './html-allowlist';
import {
  buildPageNames,
  PageName,
  PageSource,
  sanitizeTitle,
  stripDocExtension,
  typeLeaf,
} from './page-name';

// The Confluence HTML import format: one zip containing one folder, the folder
// name becoming the space name, each file becoming a page and the *file name*
// becoming the page title. There is no page hierarchy, so the document tree and
// the sidebar cannot come along — the overview page's link list replaces them.
//
// Pure and Nest-free: `ExportService` owns the zip and the streaming, this
// module owns the format.

// The overview page. `<type leaf> — <name>` is the shape of every document
// page, so a name without the separator cannot collide with one.
export const OVERVIEW_FILE = 'Overview.html';

export interface ExportPage {
  // Document path as the index records it, relative to docs/.
  doc: string;
  // Extensionless document path — the key links are rewritten against.
  docPath: string;
  type: string;
  title: string;
  file: string;
  documented: boolean;
  summary: string;
}

export interface ExportPlan {
  // Folder name inside the zip, which is the space name Confluence creates.
  space: string;
  // Pages in export order (document path order, so the zip is deterministic).
  pages: ExportPage[];
  // What `toConfluenceHtml` rewrites hrefs against.
  pageFileByDoc: Map<string, string>;
}

// `<domain> documentation` — the index carries the tenant domain, not an
// organisation display name, and a bare hostname reads poorly as a space name.
export function spaceName(tenantName: string): string {
  const name = sanitizeTitle(tenantName);
  return name ? `${name} documentation` : 'Tenant documentation';
}

// Builds the plan from `docs/index.yaml` alone.
//
// This is the export's *own* index pass, deliberately not `buildNavigation()`:
// the hrefs here are page file names rather than app routes, there is no active
// item and no `<details>` nesting, skipped documents need a place, and the
// deduplication of page names has to happen in the same pass that names the
// files. Teaching `NavSection` a second href shape would push export concerns
// into the sidebar's model.
//
// `h1ByDoc` supplies the document's own H1 for resources the index has no
// display name for; it is optional, so a plan can be built from the index only.
export function buildExportPlan(
  index: TenantIndex,
  h1ByDoc: Map<string, string> = new Map(),
): ExportPlan {
  const sources: PageSource[] = index.resources.map((r) => ({
    doc: r.doc,
    type: r.type,
    displayName: r.displayName,
    h1: h1ByDoc.get(stripDocExtension(r.doc)),
  }));
  const names = buildPageNames(sources);

  const byDocPath = new Map<string, IndexResource>();
  for (const resource of index.resources) {
    const docPath = stripDocExtension(resource.doc);
    if (docPath && !byDocPath.has(docPath)) byDocPath.set(docPath, resource);
  }

  const pages: ExportPage[] = [];
  const pageFileByDoc = new Map<string, string>();
  for (const name of orderedNames(names)) {
    const resource = byDocPath.get(name.docPath);
    if (!resource) continue;
    pages.push({
      doc: resource.doc,
      docPath: name.docPath,
      type: resource.type,
      title: name.title,
      file: name.file,
      documented: resource.documented,
      summary: resource.summary,
    });
    pageFileByDoc.set(name.docPath, name.file);
  }

  return { space: spaceName(index.tenant), pages, pageFileByDoc };
}

function orderedNames(names: Map<string, PageName>): PageName[] {
  return [...names.values()].sort((a, b) =>
    a.docPath.localeCompare(b.docPath),
  );
}

// The page body of one document: the provenance table, then the serialised
// document HTML. Frontmatter is stripped before rendering, so the provenance
// only exists here if it is written out explicitly.
export function documentPage(options: {
  title: string;
  bodyHtml: string;
  meta: Record<string, unknown>;
  docPath: string;
}): string {
  return htmlDocument(
    options.title,
    provenance(options.meta, options.docPath) + options.bodyHtml,
  );
}

// The overview page: the tenant summary (already serialised) plus the grouped
// link list that stands in for the sidebar, and an honest list of anything the
// export could not read.
export function overviewPage(options: {
  tenantName: string;
  index: TenantIndex;
  pages: ExportPage[];
  summaryHtml: string | null;
  skipped: ExportPage[];
}): string {
  const { tenantName, index, pages, summaryHtml, skipped } = options;
  const parts: string[] = [];

  parts.push(`<h1>${escapeHtml(tenantName)} documentation</h1>`);
  parts.push(
    '<p><em>Generated from an azure-resource-downloader export by the documentation' +
      ' browser. This is a one-way publish: re-importing creates another space, and' +
      ' edits made here are lost the next time it is imported.</em></p>',
  );

  const counts: string[] = [
    `${index.counts.documented} documented`,
    `${index.counts.pending} pending`,
  ];
  if (index.generatedAt) counts.push(`exported ${index.generatedAt}`);
  parts.push(`<p>${escapeHtml(counts.join(' · '))}</p>`);

  if (!index.complete) {
    const reason = index.incompleteReason
      ? `: ${index.incompleteReason}`
      : '';
    parts.push(
      `<p><strong>This export is incomplete${escapeHtml(reason)}.</strong></p>`,
    );
  }

  if (summaryHtml) parts.push(summaryHtml);

  parts.push('<h2>Pages</h2>');
  const grouped = groupByType(pages);
  if (grouped.length === 0) {
    parts.push("<p>This tenant's index lists no resources.</p>");
  }
  for (const group of grouped) {
    parts.push(`<h3>${escapeHtml(group.label)}</h3>`);
    parts.push('<ul>');
    for (const page of group.pages) {
      const summary = page.summary ? ` — ${page.summary}` : '';
      const pending = page.documented ? '' : ' (pending)';
      parts.push(
        `<li><a href="${escapeHtml(page.file)}">${escapeHtml(page.title)}</a>` +
          `${escapeHtml(summary)}${pending}</li>`,
      );
    }
    parts.push('</ul>');
  }

  if (index.counts.excluded.length > 0) {
    parts.push('<h2>Not documented (bulk types)</h2>');
    parts.push('<ul>');
    for (const entry of index.counts.excluded) {
      parts.push(
        `<li>${escapeHtml(entry.type)} (${entry.count})</li>`,
      );
    }
    parts.push('</ul>');
  }

  if (skipped.length > 0) {
    parts.push('<h2>Not exported</h2>');
    parts.push(
      '<p>These documents are listed in the index but could not be read when the' +
        ' export ran.</p>',
    );
    parts.push('<ul>');
    for (const page of skipped) {
      parts.push(`<li>${escapeHtml(page.doc)}</li>`);
    }
    parts.push('</ul>');
  }

  return htmlDocument(`${tenantName} documentation`, parts.join('\n'));
}

interface TypeGroup {
  label: string;
  pages: ExportPage[];
}

function groupByType(pages: ExportPage[]): TypeGroup[] {
  const groups = new Map<string, TypeGroup>();
  for (const page of pages) {
    let group = groups.get(page.type);
    if (!group) {
      group = { label: typeLeaf(page.type) || page.type, pages: [] };
      groups.set(page.type, group);
    }
    group.pages.push(page);
  }
  const ordered = [...groups.values()].sort((a, b) =>
    a.label.localeCompare(b.label),
  );
  for (const group of ordered) {
    group.pages.sort((a, b) => a.title.localeCompare(b.title));
  }
  return ordered;
}

// Frontmatter is stripped before rendering, so the source, the export timestamp
// and the generation hashes are emitted here or nowhere.
function provenance(meta: Record<string, unknown>, docPath: string): string {
  const rows: Array<[string, string]> = [];
  const add = (label: string, value: unknown): void => {
    if (typeof value === 'string' && value) rows.push([label, value]);
    else if (typeof value === 'number') rows.push([label, String(value)]);
  };
  add('Source', meta.source);
  add('Document', `docs/${docPath}.md`);
  add('Generated', meta.generatedAt);
  add('Source hash', meta.sourceSha256);
  add('Prompt hash', meta.promptSha256);

  const table = rows
    .map(
      ([label, value]) =>
        `<tr><td>${escapeHtml(label)}</td><td><code>${escapeHtml(value)}</code></td></tr>`,
    )
    .join('');

  return (
    `<table><tbody>${table}</tbody></table>` +
    '<p><em>Generated page — edits made in Confluence are lost the next time this' +
    ' documentation is imported.</em></p>'
  );
}

// Minimal wrapper. `<title>` is not preserved by the importer (the file name is
// the page title), but a well-formed document is what the importer parses.
function htmlDocument(title: string, body: string): string {
  return (
    '<!doctype html>\n<html lang="en">\n<head>\n<meta charset="utf-8" />\n' +
    `<title>${escapeHtml(title)}</title>\n</head>\n<body>\n${body}\n</body>\n</html>\n`
  );
}

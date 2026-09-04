import { Controller, Get, Param, Query, Res } from '@nestjs/common';
import { Response } from 'express';
import * as path from 'path';
import { TenantDiscoveryService, TenantInfo } from './tenant-discovery.service';
import { MarkdownRendererService } from './markdown-renderer.service';
import { YamlHighlighterService } from './yaml-highlighter.service';
import { resolveResource, resolveWithinTenant } from './path-safety';
import {
  buildFacetFilters,
  buildNavigation,
  countMatching,
  hasSelection,
  parseFacetSelection,
  TenantIndex,
} from './tenant-index';
import { ExportService } from './export/export.service';

// Route prefix for the source-YAML representation of a document. It is a
// *representation*, not a path segment: it never appears in the breadcrumb, and
// it cannot collide with a resource type because no Azure/Graph type segment
// starts with `_`.
export const RESOURCE_PREFIX = '_resource';

// Route prefix for a whole-tenant export. A representation prefix like
// `_resource`, and safe for the same reason.
export const EXPORT_PREFIX = '_export';

// Paths at the docs root the CLI writes as tool input, not documentation:
// generate.md is the agent prompt. They are never served. Matched without the
// optional `.md` suffix, like every other route.
const TOOL_ARTIFACTS = new Set(['generate']);

// The tenant-wide management summary is the body of the tenant landing page,
// so its own document route is a duplicate and redirects there.
const SUMMARY_ROUTE = 'summary';

@Controller()
export class DocsController {
  constructor(
    private readonly discovery: TenantDiscoveryService,
    private readonly renderer: MarkdownRendererService,
    private readonly highlighter: YamlHighlighterService,
    private readonly exporter: ExportService,
  ) {}

  // GET / — tenant picker, which is also where a tenant's export is offered.
  @Get()
  async picker(@Res() res: Response): Promise<void> {
    const tenants = (await this.discovery.list()).map((tenant) => ({
      ...tenant,
      exportHref: `/${tenant.id}/${EXPORT_PREFIX}/confluence`,
    }));
    res.render('picker', { title: 'Documentation', tenants });
  }

  // GET /healthz — discovery health.
  @Get('healthz')
  async healthz(@Res() res: Response): Promise<void> {
    const tenants = await this.discovery.list();
    const documents = tenants.reduce((n, t) => n + t.documented, 0);
    const pending = tenants.reduce((n, t) => n + t.pending, 0);
    res.json({ status: 'ok', tenants: tenants.length, documents, pending });
  }

  // GET /:tenant — the tenant landing page: the generation agent's tenant-wide
  // summary (docs/summary.md) as the body, with the index-driven navigation in
  // the sidebar. The summary is optional — an export can carry a valid index
  // and no summary — so a missing one falls back to listing the index.
  @Get(':tenant')
  async tenantIndex(
    @Param('tenant') tenant: string,
    @Query() query: Record<string, unknown>,
    @Res() res: Response,
  ): Promise<void> {
    const info = await this.discovery.get(tenant);
    if (!info) return this.notFound(res, tenant, '');

    const index = await this.discovery.getIndex(info);
    if (!index) return this.notFound(res, tenant, '');

    let summary: string | null = null;
    try {
      const page = await this.renderer.render(info.summaryPath, {
        tenant,
        docDir: '',
      });
      summary = page.html;
    } catch {
      summary = null;
    }

    res.render('tenant', {
      title: info.name,
      tenant,
      breadcrumb: [],
      summary,
      nav: this.nav(info, index, '', `/${tenant}`, query),
    });
  }

  // GET /:tenant/_export/:format — the tenant's documentation as an importable
  // archive. Declared before the document catch-all so the prefix wins. Still
  // read-only: the archive is assembled in memory and streamed, and nothing is
  // written under the docs root.
  @Get(`:tenant/${EXPORT_PREFIX}/:format`)
  async export(
    @Param('tenant') tenant: string,
    @Param('format') format: string,
    @Res() res: Response,
  ): Promise<void> {
    const info = await this.discovery.get(tenant);
    if (!info) return this.notFound(res, tenant, EXPORT_PREFIX);
    if (format !== 'confluence') {
      return this.notFound(res, tenant, `${EXPORT_PREFIX}/${format}`);
    }

    const index = await this.discovery.getIndex(info);
    if (!index) return this.notFound(res, tenant, EXPORT_PREFIX);

    await this.exporter.confluence(info, index, res);
  }

  // GET /:tenant/_resource/*path — the exported source YAML behind a document,
  // syntax highlighted; `?raw` serves it as plain text. Declared before the
  // document catch-all so the prefix wins. Read-only, like every other route.
  @Get(`:tenant/${RESOURCE_PREFIX}/*path`)
  async resource(
    @Param() params: any,
    @Query('raw') raw: string | undefined,
    @Query() query: Record<string, unknown>,
    @Res() res: Response,
  ): Promise<void> {
    const tenant: string = params.tenant;
    const relPath = joinPath(params.path ?? params['0'] ?? '');

    const info = await this.discovery.get(tenant);
    if (!info) return this.notFound(res, tenant, relPath);

    const resolved = resolveResource(info.resourcesDir, relPath);
    if (!resolved) return this.notFound(res, tenant, relPath);

    if (raw !== undefined) {
      res.sendFile(resolved, {
        headers: {
          'Content-Type': 'text/plain; charset=utf-8',
          'X-Content-Type-Options': 'nosniff',
          'Content-Disposition': 'inline',
        },
      });
      return;
    }

    const docPath = stripExtension(relPath);
    try {
      const rendered = await this.highlighter.render(resolved);
      const index = await this.discovery.getIndex(info);
      res.render('resource', {
        title: path.posix.basename(docPath),
        tenant,
        breadcrumb: this.breadcrumb(relPath),
        source: `${docPath}.yaml`,
        rawHref: `/${tenant}/${RESOURCE_PREFIX}/${docPath}?raw`,
        body: rendered.html,
        highlighted: rendered.highlighted,
        lines: rendered.lines,
        size: rendered.size,
        views: this.views(tenant, docPath, 'resource'),
        nav: index
          ? this.nav(
              info,
              index,
              docPath,
              `/${tenant}/${RESOURCE_PREFIX}/${docPath}`,
              query,
            )
          : null,
      });
    } catch {
      this.notFound(res, tenant, relPath);
    }
  }

  // GET /:tenant/*path — a document within the tenant.
  @Get(':tenant/*path')
  async doc(
    @Param() params: any,
    @Query() query: Record<string, unknown>,
    @Res() res: Response,
  ): Promise<void> {
    const tenant: string = params.tenant;
    const relPath = joinPath(params.path ?? params['0'] ?? '');

    const info = await this.discovery.get(tenant);
    if (!info) return this.notFound(res, tenant, relPath);

    const rootDoc = relPath.replace(/\.md$/i, '').toLowerCase();
    if (TOOL_ARTIFACTS.has(rootDoc)) {
      return this.notFound(res, tenant, relPath);
    }
    if (rootDoc === SUMMARY_ROUTE) {
      res.redirect(302, `/${tenant}`);
      return;
    }

    const resolved = resolveWithinTenant(info.dir, relPath);
    if (!resolved) return this.notFound(res, tenant, relPath);

    try {
      const page = await this.renderer.render(resolved, {
        tenant,
        docDir: this.docDir(relPath),
      });
      const index = await this.discovery.getIndex(info);
      const docPath = stripExtension(relPath);
      // The document's own path is what locates its source YAML (the export
      // mirrors docs/<type>/<name>.md and resources/<type>/<name>.yaml), so
      // `meta.source` stays a label and is only used as the has-a-source flag.
      const hasSource = typeof page.meta.source === 'string' && !!page.meta.source;
      res.render('page', {
        title: page.title || relPath,
        body: page.html,
        tenant,
        breadcrumb: this.breadcrumb(relPath),
        meta: page.meta,
        sourceHref: hasSource
          ? `/${tenant}/${RESOURCE_PREFIX}/${docPath}`
          : null,
        views: hasSource ? this.views(tenant, docPath, 'doc') : null,
        nav: index
          ? this.nav(info, index, relPath, `/${tenant}/${docPath}`, query)
          : null,
      });
    } catch {
      this.notFound(res, tenant, relPath);
    }
  }

  // View model for the sidebar partial: the tenant metadata that used to sit on
  // the landing page, the facet filters and the navigation tree, so all three
  // survive on every page. The selection is read from the query parameters named
  // after the index's own axis ids, and values this index cannot serve are
  // dropped rather than rendering an empty tree that would look like a broken
  // tenant.
  //
  // `matched`/`total` count distinct resources and deliberately ignore the
  // active-document exemption in `buildNavigation`, so the numbers describe the
  // selection rather than the page.
  private nav(
    info: TenantInfo,
    index: TenantIndex,
    activeDoc: string,
    basePath: string,
    query: Record<string, unknown>,
  ): Record<string, unknown> {
    const selection = parseFacetSelection(index, query);
    const sections = buildNavigation(index, info.id, activeDoc, selection);
    return {
      tenant: info.id,
      name: info.name,
      generatedAt: index.generatedAt,
      counts: index.counts,
      complete: index.complete,
      incompleteReason: index.incompleteReason,
      facets: buildFacetFilters(index, basePath, selection),
      filtering: hasSelection(selection),
      matched: countMatching(index, selection),
      total: index.resources.length,
      clearHref: basePath,
      // Whether the tree is one longer than `matched` because the document being
      // viewed is exempt from the filter, so the view can say so instead of
      // leaving the reader to reconcile the two.
      exempt: sections.some((s) => s.items.some((i) => i.exempt)),
      sections,
    };
  }

  // The document/YAML switcher for the top bar. Both representations share the
  // same extensionless path, so no extra lookup is needed.
  private views(
    tenant: string,
    docPath: string,
    kind: 'doc' | 'resource',
  ): Array<{ label: string; href: string; active: boolean }> {
    return [
      {
        label: 'Documentation',
        href: `/${tenant}/${docPath}`,
        active: kind === 'doc',
      },
      {
        label: 'YAML',
        href: `/${tenant}/${RESOURCE_PREFIX}/${docPath}`,
        active: kind === 'resource',
      },
    ];
  }

  private docDir(relPath: string): string {
    const dir = path.posix.dirname(stripExtension(relPath));
    return dir === '.' ? '' : dir;
  }

  private breadcrumb(relPath: string): Array<{ label: string }> {
    return stripExtension(relPath)
      .split('/')
      .filter(Boolean)
      .map((label) => ({ label }));
  }

  private notFound(res: Response, tenant: string, requested: string): void {
    res
      .status(404)
      .render('error', { title: 'Not found', tenant, requested });
  }
}

function joinPath(value: any): string {
  return Array.isArray(value) ? value.join('/') : String(value ?? '');
}

function stripExtension(relPath: string): string {
  return relPath.replace(/\.(md|yaml)$/i, '');
}

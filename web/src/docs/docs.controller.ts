import { Controller, Get, Param, Res } from '@nestjs/common';
import { Response } from 'express';
import * as path from 'path';
import { TenantDiscoveryService, TenantInfo } from './tenant-discovery.service';
import { MarkdownRendererService } from './markdown-renderer.service';
import { resolveWithinTenant } from './path-safety';
import { buildNavigation, TenantIndex } from './tenant-index';

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
  ) {}

  // GET / — tenant picker.
  @Get()
  async picker(@Res() res: Response): Promise<void> {
    const tenants = await this.discovery.list();
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
      nav: this.nav(info, index, ''),
    });
  }

  // GET /:tenant/*path — a document within the tenant.
  @Get(':tenant/*path')
  async doc(@Param() params: any, @Res() res: Response): Promise<void> {
    const tenant: string = params.tenant;
    let relPath: any = params.path ?? params['0'] ?? '';
    if (Array.isArray(relPath)) relPath = relPath.join('/');

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
      res.render('page', {
        title: page.title || relPath,
        body: page.html,
        tenant,
        breadcrumb: this.breadcrumb(relPath),
        meta: page.meta,
        nav: index ? this.nav(info, index, relPath) : null,
      });
    } catch {
      this.notFound(res, tenant, relPath);
    }
  }

  // View model for the sidebar partial: the tenant metadata that used to sit on
  // the landing page plus the navigation tree, so both survive on every page.
  private nav(
    info: TenantInfo,
    index: TenantIndex,
    activeDoc: string,
  ): Record<string, unknown> {
    return {
      tenant: info.id,
      name: info.name,
      generatedAt: index.generatedAt,
      counts: index.counts,
      complete: index.complete,
      incompleteReason: index.incompleteReason,
      sections: buildNavigation(index, info.id, activeDoc),
    };
  }

  private docDir(relPath: string): string {
    const dir = path.posix.dirname(relPath.replace(/\.md$/i, ''));
    return dir === '.' ? '' : dir;
  }

  private breadcrumb(relPath: string): Array<{ label: string }> {
    return relPath
      .replace(/\.md$/i, '')
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

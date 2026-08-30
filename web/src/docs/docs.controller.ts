import { Controller, Get, Param, Res } from '@nestjs/common';
import { Response } from 'express';
import * as path from 'path';
import { TenantDiscoveryService } from './tenant-discovery.service';
import { MarkdownRendererService } from './markdown-renderer.service';
import { resolveWithinTenant } from './path-safety';
import { buildNavigation } from './tenant-index';

// Paths at the docs root the CLI writes as tool input, not documentation:
// generate.md is the agent prompt. They are never served. Matched without the
// optional `.md` suffix, like every other route.
const TOOL_ARTIFACTS = new Set(['generate']);

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

  // GET /:tenant — the tenant landing page, built from docs/index.yaml. The
  // CLI no longer generates an index.md; navigation comes from the index.
  @Get(':tenant')
  async tenantIndex(
    @Param('tenant') tenant: string,
    @Res() res: Response,
  ): Promise<void> {
    const info = await this.discovery.get(tenant);
    if (!info) return this.notFound(res, tenant, '');

    const index = await this.discovery.getIndex(info);
    if (!index) return this.notFound(res, tenant, '');

    res.render('tenant', {
      title: info.name,
      tenant,
      breadcrumb: [],
      name: info.name,
      generatedAt: index.generatedAt,
      counts: index.counts,
      complete: index.complete,
      incompleteReason: index.incompleteReason,
      sections: buildNavigation(index, tenant),
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

    if (TOOL_ARTIFACTS.has(relPath.replace(/\.md$/i, '').toLowerCase())) {
      return this.notFound(res, tenant, relPath);
    }

    const resolved = resolveWithinTenant(info.dir, relPath);
    if (!resolved) return this.notFound(res, tenant, relPath);

    try {
      const page = await this.renderer.render(resolved, {
        tenant,
        docDir: this.docDir(relPath),
      });
      res.render('page', {
        title: page.title || relPath,
        body: page.html,
        tenant,
        breadcrumb: this.breadcrumb(relPath),
        meta: page.meta,
      });
    } catch {
      this.notFound(res, tenant, relPath);
    }
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

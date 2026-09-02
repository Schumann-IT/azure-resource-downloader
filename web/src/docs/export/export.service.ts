import { Injectable } from '@nestjs/common';
import { Response } from 'express';
import * as path from 'path';
import { ZipFile } from 'yazl';
import { MarkdownRendererService } from '../markdown-renderer.service';
import { resolveWithinTenant } from '../path-safety';
import { TenantInfo } from '../tenant-discovery.service';
import { TenantIndex } from '../tenant-index';
import {
  buildExportPlan,
  documentPage,
  ExportPage,
  overviewPage,
  OVERVIEW_FILE,
} from './confluence';
import { toConfluenceHtml } from './html-allowlist';
import { stripDocExtension } from './page-name';

// Builds and streams a tenant's documentation as an importable archive.
//
// Thin on purpose: the format lives in `confluence.ts`, the serialiser in
// `html-allowlist.ts`. A second format is a second method here plus its own
// format module, with the controller untouched.
//
// Read-only, like every other route: documents are enumerated from
// `docs/index.yaml`, read through `resolveWithinTenant()`, and the archive is
// assembled in memory and streamed — nothing is written under `DOCS_ROOT`, and
// no temporary file is created at all.
@Injectable()
export class ExportService {
  constructor(private readonly renderer: MarkdownRendererService) {}

  async confluence(
    info: TenantInfo,
    index: TenantIndex,
    res: Response,
  ): Promise<void> {
    const plan = buildExportPlan(index, await this.titles(info, index));
    const mtime = archiveTimestamp(index.generatedAt);
    const zip = new ZipFile();

    res.setHeader('Content-Type', 'application/zip');
    res.setHeader(
      'Content-Disposition',
      `attachment; filename="${info.id}.zip"`,
    );
    res.setHeader('X-Content-Type-Options', 'nosniff');
    zip.outputStream.pipe(res);

    const skipped: ExportPage[] = [];
    for (const page of plan.pages) {
      const html = await this.renderPage(info, page, plan.pageFileByDoc);
      if (html === null) {
        // A document the index lists but that cannot be read is normal, not
        // exceptional: it is reported on the overview page instead of failing
        // the whole export.
        skipped.push(page);
      } else {
        zip.addBuffer(Buffer.from(html, 'utf8'), `${plan.space}/${page.file}`, {
          mtime,
        });
      }
      // A whole-tenant export touches hundreds of documents on one thread;
      // yielding keeps the rest of the app responsive while it runs.
      await yieldToEventLoop();
    }

    const summaryHtml = await this.renderSummary(info, plan.pageFileByDoc);
    const overview = overviewPage({
      tenantName: info.name,
      index,
      pages: plan.pages,
      summaryHtml,
      skipped,
    });
    zip.addBuffer(
      Buffer.from(overview, 'utf8'),
      `${plan.space}/${OVERVIEW_FILE}`,
      { mtime },
    );

    zip.end();
  }

  // The document's own H1, for the resources the index has no display name for.
  // Rendering is what parses the title, and the render cache means the same
  // document is not read twice.
  private async titles(
    info: TenantInfo,
    index: TenantIndex,
  ): Promise<Map<string, string>> {
    const titles = new Map<string, string>();
    for (const resource of index.resources) {
      if (resource.displayName) continue;
      const docPath = stripDocExtension(resource.doc);
      const resolved = resolveWithinTenant(info.dir, resource.doc);
      if (!resolved) continue;
      try {
        const page = await this.renderer.render(resolved, {
          tenant: info.id,
          docDir: docDir(docPath),
        });
        if (page.title) titles.set(docPath, page.title);
      } catch {
        // Unreadable here means it will be reported as not exported below.
      }
    }
    return titles;
  }

  // Renders one document and serialises it for the import. Returns null when the
  // document cannot be read.
  private async renderPage(
    info: TenantInfo,
    page: ExportPage,
    pageFileByDoc: Map<string, string>,
  ): Promise<string | null> {
    const resolved = resolveWithinTenant(info.dir, page.doc);
    if (!resolved) return null;

    try {
      // The same render the browser gets, with the same env — the exporter adds
      // no render mode, so the mtime-keyed cache stays valid for both.
      const rendered = await this.renderer.render(resolved, {
        tenant: info.id,
        docDir: docDir(page.docPath),
      });
      return documentPage({
        title: page.title,
        bodyHtml: toConfluenceHtml(rendered.html, {
          tenant: info.id,
          pageFileByDoc,
        }),
        meta: rendered.meta,
        docPath: page.docPath,
      });
    } catch {
      return null;
    }
  }

  // The tenant-wide summary, which becomes the body of the overview page. It is
  // optional: an export can carry a valid index and no summary.
  private async renderSummary(
    info: TenantInfo,
    pageFileByDoc: Map<string, string>,
  ): Promise<string | null> {
    try {
      const rendered = await this.renderer.render(info.summaryPath, {
        tenant: info.id,
        docDir: '',
      });
      return toConfluenceHtml(rendered.html, {
        tenant: info.id,
        pageFileByDoc,
      });
    } catch {
      return null;
    }
  }
}

function docDir(docPath: string): string {
  const dir = path.posix.dirname(docPath);
  return dir === '.' ? '' : dir;
}

function yieldToEventLoop(): Promise<void> {
  return new Promise((resolve) => setImmediate(resolve));
}

// Zip entries carry the export's own timestamp rather than the wall clock, so
// exporting the same unchanged tenant twice produces the same bytes. The
// fallback is the earliest date the zip format can represent.
const ZIP_EPOCH = new Date(Date.UTC(1980, 0, 1));

function archiveTimestamp(generatedAt: string | null): Date {
  if (generatedAt) {
    const parsed = new Date(generatedAt);
    if (!Number.isNaN(parsed.getTime()) && parsed >= ZIP_EPOCH) return parsed;
  }
  return ZIP_EPOCH;
}

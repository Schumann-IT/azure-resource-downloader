import { Injectable, OnModuleInit } from '@nestjs/common';
import { Response } from 'express';
import * as path from 'path';
import pdfmake from 'pdfmake';
import { ZipFile } from 'yazl';
import { DriftService } from '../drift.service';
import { DriftReportService, FindingReport } from '../drift-report.service';
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
import { parseExportIndexMode } from './export-index-mode';
import { driftPdfDefinition } from './drift-pdf';
import { toConfluenceHtml } from './html-allowlist';
import { stripDocExtension } from './page-name';
import { CODE_FONT } from './pdf-content';

// What a drift PDF request came to: the file was sent; there is no current
// observation to export (a 404 for the caller); or the build failed before a
// single byte went out (a generic error for the caller).
export type DriftPdfOutcome = 'sent' | 'noDrift' | 'failed';

// The body font: the Roboto TTFs pdfmake ships, located through the package
// itself so no font file is copied into the app and nothing is fetched.
const ROBOTO_FILES = {
  normal: 'Roboto-Regular.ttf',
  bold: 'Roboto-Medium.ttf',
  italics: 'Roboto-Italic.ttf',
  bolditalics: 'Roboto-MediumItalic.ttf',
} as const;

// The code font: a PDF standard font, so its names are not files. pdfkit
// carries its metrics; the local access policy still has to admit the names.
const COURIER = {
  normal: CODE_FONT,
  bold: `${CODE_FONT}-Bold`,
  italics: `${CODE_FONT}-Oblique`,
  bolditalics: `${CODE_FONT}-BoldOblique`,
} as const;

// Builds and sends a tenant's exports: the documentation as an importable
// archive, and the current drift report as one PDF.
//
// Thin on purpose: the formats live in `confluence.ts` and `drift-pdf.ts`, the
// serialisers in `html-allowlist.ts` and `pdf-content.ts`.
//
// Read-only, like every other route: documents are enumerated from
// `docs/index.yaml` (the drift report from `drift/metadata.yaml`), read through
// the path-safety resolvers, and each export is assembled in memory — nothing
// is written under `DOCS_ROOT`, and no temporary file is created at all.
@Injectable()
export class ExportService implements OnModuleInit {
  constructor(
    private readonly renderer: MarkdownRendererService,
    private readonly reports: DriftReportService,
    private readonly drift: DriftService,
  ) {}

  // pdfmake is a process-wide singleton, so it is configured once here and
  // never per request. Both access policies are required: without them pdfmake
  // warns on every build (no per-request logging) and would fetch any URL or
  // read any local file a definition named. The policy admits exactly the
  // fonts; the definitions never carry an image, an svg or a URL.
  onModuleInit(): void {
    const dir = path.dirname(require.resolve(`pdfmake/fonts/Roboto/${ROBOTO_FILES.normal}`));
    const roboto = {
      normal: path.join(dir, ROBOTO_FILES.normal),
      bold: path.join(dir, ROBOTO_FILES.bold),
      italics: path.join(dir, ROBOTO_FILES.italics),
      bolditalics: path.join(dir, ROBOTO_FILES.bolditalics),
    };
    const allowed = new Set<string>([...Object.values(roboto), ...Object.values(COURIER)]);
    pdfmake.setFonts({ Roboto: roboto, [CODE_FONT]: { ...COURIER } });
    pdfmake.setUrlAccessPolicy(() => false);
    pdfmake.setLocalAccessPolicy((file) => allowed.has(file));
  }

  // The tenant's current drift observation as one PDF. Built completely in
  // memory before a header is set, so a failed build never leaves a
  // half-sent attachment.
  async driftPdf(
    info: TenantInfo,
    index: TenantIndex,
    res: Response,
  ): Promise<DriftPdfOutcome> {
    const report = await this.reports.tenantReport(info, index);
    if (report.state.kind !== 'current') return 'noDrift';
    const observation = report.state.observation;

    let pdf: Buffer;
    try {
      const findings = new Map<string, FindingReport>();
      for (const finding of observation.findings) {
        const files = await this.drift.files(info, observation, finding);
        findings.set(
          finding.key,
          await this.reports.findingReport(info, finding, files, report.audit),
        );
        // A large observation is many documents on one thread; yielding keeps
        // the rest of the app responsive while it runs.
        await yieldToEventLoop();
      }
      const definition = driftPdfDefinition({
        tenantId: info.id,
        tenantName: info.name,
        observation,
        report,
        findings,
      });
      pdf = await pdfmake.createPdf(definition).getBuffer();
    } catch {
      return 'failed';
    }

    res.setHeader('Content-Type', 'application/pdf');
    res.setHeader('Content-Disposition', `attachment; filename="${info.id}-drift.pdf"`);
    res.setHeader('Content-Length', String(pdf.length));
    res.setHeader('X-Content-Type-Options', 'nosniff');
    res.end(pdf);
    return 'sent';
  }

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
      // Read at its point of use, per export, so the operator's choice needs no
      // restart and `confluence.ts` stays env-free.
      indexMode: parseExportIndexMode(process.env.EXPORT_INDEX),
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

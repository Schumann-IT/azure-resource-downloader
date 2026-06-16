import { readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { Injectable, Logger } from '@nestjs/common';
import pLimit from 'p-limit';
import {
  buildDocPrompt,
  buildOutputIndex,
  buildTenantIndex,
  hashContent,
  listTenants,
  loadManifest,
  needsRegen,
  renderTenantIndexMarkdown,
  saveManifest,
  walkTenant,
  type Manifest,
} from '@ard/core';
import { resolveOutputDir } from '../config';
import { createProvider } from './llm/factory';
import { llmConfigFromEnv, type LlmProvider } from './llm/provider';

export interface DocgenOptions {
  tenant?: string;
  force?: boolean;
  dryRun?: boolean;
  concurrency?: number;
  provider?: string;
}

export interface DocgenSummary {
  generated: number;
  skipped: number;
  failed: number;
  wouldGenerate: number;
  failures: Array<{ resource: string; error: string }>;
  dryRun: boolean;
  provider: string;
}

/** Retry an async op with exponential backoff on transient failures. */
async function withRetry<T>(fn: () => Promise<T>, attempts = 4): Promise<T> {
  let lastErr: unknown;
  for (let i = 0; i < attempts; i++) {
    try {
      return await fn();
    } catch (err) {
      lastErr = err;
      if (i < attempts - 1) {
        const delay = 500 * 2 ** i + Math.floor(Math.random() * 250);
        await new Promise((r) => setTimeout(r, delay));
      }
    }
  }
  throw lastErr;
}

@Injectable()
export class DocgenService {
  private readonly log = new Logger('docgen');
  private readonly outputDir = resolveOutputDir();

  async run(opts: DocgenOptions): Promise<DocgenSummary> {
    const dryRun = opts.dryRun ?? false;
    const force = opts.force ?? false;
    const concurrency = Math.max(1, opts.concurrency ?? Number(process.env.DOCGEN_CONCURRENCY ?? 4));

    const cfg = { ...llmConfigFromEnv(), ...(opts.provider ? { provider: opts.provider } : {}) };
    const provider: LlmProvider = dryRun ? { name: cfg.provider, generate: async () => '' } : createProvider(cfg);

    const summary: DocgenSummary = {
      generated: 0,
      skipped: 0,
      failed: 0,
      wouldGenerate: 0,
      failures: [],
      dryRun,
      provider: provider.name,
    };

    const tenants = opts.tenant ? [opts.tenant] : this.listTenants();
    const limit = pLimit(concurrency);

    for (const tenant of tenants) {
      const tree = walkTenant(this.outputDir, tenant);
      const manifestPath = join(this.outputDir, tenant, '.docgen-manifest.json');
      const manifest: Manifest = loadManifest(manifestPath);

      const jobs: Promise<void>[] = [];

      for (const p of tree.providers) {
        for (const rt of p.resourceTypes) {
          if (!rt.docPromptPath) {
            this.log.warn(`No doc-prompt.md for ${tenant}/${p.provider}/${rt.resourceType}; skipping type`);
            continue;
          }
          const docPrompt = readFileSync(join(this.outputDir, rt.docPromptPath), 'utf8');

          for (const r of rt.resources) {
            const yamlAbs = join(this.outputDir, r.yamlPath);
            const yaml = readFileSync(yamlAbs, 'utf8');
            const hash = hashContent(yaml);
            const key = r.yamlPath;

            if (!needsRegen(manifest, key, hash, force)) {
              summary.skipped += 1;
              continue;
            }

            if (dryRun) {
              summary.wouldGenerate += 1;
              this.log.log(`[dry-run] would generate ${key}`);
              continue;
            }

            jobs.push(
              limit(async () => {
                const docAbs = `${yamlAbs.slice(0, -'.yaml'.length)}.doc.md`;
                try {
                  const prompt = buildDocPrompt(docPrompt, yaml);
                  const md = await withRetry(() => provider.generate(prompt));
                  writeFileSync(docAbs, md.endsWith('\n') ? md : `${md}\n`, 'utf8');
                  manifest[key] = { sourceHash: hash, generatedAt: new Date().toISOString() };
                  summary.generated += 1;
                  this.log.log(`generated ${key}`);
                } catch (err) {
                  summary.failed += 1;
                  summary.failures.push({ resource: key, error: String(err) });
                  this.log.error(`failed ${key}: ${String(err)}`);
                }
              }),
            );
          }
        }
      }

      await Promise.all(jobs);

      if (!dryRun) {
        saveManifest(manifestPath, manifest);
        // Per-tenant index (.md + .json) built from a fresh walk so generated
        // doc links reflect the files just written.
        const idx = buildTenantIndex(walkTenant(this.outputDir, tenant));
        writeFileSync(join(this.outputDir, tenant, 'index.md'), renderTenantIndexMarkdown(idx), 'utf8');
        writeFileSync(join(this.outputDir, tenant, 'index.json'), `${JSON.stringify(idx, null, 2)}\n`, 'utf8');
      }
    }

    if (!dryRun) {
      const top = buildOutputIndex(this.outputDir);
      writeFileSync(join(this.outputDir, 'index.json'), `${JSON.stringify(top, null, 2)}\n`, 'utf8');
    }

    this.logSummary(summary);
    return summary;
  }

  private listTenants(): string[] {
    return listTenants(this.outputDir);
  }

  private logSummary(s: DocgenSummary): void {
    if (s.dryRun) {
      this.log.log(`Dry run complete: ${s.wouldGenerate} would be generated, ${s.skipped} up-to-date (provider=${s.provider})`);
    } else {
      this.log.log(`Done: ${s.generated} generated, ${s.skipped} skipped, ${s.failed} failed (provider=${s.provider})`);
    }
    if (s.failures.length > 0) {
      this.log.warn(`${s.failures.length} resource(s) failed:`);
      for (const f of s.failures) this.log.warn(`  - ${f.resource}: ${f.error}`);
    }
  }
}

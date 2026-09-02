import { Injectable, OnModuleInit } from '@nestjs/common';
import { promises as fs } from 'fs';
import matter from 'gray-matter';
import { dynamicImport } from '../dynamic-import';
import { rewriteHref, extractTitle, LinkEnv } from './link-rewrite';
import { applyFindingsTable } from './findings-table';
import {
  applyMarkerBlocks,
  applyMetadataTable,
  applySectionHeadings,
  slugifyHeading,
  wrapSections,
} from './section-hooks';

export interface RenderedPage {
  html: string;
  title: string;
  meta: Record<string, unknown>;
}

interface CacheEntry {
  html: string;
  mtimeMs: number;
  size: number;
  meta: Record<string, unknown>;
  title: string;
}

const MAX_ENTRIES = 500;

// The generated documents echo their source file as a code-only paragraph under
// the H1. That is the same value the frontmatter already carries, and the page
// renders it as a link to the YAML view, so the echo is a duplicate. Dropped
// here rather than in the body it belongs to: it is only removed when it
// matches this document's own `source` (full path or basename) and sits alone on
// its line, so prose that mentions another resource's `.yaml` is never touched.
export function stripSourceEcho(content: string, source: unknown): string {
  if (typeof source !== 'string' || !source) return content;
  const alternatives = [source, source.split('/').pop() || source]
    .filter((v, i, all) => !!v && all.indexOf(v) === i)
    .map(escapeRegExp)
    .join('|');
  const echo = new RegExp(
    `^\`(?:${alternatives})\`[ \\t]*(?:\\r?\\n)?(?:\\r?\\n)?`,
    'im',
  );
  return content.replace(echo, '');
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

// Owns a single markdown-it instance and an mtime-keyed render cache. A
// regenerated document is picked up on the next request without a restart.
@Injectable()
export class MarkdownRendererService implements OnModuleInit {
  private md: any;
  private readonly cache = new Map<string, CacheEntry>();

  async onModuleInit(): Promise<void> {
    // markdown-it-anchor v9 is ESM-only; load both via native dynamic import.
    const mdMod = await dynamicImport<any>('markdown-it');
    const MarkdownIt = mdMod.default || mdMod;
    const anchorMod = await dynamicImport<any>('markdown-it-anchor');
    const anchor = anchorMod.default || anchorMod;

    this.md = new MarkdownIt({
      html: true, // REQUIRED — the <details> blocks are the documentation
      linkify: true,
      typographer: false, // keep off: it mangles quotes/dashes in config values
    });
    this.md.use(anchor, {
      permalink: anchor.permalink.headerLink(),
      tabIndex: false,
      // The plugin's default percent-encodes anything outside its allowed set,
      // which leaves ids only selectable as `[id="lifecycle-%26-operations"]`.
      slugify: slugifyHeading,
    });

    this.installLinkRewriter();
    this.installFindingsTable();
    this.installSectionHooks();
  }

  // Overrides the link renderer so `.md` hrefs become app routes at render time.
  private installLinkRewriter(): void {
    const md = this.md;
    const defaultRender =
      md.renderer.rules.link_open ||
      function (tokens: any, idx: number, options: any, _env: any, self: any) {
        return self.renderToken(tokens, idx, options);
      };
    md.renderer.rules.link_open = (
      tokens: any,
      idx: number,
      options: any,
      env: LinkEnv,
      self: any,
    ) => {
      const token = tokens[idx];
      const hrefIndex = token.attrIndex('href');
      if (hrefIndex >= 0) {
        const rewritten = rewriteHref(token.attrs[hrefIndex][1], env || {});
        if (rewritten != null) token.attrs[hrefIndex][1] = rewritten;
      }
      return defaultRender(tokens, idx, options, env, self);
    };
  }

  // Tags the tenant summary's Findings table so the stylesheet can reach it.
  // A core rule, not a renderer override: the work is on the token stream, and
  // it runs once per render like the rest of the pipeline.
  private installFindingsTable(): void {
    this.md.core.ruler.push('findings_table', (state: any) => {
      applyFindingsTable(state.tokens);
    });
  }

  // Tags the document's sections, its tool-maintained marker blocks and its
  // metadata table so the stylesheet can reach them. Pushed after the findings
  // rule, because the metadata table is identified as the first table that is
  // *not* a findings table.
  private installSectionHooks(): void {
    this.md.core.ruler.push('doc_sections', (state: any) => {
      applySectionHeadings(state.tokens);
      const markers = applyMarkerBlocks(state.tokens);
      applyMetadataTable(state.tokens);
      // Last: wrapping changes token indices, and it needs the heading slugs
      // and the marker ranges the earlier passes produced. `state.Token` is the
      // constructor markdown-it's own renderer expects.
      state.tokens = wrapSections(
        state.tokens,
        (type: string, tag: string, nesting: number) =>
          new state.Token(type, tag, nesting),
        markers,
      );
    });
  }

  // Renders the Markdown file at `file`, caching by mtime + size. Throws if the
  // file cannot be read (the caller maps that to a 404).
  async render(file: string, env: LinkEnv): Promise<RenderedPage> {
    const stat = await fs.stat(file);
    const cached = this.cache.get(file);
    if (cached && cached.mtimeMs === stat.mtimeMs && cached.size === stat.size) {
      return { html: cached.html, title: cached.title, meta: cached.meta };
    }

    const raw = await fs.readFile(file, 'utf8');
    const parsed = matter(raw);
    const content = stripSourceEcho(parsed.content, parsed.data.source);
    const title = extractTitle(content);
    const html = this.md.render(content, {
      tenant: env.tenant,
      docDir: env.docDir,
    });

    this.cache.set(file, {
      html,
      mtimeMs: stat.mtimeMs,
      size: stat.size,
      meta: parsed.data,
      title,
    });
    if (this.cache.size > MAX_ENTRIES) {
      const oldest = this.cache.keys().next().value;
      if (oldest !== undefined) this.cache.delete(oldest);
    }

    return { html, title, meta: parsed.data };
  }
}

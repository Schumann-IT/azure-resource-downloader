import { Injectable, OnModuleInit } from '@nestjs/common';
import { promises as fs } from 'fs';
import { dynamicImport } from '../dynamic-import';

export interface RenderedResource {
  html: string;
  // False when the file was served as a plain <pre> (too large, or the
  // highlighter is unavailable) — the view says so instead of pretending.
  highlighted: boolean;
  lines: number;
  size: number;
}

interface CacheEntry extends RenderedResource {
  mtimeMs: number;
}

// Files above this are emitted as an escaped <pre> instead: highlighting a
// multi-megabyte payload would stall the request for no real benefit.
export const MAX_HIGHLIGHT_BYTES = 512 * 1024;

const MAX_ENTRIES = 100;
const LIGHT_THEME = 'github-light';
const DARK_THEME = 'github-dark';

// Owns a single shiki highlighter and an mtime-keyed render cache, mirroring
// MarkdownRendererService: a re-downloaded resource is picked up on the next
// request without a restart.
@Injectable()
export class YamlHighlighterService implements OnModuleInit {
  private highlighter: any;
  private readonly cache = new Map<string, CacheEntry>();

  async onModuleInit(): Promise<void> {
    // shiki is ESM-only; load it through the native import() escape hatch.
    // A failure here must not take the process down — the views fall back to
    // an escaped <pre>.
    try {
      const shiki = await dynamicImport<any>('shiki');
      this.highlighter = await shiki.createHighlighter({
        themes: [LIGHT_THEME, DARK_THEME],
        langs: ['yaml'],
      });
    } catch {
      this.highlighter = undefined;
    }
  }

  // Renders the YAML file at `file`, caching by mtime + size. Throws if the
  // file cannot be read (the caller maps that to a 404).
  async render(file: string): Promise<RenderedResource> {
    const stat = await fs.stat(file);
    const cached = this.cache.get(file);
    if (cached && cached.mtimeMs === stat.mtimeMs && cached.size === stat.size) {
      return cached;
    }

    const raw = await fs.readFile(file, 'utf8');
    const lines = raw === '' ? 0 : raw.replace(/\n$/, '').split('\n').length;
    const highlight = this.highlighter && stat.size <= MAX_HIGHLIGHT_BYTES;
    const rendered: RenderedResource = {
      html: highlight ? this.highlight(raw) : plainBlock(raw),
      highlighted: !!highlight,
      lines,
      size: stat.size,
    };

    this.cache.set(file, { ...rendered, mtimeMs: stat.mtimeMs });
    if (this.cache.size > MAX_ENTRIES) {
      const oldest = this.cache.keys().next().value;
      if (oldest !== undefined) this.cache.delete(oldest);
    }
    return rendered;
  }

  // Dual-theme output (`defaultColor: false`) emits --shiki-light/--shiki-dark
  // CSS variables, so dark mode stays a `prefers-color-scheme` rule instead of
  // client-side theme switching.
  private highlight(code: string): string {
    return this.highlighter.codeToHtml(code, {
      lang: 'yaml',
      themes: { light: LIGHT_THEME, dark: DARK_THEME },
      defaultColor: false,
      transformers: [lineAnchors()],
    });
  }
}

// Gives every line an `id="L<n>"` and a clickable line number, so a single
// setting can be deep-linked (`#L42`) and highlighted with `.line:target` in
// CSS — no client-side JavaScript. The gutter is `user-select: none` so copying
// the block does not drag the numbers along.
function lineAnchors(): any {
  return {
    name: 'line-anchors',
    line(node: any, line: number) {
      node.properties = node.properties || {};
      node.properties.id = `L${line}`;
      node.children.unshift({
        type: 'element',
        tagName: 'a',
        properties: {
          class: 'line-no',
          href: `#L${line}`,
          'aria-label': `line ${line}`,
        },
        children: [{ type: 'text', value: String(line) }],
      });
    },
  };
}

function plainBlock(code: string): string {
  return `<pre class="shiki-plain"><code>${escapeHtml(code)}</code></pre>`;
}

function escapeHtml(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

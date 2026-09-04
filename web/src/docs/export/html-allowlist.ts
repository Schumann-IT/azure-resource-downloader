import { Parser } from 'htmlparser2';

// Re-serialises rendered document HTML into what Confluence's HTML import
// preserves. One pass over the HTML the browser already renders — the exporter
// never renders Markdown itself, so the single `markdown-it` instance and its
// mtime-keyed cache stay untouched.
//
// Four verdicts per element:
//   - keep    — on the importer's preserved list; emitted with allowed attributes only;
//   - unwrap  — real HTML the importer drops (a `<div>`, a dead anchor): tag goes, children stay;
//   - drop    — element *and* content go (scripts, embeds, form controls);
//   - escape  — not an HTML element name at all, so it was prose: `<key>` in a
//               macOS plist quote becomes `&lt;key&gt;` instead of a bogus element.
//
// That last verdict is why this is an allowlist rather than a blocklist: 44
// literal angle brackets in the reference corpus are prose that `html: true`
// already turns into phantom elements in the browser, and an import must not
// inherit them.

export interface ExportHtmlOptions {
  // Tenant segment the rendered hrefs carry, so they can be mapped to pages.
  tenant: string;
  // Extensionless document path -> page file name within the space folder.
  pageFileByDoc: Map<string, string>;
}

// Elements the importer preserves, with the attributes worth carrying. Anything
// not listed here for an element (`id`, `class`, `data-*`, `style`) is dropped:
// in-document anchors do not survive the import, and the stylesheet does not
// come along.
const KEEP: Record<string, string[]> = {
  h1: [],
  h2: [],
  h3: [],
  h4: [],
  h5: [],
  h6: [],
  p: [],
  br: [],
  hr: [],
  strong: [],
  b: [],
  em: [],
  i: [],
  u: [],
  s: [],
  del: [],
  ins: [],
  sub: [],
  sup: [],
  code: [],
  pre: [],
  blockquote: [],
  ul: [],
  ol: ['start'],
  li: [],
  table: [],
  thead: [],
  tbody: [],
  tfoot: [],
  caption: [],
  tr: [],
  th: ['colspan', 'rowspan'],
  td: ['colspan', 'rowspan'],
  a: ['href', 'title'],
  // Verified against a Confluence Cloud import: the importer turns each block
  // into a native collapsible expand, nesting and summary text intact. Passing
  // it through is therefore the representation, not a probe — do not unwrap or
  // flatten these, they are the documentation.
  details: [],
  summary: [],
};

// HTML the importer does not preserve but whose text content is documentation:
// the tag goes, the children stay.
const UNWRAP = new Set([
  'div',
  'span',
  'section',
  'article',
  'main',
  'header',
  'footer',
  'nav',
  'aside',
  'figure',
  'figcaption',
  'dl',
  'dt',
  'dd',
  'small',
  'mark',
  'kbd',
  'samp',
  'var',
  'abbr',
  'cite',
  'q',
  'time',
  'address',
  'center',
  'font',
  'tt',
  'big',
  'label',
  'fieldset',
  'legend',
  'html',
  'body',
  'picture',
  'colgroup',
  'col',
]);

// Elements whose *content* must not travel either.
const DROP = new Set([
  'script',
  'style',
  'noscript',
  'template',
  'head',
  'title',
  'meta',
  'link',
  'base',
  'iframe',
  'object',
  'embed',
  'param',
  'svg',
  'math',
  'canvas',
  'video',
  'audio',
  'source',
  'track',
  'form',
  'button',
  'input',
  'select',
  'option',
  'optgroup',
  'textarea',
  'progress',
  'meter',
  'dialog',
]);

const VOID = new Set([
  'br',
  'hr',
  'img',
  'meta',
  'link',
  'base',
  'input',
  'source',
  'track',
  'col',
  'param',
  'embed',
]);

type Verdict = 'keep' | 'unwrap' | 'drop' | 'escape';

interface Frame {
  name: string;
  verdict: Verdict;
}

export function escapeHtml(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
}

function escapeAttr(value: string): string {
  return escapeHtml(value).replace(/"/g, '&quot;');
}

// Rewrites one rendered href for the export, or returns null when the link
// cannot be a link any more (its target is not a page in this export) and must
// degrade to its own text.
export function rewriteExportHref(
  href: string,
  options: ExportHtmlOptions,
): string | null {
  if (!href) return null;
  // Heading permalinks and in-document anchors: a flat space has no anchors.
  if (href.startsWith('#')) return null;
  // External links survive verbatim.
  if (href.startsWith('//')) return href;
  if (/^[a-z][a-z0-9+.-]*:/i.test(href)) return href;

  const prefix = `/${options.tenant}/`;
  if (!href.startsWith(prefix)) return null;

  const rest = href.slice(prefix.length);
  const hashIdx = rest.indexOf('#');
  const docPath = decodeURI(hashIdx >= 0 ? rest.slice(0, hashIdx) : rest);
  // Representation prefixes (`_resource`, `_export`) are not exported pages.
  if (!docPath || docPath.startsWith('_')) return null;

  const file = options.pageFileByDoc.get(docPath);
  return file ?? null;
}

// Serialises `html` for the export. Never throws: an element it does not
// recognise degrades, it does not abort the page.
export function toConfluenceHtml(
  html: string,
  options: ExportHtmlOptions,
): string {
  const out: string[] = [];
  const stack: Frame[] = [];
  let dropDepth = 0;

  const verdictFor = (name: string): Verdict => {
    if (name in KEEP) return 'keep';
    if (UNWRAP.has(name)) return 'unwrap';
    if (DROP.has(name)) return 'drop';
    // `img` is handled before this is reached; anything else that is not an
    // HTML element name was prose in the document.
    return 'escape';
  };

  const parser = new Parser(
    {
      onopentag(name, attribs) {
        const tag = name.toLowerCase();

        if (dropDepth > 0) {
          stack.push({ name: tag, verdict: 'drop' });
          if (!VOID.has(tag)) dropDepth++;
          return;
        }

        // Media is not exported (one extension per served root, so the docs
        // root cannot hand out an image), so an image becomes its alt text.
        if (tag === 'img') {
          const alt = attribs.alt || attribs.title || '';
          if (alt) out.push(escapeHtml(alt));
          stack.push({ name: tag, verdict: 'drop' });
          return;
        }

        let verdict = verdictFor(tag);

        if (verdict === 'keep' && tag === 'a') {
          const rewritten = rewriteExportHref(attribs.href || '', options);
          if (rewritten === null) {
            verdict = 'unwrap';
          } else {
            attribs = { ...attribs, href: rewritten };
          }
        }

        stack.push({ name: tag, verdict });

        if (verdict === 'drop') {
          if (!VOID.has(tag)) dropDepth++;
          return;
        }
        if (verdict === 'unwrap') return;
        if (verdict === 'escape') {
          out.push(escapeHtml(`<${name}${serialiseRawAttrs(attribs)}>`));
          return;
        }

        const allowed = KEEP[tag] || [];
        let rendered = `<${tag}`;
        for (const attr of allowed) {
          const value = attribs[attr];
          if (value === undefined || value === '') continue;
          rendered += ` ${attr}="${escapeAttr(value)}"`;
        }
        rendered += VOID.has(tag) ? ' />' : '>';
        out.push(rendered);
      },

      onclosetag(name, isImplied) {
        const tag = name.toLowerCase();
        const frame = stack.pop();
        const verdict = frame?.verdict ?? 'unwrap';

        if (verdict === 'drop') {
          if (!VOID.has(tag) && dropDepth > 0) dropDepth--;
          return;
        }
        if (dropDepth > 0) return;
        if (verdict === 'unwrap') return;
        if (verdict === 'escape') {
          // A bare `<key>` in prose has no closing tag; the parser invents one
          // at the end of the document, and inventing `&lt;/key&gt;` with it
          // would put text in the page that the document never contained.
          if (!isImplied) out.push(escapeHtml(`</${name}>`));
          return;
        }
        if (!VOID.has(tag)) out.push(`</${tag}>`);
      },

      ontext(text) {
        if (dropDepth > 0) return;
        out.push(escapeHtml(text));
      },
    },
    {
      // Keep the author's casing so an escaped `<Key>` reads as it was written,
      // and decode entities so the output is escaped exactly once.
      lowerCaseTags: false,
      lowerCaseAttributeNames: true,
      decodeEntities: true,
      recognizeSelfClosing: true,
    },
  );

  parser.write(html);
  parser.end();
  return out.join('');
}

// Re-serialises the attributes of an escaped pseudo-element so a prose fragment
// such as `<string value="x">` reads back as it was written.
function serialiseRawAttrs(attribs: Record<string, string>): string {
  return Object.keys(attribs)
    .map((key) =>
      attribs[key] === '' ? ` ${key}` : ` ${key}="${attribs[key]}"`,
    )
    .join('');
}

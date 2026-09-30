import { DomUtils, parseDocument } from 'htmlparser2';
import type { Content, ContentText, TableCell } from 'pdfmake/interfaces';
import { driftHref } from '../drift-view';
import { DROP } from './html-allowlist';

// Turns the browser's rendered Markdown HTML into `pdfmake` content. One pass
// over the HTML the drift pages already render — the PDF never renders Markdown
// itself, so the single `markdown-it` instance and its cache stay untouched.
//
// The verdicts mirror `html-allowlist.ts`, laid out for paper instead of an
// import: headings, paragraphs, lists, tables, `strong`/`em`/`code`/`pre` are
// kept; a `<details>` is always expanded, its `<summary>` a bold lead line (CSS
// cannot open a collapsed disclosure, which is why print is not the answer);
// images become their alt text; the `DROP` elements go with their content;
// anything else is unwrapped, its children kept.
//
// Pure and Nest-free: no I/O, no URL is ever fetched, and the output never
// carries an `image`, `svg` or URL node — a link either becomes an internal
// jump to a finding's section of the same PDF or plain text.

// The font of `code` and `pre` runs: the PDF standard Courier, which encodes
// WinAnsi only. A run it cannot encode stays in the body font (Roboto).
export const CODE_FONT = 'Courier';

// The colour of an internal link, so it reads as one on paper.
const LINK_COLOR = '#1d4ed8';

// The query parameters that make a drift route a different representation (the
// highlighted, raw or diffed payload) — never a finding's section of the PDF.
const REPRESENTATION_PARAMS = ['yaml', 'raw', 'diff'];

// The width of the rule an `<hr>` becomes: the A4 text column at the report's
// 40 pt margins.
const RULE_WIDTH = 515;

export interface PdfContentOptions {
  // Tenant segment the rendered hrefs carry.
  tenant: string;
  // Keys of the findings the report has a section for; a link to any other
  // drift route stays plain text.
  findings: ReadonlySet<string>;
  // How many levels the document's headings move down, so a finding's analysis
  // nests under the finding's own heading. Clamped to h6.
  headingShift?: number;
}

type DomNode = ReturnType<typeof parseDocument>['children'][number];

// An inline run before it becomes a `pdfmake` text object.
interface Run {
  text: string;
  bold?: boolean;
  italics?: boolean;
  code?: boolean;
  strike?: boolean;
  link?: string;
  br?: boolean;
}

type RunStyle = Omit<Run, 'text' | 'br'>;

// Elements laid out as blocks of their own. Everything else is inline, or — if
// it holds a block — unwrapped as a block container.
const BLOCK = new Set([
  'h1',
  'h2',
  'h3',
  'h4',
  'h5',
  'h6',
  'p',
  'ul',
  'ol',
  'li',
  'table',
  'pre',
  'blockquote',
  'details',
  'summary',
  'hr',
  'div',
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
  'address',
  'center',
  'fieldset',
  'html',
  'body',
]);

const CODE = new Set(['code', 'kbd', 'samp', 'tt']);
const BOLD = new Set(['strong', 'b']);
const ITALICS = new Set(['em', 'i', 'cite']);
const STRIKE = new Set(['s', 'del', 'strike']);

// The characters Windows-1252 adds between 0x80 and 0x9F; with printable ASCII
// and Latin-1 they are what the standard Courier can encode.
const CP1252_EXTRA = new Set('€‚ƒ„…†‡ˆ‰Š‹ŒŽ‘’“”•–—˜™š›œžŸ');

// The named destination of a finding's section: stable for a key, and unique
// because keys are.
export function findingAnchor(key: string): string {
  return `finding:${key}`;
}

// Whether the standard Courier can draw `text` (WinAnsi); the code font is
// only used when it can.
export function isWinAnsi(text: string): boolean {
  for (const ch of text) {
    const c = ch.codePointAt(0) ?? 0;
    if (c === 0x0a || c === 0x0d) continue;
    if (c >= 0x20 && c <= 0x7e) continue;
    if (c >= 0xa0 && c <= 0xff) continue;
    if (CP1252_EXTRA.has(ch)) continue;
    return false;
  }
  return true;
}

// The finding section a rendered href points at, or null when it is not a
// finding of this report: the path (query and fragment removed) must be the
// finding's drift route, and the query must not ask for another representation.
export function internalDestination(href: string, options: PdfContentOptions): string | null {
  if (!href) return null;
  const hash = href.indexOf('#');
  const beforeHash = hash >= 0 ? href.slice(0, hash) : href;
  const q = beforeHash.indexOf('?');
  const pathPart = q >= 0 ? beforeHash.slice(0, q) : beforeHash;
  const params = new URLSearchParams(q >= 0 ? beforeHash.slice(q + 1) : '');
  if (REPRESENTATION_PARAMS.some((p) => params.has(p))) return null;

  let decoded: string;
  try {
    decoded = decodeURI(pathPart);
  } catch {
    return null;
  }
  const prefix = driftHref(options.tenant, '');
  if (!decoded.startsWith(prefix)) return null;
  const key = decoded.slice(prefix.length);
  return options.findings.has(key) ? findingAnchor(key) : null;
}

// The rendered HTML as `pdfmake` content. Never throws on odd markup: an
// element it does not know is unwrapped, not an error.
export function htmlToPdfContent(html: string, options: PdfContentOptions): Content[] {
  const doc = parseDocument(html, { decodeEntities: true, lowerCaseTags: true });
  return blocksOf(doc.children, options);
}

function blocksOf(nodes: DomNode[], options: PdfContentOptions): Content[] {
  const out: Content[] = [];
  let pending: Run[] = [];
  const flush = () => {
    const paragraph = textNode(pending, 'p');
    if (paragraph) out.push(paragraph);
    pending = [];
  };
  for (const node of nodes) {
    if (DomUtils.isText(node)) {
      pending.push({ text: node.data });
      continue;
    }
    if (!DomUtils.isTag(node)) continue;
    const tag = node.name.toLowerCase();
    if (DROP.has(tag)) continue;
    if (BLOCK.has(tag) || containsBlock(node)) {
      flush();
      out.push(...blockOf(node, tag, options));
      continue;
    }
    pending.push(...inlineOf(node, options, {}));
  }
  flush();
  return out;
}

function blockOf(node: DomElement, tag: string, options: PdfContentOptions): Content[] {
  const heading = /^h([1-6])$/.exec(tag);
  if (heading) {
    const level = Math.min(6, Number(heading[1]) + (options.headingShift ?? 0));
    const text = textNode(inlineRuns(node.children, options, {}), `h${level}`);
    return text ? [text] : [];
  }
  switch (tag) {
    case 'p': {
      const text = textNode(inlineRuns(node.children, options, {}), 'p');
      return text ? [text] : [];
    }
    case 'summary': {
      const text = textNode(inlineRuns(node.children, options, {}), 'summary');
      return text ? [text] : [];
    }
    case 'ul':
    case 'ol':
      return listOf(node, tag, options);
    case 'table':
      return tableOf(node, options);
    case 'pre':
      return [preOf(node)];
    case 'blockquote':
      return [{ stack: blocksOf(node.children, options), style: 'blockquote' }];
    case 'details':
      return [detailsOf(node, options)];
    case 'hr':
      return [
        {
          canvas: [
            { type: 'line', x1: 0, y1: 0, x2: RULE_WIDTH, y2: 0, lineWidth: 0.5, lineColor: '#cbd5e1' },
          ],
          margin: [0, 6, 0, 6],
        },
      ];
    default:
      // A container (and a stray `<li>`): its children are the content.
      return blocksOf(node.children, options);
  }
}

// Always expanded: the summary as a bold lead line, then everything else.
function detailsOf(node: DomElement, options: PdfContentOptions): Content {
  const stack: Content[] = [];
  const rest: DomNode[] = [];
  for (const child of node.children) {
    if (DomUtils.isTag(child) && child.name.toLowerCase() === 'summary') {
      stack.push(...blockOf(child, 'summary', options));
    } else {
      rest.push(child);
    }
  }
  stack.push(...blocksOf(rest, options));
  return { stack, style: 'details' };
}

function listOf(node: DomElement, tag: 'ul' | 'ol', options: PdfContentOptions): Content[] {
  const items: Content[] = [];
  for (const child of node.children) {
    if (!DomUtils.isTag(child) || child.name.toLowerCase() !== 'li') continue;
    const blocks = blocksOf(child.children, options);
    if (blocks.length === 0) continue;
    items.push(blocks.length === 1 ? blocks[0] : { stack: blocks });
  }
  if (items.length === 0) return [];
  if (tag === 'ul') return [{ ul: items, style: 'list' }];
  const start = Number.parseInt(node.attribs.start ?? '', 10);
  return [
    Number.isFinite(start) ? { ol: items, start, style: 'list' } : { ol: items, style: 'list' },
  ];
}

// A table with every row padded to the widest, a `colspan` honoured with the
// placeholders pdfmake expects, and the `<thead>` rows repeated on each page.
function tableOf(node: DomElement, options: PdfContentOptions): Content[] {
  const rows: Array<{ header: boolean; cells: DomElement[] }> = [];
  const collect = (parent: DomElement, header: boolean) => {
    for (const child of parent.children) {
      if (!DomUtils.isTag(child)) continue;
      const tag = child.name.toLowerCase();
      if (tag === 'tr') {
        rows.push({
          header,
          cells: child.children.filter(
            (c): c is DomElement =>
              DomUtils.isTag(c) && (c.name.toLowerCase() === 'td' || c.name.toLowerCase() === 'th'),
          ),
        });
      } else if (tag === 'thead' || tag === 'tbody' || tag === 'tfoot') {
        collect(child, tag === 'thead');
      }
    }
  };
  collect(node, false);

  const body: TableCell[][] = rows.map((row) => {
    const cells: TableCell[] = [];
    for (const cell of row.cells) {
      const span = Math.max(1, Number.parseInt(cell.attribs.colspan ?? '', 10) || 1);
      const blocks = blocksOf(cell.children, options);
      const style = cell.name.toLowerCase() === 'th' ? 'tableHeader' : undefined;
      cells.push({
        ...(blocks.length > 0 ? { stack: blocks } : { text: '' }),
        ...(style ? { style } : {}),
        ...(span > 1 ? { colSpan: span } : {}),
      });
      // pdfmake expects a placeholder for every column a cell spans.
      for (let i = 1; i < span; i++) cells.push({});
    }
    return cells;
  });
  const columns = Math.max(0, ...body.map((r) => r.length));
  if (columns === 0) return [];
  for (const row of body) {
    while (row.length < columns) row.push({ text: '' });
  }
  let headerRows = 0;
  while (headerRows < rows.length && rows[headerRows].header) headerRows++;
  return [
    {
      table: { headerRows, widths: Array(columns).fill('*'), body },
      layout: 'lightHorizontalLines',
      style: 'table',
    },
  ];
}

function preOf(node: DomElement): Content {
  const text = DomUtils.textContent(node).replace(/\t/g, '    ').replace(/\r?\n$/, '');
  const pre: ContentText = { text, style: 'pre', preserveLeadingSpaces: true };
  if (isWinAnsi(text)) pre.font = CODE_FONT;
  return pre;
}

function inlineRuns(nodes: DomNode[], options: PdfContentOptions, style: RunStyle): Run[] {
  const runs: Run[] = [];
  for (const node of nodes) {
    if (DomUtils.isText(node)) {
      runs.push({ ...style, text: node.data });
    } else if (DomUtils.isTag(node)) {
      runs.push(...inlineOf(node, options, style));
    }
  }
  return runs;
}

function inlineOf(node: DomElement, options: PdfContentOptions, style: RunStyle): Run[] {
  const tag = node.name.toLowerCase();
  if (DROP.has(tag)) return [];
  if (tag === 'br') return [{ text: '\n', br: true }];
  if (tag === 'img') {
    // Media is not exported: an image becomes its alt text.
    const alt = node.attribs.alt || node.attribs.title || '';
    return alt ? [{ ...style, text: alt }] : [];
  }
  const next: RunStyle = { ...style };
  if (BOLD.has(tag)) next.bold = true;
  if (ITALICS.has(tag)) next.italics = true;
  if (CODE.has(tag)) next.code = true;
  if (STRIKE.has(tag)) next.strike = true;
  if (tag === 'a') {
    const destination = internalDestination(node.attribs.href ?? '', options);
    if (destination) next.link = destination;
  }
  const runs = inlineRuns(node.children, options, next);
  // A block that ended up inline (a `<p>` inside a `<summary>`) still reads as
  // a separate phrase.
  return BLOCK.has(tag) ? [{ text: ' ' }, ...runs, { text: ' ' }] : runs;
}

// A paragraph-like text node, or null when it holds no visible text.
function textNode(runs: Run[], style: string): Content | null {
  const normalised = normalise(runs);
  if (normalised.every((r) => r.br)) return null;
  return { text: normalised.map(toPdfRun), style };
}

// HTML whitespace rules for inline runs: collapsed to one space, dropped at the
// start and end of a line. `\s` is not used — it would also collapse a
// non-breaking space.
function normalise(runs: Run[]): Run[] {
  const out: Run[] = [];
  let lineStart = true;
  for (const run of runs) {
    if (run.br) {
      trimLast(out);
      out.push(run);
      lineStart = true;
      continue;
    }
    let text = run.text.replace(/[ \t\n\r\f]+/g, ' ');
    if (lineStart || endsWithSpace(out)) text = text.replace(/^ /, '');
    if (text === '') continue;
    out.push({ ...run, text });
    lineStart = false;
  }
  trimLast(out);
  return out.filter((r) => r.br || r.text !== '');
}

function endsWithSpace(runs: Run[]): boolean {
  const last = runs[runs.length - 1];
  return !!last && !last.br && last.text.endsWith(' ');
}

function trimLast(runs: Run[]): void {
  const last = runs[runs.length - 1];
  if (last && !last.br) last.text = last.text.replace(/ $/, '');
}

function toPdfRun(run: Run): ContentText {
  if (run.br) return { text: '\n' };
  const out: ContentText = { text: run.text };
  if (run.bold) out.bold = true;
  if (run.italics) out.italics = true;
  if (run.code) {
    out.text = run.text.replace(/\t/g, '    ');
    out.style = 'code';
    if (isWinAnsi(run.text)) out.font = CODE_FONT;
  }
  if (run.link) {
    out.linkToDestination = run.link;
    out.color = LINK_COLOR;
  }
  if (run.strike) out.decoration = 'lineThrough';
  else if (run.link) out.decoration = 'underline';
  return out;
}

// Whether an element that is not a block itself holds one — then it is
// unwrapped as a block container instead of being flattened into a line.
function containsBlock(node: DomElement): boolean {
  return node.children.some(
    (c) => DomUtils.isTag(c) && (BLOCK.has(c.name.toLowerCase()) || containsBlock(c)),
  );
}

type DomElement = Extract<DomNode, { attribs: Record<string, string> }>;

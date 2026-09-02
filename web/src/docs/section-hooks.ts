// Makes the parts of a generated document addressable from CSS.
//
// The CLI declares each document type's H2 headings as a closed, verbatim set
// (the `<!-- doc-headings: … -->` contract in its `doc-prompt.md`), so the
// heading text is a machine contract rather than prose. That is what lets the
// stylesheet give a section its own icon and colour without any positional
// selector — and why a heading *outside* the union below is deliberately left
// unstyled instead of guessed at.
//
// Kept pure and Nest-free (like `link-rewrite.ts` and `findings-table.ts`): it
// only rewrites a markdown-it token stream, so it is unit-testable without a
// module, and it costs one linear pass inside the existing render.

export const SECTION_HEADING_CLASS = 'doc-section-heading';

export const METADATA_TABLE_CLASS = 'doc-metadata';

// The union of the six per-template heading sets, plus the tenant summary's
// own vocabulary. Slugs, not headings: `slugifyHeading` is what maps one to the
// other, and the stylesheet keys on these values.
//
//   default / singleton  references, lifecycle-and-operations, security, settings
//   arm                  references, lifecycle-and-operations, security, properties
//   group                membership, usage-as-assignment-target, security, properties
//   credential           + expiry-and-renewal
//   record               references, lifecycle-and-operations, properties
//   referenced           + usage-and-references, definition
//   summary.md           management-summary, at-a-glance, assignment-posture,
//                        coverage-caveats (+ H3 findings, recommendations)
//
// `targeted-by` and `used-by` are spliced into a document inside a marker pair
// but are real H2s in the body, so they belong here too.
export const SECTION_VOCABULARY: readonly string[] = [
  'references',
  'lifecycle-and-operations',
  'security',
  'settings',
  'properties',
  'definition',
  'membership',
  'usage-as-assignment-target',
  'usage-and-references',
  'expiry-and-renewal',
  'targeted-by',
  'used-by',
  'management-summary',
  'at-a-glance',
  'assignment-posture',
  'coverage-caveats',
  'findings',
  'recommendations',
];

const KNOWN: ReadonlySet<string> = new Set(SECTION_VOCABULARY);

// Tool-maintained marker pairs that wrap a re-spliceable block. HTML comments
// survive `html: true` into the DOM but are not selectable, so each pair
// becomes a `<div>` — a better hook than any positional selector, because the
// markers are the CLI's own contract. `<section>` is not used: the wrapper sits
// inside the flat H2 run, and a `<section>` there would be a false outline
// entry.
export const MARKER_BLOCK_CLASSES: Readonly<Record<string, string>> = {
  assignments: 'doc-assignments',
  'targeted-by': 'doc-targeted-by',
  'used-by': 'doc-used-by',
  notifications: 'doc-notifications',
};

export const SECTION_CLASS = 'doc-section';

// Where a matched marker pair opens and closes in the token stream. Section
// wrapping needs it: a spliced block must not be cut in half by a `<section>`.
export interface MarkerRange {
  name: string;
  start: number;
  end: number;
}

const MARKER = /<!--\s*([a-z][a-z-]*):(start|end)\s*-->/i;

// Slug for a heading. Replaces `markdown-it-anchor`'s default, which
// percent-encodes anything outside its allowed set: the em dash in every
// `summary.md` H1 becomes `%E2%80%94` and an ampersand `%26`, leaving ids that
// can only be selected as `[id="lifecycle-%26-operations"]`.
//
// `&` maps to `and` on purpose. The current heading contract spells
// `Lifecycle and operations`, older documents on disk spell it
// `Lifecycle & operations`, and mapping both to one slug keeps `data-section`
// (and the anchor URL) stable across a regeneration.
export function slugifyHeading(text: string): string {
  return String(text)
    .toLowerCase()
    .replace(/[`*_~]/g, '')
    .replace(/&(?:amp;)?/g, ' and ')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '');
}

export function isKnownSection(slug: string): boolean {
  return KNOWN.has(slug);
}

// Puts `data-section="<slug>"` on every H2/H3, and the styling class on the
// ones whose heading is in the declared vocabulary. Both are needed: the slug
// makes any section addressable, the class is what says "this heading is a
// contract" and gates the icon, so an unrecognised heading renders as plain
// prose instead of borrowing another section's identity.
export function applySectionHeadings(tokens: any[]): void {
  for (let i = 0; i < tokens.length; i++) {
    const token = tokens[i];
    if (token.type !== 'heading_open') continue;
    if (token.tag !== 'h2' && token.tag !== 'h3') continue;

    const inline = tokens[i + 1];
    if (!inline || inline.type !== 'inline') continue;

    const slug = slugifyHeading(String(inline.content || ''));
    if (!slug) continue;

    token.attrSet('data-section', slug);
    if (isKnownSection(slug)) {
      token.attrJoin('class', SECTION_HEADING_CLASS);
    }
  }
}

// Rewrites each *matched* marker pair into an opening and closing `<div>` and
// returns where those pairs sit, which is what keeps section wrapping from
// cutting a spliced block in half. Unmatched markers are left as comments: half
// a pair would emit an unbalanced element, and the export serialiser and the
// browser would both have to guess.
export function applyMarkerBlocks(tokens: any[]): MarkerRange[] {
  const ranges: MarkerRange[] = [];
  const open = new Map<string, Array<{ index: number; text: string }>>();
  const rewrite = new Map<number, Array<[string, string]>>();
  // Own instance: a module-level /g regex would carry `lastIndex` between
  // renders.
  const marker = new RegExp(MARKER.source, 'gi');

  const queue = (index: number, from: string, to: string): void => {
    const edits = rewrite.get(index) || [];
    edits.push([from, to]);
    rewrite.set(index, edits);
  };

  for (let i = 0; i < tokens.length; i++) {
    if (tokens[i].type !== 'html_block' && tokens[i].type !== 'html_inline') {
      continue;
    }
    const content = String(tokens[i].content || '');
    marker.lastIndex = 0;
    let match: RegExpExecArray | null;
    while ((match = marker.exec(content)) !== null) {
      const name = match[1].toLowerCase();
      const cls = MARKER_BLOCK_CLASSES[name];
      if (!cls) continue;
      if (match[2].toLowerCase() === 'start') {
        const starts = open.get(name) || [];
        starts.push({ index: i, text: match[0] });
        open.set(name, starts);
        continue;
      }
      const start = open.get(name)?.pop();
      if (!start) continue; // end without a start: leave it alone
      // The matched text, not a reconstructed marker: the comment's exact
      // spacing is the generator's, not ours.
      queue(start.index, start.text, `<div class="${cls}">`);
      queue(i, match[0], '</div>');
      ranges.push({ name, start: start.index, end: i });
    }
  }

  for (const [index, edits] of rewrite) {
    let content = String(tokens[index].content || '');
    for (const [from, to] of edits) {
      content = content.split(from).join(to);
    }
    tokens[index].content = content;
  }

  return ranges;
}

// Wraps everything between two H2s in `<section class="doc-section">` carrying
// the same `data-section` as its heading, so a section can own a panel, a rail
// or its own density. Without it, `h2#settings ~ *` is the only way to reach a
// section's content and it bleeds into every section that follows.
//
// `makeToken` comes from the caller (`state.Token`) rather than being imported:
// this file stays free of the ESM-only markdown-it import, and the tokens are
// the ones markdown-it's own renderer expects.
//
// An H2 *inside* a matched marker pair never opens a section. That single rule
// is what guarantees well-formed output: a section can only close at an H2
// outside every range, so a spliced `<div>` always ends up wholly inside one
// section (or wholly in the pre-section prelude) and can never be straddled.
// It also keeps the spliced `## Used by` / `## Targeted by` blocks reading as
// part of the section they were spliced into, which is what they are.
export function wrapSections(
  tokens: any[],
  makeToken: (type: string, tag: string, nesting: number) => any,
  ranges: readonly MarkerRange[] = [],
): any[] {
  const inRange = (index: number): boolean =>
    ranges.some((r) => index > r.start && index < r.end);

  const out: any[] = [];
  let openSection = false;

  const close = (): void => {
    if (!openSection) return;
    out.push(makeToken('section_close', 'section', -1));
    openSection = false;
  };

  for (let i = 0; i < tokens.length; i++) {
    const token = tokens[i];
    const slug =
      token.type === 'heading_open' && token.tag === 'h2'
        ? token.attrGet('data-section')
        : null;

    if (slug && !inRange(i)) {
      close();
      const open = makeToken('section_open', 'section', 1);
      // Deliberately not `block`: a block token gets a trailing newline, and the
      // Confluence exporter unwraps `<section>` but keeps the text between the
      // tags — which would add a blank line to every exported page for a wrapper
      // that exists only for the stylesheet. This keeps the export byte-identical.
      open.attrSet('class', SECTION_CLASS);
      open.attrSet('data-section', slug);
      out.push(open);
      openSection = true;
    }
    out.push(token);
  }
  close();

  return out;
}

// Classes the metadata table the generated documents open with.
//
// Located as the document's *first* table after the H1 rather than by
// `table:first-of-type`, which breaks on the 99 documents that open with an
// extra heading: the metadata table always precedes the assignments table, so
// "first" holds under both the current and the older layout. The scan stops at
// that first table either way, so the assignments table can never inherit the
// class — and a findings table (already tagged by `findings-table.ts`, which
// runs first) is not a metadata table at all.
export function applyMetadataTable(tokens: any[]): void {
  let seenTitle = false;

  for (const token of tokens) {
    if (token.type === 'heading_open' && token.tag === 'h1') {
      seenTitle = true;
      continue;
    }
    if (token.type !== 'table_open') continue;

    const classes = String(token.attrGet('class') || '');
    if (seenTitle && !classes.split(/\s+/).includes('findings')) {
      token.attrJoin('class', METADATA_TABLE_CLASS);
    }
    return;
  }
}

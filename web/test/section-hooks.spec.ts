import {
  METADATA_TABLE_CLASS,
  SECTION_CLASS,
  SECTION_HEADING_CLASS,
  SECTION_VOCABULARY,
  applyMarkerBlocks,
  applyMetadataTable,
  applySectionHeadings,
  isKnownSection,
  slugifyHeading,
  wrapSections,
} from '../src/docs/section-hooks';

// Unit tests for the pure token rewriter behind the section styling hooks. It
// is Nest-free on purpose, so this needs no module and no fixture tree; the
// end-to-end wiring is covered by test/docs.e2e.spec.ts.

// The markdown-it token surface the rules actually use.
class FakeToken {
  attrs: Array<[string, string]> = [];

  constructor(
    public type: string,
    public tag = '',
    public content = '',
  ) {}

  attrIndex(name: string): number {
    return this.attrs.findIndex(([key]) => key === name);
  }

  attrGet(name: string): string | null {
    const idx = this.attrIndex(name);
    return idx < 0 ? null : this.attrs[idx][1];
  }

  attrSet(name: string, value: string): void {
    const idx = this.attrIndex(name);
    if (idx < 0) this.attrs.push([name, value]);
    else this.attrs[idx][1] = value;
  }

  attrJoin(name: string, value: string): void {
    const current = this.attrGet(name);
    this.attrSet(name, current ? `${current} ${value}` : value);
  }
}

function heading(tag: string, text: string): FakeToken[] {
  return [
    new FakeToken('heading_open', tag),
    new FakeToken('inline', '', text),
    new FakeToken('heading_close', tag),
  ];
}

describe('slugifyHeading', () => {
  it('produces plain slugs where the anchor plugin percent-encodes', () => {
    // The em dash in every summary.md H1 became %E2%80%94.
    expect(slugifyHeading('My Tenant — Intune and Entra configuration')).toBe(
      'my-tenant-intune-and-entra-configuration',
    );
    expect(slugifyHeading('Lifecycle and operations')).toBe(
      'lifecycle-and-operations',
    );
  });

  it('maps an ampersand to `and`, so a heading keeps its slug either way', () => {
    // Documents on disk predate the current contract and spell it with `&`;
    // both must resolve to the same section and the same anchor URL.
    expect(slugifyHeading('Lifecycle & operations')).toBe(
      slugifyHeading('Lifecycle and operations'),
    );
  });

  it('drops inline markup and edge punctuation', () => {
    expect(slugifyHeading('`Settings`')).toBe('settings');
    expect(slugifyHeading('**Security**')).toBe('security');
    expect(slugifyHeading('   Usage as assignment target ')).toBe(
      'usage-as-assignment-target',
    );
  });

  it('has no slug for a heading with nothing sluggable in it', () => {
    expect(slugifyHeading('—')).toBe('');
  });

  it('covers every declared section vocabulary entry', () => {
    // Guards against a slug in the stylesheet that no heading can produce.
    for (const slug of SECTION_VOCABULARY) {
      expect(slugifyHeading(slug.replace(/-/g, ' '))).toBe(slug);
      expect(isKnownSection(slug)).toBe(true);
    }
  });
});

describe('applySectionHeadings', () => {
  it('tags a declared H2 with both the slug and the styling class', () => {
    const tokens = heading('h2', 'Security');
    applySectionHeadings(tokens as any);
    expect(tokens[0].attrGet('data-section')).toBe('security');
    expect(tokens[0].attrGet('class')).toBe(SECTION_HEADING_CLASS);
  });

  it('leaves an unrecognised heading addressable but unstyled', () => {
    const tokens = heading('h2', 'Metadata');
    applySectionHeadings(tokens as any);
    expect(tokens[0].attrGet('data-section')).toBe('metadata');
    expect(tokens[0].attrGet('class')).toBeNull();
  });

  it('tags the summary H3 vocabulary too', () => {
    const tokens = heading('h3', 'Findings');
    applySectionHeadings(tokens as any);
    expect(tokens[0].attrGet('data-section')).toBe('findings');
    expect(tokens[0].attrGet('class')).toBe(SECTION_HEADING_CLASS);
  });

  it('never touches the document title', () => {
    const tokens = heading('h1', 'Policy One');
    applySectionHeadings(tokens as any);
    expect(tokens[0].attrs).toEqual([]);
  });
});

describe('applyMarkerBlocks', () => {
  it('turns a matched marker pair into a div', () => {
    const tokens = [
      new FakeToken('html_block', '', '<!-- assignments:start -->\n'),
      new FakeToken('paragraph_open', 'p'),
      new FakeToken('html_block', '', '<!-- assignments:end -->\n'),
    ];
    applyMarkerBlocks(tokens as any);
    expect(tokens[0].content).toBe('<div class="doc-assignments">\n');
    expect(tokens[2].content).toBe('</div>\n');
  });

  it('handles the targeted-by and notifications pairs', () => {
    const tokens = [
      new FakeToken('html_block', '', '<!-- targeted-by:start -->\n'),
      new FakeToken('html_block', '', '<!-- targeted-by:end -->\n'),
      new FakeToken('html_block', '', '<!--notifications:start-->\n'),
      new FakeToken('html_block', '', '<!--notifications:end-->\n'),
    ];
    applyMarkerBlocks(tokens as any);
    expect(tokens[0].content).toContain('class="doc-targeted-by"');
    expect(tokens[1].content).toBe('</div>\n');
    // The generator owns the comment's spacing, so the exact match is replaced.
    expect(tokens[2].content).toContain('class="doc-notifications"');
    expect(tokens[3].content).toBe('</div>\n');
  });

  it('leaves an unmatched marker alone rather than emitting an unbalanced tag', () => {
    const tokens = [
      new FakeToken('html_block', '', '<!-- assignments:start -->\n'),
      new FakeToken('html_block', '', '<!-- assignments:start -->\n'),
      new FakeToken('html_block', '', '<!-- assignments:end -->\n'),
      new FakeToken('html_block', '', '<!-- targeted-by:end -->\n'),
    ];
    applyMarkerBlocks(tokens as any);
    // The innermost start pairs with the end; the outer one stays a comment.
    expect(tokens[0].content).toBe('<!-- assignments:start -->\n');
    expect(tokens[1].content).toBe('<div class="doc-assignments">\n');
    expect(tokens[2].content).toBe('</div>\n');
    // An end without a start is not a block either.
    expect(tokens[3].content).toBe('<!-- targeted-by:end -->\n');
  });

  it('ignores marker names it does not own', () => {
    const tokens = [
      new FakeToken('html_block', '', '<!-- worklist:start -->\n'),
      new FakeToken('html_block', '', '<!-- worklist:end -->\n'),
    ];
    applyMarkerBlocks(tokens as any);
    expect(tokens[0].content).toBe('<!-- worklist:start -->\n');
  });
});

describe('wrapSections', () => {
  const makeToken = (type: string, tag: string, nesting: number) =>
    new FakeToken(type, tag);

  function wrapped(tokens: FakeToken[], ranges: any[] = []) {
    applySectionHeadings(tokens as any);
    return wrapSections(tokens as any, makeToken as any, ranges).map(
      (t: FakeToken) =>
        t.type === 'section_open'
          ? `<${t.attrGet('data-section')}`
          : t.type === 'section_close'
            ? '>'
            : t.type,
    );
  }

  it('wraps each H2 run and leaves the pre-section prelude alone', () => {
    const tokens = [
      ...heading('h1', 'Policy One'),
      new FakeToken('table_open', 'table'),
      ...heading('h2', 'Security'),
      new FakeToken('paragraph_open', 'p'),
      ...heading('h2', 'Settings'),
    ];
    expect(wrapped(tokens)).toEqual([
      'heading_open',
      'inline',
      'heading_close',
      'table_open',
      '<security',
      'heading_open',
      'inline',
      'heading_close',
      'paragraph_open',
      '>',
      '<settings',
      'heading_open',
      'inline',
      'heading_close',
      '>',
    ]);
  });

  it('never opens a section inside a spliced marker block', () => {
    // `## Used by` is spliced into the middle of another section, so wrapping it
    // would close that section inside the block's <div> and emit mis-nested
    // HTML.
    const tokens = [
      ...heading('h2', 'Usage and references'),
      new FakeToken('html_block', '', '<div class="doc-used-by">\n'),
      ...heading('h2', 'Used by'),
      new FakeToken('html_block', '', '</div>\n'),
      ...heading('h2', 'Security'),
    ];
    const ranges = [{ name: 'used-by', start: 3, end: 7 }];
    expect(wrapped(tokens, ranges)).toEqual([
      '<usage-and-references',
      'heading_open',
      'inline',
      'heading_close',
      'html_block',
      'heading_open',
      'inline',
      'heading_close',
      'html_block',
      '>',
      '<security',
      'heading_open',
      'inline',
      'heading_close',
      '>',
    ]);
  });

  it('ignores H3s, so a section is one H2 run', () => {
    const tokens = [...heading('h2', 'Settings'), ...heading('h3', 'Findings')];
    const out = wrapped(tokens);
    expect(out.filter((t) => t === '>')).toHaveLength(1);
    expect(out.filter((t) => String(t).startsWith('<'))).toEqual(['<settings']);
  });

  it('carries the heading slug onto the wrapper, declared or not', () => {
    const tokens = heading('h2', 'Something Else');
    const out = wrapSections(
      (applySectionHeadings(tokens as any), tokens) as any,
      makeToken as any,
    );
    expect(out[0].attrGet('class')).toBe(SECTION_CLASS);
    expect(out[0].attrGet('data-section')).toBe('something-else');
  });
});

describe('applyMarkerBlocks ranges', () => {
  it('reports where each matched pair sits, for the section wrapper', () => {
    const tokens = [
      new FakeToken('html_block', '', '<!-- used-by:start -->\n'),
      new FakeToken('paragraph_open', 'p'),
      new FakeToken('html_block', '', '<!-- used-by:end -->\n'),
      new FakeToken('html_block', '', '<!-- assignments:end -->\n'),
    ];
    const ranges = applyMarkerBlocks(tokens as any);
    expect(ranges).toEqual([{ name: 'used-by', start: 0, end: 2 }]);
    expect(tokens[0].content).toBe('<div class="doc-used-by">\n');
  });
});

describe('applyMetadataTable', () => {
  const table = () => new FakeToken('table_open', 'table');

  it('classes the first table after the title', () => {
    const tokens = [...heading('h1', 'Policy One'), table(), table()];
    applyMetadataTable(tokens as any);
    expect(tokens[3].attrGet('class')).toBe(METADATA_TABLE_CLASS);
    // The assignments table that follows must not inherit it.
    expect(tokens[4].attrGet('class')).toBeNull();
  });

  it('survives a document that opens with an extra heading', () => {
    const tokens = [
      ...heading('h1', 'Policy One'),
      ...heading('h2', 'Metadata'),
      table(),
    ];
    applyMetadataTable(tokens as any);
    expect(tokens[6].attrGet('class')).toBe(METADATA_TABLE_CLASS);
  });

  it('does not treat a findings table as metadata, and stops there', () => {
    const findings = table();
    findings.attrSet('class', 'findings');
    const tokens = [...heading('h1', 'My Tenant'), findings, table()];
    applyMetadataTable(tokens as any);
    expect(tokens[3].attrGet('class')).toBe('findings');
    expect(tokens[4].attrGet('class')).toBeNull();
  });

  it('ignores a table that precedes any title', () => {
    const tokens = [table()];
    applyMetadataTable(tokens as any);
    expect(tokens[0].attrGet('class')).toBeNull();
  });
});

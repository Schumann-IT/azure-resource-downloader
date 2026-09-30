import { applyChangedBy, applyFindingsTable } from '../src/docs/findings-table';

// Unit tests for the pure token rewriter behind the drift index's Changed by
// column. Hand-built tokens over the minimal surface the rewriter uses; the
// end-to-end wiring is covered by test/docs.e2e.spec.ts.

class FakeToken {
  attrs: Array<[string, string]> = [];
  children: FakeToken[] | null = null;

  constructor(
    public type: string,
    public tag = '',
    public nesting = 0,
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

const make = (type: string, tag: string, nesting: number) => new FakeToken(type, tag, nesting);

function cell(tag: 'th' | 'td', content: string, href?: string): FakeToken[] {
  const inline = new FakeToken('inline', '', 0, content);
  inline.children = href
    ? [
        Object.assign(new FakeToken('link_open', 'a', 1), { attrs: [['href', href]] }),
        new FakeToken('text', '', 0, content),
        new FakeToken('link_close', 'a', -1),
      ]
    : [new FakeToken('text', '', 0, content)];
  return [new FakeToken(`${tag}_open`, tag, 1), inline, new FakeToken(`${tag}_close`, tag, -1)];
}

function table(headers: string[], rows: Array<Array<[string, string?]>>): FakeToken[] {
  const out: FakeToken[] = [
    new FakeToken('table_open', 'table', 1),
    new FakeToken('thead_open', 'thead', 1),
    new FakeToken('tr_open', 'tr', 1),
  ];
  for (const h of headers) out.push(...cell('th', h));
  out.push(new FakeToken('tr_close', 'tr', -1), new FakeToken('thead_close', 'thead', -1));
  out.push(new FakeToken('tbody_open', 'tbody', 1));
  for (const row of rows) {
    out.push(new FakeToken('tr_open', 'tr', 1));
    for (const [text, href] of row) out.push(...cell('td', text, href));
    out.push(new FakeToken('tr_close', 'tr', -1));
  }
  out.push(new FakeToken('tbody_close', 'tbody', -1), new FakeToken('table_close', 'table', -1));
  return out;
}

const HEADERS = ['Severity', 'Verdict', 'Resource', 'Judgment'];
const T = 'Microsoft.Graph/deviceConfigurations';

const CELLS = new Map([
  [`${T}/a`, { text: 'alice \u00b7 2026-01-20T09:00:00Z (+1 more)', tone: 'matched' }],
  [`${T}/b`, { text: 'no audit event in the window', tone: 'warning' }],
  ['x/renamed', { text: 'no attribution recorded', tone: 'quiet' }],
]);

function run(rows: Array<Array<[string, string?]>>, headers = HEADERS, cells = CELLS) {
  const tokens = table(headers, rows);
  applyFindingsTable(tokens);
  applyChangedBy(tokens, cells, make);
  return tokens;
}

const cellsOf = (tokens: FakeToken[]) =>
  tokens.filter((t) => t.type === 'td_open' && t.attrGet('data-column') === 'changed-by');

const textOf = (tokens: FakeToken[], open: FakeToken) => tokens[tokens.indexOf(open) + 1];

const row = (link?: string): Array<[string, string?]> => [['high'], ['changed'], ['res', link], ['j']];

describe('applyChangedBy', () => {
  it('appends a header and a cell per row, last in the row', () => {
    const tokens = run([row(`${T}/a.md`), row(`${T}/b.md`)]);
    const th = tokens.filter((t) => t.type === 'th_open' && t.attrGet('data-column') === 'changed-by');
    expect(th).toHaveLength(1);
    expect(textOf(tokens, th[0]).content).toBe('Changed by');
    expect(tokens[tokens.indexOf(th[0]) + 3].type).toBe('tr_close');

    const cells = cellsOf(tokens);
    expect(cells).toHaveLength(2);
    expect(textOf(tokens, cells[0]).content).toBe('alice \u00b7 2026-01-20T09:00:00Z (+1 more)');
    expect(cells[0].attrGet('data-attribution')).toBe('matched');
    expect(cells[1].attrGet('data-attribution')).toBe('warning');
    expect(tokens[tokens.indexOf(cells[0]) + 3].type).toBe('tr_close');
  });

  it('reads the key from a nested path, strips the anchor and the ./ prefix', () => {
    const tokens = run([row(`./${T}/a.md#section`), row(`${T}/../deviceConfigurations/b.MD`)]);
    expect(cellsOf(tokens).map((c) => c.attrGet('data-attribution'))).toEqual(['matched', 'warning']);
  });

  it('joins a rename by its new path', () => {
    const tokens = run([row('x/renamed.md')]);
    expect(cellsOf(tokens)[0].attrGet('data-attribution')).toBe('quiet');
  });

  it('leaves the cell empty without a usable key', () => {
    const tokens = run([
      row(),
      row('../../../docs/x/a.md'),
      row('/abs/a.md'),
      row('//host/a.md'),
      row(`https://e.example/${T}/a.md`),
      row(`${T}/a.txt`),
      row(`${T}/unknown.md`),
    ]);
    const cells = cellsOf(tokens);
    expect(cells).toHaveLength(7);
    for (const c of cells) {
      expect(c.attrGet('data-attribution')).toBeNull();
      expect(textOf(tokens, c).content).toBe('');
    }
  });

  it('keeps an actor with markup as a text child, never html_inline', () => {
    const cells = new Map([[`${T}/a`, { text: '<script>alert(1)</script>', tone: 'matched' }]]);
    const tokens = run([row(`${T}/a.md`)], HEADERS, cells);
    const inline = textOf(tokens, cellsOf(tokens)[0]);
    expect(inline.children!.map((c) => c.type)).toEqual(['text']);
    expect(inline.children![0].content).toBe('<script>alert(1)</script>');
    expect(tokens.some((t) => t.type === 'html_inline')).toBe(false);
  });

  it('leaves tables without a Verdict or a Resource column untouched', () => {
    const summary = table(['Severity', 'Finding', 'Affected', 'Documents'], [[['high'], ['f'], ['1'], ['d']]]);
    const noResource = table(['Severity', 'Verdict', 'Judgment'], [[['high'], ['changed'], ['j']]]);
    for (const tokens of [summary, noResource]) {
      applyFindingsTable(tokens);
      const before = tokens.map((t) => t.type);
      applyChangedBy(tokens, CELLS, make);
      expect(tokens.map((t) => t.type)).toEqual(before);
    }
  });

  it('leaves a non-findings table untouched', () => {
    const tokens = table(['Name', 'Resource'], [[['n'], ['r', `${T}/a.md`]]]);
    const before = tokens.map((t) => t.type);
    applyChangedBy(tokens, CELLS, make);
    expect(tokens.map((t) => t.type)).toEqual(before);
  });
});

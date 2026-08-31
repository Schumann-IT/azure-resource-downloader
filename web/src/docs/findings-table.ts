// The Findings table in a tenant's `docs/summary.md` is a contract from the
// CLI's generation template: columns Severity | Finding | Affected | Documents,
// with Severity drawn from a closed set and the rows already sorted by it. The
// browser needs it addressable for two reasons the shared wide-table rule
// cannot serve: its Finding column is prose and must wrap, and the severity
// reads better as an icon than as a repeated word.
//
// Detected by its header cell rather than by the `### Findings` heading above
// it: the heading is the same contract, but keying on the columns means a table
// keeps its treatment wherever it is moved, and no other table in the corpus
// leads with a Severity column.
//
// Kept pure and Nest-free (like `link-rewrite.ts`): it only rewrites a
// markdown-it token stream, so it is unit-testable without a module.

export const SEVERITIES = ['critical', 'high', 'medium'] as const;

export type Severity = (typeof SEVERITIES)[number];

export const FINDINGS_CLASS = 'findings';

const SEVERITY_HEADER = 'severity';
const KNOWN: ReadonlySet<string> = new Set<string>(SEVERITIES);

// Normalises a Severity cell to the closed set, tolerating the emphasis or code
// span a generator might wrap it in. Anything else returns null and is left
// alone: an unexpected value must render as plain text, never as a silently
// wrong icon.
export function severityOf(value: string): Severity | null {
  const normalised = value.trim().replace(/^[*_`]+|[*_`]+$/g, '').toLowerCase();
  return KNOWN.has(normalised) ? (normalised as Severity) : null;
}

// Tags every findings table in `tokens` with `.findings`, and each of its body
// rows (and that row's severity cell) with `data-severity`. Mutates in place,
// which is how markdown-it core rules work.
export function applyFindingsTable(tokens: any[]): void {
  for (let i = 0; i < tokens.length; i++) {
    if (tokens[i].type !== 'table_open') continue;

    const close = matchingClose(tokens, i);
    if (close < 0) return;

    if (hasSeverityHeader(tokens, i, close)) {
      tokens[i].attrJoin('class', FINDINGS_CLASS);
      annotateRows(tokens, i, close);
    }

    // Tables do not nest in this corpus, but skipping the body keeps the scan
    // linear and stops an inner table being matched twice.
    i = close;
  }
}

function matchingClose(tokens: any[], open: number): number {
  let depth = 0;
  for (let i = open; i < tokens.length; i++) {
    if (tokens[i].type === 'table_open') depth++;
    else if (tokens[i].type === 'table_close' && --depth === 0) return i;
  }
  return -1;
}

function hasSeverityHeader(tokens: any[], open: number, close: number): boolean {
  for (let i = open + 1; i < close; i++) {
    if (tokens[i].type === 'tr_close') return false;
    if (tokens[i].type !== 'th_open') continue;
    const text = cellText(tokens, i, close)
      .replace(/^[*_`]+|[*_`]+$/g, '')
      .toLowerCase();
    return text === SEVERITY_HEADER;
  }
  return false;
}

function annotateRows(tokens: any[], open: number, close: number): void {
  let inBody = false;

  for (let i = open + 1; i < close; i++) {
    const type = tokens[i].type;
    if (type === 'tbody_open') inBody = true;
    else if (type === 'tbody_close') inBody = false;
    else if (inBody && type === 'tr_open') {
      const cell = firstCell(tokens, i, close);
      if (cell < 0) continue;
      const severity = severityOf(cellText(tokens, cell, close));
      if (!severity) continue;
      tokens[i].attrSet('data-severity', severity);
      tokens[cell].attrSet('data-severity', severity);
      // Sighted users lose the word to the icon; the tooltip gives it back.
      // Assistive technology still reads the cell's own text.
      tokens[cell].attrSet('title', severity);
    }
  }
}

function firstCell(tokens: any[], row: number, close: number): number {
  for (let i = row + 1; i < close; i++) {
    if (tokens[i].type === 'td_open') return i;
    if (tokens[i].type === 'tr_close') return -1;
  }
  return -1;
}

// The raw source text of the cell whose opening token is at `cellOpen`. Still
// carries any inline markup, which is why `severityOf` strips it.
function cellText(tokens: any[], cellOpen: number, close: number): string {
  for (let i = cellOpen + 1; i < close; i++) {
    if (tokens[i].type === 'inline') return String(tokens[i].content || '').trim();
    if (tokens[i].type === 'th_close' || tokens[i].type === 'td_close') break;
  }
  return '';
}

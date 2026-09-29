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
// Two severity vocabularies exist by design, and each table keeps its own
// closed set: the summary's Findings table uses critical | high | medium, the
// drift index's uses high | medium | low | info (info being the tool-fed
// inventory rows). A value outside the table's own set stays plain text, so a
// summary `low` or a drift `critical` is never drawn as a wrong icon.
//
// Kept pure and Nest-free (like `link-rewrite.ts`): it only rewrites a
// markdown-it token stream, so it is unit-testable without a module.

export const SEVERITIES = ['critical', 'high', 'medium'] as const;

export type Severity = (typeof SEVERITIES)[number];

// The drift analysis' vocabulary, applied to a table that also has a Verdict
// column (see DRIFT_FINDINGS_CLASS).
export const DRIFT_SEVERITIES = ['high', 'medium', 'low', 'info'] as const;

export type DriftSeverity = (typeof DRIFT_SEVERITIES)[number];

export const FINDINGS_CLASS = 'findings';

// The drift analysis report's Findings table leads with Severity too, but adds
// a Verdict column whose values are the CLI's closed verdict set. Its other
// columns (Resource, Judgment) have no fixed order the summary's widths could
// assume, so each cell is tagged with its column name instead.
export const VERDICTS = ['added', 'changed', 'renamed', 'removed'] as const;

export type Verdict = (typeof VERDICTS)[number];

export const DRIFT_FINDINGS_CLASS = 'findings-drift';

const SEVERITY_HEADER = 'severity';
const VERDICT_HEADER = 'verdict';
const KNOWN: ReadonlySet<string> = new Set<string>(SEVERITIES);
const KNOWN_DRIFT: ReadonlySet<string> = new Set<string>(DRIFT_SEVERITIES);
const KNOWN_VERDICTS: ReadonlySet<string> = new Set<string>(VERDICTS);

function normalise(value: string): string {
  return value.trim().replace(/^[*_`]+|[*_`]+$/g, '').toLowerCase();
}

// Normalises a Severity cell to a closed set (the summary's by default, the
// drift set when given), tolerating the emphasis or code span a generator might
// wrap it in. Anything else returns null and is left alone: an unexpected value
// must render as plain text, never as a silently wrong icon.
export function severityOf(value: string): Severity | null;
export function severityOf(value: string, set: ReadonlySet<string>): Severity | DriftSeverity | null;
export function severityOf(
  value: string,
  set: ReadonlySet<string> = KNOWN,
): string | null {
  const normalised = normalise(value);
  return set.has(normalised) ? normalised : null;
}

// Same contract as `severityOf`, for the drift report's Verdict cells.
export function verdictOf(value: string): Verdict | null {
  const normalised = normalise(value);
  return KNOWN_VERDICTS.has(normalised) ? (normalised as Verdict) : null;
}

// Tags every findings table in `tokens` with `.findings`, and each of its body
// rows (and that row's severity cell) with `data-severity` from the set that
// belongs to the table kind: the drift set when a Verdict column is present,
// the summary's otherwise. Mutates in place, which is how markdown-it core rules
// work.
export function applyFindingsTable(tokens: any[]): void {
  for (let i = 0; i < tokens.length; i++) {
    if (tokens[i].type !== 'table_open') continue;

    const close = matchingClose(tokens, i);
    if (close < 0) return;

    const headers = headerCells(tokens, i, close);
    if (headers[0] === SEVERITY_HEADER) {
      const drift = headers.includes(VERDICT_HEADER);
      tokens[i].attrJoin('class', FINDINGS_CLASS);
      annotateRows(tokens, i, close, drift ? KNOWN_DRIFT : KNOWN);
      if (drift) {
        tokens[i].attrJoin('class', DRIFT_FINDINGS_CLASS);
        annotateColumns(tokens, i, close, headers);
      }
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

// The normalised text of the header row's cells, in column order.
function headerCells(tokens: any[], open: number, close: number): string[] {
  const headers: string[] = [];
  for (let i = open + 1; i < close; i++) {
    if (tokens[i].type === 'tr_close') break;
    if (tokens[i].type === 'th_open') headers.push(normalise(cellText(tokens, i, close)));
  }
  return headers;
}

// Tags every cell of a drift findings table with `data-column`, and each
// recognised Verdict cell with `data-verdict` and a tooltip, the way
// `annotateRows` treats severity.
function annotateColumns(tokens: any[], open: number, close: number, headers: string[]): void {
  let column = 0;
  for (let i = open + 1; i < close; i++) {
    const type = tokens[i].type;
    if (type === 'tr_open') {
      column = 0;
      continue;
    }
    if (type !== 'th_open' && type !== 'td_open') continue;
    const name = headers[column++];
    if (!name) continue;
    tokens[i].attrSet('data-column', name);
    if (type !== 'td_open' || name !== VERDICT_HEADER) continue;
    const verdict = verdictOf(cellText(tokens, i, close));
    if (!verdict) continue;
    tokens[i].attrSet('data-verdict', verdict);
    tokens[i].attrSet('title', verdict);
  }
}

function annotateRows(
  tokens: any[],
  open: number,
  close: number,
  set: ReadonlySet<string>,
): void {
  let inBody = false;

  for (let i = open + 1; i < close; i++) {
    const type = tokens[i].type;
    if (type === 'tbody_open') inBody = true;
    else if (type === 'tbody_close') inBody = false;
    else if (inBody && type === 'tr_open') {
      const cell = firstCell(tokens, i, close);
      if (cell < 0) continue;
      const severity = severityOf(cellText(tokens, cell, close), set);
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

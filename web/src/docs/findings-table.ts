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
// The drift index's findings table also gains a Changed by column when the
// caller hands in the audit's cells (`applyChangedBy`). The join key is the
// Resource cell's link to the resource's drift document; nothing is inferred
// from the cell's prose.
//
// Kept pure and Nest-free (like `link-rewrite.ts`): it only rewrites a
// markdown-it token stream, so it is unit-testable without a module.

import * as path from 'path';
import { driftKey } from './drift-observation';

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

export const CHANGED_BY_COLUMN = 'changed-by';
const CHANGED_BY_LABEL = 'Changed by';
const RESOURCE_HEADER = 'resource';

// One cell of the Changed by column: its text and how it is toned.
export interface ChangedByCellLike {
  text: string;
  tone: string;
}

type MakeToken = (type: string, tag: string, nesting: number) => any;

// Appends the Changed by column to every drift findings table (tagged by
// `applyFindingsTable`) that has a Resource column. `cells` is keyed by the
// extensionless `<type>/<name>` path of a finding of the observation. A row
// whose Resource cell links to no such finding gets an empty cell. The text is
// a `text` child of the cell's `inline` token, so markdown-it escapes it.
// Mutates `tokens` in place.
export function applyChangedBy(
  tokens: any[],
  cells: ReadonlyMap<string, ChangedByCellLike>,
  makeToken: MakeToken,
): void {
  const out: any[] = [];
  const row: RowState = { resourceColumn: -1, section: '', column: 0, key: '' };

  for (let i = 0; i < tokens.length; i++) {
    const token = tokens[i];
    if (token.type === 'table_open') row.resourceColumn = resourceColumnOf(tokens, i);
    else if (token.type === 'table_close') row.resourceColumn = -1;
    else if (row.resourceColumn >= 0) out.push(...trackRow(row, tokens, i, cells, makeToken));
    out.push(token);
  }

  tokens.splice(0, tokens.length, ...out);
}

interface RowState {
  resourceColumn: number;
  section: string;
  column: number;
  key: string;
}

// Follows the header/body section and the cell position inside a table with a
// Resource column; returns the tokens to insert before token `i` (the Changed
// by cell, at the end of a row).
function trackRow(
  row: RowState,
  tokens: any[],
  i: number,
  cells: ReadonlyMap<string, ChangedByCellLike>,
  makeToken: MakeToken,
): any[] {
  const type = tokens[i].type;
  if (type === 'thead_open') row.section = 'head';
  else if (type === 'tbody_open') row.section = 'body';
  else if (type === 'tr_open') {
    row.column = 0;
    row.key = '';
  } else if (type === 'th_open' || type === 'td_open') {
    if (row.column === row.resourceColumn && type === 'td_open') row.key = rowKey(tokens, i);
    row.column++;
  } else if (type === 'tr_close') return rowEndTokens(makeToken, row.section, cells, row.key);
  return [];
}

// The index of the Resource column of a drift findings table opened at
// `open`, or -1 when the table is not one or has no such column.
function resourceColumnOf(tokens: any[], open: number): number {
  const classes = String(tokens[open].attrGet('class') || '').split(/\s+/);
  if (!classes.includes(DRIFT_FINDINGS_CLASS)) return -1;
  const close = matchingClose(tokens, open);
  return close < 0 ? -1 : headerCells(tokens, open, close).indexOf(RESOURCE_HEADER);
}

// The Changed by cell that ends a header or body row.
function rowEndTokens(
  makeToken: MakeToken,
  section: string,
  cells: ReadonlyMap<string, ChangedByCellLike>,
  key: string,
): any[] {
  if (section === 'head') return cellTokens(makeToken, 'th', CHANGED_BY_LABEL, null);
  if (section === 'body') return cellTokens(makeToken, 'td', '', cells.get(key) ?? null);
  return [];
}

function cellTokens(
  makeToken: MakeToken,
  tag: 'th' | 'td',
  headerText: string,
  cell: ChangedByCellLike | null,
): any[] {
  const text = cell ? cell.text : headerText;
  const open = makeToken(`${tag}_open`, tag, 1);
  open.attrSet('data-column', CHANGED_BY_COLUMN);
  if (cell && text) open.attrSet('data-attribution', cell.tone);
  const inline = makeToken('inline', '', 0);
  inline.content = text;
  inline.children = [];
  if (text) {
    const child = makeToken('text', '', 0);
    child.content = text;
    inline.children.push(child);
  }
  return [open, inline, makeToken(`${tag}_close`, tag, -1)];
}

// The finding key a body row's Resource cell links to, or '' when it links to
// no drift document of the tree (no link, an absolute or foreign link, a path
// leaving the drift root). Runs before the link renderer rewrites the href.
function rowKey(tokens: any[], cellOpen: number): string {
  for (let i = cellOpen + 1; i < tokens.length; i++) {
    const token = tokens[i];
    if (token.type === 'td_close') break;
    if (token.type !== 'inline') continue;
    for (const child of token.children || []) {
      if (child.type !== 'link_open') continue;
      return keyOfHref(String(child.attrGet('href') || ''));
    }
  }
  return '';
}

function keyOfHref(href: string): string {
  const hash = href.indexOf('#');
  const target = hash >= 0 ? href.slice(0, hash) : href;
  if (!target || target.startsWith('/') || /^[a-z][a-z0-9+.-]*:/i.test(target)) return '';
  if (!target.toLowerCase().endsWith('.md')) return '';
  const normalised = path.posix.normalize(target);
  if (normalised === '..' || normalised.startsWith('../')) return '';
  const key = normalised.slice(0, -'.md'.length);
  return driftKey(`${key}.yaml`) === key ? key : '';
}

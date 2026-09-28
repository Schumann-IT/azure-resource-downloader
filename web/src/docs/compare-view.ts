import { createHash } from 'crypto';
import { NormalisedResource, NormaliseReport, normaliseResource } from './compare-normalise';
import { MetadataEntry, ResourcesMetadata } from './resources-metadata';
import { typeLabel } from './tenant-index';
import { DIFF_CONTEXT, diffYaml, MAX_DIFF_BYTES, YamlDiff } from './yaml-diff';

// Route prefix of the tenant compare. A root-level *representation* prefix: it
// cannot collide with a tenant because discovery skips `_`-prefixed folders,
// and it never appears in the breadcrumb.
export const COMPARE_PREFIX = '_compare';

// One side of the comparison as the listing needs it.
export interface CompareSide {
  id: string;
  metadata: ResourcesMetadata;
}

export interface ListingCell {
  key: string;
  name: string;
  href: string;
}

// One row of the two-column listing: a paired key has both cells (each
// linking to the pair diff), a single-side key one cell and an empty one
// opposite. `key` is what the sidebar marks the viewed pair by.
export interface ListingRow {
  key: string;
  left: ListingCell | null;
  right: ListingCell | null;
}

export interface ListingGroup {
  type: string;
  excluded: boolean;
  both: number;
  onlyA: number;
  onlyB: number;
  rows: ListingRow[];
}

export interface CompareListing {
  totals: { both: number; onlyA: number; onlyB: number };
  groups: ListingGroup[];
}

export function selectHref(a: string): string {
  return `/${COMPARE_PREFIX}?a=${encodeURIComponent(a)}`;
}

export function listingHref(a: string, b: string): string {
  return `${selectHref(a)}&b=${encodeURIComponent(b)}`;
}

export function pairHref(a: string, b: string, key: string): string {
  return `/${COMPARE_PREFIX}/${encodeKey(key)}?a=${encodeURIComponent(a)}&b=${encodeURIComponent(b)}`;
}

// The three-way split of two exports' resources by key, from their
// `resources/metadata.yaml` alone — no resource file is read. Only resources
// still present in the tenant are listed. Types either index counts under
// `counts.excluded` go last and are marked, so policies are read first.
export function compareListing(
  a: CompareSide,
  b: CompareSide,
  excludedTypes: Set<string>,
  resourcePrefix: string,
): CompareListing {
  const resourceHref = (tenant: string, key: string) =>
    `/${tenant}/${resourcePrefix}/${encodeKey(key)}`;
  const left = present(a.metadata);
  const right = present(b.metadata);
  const onlyA: MetadataEntry[] = [];
  const both: Array<[MetadataEntry, MetadataEntry]> = [];
  for (const [key, entry] of left) {
    const other = right.get(key);
    if (other) both.push([entry, other]);
    else onlyA.push(entry);
  }
  const onlyB = [...right.values()].filter((e) => !left.has(e.key));

  const types = new Set([
    ...onlyA.map((e) => e.type),
    ...onlyB.map((e) => e.type),
    ...both.map(([e]) => e.type),
  ]);
  const groups = [...types]
    .map((type): ListingGroup => {
      const paired = both.filter(([e]) => e.type === type);
      const aOnly = onlyA.filter((e) => e.type === type);
      const bOnly = onlyB.filter((e) => e.type === type);
      return {
        type,
        excluded: excludedTypes.has(type),
        both: paired.length,
        onlyA: aOnly.length,
        onlyB: bOnly.length,
        rows: interleaveRows(
          paired.map(([ea, eb]) => {
            const href = pairHref(a.id, b.id, ea.key);
            return [
              { key: ea.key, name: ea.displayName, href },
              { key: eb.key, name: eb.displayName, href },
            ];
          }),
          aOnly.map((e) => ({ key: e.key, name: e.displayName, href: resourceHref(a.id, e.key) })),
          bOnly.map((e) => ({ key: e.key, name: e.displayName, href: resourceHref(b.id, e.key) })),
        ),
      };
    })
    .sort(byExcludedThenType);

  return {
    totals: { both: both.length, onlyA: onlyA.length, onlyB: onlyB.length },
    groups,
  };
}

// One type's rows in reading order for the two-column listing: the pairs
// first, then what only the left side has, then what only the right side has,
// each alphabetical by the name shown on its side. Pure.
export function interleaveRows(
  paired: Array<[ListingCell, ListingCell]>,
  onlyA: ListingCell[],
  onlyB: ListingCell[],
): ListingRow[] {
  const byName = (x: ListingCell, y: ListingCell) => x.name.localeCompare(y.name);
  return [
    ...[...paired]
      .sort(([x], [y]) => byName(x, y))
      .map(([l, r]) => ({ key: l.key, left: l, right: r })),
    ...[...onlyA].sort(byName).map((c) => ({ key: c.key, left: c, right: null })),
    ...[...onlyB].sort(byName).map((c) => ({ key: c.key, left: null, right: c })),
  ];
}

// The keys both exports hold, in listing order: the only keys whose files the
// pane's status reads.
export function pairedKeys(listing: CompareListing): string[] {
  return listing.groups.flatMap((g) =>
    g.rows.filter((r) => r.left !== null && r.right !== null).map((r) => r.key),
  );
}

// One file as its tenant's normalisation sees it: the one step both the pair
// diff and the pane's status build on, so the two cannot apply different rules.
// Undefined when the YAML does not parse to an object.
export function normaliseFile(
  raw: string,
  metadata: ResourcesMetadata,
): NormalisedResource | undefined {
  return normaliseResource(raw, metadata.lookup, metadata.nameCounts);
}

// What the pane keeps per resource file, instead of its text: hashes of the
// file as exported, normalised, and normalised without `assignments`. The two
// normalised hashes are null when the file does not parse or is over the size
// cap, with the reason in `reason`. A few dozen bytes per file, so the cache
// holding them stays small however many pairs are compared.
export interface FileDigest {
  size: number;
  raw: string;
  normalised: string | null;
  withoutAssignments: string | null;
  reason: string;
}

export function fileDigest(raw: string, metadata: ResourcesMetadata): FileDigest {
  const size = Buffer.byteLength(raw);
  const digest = { size, raw: sha256(raw), normalised: null, withoutAssignments: null };
  if (size > MAX_DIFF_BYTES) return { ...digest, reason: 'too large to normalise' };
  const n = normaliseFile(raw, metadata);
  if (!n) return { ...digest, reason: 'does not parse' };
  return {
    ...digest,
    normalised: sha256(n.text),
    withoutAssignments: sha256(n.textWithoutAssignments),
    reason: '',
  };
}

export type PairState = 'identical' | 'audience' | 'different' | 'unknown';

export interface PairStatus {
  state: PairState;
  reason: string;
}

// A pair's status from its two digests — never from the texts and never through
// `pairComparison`, so no diff is computed per row. Mirrors what the pair diff
// page concludes: files equal as exported are identical; above the combined
// size cap nothing is normalised, so unequal files cannot be judged; otherwise
// the normalised hashes decide, then the ones without `assignments`. A null
// digest is a file that could not be read. Pure.
export function pairStatus(left: FileDigest | null, right: FileDigest | null): PairStatus {
  if (!left || !right) return { state: 'unknown', reason: 'a file could not be read' };
  if (left.raw === right.raw) return { state: 'identical', reason: '' };
  if (left.size + right.size > MAX_DIFF_BYTES) {
    return { state: 'unknown', reason: 'too large to normalise' };
  }
  if (left.normalised === null || right.normalised === null) {
    return { state: 'unknown', reason: left.reason || right.reason };
  }
  if (left.normalised === right.normalised) return { state: 'identical', reason: '' };
  if (left.withoutAssignments === right.withoutAssignments) {
    return { state: 'audience', reason: '' };
  }
  return { state: 'different', reason: '' };
}

export type RowState = PairState | 'onlyLeft' | 'onlyRight';

export interface PaneCell {
  name: string;
  size: string;
}

// One row of the comparison pane: one link, to the pair diff (carrying the
// view's query and its own `#row-<n>`) or, one-sided, to that tenant's YAML
// view. `n` numbers the full list, so a row keeps its anchor whether identical
// pairs are shown or not.
export interface PaneRow {
  n: number;
  key: string;
  href: string;
  selected: boolean;
  left: PaneCell | null;
  right: PaneCell | null;
  state: RowState;
  is: Record<RowState, boolean>;
  marker: string;
  label: string;
}

export interface PaneGroup {
  type: string;
  label: string;
  excluded: boolean;
  rows: PaneRow[];
}

export interface PaneTotals {
  different: number;
  onlyA: number;
  onlyB: number;
  identical: number;
  unknown: number;
}

export interface ComparePane {
  groups: PaneGroup[];
  totals: PaneTotals;
  same: boolean;
  sameHref: string;
  swapHref: string;
  listingHref: string;
  firstDifferenceHref: string | null;
  // Why the pane shows no row, or '' when it shows some.
  emptyNote: string;
}

export interface PaneOptions {
  a: string;
  b: string;
  // The pair being viewed, or '' on the listing page.
  selectedKey: string;
  // Whether identical pairs are shown (`&same`).
  same: boolean;
}

const MARKERS: Record<RowState, string> = {
  identical: '=',
  audience: '≠',
  different: '≠',
  unknown: '?',
  onlyLeft: '→',
  onlyRight: '←',
};

// The comparison pane from the listing and the two sides' digests. By default
// it shows what needs attention — different, one-sided and unknown rows — and
// always the pair being viewed, so the row that is marked and scrolled to exists
// on every pair page; `same` shows the identical pairs too. A type left without
// rows is dropped. Every row link keeps the view's query (`same`) and drops the
// key. Pure.
export function comparePane(
  listing: CompareListing,
  leftDigests: Map<string, FileDigest | null>,
  rightDigests: Map<string, FileDigest | null>,
  options: PaneOptions,
): ComparePane {
  const { a, b, selectedKey, same } = options;
  const query = same ? '&same' : '';
  const totals: PaneTotals = { different: 0, onlyA: 0, onlyB: 0, identical: 0, unknown: 0 };
  let n = 0;
  let firstDifferenceHref: string | null = null;

  const groups = listing.groups.map((group): PaneGroup => {
    const rows = group.rows.map((row): PaneRow => {
      n++;
      const ld = leftDigests.get(row.key) ?? null;
      const rd = rightDigests.get(row.key) ?? null;
      const status = rowStatus(row, ld, rd);
      const pairLink = `${pairHref(a, b, row.key)}${query}#row-${n}`;
      const href = row.left && row.right ? pairLink : (row.left ?? row.right)?.href ?? '';
      countRow(totals, status.state);
      if (!firstDifferenceHref && (status.state === 'different' || status.state === 'audience')) {
        firstDifferenceHref = pairLink;
      }
      return {
        n,
        key: row.key,
        href,
        selected: selectedKey !== '' && row.key === selectedKey,
        left: row.left ? { name: row.left.name, size: formatSize(ld?.size) } : null,
        right: row.right ? { name: row.right.name, size: formatSize(rd?.size) } : null,
        state: status.state,
        is: stateFlags(status.state),
        marker: MARKERS[status.state],
        label: rowLabel(status, a, b),
      };
    });
    return {
      type: group.type,
      label: typeLabel(group.type),
      excluded: group.excluded,
      rows: rows.filter((r) => same || r.selected || r.state !== 'identical'),
    };
  });

  const self = selectedKey ? pairHref(a, b, selectedKey) : listingHref(a, b);
  const swapped = { a: b, b: a };
  const swap = selectedKey
    ? pairHref(swapped.a, swapped.b, selectedKey)
    : listingHref(swapped.a, swapped.b);
  const visible = groups.filter((g) => g.rows.length > 0);
  return {
    groups: visible,
    totals,
    same,
    sameHref: same ? self : `${self}&same`,
    swapHref: `${swap}${query}`,
    listingHref: `${listingHref(a, b)}${query}`,
    firstDifferenceHref,
    emptyNote: visible.length > 0 ? '' : emptyNote(totals),
  };
}

function emptyNote(totals: PaneTotals): string {
  return totals.identical > 0
    ? 'Nothing needs attention: every pair is identical and nothing is one-sided.'
    : 'Neither export lists a resource still present in its tenant.';
}

function rowStatus(
  row: ListingRow,
  left: FileDigest | null,
  right: FileDigest | null,
): { state: RowState; reason: string } {
  if (!row.right) return { state: 'onlyLeft', reason: '' };
  if (!row.left) return { state: 'onlyRight', reason: '' };
  return pairStatus(left, right);
}

function countRow(totals: PaneTotals, state: RowState): void {
  if (state === 'different' || state === 'audience') totals.different++;
  else if (state === 'onlyLeft') totals.onlyA++;
  else if (state === 'onlyRight') totals.onlyB++;
  else totals[state]++;
}

function stateFlags(state: RowState): Record<RowState, boolean> {
  return {
    identical: state === 'identical',
    audience: state === 'audience',
    different: state === 'different',
    unknown: state === 'unknown',
    onlyLeft: state === 'onlyLeft',
    onlyRight: state === 'onlyRight',
  };
}

// The marker's visually hidden label, so a status is never meaning by glyph and
// colour alone; an unknown status carries its reason here, since a `title` is
// not read out.
function rowLabel(status: { state: RowState; reason: string }, a: string, b: string): string {
  switch (status.state) {
    case 'identical':
      return 'identical';
    case 'audience':
      return 'differs only in audience';
    case 'different':
      return 'different';
    case 'onlyLeft':
      return `only in ${a}`;
    case 'onlyRight':
      return `only in ${b}`;
    default:
      return `could not compare: ${status.reason}`;
  }
}

// A file size as the pane shows it; empty when the file was not read (a
// one-sided row, or a file that could not be).
export function formatSize(bytes: number | undefined): string {
  if (bytes === undefined) return '';
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

export interface PairComparison {
  diff: YamlDiff;
  // False for the `?raw` view, above the size cap, or when either file does
  // not parse — the diff is then of the files as exported.
  normalised: boolean;
  identical: boolean;
  audienceOnly: boolean;
  report: ReportView | null;
}

export interface ReportView {
  droppedKeys: string;
  left: NormaliseReport;
  right: NormaliseReport;
}

// The comparison of one paired resource: both files normalised with their own
// tenant's lookup and diffed, plus the "differs only in audience" check — the
// two documents compared again without `assignments`. A text comparison, not a
// structural diff. `context` is passed to `diffYaml` as is: the compare page
// asks for the whole file, and the cap falling back to hunks is `diffYaml`'s.
export function pairComparison(
  left: string,
  right: string,
  a: ResourcesMetadata,
  b: ResourcesMetadata,
  raw: boolean,
  context = DIFF_CONTEXT,
): PairComparison {
  const unnormalised = (): PairComparison => ({
    diff: diffYaml(left, right, context),
    normalised: false,
    identical: left === right,
    audienceOnly: false,
    report: null,
  });
  if (raw || Buffer.byteLength(left) + Buffer.byteLength(right) > MAX_DIFF_BYTES) {
    return unnormalised();
  }
  const na = normaliseFile(left, a);
  const nb = normaliseFile(right, b);
  if (!na || !nb) return unnormalised();

  const identical = na.text === nb.text;
  const audienceOnly = !identical && na.textWithoutAssignments === nb.textWithoutAssignments;
  const keys = new Set([...na.report.droppedKeys, ...nb.report.droppedKeys]);
  return {
    diff: diffYaml(na.text, nb.text, context),
    normalised: true,
    identical,
    audienceOnly,
    report: {
      droppedKeys: [...keys].sort().join(', '),
      left: na.report,
      right: nb.report,
    },
  };
}

function present(metadata: ResourcesMetadata): Map<string, MetadataEntry> {
  return new Map(
    metadata.entries.filter((e) => e.presentInTenant).map((e) => [e.key, e]),
  );
}

// Documented types first, then the excluded ones; alphabetical within each.
function byExcludedThenType(x: ListingGroup, y: ListingGroup): number {
  if (x.excluded !== y.excluded) return x.excluded ? 1 : -1;
  return x.type.localeCompare(y.type);
}

function encodeKey(key: string): string {
  return key.split('/').map(encodeURIComponent).join('/');
}

function sha256(text: string): string {
  return createHash('sha256').update(text).digest('hex');
}

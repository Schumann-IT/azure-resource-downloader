import { NormaliseReport, normaliseResource } from './compare-normalise';
import { MetadataEntry, ResourcesMetadata } from './resources-metadata';
import { diffYaml, MAX_DIFF_BYTES, YamlDiff } from './yaml-diff';

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
  name: string;
  href: string;
}

// One row of the two-column listing: a paired key has both cells (each
// linking to the pair diff), a single-side key one cell and an empty one
// opposite.
export interface ListingRow {
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
              { name: ea.displayName, href },
              { name: eb.displayName, href },
            ];
          }),
          aOnly.map((e) => ({ name: e.displayName, href: resourceHref(a.id, e.key) })),
          bOnly.map((e) => ({ name: e.displayName, href: resourceHref(b.id, e.key) })),
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
      .map(([l, r]) => ({ left: l, right: r })),
    ...[...onlyA].sort(byName).map((c) => ({ left: c, right: null })),
    ...[...onlyB].sort(byName).map((c) => ({ left: null, right: c })),
  ];
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
// structural diff.
export function pairComparison(
  left: string,
  right: string,
  a: ResourcesMetadata,
  b: ResourcesMetadata,
  raw: boolean,
): PairComparison {
  const unnormalised = (): PairComparison => ({
    diff: diffYaml(left, right),
    normalised: false,
    identical: left === right,
    audienceOnly: false,
    report: null,
  });
  if (raw || Buffer.byteLength(left) + Buffer.byteLength(right) > MAX_DIFF_BYTES) {
    return unnormalised();
  }
  const na = normaliseResource(left, a.lookup, a.nameCounts);
  const nb = normaliseResource(right, b.lookup, b.nameCounts);
  if (!na || !nb) return unnormalised();

  const identical = na.text === nb.text;
  let audienceOnly = false;
  if (!identical) {
    const opts = { withoutAssignments: true };
    const wa = normaliseResource(left, a.lookup, a.nameCounts, opts);
    const wb = normaliseResource(right, b.lookup, b.nameCounts, opts);
    audienceOnly = !!wa && !!wb && wa.text === wb.text;
  }
  const keys = new Set([...na.report.droppedKeys, ...nb.report.droppedKeys]);
  return {
    diff: diffYaml(na.text, nb.text),
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

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

export interface ListingRow {
  name: string;
  // The other tenant's display name for a paired key, when it differs.
  otherName: string | null;
  href: string;
}

export interface ListingGroup {
  type: string;
  count: number;
  excluded: boolean;
  rows: ListingRow[];
}

export interface ListingSection {
  total: number;
  groups: ListingGroup[];
}

export interface CompareListing {
  onlyA: ListingSection;
  onlyB: ListingSection;
  both: ListingSection;
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

  return {
    onlyA: section(
      onlyA.map((e) => ({ entry: e, row: { name: e.displayName, otherName: null, href: resourceHref(a.id, e.key) } })),
      excludedTypes,
    ),
    onlyB: section(
      onlyB.map((e) => ({ entry: e, row: { name: e.displayName, otherName: null, href: resourceHref(b.id, e.key) } })),
      excludedTypes,
    ),
    both: section(
      both.map(([ea, eb]) => ({
        entry: ea,
        row: {
          name: ea.displayName,
          otherName: eb.displayName === ea.displayName ? null : eb.displayName,
          href: pairHref(a.id, b.id, ea.key),
        },
      })),
      excludedTypes,
    ),
  };
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

function section(
  items: Array<{ entry: MetadataEntry; row: ListingRow }>,
  excludedTypes: Set<string>,
): ListingSection {
  const byType = new Map<string, ListingRow[]>();
  for (const { entry, row } of items) {
    const rows = byType.get(entry.type) ?? [];
    rows.push(row);
    byType.set(entry.type, rows);
  }
  const groups = [...byType.entries()]
    .map(([type, rows]) => ({
      type,
      count: rows.length,
      excluded: excludedTypes.has(type),
      rows: rows.sort((x, y) => x.name.localeCompare(y.name)),
    }))
    .sort(byExcludedThenType);
  return { total: items.length, groups };
}

// Documented types first, then the excluded ones; alphabetical within each.
function byExcludedThenType(x: ListingGroup, y: ListingGroup): number {
  if (x.excluded !== y.excluded) return x.excluded ? 1 : -1;
  return x.type.localeCompare(y.type);
}

function encodeKey(key: string): string {
  return key.split('/').map(encodeURIComponent).join('/');
}

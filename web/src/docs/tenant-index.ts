import * as yaml from 'js-yaml';

// Shape of `docs/index.yaml`, the navigation index emitted by
// `azure-rd docs generate-index`. It is the tenant marker and the only
// navigation source: the CLI no longer generates an `index.md`.
export interface TenantIndex {
  version: number;
  tenant: string;
  generatedAt: string | null;
  complete: boolean;
  incompleteReason: string | null;
  counts: {
    documented: number;
    pending: number;
    excluded: Array<{ type: string; count: number }>;
  };
  programmes: IndexProgramme[];
  facets: IndexFacet[];
  vocabularies: { platform: string[]; function: string[] };
  resources: IndexResource[];
}

// One filter axis from the index header. `id` is both the stable axis id and its
// query-parameter name; `values` are in the CLI's display order, so ordering is
// read from the data and cannot drift from a copy kept here. Zero-count values
// are kept: "this value matched nothing in this tenant" is information only the
// registry carries.
export interface IndexFacet {
  id: string;
  label: string;
  values: IndexFacetValue[];
}

// One value of an axis. A resource's membership carries the id only, so the
// label is always resolved from here.
export interface IndexFacetValue {
  id: string;
  label: string;
  count: number;
}

// A programme the CLI's taxonomy defines, in the index header. The registry is
// the only way to know a programme exists at all: one with `count: 0` matched
// nothing in this tenant, which is information, so those entries are kept.
export interface IndexProgramme {
  id: string;
  label: string;
  count: number;
}

// A resource's programme membership: a stable `id` for the URL and a separate
// display `label`, so renaming a programme cannot break a bookmarked filter.
export interface IndexGroup {
  id: string;
  label: string;
}

export interface IndexResource {
  type: string;
  doc: string;
  displayName: string;
  summary: string;
  documented: boolean;
  scope: string;
  platformGroup: string;
  functionGroup: string;
  odataType: string;
  platforms: string;
  groups: IndexGroup[];
  facets: Record<string, string[]>;
  assignments: IndexAssignments | null;
}

export interface IndexAssignments {
  groups: number;
  allUsers: boolean;
  allDevices: boolean;
  targetedBy: number;
}

// A section of the navigation: one resource type, its documents. `active` marks
// the section holding the document being viewed, so the sidebar can render it
// as an open <details> without any client-side JavaScript.
export interface NavSection {
  key: string;
  label: string;
  items: NavItem[];
  active: boolean;
}

export interface NavItem {
  href: string;
  label: string;
  summary: string;
  documented: boolean;
  badges: string[];
  active: boolean;
  // True for an item the selection excludes that is listed anyway because it is
  // the document being viewed. It is what makes the tree longer than the match
  // count, so the view must be able to say so rather than leave the reader to
  // reconcile the two numbers.
  exempt: boolean;
}

// One axis of the filter chooser: the axis label and its chips, including the
// per-axis "All" and "Uncategorised" pseudo-values.
export interface FacetAxisFilter {
  id: string;
  label: string;
  values: FacetChip[];
}

// One chip. `href` is the whole current selection with this value toggled, so a
// click never loses another axis and the selection survives a reload with no
// client-side state.
export interface FacetChip {
  id: string;
  label: string;
  count: number;
  href: string;
  active: boolean;
}

// The active filter: axis id -> selected value ids. OR within an axis, AND
// across axes. Values are de-duplicated and sorted so one selection has exactly
// one URL.
export type FacetSelection = Record<string, string[]>;

// Filter value for resources an axis matched to nothing. Prefixed like the
// `_resource`/`_export` route segments because it is a representation rather
// than data: the CLI's id pattern forbids a leading underscore, so no taxonomy
// id can collide with it.
export const UNCATEGORISED = '_uncategorised';

// Query parameters that belong to a route rather than to a facet axis. An axis
// whose id collides with one is not offered as a filter, so it can never shadow
// the route's own parameter.
const RESERVED_QUERY_PARAMS = new Set(['raw']);

// The axis a version-2 index expresses through `programmes` + per-resource
// `groups`. Naming it is confined to that compatibility shim: everything else
// here is axis-agnostic and works for any axis the CLI declares.
const LEGACY_AXIS_ID = 'programme';

// Parses `docs/index.yaml`. Returns undefined for anything that is not an index
// object, so a malformed file makes the folder *not a tenant* rather than
// crashing discovery.
//
// Any integer version >= 1 is accepted and unknown fields are ignored: the index
// is also the tenant marker, so refusing a newer schema would not degrade the
// page, it would make the whole tenant disappear. The CLI bumped the schema to 2
// when it added `groups`/`programmes`/`vocabularies`, and that bump is additive.
export function parseTenantIndex(raw: string): TenantIndex | undefined {
  let doc: unknown;
  try {
    doc = yaml.load(raw);
  } catch {
    return undefined;
  }
  if (!doc || typeof doc !== 'object' || Array.isArray(doc)) return undefined;

  const src = doc as Record<string, any>;
  const version = num(src.version);
  if (!Number.isInteger(version) || version < 1) return undefined;
  if (typeof src.tenant !== 'string' || !src.tenant) return undefined;

  const counts = (src.counts || {}) as Record<string, any>;
  const excludedRaw = (counts.excluded || {}) as Record<string, any>;
  const excluded = Object.keys(excludedRaw)
    .sort()
    .map((type) => ({ type, count: num(excludedRaw[type]) }));

  const resources = Array.isArray(src.resources)
    ? src.resources
        .filter((r: unknown) => !!r && typeof r === 'object')
        .map((r: Record<string, any>) => toResource(r))
        .filter((r: IndexResource) => r.type !== '' && r.doc !== '')
    : [];

  const vocabularies = (src.vocabularies || {}) as Record<string, any>;
  const programmes = toProgrammes(src.programmes);
  const facets = toFacets(src.facets);

  return {
    version,
    tenant: src.tenant,
    generatedAt: str(src.generatedAt) || null,
    complete: src.complete !== false,
    incompleteReason: str(src.incompleteReason) || null,
    counts: {
      documented: num(counts.documented),
      pending: num(counts.pending),
      excluded,
    },
    programmes,
    // A version-2 index carries the single programme axis only through
    // `programmes`/`groups`; synthesising the axis from them means old and new
    // exports take one code path from here on.
    facets: facets.length > 0 ? facets : legacyFacets(programmes),
    vocabularies: {
      platform: strList(vocabularies.platform),
      function: strList(vocabularies.function),
    },
    resources,
  };
}

// Groups the index into the navigation tree. Grouping is by resource type:
// `platformGroup`/`functionGroup` are optional enrichment the documents do not
// carry yet, so they are surfaced as per-item badges instead of driving the
// tree (see NEXT-ITERATIONS.md).
//
// `activeDoc` is the extensionless document path of the page being viewed
// (`Microsoft.Graph/groups/g1`), or '' on the tenant landing page.
//
// `selection` narrows the tree: OR within an axis, AND across axes, with
// UNCATEGORISED matching the resources an axis matched to nothing. Membership is
// read, never derived: the CLI resolves the taxonomy so the browser and the
// Confluence export classify identically. An index that declares no axis cannot
// be filtered at all — it renders exactly the tree it rendered before the
// taxonomy existed. The document being viewed is always kept, so a filter cannot
// make the page you are on vanish from its own sidebar.
export function buildNavigation(
  index: TenantIndex,
  tenantId: string,
  activeDoc = '',
  selection: FacetSelection = {},
): NavSection[] {
  const sections = new Map<string, NavSection>();
  const active = stripExtension(activeDoc);
  // Only axes this index can actually serve filter anything, so an index with no
  // taxonomy renders its full tree whatever the URL asked for.
  const filter = onlyFilterableAxes(index, selection);

  for (const resource of index.resources) {
    const isActive = active !== '' && stripExtension(resource.doc) === active;
    const matches = matchesSelection(resource, filter);
    if (!isActive && !matches) continue;

    let section = sections.get(resource.type);
    if (!section) {
      section = {
        key: resource.type,
        label: typeLabel(resource.type),
        items: [],
        active: false,
      };
      sections.set(resource.type, section);
    }
    if (isActive) section.active = true;
    section.items.push({
      href: docHref(tenantId, resource.doc, index, filter),
      label: resource.displayName || stripExtension(resource.doc),
      summary: resource.summary,
      documented: resource.documented,
      badges: badgesFor(index, resource),
      active: isActive,
      exempt: !matches,
    });
  }

  const ordered = [...sections.values()].sort((a, b) =>
    a.label.localeCompare(b.label),
  );
  for (const section of ordered) {
    section.items.sort((a, b) => a.label.localeCompare(b.label));
  }
  return ordered;
}

// Reads the active selection out of a request's query parameters. Each axis is
// named by its id and may repeat, so `?programme=a&programme=b&platform=windows`
// is OR within an axis and AND across axes. Values this index cannot serve are
// dropped rather than 404ing or rendering what would look like an empty tenant,
// and the result is de-duplicated and sorted so one selection has one URL.
export function parseFacetSelection(
  index: TenantIndex,
  query: Record<string, unknown>,
): FacetSelection {
  const selection: FacetSelection = {};
  for (const axis of filterableAxes(index)) {
    const known = new Set<string>(axis.values.map((v) => v.id));
    known.add(UNCATEGORISED);
    const chosen = [
      ...new Set(queryValues(query[axis.id]).filter((v) => known.has(v))),
    ].sort();
    if (chosen.length > 0) selection[axis.id] = chosen;
  }
  return selection;
}

// The filter chooser, for the sidebar: one entry per axis the index declares,
// each with "All", the axis's values in display order and the uncategorised
// bucket, which is always offered so a taxonomy that stops matching shows up as a
// full bucket rather than as a thinning tree.
//
// Counts are computed from the resources under the current selection, with *this*
// axis's own selection removed — otherwise picking one value would drive its
// siblings to 0 and the rule below would erase the axis being used. A value that
// reaches 0 because *another* axis is filtering is dropped (it is a dead end),
// while a selected value stays visible even at 0 so the choice can be undone, and
// an unfiltered zero-count value stays because "empty in this tenant" is
// information the registry carries on purpose.
//
// Returns [] when the index declares no usable axis: an export written without a
// taxonomy renders exactly the tree it rendered before.
//
// `basePath` is the current page's own path, so choosing a value keeps you on the
// page you are on.
export function buildFacetFilters(
  index: TenantIndex,
  basePath: string,
  selection: FacetSelection = {},
): FacetAxisFilter[] {
  const filters: FacetAxisFilter[] = [];
  const activeSelection = onlyFilterableAxes(index, selection);
  for (const axis of filterableAxes(index)) {
    const others = { ...activeSelection };
    delete others[axis.id];
    const base = index.resources.filter((r) => matchesSelection(r, others));
    const narrowed = Object.keys(others).length > 0;
    const chosen = activeSelection[axis.id] ?? [];

    const chips: FacetChip[] = [
      {
        id: '',
        label: 'All',
        count: base.length,
        href: selectionHref(basePath, index, others),
        active: chosen.length === 0,
      },
    ];
    const candidates = [
      ...axis.values.map((v) => ({ id: v.id, label: v.label })),
      { id: UNCATEGORISED, label: 'Uncategorised' },
    ];
    for (const value of candidates) {
      const active = chosen.includes(value.id);
      const count = base.filter((r) =>
        matchesAxis(r, axis.id, [value.id]),
      ).length;
      if (count === 0 && narrowed && !active) continue;
      chips.push({
        id: value.id,
        label: value.label,
        count,
        href: selectionHref(
          basePath,
          index,
          toggled(activeSelection, axis.id, value.id),
        ),
        active,
      });
    }
    filters.push({ id: axis.id, label: axis.label, values: chips });
  }
  return filters;
}

// How many *distinct* resources the selection matches. Value counts are not
// additive — a resource can hold several ids on one axis — so a total is always
// counted over the resource list, never summed from the header.
export function countMatching(
  index: TenantIndex,
  selection: FacetSelection = {},
): number {
  const filter = onlyFilterableAxes(index, selection);
  return index.resources.filter((r) => matchesSelection(r, filter)).length;
}

// Whether any axis is filtering. Used to decide whether to offer a reset and a
// "showing N of M" line at all.
export function hasSelection(selection: FacetSelection): boolean {
  return Object.values(selection).some((values) => values.length > 0);
}

// The axes worth offering: those the index declares, that some resource is
// actually a member of (an axis nothing matched renders as nothing rather than as
// a lone "Uncategorised" chip), and whose id does not collide with a route's own
// query parameter.
function filterableAxes(index: TenantIndex): IndexFacet[] {
  return index.facets.filter(
    (axis) =>
      !RESERVED_QUERY_PARAMS.has(axis.id) &&
      index.resources.some((r) => (r.facets[axis.id] ?? []).length > 0),
  );
}

// The selection with every axis this index cannot serve dropped, so a stale or
// hand-written URL narrows nothing instead of emptying the tree.
function onlyFilterableAxes(
  index: TenantIndex,
  selection: FacetSelection,
): FacetSelection {
  const usable = new Set(filterableAxes(index).map((axis) => axis.id));
  const filter: FacetSelection = {};
  for (const [axisId, values] of Object.entries(selection)) {
    if (usable.has(axisId) && values.length > 0) filter[axisId] = values;
  }
  return filter;
}

function matchesAxis(
  resource: IndexResource,
  axisId: string,
  values: string[],
): boolean {
  const membership = resource.facets[axisId] ?? [];
  return values.some((value) =>
    value === UNCATEGORISED
      ? membership.length === 0
      : membership.includes(value),
  );
}

function matchesSelection(
  resource: IndexResource,
  selection: FacetSelection,
): boolean {
  for (const [axisId, values] of Object.entries(selection)) {
    if (values.length === 0) continue;
    if (!matchesAxis(resource, axisId, values)) return false;
  }
  return true;
}

// The selection with one value flipped on or off, which is what every chip links
// to: a click composes with the rest of the selection instead of replacing it.
function toggled(
  selection: FacetSelection,
  axisId: string,
  valueId: string,
): FacetSelection {
  const next: FacetSelection = {};
  for (const [id, values] of Object.entries(selection)) next[id] = [...values];
  const values = next[axisId] ?? [];
  const at = values.indexOf(valueId);
  if (at >= 0) values.splice(at, 1);
  else values.push(valueId);
  if (values.length > 0) next[axisId] = values.sort();
  else delete next[axisId];
  return next;
}

// Route for a document path from the index (`<type>/<name>.md`, relative to the
// tenant's docs folder). The `.md` suffix is dropped: routes are extensionless.
// The whole selection rides along so no filter is lost by navigating.
function docHref(
  tenantId: string,
  doc: string,
  index: TenantIndex,
  selection: FacetSelection,
): string {
  return selectionHref(
    `/${tenantId}/${stripExtension(doc)}`,
    index,
    selection,
  );
}

// The one place a filtered URL is built: axes in the index's order, values
// sorted, so a given selection always produces the same URL.
export function selectionHref(
  basePath: string,
  index: TenantIndex,
  selection: FacetSelection,
): string {
  const parts: string[] = [];
  for (const axis of filterableAxes(index)) {
    for (const value of selection[axis.id] ?? []) {
      parts.push(
        `${encodeURIComponent(axis.id)}=${encodeURIComponent(value)}`,
      );
    }
  }
  return parts.length > 0 ? `${basePath}?${parts.join('&')}` : basePath;
}

function badgesFor(index: TenantIndex, resource: IndexResource): string[] {
  const badges: string[] = [];
  const seen = new Set<string>();
  const push = (value: string): void => {
    const key = value.trim().toLowerCase();
    if (key === '' || seen.has(key)) return;
    seen.add(key);
    badges.push(value);
  };
  // Taxonomy membership first, in the header's axis order: it is why a reader is
  // looking at this listing, and it is what makes the filter discoverable from a
  // resource rather than only from the chooser. Labels come from the header,
  // since a membership carries value ids only.
  for (const axis of index.facets) {
    for (const id of resource.facets[axis.id] ?? []) {
      push(facetLabel(axis, resource, id));
    }
  }
  push(resource.platformGroup);
  push(resource.functionGroup);
  push(resource.platforms);
  push(resource.scope);
  const a = resource.assignments;
  if (a) {
    if (a.allUsers) push('all users');
    if (a.allDevices) push('all devices');
    if (a.groups > 0) push(`${a.groups} group${a.groups === 1 ? '' : 's'}`);
    if (a.targetedBy > 0) push(`targeted by ${a.targetedBy}`);
    if (!a.allUsers && !a.allDevices && a.groups === 0 && a.targetedBy === 0) {
      push('unassigned');
    }
  }
  return badges;
}

// A value's display label. The header is the source of truth; a version-2 index
// whose per-resource `groups` name a value its registry does not declare still
// gets its label from the resource, and an unlabelled id renders as itself.
function facetLabel(
  axis: IndexFacet,
  resource: IndexResource,
  valueId: string,
): string {
  const value = axis.values.find((v) => v.id === valueId);
  if (value) return value.label;
  const group = resource.groups.find((g) => g.id === valueId);
  return group ? group.label : valueId;
}

function toResource(src: Record<string, any>): IndexResource {
  const assignments = src.assignments;
  const groups = toGroups(src.groups);
  return {
    type: str(src.type),
    doc: str(src.doc),
    displayName: str(src.displayName),
    summary: str(src.summary),
    documented: src.documented === true,
    scope: str(src.scope),
    platformGroup: str(src.platformGroup),
    functionGroup: str(src.functionGroup),
    odataType: str(src.odataType),
    platforms: str(src.platforms),
    groups,
    facets: toMembership(src.facets, groups),
    assignments:
      assignments && typeof assignments === 'object'
        ? {
            groups: num(assignments.groups),
            allUsers: assignments.allUsers === true,
            allDevices: assignments.allDevices === true,
            targetedBy: num(assignments.targetedBy),
          }
        : null,
  };
}

function typeLabel(type: string): string {
  const leaf = type.split('/').pop() || type;
  const spaced = leaf.replace(/([a-z0-9])([A-Z])/g, '$1 $2');
  return spaced.charAt(0).toUpperCase() + spaced.slice(1);
}

function stripExtension(doc: string): string {
  return doc.replace(/\.md$/i, '');
}

function str(value: unknown): string {
  return typeof value === 'string' ? value : '';
}

function num(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0;
}

function strList(value: unknown): string[] {
  return Array.isArray(value) ? value.map(str).filter(Boolean) : [];
}

function toGroups(value: unknown): IndexGroup[] {
  if (!Array.isArray(value)) return [];
  return value
    .filter((g: unknown) => !!g && typeof g === 'object')
    .map((g: Record<string, any>) => ({
      id: str(g.id),
      label: str(g.label) || str(g.id),
    }))
    .filter((g: IndexGroup) => g.id !== '');
}

// A resource's per-axis membership (`axis id -> value ids`). The map's key order
// is the CLI's serialisation order, not a display order, so it is never used for
// ordering — that comes from the header. A version-2 index has no map: its single
// programme axis is reconstructed from `groups`.
function toMembership(
  value: unknown,
  groups: IndexGroup[],
): Record<string, string[]> {
  if (value && typeof value === 'object' && !Array.isArray(value)) {
    const membership: Record<string, string[]> = {};
    for (const [axisId, ids] of Object.entries(value as Record<string, any>)) {
      const values = strList(ids);
      if (axisId !== '' && values.length > 0) membership[axisId] = values;
    }
    return membership;
  }
  return groups.length > 0
    ? { [LEGACY_AXIS_ID]: groups.map((g) => g.id) }
    : {};
}

function toFacets(value: unknown): IndexFacet[] {
  if (!Array.isArray(value)) return [];
  return value
    .filter((f: unknown) => !!f && typeof f === 'object')
    .map((f: Record<string, any>) => ({
      id: str(f.id),
      label: str(f.label) || str(f.id),
      values: Array.isArray(f.values)
        ? f.values
            .filter((v: unknown) => !!v && typeof v === 'object')
            .map((v: Record<string, any>) => ({
              id: str(v.id),
              label: str(v.label) || str(v.id),
              count: num(v.count),
            }))
            .filter((v: IndexFacetValue) => v.id !== '')
        : [],
    }))
    .filter((f: IndexFacet) => f.id !== '' && !f.id.startsWith('_'));
}

// The single axis a version-2 index expresses through its `programmes` registry.
function legacyFacets(programmes: IndexProgramme[]): IndexFacet[] {
  if (programmes.length === 0) return [];
  return [
    {
      id: LEGACY_AXIS_ID,
      label: 'Programme',
      values: programmes.map((p) => ({
        id: p.id,
        label: p.label,
        count: p.count,
      })),
    },
  ];
}

// Query values for one axis: a parameter may be absent, single or repeated.
function queryValues(value: unknown): string[] {
  if (typeof value === 'string') return value === '' ? [] : [value];
  if (Array.isArray(value)) return value.flatMap((v) => queryValues(v));
  return [];
}

function toProgrammes(value: unknown): IndexProgramme[] {
  if (!Array.isArray(value)) return [];
  return value
    .filter((p: unknown) => !!p && typeof p === 'object')
    .map((p: Record<string, any>) => ({
      id: str(p.id),
      label: str(p.label) || str(p.id),
      count: num(p.count),
    }))
    .filter((p: IndexProgramme) => p.id !== '');
}

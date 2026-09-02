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
  vocabularies: { platform: string[]; function: string[] };
  resources: IndexResource[];
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
}

// One choice in the programme filter, including the "All" and "Uncategorised"
// pseudo-programmes. `href` keeps the filter in the URL so the choice survives a
// reload with no client-side state.
export interface ProgrammeFilter {
  id: string;
  label: string;
  count: number;
  href: string;
  active: boolean;
}

// Filter value for resources the taxonomy matched to no programme. Prefixed like
// the `_resource`/`_export` route segments because it is a representation rather
// than data: no taxonomy id can collide with it.
export const UNCATEGORISED = '_uncategorised';

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
    programmes: toProgrammes(src.programmes),
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
// `programme` narrows the tree to one programme from the index (or to
// UNCATEGORISED, the resources the taxonomy matched to none). Membership is read,
// never derived: the CLI resolves the taxonomy so the browser and the Confluence
// export group identically. An index that declares no programmes cannot be
// filtered at all — it renders exactly the tree it rendered before the taxonomy
// existed. The document being viewed is always kept, so a filter cannot make the
// page you are on vanish from its own sidebar.
export function buildNavigation(
  index: TenantIndex,
  tenantId: string,
  activeDoc = '',
  programme = '',
): NavSection[] {
  const sections = new Map<string, NavSection>();
  const active = stripExtension(activeDoc);
  const filter = index.programmes.length > 0 ? programme : '';

  for (const resource of index.resources) {
    const isActive = active !== '' && stripExtension(resource.doc) === active;
    if (!isActive && !inProgramme(resource, filter)) continue;

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
      href: docHref(tenantId, resource.doc, filter),
      label: resource.displayName || stripExtension(resource.doc),
      summary: resource.summary,
      documented: resource.documented,
      badges: badgesFor(resource),
      active: isActive,
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

// The programme filter, for the sidebar: "All", every programme the taxonomy
// defines (in the index's display order, zero-count entries kept, since "this
// programme matched nothing here" is itself information) and the uncategorised
// bucket, which is always offered so a taxonomy that stops matching shows up as a
// full bucket rather than as a thinning tree.
//
// Returns [] when the index declares no programmes: an export written without a
// taxonomy renders exactly the tree it rendered before.
//
// `basePath` is the current page's own path, so choosing a programme keeps you on
// the page you are on.
export function buildProgrammeFilters(
  index: TenantIndex,
  basePath: string,
  activeProgramme = '',
): ProgrammeFilter[] {
  if (index.programmes.length === 0) return [];

  const uncategorised = index.resources.filter(
    (r) => r.groups.length === 0,
  ).length;

  const filters: ProgrammeFilter[] = [
    {
      id: '',
      label: 'All',
      count: index.resources.length,
      href: basePath,
      active: activeProgramme === '',
    },
  ];
  for (const programme of index.programmes) {
    filters.push({
      id: programme.id,
      label: programme.label,
      count: programme.count,
      href: programmeHref(basePath, programme.id),
      active: activeProgramme === programme.id,
    });
  }
  filters.push({
    id: UNCATEGORISED,
    label: 'Uncategorised',
    count: uncategorised,
    href: programmeHref(basePath, UNCATEGORISED),
    active: activeProgramme === UNCATEGORISED,
  });
  return filters;
}

// Whether a programme filter is a value this index can serve. An unknown value
// would otherwise render an empty tree that looks like a broken tenant.
export function isKnownProgramme(index: TenantIndex, programme: string): boolean {
  if (programme === '') return true;
  if (programme === UNCATEGORISED) return index.programmes.length > 0;
  return index.programmes.some((p) => p.id === programme);
}

function inProgramme(resource: IndexResource, programme: string): boolean {
  if (programme === '') return true;
  if (programme === UNCATEGORISED) return resource.groups.length === 0;
  return resource.groups.some((g) => g.id === programme);
}

// Route for a document path from the index (`<type>/<name>.md`, relative to the
// tenant's docs folder). The `.md` suffix is dropped: routes are extensionless.
// An active programme rides along so the filter survives navigation.
function docHref(tenantId: string, doc: string, programme = ''): string {
  return programmeHref(`/${tenantId}/${stripExtension(doc)}`, programme);
}

function programmeHref(basePath: string, programme: string): string {
  if (programme === '') return basePath;
  return `${basePath}?programme=${encodeURIComponent(programme)}`;
}

function badgesFor(resource: IndexResource): string[] {
  const badges: string[] = [];
  // Programme membership first: it is why a reader is looking at this listing,
  // and it is what makes the filter discoverable from a resource rather than
  // only from the chooser.
  for (const group of resource.groups) badges.push(group.label);
  if (resource.platformGroup) badges.push(resource.platformGroup);
  if (resource.functionGroup) badges.push(resource.functionGroup);
  if (resource.platforms) badges.push(resource.platforms);
  if (resource.scope) badges.push(resource.scope);
  const a = resource.assignments;
  if (a) {
    if (a.allUsers) badges.push('all users');
    if (a.allDevices) badges.push('all devices');
    if (a.groups > 0) {
      badges.push(`${a.groups} group${a.groups === 1 ? '' : 's'}`);
    }
    if (a.targetedBy > 0) badges.push(`targeted by ${a.targetedBy}`);
    if (!a.allUsers && !a.allDevices && a.groups === 0 && a.targetedBy === 0) {
      badges.push('unassigned');
    }
  }
  return badges;
}

function toResource(src: Record<string, any>): IndexResource {
  const assignments = src.assignments;
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
    groups: Array.isArray(src.groups)
      ? src.groups
          .filter((g: unknown) => !!g && typeof g === 'object')
          .map((g: Record<string, any>) => ({
            id: str(g.id),
            label: str(g.label) || str(g.id),
          }))
          .filter((g: IndexGroup) => g.id !== '')
      : [],
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

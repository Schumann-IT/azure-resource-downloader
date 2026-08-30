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
  resources: IndexResource[];
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
  assignments: IndexAssignments | null;
}

export interface IndexAssignments {
  groups: number;
  allUsers: boolean;
  allDevices: boolean;
  targetedBy: number;
}

// A section of the tenant landing page: one resource type, its documents.
export interface NavSection {
  key: string;
  label: string;
  items: NavItem[];
}

export interface NavItem {
  href: string;
  label: string;
  summary: string;
  documented: boolean;
  badges: string[];
}

// Parses `docs/index.yaml`. Returns undefined for anything that is not a
// version-1 index object, so a malformed file makes the folder *not a tenant*
// rather than crashing discovery.
export function parseTenantIndex(raw: string): TenantIndex | undefined {
  let doc: unknown;
  try {
    doc = yaml.load(raw);
  } catch {
    return undefined;
  }
  if (!doc || typeof doc !== 'object' || Array.isArray(doc)) return undefined;

  const src = doc as Record<string, any>;
  if (src.version !== 1) return undefined;
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

  return {
    version: 1,
    tenant: src.tenant,
    generatedAt: str(src.generatedAt) || null,
    complete: src.complete !== false,
    incompleteReason: str(src.incompleteReason) || null,
    counts: {
      documented: num(counts.documented),
      pending: num(counts.pending),
      excluded,
    },
    resources,
  };
}

// Groups the index into the landing-page navigation. Grouping is by resource
// type: `platformGroup`/`functionGroup` are optional enrichment the documents
// do not carry yet, so they are surfaced as per-item badges instead of driving
// the tree (see NEXT-ITERATIONS.md).
export function buildNavigation(
  index: TenantIndex,
  tenantId: string,
): NavSection[] {
  const sections = new Map<string, NavSection>();

  for (const resource of index.resources) {
    let section = sections.get(resource.type);
    if (!section) {
      section = { key: resource.type, label: typeLabel(resource.type), items: [] };
      sections.set(resource.type, section);
    }
    section.items.push({
      href: docHref(tenantId, resource.doc),
      label: resource.displayName || stripExtension(resource.doc),
      summary: resource.summary,
      documented: resource.documented,
      badges: badgesFor(resource),
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

// Route for a document path from the index (`<type>/<name>.md`, relative to the
// tenant's docs folder). The `.md` suffix is dropped: routes are extensionless.
function docHref(tenantId: string, doc: string): string {
  return `/${tenantId}/${stripExtension(doc)}`;
}

function badgesFor(resource: IndexResource): string[] {
  const badges: string[] = [];
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

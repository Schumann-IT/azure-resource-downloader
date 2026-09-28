import * as yaml from 'js-yaml';

// The export's own metadata, `resources/metadata.yaml`: its top-level
// `generatedAt` is the baseline a drift observation is valid against, and its
// `resources:` map lists every written resource — including the types the index
// only counts under `counts.excluded` — which is what the tenant compare lists.
export const RESOURCES_METADATA_FILE = 'metadata.yaml';

// One resource the export wrote. `key` is the extensionless
// `<APIType>/<endpoint>/<name>` path, the same string the routes and
// `resolveResource` use; the file's own map keys carry a `.yaml` suffix.
export interface MetadataEntry {
  key: string;
  type: string;
  displayName: string;
  resourceId?: string;
  odataType?: string;
  presentInTenant: boolean;
}

// `resourceId → every displayName an entry with that id carries`. Set-valued
// because neither side is unique in real exports: one id can name several
// resources, and one name can belong to several.
export type ReferenceLookup = Map<string, Set<string>>;

export interface ResourcesMetadata {
  entries: MetadataEntry[];
  // How many entries carry each display name, so a resolved reference to a
  // name shared by several resources can be counted in the compare report.
  nameCounts: Map<string, number>;
  lookup: ReferenceLookup;
}

// Parses `resources/metadata.yaml`. Shape-validated and degrading: anything
// that is not an object with a `resources:` map returns undefined, and an entry
// with an unsafe key or no usable fields is skipped — never a throw. Loaded with
// the core schema so scalars stay text (an unquoted date is not turned into a
// `Date`). Pure and Nest-free.
export function parseResourcesMetadata(raw: string): ResourcesMetadata | undefined {
  let doc: unknown;
  try {
    doc = yaml.load(raw, { schema: yaml.CORE_SCHEMA });
  } catch {
    return undefined;
  }
  if (!isRecord(doc) || !isRecord(doc.resources)) return undefined;

  const entries: MetadataEntry[] = [];
  for (const [rawKey, value] of Object.entries(doc.resources)) {
    const key = safeKey(rawKey);
    if (!key || !isRecord(value)) continue;
    entries.push({
      key,
      type: key.slice(0, key.lastIndexOf('/')),
      displayName: text(value.displayName) || key.slice(key.lastIndexOf('/') + 1),
      resourceId: text(value.resourceId) || undefined,
      odataType: text(value.odataType) || undefined,
      // Absent counts as present: the field marks resources the export keeps
      // after they left the tenant, so only an explicit `false` excludes one.
      presentInTenant: value.presentInTenant !== false,
    });
  }
  entries.sort((a, b) => a.key.localeCompare(b.key));

  const nameCounts = new Map<string, number>();
  const lookup: ReferenceLookup = new Map();
  for (const entry of entries) {
    nameCounts.set(entry.displayName, (nameCounts.get(entry.displayName) ?? 0) + 1);
    if (!entry.resourceId) continue;
    const names = lookup.get(entry.resourceId) ?? new Set<string>();
    names.add(entry.displayName);
    lookup.set(entry.resourceId, names);
  }
  return { entries, nameCounts, lookup };
}

// The extensionless key, or null when it could not be a resource path: at
// least a type and a name, and no empty, `.` or `..` segment.
function safeKey(rawKey: string): string | null {
  const key = rawKey.replace(/\.yaml$/i, '');
  const segments = key.split('/');
  if (segments.length < 2) return null;
  if (segments.some((s) => s === '' || s === '.' || s === '..' || s.includes('\0'))) {
    return null;
  }
  return key;
}

function text(value: unknown): string {
  if (typeof value === 'string') return value;
  if (typeof value === 'number' || typeof value === 'boolean') return String(value);
  return '';
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

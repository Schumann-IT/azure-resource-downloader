import * as yaml from 'js-yaml';
import { ReferenceLookup } from './resources-metadata';

// PROVISIONAL — the cross-tenant identity rule of the tenant compare proof of
// concept. It is a judgment, not a fact, and lives in the browser only to find
// out what the rule is against real stage/prod pairs; once it is stable it
// moves to the CLI (`azure-rd resource compare`, parked in go/NEXT-ITERATIONS.md)
// and this file is deleted. Every constant below is data so the next iteration
// is a data change. Pure and Nest-free.

// Keys dropped at any depth: per-tenant timestamps and counters, and the
// group identity fields that differ between tenants for the same group.
export const DROPPED_KEYS: readonly string[] = [
  'createdDateTime',
  'lastModifiedDateTime',
  'version',
  'mail',
  'mailNickname',
  'proxyAddresses',
  'securityIdentifier',
  'renewedDateTime',
];

// Keys dropped at any depth only when their value contains a GUID: the
// resource's own id and the composites built from it. Without a GUID an id is
// content — settings ordinals, `all_users`, authentication method names.
export const IDENTITY_KEYS: readonly string[] = ['id', 'sourceId'];

// Any key ending in this, at any depth: each embeds the resource's own GUID.
export const CONTEXT_SUFFIX = '@odata.context';

// Keys whose value references another exported resource by id; resolved to
// that resource's display name, never dropped.
export const REFERENCE_KEYS: readonly string[] = [
  'groupId',
  'deviceAndAppManagementAssignmentFilterId',
  'notificationTemplateId',
];

export const GUID = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/gi;

// The well-known all-zero GUIDs (`…000`, `…001`) are values meaning "none" or
// "default", not identities.
export const SENTINEL = /^00000000-0000-0000-0000-0000000000[0-9a-f]{2}$/i;

// What the normaliser did, so the page can announce it rather than hide it.
export interface NormaliseReport {
  dropped: number;
  droppedKeys: string[];
  resolved: number;
  ambiguous: number;
  unresolved: number;
  // Resolved references whose name is carried by more than one resource.
  sharedNames: number;
}

export interface NormalisedResource {
  text: string;
  report: NormaliseReport;
}

export interface NormaliseOptions {
  // Also remove the top-level `assignments`, for the "differs only in
  // audience" check.
  withoutAssignments?: boolean;
}

const DROPPED = new Set(DROPPED_KEYS);
const IDENTITY = new Set(IDENTITY_KEYS);
const REFERENCE = new Set(REFERENCE_KEYS);

// Rewrites one exported resource so that equal configuration in two tenants
// yields equal text: tenant-local identity dropped, references resolved through
// that tenant's own lookup, serialised canonically (sorted keys, core schema on
// both sides so no scalar changes type). Undefined when the YAML does not parse
// to an object — the caller shows the raw diff instead.
export function normaliseResource(
  raw: string,
  lookup: ReferenceLookup,
  nameCounts: Map<string, number>,
  options: NormaliseOptions = {},
): NormalisedResource | undefined {
  let doc: unknown;
  try {
    doc = yaml.load(raw, { schema: yaml.CORE_SCHEMA });
  } catch {
    return undefined;
  }
  if (!isRecord(doc)) return undefined;
  const source: Record<string, unknown> = { ...doc };
  if (options.withoutAssignments) delete source.assignments;

  const state: WalkState = {
    report: { dropped: 0, droppedKeys: [], resolved: 0, ambiguous: 0, unresolved: 0, sharedNames: 0 },
    keys: new Set<string>(),
    lookup,
    nameCounts,
  };
  const normalised = walk(source, state);
  state.report.droppedKeys = [...state.keys].sort();
  return {
    text: yaml.dump(normalised, {
      schema: yaml.CORE_SCHEMA,
      sortKeys: true,
      lineWidth: -1,
      noRefs: true,
    }),
    report: state.report,
  };
}

// Whether a value names a tenant-local identity: it contains a GUID that is not
// one of the all-zero sentinels.
export function carriesIdentity(value: unknown): boolean {
  if (typeof value !== 'string') return false;
  return (value.match(GUID) ?? []).some((g) => !SENTINEL.test(g));
}

interface WalkState {
  report: NormaliseReport;
  keys: Set<string>;
  lookup: ReferenceLookup;
  nameCounts: Map<string, number>;
}

function walk(value: unknown, state: WalkState): unknown {
  if (Array.isArray(value)) return value.map((v) => walk(v, state));
  if (!isRecord(value)) return value;
  const out: Record<string, unknown> = {};
  for (const [key, child] of Object.entries(value)) {
    if (dropKey(key, child)) {
      state.report.dropped++;
      state.keys.add(key.endsWith(CONTEXT_SUFFIX) ? `*${CONTEXT_SUFFIX}` : key);
      continue;
    }
    out[key] = REFERENCE.has(key) ? resolve(child, state) : walk(child, state);
  }
  return out;
}

function dropKey(key: string, value: unknown): boolean {
  if (DROPPED.has(key) || key.endsWith(CONTEXT_SUFFIX)) return true;
  return IDENTITY.has(key) && carriesIdentity(value);
}

function resolve(value: unknown, state: WalkState): unknown {
  if (typeof value !== 'string' || SENTINEL.test(value)) return value;
  const names = state.lookup.get(value);
  if (!names) {
    state.report.unresolved++;
    return value;
  }
  if (names.size !== 1) {
    state.report.ambiguous++;
    return value;
  }
  const [name] = names;
  state.report.resolved++;
  if ((state.nameCounts.get(name) ?? 0) > 1) state.report.sharedNames++;
  return name;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

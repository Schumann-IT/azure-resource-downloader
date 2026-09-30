import * as yaml from 'js-yaml';
import { DriftObservation, driftKey, isRecord, num, str, timestamp } from './drift-observation';

// The closed set of attribution statuses `azure-rd resource audit` records. A
// finding carrying anything else is dropped at parse time rather than shown as
// a status this browser cannot explain.
export const AUDIT_STATUSES = [
  'matched',
  'no-event-in-window',
  'no-join-key',
  'retention-exceeded',
  'query-failed',
  'not-queried',
] as const;
export type AuditStatus = (typeof AUDIT_STATUSES)[number];

const ACTOR_TYPES = ['user', 'application', 'unknown'] as const;
export type AuditActorType = (typeof ACTOR_TYPES)[number];

const RESULTS = ['success', 'failure', 'unknown'] as const;
export type AuditResult = (typeof RESULTS)[number];

const TABLES = ['IntuneAuditLogs', 'AuditLogs'] as const;

// One audit event that touched a drifted resource, exactly as the CLI recorded it.
export interface AuditEvent {
  at: string;
  actor: string;
  actorType: AuditActorType;
  activity: string;
  result: AuditResult;
  correlationId: string;
}

// What the audit file says about one finding. `key` is the extensionless route
// key, reduced by the same rule as the observation's.
export interface AuditAttribution {
  key: string;
  status: AuditStatus;
  table: string;
  reason: string;
  events: AuditEvent[];
}

export interface AuditTable {
  name: string;
  failed: boolean;
  reason: string;
  earliest: string;
}

export interface AuditCounts {
  matched: number;
  noEventInWindow: number;
  noJoinKey: number;
  retentionExceeded: number;
  queryFailed: number;
  notQueried: number;
}

// `drift/audit.yaml`, the attribution `azure-rd resource audit` writes beside the
// observation. Read as data and never served; the app derives nothing from it
// and joins it to the observation by finding key only.
export interface DriftAudit {
  version: number;
  observedAt: string;
  baselineGeneratedAt: string;
  tenant: string;
  toolVersion: string;
  queriedAt: string;
  workspaceId: string;
  window: { from: string; to: string };
  tables: AuditTable[];
  counts: AuditCounts;
  findings: AuditAttribution[];
  byKey: Map<string, AuditAttribution>;
}

// The audit relative to the observation on disk: absent, outdated (it describes
// another observation, so it is never shown) or current.
export type AuditState =
  | { kind: 'none' }
  | { kind: 'outdated'; audit: DriftAudit }
  | { kind: 'current'; audit: DriftAudit };

// Parses `drift/audit.yaml`. Never throws: anything that is not an object with an
// integer `version >= 1`, an `observedAt` and a `baselineGeneratedAt` returns
// undefined, so a malformed or half-understood file degrades to "no audit".
// Findings with an unsafe key or an unknown status are dropped, and so is a
// `matched` finding without a single well-formed event.
export function parseAudit(raw: string): DriftAudit | undefined {
  let doc: unknown;
  try {
    doc = yaml.load(raw);
  } catch {
    return undefined;
  }
  if (!isRecord(doc)) return undefined;
  if (!Number.isInteger(doc.version) || doc.version < 1) return undefined;
  const observedAt = timestamp(doc.observedAt);
  const baselineGeneratedAt = timestamp(doc.baselineGeneratedAt);
  if (!observedAt || !baselineGeneratedAt) return undefined;

  const window = isRecord(doc.window) ? doc.window : {};
  const findings = toAttributions(doc.findings);
  return {
    version: doc.version,
    observedAt,
    baselineGeneratedAt,
    tenant: str(doc.tenant),
    toolVersion: str(doc.toolVersion),
    queriedAt: timestamp(doc.queriedAt),
    workspaceId: str(doc.workspaceId),
    window: { from: timestamp(window.from), to: timestamp(window.to) },
    tables: toTables(doc.tables),
    counts: toCounts(doc.counts),
    findings,
    byKey: new Map(findings.map((f) => [f.key, f])),
  };
}

// The audit against the observation it must describe: exactly that observation,
// both timestamps, or it is outdated. An audit for a superseded observation is
// never asked about, because the caller only evaluates it for a current one.
export function auditState(
  audit: DriftAudit | undefined,
  observation: DriftObservation,
): AuditState {
  if (!audit) return { kind: 'none' };
  const current =
    audit.observedAt === observation.observedAt &&
    audit.baselineGeneratedAt === observation.baselineGeneratedAt;
  return current ? { kind: 'current', audit } : { kind: 'outdated', audit };
}

function toAttributions(value: unknown): AuditAttribution[] {
  if (!isRecord(value)) return [];
  const out: AuditAttribution[] = [];
  for (const [rawKey, entry] of Object.entries(value)) {
    const key = driftKey(rawKey);
    if (key === '' || !isRecord(entry) || !isStatus(entry.status)) continue;
    const events = entry.status === 'matched' ? toEvents(entry.events) : [];
    if (entry.status === 'matched' && events.length === 0) continue;
    out.push({
      key,
      status: entry.status,
      table: str(entry.table),
      reason: str(entry.reason),
      events,
    });
  }
  return out.sort((a, b) => a.key.localeCompare(b.key));
}

// Newest first, defensively: the CLI already writes them that way. Timestamps
// are whole-second RFC3339 UTC strings, so string order is time order.
function toEvents(value: unknown): AuditEvent[] {
  if (!Array.isArray(value)) return [];
  return value
    .filter(isRecord)
    .map((e) => ({
      at: timestamp(e.at),
      actor: str(e.actor),
      actorType: oneOf(ACTOR_TYPES, e.actorType),
      activity: str(e.activity),
      result: oneOf(RESULTS, e.result),
      correlationId: str(e.correlationId),
    }))
    .filter((e) => e.at !== '' && e.actor !== '')
    .sort((a, b) => b.at.localeCompare(a.at));
}

function toTables(value: unknown): AuditTable[] {
  const src = isRecord(value) ? value : {};
  return TABLES.map((name) => {
    const t = isRecord(src[name]) ? src[name] : {};
    return {
      name,
      failed: t.status === 'failed',
      reason: str(t.reason),
      earliest: timestamp(t.earliest),
    };
  });
}

function toCounts(value: unknown): AuditCounts {
  const src = isRecord(value) ? value : {};
  return {
    matched: num(src.matched),
    noEventInWindow: num(src.noEventInWindow),
    noJoinKey: num(src.noJoinKey),
    retentionExceeded: num(src.retentionExceeded),
    queryFailed: num(src.queryFailed),
    notQueried: num(src.notQueried),
  };
}

function isStatus(value: unknown): value is AuditStatus {
  return (AUDIT_STATUSES as readonly unknown[]).includes(value);
}

// A member of a closed set, or 'unknown' — the set's own fallback.
function oneOf<T extends string>(set: readonly T[], value: unknown): T {
  return (set as readonly unknown[]).includes(value) ? (value as T) : ('unknown' as T);
}

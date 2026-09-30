import * as yaml from 'js-yaml';
import { TenantIndex } from './tenant-index';

// The verdicts `azure-rd resource drift` decides. A finding carrying anything
// else is dropped at parse time rather than rendered as a verdict this browser
// cannot explain.
export const DRIFT_VERDICTS = ['added', 'changed', 'renamed', 'removed'] as const;
export type DriftVerdict = (typeof DRIFT_VERDICTS)[number];

// One dotted-path field change, with the values exactly as the CLI truncated
// and rendered them (`(absent)` for a one-sided key).
export interface DriftDelta {
  path: string;
  old: string;
  new: string;
}

// One finding of the observation. Both keys are extensionless paths relative
// to `drift/` (which mirrors `resources/`), because that is the shape every
// route in this app is addressed by: `key` is where the payload and the drift
// document sit, `baselineKey` is where the baseline file and the document sit.
// They differ for a rename, and `baselineKey` is '' for an addition.
export interface DriftFinding {
  key: string;
  verdict: DriftVerdict;
  resourceId: string;
  displayName: string;
  previousDisplayName: string;
  baselineKey: string;
  baselineSha256: string;
  payloadSha256: string;
  deltas: DriftDelta[];
  deltaNote: string;
}

export interface DriftCounts {
  compared: number;
  unchanged: number;
  changed: number;
  renamed: number;
  added: number;
  removed: number;
  unattested: number;
  excluded: number;
  failed: number;
}

// `drift/metadata.yaml`, the observation `azure-rd resource drift` writes. It
// plays the role for `drift/` that `index.yaml` plays for `docs/`: everything
// the drift views list or count is read from here, never from walking the tree.
// The lookup structures are built once at parse time so the per-page drift
// decision is constant-time on the observation side.
export interface DriftObservation {
  observedAt: string;
  tenant: string;
  toolVersion: string;
  baselineGeneratedAt: string;
  baselineToolVersion: string;
  complete: boolean;
  incompleteReason: string;
  counts: DriftCounts;
  unknownTypes: string[];
  removalsSuppressed: boolean;
  notComparable: Array<{ key: string; reason: string }>;
  // Sorted by key, so every listing built from it is deterministic.
  findings: DriftFinding[];
  // Extensionless keys of the payloads this observation wrote. A payload file
  // the observation does not name is never trusted.
  payloads: Set<string>;
  byKey: Map<string, DriftFinding>;
  byBaselineKey: Map<string, DriftFinding>;
  unknownTypeSet: Set<string>;
  notComparableByKey: Map<string, string>;
}

// The observation at tenant scope, relative to the export it sits in.
export type TenantDriftState =
  | { kind: 'none' }
  | {
      kind: 'superseded';
      observation: DriftObservation;
      baselineGeneratedAt: string | undefined;
    }
  | { kind: 'current'; observation: DriftObservation };

// The one decision both the Drift button and the drift page read, so the two
// can never disagree. See `driftState` for the evaluation order.
export type DriftState =
  | Exclude<TenantDriftState, { kind: 'current' }>
  | { kind: 'finding'; observation: DriftObservation; finding: DriftFinding }
  | { kind: 'unknownType'; observation: DriftObservation; type: string }
  | { kind: 'notComparable'; observation: DriftObservation; reason: string }
  | { kind: 'unchanged'; observation: DriftObservation }
  | { kind: 'unknown'; observation: DriftObservation };

// Parses `drift/metadata.yaml`. Version-free — the CLI writes no version — but
// shape-validated: anything that is not an observation object with an
// `observedAt` and the baseline it was compared against returns undefined, so a
// malformed or half-understood file degrades to "no observation" and never
// throws. Findings with an unknown verdict or an unsafe key are dropped.
export function parseObservation(raw: string): DriftObservation | undefined {
  let doc: unknown;
  try {
    doc = yaml.load(raw);
  } catch {
    return undefined;
  }
  if (!isRecord(doc)) return undefined;

  const observedAt = timestamp(doc.observedAt);
  const baseline = doc.baseline;
  if (!observedAt || !isRecord(baseline)) return undefined;
  const baselineGeneratedAt = timestamp(baseline.generatedAt);
  if (!baselineGeneratedAt) return undefined;
  if (doc.findings != null && !isRecord(doc.findings)) return undefined;

  const run = isRecord(doc.run) ? doc.run : {};
  const findings = toFindings(doc.findings ?? {});
  const notComparable = toNotComparable(doc.notComparable);
  const unknownTypes = strList(doc.unknownTypes);

  return {
    observedAt,
    tenant: str(doc.tenant),
    toolVersion: str(doc.toolVersion),
    baselineGeneratedAt,
    baselineToolVersion: str(baseline.toolVersion),
    complete: run.complete !== false,
    incompleteReason: str(run.incompleteReason),
    counts: toCounts(doc.counts),
    unknownTypes,
    removalsSuppressed: doc.removalsSuppressed === true,
    notComparable,
    findings,
    payloads: new Set(
      strList(doc.payloads)
        .map(driftKey)
        .filter((k) => k !== ''),
    ),
    byKey: new Map(findings.map((f) => [f.key, f])),
    byBaselineKey: new Map(
      findings.filter((f) => f.baselineKey !== '').map((f) => [f.baselineKey, f]),
    ),
    unknownTypeSet: new Set(unknownTypes),
    notComparableByKey: new Map(notComparable.map((n) => [n.key, n.reason])),
  };
}

// The top-level `generatedAt` of `resources/metadata.yaml` — the baseline
// timestamp `azure-rd docs analyze-drift` checks an observation against. Read
// by scanning for the one unindented key instead of parsing the whole file: the
// export metadata holds an entry per resource and this is the only value
// needed. Returns undefined when the key is absent or empty.
export function parseBaselineGeneratedAt(raw: string): string | undefined {
  for (const line of raw.split('\n')) {
    if (!line.startsWith('generatedAt:')) continue;
    const value = unquote(line.slice('generatedAt:'.length).trim());
    return value === '' ? undefined : value;
  }
  return undefined;
}

// The observation relative to the export: absent, superseded (compared against
// a baseline the export no longer holds — the same test the CLI applies), or
// current. A baseline that cannot be determined at all counts as superseded:
// a comparison that cannot be verified is never shown as one.
export function tenantDriftState(
  observation: DriftObservation | undefined,
  baselineGeneratedAt: string | undefined,
): TenantDriftState {
  if (!observation) return { kind: 'none' };
  if (observation.baselineGeneratedAt !== baselineGeneratedAt) {
    return { kind: 'superseded', observation, baselineGeneratedAt };
  }
  return { kind: 'current', observation };
}

// What the observation says about one resource, addressed by its extensionless
// path. In evaluation order: no observation; superseded (findings are not
// consulted); a finding by key or — for the old name of a rename — by
// `baselineKey`; a type the run could not list; a baseline entry it could not
// compare; a resource the index knows, which the observation compared without
// a finding (unchanged); otherwise unknown.
export function driftState(
  observation: DriftObservation | undefined,
  baselineGeneratedAt: string | undefined,
  index: TenantIndex | undefined,
  key: string,
): DriftState {
  const tenant = tenantDriftState(observation, baselineGeneratedAt);
  if (tenant.kind !== 'current') return tenant;
  const obs = tenant.observation;

  const finding = obs.byKey.get(key) ?? obs.byBaselineKey.get(key);
  if (finding) return { kind: 'finding', observation: obs, finding };

  const type = typeOfKey(key);
  if (obs.unknownTypeSet.has(type)) {
    return { kind: 'unknownType', observation: obs, type };
  }
  const reason = obs.notComparableByKey.get(key);
  if (reason !== undefined) {
    return { kind: 'notComparable', observation: obs, reason };
  }
  if (index?.resources.some((r) => stripMd(r.doc) === key)) {
    return { kind: 'unchanged', observation: obs };
  }
  return { kind: 'unknown', observation: obs };
}

// The resource type of an extensionless key (`<APIType>/<endpoint>/<name>`).
export function typeOfKey(key: string): string {
  const at = key.lastIndexOf('/');
  return at < 0 ? '' : key.slice(0, at);
}

function toFindings(src: Record<string, unknown>): DriftFinding[] {
  const findings: DriftFinding[] = [];
  for (const [rawKey, value] of Object.entries(src)) {
    const key = driftKey(rawKey);
    if (key === '' || !isRecord(value)) continue;
    const verdict = value.verdict;
    if (!isVerdict(verdict)) continue;
    findings.push({
      key,
      verdict,
      resourceId: str(value.resourceId),
      displayName: str(value.displayName),
      previousDisplayName: str(value.previousDisplayName),
      baselineKey: driftKey(value.baselineKey),
      baselineSha256: str(value.baselineSha256),
      payloadSha256: str(value.payloadSha256),
      deltas: toDeltas(value.deltas),
      deltaNote: str(value.deltaNote),
    });
  }
  return findings.sort((a, b) => a.key.localeCompare(b.key));
}

function toDeltas(value: unknown): DriftDelta[] {
  if (!Array.isArray(value)) return [];
  return value
    .filter(isRecord)
    .map((d) => ({ path: str(d.path), old: scalar(d.old), new: scalar(d.new) }))
    .filter((d) => d.path !== '');
}

function toNotComparable(value: unknown): Array<{ key: string; reason: string }> {
  if (!Array.isArray(value)) return [];
  return value
    .filter(isRecord)
    .map((n) => ({ key: driftKey(n.key), reason: str(n.reason) }))
    .filter((n) => n.key !== '');
}

function toCounts(value: unknown): DriftCounts {
  const src = isRecord(value) ? value : {};
  return {
    compared: num(src.compared),
    unchanged: num(src.unchanged),
    changed: num(src.changed),
    renamed: num(src.renamed),
    added: num(src.added),
    removed: num(src.removed),
    unattested: num(src.unattested),
    excluded: num(src.excluded),
    failed: num(src.failed),
  };
}

// A key as the CLI writes it (`<type>/<name>.yaml`, relative to `drift/` and
// `resources/`), reduced to the extensionless route path. Anything that is not
// a relative `.yaml` path without `..` segments is refused, so a key read from
// the observation is safe to put into an href. It is never turned into a
// filesystem path here — that is `path-safety.ts`'s job.
export function driftKey(value: unknown): string {
  if (typeof value !== 'string') return '';
  if (!value.toLowerCase().endsWith('.yaml')) return '';
  if (value.startsWith('/') || value.includes('\0')) return '';
  if (value.split('/').some((s) => s === '..' || s === '')) return '';
  return value.slice(0, -'.yaml'.length);
}

function isVerdict(value: unknown): value is DriftVerdict {
  return (DRIFT_VERDICTS as readonly unknown[]).includes(value);
}

export function isRecord(value: unknown): value is Record<string, any> {
  return !!value && typeof value === 'object' && !Array.isArray(value);
}

function stripMd(doc: string): string {
  return doc.replace(/\.md$/i, '');
}

// A timestamp as a string. The CLI quotes them, but an unquoted one would be
// loaded as a Date by js-yaml, and the gate compares strings.
export function timestamp(value: unknown): string {
  if (value instanceof Date && !Number.isNaN(value.getTime())) {
    return value.toISOString().replace('.000Z', 'Z');
  }
  return str(value);
}

// A delta value as text, whatever YAML type it was loaded as.
function scalar(value: unknown): string {
  if (value === null || value === undefined) return '';
  if (value instanceof Date) return timestamp(value);
  return typeof value === 'object' ? JSON.stringify(value) : String(value);
}

function unquote(value: string): string {
  const first = value.charAt(0);
  if (value.length >= 2 && (first === '"' || first === "'") && value.endsWith(first)) {
    return value.slice(1, -1);
  }
  return value;
}

export function str(value: unknown): string {
  return typeof value === 'string' ? value : '';
}

export function num(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0;
}

function strList(value: unknown): string[] {
  return Array.isArray(value) ? value.map(str).filter(Boolean) : [];
}

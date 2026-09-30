import {
  AuditAttribution,
  AuditEvent,
  AuditState,
  AuditStatus,
  DriftAudit,
} from './drift-audit';
import {
  DriftFinding,
  DriftObservation,
  DriftState,
  DriftVerdict,
  TenantDriftState,
  typeOfKey,
} from './drift-observation';

// Route prefix for the drift representation. A *representation* prefix like
// `_resource` and `_export`: never a breadcrumb segment, and it cannot collide
// with a resource type because no Azure/Graph type segment starts with `_`.
export const DRIFT_PREFIX = '_drift';

// One entry of the top-bar switcher. An entry without `href` is inert — shown,
// so its absence is never ambiguous, with `reason` saying why.
export interface ViewSwitch {
  label: string;
  href?: string;
  active: boolean;
  reason?: string;
}

// A badge for the `badge` partial. The tone is spelled out as booleans because
// the templates have no `eq` helper, and the Tailwind classes live in the
// partial so they are scanned.
export interface Badge {
  label: string;
  danger: boolean;
  warning: boolean;
  success: boolean;
  info: boolean;
}

type Tone = 'danger' | 'warning' | 'success' | 'info' | 'neutral';

const VERDICT_TONE: Record<DriftVerdict, Tone> = {
  added: 'success',
  changed: 'warning',
  renamed: 'info',
  removed: 'danger',
};

// The analysis's closed severity set. Anything else in a drift document's
// frontmatter is not shown as a severity at all.
const SEVERITY_TONE: Record<string, Tone> = {
  high: 'danger',
  medium: 'warning',
  low: 'info',
  info: 'neutral',
};

// How each attribution status is toned. Only a found actor is a success; a
// window or table that could not answer is a caveat (amber), and a status that
// is a fact about the resource type or the run is neutral.
export const STATUS_TONE: Record<AuditStatus, Tone> = {
  matched: 'success',
  'no-event-in-window': 'warning',
  'retention-exceeded': 'warning',
  'query-failed': 'warning',
  'not-queried': 'neutral',
  'no-join-key': 'neutral',
};

// The one-line text of every status that carries no actor.
export const STATUS_TEXT: Record<Exclude<AuditStatus, 'matched'>, string> = {
  'no-event-in-window': 'no audit event in the window',
  'retention-exceeded': "window starts before the table's retention",
  'query-failed': 'audit query failed',
  'not-queried': 'not queried',
  'no-join-key': 'no audit join key for this resource type',
};

// How an event that names neither a user nor an application is shown.
export const UNKNOWN_ACTOR = 'unknown actor';

const NO_ATTRIBUTION = 'no attribution recorded';

const NO_OBSERVATION = 'No drift observation. Run azure-rd resource drift.';

export function verdictBadge(verdict: DriftVerdict): Badge {
  return badge(verdict, VERDICT_TONE[verdict]);
}

export function severityBadge(severity: unknown): Badge | null {
  if (typeof severity !== 'string') return null;
  const tone = SEVERITY_TONE[severity.toLowerCase()];
  return tone ? badge(severity.toLowerCase(), tone) : null;
}

// The route of a resource's drift page. The page's own extensionless path is
// the key, so the button needs no lookup — for a rename the old name's key
// resolves the finding through `baselineKey`.
export function driftHref(tenant: string, key: string): string {
  return `/${tenant}/${DRIFT_PREFIX}/${key}`;
}

// The Drift entry on a documentation or YAML view, from the same decision the
// drift page renders. Present wherever the switcher is; inert when there is
// nothing to show, with the reason.
export function resourceDriftSwitch(
  state: DriftState,
  tenant: string,
  key: string,
  active: boolean,
): ViewSwitch {
  const href = driftHref(tenant, key);
  switch (state.kind) {
    case 'none':
      return { label: 'Drift', active, reason: NO_OBSERVATION };
    case 'superseded':
      return { label: 'Drift', href, active, reason: 'The drift observation is outdated' };
    case 'finding':
      return { label: `Drift · ${state.finding.verdict}`, href, active };
    case 'unknownType':
      return { label: 'Drift', href, active, reason: 'Not compared: its type could not be listed' };
    case 'notComparable':
      return { label: 'Drift', href, active, reason: 'Not compared' };
    case 'unchanged':
      return {
        label: 'Drift',
        active,
        reason: `Unchanged as of ${state.observation.observedAt}`,
      };
    default:
      return { label: 'Drift', active, reason: 'Not covered by the drift observation' };
  }
}

// The Summary | Drift switcher of the tenant landing and tenant drift pages. An
// observation — even an empty or outdated one — always links, because the page
// says what it is; only its absence is inert.
export function tenantSwitch(
  state: TenantDriftState,
  tenant: string,
  active: 'summary' | 'drift',
): ViewSwitch[] {
  const drift: ViewSwitch =
    state.kind === 'none'
      ? { label: 'Drift', active: active === 'drift', reason: NO_OBSERVATION }
      : { label: 'Drift', href: `/${tenant}/${DRIFT_PREFIX}`, active: active === 'drift' };
  return [{ label: 'Summary', href: `/${tenant}`, active: active === 'summary' }, drift];
}

// The drift part of a tenant's line on the picker, read from the same
// tenant-scope decision as the Summary | Drift switcher so the two cannot
// disagree. An outdated observation is dated but never counted: its findings
// describe a baseline the export no longer holds.
export function pickerDrift(state: TenantDriftState) {
  if (state.kind === 'none') return null;
  const findings = state.kind === 'current' ? state.observation.findings.length : 0;
  return {
    observedAt: state.observation.observedAt,
    outdated: state.kind === 'superseded',
    findings,
    resources: `${findings} resource${findings === 1 ? '' : 's'}`,
  };
}

// The observation header shown on both drift pages: when, against what, how
// complete, and every caveat the CLI recorded — so the reader can tell an
// empty finding list from a run that could not look.
export function observationSummary(
  obs: DriftObservation,
  tenant: string,
  audit: AuditState = { kind: 'none' },
) {
  const c = obs.counts;
  return {
    observedAt: obs.observedAt,
    baselineGeneratedAt: obs.baselineGeneratedAt,
    toolVersion: obs.toolVersion,
    complete: obs.complete,
    incompleteReason: obs.incompleteReason,
    verdicts: [
      { label: 'changed', count: c.changed },
      { label: 'renamed', count: c.renamed },
      { label: 'added', count: c.added },
      { label: 'removed', count: c.removed },
    ],
    compared: c.compared,
    unchanged: c.unchanged,
    unattested: c.unattested,
    excluded: c.excluded,
    failed: c.failed,
    unknownTypes: obs.unknownTypes,
    removalsSuppressed: obs.removalsSuppressed,
    notComparable: obs.notComparable.map((n) => ({
      key: n.key,
      reason: n.reason,
      href: driftHref(tenant, n.key),
    })),
    empty: obs.findings.length === 0,
    attribution: attributionSummary(audit),
  };
}

// The attribution caveat of the observation header: what was queried and what
// could not be answered, read from the audit file's own counts. An outdated
// audit is only a caveat; no audit is no line at all.
function attributionSummary(state: AuditState) {
  if (state.kind === 'none') return null;
  if (state.kind === 'outdated') return { outdated: true };
  const a = state.audit;
  return {
    outdated: false,
    workspaceId: a.workspaceId,
    queriedAt: a.queriedAt,
    window: a.window,
    tables: a.tables,
    failedTables: a.tables.filter((t) => t.failed),
    matched: a.counts.matched,
    noEvent: a.counts.noEventInWindow,
    noJoinKey: a.counts.noJoinKey,
    retention: a.counts.retentionExceeded,
    failed: a.counts.queryFailed,
    notQueried: a.counts.notQueried,
  };
}

// Every finding of the observation, grouped by resource type. Built from the
// observation alone, so a finding with no document or drift document of its own
// (an addition, an out-of-scope inventory change) is still reachable here.
export function findingGroups(
  obs: DriftObservation,
  tenant: string,
  audit?: DriftAudit,
) {
  const groups = new Map<string, ReturnType<typeof findingItem>[]>();
  for (const finding of obs.findings) {
    const type = typeOfKey(finding.key);
    const items = groups.get(type) ?? [];
    items.push(findingItem(tenant, finding, audit));
    groups.set(type, items);
  }
  return [...groups.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([type, items]) => ({
      type,
      items: items.sort((a, b) => a.label.localeCompare(b.label)),
    }));
}

function findingItem(tenant: string, finding: DriftFinding, audit: DriftAudit | undefined) {
  return {
    href: driftHref(tenant, finding.key),
    label: finding.displayName || lastSegment(finding.key),
    previous: finding.previousDisplayName,
    badge: verdictBadge(finding.verdict),
    attribution: rowAttribution(finding, audit),
  };
}

// The suffix of a finding row: the latest actor and time, or the status text.
// Null when there is no current audit at all, so the row stays as it was.
function rowAttribution(finding: DriftFinding, audit: DriftAudit | undefined) {
  if (!audit) return null;
  const a = attributionOf(finding, audit);
  if (a.latest) return { actor: a.latest.actor, at: a.latest.at, more: a.more };
  return { text: a.text, quiet: a.quiet, warning: a.tone.warning };
}

// What the audit file says about one finding, in the shape the templates read.
// A finding the file does not name is not inferred anything: it gets the
// neutral "no attribution recorded" line.
export function attributionOf(finding: DriftFinding, audit: DriftAudit | undefined) {
  const found: AuditAttribution | undefined = audit?.byKey.get(finding.key);
  if (!found) {
    return {
      recorded: false,
      status: {} as Record<string, boolean>,
      tone: badge(NO_ATTRIBUTION, 'neutral'),
      text: NO_ATTRIBUTION,
      quiet: true,
      table: '',
      reason: '',
      events: [] as AuditEvent[],
      latest: null as ReturnType<typeof displayEvent> | null,
      more: 0,
    };
  }
  const tone = STATUS_TONE[found.status];
  return {
    recorded: true,
    status: stateFlags(camel(found.status)),
    tone: badge(found.status, tone),
    text: found.status === 'matched' ? '' : STATUS_TEXT[found.status],
    quiet: found.status === 'no-join-key',
    table: found.table,
    reason: found.reason,
    events: found.events.map(displayEvent),
    latest: found.events[0] ? displayEvent(found.events[0]) : null,
    more: Math.max(found.events.length - 1, 0),
  };
}

// An event as the templates read it: a display actor for the empty one, and the
// flags the resource page's table needs.
function displayEvent(e: AuditEvent) {
  return {
    ...e,
    actor: e.actor || UNKNOWN_ACTOR,
    actorUser: e.actorType === 'user',
    failure: e.result === 'failure',
  };
}

// The tenant drift page's By actor section: every matched finding the
// observation holds, once per actor however many events that actor has on it.
export function byActor(obs: DriftObservation, audit: DriftAudit, tenant: string) {
  const blocks = new Map<
    string,
    { actor: string; actorType: string; findings: Array<{ href: string; label: string; badge: Badge; at: string }> }
  >();
  for (const finding of obs.findings) {
    const found = audit.byKey.get(finding.key);
    if (found?.status !== 'matched') continue;
    const seen = new Set<string>();
    for (const event of found.events) {
      const actor = event.actor || UNKNOWN_ACTOR;
      if (seen.has(actor)) continue;
      seen.add(actor);
      const block = blocks.get(actor) ?? {
        actor,
        actorType: event.actor ? event.actorType : 'unknown',
        findings: [],
      };
      block.findings.push({
        href: driftHref(tenant, finding.key),
        label: finding.displayName || lastSegment(finding.key),
        badge: verdictBadge(finding.verdict),
        at: event.at,
      });
      blocks.set(actor, block);
    }
  }
  const actors = [...blocks.values()]
    .sort((a, b) => a.actor.localeCompare(b.actor))
    .map((b) => ({
      ...b,
      tagged: b.actorType !== 'user',
      findings: b.findings.sort((x, y) => x.label.localeCompare(y.label)),
    }));
  return { window: audit.window, actors };
}

// `no-event-in-window` → `noEventInWindow`, the flag spelling the templates use.
function camel(status: string): string {
  return status.replace(/-(\w)/g, (_, c: string) => c.toUpperCase());
}

// Flags for the drift page templates, one per state kind.
export function stateFlags(kind: string): Record<string, boolean> {
  return { [kind]: true };
}

// What the validity gate tells the reader: which baseline the observation was
// taken against, and which one the export holds now.
export function supersededView(
  observation: DriftObservation,
  currentBaseline: string | undefined,
) {
  return {
    observedAt: observation.observedAt,
    observationBaseline: observation.baselineGeneratedAt,
    currentBaseline: currentBaseline ?? null,
  };
}

// The state-dependent part of a resource's drift page.
export function driftPageState(state: Exclude<DriftState, { kind: 'none' }>) {
  return {
    state: stateFlags(state.kind),
    observedAt: state.observation.observedAt,
    complete: state.observation.complete,
    superseded:
      state.kind === 'superseded'
        ? supersededView(state.observation, state.baselineGeneratedAt)
        : null,
    type: state.kind === 'unknownType' ? state.type : '',
    reason: state.kind === 'notComparable' ? state.reason : '',
  };
}

// The heading of a finding's drift page. The severity comes from the analysis
// agent's drift document, when there is one; the verdict from the CLI.
export function findingHeader(finding: DriftFinding, severity: unknown) {
  return {
    label: finding.displayName || lastSegment(finding.key),
    previous: finding.verdict === 'renamed' ? finding.previousDisplayName : '',
    verdictBadge: verdictBadge(finding.verdict),
    severityBadge: severityBadge(severity),
  };
}

export function lastSegment(key: string): string {
  return key.slice(key.lastIndexOf('/') + 1);
}

function badge(label: string, tone: Tone): Badge {
  return {
    label,
    danger: tone === 'danger',
    warning: tone === 'warning',
    success: tone === 'success',
    info: tone === 'info',
  };
}

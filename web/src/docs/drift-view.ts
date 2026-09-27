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

// The observation header shown on both drift pages: when, against what, how
// complete, and every caveat the CLI recorded — so the reader can tell an
// empty finding list from a run that could not look.
export function observationSummary(obs: DriftObservation, tenant: string) {
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
  };
}

// Every finding of the observation, grouped by resource type. Built from the
// observation alone, so a finding with no document or drift document of its own
// (an addition, an out-of-scope inventory change) is still reachable here.
export function findingGroups(obs: DriftObservation, tenant: string) {
  const groups = new Map<string, ReturnType<typeof findingItem>[]>();
  for (const finding of obs.findings) {
    const type = typeOfKey(finding.key);
    const items = groups.get(type) ?? [];
    items.push(findingItem(tenant, finding));
    groups.set(type, items);
  }
  return [...groups.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([type, items]) => ({
      type,
      items: items.sort((a, b) => a.label.localeCompare(b.label)),
    }));
}

function findingItem(tenant: string, finding: DriftFinding) {
  return {
    href: driftHref(tenant, finding.key),
    label: finding.displayName || lastSegment(finding.key),
    previous: finding.previousDisplayName,
    badge: verdictBadge(finding.verdict),
  };
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

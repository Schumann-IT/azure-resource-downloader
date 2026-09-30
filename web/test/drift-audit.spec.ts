import {
  AUDIT_STATUSES,
  auditState,
  parseAudit,
} from '../src/docs/drift-audit';
import { parseObservation } from '../src/docs/drift-observation';
import {
  attributionOf,
  byActor,
  changedByCells,
  findingGroups,
  observationSummary,
  STATUS_TEXT,
  STATUS_TONE,
} from '../src/docs/drift-view';

const BASELINE = '2026-01-01T00:00:00Z';
const OBSERVED = '2026-02-01T10:00:00Z';
const T = 'Microsoft.Graph/deviceConfigurations';
const LETTER_OF: Record<string, string> = { Alpha: 'a', Beta: 'b', Gamma: 'c' };

const OBSERVATION = `observedAt: "${OBSERVED}"
baseline:
    generatedAt: "${BASELINE}"
findings:
    ${T}/a.yaml:
        verdict: changed
        displayName: Alpha
    ${T}/b.yaml:
        verdict: changed
        displayName: Beta
    ${T}/c.yaml:
        verdict: removed
        displayName: Gamma
    ${T}/d.yaml:
        verdict: added
        displayName: Delta
`;

const AUDIT = `version: 1
observedAt: "${OBSERVED}"
baselineGeneratedAt: "${BASELINE}"
tenant: contoso.com
toolVersion: azure-rd v1
queriedAt: "2026-02-02T08:00:00Z"
workspaceId: ws-1
window:
    from: "${BASELINE}"
    to: "${OBSERVED}"
tables:
    IntuneAuditLogs:
        status: ok
        reason: ""
        earliest: "2025-12-01T00:00:00Z"
    AuditLogs:
        status: failed
        reason: forbidden
        earliest: ""
counts:
    matched: 2
    noEventInWindow: 1
    noJoinKey: 0
    retentionExceeded: 0
    queryFailed: 0
    notQueried: 0
findings:
    ${T}/a.yaml:
        status: matched
        table: IntuneAuditLogs
        events:
            - at: "2026-01-20T09:00:00Z"
              actor: alice@contoso.com
              actorType: user
              activity: Patch
              result: success
              correlationId: c-1
            - at: "2026-01-10T09:00:00Z"
              actor: bob@contoso.com
              actorType: user
              activity: Patch
              result: failure
              correlationId: c-0
    ${T}/b.yaml:
        status: matched
        table: AuditLogs
        events:
            - at: "2026-01-21T09:00:00Z"
              actor: alice@contoso.com
              actorType: user
              activity: Patch
              result: success
              correlationId: c-2
            - at: "2026-01-11T09:00:00Z"
              actor: alice@contoso.com
              actorType: user
              activity: Patch
              result: success
              correlationId: c-3
    ${T}/c.yaml:
        status: no-event-in-window
        table: IntuneAuditLogs
        reason: nothing found
`;

const obs = parseObservation(OBSERVATION)!;

describe('parseAudit', () => {
  it('parses the contract shape and indexes by key', () => {
    const audit = parseAudit(AUDIT)!;
    expect(audit.version).toBe(1);
    expect(audit.workspaceId).toBe('ws-1');
    expect(audit.window).toEqual({ from: BASELINE, to: OBSERVED });
    expect(audit.tables.map((t) => [t.name, t.failed])).toEqual([
      ['IntuneAuditLogs', false],
      ['AuditLogs', true],
    ]);
    expect(audit.counts.matched).toBe(2);
    expect(audit.byKey.get(`${T}/a`)?.events).toHaveLength(2);
    expect(audit.byKey.get(`${T}/c`)?.status).toBe('no-event-in-window');
  });

  it.each([
    ['not yaml', ': : :\n\t-'],
    ['a list', '- 1'],
    ['no version', AUDIT.replace('version: 1\n', '')],
    ['version 0', AUDIT.replace('version: 1', 'version: 0')],
    ['fractional version', AUDIT.replace('version: 1', 'version: 1.5')],
    ['no observedAt', AUDIT.replace(/observedAt: .*\n/, '')],
    ['no baselineGeneratedAt', AUDIT.replace(/baselineGeneratedAt: .*\n/, '')],
  ])('returns undefined for %s, never throws', (_name, raw) => {
    expect(parseAudit(raw)).toBeUndefined();
  });

  it('drops unsafe keys and unknown statuses', () => {
    const raw = `version: 1
observedAt: "${OBSERVED}"
baselineGeneratedAt: "${BASELINE}"
findings:
    ../x/a.yaml: {status: no-join-key}
    /abs/a.yaml: {status: no-join-key}
    ${T}/a.yml: {status: no-join-key}
    ${T}/a.json: {status: no-join-key}
    ${T}/b.yaml: {status: weird}
    ${T}/ok.yaml: {status: not-queried}
`;
    expect([...parseAudit(raw)!.byKey.keys()]).toEqual([`${T}/ok`]);
  });

  it('drops a matched finding without a well-formed event', () => {
    const raw = `version: 1
observedAt: "${OBSERVED}"
baselineGeneratedAt: "${BASELINE}"
findings:
    ${T}/none.yaml: {status: matched}
    ${T}/empty.yaml: {status: matched, events: []}
    ${T}/bad.yaml: {status: matched, events: [{at: "", actor: x}]}
`;
    expect(parseAudit(raw)!.findings).toEqual([]);
  });

  it('normalises unquoted timestamps and unknown actor types and results', () => {
    const raw = `version: 1
observedAt: ${OBSERVED}
baselineGeneratedAt: ${BASELINE}
findings:
    ${T}/a.yaml:
        status: matched
        events:
            - at: 2026-01-20T09:00:00Z
              actor: x
              actorType: robot
              result: maybe
`;
    const audit = parseAudit(raw)!;
    expect(audit.observedAt).toBe(OBSERVED);
    const [event] = audit.byKey.get(`${T}/a`)!.events;
    expect(event.at).toBe('2026-01-20T09:00:00Z');
    expect(event.actorType).toBe('unknown');
    expect(event.result).toBe('unknown');
  });

  it('sorts events newest first', () => {
    const raw = AUDIT.replace('2026-01-20T09:00:00Z', '2025-01-01T00:00:00Z');
    const events = parseAudit(raw)!.byKey.get(`${T}/a`)!.events;
    expect(events.map((e) => e.correlationId)).toEqual(['c-0', 'c-1']);
  });
});

describe('auditState', () => {
  it('is none, outdated on either timestamp, or current', () => {
    const audit = parseAudit(AUDIT)!;
    expect(auditState(undefined, obs).kind).toBe('none');
    expect(auditState(audit, obs).kind).toBe('current');
    expect(auditState({ ...audit, observedAt: 'x' }, obs).kind).toBe('outdated');
    expect(auditState({ ...audit, baselineGeneratedAt: 'x' }, obs).kind).toBe('outdated');
  });
});

describe('attribution view models', () => {
  const audit = parseAudit(AUDIT)!;
  const finding = (name: string) => obs.byKey.get(`${T}/${name}`)!;

  it('covers every status with a tone and a text', () => {
    for (const status of AUDIT_STATUSES) {
      expect(STATUS_TONE[status]).toBeDefined();
      if (status !== 'matched') expect(STATUS_TEXT[status]).toBeTruthy();
    }
  });

  it('attributes per status and for a finding the file does not name', () => {
    const matched = attributionOf(finding('a'), audit);
    expect(matched.status.matched).toBe(true);
    expect(matched.latest?.actor).toBe('alice@contoso.com');
    expect(matched.more).toBe(1);
    expect(matched.tone.success).toBe(true);

    const none = attributionOf(finding('c'), audit);
    expect(none.text).toBe(STATUS_TEXT['no-event-in-window']);
    expect(none.tone.warning).toBe(true);
    expect(none.quiet).toBe(false);

    const unnamed = attributionOf(finding('d'), audit);
    expect(unnamed.recorded).toBe(false);
    expect(unnamed.text).toBe('no attribution recorded');
  });

  it('shows a row suffix only with an audit', () => {
    const rows = (a?: typeof audit) =>
      findingGroups(obs, 't', a)[0].items.map((i) => i.attribution);
    expect(rows(undefined).every((r) => r === null)).toBe(true);
    expect(rows(audit)[0]).toEqual({ actor: 'alice@contoso.com', at: '2026-01-20T09:00:00Z', more: 1 });
  });

  it('builds Changed by cells that say what the row suffix says', () => {
    const { cells, fingerprint } = changedByCells(obs, audit);
    expect([...cells.keys()].sort()).toEqual(['a', 'b', 'c', 'd'].map((n) => `${T}/${n}`));
    expect(cells.get(`${T}/a`)).toEqual({
      text: 'alice@contoso.com \u00b7 2026-01-20T09:00:00Z (+1 more)',
      tone: 'matched',
    });
    expect(cells.get(`${T}/b`)).toEqual({
      text: 'alice@contoso.com \u00b7 2026-01-21T09:00:00Z (+1 more)',
      tone: 'matched',
    });
    expect(cells.get(`${T}/c`)).toEqual({ text: STATUS_TEXT['no-event-in-window'], tone: 'warning' });
    expect(cells.get(`${T}/d`)).toEqual({ text: 'no attribution recorded', tone: 'quiet' });

    // Same text as the tenant page's row suffix, finding by finding.
    for (const item of findingGroups(obs, 't', audit)[0].items) {
      const row = item.attribution as any;
      const key = `${T}/${LETTER_OF[item.label] ?? 'd'}`;
      const more = row.more ? ` (+${row.more} more)` : '';
      const text = row.actor ? `${row.actor} \u00b7 ${row.at}${more}` : row.text;
      expect(cells.get(key)!.text).toBe(text);
    }

    expect(changedByCells(obs, audit).fingerprint).toBe(fingerprint);
    const changed = parseAudit(AUDIT.replace('c-1', 'c-1x').replace('"2026-01-20T09:00:00Z"', '"2026-01-22T09:00:00Z"'))!;
    expect(changedByCells(obs, changed).fingerprint).not.toBe(fingerprint);
  });

  it('tones quiet statuses quiet and shows an unknown actor', () => {
    const quiet = parseAudit(
      AUDIT.replace(
        `${T}/c.yaml:\n        status: no-event-in-window`,
        `${T}/c.yaml:\n        status: no-join-key`,
      ),
    )!;
    expect(changedByCells(obs, quiet).cells.get(`${T}/c`)!.tone).toBe('quiet');
    const anon = parseAudit(AUDIT.replace('actor: alice@contoso.com\n              actorType: user\n              activity: Patch\n              result: success\n              correlationId: c-1', 'actor: ""\n              actorType: unknown\n              activity: Patch\n              result: success\n              correlationId: c-1'))!;
    expect(changedByCells(obs, anon).cells.get(`${T}/a`)!.text).toMatch(/^unknown actor \u00b7 /);
  });

  it('groups by actor once per finding, sorted, joined to the observation', () => {
    const { actors, window } = byActor(obs, audit, 't');
    expect(window.to).toBe(OBSERVED);
    expect(actors.map((a) => a.actor)).toEqual(['alice@contoso.com', 'bob@contoso.com']);
    // b has two events by alice: one row, at her latest.
    expect(actors[0].findings.map((f) => [f.label, f.at])).toEqual([
      ['Alpha', '2026-01-20T09:00:00Z'],
      ['Beta', '2026-01-21T09:00:00Z'],
    ]);
    expect(actors[1].findings.map((f) => f.label)).toEqual(['Alpha']);
  });

  it('ignores audit entries whose key the observation does not hold', () => {
    const other = parseObservation(OBSERVATION.replace(/ {4}Microsoft.*\/a.yaml:[\s\S]*?Alpha\n/, ''))!;
    const { actors } = byActor(other, audit, 't');
    expect(actors.flatMap((a) => a.findings.map((f) => f.label))).not.toContain('Alpha');
  });

  it('summarises attribution by audit state', () => {
    const audit0 = parseAudit(AUDIT)!;
    expect(observationSummary(obs, 't').attribution).toBeNull();
    expect(observationSummary(obs, 't', { kind: 'outdated', audit: audit0 }).attribution).toEqual({
      outdated: true,
    });
    const current = observationSummary(obs, 't', { kind: 'current', audit: audit0 }).attribution;
    expect(current).toMatchObject({ workspaceId: 'ws-1', matched: 2, noEvent: 1 });
    expect(current?.failedTables?.map((t) => t.name)).toEqual(['AuditLogs']);
  });

  describe('events that name no actor', () => {
    const event = (at: string, actor: string, id: string) =>
      `            - at: "${at}"\n              actor: "${actor}"\n              actorType: ${actor ? 'user' : 'unknown'}\n              activity: Patch\n              result: success\n              correlationId: ${id}\n`;
    const auditWith = (events: string) =>
      parseAudit(`version: 1
observedAt: "${OBSERVED}"
baselineGeneratedAt: "${BASELINE}"
findings:
    ${T}/a.yaml:
        status: matched
        events:
${events}`)!;

    it('keeps a matched finding whose only event has no actor', () => {
      const audit = auditWith(event('2026-01-20T09:00:00Z', '', 'x'));
      expect(audit.byKey.get(`${T}/a`)?.events).toHaveLength(1);
      const row = findingGroups(obs, 't', audit)[0].items[0].attribution;
      expect(row).toEqual({ actor: 'unknown actor', at: '2026-01-20T09:00:00Z', more: 0 });
      const { actors } = byActor(obs, audit, 't');
      expect(actors).toHaveLength(1);
      expect(actors[0]).toMatchObject({ actor: 'unknown actor', actorType: 'unknown', tagged: true });
      expect(actors[0].findings.map((f) => f.label)).toEqual(['Alpha']);
    });

    it('counts an actor-less event between two named ones in +N more', () => {
      const audit = auditWith(
        event('2026-01-22T09:00:00Z', 'alice@contoso.com', '1') +
          event('2026-01-21T09:00:00Z', '', '2') +
          event('2026-01-20T09:00:00Z', 'bob@contoso.com', '3'),
      );
      const a = attributionOf(obs.byKey.get(`${T}/a`)!, audit);
      expect(a.more).toBe(2);
      expect(a.events.map((e) => e.actor)).toEqual(['alice@contoso.com', 'unknown actor', 'bob@contoso.com']);
    });
  });
});

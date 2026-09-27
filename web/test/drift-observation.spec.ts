import {
  driftState,
  parseBaselineGeneratedAt,
  parseObservation,
  tenantDriftState,
  typeOfKey,
} from '../src/docs/drift-observation';
import { parseTenantIndex, TenantIndex } from '../src/docs/tenant-index';

const BASELINE = '2026-01-01T00:00:00Z';

const OBSERVATION = `observedAt: "2026-02-01T10:00:00Z"
tenant: contoso.com
toolVersion: azure-rd v1
baseline:
    generatedAt: "${BASELINE}"
    toolVersion: azure-rd v0
run:
    complete: false
    incompleteReason: 1 resource types could not be listed
counts:
    compared: 4
    unchanged: 2
    changed: 1
    renamed: 1
    added: 1
    removed: 0
    unattested: 1
unknownTypes:
    - Microsoft.Graph/organizationalBranding
removalsSuppressed: true
notComparable:
    - key: Microsoft.Graph/deviceConfigurations/unattested.yaml
      reason: baseline entry predates per-entry config attestation
findings:
    Microsoft.Graph/deviceConfigurations/changed.yaml:
        verdict: changed
        displayName: Changed
        baselineKey: Microsoft.Graph/deviceConfigurations/changed.yaml
        baselineSha256: aa
        payloadSha256: bb
        deltas:
            - path: settings.enabled
              old: "true"
              new: "false"
            - path: count
              old: 1
              new: (absent)
    Microsoft.Graph/deviceConfigurations/new_name.yaml:
        verdict: renamed
        displayName: New name
        previousDisplayName: Old name
        baselineKey: Microsoft.Graph/deviceConfigurations/old_name.yaml
    Microsoft.Graph/namedLocations/added.yaml:
        verdict: added
        displayName: Added
    Microsoft.Graph/deviceConfigurations/weird.yaml:
        verdict: exploded
    ../escape.yaml:
        verdict: changed
payloads:
    - Microsoft.Graph/deviceConfigurations/changed.yaml
    - Microsoft.Graph/namedLocations/added.yaml
`;

const INDEX = parseTenantIndex(`version: 3
tenant: contoso.com
generatedAt: "${BASELINE}"
resources:
    - type: Microsoft.Graph/deviceConfigurations
      doc: Microsoft.Graph/deviceConfigurations/changed.md
    - type: Microsoft.Graph/deviceConfigurations
      doc: Microsoft.Graph/deviceConfigurations/same.md
    - type: Microsoft.Graph/deviceConfigurations
      doc: Microsoft.Graph/deviceConfigurations/old_name.md
    - type: Microsoft.Graph/deviceConfigurations
      doc: Microsoft.Graph/deviceConfigurations/unattested.md
    - type: Microsoft.Graph/organizationalBranding
      doc: Microsoft.Graph/organizationalBranding/brand.md
`) as TenantIndex;

describe('parseObservation', () => {
  it('reads the observation and indexes findings by both keys', () => {
    const obs = parseObservation(OBSERVATION);
    expect(obs).toBeDefined();
    if (!obs) return;
    expect(obs.observedAt).toBe('2026-02-01T10:00:00Z');
    expect(obs.baselineGeneratedAt).toBe(BASELINE);
    expect(obs.complete).toBe(false);
    expect(obs.removalsSuppressed).toBe(true);
    expect(obs.counts.changed).toBe(1);
    // Keys are extensionless route paths, sorted.
    expect(obs.findings.map((f) => f.key)).toEqual([
      'Microsoft.Graph/deviceConfigurations/changed',
      'Microsoft.Graph/deviceConfigurations/new_name',
      'Microsoft.Graph/namedLocations/added',
    ]);
    expect(
      obs.byBaselineKey.get('Microsoft.Graph/deviceConfigurations/old_name')?.key,
    ).toBe('Microsoft.Graph/deviceConfigurations/new_name');
    // An addition has no baseline key, so it is not indexed under ''.
    expect(obs.byBaselineKey.has('')).toBe(false);
    expect(obs.payloads.has('Microsoft.Graph/namedLocations/added')).toBe(true);
    expect(
      obs.notComparableByKey.get('Microsoft.Graph/deviceConfigurations/unattested'),
    ).toContain('attestation');
  });

  it('keeps delta values as text, whatever YAML type they loaded as', () => {
    const obs = parseObservation(OBSERVATION);
    const deltas = obs?.byKey.get('Microsoft.Graph/deviceConfigurations/changed')?.deltas;
    expect(deltas).toEqual([
      { path: 'settings.enabled', old: 'true', new: 'false' },
      { path: 'count', old: '1', new: '(absent)' },
    ]);
  });

  it('drops findings with an unknown verdict or an unsafe key', () => {
    const obs = parseObservation(OBSERVATION);
    expect(obs?.byKey.has('Microsoft.Graph/deviceConfigurations/weird')).toBe(false);
    expect(obs?.findings.some((f) => f.key.includes('..'))).toBe(false);
  });

  it('accepts an unquoted timestamp, which js-yaml loads as a Date', () => {
    const obs = parseObservation(
      'observedAt: 2026-02-01T10:00:00Z\nbaseline:\n  generatedAt: 2026-01-01T00:00:00Z\n',
    );
    expect(obs?.observedAt).toBe('2026-02-01T10:00:00Z');
    expect(obs?.baselineGeneratedAt).toBe(BASELINE);
    expect(obs?.findings).toEqual([]);
  });

  it('returns undefined for anything that is not an observation, never throwing', () => {
    expect(parseObservation('')).toBeUndefined();
    expect(parseObservation('observedAt: [oops\n')).toBeUndefined();
    expect(parseObservation('- a\n- b\n')).toBeUndefined();
    // The baseline is what the validity gate needs: without it, no observation.
    expect(parseObservation('observedAt: "x"\n')).toBeUndefined();
    expect(
      parseObservation('observedAt: "x"\nbaseline:\n  generatedAt: "y"\nfindings: [a]\n'),
    ).toBeUndefined();
  });
});

describe('parseBaselineGeneratedAt', () => {
  it('reads the top-level key only, quoted or not', () => {
    expect(
      parseBaselineGeneratedAt(
        `tenant: x\ngeneratedAt: "${BASELINE}"\nresources:\n  a:\n    generatedAt: "nested"\n`,
      ),
    ).toBe(BASELINE);
    expect(parseBaselineGeneratedAt(`generatedAt: ${BASELINE}\r\n`)).toBe(BASELINE);
  });

  it('ignores an indented key and reports a missing one as undefined', () => {
    expect(parseBaselineGeneratedAt('run:\n  generatedAt: "x"\n')).toBeUndefined();
    expect(parseBaselineGeneratedAt('generatedAt: ""\n')).toBeUndefined();
  });
});

describe('tenantDriftState', () => {
  const obs = parseObservation(OBSERVATION);

  it('is none without an observation', () => {
    expect(tenantDriftState(undefined, BASELINE).kind).toBe('none');
  });

  it('is superseded when the export baseline moved, or cannot be determined', () => {
    expect(tenantDriftState(obs, '2026-03-03T00:00:00Z').kind).toBe('superseded');
    expect(tenantDriftState(obs, undefined).kind).toBe('superseded');
  });

  it('is current against the baseline it names', () => {
    expect(tenantDriftState(obs, BASELINE).kind).toBe('current');
  });
});

describe('driftState', () => {
  const obs = parseObservation(OBSERVATION);
  const state = (key: string, baseline = BASELINE) =>
    driftState(obs, baseline, INDEX, key);

  it('is none without an observation, whatever the key', () => {
    expect(driftState(undefined, BASELINE, INDEX, 'anything').kind).toBe('none');
  });

  it('checks superseded before consulting any finding', () => {
    expect(
      state('Microsoft.Graph/deviceConfigurations/changed', 'other').kind,
    ).toBe('superseded');
  });

  it('finds a finding by its key', () => {
    const s = state('Microsoft.Graph/deviceConfigurations/changed');
    expect(s.kind).toBe('finding');
    if (s.kind === 'finding') expect(s.finding.verdict).toBe('changed');
  });

  it("finds a rename from its old name's page, by baselineKey", () => {
    const s = state('Microsoft.Graph/deviceConfigurations/old_name');
    expect(s.kind).toBe('finding');
    if (s.kind === 'finding') {
      expect(s.finding.key).toBe('Microsoft.Graph/deviceConfigurations/new_name');
    }
  });

  it('finds an addition the index does not list', () => {
    expect(state('Microsoft.Graph/namedLocations/added').kind).toBe('finding');
  });

  it('reports a type the run could not list', () => {
    const s = state('Microsoft.Graph/organizationalBranding/brand');
    expect(s.kind).toBe('unknownType');
    if (s.kind === 'unknownType') {
      expect(s.type).toBe('Microsoft.Graph/organizationalBranding');
    }
  });

  it('reports a baseline entry that could not be compared', () => {
    const s = state('Microsoft.Graph/deviceConfigurations/unattested');
    expect(s.kind).toBe('notComparable');
    if (s.kind === 'notComparable') expect(s.reason).toContain('attestation');
  });

  it('is unchanged for an indexed resource without a finding', () => {
    expect(state('Microsoft.Graph/deviceConfigurations/same').kind).toBe('unchanged');
  });

  it('is unknown for anything else', () => {
    expect(state('Microsoft.Graph/deviceConfigurations/nope').kind).toBe('unknown');
    expect(state('metadata').kind).toBe('unknown');
  });
});

describe('typeOfKey', () => {
  it('drops the last segment', () => {
    expect(typeOfKey('Microsoft.Graph/groups/g1')).toBe('Microsoft.Graph/groups');
    expect(typeOfKey('metadata')).toBe('');
  });
});

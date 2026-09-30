import type { Content, TDocumentDefinitions } from 'pdfmake/interfaces';
import { auditState, AuditState, parseAudit } from '../src/docs/drift-audit';
import { DriftFinding, parseObservation } from '../src/docs/drift-observation';
import { FindingReport, TenantDriftReport } from '../src/docs/drift-report.service';
import {
  attributionOf,
  byActor,
  findingGroups,
  findingHeader,
  observationSummary,
} from '../src/docs/drift-view';
import { driftPdfDefinition, DriftPdfInput } from '../src/docs/export/drift-pdf';
import {
  CODE_FONT,
  findingAnchor,
  htmlToPdfContent,
  internalDestination,
  isWinAnsi,
  PdfContentOptions,
} from '../src/docs/export/pdf-content';

// The drift report PDF as data: the HTML walker's verdicts and the document
// definition built from a report model. The bytes themselves (determinism, the
// route, the headers) are covered end to end in docs.e2e.spec.ts.

const BASELINE = '2026-01-01T00:00:00Z';
const OBSERVED = '2026-02-01T10:00:00Z';
const T = 'Microsoft.Graph/deviceConfigurations';
const L = 'Microsoft.Graph/namedLocations';

const OBSERVATION = `observedAt: "${OBSERVED}"
toolVersion: azure-rd v2
baseline:
    generatedAt: "${BASELINE}"
    toolVersion: azure-rd v1
run:
    complete: false
    incompleteReason: 1 resource types could not be listed
counts:
    compared: 5
    unchanged: 1
    changed: 2
    added: 1
unknownTypes:
    - Microsoft.Graph/organizationalBranding
notComparable:
    - key: ${T}/old.yaml
      reason: predates attestation
findings:
    ${T}/a.yaml:
        verdict: changed
        displayName: Alpha
        baselineKey: ${T}/a.yaml
        deltas:
            - path: settings.enabled
              old: "true"
              new: "false"
        deltaNote: 2 more changes not shown
    ${T}/b.yaml:
        verdict: changed
        displayName: Политика доступа
        baselineKey: ${T}/b.yaml
    ${L}/c.yaml:
        verdict: added
        displayName: Kali VPN location
`;

const AUDIT = `version: 1
observedAt: "${OBSERVED}"
baselineGeneratedAt: "${BASELINE}"
workspaceId: ws-1
window:
    from: "${BASELINE}"
    to: "${OBSERVED}"
counts:
    matched: 1
    noEventInWindow: 1
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
              correlationId: corr-1
    ${T}/b.yaml:
        status: no-event-in-window
        table: IntuneAuditLogs
        reason: nothing between the timestamps
`;

const obs = parseObservation(OBSERVATION)!;
const audit = parseAudit(AUDIT)!;

function findingReport(
  finding: DriftFinding,
  state: AuditState,
  overrides: Partial<FindingReport> = {},
): FindingReport {
  return {
    finding: findingHeader(finding, undefined),
    attribution: attributionOf(finding, state.kind === 'current' ? state.audit : undefined),
    attributionShown: state.kind === 'current',
    attributionOutdated: state.kind === 'outdated',
    intact: true,
    deltas: finding.deltas,
    deltaNote: finding.deltaNote,
    analysis: null,
    ...overrides,
  };
}

function input(state: AuditState = auditState(audit, obs)): DriftPdfInput {
  const active = state.kind === 'current' ? state.audit : undefined;
  const report: TenantDriftReport = {
    state: { kind: 'current', observation: obs },
    audit: state,
    observation: observationSummary(obs, 't', state),
    groups: findingGroups(obs, 't', active),
    actors: active ? byActor(obs, active, 't') : null,
    analysis: {
      heading: '<h1 id="s"><a class="header-anchor" href="#s">Drift analysis summary</a></h1>',
      body: `<p>See <a href="/t/_drift/${T}/a">Alpha</a>.</p>`,
    },
  };
  const alpha = obs.byKey.get(`${T}/a`)!;
  const cyrillic = obs.byKey.get(`${T}/b`)!;
  const added = obs.byKey.get(`${L}/c`)!;
  const findings = new Map<string, FindingReport>([
    [
      alpha.key,
      findingReport(alpha, state, {
        finding: findingHeader(alpha, 'high'),
        analysis: '<h1>Drift: Alpha</h1><p>Tightened.</p>',
      }),
    ],
    // The files it was decided on have changed since the run.
    [cyrillic.key, findingReport(cyrillic, state, { intact: false })],
    [added.key, findingReport(added, state)],
  ]);
  return { tenantId: 't', tenantName: 'Contoso', observation: obs, report, findings };
}

// Every text of a content tree, one line per node, runs of a node joined.
function texts(node: unknown): string[] {
  if (typeof node === 'string') return [node];
  if (Array.isArray(node)) return node.flatMap(texts);
  if (!node || typeof node !== 'object') return [];
  const rec = node as Record<string, unknown>;
  if ('text' in rec) {
    const t = rec.text;
    return [Array.isArray(t) ? t.map((r) => texts(r).join('')).join('') : texts(t).join('')];
  }
  return Object.values(rec).flatMap(texts);
}

// Every object node of a content tree, for property checks.
function nodes(node: unknown): Array<Record<string, unknown>> {
  if (Array.isArray(node)) return node.flatMap(nodes);
  if (!node || typeof node !== 'object' || node instanceof Date) return [];
  const rec = node as Record<string, unknown>;
  return [rec, ...Object.values(rec).flatMap(nodes)];
}

// Text nodes (not runs) carrying the given style, in document order.
function styled(content: unknown, style: string): string[] {
  return nodes(content)
    .filter((n) => n.style === style && 'text' in n)
    .map((n) => texts(n).join(''));
}

const OPTIONS: PdfContentOptions = { tenant: 't', findings: new Set([`${T}/a`]) };

describe('pdf-content: rendered HTML to pdfmake content', () => {
  it('keeps headings, paragraphs, lists and tables', () => {
    const content = htmlToPdfContent(
      '<h2>Head</h2>\n<p>One <strong>two</strong> <em>three</em></p>\n<ul><li>x</li><li>y</li></ul>\n<table><thead><tr><th>A</th><th>B</th></tr></thead><tbody><tr><td>1</td><td>2</td></tr></tbody></table>',
      OPTIONS,
    );
    expect(content[0]).toEqual({ text: [{ text: 'Head' }], style: 'h2' });
    expect(content[1]).toEqual({
      text: [{ text: 'One ' }, { text: 'two', bold: true }, { text: ' ' }, { text: 'three', italics: true }],
      style: 'p',
    });
    expect(content[2]).toMatchObject({ ul: [{ text: [{ text: 'x' }] }, { text: [{ text: 'y' }] }] });
    const table = (content[3] as any).table;
    expect(table.headerRows).toBe(1);
    expect(table.body).toHaveLength(2);
    expect(table.body[0][0]).toMatchObject({ style: 'tableHeader' });
    expect(texts(table.body[1])).toEqual(['1', '2']);
  });

  it('moves headings down by the shift, clamped to h6', () => {
    const content = htmlToPdfContent('<h1>A</h1><h5>B</h5>', { ...OPTIONS, headingShift: 3 });
    expect(content.map((c) => (c as any).style)).toEqual(['h4', 'h6']);
  });

  it('always expands <details>, its summary a bold lead line', () => {
    const [details] = htmlToPdfContent(
      '<details><summary>More</summary>\n<p>Hidden <code>value</code></p></details>',
      OPTIONS,
    );
    expect(details).toMatchObject({ style: 'details' });
    const stack = (details as any).stack;
    expect(stack[0]).toEqual({ text: [{ text: 'More' }], style: 'summary' });
    expect(texts(stack[1])).toEqual(['Hidden value']);
  });

  it('links a finding of the report internally and every other link as plain text', () => {
    const runs = (html: string) => (htmlToPdfContent(`<p>${html}</p>`, OPTIONS)[0] as any).text;
    expect(runs(`<a href="/t/_drift/${T}/a">A</a>`)[0]).toMatchObject({
      text: 'A',
      linkToDestination: findingAnchor(`${T}/a`),
    });
    for (const href of [
      `/t/_drift/${T}/a?diff`,
      `/t/_drift/${T}/a?yaml`,
      `/t/_drift/${T}/a?raw`,
      `/t/_drift/${T}/other`,
      `/other/_drift/${T}/a`,
      `/t/${T}/a`,
      'https://example.com/',
      '#heading',
    ]) {
      const [run] = runs(`<a href="${href}">A</a>`);
      expect(run).toEqual({ text: 'A' });
    }
    // A fragment does not stop a finding link from being one.
    expect(internalDestination(`/t/_drift/${T}/a#x`, OPTIONS)).toBe(findingAnchor(`${T}/a`));
  });

  it('turns images into their alt text, unwraps unknown elements and drops scripts', () => {
    const content = htmlToPdfContent(
      '<p>Logo: <img src="https://example.com/x.png" alt="the logo"></p><foo>kept <b>bold</b></foo><script>evil()</script><p>a<style>p{}</style>b</p>',
      OPTIONS,
    );
    expect(texts(content)).toEqual(['Logo: the logo', 'kept bold', 'ab']);
    expect(JSON.stringify(content)).not.toContain('example.com');
    expect(JSON.stringify(content)).not.toContain('evil');
  });

  it('sets code in Courier only when WinAnsi can encode it', () => {
    const [p] = htmlToPdfContent('<p><code>abc – ä</code> <code>Привет</code></p>', OPTIONS);
    const [latin, , cyrillic] = (p as any).text;
    expect(latin).toMatchObject({ text: 'abc – ä', font: CODE_FONT });
    expect(cyrillic.text).toBe('Привет');
    expect(cyrillic.font).toBeUndefined();
    const [pre] = htmlToPdfContent('<pre><code>line 1\n  line 2\n</code></pre>', OPTIONS);
    expect(pre).toMatchObject({ text: 'line 1\n  line 2', font: CODE_FONT, preserveLeadingSpaces: true });
    const [cjk] = htmlToPdfContent('<pre><code>設定\n</code></pre>', OPTIONS);
    expect((cjk as any).font).toBeUndefined();
    expect(isWinAnsi('€ “quoted”')).toBe(true);
    expect(isWinAnsi('→')).toBe(false);
  });
});

describe('drift-pdf: the document definition', () => {
  const definition = driftPdfDefinition(input());
  const content = definition.content as Content[];
  const all = texts(content);

  it('opens with a cover naming the tenant, the observation and both tool versions', () => {
    expect(all.slice(0, 2)).toEqual(['Drift report', 'Contoso']);
    for (const value of [OBSERVED, BASELINE, 'azure-rd v2', 'azure-rd v1']) {
      expect(all).toContain(value);
    }
    expect(definition.info?.title).toBe('Contoso drift report');
  });

  it('carries the observation header: counts and every caveat', () => {
    expect(all).toContain('5 compared · 1 unchanged · 2 changed · 0 renamed · 1 added · 0 removed');
    expect(all).toContain(
      'Incomplete run: 1 resource types could not be listed. What was not observed is unknown, not unchanged.',
    );
    expect(all).toContain('Microsoft.Graph/organizationalBranding');
    expect(all).toContain(`${T}/old — predates attestation`);
    expect(all.some((t) => t.startsWith('Attribution from workspace ws-1'))).toBe(true);
  });

  it('carries the analysis summary with its own heading', () => {
    expect(styled(content, 'h1')).toContain('Drift analysis summary');
  });

  it('groups the findings by type, in the page order', () => {
    expect(styled(content, 'h2')).toEqual([`${T} (2)`, `${L} (1)`]);
    expect(styled(content, 'h3')).toEqual(['Alpha', 'Политика доступа', 'Kali VPN location']);
  });

  it('anchors every finding section for the internal links', () => {
    const anchors = nodes(content)
      .filter((n) => typeof n.id === 'string')
      .map((n) => n.id);
    expect(anchors).toEqual([`${T}/a`, `${T}/b`, `${L}/c`].map(findingAnchor));
    // The analysis summary's link to Alpha jumps to Alpha's section.
    expect(nodes(content).some((n) => n.linkToDestination === findingAnchor(`${T}/a`))).toBe(true);
  });

  it('carries the deltas and the not-intact warning', () => {
    expect(all).toContain('settings.enabled');
    expect(all).toContain('2 more changes not shown');
    expect(all.filter((t) => t.startsWith('The files on disk no longer match this observation'))).toHaveLength(1);
  });

  it('shows each attribution state: events, the status line, the outdated caveat', () => {
    expect(all).toContain('alice@contoso.com');
    expect(all).toContain('corr-1');
    expect(all).toContain('Attribution: no audit event in the window. nothing between the timestamps');
    expect(styled(content, 'h1')).toContain('By actor');

    const outdated = texts(driftPdfDefinition(input({ kind: 'outdated', audit })).content);
    const caveat = 'Attribution outdated: it predates this observation. Run azure-rd resource audit again.';
    // Once in the observation header, once per finding.
    expect(outdated.filter((t) => t === caveat)).toHaveLength(1 + obs.findings.length);
    expect(outdated).not.toContain('alice@contoso.com');
    expect(outdated).not.toContain('By actor');
  });

  it('says "No analysis" for a finding without one, and renders the one that exists', () => {
    expect(all.filter((t) => t === 'No analysis')).toHaveLength(2);
    expect(all).toContain('Tightened.');
    // The analysis's own H1 nests under the finding's heading.
    expect(styled(content, 'h4')).toContain('Drift: Alpha');
    expect(all).toContain('Verdict: changed · Severity: high');
  });

  it('keeps a Cyrillic display name as text', () => {
    expect(all).toContain('Политика доступа');
  });

  it('dates the file by the observation, never the clock', () => {
    expect(definition.info?.creationDate).toEqual(new Date(OBSERVED));
    expect(definition.info?.modDate).toBeUndefined();
    const broken = driftPdfDefinition({
      ...input(),
      observation: { ...obs, observedAt: 'not a date' },
    });
    expect(broken.info?.creationDate).toEqual(new Date(0));
  });

  it('carries no image, svg or URL anywhere', () => {
    const def = definition as TDocumentDefinitions & Record<string, unknown>;
    expect(def.images).toBeUndefined();
    for (const n of nodes(def)) {
      expect(n).not.toHaveProperty('image');
      expect(n).not.toHaveProperty('svg');
      expect(n).not.toHaveProperty('link');
      expect(n).not.toHaveProperty('url');
    }
    expect(JSON.stringify(def.content)).not.toMatch(/https?:/);
  });

  it('puts tenant, observation time and page n/m in the footer', () => {
    const footer = definition.footer as (current: number, count: number) => Content;
    expect(texts(footer(2, 7))).toEqual([`Contoso · observed ${OBSERVED} · page 2/7`]);
  });
});

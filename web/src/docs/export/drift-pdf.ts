import type {
  Content,
  StyleDictionary,
  TDocumentDefinitions,
} from 'pdfmake/interfaces';
import { DriftObservation } from '../drift-observation';
import { UNKNOWN_ACTOR } from '../drift-view';
import type { FindingReport, TenantDriftReport } from '../drift-report.service';
import { CODE_FONT, findingAnchor, htmlToPdfContent, isWinAnsi, PdfContentOptions } from './pdf-content';

// The drift report as a `pdfmake` document definition, built from the same
// report model the tenant drift page and the finding pages render: the cover,
// the observation header, the analysis summary, every finding grouped by type
// with its attribution, deltas and analysis, and the By actor blocks.
//
// Pure and Nest-free: the service gathers the model, this lays it out. No YAML
// payloads and no line diffs — the same line the Confluence export draws at the
// source YAML. Deterministic: the creation date is the observation's own time
// and nothing else in the definition depends on the wall clock, so the same
// observation yields the same bytes.

export interface DriftPdfInput {
  tenantId: string;
  tenantName: string;
  // The current observation the report describes.
  observation: DriftObservation;
  report: TenantDriftReport;
  // Every finding's report, by key. A key without one renders as a finding
  // with no attribution, comparison or analysis to show.
  findings: ReadonlyMap<string, FindingReport>;
}

// The text column at the report's margins, for the rules drawn under headings.
const PAGE_MARGINS: [number, number, number, number] = [40, 40, 40, 50];

const MISMATCH =
  'The files on disk no longer match this observation. The baseline or the observed payload is ' +
  'missing or has changed since the drift run, so the comparison is not shown. Re-run ' +
  'azure-rd resource drift.';

const ATTRIBUTION_OUTDATED =
  'Attribution outdated: it predates this observation. Run azure-rd resource audit again.';

const NO_FINDINGS =
  'The observation recorded no findings: everything it compared matched the baseline.';

const STYLES: StyleDictionary = {
  title: { fontSize: 26, bold: true, margin: [0, 160, 0, 8] },
  subtitle: { fontSize: 16, margin: [0, 0, 0, 24] },
  h1: { fontSize: 18, bold: true, margin: [0, 12, 0, 6] },
  h2: { fontSize: 14, bold: true, margin: [0, 10, 0, 4] },
  h3: { fontSize: 12.5, bold: true, margin: [0, 10, 0, 3] },
  h4: { fontSize: 11, bold: true, margin: [0, 6, 0, 3] },
  h5: { fontSize: 10.5, bold: true, margin: [0, 4, 0, 2] },
  h6: { fontSize: 10, bold: true, italics: true, margin: [0, 4, 0, 2] },
  p: { margin: [0, 0, 0, 6] },
  summary: { bold: true, margin: [0, 2, 0, 4] },
  details: { margin: [0, 0, 0, 4] },
  list: { margin: [0, 0, 0, 6] },
  table: { fontSize: 8.5, margin: [0, 2, 0, 8] },
  tableHeader: { bold: true },
  pre: { fontSize: 8, color: '#334155', margin: [0, 2, 0, 8] },
  code: { fontSize: 9 },
  blockquote: { italics: true, color: '#475569', margin: [12, 0, 0, 6] },
  meta: { fontSize: 9, color: '#475569', margin: [0, 0, 0, 3] },
  caveat: { fontSize: 9, color: '#92400e', margin: [0, 2, 0, 4] },
  warning: { fontSize: 9.5, color: '#991b1b', margin: [0, 4, 0, 8] },
  none: { italics: true, color: '#64748b', margin: [0, 2, 0, 6] },
  footer: { fontSize: 8, color: '#64748b' },
};

// The document definition of one tenant's current drift report.
export function driftPdfDefinition(input: DriftPdfInput): TDocumentDefinitions {
  const { observation, tenantName } = input;
  const keys = new Set(observation.findings.map((f) => f.key));
  const links: PdfContentOptions = { tenant: input.tenantId, findings: keys };
  return {
    pageSize: 'A4',
    pageMargins: PAGE_MARGINS,
    info: {
      title: `${tenantName} drift report`,
      // pdfkit derives the file ID from this date: the observation's own time
      // keeps the bytes stable across downloads.
      creationDate: creationDate(observation.observedAt),
    },
    defaultStyle: { font: 'Roboto', fontSize: 10, lineHeight: 1.2 },
    styles: STYLES,
    footer: (currentPage: number, pageCount: number) => ({
      text: `${tenantName} · observed ${observation.observedAt} · page ${currentPage}/${pageCount}`,
      style: 'footer',
      margin: [PAGE_MARGINS[0], 16, PAGE_MARGINS[2], 0],
    }),
    content: [
      cover(input),
      ...observationBlock(input),
      ...analysisBlock(input, links),
      ...findingsBlock(input, links),
      ...actorsBlock(input),
    ],
  };
}

// A `Date` for the observation time, or the Unix epoch when it does not parse,
// so a malformed timestamp never falls back to the wall clock.
export function creationDate(observedAt: string): Date {
  const parsed = new Date(observedAt);
  return Number.isNaN(parsed.getTime()) ? new Date(0) : parsed;
}

function cover(input: DriftPdfInput): Content {
  const o = input.observation;
  const facts: Array<[string, string]> = [
    ['Tenant', input.tenantId],
    ['Observed', o.observedAt],
    ['Baseline generated', o.baselineGeneratedAt],
    ['Observation tool version', o.toolVersion || '—'],
    ['Baseline tool version', o.baselineToolVersion || '—'],
    ['Findings', String(o.findings.length)],
  ];
  return {
    stack: [
      { text: 'Drift report', style: 'title' },
      { text: input.tenantName, style: 'subtitle' },
      {
        table: {
          widths: ['auto', '*'],
          body: facts.map(([label, value]) => [
            { text: label, bold: true },
            { text: value },
          ]),
        },
        layout: 'noBorders',
      },
    ],
    pageBreak: 'after',
  };
}

// `: reason` (or the given separator) when there is a reason, else nothing.
function suffix(reason: string | undefined | null, sep = ': '): string {
  return reason ? `${sep}${reason}` : '';
}

function attributionSummary(a: NonNullable<DriftPdfInput['report']['observation']>['attribution']): Content[] {
  if (a?.outdated) return [{ text: ATTRIBUTION_OUTDATED, style: 'caveat' }];
  if (!a) return [];
  const line = [
    `Attribution from workspace ${a.workspaceId}, window ${a.window?.from} to ${a.window?.to}`,
    `${a.matched} matched`,
    `${a.noEvent} no event`,
    `${a.retention} beyond retention`,
    `${a.noJoinKey} no join key`,
    `${a.failed} failed`,
    `${a.notQueried} not queried`,
  ];
  const out: Content[] = [{ text: line.join(' · '), style: 'meta' }];
  for (const t of a.failedTables ?? []) {
    out.push({ text: `${t.name} failed${suffix(t.reason)}`, style: 'caveat' });
  }
  return out;
}

// The observation header, as `partials/drift-observation.hbs` shows it.
function observationBlock(input: DriftPdfInput): Content[] {
  const s = input.report.observation;
  if (!s) return [];
  const out: Content[] = [{ text: 'Observation', style: 'h1' }];
  out.push({
    text: [
      'Observed ',
      { text: s.observedAt, bold: true },
      ' against the baseline of ',
      { text: s.baselineGeneratedAt, bold: true },
      s.toolVersion ? ` · ${s.toolVersion}` : '',
    ],
    style: 'p',
  });
  const counts = [
    `${s.compared} compared`,
    `${s.unchanged} unchanged`,
    ...s.verdicts.map((v) => `${v.count} ${v.label}`),
  ];
  if (s.unattested) counts.push(`${s.unattested} not comparable`);
  if (s.excluded) counts.push(`${s.excluded} excluded`);
  if (s.failed) counts.push(`${s.failed} failed`);
  out.push({ text: counts.join(' · '), style: 'meta' });

  out.push(...attributionSummary(s.attribution));
  if (!s.complete) {
    out.push({
      text: [
        { text: 'Incomplete run', bold: true },
        `${suffix(s.incompleteReason)}. What was not observed is unknown, not unchanged.`,
      ],
      style: 'caveat',
    });
  }
  if (s.removalsSuppressed) {
    out.push({
      text: 'Removals were suppressed: a scoped or incomplete run cannot tell a deleted resource from one it did not look at.',
      style: 'meta',
    });
  }
  if (s.unknownTypes.length > 0) {
    out.push({ text: `Types that could not be listed (${s.unknownTypes.length})`, style: 'summary' });
    out.push({ ul: s.unknownTypes.map((t) => code(t)), style: 'list' });
  }
  if (s.notComparable.length > 0) {
    out.push({
      text: `Baseline entries that could not be compared (${s.notComparable.length})`,
      style: 'summary',
    });
    out.push({
      ul: s.notComparable.map((n) => ({ text: [code(n.key), ` — ${n.reason}`] })),
      style: 'list',
    });
  }
  if (s.empty) out.push({ text: NO_FINDINGS, style: 'p' });
  return out;
}

// The analysis summary (drift/index.md) with its own H1, or a line saying the
// analysis has not been written.
function analysisBlock(input: DriftPdfInput, links: PdfContentOptions): Content[] {
  const analysis = input.report.analysis;
  if (!analysis) {
    return [
      { text: 'Analysis summary', style: 'h1' },
      { text: 'No analysis summary', style: 'none' },
    ];
  }
  return htmlToPdfContent(`${analysis.heading}${analysis.body}`, links);
}

function findingsBlock(input: DriftPdfInput, links: PdfContentOptions): Content[] {
  if (input.report.groups.length === 0) return [];
  const out: Content[] = [{ text: 'Findings', style: 'h1', pageBreak: 'before' }];
  for (const group of input.report.groups) {
    out.push({ text: `${group.type} (${group.items.length})`, style: 'h2' });
    for (const item of group.items) {
      out.push(...findingSection(item.key, item.label, input.findings.get(item.key), links));
    }
  }
  return out;
}

// One finding: its heading (the target of every internal link to it), verdict
// and severity, attribution, comparison and analysis.
function findingSection(
  key: string,
  label: string,
  report: FindingReport | undefined,
  links: PdfContentOptions,
): Content[] {
  const out: Content[] = [{ text: report?.finding.label || label, style: 'h3', id: findingAnchor(key) }];
  const facts: Content[] = [];
  if (report) {
    facts.push({ text: [{ text: 'Verdict: ', bold: true }, report.finding.verdictBadge.label] });
    if (report.finding.severityBadge) {
      facts.push({ text: [{ text: ' · Severity: ', bold: true }, report.finding.severityBadge.label] });
    }
    if (report.finding.previous) facts.push({ text: ` · was ${report.finding.previous}` });
  }
  if (facts.length > 0) out.push({ text: facts, style: 'meta' });
  out.push({ text: [code(key)], style: 'meta' });
  if (!report) {
    out.push({ text: 'No analysis', style: 'none' });
    return out;
  }

  out.push(...attributionBlock(report));
  if (report.intact) {
    if (report.deltas.length > 0) {
      out.push({ text: 'What changed', style: 'h4' });
      out.push({
        table: {
          headerRows: 1,
          widths: ['*', '*', '*'],
          body: [
            [
              { text: 'Field', style: 'tableHeader' },
              { text: 'Baseline', style: 'tableHeader' },
              { text: 'Observed', style: 'tableHeader' },
            ],
            ...report.deltas.map((d) => [code(d.path), code(d.old), code(d.new)]),
          ],
        },
        layout: 'lightHorizontalLines',
        style: 'table',
      });
    }
    if (report.deltaNote) out.push({ text: report.deltaNote, style: 'meta' });
  } else {
    out.push({ text: MISMATCH, style: 'warning' });
  }

  if (report.analysis) {
    out.push(...htmlToPdfContent(report.analysis, { ...links, headingShift: 3 }));
  } else {
    out.push({ text: 'No analysis', style: 'none' });
  }
  return out;
}

function attributionBlock(report: FindingReport): Content[] {
  if (report.attributionOutdated) return [{ text: ATTRIBUTION_OUTDATED, style: 'caveat' }];
  if (!report.attributionShown) return [];
  const a = report.attribution;
  if (a.status.matched) {
    return [
      {
        table: {
          headerRows: 1,
          widths: ['auto', '*', '*', 'auto', '*'],
          body: [
            ['When', 'Actor', 'Activity', 'Result', 'Correlation id'].map((h) => ({
              text: h,
              style: 'tableHeader',
            })),
            ...a.events.map((e) => {
              const actor = e.actor || UNKNOWN_ACTOR;
              return [
                code(e.at),
                { text: e.actorType === 'user' ? actor : `${actor} (${e.actorType})` },
                { text: e.activity },
                e.result === 'failure' ? { text: e.result, color: '#991b1b' } : { text: e.result },
                code(e.correlationId),
              ];
            }),
          ],
        },
        layout: 'lightHorizontalLines',
        style: 'table',
      },
    ];
  }
  return [
    {
      text: [{ text: 'Attribution: ', bold: true }, `${a.text}${suffix(a.reason, '. ')}`],
      style: a.tone.warning ? 'caveat' : 'meta',
    },
  ];
}

// The By actor section: every matched finding once per actor, each linked to
// its section.
function actorsBlock(input: DriftPdfInput): Content[] {
  const actors = input.report.actors;
  if (!actors || actors.actors.length === 0) return [];
  const out: Content[] = [{ text: 'By actor', style: 'h1', pageBreak: 'before' }];
  if (actors.window.from) {
    out.push({ text: `Audit window ${actors.window.from} to ${actors.window.to}`, style: 'meta' });
  }
  for (const actor of actors.actors) {
    out.push({
      text: actor.tagged ? [actor.actor, { text: ` (${actor.actorType})`, bold: false }] : actor.actor,
      style: 'h4',
    });
    out.push({
      ul: actor.findings.map((f) => ({
        text: [
          {
            text: f.label,
            linkToDestination: findingAnchor(f.key),
            color: '#1d4ed8',
            decoration: 'underline',
          },
          ` · ${f.badge.label} · ${f.at}`,
        ],
      })),
      style: 'list',
    });
  }
  return out;
}

// A run of code-like text (a key, a field path, a value) in the code font when
// it can be encoded there, and in the body font otherwise.
function code(text: string): Content {
  return isWinAnsi(text) ? { text, font: CODE_FONT, style: 'code' } : { text, style: 'code' };
}

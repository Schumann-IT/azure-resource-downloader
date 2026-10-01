import { Injectable } from '@nestjs/common';
import { auditState, AuditState } from './drift-audit';
import {
  DriftDelta,
  DriftFinding,
  DriftObservation,
  DriftState,
  driftState,
  TenantDriftState,
  tenantDriftState,
  typeOfKey,
} from './drift-observation';
import { DriftFiles, DriftService } from './drift.service';
import {
  attributionOf,
  byActor,
  changedByCells,
  DRIFT_PREFIX,
  findingGroups,
  findingHeader,
  observationSummary,
} from './drift-view';
import { MarkdownRendererService, RenderEnv } from './markdown-renderer.service';
import { resolveDriftDocument } from './path-safety';
import { splitLeadingHeading } from './section-hooks';
import { TenantInfo } from './tenant-discovery.service';
import { TenantIndex } from './tenant-index';

// The tenant drift page's model: the observation relative to the export, its
// attribution, and — for a current observation only — the header, the findings
// by type, the By actor blocks and the analysis summary split at its H1.
export interface TenantDriftReport {
  state: TenantDriftState;
  audit: AuditState;
  observation: ReturnType<typeof observationSummary> | null;
  groups: ReturnType<typeof findingGroups>;
  actors: ReturnType<typeof byActor> | null;
  analysis: { heading: string; body: string } | null;
}

// What the drift report says about one finding, independent of how it is
// shown: the heading, the attribution, the comparison and the analysis HTML.
// The page adds its links and the inline payload; the PDF adds neither.
export interface FindingReport {
  finding: ReturnType<typeof findingHeader>;
  attribution: ReturnType<typeof attributionOf>;
  attributionShown: boolean;
  attributionOutdated: boolean;
  intact: boolean;
  deltas: DriftDelta[];
  deltaNote: string;
  analysis: string | null;
}

// The one place the drift report is assembled, read by the drift pages and by
// the PDF export alike, so a page and the PDF of the same observation cannot
// disagree. Reads only through `DriftService` and `resolveDriftDocument()`, and
// renders through the shared `MarkdownRendererService` with the page's env, so
// the render cache serves both.
@Injectable()
export class DriftReportService {
  constructor(
    private readonly drift: DriftService,
    private readonly renderer: MarkdownRendererService,
  ) {}

  // The observation at tenant scope, relative to the export it sits in.
  async tenantDrift(
    info: TenantInfo,
    index: TenantIndex | undefined,
  ): Promise<TenantDriftState> {
    return tenantDriftState(
      await this.drift.observation(info),
      await this.drift.baselineGeneratedAt(info, index),
    );
  }

  // The one drift decision for a resource, read by both its Drift button and
  // its drift page so the two cannot disagree.
  async resourceDrift(
    info: TenantInfo,
    index: TenantIndex | undefined,
    key: string,
  ): Promise<DriftState> {
    return driftState(
      await this.drift.observation(info),
      await this.drift.baselineGeneratedAt(info, index),
      index,
      key,
    );
  }

  // The attribution relative to the observation it must describe. Only asked for
  // a current observation: an audit is never shown for a superseded one.
  async auditOf(info: TenantInfo, observation: DriftObservation): Promise<AuditState> {
    return auditState(await this.drift.audit(info), observation);
  }

  // The tenant drift page's model, from one read of the observation.
  async tenantReport(
    info: TenantInfo,
    index: TenantIndex | undefined,
  ): Promise<TenantDriftReport> {
    const tenant = info.id;
    const state = await this.tenantDrift(info, index);
    const current = state.kind === 'current' ? state.observation : null;
    const audit: AuditState = current ? await this.auditOf(info, current) : { kind: 'none' };
    const activeAudit = audit.kind === 'current' ? audit.audit : undefined;
    return {
      state,
      audit,
      observation: current ? observationSummary(current, tenant, audit) : null,
      groups: current ? findingGroups(current, tenant, activeAudit) : [],
      actors: current && activeAudit ? byActor(current, activeAudit, tenant) : null,
      analysis: current
        ? await this.renderSplit(info.driftIndexPath, {
            tenant,
            docDir: '',
            routeBase: DRIFT_PREFIX,
            changedBy: activeAudit ? changedByCells(current, activeAudit) : undefined,
          })
        : null,
    };
  }

  // What the report says about one finding. `files` are the finding's verified
  // files (null when they were not looked up), `audit` the observation's
  // attribution state. The comparison is only meaningful when `intact`.
  async findingReport(
    info: TenantInfo,
    finding: DriftFinding,
    files: DriftFiles | null,
    audit: AuditState,
  ): Promise<FindingReport> {
    const analysis = await this.renderAnalysis(info, finding.key);
    return {
      finding: findingHeader(finding, analysis?.meta.severity),
      attribution: attributionOf(finding, audit.kind === 'current' ? audit.audit : undefined),
      attributionShown: audit.kind === 'current',
      attributionOutdated: audit.kind === 'outdated',
      intact: files?.intact ?? false,
      deltas: finding.deltas,
      deltaNote: finding.deltaNote,
      analysis: analysis ? analysis.html : null,
    };
  }

  // The analysis agent's drift document for a finding, when it has written
  // one. Its links resolve inside the drift view. Missing or unreadable is
  // normal, never an error.
  private async renderAnalysis(info: TenantInfo, key: string) {
    const file = resolveDriftDocument(info.driftDir, key);
    if (!file) return null;
    try {
      return await this.renderer.render(file, {
        tenant: info.id,
        docDir: typeOfKey(key),
        routeBase: DRIFT_PREFIX,
      });
    } catch {
      return null;
    }
  }

  // An optional page with its H1 split off, so the view can put its facts
  // block between the title and the prose.
  private async renderSplit(
    file: string,
    env: RenderEnv,
  ): Promise<{ heading: string; body: string } | null> {
    try {
      return splitLeadingHeading((await this.renderer.render(file, env)).html);
    } catch {
      return null;
    }
  }
}

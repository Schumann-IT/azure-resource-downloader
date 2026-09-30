import { Controller, Get, Param, Query, Res } from '@nestjs/common';
import { Response } from 'express';
import * as path from 'path';
import { TenantDiscoveryService, TenantInfo } from './tenant-discovery.service';
import { MarkdownRendererService } from './markdown-renderer.service';
import { YamlHighlighterService } from './yaml-highlighter.service';
import {
  resolveDriftDocument,
  resolveResource,
  resolveWithinTenant,
} from './path-safety';
import { DriftService, DriftFiles } from './drift.service';
import { auditState, AuditState } from './drift-audit';
import {
  DriftFinding,
  DriftObservation,
  DriftState,
  driftState,
  tenantDriftState,
  typeOfKey,
} from './drift-observation';
import {
  attributionOf,
  byActor,
  DRIFT_PREFIX,
  driftHref,
  driftPageState,
  findingGroups,
  findingHeader,
  lastSegment,
  observationSummary,
  pickerDrift,
  resourceDriftSwitch,
  stateFlags,
  supersededView,
  tenantSwitch,
  ViewSwitch,
} from './drift-view';
import { LinkEnv } from './link-rewrite';
import {
  buildFacetFilters,
  buildNavigation,
  countMatching,
  exportSummary,
  hasSelection,
  parseFacetSelection,
  TenantIndex,
} from './tenant-index';
import { ExportService } from './export/export.service';
import { splitLeadingHeading } from './section-hooks';
import { diffYaml, WHOLE_FILE } from './yaml-diff';
import { CompareService } from './compare.service';
import {
  COMPARE_PREFIX,
  CompareListing,
  compareListing,
  ComparePane,
  comparePane,
  listingHref,
  pairComparison,
  pairedKeys,
  pairHref,
  selectHref,
} from './compare-view';
import { ResourcesMetadata } from './resources-metadata';

// Route prefix for the source-YAML representation of a document. It is a
// *representation*, not a path segment: it never appears in the breadcrumb, and
// it cannot collide with a resource type because no Azure/Graph type segment
// starts with `_`.
export const RESOURCE_PREFIX = '_resource';

// Route prefix for a whole-tenant export. A representation prefix like
// `_resource`, and safe for the same reason.
export const EXPORT_PREFIX = '_export';

// Paths at the docs root the CLI writes as tool input, not documentation:
// generate.md is the agent prompt. They are never served. Matched without the
// optional `.md` suffix, like every other route.
const TOOL_ARTIFACTS = new Set(['generate']);

// The tenant-wide management summary is the body of the tenant landing page,
// so its own document route is a duplicate and redirects there.
const SUMMARY_ROUTE = 'summary';

// What a 404 can be about, and the exact headline for each — asserted by the
// e2e cases. Chosen in the controller because the template has no `eq`
// helper to branch a string itself.
type NotFoundKind =
  | 'tenant'
  | 'document'
  | 'resource'
  | 'export'
  | 'noDrift'
  | 'drift'
  | 'payload'
  | 'diff'
  | 'compare'
  | 'comparePair';

const NOT_FOUND_HEADLINE: Record<NotFoundKind, string> = {
  tenant: 'Tenant not found',
  document: 'Document not found',
  resource: 'Source YAML not found',
  export: 'Export format not found',
  noDrift: 'No drift observation',
  drift: 'Drift finding not found',
  payload: 'Observed payload not found',
  diff: 'YAML diff not available',
  compare: 'Tenant comparison not available',
  comparePair: 'Resource comparison not available',
};

// A tenant as the compare routes need it: discovered, and with a readable
// `resources/metadata.yaml`, which is the listing's source.
interface CompareTenant {
  info: TenantInfo;
  index: TenantIndex | undefined;
  metadata: ResourcesMetadata;
}

@Controller()
export class DocsController {
  constructor(
    private readonly discovery: TenantDiscoveryService,
    private readonly renderer: MarkdownRendererService,
    private readonly highlighter: YamlHighlighterService,
    private readonly exporter: ExportService,
    private readonly drift: DriftService,
    private readonly compare: CompareService,
  ) {}

  // Discovered tenants paired with their freshly read index, on every call —
  // counts and generatedAt must be as fresh as the sidebar, not snapshotted
  // for the discovery TTL. A tenant whose index has gone unreadable or
  // malformed inside that TTL is dropped for this request, the same
  // conclusion the next scan would reach, so the picker and /healthz agree.
  private async withIndex(): Promise<
    Array<{ info: TenantInfo; index: TenantIndex }>
  > {
    const tenants = await this.discovery.list();
    const paired = await Promise.all(
      tenants.map(async (info) => {
        const index = await this.discovery.getIndex(info);
        return index ? { info, index } : null;
      }),
    );
    return paired.filter(
      (entry): entry is { info: TenantInfo; index: TenantIndex } =>
        entry !== null,
    );
  }

  // GET / — tenant picker, which is also where a tenant's export is offered
  // and a comparison is started.
  @Get()
  async picker(@Res() res: Response): Promise<void> {
    await this.renderPicker(res, null);
  }

  // The picker, optionally in the compare selecting state: `selecting` is
  // marked and every other eligible card offers to be the second tenant. A card
  // is eligible when its export has a readable `resources/metadata.yaml`, and
  // the offer is only made when at least two are.
  private async renderPicker(res: Response, selecting: TenantInfo | null): Promise<void> {
    const paired = await this.withIndex();
    const eligible = new Set<string>();
    await Promise.all(
      paired.map(async ({ info }) => {
        if (await this.compare.metadataOf(info)) eligible.add(info.id);
      }),
    );
    const canCompare = eligible.size >= 2;
    const tenants = await Promise.all(
      paired.map(async ({ info, index }) => ({
        id: info.id,
        name: info.name,
        documented: index.counts.documented,
        pending: index.counts.pending,
        generatedAt: index.generatedAt,
        drift: pickerDrift(await this.tenantDrift(info, index)),
        exportHref: `/${info.id}/${EXPORT_PREFIX}/confluence`,
        compareHref:
          !selecting && canCompare && eligible.has(info.id) ? selectHref(info.id) : null,
        selected: selecting?.id === info.id,
        compareWithHref:
          selecting && selecting.id !== info.id && eligible.has(info.id)
            ? listingHref(selecting.id, info.id)
            : null,
      })),
    );
    res.render('picker', {
      title: selecting ? withTenant('Compare', selecting.name) : 'Documentation',
      tenants,
      selecting: selecting ? { id: selecting.id, name: selecting.name } : null,
    });
  }

  // GET /healthz — discovery health. Always 200: the process is healthy even
  // when the docs root is not there, so a probe that only reads the status code
  // does not flap while a volume is remounted. `status` and `rootReadable` carry
  // the deployment signal for probes that read the body; the root's path itself
  // is never returned.
  @Get('healthz')
  async healthz(@Res() res: Response): Promise<void> {
    const rootReadable = await this.discovery.rootReadable();
    const tenants = await this.withIndex();
    const documents = tenants.reduce(
      (n, { index }) => n + index.counts.documented,
      0,
    );
    const pending = tenants.reduce(
      (n, { index }) => n + index.counts.pending,
      0,
    );
    res.json({
      status: rootReadable ? 'ok' : 'degraded',
      rootReadable,
      tenants: tenants.length,
      documents,
      pending,
    });
  }

  // GET /favicon.ico — browsers request this on their own for responses that
  // carry no <link rel="icon"> (JSON, raw YAML, the export download). Without a
  // route it would fall into `:tenant`, run discovery and render the 404 view
  // on every such request; the icon itself is the static /favicon.svg.
  @Get('favicon.ico')
  favicon(@Res() res: Response): void {
    res.redirect(301, '/favicon.svg');
  }

  // GET /_compare?a=&b= — the tenant compare. `a` alone is the picker in its
  // selecting state; `a` and `b` the comparison pane of the two exports'
  // resources (rows from their `resources/metadata.yaml`, each pair's status from
  // its two files) with no pair selected; `&same` also shows identical pairs.
  // Declared before `:tenant` so the prefix wins; it cannot shadow a tenant
  // because discovery skips `_`-prefixed folders. Read-only.
  @Get(COMPARE_PREFIX)
  async compareTenants(
    @Query('a') a: unknown,
    @Query('b') b: unknown,
    @Query('same') same: string | undefined,
    @Res() res: Response,
  ): Promise<void> {
    if (a === undefined) {
      res.redirect(302, '/');
      return;
    }
    const left = await this.compareTenant(a);
    if (!left) return this.compareNotFound(res, 'compare', a, b, '');
    if (b === undefined) return this.renderPicker(res, left.info);
    const right = await this.compareTenant(b);
    if (!right || right.info.id === left.info.id) {
      return this.compareNotFound(res, 'compare', a, b, '');
    }

    res.render('compare', {
      title: `Compare ${left.info.name} ↔ ${right.info.name}`,
      breadcrumb: [{ label: `${left.info.id} ↔ ${right.info.id}` }],
      left: { id: left.info.id, name: left.info.name },
      right: { id: right.info.id, name: right.info.name },
      pane: await this.paneOf(left, right, '', same !== undefined),
    });
  }

  // GET /_compare/*path?a=&b= — one resource present in both exports as a line
  // diff of the two files with tenant-local identity normalised away, under the
  // comparison pane with this pair selected; `?raw` diffs them as exported and
  // `&same` keeps identical pairs in the pane. Both files are located only
  // through `resolveResource`, and only for a key both exports list.
  @Get(`${COMPARE_PREFIX}/*path`)
  async comparePair(
    @Param() params: any,
    @Query('a') a: unknown,
    @Query('b') b: unknown,
    @Query('raw') raw: string | undefined,
    @Query('same') same: string | undefined,
    @Res() res: Response,
  ): Promise<void> {
    const relPath = joinPath(params.path ?? params['0'] ?? '');
    const key = relPath.replace(/\.yaml$/i, '');
    const left = await this.compareTenant(a);
    const right = await this.compareTenant(b);
    if (!left || !right || left.info.id === right.info.id) {
      return this.compareNotFound(res, 'compare', a, b, relPath);
    }
    const entryA = left.metadata.entries.find((e) => e.key === key && e.presentInTenant);
    const entryB = right.metadata.entries.find((e) => e.key === key && e.presentInTenant);
    const files = entryA && entryB ? await this.compare.pair(left.info, right.info, key) : null;
    if (!entryA || !entryB || !files) {
      return this.compareNotFound(res, 'comparePair', a, b, relPath);
    }

    const comparison = pairComparison(
      files.left,
      files.right,
      left.metadata,
      right.metadata,
      raw !== undefined,
      WHOLE_FILE,
    );
    const self = pairHref(left.info.id, right.info.id, key);
    res.render('compare-diff', {
      title: `Compare: ${entryA.displayName} · ${left.info.id} ↔ ${right.info.id}`,
      breadcrumb: [
        { label: `${left.info.id} ↔ ${right.info.id}` },
        ...this.breadcrumb(key),
      ],
      name: entryA.displayName,
      otherName: entryB.displayName === entryA.displayName ? null : entryB.displayName,
      left: {
        id: left.info.id,
        yamlHref: `/${left.info.id}/${RESOURCE_PREFIX}/${key}`,
      },
      right: {
        id: right.info.id,
        yamlHref: `/${right.info.id}/${RESOURCE_PREFIX}/${key}`,
      },
      rawHref: `${self}${same !== undefined ? '&same' : ''}&raw`,
      normalisedHref: `${self}${same !== undefined ? '&same' : ''}`,
      raw: raw !== undefined,
      pane: await this.paneOf(left, right, key, same !== undefined),
      ...comparison,
    });
  }

  // The listing of two comparable tenants, built in one place for both compare
  // routes so the pair page's pane is the listing page's. Types either index
  // counts under `counts.excluded` go last; the rows are read from the two
  // metadata files the tenants already hold.
  private listingOf(left: CompareTenant, right: CompareTenant): CompareListing {
    const excluded = new Set(
      [left, right].flatMap((t) => t.index?.counts.excluded.map((e) => e.type) ?? []),
    );
    return compareListing(
      { id: left.info.id, metadata: left.metadata },
      { id: right.info.id, metadata: right.metadata },
      excluded,
      RESOURCE_PREFIX,
    );
  }

  // The comparison pane of both compare routes: the listing's rows with each
  // pair's status from the two sides' per-file digests. `selectedKey` is the
  // pair being viewed, or '' on the listing page.
  private async paneOf(
    left: CompareTenant,
    right: CompareTenant,
    selectedKey: string,
    same: boolean,
  ): Promise<ComparePane> {
    const listing = this.listingOf(left, right);
    const keys = pairedKeys(listing);
    const [leftDigests, rightDigests] = await Promise.all([
      this.compare.digests(left.info, left.metadata, keys),
      this.compare.digests(right.info, right.metadata, keys),
    ]);
    return comparePane(listing, leftDigests, rightDigests, {
      a: left.info.id,
      b: right.info.id,
      selectedKey,
      same,
    });
  }

  // GET /:tenant — the tenant landing page: the generation agent's tenant-wide
  // summary (docs/summary.md) as the body, with the index-driven navigation in
  // the sidebar. The summary is optional — an export can carry a valid index
  // and no summary — so a missing one falls back to listing the index.
  @Get(':tenant')
  async tenantIndex(
    @Param('tenant') tenant: string,
    @Query() query: Record<string, unknown>,
    @Res() res: Response,
  ): Promise<void> {
    const info = await this.discovery.get(tenant);
    if (!info) return this.notFound(res, 'tenant', tenant, '');

    const index = await this.discovery.getIndex(info);
    if (!index) return this.notFound(res, 'tenant', tenant, '');

    const summary = await this.renderSplit(info.summaryPath, {
      tenant,
      docDir: '',
    });

    res.render('tenant', {
      title: info.name,
      tenant,
      breadcrumb: [],
      summary,
      exportSummary: exportSummary(index),
      views: tenantSwitch(await this.tenantDrift(info, index), tenant, 'summary'),
      nav: this.nav(info, index, '', `/${tenant}`, query),
    });
  }

  // GET /:tenant/_drift — the observation at tenant scope: its header, the
  // analysis summary (drift/index.md) when the agent has written it, and every
  // recorded finding from the observation itself. Declared before the document
  // catch-all so the prefix wins. An outdated observation renders the gate
  // instead, and none of its findings.
  @Get(`:tenant/${DRIFT_PREFIX}`)
  async driftTenant(
    @Param('tenant') tenant: string,
    @Query() query: Record<string, unknown>,
    @Res() res: Response,
  ): Promise<void> {
    const info = await this.discovery.get(tenant);
    if (!info) return this.notFound(res, 'tenant', tenant, DRIFT_PREFIX);
    const index = await this.discovery.getIndex(info);
    if (!index) return this.notFound(res, 'tenant', tenant, DRIFT_PREFIX);

    const state = await this.tenantDrift(info, index);
    const current = state.kind === 'current' ? state.observation : null;
    const audit = current ? await this.auditOf(info, current) : ({ kind: 'none' } as AuditState);
    const activeAudit = audit.kind === 'current' ? audit.audit : undefined;
    res.render('drift-tenant', {
      title: withTenant('Drift', info.name),
      tenant,
      breadcrumb: [],
      views: tenantSwitch(state, tenant, 'drift'),
      nav: this.nav(info, index, '', `/${tenant}/${DRIFT_PREFIX}`, query),
      state: stateFlags(state.kind),
      superseded:
        state.kind === 'superseded'
          ? supersededView(state.observation, state.baselineGeneratedAt)
          : null,
      observation: current ? observationSummary(current, tenant, audit) : null,
      groups: current ? findingGroups(current, tenant, activeAudit) : [],
      actors: current && activeAudit ? byActor(current, activeAudit, tenant) : null,
      analysis: current
        ? await this.renderSplit(info.driftIndexPath, {
            tenant,
            docDir: '',
            routeBase: DRIFT_PREFIX,
          })
        : null,
    });
  }

  // GET /:tenant/_drift/*path — what the observation says about one resource,
  // addressed by the same extensionless path as its documentation; `?yaml`
  // highlights the observed payload, `?raw` serves it as plain text and
  // `?diff` shows it as a line diff against the baseline, each only once the
  // files are verified against the hashes the observation recorded.
  // Declared before the document catch-all so the prefix wins. `index` is the
  // tenant drift page; everything else at the drift tree root is unreachable.
  @Get(`:tenant/${DRIFT_PREFIX}/*path`)
  async driftResource(
    @Param() params: any,
    @Query('yaml') yamlView: string | undefined,
    @Query('raw') raw: string | undefined,
    @Query('diff') diffView: string | undefined,
    @Query() query: Record<string, unknown>,
    @Res() res: Response,
  ): Promise<void> {
    const tenant: string = params.tenant;
    const relPath = joinPath(params.path ?? params['0'] ?? '');
    // Keys mirror resources/, so only `.yaml` is an optional suffix here: a
    // `.md` path never names a drift key, and so never reaches a payload.
    const key = relPath.replace(/\.yaml$/i, '');

    const info = await this.discovery.get(tenant);
    if (!info) return this.notFound(res, 'tenant', tenant, relPath);
    if (stripExtension(relPath).toLowerCase() === 'index') {
      res.redirect(302, `/${tenant}/${DRIFT_PREFIX}`);
      return;
    }

    const index = await this.discovery.getIndex(info);
    const state = await this.resourceDrift(info, index, key);
    if (state.kind === 'none') return this.notFound(res, 'noDrift', tenant, relPath);
    if (state.kind === 'unknown') return this.notFound(res, 'drift', tenant, relPath);

    const files =
      state.kind === 'finding'
        ? await this.drift.files(info, state.observation, state.finding)
        : null;
    if (diffView !== undefined) {
      const sources = files ? await this.drift.diffSources(files) : null;
      if (!sources || state.kind !== 'finding') {
        return this.notFound(res, 'diff', tenant, relPath);
      }
      return this.serveDiff(res, info, index, state.finding, sources, query);
    }
    if (raw !== undefined || yamlView !== undefined) {
      const payload = files?.intact ? files.payload : null;
      if (!payload || state.kind !== 'finding') {
        return this.notFound(res, 'payload', tenant, relPath);
      }
      return this.servePayload(res, info, index, state.finding.key, payload, raw !== undefined, query);
    }
    await this.renderDrift(res, info, index, key, state, files, query);
  }

  // GET /:tenant/_export/:format — the tenant's documentation as an importable
  // archive. Declared before the document catch-all so the prefix wins. Still
  // read-only: the archive is assembled in memory and streamed, and nothing is
  // written under the docs root.
  @Get(`:tenant/${EXPORT_PREFIX}/:format`)
  async export(
    @Param('tenant') tenant: string,
    @Param('format') format: string,
    @Res() res: Response,
  ): Promise<void> {
    const info = await this.discovery.get(tenant);
    if (!info) return this.notFound(res, 'tenant', tenant, EXPORT_PREFIX);
    if (format !== 'confluence') {
      return this.notFound(res, 'export', tenant, `${EXPORT_PREFIX}/${format}`);
    }

    const index = await this.discovery.getIndex(info);
    if (!index) return this.notFound(res, 'tenant', tenant, EXPORT_PREFIX);

    await this.exporter.confluence(info, index, res);
  }

  // GET /:tenant/_resource/*path — the exported source YAML behind a document,
  // syntax highlighted; `?raw` serves it as plain text. Declared before the
  // document catch-all so the prefix wins. Read-only, like every other route.
  @Get(`:tenant/${RESOURCE_PREFIX}/*path`)
  async resource(
    @Param() params: any,
    @Query('raw') raw: string | undefined,
    @Query() query: Record<string, unknown>,
    @Res() res: Response,
  ): Promise<void> {
    const tenant: string = params.tenant;
    const relPath = joinPath(params.path ?? params['0'] ?? '');

    const info = await this.discovery.get(tenant);
    if (!info) return this.notFound(res, 'tenant', tenant, relPath);

    const resolved = resolveResource(info.resourcesDir, relPath);
    if (!resolved) return this.notFound(res, 'resource', tenant, relPath);

    if (raw !== undefined) {
      res.sendFile(resolved, {
        headers: {
          'Content-Type': 'text/plain; charset=utf-8',
          'X-Content-Type-Options': 'nosniff',
          'Content-Disposition': 'inline',
        },
      });
      return;
    }

    const docPath = stripExtension(relPath);
    try {
      const rendered = await this.highlighter.render(resolved);
      const index = await this.discovery.getIndex(info);
      res.render('resource', {
        title: withTenant(`${path.posix.basename(docPath)}.yaml`, info.name),
        tenant,
        breadcrumb: this.breadcrumb(relPath),
        source: `${docPath}.yaml`,
        rawHref: `/${tenant}/${RESOURCE_PREFIX}/${docPath}?raw`,
        body: rendered.html,
        highlighted: rendered.highlighted,
        lines: rendered.lines,
        size: rendered.size,
        views: this.views(
          tenant,
          docPath,
          'resource',
          resourceDriftSwitch(
            await this.resourceDrift(info, index, docPath),
            tenant,
            docPath,
            false,
          ),
        ),
        nav: index
          ? this.nav(
              info,
              index,
              docPath,
              `/${tenant}/${RESOURCE_PREFIX}/${docPath}`,
              query,
            )
          : null,
      });
    } catch {
      this.notFound(res, 'resource', tenant, relPath);
    }
  }

  // GET /:tenant/*path — a document within the tenant.
  @Get(':tenant/*path')
  async doc(
    @Param() params: any,
    @Query() query: Record<string, unknown>,
    @Res() res: Response,
  ): Promise<void> {
    const tenant: string = params.tenant;
    const relPath = joinPath(params.path ?? params['0'] ?? '');

    const info = await this.discovery.get(tenant);
    if (!info) return this.notFound(res, 'tenant', tenant, relPath);

    const rootDoc = relPath.replace(/\.md$/i, '').toLowerCase();
    if (TOOL_ARTIFACTS.has(rootDoc)) {
      return this.notFound(res, 'document', tenant, relPath);
    }
    if (rootDoc === SUMMARY_ROUTE) {
      res.redirect(302, `/${tenant}`);
      return;
    }

    const resolved = resolveWithinTenant(info.dir, relPath);
    if (!resolved) return this.notFound(res, 'document', tenant, relPath);

    try {
      const page = await this.renderer.render(resolved, {
        tenant,
        docDir: this.docDir(relPath),
      });
      const index = await this.discovery.getIndex(info);
      const docPath = stripExtension(relPath);
      // The document's own path is what locates its source YAML (the export
      // mirrors docs/<type>/<name>.md and resources/<type>/<name>.yaml), so
      // `meta.source` stays a label and is only used as the has-a-source flag.
      const hasSource = typeof page.meta.source === 'string' && !!page.meta.source;
      res.render('page', {
        title: withTenant(page.title || relPath, info.name),
        body: page.html,
        tenant,
        breadcrumb: this.breadcrumb(relPath),
        meta: page.meta,
        sourceHref: hasSource
          ? `/${tenant}/${RESOURCE_PREFIX}/${docPath}`
          : null,
        views: hasSource
          ? this.views(
              tenant,
              docPath,
              'doc',
              resourceDriftSwitch(
                await this.resourceDrift(info, index, docPath),
                tenant,
                docPath,
                false,
              ),
            )
          : null,
        nav: index
          ? this.nav(info, index, relPath, `/${tenant}/${docPath}`, query)
          : null,
      });
    } catch {
      this.notFound(res, 'document', tenant, relPath);
    }
  }

  // View model for the sidebar partial: the tenant metadata that used to sit on
  // the landing page, the facet filters and the navigation tree, so all three
  // survive on every page. The selection is read from the query parameters named
  // after the index's own axis ids, and values this index cannot serve are
  // dropped rather than rendering an empty tree that would look like a broken
  // tenant.
  //
  // `matched`/`total` count distinct resources and deliberately ignore the
  // active-document exemption in `buildNavigation`, so the numbers describe the
  // selection rather than the page.
  private nav(
    info: TenantInfo,
    index: TenantIndex,
    activeDoc: string,
    basePath: string,
    query: Record<string, unknown>,
  ): Record<string, unknown> {
    const selection = parseFacetSelection(index, query);
    const sections = buildNavigation(index, info.id, activeDoc, selection);
    return {
      tenant: info.id,
      name: info.name,
      generatedAt: index.generatedAt,
      counts: index.counts,
      complete: index.complete,
      incompleteReason: index.incompleteReason,
      facets: buildFacetFilters(index, basePath, selection),
      filtering: hasSelection(selection),
      matched: countMatching(index, selection),
      total: index.resources.length,
      clearHref: basePath,
      // Whether the tree is one longer than `matched` because the document being
      // viewed is exempt from the filter, so the view can say so instead of
      // leaving the reader to reconcile the two.
      exempt: sections.some((s) => s.items.some((i) => i.exempt)),
      sections,
    };
  }

  // The Documentation | YAML | Drift switcher for the top bar. All three
  // representations share the same extensionless path, so no extra lookup is
  // needed; the Drift entry is decided by the caller from the observation.
  private views(
    tenant: string,
    docPath: string,
    kind: 'doc' | 'resource' | 'drift',
    drift: ViewSwitch,
  ): ViewSwitch[] {
    return [
      {
        label: 'Documentation',
        href: `/${tenant}/${docPath}`,
        active: kind === 'doc',
      },
      {
        label: 'YAML',
        href: `/${tenant}/${RESOURCE_PREFIX}/${docPath}`,
        active: kind === 'resource',
      },
      drift,
    ];
  }

  private async tenantDrift(info: TenantInfo, index: TenantIndex | undefined) {
    return tenantDriftState(
      await this.drift.observation(info),
      await this.drift.baselineGeneratedAt(info, index),
    );
  }

  // The one drift decision for a resource, read by both its Drift button and
  // its drift page so the two cannot disagree.
  private async resourceDrift(
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
  private async auditOf(info: TenantInfo, observation: DriftObservation): Promise<AuditState> {
    return auditState(await this.drift.audit(info), observation);
  }

  private async renderDrift(
    res: Response,
    info: TenantInfo,
    index: TenantIndex | undefined,
    key: string,
    state: Exclude<DriftState, { kind: 'none' | 'unknown' }>,
    files: DriftFiles | null,
    query: Record<string, unknown>,
  ): Promise<void> {
    const tenant = info.id;
    const finding = state.kind === 'finding' ? state.finding : null;
    // The documentation a finding belongs to sits at its baseline key; an
    // addition has none and is reached from the tenant drift page only.
    const docKey = finding ? finding.baselineKey || finding.key : key;
    const documented = indexLists(index, docKey);

    res.render('drift', {
      title: withTenant(`Drift: ${finding?.displayName || lastSegment(key)}`, info.name),
      tenant,
      breadcrumb: this.breadcrumb(key),
      views: documented
        ? this.views(tenant, docKey, 'drift', resourceDriftSwitch(state, tenant, docKey, true))
        : null,
      nav: index ? this.nav(info, index, docKey, driftHref(tenant, key), query) : null,
      tenantDriftHref: `/${tenant}/${DRIFT_PREFIX}`,
      name: lastSegment(key),
      ...driftPageState(state),
      ...(finding
        ? await this.findingView(info, state.observation, finding, files, documented)
        : {}),
    });
  }

  // The finding-specific part of a drift page. The comparison (deltas, inline
  // payload) is only filled in when every file it was decided on is intact.
  private async findingView(
    info: TenantInfo,
    observation: DriftObservation,
    finding: DriftFinding,
    files: DriftFiles | null,
    documented: boolean,
  ) {
    const verified = files ?? { baseline: null, payload: null, intact: false };
    const audit = await this.auditOf(info, observation);
    const analysis = await this.renderAnalysis(info, finding.key);
    const inline = finding.verdict === 'added' && verified.intact ? verified.payload : null;
    return {
      finding: findingHeader(finding, analysis?.meta.severity),
      attribution: attributionOf(finding, audit.kind === 'current' ? audit.audit : undefined),
      attributionShown: audit.kind === 'current',
      attributionOutdated: audit.kind === 'outdated',
      links: this.driftLinks(info.id, finding, verified, documented),
      intact: verified.intact,
      deltas: finding.deltas,
      deltaNote: finding.deltaNote,
      payload: inline ? (await this.highlighter.render(inline)).html : null,
      analysis: analysis ? analysis.html : null,
    };
  }

  // The finding's way out to its other representations, each offered only when
  // it is there: the baseline once verified, the payload once the comparison
  // is intact, the documentation when the index lists it.
  private driftLinks(
    tenant: string,
    finding: DriftFinding,
    files: DriftFiles,
    documented: boolean,
  ): Array<{ label: string; href: string }> {
    const links: Array<{ label: string; href: string }> = [];
    if (documented) {
      links.push({ label: 'Documentation', href: `/${tenant}/${finding.baselineKey}` });
    }
    const href = driftHref(tenant, finding.key);
    const observed = files.intact && files.payload;
    // Both sides verified: one diff replaces the two separate views.
    if (files.baseline && observed) {
      links.push({ label: 'YAML diff', href: `${href}?diff` });
      return links;
    }
    if (files.baseline) {
      links.push({
        label: 'Baseline YAML',
        href: `/${tenant}/${RESOURCE_PREFIX}/${finding.baselineKey}`,
      });
    }
    if (observed) {
      links.push({ label: 'Observed YAML', href: `${href}?yaml` });
      links.push({ label: 'Observed YAML (raw)', href: `${href}?raw` });
    }
    return links;
  }

  // The analysis agent's drift document for a finding, when it has written
  // one. Its links resolve inside the drift view.
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

  private async servePayload(
    res: Response,
    info: TenantInfo,
    index: TenantIndex | undefined,
    key: string,
    file: string,
    raw: boolean,
    query: Record<string, unknown>,
  ): Promise<void> {
    if (raw) {
      res.sendFile(file, {
        headers: {
          'Content-Type': 'text/plain; charset=utf-8',
          'X-Content-Type-Options': 'nosniff',
          'Content-Disposition': 'inline',
        },
      });
      return;
    }
    const tenant = info.id;
    const rendered = await this.highlighter.render(file);
    res.render('resource', {
      title: withTenant(`${lastSegment(key)}.yaml`, info.name),
      tenant,
      breadcrumb: this.breadcrumb(key),
      observed: true,
      source: `${key}.yaml`,
      rawHref: `${driftHref(tenant, key)}?raw`,
      body: rendered.html,
      highlighted: rendered.highlighted,
      lines: rendered.lines,
      size: rendered.size,
      views: null,
      nav: index ? this.nav(info, index, key, driftHref(tenant, key), query) : null,
    });
  }

  // The baseline and the observed payload of a finding as one line diff. The
  // raw links stay available for copying either side verbatim.
  private serveDiff(
    res: Response,
    info: TenantInfo,
    index: TenantIndex | undefined,
    finding: DriftFinding,
    sources: { baseline: string; observed: string },
    query: Record<string, unknown>,
  ): void {
    const tenant = info.id;
    const key = finding.key;
    res.render('drift-diff', {
      title: withTenant(`Diff: ${finding.displayName || lastSegment(key)}`, info.name),
      tenant,
      breadcrumb: this.breadcrumb(key),
      baselineSource: `${finding.baselineKey}.yaml`,
      observedSource: `${key}.yaml`,
      findingHref: driftHref(tenant, key),
      baselineRawHref: `/${tenant}/${RESOURCE_PREFIX}/${finding.baselineKey}?raw`,
      observedRawHref: `${driftHref(tenant, key)}?raw`,
      diff: diffYaml(sources.baseline, sources.observed),
      views: null,
      nav: index ? this.nav(info, index, key, driftHref(tenant, key), query) : null,
    });
  }

  // An optional page with its H1 split off, so the view can put its facts
  // block between the title and the prose.
  private async renderSplit(
    file: string,
    env: LinkEnv,
  ): Promise<{ heading: string; body: string } | null> {
    const html = await this.renderOptional(file, env);
    return html === null ? null : splitLeadingHeading(html);
  }

  private async renderOptional(file: string, env: LinkEnv): Promise<string | null> {
    try {
      return (await this.renderer.render(file, env)).html;
    } catch {
      return null;
    }
  }

  private docDir(relPath: string): string {
    const dir = path.posix.dirname(stripExtension(relPath));
    return dir === '.' ? '' : dir;
  }

  private breadcrumb(relPath: string): Array<{ label: string }> {
    return stripExtension(relPath)
      .split('/')
      .filter(Boolean)
      .map((label) => ({ label }));
  }

  private notFound(
    res: Response,
    kind: NotFoundKind,
    tenant: string,
    requested: string,
    detail?: string,
  ): void {
    const headline = NOT_FOUND_HEADLINE[kind];
    res
      .status(404)
      .render('error', { title: headline, headline, tenant, requested, detail });
  }

  // A tenant that can take part in a comparison, or null: the query value must
  // be one discovered tenant with readable resource metadata.
  private async compareTenant(value: unknown): Promise<CompareTenant | null> {
    if (typeof value !== 'string' || !value) return null;
    const info = await this.discovery.get(value);
    if (!info) return null;
    const metadata = await this.compare.metadataOf(info);
    if (!metadata) return null;
    return { info, index: await this.discovery.getIndex(info), metadata };
  }

  // A compare 404 is about two tenants, not one, so it names both in the
  // detail line and puts no tenant in the header.
  private compareNotFound(
    res: Response,
    kind: NotFoundKind,
    a: unknown,
    b: unknown,
    requested: string,
  ): void {
    const name = (v: unknown) => (typeof v === 'string' && v ? v : '(none)');
    this.notFound(res, kind, '', requested, `Comparing ${name(a)} with ${name(b)}`);
  }
}

function indexLists(index: TenantIndex | undefined, key: string): boolean {
  return !!index?.resources.some((r) => stripExtension(r.doc) === key);
}

function joinPath(value: any): string {
  return Array.isArray(value) ? value.join('/') : String(value ?? '');
}

function stripExtension(relPath: string): string {
  return relPath.replace(/\.(md|yaml)$/i, '');
}

// Page title for a view inside a tenant. The document name leads because tabs
// and the history dropdown truncate from the right, while the tenant is the
// part that repeats across every tab opened from one export.
function withTenant(label: string, tenant: string): string {
  return tenant ? `${label} · ${tenant}` : label;
}

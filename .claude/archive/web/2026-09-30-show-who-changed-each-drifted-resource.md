---
title: Show who changed each drifted resource, from the CLI's audit attribution
project: web
status: done
started: 2026-09-30
finished: 2026-09-30
branch: feat/drift-attribution
pr: 33
changelog: 0.4.0
---
## Show who changed each drifted resource, from the CLI's audit attribution

**Goal.** Answer *who changed this, and when* on the drift pages. The CLI's `resource audit` joins each drift
finding against the tenant's Log Analytics audit tables and writes the result as `drift/audit.yaml` beside the
observation. This browser reads that file and shows, per finding, the actor and time of the change — or states
explicitly why there is none — on the tenant drift page (a suffix on every finding row, a **By actor** section,
a caveat line in the observation header, and a **Changed by** column in the analysis index's findings table)
and on the resource drift page (an attribution block in the header, every event newest first). The app derives nothing: it joins by finding key, shows the recorded facts, and shows
an attribution only when the audit file describes exactly the observation on disk.

> **Why.** A drift finding says a resource's bytes moved between the baseline and the observation; the audit
> tables know who moved them. Without the join a reader cannot tell a deliberate administrative change from an
> unexplained one, and the analysis agent's prose is the only place an actor is ever named. The structured data
> is what the drift documents are checked against.
>
> **Contract.** The CLI writes `<export>/drift/audit.yaml` at the drift root beside `metadata.yaml`, only when
> the tenant has an audit workspace configured; swept with the rest of the tree, so absence is the normal state,
> never an error. Read as data through `TenantInfo.driftAuditPath`, never served. Shape: `version` (integer,
> `>= 1` accepted); `observedAt`, `baselineGeneratedAt`; `tenant`, `toolVersion`, `queriedAt`, `workspaceId`;
> `window: {from, to}` (baseline `generatedAt` → `observedAt`); `tables` with both `IntuneAuditLogs` and
> `AuditLogs`, each `{status: ok | failed, reason, earliest}` (`earliest` may be empty); `counts: {matched,
> noEventInWindow, noJoinKey, retentionExceeded, queryFailed, notQueried}` (read, never recomputed);
> `findings` keyed like `drift/metadata.yaml`'s (`<type>/<name>.yaml`, reduced by the same `driftKey` rule —
> the *new* path of a rename), each `{status, table, reason, events}`: `status` ∈ `matched | no-event-in-window
> | no-join-key | retention-exceeded | query-failed | not-queried`, `table` ∈ `IntuneAuditLogs | AuditLogs |
> ""`, `events` (`[]` unless matched; newest first, may be several) of `{at, actor (may be "": shown as
> "unknown actor"), actorType: user | application | unknown, activity, result: success | failure | unknown,
> correlationId}`. Timestamps: quoted RFC3339 UTC, whole seconds, compared as strings after Date normalisation.
> **Validity rule.** Current only when `observedAt` **and** `baselineGeneratedAt` equal the observation's;
> otherwise outdated (caveat only: run `azure-rd resource audit` again), and never shown for a superseded one.
>
> **Rendering rules.** A finding the audit file does not name gets a neutral "no attribution recorded" line,
> never an inferred status. The `docs analyze-drift` prompt gains attribution lines on the Go side, so
> agent-written drift documents may name actors in prose; this app shows the structured data regardless. The
> one exception to "no hook into agent-written Markdown" is the analysis index's findings table (the table
> `findings-table.ts` already tags as `findings-drift`): its rows link to their drift documents, and that link
> is the join key, so the app appends a **Changed by** column from `audit.yaml` rather than asking the agent to
> copy actors into prose. No other section of `drift/index.md` or of a per-resource drift document is touched.
> The `docs analyze-drift` prompt only recommends that table ("prose is fine too"), so an index written as
> prose simply gets no column — never an error.
>
> **Owner.** none — every file is under `web/`. Sequencing: none; the web degrades gracefully when the file is
> absent, so it can ship before or after the CLI's `resource audit`.
>
> **Implementer.** sonnet
>
> **Non-negotiables untouched.** Read-only (one more file read, no write path); one `markdown-it` renderer (the
> Changed-by column is a core-rule token pass like `findings-table.ts`, fed through the render `env`; the
> render cache stays keyed by the file's mtime + size plus the fingerprint of the cells it was rendered with,
> so a rerun of `resource audit` shows without a restart); the Changed-by cell tones are the one styles.css
> addition, keyed by data attribute like the rest of the `findings-drift` table, because Tailwind scans only
> `views/**/*.hbs` and cannot see classes emitted from TypeScript; path safety unchanged — `path-safety.ts` is not edited, the root file is read through a
> fixed `TenantInfo.driftAuditPath`, and `/:tenant/_drift/audit(.yaml)(?raw)` stays unreachable because both
> drift resolvers already require two segments and `driftState` never yields a finding for it; no client-side
> JavaScript (`<details>` only); counts from `counts.*`, never by walking; the new cache is a fourth `FileCache`
> with the same mtime + size freshness and bound; every new visual state gets a dark variant (`dark:` in the
> templates, `prefers-color-scheme: dark` in `styles.css`); all values `{{ }}`-escaped or emitted as
> markdown-it `text` tokens; Tailwind in the templates.

**Plan.**

- ✅ **Parser** — new pure `src/docs/drift-audit.ts`: `AUDIT_STATUSES` (the closed set above), `AuditEvent`,
  `AuditAttribution { key, status, table, reason, events }`, `DriftAudit { version, observedAt,
  baselineGeneratedAt, tenant, toolVersion, queriedAt, workspaceId, window, tables, counts, findings, byKey }`.
  `parseAudit(raw)` never throws and returns `undefined` for anything that is not an object with integer
  `version >= 1`, an `observedAt` and a `baselineGeneratedAt`; findings with an unsafe key or an unknown status
  are dropped (the `isVerdict` pattern), a `matched` finding without at least one well-formed event is dropped,
  `actorType`/`result` outside their sets become `unknown`, events are sorted newest first defensively. Export
  `driftKey`, `timestamp`, `isRecord`, `str`, `num` from `drift-observation.ts` (today module-private) so the
  two parsers key and normalise identically. `auditState(audit, observation)` → `{ kind: 'none' } |
  { kind: 'outdated', audit } | { kind: 'current', audit }` per the validity rule.
- ✅ **Service** — `tenant-discovery.service.ts`: `DRIFT_AUDIT_FILE = 'audit.yaml'`, `TenantInfo.driftAuditPath`
  (comment: read as data, never served), set where `driftObservationPath` is. `drift.service.ts`: a fourth
  `FileCache<DriftAudit | undefined>` bounded like the others and `async audit(info)` reading
  `info.driftAuditPath` through `parseAudit`.
- ✅ **View models** (`drift-view.ts`) — `STATUS_TONE: Record<AuditStatus, Tone>` = `matched: success`,
  `no-event-in-window: warning`, `retention-exceeded: warning`, `query-failed: warning`, `not-queried: neutral`,
  `no-join-key: neutral`, with a `STATUS_TEXT` map for the one-line statuses ("no audit event in the window",
  "window starts before the table's retention", "audit query failed", "not queried", "no audit join key for this
  resource type"). `attributionOf(finding, audit | undefined)` → `{ status flags via stateFlags, tone flags via
  badge(), table, reason, events (all, newest first), latest, more: events.length - 1 }`, or the neutral "no
  attribution recorded" shape when the audit has no entry for `finding.key`. `findingItem` gains
  `attribution: { actor, at, more } | { text, quiet } | null` for the row suffix (latest event, `+N more`;
  `no-join-key` marked `quiet`). `byActor(obs, audit, tenant)` → blocks sorted by actor:
  `{ actor, actorType, findings: [{ href, label, badge, at }] }` built only from `matched` entries whose key the
  observation holds, one row per finding (several events by the same actor do not repeat it), plus the window.
  `observationSummary(obs, tenant, auditState)` gains `attribution`: for `current` — `workspaceId`, `queriedAt`,
  `window`, per-table `{ name, failed, reason, earliest }`, and the `counts` fields; for `outdated` —
  `{ outdated: true }`; for `none` — `null` (no line).
- ✅ **Templates** — `drift-tenant.hbs`: after the link and "was …" in each `<li>`, a
  `<span class="text-xs text-slate-500 dark:text-slate-400">` with `actor · at (+N more)` or the status text
  (amber `text-amber-700 dark:text-amber-300` for warning statuses, plain slate for neutral); after
  `details.drift-findings`, `<section class="drift-actors mt-8">` with an H2 "By actor", one block per actor
  (name, small `application` tag when `actorType` is not `user`, the window, a `<ul>` of finding links each with
  its verdict badge and event time). `partials/drift-observation.hbs`: an attribution caveat `<p>` after the
  counts line — workspace, window, `matched / no event / beyond retention / failed / not queried` counts, one
  amber fragment per failed table with its reason — or the amber "Attribution outdated: it predates this
  observation. Run `azure-rd resource audit` again." line. `drift.hbs`: inside `<header>`, after
  `p.drift-links`, `<div class="drift-attribution mt-4 …">`: for `matched` a compact table (When | Actor |
  Activity | Result | Correlation id) styled like `section.drift-deltas`, `failure` results in the deltas' red;
  for every other status one `<p>` in its tone (`no-join-key` as quiet slate text, the amber ones on the
  `drift-incomplete` amber background, `not-queried`/`query-failed` with their `reason`); the outdated caveat as
  one amber line. Every state with its `dark:` variant; nothing new in `styles.css`.
- ✅ **Controller wiring** — `driftTenant`: read `this.drift.audit(info)` only when the observation is `current`,
  compute `auditState`, pass the audit to `observationSummary`/`findingGroups` only in the `current` audit
  state, plus `actors: byActor(...)`. `renderDrift`/`findingView`: same gate, `attribution:
  attributionOf(finding, activeAudit)` and `attributionOutdated` for the header caveat;
  `unknownType`/`notComparable`/`unchanged` pages show no attribution (the audit has no entry for them by
  construction). HTTP only — no filesystem logic in the controller.
- ✅ **Tests** — `test/drift-audit.spec.ts` (inline YAML fixtures): parses the contract shape and indexes by key;
  malformed, missing `version`, missing timestamps → `undefined`, never a throw; unsafe keys (`..`, absolute,
  `.yml`, non-`.yaml`) and unknown statuses dropped; `matched` without events dropped; unquoted timestamps loaded
  as `Date` normalise to the same string; events reordered newest first; `auditState` none / outdated on either
  timestamp differing / current; `attributionOf` per status and for a finding the file does not name; `byActor`
  grouping, ordering and the observation-key join; `STATUS_TONE` covers every status. `test/docs.e2e.spec.ts`
  drift block: `writeDrift()` writes `drift/audit.yaml` naming `changed1` (two events, two actors), `new_name`
  (`no-event-in-window`), `tampered1` (`retention-exceeded`), `new_loc` (`no-join-key`) and leaving one finding
  unnamed; assert the tenant page rows carry the latest actor and `+1 more`, the status texts, the **By actor**
  section with both actors linking `changed1`, the header caveat with workspace and counts; the resource page
  shows both events newest first with correlation ids, and one line per other status (the `no-join-key` line
  without the amber classes); the renamed finding's old (baseline) path shows the same attribution as
  `new_name`, joined by the finding's key; an audit whose `baselineGeneratedAt` alone differs is outdated too; an audit whose `observedAt` differs shows the outdated caveat and no actor
  anywhere; `/drifted/_drift/audit`, `/drifted/_drift/audit.yaml`, `/drifted/_drift/audit?raw` → 404 without
  the root path in the body; rewriting and deleting `audit.yaml` is reflected on the next request without a
  restart; the read-only snapshot and the Confluence export assertions cover the audit file (no actor name, no
  `audit.yaml` in the zip).
- ✅ **Changed-by cells** (`drift-view.ts`) — pure `changedByCells(obs, audit)` →
  `{ cells: Map<key, { text, tone: 'matched' | 'warning' | 'quiet' }>, fingerprint: string }`, one entry per
  finding **of the observation** (keys are `driftKey`'s extensionless `<type>/<name>`), built from the same
  `rowAttribution`/`attributionOf` as the tenant page's row suffix so the two can never disagree: `matched` →
  `<actor> · <at>` plus ` (+N more)` when `more > 0`, `actor` already `unknown actor` for an empty one, tone
  `matched`; any other status → its `STATUS_TEXT`, tone `warning` for the amber statuses and `quiet` for
  `no-join-key`/`not-queried`; a finding the audit does not name → "no attribution recorded", tone `quiet`.
  `fingerprint` is `JSON.stringify` of the entries sorted by key — it changes exactly when a rendered cell
  would. Called only with the `current` audit.
- ✅ **Changed-by column** — new pure, Nest-free `applyChangedBy(tokens, cells, makeToken)` in
  `findings-table.ts`, run by the existing `findings_table` core rule right after `applyFindingsTable` and only
  when `state.env.changedBy` is set (`makeToken` is `(type, tag, nesting) => new state.Token(…)`, as
  `wrapSections` takes). For each table tagged `findings-drift` that has a `resource` header: append
  `th_open`/`inline`/`th_close` with text `Changed by` to the header row, and one `td_open`/`inline`/`td_close`
  before every body row's `tr_close` (markdown-it emits a `td` per header column, so the cell is always last).
  Both carry `data-column="changed-by"`; a non-empty body cell carries `data-attribution="<tone>"`. The `inline`
  token's `children` is a single `text` token (content = cell text), so markdown-it escapes it — never
  `html_inline`. The row's key: the first `link_open` among the Resource cell's inline `children` (core rules
  run before the `link_open` renderer, so this is the original href); drop `#…`; it must not start with `/`,
  `//` or a scheme, must end in `.md` (case-insensitive), is `path.posix.normalize`d against the drift root
  and rejected when the result is `..` or starts with `../`; the key is that path without `.md`, accepted only
  when `driftKey(key + '.yaml')` returns it. No key (the inventory rows carry no link, a `../../../docs/…`
  link), or a key the map does not hold (not a finding of this observation) → an **empty** cell with no
  `data-attribution`. No `changedBy` in the env → the token stream is untouched, byte-identical to today.
- ✅ **Renderer and wiring** — `markdown-renderer.service.ts`: `render(file, env: RenderEnv)` with
  `RenderEnv = LinkEnv & { changedBy?: { cells, fingerprint } }`; the `md.render` env passes `changedBy`
  through beside `tenant`/`docDir`/`routeBase`; `CacheEntry` gains `fingerprint: string | undefined` and a
  hit requires it to equal `env.changedBy?.fingerprint` strictly (so a render with cells is never served to a
  request without, and vice versa). `docs.controller.ts` `driftTenant`: when `activeAudit` is set, pass
  `changedBy: changedByCells(current, activeAudit)` in the `renderSplit(info.driftIndexPath, …)` env
  (`renderSplit`/`renderOptional` take `RenderEnv`); no audit or an outdated one → no `changedBy`, the table
  renders exactly as today (the outdated caveat is already in the observation header). `renderAnalysis` (the
  per-resource drift documents), the documentation pages and `export/export.service.ts` pass none.
- ✅ **Tone styles** — `src/styles.css`, in the drift findings table block: `.findings-drift
  td[data-attribution="warning"]` amber (the `text-amber-700` / dark `text-amber-300` hues),
  `[data-attribution="quiet"]` slate (`text-slate-500` / dark `text-slate-400`), `[data-attribution="matched"]`
  inheriting the body colour; the dark hues under `@media (prefers-color-scheme: dark)`; the column is kept
  `white-space: nowrap` for `matched` so `actor · at` does not break mid-timestamp.
- ✅ **Tests for the Changed-by column** — `test/findings-table.spec.ts` (new; hand-built tokens over the
  minimal token surface, as `test/section-hooks.spec.ts` does): header and cells appended with `data-column`; key
  from the link for a nested type path and a rename's new path; `#anchor` stripped; inventory row, a
  `../../../docs/…` link, an absolute link and a key outside the map → empty cell; unknown-to-audit finding →
  "no attribution recorded" `quiet`; `+N more`; `unknown actor`; an actor containing `<script>` stays the content of a
  `text` child (never `html_inline`); a table without a Verdict column (the summary's Findings table) and a drift table without
  a Resource column untouched; no env → output identical to today's. `test/drift-audit.spec.ts`:
  `changedByCells` per status, same text as the row suffix, fingerprint stable across calls and different when
  one event changes. `test/styles-build.spec.ts`: the three `data-attribution` rules exist. e2e
  (`test/docs.e2e.spec.ts`): extend the existing `DRIFT_INDEX` table with a `new_name` row (its inventory row
  already exists; the existing severity/verdict assertions keep passing); the landing page shows
  `<th data-column="changed-by">Changed by</th>`, `alice@drifted.example · 2026-01-20T09:00:00Z (+1 more)`
  inside a `data-column="changed-by"` cell of the `changed1` rows (the tenant page's row suffix carries the
  same text, so assert on the cell), the `new_name` status text with `data-attribution="warning"`, and an empty cell for the
  inventory row; rewriting `audit.yaml` (e.g. swapping the two events) changes the cell on the next request
  without a restart; an `observedAt`-outdated audit and a deleted one render the table without the column (no
  `changed-by` in the body) and restore it after `writeDrift()`; the Confluence zip still contains no actor
  name.
- ✅ **`README.md`** — docs-root contract tree gains `│   ├── audit.yaml             # attribution — read, never
  served` under `drift/`, and the root-level rule names it among the unreachable files; **Drift view** gains an
  **Attribution.** bullet (what the file is, the validity rule, the per-status lines, By actor, the Changed by
  column in the analysis index's findings table and its join through the row's link, that nothing is derived); routes table unchanged; Tests table adds `test/drift-audit.spec.ts` and extends the e2e row; Project
  layout adds `drift-audit.ts` and updates the `drift.service.ts` line. With it, at *done*: `web/CLAUDE.md`
  and `.windsurf/rules/01-architecture.md` name `drift-audit.ts` among the drift modules and
  `drift/audit.yaml` beside `drift/analyze.md` as read as data, never served.
- ✅ **`CHANGELOG.md`** — under `[Unreleased]` → `### Added` → new `#### Drift view` subsection (today `Drift
  view` exists only under `### Changed` and `### Fixed`).

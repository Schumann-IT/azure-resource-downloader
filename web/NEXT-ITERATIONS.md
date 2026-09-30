# Next iterations

Outstanding work, standing decisions and parked ideas for the docs browser. `README.md` describes what it does
today and `CHANGELOG.md` records what shipped; neither is repeated here.

The *Features* below are scheduled design work, promoted from *Parked ideas* and refined against what is true
now. The *Fixes* below are scheduled too, but smaller: self-contained corrections that need no design work,
listed so they are not forgotten between features. Every idea in *Parked ideas* is deliberately unscheduled:
picking one up means promoting it into a numbered work entry with a `**Goal.**` and a `**Plan.**`, reconciling
its rationale against what is true at that point rather than copying it across.

## Features

### 1. Show who changed each drifted resource, from the CLI's audit attribution

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

- ~~**Parser** — new pure `src/docs/drift-audit.ts`: `AUDIT_STATUSES` (the closed set above), `AuditEvent`,
  `AuditAttribution { key, status, table, reason, events }`, `DriftAudit { version, observedAt,
  baselineGeneratedAt, tenant, toolVersion, queriedAt, workspaceId, window, tables, counts, findings, byKey }`.
  `parseAudit(raw)` never throws and returns `undefined` for anything that is not an object with integer
  `version >= 1`, an `observedAt` and a `baselineGeneratedAt`; findings with an unsafe key or an unknown status
  are dropped (the `isVerdict` pattern), a `matched` finding without at least one well-formed event is dropped,
  `actorType`/`result` outside their sets become `unknown`, events are sorted newest first defensively. Export
  `driftKey`, `timestamp`, `isRecord`, `str`, `num` from `drift-observation.ts` (today module-private) so the
  two parsers key and normalise identically. `auditState(audit, observation)` → `{ kind: 'none' } |
  { kind: 'outdated', audit } | { kind: 'current', audit }` per the validity rule.~~
- ~~**Service** — `tenant-discovery.service.ts`: `DRIFT_AUDIT_FILE = 'audit.yaml'`, `TenantInfo.driftAuditPath`
  (comment: read as data, never served), set where `driftObservationPath` is. `drift.service.ts`: a fourth
  `FileCache<DriftAudit | undefined>` bounded like the others and `async audit(info)` reading
  `info.driftAuditPath` through `parseAudit`.~~
- ~~**View models** (`drift-view.ts`) — `STATUS_TONE: Record<AuditStatus, Tone>` = `matched: success`,
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
  `{ outdated: true }`; for `none` — `null` (no line).~~
- ~~**Templates** — `drift-tenant.hbs`: after the link and "was …" in each `<li>`, a
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
  one amber line. Every state with its `dark:` variant; nothing new in `styles.css`.~~
- ~~**Controller wiring** — `driftTenant`: read `this.drift.audit(info)` only when the observation is `current`,
  compute `auditState`, pass the audit to `observationSummary`/`findingGroups` only in the `current` audit
  state, plus `actors: byActor(...)`. `renderDrift`/`findingView`: same gate, `attribution:
  attributionOf(finding, activeAudit)` and `attributionOutdated` for the header caveat;
  `unknownType`/`notComparable`/`unchanged` pages show no attribution (the audit has no entry for them by
  construction). HTTP only — no filesystem logic in the controller.~~
- ~~**Tests** — `test/drift-audit.spec.ts` (inline YAML fixtures): parses the contract shape and indexes by key;
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
  `audit.yaml` in the zip).~~
- **Changed-by cells** (`drift-view.ts`) — pure `changedByCells(obs, audit)` →
  `{ cells: Map<key, { text, tone: 'matched' | 'warning' | 'quiet' }>, fingerprint: string }`, one entry per
  finding **of the observation** (keys are `driftKey`'s extensionless `<type>/<name>`), built from the same
  `rowAttribution`/`attributionOf` as the tenant page's row suffix so the two can never disagree: `matched` →
  `<actor> · <at>` plus ` (+N more)` when `more > 0`, `actor` already `unknown actor` for an empty one, tone
  `matched`; any other status → its `STATUS_TEXT`, tone `warning` for the amber statuses and `quiet` for
  `no-join-key`/`not-queried`; a finding the audit does not name → "no attribution recorded", tone `quiet`.
  `fingerprint` is `JSON.stringify` of the entries sorted by key — it changes exactly when a rendered cell
  would. Called only with the `current` audit.
- **Changed-by column** — new pure, Nest-free `applyChangedBy(tokens, cells, makeToken)` in
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
- **Renderer and wiring** — `markdown-renderer.service.ts`: `render(file, env: RenderEnv)` with
  `RenderEnv = LinkEnv & { changedBy?: { cells, fingerprint } }`; the `md.render` env passes `changedBy`
  through beside `tenant`/`docDir`/`routeBase`; `CacheEntry` gains `fingerprint: string | undefined` and a
  hit requires it to equal `env.changedBy?.fingerprint` strictly (so a render with cells is never served to a
  request without, and vice versa). `docs.controller.ts` `driftTenant`: when `activeAudit` is set, pass
  `changedBy: changedByCells(current, activeAudit)` in the `renderSplit(info.driftIndexPath, …)` env
  (`renderSplit`/`renderOptional` take `RenderEnv`); no audit or an outdated one → no `changedBy`, the table
  renders exactly as today (the outdated caveat is already in the observation header). `renderAnalysis` (the
  per-resource drift documents), the documentation pages and `export/export.service.ts` pass none.
- **Tone styles** — `src/styles.css`, in the drift findings table block: `.findings-drift
  td[data-attribution="warning"]` amber (the `text-amber-700` / dark `text-amber-300` hues),
  `[data-attribution="quiet"]` slate (`text-slate-500` / dark `text-slate-400`), `[data-attribution="matched"]`
  inheriting the body colour; the dark hues under `@media (prefers-color-scheme: dark)`; the column is kept
  `white-space: nowrap` for `matched` so `actor · at` does not break mid-timestamp.
- **Tests for the Changed-by column** — `test/findings-table.spec.ts` (new; hand-built tokens over the
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
- **`README.md`** — docs-root contract tree gains `│   ├── audit.yaml             # attribution — read, never
  served` under `drift/`, and the root-level rule names it among the unreachable files; **Drift view** gains an
  **Attribution.** bullet (what the file is, the validity rule, the per-status lines, By actor, the Changed by
  column in the analysis index's findings table and its join through the row's link, that nothing is derived); routes table unchanged; Tests table adds `test/drift-audit.spec.ts` and extends the e2e row; Project
  layout adds `drift-audit.ts` and updates the `drift.service.ts` line. With it, at *done*: `web/CLAUDE.md`
  and `.windsurf/rules/01-architecture.md` name `drift-audit.ts` among the drift modules and
  `drift/audit.yaml` beside `drift/analyze.md` as read as data, never served.
- **`CHANGELOG.md`** — under `[Unreleased]` → `### Added` → new `#### Drift view` subsection (today `Drift
  view` exists only under `### Changed` and `### Fixed`).

## Fixes

Each is a numbered work entry in its own right; none touches a non-negotiable (read-only, no client-side
JavaScript, one `markdown-it` instance, path safety) and none depends on a documentation regeneration. Each
carries its own e2e or spec case and a `CHANGELOG.md` entry under `[Unreleased]`; purely internal ones say so.

A **struck-through** title or plan item has shipped and its `CHANGELOG.md` entry is written. It stays here,
struck, until the entry is done — then it is archived to `../.claude/archive/web/` with its full plan, never
deleted, and the remaining entries are renumbered.

None scheduled.

## Standing decisions

Decisions that are not work items but constrain the ideas below, recorded so the next iteration does not
relitigate them.

### Export entry points live on the tenant picker

**Decision.** Every export a *whole tenant* produces is offered on the tenant picker (`GET /`), on that
tenant's card, as a plain `<a download>` with the one-way-publish caveat beside it. Not on the tenant
landing page, not in the top bar, not on document pages.

**Why.** The landing page belongs to `docs/summary.md` — it is documentation, and the view adds no chrome of
its own to it. The picker is where a tenant is chosen *as a whole*, which is exactly the scope an export
operates on, so the button sits with the noun it applies to and stays out of the reading flow. It also means
one place to look per tenant instead of a control repeated on every page.

**How it extends to the planned types.**

- **Further whole-tenant formats** (single-file HTML, DOCX, PDF, Markdown bundle — see the parked idea):
  additional sibling links on the same card, under one `Export:` label once there is more than one. **No
  dropdown, no picker widget** — that needs client-side JavaScript, which is a non-negotiable (dropping that
  rule is its own parked idea; until it is actually dropped, this decision stands as written). If the row of
  formats ever stops fitting, the answer is a per-tenant export *page* (`GET /:tenant/_export`) listing the
  formats, not a control that needs scripting. A format that grows **options** is a second, earlier reason to
  reach for that page — see the parked idea on a single export button per tenant.
- **Partial exports** (one resource type, one document, summary only): these are the one case that must
  *not* be on the picker, because the picker cannot express the scope. Their entry point belongs next to the
  thing being exported — the document top bar next to the **Documentation | YAML** switcher for a single
  document, a sidebar section header for one type — and the picker keeps whole-tenant formats only.
- **Media and source YAML as attachments**: no entry point of its own. It changes what an existing export
  *contains*, never where it is offered.
- **Confluence REST API synchronisation**: not a download, and it mutates a remote system, so it cannot be
  an `<a>` at all — it needs a POST, and no route may mutate state today. It gets no control until that
  design change is actually made, and if it ever does it must be visibly distinct from a download rather
  than sitting in the same row.

**What any new entry point has to keep.** A plain anchor with `download` and no client-side JavaScript; the
route shape `/:tenant/_export/<format>` behind the `_export` representation prefix, declared before the
document catch-all; the one-way caveat rendered next to the link rather than only in the README;
no anchor nested inside another anchor (the picker card is a wrapper element for exactly this reason); and a
dark-mode variant plus a visible `:focus-visible` outline.

## Parked ideas

### Idea: Per-document identity on the article

Put `data-family` and `data-type` on `<article>` so the stylesheet can treat a group, a credential and a
settings-catalog policy differently — a credential document leading with its expiry, a `record` document
dropping the assignment vocabulary it never uses, `[data-family="group"] [data-section="properties"]`
differing from the same section on a policy. **Parked** because no reader has asked to tell the families
apart, and the attribute is worthless until per-family CSS exists, so it would ship as decoration.
**Revisit** when a concrete per-family styling need appears; then add the attribute and the CSS together.

What is settled if it is picked up: `data-type` is free (the document's own directory), while `data-family`
cannot come from the index or the frontmatter — `IndexResource` has no family or prompt-template field and the
frontmatter carries only `source` and `generatedAt`. The carrier is the document's **own H2 set**, which
partitions the 414 reference documents cleanly: 333 *References | Lifecycle and operations | Security |
Settings* (the `default`/`singleton` contract, indistinguishable from each other by headings alone), 36
`group`, 25 `referenced`, 6 `record`, 4 `credential`, 0 `arm`. Ignore the spliced `Targeted by` / `Used by`
headings, emit nothing when the set matches no contract, never infer the family from the resource type, and
take the headings from `markdown-it`'s tokens the way `section-hooks.ts` does — at least one document has a
`##` line inside a fenced code block, which a line-based regex would miscount.

### Idea: An actionable findings block

Let a reader of the tenant landing page narrow the findings table to a severity and get from a finding to the
documents it affects. **Parked** because the table is 15 rows in the largest reference export — short enough
to read whole — so filtering it buys little, and the `Documents` column already links to every document a row
names, which leaves the inert `Affected` count as the only real gap. **Revisit** when a summary carries enough
findings that the table stops being readable in one pass, or if the no-client-side-JavaScript rule is relaxed
(its own idea below), which would replace the `:target` construction described next with an ordinary filter and
give the `#findings` fragment back.

What is settled if it is picked up: the hooks exist (`src/docs/findings-table.ts` tags the table `.findings`
and puts `data-severity` on each body row and severity cell, with lowercase severity ids), and the filter is
expressible **without JavaScript** — renderer-emitted sibling anchors plus `:target`, e.g.
`#sev-critical:target ~ .findings tbody tr:not([data-severity="critical"]) { display: none }`. That requires
the anchors to be siblings of the table, so a wrapper is a prerequisite, and it spends the URL fragment that
already belongs to `#findings`, so a **Show all** reset is part of the feature rather than a refinement. One
caveat: `.prose table` sets `white-space: nowrap` so wide assignment tables scroll instead of wrapping
mid-GUID, and any prose-bearing table needs an opt-out from it.

### Idea: Summary table of contents

A jump list on the tenant landing page so a reader can go straight to the part of the summary they came for.
**Parked** because the summary is four H2s and two H3s long — a table of contents for six anchors competes
with the document it indexes for the top of the page. **Revisit** if the summary contract grows, or if readers
report scrolling past the management summary to reach the caveats.

What is settled if it is picked up: the four H2s are a declared CLI-side contract with stable slugs
(`#management-summary`, `#at-a-glance`, `#assignment-posture`, `#coverage-caveats`) plus the `#findings` and
`#recommendations` H3s, all verified present in both reference exports and all emitted as ids by
`markdown-it-anchor`, so the list is hard-codeable with no renderer change. Skip entries whose heading is
absent so an older summary degrades to a shorter list, and omit it entirely in the index-listing branch where
there is no summary at all.

### Idea: A name filter and per-item context in the sidebar

Narrow the sidebar by part of a name (a server-side query parameter, composing with the shipped taxonomy
filters rather than replacing them), and give each tree item the context the listing fallback already shows.
**Parked** because the taxonomy filters cut the 263-item tree to a workable size along the axes that matter,
which was the pressing half of the problem, and because a name filter without a text input is an awkward thing
to offer — no client-side JavaScript means no type-ahead. **Revisit** if narrowing by axis proves
insufficient, alongside the search idea below, which subsumes it, or if the no-client-side-JavaScript rule is
relaxed (its own idea below): a text input with type-ahead, and remembering which sections were open, are the
two halves that rule is holding back.

What is settled if it is picked up: **badges are available today** — `assignments` (231 of 263), `scope` (93),
`platforms` (73), `odataType` (137), plus the facet memberships the filter already renders. The per-item
**summary is not**: it is present on **0 of 263** and **0 of 148** resources, because `GenerateIndex` reads it
from each document's frontmatter and generated documents write only `source` and `generatedAt`. The index
schema and the CLI plumbing are both correct, so that half is gated on a documentation **regeneration** whose
template emits `summary:`, not on a change here — build it to render no second line when the field is absent
and it lights up on its own. Remembering which sections were open across navigations stays **excluded on
purpose** for as long as the no-client-side-JavaScript rule stands: that needs client-side state.

### Idea: Structure the sidebar by a taxonomy axis instead of by resource type

Let a reader reach a document by what it *is* — a Windows policy, a device-scoped configuration — rather than
by the Azure/Graph type that happens to implement it, so "how are Macs hardened" does not require knowing the
answer is spread over `deviceConfigurations`, `deviceManagementConfigurationPolicies` and
`deviceShellScripts`. Filtering narrows the tree; this would replace its *structure*, which filtering
deliberately leaves alone. **Parked** because the shipped axis filters already answer the same question from
the other direction — selecting *Platform: macOS* yields exactly that reading list — so a second structure
would be a large change to the navigation for a smaller marginal gain. **Revisit** if readers keep reaching
for the filter as a substitute for structure, or once an axis exists dense enough to carry a spine on its own.

What is settled if it is picked up. **The spine is a facet axis, not the model's grouping fields**: the
`index.yaml` the CLI writes today (schema version 4) carries the header `facets` registry and per-resource `facets` (`axis id → value ids`), and both
exports declare `programme`, `platform` and `scope` — curated, deterministic, id-bearing and ordered by the
header, which is everything a tree spine needs, on exports that exist today. `platformGroup`/`functionGroup`
stay badges: single-valued, label-only, and empty in both exports. There is no `function` axis, so a
function-shaped spine needs an operator-authored axis in the CLI config or `functionGroup` — not
classification here. **An axis is not a partition**: 55 of 263 resources hold several `programme` values, so
some appear under more than one group (show them in each and say so; picking a "primary" value is a judgement
this project may not make), and axes are sparse — 63 of 263 carry no `platform`, 170 no `scope` — so the
uncategorised group is **always rendered**. **The grouping must not be derived here**: `confluence.ts` is a
second consumer of the same index, and anything computed inside `buildNavigation()` is invisible to it, so an
exported space would silently group differently from the browser — which also means the export's own grouping
has to be decided explicitly rather than left to drift. What is cheap: hrefs come from the index `doc` field,
so **restructuring changes no URLs**, and `sidebar.hbs` renders sections purely from data, making the spine a
data change plus one nesting level; the choice belongs in the URL as a query parameter, and stays there even if
the no-client-side-JavaScript rule is relaxed (its own idea below) — a spine chosen by a widget would not be
addressable, which is a property worth keeping on its own merits.

### Idea: Search across a tenant's documents

Full-text search, or filtering by resource type and name, over everything a tenant has (including the
resource tree). **Parked** because it is the largest single feature on this list and cannot be done well
without an index and, realistically, client-side interaction — and no client-side JavaScript is a
non-negotiable. **Revisit** when the corpus is large enough that the sidebar tree stops being navigable even
with the shipped taxonomy filters, or if a server-rendered query page turns out to be enough. It subsumes the
name filter in the sidebar idea above. This is the **first feature that would justify relaxing the rule** (its
own idea below): a server-rendered `GET /:tenant/_search?q=` page is worth trying first, because it needs no
script at all and would show whether the interactive version is wanted.

### Idea: Syntax highlighting inside documents

The `yaml`/`bash`/`powershell`/`json`/`xml` fences *inside* the generated Markdown are unstyled.
`@shikijs/markdown-it` could reuse the highlighter `YamlHighlighterService` owns, with the extra languages
loaded. **Parked** because it costs render time on every document for 182 fences in the whole reference
corpus. **Revisit** if documents start carrying substantial code, or once the highlighter is warm anyway for
other reasons.

### Idea: Multi-segment tenants

Discovery walks up to 3 levels, but routing uses a single `:tenant` segment, so only top-level tenant
folders are addressable. **Parked** because nested tenants would need a different route shape, which
interacts with every `_`-prefixed representation prefix. **Revisit** the first time a real export tree nests
tenants under a grouping folder.

### Idea: Explicit dark-mode toggle

Theme selection follows `prefers-color-scheme`, with no way to override it. **Parked** because remembering a
choice needs either client-side state or a cookie plus a mutating route, both of which cut against the
no-JavaScript and read-only rules. **Revisit** if a reader needs one theme in a browser set to the other,
e.g. for a presentation or a screenshot, or if the no-client-side-JavaScript rule is relaxed (its own idea
below) — a toggle that stores the choice client-side is the cheapest thing that relaxation would buy, and it
needs no route and no state on the server.

### Idea: Watch-based cache invalidation

An `fs.watch` layer could pre-warm and evict cache entries instead of validating them per request. **Parked**
because the per-request `stat()` delivers the no-restart freshness invariant at negligible cost. **Revisit**
if `stat()` becomes measurable on a slow or networked docs root.

### Idea: A resource landing page

A new `GET /:tenant/_resource` listing every source YAML (index-derived, plus the excluded types), linked
once from the sidebar footer. Sidebar untouched, no extra tree. **Parked** because it costs a route and a
view to reach resources the top-bar **Documentation | YAML** switcher already gets to. **Revisit** if the
switcher proves hard to find, or as the carrier for browsable excluded bulk types (below).

### Idea: A two-group navigation tree

`buildNavigation` returns two top-level `NavGroup`s (*Documentation*, *Resources*), each holding the per-type
`NavSection[]`, with only the group containing the current page `open` and the active marker becoming
`{ kind: 'doc' | 'resource', path }`. Both trees built in one index pass with two href shapes, so they cannot
drift. **Parked** because the resource tree duplicates the doc tree (263 items twice in the reference export)
and `sidebar.hbs` gains a second `<details>` level. **Revisit** only if the **Documentation | YAML** switcher
turns out to be too hard to find and a resource landing page does not fix it.

### Idea: Clickable breadcrumb segments

Turn `Microsoft.Graph / depOnboardingSettings` in the breadcrumb into links to a per-type listing page.
**Parked** because it needs a new route and view: CSS alone cannot open a collapsed `<details>` section from
an anchor, so linking into the existing sidebar tree is impossible without client-side state. **Revisit**
after a per-type listing page exists for another reason, when this becomes additive and nearly free, or if the
no-client-side-JavaScript rule is relaxed (its own idea below), which removes the reason for the new route
entirely: the segment could then open and scroll to its own sidebar section.

### Idea: Browsable excluded bulk types

Types the index merely counts under `counts.excluded` (Autopilot identities and the like), and any resource
with no document, are unreachable from the navigation — which is what keeps navigation purely index-derived
and the **"counts and listings derive from the index, never from walking the tree"** non-negotiable intact.
Unreferenced `Microsoft.Graph/groups` (the CLI documents a group only when an assignment references it) are
in the same bucket. They are not unservable: the YAML view serves any `.yaml` under `resources/` by URL and
never consulted the index, and the scheduled tenant-compare entry lists and links them from
`resources/metadata.yaml` read as data — neither walks the tree, so neither touches this idea. **Parked**
because *navigating* to them from the index-driven tree requires amending that non-negotiable. **Revisit** if
operators ask for the raw bulk YAML; the shape is settled — for each type named in `counts.excluded`, a single
**non-recursive** `readdir` of `resources/<type>/` (`.yaml` only, sorted, cached under the existing discovery
TTL, unreadable ⇒ empty), producing file names only, with counts still taken from the index. It reopens two
UX questions: whether a `readdir`-vs-`counts.excluded` mismatch should be flagged as a stale index, and
whether resources without documentation should be visually de-emphasised.

### Idea: Media and source YAML as page attachments

Confluence's HTML import gives each page a media folder named after it, which is where the two images in the
reference corpus — and, in principle, each document's source YAML — could travel. **Parked** because
`resolveWithinRoot()` serves exactly **one** extension per root (`.md` under `docs/`, `.yaml` under
`resources/`), and widening that to an extension list is a non-negotiable: images currently travel as their
`alt` text and the export attaches nothing. Doing it properly means a third served root with its own single
extension policy — a design change, not a feature. **Revisit** if generated documents start carrying
diagrams that matter, or if readers of an imported space ask for the YAML next to the page. It gets no entry
point of its own either way (see *Export entry points live on the tenant picker*).

### Idea: Confluence REST API synchronisation

Create and update pages in place, with labels, a page tree and attachments — the proper answer to "keep
Confluence up to date". **Parked** because it is a much larger feature than HTML import: authentication, a
persisted mapping from resource to page id, and conflict handling, the last two of which sit awkwardly with
a read-only browser that stores no state. **Revisit** once one-way HTML publishing is in real use and its
re-import cost is felt. Note that it cannot reuse the export link's shape at all — it mutates a remote
system, so it needs a POST rather than an `<a download>` (see *Export entry points live on the tenant
picker*).

### Idea: Further export formats and partial exports

Single-file HTML, DOCX, PDF via a print stylesheet, a Markdown bundle; and exporting one type, one document
or only the summary. **Parked** deliberately: the Confluence exporter is whole-tenant only, and the
`src/docs/export/` seam exists so a second format is a second `ExportService` method plus its own format
module, with the controller and the serialiser untouched. **Revisit** per format when someone actually needs
it. Preserving the document tree in an export is not expressible through Confluence HTML import at all —
re-parenting by hand or the REST API are the only routes. Where each of these would be offered is already
settled, including why the partial exports are the exception: see *Export entry points live on the tenant
picker*.

### Idea: One export button per tenant, leading to an export page with per-format options

Replace the per-format download links on the tenant picker with a **single** *Export* link per tenant card,
pointing at a new HTML page (`GET /:tenant/_export`) that lists the available formats and lets the operator set
that format's own options before downloading. First concrete option: the **Confluence index format** — group the
overview's page list by resource type, by a taxonomy axis, or both. **Parked** because there is exactly one
format today, and its one real option is settled per *server* rather than per request: the Confluence overview
index (by type, by axis, or both) is chosen by the `EXPORT_INDEX` environment variable, so the page would still
be a route, a view and a control for a single button that already works. **Revisit** the moment a second whole-tenant format
appears, or a reader needs two different exports of the same tenant from one server — which is exactly what an
environment variable cannot express, and the honest trigger for making the choice per request.

What is settled if it is picked up. **The download route keeps its shape**: `GET /:tenant/_export/:format`
streams the archive, options ride as query parameters on it, and every option has a default so a bookmarked
option-less URL keeps producing today's export. The new page is the *only* addition, at the bare `_export`
prefix, declared before the document catch-all like its sibling. Options are **validated the way
`parseFacetSelection()` validates a selection** — an unknown or malformed value falls back to the default rather
than 404ing — and each is a named choice with a stable id, so a chosen variant has exactly one URL. The page is
also the honest place for the things currently crammed onto a picker card: the one-way-publish caveat stated once
per format, and, cheaply, what the export will contain (page count, pending documents, an incomplete index),
all index-derived.

One open question, plus one that is now settled elsewhere. **How the controls are expressed** is undecided, with
three candidates. **(a)** A plain
`<form method="get" action="/:tenant/_export/confluence">` with radio buttons and a submit button — pure HTML,
no script, still read-only, and it scales to several options; it costs the app's first form and first non-anchor
control, and a form cannot carry `download`, so attachment behaviour rests on the `Content-Disposition` header
`ExportService` already sets. **(b)** One `<a download>` per option combination ("Confluence, index by type",
"Confluence, index by axis") — keeps the anchor-only shape the standing decision names, but is combinatorial as
soon as a format has two options. **(c)** Anchors first, with the GET form named as the sanctioned escape hatch
once a format carries more than one option, and the query-parameter shape fixed up front so swapping the control
changes no URL. Note that **(a) does not conflict with the no-client-side-JavaScript rule** — that rule bans
shipped script and client-side state, not HTML controls; the objection to it is precedent, not compliance.
Second, the axis-grouped index no longer poses an open question: it shipped as the `EXPORT_INDEX` environment
variable (`type` | `both` | `axis`, default `type`). If this page is built, the
same three ids become that format's query parameter with `EXPORT_INDEX` as its default, so an option-less URL
keeps producing what the operator configured — which makes the index format a *migration* of an existing choice
rather than the first option, and moves the trigger for the page to a second format or a per-request need. Note
that because the variable defaults to today's by-type index, a reader who wants the axis view on a server
configured for `type` is the most likely first caller for this page.

This amends *Export entry points live on the tenant picker*: the standing decision already names
`GET /:tenant/_export` as the answer when a row of format links stops fitting, and this makes per-format
options a second, earlier trigger for the same page. What the decision requires is unchanged — the picker stays
the entry point for whole-tenant scope, the link is a plain anchor with no nesting, the caveat travels with it,
and dark mode plus a visible `:focus-visible` outline apply to whatever control the page ends up using.

### Idea: Drop the no-client-side-JavaScript rule

Lift the non-negotiable that this app ships no script, so the features currently blocked by it become possible.
It is stated in `.windsurf/rules/01-architecture.md` (*"No client-side JavaScript. Everything is
server-rendered"*), repeated in `02-style-and-quality.md` (*"Do not introduce client-side JavaScript or a
frontend framework"*) and claimed in `README.md`'s Frontend section. **Parked** because it is a rule change, not
a feature: nothing is unblocked until a specific blocked feature is actually wanted, and every one of them is
itself parked. **Revisit** when a feature someone has asked for cannot be built server-side at acceptable cost —
tenant-wide search is the honest candidate — or when the alternative has become visibly worse than the script
would be (a `:target` hack, a route invented only to compensate, a workaround nobody can explain). The tenant
compare's IDE-style comparison pane was checked against the rule and needed no script: statuses are computed
on the server, selecting a row is a link, the selected row is scrolled to with a fragment and the pane is
resized with CSS.

**Note what the rule does and does not forbid.** It bans *shipped script* — no `<script>`, no bundler, no
framework, no client-side state. It does not ban HTML interactivity: `<details>`/`<summary>`, `:target`,
`:focus-visible`, `prefers-color-scheme` and a `<form method="get">` are all in bounds today. Several things that
feel scripted are already legal without it.

**Where the rule earns its keep.** Server-only rendering is a real benefit in its own right and not merely the
absence of a client — a document arrives complete in the first response, so there is no loading state, no
hydration, nothing to re-render and nothing that can fail after the HTML has landed — but it is a *separate*
claim from banning script, and only the reasons below argue for the ban rather than for server rendering.
*The corpus is documents.* A configuration document has to survive printing, archiving, a text browser and a
saved copy; server-rendered HTML is that durable form, and anything only a runtime can assemble is not.
*It has no client toolchain.* The only build step is Tailwind's CSS pass — no bundler, no framework dependency
tree, no client supply chain to audit or keep current in a tool that renders tenant configuration. *It is
trivially auditable.* "This app ships no script" is a claim an operator can verify at a glance, and it composes
with the read-only rule to make the browser obviously inert; note that it is **not** an XSS boundary, because
`markdown-it` runs with `html: true` and although the CSP now refuses to run script, raw HTML from the docs
root still renders, so the real boundary is the trust placed in the docs root either way. *It forces state into
the URL.* Every view — including a filtered sidebar — is addressable,
bookmarkable, shareable and reproducible, and the whole test suite can therefore be `supertest` against server
HTML rather than a browser harness. *It caps complexity*: one rendering path, and no logic duplicated across
server and client.

**What it has already cost.** Working machinery exists purely to route around it: `toggled()` and
`selectionHref()` rebuild the entire facet selection into every chip and every document link, so a 263-item tree
re-renders server-side on each click; the `exempt` flag plus the `matched`/`total` reconciliation exist so a
filter cannot hide the page you are on; `NavSection.active` exists to render one `<details open>`; `lineAnchors()`
plus `.line:target` deliver `#L42`; `shiki` emits dual-theme output so dark mode can stay
`prefers-color-scheme`; and `findings-table.ts` already tags rows with `data-severity` for a filter that cannot
be built yet. Five parked ideas are blocked or deformed by it: **search across a tenant's documents** (the largest
item on this list, parked squarely on it), **a name filter and per-item context in the sidebar** (no type-ahead
without script, and remembering which sections were open is excluded on purpose), **an actionable findings
block** (expressible only as sibling anchors plus `:target`, which spends the URL fragment `#findings` already
owns and needs a *Show all* reset as part of the feature), **clickable breadcrumb segments** (CSS cannot open a
collapsed `<details>` from an anchor, so it needs a whole new route and view), and **an explicit dark-mode
toggle**. *Tenant diff* used to be the sixth, on the claim that a readable diff of a 317-setting document needs
interaction — the drift view's server-rendered YAML diff disproved that for the per-resource case, and the idea
has since shipped as the tenant compare, with two-click link selection and no script. The export standing
decision inherits it too: *no dropdown, no picker widget*.

**Not a binary decision, if it is picked up.** Three tiers, all open. **(a) Keep it** and pay the workaround
cost knowingly, which is the status quo. **(b) Progressive enhancement only**: a small dependency-free script
served from `public/`, no bundler and no framework, allowed only to improve something that already works without
it — remembering open sections, toggling a chip without a round trip — with every page still fully functional
with script disabled, and the rule rewritten as *"the server renders everything; script may only enhance"*
rather than deleted. Either tier also means widening `SECURITY_HEADERS`' `script-src` in the same edit — today
it is unnamed (falls to `default-src 'none'`), and it would need to name the script's own origin. **(c) Full
lift**: a real client bundle for search and diff, which brings a build step, a dependency tree and a second
rendering path, and turns the testing story into a browser harness.

**What has to change with it, whichever tier wins.** Both rule files and the README Frontend section, in the
same edit — the rule is quoted in enough places that a half-removed version would be worse than either state —
plus a `CHANGELOG.md` entry, because a page that needs script to work is operator-visible. The many released
changelog entries that boast *"still no client-side JavaScript"* are history and stay as written. Two invariants
must be restated rather than dropped by accident: state belongs in the URL (so views stay addressable), and no
route may mutate anything under the docs root, however the client is built.

### Idea: Clear the ESLint suppressions baseline

Pay off the 14 findings in `eslint-suppressions.json` — what is left, in 9 files, of the ones that existed
when the sonarjs rules were switched on — a rule at a time, pruning after each, until the file, the `lint:baseline` / `lint:prune`
scripts and the paragraphs describing them can be deleted. **Parked** because the baseline already delivers the
property that mattered: every rule stays enabled, `npm run lint` and both readiness gates are usable, and new
code — including new code in the baselined files — is held to the full set, since one more violation of a
baselined rule there exceeds the recorded count and reports. What is left is latent, not broken: the regexes
backtrack over the operator's own generated export tree in a read-only app, and the complex functions are the
ones whose shape the score measures rather than their risk. **Revisit** when a baselined file is being touched
for another reason (pay its entry off in the same edit, which is how this shrinks without a campaign), when a
render actually turns out slow on a pathological document, or if the baseline ever stops shrinking — that would
mean it has become a place where findings accumulate, which is the one thing it must not be.

What is settled if it is picked up. It is a **debt ledger, not a policy**: nothing in it is a rule this project
disagrees with, and none of it is exempted in `eslint.config.mjs` — the rules switched off there for `test/` are
a separate, permanent parity decision and are not part of this. Work rule by rule and run `npm run lint:prune`
after each, so the diff shows what was paid off. Only `sonarjs/super-linear-regex` (6: `page-name.ts` ×2,
`findings-table.ts` ×2, `link-rewrite.ts`, `section-hooks.ts`) changes behaviour — these patterns run over
generated documents, so each rewrite needs a spec case pinning the same accepted and rejected inputs. The rest
are refactors the existing suite covers and are internal, carrying no `CHANGELOG.md` entry unless a reader sees
a difference: `cognitive-complexity` (4: `confluence.ts`, `export/html-allowlist.ts`, `section-hooks.ts`,
`tenant-index.ts`), split along the seams those functions already have and keeping the pure/Nest-free split
intact; `misplaced-loop-counter` in `page-name.ts`, a `while` written as a `for`; `no-nested-template-literals`
in `confluence.ts`; and `prefer-specific-assertions` in two specs. One is a decision rather than a fix, and
either way it moves **out** of the baseline: `updated-loop-counter` in `findings-table.ts` (the scan assigns
`i = close` to skip a matched table's body — deliberate and documented in place); if accepted, it becomes an
`eslint-disable-next-line` at its site, because a baseline must not be where a standing choice hides — exactly
what happened to `no-os-command-from-path`, which left the ledger when the git calls moved into
`scripts/lib/git.js` behind one directive with its reason. Deleting
the file and the two scripts is the last step and is the only operator-visible part, so that one does get a
`CHANGELOG.md` entry.

### Idea: Manual pairing and a rename heuristic for the tenant compare

The tenant compare pairs resources by their export path (`<APIType>/<endpoint>/<name>`), a heuristic the page
states: two tenants can hold different policies under the same name, and a renamed policy shows as one row only
in each side. Let a reviewer pair an only-in-a row with an only-in-b row by hand (`?left=<key>&right=<key>`, two
steps via links, both keys still required to be listed as present and located only through `resolveResource`),
and offer a **rename heuristic** that suggests such a pair when the two files' normalised hashes are identical —
the per-file digests the comparison pane already keeps make that a lookup, not a read. **Parked** because the
compare is a proof of concept and nobody has yet reported a mispaired or split resource in a real review.
**Revisit** when a reviewer does, or when the one-sided rows of a real pair turn out to be mostly renames.

### Idea: Move the compare normalisation to the CLI

The cross-tenant identity rule (`src/docs/compare-normalise.ts`) was accepted in the browser on one condition:
it is provisional and moves to the CLI once it is stable. The CLI side is the parked *`resource compare`* idea
in `../go/NEXT-ITERATIONS.md` — reading two exports offline, applying the rule, emitting verdicts, dotted-path
deltas, payloads and an `analyze.md` so the drift-analysis agent can judge the *impact* of each difference.
The browser then renders that tree the way it renders `drift/` and stops normalising itself; the comparison
pane's statuses come from the tree instead of the digest cache. Two questions travel with it: **where the
comparison tree lives** (everything today is under `<output>/<tenant>/`, and a comparison belongs to neither
tenant), and that the rule must be *statable and testable* before it is frozen. **Parked** until real reviews
have stopped changing the rule. **Revisit** when the drop list and reference fields have held unchanged across
several real stage/prod pairs; until then any change to the rule is mirrored into that Go idea, which quotes it.

### Idea: A one-sided resource in the compare's diff area

Selecting a `←` / `→` row in the comparison pane leaves the page for that tenant's YAML view. The reference IDE
instead shows the one file in the area under the pane, so the reviewer keeps their place. It needs the shared
highlighter on the compare pages and a way to say *this side only* in the URL, located through the existing
`resolveResource` and escaped like the YAML view. **Parked** because the YAML view already answers the question
and one-sided rows are read far less often than differing pairs. **Revisit** if reviewers work through the
one-sided rows of a comparison as routinely as the differing ones.

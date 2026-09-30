# Next iterations

Outstanding work, standing decisions and parked ideas for the docs browser. `README.md` describes what it does
today and `CHANGELOG.md` records what shipped; neither is repeated here.

The *Features* below are scheduled design work, promoted from *Parked ideas* and refined against what is true
now. The *Fixes* below are scheduled too, but smaller: self-contained corrections that need no design work,
listed so they are not forgotten between features. Every idea in *Parked ideas* is deliberately unscheduled:
picking one up means promoting it into a numbered work entry with a `**Goal.**` and a `**Plan.**`, reconciling
its rationale against what is true at that point rather than copying it across.

## Features

### 1. Export the drift report as a single PDF

**Goal.** An operator can download a tenant's current drift report as one self-contained PDF from the tenant
drift page — everything the drift pages show about the observation, the analysis and every finding, with
nothing hidden behind a collapsed `<details>` — to hand to someone who has no access to the browser or the
export tree.

> **Why a library, not print CSS or a headless browser.** Printing the drift pages drops every collapsed
> `<details>` (CSS cannot open one; "All recorded findings" is collapsed whenever an analysis exists) and
> needs one print per page. A headless Chromium adds ~150 MB, a sandbox and a second rendering stack. `pdfmake`
> is pure JavaScript, builds the document in memory — read-only holds, no temporary file — and can produce
> deterministic bytes. Accepted trade-off: the PDF is laid out in its own right, not a pixel copy of the page.
>
> **Scope.** Only a *current* observation is exported: when `tenantDriftState` is `none` or `superseded` the
> link is not rendered and the route answers 404. No YAML payloads and no line diffs — the same line the
> Confluence export draws at source YAML; the "What changed" deltas table is included.
>
> **Entry point.** The drift report is a *scoped* export (the drift observation, not the tenant's
> documentation), so under *Export entry points live on the tenant picker* it sits next to the thing it
> exports — the tenant drift page — not on the picker; that standing decision now says so. The parked idea on
> one export button per tenant is not triggered: this is not a second whole-tenant format.
>
> **Relation to the parked idea "Further export formats and partial exports".** A documentation PDF stays
> parked there; once this ships, the `pdfmake` dependency and the HTML→PDF content walker exist, so it would
> be a second format module rather than a new engine.
>
> No regeneration gating (web only); no go counterpart.
>
> **Glyph coverage.** The bundled Roboto covers Latin, Greek and Cyrillic; CJK and emoji render as a missing
> glyph. Courier (a PDF standard font) encodes WinAnsi only, so code runs outside it fall back to Roboto.
> Both are known limitations, not bugs. Checked against the pdfmake 0.3.11 Roboto (fontkit
> `hasGlyphForCodePoint`) during plan review: it lacks `→` U+2192, `←` U+2190, `↔` U+2194, `⇒` U+21D2,
> `✓` U+2713 and `✗` U+2717 — these six are substituted with ASCII in the PDF. It has `≠`, `≤`, `≥`, `…`,
> `•`, `—` and `·`; none of them is WinAnsi, so a code run holding one already falls back to Roboto and
> renders it — they are not mapped. It has U+200B (zero-width space) with zero advance, the table break
> character; standard Courier cannot encode U+200B (pdfkit would write its code unit as garbage bytes), so
> it never appears inside a Courier run. The break limit applies per inline run: a token spanning two runs
> without whitespace (`` `settings`.enabledForAllUsers ``, a link followed by a suffix) is not cut across
> the run boundary — a known limitation. Copied text carries U+200B at the break points, and a table row
> taller than a page is not broken.
>
> **Decision.** Which PDF engine: pure-JavaScript `pdfmake` — no headless browser, no print-CSS route.
>
> **Decision.** What the PDF contains: the full drift report, without YAML payloads and without line diffs.
>
> **Decision.** Where it is offered: the tenant drift page only, not the tenant picker.
>
> **Contract.** Web-only; it reads the CLI's drift tree exactly as the drift view already does and asks
> nothing new of `azure-rd`: `drift/metadata.yaml` (observation — `observedAt`, `toolVersion`,
> `baselineGeneratedAt`, `baselineToolVersion`, `complete`/`incompleteReason`, `counts`, `unknownTypes`,
> `removalsSuppressed`, `notComparable`, `findings` with `deltas`/`deltaNote`), `drift/audit.yaml`
> (attribution, read as data), `drift/index.md` (analysis summary) and `drift/<key>.md` per finding
> (frontmatter `severity`), validity-gated against `resources/metadata.yaml`'s `generatedAt` as today. No file under the export tree is added, renamed or written. The download URL is
> `GET /:tenant/_export/drift-pdf`, the filename `<tenant>-drift.pdf`; the same observation yields the same
> bytes. The layout follow-ups (table widths and break points, unbroken rows, symbol substitution) change
> only the PDF's layout and text; the route, the filename, the files read and the drift pages stay as they
> are.
>
> **Owner.** none — every file is under `web/`; no sequencing.
>
> **Implementer.** opus

**Plan.**

- ~~**Dependency.** `pdfmake` `^0.3` (0.3.11 current; CommonJS `js/index.js`, so a plain import, not
  `dynamicImport`) in `dependencies`, `@types/pdfmake` `^0.3` in `devDependencies`. Fonts: the Roboto TTFs
  the package ships under `pdfmake/fonts/Roboto/` (located with `require.resolve`, registered with
  `setFonts`) for text, and the pdfkit standard font Courier for `code`/`pre` runs that are WinAnsi-encodable
  (any other run stays in Roboto). No network, no font files under `public/`.~~
- ~~**Configure the pdfmake singleton once**, in the owning service's constructor or `onModuleInit`, never per
  request: `setUrlAccessPolicy(() => false)` and `setLocalAccessPolicy()` allowing exactly the four Roboto
  font files. Both are required — without them pdfmake `console.warn`s on every `createPdf` (no per-request
  logging, `no-console`) and would fetch URLs or read local files named in a definition. The document
  definition never carries `image`, `svg` or a URL.~~
- ~~**One view model for the page and the PDF.** A `DriftReportService` in `src/docs/drift-report.service.ts`
  (registered in `docs.module.ts`) takes over what `docs.controller.ts` assembles today: the tenant report
  (`tenantDrift` state, `auditOf`, `observationSummary`, `findingGroups`, `byActor`, `changedByCells`, the
  split `drift/index.md` render) and the finding view (`findingView`, `renderAnalysis`, the severity from
  the analysis frontmatter, `attributionOf`, deltas, `intact`). The controller keeps HTTP, `driftLinks` and
  the inline payload; `ExportService` injects the same service, so the drift page and the PDF read one
  decision. The drift pages render byte-identically; the existing e2e cases stay green.~~
- ~~**`src/docs/export/pdf-content.ts`** (pure, Nest-free): an `htmlparser2` walker from the browser's rendered
  Markdown HTML to `pdfmake` content, mirroring the keep / unwrap / drop verdicts of `html-allowlist.ts` —
  headings, paragraphs, lists, tables, `strong`/`em`/`code`/`pre` kept; `<details>` always expanded with its
  `<summary>` as a bold lead line; images become their alt text; a link whose path (query and fragment
  removed) is `driftHref(tenant, key)` for a finding in the report and that carries no `yaml`/`raw`/`diff`
  query becomes an internal link to that finding's section (`linkToDestination` / `id` = a stable id derived
  from the key), any other link plain text; unknown elements unwrapped; `script`/`style` dropped. The drift
  index table's severity and **Changed by** columns come through because the input is the page's own render
  (`env.changedBy` included).~~
- ~~**`src/docs/export/drift-pdf.ts`** (pure): the document definition from the report model — cover (tenant,
  "Drift report", `observedAt`, baseline `generatedAt`, both tool versions); the observation block as in
  `partials/drift-observation.hbs` (counts, incomplete run, removals suppressed, unknown types, not comparable,
  attribution-outdated caveat); the analysis summary (`drift/index.md`); the findings grouped by type
  (`findingGroups`), each with its header (`findingHeader`), verdict and severity, attribution (events table
  or the one-line status), the deltas table, the mismatch warning when its files are not intact, and its
  analysis — "No analysis" when that document is missing or unreadable, never a failed export; then "By actor"
  (`byActor`). Footer: tenant · observed at · page n/m. Determinism: `info.creationDate` is a `Date` parsed
  from `observedAt` (the Unix epoch when it does not parse) — pdfkit derives the file ID from it — no
  `modDate`, fixed `title` "<tenant name> drift report", so the same observation gives the same bytes.~~
- ~~**`ExportService.driftPdf(info, index, res)`**: gets the report from `DriftReportService` (observation,
  audit and analysis documents through `DriftService` and `resolveDriftDocument()` only, rendered by
  `MarkdownRendererService` with the drift route base and the page's env, so the render cache stays
  shared), yields to the event loop between findings, builds the PDF in memory with `getBuffer()` and only
  then sets `Content-Type: application/pdf`, `Content-Disposition: attachment;
  filename="<tenant>-drift.pdf"`, `Content-Length` and `nosniff` and ends the response — a failed build
  answers a generic error without a half-sent attachment, a path or a message. No temporary file, nothing
  written under `DOCS_ROOT`.~~
- ~~**Route.** `GET /:tenant/_export/:format` accepts `drift-pdf` beside `confluence`; 404 for an unknown
  tenant or index (`tenant` kind), an unknown format (`export` kind) and a drift state other than `current`
  (`noDrift` kind), without leaking paths.~~
- ~~**Entry point.** `views/partials/header.hbs` gains an optional `download` slot (`href`, `label`, `caveat`)
  rendered beside the view switcher; only the `driftTenant` handler fills it, and only for a current
  observation: a plain `<a download>` "Download drift report (PDF)" to `/<tenant>/_export/drift-pdf` with
  "A snapshot of this observation; it is not updated." as visible text beside it; dark variant, visible
  `:focus-visible`, no nested anchor. Every other page passes no `download` and renders unchanged.~~
- ~~**Tests.** `test/export-drift-pdf.spec.ts` (pure, `mkdtemp` fixtures): the `pdf-content` verdicts
  (details expanded, internal vs plain links — a finding link with `?diff` stays plain —, images to alt,
  unknown unwrapped, script dropped, a non-WinAnsi code run kept in Roboto); the document definition carries
  the counts, the type groups in order, the deltas, the not-intact warning, each attribution state (matched,
  status line, outdated caveat), "No analysis" for a missing document, a Cyrillic display name,
  `creationDate` equal to `observedAt` and no `image`/URL node anywhere.
  `test/docs.e2e.spec.ts`: a current observation answers 200 `application/pdf`, attachment filename, body
  starting `%PDF-`, identical bytes on a second request, security headers, no `console.warn` during the
  export; 404 for `none`, `superseded`, an unknown format and a traversal tenant; the link is on a current
  drift page and absent otherwise, and absent from the picker and the landing page; the docs root is
  untouched. `test/styles-build.spec.ts` if
  `src/styles.css` changes.~~
- ~~**Follow-up: every table fits the A4 content width.** Found in review of a real 155-page export: almost no
  table fits. Two causes. First, `pdf-content.ts` gives every Markdown table column `'*'`, so the drift index
  table's short Severity and Verdict columns take as much room as Resource, and Resource and Changed by run
  off the right edge. Second, pdfmake never lets a `'*'` column shrink below its longest unbreakable token,
  and the cells are full of them. Examples: dotted delta paths such as
  `scheduledActionsForRule[0].scheduledActionConfigurations[0].gracePeriodHours`, GUIDs,
  `GBL_CP_PRD_…` names with underscores, UPNs and Courier type keys. In "What changed" this pushes Baseline
  and Observed off the page even though its widths are `'*', '*', '*'`.~~
  - ~~**Break points in long tokens.** One exported helper in `pdf-content.ts` (the limit a named constant,
    12 characters: a Courier run at 9 pt fits a six-column `'*'` table's ~72 pt column) turns a string
    into inline runs. A whitespace-free token longer than 12 characters is cut greedily: each segment is
    at most 12 characters and ends after the last `.`, `/`, `_`, `-`, `@`, `:` or `]` inside that window,
    or is cut hard at 12 when the window holds none. Between two segments goes a run holding only U+200B
    (zero-width space) in `font: 'Roboto'` with the segment's other properties (style, link); the segments
    keep their own font. U+200B never enters a Courier run — standard Courier cannot encode it — and a run
    that splits into segments must not end up in Roboto only because of it: the WinAnsi check runs on the
    segment text. pdfmake glues adjacent runs into one word unless the join is a Unicode break
    opportunity, and U+200B is one, so the break is taken only where a line overflows. Tokens of 12
    characters or fewer are left as one run. The copy-paste text gains nothing visible (U+200B only).
    Applied to every table cell (Markdown tables in `tableOf`, the "What changed" and events tables and
    the cover facts table in `drift-pdf.ts`), to inline `code` runs, to `pre` blocks and to `code()` in
    `drift-pdf.ts`; prose outside tables is left alone.~~
  - ~~**Content-sized widths.** In `tableOf`, a Markdown column whose longest cell text (header included,
    whitespace-normalised, after symbol substitution) is at most 12 characters is `'auto'`; every other
    column is `'*'`. The events table's `'auto'`, `'*'`, `'*'`, `'auto'`, `'*'` (When, Actor, Activity,
    Result, Correlation id) and "What changed"'s `'*'`, `'*'`, `'*'` (Field, Baseline, Observed) stay as
    they are: their overflow is the unbreakable tokens, which the break points fix.~~
  - ~~**Portrait A4 stays.** No landscape pages, no change to `pageSize` or `pageMargins`, and no table text
    below today's 8.5 pt (`table` stays 8.5, `code` stays 9). The drift pages render unchanged.~~
  - ~~**Test.** `test/export-drift-pdf.spec.ts` gets a case that fails on the current code. It builds a
    definition from the worst cases above (the dotted delta path, a GUID, a `GBL_CP_PRD_…` name with
    underscores, `jan.schumann@extern.cb-gmbh.com`, a Courier type key) in "What changed", the events
    table and a six-column Markdown table (Severity, Verdict, Resource, Changed by and two long columns).
    Walking every table cell, it joins adjacent inline runs, splits on whitespace and U+200B, and asserts
    no piece is longer than 12 characters; that no run with `font: 'Courier'` contains U+200B; that the
    cell text with U+200B removed equals the input; and that the Severity and Verdict columns are
    `'auto'` while Resource and Changed by are `'*'`.~~
- ~~**Follow-up: table rows never split across a page break.** On the real export, an events-table row
  starts at the bottom of one page and continues on the next ("jan.schumann@extern.cb-" on one page,
  "gmbh.com" on the next). Every table the definition carries — Markdown tables from `tableOf`, "What
  changed", the events table and the cover facts table — sets `dontBreakRows: true`. `headerRows` stays
  1 on "What changed" and the events table and stays the `<thead>` row count on Markdown tables, so the
  header repeats on a continuation page; the cover facts table has no header row and keeps none. A row
  taller than a page is not expected in a drift report and is accepted as a known limitation.~~
  - ~~**Test.** Walking the whole definition, every `table` node has `dontBreakRows: true`, and "What
    changed", the events table and a Markdown table with a `<thead>` have `headerRows: 1`.~~
- ~~**Follow-up: the arrow and other common symbols render as a box.** The Roboto bundled with pdfmake has no
  `→` (U+2192), so every "`a → b`" in the analyses prints `□`, in prose and in code runs alike.~~
  - ~~**Substitution map.** One exported pure function in `pdf-content.ts` maps the six symbols Roboto lacks
    (the coverage check is done and recorded in the Glyph coverage note): `→ ->`, `← <-`, `↔ <->`,
    `⇒ =>`, `✓ yes`, `✗ no`. `≠`, `≤` and `≥` are not mapped: Roboto has them, and a code run holding
    one falls back to Roboto through the WinAnsi check. The function runs on every string before the
    WinAnsi font choice and before the break points: text runs and `pre` in `pdf-content.ts`, and in
    `drift-pdf.ts` `code()`, table cells, finding labels, verdict and severity labels, attribution text,
    `deltaNote`, the By actor lines and the cover. A code run that held only a mapped symbol outside
    WinAnsi is therefore drawn in Courier after substitution. The mapping applies to the PDF only; the
    drift pages and the Markdown render cache keep the original character.~~
  - ~~**Test.** A pure test for each of the six symbols, in prose and in a code run (the code run ends up
    `font: 'Courier'`); `≠` in a code run is kept and the run is in Roboto; a walk of a whole definition
    built from fixtures holding all six finds none of them in any `text`.~~
- **Documentation** (at done). `README.md`: the route in the Routes table, a "Drift report PDF" section
  (content, scope, determinism, no payloads or diffs, current observation only), the Printing section
  pointing at it, known limitations, the layout for the new modules. `CHANGELOG.md` `[Unreleased]` → Added.

## Fixes

Each is a numbered work entry in its own right; none touches a non-negotiable (read-only, no client-side
JavaScript, one `markdown-it` instance, path safety) and none depends on a documentation regeneration. Each
carries its own e2e or spec case and a `CHANGELOG.md` entry under `[Unreleased]`; purely internal ones say so.

A **struck-through** title or plan item has shipped and its `CHANGELOG.md` entry is written. It stays here,
struck, until the entry is done — then it is archived to `../.claude/archive/web/` with its full plan, never
deleted, and the remaining entries are renumbered.

### 2. Count an archived entry as a backlog change in the branch gate

**Goal.** `npm run branch-ready` (and `make branch-ready-web`) must accept a branch that planned an entry and
archived it again: today it fails with "NEXT-ITERATIONS.md is unchanged on this branch" whenever every entry
the branch touched was both added and archived on it, because the backlog file then ends up identical to
`main`. A branch that archived an entry has visibly delivered its backlog, so that must count.

> **Why.** `scripts/lib/branch.js` sets `backlogChanged` from `git diff --quiet <base> HEAD --
> NEXT-ITERATIONS.md`, so only the net difference counts. The rule it guards — every branch that changes
> `web/` delivers, refines or adds an entry — is still met when the entry is added and archived on the same
> branch; `facts.archived`, computed a few lines further down from the files added under
> `.claude/archive/web/`, is the evidence. First seen on `feat/drift-attribution`, where the drift
> attribution entry and the page-width fix were both planned and closed on the branch.
>
> **Owner.** none — the root `CLAUDE.md` and `.claude/rules/next-iterations.md` wording is updated by the go
> entry of the same name; no sequencing between them.
>
> **Implementer.** sonnet

**Plan.**

- `scripts/lib/branch.js` exposes the fact the gate needs: the backlog counts as touched when
  `NEXT-ITERATIONS.md` differs from the merge-base **or** `facts.archived` is non-empty. `scripts/branch-ready.js`
  check 6 uses it; its ok line says which ("NEXT-ITERATIONS.md changed on this branch" or "NEXT-ITERATIONS.md
  delivered on this branch (<n> archived entry(ies))"). The failure message is unchanged.
- `test/readiness-git.spec.ts` (temp `git init` repositories, never this checkout) gains the cases: backlog
  unchanged and nothing archived → not touched; an entry added and archived on the branch → touched; backlog
  edited → touched.
- `CHANGELOG.md`: none — internal tooling (say so when the entry is closed).


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
- **Scoped reports** (the drift report PDF): an export of something other than the tenant's documentation
  is the same case as a partial export — its link sits on the page of the thing it exports (the tenant drift
  page, next to the **Summary | Drift** switch), shown only when there is something current to export, and
  never on the picker.
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
it. For a PDF of the documentation, the drift report PDF entry brings `pdfmake` and an HTML→PDF content
walker (`src/docs/export/pdf-content.ts`) — reuse those rather than a print stylesheet or a second engine.
Preserving the document tree in an export is not expressible through Confluence HTML import at all —
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

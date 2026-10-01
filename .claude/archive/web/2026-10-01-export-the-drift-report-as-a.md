---
title: Export the drift report as a single PDF
project: web
status: done
started: 2026-09-30
finished: 2026-10-01
branch: feat/web-drift-pdf
pr: 35
changelog: Unreleased
---
## Export the drift report as a single PDF

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

- ✅ **Dependency.** `pdfmake` `^0.3` (0.3.11 current; CommonJS `js/index.js`, so a plain import, not
  `dynamicImport`) in `dependencies`, `@types/pdfmake` `^0.3` in `devDependencies`. Fonts: the Roboto TTFs
  the package ships under `pdfmake/fonts/Roboto/` (located with `require.resolve`, registered with
  `setFonts`) for text, and the pdfkit standard font Courier for `code`/`pre` runs that are WinAnsi-encodable
  (any other run stays in Roboto). No network, no font files under `public/`.
- ✅ **Configure the pdfmake singleton once**, in the owning service's constructor or `onModuleInit`, never per
  request: `setUrlAccessPolicy(() => false)` and `setLocalAccessPolicy()` allowing exactly the four Roboto
  font files. Both are required — without them pdfmake `console.warn`s on every `createPdf` (no per-request
  logging, `no-console`) and would fetch URLs or read local files named in a definition. The document
  definition never carries `image`, `svg` or a URL.
- ✅ **One view model for the page and the PDF.** A `DriftReportService` in `src/docs/drift-report.service.ts`
  (registered in `docs.module.ts`) takes over what `docs.controller.ts` assembles today: the tenant report
  (`tenantDrift` state, `auditOf`, `observationSummary`, `findingGroups`, `byActor`, `changedByCells`, the
  split `drift/index.md` render) and the finding view (`findingView`, `renderAnalysis`, the severity from
  the analysis frontmatter, `attributionOf`, deltas, `intact`). The controller keeps HTTP, `driftLinks` and
  the inline payload; `ExportService` injects the same service, so the drift page and the PDF read one
  decision. The drift pages render byte-identically; the existing e2e cases stay green.
- ✅ **`src/docs/export/pdf-content.ts`** (pure, Nest-free): an `htmlparser2` walker from the browser's rendered
  Markdown HTML to `pdfmake` content, mirroring the keep / unwrap / drop verdicts of `html-allowlist.ts` —
  headings, paragraphs, lists, tables, `strong`/`em`/`code`/`pre` kept; `<details>` always expanded with its
  `<summary>` as a bold lead line; images become their alt text; a link whose path (query and fragment
  removed) is `driftHref(tenant, key)` for a finding in the report and that carries no `yaml`/`raw`/`diff`
  query becomes an internal link to that finding's section (`linkToDestination` / `id` = a stable id derived
  from the key), any other link plain text; unknown elements unwrapped; `script`/`style` dropped. The drift
  index table's severity and **Changed by** columns come through because the input is the page's own render
  (`env.changedBy` included).
- ✅ **`src/docs/export/drift-pdf.ts`** (pure): the document definition from the report model — cover (tenant,
  "Drift report", `observedAt`, baseline `generatedAt`, both tool versions); the observation block as in
  `partials/drift-observation.hbs` (counts, incomplete run, removals suppressed, unknown types, not comparable,
  attribution-outdated caveat); the analysis summary (`drift/index.md`); the findings grouped by type
  (`findingGroups`), each with its header (`findingHeader`), verdict and severity, attribution (events table
  or the one-line status), the deltas table, the mismatch warning when its files are not intact, and its
  analysis — "No analysis" when that document is missing or unreadable, never a failed export; then "By actor"
  (`byActor`). Footer: tenant · observed at · page n/m. Determinism: `info.creationDate` is a `Date` parsed
  from `observedAt` (the Unix epoch when it does not parse) — pdfkit derives the file ID from it — no
  `modDate`, fixed `title` "<tenant name> drift report", so the same observation gives the same bytes.
- ✅ **`ExportService.driftPdf(info, index, res)`**: gets the report from `DriftReportService` (observation,
  audit and analysis documents through `DriftService` and `resolveDriftDocument()` only, rendered by
  `MarkdownRendererService` with the drift route base and the page's env, so the render cache stays
  shared), yields to the event loop between findings, builds the PDF in memory with `getBuffer()` and only
  then sets `Content-Type: application/pdf`, `Content-Disposition: attachment;
  filename="<tenant>-drift.pdf"`, `Content-Length` and `nosniff` and ends the response — a failed build
  answers a generic error without a half-sent attachment, a path or a message. No temporary file, nothing
  written under `DOCS_ROOT`.
- ✅ **Route.** `GET /:tenant/_export/:format` accepts `drift-pdf` beside `confluence`; 404 for an unknown
  tenant or index (`tenant` kind), an unknown format (`export` kind) and a drift state other than `current`
  (`noDrift` kind), without leaking paths.
- ✅ **Entry point.** `views/partials/header.hbs` gains an optional `download` slot (`href`, `label`, `caveat`)
  rendered beside the view switcher; only the `driftTenant` handler fills it, and only for a current
  observation: a plain `<a download>` "Download drift report (PDF)" to `/<tenant>/_export/drift-pdf` with
  "A snapshot of this observation; it is not updated." as visible text beside it; dark variant, visible
  `:focus-visible`, no nested anchor. Every other page passes no `download` and renders unchanged.
- ✅ **Tests.** `test/export-drift-pdf.spec.ts` (pure, `mkdtemp` fixtures): the `pdf-content` verdicts
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
  `src/styles.css` changes.
- ✅ **Follow-up: every table fits the A4 content width.** Found in review of a real 155-page export: almost no
  table fits. Two causes. First, `pdf-content.ts` gives every Markdown table column `'*'`, so the drift index
  table's short Severity and Verdict columns take as much room as Resource, and Resource and Changed by run
  off the right edge. Second, pdfmake never lets a `'*'` column shrink below its longest unbreakable token,
  and the cells are full of them. Examples: dotted delta paths such as
  `scheduledActionsForRule[0].scheduledActionConfigurations[0].gracePeriodHours`, GUIDs,
  `GBL_CP_PRD_…` names with underscores, UPNs and Courier type keys. In "What changed" this pushes Baseline
  and Observed off the page even though its widths are `'*', '*', '*'`.
  - ✅ **Break points in long tokens.** One exported helper in `pdf-content.ts` (the limit a named constant,
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
    `drift-pdf.ts`; prose outside tables is left alone.
  - ✅ **Content-sized widths.** In `tableOf`, a Markdown column whose longest cell text (header included,
    whitespace-normalised, after symbol substitution) is at most 12 characters is `'auto'`; every other
    column is `'*'`. The events table's `'auto'`, `'*'`, `'*'`, `'auto'`, `'*'` (When, Actor, Activity,
    Result, Correlation id) and "What changed"'s `'*'`, `'*'`, `'*'` (Field, Baseline, Observed) stay as
    they are: their overflow is the unbreakable tokens, which the break points fix.
  - ✅ **Portrait A4 stays.** No landscape pages, no change to `pageSize` or `pageMargins`, and no table text
    below today's 8.5 pt (`table` stays 8.5, `code` stays 9). The drift pages render unchanged.
  - ✅ **Test.** `test/export-drift-pdf.spec.ts` gets a case that fails on the current code. It builds a
    definition from the worst cases above (the dotted delta path, a GUID, a `GBL_CP_PRD_…` name with
    underscores, `jan.schumann@extern.cb-gmbh.com`, a Courier type key) in "What changed", the events
    table and a six-column Markdown table (Severity, Verdict, Resource, Changed by and two long columns).
    Walking every table cell, it joins adjacent inline runs, splits on whitespace and U+200B, and asserts
    no piece is longer than 12 characters; that no run with `font: 'Courier'` contains U+200B; that the
    cell text with U+200B removed equals the input; and that the Severity and Verdict columns are
    `'auto'` while Resource and Changed by are `'*'`.
- ✅ **Follow-up: table rows never split across a page break.** On the real export, an events-table row
  starts at the bottom of one page and continues on the next ("jan.schumann@extern.cb-" on one page,
  "gmbh.com" on the next). Every table the definition carries — Markdown tables from `tableOf`, "What
  changed", the events table and the cover facts table — sets `dontBreakRows: true`. `headerRows` stays
  1 on "What changed" and the events table and stays the `<thead>` row count on Markdown tables, so the
  header repeats on a continuation page; the cover facts table has no header row and keeps none. A row
  taller than a page is not expected in a drift report and is accepted as a known limitation.
  - ✅ **Test.** Walking the whole definition, every `table` node has `dontBreakRows: true`, and "What
    changed", the events table and a Markdown table with a `<thead>` have `headerRows: 1`.
- ✅ **Follow-up: the arrow and other common symbols render as a box.** The Roboto bundled with pdfmake has no
  `→` (U+2192), so every "`a → b`" in the analyses prints `□`, in prose and in code runs alike.
  - ✅ **Substitution map.** One exported pure function in `pdf-content.ts` maps the six symbols Roboto lacks
    (the coverage check is done and recorded in the Glyph coverage note): `→ ->`, `← <-`, `↔ <->`,
    `⇒ =>`, `✓ yes`, `✗ no`. `≠`, `≤` and `≥` are not mapped: Roboto has them, and a code run holding
    one falls back to Roboto through the WinAnsi check. The function runs on every string before the
    WinAnsi font choice and before the break points: text runs and `pre` in `pdf-content.ts`, and in
    `drift-pdf.ts` `code()`, table cells, finding labels, verdict and severity labels, attribution text,
    `deltaNote`, the By actor lines and the cover. A code run that held only a mapped symbol outside
    WinAnsi is therefore drawn in Courier after substitution. The mapping applies to the PDF only; the
    drift pages and the Markdown render cache keep the original character.
  - ✅ **Test.** A pure test for each of the six symbols, in prose and in a code run (the code run ends up
    `font: 'Courier'`); `≠` in a code run is kept and the run is in Roboto; a walk of a whole definition
    built from fixtures holding all six finds none of them in any `text`.
- ✅ **Documentation** (at done). `README.md`: the route in the Routes table, a "Drift report PDF" section
  (content, scope, determinism, no payloads or diffs, current observation only), the Printing section
  pointing at it, known limitations, the layout for the new modules. `CHANGELOG.md` `[Unreleased]` → Added.

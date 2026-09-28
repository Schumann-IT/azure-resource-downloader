# Next iterations

Outstanding work, standing decisions and parked ideas for the docs browser. `README.md` describes what it does
today and `CHANGELOG.md` records what shipped; neither is repeated here.

The *Features* below are scheduled design work, promoted from *Parked ideas* and refined against what is true
now. The *Fixes* below are scheduled too, but smaller: self-contained corrections that need no design work,
listed so they are not forgotten between features. Every idea in *Parked ideas* is deliberately unscheduled:
picking one up means promoting it into a numbered work entry with a `**Goal.**` and a `**Plan.**`, reconciling
its rationale against what is true at that point rather than copying it across.

## Features

### ~~1. Tenant compare — a proof of concept, web-only~~

**Goal.** Put two exports side by side and answer, per resource, *is this configured the same in both tenants,
and if not, where exactly does it differ* — without the noise that makes every cross-tenant diff read as
"everything is different". The administrator picks two tenants on the picker, gets a three-way listing of
their resources (only in A, only in B, in both), and opens any paired resource as one line diff of the two
YAML files with tenant-local identity normalised away. Promoted from the parked idea *Tenant diff* (use case
**a**, stage against prod); use case **b** — what the differences *mean* — is the Go-side follow-up below, not
this entry.

> **The data is already on disk; only the identity rule is missing.** Every export uses the same
> `<APIType>/<endpoint>/<name>` key under `resources/`, and `resources/metadata.yaml` lists **every** written
> resource — including the types `index.yaml` only counts under `counts.excluded` — with `displayName`,
> `resourceId`, `odataType` and `presentInTenant`. That file, not the index, is the listing's source: read as
> data, the way the drift view reads `drift/metadata.yaml`, never by walking the tree. The two reference
> exports already in the docs root are the stage/prod pair the parked idea waited for: 1029 against 181
> resources, **114 keys pairing by filename** (42 settings-catalog policies, 19 groups, 11 device
> configurations, 9 compliance policies, …), so exact-key pairing yields a useful subset without any
> heuristic. What no export carries is a notion of cross-tenant identity — that is the one thing this entry
> adds, and it adds it provisionally (next note).
>
> **The normalisation rule is provisional and must move to the Go side.** Before diffing, both documents are
> rewritten identically. **Drop** the fields that are pure per-tenant noise — measured against the 114
> reference pairs, not guessed. An `id` or `sourceId` at **any** depth whose value **contains a GUID**: the
> resource's own, the `<policyId>_<groupId>` and `<scriptId>:<groupId>` assignment composites, the
> `<templateId>_<locale>` message ids, the `scheduledActionsForRule[].id` GUIDs inside compliance policies —
> 81 of the 114 paired files carry a nested id. An `id` **without** a GUID is content and stays: the settings
> ordinals (`"0"`, `"1"`, …), `all_users`, the method names `Fido2`, `MicrosoftAuthenticator`, … in the
> authentication methods policy, the singleton names, and the well-known all-zero sentinels
> (`00000000-0000-0000-0000-00000000000x`), which are values, not identities. Every key ending in
> `@odata.context` at any depth (`settings@odata.context`, `scheduledActionsForRule@odata.context`,
> `includeTargets@odata.context`, … — each embeds the resource's own GUID and every one of the 114 pairs
> carries at least one). `createdDateTime`, `lastModifiedDateTime`, `version`, and the group identity fields
> `mail`, `mailNickname`, `proxyAddresses`, `securityIdentifier`, `renewedDateTime`, without which all 19
> paired groups differ on noise alone. **Resolve, never drop,** the fields that reference another exported
> resource: `assignments[].target.groupId`, `assignments[].target.deviceAndAppManagementAssignmentFilterId`
> and `notificationTemplateId` each become the `displayName` of the entry in that tenant's own
> `metadata.yaml` whose `resourceId` matches — **one lookup over every entry**, not a groups-only table — so
> the same audience, filter or template compares equal and a different one is a real finding. The lookup is
> set-valued because neither side of it is unique in real exports: one `resourceId` in the reference export
> names two different resources, five enrollment configurations share one, and 48 group display names occur
> twice. A reference resolves only when every entry with that `resourceId` agrees on the name; otherwise the
> GUID stays and is flagged *ambiguous*, an id with no entry at all stays and is flagged *unresolved* (six
> real ones in the reference pair, groups the export never listed), and a name shared by several entries is
> counted in the report. A zero-sentinel reference means *none* and passes through unflagged (46 of them in
> the reference pair). Settings-catalog `settingDefinitionId`s and template references are Microsoft-global
> and stay as they are, which is why the settings-catalog pairs are the most valuable part of the PoC.
> **Measured baseline**, from the shipped normaliser over the 114 pairs: **19 are identical**; **36** are
> identical once `assignments` is removed as well (an earlier line-based estimate said 27); with only the
> original top-level drop list it was 8. The 17 in between are the same policy targeting a group with a
> different name in each tenant (`M365-CO-STA-…` against `M365-CO-DYN-…`) — a real finding by the rule, and a
> systematic one in this pair, so the diff page says **"differs only in audience"** when the two documents
> are identical after removing `assignments` too: two normalisations and a text comparison, no structural
> diff. The first real run is judged against those numbers. Two rules the normaliser obeys: it is applied to both sides
> identically, and it is **announced** — the diff page states exactly which keys were dropped, how many
> references were resolved, ambiguous or unresolved, and `?raw` shows the unnormalised diff.
> This rule is a *judgment*, not a fact, and the browser's own principle applies to it as it does to
> taxonomy: a rule derived here can disagree with what any other consumer derives. **It lives in the web only
> to find out what the rule is.** Once it is stable it belongs to the CLI (a `resource compare` that reads two
> exports offline, reuses the drift engine's verdicts and dotted-path deltas, and writes a comparison tree the
> browser renders like `drift/`), and the browser reads the result instead of computing it. That migration is
> the follow-up; this entry states it so it is planned, not rediscovered.
>
> **Selection is two clicks of plain links, not a widget.** Each picker card whose export has a
> `resources/metadata.yaml` gets a *Compare with…* link → `GET /_compare?a=<tenant>` re-renders the picker
> with `a` marked and every *other* card offering *Compare* → `GET /_compare?a=<a>&b=<b>`. Nothing appears
> client-side and no form is introduced; a `<form method="get">` with checkboxes and an always-visible button
> would be legal under the no-client-side-JavaScript rule but would be the app's first form, and the
> export-page idea already debates that precedent — links first. Cards without that file (docs-only copies)
> get no link rather than a 404 later.
>
> **Pairing by key is a heuristic and the page says so.** Two tenants can each hold a
> `gbl_c_prd_d_win_integrity_validation` that are different policies, and a renamed policy shows as
> only-A plus only-B. Acceptable for the PoC; the fix — manual pairing of an only-A row with an only-B row
> (`?left=<key>&right=<key>`, two-step via links) — is the "select files on both sides" of the original
> request and a natural second step, listed under follow-ups.
>
> **The single-side listings are dominated by bulk types.** Only-in-A is 915 rows for the reference pair, 780
> of them unreferenced `Microsoft.Graph/groups` and `windowsAutopilotDeviceIdentities` — the types
> `index.yaml` only counts under `counts.excluded`. Type groups are therefore rendered collapsed (`<details>`,
> like the sidebar), and a type either index lists under `counts.excluded` is rendered last and marked, so a
> reviewer reads policies first and device identities never. Those rows still link to the tenant's YAML view,
> which makes this the first page that *links* to an excluded type's YAML. That widens nothing:
> `GET /:tenant/_resource/*path` already serves any `.yaml` under `resources/` by URL and never consulted the
> index; what the *Browsable excluded bulk types* idea guards is navigation built by walking the tree, and this
> listing walks nothing — it reads `metadata.yaml` as data. That idea's wording is reconciled to say so.
>
> **Non-negotiables, preserved.** Read-only (only reads). Path safety: both tenants must be discovered tenants
> and distinct, both keys go through `resolveResource(tenant, key)` — the existing boundary, called twice; no
> new resolver. `_compare` is a root-level representation prefix, declared before `:tenant`; it cannot collide
> with a tenant because discovery skips `_`-prefixed folders, and it stays out of the breadcrumb like
> `_resource` and `_drift`. No client-side JavaScript. One `markdown-it` instance, untouched. Freshness: the
> metadata reader and any pair cache are keyed by mtime + size. The rule "counts and listings derive from the
> index, never by walking the tree" is **not** violated — `metadata.yaml` is read as data — but its wording
> names the index only, so the rules file is reworded, not amended. Line numbers in the diff are those of the
> *normalised* text, so they cannot anchor into the YAML view; the page says so.

**Plan.**

- ~~**Metadata reader.** `src/docs/resources-metadata.ts`: a pure, Nest-free parser for `resources/metadata.yaml`
  returning, per key, `displayName`, `resourceId`, `odataType`, `presentInTenant` — shape-validated, degrading
  to "no metadata" on anything malformed. `resourceId` and `odataType` are optional (one entry in the
  reference export has no `resourceId`; `odataType` is on 630 of 1029), and the map's keys carry the `.yaml`
  suffix, unlike the drift observation's — strip it so a key is the same string the routes and
  `resolveResource` use. Only `presentInTenant: true` entries are listed: the export retains entries for
  resources gone from the tenant, and a comparison is about what *is* configured (neither reference export
  has a `false` entry, so the fixture must supply one). The reader is loaded with a schema that keeps scalars
  as text. The per-tenant cache reuses the mtime + size `cached()` helper and the metadata file constant that
  `DriftService` already has for this very file, moved to a shared place rather than duplicated; it exposes the
  all-types `resourceId → Set<displayName>` lookup the normaliser needs.~~
- ~~**Normaliser.** `src/docs/compare-normalise.ts`: pure. Takes a parsed document plus that tenant's reference
  lookup (`resourceId → Set<displayName>`), applies the rule from the design note — GUID-containing `id` and
  `sourceId` at any depth, any-depth `*@odata.context`, the named top-level and group fields, the three
  reference fields resolved with the ambiguous / unresolved / sentinel outcomes — serialises canonically
  (sorted keys, one style, `CORE_SCHEMA` on both load and dump so an unquoted date-like scalar is not turned
  into a `Date` and re-serialised differently; verified stable on the reference files) and returns the text
  **and a report** (keys dropped; references resolved, ambiguous, unresolved; names shared by several
  entries). An option removes `assignments` as well, for the audience-only check. The drop list, the
  reference-field list and the GUID test are exported constants, so the next iteration after real use is a
  data change. Marked in its header comment as provisional and destined for the CLI.~~
- ~~**Routes.** `GET /_compare` (`a` only → picker in selecting state; `a` and `b` → listing;
  `a === b`, an unknown tenant, or a tenant without `resources/metadata.yaml` → the 404 view with a new
  `NotFoundKind` and its own headline; `notFound()` learns to render without a single tenant in the header)
  and `GET /_compare/*path?a=&b=` (the diff). Declared before `:tenant`, beside `healthz` and `favicon.ico`,
  the two root-level routes that already precede it; controller does HTTP concerns only.~~
- ~~**Picker.** *Compare with…* per eligible card, where eligible means `resources/metadata.yaml` is present —
  one `stat` per card at render time, the way the drift line is decided, since `TenantInfo.resourcesDir` is a
  path and not a fact. In selecting state, `a` is marked and the other eligible cards offer *Compare*; a
  *cancel* link returns to `GET /`. Consistent with the standing decision that whole-tenant actions live on
  the card. *Shipped as:* eligible means the metadata file is present **and parses** (the same cached read
  the listing uses, so a card never links to a 404), and the link is only offered while at least two cards
  are eligible.~~
- ~~**Listing.** Three-way split — only in A, only in B, in both — grouped by type, each group a collapsed
  `<details>` with its count, excluded types last and marked (design note above), a total per side, each
  *both* row linking to its diff, each single-side row linking to that tenant's YAML view. **No
  same/different state in this pass**: it needs both files read and normalised for every pair (228 reads for
  the reference pair), while the three-way split is computed from the two metadata files alone. *Other
  option, for a later pass:* compute same/different eagerly on the listing by comparing the two normalised
  texts, cached per pair by both files' mtimes, and show the count of genuinely differing pairs — the number
  a stage-to-prod reviewer actually wants first.~~
- ~~**Diff partial.** The diff table is inline in `views/drift-diff.hbs` today; extract it into a
  `views/partials/` partial taking the `YamlDiff` view model, used by the drift diff and the compare diff
  alike so the two cannot render a hunk differently. Behaviour-preserving; the existing drift e2e cases
  cover it.~~
- ~~**Diff page.** Resolve both keys via `resolveResource`, normalise both, `diffYaml` (the drift view's
  function, unchanged), rendered through that partial. Above it: which tenant is left/right, the
  normalisation report as a caption, a `?raw` link for the unnormalised diff, and links to each side's YAML
  view. An identical pair renders "no differences after normalisation" plus the report, never an empty page;
  a pair that is identical only once `assignments` is removed as well says "differs only in audience" above
  the diff, so the reviewer's first question is answered before the first hunk.~~
- ~~**Tests.** `test/path-safety.spec.ts`: nothing new in the resolver, but an e2e case that a traversal in
  either key and a `_`-prefixed or unknown tenant in `a`/`b` 404 without leaking a path.
  `test/compare-normalise.spec.ts`: each dropped field including a nested `id` and a nested
  `*@odata.context`; the id shapes — `GUID`, `GUID_GUID`, `GUID:GUID`, `GUID_en-us` dropped, `"0"`,
  `all_users`, `Fido2` and the zero sentinel kept; resolution of a group id, a filter id and a notification
  template id; the ambiguous flag for a `resourceId` with two names and the unresolved flag for none; a
  zero-sentinel reference passing unflagged; the group identity fields; identical output for identical
  configuration under different ids; the audience-only outcome; report counts; and a date-like unquoted
  scalar surviving load/dump unchanged. `test/docs.e2e.spec.ts`: picker link only on cards with
  `resources/metadata.yaml`; selecting state; `a === b` refused; three-way counts from two fixtures with a
  `presentInTenant: false` entry left out and an excluded type rendered last; diff ignores ids and
  timestamps, resolves a group id, `?raw` shows them again; "differs only in audience" for a pair that
  differs in its target group alone; an edited YAML or metadata file is reflected on the next request.
  `test/styles-build.spec.ts` if any new rule lands in `src/styles.css`.~~
- ~~**Docs and rules.** `README.md`: routes table, the compare contract (source of the listing, the drop list
  and reference fields, the measured baseline, the provisional status), `_compare` as a reserved prefix.
  `CHANGELOG.md` under `[Unreleased]`, stating that the normalisation is provisional and where it is going.
  `.windsurf/rules/01-architecture.md`: the new files, `_compare` beside `_resource`/`_drift`, and the
  reworded listing rule. The *Browsable excluded bulk types* idea below is reconciled per the design note.
  Any change to the rule is mirrored into the `resource compare` idea in `go/NEXT-ITERATIONS.md`, which
  quotes it.~~

**Follow-ups (not in this entry).**

- **Eager same/different on the listing** — the other option above, once the pair count and read cost are
  known from real use.
- **Manual pairing** (`?left=&right=`) and a **rename heuristic** (pair an only-A with an only-B whose
  normalised texts are identical).
- **Side-by-side views** for the listing and the pair diff — promoted to a numbered entry of its own below.
- **Move the normalisation to the CLI** — the condition this entry was accepted under. A `resource compare
  <a> <b>` in `go/` that reads two exports offline, applies the (by then stable and tested) identity rule,
  emits verdicts, dotted-path deltas and payloads, and writes an `analyze.md` so the drift-analysis agent can
  judge the *impact* of each difference — use case **b** of the original idea. The browser then renders that
  tree the way it renders `drift/` and stops normalising itself. Two open questions travel with it: **where
  the comparison tree lives** (everything today is under `<output>/<tenant>/`, and a comparison belongs to
  neither tenant), and that the rule must be *statable and testable* before it is frozen — which is what
  this PoC exists to find out. The Go side already holds this as the parked idea *`resource compare`* in
  `go/NEXT-ITERATIONS.md`, which quotes the rule as stated here; the two are kept in step.

### ~~2. Side-by-side layout for the tenant compare and the drift diff~~

**Goal.** Let a reviewer read *left, right* wherever the browser puts two things next to each other: the
tenant compare's resource listing as two columns with paired resources on one row and a gap opposite anything
only one side has; a paired resource's diff as two panes with the two normalised texts aligned line by line;
and the drift view's per-resource diff — baseline against observed — as the same two panes. Today all three
are single-column: three stacked sections and, for both diffs, a unified table, which answers *what differs*
but makes the reader reconstruct *which side has what*.

> **Same data, second layout.** Nothing new is read or computed. The listing's view model already holds the
> three-way split per type; a two-column rendering only needs the paired rows and the single-side rows of one
> type interleaved into one ordered table (paired first, alphabetical; then A-only; then B-only, each with an
> empty cell opposite). The pair diff's `YamlDiff` already carries `oldNo` and `newNo` per line, which is
> exactly what a two-pane view needs: a removed line sits left with an empty right cell, an added line sits
> right with an empty left cell, a context line spans both. The two-pane variant is therefore a second
> template over the same hunks, not a second diff. The drift diff is the same `YamlDiff` from the same
> function, so it gets the two panes for free — the only thing that differs between the two pages is what
> the panes are called: *baseline* and *observed* on the drift diff, the two tenant ids on the compare. No
> new dependency: the `diff` package already produces the line-level change set, and its only formatters
> emit unified-patch text, so side-by-side is a template over what `diffLines` returns, not a library.
> One pure step is still needed for it to read well: `diffLines` emits a changed block as a run of removed
> lines followed by a run of added lines, and a split view should show the first removed line *beside* the
> first added one, so a modified value is one row, not two. Within each hunk, a run of one kind is zipped
> against the run of the other kind that follows it — whichever comes first — the overhang stays one-sided,
> and context lines pair with themselves. That pairing is a view-model derivation from `YamlDiff`; `diffYaml`
> and the flat `lines` are untouched.
>
> **One grid, reflowing — not two copies.** Rendering a unified table *and* a two-pane grid from the same
> hunks and toggling them by CSS would double the diff markup on every response, bounded only by the 1 MiB
> input cap — several megabytes of HTML at the worst case. Instead the partial renders the paired rows
> **once**, as a CSS grid: four columns (left number, left text, right number, right text) when there is
> room, and below that each row stacks its left cell over its right cell. The narrow view is thereby a
> unified diff of the *paired* lines — a removed line directly above the added line it pairs with — rather
> than today's run of `−` followed by a run of `+`. That is a deliberate change to the released drift diff,
> recorded under `### Changed`; it is still one renderer, one DOM, and the same escaped data. The `<table>`
> is retired; the `diff-added` / `diff-removed` / `diff-context` class hooks move onto the cells, so the
> existing drift e2e assertions keep matching without change.
>
> **The partial measures itself, not the viewport.** The drift diff page has the sidebar (`doc-layout`,
> `lg:flex-row`), so at `lg` its main column is about 700px — two panes of `text-xs` monospace YAML at 300px
> each are useless — while the compare diff page has no sidebar and runs to `max-w-7xl`. No viewport
> breakpoint fits both, and the partial must not know which page it is on. It therefore switches on its
> **own width** with a container query: `@container` on the wrapper, the four-column layout from `@5xl`
> (64rem, about 30rem of text per pane). Tailwind v4 ships container-query variants, so this is utilities in
> the template and no `src/styles.css` rule. No client-side JavaScript, so no toggle; the width decides.
>
> **Long lines wrap inside a pane.** The unified table scrolls horizontally on `whitespace-pre`. In two panes
> that is wrong: one 140-character line would widen the grid and push the right pane off-screen. Pane text
> is `whitespace-pre-wrap break-all`, so a wrapped line stays on its row beside its counterpart — which is
> the point of side-by-side. The stacked narrow view keeps `whitespace-pre` with the wrapper's horizontal
> scroll, as today.
>
> **Non-negotiables, preserved.** Read-only; no new route, resolver or query parameter; every value stays
> escaped, as in the unified table today; the hunk still has exactly one renderer, so the two diffs cannot
> come apart in how a line looks. The drift page's own states — too large, identical, the raw links, the
> finding link — sit outside the partial and are untouched.

**Plan.**

- ~~**Row pairing.** A pure `pairRows(hunk)` beside `diffYaml` in `yaml-diff.ts`: walks a hunk's lines, zips
  each run of removed or added lines against the run of the other kind that immediately follows it into
  `{ left, right }` rows (either side `null` on the overhang; a context line on both sides), so the grid has
  one row per visual line. `diffYaml`, `DiffLine` and the hunk's flat `lines` are unchanged. *Shipped as:*
  `diffYaml` attaches the result to every hunk as an added `rows` field, since a template cannot call a
  function; the existing fields are untouched.~~
- ~~**Diff partial becomes a reflowing grid.** `views/partials/diff-table.hbs` renders each hunk's paired
  rows once, as a grid inside an `@container` wrapper: below `@5xl` every row stacks left cell over right
  cell (a one-sided row shows its one cell); from `@5xl` the grid is four columns — left number, left text,
  right number, right text — with the hunk header spanning the row. Pane text wraps (`whitespace-pre-wrap
  break-all`) in the wide layout and keeps `whitespace-pre` in the stacked one. It takes `left` and `right`
  labels that head the two panes in the wide layout. The `diff-added` / `diff-removed` / `diff-context`
  classes stay, on the cells. The partial remains the only place a hunk is rendered.~~
- ~~**Drift diff page.** `views/drift-diff.hbs` passes `left="baseline"` and `right="observed"`; nothing
  else on the page changes — the source line, the counts, the raw and finding links, the too-large and
  identical states.~~
- ~~**Compare diff page.** `views/compare-diff.hbs` passes the two tenant ids; the report caption, the
  audience line and the raw/normalised links are unchanged, as are the identical and too-large states.~~
- ~~**Two-column listing.** A pure function in `compare-view.ts` interleaves each type's paired, A-only and
  B-only rows into one ordered list of `{ left, right }` cells (paired: both set; single-side: one empty),
  and `views/compare.hbs` renders each type group as a two-column table headed by the two tenant ids, still
  collapsed per type and with excluded types last. A paired row shows A's name left and B's name right —
  `otherName` goes away — and **both** cells link to the pair diff; a single-side row's one cell links to
  that tenant's YAML view, as today. The three per-side totals stay in the summary line; the three stacked
  sections, and their `#compare-*` anchors (nothing links to them), are replaced, not kept beside the
  table.~~
- ~~**Styles.** Expected to be utilities only (container variants, arbitrary grid columns). Only if a rule
  Tailwind cannot express is needed does it go in `src/styles.css`, with a case in
  `test/styles-build.spec.ts`; every state gets a dark variant.~~
- ~~**Tests.** `test/docs.e2e.spec.ts`: on both diff pages a removed line renders in a left cell only, an
  added line in a right cell only, and a modified line as one row carrying both; the drift panes are headed
  *baseline* / *observed* and the compare panes by the two tenant ids; the existing drift diff cases
  (counts, raw links, too-large, identical, the class-hook regexes) pass unchanged; the listing renders a
  paired resource on one row with both cells linking to its diff and a single-side resource with an empty
  opposite cell, excluded types still last. Spec cases for `pairRows` (a modified line becomes one row; a
  longer removed run leaves one-sided rows; added-before-removed pairs the same way; context pairs with
  itself) and for the interleaving function's ordering.~~
- ~~**Docs.** `README.md` (the Drift view and Tenant compare sections: the two layouts, that the partial
  switches on its own width, and that the narrow view pairs lines); `CHANGELOG.md` under `[Unreleased]` —
  inside the tenant compare entry for the compare pages, and a `### Changed` entry for the drift diff, which
  is a released feature changing shape in both layouts.~~

### ~~3. The tenant compare gets the sidebar's endpoint navigation~~

**Goal.** Let a reviewer move between the pairs of a comparison the way they move between a tenant's documents:
from the sidebar, by resource type, without going back to the listing. Today both compare pages are
single-column — the listing is the only way from one pair diff to the next — while every tenant page has the
tree of types in a sidebar. The compare pages get the same sidebar shell and the same tree, headed by the two
tenants instead of one.

> **Same data, second consumer.** The tenant sidebar is built from one tenant's `docs/index.yaml`; the compare
> listing must come from the two `resources/metadata.yaml` files alone, and the rule stands. The compare
> sidebar is therefore derived from the listing's own view model, not from either index: one section per type
> group in the listing's order (documented types first, excluded types last and marked *not documented*), one
> item per row — a pair links to its diff, a resource only one side has links to that tenant's YAML view and
> says *only in `<tenant>`*. Sidebar and body agree by construction, and nothing new is read. On the pair diff
> page the pair being viewed is the active item and its section is open, as a document is on its own page; on
> the listing page nothing is active. A row today carries names and hrefs only, so it gains its key — the
> one datum the active marker needs, and what `?raw` must not disturb. Sections are labelled the way the
> tenant sidebar labels endpoints (*Device Compliance Policies*, not the raw type), which is what makes the
> two sidebars read as one thing.
>
> **One tree renderer.** `views/partials/sidebar.hbs` renders three things: a tenant header (name linking to
> `/<tenant>`, counts, the incomplete-export banner, the excluded-types footer), the filter chips and the
> type tree. Only the header is tenant-specific. It is split into partials — the tree and the chips each
> become one — and a compare sidebar composes its own header (`a ↔ b`, the three totals, *swap sides*, the
> way back to the listing) with the shared tree and chip partials. The same principle as `diff_table`: the
> tenant pages and the compare pages cannot come to render a tree differently. The tree's *pending* marker
> becomes a generic per-item note, set to *pending* by the tenant navigation and *only in `<tenant>`* by the
> compare one, so the rendered tenant sidebar does not change by a byte.
>
> **Layout.** Both compare pages move onto the `doc-layout` shell the tenant pages use (`lg:flex-row`,
> sidebar left, `max-w-7xl`), so the header row lines up with the content as it does everywhere else. Below
> `lg` that shell puts the sidebar *under* the content, so the listing's own totals line and *swap sides* stay
> in the body where a phone reads them first; the sidebar header repeats the counts, as the tenant sidebar
> repeats the export summary's. The diff partial switches on its own width, so the pair diff now behaves
> exactly as the drift diff does beside a sidebar — two panes from about 800px, stacked between 1024px and
> about 1152px, two panes again above. The README's compare section already defers to the drift diff's
> description, which this makes literally true. **It changes one expectation elsewhere:** the drift diff
> fix's pending browser check expects the compare diff to be two panes at every width; once this lands it
> stacks at 1100px like the drift diff, and the check is read that way.
>
> **Non-negotiables, preserved.** Read-only; no new route, resolver or query parameter; the listing is still
> computed from the two metadata files and nothing walks a directory; `_compare` stays out of the breadcrumb;
> every value stays escaped. No client-side JavaScript: the open section is a server-set `open` attribute, as
> today.

**Plan.**

- ~~**Sidebar partials.** Split `views/partials/sidebar.hbs` into `sidebar_tree.hbs` (the `<details>` per
  type with its items, and the *No documents match these filters* fallback it renders when the tree is empty
  and chips are offered) and `sidebar_facets.hbs` (the chip groups and the *Showing N of M · Clear filters*
  line), included by `sidebar.hbs` around its tenant header and excluded-types footer. Markup identical to
  today's. *Shipped as:* the files are `sidebar-tree.hbs` and `sidebar-facets.hbs`, registered as
  `sidebar_tree` / `sidebar_facets` like every other dashed partial.~~
- ~~**Generic item note.** `NavItem` gains `note: string` (`pending` when the resource is not documented,
  empty otherwise) and `NavSection` an optional `note`; `sidebar_tree.hbs` renders the notes instead of
  testing `documented`. `tenant.hbs`'s landing-page fallback listing keeps `documented`.~~
- ~~**Compare navigation.** `ListingRow` gains `key`. A pure `compareNavigation(listing, leftId, rightId,
  activeKey)` in `compare-view.ts` returning `NavSection[]` from the `CompareListing`: sections in the
  listing's order, excluded ones noted *not documented*, labels through `typeLabel` (exported from
  `tenant-index.ts`); items from the rows — pair → pair diff href, single side → that tenant's YAML href with
  note *only in `<id>`*; `active` on the row whose key is being viewed and on its section, `exempt` always
  false here. *Shipped as:* `ListingCell` carries the key too, and `interleaveRows` takes the row's key from
  its cell.~~
- ~~**Compare sidebar and shell.** `views/partials/compare_sidebar.hbs`, an `<aside>` with its own
  `aria-label` (*Comparison navigation*; the tenant one stays *Tenant navigation*, which an e2e case asserts):
  the two tenants linking to the listing, the three totals and *swap sides*, then the shared chip and tree
  partials. `views/compare.hbs` and `views/compare-diff.hbs` switch to the `doc-layout` shell (`max-w-7xl`,
  header at its default width) and include it; the listing body keeps its heading, totals line, note and type
  groups, the diff page its source line and *all resources* link. *Shipped as:* `compare-sidebar.hbs`; on a
  pair's diff *swap sides* opens the same pair swapped rather than the swapped listing.~~
- ~~**Controller.** One private helper builds the `CompareListing` for two `CompareTenant`s — the excluded set
  from both indexes plus `compareListing` — and both routes call it, so the pair route gets the listing from
  the two metadata files it already holds with no extra read and no second construction. Both pass `nav`.~~
- ~~**Tests.** Spec cases for `compareNavigation` (order, notes, active pair also under `?raw`, hrefs).
  `test/docs.e2e.spec.ts`: both compare pages render the sidebar under its own label, the pair diff page
  opens the viewed pair's section and marks the pair, a one-sided item links to the YAML view with its note,
  excluded sections come last; the existing compare cases (per-row href counts, the excluded-last `indexOf`
  check, the totals wording) and the tenant sidebar cases pass unchanged. *Shipped as:* the `?raw` case is
  covered in the e2e suite, where the query exists, rather than in the spec.~~
- ~~**Docs.** `README.md` (*Sidebar navigation on every page* now includes the compare pages; the Tenant
  compare section names the sidebar and what a one-sided item links to; the routes table); `.windsurf/rules/
  01-architecture.md` (Tenant compare: the sidebar is derived from the listing, never from an index;
  Boundaries: the tree and the chips each have one partial); `CHANGELOG.md` inside the tenant compare entry
  under `[Unreleased]`. *Shipped as:* the partials rule sits under Tenant compare, beside the `diff_table`
  one, rather than under Boundaries.~~

### 4. The taxonomy filters narrow the tenant compare

**Goal.** Let a reviewer compare *the Windows compliance policies* or *the Defender programme* of two tenants
rather than everything both hold, with the chip groups the tenant sidebar already offers and the same
behaviour: OR within an axis, AND across, the selection in the URL, counts that follow it. Depends on the
endpoint navigation above being in place, since the chips live in that sidebar.

> **Membership comes from both indexes, joined through the sanctioned key mapping.** A metadata entry carries
> no taxonomy; the index does (`resources[].facets`). The compare routes already load both tenants' indexes,
> and the CLI's own mapping `docs/<type>/<name>.md` ↔ `resources/<type>/<name>.yaml` makes an index resource
> and a metadata key the same thing, so each listing row can be given a membership without reading a file:
> for a pair, the union of the two sides' memberships on each axis; for a one-sided row, that side's. A row
> neither index lists — the excluded bulk types — has an empty membership and falls into *Uncategorised*,
> which is already how a resource an axis matched to nothing is shown. **A pair matches when either side
> matches**, so a policy one tenant categorised and the other did not is still found under its value; the
> alternative (both sides) would hide exactly the difference a reviewer is looking for.
>
> **Axes are the union of what the two indexes declare.** The CLI resolves the taxonomy from its config, so
> two exports of the same operator declare the same axes and usually the same values, but their counts differ
> and a value one tenant matched to nothing may be missing from the other's header. Axes are merged by id,
> values by id in the left header's order with the right header's extras appended, labels from the left
> header first. Only axes `filterableAxes` offers on at least one side are offered.
>
> **One matching rule, not a second one.** `buildFacetFilters`, `countMatching` and the selection matching
> in `tenant-index.ts` are written against an index. Their core — axes, a list of members each carrying a
> per-axis membership, the selection — is extracted so the tenant sidebar and the compare sidebar call one
> function; the index-shaped entry points stay as wrappers. This is the same reason the Confluence export
> classifies through `filterableAxes`: a second definition of *matches* is exactly the drift to avoid.
>
> **Reserved parameters and hrefs.** `a`, `b` and `raw` are the compare routes' own query parameters. `a` and
> `b` join `raw`, `yaml` and `diff` in the reserved set, globally: an axis with either id is not offered on
> any page, which is the one rule the reserved set already states. Every compare href carries `?a=&b=`, so
> `selectionHref` appends with `&` when its base already has a query string; the selection rides along on
> every item href and on *swap sides*, and *Clear filters* is the plain listing href.
>
> **What is narrowed.** On the listing page the filter narrows the sidebar **and** the type groups in the
> body, with the *Showing N of M* line: the body is a listing of resources, not a document, and a listing
> that ignored its own sidebar's filter would read as broken. On the pair diff page the filter narrows the
> sidebar only, and the pair being viewed stays listed and marked *outside the filter*, exactly as a
> document does. The README's sentence that filters narrow navigation, not page bodies, is refined to say so.
>
> **Non-negotiables, preserved.** Read-only; no new route or resolver; the listing is still built from the
> two metadata files, the indexes only *annotate* rows that already exist and never add one; a tenant whose
> index cannot be read inside the discovery TTL simply contributes no membership; `a`/`b`/`raw` cannot be
> shadowed; every value stays escaped and no script is introduced.

**Plan.**

- **Filter core.** In `tenant-index.ts`, extract the member-based core of `buildFacetFilters`,
  `countMatching` and `matchesSelection` — `(axes, members, selection, hrefFor)` — and re-express the
  index-shaped functions over it. Existing spec cases pass unchanged. Add `a` and `b` to the reserved query
  parameters and teach `selectionHref` to append to an existing query string.
- **Compare axes and membership.** Pure functions in `compare-view.ts`: merge the two indexes' filterable
  axes by id (left order, right extras, left labels first); annotate each listing row with its per-axis
  membership through the `docs/<type>/<name>.md` ↔ key mapping, union for a pair; the compare selection is
  parsed with the existing `parseFacetSelection` against the merged axes.
- **Narrowing.** `compareListing` (or a pure step after it) drops rows the selection excludes and recomputes
  the group counts and totals; `compareNavigation` keeps the viewed pair as an exempt item on the diff page.
  Chip hrefs, item hrefs and *swap sides* carry the selection; *Clear filters* is the listing href.
- **Views.** `compare_sidebar.hbs` includes `sidebar_facets` with the compare filters and the *Showing N of
  M* line; the listing body renders the narrowed groups. No new markup beyond what the shared partial has.
- **Tests.** `test/tenant-index.spec.ts`: `a`/`b` are not offered as axes, `selectionHref` appends with `&`,
  the refactor keeps every existing case. Spec cases for the merge (order, labels, a value only one side
  declares), the membership join (pair union, one-sided row, unlisted row → uncategorised) and the narrowed
  counts. `test/docs.e2e.spec.ts`: a filtered listing shows only matching groups and rows with the count
  line, the chips carry `a` and `b`, the diff page keeps the viewed pair *outside the filter*, an axis id
  colliding with `a` is ignored.
- **Docs.** `README.md` (Taxonomy filters: the compare pages, the either-side rule, the refined
  navigation-not-bodies sentence; routes table: the compare routes take the filter parameters);
  `.windsurf/rules/01-architecture.md` (Tenant compare: indexes annotate rows, never add them; `a`/`b`
  reserved everywhere); `CHANGELOG.md` inside the tenant compare entry under `[Unreleased]`.

### 6. The compare diff reads like an IDE's file comparison

**Goal.** Let a reviewer read a pair the way an IDE's *Compare Files* view shows it: both files **whole**,
side by side, equal lines level with each other, and the eye drawn only to what differs — down to the
characters that differ inside a changed line. Today the compare diff shows hunks: three lines of context around
each change, a `@@` header per hunk, red and green lines. That suits a patch; for two configurations it hides
where in the file a difference sits and what surrounds it, and a changed value reads as *a whole line removed,
another added* even when two characters moved. The reference is the IDE's two-pane view: full text, both line
numbers together in a centre gutter, changed lines tinted on both sides, the differing characters marked more
strongly, and a count of differences with a way to jump from one to the next.

> **Most of it is already there.** Entry 2 made every changed line one row beside its counterpart
> (`pairRows`); the diff is computed from the two normalised texts and emitted as plain data. What differs from
> the reference is (1) *how much* is shown, (2) *how* a changed row is marked, (3) where the line numbers sit
> and (4) how to get to the next difference. None needs a script.
>
> **Whole file, through the same data.** `diffYaml` already takes the context size; the whole file is the
> same computation with unbounded context, which yields one hunk holding every line — no second diff, no
> second row model, and the partial renders it through the same rows. The hunk header is noise when the hunk
> is the file, so the partial omits it when told the diff is whole. Size stays bounded: `MAX_DIFF_BYTES`
> already caps the input, and above a line cap of **3,000 lines** (`MAX_FULL_LINES`, beside it — a 3,000-row
> two-pane table is already one to two megabytes of HTML) the page falls back to hunks and says so, so a huge
> pair cannot produce an unreadable page. The choice reaches `diffYaml` through an option on
> `pairComparison`, which the compare controller sets to a constant; the fallback is decided inside
> `diffYaml`, so no controller holds diff logic.
>
> **What changed inside a line, as data.** For a row that pairs a removed line with an added one (a
> *modified* row), a character diff of the two texts (`diffChars`, already in `diff`) splits each side into
> parts — equal or changed. This is what marks `31:35Z` against `29:37Z` rather than the whole timestamp. A
> character diff of two unrelated lines is confetti, and `pairRows` produces such rows on purpose: it zips runs
> by position, so three removed lines against one added make a modified row of two lines that have nothing
> in common. Hence a threshold: when the changed characters divided by the longer line's length exceed **0.5**,
> no parts are produced and the whole row reads as changed. Lines over **500 characters** skip the character
> pass for the same reason. The parts are plain `{ text, changed }` values, rendered as escaped `<span>`s,
> never as HTML — the diff-as-plain-data rule stands unchanged.
>
> **Colour follows the IDE's meaning.** A modified row is tinted on **both** sides in one neutral change hue
> (blue in light, its muted counterpart in dark), with the changed parts in a stronger shade of it **and**
> underlined, so the marking is not carried by colour alone; a line only one side has stays green on its side
> against an empty filler cell opposite, as today; equal lines are untinted. The `−` / `+` signs disappear in
> the two-pane layout, where the side says which file a line is from, and stay in the stacked layout, where
> both halves of a modified row carry the same tint and the sign is the only way to tell left from right. The
> existing `diff-removed` / `diff-added` / `diff-context` class hooks stay, and a `diff-modified` hook is added
> after them, so the drift and compare e2e assertions keep working.
>
> **Centre gutter, both numbers.** In the two-pane layout each row becomes *left text · left number · right
> number · right text*, the numbers meeting in the middle as in the reference; below the container threshold
> the stacked layout of entry 2 keeps *number · text* on both halves (a centre gutter needs both panes). That
> is one markup path: the left half's number span takes `@3xl:order-last`, not a second row template.
>
> **Differences are counted and linked, without a script.** A *difference* is a block of adjacent changed
> rows, not a line. `YamlDiff` gains the number of blocks; the first row of each block carries an anchor
> (`#change-1`, `#change-2`, …) and a gutter link to the next block, the last one back to the first, each with
> an `aria-label` (*next difference*, *first difference*) since its text is a glyph. The page's source line
> says *N differences* and links to the first. Fragment links are HTML, so the no-script rule holds; the
> sticky header is `top-14`, so `scroll-mt-16` on the anchored row keeps a target from landing under it —
> `src/styles.css` is not touched.
>
> **Decision: the drift diff shares all but the whole-file mode.** The partial is shared on purpose — drift
> and compare must not render a row differently — so the modified tint, the character parts, the centre
> gutter and the change anchors reach the drift diff too, where they are equally useful. Only *whole file*
> is chosen per page: the compare diff shows the whole file, the drift diff keeps hunks, since its payloads are
> the full observed resource and its question is *what changed since the baseline*, not *how do these two
> read side by side*. If the drift diff should stay exactly as it is, the alternative is a second partial —
> which would break the one-partial rule and is not recommended.
>
> **Out of scope, already parked.** The reference's top half — a folder listing with a `≠` / `←` / `→`
> status column — is entry 2's two-column listing plus the **eager same/different** follow-up of entry 1,
> which stays parked for its read cost. Editing, copying a line across and a scroll-synchronised pair of
> panes are IDE features that need a script or a write path, and are not planned.
>
> **Non-negotiables, preserved.** Read-only; no new route, resolver or query parameter; both files still go
> through `resolveResource` and are hash- or metadata-gated as today; every line and every part stays escaped;
> no client-side JavaScript.

**Plan.**

- **Diff data.** In `yaml-diff.ts`: a named whole-file context constant for `diffYaml`, `MAX_FULL_LINES`
  (3,000) beyond which a whole-file request is served as hunks with a flag the page can state, and the
  character-pass constants (ratio 0.5, 500 characters). `DiffRow` gains `modified` (both sides present, not
  context), per-side `parts` (`{ text, changed }[]`, or none when over the threshold or the length cap) and
  the index of the difference block it opens, if any; `YamlDiff` gains the block count. `pairRows` stays the
  only pairing step; the character pass runs on its modified rows only. `pairComparison` gains the
  whole-file option and passes it through.
- **Partial.** `views/partials/diff-table.hbs`: a `full` parameter that omits the hunk header; the modified
  tint, the stronger underlined part shade (with dark variants); the two-pane row as *text · number · number
  · text* through `@3xl:order-last` on the left number, signs hidden from `@3xl`; the per-block anchor with
  `scroll-mt-16` and the labelled next-difference link in the gutter; the stacked layout below `@3xl` as
  today. Update its header comment for the new row shape.
- **Pages.** `compare-diff.hbs` passes `full` and shows *N differences* with a link to the first and, when
  capped, the fallback note. `drift-diff.hbs` shows the difference count too and keeps hunks. The compare
  controller sets the whole-file option on `pairComparison` — no other logic there.
- **Tests.** `test/yaml-diff.spec.ts`: whole-file context yields one hunk with every line and line numbers
  intact, and no hunk at all for equal texts; the line cap falls back to hunks with the flag set; a modified
  row's parts mark only the differing characters; two unrelated lines (over the ratio) and an over-long line
  get no parts; a 3-against-1 run yields one modified row and two one-sided rows; block count and block
  starts for adjacent and separate changes; a part containing `<` stays plain text. `test/docs.e2e.spec.ts`:
  the compare diff shows a line far from any change, no `@@` header, a `diff-modified` row with an escaped
  underlined changed part and no sign in the two-pane markup, *N differences*, `#change-1` with `scroll-mt-16`
  and the labelled next-difference link; the drift diff still shows its `@@` headers and gains the modified row
  and anchors; the existing class-hook assertions pass unchanged. No `src/styles.css` change, so no
  `styles-build` case.
- **Manual check in a browser.** The reference pair from the screenshot's exports and one short policy pair,
  light and dark, at 900 and 1440px: equal lines level, characters marked, gutter numbers meeting in the
  middle, next-difference links landing below the sticky header, stacked view still readable.
- **Docs.** `README.md` (Tenant compare: the diff shows the whole file, what is marked and how to jump between
  differences, the line cap; Drift view: the shared marking and the count); `.windsurf/rules/01-architecture.md`
  (Tenant compare: the whole-file mode is a partial parameter, not a second partial; the character parts are
  plain data); `CHANGELOG.md` inside the tenant compare entry and the drift diff's `### Changed` entry under
  `[Unreleased]`, both still unreleased.

### 7. The compare listing becomes the IDE's comparison pane, above the diff

**Goal.** Put the top half of an IDE's folder comparison on the compare pages: every resource of both exports in
one scrollable pane, a pair on one row, a status column saying at a glance whether the pair is **identical**
(`=`), **different** (`≠`) or present on **one side only** (`←` / `→`), and the diff of the selected pair
directly underneath, so a reviewer works down the differences without leaving the page. Today the listing
cannot say whether a pair differs (entry 1 left that out for its read cost), lives on its own page, and hides
its types in collapsed groups; the diff is a second page.

> **No script needed, so the rule stays.** Everything the reference shows can be rendered on the server: the
> status is computed per request from cached reads, selecting a row is a link to the pair's URL, the selected
> row is marked server-side, the pane scrolls with `overflow-y: auto`, and the divider between pane and diff
> can be dragged with CSS `resize: vertical`. What only a script could add is IDE *behaviour*, not layout:
> arrow-key selection, loading a diff without a page load, keeping the pane's scroll position across that
> load exactly, synchronised scrolling of the two diff panes. The first three are replaced below by links
> and a fragment; the last one is not needed for a row-aligned diff. So *Idea: Drop the
> no-client-side-JavaScript rule* stays parked, and this entry adds a note there that it was checked.
>
> **Status is computed eagerly: entry 1's follow-up, promoted.** For each pair both files are read through
> `resolveResource`, normalised with the same rule as the diff, and compared as text: equal → `=`; equal once
> `assignments` is removed → `≠` marked *audience only*; otherwise `≠`. A pair that cannot be read, does not
> parse or is over `MAX_DIFF_BYTES` shows `?` with the reason as its `title`, never a guess. The result is
> cached per pair and validated by the mtime + size of **both** files **and** of both `metadata.yaml` files,
> since the reference lookup comes from those: an edited resource or a re-downloaded export flips the status on
> the next request, with no restart. Bounded like every cache. The cost is what entry 1 measured — about 228
> pairs, so 456 reads and normalisations on a cold cache for the reference exports, then two `stat()` calls per
> pair per request — and is checked against the reference exports before this ships; if the cold load is too
> slow, reads run with a small concurrency bound, and there is no fallback that would show a stale status.
>
> **Rows still come from the metadata alone.** Which rows exist and where they sit is unchanged: the two
> `resources/metadata.yaml` files, no directory walked. Only the *status of a pair* reads resource files,
> and only for keys both exports list as present, only through `resolveResource`. The architecture rule
> *"no resource file is read"* for the listing is narrowed to exactly that, rather than dropped.
>
> **One page shape for both routes.** `GET /_compare?a=&b=` and `GET /_compare/<key>?a=&b=` render the same
> layout: header, the pane, then the diff area. Without a selected pair the diff area says so and points at
> the first difference; with one, it is the pair diff (entry 6's whole-file view once that lands, today's
> hunks until then). No new route. Each row is one link to its pair URL (or, one-sided, to that tenant's YAML
> view, as today), the selected one is `aria-current` and highlighted, and there are no nested anchors.
> Links carry `#row-<n>`, so after the page load the browser brings the selected row into view inside the
> pane. A fragment scrolls the page as well as the pane, so the row gets a `scroll-margin-top` that keeps
> the page itself from moving. This is the one layout detail the manual check has to confirm.
>
> **The pane.** A flat table like the reference, not collapsed groups: one header row per type (labelled as
> in the sidebar, the excluded bulk types last and marked *not documented*), then its rows. Columns: left
> name, left size, status, right size, right name. Sizes come from the `stat()` the cache makes anyway. The
> reference's date column is left out, because every file of an export carries the same download time,
> which the header already shows. By default the pane shows **what needs attention**: different, one-sided
> and unknown rows. Identical pairs are counted in the header and shown with a `&same` link, a GET parameter
> that is added to the compare routes' reserved set. The totals become the four counts (*N different · N only
> in a · N only in b · N identical*), plus *swap sides*. Max height about 40% of the viewport, and vertically
> resizable. Below `lg` the size columns drop, and the pane stays above the diff.
>
> **Decision: the pane replaces the compare sidebar's tree.** Entry 3 gave the compare pages the tenant
> sidebar's tree so a reviewer could move between pairs. The pane does that better and in the reference's
> place, and keeping both would show the same list twice beside a diff that needs the width. So the compare
> pages drop back to full width, with no `<aside>`. `compareNavigation` and the compare sidebar partial are
> removed. The shared `sidebar_tree` / `sidebar_facets` split stays, because the tenant sidebar uses it and
> entry 4's chips can sit above the pane through `sidebar_facets`. Entry 3 is unreleased, so this amends its
> part of the tenant compare changelog entry rather than adding a *Changed* one. *Other option:* keep the
> sidebar and add the pane. Not recommended, for the duplication.
>
> **Markers and colour.** `≠` is the change hue of entry 6, `=` neutral, `←` / `→` the one-side green of the
> diff, `?` amber. The arrows point from the side that has the resource towards the side that lacks it, as in
> the reference, and every marker carries a visually hidden label (*identical*, *different*, *only in stage*,
> *could not compare*) so it is not meaning by glyph and colour alone. Every marker has a dark-mode variant
> and a visible `:focus-visible` outline on its row link.
>
> **Non-negotiables, preserved.** Read-only; no client-side JavaScript; no new route or resolver; the pair
> files are only ever located through `resolveResource`; rows are still data from the two metadata files;
> the normalisation rule is unchanged and applied identically to both sides (nothing to mirror into the Go
> idea); `_compare` stays out of the breadcrumb; every name, size and marker is escaped; no-restart freshness
> holds through the validated cache.

**Plan.**

- **Pair status.** A pure `pairStatus(left, right, a, b)` in `compare-view.ts` built on `pairComparison`'s
  normalisation (one rule, not a second): `identical` / `audience` / `different` / `unknown` + reason. In
  `compare.service.ts`, a bounded status cache keyed by `a`, `b` and the key, validated by the four
  mtime + size pairs, and a `statuses(a, b, keys)` that stats and reads with a small concurrency bound.
  Sizes come back with it.
- **Listing model.** `compareListing` rows gain `status` and the two sizes. A pure step splits the default view
  (non-identical) from `&same`, and computes the four totals. Rows get a stable `n` for `#row-<n>`, and the
  selected key is marked. `same` joins the reserved query parameters.
- **Views.** A `compare_pane` partial (the table, header rows per type, markers with hidden labels, the resize
  container). `compare.hbs` and `compare-diff.hbs` become one layout, pane then diff area, with no sidebar.
  `compare-diff.hbs` keeps the diff, and the listing page shows the *select a pair* state. Remove
  `compare-sidebar.hbs` and `compareNavigation`, with its spec cases, since their purpose moves to the pane.
- **Controller.** Both compare routes build the listing with statuses through the existing `listingOf`
  helper, then pass the selected key and `same`. No filesystem logic in the controller.
- **Tests.** Spec: `pairStatus` for identical, audience-only, different, unparsable and too-large; the default
  / `&same` split and the totals; row numbering stable across both views. `test/docs.e2e.spec.ts`: the
  markers and hidden labels for each state, identical pairs hidden by default and counted, `&same` showing
  them, the selected row marked on the pair page and its link carrying `#row-<n>`, one-sided rows linking to
  the YAML view, excluded types last, no `<aside>` on either compare page, and no nested anchors. Freshness:
  an edited resource flips `=` to `≠`, and an edited `metadata.yaml` re-evaluates a pair whose reference
  resolution changes, each on the next request. Plus writes-nothing and no-path-leak for an unreadable pair. The
  existing compare cases are updated where they asserted the collapsed groups or the sidebar. That is the
  behaviour this entry changes, stated in the entry, not a weakened assertion.
- **Manual check in a browser.** The reference exports: cold and warm load time of the listing; the pane
  scroll landing on the selected row without the page jumping; the divider dragging; light and dark; 900
  and 1440px.
- **Docs.** `README.md` (Tenant compare: the pane, the status and what it costs, `&same`, one page shape;
  routes table). `.windsurf/rules/01-architecture.md`: in Tenant compare, the rows-from-metadata /
  status-from-`resolveResource` split, the bounded validated status cache, `same` reserved, and the compare
  sidebar rule replaced by the pane. `CHANGELOG.md`: amend the tenant compare entry under `[Unreleased]`
  (the sidebar sentence becomes the pane, and the *same/different column is a follow-up* sentence goes).
  Here, entry 1's *Eager same/different* follow-up is marked as promoted into this entry, and *Idea: Drop the
  no-client-side-JavaScript rule* gets one sentence saying this pane was checked against the rule and needed
  no script.

## Fixes

Each is a numbered work entry in its own right; none touches a non-negotiable (read-only, no client-side
JavaScript, one `markdown-it` instance, path safety) and none depends on a documentation regeneration. Each
carries its own e2e or spec case and a `CHANGELOG.md` entry under `[Unreleased]`; purely internal ones say so.

A **struck-through** title or plan item has shipped and its `CHANGELOG.md` entry is written. It stays here,
struck, until the branch is closed and the release is cut — that is when the entry is deleted, not the moment
the code lands.

### ~~5. The drift diff never goes side by side~~

**Symptom.** The drift view's YAML diff (`/<tenant>/_drift/<key>?diff`) stays in the stacked layout on every
screen, however wide; only the tenant compare's diff ever shows two panes. Entry 2 shipped the layout for
both, so the drift half of it does not work.

> **Cause: the page can never reach the threshold.** The diff partial switches to two panes when its own
> width reaches `@5xl` (64rem). The drift diff page keeps the tenant sidebar: the layout is capped at
> `max-w-7xl` (80rem), minus the `px-4` padding (2rem), the sidebar's `lg:w-80` (20rem) and the `gap-8`
> (2rem), which leaves the main column **56rem at most** — 8rem short, by construction. The compare diff page
> has no sidebar and gets up to 78rem, which is why it works there. The container query itself is right (it
> is what makes one partial serve both pages); the threshold and the drift page's width do not fit each
> other. Entry 2's review estimated the drift column at `lg` only and missed that it never grows past 56rem.
>
> **Options.** (a) **Lower the threshold** to `@3xl` (48rem): after the number and marker columns each pane
> gets about 19rem of text, some 42 characters of `text-xs` monospace — enough for most YAML keys and
> values, with wrapping for the rest, and it applies to the compare page too. (b) **Widen the drift diff
> page**: let the layout grow past `max-w-7xl` on this page only (e.g. `max-w-screen-2xl`), so the column
> reaches 64rem on a large monitor — but not on a laptop. The header is sized separately (`headerWidth`) and
> an e2e case asserts it lines up with the page, so (b) has to pass the matching header width too. (c) **Drop
> the sidebar on the diff page**, as the compare diff has none — loses the navigation the drift view
> otherwise keeps. Recommended: **(a)**, optionally with (b), so a laptop gets two narrow panes and a large
> screen two comfortable ones; (c) only if (a) reads too cramped in practice. The number stays one constant
> in the partial, never per page.
>
> **The drift page switches twice as the window grows.** Below `lg` the sidebar stacks under the content
> (`flex-col-reverse`), so the diff has the full width; at `lg` it moves beside it and takes 22rem. With a
> 48rem threshold the drift diff is therefore side by side from a window of about 800px, stacked again from
> 1024px (where the sidebar moves beside the content), and side by side again from about 1152px. That is
> the container query working as intended — it follows the room the diff actually has — and is accepted,
> not worked around; it is stated so it is not reported as a new bug.
>
> **Why entry 2 passed its tests while broken.** The e2e suite asserts class names, not layout; it cannot
> see that a container never reaches its threshold. The constraint that actually failed — *the threshold
> must fit inside the narrowest page that uses the partial, today the drift page's 56rem* — is therefore
> written down where the next person will change it: the partial's header comment, which currently names
> `@5xl`. And the fix is verified in a browser, not only by the suite.

**Plan.**

- ~~**Threshold.** Change the partial's `@5xl:` variants to `@3xl:` (48rem) — all twelve, including the inner
  wrapper's `w-full`, so the pane labels, the grid, the empty cells, the right-hand context cells and the
  wrapping all switch together. Update the partial's header comment to name `@3xl` and to state the
  constraint: the threshold must stay below the drift diff page's widest column (56rem), since that page
  keeps the sidebar.~~
- ~~**Optional width.** If the manual check finds the panes too cramped, widen the drift diff page's layout
  (`max-w-7xl` → `max-w-screen-2xl`) on that page only, pass the matching `headerWidth`, and keep the
  header-alignment e2e case green; the documentation pages stay as they are. *Closed without shipping:* the
  manual check did not call for it.~~
- ~~**Tests.** `test/docs.e2e.spec.ts`: the drift and compare diff cases assert the new variant (`@3xl:grid` on
  the right-hand context cell, `@3xl:grid-cols-2` on the rows) instead of `@5xl`. No compiled-CSS assertion
  is added, since `src/styles.css` is not touched; the built CSS is checked by hand for the 48rem container
  query.~~
- ~~**Manual check in a browser** — the step entry 2 skipped. Both diff pages at window widths 900, 1100, 1280
  and 1440px, light and dark: the drift diff is two panes at 900, stacked at 1100, two panes at 1280 and
  1440; the compare diff is two panes at all four; long lines wrap inside their pane; the stacked view still
  scrolls sideways. *Closed as:* since entry 3 the compare diff has the sidebar too and switches at the same
  widths as the drift diff.~~
- ~~**Docs.** `README.md`'s Drift view entry says 64rem; change it to 48rem. `CHANGELOG.md`: completes the
  drift diff's `### Changed` entry under `[Unreleased]` — no new entry, as the side-by-side layout has not
  been released yet. *Shipped as:* `README.md` also states the window widths at which the drift diff
  switches; the changelog entry needed no edit, since it never named a threshold and now simply holds.~~

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

What is settled if it is picked up. **The spine is a facet axis, not the model's grouping fields**: v3
`index.yaml` carries the header `facets` registry and per-resource `facets` (`axis id → value ids`), and both
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
format today, and its one real option is settled per *server* rather than per request: the scheduled axis-index
entry above puts that choice in the `EXPORT_INDEX` environment variable, so the page would still be a route, a
view and a control for a single button that already works. **Revisit** the moment a second whole-tenant format
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
Second, the axis-grouped index no longer poses an open question: the scheduled axis-index entry above settles it
as the `EXPORT_INDEX` environment variable (`type` | `both` | `axis`, default `type`). If this page is built, the
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
would be (a `:target` hack, a route invented only to compensate, a workaround nobody can explain).

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
is now scheduled as entry 1 with two-click link selection. The export standing decision inherits it too: *no
dropdown, no picker widget*.

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

Pay off the 16 findings in `eslint-suppressions.json` — the ones that existed in 9 files when the sonarjs rules
were switched on — a rule at a time, pruning after each, until the file, the `lint:baseline` / `lint:prune`
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
in `confluence.ts`; and `prefer-specific-assertions` in two specs. Two are decisions rather than fixes, and
either way they move **out** of the baseline: `updated-loop-counter` in `findings-table.ts` (the scan assigns
`i = close` to skip a matched table's body — deliberate and documented in place) and `no-os-command-from-path`
in `scripts/working-tree-clean.js` (the preflight resolves `git` through `PATH`); if accepted, each becomes an
`eslint-disable-next-line` at its site, because a baseline must not be where a standing choice hides. Deleting
the file and the two scripts is the last step and is the only operator-visible part, so that one does get a
`CHANGELOG.md` entry.

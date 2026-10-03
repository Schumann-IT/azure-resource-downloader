# Next iterations

Outstanding work and parked ideas for the docs browser. `README.md` says what it does today and `CHANGELOG.md` what
shipped and why; neither is repeated here. How entries and ideas are written, promoted, implemented and
archived is `../.claude/rules/next-iterations.md`.

Numbered entries are scheduled work: committed here before they are implemented, struck through as they land,
and archived to `../.claude/archive/web/` once done. Parked ideas, grouped by area below, are
deliberately unscheduled; each says why it is parked and what would make it worth doing.

## 1. Per-item context in the sidebar

*Kind:* feat

**Goal.** A reader sees what each document in the sidebar is about without opening it: every tree item whose
document carries a one-sentence summary shows it as a short second line under its name, so two similarly named
policies can be told apart at a glance. Items whose document has no summary yet look exactly as they do today.

> **Why now.** The data path is complete: `docs generate-index` copies each document's frontmatter `summary` into
> `docs/index.yaml`, and `buildNavigation()` (`src/docs/tenant-index.ts`) already fills `NavItem.summary`, which
> the listing fallback (`views/tenant.hbs`) renders — only `views/partials/sidebar-tree.hbs` draws nothing. The
> field was empty (0 of 263 and 0 of 148 resources) because the run prompt never asked for it; the go entry
> *Consistent prompt templates, run-prompt fixes and the `summary:` frontmatter line* makes it required, so it fills
> on the next regeneration.
>
> **Reconciled.** The parked idea assumed the compare pages share the tree partial; they have no sidebar (README,
> *Sidebar navigation*), so the tree is drawn only through `views/partials/sidebar.hbs` on the document, resource,
> drift and tenant pages. The `NavItem.note` comment still mentions a compare tree and is corrected here.
>
> **Contract.** The browser reads `resources[].summary` from `docs/index.yaml`: one line of plain text, absent or
> empty meaning no second line. No CLI change.
>
> **Decision.** Badges in the tree: summary only for now; badges are the parked idea *Badges in the sidebar tree*.
>
> **Owner.** none — every file is under `web/`. Sequencing: ships with or after the prompt-consistency pair;
> harmless before the regeneration. Not regeneration-gated (the browser moves no hash).
>
> **Implementer.** sonnet

**Plan.**

- `views/partials/sidebar-tree.hbs`: under each item's label render `{{this.summary}}` (escaped) as a second line,
  only when it is non-empty — small and muted like the item `note`, clamped to two lines, full text in `title`; it
  stays inside the item's `<a>`, so the item remains one link and no anchor is nested.
- `src/styles.css` (or Tailwind utilities in the partial — `line-clamp-2` ships with v4): the clamp, a dark
  variant, and a summary that stays readable on the active item's highlighted background; the existing
  `:focus-visible` outline is unchanged.
- `src/docs/tenant-index.ts`: correct the `NavItem.note` comment (no compare tree exists).
- Tests: `test/docs.e2e.spec.ts` (via `sidebarOf()`) — a resource whose index entry has a `summary` shows it in
  the sidebar; one without adds no second line; a summary containing `<b>` is escaped; the filter, `exempt` and
  *pending* behaviour are unchanged. `test/styles-build.spec.ts`: the clamp rule survives the build.
- Documentation at *done*: `README.md` *Sidebar navigation* (each item shows the document's one-line summary when
  the index carries one); `CHANGELOG.md` `### Added` (Views and navigation), noting the summaries appear once the
  documentation is regenerated with the current CLI.

## 2. The consistency view

*Kind:* feat

**Goal.** A reader of a tenant can open the consistency analysis — the contradictions and redundancies across its
policies — in the browser, rendered like every other document, from the tenant's pages.

> **Contract.** The go entry *The consistency analysis job* adds `<tenant>/consistency/`: `index.md` is the human
> report the browser renders; `findings.yaml`, `mechanical.yaml`, `metadata.yaml`, `analyze.md` and `chunks/` are
> never served. The tree is cleared by a re-baselining download, like `drift/`, so a missing `index.md` is normal.
>
> **Decision.** The browser renders the consistency tree in v1 (shipped as a pair with that go entry).
>
> **Owner.** none — every file is under `web/`. Ships as a pair with go *The consistency analysis job*.

**Plan.**

- `src/docs/path-safety.ts`: a resolver for `consistency/` serving exactly `.md` (one extension, like the drift
  resolvers), re-verifying containment after `realpath()`, refusing `analyze.md` and anything under `chunks/`; cases
  in `test/path-safety.spec.ts`. `consistency/` is not a discovery marker.
- A route behind a representation prefix (declared before the `:tenant/*path` catch-all, like `_drift`) and a view
  rendering `consistency/index.md` through the one `markdown-it` instance, with a clear empty state when the tree is
  missing or not yet analysed; the entry point on the tenant pages decided at plan review.
- Tests: e2e cases for the rendered report, the empty state, and refused paths (`analyze.md`, `chunks/`, `.yaml`).
- Documentation at *done*: `README.md` (route, the served `consistency/` root, the docs-root tree);
  `CHANGELOG.md` `### Added`.

## 3. Align tables by content, not by column position

*Kind:* fix

**Goal.** The tenant summary's *At a glance* table — and any table after it — reads cleanly whatever column order
the documentation agent chose: counts right-aligned and compact, text wrapping at full width. A test fails whenever
a stylesheet rule styles a table column by its position.

> **Why.** `src/styles.css` forces the at-a-glance table's *last* column to `width: 5rem; text-align: right`,
> assuming a count sits there. The go template leaves the table free ("as prose or one small table"), and real
> exports differ: `Area | Resources | Types` squeezes the type list into the 5rem right-aligned column and leaves
> the count left-aligned, while `Area | Types (count) | Resources` happens to look right. `styles-build.spec.ts`
> only checks that a rule survives compilation, not which cells it reaches, so nothing caught it.
>
> **Scope.** Web-only: no contract change, no regeneration; existing exports are fixed on the next request. The
> rule it enforces (tables styled by content, never by position; counts right-aligned) is already in
> `.claude/rules/web-style.md` and its Windsurf twin. The right-alignment is scoped to the at-a-glance section;
> widening it to every document table is not part of this entry. The Confluence export drops `data-*`
> attributes (`html-allowlist.ts`), so it is unaffected, as it was by the positional rule.
>
> **Contract.** Read from the go CLI's current output, unchanged: `docs/summary.md` carries the H2
> `## At a glance` (slug `at-a-glance`, wrapped by `wrapSections` as `[data-section="at-a-glance"]`), whose
> table — if any — has no fixed column order or column names. Positional styling stays only where the order is
> a CLI contract: the summary's Findings table (`Severity | Finding | Affected | Documents`, tagged `.findings`)
> and the drift index's findings table led by Severity (`.findings.findings-drift`); and the label | value
> metadata table every document opens with (`.doc-metadata`). `data-numeric` is a browser-internal attribute,
> not part of the contract.
>
> **Owner.** none — every file is under `web/`. No sequencing.
>
> **Implementer.** sonnet

**Plan.**

- `src/docs/section-hooks.ts`: a pure, exported pass `applyNumericColumns(tokens)` — per table, a column is
  numeric when it has at least one body cell holding an integer and every other body cell is empty or a dash
  placeholder (`-`, `–`, `—`). Integer means `^\d+$` or `^\d{1,3}(,\d{3})+$` after trimming and stripping
  surrounding `*`, `_` and backticks — export `normalise` from `findings-table.ts` (its lowercasing is
  harmless for digits) rather than duplicating it. Every `th` and `td` of such a column gets
  `data-numeric` (attribute present, no value needed). Called from the `doc_sections` rule in
  `markdown-renderer.service.ts`, after `applyMetadataTable` and before `wrapSections`. Every table is tagged;
  the visual effect is scoped in CSS. A short "why" comment on the export, as for the other passes.
- `src/styles.css`: replace the at-a-glance `td:last-child` / `th:last-child` rule with
  `.prose .doc-section[data-section="at-a-glance"] [data-numeric]` (`text-align: right`, `white-space: nowrap`,
  `font-variant-numeric: tabular-nums`, `width: 1%`); at-a-glance `th`/`td` get `vertical-align: top` and
  `overflow-wrap: anywhere`; correct the stale comment ("a sentence per row in its first column") to say the
  column order is the agent's and counts are found by content. Extend the `.doc-metadata` block comment so it
  names the label | value contract its `:first-child` rule relies on (the Findings block already names its
  column contract).
- `test/section-hooks.spec.ts` (inline fixtures, never `output/`): `Area | Resources | Types` and
  `Area | Types (count) | Resources` each tag exactly the count column, header and body; `1,234`, `**12**` and
  `` `7` `` count as integers; a dash placeholder cell keeps the column numeric; a column with one non-numeric
  cell (`12 (3 disabled)`, `1.5`) is not tagged; an all-empty or all-dash column is not tagged; header-only and
  empty tables are untouched.
- `test/docs.e2e.spec.ts`: a rendered `summary.md` fixture in each column order carries `data-numeric` on the
  count cells inside `[data-section="at-a-glance"]` and on no other cell of that table.
- `test/styles-build.spec.ts`: a guard over the compiled CSS. Split each rule's selector list on top-level
  commas (commas inside `:where(…)` / `:not(…)` stay together); a selector is a violation when a `td` or `th`
  compound carries `:first-child`, `:last-child`, `:nth-child(`, `:nth-last-child(`, `:first-of-type`,
  `:last-of-type` or `:nth-of-type(`, unless the selector also carries the class `findings` or `doc-metadata`
  (the contract tables; `findings-drift` only ever appears with `findings`). Two non-violations by
  construction: `@tailwindcss/typography`'s zero-specificity edge-padding defaults, i.e. a positional cell
  selector that sits entirely inside `.prose :where(…)` (`thead th:first-child`, `tbody td:last-child`, …), and
  row or container positions (`tbody tr:last-child`, `.doc-assignments > :first-child`). The test reports every
  offending selector. The matcher is a helper in the spec with its own cases: the removed
  `.prose .doc-section[data-section="at-a-glance"] td:last-child` is a violation; the typography `:where`
  defaults, `.prose table.findings td:nth-child(3)`, `.prose table.doc-metadata th:first-child` and
  `.doc-assignments>:first-child` are not. Plus an assertion that the at-a-glance `[data-numeric]` rule
  survives with `text-align: right`.
- Documentation at *done*: `CHANGELOG.md` `### Fixed`. No README change (no route, flag or setting).

## Parked ideas

**Legend.** *Area* — **contract** (Go → web data on disk: `index.yaml`, `drift/`, frontmatter, section
headings), **templates** (documentation and analysis prompts; *regen-gated* when it moves `promptSha256`),
**export & metadata** (what `resource download` fetches and records), **drift & compare**, **navigation**
(sidebar, breadcrumbs, landing pages, search, routing), **document view** (how one article renders), **export
formats**, **platform rule** (a non-negotiable itself), **dependencies**, **housekeeping** (lint ledgers,
caching internals). *Impact* — operator value: high / medium / low. *Effort* — S (a day or less), M (one
branch), L (several branches or a design change).

**Ships together.**

1. **The pre-regeneration batch** has shipped on both sides (the CA `Conditions` styling, the harmonised heading
   sets and the go templates with the `summary:` field); one documentation regeneration remains. It lights up
   `summary:`, which the scheduled *Per-item context in the sidebar* and the idea *per-document identity* build
   on.
2. **The compare track** (cross-project, must): *Move the compare normalisation to the CLI* ships with go
   *`resource compare`*; *manual pairing* and *a one-sided resource* follow.
3. **The no-JS keystone**: decide *Drop the no-client-side-JavaScript rule* first; *search* (which subsumes the
   name filter), *an actionable findings block*, *explicit dark-mode toggle*, *clickable breadcrumb segments* and
   an export dropdown all wait on it.
4. **Navigation**: *a resource landing page* → *browsable excluded bulk types*; *a two-group navigation tree*
   only as its fallback; *sidebar by a taxonomy axis* after the go regeneration batch fills the group fields.
5. **Exports**: *further export formats* → *one export button and an export page*; *media attachments* with
   Confluence work; *REST synchronisation* is a read-only design change. All under `.claude/rules/web-export.md`.
6. **Dependencies**: *toolchain* → *NestJS 12* and *rendering stack* (golden test first) → *syntax highlighting*.
7. **Housekeeping** is opportunistic: the ESLint ledger is paid off by whichever entry edits a baselined file.

**Suggested order after the scheduled batch** (impact over effort): quick wins — *a resource landing page*,
*summary table of contents*; unblockers — *the toolchain upgrade* and the no-JS decision; strategic — the compare
track, *search*, *further export formats*; then the rest by impact.

## Parked ideas — platform rule

### Idea: Drop the no-client-side-JavaScript rule

*Area:* platform rule · *Impact:* high (keystone) · *Effort:* S to decide, L to follow through · *Ships with:*
decide first: unblocks *search*, *findings block*, *dark-mode toggle*, *clickable breadcrumbs* and an export
dropdown

Lift the non-negotiable that this app ships no script, so the features currently blocked by it become possible.
It is stated in `web/CLAUDE.md` (*Context*), repeated in `.claude/rules/web-style.md` and both Windsurf twins,
and claimed in `README.md`'s Frontend section. **Parked** because it is a rule change, not
a feature: nothing is unblocked until a specific blocked feature is actually wanted, and every one of them is
itself parked. **Revisit** when a feature someone has asked for cannot be built server-side at acceptable cost —
tenant-wide search is the honest candidate — or when the alternative has become visibly worse than the script
would be (a `:target` hack, a route invented only to compensate, a workaround nobody can explain). The tenant
compare's IDE-style comparison pane was checked against the rule and needed no script: statuses are computed
on the server, selecting a row is a link, the selected row is scrolled to with a fragment and the pane is
resized with CSS.

**What the rule does and does not forbid** is stated in `web/CLAUDE.md`: shipped script and client-side state
are banned, HTML interactivity is not — several things that feel scripted are already legal without lifting it.

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
item on this list, parked squarely on it), **a name filter in the sidebar** (no type-ahead without script, and
remembering which sections were open is excluded on purpose), **an actionable findings
block** (expressible only as sibling anchors plus `:target`, which spends the URL fragment `#findings` already
owns and needs a *Show all* reset as part of the feature), **clickable breadcrumb segments** (CSS cannot open a
collapsed `<details>` from an anchor, so it needs a whole new route and view), and **an explicit dark-mode
toggle**. *Tenant diff* used to be the sixth, on the claim that a readable diff of a 317-setting document needs
interaction — the drift view's server-rendered YAML diff disproved that for the per-resource case, and the idea
has since shipped as the tenant compare, with two-click link selection and no script. The export rule
(`.claude/rules/web-export.md`) inherits it too: *no dropdown, no picker widget*.

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

## Parked ideas — navigation

### Idea: Search across a tenant's documents

*Area:* navigation · *Impact:* high · *Effort:* L · *Ships with:* after the no-JS decision (try a server-rendered
`_search` page first); subsumes the name filter

Full-text search, or filtering by resource type and name, over everything a tenant has (including the
resource tree). **Parked** because it is the largest single feature on this list and cannot be done well
without an index and, realistically, client-side interaction — and no client-side JavaScript is a
non-negotiable. **Revisit** when the corpus is large enough that the sidebar tree stops being navigable even
with the shipped taxonomy filters, or if a server-rendered query page turns out to be enough. It subsumes the
name filter in the sidebar idea below. This is the **first feature that would justify relaxing the rule** (its
own idea below): a server-rendered `GET /:tenant/_search?q=` page is worth trying first, because it needs no
script at all and would show whether the interactive version is wanted.

### Idea: Badges in the sidebar tree

*Area:* navigation · *Impact:* low · *Effort:* S · *Ships with:* after *Per-item context in the sidebar*

Show the count-only badges the listing fallback already shows (`assignments`, `scope`, `platforms`, `@odata.type`)
under each sidebar tree item, next to its summary line. The data is there: `NavItem.badges` is filled by
`buildNavigation()` and `views/tenant.hbs` renders it. **Parked** because 263 items with badges would roughly
double the tree's height, and the facet chips already narrow the tree by scope and platform. **Revisit** when
readers ask for the badges in the tree, or if the tree gets a compact/expanded view choice (a query parameter,
like the facets).

### Idea: A name filter in the sidebar

*Area:* navigation · *Impact:* low · *Effort:* M · *Ships with:* subsumed by *search*; easier after the no-JS
decision

Narrow the sidebar by part of a name — a server-side query parameter composing with the shipped taxonomy filters
rather than replacing them. **Parked** because the taxonomy filters cut the 263-item tree to a workable size along
the axes that matter, and because a name filter without type-ahead is an awkward thing to offer. **Revisit** if
narrowing by axis proves insufficient, alongside the search idea above, which subsumes it, or if the
no-client-side-JavaScript rule is relaxed (its own idea below): a text input with type-ahead, and remembering which
sections were open, are the two halves that rule is holding back.

What is settled if it is picked up: it needs nothing from the CLI (display names are in the index). A `?name=`
parameter carried through `selectionHref()` / `toggled()` like the facet selection; validated per the
query-parameter rule in `.claude/rules/web-style.md` (trimmed, length-limited, empty or invalid means no filter,
never a 404); a case-insensitive substring match on `displayName`; the same `exempt` handling and match-count
reconciliation as the facets; a `<form method="get">` text input — in bounds under the no-JS rule, but the app's
first form. Remembering which sections were open across navigations stays **excluded on purpose** for as long as
the no-client-side-JavaScript rule stands: that needs client-side state.

### Idea: A resource landing page

*Area:* navigation · *Impact:* medium · *Effort:* S–M · *Ships with:* carrier for *browsable excluded bulk types*

A new `GET /:tenant/_resource` listing every source YAML (index-derived, plus the excluded types), linked
once from the sidebar footer. Sidebar untouched, no extra tree. **Parked** because it costs a route and a
view to reach resources the top-bar **Documentation | YAML** switcher already gets to. **Revisit** if the
switcher proves hard to find, or as the carrier for browsable excluded bulk types (below).

### Idea: Structure the sidebar by a taxonomy axis instead of by resource type

*Area:* navigation · *Impact:* medium · *Effort:* L · *Ships with:* function-shaped spine needs `functionGroup`
(filled by the go regeneration batch) or a CLI axis; Confluence export must group the same way

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

### Idea: Summary table of contents

*Area:* navigation · *Impact:* low · *Effort:* S · *Ships with:* standalone

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

### Idea: Browsable excluded bulk types

*Area:* navigation · *Impact:* low · *Effort:* M · *Ships with:* after *a resource landing page*; amends a
discovery non-negotiable

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

### Idea: Clickable breadcrumb segments

*Area:* navigation · *Impact:* low · *Effort:* M · *Ships with:* after a per-type listing page, or the no-JS
decision

Turn `Microsoft.Graph / depOnboardingSettings` in the breadcrumb into links to a per-type listing page.
**Parked** because it needs a new route and view: CSS alone cannot open a collapsed `<details>` section from
an anchor, so linking into the existing sidebar tree is impossible without client-side state. **Revisit**
after a per-type listing page exists for another reason, when this becomes additive and nearly free, or if the
no-client-side-JavaScript rule is relaxed (its own idea below), which removes the reason for the new route
entirely: the segment could then open and scroll to its own sidebar section.

### Idea: A two-group navigation tree

*Area:* navigation · *Impact:* low · *Effort:* M · *Ships with:* only if *a resource landing page* does not fix
discoverability

`buildNavigation` returns two top-level `NavGroup`s (*Documentation*, *Resources*), each holding the per-type
`NavSection[]`, with only the group containing the current page `open` and the active marker becoming
`{ kind: 'doc' | 'resource', path }`. Both trees built in one index pass with two href shapes, so they cannot
drift. **Parked** because the resource tree duplicates the doc tree (263 items twice in the reference export)
and `sidebar.hbs` gains a second `<details>` level. **Revisit** only if the **Documentation | YAML** switcher
turns out to be too hard to find and a resource landing page does not fix it.

### Idea: Multi-segment tenants

*Area:* navigation · *Impact:* low · *Effort:* L · *Ships with:* standalone; touches discovery, routing and path
safety

Discovery walks up to 3 levels, but routing uses a single `:tenant` segment, so only top-level tenant
folders are addressable. **Parked** because nested tenants would need a different route shape, which
interacts with every `_`-prefixed representation prefix. **Revisit** the first time a real export tree nests
tenants under a grouping folder.

## Parked ideas — document view

### Idea: Per-document identity on the article

*Area:* document view · *Impact:* low · *Effort:* S–M · *Ships with:* with the first per-family CSS need; reads
the H2 set go templates emit

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
read the headings from `markdown-it`'s tokens (`.claude/rules/web-style.md`).

### Idea: An actionable findings block

*Area:* document view · *Impact:* low · *Effort:* M · *Ships with:* after the no-JS decision (else the `:target`
construction)

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

### Idea: Syntax highlighting inside documents

*Area:* document view · *Impact:* low · *Effort:* S–M · *Ships with:* ride *the rendering stack upgrade* (shiki)

The `yaml`/`bash`/`powershell`/`json`/`xml` fences *inside* the generated Markdown are unstyled.
`@shikijs/markdown-it` could reuse the highlighter `YamlHighlighterService` owns, with the extra languages
loaded. **Parked** because it costs render time on every document for 182 fences in the whole reference
corpus. **Revisit** if documents start carrying substantial code, or once the highlighter is warm anyway for
other reasons.

### Idea: Explicit dark-mode toggle

*Area:* document view · *Impact:* low · *Effort:* S · *Ships with:* blocked by the no-JS rule

Theme selection follows `prefers-color-scheme`, with no way to override it. **Parked** because remembering a
choice needs either client-side state or a cookie plus a mutating route, both of which cut against the
no-JavaScript and read-only rules. **Revisit** if a reader needs one theme in a browser set to the other,
e.g. for a presentation or a screenshot, or if the no-client-side-JavaScript rule is relaxed (its own idea
below) — a toggle that stores the choice client-side is the cheapest thing that relaxation would buy, and it
needs no route and no state on the server.

## Parked ideas — export formats

### Idea: Further export formats and partial exports

*Area:* export formats · *Impact:* medium · *Effort:* M per format · *Ships with:* the second whole-tenant format
triggers *one export button per tenant*

Single-file HTML, DOCX, PDF via a print stylesheet, a Markdown bundle; and exporting one type, one document or
only the summary. **Parked** deliberately: the Confluence exporter is whole-tenant only, and the
`src/docs/export/` seam exists so a second format is a second `ExportService` method plus its own format module,
with the controller and the serialiser untouched. **Revisit** per format when someone actually needs it; a PDF
reuses the existing engine (`.claude/rules/web-export.md`). Preserving the document tree in an export is not
expressible through Confluence HTML import at all — re-parenting by hand or the REST API are the only routes.
Where each of these would be offered is already settled, including why the partial exports are the exception:
`.claude/rules/web-export.md`.

### Idea: One export button per tenant, leading to an export page with per-format options

*Area:* export formats · *Impact:* low–medium · *Effort:* M · *Ships with:* with the second format; amends
`.claude/rules/web-export.md`

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
prefix, declared before the document catch-all like its sibling. Options follow the query-parameter rule in
`.claude/rules/web-style.md` (stable ids, one URL per variant, unknown values fall back to the default). The page is
also the honest place for the things currently crammed onto a picker card: the one-way-publish caveat stated once
per format, and, cheaply, what the export will contain (page count, pending documents, an incomplete index),
all index-derived.

One open question, plus one that is now settled elsewhere. **How the controls are expressed** is undecided, with
three candidates. **(a)** A plain
`<form method="get" action="/:tenant/_export/confluence">` with radio buttons and a submit button — pure HTML,
no script, still read-only, and it scales to several options; it costs the app's first form and first non-anchor
control, and a form cannot carry `download`, so attachment behaviour rests on the `Content-Disposition` header
`ExportService` already sets. **(b)** One `<a download>` per option combination ("Confluence, index by type",
"Confluence, index by axis") — keeps the anchor-only shape the export rule names, but is combinatorial as
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

This amends the export entry-point rule (`.claude/rules/web-export.md`): it already names
`GET /:tenant/_export` as the answer when a row of format links stops fitting, and this makes per-format
options a second, earlier trigger for the same page. What the rule requires is unchanged — the picker stays
the entry point for whole-tenant scope, the link is a plain anchor with no nesting, the caveat travels with it,
and dark mode plus a visible `:focus-visible` outline apply to whatever control the page ends up using.

### Idea: Confluence REST API synchronisation

*Area:* export formats · *Impact:* medium–high · *Effort:* L · *Ships with:* a design change: it mutates a remote
system from a read-only app

Create and update pages in place, with labels, a page tree and attachments — the proper answer to "keep
Confluence up to date". **Parked** because it is a much larger feature than HTML import: authentication, a
persisted mapping from resource to page id, and conflict handling, the last two of which sit awkwardly with
a read-only browser that stores no state. **Revisit** once one-way HTML publishing is in real use and its
re-import cost is felt. Note that it cannot reuse the export link's shape at all — it mutates a remote
system, so it needs a POST rather than an `<a download>` (`.claude/rules/web-export.md`).

### Idea: Media and source YAML as page attachments

*Area:* export formats · *Impact:* low · *Effort:* M–L · *Ships with:* with Confluence work; a third served root

Confluence's HTML import gives each page a media folder named after it, which is where the two images in the
reference corpus — and, in principle, each document's source YAML — could travel. **Parked** because
`resolveWithinRoot()` serves exactly **one** extension per root (`.md` under `docs/`, `.yaml` under
`resources/`), and widening that to an extension list is a non-negotiable: images currently travel as their
`alt` text and the export attaches nothing. Doing it properly means a third served root with its own single
extension policy — a design change, not a feature. **Revisit** if generated documents start carrying
diagrams that matter, or if readers of an imported space ask for the YAML next to the page. It gets no entry
point of its own either way (`.claude/rules/web-export.md`).

## Parked ideas — drift & compare

### Idea: Move the compare normalisation to the CLI

*Area:* drift & compare · *Impact:* high · *Effort:* L · *Ships with:* **must** ship with go *`resource compare`*

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

### Idea: Manual pairing and a rename heuristic for the tenant compare

*Area:* drift & compare · *Impact:* medium · *Effort:* M · *Ships with:* after *move the compare normalisation*
(the heuristic lands in the CLI rule)

The tenant compare pairs resources by their export path (`<APIType>/<endpoint>/<name>`), a heuristic the page
states: two tenants can hold different policies under the same name, and a renamed policy shows as one row only
in each side. Let a reviewer pair an only-in-a row with an only-in-b row by hand (`?left=<key>&right=<key>`, two
steps via links, both keys still required to be listed as present and located only through `resolveResource`),
and offer a **rename heuristic** that suggests such a pair when the two files' normalised hashes are identical —
the per-file digests the comparison pane already keeps make that a lookup, not a read. **Parked** because the
compare is a proof of concept and nobody has yet reported a mispaired or split resource in a real review.
**Revisit** when a reviewer does, or when the one-sided rows of a real pair turn out to be mostly renames.

### Idea: A one-sided resource in the compare's diff area

*Area:* drift & compare · *Impact:* low · *Effort:* S–M · *Ships with:* standalone, or after the move

Selecting a `←` / `→` row in the comparison pane leaves the page for that tenant's YAML view. The reference IDE
instead shows the one file in the area under the pane, so the reviewer keeps their place. It needs the shared
highlighter on the compare pages and a way to say *this side only* in the URL, located through the existing
`resolveResource` and escaped like the YAML view. **Parked** because the YAML view already answers the question
and one-sided rows are read far less often than differing pairs. **Revisit** if reviewers work through the
one-sided rows of a comparison as routinely as the differing ones.

## Parked ideas — dependencies

### Idea: Upgrade the web toolchain — TypeScript 7, Jest 30 and a matching ts-jest

*Area:* dependencies · *Impact:* medium · *Effort:* M · *Ships with:* first: unblocks *NestJS 12* and *the
rendering stack*

Move the test and build toolchain to TypeScript 7 and Jest 30 (with `@types/jest` 30 and a `ts-jest` release that
supports both), and bring `@types/node` back in line with the Node version the project actually targets
(`engines`: Node ≥ 20; Dependabot proposed `@types/node` 26). **Parked** because the first grouped Dependabot pull
request (#42, 2026-10-01) showed it is not a bump: `npm ci` failed with `ERESOLVE` — `ts-jest@29.4.14` declares
`typescript <7` — and Jest 30 changes the ESM handling the suite relies on (`--experimental-vm-modules` for the
ESM-only `markdown-it-anchor` and `shiki`). It needs a ts-jest release with TypeScript 7 support, a check of the
`dynamicImport` escape hatch under the new compiler, and the lint toolchain (`typescript-eslint`) to follow.
**Revisit when** a `ts-jest` release supports TypeScript 7 (or the suite moves to another transformer), or when a
security advisory forces the compiler or Jest forward. Toolchain only — no rendered output may change.

### Idea: Upgrade the rendering stack behind a rendered-output golden test

*Area:* dependencies · *Impact:* medium · *Effort:* L · *Ships with:* after the toolchain; golden test first;
carries *syntax highlighting*

Move the libraries that decide what a reader sees — `markdown-it` 14 → 15, `markdown-it-anchor` 9 → 10, `shiki`
3 → 4, `htmlparser2` 10 → 12, plus `js-yaml` 4 → 5 and `diff` 8 → 9 — but only behind a guard like the CLI's
exported-YAML golden test: fixture Markdown and YAML rendered through the real `MarkdownRendererService`, section
hooks, link rewriting, the Shiki highlighter, the YAML diff, the Confluence allowlist and the PDF content walker,
compared byte for byte with checked-in output. **Parked** because these sit on the non-negotiables (one
`markdown-it` instance with `html: true`, ESM loading through `dynamicImport`, heading slugs and anchors the drift
and compare links target, the allowlist serialiser behind the Confluence export and the drift PDF) and a quiet
change in rendered HTML would surface only where a test happens to look. Bundled into the failed grouped pull
request (#42) with no such guard. **Revisit when** a security advisory touches one of them, or the toolchain
upgrade has landed; build the golden test first (on today's versions, like the CLI did), then move the libraries
one at a time, each proven by it.

### Idea: Upgrade to NestJS 12

*Area:* dependencies · *Impact:* low–medium · *Effort:* M · *Ships with:* after the toolchain

Move `@nestjs/common`, `@nestjs/core`, `@nestjs/platform-express`, `@nestjs/testing`, `@nestjs/cli` and
`@nestjs/schematics` from 11 to 12 together. **Parked** because it was bundled into the failed grouped pull request
(#42) and is a framework major: it touches bootstrap and `configureViews` (security headers, static assets, the
`hbs` view engine), the controller routing order the representation prefixes depend on (`_resource`, `_drift`,
`_export` before the `:tenant/*path` catch-all), Express integration and the e2e wiring through `AppModule`.
**Revisit when** NestJS 11 leaves support or a security advisory requires 12; do it as its own entry, after the
toolchain upgrade, with the e2e suite (routes, 404 kinds, headers, the read-only invariants) as the proof.

## Parked ideas — housekeeping

### Idea: Clear the ESLint suppressions baseline

*Area:* housekeeping · *Impact:* low · *Effort:* M · *Ships with:* opportunistic: whichever entry edits a
baselined file

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
a separate, permanent parity decision and are not part of this. The procedure (rule by rule, prune after each,
what gets a changelog entry) is the ledger paragraph in `.claude/rules/web-style.md`. Only
`sonarjs/super-linear-regex` (6: `page-name.ts` ×2, `findings-table.ts` ×2, `link-rewrite.ts`,
`section-hooks.ts`) changes behaviour — these patterns run over generated documents, so each rewrite needs a spec
case pinning the same accepted and rejected inputs. The rest are refactors the existing suite covers and are
internal, carrying no `CHANGELOG.md` entry unless a reader sees a difference: `cognitive-complexity` (4:
`confluence.ts`, `export/html-allowlist.ts`, `section-hooks.ts`, `tenant-index.ts`), split along the seams those
functions already have and keeping the pure/Nest-free split intact; `misplaced-loop-counter` in `page-name.ts`, a
`while` written as a `for`; `no-nested-template-literals` in `confluence.ts`; and `prefer-specific-assertions` in
two specs. One is a decision rather than a fix, and either way it moves **out** of the baseline:
`updated-loop-counter` in `findings-table.ts` (the scan assigns `i = close` to skip a matched table's body —
deliberate and documented in place); if accepted, it becomes an `eslint-disable-next-line` at its site, because a
baseline must not be where a standing choice hides — exactly what happened to `no-os-command-from-path`, which
left the ledger when the git calls moved into `scripts/lib/git.js` behind one directive with its reason.

### Idea: Watch-based cache invalidation

*Area:* housekeeping · *Impact:* low · *Effort:* M · *Ships with:* standalone

An `fs.watch` layer could pre-warm and evict cache entries instead of validating them per request. **Parked**
because the per-request `stat()` delivers the no-restart freshness invariant at negligible cost. **Revisit**
if `stat()` becomes measurable on a slow or networked docs root.

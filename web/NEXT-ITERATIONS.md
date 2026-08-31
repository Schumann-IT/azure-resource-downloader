# Next iterations

Outstanding work, standing decisions and parked ideas for the docs browser. `README.md` describes what it
does today and `CHANGELOG.md` records what shipped; neither is repeated here.

## 1. Section-level styling hooks for the document page

**Goal.** Make the parts of a rendered document individually addressable from CSS, so sections, settings and
the assignments block can be styled distinctly instead of all reading as one undifferentiated article.

> **The gaps, in priority order.** No section wrapper — everything between two H2s is a flat sibling run, so
> a section cannot get a panel or its own density without `h2#settings ~ *` selectors that bleed into the
> next section; this is the main gap and the only structural change on the list. The assignments block is
> invisible to CSS: `<!-- assignments:start -->` / `<!-- assignments:end -->` survive into the DOM
> (`html: true`) but HTML comments are not selectable, even though the markers are a *tool-maintained
> contract* and therefore a better hook than any positional selector. The metadata table has no class, and
> `table:first-of-type` breaks the moment a document opens with an extra heading (99 documents in the
> reference exports do). `Lifecycle & operations` slugs to `lifecycle-%26-operations`, usable only as
> `[id="lifecycle-%26-operations"]`. Setting `<details>` depth is positional. `<article>` has no
> per-document hook, so a group document and a Win32 app document render identically. Template chrome (the
> source/generated line, the layout grid, `<main>`, the view switcher) is pure Tailwind utilities, so
> `src/styles.css` cannot reach it.
>
> **Waiting on a regeneration.** These hooks key off the H2 vocabulary, which the CLI now declares as a
> closed, verbatim set per template (carried machine-readably in a `<!-- doc-headings: … -->` marker), along
> with `data-setting` / `data-note` on setting `<details>` blocks. **The documents on disk predate all of
> it**, and still show the old drift (`## Metadata` in 99 of them, `## Assignments` in 4). Everything up to
> and including the metadata-table class degrades gracefully on them (an unrecognised heading simply gets an
> unstyled `data-section`); the section wrapper and any per-section visual treatment should land after the
> regeneration, and `data-setting` / `data-note` styling is worth building only once documents carry them.
>
> **Not asked of the generator on purpose.** Section wrappers, the metadata-table class and the assignments
> wrapper are all derivable here, and 340+ documents of hand-written wrapper tags would eventually produce
> unbalanced HTML that only a regeneration could fix.

**Plan.** Cheapest first; the first two are self-contained and worth doing on their own. All of it stays
server-side and keeps the single `markdown-it` instance, and each step needs a case in
`test/docs.e2e.spec.ts`.

- **Semantic classes in the templates**: `#doc-page` on `<body>`, `.doc-main`, `.doc-source` on the
  source/generated line, `.doc-body` alongside `prose` on `<article>`, `.site-header`, `.view-switcher`.
  Template-only, no renderer change.
- **`data-section` on headings**: a `heading_open` rule adding `class="doc-section-heading"` and
  `data-section="<slug>"`, plus a custom `slugify` that drops the percent-encoding (`%26` from
  `Lifecycle & operations`, `%E2%80%94` from the em dash in every `summary.md` H1). Enables
  `[data-section="security"]` without touching document structure; the slug change alters existing anchor
  URLs, so it needs a changelog note.
- **Turn the assignment markers into elements**: an `html_block` rule mapping the `assignments:start`/`end`
  and `targeted-by:start`/`end` marker pairs to `<div class="doc-assignments">` and `.doc-targeted-by`.
- **Class the metadata table** by its position *after* the H1 and summary paragraph rather than by
  `first-of-type`.
- **`data-family` / `data-type` on `<article>`**, from the document path and (for the family) from
  `docs/index.yaml` — the CLI knows which prompt template a type uses; the frontend must not guess.
- **Wrap each H2 run in `<section class="doc-section" data-section="…">`** via a token post-process. Solves
  the main gap and is the only invasive item; it also changes the `details details` depth selectors, so do it
  last and only when per-section backgrounds are actually wanted.

## 2. Findings block beyond the severity icon

**Goal.** Let a reader of the tenant landing page act on the findings table — narrow it to what matters and
get from a finding to the documents it affects.

> The hooks are in place — `src/docs/findings-table.ts` tags the table `.findings` and puts `data-severity`
> on each body row and severity cell — but nothing consumes `data-severity` beyond the severity icon, the
> `Affected` count is inert, and `#findings` is an unstyled H3 above a styled table.
>
> **Caveat.** `.prose table` sets `white-space: nowrap` so wide assignment tables scroll instead of wrapping
> mid-GUID. Any prose-bearing table needs an opt-out from it.

**Plan.**

- Put `data-severity` on a wrapper (or a per-severity count line) so a severity summary and CSS-only
  filtering become expressible without client-side state.
- Link the `Affected` count to the documents it names, reusing the same relative-link rewriting as document
  bodies.
- Style `#findings` itself.

## 3. Summary table of contents

**Goal.** Give the tenant landing page a jump list so a reader can go straight to the part of the summary
they came for instead of scrolling.

> The four summary H2s are a declared contract on the CLI side with stable, clean slugs —
> `#management-summary`, `#at-a-glance`, `#assignment-posture`, `#coverage-caveats` — so they are safe to
> hard-code, and `markdown-it-anchor` emits the ids without further work.
>
> The landing page's structure is tight (a heading followed by one list or a short run of paragraphs), so
> positional selectors that would be fragile in a resource document hold up here (`#recommendations + ol`,
> `[id="at-a-glance"] ~ p`) and no renderer change is needed.

**Plan.**

- Render the jump list in `tenant.hbs` from the known heading set, skipping entries whose heading is absent
  so an older summary degrades to a shorter list.
- Assert in `test/docs.e2e.spec.ts` that the list links to the ids the rendered summary actually contains.

## 4. Sidebar refinements

**Goal.** Make the sidebar usable at export scale, where it lists every in-scope document (263 in the
reference export) as a flat per-type tree with no way to narrow it and no context per item.

> The item summaries and badges the index carries are rendered only by the listing fallback, not by the tree.
>
> **Excluded on purpose:** remembering which sections were open across navigations. That needs client-side
> state, and no client-side JavaScript is a non-negotiable.

**Plan.**

- A filter over the tree that works without JavaScript (server-side query parameter narrowing the rendered
  items, with the current filter reflected in the URL).
- Per-item summary/badge rendering in `sidebar.hbs`, from the same `docs/index.yaml` fields the listing
  fallback reads.

## 5. Settle how a settings block is represented in the Confluence export

**Goal.** Make an imported space usable for the reader who came for one setting. The export ships the
`<details>` blocks untouched, which is a probe rather than an answer: nobody has yet seen what the importer
does with them.

> **What the corpus contains** (two reference exports, 411 served documents, 6.4 MB of Markdown):
> **7,208 `<details>`/`<summary>` blocks** — up to 317 in one document, nested up to 5 deep — 14,345 inline
> `<code>`, 6,048 table rows and 182 fenced code blocks (which
> [the importer flattens to plain text](https://support.atlassian.com/confluence-cloud/docs/faq-import-data-from-html-to-confluence/)).
> A single page therefore has to carry up to 317 settings, five levels deep, with no collapse affordance if
> the importer drops the block.
>
> **Why it is open.** `<details>` is the dominant element and the architecture notes call it *the*
> documentation, it is not on the importer's preserved list, and the FAQ does not say what it does with it.
> No Confluence instance was available to settle it, and choosing between the transforms from the FAQ alone
> would be guessing — so passthrough shipped, and the README calls the export provisional. The
> alternatives, for whoever runs the first real import: **A, bold paragraph + blockquote** (lowest effort,
> keeps nesting visually and the `path = value` summary verbatim, loses the collapse affordance — a
> 317-setting document becomes one very long page); **B, headings by depth** (the only option giving each
> setting its own anchor and search entry, but a 317-entry page outline, and depth 5 does not fit under
> `h6`); **C, one table per settings section** (most information per screen and matches how these documents
> are read, but flattens nesting, has no home for group-label blocks such as `Sub-options (1)`, and is the
> only option that must **parse** the summary into key/value — so the only one that can be silently wrong
> when a value itself contains ` = `). Passthrough is what makes that import cheap to run.
>
> **The seam already exists.** `src/docs/export/details-strategy.ts` has one member and one decision
> function; a transform is a second member plus a branch in the serialiser, which works on the rendered DOM,
> so nothing about the single `markdown-it` instance or its cache is involved. `test/export.spec.ts` holds
> the shared fixture the transforms have to answer for — a nested block, a group-label block with no value,
> a link inside a block, and a value that itself contains ` = ` (the trap that makes C parse-dependent).
> `?details=` already 404s for anything not implemented, so adding a second value is additive.

**Plan.**

- **Import one passthrough zip into a scratch space** and answer four questions: does the block survive at
  all, is the 317-setting document usable, does each setting have an anchor, and does search find a setting
  name. Also check the page size the importer accepts — the largest current page is 50 kB of HTML.
- **Record the answer in this entry**, so the next attempt does not re-run the experiment.
- **If passthrough survives**, drop the *provisional* wording from the README's Confluence section and note
  in `CHANGELOG.md` what the import actually does with the block.
- **Otherwise implement exactly one transform** as a second `DetailsStrategy`, against the fixture that is
  already in `test/export.spec.ts`. Accept both values on `?details=` while the comparison is open, then
  default to the winner and delete the loser — a query parameter is not a configuration mechanism and must
  not outlive the comparison.
- **Re-check the assignment and metadata tables** in whichever representation wins: `.prose table` does not
  travel, so a table that relied on the browser's horizontal scroll may need different markup.

## Standing decisions

Decisions that are not work items but constrain the work items below, recorded so the next iteration does
not relitigate them.

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
  dropdown, no picker widget** — that needs client-side JavaScript, which is a non-negotiable. If the row of
  formats ever stops fitting, the answer is a per-tenant export *page* (`GET /:tenant/_export`) listing the
  formats, not a control that needs scripting.
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
document catch-all; the one-way/provisional caveat rendered next to the link rather than only in the README;
no anchor nested inside another anchor (the picker card is a wrapper element for exactly this reason); and a
dark-mode variant plus a visible `:focus-visible` outline.

## Parked ideas

### Idea: Search across a tenant's documents

Full-text search, or filtering by resource type and name, over everything a tenant has (including the
resource tree). **Parked** because it is the largest single feature on this list and cannot be done well
without an index and, realistically, client-side interaction — and no client-side JavaScript is a
non-negotiable. **Revisit** when the corpus is large enough that the sidebar tree stops being navigable even
with the planned filter, or if a server-rendered query page turns out to be enough.

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

### Idea: Group-driven navigation

`docs/index.yaml` can carry `platformGroup`/`functionGroup` per resource, so the landing page and sidebar
tree could become platform → function instead of grouping by resource type. **Parked** because the documents
do not write those fields — the CLI generation template's required frontmatter does not ask for them — so
there is nothing to group by. **Revisit** when the template requires both fields and a regenerated export
carries them.

### Idea: Explicit dark-mode toggle

Theme selection follows `prefers-color-scheme`, with no way to override it. **Parked** because remembering a
choice needs either client-side state or a cookie plus a mutating route, both of which cut against the
no-JavaScript and read-only rules. **Revisit** if a reader needs one theme in a browser set to the other,
e.g. for a presentation or a screenshot.

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
after a per-type listing page exists for another reason, when this becomes additive and nearly free.

### Idea: Browsable excluded bulk types

Types the index merely counts under `counts.excluded` (Autopilot identities and the like), and any resource
with no document, are unreachable — which is what keeps navigation purely index-derived and the **"counts and
listings derive from the index, never from walking the tree"** non-negotiable intact. Unreferenced
`Microsoft.Graph/groups` (the CLI documents a group only when an assignment references it) are in the same
bucket. **Parked** because serving them requires amending that non-negotiable. **Revisit** if operators ask
for the raw bulk YAML; the shape is settled — for each type named in `counts.excluded`, a single
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

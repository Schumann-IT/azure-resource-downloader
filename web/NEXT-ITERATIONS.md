# Next iterations

Outstanding work and parked ideas for the docs browser. `README.md` describes what it does today and
`CHANGELOG.md` records what shipped; neither is repeated here.

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

## 5. Export a tenant's documentation as Confluence HTML

**Goal.** Publish a tenant's documentation into systems the operator's organisation already reads, without
turning the browser into a writer. Several formats are wanted; Confluence HTML is the first, and the seam
matters more than the format.

> **The Confluence contract**, from [Atlassian's HTML import FAQ](https://support.atlassian.com/confluence-cloud/docs/faq-import-data-from-html-to-confluence/),
> which the exporter has to satisfy exactly: upload is **one `.zip` containing one folder**, the folder name
> becomes the space name, each `.html` file becomes a page and **the file name becomes the page title**; a
> page's media sits in a folder named after that page; import needs create-space permission and an
> attachment size limit above the zip. Preserved: headings, paragraphs, bold, italic, links, images, tables,
> lists, quotes, dividers, inline code, superscript, centre alignment, emoji. Not preserved: `<title>`,
> `<figure>`, `<nav>`, `<iframe>`, `<button>`, audio — and **code blocks arrive as plain text**.
>
> **Two consequences.** There is **no page hierarchy** — a space is a flat set of pages, so our
> `docs/<type>/<name>.md` tree and the sidebar cannot come along (`<nav>` is unsupported anyway). And import
> **creates** a space rather than updating one, so re-importing yields a second space or a conflict: this is
> a **one-way publish** and the README must say so.
>
> **What the corpus contains** (two reference exports, 411 served documents, 6.4 MB of Markdown):
> **7,208 `<details>`/`<summary>` blocks** — up to 317 in one document, nested up to 5 deep — 14,345 inline
> `<code>`, 6,048 table rows, 182 fenced code blocks (which flatten to plain text), 2 images, and **44
> literal angle brackets in prose** (`<key>`, `<endpoint>`, `<string>`, …). That last one is a latent bug the
> export surfaces: macOS plist payloads are quoted with bare angle brackets and `html: true` already turns
> them into bogus elements in the browser — harmless there, unknown on import. So the exporter must
> serialise through an **allowlist that escapes anything not in it**.
>
> **`<details>` is the one open decision.** It is the dominant element and the architecture notes call it
> *the* documentation, it is not on the supported list, and the FAQ does not say what happens to it — so
> passing it through is a gamble on undefined behaviour. Four candidates, to be compared from a real import
> rather than from the FAQ: **A, bold paragraph + blockquote** (lowest effort, keeps nesting visually and the
> `path = value` summary verbatim, loses the collapse affordance — a 317-setting document becomes one very
> long page); **B, headings by depth** (the only option giving each setting its own anchor and search entry,
> but a 317-entry page outline, and depth 5 does not fit under `h6`); **C, one table per settings section**
> (most information per screen and matches how these documents are read, but flattens nesting, has no home
> for group-label blocks such as `Sub-options (1)`, and is the only option that must **parse** the summary
> into key/value — so the only one that can be silently wrong when a value itself contains ` = `);
> **D, passthrough**, not a shipping candidate but the cheapest way to turn the FAQ's silence into a fact,
> and if the importer keeps the block it beats all three transforms.
>
> **Traps.** The render cache is keyed by file path only, so an export render would return or poison the
> browser's HTML, and the second `<details>` strategy tried would silently serve the first one's output —
> mode and strategy must be part of the cache key, or export renders must not be cached while the strategies
> are compared. Page titles come from basenames, which are unique in both reference tenants today (the CLI
> dedupes slugs with a hash suffix) but must hard-fail rather than overwrite on a collision; the longest is
> 87 characters, so check Confluence's title limit.

**Plan.**

- **Route**: `GET /:tenant/_export/confluence` → `200 application/zip` with
  `Content-Disposition: attachment; filename="<tenant>.zip"`. `_export` is a representation prefix like
  `_resource`, so declare it **before** the `:tenant/*path` catch-all; no resource type segment starts with
  `_`, so it cannot collide.
- **Layout** under `src/docs/export/`: `confluence.ts` (the format), `html-allowlist.ts` (the serialiser),
  `details-strategy.ts` (`type DetailsStrategy = 'blockquote' | 'headings' | 'table' | 'passthrough'`), each
  pure and Nest-free, plus one thin `ExportService` doing the zip and the streaming so a second format drops
  in without touching the controller. Add a streaming zip dependency (`archiver` or `yazl`).
- **Zip contents**: `<tenant>/` (the space name) holding a `<Tenant> overview.html` built from `summary.md`
  plus a grouped link list that replaces the sidebar, one flat `<page>.html` per document, and a `<page>/`
  folder only for pages that have media.
- **Select the `<details>` strategy with a query parameter** (`?details=headings`), defaulting to A. A route
  parameter, not a new configuration mechanism — the environment-variables-only rule stands and the
  browser's own rendering is unchanged.
- **Run the comparison spike**, passthrough first: import one zip per strategy into a scratch space and
  check whether the block survives, whether the 317-setting document is usable, whether per-setting anchors
  exist and whether search finds setting names. Record the answer here and **delete the losing strategies**.
- **Links**: add a second `rewriteHref` mode producing `<a href="<Page Name>.html">`. Keep the single
  `markdown-it` instance by making the mode part of the existing env, not a second instance. In-document
  anchors (`#security`) will not survive — accepted loss.
- **Provenance**: frontmatter is stripped before rendering, so emit `source`, `generatedAt` and the shas as
  a small table at the top of each page, with a line saying the page is generated and local edits are lost
  on the next import.
- **Keep read-only honest**: build the zip in memory and stream it; nothing is written under `DOCS_ROOT`,
  and an unavoidable temp file belongs in `os.tmpdir()`. Enumerate documents from `docs/index.yaml` and read
  each through `resolveWithinTenant()`. Offer the export as a plain download link in the tenant top bar — no
  client-side JavaScript.
- **Omit the YAML view** in v1; attaching source YAML as page media is a follow-up.
- **Tests**: e2e for the content type, `Content-Disposition` and a `<tenant>/<page>.html` entry from a
  fixture; unit tests for each strategy against one shared fixture (nested block, group-label block with no
  value, a body with a link, a value containing ` = `), the allowlist serialiser **including that `<key>` in
  prose is escaped**, the export link mode and the title-collision failure; a case proving two strategies of
  the same document return different bodies (this is what catches the cache-key trap); and an assertion that
  the export never writes inside the docs root. No network, temp-dir fixtures as everywhere else.

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

### Idea: Confluence REST API synchronisation

Create and update pages in place, with labels, a page tree and attachments — the proper answer to "keep
Confluence up to date". **Parked** because it is a much larger feature than HTML import: authentication, a
persisted mapping from resource to page id, and conflict handling, the last two of which sit awkwardly with
a read-only browser that stores no state. **Revisit** once one-way HTML publishing is in real use and its
re-import cost is felt.

### Idea: Further export formats and partial exports

Single-file HTML, DOCX, PDF via a print stylesheet, a Markdown bundle; and exporting one type, one document
or only the summary. **Parked** deliberately: whole-tenant Confluence HTML first, and the `src/docs/export/`
seam exists so none of these has to rewrite the controller. **Revisit** per format when someone actually
needs it. Preserving the document tree in an export is not expressible through Confluence HTML import at all
— re-parenting by hand or the REST API are the only routes.

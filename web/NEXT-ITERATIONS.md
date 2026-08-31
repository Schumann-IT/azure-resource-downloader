# Next iterations — deliberately out of scope for iteration 1

Iteration 1 only discovers tenants and renders their Markdown as HTML. Things noticed while building it,
left out on purpose:

- **Search.** Full-text or type/name filtering across a tenant's documents.
- **Syntax highlighting inside documents.** `shiki` now highlights the source-YAML view, but the
  `yaml`/`bash`/`powershell`/`json`/`xml` fences *inside* the generated Markdown are still unstyled.
  `@shikijs/markdown-it` could reuse the highlighter `YamlHighlighterService` already owns (it would need
  the extra languages loaded).
- **Multi-segment tenants.** Discovery walks up to 3 levels, but routing uses a single `:tenant` segment,
  so only top-level tenant folders are addressable. Nested tenants would need a different route shape.
- **Group-driven navigation.** `docs/index.yaml` can carry `platformGroup`/`functionGroup` per resource,
  but the documents do not yet write them (the generation template's required frontmatter does not ask
  for them), so the landing page groups by resource type and shows the groups as badges when present.
  Once the template requires them, the tree (and the sidebar planned below) can become platform → function.
- **Summary table of contents.** `markdown-it-anchor` already gives the summary's `##` headings ids; a
  jump list (Management summary / At a glance / Assignment posture / Coverage caveats) is not built. Those
  four headings are now a declared contract on the CLI side, so the slugs are safe to hard-code — see
  *Section-level styling hooks* below.
- **Explicit dark-mode toggle.** Currently follows `prefers-color-scheme` only.
- **Watch-based cache invalidation.** The cache is mtime-checked per request; an `fs.watch` layer could
  pre-warm/evict, but per-request `stat()` already gives no-restart refresh.
- **Sidebar refinements.** The sidebar lists every in-scope document (263 in the reference export) with
  no filter and no item summaries/badges — those are only on the index-listing fallback. It also does not
  remember which sections were open across navigations (that would need client-side state).

## Section-level styling hooks for the document page

`GET /:tenant/*path` renders the whole document into one `<article class="prose">`, so almost nothing in it
can be addressed. An audit of the two reference exports found these hooks *already usable*: `.prose details`
/ `.prose summary` / `.prose details details`, `h2[id]` and `.header-anchor` from `markdown-it-anchor`
(`#references`, `#security`, `#settings`, `#properties`, `#membership`, `#targeted-by`, `#definition`, …),
`.prose > h1`, `.prose > h1 + p` (the summary paragraph, reliable because `stripSourceEcho` removes the
`` `x.yaml` `` echo first), `.prose > table:first-of-type`, and `.nav-tree` in the sidebar.

What is missing, roughly in priority order:

- **No section wrapper.** Everything between two H2s is a flat sibling run, so a section cannot get a panel,
  a tint or its own density without `h2#settings ~ *` selectors that bleed into the next section. This is the
  main gap and the only structural change on the list.
- **The assignments block is invisible to CSS.** `<!-- assignments:start -->` / `<!-- assignments:end -->`
  survive into the DOM (`html: true`) but HTML comments are not selectable, so the assignments intro
  paragraph and table are indistinguishable from any other paragraph and table — even though the markers are
  a *tool-maintained contract*, and therefore a better hook than any positional selector.
- **The metadata table has no class.** `table:first-of-type` breaks the moment a document opens with an extra
  heading — which 99 documents in the reference exports do (`## Metadata`, see the Go side below).
- **`Lifecycle & operations` slugs to `lifecycle-%26-operations`.** Usable only as
  `[id="lifecycle-%26-operations"]`, and an ugly deep link.
- **Setting `<details>` depth is positional.** `.prose details details` is the only way to distinguish a
  nested sub-setting, and it breaks as soon as sections are wrapped.
- **No per-document hook on `<article>`.** A group document and a Win32 app document render identically;
  neither the resource type nor the document family can be targeted.
- **Template chrome has no semantic classes.** The source/generated line, the layout grid, `<main>` and the
  view switcher are pure Tailwind utilities, so `src/styles.css` cannot reach them.

Planned shape, cheapest first — the first two are self-contained and worth doing on their own:

1. **Semantic classes in the templates.** `#doc-page` on `<body>`, `.doc-main`, `.doc-source` on the
   source/generated line, `.doc-body` alongside `prose` on `<article>`, `.site-header`, `.view-switcher`.
   Template-only, no renderer change.
2. **`data-section` on headings.** A `heading_open` rule adding `class="doc-section-heading"` and
   `data-section="<slug>"`, plus a custom `slugify` that drops the percent-encoding — `%26` from
   `Lifecycle & operations` (now renamed away on the Go side) and `%E2%80%94` from the em dash in every
   `summary.md` H1. Immediately enables `[data-section="security"]` without touching document structure. The
   slug change alters existing anchor URLs, so it needs a changelog note.
3. **Turn the assignment markers into elements.** An `html_block` rule mapping the
   `assignments:start`/`end` and `targeted-by:start`/`end` marker pairs to `<div class="doc-assignments">`
   and `.doc-targeted-by`. Small, and it consumes a contract the CLI already guarantees.
4. **Class the metadata table** by its position *after* the H1 and summary paragraph rather than by
   `first-of-type`.
5. **`data-family` / `data-type` on `<article>`**, from the document path and (for the family) from
   `docs/index.yaml` — the CLI knows which prompt template a type uses; the frontend must not guess.
6. **Wrap each H2 run in `<section class="doc-section" data-section="…">`** via a token post-process. Solves
   the first gap, and is the only invasive item — deferred until per-section backgrounds are actually
   wanted, since it also changes the `details details` depth selectors.

All of it stays server-side and keeps the single `markdown-it` instance; each step needs a case in
`test/docs.e2e.spec.ts`.

**Depends on the Go side — now unblocked.** These hooks key off the H2 vocabulary, which used to be only
*described* by the prompt templates and had drifted (`## Metadata` in 99 documents, `## Assignments` in 4).
That is fixed: `../go/NEXT-ITERATIONS.md` §1 has shipped — all seven templates declare a closed, verbatim H2
set (carried machine-readably in a `<!-- doc-headings: … -->` marker and checked by the generation template's
§4 script), the three `&` headings were renamed, and the setting `<details>` blocks now carry
`data-setting` / `data-note`. **What has not happened is the regeneration**, so the documents on disk still
predate all of it.

Steps 1–4 degrade gracefully on today's documents (an unrecognised heading simply gets an unstyled
`data-section`), so they need not wait. Step 6 and any per-section visual treatment should land after the
regeneration. `data-setting` / `data-note` hooks are worth building only once documents actually carry them.

### The tenant landing page is the first page that can be styled

`docs/summary.md` does not feed `promptSha256`, so it can be reissued on its own — and one tenant already
was. A regenerated `iis.mitarbeiterangebote-staging.de` summary renders exactly four H2s with clean slugs and
zero drift: `#management-summary`, `#at-a-glance`, `#assignment-posture`, `#coverage-caveats`. Because the
structure is tight (a heading followed by one list or a short run of paragraphs), positional selectors that
would be fragile in a resource document hold up here, so `tenant.hbs` can be styled **before** any of the
steps above — `#recommendations + ol` and `[id="at-a-glance"] ~ p` hold up without any renderer change.

**Shipped: the Findings table.** Both tenants have since been regenerated, and the findings are now a table
(`Severity | Finding | Affected | Documents`, severity from the closed set `critical`/`high`/`medium`) rather
than a list. It is rendered by a core rule in `src/docs/findings-table.ts` that tags the table `.findings` and
puts `data-severity` on each body row and severity cell, with `src/styles.css` drawing the severity as a
masked SVG icon. Two decisions worth keeping:

- **Keyed off the `Severity` header cell, not the `### Findings` heading.** The heading is equally
  contractual, but matching on columns means the table keeps its treatment wherever the generator moves it,
  and no other table in the corpus leads with a Severity column. `#findings` remains available for styling
  the heading itself.
- **Fixed a general bug in passing.** `.prose table` sets `white-space: nowrap` so wide assignment tables
  scroll instead of wrapping mid-GUID. The findings table inherited it and ran its prose off-screen; any
  future prose-bearing table needs the same opt-out.

Still open for the findings block: `data-severity` is not yet used for anything beyond the icon (filtering or
a severity summary would need it on a wrapper), and the `Affected` count is not linked to anything.

Also unblocked by the same change: the **summary table of contents** listed at the top of this file. The four
headings are now a declared contract with stable slugs, so a jump list no longer risks pointing at headings
that may be renamed.

Deliberately **not** requested from the generation prompt: section wrappers, the metadata-table class and
the assignments wrapper. All three are derivable here, and 340+ documents of hand-written wrapper tags would
eventually produce unbalanced HTML that only a regeneration could fix.

## Resource (YAML) rendering — shipped as option A

Implemented: `<export>/resources` is a second read-only served root, `GET /:tenant/_resource/*path`
renders the source YAML (shiki, `#L42` line anchors, `?raw` plain text, escaped `<pre>` above 512 KB), and
a **Documentation | YAML** switcher in the top bar flips between the two representations — suppressed when
a document has no `source` frontmatter. Only resources listed in `docs/index.yaml` are reachable; a
document's source is found by inverting the CLI's `docs/<type>/<name>.md` ↔ `resources/<type>/<name>.yaml`
mapping, so no directory is walked. See `CHANGELOG.md` for the invariants and the README for the routes.

The navigation alternatives considered alongside option A are kept here as possible follow-ups:

- **Option B — switcher plus a resource landing page.** Sidebar untouched, no extra section. A new
  `GET /:tenant/_resource` lists every source YAML (index-derived, plus the excluded types), linked once
  from the sidebar footer. Keeps the tree lean and gives the bulk types a real home, at the cost of one
  more route and view. Natural follow-up to A: it is where the bulk types (and unreferenced groups) would
  get a real home.
- **Option C — switcher plus a two-group tree.** `buildNavigation` returns two top-level `NavGroup`s
  (*Documentation*, *Resources*), each holding the existing per-type `NavSection[]`; only the group
  containing the current page renders `open`, and the active marker becomes
  `{ kind: 'doc' | 'resource', path }`. Both trees are built in one index pass with two href shapes, so
  they cannot drift. Most navigable and most explicit, but the resource tree duplicates the doc tree
  (263 items twice in the reference export) and `sidebar.hbs` gains a second `<details>` level (plus a
  `.nav-tree details details` style). Only worth doing if the switcher turns out to be too hard to find.
- **Option D — clickable breadcrumb segments (additive; last, if at all).** Turn
  `Microsoft.Graph / depOnboardingSettings` in the breadcrumb into links to a per-type listing page. Needs
  a new route and view: CSS alone cannot open a collapsed `<details>` section from an anchor, so linking
  into the existing sidebar tree is not an option without client-side state.

**Excluded bulk types — deliberately not browsable.** Types the index merely counts under
`counts.excluded` (Autopilot identities and the like), and any resource with no document at all, have no
YAML view. That is what keeps navigation purely index-derived and left the **"counts and listings derive
from the index, never from walking the tree"** non-negotiable untouched.

If they are wanted later (option B is the natural carrier), the shape is settled: for each type named in
`counts.excluded`, a single **non-recursive** `readdir` of `resources/<type>/` (`.yaml` only, sorted, cached
under the existing discovery TTL, unreadable ⇒ empty), producing file names only — counts shown to the user
would still come from the index. That change *does* need the rule amended, and it reopens two UX questions:
whether to flag a `readdir`-vs-`counts.excluded` mismatch as a stale index, and whether resources without
documentation should be visually de-emphasised. Unreferenced `Microsoft.Graph/groups` (the CLI documents a
group only when an assignment references it) fall in the same bucket.

Also left out: any search or filter over the resource tree, and highlighting the code fences inside the
generated Markdown (see the syntax-highlighting item at the top of this file).

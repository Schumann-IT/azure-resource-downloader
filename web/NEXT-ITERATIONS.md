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

## Export formats, starting with Confluence HTML

**Goal.** Publish a tenant's documentation into systems the operator's organisation already reads, without
turning the browser into a writer. Several formats are wanted; Confluence HTML is the first, and the seam
matters more than the format.

### The Confluence contract

From [Atlassian's HTML import FAQ](https://support.atlassian.com/confluence-cloud/docs/faq-import-data-from-html-to-confluence/),
which the exporter has to satisfy exactly:

- Upload is **one `.zip` containing one folder**. The **folder name becomes the space name**; each `.html`
  file inside becomes a page, and **the file name becomes the page title**.
- Media for a page must sit in a folder **with the same name as that page**. Unsupported media is ignored.
- Import requires create-space permission, and the admin **attachment size limit must exceed the zip**.
- **Preserved**: headings, paragraphs, bold, italic, hyperlinks, images, tables, ordered and bulleted lists,
  quotes, dividers, inline code, superscript, centre alignment, emoji.
- **Not preserved**: `<title>`, `<figure>`, `<nav>`, `<iframe>`, `<button>`, audio. **Code blocks and
  equations arrive as plain text**; embedded video and page links degrade to plain hyperlinks; coloured or
  background-styled text is flattened.

Two consequences to internalise before writing any code:

- **There is no page hierarchy.** A space is a flat set of pages. Our documents are a tree
  (`docs/<type>/<name>.md`), and the sidebar cannot come along — `<nav>` is unsupported anyway.
- **Import creates a space; it does not update one.** Re-importing is not a sync — it yields a second space
  or a conflict. This is a **one-way publish**, and the README must say so. Real synchronisation means the
  Confluence REST API and storage format, which is a different and much larger feature; parked below.

### What the corpus actually contains

Measured over the two reference exports (411 served documents, 265 + 150 files under `docs/`, 3.9 MB and
2.5 MB of Markdown):

| Feature | Count | Confluence outcome |
|---|---|---|
| `<details>` / `<summary>` blocks | **7,208**, up to 317 in one document, nested up to 5 deep | **Not on the supported list** — must be transformed |
| Inline `<code>` | 14,345 | Supported |
| Table rows | 6,048 | Supported |
| Fenced code blocks | 182 (124 unlabelled, 32 `powershell`, 8 `bash`, 7 `zsh`) | **Plain text** — all formatting lost |
| Images | 2 | Supported, needs the per-page media folder |
| Literal angle brackets in prose (`<key>`, `<endpoint>`, `<APIType>`, `<string>`, `<name>`) | 44 | Unknown tags — **must be escaped** |

The last row is a latent bug the export surfaces: macOS plist payloads are quoted in prose with bare angle
brackets, and `html: true` already turns them into bogus elements in the browser. Harmless there, unknown
on import. The exporter must serialise through an **allowlist that escapes anything not in it**.

### The `<details>` question — the one open decision

`<details>` is the dominant element in these documents and the architecture notes call it *the*
documentation: 7,208 blocks, up to 317 in a single policy document, nested up to five deep. It is not on
Confluence's supported list, and the FAQ does not say what happens to it either — passing it through is a
gamble on undefined behaviour. Every candidate below therefore **transforms** it into supported markup.

**Decision deferred on purpose.** Build the transform as a swappable strategy, ship a default, and choose
properly once a real import has been eyeballed. See *Trying them* at the end of this section.

#### What the transform receives

A real block, from `gbl_cp_prd_d_win_app_m365_apps_baseline.md`:

```html
<details><summary>Sub-options (1)</summary>
  <details><summary><code>…pol_secguide_block_flash</code> = <code>…block embedded flash activation only</code></summary>

  Child of the SecGuide 'Block Flash activation in Office documents' policy: selects the specific
  blocking level applied…

  Configured value. Recommended value: Block Flash content in Office documents… Reference: https://…
  </details>
</details>
```

Three properties every strategy has to cope with, and which the options below are judged on:

- **The summary is usually `<code>path</code> = <code>value</code>`**, not a free-form label — it is already
  a key/value pair, which is what makes the table option viable at all.
- **Some summaries are group labels** (`Sub-options (1)`) with no value and no body of their own.
- **Bodies are multi-paragraph prose**, sometimes with links and their own nested blocks.

#### Option A — bold paragraph + blockquote

The summary becomes a bold paragraph, the body a blockquote; nesting becomes nested blockquotes.

```html
<p><strong><code>…block_flash</code> = <code>…block embedded flash activation only</code></strong></p>
<blockquote>
  <p>Child of the SecGuide 'Block Flash activation…' policy…</p>
</blockquote>
```

- **Keeps** nesting depth (visibly, through indentation), the key/value summary verbatim, all inline
  formatting, and every element is explicitly on Confluence's supported list.
- **Loses** the collapse affordance. A 317-setting document becomes one very long page with no way to skim
  it — the single biggest objection to this option.
- **Effort**: lowest. A mechanical token/DOM rewrite with no interpretation of the summary's contents.
- **Risk**: low. Blockquotes nested five deep may indent far enough to look broken; cap the visual depth.
- **Best when** fidelity and a cheap first version matter more than navigability.

#### Option B — headings by depth

The summary becomes `h3`/`h4`/`h5`/`h6` according to nesting level, the body follows as ordinary content.

- **Keeps** nesting as real document structure, and this is the **only option that makes each setting
  individually addressable** — Confluence gives every heading an anchor, so a colleague can link to one
  setting, and Confluence's page outline and search index both pick them up.
- **Loses** readability of the outline at scale: the 317-block document produces a 317-entry table of
  contents, and depth 5 exceeds `h6` once the document's own `h2` sections are accounted for (settings start
  at `h3`, so only three levels of nesting fit).
- **Effort**: low, plus a rule for what to do past `h6` — clamp, or fall back to Option A below the cap.
- **Risk**: medium. Long `path = value` summaries make very long headings; Confluence may truncate them in
  the outline.
- **Best when** the export is meant to be linked into and searched, rather than read top to bottom.

#### Option C — one table per settings section

Each settings section collapses into a single table: one row per setting, `Setting | Value | Notes`, with
nested settings as a list inside the row's cell.

| Setting | Value | Notes |
|---|---|---|
| `…block_flash` | `…block embedded flash activation only` | Child of the SecGuide 'Block Flash…' policy… |

- **Keeps** the most information per screen by a wide margin, and matches how these documents are actually
  used (scanning for a setting). Tables are well supported by the importer, and it reuses the `path = value`
  structure that is already in the summary.
- **Loses** nesting — five levels have to flatten into a cell, and group-label blocks (`Sub-options (1)`)
  have no natural home. Multi-paragraph bodies with links inside a table cell read poorly.
- **Effort**: highest. It is the only option that has to **parse** the summary into key and value, and
  therefore the only one that can get the split wrong (values legitimately contain ` = `).
- **Risk**: highest, and the failure is silent — a mis-split summary produces a plausible-looking wrong row.
  Needs the strictest tests of the four.
- **Best when** the audience is auditors comparing settings, not readers.

#### Option D — pass through unchanged, and measure

Emit the `<details>` markup as-is and import it, purely to find out what Confluence does with it. Not a
shipping candidate — the FAQ's silence means the answer could be "renders collapsed", "renders expanded",
"drops the summary" or "drops the block" — but it is the cheapest way to turn an assumption into a fact, and
if the importer happens to keep it, that outcome beats all three transforms.

- **Effort**: near zero (it is the absence of a transform).
- **Do this first**, in the same spike as the others, and record the answer in this file.

#### Comparison

| | A: bold + quote | B: headings | C: table | D: passthrough |
|---|---|---|---|---|
| Nesting preserved | yes (visual) | yes (structural) | no | unknown |
| Per-setting anchor | no | **yes** | no | unknown |
| Skimmable at 317 blocks | no | partly | **yes** | unknown |
| Page outline pollution | none | **severe** | none | unknown |
| Interprets the summary | no | no | **yes** | no |
| Implementation effort | low | low | high | none |
| Risk of silent wrongness | low | low | **high** | n/a |

#### Trying them

Make the strategy the one thing that varies:

```ts
// src/docs/export/details-strategy.ts — pure, Nest-free, one function per option
export type DetailsStrategy = 'blockquote' | 'headings' | 'table' | 'passthrough';
```

- Select it with a **query parameter on the export route** (`?details=headings`), defaulting to Option A.
  A route parameter, not a new configuration mechanism — the environment-variables-only rule stands, and
  nothing about the browser's own rendering changes.
- Unit-test each strategy against the same fixture: the nested example above, a group-label block with no
  value, a body containing a link, and a summary whose value itself contains ` = ` (the case Option C can
  get wrong).
- **Decide from a real import, not from the FAQ.** Import one zip per strategy into a scratch space and
  compare: does the block survive; is the 317-setting document usable; do per-setting anchors exist; is
  search finding setting names. Write the answer back into this section and drop the losing strategies —
  carrying four transforms indefinitely is its own cost.

### Shape of the work

```
GET /:tenant/_export/confluence                  →  200 application/zip
GET /:tenant/_export/confluence?details=headings     Content-Disposition: attachment; filename="<tenant>.zip"
```

- `_export` is a **representation prefix** like `_resource`: it must be declared **before** the
  `:tenant/*path` catch-all, and it cannot collide with a resource type because no Azure/Graph type segment
  starts with `_`.
- Suggested layout under `src/docs/export/`: `confluence.ts` (the format), `html-allowlist.ts` (the
  serialiser), `details-strategy.ts` (the four transforms above), each pure and Nest-free; one thin
  `ExportService` doing the zip and the streaming. A second format then drops in beside `confluence.ts`
  without touching the controller.
- Zip layout:
  ```
  <tenant>.zip
  └── <tenant>/                    ← becomes the space name
      ├── <Tenant> overview.html   ← from summary.md + a grouped link list (replaces the sidebar)
      ├── <page>.html              ← one per document, flat
      └── <page>/                  ← that page's media, only if it has any
  ```
- **Page titles.** Basenames are unique within each reference tenant today (0 collisions), because the CLI
  dedupes slugs with a hash suffix. Use the basename, but **hard-fail the export on a collision** rather
  than silently overwriting a page; the documented fallback is prefixing the resource type. Longest basename
  today is 87 characters — check it against Confluence's title limit before relying on it.
- **Links.** `rewriteHref` currently turns relative `.md` links into app routes. The export needs a second
  strategy producing `<a href="<Page Name>.html">` so the importer resolves them; this is an added mode, not
  a change to the browser's behaviour. In-document anchors (`#security`) will not survive — accepted loss.
- **Provenance.** Frontmatter is stripped before rendering and would otherwise vanish. Emit `source`,
  `generatedAt` and the shas as a small table at the top of each page, with a line saying the page is
  generated and local edits are lost on the next import.
- **The YAML view has no Confluence equivalent.** Omit those links in v1; attaching the source YAML as page
  media is a follow-up.

### Non-negotiables, and how each survives

- **Read-only.** The zip is built in memory and streamed to the response. Nothing is written under
  `DOCS_ROOT` — ever — and no temp file is created there; if one is unavoidable it belongs in
  `os.tmpdir()`. The route is a `GET` and mutates no state.
- **Path safety.** Every document read still goes through `resolveWithinTenant()`. The export enumerates
  from `docs/index.yaml`, not by walking the tree, so it cannot reach outside it.
- **One `markdown-it` instance.** The export needs different link rewriting and a different serialisation,
  which is tempting to solve with a second instance. Don't: `rewriteHref` already takes an env, so make the
  mode part of that env and keep the single instance.
- **Watch the render cache.** It is keyed by file path only. Rendering the same document in export mode
  would otherwise return the browser's HTML (or poison the browser's entry with the export's), and with a
  swappable `<details>` strategy the second strategy tried would silently serve the first one's output.
  **Both the mode and the strategy must become part of the cache key** — the easiest bug to introduce here.
  Simplest safe answer while the strategies are being compared: do not cache export renders at all.
- **No client-side JavaScript.** A plain download link in the tenant page's top bar.
- A new dependency is needed for zipping (`archiver` or `yazl`); prefer streaming over buffering even though
  the corpus is small.

### Testing

- e2e: the route returns `application/zip` with the expected `Content-Disposition`, and the archive contains
  `<tenant>/<page>.html` for a fixture document.
- Unit: **each `<details>` strategy against one shared fixture** (see *Trying them*); the allowlist
  serialiser, **including that `<key>` in prose is escaped**; the export link strategy; and the
  title-collision failure.
- A case proving two strategies of the same document return different bodies, which is what catches the
  cache-key trap above.
- An assertion that the export never writes inside the docs root, keeping the read-only invariant honest.
- No network, temp-dir fixtures, as everywhere else.

### Deliberately out of scope for the first export

- **Confluence REST API sync** (create/update pages in place, labels, page tree, attachments). The proper
  answer to "keep Confluence up to date", and a much larger feature: authentication, a state mapping from
  resource to page id, and conflict handling. HTML import is the cheap one-way version.
- **Other formats** — single-file HTML, DOCX, PDF via print stylesheet, a Markdown bundle. The `export/`
  seam above exists so these do not each rewrite the controller.
- **Partial exports** (one type, one document, only the summary). Whole-tenant first.
- **Preserving the tree.** Not expressible through HTML import; re-parenting in Confluence afterwards, or
  the REST API, are the only routes.

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

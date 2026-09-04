# Changelog

All notable changes to this project (the documentation browser in `web/`) are documented in this file.
Changes to the Go CLI live in [`../go/CHANGELOG.md`](../go/CHANGELOG.md).

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **The sidebar can be filtered along every taxonomy axis the CLI resolves, combining them, with counts that
  react to the selection.** `docs generate-index` with a `taxonomy:` section classifies each resource on one
  or more **axes** — *Programme* (CIS hardening, Defender, VPN…), *Platform*, *Assignment scope*, whatever the
  operator declares — and writes a header `facets` registry (axes and values in display order, with per-tenant
  counts) plus a per-resource `facets` map of value ids into `docs/index.yaml` (schema version 3). Every page
  that renders the sidebar now offers one chip group per axis, headed by **the axis label from the index**, and
  accepts **one repeatable query parameter per axis id** — `?programme=defender&programme=vpn&platform=macos` —
  narrowing the tree server-side with **OR within an axis and AND across axes**. The whole selection rides
  along in every document link and in every chip link, so a click toggles exactly one value and never drops
  another axis, and a **Clear filters** reset plus a *showing N of M* line state what is currently applied.
  **No client-side JavaScript**: the filter is a set of query parameters and links, not a widget. The
  implementation is **axis-agnostic** — no axis is named anywhere in the code or the templates — so an axis
  added to the CLI's config appears here with no change, and membership is **read, never derived**, because
  deriving it here would let the browser and the Confluence export (a second consumer of the same index)
  disagree.

  What the design protects: each axis offers **`_uncategorised`**, listing what that axis matched to nothing,
  so a taxonomy that stops matching appears as a full bucket instead of a quietly thinning tree; **counts are
  computed against the current selection with the axis's own choice removed**, so picking one value does not
  drive its siblings to zero; a value that **another** filter has emptied is **no longer offered** (it is a
  dead end), while a **selected** value stays visible even at zero so the choice that emptied the tree can be
  undone, and an **unfiltered zero-count value stays listed** because "empty in this tenant" is information the
  registry carries on purpose; totals **count distinct resources** and never sum the value counts, which are
  non-additive since a resource can hold several values on one axis; the **document you are viewing is always
  kept in its own sidebar** even when the filter excludes it, is deliberately **not counted** into the
  selection, and is **marked as being outside the filter** with the count line saying so, so the tree being one
  row longer than the count is explained rather than left to be reconciled; **unknown values and unknown axes
  are ignored** instead of rendering what would look like an empty tenant; an axis whose id would collide with
  a route's own parameter (`?raw`) is not offered; and an
  index with **no taxonomy is unfilterable**, rendering exactly the per-type tree it did before. A **version-2
  index** (`programmes` + per-resource `groups`, no `facets`) keeps working: its single programme axis is
  synthesised from those fields, so old exports on disk need no regeneration. Resources carry their membership
  labels as badges in the index listing, resolved from the header, so membership is visible from a resource and
  not only from the chooser. Grouping *by* an axis — replacing the per-type tree — is deliberately not part of
  this, see [`NEXT-ITERATIONS.md`](NEXT-ITERATIONS.md).

- **A tenant's documentation can be exported as Confluence HTML.**
  `GET /:tenant/_export/confluence` streams one zip containing one folder, which is what Confluence's
  HTML import expects: the folder name (`<tenant domain> documentation`) becomes the space name, and
  each file name becomes a page title. Pages are titled `<type leaf> — <display name>` from
  `docs/index.yaml` (falling back to the document's H1, then its base name), which is what keeps a
  flat, hierarchy-less space readable and makes cross-type name clashes impossible; characters that
  are illegal in a file name or a Confluence title are replaced, and a residual collision gets a
  numbered suffix and a line on the overview page rather than overwriting a page. An `Overview.html`
  built from `docs/summary.md` plus a grouped link list stands in for the sidebar, since an imported
  space has no page tree, and each page opens with the provenance (source, export timestamp,
  generation hashes) that stripping the frontmatter would otherwise lose. The export is offered as a
  plain download link per tenant on the picker, keeping the tenant landing page documentation-only —
  still no client-side JavaScript.

  The non-negotiables are preserved as follows. **Read-only:** documents are enumerated from
  `docs/index.yaml` and read through `resolveWithinTenant()`, the archive is assembled in memory and
  streamed, and no temporary file is created — an e2e case asserts that an export changes not one byte
  under the docs root. **One `markdown-it` instance, and the cache:** the exporter does not render, it
  re-serialises the HTML the browser already produced with the same render env, so the mtime-keyed
  render cache needs no extra key and cannot be poisoned or bypassed by an export (also asserted).
  **Path safety:** untouched, and it is the reason images cannot travel — a served root hands out
  exactly one extension, so an image travels as its `alt` text instead of the extension policy being
  widened. `_export` is a representation prefix like `_resource`, declared before the document
  catch-all, and an unknown format is a 404.

  Serialisation is an allowlist, not a blocklist: HTML the importer does not preserve is unwrapped,
  scripts and embeds are dropped, and an element name that is not HTML at all is escaped. That last
  rule fixes a latent bug the export surfaced — macOS plist payloads are quoted in the documents with
  bare angle brackets, which `html: true` already turns into phantom `<key>`/`<string>` elements in
  the browser; the export now escapes them instead of shipping them. Heading permalinks and
  in-document anchors are unwrapped (a flat space has nowhere to point them), and a link whose target
  is not a page in the export degrades to its own text. A document the index lists but that cannot be
  read is reported under *Not exported* on the overview page instead of failing the export, and zip
  entries carry the export's own `generatedAt` rather than the wall clock, so re-exporting an
  unchanged tenant produces the same bytes.

  **One-way**, as the README says: importing creates a space rather than updating one, so re-importing
  yields a second space and edits made in Confluence are lost. The `<details>` settings blocks that are
  the bulk of the documentation are passed through untouched, and a real Confluence Cloud import
  confirms that this is the right representation: the importer turns each block into a **native
  collapsible expand**, keeping its `path: value` summary, its nesting and its inline formatting — so
  the block is now on the serialiser's allowlist as a settled decision rather than a probe, no summary
  is ever parsed into key/value (a value containing ` = ` cannot be mangled), and the `?details=`
  strategy switch that existed only to compare alternatives is gone with it. The media and per-format
  follow-ups remain in `NEXT-ITERATIONS.md`. New dependencies: `htmlparser2` for the serialisation pass
  and `yazl` for the streamed zip.

- **The tenant summary's Findings table is styled as a findings list, with the severity as an icon.**
  A markdown-it core rule tags any table whose first header cell is `Severity` with `class="findings"`
  and puts `data-severity` on each body row and on its severity cell, so `src/styles.css` can reach
  them. The severity word is replaced visually by a masked SVG icon — an octagon for `critical`, a
  triangle for `high`, a circle for `medium`, each with its own colour and a dark-mode variant — and a
  coloured stripe down the row's leading edge. The table is keyed off its columns rather than off the
  `### Findings` heading above it, so it keeps its treatment wherever the generator moves it, and a
  severity outside the closed `critical`/`high`/`medium` set is left as plain text rather than being
  given a misleading icon.

  This is a rendering change only: the token rule mutates attributes and adds no markup, the single
  `markdown-it` instance is unchanged, and no client-side JavaScript is involved — the icons are CSS
  masks and the word stays in the DOM (pulled out of view, image-replacement style) so assistive
  technology still reads it, with a `title` giving it back on hover. The severity vocabulary and the
  column order are a contract from the CLI's generation template, not a guess by the browser.

- **The exported source YAML is now browsable next to the documentation.**
  `GET /:tenant/_resource/<type>/<name>` renders the resource a document was written from, syntax
  highlighted, with every line addressable as `#L42`; `?raw` serves the same file as
  `text/plain; charset=utf-8` with `X-Content-Type-Options: nosniff` for copy-paste. A
  **Documentation | YAML** switcher in the top bar flips between the two representations, and the
  document's `source` frontmatter became a link to its YAML. `_resource` is a *representation* prefix,
  not a path segment: it never appears in the breadcrumb and cannot collide with a resource type,
  because no Azure/Graph type segment starts with `_`. Consequently `<export>/resources` is now a
  **second served root** — the one architectural invariant this change replaces, and it is replaced by
  an equally tight one rather than loosened: `resolveWithinRoot()` in `src/docs/path-safety.ts` is the
  single guard for both roots and allows exactly **one** extension per root (`.md` for `docs/`,
  `.yaml` for `resources/`, `.yml` deliberately not served), so a document can never be served from
  the resources root, nor a resource from `docs/`, and `..` cannot cross between them; null bytes,
  absolute paths and `..` are still rejected up front and containment is still re-verified *after*
  `realpath()`. The app remains **read-only**: both routes only read, and nothing under the export is
  written, moved or deleted. Freshness is unchanged in kind — highlighted output is cached by
  `mtimeMs` + `size` in a bounded cache, so a re-downloaded resource appears without a restart.
  Discovery is unchanged: `resources/` is *not* a marker, so an export whose `docs/` were copied
  without it stays a valid tenant whose YAML views simply 404. Navigation stays purely index-derived —
  a document's source is located by inverting the CLI's own `docs/<type>/<name>.md` ↔
  `resources/<type>/<name>.yaml` mapping, so no directory is walked and the "counts and listings come
  from the index" non-negotiable is untouched; resources the index only counts (excluded bulk types,
  unreferenced groups) therefore have no YAML view yet (see `NEXT-ITERATIONS.md`). Highlighting uses
  one `shiki` instance built in `onModuleInit` (mirroring the single `markdown-it` instance) and loaded
  through `dynamic-import.ts` because `shiki` is ESM-only; it emits dual-theme CSS variables, so dark
  mode and the `:target` line highlight stay `prefers-color-scheme`/CSS — **still no client-side
  JavaScript**. Files above 512 KB, or a highlighter that failed to load, degrade to an escaped
  `<pre>` instead of stalling or crashing a request.

- **The tenant landing page is now the generation agent's summary (`docs/summary.md`).** The prompt's
  section 7 writes a tenant-wide management summary at the docs root — findings, recommendations,
  assignment posture and coverage caveats — and that is what `GET /:tenant` renders. It is deliberately
  optional and is *not* a discovery marker: `docs/index.yaml` still is, and a tenant without a summary
  falls back to the previous index listing rather than 404ing. Its existence is checked at render time,
  not at discovery time, so a summary written after the 30 s discovery cache was filled still appears on
  the next request. Freshness is unchanged in kind: the summary goes through the one
  `MarkdownRendererService`, so it is cached by `mtimeMs` + `size` and a regenerated summary shows up
  without a restart. Its links are relative to `docs/` and resolve through the existing rewriting with
  `docDir: ''` — no change to `link-rewrite.ts`. The app remains read-only: nothing under the docs root
  is written, moved or deleted.

- **Sidebar navigation on every page (`views/partials/sidebar.hbs`).** The per-document list moved out of
  the landing-page body into a persistent sidebar shared by the landing page and every document page, so
  the tenant metadata (counts, export timestamp, incomplete-export banner, excluded bulk types) survives
  everywhere instead of only on the landing page. It groups by resource type in one collapsible
  `<details>` per type; `buildNavigation()` takes the current document and marks it (`aria-current`) and
  its section (rendered `open`), which is what makes the tree usable **without any client-side
  JavaScript** — the one non-negotiable a sidebar could easily have broken. On narrow viewports the
  column stacks below the content (`flex-col-reverse`) rather than pushing the document down the page.
  Grouping stays by type because `platformGroup`/`functionGroup` are still absent from the index
  (see `NEXT-ITERATIONS.md`).

- **Index-driven tenant landing page (`views/tenant.hbs`).** `GET /:tenant` no longer renders an
  `index.md`; it renders the navigation built from `docs/index.yaml`: resources grouped by type
  (sorted, with their document routes), the LLM-authored `summary` when present, a *pending* marker
  for in-scope resources that have no document yet, count-only assignment badges (`all users`,
  `all devices`, `N groups`, `targeted by N`, `unassigned`) so resolved names stay in the documents,
  the excluded bulk types as counts, and a banner when the export reports itself incomplete. Still no
  client-side JavaScript. Grouping is by resource type because the documents do not yet carry the
  `platformGroup`/`functionGroup` frontmatter the index can enrich them with; when present they are
  shown as badges (see `NEXT-ITERATIONS.md`). This listing is now the *fallback* body, used when the
  export carries no `docs/summary.md`; the tree itself moved into the sidebar (above).

- **`docs/index.yaml` parsing and navigation building (`src/docs/tenant-index.ts`).** A pure,
  Nest-free module — parsing and grouping stay unit-testable without a module — with `js-yaml` added
  as an explicit dependency. It validates defensively (`version: 1` plus a tenant name, unexpected
  field types coerced, unknown fields ignored) so operator-supplied content cannot take the process
  down.

- **The parsed index is cached by `mtimeMs` + `size`**, like rendered documents, so a regenerated
  index is reflected on the next request without a restart.

- **Tests**: `test/tenant-index.spec.ts` for parsing (including rejection of malformed and
  non-`version: 1` files) and navigation building; the e2e fixture was rebuilt on the new layout and
  now also covers the landing page, a pending resource, the excluded-type counts, the malformed-index
  folder being skipped, `generate.md` not being served, and no-restart refresh of the index itself.

- **Each declared document section now has a visual identity.** The CLI declares every document
  type's `##` headings as a closed, verbatim set (the `<!-- doc-headings: … -->` contract in its
  `doc-prompt.md`), so the heading text is a machine contract rather than prose. `section-hooks.ts`
  puts `data-section="<slug>"` on every `h2`/`h3` and `class="doc-section-heading"` on the ones in
  that vocabulary, and the stylesheet gives each an icon and one of four role colours — risk
  (*Security*, *Findings*, *Expiry and renewal*, *Coverage caveats*), substance (*Settings*,
  *Properties*, *Definition*, *Membership*), relations (*Usage as assignment target*, *Usage and
  references*, *Assignment posture*, *Targeted by*) and meta (*References*, *Lifecycle and
  operations*, the summary's own four sections). Four roles rather than fourteen colours, reusing the
  findings-table hues so a Security section and a critical finding read as one language. A heading
  *outside* the vocabulary keeps the attribute and gets no treatment at all, which is what makes this
  safe on the documents currently on disk: they predate the contract, and an unrecognised heading such
  as `## Metadata` renders exactly as it did before.

  Two more hooks come with it. The tool-maintained marker pairs become selectable elements — a matched
  `<!-- assignments:start -->` / `<!-- assignments:end -->` pair (likewise `targeted-by` and
  `notifications`) renders as `<div class="doc-assignments">`, because an HTML comment survives into
  the DOM under `html: true` but cannot be styled, and an unmatched marker is deliberately left as a
  comment rather than emitting an unbalanced element. And the metadata table each document opens with
  is classed `.doc-metadata`, located as the first table after the H1 rather than by
  `table:first-of-type`, which breaks on the 99 reference documents that open with an extra heading;
  it opts out of the shared wide-table `nowrap` the same way `.findings` does. The templates gained
  the matching chrome hooks (`#doc-page`, `.doc-main`, `.doc-body`, `.doc-source`, `.site-header`,
  `.view-switcher`).

  The non-negotiables hold. **No client-side JavaScript:** icons are masked SVGs driven by one custom
  property per section, so a single value covers light and dark, and the `:target` landing highlight is
  plain CSS. **One `markdown-it` instance:** all of it is one linear core rule over the token stream,
  pushed after the findings rule, inside the existing render and its mtime-keyed cache — no extra pass
  and no per-request instance. **Confluence export unaffected:** `<div>` and `<section>` unwrap and
  neither `class` nor `data-*` is on an element allowlist, asserted in `test/export.spec.ts`.

- **Each section is now an element, so it can carry a panel and its own density.** Everything between two
  H2s is wrapped in `<section class="doc-section" data-section="…">` with the same slug as its heading.
  This is what the heading hooks could not give: `h2#settings ~ *` is the only alternative and it bleeds
  into every section that follows. With it, *Settings*, *Properties*, *Definition* and *Membership* drop
  into a denser mode — smaller type, tighter `<details>` margins, a shaded rail at depth — which is the
  point, because one document in the reference export carries 317 settings nested five levels deep;
  *Security* and *Expiry and renewal* get a rail and deliberately **no** tint, so they stay a signal
  rather than decoration; *References* becomes a tighter, breakable link list; and the summary's
  *At a glance* table takes the same wide-table `nowrap` opt-out `.findings` has.

  An H2 *inside* a matched marker pair never opens a section. That one rule is what guarantees
  well-formed output — a section can only close at an H2 outside every spliced block, so a block is
  always wholly inside one section and can never be straddled — and it is also the right reading: the
  spliced `## Used by` and `## Targeted by` blocks belong to the section they were spliced into. Verified
  against both reference tenants: balanced wrappers on every document family, and no marker block cut in
  half.

  **The Confluence export is byte-identical to before this change** — checked by exporting both reference
  tenants from this build and from the previous revision and diffing the unpacked zips. `<section>` is on
  the importer's unwrap list, and the wrapper is emitted as a non-block token on purpose: a block token
  carries a trailing newline, which the unwrapping serialiser keeps, and that would have added a blank
  line to all 264 pages for a wrapper that exists only for the stylesheet. Against Atlassian's
  [HTML import FAQ](https://support.atlassian.com/confluence-cloud/docs/faq-import-data-from-html-to-confluence/),
  the exported pages still contain only supported constructs — headings, paragraphs, lists, tables, links,
  inline code, quotes, `strong`/`em` — with `lang`, `charset` and `href` as the only attributes anywhere;
  no `nav`, `figure`, `iframe`, `button`, no styled text, and none of the new `class`/`data-*` hooks.

- **Setting blocks are styled from the attributes the generator now writes.** Every block is emitted as
  `<details data-setting="<exact YAML path>">`, with `data-note="security"` for a setting called out in
  the Security section and `data-note="inert"` for one that is present but has no effect (7324 and 1967
  occurrences respectively across the reference exports). A `security` block gets the risk rail and a
  `security` chip on its own `<summary>`; an `inert` block is de-emphasised behind a dashed rail with a
  `no effect` chip. The chips are `::after` content driven by one custom property, so they cost no markup
  and no JavaScript, and they are decorative by construction — the same fact is stated in the block's
  prose, so nothing is lost where CSS is not. `>` throughout, so a nested block never inherits its
  parent's chip.

- **`used-by` joins the tool-maintained marker blocks.** The regenerated exports splice a reverse-reference
  block into notification-template documents (11 pairs), which the browser had no element for. It now
  renders as `<div class="doc-used-by">` and `## Used by` is part of the declared section vocabulary,
  sharing the relations role and icon with `## Targeted by`.

  Still outstanding: `data-family` / `data-type` on `<article>`, so a group and a credential document can
  differ — see [`NEXT-ITERATIONS.md`](NEXT-ITERATIONS.md).

### Changed

- **BREAKING: A tenant is now a directory containing `docs/index.yaml`; `index.md` and `.doc-manifest.json` are
  gone.** The CLI stopped generating both: `azure-rd docs generate-index` writes a machine-readable
  navigation index instead, and the manifest was retired because `resources/metadata.yaml` plus the
  documents' frontmatter already carry the generation state it used to hold. Discovery is keyed off
  that file, and a tenant's document root is now `<export>/docs` rather than the export folder — which
  is what the relative `../<type>/<name>.md` links inside the documents were always relative to, so
  cross-document links resolve without a rewrite change. An export that has not had
  `docs generate-index` run for it is not discovered. The tenant-discovery invariants are preserved:
  a matched tenant still owns its whole subtree, `_`/`.`-prefixed directories are still skipped, depth
  is still bounded, and counts are still derived from the index rather than by walking the tree. A
  malformed or unreadable index makes the folder *not a tenant* instead of crashing discovery.

- **Heading anchors are no longer percent-encoded.** `markdown-it-anchor`'s default slug encodes
  anything outside its allowed set, which produced ids only selectable as
  `[id="lifecycle-%26-operations"]` and a `%E2%80%94` in every `summary.md` H1 anchor. A local
  `slugify` replaces it, so those become `#lifecycle-and-operations` and a clean title slug.
  **Existing links to a heading anchor that contained an encoded character will not resolve any more**
  — in-document anchors are generated, never hand-written, so this affects bookmarks and external
  links only. `&` slugs to `and` on purpose: a heading spelled `Lifecycle & operations` (as the
  documents on disk still spell it) and `Lifecycle and operations` (as the current contract requires)
  get the same anchor and the same section identity, so neither the URLs nor the styling move again at
  the regeneration.

- **A document's own source-file echo is no longer rendered.** The generated documents repeat their
  source file as a code-only paragraph directly under the H1 (`` `resources/.../foo.yaml` `` or just
  `` `foo.yaml` ``), which is the same value the frontmatter already carries and the page now renders as
  a link to the YAML view — so it was shown twice. It is dropped at render time, before `markdown-it`
  sees the body, and only when it matches *that document's* `source` (full path or basename) **and**
  sits alone on its line; a sentence that mentions another resource's `.yaml` is untouched. Documents
  without a `source` frontmatter are left exactly as they are. This is a rendering-only change: nothing
  is written to the export, and the removal will become a no-op once the generation template stops
  emitting the line.

- **`GET /:tenant/summary` redirects (302) to `/:tenant`.** The summary *is* the landing page body, so
  its own document route would serve the same content twice under two URLs. It is a redirect rather than
  a 404 because, unlike `generate.md`, the file is documentation and links to it should land somewhere
  sensible. `path-safety.ts` is untouched: this is a serving decision taken before any path resolution.

- **`GET /healthz` reports `{ status, tenants, documents, pending }`.** `documents` is now the index's
  `counts.documented` (previously the manifest's resource count) and `pending` is new, so the health
  endpoint distinguishes a fully documented export from a partly generated one. The picker shows the
  same split.

- **`docs/generate.md` is never served.** The agent prompt sits at the docs root and would otherwise
  be routable at `/:tenant/generate`; it is tool input, not documentation. Blocked in the controller
  by its extensionless root-relative path, so a real document named `generate.md` deeper in the tree
  is unaffected. This is a serving decision, not a path-safety change: `resolveWithinTenant()` remains
  the only way a request-derived path becomes a filesystem path, and its guarantees are untouched.

- **The project moved to `web/` and became self-contained.** The Go CLI now lives in the sibling `go/`
  folder and the shared export tree stays at the repository root, so the `DOCS_ROOT` default
  (`../output`, resolved against `process.cwd()`) still points at it. This app's architecture and
  style rules live in `web/.windsurf/rules/`; the Go rules in the parent repository do not apply
  here. `DOCS_ROOT` remains the **only** coupling to the downloader — nothing in `web/` imports from,
  shells out to, or otherwise depends on the Go project, so it can be split into its own repository
  without changes.

- **`NEXT-ITERATIONS.md` holds only outstanding work and parked ideas.** It had accumulated
  descriptions of things that shipped — the source-YAML view, the Findings table, the Go-side heading
  contract — which made it a second, drifting account of what the browser does. Those are removed:
  this changelog is the record of what shipped and `README.md` of what exists today. What remains is
  restructured into numbered `Goal`/`Notes`/`Plan` entries for work that is actually outstanding
  (document styling hooks, the findings block, a summary table of contents, sidebar refinements,
  Confluence HTML export) and a `Parked ideas` area where each idea states why it is parked and what
  would make it worth doing. Entries are self-contained and get deleted once delivered, so numbering
  shifts — nothing outside the file references a section number. No behaviour, route or dependency
  changed.

### Fixed

- **Tenants written by a current CLI are discovered again: the index schema gate accepts `version >= 1`.**
  `docs generate-index` bumped `docs/index.yaml` to `version: 2` when it added per-resource `groups` and the
  header `programmes`/`vocabularies`. `parseTenantIndex()` required exactly `1`, and the index is *also* the
  tenant marker — so every real export vanished from the picker and every document route 404ed, an outright
  disappearance rather than a degradation (`DOCS_ROOT=../output` reported `{"tenants":0}`). Any integer
  version `>= 1` now parses, the value is carried on `TenantIndex` instead of being hard-coded, and unknown
  fields are ignored as they always were — which is what makes the CLI's additive bump additive in practice.
  A non-integer version, a version below 1 and a malformed file are still rejected, so such a folder is still
  *not a tenant* and discovery still cannot crash on one.

## [RC1]

First iteration: discover exported tenants on disk and render their generated Markdown as HTML.

### Added

- **Read-only docs browser (NestJS 11 + Express, Handlebars, Tailwind CSS v4).** Server-rendered
  throughout, with **no client-side JavaScript**. The app never writes, moves or deletes anything
  under the docs root and never calls Azure; no route mutates state.

- **Tenant discovery (`TenantDiscoveryService`).** Walks `DOCS_ROOT` to a bounded depth and treats a
  directory as a tenant only when it contains **both** `index.md` **and** `.doc-manifest.json` — one
  marker is not enough. A matched tenant owns its whole subtree (discovery does not descend looking
  for nested tenants), directories starting with `_` or `.` are skipped (housekeeping folders such as
  `_to_delete/`), and the resource count shown in the picker is derived from the manifest (the sum of
  `types[*].resources`) rather than by walking the tree. A malformed or unreadable manifest makes the
  folder *not a tenant* instead of crashing discovery.

- **Markdown rendering (`MarkdownRendererService`).** Exactly one `markdown-it` instance, built in
  `onModuleInit`, with `html: true` because the `<details>`/`<summary>` disclosure blocks *are* the
  generated documentation and must pass through untouched. `typographer` stays off: it mangles quotes
  and dashes in configuration values. Headings get anchors via `markdown-it-anchor`. Frontmatter is
  stripped with `gray-matter` and exposed as page metadata (`source`, `generatedAt`), so it never
  reaches the rendered body.

- **Link rewriting (`link-rewrite.ts`).** Only *relative* `.md` links are rewritten, resolved against
  the current document's directory and prefixed with the tenant segment
  (`../groups/g1.md` → `/<tenant>/Microsoft.Graph/groups/g1`). Anchors, absolute routes, schemes,
  protocol-relative URLs, non-`.md` targets and links escaping the tenant root are left unchanged.

- **Routes.** `GET /` tenant picker, `GET /healthz` (`{ status, tenants, documents }`),
  `GET /:tenant` for the tenant's `index.md`, and `GET /:tenant/*path` for a document inside the
  tenant with an optional `.md` suffix. Anything that does not resolve to a Markdown file inside the
  tenant renders the 404 view.

- **Path safety (`resolveWithinTenant`).** `*path` is attacker-controllable, so a single guard turns a
  request-derived path into a filesystem path: it rejects null bytes, absolute paths and any `..`
  segment before touching the filesystem, serves only `.md` files, and re-verifies containment
  **after** `realpath()` so a symlink cannot escape the tenant folder. Error responses never leak an
  absolute filesystem path, a stack trace or a raw exception message.

- **No-restart freshness.** Regenerated documents appear on the next request: renders are cached by
  `mtimeMs` + `size` from a per-request `stat()`, and the cache is bounded (it evicts at
  `MAX_ENTRIES`). Newly generated tenants appear within the 30 s discovery TTL.

- **Shared app wiring (`configureViews`).** Static assets, views directory, partial registration and
  the view engine are configured in one place used by both `main.ts` and the e2e tests, so tests
  exercise the same wiring as production.

- **ESM escape hatch (`dynamic-import.ts`).** A `new Function('return import(specifier)')` helper,
  needed because TypeScript would down-level `import()` to `require()`, which cannot load the
  ESM-only `markdown-it-anchor` v9.

- **Configuration by environment variables only** — `DOCS_ROOT` (default `../output`, resolved
  against `process.cwd()`) and `PORT` (default `3000`), read at their point of use. No config file.

- **Tests** (Jest + supertest, `*.spec.ts`), which never hit the network and never read the real
  export tree — fixtures are built in a temp dir and cleaned up:
  - `test/path-safety.spec.ts` — traversal, symlink escape, null bytes, absolute paths;
  - `test/docs.e2e.spec.ts` — discovery (ignoring `_`-prefixed and marker-incomplete folders), the
    picker, index render + link rewrite, nested `<details>`, cross-type link resolution, 404s,
    traversal, and an edited file being reflected on the next request;
  - `test/styles-build.spec.ts` — compiles `src/styles.css` with the local Tailwind CLI and asserts
    the custom `<details>`/`<summary>` and dark-mode rules survive.

Deliberate iteration-1 scope cuts are listed in [`NEXT-ITERATIONS.md`](NEXT-ITERATIONS.md): no
search, no syntax highlighting, single-segment tenant routes only, no manifest-driven navigation
tree, no explicit dark-mode toggle, no watch-based cache invalidation.

# azure-rd-docs-web

A **read-only browser** for the documentation that [`azure-resource-downloader`](../README.md) and its
documentation agent produce for an Entra ID / Intune tenant. It discovers tenant exports on disk, renders the
generated Markdown as HTML with the source YAML one click away, lets the reader slice the tenant along the
operator's taxonomy, and packages a tenant for import into Confluence.

It is the last stage of the pipeline described in the [monorepo README](../README.md): `azure-rd download`
exports the tenant, an AI agent writes the documents, `azure-rd docs generate-index` writes `docs/index.yaml`
— and this app reads that tree. It **only reads**: nothing here calls Azure, writes a document or mutates the
export in any way. It is self-contained (its own dependencies, tests, changelog and version line) and shares
nothing with the Go CLI but the export tree on disk; `DOCS_ROOT` is the only coupling.

## Table of contents

- [What it does](#what-it-does)
- [Quick start](#quick-start)
- [Configuration](#configuration)
- [The docs root contract](#the-docs-root-contract)
- [Routes](#routes)
- [Taxonomy filters](#taxonomy-filters)
- [Confluence export](#confluence-export)
- [Rendering](#rendering)
- [Security](#security)
- [Tests](#tests)
- [Project layout](#project-layout)
- [Development conventions](#development-conventions)
- [Known limitations](#known-limitations)

## What it does

- **Tenant discovery** — walks `DOCS_ROOT` (up to 3 levels deep) and treats any directory that contains a
  readable `docs/index.yaml` as a tenant. That file is the navigation index written by
  `azure-rd docs generate-index`; documents are resolved against the tenant's `docs/` folder.
- **Tenant landing page** — `GET /:tenant` renders `docs/summary.md`, the tenant-wide management summary the
  generation agent writes (posture, severity-ranked findings, coverage caveats). It is optional: an export
  with no summary falls back to listing the index — resources grouped by type, with the LLM-authored one-line
  summary, a *pending* marker for resources with no document yet, and count-only assignment badges.
- **Sidebar navigation on every page** — one collapsible `<details>` per resource type, the section of the
  current document opened and the document marked, plus the tenant counts, export timestamp, the
  incomplete-export banner and the excluded bulk types. Collapsing is pure HTML.
- **Taxonomy filters** — when the index carries a taxonomy, the sidebar offers one chip group per axis the
  operator declared (Programme, Platform, Assignment scope, …) and narrows the tree server-side from query
  parameters, several axes at once, with counts that follow the selection. See
  [Taxonomy filters](#taxonomy-filters).
- **Source YAML view** — every document links to the exported resource it was written from
  (`GET /:tenant/_resource/<type>/<name>`), syntax highlighted with `shiki`, one addressable line per `#L42`
  anchor, `?raw` for plain text, and a **Documentation | YAML** switcher in the top bar.
- **Document rendering** — `markdown-it` with raw HTML enabled, so the `<details>`/`<summary>` settings blocks
  that make up the bulk of a document pass through; headings get stable anchors; relative `.md` links become
  app routes; frontmatter is shown as metadata, never rendered into the body; the closed heading contract the
  CLI declares per type drives per-section styling. See [Rendering](#rendering).
- **Confluence HTML export** — `GET /:tenant/_export/confluence` streams the whole tenant as a zip ready for
  Confluence's HTML import, offered as a download link per tenant on the picker. **One-way**; see
  [Confluence export](#confluence-export).
- **No-restart refresh** — regenerated documents, re-downloaded resources and a regenerated `index.yaml`
  appear on the next request (per-request `stat()` against an mtime/size-keyed cache); newly generated
  tenants appear within the 30 s discovery TTL.
- **No client-side JavaScript.** Everything is server-rendered Handlebars + Tailwind; dark mode follows
  `prefers-color-scheme`.

## Quick start

Requirements: Node.js **>= 20** and an export tree produced by `azure-rd` with `azure-rd docs generate-index`
already run for each tenant (the browser needs `docs/index.yaml`; documents and `summary.md` are optional
and show up as they are written).

```bash
cd web
npm install
npm run start:prod        # build CSS + Nest, then serve http://localhost:3000
```

For development, `npm run start:dev` runs the Tailwind watcher and Nest in watch mode together. `npm start`
only runs the already-built `dist/main.js` — run `npm run build` first.

> Views (`views/`) and static assets (`public/`) are resolved from `process.cwd()`, so the server must be
> started from the `web/` directory in both dev and prod. `public/app.css` is a build artifact (gitignored);
> `build`, `start:dev` and `start:prod` all generate it.

## Configuration

Environment variables only; there is no config file and no `dotenv`.

| Variable | Default | Purpose |
| --- | --- | --- |
| `DOCS_ROOT` | `../output` (relative to `process.cwd()`) | Root that is scanned for tenant folders. With the monorepo layout the default already points at the shared export tree at the repo root. |
| `PORT` | `3000` | HTTP listen port. |
| `EXPORT_INDEX` | `type` | Which index the Confluence export writes onto `Overview.html`: `type` (the by-type **Pages** list only), `both` (that list, then one collapsible section per taxonomy axis) or `axis` (the axis sections only). Unset, empty or unrecognised means `type`, so a typo can neither fail an export nor change what it contains. |

```bash
DOCS_ROOT=/path/to/output PORT=4000 npm run start:prod
```

[`.env.example`](.env.example) lists every variable at its default, as a reference to copy and source into
your own shell (the app does not load it):

```bash
cp .env.example .env
set -a; source .env; set +a
npm run start:prod
```

## The docs root contract

```
<DOCS_ROOT>/
└── <tenant>/                      # e.g. contoso.onmicrosoft.com/  (may be nested, up to 3 levels)
    ├── resources/                 # written by `azure-rd download` — served read-only as the YAML view
    │   └── Microsoft.Graph/
    │       └── <endpoint>/
    │           └── <name>.yaml
    └── docs/
        ├── index.yaml             # required marker (azure-rd docs generate-index)
        ├── generate.md            # agent prompt — never served
        ├── summary.md             # optional tenant summary — the landing page body
        ├── report-*.md            # agent run reports — not indexed
        └── Microsoft.Graph/
            └── <endpoint>/
                └── <name>.md      # one document per resource, mirroring resources/
```

Discovery and resolution rules:

- A directory is a tenant when `docs/index.yaml` exists **and parses** as an index object with an integer
  `version` of **1 or later**. A malformed or unreadable index makes the folder *not* a tenant instead of
  crashing discovery. Later schema versions and unknown fields are accepted and ignored — the index is the
  tenant marker, so rejecting a newer schema would hide the export rather than degrade a page. The CLI
  currently writes schema version 4; a version-2 index (`programmes` + per-resource `groups`, no `facets`)
  is still understood.
- Documents are resolved against `<tenant>/docs`, which is what the relative `../<type>/<name>.md` links inside
  the documents are relative to. Source YAML is resolved against the sibling `<tenant>/resources` — a second,
  separate served root, restricted to `.yaml`.
- `resources/` is **not** a discovery marker: an export whose `docs/` were copied without it stays a valid
  tenant whose YAML views simply 404.
- A document's source is located by mirroring its own path (`docs/<type>/<name>.md` ↔
  `resources/<type>/<name>.yaml`) — exactly inverting how the CLI derives the document path. The `source`
  frontmatter is only a label, and only resources listed in `docs/index.yaml` are reachable.
- A matched tenant **owns its whole subtree**; discovery does not descend further looking for nested tenants.
- Directories whose name starts with `_` or `.` are skipped (housekeeping folders such as `_to_delete/`).
- Counts in the picker and sidebar come from the index (`counts.documented`, `counts.pending`,
  `counts.excluded`), never from walking the tree. The tenant's display name is the index's `tenant` field
  (the Entra default domain), falling back to the folder name.

## Routes

| Route | Response |
| --- | --- |
| `GET /` | Tenant picker (`views/picker.hbs`), with each tenant's export download link. |
| `GET /healthz` | JSON `{ status, tenants, documents, pending }`. |
| `GET /:tenant` | The tenant landing page: `docs/summary.md`, or the `docs/index.yaml` listing when there is none. Takes one repeatable filter parameter per taxonomy axis. |
| `GET /:tenant/summary` | `302` to `/:tenant` — the summary is that page's body, not a separate document. |
| `GET /:tenant/_export/confluence` | The whole tenant as an `application/zip` attachment for Confluence's HTML import. Any other format 404s. |
| `GET /:tenant/_resource/*path` | The source YAML behind a document, syntax highlighted; the `.yaml` suffix is optional. |
| `GET /:tenant/_resource/*path?raw` | The same file as `text/plain; charset=utf-8` (`nosniff`), for copy-paste. |
| `GET /:tenant/*path` | A document inside the tenant's `docs/` folder; the `.md` suffix is optional. Filter parameters apply to its sidebar. |

Anything that does not resolve to a Markdown file inside the tenant's `docs/` — or to a `.yaml` file inside
its `resources/` — renders the 404 view, which never leaks a filesystem path. `docs/generate.md` is tool
input, not documentation, and is never served.

`_resource` and `_export` are *representation* prefixes, not path segments: they never appear in the
breadcrumb, and they cannot collide with a resource type because no Azure/Graph type segment starts with `_`.

## Taxonomy filters

`azure-rd docs generate-index` can classify each resource along one or more **axes** — *Programme* (CIS
hardening, Defender, VPN, …), *Platform*, *Assignment scope*, whatever the operator declares in the CLI's
`taxonomy:` config section. When it does, `docs/index.yaml` carries a header `facets` registry (each axis: `id`,
`label`, and its `values` in display order with per-tenant `count`s) and a per-resource `facets` map of **value
ids only**; labels are always resolved from the header.

The browser **reads that membership and derives none of its own**, and names no axis anywhere in its code or
templates: an axis added to the CLI's config shows up here with no change. The classification is resolved once,
at index time, so every consumer of the index sees the same one.

- Every page that shows the sidebar offers **one chip group per axis**, headed by the axis's label, and accepts
  **one repeatable query parameter per axis id**: `?programme=defender&programme=vpn&platform=macos`. Values are
  **OR-ed within an axis** and **AND-ed across axes**.
- **Every chip toggles its own value** and keeps the rest of the selection. The whole selection rides along in
  every document link, and a **Clear filters** link plus a *showing N of M* line state what is applied — all in
  the URL, with no client-side state.
- **`?<axis>=_uncategorised`** lists what that axis matched to nothing, so a taxonomy that stops matching shows
  up as a full bucket rather than as a quietly thinning tree.
- **Counts follow the selection.** Each value is counted against the resources the *other* axes allow, so
  picking one value does not zero its siblings. A value another filter has emptied is no longer offered; a
  **selected** value stays visible even at 0 so the choice can be undone; while nothing is filtering, a
  zero-count value stays listed — "empty here" is information. Totals count **distinct resources**, since a
  resource can hold several values on one axis.
- **The document you are viewing stays in its sidebar** even when the filter excludes it. It is not counted
  into the selection, so it is marked *outside the filter* and the count line adds *plus the document you are
  viewing* — which is why the tree can hold one row more than the count says. Unknown values, unknown axes and
  an axis id that would collide with a route's own parameter (`?raw`) are ignored rather than rendering what
  would look like an empty tenant.
- An index written **without** a taxonomy offers no filter and renders the per-type tree unchanged. A
  version-2 index still filters: its single programme axis is synthesised from `programmes`/`groups`.

The filters narrow *navigation*, not page bodies. The Confluence export ignores a selection — it always exports
the whole tenant — but it *indexes* by the same axes through the same rule (see `EXPORT_INDEX`).

Structuring the sidebar *by* an axis instead of by resource type is the other half of this and is not
implemented; see [`NEXT-ITERATIONS.md`](NEXT-ITERATIONS.md).

## Confluence export

`GET /:tenant/_export/confluence` returns one zip containing one folder, which is what Confluence's HTML import
expects: the folder name becomes the space name, each `.html` file becomes a page, and **the file name becomes
the page title**.

- **Space name** — `<tenant domain> documentation`.
- **Page titles** — `<type leaf> — <display name>`, taking the display name from `docs/index.yaml`, then the
  document's H1, then the file's base name. Characters illegal in a file name or a Confluence title are
  replaced; a residual collision gets a `(2)` suffix and a line on the overview page — never an overwrite.
- **Overview page** — `Overview.html`, built from `docs/summary.md` plus a link list that stands in for the
  sidebar, since an imported space is a **flat** set of pages with no hierarchy. Its index is configurable
  through `EXPORT_INDEX`: by resource type (default), by taxonomy axis — one `<h2>` per axis and one collapsible
  `<details>` per value, which the importer turns into a native expand — or both. The axis sections classify
  through the *same* rule as the sidebar filter, so a page appears under every value it holds, the
  uncategorised bucket is always rendered, and each count equals the links beneath it. An index that declares
  no usable axis renders the by-type list whatever the variable asks for.
- **Provenance** — each page opens with the source, export timestamp and generation hashes from the document's
  frontmatter, and a note that the page is generated.
- **Determinism** — zip entries carry the export's own `generatedAt`, not the wall clock, so exporting an
  unchanged tenant twice produces the same bytes.

It stays read-only: documents are enumerated from `docs/index.yaml`, read through the same path guard as every
other route, and the archive is assembled in memory and streamed — no temporary file, nothing written under
`DOCS_ROOT`. A document the index lists but that cannot be read is reported under *Not exported* on the overview
page instead of failing the export.

**One-way.** Import *creates* a space rather than updating one, so re-importing yields a second space, and
edits made in Confluence are lost the next time the export is imported. Beyond that:

- **`<details>` blocks are passed through untouched**, which is what the importer wants: verified against a
  Confluence Cloud import, each block becomes a **native collapsible expand** with its `path: value` summary,
  nesting and inline formatting intact. The summary is never parsed into key/value, so a value that itself
  contains ` = ` cannot be mangled.
- **No media.** Each served root hands out exactly one extension (`.md` under `docs/`), so the exporter cannot
  read an image; images travel as their `alt` text.
- **In-document anchors do not survive**, because the flat space has no place for them. Heading permalinks are
  unwrapped, and a link whose target is not a page in the export degrades to its text.
- **Only what the importer preserves is emitted.** The serialiser is an allowlist: unsupported HTML is
  unwrapped, scripts and embeds are dropped, and a bare `<key>` in prose (macOS plist quotes are full of them)
  is escaped rather than shipped as a phantom element.
- **Source YAML is not attached.** Whole-tenant only — no per-type or single-document export.

## Rendering

One `markdown-it` instance (built once, in `MarkdownRendererService`) renders every document:

- **Raw HTML is enabled** (`html: true`) because the `<details>`/`<summary>` settings blocks *are* the
  documentation; the trust boundary is the docs root, which is operator-supplied content. `typographer` stays
  off so configuration values keep their quotes and dashes.
- **Frontmatter** is parsed with `gray-matter`: `source` (linked to the YAML view) and `generatedAt` are shown
  as page metadata; nothing from it reaches the body. The documents also echo their source file as a code-only
  paragraph under the H1; that duplicate is dropped at render time (only when it matches the document's own
  `source` and stands alone on its line).
- **Links**: relative `.md` links are resolved against the current document's directory and turned into app
  routes (`../groups/g1.md` → `/<tenant>/Microsoft.Graph/groups/g1`). Anchors, absolute routes, schemes,
  protocol-relative URLs, non-`.md` targets and links escaping the tenant root are left unchanged.
- **Caching**: renders are cached by `mtimeMs` + `size` from a per-request `stat()`, bounded by an entry cap,
  so a regenerated document appears without a restart.

### Section styling

The CLI declares each document type's `##` headings as a closed, verbatim set (the `<!-- doc-headings: … -->`
contract in its `doc-prompt.md`), which makes the heading text a machine contract rather than prose.
`src/docs/section-hooks.ts` uses it to give the stylesheet something to reach:

- **`data-section="<slug>"` on every `h2`/`h3`**, plus `class="doc-section-heading"` on those whose heading is
  in the declared vocabulary. Each of those gets an icon and one of four role colours (risk, substance,
  relations, meta) drawn as a masked SVG, so one value drives both themes. A heading outside the vocabulary
  keeps the attribute and gets no treatment, which is why documents generated before the contract still render
  as plain prose.
- **Heading ids are slugged locally** rather than by `markdown-it-anchor`'s percent-encoding default:
  `#lifecycle-and-operations` instead of `#lifecycle-%26-operations`, and an em dash in a `summary.md` H1 no
  longer produces `%E2%80%94`. `&` slugs to `and`, so a heading has the same anchor and section identity
  whichever way it is spelled.
- **Each H2 run is wrapped in `<section class="doc-section" data-section="…">`**, which lets a section own a
  panel, a rail or its own density. *Settings*, *Properties*, *Definition* and *Membership* switch to a denser
  mode (one document in the reference export holds 317 settings nested five deep); *Security* and *Expiry and
  renewal* get a rail and deliberately no tint.
- **The tool-maintained marker pairs become elements**: a matched `<!-- assignments:start -->` /
  `<!-- assignments:end -->` pair (likewise `targeted-by`, `used-by` and `notifications`) renders as a
  `<div class="doc-assignments">`, because an HTML comment survives into the DOM but cannot be selected. An
  unmatched marker is left as a comment. An H2 *inside* a matched pair never opens a section — the one rule that
  keeps a spliced block from being straddled by a `<section>`, and keeps `## Used by` reading as part of the
  section it was spliced into.
- **Setting blocks are styled from their own attributes.** The generator opens each as
  `<details data-setting="<YAML path>">` and marks it `data-note="security"` or `data-note="inert"`; those get
  a risk rail plus a `security` chip, and a dashed, de-emphasised frame plus a `no effect` chip. The chips are
  `::after` content on the block's own `<summary>` — decorative only, since the same fact is in the prose.
- **The metadata table** each document opens with is classed `.doc-metadata` (the first table after the H1,
  so an extra heading before it does not break the match). The summary's **Findings** table is classed
  `.findings` with a `data-severity` per row, so severity can be coloured.

None of this reaches the Confluence export: `<div>` and `<section>` unwrap and the attributes are on no
element allowlist, so an exported page is unaffected.

### YAML view

One `shiki` highlighter (built once, in `YamlHighlighterService`, loaded through the ESM `dynamicImport`
escape hatch) renders the source YAML with dual-theme output, so dark mode and the `#L42` line highlight are
plain `prefers-color-scheme` and `:target` CSS. A highlighter load failure, or a file above the size cap,
degrades to an escaped `<pre>` rather than an error.

### CSS

Tailwind CSS v4 with `@tailwindcss/typography`. Utility classes live in the `.hbs` templates, so
`src/styles.css` declares `@source "../views/**/*.hbs"` — templates outside `views/` must be added there or
their classes are purged. `styles.css` holds only the theme tokens and the rules Tailwind cannot express
(`<details>`/`<summary>`, table overflow, section identity, dark mode).

## Security

`*path` is attacker-controllable, and `resolveWithinRoot()` in `src/docs/path-safety.ts` is the single guard
for it — used through `resolveWithinTenant()` for documents and `resolveResource()` for source YAML. It:

- rejects null bytes, absolute paths and any `..` segment before touching the filesystem;
- serves only files ending in the **one** extension that root allows — `.md` for `docs/`, `.yaml` for
  `resources/` — so a document can never be served from the resources root, nor a resource from `docs/`, and
  `..` cannot cross between them (`.yml` is deliberately not served);
- re-checks, **after** `realpath()` resolution, that the target is still inside that root, so a symlink cannot
  escape.

All request-derived filesystem access goes through that function. Error responses never surface an absolute
filesystem path, stack trace or raw exception message. The app stays read-only: no route writes, moves or
deletes anything under the docs root.

## Tests

```bash
npm test
```

Jest (`ts-jest`, `testRegex: .*\.spec\.ts$`), run with `--experimental-vm-modules` because `markdown-it-anchor`
v9 and `shiki` are ESM-only. Tests never hit the network and never read the real `output/` export: fixtures are
built in a temp directory and removed afterwards.

| Suite | Covers |
| --- | --- |
| `test/path-safety.spec.ts` | Traversal, symlink escape, null bytes, absolute paths, one extension per root for both roots. |
| `test/tenant-index.spec.ts` | `index.yaml` parsing (malformed file rejected, later schema accepted, `facets`/`programmes`/`groups`/vocabularies), navigation building, every taxonomy-filter rule listed above (OR/AND, selection-aware counts, uncategorised bucket, exempt active document, version-2 synthesis, no-taxonomy passthrough), and `filterableAxes`/`groupByAxis` — the rule the export shares. |
| `test/section-hooks.spec.ts` | Heading slugs, declared vs undeclared headings, matched/unmatched marker pairs, section wrapping (an H2 inside a spliced block never opens one), metadata-table detection. |
| `test/docs.e2e.spec.ts` | supertest against fixture tenants: discovery, picker, landing page and its index fallback, the `/summary` redirect, sidebar, `<details>` passthrough, cross-type links, 404s and traversal, `generate.md` not served, no-restart refresh of a document, the summary, the index and a resource; the section hooks in rendered output; the YAML view with `#L` anchors, `?raw` and the switcher; the Confluence export's content type, archive shape, `EXPORT_INDEX` per request, and the invariant that an export changes neither the rendered HTML nor a byte under the docs root. |
| `test/export.spec.ts` | The exporter's pure modules: page titles, the allowlist serialiser, href rewriting, the format (space, page plan, provenance, overview and its axis index in all three modes), `parseExportIndexMode`, and the shared `<details>` fixture. |
| `test/styles-build.spec.ts` | Compiles `src/styles.css` with the local Tailwind CLI and asserts the custom rules survive. |

Required coverage for a change: `path-safety.ts` → `path-safety.spec.ts`; `tenant-index.ts` →
`tenant-index.spec.ts`; routes, discovery, rendering, highlighting or link rewriting → `docs.e2e.spec.ts`;
`styles.css` → `styles-build.spec.ts`.

## Project layout

```
web/
├── src/
│   ├── main.ts                          # bootstrap (PORT)
│   ├── configure-app.ts                 # hbs view engine + static assets (shared with e2e tests)
│   ├── dynamic-import.ts                # native import() escape hatch for ESM-only deps
│   ├── app.module.ts
│   ├── styles.css                       # Tailwind v4 entry (+ @source for .hbs)
│   └── docs/
│       ├── docs.module.ts
│       ├── docs.controller.ts           # routes, breadcrumb, 404 mapping
│       ├── tenant-discovery.service.ts  # DOCS_ROOT scan + 30 s TTL cache + index cache
│       ├── tenant-index.ts              # docs/index.yaml parsing + navigation + facet filters
│       ├── markdown-renderer.service.ts # markdown-it instance + mtime render cache
│       ├── yaml-highlighter.service.ts  # shiki highlighter + mtime render cache
│       ├── link-rewrite.ts              # .md href → app route, H1 title extraction
│       ├── findings-table.ts            # the summary's Findings table → .findings + data-severity
│       ├── section-hooks.ts             # heading slugs, data-section, marker blocks, metadata table
│       ├── path-safety.ts               # the security boundary
│       └── export/
│           ├── export.service.ts        # zip assembly + streaming (the only Nest piece)
│           ├── confluence.ts            # the format: space, page plan, overview, provenance
│           ├── export-index-mode.ts     # EXPORT_INDEX → by-type / axis / both overview index
│           ├── html-allowlist.ts        # rendered HTML → what the importer preserves
│           └── page-name.ts             # page titles = file names, sanitised and deduplicated
├── views/                               # page/tenant/resource/picker/error + partials/{header,sidebar}
├── public/                              # app.css (generated, gitignored)
├── test/                                # *.spec.ts
├── .env.example                         # every variable at its default
├── CHANGELOG.md                         # Keep a Changelog; released sections match web/vX.Y.Z tags
└── NEXT-ITERATIONS.md                   # outstanding work and parked ideas
```

## Development conventions

- Run everything through the npm scripts from this folder (`npm run build`, `npm run start:dev`,
  `npm run start:prod`, `npm test`). The Go `Makefile` in `../go` does not apply here, and there is no lint
  script wired up yet.
- The architecture invariants and style/testing requirements live in `.windsurf/rules/` **in this folder**
  (`01-architecture.md`, `02-style-and-quality.md`, `06-next-iterations.md`); the Go rules in `../go` do not
  apply.
- Non-negotiables worth knowing before touching the code: read-only (no route mutates anything); one path guard
  (`resolveWithinRoot`) with one extension per root; one `markdown-it` instance and one `shiki` highlighter,
  both built at module init; `html: true` stays on; no client-side JavaScript; regenerated files must appear
  without a restart; environment variables are the only configuration and `DOCS_ROOT` the only link to the CLI.
- Every user- or operator-visible change gets an entry in [`CHANGELOG.md`](CHANGELOG.md) under `## [Unreleased]`
  in the same edit. Released sections are `## [X.Y.Z] - YYYY-MM-DD` matching a `web/vX.Y.Z` tag, with `version`
  in `package.json` kept in step; the release procedure is in the [monorepo README](../README.md#releasing).
  Changes to the Go CLI go in [`../go/CHANGELOG.md`](../go/CHANGELOG.md) instead.
- This README is the single source of truth for what the browser does today; deliberate scope cuts go in
  [`NEXT-ITERATIONS.md`](NEXT-ITERATIONS.md). No other Markdown files live here.

## Known limitations

Deliberate scope cuts are listed in [`NEXT-ITERATIONS.md`](NEXT-ITERATIONS.md): no search, no highlighting of
code fences *inside* documents, no YAML view for resources the index does not list (excluded bulk types,
unreferenced groups), single-segment tenant routes only, no table of contents for the summary, no explicit
dark-mode toggle, and no export format other than Confluence HTML (whose own limits are under
[Confluence export](#confluence-export)). Navigation groups by resource type; the `platformGroup`/`functionGroup`
frontmatter the index can carry is shown as badges when present rather than driving the tree.

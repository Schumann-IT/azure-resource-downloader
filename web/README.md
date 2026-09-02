# azure-rd-docs-web

A read-only documentation browser for the Markdown exports produced by
[`azure-resource-downloader`](../README.md). It discovers tenant folders on disk, renders their
generated Markdown as HTML, and rewrites the cross-document `.md` links into application routes.

It **only reads** the export tree. Nothing in this app fetches from Azure, writes documents, or
mutates the export in any way.

## What it does

- **Tenant discovery** — walks `DOCS_ROOT` (up to 3 levels deep) and treats any directory that
  contains a readable `docs/index.yaml` as a tenant. That file is the navigation index written by
  `azure-rd docs generate-index`; documents are resolved against the `docs/` folder.
- **Summary landing page** — `GET /:tenant` renders `docs/summary.md`, the tenant-wide management
  summary the generation agent writes at the docs root. It is optional: an export that has no summary
  falls back to listing `docs/index.yaml` (resources grouped by type, with the LLM-authored summary,
  a *pending* marker for resources with no document yet, and count-only assignment badges).
- **Sidebar navigation** — every page carries the index-driven tree: one collapsible `<details>` per
  resource type, the section of the document being viewed opened and the document itself marked, plus
  the tenant counts, export timestamp, the incomplete-export banner and the excluded bulk types.
  Collapsing is pure HTML — there is still no client-side JavaScript.
- **Source YAML view** — every document links to the exported resource it was written from
  (`GET /:tenant/_resource/<type>/<name>`), syntax highlighted with `shiki`, one addressable line per
  `#L42` anchor, and `?raw` for plain text. A **Documentation | YAML** switcher in the top bar flips
  between the two representations of the same resource.
- **Rendering** — `markdown-it` with `html: true`, so the `<details>`/`<summary>` disclosure blocks
  that make up the bulk of the generated docs pass through untouched. Headings get anchors via
  `markdown-it-anchor`.
- **Link rewriting** — relative `.md` links are resolved at render time against the current
  document's directory and turned into absolute app routes (`../groups/g1.md` →
  `/<tenant>/Microsoft.Graph/groups/g1`).
- **Frontmatter** — parsed with `gray-matter`; `source` (linked to the YAML view) and `generatedAt`
  are shown as page metadata, the rest is never rendered into the body. The documents also echo their
  source file as a code-only paragraph under the H1; that duplicate is dropped at render time (only
  when it matches the document's own `source` and stands alone on its line).
- **Confluence HTML export** — `GET /:tenant/_export/confluence` streams the whole tenant as a zip
  ready for Confluence's HTML import, offered as a download link per tenant on the picker.
  **Provisional** — see [Confluence export](#confluence-export) for what it cannot do.
- **No-restart refresh** — regenerated documents, re-downloaded resources *and* a regenerated
  `index.yaml` appear on the next request (per-request `stat()` against an mtime/size-keyed cache);
  newly generated tenants appear within the 30 s discovery TTL.

## Requirements

- Node.js **>= 20**
- An export tree produced by `azure-rd`, with `azure-rd docs generate-index` already run for each
  tenant (the browser needs `docs/index.yaml`)

## Setup

```bash
npm install
```

## Run

```bash
# development: Tailwind watch + Nest watch
npm run start:dev

# production-style: build then run the compiled output
npm run start:prod
```

Then open <http://localhost:3000>.

`npm start` runs `dist/main.js` only — run `npm run build` first.

> Views (`views/`) and static assets (`public/`) are resolved from `process.cwd()`, so the server
> must be started from the `web/` directory in both dev and prod.

## Configuration

Configuration is environment variables only; there is no config file.

| Variable | Default | Purpose |
| --- | --- | --- |
| `DOCS_ROOT` | `../output` (relative to `process.cwd()`) | Root that is scanned for tenant folders. |
| `PORT` | `3000` | HTTP listen port. |

```bash
DOCS_ROOT=/path/to/output PORT=4000 npm run start:prod
```

### Expected docs root layout

```
<DOCS_ROOT>/
└── <tenant>/                      # e.g. contoso.com/  (may be nested, up to 3 levels)
    ├── resources/                 # written by `azure-rd download` — served read-only as the YAML view
    │   └── Microsoft.Graph/
    │       └── <endpoint>/
    │           └── <name>.yaml
    └── docs/
        ├── index.yaml             # required marker (azure-rd docs generate-index)
        ├── generate.md            # agent prompt — never served
        ├── summary.md             # optional tenant summary — the landing page body
        └── Microsoft.Graph/
            └── <endpoint>/
                └── <name>.md
```

Discovery rules:

- A directory counts as a tenant when `docs/index.yaml` exists **and parses** as a `version: 1`
  index; a malformed or unreadable index makes the folder *not* a tenant instead of crashing
  discovery.
- Documents are resolved against `<tenant>/docs`, which is what the relative `../<type>/<name>.md`
  links inside the documents are relative to. Source YAML is resolved against the sibling
  `<tenant>/resources` — a second, separate served root, restricted to `.yaml`.
- `resources/` is **not** a discovery marker: an export whose `docs/` were copied without it stays a
  valid tenant whose YAML views simply 404.
- A document's source is located by mirroring its own path (`docs/<type>/<name>.md` ↔
  `resources/<type>/<name>.yaml`), exactly inverting what the CLI does when it derives the document
  path. The `source` frontmatter is only a label, and only resources listed in `docs/index.yaml` are
  reachable.
- A matched tenant **owns its whole subtree** — discovery does not descend further looking for
  nested tenants.
- Directories whose name starts with `_` or `.` are skipped (e.g. `_to_delete/`).
- The counts in the picker and on the landing page come from the index (`counts.documented`,
  `counts.pending`, `counts.excluded`), never from walking the tree.

## Routes

| Route | Response |
| --- | --- |
| `GET /` | Tenant picker (`views/picker.hbs`), with each tenant's export download link. |
| `GET /healthz` | JSON `{ status, tenants, documents, pending }`. |
| `GET /:tenant` | The tenant landing page: `docs/summary.md`, or the `docs/index.yaml` listing when there is none. |
| `GET /:tenant/summary` | `302` to `/:tenant` — the summary is that page's body, not a separate document. |
| `GET /:tenant/_export/confluence` | The whole tenant as a `application/zip` attachment for Confluence's HTML import. |
| `GET /:tenant/_resource/*path` | The source YAML behind a document, syntax highlighted; the `.yaml` suffix is optional. |
| `GET /:tenant/_resource/*path?raw` | The same file as `text/plain; charset=utf-8` (`nosniff`), for copy-paste. |
| `GET /:tenant/*path` | A document inside the tenant's `docs/` folder; the `.md` suffix is optional. |

Anything that does not resolve to a Markdown file inside the tenant — or to a `.yaml` file inside its
`resources/` folder — renders the 404 view. The 404 page never leaks a filesystem path.
`docs/generate.md` is tool input, not documentation, and is never served.

`_resource` and `_export` are *representation* prefixes, not path segments: they never appear in the
breadcrumb, and they cannot collide with a resource type because no Azure/Graph type segment starts
with `_`.

## Confluence export

`GET /:tenant/_export/confluence` returns one zip containing one folder, which is what Confluence's
HTML import expects: the folder name becomes the space name, each `.html` file becomes a page, and
**the file name becomes the page title**.

- **Space name** — `<tenant domain> documentation`.
- **Page titles** — `<type leaf> — <display name>`, taking the display name from `docs/index.yaml`,
  then the document's H1, then the file's base name. Characters that are illegal in a file name or a
  Confluence title are replaced, and a residual collision gets a `(2)` suffix and a line on the
  overview page — never an overwrite.
- **Overview page** — `Overview.html`, built from `docs/summary.md` plus a grouped link list that
  stands in for the sidebar, since an imported space is a **flat** set of pages with no hierarchy.
- **Provenance** — each page opens with the source, export timestamp and generation hashes from the
  document's frontmatter, and a note that the page is generated.
- **Determinism** — zip entries carry the export's own `generatedAt`, not the wall clock, so
  exporting an unchanged tenant twice produces the same bytes.

It stays read-only: documents are enumerated from `docs/index.yaml`, read through the same path guard
as every other route, and the archive is assembled in memory and streamed — no temporary file, and
nothing written under `DOCS_ROOT`. A document the index lists but that cannot be read is reported
under *Not exported* on the overview page instead of failing the export.

**Provisional, and one-way.** Import *creates* a space rather than updating one, so re-importing
yields a second space — and edits made in Confluence are lost the next time the export is imported.
Beyond that:

- **`<details>` blocks are passed through untouched.** They are the bulk of the documentation, and
  Confluence's import FAQ neither lists them as preserved nor says what happens to them. Passthrough
  is what makes the first real import cheap to evaluate; alternatives are in
  [`NEXT-ITERATIONS.md`](NEXT-ITERATIONS.md).
- **No media.** Each served root hands out exactly one extension (`.md` under `docs/`), so the
  exporter cannot read an image; images travel as their `alt` text.
- **In-document anchors do not survive**, because the flat space has no place for them. Heading
  permalinks are unwrapped, and a link whose target is not a page in the export degrades to its text.
- **Only what the importer preserves is emitted.** The serialiser is an allowlist: unsupported HTML
  is unwrapped, scripts and embeds are dropped, and a bare `<key>` in prose (macOS plist quotes are
  full of them) is escaped rather than shipped as a phantom element.
- **Source YAML is not attached.** Whole-tenant only — no per-type or single-document export.

## Security

`*path` is attacker-controllable, and `resolveWithinRoot()` in `src/docs/path-safety.ts` is the
single guard for it — used through `resolveWithinTenant()` for documents and `resolveResource()` for
source YAML. It:

- rejects null bytes, absolute paths and any `..` segment before touching the filesystem;
- serves only files ending in the **one** extension that root allows — `.md` for `docs/`, `.yaml` for
  `resources/` — so a document can never be served from the resources root, nor a resource from
  `docs/`, and `..` cannot cross between them (`.yml` is deliberately not served);
- re-checks, **after** `realpath()` resolution, that the target is still inside that root, so a
  symlink cannot escape.

All request-derived filesystem access must go through that function. See `test/path-safety.spec.ts`
and the traversal cases in `test/docs.e2e.spec.ts`. The app stays read-only: the YAML routes only
read and never write, move or delete anything under the docs root.

## Tests

```bash
npm test
```

Jest (`ts-jest`, `testRegex: .*\.spec\.ts$`), run with `--experimental-vm-modules` because
`markdown-it-anchor` v9 and `shiki` are ESM-only.

- `test/path-safety.spec.ts` — traversal, symlink escape, null bytes, absolute paths, and the
  one-extension-per-root rule for both roots.
- `test/tenant-index.spec.ts` — `index.yaml` parsing (including rejection of a malformed or
  non-`version: 1` file) and navigation building.
- `test/section-hooks.spec.ts` — heading slugs (the em dash, `&` → `and`, inline markup), the
  declared-vs-undeclared heading split, matched and unmatched marker pairs with the ranges they report,
  section wrapping (including that an H2 inside a spliced block never opens one), and locating the
  metadata table.
- `test/docs.e2e.spec.ts` — supertest against self-contained fixture tenants in a temp dir:
  discovery, picker, the summary landing page and its index fallback, the `/:tenant/summary`
  redirect, the sidebar with its active document, nested `<details>`, cross-type link resolution,
  404s, traversal, `generate.md` not being served, no-restart refresh of a document, the summary and
  the index; the section hooks (a declared heading styled, an undeclared one left alone, one balanced
  `<section>` per H2 run, a spliced `used-by` block that no section straddles, the assignments marker
  pair as an element, the generator's `data-setting`/`data-note` passed through, the metadata table
  classed and the findings table not);
  plus the YAML view with its `#L` anchors, `?raw`, the top-bar switcher (and its absence
  for a document without a `source`), and no-restart refresh of a re-downloaded resource. For the
  Confluence export: the content type, `Content-Disposition`, the space folder and page entries in the
  zip, the download link being on the picker and not on the landing page, the 404s for an unknown format or an unimplemented `<details>` strategy, and — as the
  read-only and cache invariants — that an export changes neither the browser's rendered HTML nor a
  single byte under the docs root.
- `test/export.spec.ts` — the Confluence exporter's pure modules: page-title derivation (illegal
  characters, the display-name/H1/base-name fallbacks, deterministic deduplication), the allowlist
  serialiser (a bare `<key>` escaped, unsupported HTML unwrapped, scripts dropped, heading permalinks
  unwrapped, images reduced to `alt` text), href rewriting, and the format (space name, page plan,
  provenance, the overview's grouped link list). The `<details>` fixture — nested blocks, a
  group-label block with no value, a link inside a block, a value containing ` = ` — is where a
  future transform gets its assertions.
- `test/styles-build.spec.ts` — compiles `src/styles.css` with the local Tailwind CLI and asserts the
  custom `<details>`/`<summary>`, YAML-view (shiki variables, line gutter, `:target`), findings-table,
  section-identity (icons, role tokens, marker blocks, metadata table) and dark-mode rules survive.

Tests never hit the network and never read the real `output/` export.

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
│       ├── tenant-index.ts              # docs/index.yaml parsing + navigation building
│       ├── markdown-renderer.service.ts # markdown-it instance + mtime render cache
│       ├── yaml-highlighter.service.ts  # shiki highlighter + mtime render cache
│       ├── link-rewrite.ts              # .md href → app route, H1 title extraction
│       ├── findings-table.ts            # the summary's Findings table → .findings + data-severity
│       ├── section-hooks.ts             # heading slugs, data-section, marker blocks, metadata table
│       ├── path-safety.ts               # the security boundary
│       └── export/
│           ├── export.service.ts        # zip assembly + streaming (the only Nest piece)
│           ├── confluence.ts            # the format: space, page plan, overview, provenance
│           ├── html-allowlist.ts        # rendered HTML → what the importer preserves
│           ├── page-name.ts             # page titles = file names, sanitised and deduplicated
│           └── details-strategy.ts      # the seam for the open <details> question
├── views/                               # page/tenant/resource/picker/error + partials/{header,sidebar}
├── public/                              # app.css (generated, gitignored)
└── test/
```

## Frontend

Server-rendered Handlebars + Tailwind CSS v4 with `@tailwindcss/typography`; no client-side
JavaScript. Syntax highlighting is server-side (`shiki`, dual-theme output), so dark mode and the
`#L42` line highlight are plain `prefers-color-scheme` and `:target` CSS.

### Section styling

The CLI declares each document type's `##` headings as a closed, verbatim set (the
`<!-- doc-headings: … -->` contract in its `doc-prompt.md`), which makes the heading text a machine
contract rather than prose. `src/docs/section-hooks.ts` uses it to give the stylesheet something to
reach:

- **`data-section="<slug>"` on every `h2`/`h3`**, plus `class="doc-section-heading"` on the ones whose
  heading is in the declared vocabulary. Each of those gets an icon and one of four role colours
  (risk, substance, relations, meta) drawn as a masked SVG, so one value drives both themes. A heading
  outside the vocabulary keeps the attribute and gets **no** treatment, which is why documents
  generated before the contract still render as plain prose.
- **Heading ids are slugged locally** rather than by `markdown-it-anchor`'s percent-encoding default,
  so `#lifecycle-and-operations` replaces `#lifecycle-%26-operations` and the em dash in a
  `summary.md` H1 no longer produces `%E2%80%94`. `&` slugs to `and`, so a heading has the same anchor
  and the same section identity whichever way it is spelled.
- **Each H2 run is wrapped in `<section class="doc-section" data-section="…">`**, which is what lets a
  section own a panel, a rail or its own density — unreachable from the heading alone, because
  `h2#settings ~ *` bleeds into every section that follows. *Settings*, *Properties*, *Definition* and
  *Membership* switch to a denser mode (one document in the reference export holds 317 settings nested
  five deep); *Security* and *Expiry and renewal* get a rail and deliberately no tint.
- **The tool-maintained marker pairs become elements**: a matched
  `<!-- assignments:start -->` / `<!-- assignments:end -->` pair (likewise `targeted-by`, `used-by` and
  `notifications`) renders as a `<div class="doc-assignments">`, because an HTML comment survives into
  the DOM but cannot be selected. An unmatched marker is left as a comment. An H2 *inside* a matched pair
  never opens a section — that single rule is what keeps a spliced block from being straddled by a
  `<section>`, and it keeps `## Used by` reading as part of the section it was spliced into.
- **Setting blocks are styled from their own attributes.** The generator opens each as
  `<details data-setting="<YAML path>">` and marks it `data-note="security"` or `data-note="inert"`;
  those get a risk rail plus a `security` chip, and a dashed, de-emphasised frame plus a `no effect`
  chip, respectively. The chips are `::after` content on the block's own `<summary>` — decorative only,
  since the same fact is in the prose.
- **The metadata table** each document opens with is classed `.doc-metadata` — found as the first
  table after the H1, so an extra heading before it does not break the match.

None of this reaches the Confluence export: `<div>` and `<section>` unwrap and the attributes are not on
any element allowlist, so an exported page is unaffected.

Because the utility classes live in `.hbs` files outside the CSS directory, `src/styles.css` declares
`@source "../views/**/*.hbs"` — new templates outside `views/` must be added there or their classes
get purged. Dark mode follows `prefers-color-scheme`.

`public/app.css` is a build artifact (`npm run css:build`) and is gitignored, so a fresh clone must
build the CSS before the pages look right — `build`, `start:dev` and `start:prod` all do this.

## Development conventions

The architecture invariants and the style/testing requirements for this project live in
`.windsurf/rules/` **in this folder** (`01-architecture.md`, `02-style-and-quality.md`). They are
self-contained: the Go rules in the parent repository do not apply here.

User-visible changes are recorded in [`CHANGELOG.md`](CHANGELOG.md) (Keep a Changelog + SemVer);
changes to the Go CLI go in [`../go/CHANGELOG.md`](../go/CHANGELOG.md) instead.

## Known limitations

Deliberate scope cuts are listed in [`NEXT-ITERATIONS.md`](NEXT-ITERATIONS.md) — no search, no
highlighting of the code fences *inside* documents, no YAML view for resources the index does not
list (excluded bulk types, unreferenced groups), single-segment tenant routes only, no table of
contents for the summary, no explicit dark-mode toggle, and no export format other than Confluence
HTML (whose own limits are listed under [Confluence export](#confluence-export)). Navigation groups by resource type: the documents do not yet carry the
`platformGroup`/`functionGroup` frontmatter the index can enrich them with, so those are shown as
badges when present rather than driving the tree.

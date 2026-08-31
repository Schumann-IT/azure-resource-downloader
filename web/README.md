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
| `GET /` | Tenant picker (`views/picker.hbs`). |
| `GET /healthz` | JSON `{ status, tenants, documents, pending }`. |
| `GET /:tenant` | The tenant landing page: `docs/summary.md`, or the `docs/index.yaml` listing when there is none. |
| `GET /:tenant/summary` | `302` to `/:tenant` — the summary is that page's body, not a separate document. |
| `GET /:tenant/_resource/*path` | The source YAML behind a document, syntax highlighted; the `.yaml` suffix is optional. |
| `GET /:tenant/_resource/*path?raw` | The same file as `text/plain; charset=utf-8` (`nosniff`), for copy-paste. |
| `GET /:tenant/*path` | A document inside the tenant's `docs/` folder; the `.md` suffix is optional. |

Anything that does not resolve to a Markdown file inside the tenant — or to a `.yaml` file inside its
`resources/` folder — renders the 404 view. The 404 page never leaks a filesystem path.
`docs/generate.md` is tool input, not documentation, and is never served.

`_resource` is a *representation* prefix, not a path segment: it never appears in the breadcrumb, and
it cannot collide with a resource type because no Azure/Graph type segment starts with `_`.

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
- `test/docs.e2e.spec.ts` — supertest against self-contained fixture tenants in a temp dir:
  discovery, picker, the summary landing page and its index fallback, the `/:tenant/summary`
  redirect, the sidebar with its active document, nested `<details>`, cross-type link resolution,
  404s, traversal, `generate.md` not being served, no-restart refresh of a document, the summary and
  the index; plus the YAML view with its `#L` anchors, `?raw`, the top-bar switcher (and its absence
  for a document without a `source`), and no-restart refresh of a re-downloaded resource.
- `test/styles-build.spec.ts` — compiles `src/styles.css` with the local Tailwind CLI and asserts the
  custom `<details>`/`<summary>`, YAML-view (shiki variables, line gutter, `:target`) and dark-mode
  rules survive.

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
│       └── path-safety.ts               # the security boundary
├── views/                               # page/tenant/resource/picker/error + partials/{header,sidebar}
├── public/                              # app.css (generated, gitignored)
└── test/
```

## Frontend

Server-rendered Handlebars + Tailwind CSS v4 with `@tailwindcss/typography`; no client-side
JavaScript. Syntax highlighting is server-side (`shiki`, dual-theme output), so dark mode and the
`#L42` line highlight are plain `prefers-color-scheme` and `:target` CSS.

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
contents for the summary, no explicit dark-mode toggle. Navigation groups by resource type: the documents do not yet carry the
`platformGroup`/`functionGroup` frontmatter the index can enrich them with, so those are shown as
badges when present rather than driving the tree.

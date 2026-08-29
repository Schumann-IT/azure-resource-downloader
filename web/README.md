# azure-rd-docs-web

A read-only documentation browser for the Markdown exports produced by
[`azure-resource-downloader`](../README.md). It discovers tenant folders on disk, renders their
generated Markdown as HTML, and rewrites the cross-document `.md` links into application routes.

It **only reads** the export tree. Nothing in this app fetches from Azure, writes documents, or
mutates the export in any way.

## What it does

- **Tenant discovery** — walks `DOCS_ROOT` (up to 3 levels deep) and treats any directory that
  contains **both** `index.md` **and** `.doc-manifest.json` as a tenant.
- **Rendering** — `markdown-it` with `html: true`, so the `<details>`/`<summary>` disclosure blocks
  that make up the bulk of the generated docs pass through untouched. Headings get anchors via
  `markdown-it-anchor`.
- **Link rewriting** — relative `.md` links are resolved at render time against the current
  document's directory and turned into absolute app routes (`../groups/g1.md` →
  `/<tenant>/Microsoft.Graph/groups/g1`).
- **Frontmatter** — parsed with `gray-matter`; `source` and `generatedAt` are shown as page
  metadata, the rest is never rendered into the body.
- **No-restart refresh** — regenerated documents appear on the next request (per-request `stat()`
  against an mtime/size-keyed cache); newly generated tenants appear within the 30 s discovery TTL.

## Requirements

- Node.js **>= 20**
- An export tree produced by `azure-rd` (documents plus `index.md` and `.doc-manifest.json` per
  tenant)

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
    ├── index.md                   # required marker
    ├── .doc-manifest.json         # required marker
    └── Microsoft.Graph/
        └── <endpoint>/
            └── <name>.md
```

Discovery rules:

- A directory needs **both** markers to count as a tenant; a folder with only `index.md` is ignored.
- A matched tenant **owns its whole subtree** — discovery does not descend further looking for
  nested tenants.
- Directories whose name starts with `_` or `.` are skipped (e.g. `_to_delete/`).
- `resourceCount` in the picker is computed from the manifest: the sum of
  `types[*].resources` entry counts.

## Routes

| Route | Response |
| --- | --- |
| `GET /` | Tenant picker (`views/picker.hbs`). |
| `GET /healthz` | JSON `{ status, tenants, documents }`. |
| `GET /:tenant` | That tenant's `index.md`. |
| `GET /:tenant/*path` | A document inside the tenant; the `.md` suffix is optional. |

Anything that does not resolve to a Markdown file inside the tenant renders the 404 view. The 404
page never leaks a filesystem path.

## Security

`*path` is attacker-controllable, and `resolveWithinTenant()` in `src/docs/path-safety.ts` is the
single guard for it. It:

- rejects null bytes, absolute paths and any `..` segment before touching the filesystem;
- serves only files ending in `.md`;
- re-checks, **after** `realpath()` resolution, that the target is still inside the tenant folder,
  so a symlink cannot escape.

All filesystem access for documents must go through that function. See
`test/path-safety.spec.ts` and the traversal case in `test/docs.e2e.spec.ts`.

## Tests

```bash
npm test
```

Jest (`ts-jest`, `testRegex: .*\.spec\.ts$`), run with `--experimental-vm-modules` because
`markdown-it-anchor` v9 is ESM-only.

- `test/path-safety.spec.ts` — traversal, symlink escape, null bytes, absolute paths.
- `test/docs.e2e.spec.ts` — supertest against a self-contained fixture tenant in a temp dir:
  discovery, picker, index render + link rewrite, nested `<details>`, cross-type link resolution,
  404s, traversal, no-restart refresh.
- `test/styles-build.spec.ts` — compiles `src/styles.css` with the local Tailwind CLI and asserts the
  custom `<details>`/`<summary>` and dark-mode rules survive.

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
│       ├── tenant-discovery.service.ts  # DOCS_ROOT scan + 30 s TTL cache
│       ├── markdown-renderer.service.ts # markdown-it instance + mtime render cache
│       ├── link-rewrite.ts              # .md href → app route, H1 title extraction
│       └── path-safety.ts               # the security boundary
├── views/                               # Handlebars templates (+ partials/header.hbs)
├── public/                              # app.css (generated, gitignored)
└── test/
```

## Frontend

Server-rendered Handlebars + Tailwind CSS v4 with `@tailwindcss/typography`; no client-side
JavaScript. Because the utility classes live in `.hbs` files outside the CSS directory,
`src/styles.css` declares `@source "../views/**/*.hbs"` — new templates outside `views/` must be
added there or their classes get purged. Dark mode follows `prefers-color-scheme`.

`public/app.css` is a build artifact (`npm run css:build`) and is gitignored, so a fresh clone must
build the CSS before the pages look right — `build`, `start:dev` and `start:prod` all do this.

## Development conventions

The architecture invariants and the style/testing requirements for this project live in
`.windsurf/rules/` **in this folder** (`01-architecture.md`, `02-style-and-quality.md`). They are
self-contained: the Go rules in the parent repository do not apply here.

User-visible changes are recorded in [`CHANGELOG.md`](CHANGELOG.md) (Keep a Changelog + SemVer);
changes to the Go CLI go in [`../go/CHANGELOG.md`](../go/CHANGELOG.md) instead.

## Known limitations

Deliberate iteration-1 scope cuts are listed in [`NEXT-ITERATIONS.md`](NEXT-ITERATIONS.md) — no
search, no syntax highlighting, single-segment tenant routes only, no manifest-driven navigation
tree, no explicit dark-mode toggle.

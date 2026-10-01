---
trigger: always_on
description: Architecture of the documentation browser (NestJS)
globs: 
---

# Docs Browser — Architecture

This folder is a **self-contained project**: a read-only browser for the Markdown documentation
produced from `azure-resource-downloader` exports. It shares nothing with the Go CLI in the sibling
`go/` folder except the export tree on disk. The Go rules in `go/.windsurf/rules/` do not apply
here. `README.md` in this folder is the single source of truth (no further Markdown files besides
`NEXT-ITERATIONS.md` and `CHANGELOG.md`).

## Context
- **Stack**: Node >= 20, TypeScript (CommonJS output), NestJS 11 + Express, Handlebars (`hbs`),
  Tailwind CSS v4 + `@tailwindcss/typography`, `markdown-it` (+ `markdown-it-anchor`), `shiki`,
  `gray-matter`, `js-yaml`, `diff` (jsdiff), Jest + supertest.
- **No client-side JavaScript.** Everything is server-rendered; state lives in the URL. The ban is on shipped
  script and client-side state, not on HTML interactivity (`<details>`/`<summary>`, `:target`, `:focus-visible`,
  `prefers-color-scheme`, `<form method="get">`). Where export entry points live: `03-exports.md`.

## Layout
- `src/main.ts` → bootstrap, reads `PORT` via `resolvePort`.
- `src/port.ts` → `resolvePort`, pure `PORT` parsing + fallback (unit tested; `main.ts` cannot be).
- `src/configure-app.ts` → `configureViews(app)`: security headers, static assets, base views dir,
  partial registration, view engine. **Shared by `main.ts` and the e2e tests** so both configure the
  app identically — new view/asset wiring goes here, never inline in `main.ts`.
- `src/dynamic-import.ts` → `dynamicImport`, a `new Function('return import(specifier)')` escape
  hatch. Required because TypeScript would down-level `import()` to `require()`, which cannot load
  ESM-only packages (`markdown-it-anchor` v9, `shiki`). Load ESM-only deps through it, never with
  `require`.
- `src/docs/` → the single feature module (`DocsModule`):
  - `docs.controller.ts` — routes, breadcrumb, 404 mapping.
  - `tenant-discovery.service.ts` — `DOCS_ROOT` scan + TTL cache + `index.yaml` cache.
  - `tenant-index.ts` — pure functions (`parseTenantIndex`, `buildNavigation`).
  - `markdown-renderer.service.ts` — the one `markdown-it` instance + render cache.
  - `yaml-highlighter.service.ts` — the one `shiki` highlighter + render cache.
  - `link-rewrite.ts` — pure functions (`rewriteHref`, `extractTitle`).
  - `path-safety.ts` — `resolveWithinRoot` (+ the `resolveWithinTenant`/`resolveResource`/
    `resolveDriftDocument`/`resolveDriftPayload` wrappers), the security boundary.
  - `drift-observation.ts` — pure functions (`parseObservation`, `driftState`, `tenantDriftState`).
  - `drift-audit.ts` — pure functions (`parseAudit`, `auditState`) for `drift/audit.yaml`, the CLI's
    attribution: read as data, never served.
  - `drift.service.ts` — observation, baseline timestamp, verified-hash and audit reads, mtime-cached.
  - `drift-view.ts` — pure view models for the drift pages and the Drift switcher entries.
  - `yaml-diff.ts` — pure functions (`diffYaml`, `pairRows`) behind the drift and compare YAML diffs, over
    `diff` (jsdiff); `pairRows` gives each hunk its side-by-side rows.
  - `file-cache.ts` — `FileCache`, the bounded mtime + size parsed-file cache the drift and compare
    services share.
  - `resources-metadata.ts` — pure `parseResourcesMetadata` for `resources/metadata.yaml` (+ the
    `RESOURCES_METADATA_FILE` constant and the set-valued reference lookup).
  - `compare-normalise.ts` — pure `normaliseResource`, the **provisional** cross-tenant identity rule.
  - `compare-view.ts` — pure view models for the compare listing, its comparison pane (per-file digests and
    pair status) and pair diff.
  - `compare.service.ts` — cached metadata reads, the per-file digest cache and the two files of a pair.
- `views/` + `views/partials/` → Handlebars templates. `public/app.css` is generated and gitignored.
- `test/` → `*.spec.ts` only (Jest `testRegex`).

## Non-negotiables

### Changelog
- **Every change is reflected in `CHANGELOG.md`.** Any change a user or operator can notice (routes,
  views, discovery/rendering behaviour, environment variables, scripts, dependencies, security
  boundaries, bug fixes) is only complete once it has an entry under `## [Unreleased]`, written when
  the backlog entry is declared done — never left to a release. Details of format and released sections:
  `02-style-and-quality.md`.

### Read-only
- The app **never writes, moves or deletes anything** under the docs root, and never calls Azure.
  No route may mutate state. Adding a write path is a design change, not a feature.

### Path safety
- `resolveWithinRoot()` in `src/docs/path-safety.ts` is the **only** way a request-derived path may
  become a filesystem path — through `resolveWithinTenant()` for documents, `resolveResource()`
  for source YAML, and `resolveDriftDocument()` / `resolveDriftPayload()` for the drift tree. Never
  `path.join` user input and read it directly.
- Its guarantees must be preserved: reject null bytes / absolute paths / `..` segments up front,
  serve exactly **one** extension per resolver (`.md` under `docs/`, `.yaml` under `resources/`;
  under `drift/`, `.md` for drift documents and `.yaml` for payloads; `.yml` is never served), and
  re-verify containment **after** `realpath()` so symlinks cannot escape. One extension per resolver
  is what keeps the representations apart — never widen one to an extension list.
- Both drift resolvers require **at least two path segments**, so the drift tree's top level
  (`metadata.yaml`, `analyze.md`, `audit.yaml`, `index.md`) is unreachable through them by construction.
- Every change to that file needs a matching case in `test/path-safety.spec.ts`.
- Error responses must not leak absolute filesystem paths (asserted in the e2e suite).

### Tenant discovery
- A tenant is a directory containing a **readable, `version: 1`** `docs/index.yaml` — the navigation
  index written by `azure-rd docs generate-index`. There is no `index.md` and no `.doc-manifest.json`.
- A tenant's document root is `<export>/docs`, not the export folder: that is what the relative
  `.md` links inside the documents resolve against. `<export>/resources` is a **second, separate**
  served root (`.yaml` only, read-only) — never merge the two or resolve one against the other.
- `resources/` is **not** a discovery marker: an export whose `docs/` were copied without it stays a
  valid tenant whose YAML views 404.
- `docs/generate.md` is tool input, not documentation — it is never served.
- `<export>/drift` is a **third** served root and **not** a discovery marker. It is ephemeral (the
  CLI deletes it wholesale), so its absence is the normal *no observation* state, never an error.
- A matched tenant owns its whole subtree — do not descend into it looking for more tenants.
- Skip directories starting with `_` or `.` (housekeeping folders such as `_to_delete/`).
- Depth is bounded (`MAX_DEPTH`); keep it bounded.
- Counts and listings shown to the user are **read as data, never by walking the tree**: from the index
  (`counts.documented` / `counts.pending` / `counts.excluded`), and for the tenant compare only from each
  export's `resources/metadata.yaml`.
- A malformed/unreadable index makes the folder *not a tenant* — it must never crash discovery.

### Source YAML view
- Exactly **one** `shiki` highlighter, owned by `YamlHighlighterService` and built in `onModuleInit`.
  It is loaded through `dynamicImport` (ESM-only) and a load failure must degrade to an escaped
  `<pre>`, never crash the app; the same applies above the size cap.
- A document's source is located by **inverting the CLI's mapping** (`docs/<type>/<name>.md` ↔
  `resources/<type>/<name>.yaml`). The `source` frontmatter is a label, never a path input, and no
  directory is walked to find or list resources.
- `_resource` is a *representation* prefix, not a path segment: keep it out of the breadcrumb, and
  keep its route declared **before** the `:tenant/*path` catch-all.
- Highlighted HTML is trusted only because it comes from the highlighter; every other value in
  `views/resource.hbs` stays escaped.

### Drift view
- The comparison is the CLI's job; the app only renders `drift/` and never acts on it.
- The Drift button and the drift page read **one** decision (`driftState` / `tenantDriftState`) so
  they cannot disagree. The button is always rendered; it is inert only when there is nothing to
  land on (no observation, or compared and unchanged), with the reason as its `title`.
- The validity gate compares the observation's baseline against `resources/metadata.yaml`'s
  `generatedAt` (the index's only when `resources/` is absent). A superseded observation is never
  shown as a comparison.
- A comparison or payload is shown only after its files' hashes match the observation.
- `_drift` is a *representation* prefix: out of the breadcrumb, routes declared before the
  `:tenant/*path` catch-all. `raw`, `yaml` and `diff` are reserved query parameters, never taxonomy axes.
- The YAML diff is computed only from two verified files and emitted as plain data, escaped by the
  template — never as trusted HTML.

### Tenant compare
- A **proof of concept**. The normalisation rule in `compare-normalise.ts` is provisional and destined for
  the CLI (`resource compare`, parked in `../go/NEXT-ITERATIONS.md`); keep it data (exported constants), keep
  it applied identically to both sides, keep it announced (the report caption and `&raw`), and mirror any
  change into that Go idea.
- Which rows exist, and where, comes from the two `resources/metadata.yaml` files alone; no directory is
  walked for it. A tenant takes part only with a readable metadata file. Only a pair's *status* reads resource
  files — only for keys both exports list as present, only through `resolveResource`.
- The status is cached **per file, not per pair**: `CompareService` keeps each file's hashes (as exported,
  normalised, normalised without `assignments`) and size, never a text, validated by the file's mtime + size
  and its export's metadata stat, and bounded. A pair's status comes from two digests through `pairStatus`,
  never through `pairComparison`, so no diff is computed per row; the per-file normalisation step
  (`normaliseFile`) is shared by both, so the pane and the diff page cannot apply different rules.
- Both files of a pair are located only through `resolveResource`, and only for a key both exports list as
  present — no new resolver.
- `_compare` is a root-level *representation* prefix: out of the breadcrumb, both routes declared before
  `:tenant`. It cannot shadow a tenant because discovery skips `_`-prefixed folders. `a`, `b`, `raw` and
  `same` are its reserved query parameters.
- Diff hunks go through the shared `diff_table` partial, so drift and compare cannot render a hunk
  differently. The compare diff's whole-file view is `diffYaml`'s `WHOLE_FILE` context plus the partial's
  `full` parameter, never a second partial; the per-line character parts are plain `{ text, changed }` data,
  escaped like every other diff value.
- Both compare pages render the **comparison pane** (`compare_pane`, from `comparePane`) above the diff area,
  full width with no sidebar. Each row is one `<a>` — a CSS grid, never a `<table>` — with no anchor inside
  it; every status marker carries a visually hidden label. The pair being viewed is always shown, whatever
  its status, and row links keep the view's query minus the key.

### Rendering
- Exactly **one** `markdown-it` instance, owned by `MarkdownRendererService` and built in
  `onModuleInit`. Do not construct per request.
- `html: true` is **required** — the `<details>` blocks *are* the documentation. Do not "harden" it
  by disabling raw HTML. The trust boundary is the docs root, which is operator-supplied content.
- `typographer` stays **off**: it mangles quotes and dashes in configuration values.
- Frontmatter is stripped by `gray-matter` and exposed as `meta`; it must never reach the rendered
  body.
- Only *relative* `.md` links are rewritten, resolved against the current document's directory and
  prefixed with the tenant segment. Anchors, absolute routes, schemes, protocol-relative URLs,
  non-`.md` targets and links escaping the tenant root are returned unchanged (`null`). Drift
  documents render with the `_drift` route base, so their links stay in the drift view; a link
  into the sibling `docs/` reaches the documentation route.

### Caching / freshness
- Regenerated documents must appear **without a restart**. Renders are cached by `mtimeMs` + `size`
  from a per-request `stat()`; discovery is cached with a short TTL. Any new cache must keep this
  property and stay bounded (the render cache evicts at `MAX_ENTRIES`).

### Configuration
- Environment variables only, read at their point of use: `DOCS_ROOT` (default `../output`,
  resolved against `process.cwd()`) and `PORT` (default `3000`). No config file, no new
  configuration mechanism.
- `views/` and `public/` are resolved from `process.cwd()`, so the server runs from this folder.
- `DOCS_ROOT` is the **only** coupling to the downloader. Keep it a plain path pointing at an export
  tree — never import from, shell out to, or depend on the Go project.

### Boundaries
- Controllers do HTTP concerns only (params, render, status); filesystem and Markdown logic lives in
  the services and the pure helpers.
- Keep `link-rewrite.ts` and `path-safety.ts` **pure/synchronous and Nest-free** so they stay unit
  testable without a module.
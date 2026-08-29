---
trigger: always_on
description: Architecture of the documentation browser (NestJS)
---

# Docs Browser — Architecture

This folder is a **self-contained project**: a read-only browser for the Markdown documentation
produced from `azure-resource-downloader` exports. It shares nothing with the Go CLI in the sibling
`go/` folder except the export tree on disk. The Go rules in `go/.windsurf/rules/` do not apply
here. `README.md` in this folder is the single source of truth (no further Markdown files besides
`NEXT-ITERATIONS.md`).

## Context
- **Stack**: Node >= 20, TypeScript (CommonJS output), NestJS 11 + Express, Handlebars (`hbs`),
  Tailwind CSS v4 + `@tailwindcss/typography`, `markdown-it` (+ `markdown-it-anchor`),
  `gray-matter`, Jest + supertest.
- **No client-side JavaScript.** Everything is server-rendered.

## Layout
- `src/main.ts` → bootstrap, reads `PORT`.
- `src/configure-app.ts` → `configureViews(app)`: static assets, base views dir, partial
  registration, view engine. **Shared by `main.ts` and the e2e tests** so both configure the app
  identically — new view/asset wiring goes here, never inline in `main.ts`.
- `src/dynamic-import.ts` → `dynamicImport`, a `new Function('return import(specifier)')` escape
  hatch. Required because TypeScript would down-level `import()` to `require()`, which cannot load
  ESM-only packages (`markdown-it-anchor` v9). Load ESM-only deps through it, never with `require`.
- `src/docs/` → the single feature module (`DocsModule`):
  - `docs.controller.ts` — routes, breadcrumb, 404 mapping.
  - `tenant-discovery.service.ts` — `DOCS_ROOT` scan + TTL cache.
  - `markdown-renderer.service.ts` — the one `markdown-it` instance + render cache.
  - `link-rewrite.ts` — pure functions (`rewriteHref`, `extractTitle`).
  - `path-safety.ts` — `resolveWithinTenant`, the security boundary.
- `views/` + `views/partials/` → Handlebars templates. `public/app.css` is generated and gitignored.
- `test/` → `*.spec.ts` only (Jest `testRegex`).

## Non-negotiables

### Read-only
- The app **never writes, moves or deletes anything** under the docs root, and never calls Azure.
  No route may mutate state. Adding a write path is a design change, not a feature.

### Path safety
- `resolveWithinTenant()` in `src/docs/path-safety.ts` is the **only** way a request-derived path may
  become a filesystem path. Never `path.join` user input and read it directly.
- Its guarantees must be preserved: reject null bytes / absolute paths / `..` segments up front,
  serve only `.md`, and re-verify containment **after** `realpath()` so symlinks cannot escape.
- Every change to that file needs a matching case in `test/path-safety.spec.ts`.
- Error responses must not leak absolute filesystem paths (asserted in the e2e suite).

### Tenant discovery
- A tenant is a directory containing **both** `index.md` and `.doc-manifest.json`. One marker is not
  enough.
- A matched tenant owns its whole subtree — do not descend into it looking for more tenants.
- Skip directories starting with `_` or `.` (housekeeping folders such as `_to_delete/`).
- Depth is bounded (`MAX_DEPTH`); keep it bounded.
- Counts shown to the user are **derived from the manifest** (sum of `types[*].resources`), never by
  walking the tree.
- A malformed/unreadable manifest makes the folder *not a tenant* — it must never crash discovery.

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
  non-`.md` targets and links escaping the tenant root are returned unchanged (`null`).

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

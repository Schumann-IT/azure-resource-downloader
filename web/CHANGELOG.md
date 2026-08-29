# Changelog

All notable changes to this project (the documentation browser in `web/`) are documented in this file.
Changes to the Go CLI live in [`../go/CHANGELOG.md`](../go/CHANGELOG.md).

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- **The project moved to `web/` and became self-contained.** The Go CLI now lives in the sibling `go/`
  folder and the shared export tree stays at the repository root, so the `DOCS_ROOT` default
  (`../output`, resolved against `process.cwd()`) still points at it. This app's architecture and
  style rules live in `web/.windsurf/rules/`; the Go rules in the parent repository do not apply
  here. `DOCS_ROOT` remains the **only** coupling to the downloader — nothing in `web/` imports from,
  shells out to, or otherwise depends on the Go project, so it can be split into its own repository
  without changes.

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

### Known limitations

Deliberate iteration-1 scope cuts are listed in [`NEXT-ITERATIONS.md`](NEXT-ITERATIONS.md): no
search, no syntax highlighting, single-segment tenant routes only, no manifest-driven navigation
tree, no explicit dark-mode toggle, no watch-based cache invalidation.

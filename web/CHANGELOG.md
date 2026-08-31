# Changelog

All notable changes to this project (the documentation browser in `web/`) are documented in this file.
Changes to the Go CLI live in [`../go/CHANGELOG.md`](../go/CHANGELOG.md).

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Breaking

- **A tenant is now a directory containing `docs/index.yaml`; `index.md` and `.doc-manifest.json` are
  gone.** The CLI stopped generating both: `azure-rd docs generate-index` writes a machine-readable
  navigation index instead, and the manifest was retired because `resources/metadata.yaml` plus the
  documents' frontmatter already carry the generation state it used to hold. Discovery is keyed off
  that file, and a tenant's document root is now `<export>/docs` rather than the export folder — which
  is what the relative `../<type>/<name>.md` links inside the documents were always relative to, so
  cross-document links resolve without a rewrite change. An export that has not had
  `docs generate-index` run for it is not discovered. The tenant-discovery invariants are preserved:
  a matched tenant still owns its whole subtree, `_`/`.`-prefixed directories are still skipped, depth
  is still bounded, and counts are still derived from the index rather than by walking the tree. A
  malformed or non-`version: 1` index makes the folder *not a tenant* instead of crashing discovery.

### Added

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

### Changed

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

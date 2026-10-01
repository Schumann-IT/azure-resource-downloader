# azure-rd-docs-web — documentation browser

A **self-contained, read-only** NestJS browser for the Markdown documentation produced from
`azure-resource-downloader` exports. `README.md` in this folder is the single source of truth (routes,
environment variables, the docs-root contract, layout); update it in the same change. Workflow, gates and
repo-wide rules: `../CLAUDE.md`. Style, testing and lint detail loads automatically from
`../.claude/rules/web-style.md`. The Go rules in `../go` do not apply here, and the Go `Makefile` is never
used from this folder.

## Context

- Node >= 20, TypeScript (CommonJS output, `strictNullChecks` on, `noImplicitAny` off), NestJS 11 + Express,
  Handlebars (`hbs`), Tailwind CSS v4 + `@tailwindcss/typography`, `markdown-it` (+ `markdown-it-anchor`),
  `shiki`, `gray-matter`, `js-yaml`, `diff`, `yazl`, `pdfmake`, Jest + supertest.
- **No client-side JavaScript.** Everything is server-rendered; the CSP names no `script-src`, so no script
  can run. State lives in the URL. Dropping this rule is a parked idea in `NEXT-ITERATIONS.md`, not a
  decision you may take.
- Version is `version` in `package.json`, bumped only at release; releases are tagged `web/vX.Y.Z`.

## Layout

- `src/main.ts` bootstrap (reads `PORT` via `src/port.ts`); `src/configure-app.ts` → `configureViews(app)`:
  security headers, static assets, views dir, partials, view engine — **shared by `main.ts` and the e2e
  tests**, so new view/asset wiring goes there, never inline in `main.ts`.
- `src/dynamic-import.ts` — the `new Function('return import(...)')` escape hatch for ESM-only deps
  (`markdown-it-anchor` v9, `shiki`). Load ESM-only packages through it, never with `require`.
- `src/docs/` — the single feature module: `docs.controller.ts` (routes, breadcrumb, 404 mapping);
  `tenant-discovery.service.ts` (`DOCS_ROOT` scan, 30 s TTL cache, index cache); `tenant-index.ts` (pure:
  `parseTenantIndex`, `buildNavigation`, facet filters); `markdown-renderer.service.ts` (the one `markdown-it`
  instance + mtime render cache); `yaml-highlighter.service.ts` (the one `shiki` highlighter + cache);
  `link-rewrite.ts`, `section-hooks.ts`, `findings-table.ts` (pure); `path-safety.ts` (**the security
  boundary**); `drift-observation.ts`, `drift-audit.ts`, `drift.service.ts`, `drift-view.ts`, `yaml-diff.ts`,
  `file-cache.ts`, `resources-metadata.ts`; `compare-normalise.ts` (provisional cross-tenant identity rule),
  `compare-view.ts`, `compare.service.ts`; `drift-report.service.ts` (drift reports shared by the drift pages and
  the PDF); `export/` (Confluence zip and drift report PDF: `export.service.ts`, `confluence.ts`,
  `export-index-mode.ts`, `html-allowlist.ts`, `page-name.ts`, `drift-pdf.ts`, `pdf-content.ts`).
- `views/` + `views/partials/` Handlebars templates; `public/` (`favicon.svg`; `app.css` is generated and
  gitignored); `test/` `*.spec.ts` only; `scripts/` readiness reports; `eslint.config.mjs` (single lint truth);
  `eslint-suppressions.json` (debt ledger); `.env.example`. `scripts/lib/git.js` is the one place the tooling
  shells out (read-only git); every other script goes through it.

## Commands — always the npm scripts, from this folder

`npm install` · `npm run build` (CSS + `nest build`) · `npm run start:dev` (Tailwind watch + Nest watch) ·
`npm run start:prod` · `npm test` (Jest with `--experimental-vm-modules`) · `npm run lint` (reports; in both
gates) · `npm run lint:fix`, `npm run lint:baseline`, `npm run lint:prune` (rewrite files; in no gate) ·
`npm run start-item -- <n>` (gate before implementing entry n) · `npm run branch-ready` (gate) ·
`npm run release-ready` (report). No Prettier exists; do not reference it. Workflow procedures:
`/promote-idea`, `/implement-item`, `/implement-pair` (a go/web pair through the agent pipeline),
`/item-done`, `/close-branch`; finished entries live in `../.claude/archive/web/` (`/archive`).
The server resolves `views/` and `public/` from `process.cwd()`, so it runs from `web/`.

## Non-negotiables

- **Read-only.** No route writes, moves or deletes anything under the docs root, and nothing calls Azure.
  A write path is a design change, not a feature.
- **Path safety.** `resolveWithinRoot()` in `src/docs/path-safety.ts` is the *only* way a request-derived path
  becomes a filesystem path — via `resolveWithinTenant()` (documents), `resolveResource()` (source YAML),
  `resolveDriftDocument()` / `resolveDriftPayload()` (drift tree). It rejects null bytes, absolute paths and
  `..` up front, serves exactly **one** extension per resolver (`.md` under `docs/`, `.yaml` under
  `resources/`; drift: `.md` documents, `.yaml` payloads; `.yml` never), re-verifies containment **after**
  `realpath()`, and both drift resolvers require at least two segments so the tree root is unreachable. Never
  widen an extension to a list; never `path.join` user input and read it. Every change there needs a case in
  `test/path-safety.spec.ts`. Error responses never leak absolute paths, stack traces or exception messages.
- **Tenant discovery.** A tenant is a directory with a readable `docs/index.yaml` of integer `version >= 1`
  (CLI writes v4; v2 still understood). Document root is `<export>/docs`; `<export>/resources` is a second,
  separate served root and **not** a discovery marker; `<export>/drift` is a third, ephemeral root and not a
  marker either. `docs/generate.md` and `drift/analyze.md` are tool input, never served; `drift/audit.yaml`
  (the CLI's attribution) is read as data, never served. Skip `_`- and `.`-prefixed directories; depth stays
  bounded (`MAX_DEPTH`); a matched tenant owns its subtree. A malformed index makes the folder *not a tenant*,
  never a crash. Counts and listings are read as data (index `counts.*`; compare: `resources/metadata.yaml`),
  never by walking the tree.
- **Source YAML view.** One `shiki` highlighter, built in `onModuleInit`, loaded through `dynamicImport`;
  a load failure or oversize file degrades to an escaped `<pre>`. A document's source is found by inverting the
  CLI mapping (`docs/<type>/<name>.md` ↔ `resources/<type>/<name>.yaml`); `source` frontmatter is a label,
  never a path input. `_resource`, `_drift`, `_export` are representation prefixes (out of the breadcrumb,
  declared before the `:tenant/*path` catch-all); `_compare` is the root-level counterpart.
- **Drift view.** The comparison is the CLI's job; the app only renders `drift/`. The Drift button and page read
  **one** decision (`driftState`/`tenantDriftState`); the button is always rendered and inert only with the
  reason as `title`. Validity gate: observation baseline vs `resources/metadata.yaml`'s `generatedAt`. A
  comparison or payload is shown only after its hashes match the observation. `raw`, `yaml`, `diff` are
  reserved query parameters. The YAML diff is plain data escaped by the template, never trusted HTML.
- **Tenant compare** is a proof of concept. `compare-normalise.ts` is provisional and destined for the CLI
  (`resource compare`, parked in `../go/NEXT-ITERATIONS.md`): keep it data, applied identically to both
  sides, announced in the caption and `&raw`, and mirror any change into that Go idea. Rows come from the two
  `resources/metadata.yaml` files only; status is cached per file (hashes, never text), decided via
  `pairStatus` from two digests; both files located only through `resolveResource`. Diff hunks render through
  the shared `diff_table` partial; the pane is a CSS grid of `<a>` rows, never a `<table>`.
- **Rendering.** One `markdown-it` instance, built in `onModuleInit`; `html: true` is **required** (the
  `<details>` blocks *are* the documentation; the trust boundary is the operator-supplied docs root);
  `typographer` stays off; frontmatter is stripped by `gray-matter` and never reaches the body; only relative
  `.md` links are rewritten (drift documents use the `_drift` route base).
- **Caching / freshness.** Regenerated files appear **without a restart**: renders are cached by `mtimeMs +
  size` from a per-request `stat()`, discovery by a short TTL. Any new cache keeps that property and stays
  bounded.
- **Configuration.** Environment variables only, read at their point of use: `DOCS_ROOT` (default `../output`),
  `PORT` (default `3000`), `EXPORT_INDEX`. No config file, no `dotenv`, no new mechanism.
- **Boundaries.** Controllers do HTTP only; filesystem and Markdown logic live in services and pure helpers.
  `link-rewrite.ts` and `path-safety.ts` stay pure, synchronous and Nest-free.
- Security headers (`nosniff`, `Referrer-Policy`, `X-Frame-Options: DENY`, the CSP) are set once in
  `configureViews()`; widening any directive is a changelog-worthy decision.

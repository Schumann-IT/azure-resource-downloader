# Next iterations — deliberately out of scope for iteration 1

Iteration 1 only discovers tenants and renders their Markdown as HTML. Things noticed while building it,
left out on purpose:

- **Search.** Full-text or type/name filtering across a tenant's documents.
- **Syntax highlighting.** `@shikijs/markdown-it` for the `yaml`/`bash`/`powershell`/`json`/`xml` fences.
  Left off in iteration 1 (optional per the prompt); it is an ESM-only add wired the same way as
  `markdown-it-anchor`.
- **Multi-segment tenants.** Discovery walks up to 3 levels, but routing uses a single `:tenant` segment,
  so only top-level tenant folders are addressable. Nested tenants would need a different route shape.
- **Group-driven navigation.** `docs/index.yaml` can carry `platformGroup`/`functionGroup` per resource,
  but the documents do not yet write them (the generation template's required frontmatter does not ask
  for them), so the landing page groups by resource type and shows the groups as badges when present.
  Once the template requires them, the tree (and the sidebar planned below) can become platform → function.
- **Summary table of contents.** `markdown-it-anchor` already gives the summary's `##` headings ids; a
  jump list (Management summary / At a glance / Assignment posture / Coverage caveats) is not built.
- **Explicit dark-mode toggle.** Currently follows `prefers-color-scheme` only.
- **Watch-based cache invalidation.** The cache is mtime-checked per request; an `fs.watch` layer could
  pre-warm/evict, but per-request `stat()` already gives no-restart refresh.

---

# Planned — summary landing page + sidebar navigation

The generation prompt (`docs/generate.md`, section 7) now writes a second agent-owned file at the docs
root: `docs/summary.md`, a tenant-wide management summary. It is meant to be the tenant's landing page.
The index-driven list of every document moves out of the page body and into a persistent sidebar.

## Findings from the export review (constraints these tasks must respect)

- **`docs/summary.md` is optional.** It is written by the agent, not by the CLI, so an export can have a
  valid `docs/index.yaml` and no summary (older exports, aborted runs). `index.yaml` stays the tenant
  marker; a missing summary must degrade to the current index-driven landing page, never a 404.
- **Its links already resolve.** Links inside it are relative to `docs/` (`Microsoft.Graph/<type>/<x>.md`),
  which is exactly what `rewriteHref` produces with `docDir: ''`. No change to `link-rewrite.ts`.
- **It owns its own `<h1>`** (`# <tenant> — Intune and Entra configuration`) and carries **no frontmatter**.
  The landing page must not print a second heading above it, and `meta.source` will be empty.
- **It is currently reachable at `/:tenant/summary`.** Once it is the landing body that route is a
  duplicate — decide explicitly (redirect to `/:tenant`, or keep it as a permalink).
- **It is regenerated on every run** (section 7 is unconditional, even for an empty work list), so it must
  obey the no-restart freshness rule — render it through `MarkdownRendererService`, which is already keyed
  on `mtimeMs` + `size`.
- **Sidebar size is real but bounded.** The reference export lists 263 in-scope resources across 29 types
  (groups and Autopilot identities are `counts.excluded`, 491 + 272). A flat list is unusable; grouping
  must collapse, and with no client-side JS that means `<details>`/`<summary>` per type, with the current
  document's type open.
- **`platformGroup`/`functionGroup` are still empty** in the emitted index, so the sidebar groups by type,
  same as today's landing sections.
- **The scratch tree is outside the served root.** The agent's `chunks/` folder (`_common.md`, `NN.md`,
  `*.py`, `mtimes.json`) sits at the *export* root, not under `docs/`, so it is already unreachable.
  The tool-owned files at the docs root remain `generate.md` (blocked), `index.yaml` (not `.md`, blocked
  by `path-safety`) and now `summary.md` (deliberately served).

## Tasks

1. **Expose the summary from discovery.** Add a `summaryPath` to `TenantInfo` in
   `tenant-discovery.service.ts` (`<export>/docs/summary.md`) plus a cheap existence check. Do not make it
   a discovery requirement and do not walk the tree for it.
2. **Render it on `GET /:tenant`.** The controller renders `summary.md` via `MarkdownRendererService` with
   `{ tenant, docDir: '' }` and passes the HTML to the tenant view. On a missing/unreadable summary fall
   back to the existing index-driven sections. Keep controller logic to HTTP concerns only.
3. **Decide `/:tenant/summary`.** Either 302 to `/:tenant` or leave it as a permalink; whichever is chosen,
   assert it in the e2e suite so the behaviour is intentional. Do not add it to `TOOL_ARTIFACTS`.
4. **Extract a `sidebar` partial.** Move the grouped navigation out of `tenant.hbs` into
   `views/partials/sidebar.hbs`, driven by the existing `buildNavigation()` output — one `<details>` per
   type, with the item count in the `<summary>` and the current document's type `open`.
5. **Give `buildNavigation()` a current-document notion.** Extend it (or add a thin helper in
   `tenant-index.ts`, kept pure and Nest-free) to mark the active item/section from the request path, so
   the sidebar can highlight it and open the right group. Cover it in `test/tenant-index.spec.ts`.
6. **Put the sidebar on the document pages too.** `page.hbs` and `tenant.hbs` share a two-column layout
   (sidebar + content). The document route must therefore load the tenant index — it already resolves the
   tenant, so this is one `getIndex()` call, served from the mtime-keyed index cache.
7. **Keep the tenant metadata visible.** Counts, `generatedAt`, the incomplete-export banner and the
   excluded-types line currently live in `tenant.hbs` above the sections; with the summary owning the body
   they belong in the sidebar header (or a strip above it) so they survive on document pages.
8. **No client-side JS, responsive.** The sidebar collapses into a single `<details>` above the content on
   narrow viewports; every state needs a dark variant and a visible `:focus-visible` outline. Any new
   `<details>` styling goes in `src/styles.css` with a case in `test/styles-build.spec.ts`.
9. **Tests.** Extend `test/docs.e2e.spec.ts`: landing page renders the summary body (fixture with an H1 and
   a relative `Type/x.md` link that must come out as `/tenant/Type/x`), landing page falls back to the
   index list when `summary.md` is absent, an edited `summary.md` is reflected on the next request, the
   sidebar appears on a document page with the active item marked, and `generate.md` is still 404.
10. **Docs.** Update `README.md` (docs-root contract now names `summary.md` as an agent-written, served
    file; describe the landing page and sidebar) and add a `CHANGELOG.md` entry under `## [Unreleased]`
    stating how the no-restart freshness and read-only invariants are preserved.

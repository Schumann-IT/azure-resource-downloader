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
- **Sidebar refinements.** The sidebar lists every in-scope document (263 in the reference export) with
  no filter and no item summaries/badges — those are only on the index-listing fallback. It also does not
  remember which sections were open across navigations (that would need client-side state).

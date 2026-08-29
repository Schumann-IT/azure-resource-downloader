# Next iterations — deliberately out of scope for iteration 1

Iteration 1 only discovers tenants and renders their Markdown as HTML. Things noticed while building it,
left out on purpose:

- **Search.** Full-text or type/name filtering across a tenant's documents.
- **Syntax highlighting.** `@shikijs/markdown-it` for the `yaml`/`bash`/`powershell`/`json`/`xml` fences.
  Left off in iteration 1 (optional per the prompt); it is an ESM-only add wired the same way as
  `markdown-it-anchor`.
- **Multi-segment tenants.** Discovery walks up to 3 levels, but routing uses a single `:tenant` segment,
  so only top-level tenant folders are addressable. Nested tenants would need a different route shape.
- **Manifest-driven navigation.** A left-hand tree from `.doc-manifest.json` (types → resources) instead
  of relying solely on `index.md` links.
- **Explicit dark-mode toggle.** Currently follows `prefers-color-scheme` only.
- **Watch-based cache invalidation.** The cache is mtime-checked per request; an `fs.watch` layer could
  pre-warm/evict, but per-request `stat()` already gives no-restart refresh.

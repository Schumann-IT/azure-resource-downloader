# Next iterations — deliberately out of scope for iteration 1

Iteration 1 only discovers tenants and renders their Markdown as HTML. Things noticed while building it,
left out on purpose:

- **Search.** Full-text or type/name filtering across a tenant's documents.
- **Syntax highlighting inside documents.** `shiki` now highlights the source-YAML view, but the
  `yaml`/`bash`/`powershell`/`json`/`xml` fences *inside* the generated Markdown are still unstyled.
  `@shikijs/markdown-it` could reuse the highlighter `YamlHighlighterService` already owns (it would need
  the extra languages loaded).
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

## Resource (YAML) rendering — shipped as option A

Implemented: `<export>/resources` is a second read-only served root, `GET /:tenant/_resource/*path`
renders the source YAML (shiki, `#L42` line anchors, `?raw` plain text, escaped `<pre>` above 512 KB), and
a **Documentation | YAML** switcher in the top bar flips between the two representations — suppressed when
a document has no `source` frontmatter. Only resources listed in `docs/index.yaml` are reachable; a
document's source is found by inverting the CLI's `docs/<type>/<name>.md` ↔ `resources/<type>/<name>.yaml`
mapping, so no directory is walked. See `CHANGELOG.md` for the invariants and the README for the routes.

The navigation alternatives considered alongside option A are kept here as possible follow-ups:

- **Option B — switcher plus a resource landing page.** Sidebar untouched, no extra section. A new
  `GET /:tenant/_resource` lists every source YAML (index-derived, plus the excluded types), linked once
  from the sidebar footer. Keeps the tree lean and gives the bulk types a real home, at the cost of one
  more route and view. Natural follow-up to A: it is where the bulk types (and unreferenced groups) would
  get a real home.
- **Option C — switcher plus a two-group tree.** `buildNavigation` returns two top-level `NavGroup`s
  (*Documentation*, *Resources*), each holding the existing per-type `NavSection[]`; only the group
  containing the current page renders `open`, and the active marker becomes
  `{ kind: 'doc' | 'resource', path }`. Both trees are built in one index pass with two href shapes, so
  they cannot drift. Most navigable and most explicit, but the resource tree duplicates the doc tree
  (263 items twice in the reference export) and `sidebar.hbs` gains a second `<details>` level (plus a
  `.nav-tree details details` style). Only worth doing if the switcher turns out to be too hard to find.
- **Option D — clickable breadcrumb segments (additive; last, if at all).** Turn
  `Microsoft.Graph / depOnboardingSettings` in the breadcrumb into links to a per-type listing page. Needs
  a new route and view: CSS alone cannot open a collapsed `<details>` section from an anchor, so linking
  into the existing sidebar tree is not an option without client-side state.

**Excluded bulk types — deliberately not browsable.** Types the index merely counts under
`counts.excluded` (Autopilot identities and the like), and any resource with no document at all, have no
YAML view. That is what keeps navigation purely index-derived and left the **"counts and listings derive
from the index, never from walking the tree"** non-negotiable untouched.

If they are wanted later (option B is the natural carrier), the shape is settled: for each type named in
`counts.excluded`, a single **non-recursive** `readdir` of `resources/<type>/` (`.yaml` only, sorted, cached
under the existing discovery TTL, unreadable ⇒ empty), producing file names only — counts shown to the user
would still come from the index. That change *does* need the rule amended, and it reopens two UX questions:
whether to flag a `readdir`-vs-`counts.excluded` mismatch as a stale index, and whether resources without
documentation should be visually de-emphasised. Unreferenced `Microsoft.Graph/groups` (the CLI documents a
group only when an assignment references it) fall in the same bucket.

Also left out: any search or filter over the resource tree, and highlighting the code fences inside the
generated Markdown (see the syntax-highlighting item at the top of this file).

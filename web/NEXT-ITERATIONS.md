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

## Planned: resource (YAML) rendering

Design agreed, not yet implemented. Goal: make the exported source YAML browsable next to the generated
documentation, as a syntax-highlighted read-only view. **Navigation shape: option A**, the smallest
additive slice; B/C/D below stay documented as possible follow-ups, not as pending decisions.

**Architectural consequence, decided deliberately:** `<export>/resources` becomes a *second* read-only
served root per tenant. Today it is explicitly outside the served tree (README's docs-root layout and a
non-negotiable in `.windsurf/rules/01-architecture.md`). That invariant is replaced by an equally tight
one — a single parameterized path guard per root — and both documents must be updated in the same change.
Nothing about read-only changes: no route mutates anything.

- **Boundary.** Extract `resolveWithinRoot(rootDir, relPath, ext)` from `resolveWithinTenant()`, keeping
  every current guarantee verbatim (null-byte / absolute / `..` pre-checks, realpath containment re-check)
  and adding a single allowed extension. `resolveWithinTenant()` stays as the `.md` wrapper, so no call
  site changes; resources resolve with `('.yaml')` — exports only ever write `.yaml`, so the allow-list
  stays exactly one extension per root and `.yml` is deliberately not served. A `.md` can then never be
  served from `resources/`, nor a `.yaml` from `docs/`, and `..` cannot cross between the two roots.
- **Discovery.** `TenantInfo` gains `resourcesDir` = `<export>/resources`. It is *not* a discovery marker
  and its existence is checked per request, so an export without a `resources/` folder (docs copied
  standalone) stays a valid tenant whose YAML views simply 404.
- **Routes.** `GET /:tenant/_resource/*path` renders the YAML view (`.yaml` suffix optional, like
  documents); `?raw` serves the file as `text/plain; charset=utf-8` with `X-Content-Type-Options: nosniff`
  for copy-paste. Declared before the `:tenant/*path` catch-all; `_resource` cannot collide because no
  Azure/Graph type segment starts with `_`.
- **Highlighting.** One `shiki` highlighter, created in `onModuleInit` of a new `YamlHighlighterService`
  and loaded through `dynamicImport` (shiki is ESM-only — same reason as `markdown-it-anchor`), `yaml`
  language only, dual light/dark theme so `prefers-color-scheme` still works with no client-side
  JavaScript. Highlighted HTML is cached by `mtimeMs` + `size` and bounded, exactly like the Markdown
  render cache, so a re-downloaded resource shows up without a restart. **Above ~512 KB** the view skips
  highlighting and emits an escaped `<pre>` instead, rather than stalling a request on a huge payload (the
  cap is a starting value, revisit if a real export trips it).
- **Line numbers and `#L42` deep links.** A shiki line transformer gives every line span an `id="L<n>"` and
  prepends the number as an `<a href="#L42">` gutter link, so a specific line can be linked, copied from the
  address bar and highlighted purely with `.line:target` in CSS — still no client-side JavaScript. The
  plain-`<pre>` fallback above the size cap has no line ids; that is accepted.
- **Source paths come from the index**, not from walking the tree: `doc: <type>/<name>.md` maps to
  `/:tenant/_resource/<type>/<name>`, i.e. `resources/<type>/<name>.yaml`. That mirroring is **confirmed**:
  the CLI derives each document path from its source key by swapping the `resources` root for `docs` and
  `.yaml` for `.md`, so inverting it is exact, not a heuristic. No per-item `stat()` (263 items per page); a
  renamed or missing source file is an honest 404 on click. A `source:` field emitted by
  `azure-rd docs generate-index` would remove even the inversion, but that is a Go-side change and out of
  scope here.
- **Controller.** The resource route renders `views/resource.hbs` with the same breadcrumb as its document
  (`_resource` stripped before the breadcrumb is built, since it is a representation, not a segment), the
  document title where available, and the existing 404 mapping — `error.hbs` gets the requested
  *route* path, never a filesystem path.
- **Views.** New `views/resource.hbs` (outside `.prose`, since typography fights a `<pre>`), showing the
  relative path, size, a link back to the document and the raw link. `page.hbs` turns the existing
  `meta.source` line into a **link to the YAML view**; the href is built from the document's own path (the
  exact inversion above), *not* by parsing `meta.source` — so it works whether the frontmatter carries a bare
  `p1.yaml` or a full `resources/<type>/<name>.yaml`, which stays purely a label. `src/styles.css` gains the
  shiki dark-mode variables plus the line-number gutter and `:target` line highlight.
- **Tests.** `path-safety.spec.ts`: extension allow-list, cross-root rejection, traversal, symlink escape,
  null byte. `tenant-index.spec.ts`: the derived resource href.
  `docs.e2e.spec.ts`: the resource view, per-line `id="L<n>"` anchors, the `?raw` content type, the top-bar
  switcher on both the document and the resource page, the switcher's YAML entry *absent* for a document
  without `meta.source`, 404 without a leaked path, an edited YAML reflected on the next request, traversal
  into `../docs`. `styles-build.spec.ts`: the shiki, gutter and `:target` rules survive compilation.
- **Also part of the change:** `shiki` pinned in `package.json` (grammar/theme loading happens once in
  `onModuleInit`; Jest already runs with `--experimental-vm-modules`, so the ESM import needs no new
  wiring), the README (routes table, docs-root layout — the "never read by this app" note on `resources/`
  has to go, security section, project layout), `.windsurf/rules/01-architecture.md` (the second served
  root and the parameterized guard) and a `CHANGELOG.md` entry under `## [Unreleased] / ### Added` stating
  how read-only, path safety and no-restart freshness are preserved.

### Navigation shape — option A first, B/C/D as follow-ups

All four share the boundary, routes, highlighting and views above; they differ only in how a user *reaches*
a YAML view and where resources without a document live.

**Common building block (A–C all use it): a top-bar view switcher.** A document and its source share every
breadcrumb segment — `/:tenant/Microsoft.Graph/depOnboardingSettings/cb_production` and
`/:tenant/_resource/Microsoft.Graph/depOnboardingSettings/cb_production` — so `_resource` is a
*representation*, not a path segment, and never appears in the breadcrumb. Instead `header.hbs` gets a
two-item switcher (**Documentation | YAML**, current one marked `aria-current="page"`), built from the
extensionless path both routes already compute:

```ts
views: [
  { label: 'Documentation', href: `/${tenant}/${docPath}`,            active: kind === 'doc' },
  { label: 'YAML',          href: `/${tenant}/_resource/${docPath}`,  active: kind === 'resource' },
]
```

No new source of truth, no JavaScript. **The YAML entry is suppressed when the document has no
`meta.source` frontmatter** — a free signal from the already-parsed frontmatter, no extra `stat()`, and it
keeps the switcher from advertising a link that would 404. On a resource page both entries always render
(a YAML view is only ever reached from its document in this slice).

- **Option A — switcher, no Resources group. CHOSEN as the first slice.** Sidebar stays exactly as today
  (no `NavGroup` refactor). Documented resources are reached only from the top bar and from the
  `meta.source` link on the document page. Smallest, purely additive diff: every risky piece (the second
  served root, the guard refactor, shiki) is exercised once, in isolation from the navigation question, and
  nothing in the sidebar or the "never walk the tree" rule has to move. The trade-off accepted: discovering
  the YAML view depends on the user noticing the switcher.
  Only resources the index lists are browsable — see *Excluded bulk types* below.
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
- **Option D — clickable breadcrumb segments (additive to A, B or C; last, if at all).** Turn
  `Microsoft.Graph / depOnboardingSettings` in the breadcrumb into links to a per-type listing page. Needs
  a new route and view: CSS alone cannot open a collapsed `<details>` section from an anchor, so linking
  into the existing sidebar tree is not an option without client-side state.

**Excluded bulk types — out of scope for now (decided).** Only resources listed in `docs/index.yaml` get a
YAML view; types that the index merely counts under `counts.excluded` (Autopilot identities and the like),
and any resource with no document at all, stay unbrowsable. The upside is that the navigation stays purely
index-derived and the **"counts and listings derive from the index, never from walking the tree"**
non-negotiable needs no amendment at all.

If they are wanted later (option B is the natural carrier), the shape is settled: for each type named in
`counts.excluded`, a single **non-recursive** `readdir` of `resources/<type>/` (`.yaml` only, sorted, cached
under the existing discovery TTL, unreadable ⇒ empty), producing file names only — counts shown to the user
would still come from the index. That change *does* need the rule amended, and it reopens two UX questions:
whether to flag a `readdir`-vs-`counts.excluded` mismatch as a stale index, and whether resources without
documentation should be visually de-emphasised. Unreferenced `Microsoft.Graph/groups` (the CLI documents a
group only when an assignment references it) fall in the same bucket.

Still deliberately out of scope for this feature: highlighting the `yaml`/`bash`/`json` fences *inside* the
generated Markdown (the same shiki instance could later back `@shikijs/markdown-it`), and any search or
filter over the resource tree.

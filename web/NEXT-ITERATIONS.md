# Next iterations

Outstanding work, standing decisions and parked ideas for the docs browser. `README.md` describes what it
does today and `CHANGELOG.md` records what shipped; neither is repeated here.

## 1. Per-document identity on the article

**Goal.** Let a reader tell what *kind* of resource a page describes before reading it, and let the
stylesheet treat a group, a credential and a settings-catalog policy differently. `<article>` currently
carries no per-document hook, so all six document families render identically.

> **`data-family` cannot come from `docs/index.yaml`.** `IndexResource` carries
> `type/doc/displayName/summary/documented/scope/platformGroup/functionGroup/odataType/platforms/assignments`
> and no family or prompt-template field; the frontmatter carries none either. The document's **own H2 set**
> is the carrier, and in the reference exports it partitions cleanly — 340 documents with
> *References | Lifecycle and operations | Security | Settings* (the `default`/`singleton` contract, which
> share one heading list and so cannot be told apart), 36 `group`, 25 `referenced`, 6 `record`, 4
> `credential`, 0 `arm`. Match the set, ignoring the spliced `Targeted by` / `Used by` headings, and emit
> nothing when it matches no contract. Do not infer the family from the resource type, and do not add a
> seventh vocabulary here — if a family needs to be distinguishable beyond its headings, ask the CLI for the
> field.
>
> **`data-type` is free**: it is the document's own directory (`Microsoft.Graph/groups`), already known to the
> controller.
>
> **What it buys**, and the reason it is worth doing at all rather than being merely available: a credential
> document can lead with its expiry, a `record` document can drop the assignment vocabulary it never uses,
> and `[data-family="group"] [data-section="properties"]` can differ from the same section on a policy — none
> of which is expressible today.

**Plan.**

- **Derive the family from the observed H2 slugs** in a pure helper next to `section-hooks.ts`, with a case
  per contract in a unit spec, including a document whose set matches none.
- **Expose `family` and `type` from the render** and put `data-family` / `data-type` on `<article>` in
  `page.hbs`, with a case in `test/docs.e2e.spec.ts`.
- **Only then** add per-family CSS, so the attribute does not ship as decoration.

## 2. Findings block beyond the severity icon

**Goal.** Let a reader of the tenant landing page act on the findings table — narrow it to what matters and
get from a finding to the documents it affects.

> The hooks are in place — `src/docs/findings-table.ts` tags the table `.findings` and puts `data-severity`
> on each body row and severity cell — but nothing consumes `data-severity` beyond the severity icon, the
> `Affected` count is inert, and `#findings` is an unstyled H3 above a styled table.
>
> **Caveat.** `.prose table` sets `white-space: nowrap` so wide assignment tables scroll instead of wrapping
> mid-GUID. Any prose-bearing table needs an opt-out from it.
>
> **The filter mechanism is expressible without JavaScript**, which is what makes this entry worth doing:
> renderer-emitted sibling anchors plus `:target`, e.g.
> `#sev-critical:target ~ .findings tbody tr:not([data-severity="critical"]) { display: none }`. It needs the
> anchors to be siblings of the table, so the wrapper below is a prerequisite, and it spends the URL fragment
> — which already belongs to `#findings` — so a **Show all** reset link is part of the feature, not a
> refinement.

**Plan.**

- Put `data-severity` on a wrapper emitted next to the table, with one anchor per severity and a **Show all**
  reset, so the `:target` filtering above and a per-severity count line become expressible.
- Link the `Affected` count to the documents the row names, reusing the same relative-link rewriting as
  document bodies — or drop the count, since the `Documents` column already links.
- Style `#findings` itself, with the risk-role icon and colour the Security section uses.

## 3. Summary table of contents

**Goal.** Give the tenant landing page a jump list so a reader can go straight to the part of the summary
they came for instead of scrolling.

> The four summary H2s are a declared contract on the CLI side with stable, clean slugs —
> `#management-summary`, `#at-a-glance`, `#assignment-posture`, `#coverage-caveats` — so they are safe to
> hard-code, and `markdown-it-anchor` emits the ids without further work.
>
> The landing page's structure is tight (a heading followed by one list or a short run of paragraphs), so
> positional selectors that would be fragile in a resource document hold up here (`#recommendations + ol`,
> `[id="at-a-glance"] ~ p`) and no renderer change is needed.

**Plan.**

- Render the jump list in `tenant.hbs` from the known heading set, as a labelled `<nav>`, skipping entries
  whose heading is absent so an older summary degrades to a shorter list, and omitting it entirely in the
  index-listing branch where there is no summary at all.
- Nest `#findings` and `#recommendations` under *Management summary* — they are the entries a reader actually
  jumps to — reusing the same closed H3 vocabulary the CLI declares.
- Assert in `test/docs.e2e.spec.ts` that the list links to the ids the rendered summary actually contains.

## 4. Sidebar refinements

**Goal.** Make the sidebar usable at export scale, where it lists every in-scope document (263 in the
reference export) as a flat per-type tree with no way to narrow it and no context per item.

> The item summaries and badges the index carries are rendered only by the listing fallback, not by the tree.
>
> **Excluded on purpose:** remembering which sections were open across navigations. That needs client-side
> state, and no client-side JavaScript is a non-negotiable.

**Plan.**

- A filter over the tree that works without JavaScript (server-side query parameter narrowing the rendered
  items, with the current filter reflected in the URL). This is a *name* filter, distinct from the shipped
  `?programme=` facet, and must compose with it rather than replace it.
- Per-item summary/badge rendering in `sidebar.hbs`, from the same `docs/index.yaml` fields the listing
  fallback reads.

## 5. Structure the sidebar by platform and function instead of by resource type

**Goal.** Let a reader reach a document by what it *is* — a Windows security policy, a macOS configuration —
rather than by the Azure/Graph type that happens to implement it. The type tree is the vendor's taxonomy: an
administrator looking for "how are Macs hardened" has to know the answer is spread over
`deviceConfigurations`, `deviceManagementConfigurationPolicies` and `deviceShellScripts` before they can
start. The programme filter already narrows the tree by initiative; this replaces the tree's *structure*,
which the filter deliberately left alone.

> **Blocked on data, not on design.** `platformGroup`/`functionGroup` are populated on **0** of 263 resources
> in both exports in `../output`, and 0 documents on disk carry them in frontmatter. The producer side has
> shipped — every non-`record` type's `doc-prompt.md` now carries a `<!-- doc-groups: … -->` marker, the
> generation template requires and validates both fields, and `docs generate-index` already harvests them —
> but the exports in `../output` predate the marker, and it moves every affected type's `promptSha256`, so
> the fields only appear after a **full documentation regeneration** (`azure-rd docs generate-prompt`, the doc
> pass, then `generate-index`). Do not start here before those fields are non-empty: grouping by fields empty
> everywhere yields one *Ungrouped* section, which is worse than grouping by type.
>
> **What is already in place.** The axis vocabularies travel in the index header — `vocabularies.platform`
> (`Windows, macOS, iOS/iPadOS, Android, Linux, Cross-platform, n/a`) and `vocabularies.function`
> (`Identity & access, Compliance, Configuration, Security, Apps, …`), always emitted, in display order — so
> the group ordering is read from the data and cannot drift from a copy kept here. The reserved **`n/a`**
> value distinguishes *this axis does not apply to this type* (a tenant-level singleton has no platform) from
> *unclassified*, so the fallback bucket cannot merge a real gap with a category error. `parseTenantIndex()`
> already reads both, and the filter machinery (`buildNavigation`'s pure filtering, the URL-carried choice)
> is there to build on.
>
> **The web side must not compute the grouping.** It is tempting to derive it here from
> `odataType`/`platforms`/naming conventions — 93 of 225 named resources match
> `GBL_<kind>_<env>_<scope>_[CIS_]<platform>_<area>[_L1]`. But `src/docs/export/confluence.ts` is a second
> consumer of the same index, and anything derived inside `buildNavigation()` is invisible to it: an exported
> space would silently fall back to endpoint grouping while the browser showed something else.
>
> **What is cheap.** Hrefs come from the index `doc` field and are independent of how `buildNavigation()`
> groups, so **restructuring the sidebar changes no URLs** — no redirects, and no broken links into an
> already-imported Confluence space. `sidebar.hbs` renders `nav.sections` purely from data, so the spine is a
> data change plus one nesting level in the partial. Any switch between structures is a route or a query
> parameter reflected in the URL, never a widget: no client-side JavaScript.

**Plan.**

- **Group in a pure function** next to `buildNavigation()` in `src/docs/tenant-index.ts`, reading only what
  the index carries and deriving nothing, ordering groups by the header vocabularies rather than
  alphabetically, and putting resources with no group in a named bucket that is **always rendered** — a
  taxonomy that quietly stops matching must show up as a full bucket, never as a thinning tree. Cases in
  `test/tenant-index.spec.ts` for a fully grouped index, a partially grouped one, and an index with no
  grouping at all; the last must produce today's per-type tree unchanged.
- **Nest the spine in `sidebar.hbs`** as one more `<details>` level, and keep the active structure in the URL
  so the choice survives a reload without state. It must compose with the existing `?programme=` filter:
  the filter narrows whichever structure is active, and the two choices coexist in one URL.
- **Keep the endpoint type reachable** as an alternate structure once it is no longer the default, so an
  API-shaped question is still answerable.
- **Drop the `platformGroup`/`functionGroup` badges** from the listing once the same values drive the tree,
  so a resource does not state its group twice on one screen. The programme badges stay — they are the other
  dimension.
- **Verify both consumers agree** before calling it done: the Confluence export groups its overview page
  from the same fields, so a grouped browser and an endpoint-grouped export means the grouping leaked into
  `buildNavigation()` after all.

## 6. Multi-axis sidebar filtering with selection-aware counts

**Goal.** Let a reader narrow the sidebar by *platform* (Windows, macOS, …) as well as by programme, combine
several filters at once — including several values on one axis — and trust the number next to each filter:
counts reflect what is currently selected, and a filter that would lead nowhere is not offered at all. Today
there is exactly one axis (`?programme=`), it holds exactly one value, and its counts are the tenant-wide
registry counts from the index header, so they never move and a reader can pick a combination that yields an
empty tree.

> **The filter axes come from the CLI, generically.** The browser must not classify: the index-time taxonomy
> is curated, deterministic and resolved from exported facts, and this project only *reads* membership.
> Deriving a platform axis here from `odataType`/`platforms`/naming conventions would move the taxonomy into
> the wrong project. `platforms` is a raw endpoint fact (`windows10`, `macOS`; populated on 73 of 263 and 44
> of 148 resources in `../output`) and stays a badge; `platformGroup` is model-authored, single-valued and
> label-only, so it has no id to put in a URL and stays a badge too.
>
> **This is one mechanism for N axes, not a second hard-coded filter.** The producer-side work is a
> generalisation of the taxonomy from its single hard-coded *programme* axis to named axes, emitting a header
> `facets: [{id, label, values: [{id, label, count}]}]` (in the CLI's display order) and a per-resource
> `facets: {<axisId>: [valueId…]}`. So the implementation here is axis-agnostic: nothing in this project may
> name *platform* or *programme* in a type, a selector or a template branch. It also means the feature needs
> **no documentation regeneration** — a curated axis is resolved by one `docs generate-index` run over an
> export that already exists.
>
> **Degradation is not optional.** An index with no `facets` (every export written so far) must render the
> shipped programme filter from `programmes`/`groups` and today's tree, unchanged; an axis whose values are
> all empty renders as nothing at all, never as a lone *Unclassified* chip.
>
> **Settled URL grammar.** One query parameter per axis, named by the axis id, repeated for multiple values,
> carrying value **ids** and emitted in a canonical order (axes in index order, values sorted), so one
> selection has exactly one URL: `?programme=cis-hardening&programme=defender&platform=windows`. Each axis
> also offers the reserved `_uncategorised` value, prefixed like the `_resource`/`_export` route segments
> because it is a representation rather than data, so no CLI id can collide with it. Unknown values are
> dropped per axis — as `?programme=` already does — rather than 404ing or rendering an empty tenant.
>
> **Selection-aware counts, with the standard facet semantics.** OR within an axis, AND across axes, and each
> facet counted with *its own axis's* selection removed. Without that last part, selecting one value drives
> its siblings to 0 and the hide-empty rule below would erase the axis the reader is using.
>
> **Header counts are non-additive — never sum them.** A resource can carry several value ids on one axis
> (real today on the *programme* axis: 55 of 263 resources in `cb-gmbh`, e.g. a Defender validation policy is
> both `compliance` and `defender`), so the header `facets[].values[].count` sum *exceeds* the number of
> distinct resources (316 across 261 on that axis in that export). The *showing N of M* line and any axis
> total must therefore count **distinct resources**, computed from `resources[]`, and never add value counts
> together. This is the same OR-within-axis membership the filter already applies; it is called out here
> because the counting code is the place the additive assumption sneaks in.
>
> **Hiding empty facets narrows a shipped decision, so state it rather than drift.**
> `buildProgrammeFilters()` deliberately keeps zero-count programmes ("this programme matched nothing in this
> tenant" is information the registry carries), asserted by *offers the programme filter from the index
> registry, zero-count included* in `test/docs.e2e.spec.ts`. That stays true for the **unfiltered** view; the
> new rule applies only to facets that reach 0 *because of another active filter*, and a **selected** facet
> stays visible even at 0, or the reader cannot undo the choice that emptied the tree.
>
> **The active document is exempt from filtering and must stay exempt from counting.** `buildNavigation()`
> always keeps the document being viewed so a filter cannot make the current page vanish from its own
> sidebar. That item is not part of the facet population: counting it would make the numbers disagree with
> the list on exactly the page where the discrepancy is visible.
>
> **Scope boundaries.** The sidebar header's `counts.documented`/`counts.pending` describe the *export* and
> stay tenant-wide; the filtered total belongs next to the tree as a *showing N of M* line, so the two numbers
> cannot be mistaken for each other. The Confluence export is whole-tenant and unfiltered — a browsing
> selection never changes what an export contains. And no client-side JavaScript: multi-select is plain
> anchors whose `href` is the current selection with one value toggled.

**Plan.**

- **Read the axes** in `parseTenantIndex()`: the header `facets` registry and each resource's per-axis value
  ids, ignoring an axis with no id and tolerating their complete absence. When `facets` is missing, synthesise
  the single `programme` axis from `programmes`/`groups` so old and new exports take one code path.
- **Replace the single `programme` parameter** of `buildNavigation()`/`docHref()` in
  `src/docs/tenant-index.ts` with an axis-keyed selection (`Record<string, string[]>`), applying OR within an
  axis and AND across axes, plus the `_uncategorised` bucket per axis. An index with no axes must produce
  today's tree byte for byte.
- **Build the chooser from one axis-agnostic helper** replacing `buildProgrammeFilters()`: counts computed
  from the resources under the current selection with the axis's own selection removed, facets that reach 0
  under a selection dropped, selected facets and unfiltered zero-count facets kept, axes and values in index
  order. Cases in `test/tenant-index.spec.ts` for: a legacy index (identical output to today), one axis, two
  axes combined, two values on one axis, a dead-end combination, and the unfiltered zero-count facet.
- **One toggle-href helper** used by the chips *and* by every document href, plus a **Clear filters** reset,
  so no view builds a URL by hand and no click silently loses another axis.
- **Accept the selection on every route that renders the sidebar** — `tenantIndex`, `doc` and `resource` in
  `docs.controller.ts` — reading arbitrary axis-named parameters rather than a fixed `?programme=`, and
  normalising repeated values (de-duplicate, drop unknowns, canonical order). E2e cases for a two-axis URL, a
  toggle-off link, the reset, an unknown value, and the empty-result message.
- **Render the axes as a loop in `sidebar.hbs`**, one labelled `<nav>` per axis with today's chip treatment,
  dark variant and `:focus-visible` outline, the *showing N of M* line above the tree, and the empty-tree
  message generalised from *No documents in this programme* to one that names the selection.
- **Extend the fixtures**: `test/docs.e2e.spec.ts`'s `INDEX_YAML` carries no `facets`, so add a two-axis
  fixture and keep the existing one as the legacy-degradation case. Real-data verification waits on a
  `generate-index` run with a multi-axis taxonomy.
- **Keep the filters independent of the tree's structure**: whatever the sidebar groups by, the filters narrow
  whichever structure is active, and no grouping or classification logic moves into the filter code.

## Standing decisions

Decisions that are not work items but constrain the work items below, recorded so the next iteration does
not relitigate them.

### Export entry points live on the tenant picker

**Decision.** Every export a *whole tenant* produces is offered on the tenant picker (`GET /`), on that
tenant's card, as a plain `<a download>` with the one-way-publish caveat beside it. Not on the tenant
landing page, not in the top bar, not on document pages.

**Why.** The landing page belongs to `docs/summary.md` — it is documentation, and the view adds no chrome of
its own to it. The picker is where a tenant is chosen *as a whole*, which is exactly the scope an export
operates on, so the button sits with the noun it applies to and stays out of the reading flow. It also means
one place to look per tenant instead of a control repeated on every page.

**How it extends to the planned types.**

- **Further whole-tenant formats** (single-file HTML, DOCX, PDF, Markdown bundle — see the parked idea):
  additional sibling links on the same card, under one `Export:` label once there is more than one. **No
  dropdown, no picker widget** — that needs client-side JavaScript, which is a non-negotiable. If the row of
  formats ever stops fitting, the answer is a per-tenant export *page* (`GET /:tenant/_export`) listing the
  formats, not a control that needs scripting.
- **Partial exports** (one resource type, one document, summary only): these are the one case that must
  *not* be on the picker, because the picker cannot express the scope. Their entry point belongs next to the
  thing being exported — the document top bar next to the **Documentation | YAML** switcher for a single
  document, a sidebar section header for one type — and the picker keeps whole-tenant formats only.
- **Media and source YAML as attachments**: no entry point of its own. It changes what an existing export
  *contains*, never where it is offered.
- **Confluence REST API synchronisation**: not a download, and it mutates a remote system, so it cannot be
  an `<a>` at all — it needs a POST, and no route may mutate state today. It gets no control until that
  design change is actually made, and if it ever does it must be visibly distinct from a download rather
  than sitting in the same row.

**What any new entry point has to keep.** A plain anchor with `download` and no client-side JavaScript; the
route shape `/:tenant/_export/<format>` behind the `_export` representation prefix, declared before the
document catch-all; the one-way caveat rendered next to the link rather than only in the README;
no anchor nested inside another anchor (the picker card is a wrapper element for exactly this reason); and a
dark-mode variant plus a visible `:focus-visible` outline.

## Parked ideas

### Idea: Search across a tenant's documents

Full-text search, or filtering by resource type and name, over everything a tenant has (including the
resource tree). **Parked** because it is the largest single feature on this list and cannot be done well
without an index and, realistically, client-side interaction — and no client-side JavaScript is a
non-negotiable. **Revisit** when the corpus is large enough that the sidebar tree stops being navigable even
with the planned filter, or if a server-rendered query page turns out to be enough.

### Idea: Syntax highlighting inside documents

The `yaml`/`bash`/`powershell`/`json`/`xml` fences *inside* the generated Markdown are unstyled.
`@shikijs/markdown-it` could reuse the highlighter `YamlHighlighterService` owns, with the extra languages
loaded. **Parked** because it costs render time on every document for 182 fences in the whole reference
corpus. **Revisit** if documents start carrying substantial code, or once the highlighter is warm anyway for
other reasons.

### Idea: Multi-segment tenants

Discovery walks up to 3 levels, but routing uses a single `:tenant` segment, so only top-level tenant
folders are addressable. **Parked** because nested tenants would need a different route shape, which
interacts with every `_`-prefixed representation prefix. **Revisit** the first time a real export tree nests
tenants under a grouping folder.

### Idea: Explicit dark-mode toggle

Theme selection follows `prefers-color-scheme`, with no way to override it. **Parked** because remembering a
choice needs either client-side state or a cookie plus a mutating route, both of which cut against the
no-JavaScript and read-only rules. **Revisit** if a reader needs one theme in a browser set to the other,
e.g. for a presentation or a screenshot.

### Idea: Watch-based cache invalidation

An `fs.watch` layer could pre-warm and evict cache entries instead of validating them per request. **Parked**
because the per-request `stat()` delivers the no-restart freshness invariant at negligible cost. **Revisit**
if `stat()` becomes measurable on a slow or networked docs root.

### Idea: A resource landing page

A new `GET /:tenant/_resource` listing every source YAML (index-derived, plus the excluded types), linked
once from the sidebar footer. Sidebar untouched, no extra tree. **Parked** because it costs a route and a
view to reach resources the top-bar **Documentation | YAML** switcher already gets to. **Revisit** if the
switcher proves hard to find, or as the carrier for browsable excluded bulk types (below).

### Idea: A two-group navigation tree

`buildNavigation` returns two top-level `NavGroup`s (*Documentation*, *Resources*), each holding the per-type
`NavSection[]`, with only the group containing the current page `open` and the active marker becoming
`{ kind: 'doc' | 'resource', path }`. Both trees built in one index pass with two href shapes, so they cannot
drift. **Parked** because the resource tree duplicates the doc tree (263 items twice in the reference export)
and `sidebar.hbs` gains a second `<details>` level. **Revisit** only if the **Documentation | YAML** switcher
turns out to be too hard to find and a resource landing page does not fix it.

### Idea: Clickable breadcrumb segments

Turn `Microsoft.Graph / depOnboardingSettings` in the breadcrumb into links to a per-type listing page.
**Parked** because it needs a new route and view: CSS alone cannot open a collapsed `<details>` section from
an anchor, so linking into the existing sidebar tree is impossible without client-side state. **Revisit**
after a per-type listing page exists for another reason, when this becomes additive and nearly free.

### Idea: Browsable excluded bulk types

Types the index merely counts under `counts.excluded` (Autopilot identities and the like), and any resource
with no document, are unreachable — which is what keeps navigation purely index-derived and the **"counts and
listings derive from the index, never from walking the tree"** non-negotiable intact. Unreferenced
`Microsoft.Graph/groups` (the CLI documents a group only when an assignment references it) are in the same
bucket. **Parked** because serving them requires amending that non-negotiable. **Revisit** if operators ask
for the raw bulk YAML; the shape is settled — for each type named in `counts.excluded`, a single
**non-recursive** `readdir` of `resources/<type>/` (`.yaml` only, sorted, cached under the existing discovery
TTL, unreadable ⇒ empty), producing file names only, with counts still taken from the index. It reopens two
UX questions: whether a `readdir`-vs-`counts.excluded` mismatch should be flagged as a stale index, and
whether resources without documentation should be visually de-emphasised.

### Idea: Media and source YAML as page attachments

Confluence's HTML import gives each page a media folder named after it, which is where the two images in the
reference corpus — and, in principle, each document's source YAML — could travel. **Parked** because
`resolveWithinRoot()` serves exactly **one** extension per root (`.md` under `docs/`, `.yaml` under
`resources/`), and widening that to an extension list is a non-negotiable: images currently travel as their
`alt` text and the export attaches nothing. Doing it properly means a third served root with its own single
extension policy — a design change, not a feature. **Revisit** if generated documents start carrying
diagrams that matter, or if readers of an imported space ask for the YAML next to the page. It gets no entry
point of its own either way (see *Export entry points live on the tenant picker*).

### Idea: Confluence REST API synchronisation

Create and update pages in place, with labels, a page tree and attachments — the proper answer to "keep
Confluence up to date". **Parked** because it is a much larger feature than HTML import: authentication, a
persisted mapping from resource to page id, and conflict handling, the last two of which sit awkwardly with
a read-only browser that stores no state. **Revisit** once one-way HTML publishing is in real use and its
re-import cost is felt. Note that it cannot reuse the export link's shape at all — it mutates a remote
system, so it needs a POST rather than an `<a download>` (see *Export entry points live on the tenant
picker*).

### Idea: Tenant diff

*As an administrator I would like to diff the configuration of two tenants, so I can detect and understand
drift.* Two use cases: **a)** diff a staging tenant against production, so a reviewed change can be moved
from stage to prod quickly and nothing else moves with it; **b)** compare configuration *and* documentation
across several tenants and see what the differences actually mean, not just that bytes differ. The pairing
key already exists — every export uses the same `<type>/<name>` layout under `docs/` and `resources/`, and
`docs/index.yaml` gives per-tenant type, scope and counts without walking the tree — so a first version could
be a three-way listing (only in A, only in B, in both but different) over the index, refined to a per-document
comparison. **Parked** because it is a comparison *engine*, not a view: identity does not survive across
tenants (GUIDs, assignment group ids and display names all differ, so equal configuration reads as different
and the interesting drift hides in the noise), a readable diff of a 317-setting document needs interaction
the no-client-side-JavaScript rule forbids, every route today is scoped to one `:tenant`, and it is not
settled whether the comparison belongs here at all rather than in the CLI, which holds the facts
(`resources/metadata.yaml`, the per-resource hashes) that make a semantic diff cheap. **Revisit** when a
stage/prod tenant pair is actually exported side by side into one docs root, and once there is an answer for
cross-tenant identity — a normalisation of tenant-local ids that can be stated and tested, not guessed per
resource type.

### Idea: Further export formats and partial exports

Single-file HTML, DOCX, PDF via a print stylesheet, a Markdown bundle; and exporting one type, one document
or only the summary. **Parked** deliberately: the Confluence exporter is whole-tenant only, and the
`src/docs/export/` seam exists so a second format is a second `ExportService` method plus its own format
module, with the controller and the serialiser untouched. **Revisit** per format when someone actually needs
it. Preserving the document tree in an export is not expressible through Confluence HTML import at all —
re-parenting by hand or the REST API are the only routes. Where each of these would be offered is already
settled, including why the partial exports are the exception: see *Export entry points live on the tenant
picker*.

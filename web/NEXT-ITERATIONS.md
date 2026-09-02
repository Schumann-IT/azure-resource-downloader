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
  items, with the current filter reflected in the URL).
- Per-item summary/badge rendering in `sidebar.hbs`, from the same `docs/index.yaml` fields the listing
  fallback reads.

## 5. Group-driven navigation, with a programme facet over it

**Goal.** Let a reader find a resource by what it *is for* — its platform, its function, and the programme
it belongs to — instead of by Azure/Graph resource type. The type tree is the vendor's taxonomy: an
administrator looking for "how are Macs hardened" has to know that the answer is spread over
`deviceConfigurations`, `deviceManagementConfigurationPolicies` and `deviceShellScripts` before they can
start.

> **The shape, in two dimensions.** A **spine of platform → function**, one path to each document,
> replacing the per-type sections; a **programme facet over it** (`CIS L1 hardening`, `Defender / MDE`,
> `VPN`, `Update rings`, `Autopilot`, `App delivery`) that is **many-to-many** — a resource belongs to a
> programme *and* keeps its place in the spine, which is exactly what a single-parent tree cannot express —
> rendered as a filter and alongside the existing `badgesFor()` badges, never as a second tree; and the
> endpoint type kept as a *filter* rather than as the structure, since it is still the right lens when
> someone is reasoning about the API rather than about the tenant.
>
> **The web side must not compute any of it.** It is tempting to derive both dimensions here from
> `odataType`/`platforms`/naming conventions — 93 of 225 named resources match
> `GBL_<kind>_<env>_<scope>_[CIS_]<platform>_<area>[_L1]`, and one rule over that catches all 32 CIS-named
> resources. But `src/docs/export/confluence.ts` is a second consumer of the same index, and anything
> derived inside `buildNavigation()` is invisible to it: an exported space would silently fall back to
> endpoint grouping while the browser showed programmes. The taxonomy is resolved once at index time, in
> the CLI, so both consumers read the same fields — the producer-side entry is *Group documentation by
> purpose, resolved at index time* in `go/NEXT-ITERATIONS.md`.
>
> **Which is why the source of a programme is not this entry's decision to make**, only its constraint.
> Four candidates are on the table: **a) model-authored per document**, a third frontmatter field with a
> closed vocabulary — captures intent, but costs a full regeneration and gives each document exactly one
> programme, which CIS hardening does not fit; **b) derived from the resource name**, deterministic and
> free (`GBL_AF_PRD_D_WIN_MGM_ModernWorkplace` → `MGM_ModernWorkplace`), but it assumes a naming convention
> not every tenant follows; **c) a curated taxonomy resolved at index time**, rules over facts plus name
> patterns, multi-valued and needing no regeneration, at the price of a file to maintain whose rules rot
> silently; **d) no programme facet at all**, if platform × function answers the question. The Go entry
> currently leans to model-authored axes as the spine with a curated index-time taxonomy as the programme
> dimension; whatever is picked, **the browser reads the fields the index carries and adds none of its
> own**. What the choice does decide here is cardinality —
> single-valued is a tree level, multi-valued can only be a filter — so the facet UI waits until that is
> settled rather than guessing.
>
> **Nothing carries the data yet.** In the reference export `platformGroup` and `functionGroup` are
> populated on **0** of 263 resources (against `platforms` 73, `odataType` 137, `scope` 93, `assignments`
> 231), because the CLI's generation template does not ask the documents for them. Grouping by fields that
> are empty everywhere yields one *Ungrouped* section, which is worse than grouping by type — so this entry
> is only implementable behind a degradation rule, not a flag day.
>
> **What is cheap once the index carries it.** Hrefs come from the index `doc` field and are independent of
> how `buildNavigation()` groups, so **regrouping the sidebar changes no URLs** — no redirects, and no
> broken links into an already-imported Confluence space. `sidebar.hbs` renders `nav.sections` purely from
> data, so the spine is a data change plus one nesting level in the partial. A facet switch is a route or a
> query parameter reflected in the URL, never a widget: no client-side JavaScript.

**What the CLI must provide before the spine can be built.** Implementation starts when all three hold;
none of them can be substituted here.

- **The two fields, actually populated** on in-scope resources in `docs/index.yaml`. The harvest is already
  wired on the producer side — the values are empty because no prompt asks for them — so this costs a full
  documentation regeneration.
- **A declared vocabulary with a display order, carried in the data.** Ordering "by the declared vocabulary"
  is only possible if the order is readable at runtime: either the index header carries the vocabulary per
  axis in display order (self-describing, no shared constant to drift), or it is published as a constant
  this project copies — which is then a contract in two repositories. The first is strongly preferred, and
  it is the requirement most easily forgotten, because on the producer side the vocabulary looks like a
  prompt-authoring concern.
- **Defined semantics for "no group"**, distinguishing *the model did not classify this* from *this type has
  no meaningful platform* (a tenant-level singleton is not an uncategorised Windows resource). Either an
  explicit value, or a written statement that empty means uncategorised and which types are expected to be
  empty. Without it the uncategorised bucket silently merges a real gap with a category error.

**Release ordering is a constraint today, and cheap to remove.** `parseTenantIndex()` rejects anything whose
`version` is not exactly `1`, and the index is also the *tenant marker* — so if the CLI bumps the schema to
carry these fields, every export disappears from the picker and every route 404s, rather than degrading to
the current tree. Unknown *fields* are already ignored, so the fix is the version comparison alone.

**And, additionally, before the programme facet:** a multi-valued membership field carrying a **stable id**
(it becomes the facet value in the URL, so renaming a programme must not break a bookmark) **and a display
label**; plus, to render the chooser, the set of programmes that exist — from per-resource membership alone,
a programme with no matches in this tenant is invisible, which is itself information.

**Plan.**

- **Accept a later index schema first**, before the CLI ships anything: make `parseTenantIndex()` take any
  integer `version >= 1` and carry the parsed value, with a case in `test/tenant-index.spec.ts` and one in
  the e2e suite proving a newer index still *discovers* as a tenant. It is a few lines, it is worth having
  whether or not the rest of this entry ever happens, and it means the two projects can release in either
  order.
- **Ship the spine first, the programme facet second**, and treat them as separate deliveries: the spine
  needs one group per document, the facet needs multi-valued membership, and building the facet UI before
  the CLI emits real membership is guessing at its cardinality.
- **Group in a pure function** next to `buildNavigation()` in `src/docs/tenant-index.ts`, reading only what
  the index carries and deriving nothing, taking the facet as an argument, ordering groups by the declared
  vocabulary rather than alphabetically, and putting uncategorised documents in a named bucket that is
  **always rendered** — a taxonomy that quietly stops matching must show up as a full bucket, never as a
  thinning tree. Cases in `test/tenant-index.spec.ts` for a fully grouped index, a partially grouped one,
  and an index with no grouping at all; the last must produce today's per-type tree unchanged.
- **Nest the spine in `sidebar.hbs`** as one more `<details>` level, and put the active facet in the URL so
  the choice survives a reload without state. If a tree filter exists by then, it narrows whichever
  grouping is active rather than only the type tree.
- **Keep the endpoint type reachable** as a filter or an alternate facet once it is no longer the
  structure, so an API-shaped question is still answerable.
- **Drop the grouping badges** from the listing fallback once the same values drive the tree, so a resource
  does not state its group twice on one screen.
- **Verify both consumers agree** before calling it done: the Confluence export groups its overview page
  from the same fields, so a grouped browser and an endpoint-grouped export means the taxonomy leaked into
  `buildNavigation()` after all.

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

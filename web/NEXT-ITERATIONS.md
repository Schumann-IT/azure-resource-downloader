# Next iterations

Standing decisions and parked ideas for the docs browser. `README.md` describes what it does today and
`CHANGELOG.md` records what shipped; neither is repeated here.

**Nothing is currently scheduled.** Every idea below is deliberately unscheduled: picking one up means
promoting it into a numbered work entry with a `**Goal.**` and a `**Plan.**`, reconciling its rationale
against what is true at that point rather than copying it across.

## Standing decisions

Decisions that are not work items but constrain the ideas below, recorded so the next iteration does not
relitigate them.

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

### Idea: Per-document identity on the article

Put `data-family` and `data-type` on `<article>` so the stylesheet can treat a group, a credential and a
settings-catalog policy differently — a credential document leading with its expiry, a `record` document
dropping the assignment vocabulary it never uses, `[data-family="group"] [data-section="properties"]`
differing from the same section on a policy. **Parked** because no reader has asked to tell the families
apart, and the attribute is worthless until per-family CSS exists, so it would ship as decoration.
**Revisit** when a concrete per-family styling need appears; then add the attribute and the CSS together.

What is settled if it is picked up: `data-type` is free (the document's own directory), while `data-family`
cannot come from the index or the frontmatter — `IndexResource` has no family or prompt-template field and the
frontmatter carries only `source` and `generatedAt`. The carrier is the document's **own H2 set**, which
partitions the 414 reference documents cleanly: 333 *References | Lifecycle and operations | Security |
Settings* (the `default`/`singleton` contract, indistinguishable from each other by headings alone), 36
`group`, 25 `referenced`, 6 `record`, 4 `credential`, 0 `arm`. Ignore the spliced `Targeted by` / `Used by`
headings, emit nothing when the set matches no contract, never infer the family from the resource type, and
take the headings from `markdown-it`'s tokens the way `section-hooks.ts` does — at least one document has a
`##` line inside a fenced code block, which a line-based regex would miscount.

### Idea: An actionable findings block

Let a reader of the tenant landing page narrow the findings table to a severity and get from a finding to the
documents it affects. **Parked** because the table is 15 rows in the largest reference export — short enough
to read whole — so filtering it buys little, and the `Documents` column already links to every document a row
names, which leaves the inert `Affected` count as the only real gap. **Revisit** when a summary carries enough
findings that the table stops being readable in one pass.

What is settled if it is picked up: the hooks exist (`src/docs/findings-table.ts` tags the table `.findings`
and puts `data-severity` on each body row and severity cell, with lowercase severity ids), and the filter is
expressible **without JavaScript** — renderer-emitted sibling anchors plus `:target`, e.g.
`#sev-critical:target ~ .findings tbody tr:not([data-severity="critical"]) { display: none }`. That requires
the anchors to be siblings of the table, so a wrapper is a prerequisite, and it spends the URL fragment that
already belongs to `#findings`, so a **Show all** reset is part of the feature rather than a refinement. One
caveat: `.prose table` sets `white-space: nowrap` so wide assignment tables scroll instead of wrapping
mid-GUID, and any prose-bearing table needs an opt-out from it.

### Idea: Summary table of contents

A jump list on the tenant landing page so a reader can go straight to the part of the summary they came for.
**Parked** because the summary is four H2s and two H3s long — a table of contents for six anchors competes
with the document it indexes for the top of the page. **Revisit** if the summary contract grows, or if readers
report scrolling past the management summary to reach the caveats.

What is settled if it is picked up: the four H2s are a declared CLI-side contract with stable slugs
(`#management-summary`, `#at-a-glance`, `#assignment-posture`, `#coverage-caveats`) plus the `#findings` and
`#recommendations` H3s, all verified present in both reference exports and all emitted as ids by
`markdown-it-anchor`, so the list is hard-codeable with no renderer change. Skip entries whose heading is
absent so an older summary degrades to a shorter list, and omit it entirely in the index-listing branch where
there is no summary at all.

### Idea: A name filter and per-item context in the sidebar

Narrow the sidebar by part of a name (a server-side query parameter, composing with the shipped taxonomy
filters rather than replacing them), and give each tree item the context the listing fallback already shows.
**Parked** because the taxonomy filters cut the 263-item tree to a workable size along the axes that matter,
which was the pressing half of the problem, and because a name filter without a text input is an awkward thing
to offer — no client-side JavaScript means no type-ahead. **Revisit** if narrowing by axis proves
insufficient, or alongside the search idea below, which subsumes it.

What is settled if it is picked up: **badges are available today** — `assignments` (231 of 263), `scope` (93),
`platforms` (73), `odataType` (137), plus the facet memberships the filter already renders. The per-item
**summary is not**: it is present on **0 of 263** and **0 of 148** resources, because `GenerateIndex` reads it
from each document's frontmatter and generated documents write only `source` and `generatedAt`. The index
schema and the CLI plumbing are both correct, so that half is gated on a documentation **regeneration** whose
template emits `summary:`, not on a change here — build it to render no second line when the field is absent
and it lights up on its own. Remembering which sections were open across navigations stays **excluded on
purpose**: that needs client-side state.

### Idea: Structure the sidebar by a taxonomy axis instead of by resource type

Let a reader reach a document by what it *is* — a Windows policy, a device-scoped configuration — rather than
by the Azure/Graph type that happens to implement it, so "how are Macs hardened" does not require knowing the
answer is spread over `deviceConfigurations`, `deviceManagementConfigurationPolicies` and
`deviceShellScripts`. Filtering narrows the tree; this would replace its *structure*, which filtering
deliberately leaves alone. **Parked** because the shipped axis filters already answer the same question from
the other direction — selecting *Platform: macOS* yields exactly that reading list — so a second structure
would be a large change to the navigation for a smaller marginal gain. **Revisit** if readers keep reaching
for the filter as a substitute for structure, or once an axis exists dense enough to carry a spine on its own.

What is settled if it is picked up. **The spine is a facet axis, not the model's grouping fields**: v3
`index.yaml` carries the header `facets` registry and per-resource `facets` (`axis id → value ids`), and both
exports declare `programme`, `platform` and `scope` — curated, deterministic, id-bearing and ordered by the
header, which is everything a tree spine needs, on exports that exist today. `platformGroup`/`functionGroup`
stay badges: single-valued, label-only, and empty in both exports. There is no `function` axis, so a
function-shaped spine needs an operator-authored axis in the CLI config or `functionGroup` — not
classification here. **An axis is not a partition**: 55 of 263 resources hold several `programme` values, so
some appear under more than one group (show them in each and say so; picking a "primary" value is a judgement
this project may not make), and axes are sparse — 63 of 263 carry no `platform`, 170 no `scope` — so the
uncategorised group is **always rendered**. **The grouping must not be derived here**: `confluence.ts` is a
second consumer of the same index, and anything computed inside `buildNavigation()` is invisible to it, so an
exported space would silently group differently from the browser — which also means the export's own grouping
has to be decided explicitly rather than left to drift. What is cheap: hrefs come from the index `doc` field,
so **restructuring changes no URLs**, and `sidebar.hbs` renders sections purely from data, making the spine a
data change plus one nesting level; the choice belongs in the URL as a query parameter, never a widget.

### Idea: Search across a tenant's documents

Full-text search, or filtering by resource type and name, over everything a tenant has (including the
resource tree). **Parked** because it is the largest single feature on this list and cannot be done well
without an index and, realistically, client-side interaction — and no client-side JavaScript is a
non-negotiable. **Revisit** when the corpus is large enough that the sidebar tree stops being navigable even
with the shipped taxonomy filters, or if a server-rendered query page turns out to be enough. It subsumes the
name filter in the sidebar idea above.

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

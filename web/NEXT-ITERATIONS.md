# Next iterations

Outstanding work, standing decisions and parked ideas for the docs browser. `README.md` describes what it does
today and `CHANGELOG.md` records what shipped; neither is repeated here.

The *Features* below are scheduled design work, promoted from *Parked ideas* and refined against what is true
now. The *Fixes* below are scheduled too, but smaller: self-contained corrections that need no design work,
listed so they are not forgotten between features. Every idea in *Parked ideas* is deliberately unscheduled:
picking one up means promoting it into a numbered work entry with a `**Goal.**` and a `**Plan.**`, reconciling
its rationale against what is true at that point rather than copying it across.

## Features

### 1. ~~A drift view: what changed in the tenant since the export was taken~~

**Goal.** A reader of any resource's documentation can ask *"has this changed in the tenant since this export
was taken?"* and get the answer in one click — a **Drift** button beside **Documentation | YAML**, leading to
that resource's recorded field changes, the analysis written about them, and the observed configuration itself.
At tenant scope the same switcher appears on the landing page as **Summary | Drift**, so the observation as a
whole sits beside the tenant summary rather than hidden behind a URL. The button is **always rendered and goes
inert** when there is nothing to land on, so the question is visibly asked and answered on every page rather
than the control appearing and disappearing. The comparison stays the CLI's job; this app only renders what
`azure-rd resource drift` observed and what `azure-rd docs analyze-drift` had analysed.

> **What is on disk, and it is all already there.** `resource drift` writes `<export>/drift/`, mirroring
> `resources/` exactly. `drift/metadata.yaml` is the observation: `observedAt`, `tenant`, `toolVersion`,
> `baseline` (`generatedAt`, `toolVersion`, `transformConfigSha256`), `run` (`complete`, `incompleteReason`,
> `scope`), `counts` (`compared`, `unchanged`, `changed`, `renamed`, `added`, `removed`, `unattested`,
> `excluded`, `failed`), `unknownTypes`, `removalsSuppressed`, `notComparable`, `findingsSha256`, `payloads`,
> and a `findings` map keyed by `<APIType>/<endpoint>/<name>.yaml` whose entries carry `verdict`,
> `resourceId`, `displayName`, `previousDisplayName`, `baselineKey`, `baselineSha256`, `payloadSha256`,
> `docPath` and dotted-path `deltas` (`path`, `old`, `new`) or a `deltaNote`. The observed bytes sit at
> `drift/<key>`, marshalled exactly as the export marshals them. An analysis pass adds one **drift document**
> per in-scope finding at `drift/<key-without-.yaml>.md` (frontmatter `observedAt`, `baselineGeneratedAt`,
> `verdict`, `severity`) plus the summary `drift/index.md` (frontmatter `observedAt`,
> `baselineGeneratedAt`, `findings`, `severities`). `drift/analyze.md` is tool input, like `docs/generate.md`,
> and `drift/metadata.yaml` belongs to the CLI: neither is ever served.
>
> **The join key is free — with one rename caveat.** A finding's key minus `.yaml` is exactly the
> extensionless path the top-bar switcher already builds **Documentation** and **YAML** from, so the three
> become three renderings of one path and the button's `href` needs no lookup. Its *state* is one lookup on
> the observation, indexed at parse time by finding key **and by `baselineKey`**: a *renamed* resource's
> finding is keyed by its **new** name while its document and baseline still sit at the old one, so a lookup by
> key alone would classify that page as unchanged and render a lying inert button. For the same reason
> `docPath` is never used: the CLI derives it from the finding key, so for a rename (and an addition) it names
> a document that does not exist yet — the documentation link derives from `baselineKey`, offered only when
> the index lists that document. Nothing is walked; the counts-and-listings-derive-from-the-index
> non-negotiable holds with the observation playing the role `index.yaml` plays for `docs/`.
>
> **The validity gate is mandatory, and it compares against what the CLI compares against.** The observation
> names its baseline as `baseline.generatedAt`; `azure-rd docs analyze-drift` declares it superseded when that
> differs from `resources/metadata.yaml`'s `generatedAt`, and this app applies the **same** test so the two
> tools never disagree about one export. That is a new, small read: only the top-level `generatedAt` of
> `resources/metadata.yaml`, cached by mtime + size like the index. The index's own `generatedAt` is the same
> timestamp on a consistent export (verified identical in the reference export) but is **not** the gate:
> `generate-index` runs after a re-download, so between the two the index is stale and a gate built on it
> would tell the operator to re-run drift when the real remedy is to regenerate the index. It is used only as
> the fallback when `resources/` is absent altogether (an export copied without it). A superseded observation
> renders the whole drift surface as *outdated — re-run `azure-rd resource drift`*, never as a comparison
> against the wrong baseline. Per resource, `baselineSha256` and `payloadSha256` are re-checked before anything
> is shown as a comparison; a mismatch withholds it and says so.
>
> **Path safety: `drift/` stays where it is, served by two pinned resolvers.** The tree holds **both**
> extensions — agent-written `.md` beside CLI-written `.yaml` — while the rule says one extension per served
> root. The answer is two resolvers pinned to one extension each over the same root (`.md` for drift documents,
> `.yaml` for payloads), never an extension list and never a widened `resolveWithinRoot`, plus a **depth
> guard**: a drift path needs at least two segments, so everything at the tree root (`metadata.yaml`,
> `analyze.md`, `index.md`) is unreachable through them by construction rather than by a name list — a stronger
> guarantee than `resources/` has today, where `_resource/metadata` serves the export's own metadata. Every
> guarantee of `resolveWithinRoot` is kept unchanged (no null bytes, no absolute paths, no `..`, realpath
> containment, exactly one extension per call), so the rule wording moves from one extension per *root* to one
> extension per *resolver* — a clarification of the same principle, in
> `.windsurf/rules/01-architecture.md` in the same edit. It is no new class of exposure either: the **YAML**
> view already serves exported source bytes from the same trust boundary.
>
> **Rejected: relocating the tree into `resources/drift/` and `docs/drift/`.** It would let the two existing
> resolvers serve everything and need no rule change here at all — and it is the wrong trade, because it buys a
> wording clarification on this side by breaking three stronger invariants on the CLI side. `drift.ClearTree`
> is a single constructed path today, provably unable to reach `resources/` or `docs/`; after the move the
> ephemeral tree is *inside* both, so every drift run and every re-baselining download would delete a subtree
> of the source of truth. `--prune` walks `resources/` and deletes what the export metadata does not describe,
> which is every drift payload. And the `docs/<key>.md` ↔ `resources/<key>.yaml` bijection — relied on by the
> CLI, this app and the documentation agent — would gain a namespace that is not a resource type, with
> `generate-prompt`'s orphan and migration logic to teach about it. A third sibling tree that nothing else
> writes to or walks is the cheaper boundary; keep it.
>
> **If even the clarified wording is unwanted**, the fallback is to serve **`.md` only** from `drift/` — one
> root, one extension, no amendment whatsoever, with the observation read as data exactly as `docs/index.yaml`
> is read and never served. The cost is precise: *changed* and *renamed* resources still read well from the
> recorded deltas, but an **addition has no deltas and no baseline**, so its observed payload is the only
> description of it that exists, and without it the view can offer nothing but a display name and the analysis
> prose. That case is why the payload is in scope.
>
> **The tree is deleted wholesale, and that is normal.** The next `resource drift` run and any re-baselining
> `resource download` remove `drift/` entirely. So it is **not** a discovery marker (a tenant without it stays
> a tenant, exactly as one without `resources/` does), every read degrades to *no observation*, and the
> no-restart freshness invariant has to hold in the delete direction too: a cache entry whose `stat()` fails
> is dropped, and the drift surface disappears on the next request without a restart.
>
> **Partial coverage is the normal case, so "no finding" is not "unchanged" — and that decides when the button
> is inert.** The reference observation is `complete: false` with four types unlisted and
> `removalsSuppressed: true`. A resource has no finding either because it is unchanged, or because its type
> could not be listed (`unknownTypes`), or because its baseline entry could not be compared (`notComparable`) —
> so the inert state may never stand for all three. It is reserved for the one case that is a real answer:
> **the observation compared this resource and it had not changed**, said so in the button's own tooltip with
> the observation date. Anything the observation has something to *say* about — a verdict, a reason it could
> not compare, or that it is stale — stays clickable, because an inert control there would silently deny what
> the page could state. The full state list is in *One decision, two consumers* below.
>
> **Only "nothing to land on" is inert**, at either scope: no observation on disk for the tenant. An
> observation that exists but has **no findings** is still worth a page (*observed at X, nothing had changed*),
> and so is one whose analysis has not run yet — `drift/index.md` missing means the agent has not written the
> summary, not that there is nothing to show, because the findings list is rendered from the observation
> itself. Inertness tracks the observation, never the analysis.
>
> **Not every finding has neighbours.** Analysis scope is applied when the prompt is rendered, not in the
> observation, so findings exist for types that have no document and get no drift document (Autopilot
> identities, unreferenced groups) and are reachable only from the tenant drift page — which also means the
> index cannot be the sole oracle for "does this export know this key"; an addition has no baseline; a removal
> has no payload and its baseline file may since have been pruned.
>
> **One decision, two consumers.** The button and the page it leads to must never disagree, so both read one
> pure function over `(observation, baselineGeneratedAt, index, key)` — the same discipline the CLI applies to
> prune, where preview and deletion share one eligibility decision. Its states, in evaluation order:
> *no observation* (button inert), *superseded* (clickable, plain label, page is the gate, findings not
> consulted), *finding by key or by `baselineKey`* (clickable, verdict in the label), *type in `unknownTypes`*
> and *key in `notComparable`* (clickable, plain label, page states why it was not compared), *known to the
> index* (inert, *unchanged as of `observedAt`*; page states the same when the URL is typed), otherwise
> *unknown* (404). `parseObservation()` indexes `findings` by both keys and turns `unknownTypes` and
> `notComparable` into sets, so the decision is constant-time on every render.
>
> **No diff algorithm in this entry.** The CLI already recorded the dotted-path `old → new` changes (values
> truncated, one-sided keys rendered `(absent)`), which is the readable answer for the common case and needs
> no dependency and no script. Both full versions stay one click away through the existing highlighter. A
> computed unified diff — the thing the no-client-side-JavaScript rule actually makes awkward — is a separate,
> later question.
>
> **Out of scope, deliberately.** A computed line diff; drift markers on sidebar tree items; drift in the
> Confluence export; and anything that acts on drift (accepting a change, triggering a re-baseline) — that
> would need a write path and a mutating route, both forbidden. This entry is not regeneration-gated: it
> renders artifacts the CLI and an analysis run already produce.

**Plan.**

- ~~**Observation reader.** `parseObservation()` in `src/docs/drift-observation.ts` — pure, synchronous,
  Nest-free, mirroring `parseTenantIndex()`: version-free but shape-validated, malformed or unreadable ⇒
  `undefined`, never a throw. A service method beside `getIndex()` reads it cached by mtime + size and drops
  the entry when the file is gone. `TenantInfo` gains `driftDir`, `driftObservationPath` and `driftIndexPath`;
  discovery keeps keying on `docs/index.yaml` only.~~
- ~~**Path safety.** `resolveDriftDocument()` (`.md`) and `resolveDriftPayload()` (`.yaml`) over `driftDir`,
  both through the unchanged `resolveWithinRoot()`, plus the ≥ 2-segment guard. New cases in
  `test/path-safety.spec.ts`: a `.md` is not servable as a payload and a `.yaml` not as a document, traversal
  out of the drift root 404s, and nothing at the tree root (`metadata.yaml`, `analyze.md`, `index.md`) is
  reachable through either resolver.~~
- ~~**Drift state.** `driftState()` beside `parseObservation()` in `src/docs/drift-observation.ts`, pure and
  Nest-free, returning the states above with the data each needs (verdict, `observedAt`, reason); a unit spec
  covers every state, the rename-by-`baselineKey` case and the evaluation order.~~
- ~~**Link rewriting.** `LinkEnv` gains an optional route base so relative `.md` links inside `drift/index.md`
  and the drift documents (the analysis template has them link each other by relative path) resolve to
  `/:tenant/_drift/…` instead of the documentation route; documents pass no base, so their behaviour is
  byte-identical. `rewriteHref` has no unit spec today — it is covered by the e2e cross-type link case — so
  this adds `test/link-rewrite.spec.ts` for both bases plus an e2e case for a link inside a drift document.~~
- ~~**Routes**, behind a `_drift` representation prefix declared before the document catch-all, like
  `_resource` and `_export`:~~
  - ~~`GET /:tenant/_drift` — the observation as a whole, and the **tenant-scope counterpart of the landing
    page**: header (observed at, baseline, verdict counts, completeness, unknown types, not-comparable
    entries, suppressed removals), `drift/index.md` as the body when the analysis has written it, otherwise a
    findings list grouped by resource type, built from the observation alone. Same layout as the landing page —
    sidebar, prose column — so the two read as two views of one tenant.~~
  - ~~`GET /:tenant/_drift/index` redirects to `/:tenant/_drift`, mirroring `summary` → `/:tenant`: the drift
    index has one address, and the depth guard already keeps it out of the per-resource route.~~
  - ~~`GET /:tenant/_drift/*path` — one resource, rendered from its drift state: the verdict, the recorded
    deltas as a table (values verbatim as the CLI rendered them, escaped, in code cells), the drift document
    when one exists, and links to the baseline YAML (`_resource`, from `baselineKey`), the observed payload
    and the documentation (when the index lists it). Before a *changed*/*renamed* comparison is shown, the
    baseline file and the payload are hashed and checked against `baselineSha256`/`payloadSha256` (hashes
    cached by mtime + size); a mismatch renders a warning instead of the comparison and offers no payload. The
    *not compared* states and the *superseded* gate render as pages, a key the observation compared without a
    finding renders *unchanged as of `observedAt`*, and only the *unknown* state is a 404.~~
  - ~~`?yaml` renders the observed payload highlighted (reusing `YamlHighlighterService`); `?raw` serves it as
    `text/plain` with `nosniff`, exactly as `_resource?raw` does. Both exist only for states with a payload.~~
- ~~**The buttons**, both extending the view model `partials/header.hbs` renders to
  `{ label, href, active, reason }`, where a **missing `href` is the inert state** — the one template change,
  shared by both scopes, and the only logic the partial gains: an `<a>` when there is an `href`, otherwise a
  muted `<span>` carrying `reason` as its `title` (and `aria-disabled`), which keeps it out of the tab order
  without a `disabled` attribute no anchor honours.~~
  - ~~**Resource scope**: `views()` gains a third entry, **Drift**, always present wherever the switcher is
    shown today, with `kind: 'doc' | 'resource' | 'drift'`, its label, `href` and `reason` taken from the drift
    state and nowhere else. The observation and the baseline timestamp are therefore read on every document
    and YAML render — two cached reads, degrading to the inert *no drift observation* state when missing.~~
  - ~~**Tenant scope**: the landing page gains its first switcher, **Summary | Drift**, from a sibling helper.
    **Drift** is inert only when no observation exists (*run `azure-rd resource drift`*); an empty or
    not-yet-analysed observation and a superseded one are all clickable, the last because an operator who
    re-downloaded needs to be told their observation is stale, not left with a dead control. No sidebar line
    and no picker link: one entry point per scope, beside the noun it applies to.~~
- ~~**Views.** `drift.hbs` for a resource and a tenant-scope view that reuses the landing page's shape, verdict
  and severity as plain badges, every state with a dark variant; `{{{ }}}` only for renderer- and
  highlighter-produced HTML. The inert button gets its own muted treatment with `cursor-not-allowed` and no
  hover state, while the visible `:focus-visible` outline stays on the anchor variant, which is the only
  focusable one. The sidebar marks the resource's own item active on its drift page, as the YAML view does.
  `_drift` never appears in a breadcrumb, exactly like `_resource`.~~
- ~~**Docs and rules.** README: the two routes, the drift root contract, the ephemerality note, and that
  `analyze.md` and `metadata.yaml` are never served. `.windsurf/rules/01-architecture.md`: the amended
  one-extension-per-*resolver* wording, drift as a third served root that is not a discovery marker.
  `CHANGELOG.md` under `[Unreleased]`.~~
- ~~**Tests.** `test/docs.e2e.spec.ts`: the button is present on the documentation and YAML views of every
  resource, as a **link** for a finding, an unlisted type and a not-comparable entry, and as an **inert span
  stating why** for an unchanged one and for a tenant with no observation; a renamed resource's page at its
  old name gets the link, not the inert span; a changed resource renders its deltas and its drift document; an
  addition renders its payload; a tampered payload renders the warning and no comparison; the landing page
  always offers **Summary | Drift**, inert without an observation and a link with an observation that has no
  findings and no `index.md`; `_drift/index` redirects; a superseded observation renders the gate at both
  scopes and never a comparison, and a stale index alone does not trigger it; `analyze.md` and `metadata.yaml`
  are not served; a rewritten observation and a deleted tree are both reflected on the next request without a
  restart.~~

## Fixes

Each is a numbered work entry in its own right; none touches a non-negotiable (read-only, no client-side
JavaScript, one `markdown-it` instance, path safety) and none depends on a documentation regeneration. Each
carries its own e2e or spec case and a `CHANGELOG.md` entry under `[Unreleased]`; purely internal ones say so.

A **struck-through** title or plan item has shipped and its `CHANGELOG.md` entry is written. It stays here,
struck, until the branch is closed and the release is cut — that is when the entry is deleted, not the moment
the code lands.

*None outstanding.*

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
  dropdown, no picker widget** — that needs client-side JavaScript, which is a non-negotiable (dropping that
  rule is its own parked idea; until it is actually dropped, this decision stands as written). If the row of
  formats ever stops fitting, the answer is a per-tenant export *page* (`GET /:tenant/_export`) listing the
  formats, not a control that needs scripting. A format that grows **options** is a second, earlier reason to
  reach for that page — see the parked idea on a single export button per tenant.
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
findings that the table stops being readable in one pass, or if the no-client-side-JavaScript rule is relaxed
(its own idea below), which would replace the `:target` construction described next with an ordinary filter and
give the `#findings` fragment back.

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
insufficient, alongside the search idea below, which subsumes it, or if the no-client-side-JavaScript rule is
relaxed (its own idea below): a text input with type-ahead, and remembering which sections were open, are the
two halves that rule is holding back.

What is settled if it is picked up: **badges are available today** — `assignments` (231 of 263), `scope` (93),
`platforms` (73), `odataType` (137), plus the facet memberships the filter already renders. The per-item
**summary is not**: it is present on **0 of 263** and **0 of 148** resources, because `GenerateIndex` reads it
from each document's frontmatter and generated documents write only `source` and `generatedAt`. The index
schema and the CLI plumbing are both correct, so that half is gated on a documentation **regeneration** whose
template emits `summary:`, not on a change here — build it to render no second line when the field is absent
and it lights up on its own. Remembering which sections were open across navigations stays **excluded on
purpose** for as long as the no-client-side-JavaScript rule stands: that needs client-side state.

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
data change plus one nesting level; the choice belongs in the URL as a query parameter, and stays there even if
the no-client-side-JavaScript rule is relaxed (its own idea below) — a spine chosen by a widget would not be
addressable, which is a property worth keeping on its own merits.

### Idea: Search across a tenant's documents

Full-text search, or filtering by resource type and name, over everything a tenant has (including the
resource tree). **Parked** because it is the largest single feature on this list and cannot be done well
without an index and, realistically, client-side interaction — and no client-side JavaScript is a
non-negotiable. **Revisit** when the corpus is large enough that the sidebar tree stops being navigable even
with the shipped taxonomy filters, or if a server-rendered query page turns out to be enough. It subsumes the
name filter in the sidebar idea above. This is the **first feature that would justify relaxing the rule** (its
own idea below): a server-rendered `GET /:tenant/_search?q=` page is worth trying first, because it needs no
script at all and would show whether the interactive version is wanted.

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
e.g. for a presentation or a screenshot, or if the no-client-side-JavaScript rule is relaxed (its own idea
below) — a toggle that stores the choice client-side is the cheapest thing that relaxation would buy, and it
needs no route and no state on the server.

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
after a per-type listing page exists for another reason, when this becomes additive and nearly free, or if the
no-client-side-JavaScript rule is relaxed (its own idea below), which removes the reason for the new route
entirely: the segment could then open and scroll to its own sidebar section.

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
the no-client-side-JavaScript rule forbids (relaxing it has its own idea below, and would remove only this one of
the four obstacles), every route today is scoped to one `:tenant`, and it is not settled whether the comparison
belongs here at all rather than in the CLI, which holds the facts (`resources/metadata.yaml`, the per-resource
hashes) that make a semantic diff cheap. **Revisit** when a stage/prod tenant pair is actually exported side by
side into one docs root, and once there is an answer for cross-tenant identity — a normalisation of tenant-local
ids that can be stated and tested, not guessed per resource type.

### Idea: Further export formats and partial exports

Single-file HTML, DOCX, PDF via a print stylesheet, a Markdown bundle; and exporting one type, one document
or only the summary. **Parked** deliberately: the Confluence exporter is whole-tenant only, and the
`src/docs/export/` seam exists so a second format is a second `ExportService` method plus its own format
module, with the controller and the serialiser untouched. **Revisit** per format when someone actually needs
it. Preserving the document tree in an export is not expressible through Confluence HTML import at all —
re-parenting by hand or the REST API are the only routes. Where each of these would be offered is already
settled, including why the partial exports are the exception: see *Export entry points live on the tenant
picker*.

### Idea: One export button per tenant, leading to an export page with per-format options

Replace the per-format download links on the tenant picker with a **single** *Export* link per tenant card,
pointing at a new HTML page (`GET /:tenant/_export`) that lists the available formats and lets the operator set
that format's own options before downloading. First concrete option: the **Confluence index format** — group the
overview's page list by resource type, by a taxonomy axis, or both. **Parked** because there is exactly one
format today, and its one real option is settled per *server* rather than per request: the scheduled axis-index
entry above puts that choice in the `EXPORT_INDEX` environment variable, so the page would still be a route, a
view and a control for a single button that already works. **Revisit** the moment a second whole-tenant format
appears, or a reader needs two different exports of the same tenant from one server — which is exactly what an
environment variable cannot express, and the honest trigger for making the choice per request.

What is settled if it is picked up. **The download route keeps its shape**: `GET /:tenant/_export/:format`
streams the archive, options ride as query parameters on it, and every option has a default so a bookmarked
option-less URL keeps producing today's export. The new page is the *only* addition, at the bare `_export`
prefix, declared before the document catch-all like its sibling. Options are **validated the way
`parseFacetSelection()` validates a selection** — an unknown or malformed value falls back to the default rather
than 404ing — and each is a named choice with a stable id, so a chosen variant has exactly one URL. The page is
also the honest place for the things currently crammed onto a picker card: the one-way-publish caveat stated once
per format, and, cheaply, what the export will contain (page count, pending documents, an incomplete index),
all index-derived.

One open question, plus one that is now settled elsewhere. **How the controls are expressed** is undecided, with
three candidates. **(a)** A plain
`<form method="get" action="/:tenant/_export/confluence">` with radio buttons and a submit button — pure HTML,
no script, still read-only, and it scales to several options; it costs the app's first form and first non-anchor
control, and a form cannot carry `download`, so attachment behaviour rests on the `Content-Disposition` header
`ExportService` already sets. **(b)** One `<a download>` per option combination ("Confluence, index by type",
"Confluence, index by axis") — keeps the anchor-only shape the standing decision names, but is combinatorial as
soon as a format has two options. **(c)** Anchors first, with the GET form named as the sanctioned escape hatch
once a format carries more than one option, and the query-parameter shape fixed up front so swapping the control
changes no URL. Note that **(a) does not conflict with the no-client-side-JavaScript rule** — that rule bans
shipped script and client-side state, not HTML controls; the objection to it is precedent, not compliance.
Second, the axis-grouped index no longer poses an open question: the scheduled axis-index entry above settles it
as the `EXPORT_INDEX` environment variable (`type` | `both` | `axis`, default `type`). If this page is built, the
same three ids become that format's query parameter with `EXPORT_INDEX` as its default, so an option-less URL
keeps producing what the operator configured — which makes the index format a *migration* of an existing choice
rather than the first option, and moves the trigger for the page to a second format or a per-request need. Note
that because the variable defaults to today's by-type index, a reader who wants the axis view on a server
configured for `type` is the most likely first caller for this page.

This amends *Export entry points live on the tenant picker*: the standing decision already names
`GET /:tenant/_export` as the answer when a row of format links stops fitting, and this makes per-format
options a second, earlier trigger for the same page. What the decision requires is unchanged — the picker stays
the entry point for whole-tenant scope, the link is a plain anchor with no nesting, the caveat travels with it,
and dark mode plus a visible `:focus-visible` outline apply to whatever control the page ends up using.

### Idea: Drop the no-client-side-JavaScript rule

Lift the non-negotiable that this app ships no script, so the features currently blocked by it become possible.
It is stated in `.windsurf/rules/01-architecture.md` (*"No client-side JavaScript. Everything is
server-rendered"*), repeated in `02-style-and-quality.md` (*"Do not introduce client-side JavaScript or a
frontend framework"*) and claimed in `README.md`'s Frontend section. **Parked** because it is a rule change, not
a feature: nothing is unblocked until a specific blocked feature is actually wanted, and every one of them is
itself parked. **Revisit** when a feature someone has asked for cannot be built server-side at acceptable cost —
tenant-wide search is the honest candidate — or when the alternative has become visibly worse than the script
would be (a `:target` hack, a route invented only to compensate, a workaround nobody can explain).

**Note what the rule does and does not forbid.** It bans *shipped script* — no `<script>`, no bundler, no
framework, no client-side state. It does not ban HTML interactivity: `<details>`/`<summary>`, `:target`,
`:focus-visible`, `prefers-color-scheme` and a `<form method="get">` are all in bounds today. Several things that
feel scripted are already legal without it.

**Where the rule earns its keep.** Server-only rendering is a real benefit in its own right and not merely the
absence of a client — a document arrives complete in the first response, so there is no loading state, no
hydration, nothing to re-render and nothing that can fail after the HTML has landed — but it is a *separate*
claim from banning script, and only the reasons below argue for the ban rather than for server rendering.
*The corpus is documents.* A configuration document has to survive printing, archiving, a text browser and a
saved copy; server-rendered HTML is that durable form, and anything only a runtime can assemble is not.
*It has no client toolchain.* The only build step is Tailwind's CSS pass — no bundler, no framework dependency
tree, no client supply chain to audit or keep current in a tool that renders tenant configuration. *It is
trivially auditable.* "This app ships no script" is a claim an operator can verify at a glance, and it composes
with the read-only rule to make the browser obviously inert; note that it is **not** an XSS boundary, because
`markdown-it` runs with `html: true` and although the CSP now refuses to run script, raw HTML from the docs
root still renders, so the real boundary is the trust placed in the docs root either way. *It forces state into
the URL.* Every view — including a filtered sidebar — is addressable,
bookmarkable, shareable and reproducible, and the whole test suite can therefore be `supertest` against server
HTML rather than a browser harness. *It caps complexity*: one rendering path, and no logic duplicated across
server and client.

**What it has already cost.** Working machinery exists purely to route around it: `toggled()` and
`selectionHref()` rebuild the entire facet selection into every chip and every document link, so a 263-item tree
re-renders server-side on each click; the `exempt` flag plus the `matched`/`total` reconciliation exist so a
filter cannot hide the page you are on; `NavSection.active` exists to render one `<details open>`; `lineAnchors()`
plus `.line:target` deliver `#L42`; `shiki` emits dual-theme output so dark mode can stay
`prefers-color-scheme`; and `findings-table.ts` already tags rows with `data-severity` for a filter that cannot
be built yet. Six parked ideas are blocked or deformed by it: **search across a tenant's documents** (the largest
item on this list, parked squarely on it), **a name filter and per-item context in the sidebar** (no type-ahead
without script, and remembering which sections were open is excluded on purpose), **an actionable findings
block** (expressible only as sibling anchors plus `:target`, which spends the URL fragment `#findings` already
owns and needs a *Show all* reset as part of the feature), **clickable breadcrumb segments** (CSS cannot open a
collapsed `<details>` from an anchor, so it needs a whole new route and view), **an explicit dark-mode toggle**,
and **tenant diff** (a readable diff of a 317-setting document needs interaction). The export standing decision
inherits it too: *no dropdown, no picker widget*.

**Not a binary decision, if it is picked up.** Three tiers, all open. **(a) Keep it** and pay the workaround
cost knowingly, which is the status quo. **(b) Progressive enhancement only**: a small dependency-free script
served from `public/`, no bundler and no framework, allowed only to improve something that already works without
it — remembering open sections, toggling a chip without a round trip — with every page still fully functional
with script disabled, and the rule rewritten as *"the server renders everything; script may only enhance"*
rather than deleted. Either tier also means widening `SECURITY_HEADERS`' `script-src` in the same edit — today
it is unnamed (falls to `default-src 'none'`), and it would need to name the script's own origin. **(c) Full
lift**: a real client bundle for search and diff, which brings a build step, a dependency tree and a second
rendering path, and turns the testing story into a browser harness.

**What has to change with it, whichever tier wins.** Both rule files and the README Frontend section, in the
same edit — the rule is quoted in enough places that a half-removed version would be worse than either state —
plus a `CHANGELOG.md` entry, because a page that needs script to work is operator-visible. The many released
changelog entries that boast *"still no client-side JavaScript"* are history and stay as written. Two invariants
must be restated rather than dropped by accident: state belongs in the URL (so views stay addressable), and no
route may mutate anything under the docs root, however the client is built.

### Idea: Clear the ESLint suppressions baseline

Pay off the 16 findings in `eslint-suppressions.json` — the ones that existed in 9 files when the sonarjs rules
were switched on — a rule at a time, pruning after each, until the file, the `lint:baseline` / `lint:prune`
scripts and the paragraphs describing them can be deleted. **Parked** because the baseline already delivers the
property that mattered: every rule stays enabled, `npm run lint` and both readiness gates are usable, and new
code — including new code in the baselined files — is held to the full set, since one more violation of a
baselined rule there exceeds the recorded count and reports. What is left is latent, not broken: the regexes
backtrack over the operator's own generated export tree in a read-only app, and the complex functions are the
ones whose shape the score measures rather than their risk. **Revisit** when a baselined file is being touched
for another reason (pay its entry off in the same edit, which is how this shrinks without a campaign), when a
render actually turns out slow on a pathological document, or if the baseline ever stops shrinking — that would
mean it has become a place where findings accumulate, which is the one thing it must not be.

What is settled if it is picked up. It is a **debt ledger, not a policy**: nothing in it is a rule this project
disagrees with, and none of it is exempted in `eslint.config.mjs` — the rules switched off there for `test/` are
a separate, permanent parity decision and are not part of this. Work rule by rule and run `npm run lint:prune`
after each, so the diff shows what was paid off. Only `sonarjs/super-linear-regex` (6: `page-name.ts` ×2,
`findings-table.ts` ×2, `link-rewrite.ts`, `section-hooks.ts`) changes behaviour — these patterns run over
generated documents, so each rewrite needs a spec case pinning the same accepted and rejected inputs. The rest
are refactors the existing suite covers and are internal, carrying no `CHANGELOG.md` entry unless a reader sees
a difference: `cognitive-complexity` (4: `confluence.ts`, `export/html-allowlist.ts`, `section-hooks.ts`,
`tenant-index.ts`), split along the seams those functions already have and keeping the pure/Nest-free split
intact; `misplaced-loop-counter` in `page-name.ts`, a `while` written as a `for`; `no-nested-template-literals`
in `confluence.ts`; and `prefer-specific-assertions` in two specs. Two are decisions rather than fixes, and
either way they move **out** of the baseline: `updated-loop-counter` in `findings-table.ts` (the scan assigns
`i = close` to skip a matched table's body — deliberate and documented in place) and `no-os-command-from-path`
in `scripts/working-tree-clean.js` (the preflight resolves `git` through `PATH`); if accepted, each becomes an
`eslint-disable-next-line` at its site, because a baseline must not be where a standing choice hides. Deleting
the file and the two scripts is the last step and is the only operator-visible part, so that one does get a
`CHANGELOG.md` entry.

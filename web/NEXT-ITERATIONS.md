# Next iterations

Outstanding work, standing decisions and parked ideas for the docs browser. `README.md` describes what it does
today and `CHANGELOG.md` records what shipped; neither is repeated here.

The *Fixes* below are scheduled: small, self-contained corrections that need no design work, listed so they
are not forgotten between features. Every idea in *Parked ideas* is deliberately unscheduled: picking one up
means promoting it into a numbered work entry with a `**Goal.**` and a `**Plan.**`, reconciling its rationale
against what is true at that point rather than copying it across.

## Fixes

Each is a numbered work entry in its own right; none touches a non-negotiable (read-only, no client-side
JavaScript, one `markdown-it` instance, path safety) and none depends on a documentation regeneration. Each
carries its own e2e or spec case and a `CHANGELOG.md` entry under `[Unreleased]`; purely internal ones say so.

A **struck-through** title or plan item has shipped and its `CHANGELOG.md` entry is written. It stays here,
struck, until the branch is closed and the release is cut — that is when the entry is deleted, not the moment
the code lands.

### ~~1. Security headers, including a CSP that enforces the no-script rule~~

**Goal.** Every response carries the baseline hardening headers, and the *no client-side JavaScript* rule is
enforced by the browser rather than only promised by the README.

> **Today.** No response carries a `Content-Security-Policy`, `Referrer-Policy` or `X-Frame-Options` header;
> `X-Content-Type-Options: nosniff` is set by hand on exactly two responses (the `?raw` YAML in
> `docs.controller.ts` and the export zip in `export/export.service.ts`). The parked *Drop the
> no-client-side-JavaScript rule* idea below says in so many words that "there is no CSP header". Everything
> the app wires into Express lives in `configureViews()` in `src/configure-app.ts`, which both `main.ts` and the
> e2e suite call — so that is the one place a header middleware belongs. No new dependency: Express `app.use()`
> and `res.setHeader()` are enough.
>
> **What the pages actually load**, verified in the source, which fixes the policy: one stylesheet (`/app.css`,
> same origin); inline `style="--shiki-light:…;--shiki-dark:…"` attributes on every token of the YAML view
> (`yaml-highlighter.service.ts` renders with `defaultColor: false`), so `style-src` **must** carry
> `'unsafe-inline'`; `data:` SVG icons via CSS `mask` for the severity and section icons in `src/styles.css`
> (mask fetches are image loads, so `img-src` **must** allow `data:`); `/favicon.svg`; and whatever image a
> generated document embeds (`https:` at most). No fonts, no `fetch`, no frames, no forms, no media, no
> workers — so `default-src 'none'` is safe and is what makes `script-src` fall to none without being named.
>
> **Policy (exact header values).**
> `Content-Security-Policy: default-src 'none'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:;
> frame-ancestors 'none'; base-uri 'none'` — `X-Content-Type-Options: nosniff` — `Referrer-Policy: same-origin`
> — `X-Frame-Options: DENY` (redundant with `frame-ancestors` for modern browsers, kept for older ones). The
> headers go on **every** response, including JSON, raw YAML, the zip, redirects, 404s and static assets: they
> are harmless where irrelevant and a single unconditional middleware is the whole point. Still read-only,
> still no script; this changes headers only. Not an XSS boundary — `markdown-it` keeps `html: true` — but the
> browser now refuses to run a script even if one arrived from the docs root.

**Plan.**

- ~~In `src/configure-app.ts` add `export const SECURITY_HEADERS: Readonly<Record<string, string>>` with the four
  header/value pairs above (the CSP on one line, directives separated by `; `), with a *why* comment naming
  what each CSP directive permits and for which page element. Then, as the **first** statement of
  `configureViews()` — before `app.useStaticAssets()`, so `/favicon.svg` and `/app.css` are covered too — add
  `app.use((_req, res, next) => { for (const [name, value] of Object.entries(SECURITY_HEADERS)) res.setHeader(name, value); next(); })`.
  Type the callback parameters with `Request`, `Response`, `NextFunction` from `express` (already a dependency).
  Update the function's comment: it wires the view engine, static assets **and the security headers**. Leave
  the two existing per-route `X-Content-Type-Options` calls alone; they now duplicate the global value.~~
- ~~e2e, in `test/docs.e2e.spec.ts`: one new case *every response carries the security headers* with a local
  helper `expectSecurityHeaders(res)` asserting the **literal** values above for all four headers (not the
  exported constant — the test must fail if the constant is wrong). Apply it to: `GET /` (picker),
  `GET /healthz` (JSON), `GET /favicon.svg` (static — proves the middleware runs before `express.static`),
  `GET /mytenant/Microsoft.Graph/groups/g1` (document), `GET /mytenant/_resource/Microsoft.Graph/deviceManagementConfigurationPolicies/p1`
  (YAML view) and the same with `?raw`, `GET /mytenant/_export/confluence` (zip; use `.buffer().parse(binaryParser)`
  like the existing export cases), and `GET /nope-tenant` (a 404). No fixture changes.~~
- ~~Manual step, run against a real export with DevTools Console open: headers confirmed present with the
  exact values (`curl -D -`, since the browser's own `fetch` is itself subject to `connect-src` and cannot be
  used to check them); a document's section icons render (`::before` mask set on each `h3.doc-section-heading`);
  a findings table's severity icons render (`::before` mask set on each severity cell); the YAML view's inline
  `--shiki-light`/`--shiki-dark` colours compute correctly in both light and dark scheme. No
  `securitypolicyviolation` event fired on any of them. No directive needed widening.~~
- ~~README: in **Security**, after the path-safety bullets, a short paragraph *Response headers* listing the
  four headers with their values and one clause per CSP directive saying what it permits and why (stylesheet
  + shiki inline colours; same-origin and `data:` icons and `https:` document images; no framing; no scripts by
  omission). In the **Features** list, the *No client-side JavaScript* bullet gains "…and a `Content-Security-Policy`
  that lets no script run". In the **Layout** tree, the `configure-app.ts` line becomes
  `# hbs view engine + static assets + security headers (shared with e2e tests)`. Make the same one-line change
  to the `configure-app.ts` entry in `.windsurf/rules/01-architecture.md`.~~
- ~~In this file, the parked *Drop the no-client-side-JavaScript rule* idea: replace "and there is no CSP header,
  so the real boundary is the trust placed in the docs root either way" with "and although the CSP now
  refuses to run script, raw HTML from the docs root still renders, so the real boundary is the trust placed
  in the docs root either way"; and add one sentence to its tier (b)/(c) paragraph that relaxing the rule means
  widening `script-src` in the same edit.~~
- ~~`CHANGELOG.md`, `[Unreleased]` → `### Added` → `#### The browser`: one bolded lead-in (*Every response
  carries security headers, including a Content Security Policy that lets no script run*) plus two or three
  sentences — which headers, that the policy is derived from what the pages actually load (stylesheet, shiki's
  inline colours, `data:` icons, document images) and is listed in the README, and that this makes the
  no-client-side-JavaScript rule browser-enforced while the app stays read-only and ships no script. Then
  strike this entry's plan items and title.~~

### 2. Either wire ESLint or drop the dead `eslint-disable` comments

**Goal.** The source contains no directives for a tool that is not configured.

> `main.ts` and `configure-app.ts` carry `eslint-disable` comments; the README and the rules say there is no
> lint script. Either state is fine; the mismatch is not.

**Plan.**

- Preferred: add `eslint` + `typescript-eslint` with a minimal flat config and an `npm run lint` script, run it
  in `release-ready`, and update the README *Development conventions* and `02-style-and-quality.md`
  (which currently say lint is not wired). A `CHANGELOG.md` entry, since it adds a script.
- Otherwise: delete the two comments (internal, no entry).

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

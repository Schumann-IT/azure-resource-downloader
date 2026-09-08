# TODO — backlog review

Snapshot taken 2026-09-08 from `go/NEXT-ITERATIONS.md` and `web/NEXT-ITERATIONS.md`. Those two files stay
authoritative; this is a cross-project reading of them. Both hold parked ideas only — the scheduled web fixes
that this file once planned have all shipped and are recorded in `web/CHANGELOG.md`.

## 1. Backlog overview

### Go CLI (`go/NEXT-ITERATIONS.md`)

Scheduled work: **none**. Parked ideas only:

- **Per-finding severity in `Security` sections** — tag each callout `[risk]` / `[review]` / `[ok]` so the web
  can colour or filter. Parked: subjective model-only judgement, made 400+ times, unvalidatable.
  **Regeneration-gated** (touches all seven templates).
- **Resolve Graph ids to names inside the YAML** — `groupId: <guid> (name)` at export time. Parked:
  `docs generate-prompt` already resolves incrementally via splice blocks; embedding a name turns a fact into a
  decision and moves `sourceSha256` on every group rename. Revisit only for a non-doc consumer of `resources/`.
- **Bootstrap taxonomy from per-document LLM suggestions** — harvest programme labels into
  `docs/taxonomy-suggestions.yaml`. Parked: taxonomy is small today. **Regeneration-gated** (suggestion
  instruction lives in per-type templates).

### Web docs browser (`web/NEXT-ITERATIONS.md`)

Scheduled work: **none**; the *Fixes* section reads *None outstanding.*

Standing decision: whole-tenant exports live on the picker card as plain `<a download>`; partial exports live
next to the thing exported; Confluence REST sync needs POST and is therefore not offered at all.

Parked ideas, grouped:

- **Presentation**: per-document identity on `<article>`; summary TOC; syntax highlighting inside documents;
  explicit dark-mode toggle.
- **Navigation**: name filter + per-item context in sidebar; sidebar spine by taxonomy axis; clickable
  breadcrumb segments; resource landing page; two-group nav tree; browsable excluded bulk types;
  multi-segment tenants.
- **Findings**: actionable findings block (severity filter via `:target`).
- **Search / diff**: tenant-wide search; tenant diff.
- **Export**: further formats + partial exports; media / YAML as attachments; Confluence REST sync; single
  export button → `GET /:tenant/_export` options page.
- **Infra**: watch-based cache invalidation.
- **Rule change**: drop the no-client-side-JavaScript rule (tiers: keep / progressive enhancement / full lift).

## 2. Inter-project dependencies

Reassessed 2026-09-08 against the code, not the backlog prose: each claim below was checked in
`go/internal/docs/` (`generateindex.go`, `generateprompt.go`, `taxonomy.go`, `generate_prompt_template.md`) and
`web/src/docs/tenant-index.ts`. Intra-project couplings that the authoritative backlogs already state in full
(the no-JS rule's blast radius, the export chain, the CSP/`script-src` coupling) are **not** repeated here — they
drift when copied.

### The one hard dependency, and it is tracked by neither backlog

**Go's prompt template never asks for `summary:`.** The plumbing on both sides is complete: `docFrontmatter`
has a `Summary` field, `GenerateIndex` copies it into each `index.yaml` resource, and the web renders it as
per-item context when present. But the template's *Frontmatter (required)* section lists `source`,
`sourceSha256`, `promptSha256`, `platformGroup`, `functionGroup`, `generatedAt` — and nothing else. The web
backlog's "0 of 263 and 0 of 148 resources carry it" is therefore **by construction**, not model behaviour, and
the web idea *name filter and per-item context* is blocked on a Go change that `go/NEXT-ITERATIONS.md` does not
list. **Recommendation:** add it to Go's backlog as a regeneration-gated item (one frontmatter line in the
template plus its rule text), explicitly batched with the two existing regeneration-gated ideas — it must not
ship alone, because any template edit moves `promptSha256` for every type.

A second finding on the same evidence: the template already **requires** `platformGroup` / `functionGroup`
(chosen from each type's `<!-- doc-groups -->` marker), yet the web reports both **empty in both reference
exports**. So the reference exports predate the current template. The next regeneration fills them with no
further Go change, and the web's existing badge rendering lights up on its own.

### Regeneration is the single cross-project lever

Four things ride the next documentation regeneration, and only one is currently written down on the Go side as
work: `summary:` (above — **not tracked**), refreshed `platformGroup`/`functionGroup` (free), *per-finding
severity* (Go parked) and *taxonomy bootstrap* (Go parked). The Go-internal rule stands — promoting any one
obliges surveying the others, because the cost (a full regeneration of every non-`record` type) is paid once —
and *taxonomy bootstrap* must ride the non-hashed `docs/generate.md`, never a per-type `doc-prompt.md`, or it
couples a cheap taxonomy edit to that expensive regeneration.

### Facets and taxonomy (Go → web) — verified compatible

- Go emits **`index.yaml` version 4**: `facets` is the sole grouping surface; the transitional `programmes` /
  `groups` mirrors of v3 are gone. The web accepts any version ≥ 1, reads `facets` first and synthesises a
  single programme axis from `programmes` only for a v2 index, so v4 is read correctly today. The web backlog's
  *sidebar by taxonomy axis* still says "v3" — wording only, harmless, fix at next edit.
- A **function-shaped spine needs no Go change**: `TaxonomyConfig` accepts arbitrary `axes`, each value matched
  by rules over the exported facts (`name`, `type`, `odataType`, `platforms` as regex; `scope` exact). A
  `function` axis is an operator-authored config section, not a feature. Go's *taxonomy bootstrap* would help
  author it from evidence, which is its only bearing on the web spine — it enlarges the *values* an operator can
  promote, not the axis mechanism.
- `functionGroup` as the alternative spine carrier is weaker than the config axis: it is single-valued,
  label-only and LLM-chosen, whereas an axis is id-bearing, ordered by the header and deterministic — the
  properties the web idea says a spine needs.

### Mutual waits with no owner

- **Per-finding severity.** Go revisits "when the web side needs per-item severity"; the web's *actionable
  findings block* revisits "when the findings table stops being readable in one pass" (15 rows today). A
  deadlock by design, and a correct one while the table is short. The trigger belongs to the **web** side
  (reader demand), and it is regeneration-gated on the Go side, so it queues behind the lever above.
- **Tenant diff.** Ownership is explicitly undecided: Go holds the facts that make a semantic diff cheap
  (`metadata.yaml`: `sourceSha256`, `resourceId`, `displayName`, assignment targets per resource), the web holds
  the view and every route is single-`:tenant`. The shared prerequisite — a **stateable, testable normalisation
  of tenant-local identity** (GUIDs, group ids, names) — is work neither backlog lists. Until a stage/prod pair
  is actually exported into one docs root, leaving it unowned is the right call.

### One-directional, verified stable

- **Browsable excluded bulk types** (web) rests on Go's `counts.excluded` (emitted per type) and the in-scope
  rule that documents a group only when an assignment references it (`inScope` in `generateindex.go`). Both are
  stable contracts; if picked up it is web-only work.
- **Resolve Graph ids in YAML** (Go) is **not** a dependency, despite its revisit trigger naming "a consumer of
  `resources/` other than the doc pipeline". The web YAML view is such a consumer and shows raw GUIDs **by
  design** — the two backlogs made the same choice from opposite ends (names belong in the documentation, facts
  stay facts). Nothing to reconcile.

### Housekeeping in the authoritative files

- `web/NEXT-ITERATIONS.md` still cites "the scheduled axis-index entry above" twice (in *one export button per
  tenant*); that entry shipped as `EXPORT_INDEX` and is gone. Rephrase at next edit — describe the work, do not
  cite an entry.
- Same file, *sidebar by taxonomy axis*: "v3" → v4 (see above).

### Observations

- Neither project has scheduled work left; both lint configurations (`go/.golangci.yml`, `web/eslint.config.mjs`)
  are committed and gate merges, so the next branch starts from a clean baseline.
- The **regeneration is the lever**, and the `summary:` template gap is the one concrete, cheap Go change waiting
  to be scheduled for it. It is also the only cross-project blocker where one side is finished and the other side
  has not written the work down — the gap this reassessment exists to catch.
- The **no-JS rule** stays the pivotal web decision; the CSP now hardens it and roughly a third of parked web ideas
  wait on relaxing it. Its full blast radius is stated in the web backlog and is not repeated here.

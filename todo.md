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

## 2. Dependencies

### Cross-project (Go → web)

- **Per-item summary in the sidebar** (web *name filter and per-item context*) is gated on a Go
  **regeneration** whose template writes `summary:` frontmatter — today 0 of 263 resources carry it. Web
  plumbing is ready; the blocker is Go.
- **Per-finding severity** (Go parked) exists solely to serve web colouring/filtering; web's *actionable
  findings block* already tags `data-severity` at tenant-summary level. Go's revisit condition is "the web side
  needs per-item severity" — mutual wait.
- **Sidebar by taxonomy axis** (web) consumes the v3 `index.yaml` `facets` registry Go emits; a
  function-shaped spine needs a Go-side operator-authored axis or a populated `functionGroup`. Go
  **taxonomy bootstrap** work would enlarge the axes the web spine depends on.
- **Tenant diff** (web) is explicitly undecided on *which project owns it*: Go holds the facts
  (`metadata.yaml`, hashes); web holds the view. Cross-tenant identity normalisation is unresolved on both sides.
- **Browsable excluded bulk types** (web) depends on Go's `counts.excluded` and the "groups only documented
  when referenced" rule.
- **Resolve Graph ids in YAML** (Go) — its revisit trigger is "a consumer of `resources/` other than the doc
  pipeline"; the web YAML view arguably is one, and shows raw GUIDs by design today.

### Go-internal

- **Batching**: both regeneration-gated ideas (*per-finding severity*, *taxonomy bootstrap*) must ride the
  **same** regeneration; promoting one obliges surveying the other. Any future template change is the natural
  carrier for both, plus the `summary:` frontmatter the web is waiting on.
- **Taxonomy bootstrap** must ride the non-hashed `docs/generate.md`, never `doc-prompt.md` — hard coupling to
  the hash design in `internal/docs/generateprompt.go`.

### Web-internal

- **The CSP** (`SECURITY_HEADERS` in `src/configure-app.ts`) is the enforcement mechanism for the no-JS rule
  and conflicts with **Drop the no-JS rule**; relaxing to tier (b)/(c) means widening `script-src` in the same
  edit — today it is unnamed and falls to `default-src 'none'`.
- **Drop the no-JS rule** blocks or deforms six ideas: search, name filter, actionable findings, clickable
  breadcrumbs, dark-mode toggle, tenant diff. Search is the honest first trigger; try a server-rendered
  `GET /:tenant/_search` first.
- **Search** subsumes **name filter in sidebar**.
- **Clickable breadcrumbs** become nearly free once a per-type listing page exists.
- **Two-group nav tree** is a fallback only if **resource landing page** fails to make the YAML switcher
  discoverable; **resource landing page** is also the carrier for **browsable excluded bulk types**.
- **Export chain**: further formats → single export button / `_export` page; attachments need a third served
  root (path-safety design change); REST sync requires abandoning read-only.
- **Stale reference**: several web parked ideas cite "the scheduled axis-index entry above"; that entry has
  shipped (`EXPORT_INDEX`) and is gone — rephrase at next edit per the "describe the work, don't cite §N" rule.

### Observations

- Neither project has scheduled work left; both lint configurations (`go/.golangci.yml`, `web/eslint.config.mjs`)
  are committed and gate merges, so the next branch starts from a clean baseline.
- The most leveraged single item is a **Go template regeneration**: it unlocks web per-item summaries and is
  the only sane moment to fold in both Go regeneration-gated ideas.
- The **no-JS rule** is the pivotal web decision — the CSP now hardens it; roughly a third of parked web ideas
  wait on relaxing it.

# TODO — backlog review and web fix plan

Snapshot taken 2026-09-08 from `go/NEXT-ITERATIONS.md` and `web/NEXT-ITERATIONS.md`. Those two files stay
authoritative; this is a working plan on top of them.

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

Scheduled fixes (all self-contained, none touching a non-negotiable). Numbers follow `web/NEXT-ITERATIONS.md`
after Branch D closed (the former 1 — security headers/CSP — shipped and was removed; the former 2 is now 1):

1. Wire ESLint or delete the dead `eslint-disable` comments

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
- **Fix 1 (ESLint)** touches `release-ready`, which shares parsing with `branch-ready`
  (`web/scripts/lib/changelog.js`).
- **Stale reference**: several web parked ideas cite "the scheduled axis-index entry above"; that entry has
  shipped (`EXPORT_INDEX`) and is gone — rephrase at next edit per the "describe the work, don't cite §N" rule.

### Observations

- Go has zero scheduled work; web has one small fix left, ready without design (four shipped: two in Branch A,
  one in Branch B, one in Branch C, one in Branch D).
- The most leveraged single item is a **Go template regeneration**: it unlocks web per-item summaries and is
  the only sane moment to fold in both Go regeneration-gated ideas.
- The **no-JS rule** is the pivotal web decision — the CSP now hardens it; roughly a third of parked web ideas
  wait on relaxing it.

## 3. Web fixes — branch plan

Grouping principle: one branch per layer touched, so each has one test surface and one CHANGELOG area. Effort
scale: XS < S < M.

### Branch A — `fix/web-ui-polish` (former fixes 4, 5, 6) — **DONE**

Templates + `styles.css` only; no service or route change. Test surface: `docs.e2e.spec.ts` +
`styles-build.spec.ts`. One CHANGELOG `### Fixed` group under *Views and navigation*. Branch closed: the three
entries were deleted from `web/NEXT-ITERATIONS.md` and the survivors renumbered 1–5.

- [x] `aria-label="Tenant navigation"` on `<aside>` in `views/partials/sidebar.hbs`.
- [x] Header width as a partial parameter (`views/partials/header.hbs`): picker/error `max-w-5xl`,
      document/tenant/resource `max-w-7xl`.
- [x] Print stylesheet — `@media print` in `src/styles.css`; `styles-build.spec.ts` assertion; README sentence
      about collapsed `<details>` printing collapsed.

### Branch B — `fix/web-startup-and-404` (former fixes 2, 3) — **DONE**

Both correct the two non-happy paths; both touch `README.md`. Branch closed: the two entries were deleted from
`web/NEXT-ITERATIONS.md` and the survivors renumbered 1–3.

- [x] `PORT` validation — pure `resolvePort()` in `src/port.ts` (unit tested in `test/port.spec.ts`), wired into
      `src/main.ts`; unset/empty falls back to `3000` silently, anything else falls back to `3000` with a note
      in the startup line. README configuration table + `.env.example` + `.windsurf/rules/01-architecture.md`.
- [x] 404 kinds — `notFound()` in `src/docs/docs.controller.ts` gained a `NotFoundKind`
      (`tenant` | `document` | `resource` | `export`); all call sites updated. `views/error.hbs` renders
      `{{headline}}`; body stays free of filesystem paths. e2e cases extended for unknown tenant, unknown
      document, missing resource and unknown export format.

### Branch C — `fix/web-picker-freshness` (former fix 1, alone) — **DONE**

Branch closed: the entry was deleted from `web/NEXT-ITERATIONS.md` and the survivors renumbered 1–2.
`npm run branch-ready` green (194 tests, build, no strikeouts, contiguous numbering).

- [x] Picker / `healthz` read counts and `generatedAt` through `getIndex(info)` — dropped `documented` /
      `pending` / `generatedAt` from `TenantInfo` (`src/docs/tenant-discovery.service.ts`); added a private
      `withIndex()` helper in `docs.controller.ts` (pairs each discovered tenant with a fresh `getIndex()` read,
      drops any that turned unreadable) and rewired `healthz()` and `picker()` onto it; no other `TenantInfo`
      consumer (`src/docs/export/`) touched those fields. `views/picker.hbs` needed no change. e2e case added:
      regenerates a fixture's `index.yaml` counts and `generatedAt` and asserts `/` and `/healthz` reflect them
      without waiting out the TTL. README (*No-restart refresh* feature bullet + `/healthz` routes row) and
      CHANGELOG (`### Fixed` → `#### The browser`) updated in the same edit.

### Branch D — `chore/web-security-headers` (former fix 1, alone) — **DONE**

Branch closed: the entry was deleted from `web/NEXT-ITERATIONS.md` and the survivor renumbered to 1.

- [x] `SECURITY_HEADERS` middleware in `src/configure-app.ts` (first statement of `configureViews()`, ahead of
      static assets), sent on every response: `Content-Security-Policy: default-src 'none'; style-src 'self'
      'unsafe-inline'; img-src 'self' data: https:; frame-ancestors 'none'; base-uri 'none'`,
      `X-Content-Type-Options: nosniff`, `Referrer-Policy: same-origin`, `X-Frame-Options: DENY`. Policy derived
      from what the pages actually load — shiki's inline `--shiki-light`/`--shiki-dark` vars need
      `'unsafe-inline'`; the `data:` SVG mask icons need `img-src data:`; nothing else is loaded, so
      `default-src 'none'` leaves `script-src` unnamed. e2e case in `test/docs.e2e.spec.ts` asserts the literal
      header values on the picker, JSON, static favicon, a document, the YAML view (html and `?raw`), the
      export zip and a 404. Manually verified against a real export with DevTools open (`curl -D -` for exact
      header values, since the browser's own `fetch` is itself blocked by `connect-src`): section-heading icons,
      findings-table severity icons and the YAML view's inline colours all render with no
      `securitypolicyviolation` event — no directive needed widening. README (*No client-side JavaScript*
      feature bullet, new *Response headers* subsection under Security, Layout tree) and
      `.windsurf/rules/01-architecture.md` updated in the same edit; CHANGELOG under `### Added` → `#### The
      browser`.

### Branch E — `chore/web-eslint` (fix 1, alone)

- [ ] **1** Preferred: add `eslint` + `typescript-eslint`, minimal flat config, `npm run lint`, hook into
      `release-ready` (and `branch-ready`), update README *Development conventions* +
      `.windsurf/rules/02-style-and-quality.md`, CHANGELOG entry — **M** (the cost is triaging first-run
      findings). Alternative: delete the two `eslint-disable` comments in `src/main.ts` and
      `src/configure-app.ts` — **XS**, internal, no CHANGELOG entry.

Kept separate: tooling/rules, and it modifies the scripts that gate the other branches.

### Order

1. ~~**A** `fix/web-ui-polish` — fastest win, zero risk.~~ Done.
2. ~~**B** `fix/web-startup-and-404` — small, self-contained.~~ Done.
3. ~~**C** `fix/web-picker-freshness` — interface change, review alone.~~ Done.
4. ~~**D** `chore/web-security-headers` — empirical verification.~~ Done.
5. **E** `chore/web-eslint` — last, so new lint findings don't churn the branches above and the gate exists
   before the next feature branch. **Next.**

(A and B were candidates for one merged branch; each shipped on its own instead.)

### Per-branch checklist

- Strike delivered plan items in `web/NEXT-ITERATIONS.md` (`~~…~~`), title too once the entry is complete; do
  not delete.
- `CHANGELOG.md` entry under `## [Unreleased]` in the same edit (except purely internal changes).
- Required test coverage per `02-style-and-quality.md` (e2e / spec / styles-build).
- `npm run branch-ready` green before merge; entries are deleted and renumbered only at branch close.

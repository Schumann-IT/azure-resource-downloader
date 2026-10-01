# azure-rd-docs-web

A **read-only browser** for the documentation that [`azure-resource-downloader`](../README.md) and its
documentation agent produce for an Entra ID / Intune tenant. It discovers tenant exports on disk, renders the
generated Markdown as HTML with the source YAML one click away, lets the reader slice the tenant along the
operator's taxonomy, packages a tenant for import into Confluence, and hands a drift report out as one PDF.

It is the last stage of the pipeline described in the [monorepo README](../README.md): `azure-rd download`
exports the tenant, an AI agent writes the documents, `azure-rd docs generate-index` writes `docs/index.yaml`
— and this app reads that tree. It **only reads**: nothing here calls Azure, writes a document or mutates the
export in any way. It is self-contained (its own dependencies, tests, changelog and version line) and shares
nothing with the Go CLI but the export tree on disk; `DOCS_ROOT` is the only coupling.

## Table of contents

- [What it does](#what-it-does)
- [Quick start](#quick-start)
- [Configuration](#configuration)
- [The docs root contract](#the-docs-root-contract)
- [Routes](#routes)
- [Taxonomy filters](#taxonomy-filters)
- [Confluence export](#confluence-export)
- [Drift view](#drift-view)
- [Drift report PDF](#drift-report-pdf)
- [Rendering](#rendering)
- [Security](#security)
- [Tests](#tests)
- [Project layout](#project-layout)
- [Development conventions](#development-conventions)
- [Known limitations](#known-limitations)

## What it does

- **Tenant discovery** — walks `DOCS_ROOT` (up to 3 levels deep) and treats any directory that contains a
  readable `docs/index.yaml` as a tenant. That file is the navigation index written by
  `azure-rd docs generate-index`; documents are resolved against the tenant's `docs/` folder.
- **Tenant landing page** — `GET /:tenant` renders `docs/summary.md`, the tenant-wide management summary the
  generation agent writes (posture, severity-ranked findings, coverage caveats), with an export header between
  its H1 and its prose: the index's `generatedAt`, its documented/pending/excluded counts and its
  completeness. The tenant drift page places the observation header the same way. The summary is optional: an export
  with no summary falls back to listing the index — resources grouped by type, with the LLM-authored one-line
  summary, a *pending* marker for resources with no document yet, and count-only assignment badges.
- **Sidebar navigation on every page** — one collapsible `<details>` per resource type, the section of the
  current document opened and the document marked, plus the tenant counts, export timestamp, the
  incomplete-export banner and the excluded bulk types. Collapsing is pure HTML. The tenant compare pages have
  no sidebar; their comparison pane lists the pairs instead (see [Tenant compare](#tenant-compare)).
- **Taxonomy filters** — when the index carries a taxonomy, the sidebar offers one chip group per axis the
  operator declared (Programme, Platform, Assignment scope, …) and narrows the tree server-side from query
  parameters, several axes at once, with counts that follow the selection. See
  [Taxonomy filters](#taxonomy-filters).
- **Source YAML view** — every document links to the exported resource it was written from
  (`GET /:tenant/_resource/<type>/<name>`), syntax highlighted with `shiki`, one addressable line per `#L42`
  anchor, `?raw` for plain text, and a **Documentation | YAML | Drift** switcher in the top bar.
- **Drift view** — what `azure-rd resource drift` observed about each resource since the export was taken:
  its verdict, the recorded field changes, the analysis written about them and the observed configuration,
  plus the whole observation beside the tenant summary (**Summary | Drift**). See [Drift view](#drift-view).
- **Document rendering** — `markdown-it` with raw HTML enabled, so the `<details>`/`<summary>` settings blocks
  that make up the bulk of a document pass through; headings get stable anchors; relative `.md` links become
  app routes; frontmatter is shown as metadata, never rendered into the body; the closed heading contract the
  CLI declares per type drives per-section styling. See [Rendering](#rendering).
- **Confluence HTML export** — `GET /:tenant/_export/confluence` streams the whole tenant as a zip ready for
  Confluence's HTML import, offered as a download link per tenant on the picker. **One-way**; see
  [Confluence export](#confluence-export).
- **Drift report PDF** — `GET /:tenant/_export/drift-pdf` returns the tenant's current drift observation, its
  analysis and every finding as one PDF, offered as a download link on the tenant drift page. See
  [Drift report PDF](#drift-report-pdf).
- **No-restart refresh** — regenerated documents, re-downloaded resources and a regenerated `index.yaml`
  appear on the next request (per-request `stat()` against an mtime/size-keyed cache), including the counts
  and export timestamp shown on the tenant picker and `/healthz`; only a newly generated tenant folder waits
  out the 30 s discovery TTL.
- **No client-side JavaScript.** Everything is server-rendered Handlebars + Tailwind; dark mode follows
  `prefers-color-scheme`, and a `Content-Security-Policy` that lets no script run.
  See [Security](#security).

## Quick start

Requirements: Node.js **>= 20** and an export tree produced by `azure-rd` with `azure-rd docs generate-index`
already run for each tenant (the browser needs `docs/index.yaml`; documents and `summary.md` are optional
and show up as they are written).

```bash
cd web
npm install
npm run start:prod        # build CSS + Nest, then serve http://localhost:3000
```

For development, `npm run start:dev` runs the Tailwind watcher and Nest in watch mode together. `npm start`
only runs the already-built `dist/main.js` — run `npm run build` first.

> Views (`views/`) and static assets (`public/`) are resolved from `process.cwd()`, so the server must be
> started from the `web/` directory in both dev and prod. `public/app.css` is a build artifact (gitignored);
> `build`, `start:dev` and `start:prod` all generate it.

## Configuration

Environment variables only; there is no config file and no `dotenv`.

| Variable | Default | Purpose |
| --- | --- | --- |
| `DOCS_ROOT` | `../output` (relative to `process.cwd()`) | Root that is scanned for tenant folders. With the monorepo layout the default already points at the shared export tree at the repo root. |
| `PORT` | `3000` | HTTP listen port. Accepts 1 to 5 ASCII digits in `1..65535`; unset or empty means the default, silently, and anything else falls back to the default too, with a note in the startup line. |
| `EXPORT_INDEX` | `type` | Which index the Confluence export writes onto `Overview.html`: `type` (the by-type **Pages** list only), `both` (that list, then one collapsible section per taxonomy axis) or `axis` (the axis sections only). Unset, empty or unrecognised means `type`, so a typo can neither fail an export nor change what it contains. |

```bash
DOCS_ROOT=/path/to/output PORT=4000 npm run start:prod
```

[`.env.example`](.env.example) lists every variable at its default, as a reference to copy and source into
your own shell (the app does not load it):

```bash
cp .env.example .env
set -a; source .env; set +a
npm run start:prod
```

## The docs root contract

```
<DOCS_ROOT>/
└── <tenant>/                      # e.g. contoso.onmicrosoft.com/  (may be nested, up to 3 levels)
    ├── resources/                 # written by `azure-rd download` — served read-only as the YAML view
    │   └── Microsoft.Graph/
    │       └── <endpoint>/
    │           └── <name>.yaml
    ├── drift/                     # optional, ephemeral — written by `azure-rd resource drift`
    │   ├── metadata.yaml          # the observation — read, never served
    │   ├── analyze.md             # analysis prompt — never served
    │   ├── audit.yaml             # optional attribution (`azure-rd resource audit`) — read, never served
    │   ├── index.md               # optional analysis summary — the tenant drift page body
    │   └── Microsoft.Graph/
    │       └── <endpoint>/
    │           ├── <name>.yaml    # observed payload, mirroring resources/
    │           └── <name>.md      # optional drift document written by the analysis
    └── docs/
        ├── index.yaml             # required marker (azure-rd docs generate-index)
        ├── generate.md            # agent prompt — never served
        ├── summary.md             # optional tenant summary — the landing page body
        ├── report-*.md            # agent run reports — not indexed
        └── Microsoft.Graph/
            └── <endpoint>/
                └── <name>.md      # one document per resource, mirroring resources/
```

Discovery and resolution rules:

- A directory is a tenant when `docs/index.yaml` exists **and parses** as an index object with an integer
  `version` of **1 or later**. A malformed or unreadable index makes the folder *not* a tenant instead of
  crashing discovery. Later schema versions and unknown fields are accepted and ignored — the index is the
  tenant marker, so rejecting a newer schema would hide the export rather than degrade a page. The CLI
  currently writes schema version 4; a version-2 index (`programmes` + per-resource `groups`, no `facets`)
  is still understood.
- Documents are resolved against `<tenant>/docs`, which is what the relative `../<type>/<name>.md` links inside
  the documents are relative to. Source YAML is resolved against the sibling `<tenant>/resources` — a second,
  separate served root, restricted to `.yaml`.
- `resources/` is **not** a discovery marker: an export whose `docs/` were copied without it stays a valid
  tenant whose YAML views simply 404.
- `drift/` is a third served root and not a discovery marker either. It is deleted wholesale by the next drift
  run or re-baselining download, so a missing tree is the normal *no observation* state, reflected on the next
  request without a restart. Everything at its top level (`metadata.yaml`, `analyze.md`, `audit.yaml`,
  `index.md`) is unreachable through the per-resource route; `index.md` is only ever the tenant drift page's
  body, and `audit.yaml` is only ever read as data for the attribution below.
- A document's source is located by mirroring its own path (`docs/<type>/<name>.md` ↔
  `resources/<type>/<name>.yaml`) — exactly inverting how the CLI derives the document path. The `source`
  frontmatter is only a label, and only resources listed in `docs/index.yaml` are reachable.
- A matched tenant **owns its whole subtree**; discovery does not descend further looking for nested tenants.
- Directories whose name starts with `_` or `.` are skipped (housekeeping folders such as `_to_delete/`).
- Counts in the picker and sidebar come from the index (`counts.documented`, `counts.pending`,
  `counts.excluded`), never from walking the tree. The tenant compare is the one exception to *the index*, not
  to *never walking*: its rows are read from each export's `resources/metadata.yaml` as data, and only a pair's
  status reads the two resource files the metadata names. The tenant's display name is the index's `tenant`
  field (the Entra default domain), falling back to the folder name.

## Routes

| Route | Response |
| --- | --- |
| `GET /` | Tenant picker (`views/picker.hbs`), with each tenant's export download link and, when at least two exports carry `resources/metadata.yaml`, a *Compare with…* link on each of those. |
| `GET /_compare?a=<tenant>` | The picker in its compare selecting state: `a` is marked, every other eligible tenant offers *Compare with*, and *Cancel* returns to `/`. A bare `GET /_compare` redirects to `/`. |
| `GET /_compare?a=<tenant>&b=<tenant>` | The comparison pane of the two exports — every pair with its status, one-sided resources, what needs attention first — above an empty diff area pointing at the first difference; `&same` also shows identical pairs. 404 for an unknown or `_`-prefixed tenant, the same tenant twice, or a tenant without a readable `resources/metadata.yaml`. |
| `GET /_compare/*path?a=&b=` | The same pane with this pair selected, above a line diff of the two files after normalisation; `&raw` diffs them as exported, `&same` keeps identical pairs in the pane. The `.yaml` suffix is optional. 404 for a key either export does not list as present. |
| `GET /healthz` | JSON `{ status, rootReadable, tenants, documents, pending }`. Always `200`: the process is healthy even when `DOCS_ROOT` is missing or unreadable, so a probe reading only the status code does not flap while a volume is remounted. In that case `rootReadable` is `false` and `status` is `degraded` instead of `ok` — the way to tell an empty tree from a missing one. The root's path is never returned. `documents` and `pending` are read from each tenant's `index.yaml` on every call, not cached, so a regenerated index is reflected immediately. |
| `GET /favicon.ico` | `301` to `/favicon.svg`, the static icon every page links to. Declared so a browser's own probe is not read as a tenant named `favicon.ico`. |
| `GET /:tenant` | The tenant landing page: `docs/summary.md`, or the `docs/index.yaml` listing when there is none. Takes one repeatable filter parameter per taxonomy axis. |
| `GET /:tenant/summary` | `302` to `/:tenant` — the summary is that page's body, not a separate document. |
| `GET /:tenant/_export/confluence` | The whole tenant as an `application/zip` attachment for Confluence's HTML import. |
| `GET /:tenant/_export/drift-pdf` | The tenant's current drift report as an `application/pdf` attachment, `<tenant>-drift.pdf`. 404 when there is no observation or it is outdated; any other format 404s. |
| `GET /:tenant/_resource/*path` | The source YAML behind a document, syntax highlighted; the `.yaml` suffix is optional. |
| `GET /:tenant/_resource/*path?raw` | The same file as `text/plain; charset=utf-8` (`nosniff`), for copy-paste. |
| `GET /:tenant/_drift` | The drift observation at tenant scope: header, analysis summary and every recorded finding. |
| `GET /:tenant/_drift/index` | `302` to `/:tenant/_drift`. |
| `GET /:tenant/_drift/*path` | What the observation says about one resource, at the same path as its documentation; the `.yaml` suffix is optional. |
| `GET /:tenant/_drift/*path?yaml` / `?raw` | The verified observed payload, highlighted or as `text/plain` (`nosniff`). 404 when there is none or it no longer matches the observation. |
| `GET /:tenant/_drift/*path?diff` | A unified line diff of the verified baseline against the verified observed payload (changes and renames). 404 when either side is missing or no longer matches the observation. |
| `GET /:tenant/*path` | A document inside the tenant's `docs/` folder; the `.md` suffix is optional. Filter parameters apply to its sidebar. |

Anything that does not resolve to a Markdown file inside the tenant's `docs/` — or to a `.yaml` file inside
its `resources/` — renders the 404 view, which never leaks a filesystem path. The view names what was
missing — tenant, document, source YAML, export format, drift observation, drift finding, observed payload, YAML diff,
tenant comparison or resource comparison — rather than always saying *Document not found*.
`docs/generate.md` is tool input, not documentation, and is never served.

`_resource`, `_drift` and `_export` are *representation* prefixes, not path segments: they never appear in the
breadcrumb, and they cannot collide with a resource type because no Azure/Graph type segment starts with `_`.
`_compare` is the root-level counterpart: it cannot collide with a tenant because discovery skips `_`-prefixed
folders.

## Taxonomy filters

`azure-rd docs generate-index` can classify each resource along one or more **axes** — *Programme* (CIS
hardening, Defender, VPN, …), *Platform*, *Assignment scope*, whatever the operator declares in the CLI's
`taxonomy:` config section. When it does, `docs/index.yaml` carries a header `facets` registry (each axis: `id`,
`label`, and its `values` in display order with per-tenant `count`s) and a per-resource `facets` map of **value
ids only**; labels are always resolved from the header.

The browser **reads that membership and derives none of its own**, and names no axis anywhere in its code or
templates: an axis added to the CLI's config shows up here with no change. The classification is resolved once,
at index time, so every consumer of the index sees the same one.

- Every page that shows the sidebar offers **one chip group per axis**, headed by the axis's label, and accepts
  **one repeatable query parameter per axis id**: `?programme=defender&programme=vpn&platform=macos`. Values are
  **OR-ed within an axis** and **AND-ed across axes**.
- **Every chip toggles its own value** and keeps the rest of the selection. The whole selection rides along in
  every document link, and a **Clear filters** link plus a *showing N of M* line state what is applied — all in
  the URL, with no client-side state.
- **`?<axis>=_uncategorised`** lists what that axis matched to nothing, so a taxonomy that stops matching shows
  up as a full bucket rather than as a quietly thinning tree.
- **Counts follow the selection.** Each value is counted against the resources the *other* axes allow, so
  picking one value does not zero its siblings. A value another filter has emptied is no longer offered; a
  **selected** value stays visible even at 0 so the choice can be undone; while nothing is filtering, a
  zero-count value stays listed — "empty here" is information. Totals count **distinct resources**, since a
  resource can hold several values on one axis.
- **The document you are viewing stays in its sidebar** even when the filter excludes it. It is not counted
  into the selection, so it is marked *outside the filter* and the count line adds *plus the document you are
  viewing* — which is why the tree can hold one row more than the count says. Unknown values, unknown axes and
  an axis id that would collide with a route's own parameter (`?raw`) are ignored rather than rendering what
  would look like an empty tenant.
- An index written **without** a taxonomy offers no filter and renders the per-type tree unchanged. A
  version-2 index still filters: its single programme axis is synthesised from `programmes`/`groups`.

The filters narrow *navigation*, not page bodies. The Confluence export ignores a selection — it always exports
the whole tenant — but it *indexes* by the same axes through the same rule (see `EXPORT_INDEX`).

Structuring the sidebar *by* an axis instead of by resource type is the other half of this and is not
implemented; see [`NEXT-ITERATIONS.md`](NEXT-ITERATIONS.md).

## Confluence export

`GET /:tenant/_export/confluence` returns one zip containing one folder, which is what Confluence's HTML import
expects: the folder name becomes the space name, each `.html` file becomes a page, and **the file name becomes
the page title**.

- **Space name** — `<tenant domain> documentation`.
- **Page titles** — `<type leaf> — <display name>`, taking the display name from `docs/index.yaml`, then the
  document's H1, then the file's base name. Characters illegal in a file name or a Confluence title are
  replaced; a residual collision gets a `(2)` suffix and a line on the overview page — never an overwrite.
- **Overview page** — `Overview.html`, built from `docs/summary.md` plus a link list that stands in for the
  sidebar, since an imported space is a **flat** set of pages with no hierarchy. Its index is configurable
  through `EXPORT_INDEX`: by resource type (default), by taxonomy axis — one `<h2>` per axis and one collapsible
  `<details>` per value, which the importer turns into a native expand — or both. The axis sections classify
  through the *same* rule as the sidebar filter, so a page appears under every value it holds, the
  uncategorised bucket is always rendered, and each count equals the links beneath it. An index that declares
  no usable axis renders the by-type list whatever the variable asks for.
- **Provenance** — each page opens with the source, export timestamp and generation hashes from the document's
  frontmatter, and a note that the page is generated.
- **Determinism** — zip entries carry the export's own `generatedAt`, not the wall clock, so exporting an
  unchanged tenant twice produces the same bytes.

It stays read-only: documents are enumerated from `docs/index.yaml`, read through the same path guard as every
other route, and the archive is assembled in memory and streamed — no temporary file, nothing written under
`DOCS_ROOT`. A document the index lists but that cannot be read is reported under *Not exported* on the overview
page instead of failing the export.

**One-way.** Import *creates* a space rather than updating one, so re-importing yields a second space, and
edits made in Confluence are lost the next time the export is imported. Beyond that:

- **`<details>` blocks are passed through untouched**, which is what the importer wants: verified against a
  Confluence Cloud import, each block becomes a **native collapsible expand** with its `path: value` summary,
  nesting and inline formatting intact. The summary is never parsed into key/value, so a value that itself
  contains ` = ` cannot be mangled.
- **No media.** Each served root hands out exactly one extension (`.md` under `docs/`), so the exporter cannot
  read an image; images travel as their `alt` text.
- **In-document anchors do not survive**, because the flat space has no place for them. Heading permalinks are
  unwrapped, and a link whose target is not a page in the export degrades to its text.
- **Only what the importer preserves is emitted.** The serialiser is an allowlist: unsupported HTML is
  unwrapped, scripts and embeds are dropped, and a bare `<key>` in prose (macOS plist quotes are full of them)
  is escaped rather than shipped as a phantom element.
- **Source YAML is not attached.** Whole-tenant only — no per-type or single-document export.

## Drift view

`azure-rd resource drift` compares the live tenant against the export and writes `<export>/drift/`; an analysis
pass (`azure-rd docs analyze-drift`) may add a drift document per finding and a summary. The comparison is the
CLI's job — this app only renders what is on disk, and never acts on it.

- **The Drift button** sits beside **Documentation | YAML** on every resource that has a source, and is always
  rendered. It links whenever the observation has something to say — a verdict (shown in the label), a type it
  could not list, an entry it could not compare, or that it is outdated. It is **inert**, with the reason as
  its tooltip, only when there is no observation or the observation compared the resource and found it
  unchanged. The button and the page it leads to read one decision, so they cannot disagree.
- **Summary | Drift** on the landing page links to the tenant drift page whenever an observation exists —
  including one with no findings, or not analysed yet — and is inert only without one.
- **Tenant picker.** Each tenant's line adds its drift state after the export time: *drift detected on N
  resources* with the observation time, *no drift detected*, or *drift observation outdated* — dated but never
  counted — read from the same decision as the **Summary | Drift** switcher. Without an observation nothing is added.
- **Renames** are reached from the old name's page: the finding is matched by its baseline key as well.
- **Partial runs** are the normal case: the tenant drift page states when the run was incomplete, which types
  could not be listed, which entries could not be compared and whether removals were suppressed, so a missing
  finding is never read as *unchanged*.
- **Validity gate.** An observation taken against a baseline other than the one in `resources/metadata.yaml`
  (the same test the CLI applies; the index's `generatedAt` only when `resources/` is absent) is shown as
  *outdated* at both scopes, never as a comparison.
- **Integrity.** Before a comparison or payload is shown, the baseline and observed files are hashed and
  checked against the observation; a mismatch withholds both and says so.
- **YAML diff.** When both sides of a change or rename are verified, the finding links one **YAML diff**
  instead of separate baseline and observed views: a line diff with three lines of context, line numbers for
  both sides, and raw links to either file. It is laid out **side by side** — *baseline* left, *observed*
  right, a modified line as one row — whenever the diff itself is at least 48rem wide; narrower, each row
  stacks its baseline line above its observed one, so the narrow view is a unified diff with every changed
  line directly above its replacement. The switch is a container query on the diff's own width, not the
  window's, so the sidebar is accounted for; no script is involved. With the sidebar beside it that takes a
  window of about 1152px; between 1024px and that the diff stacks, and below 1024px, where the sidebar moves
  under the content, it is side by side again from about 800px. Side by side, both line numbers meet in the
  middle and the `−` / `+` signs are dropped; stacked, the signs stay. A modified line is tinted on both
  sides and its differing characters are shaded and underlined (a line that changed by more than half, or is
  over 500 characters, is tinted whole). The page counts the differences — runs of adjacent changed lines —
  and links the first; each difference links to the next, the last back to the first. It is computed per
  request as plain data and escaped by the template; above 1 MiB combined it is not computed and the raw
  files are offered instead.
  An addition, which has no baseline, keeps its observed YAML links.
- **Links** inside the analysis documents resolve within the drift view; a link out to `docs/` reaches the
  documentation route.
- **Attribution.** *Who changed this, and when* comes from `drift/audit.yaml`, which the CLI writes beside the
  observation — `azure-rd resource audit`, or `azure-rd resource drift` when the tenant profile sets
  `audit-workspace-id` — by joining each finding against the tenant's Log Analytics audit tables. The file is
  optional; without it the drift pages look exactly as before. It is shown only when its `observedAt` **and**
  `baselineGeneratedAt` both equal the observation's; otherwise the observation header says *Attribution
  outdated: it predates this observation. Run `azure-rd resource audit` again.* and no actor appears anywhere.
  An outdated observation never shows attribution at all. When current:
  - **Tenant drift page, finding rows.** Each finding in *All recorded findings* ends with `actor · time`, plus
    `(+N more)` when the window holds several events. A finding the CLI could not attribute shows why: *no
    audit event in the window*, *window starts before the table's retention* and *audit query failed* in
    amber; *not queried* and *no audit join key for this resource type* in quiet slate. A finding the file does
    not name shows *no attribution recorded*. An event that names neither a user nor an application shows as
    *unknown actor*.
  - **By actor.** A section below the findings lists each actor once, with a tag when the actor is not a user,
    the audit window, and the findings they changed, each with its verdict badge and the time of their latest
    event on it. Only findings with a recorded event are listed.
  - **Observation header.** A caveat line gives the workspace, the window and the counts — matched, no event,
    beyond retention, no join key, failed, not queried — read from the file, never recomputed, with a note per
    audit table that could not be queried.
  - **Resource drift page.** The header gains the finding's events, newest first: when, actor, activity,
    result (a failure in red) and correlation id — or the one-line reason there are none. A renamed resource's
    old-name page shows the same attribution as its new name.
  - **Changed by column.** The analysis index's findings table (the one with Severity and Verdict columns)
    gains a last column, **Changed by**, with the same text as the finding row. Each row is joined to its
    finding through the link in its Resource cell; a row without a link, such as the inventory rows, or whose
    link names no finding of this observation, stays empty. An index written as prose gets no column.
  The app derives nothing: it joins by finding key and shows the recorded facts. Rewriting or deleting
  `audit.yaml` shows on the next request without a restart.
- **Download.** While the observation is current, the tenant drift page's top bar offers **Download drift
  report (PDF)** beside **Summary | Drift**, with *A snapshot of this observation; it is not updated.* next to
  it. See [Drift report PDF](#drift-report-pdf).

## Drift report PDF

`GET /:tenant/_export/drift-pdf` turns what the drift pages show into one A4 PDF, for a reader who has no
access to the browser or the export tree. It is offered only on the tenant drift page — the report is scoped
to the drift observation, not the tenant's documentation, so it is not on the picker — and only while the
observation is current; with no observation or an outdated one the link is absent and the route answers 404.

- **Content.** A cover (tenant, observation time, baseline time, both tool versions, finding count); the
  observation block as on the drift page — counts, attribution or its outdated caveat, incomplete run,
  suppressed removals, types that could not be listed, entries that could not be compared; the analysis
  summary (`drift/index.md`) with its Severity and **Changed by** columns; every finding grouped by type, each
  with verdict, severity, attribution (the events table or the reason there are none), the *What changed*
  deltas, the integrity warning when its files no longer match, and its analysis — or *No analysis*; then
  *By actor*. Every `<details>` block is expanded. A link to another finding of the report jumps inside the
  PDF; every other link is plain text and images become their alt text. Each page's footer names the tenant,
  the observation time and the page.
- **Not included.** No observed YAML payloads and no line diffs — the same line the Confluence export draws at
  source YAML.
- **Layout.** Portrait A4. Tables fit the page: short columns are sized to their content, and long unbroken
  values — dotted paths, GUIDs, underscored names, UPNs, type keys — wrap after `.` `/` `_` `-` `@` `:` `]`
  (or every 12 characters) at an invisible zero-width space. A table row never splits across a page; the
  header row repeats on the next.
- **Fonts.** Roboto, bundled with `pdfmake`, for text; Courier for code a PDF standard font can encode.
  `→ ← ↔ ⇒ ✓ ✗`, which Roboto lacks, print as `-> <- <-> => yes no` — in the PDF only; the drift pages keep
  the original characters.
- **Deterministic.** The PDF's creation date is the observation time, so the same observation downloads as
  the same bytes.
- **Read-only and offline.** The report is assembled in memory with `pdfmake` from the same reads the drift
  pages use and sent once complete — no temporary file, nothing under `DOCS_ROOT` written. `pdfmake` is
  configured to fetch no URL and to read no local file other than its four Roboto fonts (it also passes the
  four Courier standard-font names through the same check). A failed build answers a plain `500` *The drift
  report could not be built.*, never a half-sent attachment, a path or an error message.
- **Limits.** CJK and emoji have no glyph in Roboto. Copied text carries the invisible break character at
  the wrap points. The 12-character wrap applies per inline run, so a token spanning two differently
  formatted runs without whitespace is not cut where they meet. A single table row taller than a page is
  not broken.

## Tenant compare

A **proof of concept**: put two exports side by side — stage against prod — and see, per resource, whether it
is configured the same in both, and where it differs. Offline and read-only like everything else; no Azure call.

- **Selection** is two plain links on the picker: *Compare with…* on the first tenant, then *Compare with …*
  on the second. A tenant takes part only when its export has a readable `resources/metadata.yaml`, so
  docs-only copies get no link.
- **The comparison pane** heads both compare pages, full width, like the top half of an IDE's folder
  comparison. Which rows exist comes from the two `resources/metadata.yaml` files alone — no directory walked.
  Resources are paired by their export path (`<APIType>/<endpoint>/<name>`), which is a heuristic the page
  states: different policies can share a name, and a renamed policy shows once on each side. Only resources
  still present in the tenant (`presentInTenant` not `false`) are listed. One flat, scrollable list: a header
  row per type, types either index counts under `counts.excluded` last and marked *not documented*; then per
  row the left name and size, a status, the right size and name. Pairs come first in each type, then the
  left-only, then the right-only rows. Each row is one link: a pair to its diff, a resource only one side has
  to that tenant's YAML view. The pane opens at 40% of the window and can be dragged taller or shorter; below
  1024px the sizes drop.
- **The status** of a pair is `=` *identical*, `≠` *different* (lighter when it *differs only in audience*),
  or `?` *could not compare* with the reason — a file that does not parse, cannot be read, or is too large to
  normalise and differs as exported. `→` / `←` mark a resource only one side has, pointing away from that side.
  Every marker has a hidden label, so it is never meaning by glyph and colour alone. It is decided the way the
  diff page decides it, from the same normalisation, over hashes of each file kept per file (never its text)
  and validated by that file's and its export's metadata timestamps and sizes, so an edited resource or a
  re-downloaded export shows on the next request. Its cost is one read and normalisation per paired file on a
  cold cache, then only file checks.
- **What is shown**: by default the pane lists what needs attention — different, one-sided and unknown rows —
  and counts everything (*N different · N only in a · N only in b · N identical*); *show identical* (`&same`)
  lists the identical pairs too, and every row link keeps that choice. The pair being viewed is always listed
  and marked, whatever its status, and the page opens with it scrolled into view. *swap sides* keeps the pair.
  Without a selected pair the diff area links the first difference.
- **The diff** normalises both files identically, then shows a line diff through the same partial as the drift
  diff — side by side under the two tenant ids when it is wide enough, stacked otherwise, with the same
  marking, count and jump links, exactly as described for the drift diff above. Unlike the drift diff it shows
  **the whole file**, the way an IDE compares two files, with no hunk headers; above 3,000 lines it falls back
  to the changes with three lines of context and says so. A caption states exactly which keys were dropped
  and how many references were resolved, ambiguous or unresolved; `&raw` shows the files as exported. A pair
  identical after normalisation says *No differences after normalisation*; one that is identical once
  `assignments` is removed as well says *Differs only in audience*. Line numbers are those of the normalised
  text. Above 1 MiB combined, or when a file does not parse, nothing is normalised.
- **The normalisation rule is provisional** and destined for the CLI (`azure-rd resource compare`, parked in
  `../go/NEXT-ITERATIONS.md`); until then it lives in `src/docs/compare-normalise.ts` as data:
  - **dropped** at any depth: `id` and `sourceId` when their value contains a GUID other than the all-zero
    sentinels (ids without one — settings ordinals, `all_users`, authentication method names — are content);
    every key ending in `@odata.context`; `createdDateTime`, `lastModifiedDateTime`, `version`, and the group
    identity fields `mail`, `mailNickname`, `proxyAddresses`, `securityIdentifier`, `renewedDateTime`;
  - **resolved**, never dropped: `groupId`, `deviceAndAppManagementAssignmentFilterId` and
    `notificationTemplateId` become the `displayName` of the entry in that tenant's own `metadata.yaml` with
    that `resourceId` — only when every such entry agrees on the name (otherwise *ambiguous*); an id with no
    entry stays and counts as *unresolved*; an all-zero sentinel means *none* and passes through;
  - **serialised** canonically: sorted keys, core schema on both load and dump, so no scalar changes type.
- **Measured baseline** against the two reference exports (1029 and 181 resources, 114 paired): 19 pairs are
  identical after normalisation, 17 more differ only in audience, 6 references stay unresolved. A change to the
  rule is judged against these numbers.

## Rendering

One `markdown-it` instance (built once, in `MarkdownRendererService`) renders every document:

- **Raw HTML is enabled** (`html: true`) because the `<details>`/`<summary>` settings blocks *are* the
  documentation; the trust boundary is the docs root, which is operator-supplied content. `typographer` stays
  off so configuration values keep their quotes and dashes.
- **Frontmatter** is parsed with `gray-matter`: `source` (linked to the YAML view) and `generatedAt` are shown
  as page metadata; nothing from it reaches the body. The documents also echo their source file as a code-only
  paragraph under the H1; that duplicate is dropped at render time (only when it matches the document's own
  `source` and stands alone on its line).
- **Links**: relative `.md` links are resolved against the current document's directory and turned into app
  routes (`../groups/g1.md` → `/<tenant>/Microsoft.Graph/groups/g1`). Anchors, absolute routes, schemes,
  protocol-relative URLs, non-`.md` targets and links escaping the tenant root are left unchanged.
- **Caching**: renders are cached by `mtimeMs` + `size` from a per-request `stat()`, bounded by an entry cap,
  so a regenerated document appears without a restart.

### Section styling

The CLI declares each document type's `##` headings as a closed, verbatim set (the `<!-- doc-headings: … -->`
contract in its `doc-prompt.md`), which makes the heading text a machine contract rather than prose.
`src/docs/section-hooks.ts` uses it to give the stylesheet something to reach:

- **`data-section="<slug>"` on every `h2`/`h3`**, plus `class="doc-section-heading"` on those whose heading is
  in the declared vocabulary. Each of those gets an icon and one of four role colours (risk, substance,
  relations, meta) drawn as a masked SVG, so one value drives both themes. A heading outside the vocabulary
  keeps the attribute and gets no treatment, which is why documents generated before the contract still render
  as plain prose.
- **Heading ids are slugged locally** rather than by `markdown-it-anchor`'s percent-encoding default:
  `#lifecycle-and-operations` instead of `#lifecycle-%26-operations`, and an em dash in a `summary.md` H1 no
  longer produces `%E2%80%94`. `&` slugs to `and`, so a heading has the same anchor and section identity
  whichever way it is spelled.
- **Each H2 run is wrapped in `<section class="doc-section" data-section="…">`**, which lets a section own a
  panel, a rail or its own density. *Settings*, *Properties*, *Definition* and *Membership* switch to a denser
  mode (one document in the reference export holds 317 settings nested five deep); *Security* and *Expiry and
  renewal* get a rail and deliberately no tint.
- **The tool-maintained marker pairs become elements**: a matched `<!-- assignments:start -->` /
  `<!-- assignments:end -->` pair (likewise `targeted-by`, `used-by` and `notifications`) renders as a
  `<div class="doc-assignments">`, because an HTML comment survives into the DOM but cannot be selected. An
  unmatched marker is left as a comment. An H2 *inside* a matched pair never opens a section — the one rule that
  keeps a spliced block from being straddled by a `<section>`, and keeps `## Used by` reading as part of the
  section it was spliced into.
- **Setting blocks are styled from their own attributes.** The generator opens each as
  `<details data-setting="<YAML path>">` and marks it `data-note="security"` or `data-note="inert"`; those get
  a risk rail plus a `security` chip, and a dashed, de-emphasised frame plus a `no effect` chip. The chips are
  `::after` content on the block's own `<summary>` — decorative only, since the same fact is in the prose.
- **The metadata table** each document opens with is classed `.doc-metadata` (the first table after the H1,
  so an extra heading before it does not break the match). The summary's **Findings** table is classed
  `.findings` with a `data-severity` per row, so severity can be coloured; the drift index's Findings table
  (Severity first, with a Verdict column) is additionally classed `.findings-drift` and tagged against the
  drift analysis's own set, `high / medium / low / info`, on the drift page's colour scale. Each table keeps
  its closed set: a value outside it stays plain text rather than getting a wrong icon. On the tenant drift
  page, with a current audit, the drift index's table also gains the **Changed by** column (see
  [Drift view](#drift-view)), its cells tagged `data-column="changed-by"` and toned by `data-attribution`
  (`matched`, `warning`, `quiet`). It is added in the same token pass through the one `markdown-it` instance,
  as escaped text; the render cache keys on the column's content as well as the file, so it stays fresh
  without a restart.

None of this reaches the Confluence export: `<div>` and `<section>` unwrap and the attributes are on no
element allowlist, so an exported page is unaffected.

### YAML view

One `shiki` highlighter (built once, in `YamlHighlighterService`, loaded through the ESM `dynamicImport`
escape hatch) renders the source YAML with dual-theme output, so dark mode and the `#L42` line highlight are
plain `prefers-color-scheme` and `:target` CSS. A highlighter load failure, or a file above the size cap,
degrades to an escaped `<pre>` rather than an error.

### Printing

Printing a page, or saving it as a PDF, yields the document alone: the top bar and the navigation sidebar are
hidden, the layout un-sticks into a single column and the wide tables wrap instead of scrolling. **Collapsed
`<details>` blocks print collapsed** — CSS cannot open a disclosure element, and there is no client-side
JavaScript to do it — so expand the settings blocks you want on paper before printing. For drift, the
[Drift report PDF](#drift-report-pdf) is the whole observation with every block expanded.

### CSS

Tailwind CSS v4 with `@tailwindcss/typography`. Utility classes live in the `.hbs` templates, so
`src/styles.css` declares `@source "../views/**/*.hbs"` — templates outside `views/` must be added there or
their classes are purged. `styles.css` holds only the theme tokens and the rules Tailwind cannot express
(`<details>`/`<summary>`, table overflow, section identity, dark mode, print).

## Security

`*path` is attacker-controllable, and `resolveWithinRoot()` in `src/docs/path-safety.ts` is the single guard
for it — used through `resolveWithinTenant()` for documents, `resolveResource()` for source YAML, and
`resolveDriftDocument()` / `resolveDriftPayload()` for the drift tree. It:

- rejects null bytes, absolute paths and any `..` segment before touching the filesystem;
- serves only files ending in the **one** extension that root allows — `.md` for `docs/`, `.yaml` for
  `resources/` — so a document can never be served from the resources root, nor a resource from `docs/`, and
  `..` cannot cross between them (`.yml` is deliberately not served). `drift/` holds both kinds, so it is
  served by two resolvers pinned to **one extension each** (`.md` for drift documents, `.yaml` for payloads)
  and both require at least two path segments, which keeps the tree's top-level files unreachable;
- re-checks, **after** `realpath()` resolution, that the target is still inside that root, so a symlink cannot
  escape.

All request-derived filesystem access goes through that function. Error responses never surface an absolute
filesystem path, stack trace or raw exception message. The app stays read-only: no route writes, moves or
deletes anything under the docs root.

### Response headers

Every response carries `X-Content-Type-Options: nosniff`, `Referrer-Policy: same-origin`,
`X-Frame-Options: DENY` and a `Content-Security-Policy`, set once in `configureViews()` so runtime and e2e
agree. The policy is `default-src 'none'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:;
frame-ancestors 'none'; base-uri 'none'` — no directive is broader than what the pages load: `style-src`
allows `'unsafe-inline'` for the inline colours shiki's dual-theme output writes on every YAML token;
`img-src` allows `data:` for the severity/section icons (`data:` SVGs applied through a CSS mask) and
`https:` for an image a document embeds; nothing permits a frame; and no directive names `script-src`, so no
script may run at all. This is not an XSS boundary — `markdown-it` still renders with `html: true` — but it
makes the no-client-side-JavaScript rule enforceable by the browser, not only stated here.

## Tests

```bash
npm test
```

Jest (`ts-jest`, `testRegex: .*\.spec\.ts$`), run with `--experimental-vm-modules` because `markdown-it-anchor`
v9 and `shiki` are ESM-only. Tests never hit the network and never read the real `output/` export: fixtures are
built in a temp directory and removed afterwards.

| Suite | Covers |
| --- | --- |
| `test/path-safety.spec.ts` | Traversal, symlink escape, null bytes, absolute paths, one extension per root for both roots, one extension per drift resolver and the drift depth guard. |
| `test/drift-audit.spec.ts` | `drift/audit.yaml` parsing (malformed rejected, never a throw; unsafe keys and unknown statuses dropped; actor-less events kept), the validity rule, the per-status row and page attribution, *By actor* grouping, and the Changed by cells. |
| `test/findings-table.spec.ts` | The Changed by column on the drift index's findings table: the header and cells, the key read from the Resource link, empty inventory rows, unknown keys, escaping, and tables it must leave alone. |
| `test/drift-observation.spec.ts` | Observation parsing (malformed rejected, never a throw), the baseline timestamp read, and every drift state in evaluation order, including renames and the validity gate. |
| `test/link-rewrite.spec.ts` | `.md` href rewriting for the documentation and drift route bases. |
| `test/yaml-diff.spec.ts` | The line diff: per-side line numbers, hunk grouping and headers, identical files, verbatim text, the size cap, and the pairing of removed and added lines into side-by-side rows. |
| `test/compare-view.spec.ts` | The compare listing's row order within a type: pairs, then left-only, then right-only, each alphabetical. |
| `test/compare-normalise.spec.ts` | The compare's normalisation rule — every dropped key, the id shapes kept and dropped, reference resolution with its ambiguous, unresolved and sentinel outcomes, the audience-only variant, scalar stability — and `resources/metadata.yaml` parsing. |
| `test/tenant-index.spec.ts` | `index.yaml` parsing (malformed file rejected, later schema accepted, `facets`/`programmes`/`groups`/vocabularies), navigation building, every taxonomy-filter rule listed above (OR/AND, selection-aware counts, uncategorised bucket, exempt active document, version-2 synthesis, no-taxonomy passthrough), and `filterableAxes`/`groupByAxis` — the rule the export shares. |
| `test/section-hooks.spec.ts` | Heading slugs, declared vs undeclared headings, matched/unmatched marker pairs, section wrapping (an H2 inside a spliced block never opens one), metadata-table detection. |
| `test/docs.e2e.spec.ts` | supertest against fixture tenants: discovery, picker, landing page and its index fallback, the `/summary` redirect, sidebar, `<details>` passthrough, cross-type links, 404s and traversal, `generate.md` not served, no-restart refresh of a document, the summary, the index and a resource; the section hooks in rendered output; the YAML view with `#L` anchors, `?raw` and the switcher; the drift view's buttons, states, gate, integrity check, unreachable tree root and no-restart freshness; the attribution on both drift pages, the Changed by column, the outdated caveat, an unreachable `audit.yaml` and its no-restart freshness; the drift report PDF's content type, filename, identical bytes on repeat, 404s, the generic 500 and its link placement; the Confluence export's content type, archive shape, `EXPORT_INDEX` per request, the invariant that an export changes neither the rendered HTML nor a byte under the docs root, and that it carries nothing from the drift tree; the tenant compare's picker eligibility and selecting state, refused pairs, three-way listing, normalised, raw, audience-only and identical diffs, traversal, no-restart freshness and read-only invariant. |
| `test/export-drift-pdf.spec.ts` | The drift report PDF's pure modules: the HTML-to-PDF walker (expanded `<details>`, internal and plain links, images, dropped and unwrapped elements), the document definition (cover, observation, groups, deltas, attribution states, *No analysis*, non-Latin names, creation date, no image or URL node), table widths and wrap points, unbroken rows and the symbol substitution. |
| `test/export.spec.ts` | The exporter's pure modules: page titles, the allowlist serialiser, href rewriting, the format (space, page plan, provenance, overview and its axis index in all three modes), `parseExportIndexMode`, and the shared `<details>` fixture. |
| `test/styles-build.spec.ts` | Compiles `src/styles.css` with the local Tailwind CLI and asserts the custom rules survive. |
| `test/readiness-readers.spec.ts` | The `CHANGELOG.md`, `NEXT-ITERATIONS.md` and archive-frontmatter readers behind the readiness reports and the start gate. |
| `test/readiness-git.spec.ts` | The start gate and the branch facts against a throwaway `git init` repository (skipped without git). |

Required coverage for a change: `path-safety.ts` → `path-safety.spec.ts`; `tenant-index.ts` →
`tenant-index.spec.ts`; routes, discovery, rendering, highlighting or link rewriting → `docs.e2e.spec.ts`;
`styles.css` → `styles-build.spec.ts`.

## Project layout

```
web/
├── src/
│   ├── main.ts                          # bootstrap (PORT)
│   ├── port.ts                          # PORT parsing + fallback
│   ├── configure-app.ts                 # hbs view engine + static assets + security headers (shared with e2e tests)
│   ├── dynamic-import.ts                # native import() escape hatch for ESM-only deps
│   ├── app.module.ts
│   ├── styles.css                       # Tailwind v4 entry (+ @source for .hbs)
│   └── docs/
│       ├── docs.module.ts
│       ├── docs.controller.ts           # routes, breadcrumb, 404 mapping
│       ├── tenant-discovery.service.ts  # DOCS_ROOT scan + 30 s TTL cache + index cache
│       ├── tenant-index.ts              # docs/index.yaml parsing + navigation + facet filters
│       ├── markdown-renderer.service.ts # markdown-it instance + mtime render cache (+ Changed by fingerprint)
│       ├── yaml-highlighter.service.ts  # shiki highlighter + mtime render cache
│       ├── link-rewrite.ts              # .md href → app route, H1 title extraction
│       ├── findings-table.ts            # the summary's and the drift index's Findings tables → .findings(-drift) + data-severity, Changed by column
│       ├── section-hooks.ts             # heading slugs, data-section, marker blocks, metadata table
│       ├── path-safety.ts               # the security boundary
│       ├── drift-observation.ts         # drift/metadata.yaml parsing + the one drift decision
│       ├── drift-audit.ts               # drift/audit.yaml parsing + the attribution validity rule
│       ├── drift.service.ts             # observation / baseline / hash / audit reads, mtime-cached
│       ├── drift-view.ts                # drift view models: switcher entries, badges, finding lists, attribution
│       ├── drift-report.service.ts      # the tenant and finding drift reports shared by the drift pages and the PDF
│       ├── yaml-diff.ts                 # line diff of two YAML texts, hunks + side-by-side rows
│       ├── file-cache.ts                # mtime + size parsed-file cache shared by drift and compare
│       ├── resources-metadata.ts        # resources/metadata.yaml parsing + reference lookup
│       ├── compare-normalise.ts         # the provisional cross-tenant identity rule
│       ├── compare-view.ts              # compare view models: three-way listing, pair comparison
│       ├── compare.service.ts           # metadata reads (cached) + the two files of a pair
│       └── export/
│           ├── export.service.ts        # Confluence zip + drift PDF assembly (the only Nest piece)
│           ├── confluence.ts            # the format: space, page plan, overview, provenance
│           ├── drift-pdf.ts             # the drift report PDF's document definition
│           ├── pdf-content.ts           # rendered HTML → pdfmake content: wrap points, symbols, fonts
│           ├── export-index-mode.ts     # EXPORT_INDEX → by-type / axis / both overview index
│           ├── html-allowlist.ts        # rendered HTML → what the importer preserves
│           └── page-name.ts             # page titles = file names, sanitised and deduplicated
├── views/                               # page/tenant/resource/drift/drift-tenant/drift-diff/compare/compare-diff/picker/error + partials/
├── public/                              # favicon.svg; app.css (generated, gitignored)
├── test/                                # *.spec.ts
├── scripts/
│   ├── start-item.js                    # npm run start-item -- N: may entry N of the backlog be implemented?
│   ├── branch-ready.js                  # npm run branch-ready: is this feature/fix branch ready to ship?
│   ├── working-tree-clean.js            # its preflight: refuse to report on uncommitted changes
│   ├── release-ready.js                 # npm run release-ready: can a release be cut? (all change nothing)
│   └── lib/
│       ├── changelog.js                 # CHANGELOG.md / NEXT-ITERATIONS.md / archive-frontmatter readers all three share
│       ├── branch.js                    # the git facts behind branch-ready's branch checks
│       └── git.js                       # the one place the tooling shells out (read-only git)
├── .env.example                         # every variable at its default
├── eslint.config.mjs                    # the rule set `npm run lint` and WebStorm both run
├── eslint-suppressions.json             # the pre-existing findings it exempts (npm run lint:baseline)
├── CHANGELOG.md                         # Keep a Changelog; released sections match web/vX.Y.Z tags
└── NEXT-ITERATIONS.md                   # outstanding work, shipped-but-uncleared entries, parked ideas
```

## Development conventions

- Run everything through the npm scripts from this folder (`npm run build`, `npm run start:dev`,
  `npm run start:prod`, `npm test`, `npm run lint`, `npm run lint:fix`, `npm run lint:baseline`,
  `npm run lint:prune`, `npm run start-item -- <n>`, `npm run branch-ready`, `npm run branch-ready:report`,
  `npm run release-ready`). The Go
  `Makefile` in `../go` does not apply here.
- `eslint.config.mjs` is the single lint truth. `npm run lint` reports and is part of both readiness gates;
  `npm run lint:fix` rewrites files and is deliberately in neither, since a gate must not change the tree.
  **WebStorm needs no setup**: its default *Automatic ESLint configuration* runs the ESLint in this folder's
  `node_modules` against this file, so an editor squiggle and a `npm run lint` finding are the same thing — if
  the two disagree, the IDE is not in automatic mode. The bundled TypeScript inspections are a separate engine;
  what only they report is an editor hint, not a merge gate. Findings are fixed in the code or silenced at the
  one site with an `eslint-disable-next-line` naming the rule, and stale directives are themselves errors.
  `@typescript-eslint/no-explicit-any` is off, because `any` is sanctioned on the untyped surfaces this app is
  built on (markdown-it tokens, parsed YAML, the dynamically imported highlighter) and WebStorm does not flag it
  by default either; `no-console` is on, which with the one directive in `src/main.ts` is what keeps `console`
  to that single startup line.
- **`eslint-plugin-sonarjs` makes the SonarQube findings local.** It is generated from the same analyzer the
  server runs, so on the rules it covers ESLint reports what a scan would, in the same places — measured, not
  assumed. It applies to `src/`, `test/` and `scripts/`, which means it also covers the specs, where the server
  deliberately applies a reduced rule set. Two gaps are known: rules that **require type information**
  (a `sort()` without a compare function, and misleading array mutation) stay silent because this config does not
  enable type-aware parsing, and the `node:`-protocol / `replaceAll` / optional-chaining family the server takes
  from other plugins is not covered at all. Closing either gap is a separate decision — a dependency and a
  slower lint, respectively. Analysis itself is described in the **Static analysis** section of the
  [repository README](../README.md).
- **The code that predates those rules is baselined, not exempted.** `eslint-suppressions.json` records the 14
  findings still left from when the rules were switched on as a count per file and per rule — ESLint's own bulk
  suppressions — so `npm run lint` passes and both readiness gates are usable, while **new code is held to the
  full rule set**: a second violation of a suppressed rule in a suppressed file exceeds the recorded count and
  reports. That is why the baseline is a file of counts rather than `off` entries in `eslint.config.mjs`, which
  would blind the rule for a whole file including code written tomorrow. Regenerate it with
  `npm run lint:baseline` and drop entries a refactor has made unnecessary with `npm run lint:prune`; both
  rewrite the file, so — like `lint:fix` — neither is part of a gate. Clearing it is a **parked idea** in
  `NEXT-ITERATIONS.md` rather than scheduled work: the occasion to pay an entry off is a file being touched for
  another reason. Growing it is a decision to take deliberately, not a routine.
- The architecture invariants and style/testing requirements live in `.windsurf/rules/` **in this folder**
  (`01-architecture.md`, `02-style-and-quality.md`, `06-next-iterations.md`); the Go rules in `../go` do not
  apply. Claude Code reads the same rules from `CLAUDE.md`, `../.claude/rules/` and the procedures in
  `../.claude/skills/`.
- Non-negotiables worth knowing before touching the code: read-only (no route mutates anything); one path guard
  (`resolveWithinRoot`) with one extension per root; one `markdown-it` instance and one `shiki` highlighter,
  both built at module init; `html: true` stays on; no client-side JavaScript; regenerated files must appear
  without a restart; environment variables are the only configuration and `DOCS_ROOT` the only link to the CLI.
- Every user- or operator-visible change gets an entry in [`CHANGELOG.md`](CHANGELOG.md) under `## [Unreleased]`,
  written when its backlog entry is declared done. Released sections are `## [X.Y.Z] - YYYY-MM-DD` matching a `web/vX.Y.Z` tag, with `version`
  in `package.json` kept in step — both edited by hand. `npm run release-ready` only reports whether a release
  can be cut (changelog closed into an undated `## [X.Y.Z]` heading, `package.json` matching, nothing struck out
  in `NEXT-ITERATIONS.md`); branch/working-tree checks, date stamping, tagging and the GitHub release happen from
  the repository root — see the [monorepo README](../README.md#development-workflow). Changes to the Go CLI go in
  [`../go/CHANGELOG.md`](../go/CHANGELOG.md) instead.
- This README is the single source of truth for what the browser does today; deliberate scope cuts go in
  [`NEXT-ITERATIONS.md`](NEXT-ITERATIONS.md). No other Markdown files live here.
- **Work starts from the backlog.** Every change is a numbered entry in `NEXT-ITERATIONS.md`, committed
  before it is implemented — a one-line fix included, as a tiny entry under *Fixes*. `npm run start-item -- <n>`
  is the gate: it refuses on the release branch, on a dirty `web/`, when entry `n` is not in **`HEAD`'s**
  backlog (the working copy does not count) or when it has no outstanding plan item, and otherwise prints the
  entry's Goal and Plan. Exit `2` is a usage error, `1` a refusal.
- Delivered plan items are **struck through** in `NEXT-ITERATIONS.md` while the branch is open, so it can be
  reviewed against what its entries set out to do. `README.md` and `CHANGELOG.md` are written when the entry
  is declared done, after the work has been verified by hand — from the entry, the diff and the
  implementation notes — in the same commit as the archive. When an entry is done it is **archived, never
  deleted**: moved with its full plan to
  `../.claude/archive/web/<finished-date>-<slug>.md` (frontmatter: title, status `done` or `dropped`, dates,
  branch, and `changelog: Unreleased` until the release stamps the version), and the remaining entries are
  renumbered. The changelog records what shipped and why; the archive keeps how. Nothing under the archive is
  read unless asked for. An abandoned entry is archived as `dropped` with a one-line reason.
- `npm run branch-ready` (or `make branch-ready-web` from the repository root) reports whether a feature or
  fix branch is ready to ship: tests, lint and build pass, nothing is left struck out in `NEXT-ITERATIONS.md`
  (done entries archived) and the rest is numbered contiguously, `## [Unreleased]` records the work, `version`
  is untouched — bumping it and closing the changelog belong to the release — the branch is not the release
  branch, `NEXT-ITERATIONS.md` changed or an entry was archived on the branch, and every entry archived as
  done on the branch grew `## [Unreleased]`. It edits nothing, and unlike `release-ready` it reports every
  check and **exits non-zero if any of them failed**, so it can gate a merge. An empty `[Unreleased]` is
  reported, not failed: a branch with no user- or operator-visible effect legitimately has none. On GitHub the
  pipeline (tests, lint, build) runs as the status check `ci-web` on every push and the report half (`npm run
  branch-ready:report`: preflight and branch report, no pipeline) as the status check `branch-ready-web` on
  the pull request (the `branch-ready` workflow in `.github/workflows/`); branch protection requires both —
  see the [monorepo README](../README.md#development-workflow).
- It **refuses to run while `web/` has uncommitted changes**, before the tests and the build, so the verdict
  describes the commit that will be merged rather than the editor's current state. That preflight is a
  read-only `git status --porcelain`, scoped to `web/` so an unrelated edit in `../go` cannot block it. The
  branch checks and the start gate run further read-only git (`rev-parse`, `merge-base`, `diff`, `show`)
  against the merge-base with the release branch (`RELEASE_BRANCH`, default `main`), all through
  `scripts/lib/git.js`; outside a clone (no git, no repository) every git-backed check says so and is skipped.
  `release-ready` runs no git at all — it only adds a line listing the archived entries the release will stamp.

## Known limitations

Deliberate scope cuts are listed in [`NEXT-ITERATIONS.md`](NEXT-ITERATIONS.md): no search, no highlighting of
code fences *inside* documents, no YAML view for resources the index does not list (excluded bulk types,
unreferenced groups), single-segment tenant routes only, no table of contents for the summary, no explicit
dark-mode toggle, and no export format other than Confluence HTML and the drift report PDF (whose own limits
are under [Confluence export](#confluence-export) and [Drift report PDF](#drift-report-pdf)). Navigation groups by resource type; the `platformGroup`/`functionGroup`
frontmatter the index can carry is shown as badges when present rather than driving the tree.

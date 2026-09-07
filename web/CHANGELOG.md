# Changelog

All notable changes to this project (the documentation browser in `web/`) are documented in this file.
Changes to the Go CLI live in [`../go/CHANGELOG.md`](../go/CHANGELOG.md).

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

This project is released independently of the CLI in `go/`: its releases are tagged `web/vX.Y.Z` in the
monorepo and the versions below are its own, unrelated to `go/`'s. Compatibility with an export is stated per
release as the highest `docs/index.yaml` schema version this browser reads. See the **Releasing** section of
the [repository README](../README.md) for the procedure.

## [Unreleased]

## [0.1.0] - 2026-09-07

### Added

#### Release Workflow 

- **`npm run release-ready` reports whether a release can be cut.** It runs the tests and the build first, then
  looks at this changelog: when `[Unreleased]` has not been closed into a new, still undated version section it
  reports that no release is needed and succeeds; otherwise it checks that `NEXT-ITERATIONS.md` has no
  struck-out entries (work that shipped but was never recorded here), that `[Unreleased]` is empty and that
  `package.json` carries that version. Every check is run and reported with what to do about it; the script only
  fails when all of them fail. It edits nothing and runs no git command: closing the changelog and bumping the
  version are done by hand, and branch and working-tree checks, stamping the release date, tagging and the GitHub
  release are a separate step at the repository root that runs this report first and acts on every project whose
  newest heading is undated, so the browser and the CLI stay independently versioned. Procedure in the monorepo
  README.

#### The browser

- **Read-only docs browser (NestJS 11 + Express, Handlebars, Tailwind CSS v4).** Server-rendered throughout,
  with **no client-side JavaScript**. The app never writes, moves or deletes anything under the docs root and
  never calls Azure; no route mutates state.

- **Routes.** A tenant picker, a health endpoint reporting how many documents exist and how many in-scope
  resources are still pending, a tenant landing page, and a document route with an optional `.md` suffix.
  Anything that does not resolve to a Markdown file inside the tenant renders the 404 view. The agent prompt
  the CLI writes at the docs root is **never served**: it is tool input, not documentation, and blocking it is
  a serving decision that leaves a real document of the same name deeper in the tree unaffected.

- **Path safety.** A request path is attacker-controllable, so one guard turns it into a filesystem path: it
  rejects null bytes, absolute paths and any `..` segment before touching the filesystem, serves exactly one
  extension per served root, and re-verifies containment **after** resolving symlinks so none can escape the
  tenant folder. Error responses never leak an absolute path, a stack trace or a raw exception message.

- **Tenant discovery.** A tenant is a directory containing a readable `docs/index.yaml`, the navigation index
  `azure-rd docs generate-index` writes — so an export that has not had it run is not discovered, and there is
  no `index.md` or manifest to maintain. A tenant's document root is the export's `docs/` folder rather than
  the export itself, which is what the relative links inside the documents resolve against. The scan is
  depth-bounded, a matched tenant owns its whole subtree, housekeeping folders are skipped, and counts come
  from the index rather than from walking the tree. A malformed or unreadable index makes the folder *not a
  tenant* instead of crashing discovery.

- **Index parsing and navigation building.** A pure, Nest-free module, so parsing and grouping stay
  unit-testable without a module. It validates defensively — unexpected field types coerced, unknown fields
  ignored — so operator-supplied content cannot take the process down, and it accepts any schema version at or
  above the floor rather than an exact match: the index is *also* the tenant marker, so refusing a newer
  schema would not degrade a page, it would make the whole tenant disappear from the picker.

- **Markdown rendering.** Exactly one `markdown-it` instance, with raw HTML enabled because the
  `<details>`/`<summary>` disclosure blocks *are* the generated documentation and must pass through untouched.
  The typographer stays off: it mangles quotes and dashes in configuration values. Frontmatter is stripped and
  exposed as page metadata, so it never reaches the body. Headings get anchors through a local slug function,
  because the default percent-encodes anything outside its allowed set, and `&` slugs to `and` so a heading
  keeps one identity across the CLI's rename of it.

- **Link rewriting.** Only *relative* `.md` links are rewritten, resolved against the current document's
  directory and prefixed with the tenant segment. Anchors, absolute routes, schemes, protocol-relative URLs,
  non-`.md` targets and links escaping the tenant root are left unchanged.

- **No-restart freshness.** Regenerated documents appear on the next request: renders are keyed by the file's
  modification time and size from a per-request stat, in a bounded cache. The parsed index is cached the same
  way, and newly generated tenants appear within the discovery TTL.

- **Shared app wiring.** Static assets, views directory, partial registration and the view engine are
  configured in one place used by both the bootstrap and the e2e tests, so tests exercise the same wiring as
  production.

- **ESM escape hatch.** A small dynamic-import helper, needed because TypeScript would otherwise down-level
  `import()` to `require()`, which cannot load the project's ESM-only dependencies.

- **Self-contained project under `web/`.** The Go CLI lives in the sibling `go/` folder and the export tree at
  the repository root, which the `DOCS_ROOT` default points at. That variable is the **only** coupling to the
  downloader — nothing here imports from, shells out to, or depends on the Go project — so this folder could
  be split into its own repository unchanged. It carries its own rules, README and changelog.

- **`.env.example` documents every environment variable the browser reads**, with its built-in default, so
  sourcing it unchanged behaves exactly like starting the server with an empty environment. It is a
  **reference, not a mechanism**: nothing loads it and there is no config file, so configuration remains
  environment variables read at their point of use. A real `.env` is gitignored so a copy holding an
  operator's paths cannot be committed.

#### Views and navigation

- **Tenant landing page.** It renders the generation agent's tenant-wide management summary when the export
  carries one, and otherwise falls back to a listing built from the index: resources grouped by type, their
  one-line summaries, a *pending* marker for resources with no document yet, count-only assignment badges so
  resolved names stay in the documents, and a banner when the export reports itself incomplete. The summary is
  deliberately optional and is *not* the discovery marker, and its existence is checked at render time, so one
  written after the discovery cache was filled still appears on the next request.

- **Sidebar navigation on every page.** The per-document list moved out of the landing-page body into a
  persistent sidebar, so the tenant's counts, export timestamp and caveats survive on document pages instead
  of only on the landing page. It groups by resource type in one collapsible section per type and marks the
  current document and opens its section server-side, which is what makes the tree usable **without any
  client-side JavaScript** — the one non-negotiable a sidebar could easily have broken. On narrow viewports
  the column stacks below the content rather than pushing the document down the page.

- **The sidebar can be filtered along every taxonomy axis the CLI resolves, combining them.** Each axis the
  index declares becomes a chip group headed by the axis's own label, selectable through repeatable query
  parameters that narrow the tree server-side with OR within an axis and AND across axes. The whole selection
  rides along in every link, so a click toggles exactly one value and never drops another axis, and a reset
  plus a *showing N of M* line state what is applied. **No client-side JavaScript**: the filter is query
  parameters and links, not a widget. It is axis-agnostic — no axis is named in the code — so an axis added to
  the CLI's config appears here with no change, and membership is read from the index, never derived, because
  deriving it would let the browser and the Confluence export disagree.

  Each axis offers an *uncategorised* bucket, so a taxonomy that stops matching shows up as a full bucket
  rather than a quietly thinning tree, and counts are computed with the axis's own choice removed, so picking
  one value does not drive its siblings to zero. An index with no taxonomy renders exactly the per-type tree
  it did before, and an older single-axis index keeps working through a shim, so exports on disk need no
  regeneration.

- **The exported source YAML is browsable next to the documentation.** A resource route renders the YAML a
  document was written from, syntax highlighted, with every line addressable, plus a raw form for copy-paste;
  a switcher in the top bar flips between the two representations. This makes the export's `resources/` folder
  a **second served root** — the one architectural invariant the feature replaces, and it is replaced by an
  equally tight one rather than loosened: a single guard serves both roots and allows exactly one extension
  each, so a document can never be served from the resources root, nor a resource from the docs root. The app
  remains **read-only**, freshness is unchanged in kind, and navigation stays index-derived — a document's
  source is located by inverting the CLI's own path mapping, so no directory is walked. Highlighting degrades
  to plain preformatted text for very large files or a failed load rather than stalling a request.

  The document's own echo of its source path is not rendered, since the frontmatter carries the same value and
  the page already renders it as a link. It is dropped only when it matches *that* document's source and sits
  alone on its line, so a sentence mentioning another resource's file is untouched; nothing is written to the
  export, and the removal becomes a no-op once the generation template stops emitting the line.

- **Every declared document section has a visual identity.** The CLI declares each document type's headings as
  a closed, verbatim set, which makes the heading text a machine contract rather than prose: each heading and
  the whole block between two of them is tagged with its slug, and the stylesheet gives each an icon and one
  of four role colours — risk, substance, relations, meta — rather than a colour per heading. The
  settings-bearing sections drop into a denser mode, which is the point: one document in the reference export
  carries 317 settings nested five levels deep. A heading *outside* the vocabulary is left entirely untreated,
  which is what makes this safe on documents that predate the contract, and the tool-maintained marker pairs
  become selectable elements without ever straddling a section boundary. The non-negotiables hold: the icons
  are CSS masks, so **still no client-side JavaScript**; it is one pass over the token stream inside the
  single renderer and its cache; and the Confluence export stays byte-identical.

- **Setting blocks are styled from the attributes the generator writes.** A block the generator called out in
  the Security section gets a risk rail and a chip on its own summary line; one it marked as present but
  without effect is de-emphasised behind a dashed rail. The chips are generated content driven by one custom
  property, so they cost no markup and no JavaScript, and they are decorative by construction — the same fact
  is stated in the block's prose, so nothing is lost where CSS is not.

- **The tenant summary's Findings table is styled as a findings list, with the severity as an icon.** The
  severity word is replaced visually by a masked icon per level with a coloured stripe down the row, while the
  word stays in the DOM so assistive technology still reads it. The table is keyed off its columns rather than
  off the heading above it, so it keeps its treatment wherever the generator moves it, and a severity outside
  the closed set is left as plain text rather than given a misleading icon. The vocabulary and column order
  are a contract from the CLI's generation template, not a guess by the browser.

#### Confluence export

- **A tenant's documentation can be exported as Confluence HTML.** One zip containing one folder, which is
  what Confluence's HTML import expects: the folder name becomes the space name and each file name becomes a
  page title. Pages are titled from the index, which keeps a flat, hierarchy-less space readable and makes
  cross-type name clashes impossible, and an overview page built from the tenant summary stands in for the
  sidebar an imported space does not have. Serialisation is an allowlist, not a blocklist: constructs the
  importer does not preserve are unwrapped, scripts and embeds are dropped, and anything that is not HTML at
  all is escaped. The `<details>` settings blocks pass through untouched, and a real Confluence Cloud import
  confirms this is the right representation: each becomes a **native collapsible expand**, keeping its summary
  line, nesting and inline formatting. The app stays **read-only** — nothing is written, and the archive is
  assembled in memory and streamed — and the export re-serialises the HTML the browser already produced rather
  than rendering again, so it cannot bypass the render cache. Re-exporting an unchanged tenant produces the
  same bytes. It is **one-way**: importing creates a space rather than updating one.

- **The Confluence export can index a space by taxonomy axis, through the same rule as the sidebar filter.**
  The overview page used to offer a by-type list only, so everything the sidebar chips can slice was dropped
  at the export boundary — a reader of an imported space could reach a page by its Azure type but not by
  platform or programme. The axis rule is now one implementation with two consumers, so the browser and the
  export cannot come to classify differently, and membership, labels and display order stay read from the
  index. Each axis renders as a heading with one collapsed section per value, which the importer turns into a
  native expand, so several axes over hundreds of resources read as a few scannable lines each. An axis is not
  a partition: a page appears under every value it holds, the uncategorised bucket is always rendered, and
  every count is the number of links actually beneath it. Which index a space gets is an operator default read
  from the environment — by type, by axis, or both — with by-type the default, so an operator who upgrades and
  changes nothing gets the same bytes as before, and an unrecognised value lands on that same no-change mode
  rather than failing an export.

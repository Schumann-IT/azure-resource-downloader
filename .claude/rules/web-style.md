---
paths:
  - "web/src/**"
  - "web/test/**"
  - "web/views/**"
  - "web/scripts/**"
  - "web/eslint.config.mjs"
  - "web/eslint-suppressions.json"
---

# Web style, lint and testing (`web/`)

## Lint
- `eslint.config.mjs` is the single lint truth (WebStorm's automatic ESLint mode runs the same file). Fix the
  finding, or silence it at the one site with `eslint-disable-next-line <rule>` and a reason; stale directives
  are errors. Never weaken the config to go green; adding or removing a rule belongs in `CHANGELOG.md`.
  `@typescript-eslint/no-explicit-any` is deliberately off; `no-console` is on (the one directive in
  `src/main.ts` keeps `console` to the single startup line).
- `eslint-suppressions.json` is a **debt ledger, not an escape hatch**: counts per file and rule for findings
  that predate the sonarjs rules. **Never run `npm run lint:baseline` to make your own change pass** — a new
  violation exceeding a count is the signal it exists for. It only shrinks: when editing a baselined file anyway,
  pay the finding off and run `npm run lint:prune`. Growing it needs the user's agreement and a changelog entry.
  Pay it off a rule at a time and run `npm run lint:prune` after each, so the diff shows what was paid off; a
  finding you decide to keep becomes an `eslint-disable-next-line` with its reason at the site, never a ledger
  entry (a baseline must not be where a standing choice hides). Paying off is internal (no changelog unless a
  reader sees a difference; a rewritten regex gets a spec case pinning the same accepted and rejected inputs);
  deleting the ledger and its two scripts is operator-visible and gets one.

## TypeScript style
- CommonJS modules, `strictNullChecks` on: handle `undefined` explicitly, no `!` assertions.
- Exported functions, services and interfaces get a short comment stating **why**. Keep the existing "why"
  comments (ESM import hack, `html: true`, `typographer: false`) when editing nearby.
- `async`/`await` with `fs.promises`. The only sanctioned sync filesystem calls are the `realpathSync`
  containment checks in `path-safety.ts`.
- Imports at the top; the one exception is `require('hbs')` in `configure-app.ts` (CommonJS singleton whose
  `registerPartials` must stay bound).
- Nest DI by constructor injection with `private readonly`; no module-level mutable state — caches are
  instance fields on a service.
- `any` stays confined to the untyped `markdown-it` plugin surface, parsed YAML and the dynamically imported
  highlighter; new code gets real types (review rule, not a lint gate).

## Error handling
- Missing or unreadable tenants/documents are **normal**: map to the 404 render.
  `MarkdownRendererService.render` throws and the controller converts — keep that contract.
- Never surface an absolute filesystem path, stack trace or raw exception message to the client.
- A malformed `index.yaml`, unreadable directory or bad file degrades (skip / 404), never takes the process down.
- No per-request logging.

## Frontend / CSS
- Tailwind utility classes live in the `.hbs` templates; `src/styles.css` holds only theme tokens and what
  Tailwind/typography cannot express (`<details>`/`<summary>`, table overflow, section identity, dark mode,
  print). Templates outside `views/` must be added with `@source` or their classes are purged.
- Every visual state has a dark variant (`prefers-color-scheme` / `dark:`); interactive elements keep a
  visible `:focus-visible` outline; no anchor nested inside an anchor.
- Read a document's headings from `markdown-it`'s tokens, the way `section-hooks.ts` does, never with a
  line-based regex: generated documents contain `##` lines inside fenced code blocks.
- **Query-parameter options** (facets, export options, view switches): a named choice with a stable id, so a
  chosen variant has exactly one URL; an unknown or malformed value falls back to the default, never a 404 —
  the way `parseFacetSelection()` validates a selection; every option has a default, so an option-less URL keeps
  working.
- **Tables are styled by content, never by position.** Column treatments (alignment, width, wrapping) key on
  attributes the renderer sets from the token stream — `data-numeric` on a count column, or a column-name tag
  the way `findings-table.ts` tags the drift findings' cells — never on `:first-child`, `:last-child` or
  `:nth-child()`: the generator leaves most tables' column order to the agent, and both orders occur. The one
  exception is a table whose column order is a CLI contract — today the Findings tables (`table.findings`, led
  by Severity) and the label | value metadata table (`table.doc-metadata`); its selector names that contract in
  a comment.
- Counts are right-aligned, `tabular-nums` and `nowrap`; prose and list columns wrap (`white-space: normal`,
  `overflow-wrap: anywhere`).
- `{{{body}}}` (triple-stache) only for already-rendered Markdown HTML; everything else `{{ }}` escaped.
- **No client-side JavaScript and no frontend framework.** HTML interactivity (`<details>`, `:target`,
  `<form method="get">`) is in bounds; shipped script and client-side state are not.

## Testing
- Jest, `*.spec.ts` under `test/`. **No network, no dependency on the real export tree**: fixtures in
  `fs.mkdtemp` directories, cleaned up in `afterAll`. e2e tests build the app from `AppModule` and call
  `configureViews(app)` — the same wiring as production.
- Required coverage: `path-safety.ts` → `test/path-safety.spec.ts`; `tenant-index.ts` →
  `test/tenant-index.spec.ts`; routes, discovery, rendering, highlighting or link rewriting →
  `test/docs.e2e.spec.ts`; `src/styles.css` → `test/styles-build.spec.ts`.
- A new or changed table treatment gets a renderer test for **every column order the generator may produce**,
  asserting the tag lands on the right cells; the guard in `test/styles-build.spec.ts` fails on any positional
  table-column selector outside the contract allow-list.
- Keep the invariant tests: `_`-prefixed and index-less/malformed folders ignored; `docs/generate.md` never
  served; frontmatter never in the body; `<details>` passes through; cross-type links resolve; traversal
  404s; an edited document, resource or index is reflected on the next request; no `.md` servable from
  `resources/`; the switcher absent for a document with no `source`.
- Tests get **no** changelog entry.
- Failing tests: assume they are right; produce the Failure Handling Report (`/test-failure-report`) before
  editing anything; never weaken or delete an assertion to go green.

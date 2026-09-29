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
  violation exceeding a count is the signal it exists for. It only shrinks: when editing a baselined file
  anyway, pay the finding off and run `npm run lint:prune`. Growing it needs the user's agreement and a
  changelog entry.

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
- Keep the invariant tests: `_`-prefixed and index-less/malformed folders ignored; `docs/generate.md` never
  served; frontmatter never in the body; `<details>` passes through; cross-type links resolve; traversal
  404s; an edited document, resource or index is reflected on the next request; no `.md` servable from
  `resources/`; the switcher absent for a document with no `source`.
- Tests get **no** changelog entry.
- Failing tests: assume they are right; produce the Failure Handling Report (`/test-failure-report`) before
  editing anything; never weaken or delete an assertion to go green.

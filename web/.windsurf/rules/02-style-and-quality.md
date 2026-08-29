---
trigger: always_on
description: Style, quality and testing requirements for the documentation browser
---

# Docs Browser — Style & Quality

## Commands
Run everything from this folder (npm scripts, not raw binaries):

- `npm install` — dependencies
- `npm run build` — CSS build + `nest build`
- `npm run start:dev` — Tailwind watch + Nest watch
- `npm run start:prod` — build, then run `dist/main.js`
- `npm test` — Jest (needs `--experimental-vm-modules`, already in the script)

There is no lint script and no ESLint config in this project; the `eslint-disable` comments in the
source are historical. Do not reference `eslint`/`prettier` commands in docs until they are actually
wired up. **Never use the Go `Makefile` in the parent folder** — it does not apply here.

## TypeScript style
- Match `tsconfig.json`: CommonJS modules, `strictNullChecks: true`, `noImplicitAny: false`.
  `strictNullChecks` is on — handle `undefined` explicitly rather than asserting with `!`.
- Public/exported functions, services and interfaces get a short comment stating **why**, not what.
  Keep the existing "why" comments (they document non-obvious constraints such as the ESM import
  hack, `html: true`, `typographer: false`) — do not delete them while editing nearby code.
- Prefer `async`/`await` with `fs.promises`. The only sanctioned sync filesystem calls are the
  `realpathSync` containment checks in `path-safety.ts` (that function is deliberately synchronous
  and pure).
- Imports at the top of the file. The one exception is the `require('hbs')` in `configure-app.ts`,
  which is intentional (CommonJS singleton whose `registerPartials` must stay bound).
- Nest DI via constructor injection with `private readonly`. No module-level mutable state; caches
  live as instance fields on a service.
- Keep `any` confined to the untyped `markdown-it` plugin surface. New code gets real types.

## Error handling
- Missing or unreadable documents/tenants are **normal**, not exceptional: map them to a 404 render.
  `MarkdownRendererService.render` throws and the controller converts it — keep that contract.
- Never surface an absolute filesystem path, stack trace or raw exception message to the client.
- A malformed manifest, unreadable directory or bad file must degrade (skip / 404), never take the
  process down.
- Do not log per-request noise; `console` use is limited to the single startup line in `main.ts`.

## Frontend / CSS
- Tailwind utility classes go in the `.hbs` templates; `src/styles.css` holds only the theme tokens
  and the rules Tailwind/typography cannot express (`<details>`/`<summary>`, table overflow, dark
  mode).
- Tailwind v4 scans sources: any new template directory outside `views/` must be added with
  `@source` in `src/styles.css`, or its classes are purged.
- Every visual state needs a dark variant (`prefers-color-scheme` / `dark:`).
- Interactive elements keep a visible `:focus-visible` outline.
- `{{{body}}}` (triple-stache) is used **only** for already-rendered Markdown HTML. All other values
  use `{{ }}` escaping.
- Do not introduce client-side JavaScript or a frontend framework.

## Testing
- Jest, `*.spec.ts`, under `test/`.
- **No network, no dependency on the real export tree.** Tests create fixtures in a temp dir
  (`fs.mkdtemp`) and clean up in `afterAll`.
- e2e tests build the app from `AppModule` and call `configureViews(app)` — the same wiring as
  production.
- Required coverage for changes:
  - touching `path-safety.ts` → new/updated cases in `test/path-safety.spec.ts`;
  - touching routes, discovery, rendering or link rewriting → a case in `test/docs.e2e.spec.ts`;
  - touching `src/styles.css` → assert the rule survives compilation in `test/styles-build.spec.ts`.
- Keep the invariant tests: discovery must ignore `_`-prefixed and marker-incomplete folders,
  frontmatter must not appear in the body, `<details>` must pass through, cross-type links must
  resolve, traversal must 404, and an edited file must be reflected on the next request.

## Failing tests: analyze, don't auto-fix
Assume the tests are right and the implementation is wrong. Before editing, produce a short report:
failing test + error, root-cause hypothesis, the minimal implementation fix, and (only if the test
genuinely encodes outdated behaviour) why the test should change instead. Never weaken or delete an
assertion to get green.

## Documentation
`README.md` in this folder is the single source of truth for this project: update it when routes,
environment variables, scripts, layout or the docs-root contract change. Deliberate scope cuts go in
`NEXT-ITERATIONS.md`. Do not create additional Markdown files here.

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
- `npm run branch-ready` — clean-tree preflight, tests + build, then report whether this feature/fix branch
  is ready to ship

There is no lint script and no ESLint config in this project; the `eslint-disable` comments in the
source are historical. Do not reference `eslint`/`prettier` commands in docs until they are actually
wired up. **Never use the Go `Makefile` in the sibling `go/` folder** — it does not apply here.

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
- A malformed `index.yaml`, unreadable directory or bad file must degrade (skip / 404), never take
  the process down.
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
  - touching `tenant-index.ts` → new/updated cases in `test/tenant-index.spec.ts`;
  - touching routes, discovery, rendering, highlighting or link rewriting → a case in
    `test/docs.e2e.spec.ts`;
  - touching `src/styles.css` → assert the rule survives compilation in `test/styles-build.spec.ts`.
- Keep the invariant tests: discovery must ignore `_`-prefixed folders and folders whose
  `docs/index.yaml` is missing or malformed, `docs/generate.md` must not be served, frontmatter must
  not appear in the body, `<details>` must pass through, cross-type links must resolve, traversal
  must 404, and an edited document, resource or index must be reflected on the next request. For the
  YAML view specifically: a `.md` must not be servable from `resources/`, traversal out of the
  resources root must 404, and the top-bar switcher must be absent for a document with no `source`.

## Failing tests: analyze, don't auto-fix
Assume the tests are right and the implementation is wrong. Before editing, produce a short report:
failing test + error, root-cause hypothesis, the minimal implementation fix, and (only if the test
genuinely encodes outdated behaviour) why the test should change instead. Never weaken or delete an
assertion to get green.

## Documentation
`README.md` in this folder is the single source of truth for this project: update it when routes,
environment variables, scripts, layout or the docs-root contract change. Deliberate scope cuts go in
`NEXT-ITERATIONS.md`. Do not create additional Markdown files here.

## Changelog: update it with every change
`CHANGELOG.md` is part of the change, not a follow-up. **Every** change that a user or operator can
notice — routes, views, discovery/rendering behaviour, environment variables, scripts, dependencies,
security boundaries, bug fixes — gets an entry in the same commit/edit, before you report the work as
done. Purely internal edits that change no observable behaviour (a rename, a comment, a test-only
addition) do not need one.

Rules for entries:

- **Format**: Keep a Changelog + SemVer, same prose style as `../go/CHANGELOG.md` — a bolded lead-in
  followed by sentences, not bare one-liners.
- **Place new entries under `## [Unreleased]`**, in the matching `### Added` / `### Changed` /
  `### Fixed` / `### Breaking` subsection (create the subsection if it is missing). Never edit an
  already-released section to sneak in new work.
- **Group entries by feature area under `####` subheadings** once a section gets long — 0.1.0 uses
  *The browser*, *Views and navigation* and *Confluence export*. Put a new entry in the subheading it
  belongs to; add a subheading only for a genuinely new area.
- **One entry per user-visible feature**, the way a squash-merged feature branch would read. A fix that
  only completes an existing feature belongs **inside that feature's entry**; a change with no observable
  effect gets none.
- **Tests get no entry.** Testing is assumed, not announced — the required coverage for a change is listed
  in the Testing section above, and repeating it here adds nothing a reader can act on.
- **Keep entries short: a bolded lead-in plus a few sentences.** Leave out implementation detail (internal
  file, service and symbol names, occurrence counts, defect-by-defect narratives) and configuration detail
  (variable enumerations, defaults, syntax) — **configuration and routes are documented in `README.md`**,
  the single source of truth for them. Say that an option exists and what it is for, and point there.
- **Released sections are SemVer versions with a date** (`## [0.1.0] - 2026-09-06`), newest first, each one
  matching a `web/vX.Y.Z` git tag. This project's version line is its own and is unrelated to `go/`'s.
  Cutting a release means renaming `[Unreleased]` to a bare, **undated** `## [X.Y.Z]`, starting a fresh
  empty `[Unreleased]`, and bumping `version` in `package.json` to match — by hand, and only when asked; the
  date is stamped by the root release script when it publishes, never by hand. `npm run release-ready`
  never edits these files: it only reports whether a release can be cut (empty `[Unreleased]`,
  `package.json` matching, no struck-out `NEXT-ITERATIONS.md` entries, newest heading undated) and runs no
  git command. `npm run branch-ready` is its counterpart for a feature/fix branch and asks the opposite
  questions (`[Unreleased]` **written**, strikeouts cleared and entries renumbered, `version` **untouched**);
  it also changes nothing, but it reports every check and exits non-zero if **any** of them failed, so it can
  gate a merge. It is the one place in this project's tooling that runs git: a read-only
  `git status --porcelain` scoped to `web/`, as a **preflight** that refuses to report on uncommitted changes
  (so the verdict describes the commit that will be merged) and degrades to a skip outside a clone. The
  repository-wide branch and working-tree checks, date stamping, tagging and the GitHub release happen from
  the repository root; the procedure lives in the **Development workflow** section of `../README.md`. The earlier
  `RC1`/`RC2` naming is retired, so do not reintroduce it.
- **Explain *why* and which invariant now holds**, not just what moved. If a change touches a
  non-negotiable (path safety, read-only, one `markdown-it` instance, no client-side JS, no-restart
  freshness), say explicitly how it is preserved.
- **Deliberate scope cuts belong in `NEXT-ITERATIONS.md`** and are referenced from the changelog, not
  duplicated into it.

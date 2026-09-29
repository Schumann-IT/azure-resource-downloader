# Azure Resource Downloader — monorepo

Two independent projects joined only by the export tree on disk (`output/<tenant>/{resources,docs,drift}/`,
gitignored — it is customer configuration):

- `go/` — **azure-rd**, a Go CLI. Exports an Entra ID / Intune tenant as deterministic YAML plus a facts-only
  `resources/metadata.yaml`, writes the prompts an AI agent executes to document the export, compares the live
  tenant against the export (`resource drift`) and builds the browser's `docs/index.yaml`. → `go/CLAUDE.md`
- `web/` — **azure-rd-docs-web**, a read-only NestJS browser for that documentation (source YAML view, drift
  view, tenant compare, Confluence export). Never calls Azure, never writes. → `web/CLAUDE.md`

Each project has its own `README.md` (the single source of truth for what it does today), `CHANGELOG.md`,
`NEXT-ITERATIONS.md`, version line and tag prefix (`go/vX.Y.Z`, `web/vX.Y.Z`). They share nothing else: the
Go rules do not apply under `web/` and vice versa, and `web/` must never import from, shell out to or depend
on `go/` — `DOCS_ROOT` pointing at an export tree is the only coupling.

## Where guidance lives

| Location | Scope |
|---|---|
| this file | monorepo layout, workflow, gates, release |
| `go/CLAUDE.md`, `web/CLAUDE.md` | per-project context, layout, non-negotiables, commands (loaded when working in that folder) |
| `.claude/rules/*.md` | path-scoped detail: Go style, handlers, export safety; web style; changelog policy; backlog lifecycle |
| `.claude/skills/*/SKILL.md` | procedures: `new-handler`, `add-command`, `add-config-option`, `test-failure-report`, `close-branch`, `release` |
| `go/.windsurf/rules/`, `web/.windsurf/rules/` | the Windsurf originals these files were migrated from, kept until that subscription ends. When a rule changes, change it in both places. |

## Development workflow

Work happens on branches off `main`, one feature or fix per branch. Never implement on `main`: the only
commits `main` takes directly are a changelog close and the release stamp.

**The backlog is the only way work enters the codebase.** What a request lets you do
(`.claude/rules/next-iterations.md` has the full protocol):

- Anything that is not `implement item N` (an idea, a promotion, a refinement, a follow-up, an assessment) →
  edit that project's `NEXT-ITERATIONS.md` only. No code. A one-line fix is a tiny entry, not an exemption.
- `promote idea <title>` / `plan idea <title>` / `plan item N` → `/promote-idea`: plan mode seeded with the
  idea, the approved plan becomes the entry, commit. No implementation.
- `implement item N` → `/implement-item`: the start gate first (`make -C go start-item N=<n>` /
  `npm --prefix web run start-item -- <n>`), then the plan, striking items and writing the changelog as they
  land; stop and ask for follow-ups.
- `item N is done` / `drop item N` → `/item-done`: archive the entry to `.claude/archive/<project>/` (kept
  forever, never auto-loaded — `/archive` lists and shows them), renumber, commit.

Three things travel with the code **in the same commit**, never as a follow-up:

1. **`CHANGELOG.md`** of the project — every user- or operator-visible effect gets an entry under
   `## [Unreleased]`; purely internal changes and tests get none. Format and grouping: `.claude/rules/changelog.md`.
2. **`NEXT-ITERATIONS.md`** of the project — a delivered plan item is **struck out** (`~~…~~`), never deleted,
   while the branch is open; a done entry is archived, not deleted. Lifecycle: `.claude/rules/next-iterations.md`.
3. **`README.md`** of the project — routes, flags, settings, environment variables, scripts and supported types
   are documented there, not in the changelog.

Do **not** touch versions: `web/package.json`'s `version` and the changelog version headings are release
concerns and are edited by hand only when the user asks.

### Gates and release (run from the repository root; all of them only report)

- `make branch-ready-go`, `make branch-ready-web`, `make branch-ready` — each refuses while its own folder has
  uncommitted changes, runs the project pipeline, then checks: no strikeouts left in `NEXT-ITERATIONS.md`
  (done entries archived), entries numbered `1..N`, `## [Unreleased]` written (empty is reported, not
  failed), `web/` `version` untouched, not on `main`, the backlog changed on the branch, every entry archived
  as done recorded under `[Unreleased]`. Read-only git only; exit non-zero on any failed check. **Commit first,
  then run**; fix, commit, rerun. Archiving done entries and running the gate is `/close-branch`.
- `make release-ready-go`, `make release-ready-web`, `make release-status` — reports. `make release` stamps
  dates and archive versions, commits, tags, pushes and creates GitHub releases; it runs only on `main` with
  a clean tree and only when the user asks — `/release`. Never tag, push or create a release on your own
  initiative.
- `make sonarqube-*` — optional local SonarQube; gates nothing, never analyses `output/`.

Per-project pipelines (all read-only): `make -C go check` / `make -C go ci`; in `web/`: `npm test`,
`npm run lint`, `npm run build`.

## Repo-wide rules

- **Makefile targets and npm scripts only.** Never run or document raw `go`, `gofmt`, `golangci-lint`, `jest`,
  `eslint`, `tsc` or `nest` commands; use `go/Makefile` (`make -C go help`) and `web/package.json` scripts.
  The only exception is the Makefile's own recipes.
- **`output/` is customer data.** Never commit it, never upload it anywhere, and read from it only when a task
  needs a real export; tests build their own fixtures in temp directories and never touch it.
- **Documentation policy.** Per project exactly three Markdown files: `README.md`, `CHANGELOG.md`,
  `NEXT-ITERATIONS.md`. No `CONTRIBUTING.md`, `docs/` folders or ad-hoc notes; new information goes into the
  project README. The embedded templates (`go/internal/docs/generate_prompt_template.md`,
  `go/internal/drift/analyze_drift_template.md`) are program input, not documentation. Finished backlog
  entries live under `.claude/archive/<project>/`; read them only on request or when reworking the same area.
- **Failing tests: analyse, do not auto-fix.** Assume the tests are right and the implementation is wrong.
  Produce a Failure Handling Report with proposed, unapplied patches first (`/test-failure-report`); never
  weaken or delete an assertion to go green, and apply neither option without explicit confirmation.
- **Secrets.** Never log, print or commit tokens, client secrets, resolved OMA-URI values or `SONAR_TOKEN`.
- **Commit messages** follow the existing history: `feat(go): …`, `fix(web): …`, `chore: …`; the release
  script alone writes `release: go vX.Y.Z, web vX.Y.Z`.

## Toolchain

Go per `go/go.mod` (currently 1.26; the READMEs still say 1.24+), Node.js >= 20, Azure CLI (`az login`),
golangci-lint v2, `gh` for releases. No CI exists: every gate is local.

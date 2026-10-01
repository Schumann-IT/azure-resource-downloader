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
| `.claude/skills/*/SKILL.md` | procedures: `new-handler`, `add-command`, `add-config-option`, `test-failure-report`; workflow: `promote-idea`, `implement-item`, `implement-pair`, `item-done`, `archive`, `close-branch`, `pull-request`, `release` |
| `.claude/agents/*.md`, `.claude/hooks/` | the pipeline agents (`plan-reviewer`, `implementer`, `impl-reviewer`, `qa`) and the Bash guard they run under; launched only by `/implement-pair`. `session-start.sh` reports at startup whether `gh` is logged in |
| `.github/PULL_REQUEST_TEMPLATE.md` | the pull request description every PR follows; `/pull-request` fills it |
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
  `npm --prefix web run start-item -- <n>`), then the plan — code and tests, striking items as they land,
  **no README or changelog yet**; stop so the user can verify. By default this runs the agent pipeline for
  one side.
- `implement pair go N web M` → `/implement-pair`: one Opus plan reviewer refines and commits the backlog
  (it asks you only for decisions that change what ships, and reviews again after each answer), then per
  side in parallel: implementer (tier from the entry), Opus reviewer, Sonnet QA. The session commits per
  project, pushes once after QA and starts a CI monitor that reports back. **Agents never push; only the
  plan reviewer commits, and only the backlog files.**
- `item N is done` / `drop item N` → `/item-done`: **now** write the project's `README.md` and
  `CHANGELOG.md` (from the entry, the diff and the reports), archive the entry to
  `.claude/archive/<project>/` (kept forever, never auto-loaded — `/archive` lists and shows them),
  renumber, commit all of it together.

What travels with what:

1. **Implementation** = code, tests and the **struck-out** plan bullets in `NEXT-ITERATIONS.md` (`~~…~~`,
   never deleted). Nothing else. Lifecycle: `.claude/rules/next-iterations.md`.
2. **Done** = `CHANGELOG.md` (every user- or operator-visible effect under `## [Unreleased]`; internal
   changes and tests get none — `.claude/rules/changelog.md`) + `README.md` (routes, flags, settings,
   environment variables, scripts, supported types: the single source of truth) + the archive file, in one
   commit. The gate refuses an archived entry whose changelog did not grow.

Do **not** touch versions: `web/package.json`'s `version` and the changelog version headings are release
concerns and are edited by hand only when the user asks.

### Gates and release (run from the repository root; all of them only report)

- `make branch-ready-go`, `make branch-ready-web`, `make branch-ready` — each refuses while its own folder has
  uncommitted changes, runs the project pipeline, then checks: no strikeouts left in `NEXT-ITERATIONS.md`
  (done entries archived), entries numbered `1..N`, `## [Unreleased]` written (empty is reported, not failed),
  `web/` `version` untouched, not on `main`, the backlog changed or an entry was archived on the branch,
  Conventional Commits, every entry archived as done recorded under `[Unreleased]`. Read-only git only; exit
  non-zero on any failed check. **Commit first, then run**; fix, commit, rerun. The `-report` variants (`make
  branch-ready-report-go` / `-web`) run the same checks without the pipeline.
- **CI is the quality authority.** The `branch-ready` workflow runs the pipelines (`ci-go`, `ci-web`) on
  every push and the branch reports (`branch-ready-go`, `branch-ready-web`) on the pull request; branch
  protection requires all four. Locally: implementers run build and tests before anything is committed, no
  agent runs lint, and `/close-branch` runs the report targets, pushes and starts a CI monitor that
  reports back.
- `/pull-request` — after `/close-branch`: builds the title (Conventional Commits) and the description
  from the template, shows both for editing, pushes, creates the PR with `gh`, links `#N` back into the
  changelog entries and the archive files. Merging is done on GitHub (squash).
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
- **Agents never commit** — except the plan reviewer, which commits only `NEXT-ITERATIONS.md` files. Git
  otherwise belongs to the main session; a subagent reports what should be committed.
  `.claude/hooks/agent-bash-guard.sh` enforces it for the pipeline agents.
- **Commit messages** follow Conventional Commits strictly — `.claude/rules/commits.md`: `type(go|web)!:
  description`, no scope for repository-level commits, `chore(release): …` from the release script only.
  The branch gate fails on any other subject. Pull request titles obey the same rule (squash merges).
- **GitHub needs `gh`.** `/pull-request` and `/release` check `gh auth status` first and stop without it;
  the session-start hook says at startup whether it is logged in.

## Toolchain

Go per `go/go.mod` (currently 1.26; the READMEs still say 1.24+), Node.js >= 20, Azure CLI (`az login`),
golangci-lint v2, `gh` for releases. The `branch-ready` workflow runs both gates on every pull request into
`main`; branch protection requires them, so the local gates and the CI gate are the same scripts.

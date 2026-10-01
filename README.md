# Azure Resource Downloader

Reproducible, AI-generated documentation of an **Entra ID / Intune tenant** — built from an export of the
tenant's configuration, not from a live session.

The problem it solves: an Intune tenant is hundreds of policies, profiles, apps, scripts and groups whose
relationships (who is assigned what, which group a filter narrows, which template a compliance policy notifies
through) exist only as GUIDs spread across dozens of Graph endpoints. This monorepo turns that into a browsable,
Confluence-exportable set of documents that stays current as the tenant changes, without regenerating
everything on every run.

It is two independent projects plus one shared contract — the export tree on disk:

| Folder | Project | Purpose |
|---|---|---|
| [`go/`](go/README.md) | **`azure-rd`** — Go CLI | Exports the tenant's configuration as clean YAML, records facts about the export in `metadata.yaml`, and drives the AI documentation run: it decides *what* to (re)generate and emits the prompt an agent executes. Also builds the navigation index the browser reads. |
| [`web/`](web/README.md) | **azure-rd-docs-web** — NestJS | Read-only browser for the generated documentation: tenant picker, per-document pages with the source YAML alongside, facet navigation, tenant summary, and a Confluence-importable export. Never calls Azure and never writes. |

The two share nothing but the export tree. Each folder has its own README (the single source of truth for that
project), `CHANGELOG.md`, `NEXT-ITERATIONS.md`, version line and git tags.

## How the pieces fit

```
                azure-rd download                 azure-rd docs generate-prompt      AI agent
 Entra / Intune ───────────────────▶ resources/ ────────────────────────────▶ docs/generate.md ──▶ docs/**/*.md
 (delegated                          ├─ metadata.yaml   (facts)                (work list: what is           docs/summary.md
  Graph + ARM)                       ├─ <type>/*.yaml                           missing or stale)            docs/report-*.md
                                     └─ <type>/doc-prompt.md (per-type spec)
                                                  │
                                                  │  azure-rd docs generate-index
                                                  ▼
                                            docs/index.yaml  ─────────────▶  web/  (browser + Confluence export)
```

1. **Export.** `azure-rd download` signs in as *you* (delegated permissions only — no service principal), lists
   every supported resource type, and writes one YAML per resource under `output/<tenant>/resources/`. The
   output is deterministic (sorted keys, stable list order, collision-free file names), so an unchanged
   resource produces identical bytes and an identical hash across runs. Alongside the YAML it writes
   `metadata.yaml` — facts only: hashes, display names, `@odata.type`, assignment targets — and one
   `doc-prompt.md` per type, the specification an AI must follow when documenting that type.
2. **Decide what to document.** `azure-rd docs generate-prompt` compares `metadata.yaml` against the documents
   already under `docs/` (each document records the hashes it was generated from in its frontmatter) and writes
   `docs/generate.md`: a closed work list of exactly the documents that are missing or stale, plus the blocks
   that must be re-rendered because something *they reference* changed (a group renamed, a policy re-targeted).
   It runs offline and never touches `resources/`.
3. **Generate.** Paste `docs/generate.md` into an AI agent session. The agent writes the documents, resolves
   assignment GUIDs to group names in both directions, writes the tenant landing page `docs/summary.md` and a
   run report `docs/report-<timestamp>.md`. Nothing else in the tree is written by the agent.
4. **Index and browse.** `azure-rd docs generate-index` writes `docs/index.yaml` — the navigation index the
   browser keys everything off, optionally classified along operator-defined facets (a `taxonomy:` in the
   config file). Point `web/` at the output tree and open it; export to Confluence from the tenant picker.

Re-running the whole loop after a tenant change regenerates only what actually moved.

## Prerequisites

- **Go 1.26+** to build the CLI, **Node.js 20+** to run the browser.
- **Azure CLI**, signed in with `az login` as a user who can read the tenant's Intune / Entra configuration.
- **An Entra app registration** for device-code sign-in. Every Microsoft Graph resource type needs delegated
  scopes the Azure CLI's first-party app cannot obtain (`DeviceManagementConfiguration.Read.All`,
  `Policy.Read.All`, …), so a full export always requires one — `az login` alone covers only the three ARM
  types. The Go README walks through creating it:
  [Authentication → Create the app registration](go/README.md#create-the-app-registration).
- An AI agent capable of running a multi-step, multi-file task (the prompt fans generation out to parallel
  subagents and runs scripted verification passes).

## Quick start

The output tree lives at the repo root by default (`output/`, gitignored). The CLI is run from `go/`, so point
it one level up; the browser's default `DOCS_ROOT` already resolves to `../output`.

**1. Export the tenant** (Graph types prompt for the app registration's client and tenant id on first use —
pass `--client-id`/`--tenant-id` to skip the prompt):

```bash
export AZURE_RD_OUTPUT="../output"
cd go
make build
./azure-rd download
```

**2. Emit the documentation prompt** — offline, `--domain` is the export folder name (the tenant's Entra
default domain):

```bash
./azure-rd docs generate-prompt --domain contoso.onmicrosoft.com
```

**3. Run the agent.** Paste `output/<tenant>/docs/generate.md` into a fresh agent session and let it run to
completion. It produces the documents, `docs/summary.md` and a `docs/report-*.md`. Re-running
`generate-prompt --dry-run` afterwards should report nothing pending.

**4. Build the index and browse:**

```bash
./azure-rd docs generate-index --domain contoso.onmicrosoft.com
cd ../web
npm install
npm run start:prod        # http://localhost:3000
```

Repeat from step 1 whenever the tenant changes; steps 2–4 only touch what moved.

## Repository layout

```
azure-resource-downloader/
├── go/          azure-rd CLI (Go 1.26) — see go/README.md
├── web/         documentation browser (NestJS, TypeScript) — see web/README.md
├── output/      export tree, gitignored: output/<tenant>/{resources,docs}/
├── .claude/     Claude Code rules and skills; archive/<project>/ keeps every finished backlog entry with its plan
├── Makefile     branch-ready gates and release entry points for both projects (see Development workflow)
├── scripts/     release.sh — tags and publishes prepared releases
└── README.md    this file
```

Per-project rules for editors and AI assistants live in `go/.windsurf/rules/` and `web/.windsurf/rules/`;
they apply only to their own folder. Claude Code reads the same rules from `CLAUDE.md` (root, `go/`, `web/`),
the path-scoped files in `.claude/rules/` and the procedures in `.claude/skills/`; the two sets are kept in step.

## Development workflow

Work happens on branches and ships in four steps: plan, implement, close the branch, release. Every change —
a feature, a fix, a one-line correction — **starts as a numbered entry** in the project's `NEXT-ITERATIONS.md`,
committed before it is implemented, and ends as an **archived** entry whose full plan is kept for later
review. The first three steps are per branch and gated by tooling that only *reports*; the fourth is run from
`main` when there is something to publish. Each project has its own SemVer line, tagged with a folder prefix so
the two never collide and each project's `git describe` finds only its own tags:

| Project | Tag pattern | Version source |
|---|---|---|
| `go/` | `go/vX.Y.Z` | `git describe --match 'go/v*'` at build time — `make build` stamps it into `--version` and into every `resources/metadata.yaml` (`toolVersion`) |
| `web/` | `web/vX.Y.Z` | `version` in `web/package.json` |

### 1. Plan: the entry comes first

Branch off `main`, one feature or fix per branch, in whichever project(s) it touches. Before any code, the
work is an entry in that project's `NEXT-ITERATIONS.md` — a `**Goal.**`, optional notes, and a `**Plan.**`
of concrete items (the anatomy is in each project's rules). A parked idea is *promoted* into an entry
rather than copied; a trivial fix gets a tiny entry. **Commit the entry.** The start gate checks exactly
that:

```bash
make -C go start-item N=2            # go/: may entry 2 be implemented?
npm --prefix web run start-item -- 1 # web/: may entry 1 be implemented?
```

Each refuses on `main`, on a dirty project folder, when entry `N` is not in **`HEAD`'s** backlog or has no
outstanding plan item — and otherwise prints the entry's Goal and Plan. Exit `2` is a usage error, `1` a
refusal.

### 2. Implement from the entry

Implementation is code, tests and the plan: as plan items land, **strike them out** in `NEXT-ITERATIONS.md`
(`~~…~~`) rather than delete them, so a reviewer sees what the branch set out to do beside what the diff
does; strike the title once the whole plan is delivered. Follow-ups discovered on the way are new, unstruck
plan items or a new entry. `README.md` and `CHANGELOG.md` are **not** written at this point — see step 3 —
so the work can be verified by hand first; documentation-only plan items stay unstruck until then.

Do **not** touch the version: `web/package.json`'s `version` and the changelog's version headings are release
concerns (step 4). Run the project's own checks as you go — `make -C go check` / `make -C go ci`, or `npm test`,
`npm run lint` and `npm run build` in `web/`; all of them are read-only and leave the tree as they found it.

### 3. Close the branch: document and archive what is done, then gate

Once the work is verified, an entry is declared done. That is when its documentation is written and it is
**archived, never deleted**, in one commit:

- **`CHANGELOG.md`** — every user- or operator-visible effect gets an entry under `## [Unreleased]`, written
  the way a squash-merged branch would read, from the entry's goal, the diff and the implementation notes.
  Purely internal changes get none. Each project's rules spell out the format
  (`go/.windsurf/rules/02-style-and-quality.md`, `web/.windsurf/rules/02-style-and-quality.md`).
- **`README.md` of the project** — the single source of truth for what the tool does today; routes, flags,
  settings, environment variables and scripts are documented there, not in the changelog.
- **The archive** — the entry (or, for a partially delivered one, its struck plan items) moves with its full
  text to `.claude/archive/<project>/<finished-date>-<slug>.md`, with a frontmatter naming the title,
  `status: done` (or `dropped`, for an abandoned entry, with a one-line reason), the dates, the branch and
  `changelog: Unreleased` — the release stamps the version later. The remaining entries are renumbered
  `1..N`. The changelog records *what* shipped and *why*; the archive keeps *how*, and nothing under it is
  read unless asked for.

Then run the gate for each project the branch touched (or `make branch-ready` for both):

```bash
make branch-ready-go     # → make -C go branch-ready
make branch-ready-web    # → npm --prefix web run branch-ready
```

Each gate first **refuses to run while its own folder has uncommitted changes** — a read-only
`git status --porcelain` scoped to that folder, so the verdict describes the commit that will be merged and
an edit in the sibling project cannot block it. It then runs the project's own pipeline (`go/`: `make ci`;
`web/`: tests, lint and build) and reports: nothing left struck out in `NEXT-ITERATIONS.md`, the remaining
entries numbered `1..N`, `## [Unreleased]` written, the branch is not `main`, `NEXT-ITERATIONS.md` changed or
an entry was archived on the branch (every change starts as an entry — except a **dependency-only** branch that
changes only `go/go.mod`/`go.sum` or `web/package.json`/`package-lock.json`, such as a weekly Dependabot pull
request from `.github/dependabot.yml`; its `ci-*` checks and the Go golden test prove it), and every entry
archived as done on the branch grew
`## [Unreleased]`; `web/` also checks that `version` was left alone (`go/` has no version file — its version
is the tag). The branch checks use read-only git (`rev-parse`, `merge-base` against `RELEASE_BRANCH`, `diff`,
`show`) and are skipped with a note outside a clone. An empty `[Unreleased]` is reported, not failed: a
branch with no user-visible effect legitimately has none. Every check is reported, nothing is edited, and the
gate **exits non-zero if any check failed**, so it can back a merge check. Details in the
[go README](go/README.md#development) and the [web README](web/README.md#development-conventions).

The same gates run on GitHub, in two rhythms. The `branch-ready` workflow (`.github/workflows/branch-ready.yml`)
runs each project's **pipeline** (`make -C go ci`; `npm test`, `npm run lint`, `npm run build`) as the status
checks `ci-go` and `ci-web` on **every push**, and each project's **branch report** — the gate without its
pipeline, `make branch-ready-report-go` / `-web`, seconds — as the status checks `branch-ready-go` and
`branch-ready-web` on the **pull request**. The report checks are expected to be red while entries are still
struck out and turn green once the branch is closed: they are the merge gate. A `changes` job runs a
project's jobs only when the pull request touches that project, its archive, the root `Makefile` or the
workflow; a skipped job counts as passed, so every check always reports (there is deliberately no path filter
on the trigger — a required check that never reports would block merging for good). Branch protection on
`main` requires all four checks and an up-to-date branch, applies to administrators, and merges are squash
only with the pull request title as the commit subject — repository settings made by hand.

Every commit on the branch follows [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/)
— `type(go|web)!: description`, no scope for repository-level commits — and the gate fails on any subject that
does not. Open the pull request once the gate is green: its title is the squash commit that lands on `main`
and follows the same rule, and its description follows `.github/PULL_REQUEST_TEMPLATE.md` (summary, backlog
entries, the changelog lines added, user actions, gate output, and the archived plans). Once the pull request
exists, its number is appended to the changelog entries it carries (` (#N)`) and recorded in the archive files
(`pr:`). Merge with squash-and-merge. Nothing is tagged or published at this point; `main` accumulates
`[Unreleased]` entries from every merged branch until someone decides to release. A project whose
`[Unreleased]` stays empty is simply never released.

### 4. Release

Releasing is run from the repository root on `main`. A project that did not change is never closed, so it gets
no tag and no release.

**Close the changelog by hand** for each project that changed: in its `CHANGELOG.md`, rename
`## [Unreleased]` to a bare `## [X.Y.Z]` — **no date**; the publish step stamps it — and start a fresh, empty
`## [Unreleased]` above it. For `web/`, also set `version` in `package.json` and `package-lock.json`
(`npm version X.Y.Z --no-git-tag-version` in `web/`). Commit and merge to `main`.

**Report whether a release can be cut** — each project's `release-ready` goal only reports; it changes nothing
and runs no git command:

```bash
make release-ready-go    # → make -C go release-ready
make release-ready-web   # → npm --prefix web run release-ready
```

Both first run the project's own pipeline, then look at `CHANGELOG.md`. If its newest version heading is not an
undated `## [X.Y.Z]` — there is none, or it already carries a date — nothing has been closed and the goal
reports **no release needed** and succeeds. Otherwise it checks that `NEXT-ITERATIONS.md` has no struck-out
entries (the backstop for step 3) and that `## [Unreleased]` is empty; `web/` additionally checks that
`package.json` carries that version; both list the archived entries the release will stamp. Every check is run
and reported — passing ones with ✅, failing ones with ❌ and what to do about it — and the goal exits non-zero
only when *all* checks fail, so it is a report to read, not a gate that stops on the first problem.

**Publish** — `make release` (and `make release-status`) run both `release-ready` goals first as make
prerequisites, so a project whose pipeline fails stops the release before anything happens. Then the publisher
verifies the repository state: the current branch must be `main` (override with `RELEASE_BRANCH=<name>`) and
the working tree clean, since it tags and pushes `HEAD`. These two checks live only here — the `release-ready`
goals run no git at all, and the branch gates run only read-only git.

Then, for every project whose newest changelog heading is an undated `## [X.Y.Z]`, it stamps today's date onto
that heading (`## [X.Y.Z] - YYYY-MM-DD`), stamps `changelog: X.Y.Z` into every archived entry of that project
still marked `changelog: Unreleased`, and commits (`chore(release): go vX.Y.Z, web vX.Y.Z`) — the only changelog and
archive edits any tooling makes — then tags that commit `<project>/vX.Y.Z`, pushes the branch and the tags, and
creates a GitHub release titled `<project> vX.Y.Z` whose notes are that changelog section. Projects whose
newest heading is already dated are skipped; an undated heading whose tag already exists is refused.

```bash
make release-status  # readiness of both projects, then: which have an undated version heading
make release         # readiness, branch + tree, then stamp + commit + tag + push + release; needs gh (gh auth login)
```

Afterwards, `make build` in `go/` on the tagged commit reports `vX.Y.Z`; a later commit reports
`vX.Y.Z-N-g<sha>` and an uncommitted tree adds `-dirty`, so a metadata file always names the build that
produced it.

## Static analysis

A local SonarQube (Community Edition) can analyse both projects. It is **optional and gates nothing**: the
readiness gates in the workflow above never call it, and no release depends on it. It runs on your machine only
— nothing is sent anywhere.

Each project is its own Sonar project (`azure-rd-go`, `azure-rd-web`), analysed from its own folder with its own
`sonar-project.properties`, because the two have different toolchains, coverage formats and release lines. A
tenant export is **never** analysed: `output/` is excluded, since it is customer configuration and has no place
on a code-quality server.

```bash
make sonarqube-start          # server + database (waits until it reports UP; ~1 min on a cold start)
make sonarqube-analyze        # coverage + scan for both projects (or -go / -web)
make sonarqube-report         # download the findings to ./.sonar-reports (or -go / -web)
make sonarqube-stop           # stop; sonarqube-clean also deletes ./.sonar and the scanner work dirs
```

The server URL comes from `SONAR_HOST_URL` (default `http://localhost:9000`); the first login is `admin`/`admin`
and forces a password change. Two kinds of token are needed, because Community Edition distinguishes them:

| Variable | Token | Used for |
| -------- | ----- | -------- |
| `SONAR_TOKEN` | analysis (`sqa_…`) | `sonarqube-analyze` |
| `SONAR_REPORT_TOKEN` | user (`squ_…`), falls back to `SONAR_TOKEN` | `sonarqube-report` |

An analysis token authenticates but grants *Execute Analysis* only, so it can list issues while project
metadata and security hotspots come back `403`. The report is still written in that case, with the gap stated in
it. Reports land in `.sonar-reports/` (gitignored) as raw JSON plus a Markdown breakdown grouped by rule —
highest count first, which is the view that separates "encode this as a lint rule" from "fix these three
places".

**Because Community Edition cannot export a report, that Markdown breakdown is the deliverable**; there is no
PDF or CSV to download from the UI.

### Parity with the local linters

Where a Sonar rule has an equivalent in a project's own linter, the linter is configured to **use Sonar's
parameters rather than its own defaults**, so a finding is visible in the editor and on the command line instead
of only after a scan. Both `go/.golangci.yml` and `web/eslint.config.mjs` name Sonar as a second source they
mirror, and each project's README documents which rules that covers. Two limits are deliberate and recorded in
those files:

- **A rule pair that cannot be made to agree stays Sonar-only** rather than being approximated. Go's duplicated
  string-literal rule is the example: Sonar counts per file and ignores identifier-like literals, the linter
  counts per package and skips call arguments, and settings that equalise the counts still produce a different
  set of findings.
- **Some rules need type information** that `web/`'s ESLint setup does not currently produce, so they are
  reported by the server only.

Enabling these rules on code that has never been measured makes a backlog of findings visible at once, so **both
projects baseline what predates the rules instead of weakening them**: every rule stays enabled at the server's
parameters, the gates pass, and **new code is held to the full set**. Neither baseline switches a rule off for a
file, which would also have blinded it for code written later; each pins the findings that existed. The server
keeps reporting all of them, so the deferred work stays counted where it is visible. That is the line between the
two kinds of exception these files contain: a **suppression** — a finding that should not be reported anywhere,
because the rule misreads a shape — is mirrored in `sonar-project.properties` so the two tools cannot disagree,
while a **baseline** is deferred work and is deliberately local. Clearing them is a parked idea in each project's
`NEXT-ITERATIONS.md`, paid off as those files are touched for other reasons.

The mechanisms differ because the toolchains do:

- **`web/eslint-suppressions.json`** is ESLint's own bulk-suppressions file: a **count per file and per rule**,
  so one more violation of a baselined rule in a baselined file exceeds the count and reports. `npm run lint:baseline`
  regenerates it and `npm run lint:prune` drops what a refactor has paid off; both rewrite it, so neither is part
  of a gate.
- **`go/.golangci.yml`** has no such mechanism, so the baseline is expressed in the config as exclusion rules
  that each match **one function by name**, listed at the end of `exclusions.rules`. A new complex function in a
  listed file is reported. The trade-offs are stated there: a listed function can still get worse unnoticed
  (the match is on its name, not its score), and a stale entry is invisible, because golangci-lint does not
  report an exclusion that matched nothing — so an entry is deleted in the same commit as the refactor that
  fixes its function.

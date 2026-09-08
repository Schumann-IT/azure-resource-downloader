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

- **Go 1.24+** to build the CLI, **Node.js 20+** to run the browser.
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
├── go/          azure-rd CLI (Go 1.24) — see go/README.md
├── web/         documentation browser (NestJS, TypeScript) — see web/README.md
├── output/      export tree, gitignored: output/<tenant>/{resources,docs}/
├── Makefile     release entry points for both projects, plus branch-ready-web (see Releasing)
├── scripts/     release.sh — tags and publishes prepared releases
└── README.md    this file
```

Per-project rules for editors and AI assistants live in `go/.windsurf/rules/` and `web/.windsurf/rules/`;
they apply only to their own folder.

## Releasing

Each project has its own SemVer line, tagged with a folder prefix so the two never collide and each
project's `git describe` finds only its own tags:

| Project | Tag pattern | Version source |
|---|---|---|
| `go/` | `go/vX.Y.Z` | `git describe --match 'go/v*'` at build time — `make build` stamps it into `--version` and into every `resources/metadata.yaml` (`toolVersion`) |
| `web/` | `web/vX.Y.Z` | `version` in `web/package.json` |

Releasing is three steps, run from the repository root on `main`. A project that did not change is simply
never closed, so it gets no tag and no release.

**1. Close the changelog by hand** for each project that changed: in its `CHANGELOG.md`, rename
`## [Unreleased]` to a bare `## [X.Y.Z]` — **no date**; the publish step stamps it — and start a fresh, empty
`## [Unreleased]` above it. For `web/`, also set `version` in `package.json` and `package-lock.json`
(`npm version X.Y.Z --no-git-tag-version` in `web/`). Delete any struck-out entries from the project's
`NEXT-ITERATIONS.md` — a strikeout marks work that shipped and is waiting to be cleared out — and check the
changelog records them. Commit and merge to `main`.

For `web/` this is also checked *per branch*, before the merge rather than at release time:
`make branch-ready-web` (→ `npm --prefix web run branch-ready`) runs the tests and the build, then reports
whether the branch cleared its struck-out entries, renumbered the rest and wrote its `## [Unreleased]` entry,
and that it left `version` alone. It changes nothing, but it exits non-zero if any check failed, so it can
gate a merge. It refuses to run at all while `web/` has uncommitted changes — a read-only
`git status --porcelain` scoped to that folder, so the verdict describes the commit that will be merged.
Details in the [web README](web/README.md#development-conventions).

**2. Report whether a release can be cut** — each project's `release-ready` goal only reports; it changes
nothing and runs no git command:

```bash
make release-ready-go    # → make -C go release-ready
make release-ready-web   # → npm --prefix web run release-ready
```

Both first run the project's own pipeline (`go/`: `make ci`; `web/`: `npm test` and `npm run build`), then look
at `CHANGELOG.md`. If its newest version heading is not an undated `## [X.Y.Z]` — there is none, or it already
carries a date — nothing has been closed and the goal reports **no release needed** and succeeds. Otherwise it
checks that `NEXT-ITERATIONS.md` has no struck-out entries and that `## [Unreleased]` is empty; `web/`
additionally checks that `package.json` carries that version. Every check is run and reported — passing ones
with ✅, failing ones with ❌ and what to do about it — and the goal exits non-zero only when *all* checks
fail, so it is a report to read, not a gate that stops on the first problem.

**3. Publish** — `make release` (and `make release-status`) run both `release-ready` goals first as make
prerequisites, so a project whose pipeline fails stops the release before anything happens. Then the publisher
verifies the repository state: the current branch must be `main` (override with `RELEASE_BRANCH=<name>`) and
the working tree clean, since it tags and pushes `HEAD`. These two checks live only here — the `release-ready`
goals run no git at all, and the one other git command in the repository's tooling is `branch-ready-web`'s
web/-scoped clean-tree preflight.

Then, for every project whose newest changelog heading is an undated `## [X.Y.Z]`, it stamps today's date onto
that heading (`## [X.Y.Z] - YYYY-MM-DD`) and commits (`release: go vX.Y.Z, web vX.Y.Z`) — the only changelog edit any
tooling makes — then tags that commit `<project>/vX.Y.Z`, pushes the branch and the tags, and creates a GitHub
release titled `<project> vX.Y.Z` whose notes are that changelog section. Projects whose newest heading is
already dated are skipped; an undated heading whose tag already exists is refused.

```bash
make release-status  # readiness of both projects, then: which have an undated version heading
make release         # readiness, branch + tree, then stamp + commit + tag + push + release; needs gh (gh auth login)
```

Afterwards, `make build` in `go/` on the tagged commit reports `vX.Y.Z`; a later commit reports
`vX.Y.Z-N-g<sha>` and an uncommitted tree adds `-dirty`, so a metadata file always names the build that
produced it.

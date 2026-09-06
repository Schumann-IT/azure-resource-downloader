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

To cut a release of either project:

1. In its `CHANGELOG.md`, rename `## [Unreleased]` to `## [X.Y.Z] - YYYY-MM-DD` and start a fresh, empty
   `## [Unreleased]` above it. For `web/`, also bump `version` in `package.json` to match.
2. Commit, then tag with the folder prefix and push the tag:

   ```bash
   git tag -a go/vX.Y.Z -m "go vX.Y.Z"     # or: web/vX.Y.Z
   git push origin go/vX.Y.Z
   ```

3. For `go/`, `make build` on the tagged commit now reports `vX.Y.Z`; a later commit reports
   `vX.Y.Z-N-g<sha>` and an uncommitted tree adds `-dirty`, so a metadata file always names the build that
   produced it.

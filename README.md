# Azure Resource Downloader

Export a Microsoft Azure / Entra tenant's configuration to clean YAML, turn it
into readable AI-generated documentation, and browse it locally.

This is a monorepo with two projects:

- **`go/`** — the `azure-rd` CLI. Downloads tenant resources, writes them as YAML
  under `output/<tenant>/resources/`, and produces the prompts used to generate
  the documentation.
- **`web/`** — a read-only NestJS browser that renders the generated documentation.

The shared export tree lives at **`output/`** in the repo root: `go/` writes it,
`web/` reads it.

## Prerequisites

- Go 1.24+ and Node.js 20+
- The Azure CLI (`az`)
- An Entra app registration (client ID + tenant ID) for the Microsoft Graph
  scopes the tool reads (delegated user auth only — no service principals)

## Workflow

### 1. Sign in

```bash
az login

export AZURE_RD_CLIENT_ID="<app-registration-client-id>"
export AZURE_RD_TENANT_ID="<tenant-id>"
export AZURE_RD_OUTPUT="../output"   # shared export tree at the repo root
```

`az login` establishes the delegated user session. `AZURE_RD_CLIENT_ID` /
`AZURE_RD_TENANT_ID` point the tool at the app registration used to obtain the
Microsoft Graph scopes (device-code sign-in).

### 2. Download the tenant

```bash
cd go
make build
./azure-rd download
```

Writes clean YAML to `output/<tenant>/resources/`, plus a per-resource-type
documentation prompt.

### 3. Generate the documentation

```bash
./azure-rd docs generate-prompt   # writes output/<tenant>/docs/generate.md
```

Hand `docs/generate.md` to an AI coding agent: it writes one Markdown document
per resource under `output/<tenant>/docs/`. Then build the navigation index the
browser reads:

```bash
./azure-rd docs generate-index    # writes output/<tenant>/docs/index.yaml
```

### 4. View the documentation

```bash
cd ../web
npm install
npm run start:dev
```

Open <http://localhost:3000>. The browser reads `../output` by default
(override with `DOCS_ROOT`, port with `PORT`).

### 5. Sign out

```bash
az logout
```

## Releasing

The two projects are **released independently**. There is no repository-wide version: a release names one
project, and the tag prefix says which.

| Project | Tag | Version source of truth | Release notes |
| --- | --- | --- | --- |
| `go/` | `go/vX.Y.Z` | the tag (stamped into the binary via `-ldflags`) | that version's section of [`go/CHANGELOG.md`](go/CHANGELOG.md) |
| `web/` | `web/vX.Y.Z` | the tag, mirrored into `web/package.json` | that version's section of [`web/CHANGELOG.md`](web/CHANGELOG.md) |

Each project's version line moves on its own: `go/` can reach `v0.5.0` while `web/` stays at `v0.1.0`. **If
only one project changed, only that project is tagged** — no empty release and no version bump for the other.
A tag points at a commit of the shared history, so it records *which repository state produced this artifact*,
not which files changed.

Because the tags of both projects live in one repository, `go/Makefile` restricts its version lookup to
`git describe --match 'go/v*'`. Without that filter a `web/` release would be stamped into `azure-rd
--version` and into `toolVersion` in every `resources/metadata.yaml`.

**Compatibility between the two is stated by the artifact, not by the version numbers.** They meet at
`output/<tenant>/docs/index.yaml`, which carries its own `version:` schema field: a `go/` release says which
schema version it *writes*, a `web/` release which versions it *reads*. Comparing `go/` and `web/` version
numbers means nothing.

To cut a release of `<project>` (`go` or `web`):

1. Make sure the working tree is clean and `main` is up to date, and that the project's checks pass
   (`make check` in `go/`, `npm test && npm run build` in `web/`).
2. In that project's `CHANGELOG.md`, rename `## [Unreleased]` to `## [X.Y.Z] - <date>` and add a fresh, empty
   `## [Unreleased]` above it. For `web/`, also set `version` in `package.json`.
3. Commit (`chore(<project>): release vX.Y.Z`), then tag and push:

   ```bash
   git tag -a <project>/vX.Y.Z -m "<project> vX.Y.Z"
   git push origin main --follow-tags
   ```

4. Publish a GitHub release for the tag, pasting that changelog section as the notes. For `go/`, attach
   binaries built with `make build VERSION=vX.Y.Z` (the tag also produces this version automatically).

## More

- CLI reference, flags, config and supported resource types: [`go/README.md`](go/README.md)
- Documentation browser details: [`web/README.md`](web/README.md)

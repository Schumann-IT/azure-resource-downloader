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

## More

- CLI reference, flags, config and supported resource types: [`go/README.md`](go/README.md)
- Documentation browser details: [`web/README.md`](web/README.md)

# Tenant Configuration Explorer & Documentation Portal

A TypeScript monorepo that browses, diffs, and documents the `output/` tree produced by
**azure-resource-downloader**. It is read-only with respect to the source YAML and
`doc-prompt.md` files — the docgen only *adds* `.doc.md`, `index.*`, and manifest files.

```
web/
  packages/core/   @ard/core — shared tree walker, YAML parsing, semantic diff,
                   index builder, manifest hashing, prompt assembly (fully unit-tested)
  api/             @ard/api  — read-only NestJS API over output/ + the docgen CLI command
  app/             @ard/app  — React + Vite + Tailwind SPA that talks to the API
```

The Go tool keeps writing `output/`; nothing here mutates it. The API reads from disk on
demand (no database), matching the spec's file-based design.

## Prerequisites

- Node.js 20+ (uses npm workspaces)

## Install & build

```bash
cd web
npm install
npm run build        # builds core, api, and app
npm test             # runs the @ard/core test suite (walker, diff, index, manifest)
```

## Run the API

```bash
# Point at the azure-resource-downloader output tree (defaults to ../output at the repo root)
export ARD_OUTPUT_DIR=../output
npm run dev:api      # http://localhost:3001/api  (watch mode)
# or: node api/dist/main.js  after `npm run build`
```

### Endpoints

| Method & path | Purpose |
|---|---|
| `GET /api/health` | Liveness + resolved output directory |
| `GET /api/tenants` | List tenant domains |
| `GET /api/tree?tenant=` | Full provider → type → resource tree for a tenant |
| `GET /api/resource?tenant=&provider=&type=&slug=` | Source YAML, parsed value, and `.doc.md` (if generated) |
| `GET /api/doc-prompt?tenant=&provider=&type=` | The resource type's `doc-prompt.md` |
| `GET /api/index` | Top-level index across all tenants |
| `GET /api/diff?left=&right=&ignore=id,lastModifiedDateTime` | Tenant-vs-tenant file diff (added/removed/changed/unchanged) |
| `GET /api/diff/resource?left=&right=&provider=&type=&slug=&ignore=` | Key-by-key semantic diff of one resource |

## Run the frontend

```bash
npm run dev:app      # http://localhost:5173  (proxies /api to :3001)
```

Tenant browser with search, resource detail (Configuration / Documentation tabs),
and a tenant-vs-tenant diff view with an "ignore volatile fields" toggle.

## Generate documentation (docgen)

The docgen walks every `<slug>.yaml`, prepends its sibling `doc-prompt.md`, calls an LLM,
and writes `<slug>.doc.md` next to the source. It is incremental (content-hash manifest
per tenant) and resumable.

```bash
export ARD_OUTPUT_DIR=../output

# Preview only — no LLM calls, no writes:
npm run docgen -- --dry-run

# Generate for real (Anthropic by default):
export ANTHROPIC_API_KEY=sk-ant-...      # never commit this
export ANTHROPIC_MODEL=claude-opus-4-8   # optional
npm run docgen                           # all tenants
npm run docgen -- --tenant cb-gmbh.com   # one tenant
npm run docgen -- --force                # ignore the manifest, regenerate all
npm run docgen -- --provider noop        # placeholder output, no API key needed
```

Flags: `--tenant <domain>`, `--force`, `--dry-run`, `--concurrency <n>` (default 4),
`--provider <anthropic|noop>`. Without an API key the Anthropic provider falls back to the
`noop` provider so dry runs and tests never need a secret. After generation it writes a
per-tenant `index.md` + `index.json` and a top-level `output/index.json`.

### LLM providers

`LLM_PROVIDER` selects the backend (`anthropic` default, `noop` for offline). `openai` and
`azure-openai` slot in behind the same `LlmProvider` interface in
`api/src/docgen/llm/` — add a class and a `case` in `factory.ts`.

## Notes / next steps

- The frontend uses Tailwind with hand-rolled primitives; the spec's shadcn/ui components
  can be layered in via `npx shadcn@latest init` inside `app/` without structural changes.
- Diffing currently compares two tenant folders; a future `snapshots/<tenant>/<timestamp>/`
  layout drops into the same `diffFileSets` logic.
- Secrets stay server-side: the SPA never sees an API key; only the docgen calls the LLM.

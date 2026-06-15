# Prompt: Build a Frontend + Docs Generator over `azure-resource-downloader` Output

# Project: Tenant Configuration Explorer & Documentation Portal

## Goal
Build a web application + a documentation-generation tool on top of the output produced
by `azure-resource-downloader`. The app must:
1. Browse the exported configuration of one or more tenants.
2. Diff any two tenants (or two snapshots of the same tenant), including files that were
   added or removed.
3. Generate human-readable Markdown documentation for each exported resource using the
   per-folder `doc-prompt.md` as the LLM prompt, plus a generated index.
4. Render the generated Markdown documentation in the frontend.

## Input: the `output/` directory (authoritative data source)
The tool writes a deterministic, file-based tree. Do NOT assume any database or API.

```
output/
  <tenant-domain>/                         # e.g. cb-gmbh.com  -> one folder per tenant
    Microsoft.Graph/                       # API group / provider namespace
      <resourceType>/                      # e.g. deviceCompliancePolicies, organization, groups
        doc-prompt.md                      # ONE per resource-type folder: the LLM doc prompt
        <resource-slug>.yaml               # ONE per resource instance (snake_case slug of displayName)
        ...
    Microsoft.Resources/ ...               # other provider namespaces may also exist (ARM)
```

Concrete real example:
```
output/cb-gmbh.com/Microsoft.Graph/deviceCompliancePolicies/
  doc-prompt.md
  gbl_c_prd_d_mac_os_validation.yaml
  gbl_c_prd_d_win_defender_validation.yaml
  ... (one .yaml per policy)
output/cb-gmbh.com/Microsoft.Graph/organization/
  doc-prompt.md
  corporate_benefits_gmbh.yaml
```

### Resource YAML shape
- Plain YAML exported from Microsoft Graph / ARM. Keys preserve original casing and may
  contain dots and `@`, e.g. `'@odata.type'`, `'@odata.context'`, `scheduledActionsForRule@odata.context`.
- Common identity fields: `id`, `displayName`, `createdDateTime`, `lastModifiedDateTime`, `version`.
- Nested structures and arrays are common (`assignments`, `scheduledActionsForRule`, etc.).
- Some values may be masked/redacted (secrets are filtered out by the exporter). Treat any
  redacted value as opaque; never invent values.
- Files are UTF-8, indented with spaces. Parse with a robust YAML library; keys are NOT
  guaranteed to be valid identifiers, so keep them as map keys/strings.

### `doc-prompt.md` shape (do not rewrite it — use it verbatim as the prompt)
Each resource-type folder contains exactly one `doc-prompt.md`. It begins with a markdown
H1 like `# Documentation prompt for Microsoft.Graph/deviceCompliancePolicies`, an HTML
comment, and then the prompt body. It already instructs the LLM to:
- summarize the resource,
- document EVERY setting in a table (Setting / Configured value / What it does /
  Recommended value / Reference),
- expand embedded/encoded payloads (configurationXml, omaSettings, payloadJson, base64),
- flag security-sensitive settings,
- describe assignments.
The prompt is generic to the resource TYPE; the specific resource YAML must be appended to it.

## Deliverable 1 — Documentation generator (CLI/script)
Write a standalone script/tool (language of your choice; prefer TypeScript/Node or Python)
that:
1. Walks `output/<tenant>/<provider>/<resourceType>/`.
2. For each `<resource-slug>.yaml`:
   - Reads the sibling `doc-prompt.md` from the SAME folder.
   - Builds the final LLM input = contents of `doc-prompt.md` + a fenced ```yaml block
     containing the resource YAML.
   - Calls an LLM (make the provider/model + API key configurable via env vars; never
     hard-code secrets) to produce Markdown documentation.
   - Writes the result next to the YAML as `<resource-slug>.doc.md` (keep generated docs
     clearly distinguishable from source; do not overwrite YAML or `doc-prompt.md`).
3. Is idempotent and incremental: skip regeneration when the source YAML's content hash is
   unchanged since the last run (store a small manifest, e.g. `.docgen-manifest.json`, with
   path -> {sourceHash, generatedAt}). Add a `--force` flag to regenerate all.
4. Runs with bounded concurrency, retries on transient API errors, and continues past
   per-resource failures (collect and report a summary at the end).
5. After all individual docs are generated, produces an index:
   - A per-tenant `index.md` (and/or `index.json`) under `output/<tenant>/` listing every
     resource type and every resource, linking to its `.doc.md`, with metadata pulled from
     the YAML (displayName, id, lastModifiedDateTime, counts per type).
   - A top-level `output/index.json` enumerating tenants -> providers -> resource types ->
     resources, so the frontend can build navigation without re-walking the tree at runtime.
6. Has a `--dry-run` mode that lists what it would generate without calling the LLM or
   writing files.

## Deliverable 2 — Frontend web app
Tech: React + TypeScript, Vite, TailwindCSS, shadcn/ui components, lucide-react icons.
Use a Markdown renderer (e.g. react-markdown + remark-gfm) with syntax highlighting, and a
YAML/structured diff library for the diff view. Keep it a static-friendly SPA that reads the
generated `index.json` and the files under `output/` (served as static assets or via a thin
read-only API).

Features:
1. **Tenant browser**
   - Sidebar/tree: tenant -> provider -> resource type -> resource.
   - Search/filter across resource names and types.
   - Resource detail view with two tabs:
     - **Configuration**: pretty-rendered YAML (collapsible tree + raw view toggle).
     - **Documentation**: rendered `<resource-slug>.doc.md` (Markdown). Show a clear
       empty-state with a "not generated yet" hint if the `.doc.md` is missing.
2. **Diff view**
   - Pick a left side and right side: two different tenants, OR the same resource type
     across tenants, OR two snapshots of one tenant (design for a future `snapshots/`
     layout but base v1 on diffing two tenant folders).
   - Show per-resource-type and per-file diffs.
   - Explicitly surface **added** files (present on right, missing on left) and **removed**
     files (present on left, missing on right), plus **changed** and **unchanged**.
   - For changed files, show a structured/semantic YAML diff (key-by-key), not just a raw
     text diff; ignore volatile fields if configurable (e.g. `lastModifiedDateTime`, `id`)
     via a toggle.
   - Summary header with counts: added / removed / changed / unchanged.
3. **Docs index**
   - Render the generated index (`index.md`/`index.json`) as a landing page per tenant with
     links into each resource's documentation.
4. **Rendering of generated Markdown**
   - GitHub-flavored Markdown: tables, code blocks, links open in new tab, sanitized HTML.
   - Sticky table-of-contents from H2/H3 headings.

## Cross-cutting requirements
- Read-only with respect to source YAML and `doc-prompt.md`; the generator only ADDS
  `.doc.md`, `index.*`, and manifest files.
- Handle YAML keys with dots/`@`/special characters safely throughout.
- Handle large trees (hundreds of files per tenant) with lazy loading/virtualization.
- No secrets in client code; LLM calls happen only in the generator (server/CLI side).
- Provide a README with: install, how to run the doc generator (with env vars for the LLM),
  how to run the frontend in dev, and how to build a static bundle.
- Include tests for: the tree walker, the YAML diff (added/removed/changed cases), and the
  index generation.

## Suggested project layout
```
frontend/        # React + Vite app
  src/
    lib/         # tree walker, index loader, yaml-diff
    components/  # TreeNav, ResourceView, YamlView, DiffView, MarkdownView
    pages/       # Tenants, Resource, Diff, DocsIndex
docgen/          # documentation generator CLI
  src/
    walk.ts      # discover tenants/types/resources + doc-prompt.md
    generate.ts  # build prompt, call LLM, write .doc.md
    index.ts     # build per-tenant + top-level index
    manifest.ts  # incremental hashing
```

## Acceptance criteria
- Running the generator against `output/` produces a `.doc.md` next to every `*.yaml`
  (excluding `doc-prompt.md`), plus per-tenant and top-level index files; re-running without
  `--force` regenerates nothing.
- The frontend lists every tenant under `output/`, lets me open any resource and see both
  its YAML and its rendered documentation, and lets me diff two tenants with added/removed
  files clearly marked.

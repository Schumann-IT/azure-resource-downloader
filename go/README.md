# azure-rd — Azure Resource Downloader

`azure-rd` exports the configuration of an **Entra ID / Intune tenant** (plus a few Azure Resource Manager
types) as clean, reproducible YAML, and drives the **incremental, AI-generated documentation** of that export.

It is the CLI half of the [azure-resource-downloader](../README.md) monorepo; the sibling
[`web/`](../web/README.md) project browses what this tool and the documentation agent produce. The two share
nothing but the export tree on disk.

What it does, in one paragraph: `azure-rd download` signs in as *you* (delegated permissions only — never a
service principal), enumerates 53 resource types across Microsoft Graph and ARM, and writes one YAML per
resource under `output/<tenant>/resources/` together with a facts-only `metadata.yaml` and a per-type
documentation specification (`doc-prompt.md`). `azure-rd docs generate-prompt` then compares that metadata
against the documents already under `output/<tenant>/docs/` and writes a single prompt naming exactly what an AI
agent must (re)generate — nothing more. `azure-rd docs generate-index` builds the navigation index the browser
reads. Every run after the first regenerates only what actually changed in the tenant.

## Table of contents

- [How it fits together](#how-it-fits-together)
- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Quick start](#quick-start)
- [Commands](#commands)
  - [`download`](#download)
  - [`list`](#list)
  - [`docs generate-prompt`](#docs-generate-prompt)
  - [`docs generate-index`](#docs-generate-index)
  - [`--debug`](#--debug)
- [Authentication](#authentication)
  - [Create the app registration](#create-the-app-registration)
- [Configuration & precedence](#configuration--precedence)
- [Output layout](#output-layout)
- [Export metadata (`metadata.yaml`)](#export-metadata-metadatayaml)
- [Per-type documentation prompts (`doc-prompt.md`)](#per-type-documentation-prompts-doc-promptmd)
- [Incremental documentation (`docs/generate.md`)](#incremental-documentation-docsgeneratemd)
- [Navigation index (`docs/index.yaml`)](#navigation-index-docsindexyaml)
- [Supported resource types](#supported-resource-types)
- [Transformations](#transformations)
- [Filters](#filters)
- [Concurrency, retries and timeouts](#concurrency-retries-and-timeouts)
- [Dry-run](#dry-run)
- [Logging](#logging)
- [Security notes](#security-notes)
- [Architecture](#architecture)
- [Extending](#extending)
- [Development](#development)
- [Known limitations](#known-limitations)
- [License](#license)

## How it fits together

```
 download                      docs generate-prompt              AI agent (paste generate.md)
 ────────────────────────▶     ──────────────────────────▶       ─────────────────────────────▶
 resources/                    docs/generate.md                  docs/<APIType>/<endpoint>/<name>.md
 ├─ metadata.yaml   facts       work list: missing / stale        docs/summary.md
 ├─ <type>/*.yaml   sources     docs + blocks to re-splice        docs/report-<timestamp>.md
 └─ <type>/doc-prompt.md  spec
                               docs generate-index
                               ──────────────────────────▶       docs/index.yaml  →  web/
```

Three artifacts carry the contract between the stages:

- **The resource YAML** is deterministic: keys sorted, scalar lists sorted, collision-free file names decided
  by resource id. An unchanged resource yields identical bytes and an identical `sourceSha256` across runs —
  that is what lets staleness be a hash comparison.
- **`resources/metadata.yaml` records facts, never decisions**: hashes, display names, `@odata.type`,
  assignment targets, artifact names, presence in the tenant. Anything that follows from a rule you might
  revise (grouping, classification) is computed later, from these facts, so revising a rule never requires
  re-downloading a tenant.
- **Each document's frontmatter records the hashes it was generated from** (`sourceSha256`, `promptSha256`,
  and one hash per spliced block). `generate-prompt` reads them back to decide what is stale. The documents are
  self-describing; no separate state file exists.

`azure-rd` writes exclusively under `resources/`, with exactly two exceptions under `docs/`: `generate.md`
and `index.yaml`, both at the tree root where no document can be. Everything else under `docs/` is the
agent's.

## Prerequisites

- **Go 1.24+** (build only).
- **Azure CLI**, signed in with `az login` as a user who can read the tenant. The CLI session provides the
  default token and the tenant/subscription defaults.
- **An Entra app registration** with delegated Microsoft Graph permissions. Every `Microsoft.Graph/*` type
  requires one — the Azure CLI's first-party app cannot obtain the Intune and policy scopes — so a full export
  always prompts for it. Only the three ARM types work with `az login` alone. See
  [Create the app registration](#create-the-app-registration).
- Read access in the tenant matching the types you export (Intune Read Only Operator / Global Reader covers
  most; the [Supported resource types](#supported-resource-types) table lists the scopes per type).

## Installation

```bash
cd go
make build            # ./azure-rd, version stamped from the go/vX.Y.Z tag
make install          # into $(go env GOPATH)/bin
./azure-rd --version
```

Releases are tagged `go/vX.Y.Z`; `make build` derives the version from the nearest such tag (`vX.Y.Z`,
`vX.Y.Z-N-g<sha>`, `-dirty` for an uncommitted tree, `dev` outside a checkout) and stamps it into `--version`
and into every `resources/metadata.yaml` it writes. `make release-ready` only reports whether a release can be
cut (changelog closed into an undated `## [X.Y.Z]` heading, nothing struck out in `NEXT-ITERATIONS.md`); closing
the changelog is done by hand, and date stamping, tagging and publishing happen from the repository root — see
the [monorepo README](../README.md#development-workflow). Its counterpart for a feature or fix branch is
`make branch-ready` (see [Development](#development)).

## Quick start

```bash
# 1. Sign in and export everything. Graph types prompt once for the app
#    registration's client id (and tenant id); pass them to skip the prompt.
az login
./azure-rd download --output ../output
./azure-rd download --output ../output --client-id <app-id> --tenant-id <tenant-id>

# 2. Decide what to document (offline; --domain is the export folder name)
./azure-rd docs generate-prompt --output ../output --domain contoso.onmicrosoft.com

# 3. Paste output/<tenant>/docs/generate.md into an AI agent session and let it finish.

# 4. Build the navigation index the browser reads
./azure-rd docs generate-index --output ../output --domain contoso.onmicrosoft.com

# Later: has anything gone stale?
./azure-rd docs generate-prompt --output ../output --domain contoso.onmicrosoft.com --dry-run --exit-code
```

Set `AZURE_RD_OUTPUT=../output` once instead of repeating `--output`; the monorepo layout expects the export
tree at the repo root, which is also the browser's default `DOCS_ROOT`.

## Commands

Only four flags are global (`--config`, `--output`, `--dry-run`, `--log-level`). Everything else belongs to a
command and must follow it: `azure-rd download --type X`, not `azure-rd --type X download`. `--help` on any
command lists its flags; this section explains what they do.

### `download`

Exports resources. With no selection flag it exports **every registered type** — a full export.

```bash
azure-rd download                                   # full export
azure-rd download --type Microsoft.Graph/deviceCompliancePolicies \
                  --type Microsoft.Graph/groups     # only these types
azure-rd download --resource-group my-rg            # one ARM resource group (the group itself)
azure-rd download --resource-id /subscriptions/…/storageAccounts/acct   # explicit ids
azure-rd download --dry-run                         # list what would be downloaded
azure-rd download --prune                           # also delete files for resources gone from the tenant
```

| Flag | Meaning |
|---|---|
| `--type` (repeatable) | Restrict to these resource types. Selection precedence: `--resource-id` > `--resource-group` > `--type` > everything. |
| `--resource-id`, `--resource-group` | Explicit ARM selection. A run scoped this way covers no *type*, so it never marks anything absent (see [metadata](#export-metadata-metadatayaml)). |
| `--subscription` | ARM subscription. Default: the CLI session's default subscription. With none at all, Graph types still download and ARM types are skipped with a warning. |
| `--client-id`, `--tenant-id` | Sign in to a dedicated app registration by device code instead of reusing the CLI token. Prompted interactively when a selected type needs it and they are not set. |
| `--workers`, `--timeout` | Concurrency and per-operation timeout; see [Concurrency](#concurrency-retries-and-timeouts). |
| `--no-prompt` | Do not write the per-type `doc-prompt.md` files. Documents of a type can then not be generated until a run without it. |
| `--resolve-secrets` | Decrypt Intune OMA-URI secrets and write them **in plaintext**. Off by default; see [Security notes](#security-notes). |
| `--prune` | After a complete, failure-free run, delete files under `resources/` for resources this run established are gone. The only delete path in the tool; see [prune](#prune). |

**Exit codes:** `0` when no resource *failed* — resources skipped for missing permissions, filtered out, or
types that could not be listed do not fail the run; they are reported in the summary. `1` on any failed
resource, a configuration error (unknown type, unreadable `--config`), or no resources to download.
Completeness is reported separately from the exit code: a run is *complete* when every type in scope listed,
nothing was cancelled and every request produced a result — an incomplete run can still exit `0`, and
`metadata.yaml` records which.

**What a run prints:** per-type counts as listing finishes, progress every 10 %, then a summary with
successful / skipped / filtered / cancelled / failed counts, the types that could not be listed (with the
reason) and the types that listed empty, and finally whether the run is complete.

### `list`

Prints every registered resource type with its API (`Microsoft.Graph` or `Azure Resource Manager`). It is
**offline** — no sign-in, no network — and is the reference for `--type` values.

```bash
azure-rd list
```

### `docs generate-prompt`

Compares `resources/metadata.yaml` against the documents under `docs/` and writes
`output/<tenant>/docs/generate.md`: the incremental documentation prompt. Never fetches a resource, never
writes under `resources/`.

```bash
azure-rd docs generate-prompt --domain contoso.onmicrosoft.com              # offline
azure-rd docs generate-prompt                                               # resolve tenant via az login
azure-rd docs generate-prompt --domain … --dry-run                          # report only
azure-rd docs generate-prompt --domain … --exit-code                        # CI gate
azure-rd docs generate-prompt --domain … --prompt my-template.md --out /tmp/p.md
```

| Flag | Meaning |
|---|---|
| `--domain` | The export folder under `--output` (the tenant's Entra default domain). Skips authentication entirely. Without it the command signs in only to resolve the domain, exactly as `download` does, and refuses if the resolved domain does not match the export's `metadata.yaml`. |
| `--out` | Where to write the prompt (default `<output>/<tenant>/docs/generate.md`). |
| `--prompt` | A template file overriding the built-in one. The template's marked blocks are what the tool fills; the prose around them is yours to edit. |
| `--exit-code` | Exit `3` when any document is missing, stale, needs a block re-spliced or needs marker migration — i.e. when the documentation on disk does not match the export. |

**Exit codes:** `0` clean (or pending work without `--exit-code`); `2` could not answer (export folder not
resolvable, no `metadata.yaml`, tenant mismatch, unreadable `--prompt` template); `3` pending work found with
`--exit-code`.

What the comparison decides, and what the agent then does, is described under
[Incremental documentation](#incremental-documentation-docsgeneratemd).

### `docs generate-index`

Builds `output/<tenant>/docs/index.yaml`, the machine-readable navigation index the browser keys everything
off, from `metadata.yaml` plus each document's frontmatter (its one-line summary and grouping). Run it after
a documentation pass; run before any document exists and every in-scope resource is listed as *pending*.

```bash
azure-rd docs generate-index --domain contoso.onmicrosoft.com
azure-rd docs generate-index --domain … --config azure-rd.yaml     # with a taxonomy: section → facets
azure-rd docs generate-index --domain … --dry-run
```

Same `--domain`/`--out`/auth flags as `generate-prompt` (no `--prompt`, no `--exit-code`); exit `2` when it
cannot answer, including an invalid `taxonomy:` section. Classification along operator-defined axes comes from
the config file's `taxonomy:` section — see [Navigation index](#navigation-index-docsindexyaml).

### `--debug`

`azure-rd --debug` prints a diagnostic report of the session a `download` would use and exits without writing
anything: tool version, authentication method (CLI session or device-code app), config file in use, the
signed-in user (UPN, tenant id, object id), the resolved subscription (or that ARM types would be skipped), the
tenant's default domain and the resulting output directory, and the number of registered type handlers. Run it
when a type is unexpectedly skipped or the export lands in the wrong folder. It accepts
`--client-id`/`--tenant-id`/`--subscription` to inspect that session instead of the CLI one.

To see which app and scopes a CLI token actually carries:

```bash
az account get-access-token --resource https://graph.microsoft.com -o tsv --query accessToken \
  | python3 -c "import sys,base64,json; t=sys.stdin.read().strip().split('.')[1]; t+='='*(-len(t)%4); c=json.loads(base64.urlsafe_b64decode(t)); print(json.dumps({k:c.get(k) for k in ['appid','app_displayname','scp']}, indent=2))"
```

## Authentication

`azure-rd` uses **delegated user authentication only**. It never authenticates as an application, so it can
only ever see what the signed-in user can see, and every request is attributable to that user in the audit log.
Service principals and client secrets are not supported by design.

Two credential paths exist; both yield one delegated token used for ARM and Microsoft Graph alike:

| Path | When | How |
|---|---|---|
| **Azure CLI session** (default) | Only ARM types selected (resource groups, storage accounts, VMs) | Reuses the `az login` token via `azidentity.AzureCLICredential`. No app registration involved. |
| **Device-code sign-in to your app registration** | Any `Microsoft.Graph/*` type selected | `--client-id` + `--tenant-id` (or `AZURE_RD_CLIENT_ID`/`AZURE_RD_TENANT_ID`, or the config file). The tool prints a device code and URL; sign in in a browser as the same user. |

Why the second path is unavoidable for Graph: the delegated Intune and policy scopes
(`DeviceManagementConfiguration.Read.All`, `DeviceManagementApps.Read.All`, `Policy.Read.All`, …) are not
consentable for Microsoft's first-party Azure CLI app, so a CLI token can never carry them. Every Graph handler
in this tool therefore declares that it needs a dedicated app. When you run `download` (or a full export) and
the flags are unset, the tool lists the affected types with the scopes each needs and **prompts for the client
id and tenant id** (the tenant defaults to the CLI session's tenant; press Enter to accept). Refusing the prompt
aborts the run — a partial export that silently skipped every Graph type would be worse than stopping.

Once signed in, the token's scopes decide what is read. A type whose scope the token lacks is **skipped with a
warning**, never a failure: its listing is recorded as "could not be listed", the run is marked incomplete,
and nothing is inferred about its resources. The token decoder under [`--debug`](#--debug) shows which scopes
a CLI token actually carries.

The tenant's **Entra default domain** (e.g. `contoso.onmicrosoft.com`) is resolved through the ARM Tenants API
for the signed-in identity (preferring the configured tenant, then the subscription's tenant) and becomes the
export folder name `output/<domain>/`. If it cannot be resolved, output falls back to the base directory with a
warning.

### Create the app registration

One-off, once per tenant, by an admin who can grant consent. The script creates a public-client app (device
code needs it), adds every delegated scope the supported types use — resolved **by name** from the Microsoft
Graph service principal, so it stays correct as Graph evolves — and grants admin consent. Drop any scope you do
not need; the types that require it are then skipped with a warning.

```bash
GRAPH="00000003-0000-0000-c000-000000000000"
ARM="797f4846-ba00-4fd7-ba43-dac1f8f63013"
ARM_USER_IMP="41094075-9dad-400e-a0bd-54e686782033"      # user_impersonation (delegated)

# Delegated Microsoft Graph scopes covering every supported resource type.
# DeviceManagementConfiguration.ReadWrite.All is only needed for --resolve-secrets;
# all other scopes are read-only.
GRAPH_SCOPES=(
  Policy.Read.All
  DeviceManagementConfiguration.Read.All
  DeviceManagementConfiguration.ReadWrite.All
  DeviceManagementManagedDevices.Read.All
  DeviceManagementScripts.Read.All
  DeviceManagementRBAC.Read.All
  DeviceManagementServiceConfig.Read.All
  DeviceManagementApps.Read.All
  Organization.Read.All
  OrganizationalBranding.Read.All
  OnPremDirectorySynchronization.Read.All
  Group.Read.All
  Agreement.Read.All
)

# Create the app with public-client (device-code) flow enabled
APP_ID=$(az ad app create --display-name "azure-resource-downloader" \
  --is-fallback-public-client true --query appId -o tsv)

# Resolve each Graph scope name to its delegated permission ID and add it
for scope in "${GRAPH_SCOPES[@]}"; do
  SCOPE_ID=$(az ad sp show --id "$GRAPH" \
    --query "oauth2PermissionScopes[?value=='$scope'].id" -o tsv)
  az ad app permission add --id "$APP_ID" --api "$GRAPH" \
    --api-permissions "$SCOPE_ID=Scope"
done

# ARM delegated permission (resourceGroups, storageAccounts, virtualMachines)
az ad app permission add --id "$APP_ID" --api "$ARM" \
  --api-permissions "$ARM_USER_IMP=Scope"

# Service principal, then admin consent (allow ~60 s for replication)
az ad sp create --id "$APP_ID"
az ad app permission admin-consent --id "$APP_ID"

echo "Client ID: $APP_ID"
echo "Tenant ID: $(az account show --query tenantId -o tsv)"
```

Then run with it:

```bash
azure-rd download --client-id "$APP_ID" --tenant-id "<tenant-id>"
```

> **Pass `--client-id` to `azure-rd`, not to `az login`.** Tokens from `az account get-access-token` are always
> minted for the Azure CLI first-party app (`04b07795-8ddb-461a-bbee-02f9e1bf7b46`) — even after
> `az login --client-id <app>` — so the extra Graph scopes never appear in them. Only the tool's own
> `--client-id`/`--tenant-id` (device-code sign-in) produce a token for your app. Also check `$APP_ID` is
> actually set: an empty value silently falls back to the CLI session and every Graph type is skipped.

To add a scope later (a new resource type, or one you dropped), resolve it the same way and re-consent:

```bash
SCOPE_ID=$(az ad sp show --id "$GRAPH" --query "oauth2PermissionScopes[?value=='DeviceManagementRBAC.Read.All'].id" -o tsv)
az ad app permission add --id "$APP_ID" --api "$GRAPH" --api-permissions "$SCOPE_ID=Scope"
az ad app permission admin-consent --id "$APP_ID"
```

These are **delegated** permissions: the token acts as the signed-in user, who still needs the matching
directory / Intune / Azure RBAC roles. If a Graph call fails with "required scopes are missing" on the CLI
path (ARM-only runs), refresh the session with `az logout && az login --scope https://graph.microsoft.com/.default`.

## Configuration & precedence

Every option can come from four places. Highest wins, silently:

```
CLI flag  >  AZURE_RD_* environment variable  >  --config file  >  built-in default
```

- **Environment variables** are the flag name upper-cased with `AZURE_RD_` prefixed and `-` → `_`:
  `AZURE_RD_OUTPUT`, `AZURE_RD_SUBSCRIPTION`, `AZURE_RD_CLIENT_ID`, `AZURE_RD_TENANT_ID`, `AZURE_RD_WORKERS`,
  `AZURE_RD_TIMEOUT`, `AZURE_RD_LOG_LEVEL`, …
- **The config file is read only when `--config <path>` is given.** There is no auto-discovery of
  `~/.azure-rd.yaml`; a mistyped path is a fatal error rather than a silent fallback to defaults.
- [`config.example.yaml`](config.example.yaml) is the **reference schema**: every key with its built-in
  default and a comment describing the alternatives. It carries a guarantee — loading it unmodified behaves
  byte-for-byte like running with no config file, including every hash in `metadata.yaml` — so copy it and
  change only what you need. Options that exist **only** in the config file (no flag): `workers-by-api`,
  `transformers`, `filters`, `taxonomy`.
- [`config-tailored-intune.yaml`](config-tailored-intune.yaml) is a worked, opinionated configuration for an
  Intune-centred tenant: secrets resolved, the cleaning and id-resolution transformers dropped, and a full
  `taxonomy:` (programmes, platform and scope axes) for `docs generate-index`. Use it as a starting point,
  not as a default — it changes the recorded hashes.

```bash
azure-rd download --config ./azure-rd.yaml
AZURE_RD_OUTPUT=../output AZURE_RD_LOG_LEVEL=debug azure-rd download
```

## Output layout

Everything lives under `<output>/<tenant>/` in two sibling trees that mirror each other exactly:

```
output/
└── contoso.onmicrosoft.com/                      the tenant's Entra default domain
    ├── resources/                                written by azure-rd download — and only by it
    │   ├── metadata.yaml                         facts about this export (see below)
    │   ├── Microsoft.Graph/
    │   │   ├── deviceCompliancePolicies/
    │   │   │   ├── doc-prompt.md                 the documentation spec for this type
    │   │   │   ├── gbl_c_prd_d_win_os_validation.yaml
    │   │   │   └── …
    │   │   ├── deviceConfigurations/
    │   │   │   ├── doc-prompt.md
    │   │   │   ├── gbl_dc_prd_d_mac_wifi.yaml
    │   │   │   ├── gbl_dc_prd_d_mac_wifi.mobileconfig      decoded sidecar artifact
    │   │   │   └── …
    │   │   ├── deviceManagementScripts/
    │   │   │   ├── …yaml  +  ….ps1                        decoded script sidecar
    │   │   ├── groups/
    │   │   └── … one directory per type
    │   ├── Microsoft.Resources/resourceGroups/
    │   └── Microsoft.Storage/storageAccounts/
    └── docs/                                     the documentation tree
        ├── generate.md                           written by azure-rd docs generate-prompt (agent input)
        ├── index.yaml                            written by azure-rd docs generate-index (browser input)
        ├── summary.md                            written by the agent: tenant landing page
        ├── report-2026-08-31-200617.md           written by the agent: one per documentation run
        └── Microsoft.Graph/deviceCompliancePolicies/
            └── gbl_c_prd_d_win_os_validation.md  written by the agent, mirrors the YAML path
```

- **Path mirroring is mechanical**: `resources/<APIType>/<endpoint>/<name>.yaml` ↔
  `docs/<APIType>/<endpoint>/<name>.md`. Nothing stores the document path; it is always derived. Documents are
  at least two levels deep, which is why the tool-written files at the `docs/` root can never collide with one.
- **File names** are the resource's display name, sanitised: lower-cased, runs of whitespace and hyphens
  become `_`, anything outside `[a-z0-9_]` is dropped, a leading digit gets a `resource_` prefix, and an empty
  result becomes `unnamed`. When two resources of a type sanitise to the same name, the one with the lowest
  resource id keeps the bare name and the others get a deterministic suffix derived from `sha256(id)` —
  decided by id, never by which finished first, so the layout is stable across runs.
- **Sidecar artifacts** are decoded payloads the `base64-decode` transformer writes next to the YAML
  (`.mobileconfig`, `.ps1`, `.sh`, …). They are recorded in `metadata.yaml` and pruned with their resource.
- The agent also creates a `chunks/` working directory in the tenant folder during a documentation run; it is
  scratch space the prompt asks for, not part of the export contract.

## Export metadata (`metadata.yaml`)

`resources/metadata.yaml` describes **the export directory**, not the tenant. It exists so that later steps —
staleness detection, index generation, prune — can be answered from disk without touching Azure.

```yaml
generatedAt: 2026-08-31T18:04:12Z
tenant: contoso.onmicrosoft.com
toolVersion: v0.9.2
run:
  complete: true                 # every type in scope listed, nothing cancelled, every request produced a result
  incompleteReason: ""
  scope: { types: [], resourceIds: [], resourceGroup: "" }   # empty = full export
  transformConfigSha256: 7f…     # hash of the effective transformer config + resolve-secrets switch
  resolveSecrets: false
  writePrompts: true
  pruned: false
types:
  Microsoft.Graph/deviceCompliancePolicies:
    promptSha256: 95cb…          # sha256 of the assembled doc-prompt.md bytes on disk
    promptFileWritten: true
    hasAssignments: true         # declared by the handler: this type has an assignments concept
    lastCoveredAt: 2026-08-31T18:04:12Z
    lastCoveredBy: full          # full | --type | --resource-id | --resource-group
resources:
  Microsoft.Graph/deviceCompliancePolicies/gbl_c_prd_d_win_os_validation.yaml:
    resourceId: 3f0c…
    displayName: GBL_C_PRD_D_WIN_OS_Validation
    sourceSha256: 5d6b…          # sha256 of the YAML bytes on disk
    odataType: "#microsoft.graph.windows10CompliancePolicy"
    platforms: windows10
    artifacts: []
    presentInTenant: true
    lastSeenAt: 2026-08-31T18:04:12Z
    assignmentTargets: [ … raw assignment targets … ]
    notificationTemplateRefs: [ 9a1e… ]   # compliance policies: templates named by noncompliance actions
  Microsoft.Graph/groups/gbl_d_win_all.yaml:
    groupTypes: [DynamicMembership]       # group-only facts, so a referenced group's kind
    securityEnabled: true                 # can be rendered without reading its YAML
notListed:
  types: []                     # could not be listed this run (permissions) — count unknown
  empty: [Microsoft.Graph/vppTokens]     # listed successfully to zero resources
```

Rules the tool holds itself to, because `--prune` is built on them:

- **Merge, never truncate.** A `--type`-scoped run updates only the types it covered and leaves every other
  entry (and its `lastCoveredAt`) untouched. An entry is never removed while its file exists on disk.
- **"Covered" means the type's listing succeeded**, not that it returned resources. A type that listed to zero
  (`notListed.empty`) is covered — absence there is real. A type that could not be listed (`notListed.types`)
  is not — nothing is known about its resources.
- **Absence is only inferred from a complete run.** A resource not seen by a complete run that covered its
  type is marked `presentInTenant: false` — file and facts retained. An incomplete run marks nothing absent,
  because it cannot tell a deleted resource from one it never reached.
- **Skipped and filtered resources are re-observed, not rewritten**: their entry keeps its facts and gains
  `skipped: true` / `filtered: true`.
- **Facts, not decisions.** A value is recorded only if it is read from the resource or computed from its bytes.
  Grouping and classification belong to `generate-index` and its taxonomy, never here.
- **`--dry-run` neither writes nor updates the file**, but performs the merge in memory so the prune preview is
  exact.

### Prune

`--prune` is the **only** delete path in the codebase. After the merge it deletes the YAML (and sidecar
artifacts) of every entry marked `presentInTenant: false` within a type this run covered, drops the entry, and
sets `run.pruned: true`. It refuses to run — and says why — unless the run is complete and had no failed
resources. It never leaves `resources/`, never deletes `metadata.yaml`, removes a type's `doc-prompt.md` only
when the type emptied out entirely, and **never touches `docs/`**: a pruned resource leaves its document behind
as an orphan, which `generate-prompt` reports and leaves in place. Every deletion is logged, with a total.
`--dry-run --prune` lists exactly what a real run would delete, from the same selection.

## Per-type documentation prompts (`doc-prompt.md`)

Every resource type directory receives a `doc-prompt.md` — the **specification** an AI must follow to
document a resource of that type. It is written on every run unless `--no-prompt` is given, and its hash is
recorded as the type's `promptSha256`, so editing a template invalidates exactly the documents of the affected
types on the next `generate-prompt`.

Each prompt is built from a shared body plus per-type metadata declared by the handler (purpose, subtype
guidance, required permissions, lifecycle notes, Microsoft Learn links, related types, key settings, embedded
payloads to decode). Seven template families cover the type shapes:

| Family | Types | What differs |
|---|---|---|
| default (`internal/models/documentation_prompt.tmpl`) | every settings-bearing Graph policy, profile, app and script type | full layout: metadata table, assignments block, `References`, `Lifecycle and operations`, `Security`, `Settings` with one collapsible `<details data-setting="…">` per setting |
| `referenced` | `assignmentFilters`, `roleScopeTags`, `roleDefinitions`, `notificationMessageTemplates`, `namedLocations`, `authenticationStrengthPolicies`, `termsOfUseAgreements` | objects other policies reference *by id*: explains what referencing them means; templates carry the `Used by` block |
| `group` | `groups` | dynamic membership rule explained clause by clause; `Targeted by` block |
| `record` | `windowsAutopilotDeviceIdentities`, `deviceCategories`, `mobileThreatDefenseConnectors`, `ndesConnectors` | short registry-record layout, no settings payload, no grouping axes |
| `singleton` | `deviceManagement`, `organization`, `onPremisesSynchronization`, `authenticationMethodsPolicy`, `authorizationPolicy` | tenant-wide single objects |
| `credential` | `applePushNotificationCertificate`, `depOnboardingSettings`, `vppTokens` | expiry and renewal obligations; masked token values |
| `arm` (`internal/handlers/arm/arm_prompt.tmpl`) | the three ARM types | ARM-specific metadata and references |

The Graph overrides live in `internal/handlers/graph/*_prompt.tmpl`. A handler picks its family through the
`Template` field of its `models.ResourceDocumentation`; `models.BuildDocumentationPrompt` renders it and appends
the grouping vocabulary marker unless the type opts out (`OmitGroupAxes`).

Contracts every document must honour, because the browser and the incremental engine depend on them:

- **Closed H2 set.** The `##` headings are fixed per family (`References | Lifecycle and operations | Security
  | Settings` for the default family) and recorded in the prompt as `<!-- doc-headings: … -->`. The browser
  styles and deep-links sections by heading, so no other H2 may appear.
- **Frontmatter** with `source`, `sourceSha256`, `promptSha256` and the block hashes (below), plus the
  LLM-authored index signals `summary`, `platformGroup`, `functionGroup` (the allowed grouping values are
  emitted in the prompt as `<!-- doc-groups: … -->`).
- **Marked splice blocks.** Assignments tables sit between `<!-- assignments:start -->`/`end` markers,
  noncompliance-notification references between `<!-- notifications:start -->`/`end`, a group's reverse index
  between `<!-- targeted-by:start -->`/`end`, a template's between `<!-- used-by:start -->`/`end`. They are
  re-rendered mechanically without regenerating the document.
- **Credential redaction.** A credential-shaped value in a free-text field or decoded payload that the service
  did not mask is replaced by `«redacted — secret present in source»`, marked `data-note="security"` and called
  out under `Security`; the literal stays only in the YAML.

## Incremental documentation (`docs/generate.md`)

`docs generate-prompt` turns "document this tenant" into a closed, exact work list. It reads
`metadata.yaml` and every in-scope document's frontmatter once, then splices its findings into the prompt
template and writes `docs/generate.md`.

**Scope.** Every resource of every type is in scope except two bulk directory types:
`Microsoft.Graph/windowsAutopilotDeviceIdentities` (never documented) and `Microsoft.Graph/groups`, which are
documented **only when referenced by an assignment**. A type whose `doc-prompt.md` is missing (a `--no-prompt`
export) is reported and skipped — no document of that type can be produced. Resources marked
`presentInTenant: false` are reported as orphans and never regenerated or deleted.

**What is decided, per resource:**

| List | Condition | Agent action |
|---|---|---|
| **Generate** | no document, unreadable frontmatter, `sourceSha256` moved (resource changed), or `promptSha256` moved (spec changed) | write the document wholesale |
| **Re-splice, forward** | document current, but `assignmentsSha256` no longer matches — a group or filter it targets was renamed, changed kind or (dis)appeared | re-render the assignments block only |
| **Re-splice, notifications** | `notificationsSha256` moved — a notification template the policy names was renamed | re-render the notifications block only |
| **Re-splice, reverse** | a group's `targetedBySha256` moved — the set of resources assigning it changed | re-render the group's `Targeted by` block |
| **Re-splice, used-by** | a template's `usedBySha256` moved — the set of policies referencing it changed | re-render the template's `Used by` block |
| **Migrate** | an assignment-capable document predates the markers | insert the markers so it joins the cycle |

The forward/reverse split is the point: renaming one group changes no policy's YAML, so no policy is
regenerated — only their assignments blocks are re-spliced, from a **reference map** the tool renders
(GUID → name, kind, document path) so every page names the same thing the same way. All hashes are computed by
the tool and carried on the work-list rows; the agent copies them into frontmatter and never computes one.

**What the prompt makes the agent do** (the numbered sections of the template):

0. Ground rules — never invent, masked values are not findings, `resources/` is read-only, never hash.
1. The work list, grouped by type, with reason and hashes per row.
2. Read each type's `doc-prompt.md` in full; frontmatter, heading and marker contracts.
3. Fan generation out to parallel subagents, one type per chunk, sized by expected output.
4. Scripted structural verification (frontmatter, headings, balanced `<details>`, coverage against the list).
5. Deterministic splice of all four block kinds from the reference maps; marker migration.
6. Scripted referential verification (every GUID resolved, links symmetric).
7. Write `docs/summary.md` — the tenant landing page (management summary with severity-ranked findings, at a
   glance, assignment posture, coverage caveats), fed from a `summary-facts` block the tool computes from
   `metadata.yaml` so it describes the tenant even on a run that wrote nothing.
8. Write `docs/report-<UTC timestamp>.md` — the run's plan, what was written and re-spliced, checks passed,
   dangling references, findings.

The default template is embedded in the binary (`internal/docs/generate_prompt_template.md`); `--prompt`
substitutes your own. Only the marked blocks (`export`, `worklist`, `refmap`, `usedbymap`, `resplice`,
`migrate`, `expected`, `summary-facts`) are filled by the tool; the prose is editable.

**Dangling references** — an assignment pointing at a group GUID with no group in the export, a filter or a
notification template likewise — are listed in the result and rendered as such in the documents (never
invented, never silently dropped). They usually mean a deleted object still assigned in the tenant.

## Navigation index (`docs/index.yaml`)

`docs generate-index` writes the single file the browser reads to know a tenant exists and how to navigate it.
It is fully derived (from `metadata.yaml` and document frontmatter) and carries no wall-clock time, so
re-running over an unchanged export produces byte-identical output — delete and regenerate freely.

```yaml
version: 4                     # schema version; consumers accept >= 1 and ignore unknown fields
tenant: contoso.onmicrosoft.com
generatedAt: 2026-08-31T18:04:12Z      # mirrors the export
complete: true
vocabularies: { platform: [...], function: [...] }   # allowed platformGroup / functionGroup values
facets:                        # only with a taxonomy: one axis per entry, values in display order
  - id: programme
    label: Programme
    values: [ { id: cis-hardening, label: CIS hardening, count: 41 }, … ]   # zero counts kept
counts:
  documented: 412
  pending: 3                   # in scope, no document yet
  excluded: { Microsoft.Graph/windowsAutopilotDeviceIdentities: 250, Microsoft.Graph/groups: 380 }
resources:
  - type: Microsoft.Graph/deviceCompliancePolicies
    doc: Microsoft.Graph/deviceCompliancePolicies/gbl_c_prd_d_win_os_validation.md
    displayName: GBL_C_PRD_D_WIN_OS_Validation
    summary: Validates Windows 11 OS version and BitLocker state …     # from the document's frontmatter
    documented: true
    scope: device                # from the _d_ / _u_ naming token
    platformGroup: windows       # LLM-chosen grouping, from frontmatter
    functionGroup: compliance
    odataType: "#microsoft.graph.windows10CompliancePolicy"
    facets: { programme: [compliance, cis-hardening], platform: [windows], scope: [device] }
    assignments: { groups: 3, allDevices: false }   # count-only; omitted for types with no assignments concept
```

Scope is the same as `generate-prompt` (same excluded types, same referenced-groups rule), so the index can
never list a resource the prompt would not document.

**Taxonomy.** A `taxonomy:` section in the config file (config-only; see `config.example.yaml` for the schema
and `config-tailored-intune.yaml` for a complete one) defines **axes** — independent facets such as
*programme* (CIS hardening, Defender, VPN, …), *platform*, *scope* — each a list of values with match rules over
`displayName` (regex), `type` (exact), `odataType` (regex), `platforms` (regex) and `scope`. A resource joins a
value if any rule matches; it may hold several values on one axis. The taxonomy records **rules**, so editing it
and re-running `generate-index` reclassifies everything without a download. Resources matching no value on an
axis are reported as uncategorised per axis, plus a headline count of resources uncategorised on *every* axis.
Value ids appear in browser URLs — never rename or reuse one; labels are free to change.

## Supported resource types

53 types: 3 Azure Resource Manager, 50 Microsoft Graph. `azure-rd list` prints the same list. Graph types use the
**beta** endpoint unless marked *v1.0*; all Graph permissions are **delegated** scopes, so the signed-in user
also needs the matching Intune / Entra role.

**Azure Resource Manager** — `Azure Service Management/user_impersonation` plus your Azure RBAC; skipped when
no subscription is available.

| Type | Notes |
|---|---|
| `Microsoft.Resources/resourceGroups` | Hand-picked property set. |
| `Microsoft.Storage/storageAccounts` | Access keys and connection strings are never written. |
| `Microsoft.Compute/virtualMachines` | `adminPassword` and similar secrets are never written. |

**Entra ID (identity, policies, tenant)**

| Type | Permission | Notes |
|---|---|---|
| `Microsoft.Graph/conditionalAccessPolicies` | `Policy.Read.All` | v1.0. |
| `Microsoft.Graph/authenticationStrengthPolicies` | `Policy.Read.All` | v1.0. Referenced by CA policies. |
| `Microsoft.Graph/namedLocations` | `Policy.Read.All` | Referenced by CA policies. |
| `Microsoft.Graph/authenticationMethodsPolicy` | `Policy.Read.All` | v1.0 tenant singleton. |
| `Microsoft.Graph/authorizationPolicy` | `Policy.Read.All` | v1.0 tenant singleton. |
| `Microsoft.Graph/termsOfUseAgreements` | `Agreement.Read.All` | The ToU PDFs are embedded base64. |
| `Microsoft.Graph/organization` | `Organization.Read.All` | v1.0 tenant information object. |
| `Microsoft.Graph/organizationalBranding` | `OrganizationalBranding.Read.All` (+ `Organization.Read.All`) | Singleton under the organization, per-locale `localizations` expanded; no file when branding is unconfigured. |
| `Microsoft.Graph/onPremisesSynchronization` | `OnPremDirectorySynchronization.Read.All` | v1.0. One file in hybrid tenants, none in cloud-only ones. |
| `Microsoft.Graph/groups` | `Group.Read.All` | v1.0. The **full** directory group list incl. dynamic membership rules — large in big tenants. Documented only when referenced by an assignment. |

**Intune — device configuration and compliance** — `DeviceManagementConfiguration.Read.All`

| Type | Notes |
|---|---|
| `Microsoft.Graph/deviceManagementConfigurationPolicies` | Settings Catalog, full settings tree via `$expand=settings`. Also carries Apple **DDM** policies (`technologies` contains `appleRemoteManagement`). |
| `Microsoft.Graph/deviceConfigurations` | Legacy profiles, polymorphic incl. Custom/OMA-URI. `--resolve-secrets` additionally needs `DeviceManagementConfiguration.ReadWrite.All`. |
| `Microsoft.Graph/deviceCompliancePolicies` | Classic per-platform policies, `$expand=scheduledActionsForRule($expand=scheduledActionConfigurations)`; notification template references are recorded as facts. |
| `Microsoft.Graph/compliancePolicies` | Settings Catalog based (Linux), `$expand=settings,scheduledActionsForRule(…)`, named via `name`. |
| `Microsoft.Graph/groupPolicyConfigurations` | Administrative Templates; `definitionValues?$expand=definition` child collection attached. |
| `Microsoft.Graph/deviceManagementIntents` | Legacy Endpoint Security; `settings` child collection attached. |
| `Microsoft.Graph/reusablePolicySettings` | Reusable settings referenced by id from Endpoint Security / Settings Catalog policies. |
| `Microsoft.Graph/assignmentFilters` | Referenced by assignments; resolved in the documentation. |
| `Microsoft.Graph/windowsFeatureUpdateProfiles`, `windowsQualityUpdateProfiles`, `windowsDriverUpdateProfiles` | Windows Update profiles. |
| `Microsoft.Graph/deviceComplianceScripts` | Windows custom-compliance detection scripts (base64 `detectionScriptContent`). Distinct from Remediations. |
| `Microsoft.Graph/mobileThreatDefenseConnectors` | MTD partner connectors; no display name, named by partner id. |
| `Microsoft.Graph/ndesConnectors` | NDES/SCEP connector state; named by friendly name, falling back to id. |

**Intune — scripts** — `DeviceManagementScripts.Read.All`. Script bodies are base64 (`scriptContent`, or
`detectionScriptContent`/`remediationScriptContent` for Remediations); the `base64-decode` transformer decodes
them inline by default or, in `file` mode, into `.ps1`/`.sh` sidecars named after the resource's `fileName`
(`<name>_detection.ps1` / `<name>_remediation.ps1` for Remediations).

| Type | Notes |
|---|---|
| `Microsoft.Graph/deviceManagementScripts` | Windows platform scripts (PowerShell). |
| `Microsoft.Graph/deviceShellScripts` | macOS shell scripts. Assignments read via `$expand=assignments` (no `/assignments` route in beta). |
| `Microsoft.Graph/deviceCustomAttributeShellScripts` | macOS custom attribute scripts; same assignment caveat. |
| `Microsoft.Graph/deviceHealthScripts` | Remediations (detection + remediation script pair). |

**Intune — apps and app protection** — `DeviceManagementApps.Read.All`

| Type | Notes |
|---|---|
| `Microsoft.Graph/mobileApps` | Highly polymorphic (`win32LobApp`, `winGetApp`, `macOSPkgApp`, `iosStoreApp`, `officeSuiteApp`, …), includes Microsoft built-in apps. |
| `Microsoft.Graph/iosManagedAppProtections`, `androidManagedAppProtections`, `windowsManagedAppProtections` | App protection policies, `$expand=apps`. |
| `Microsoft.Graph/mdmWindowsInformationProtectionPolicies`, `windowsInformationProtectionPolicies` | WIP — deprecated by Microsoft; exported for documentation/backup. |
| `Microsoft.Graph/mobileAppConfigurations` | Managed-device app configuration, platform-polymorphic. |
| `Microsoft.Graph/targetedManagedAppConfigurations` | Managed-app configuration, `$expand=apps`. |
| `Microsoft.Graph/vppTokens` | Apple VPP tokens; the token secret is masked by the service. |
| `Microsoft.Graph/intuneBrandingProfiles` | Company Portal branding. |

**Intune — enrollment, Autopilot and tenant** — `DeviceManagementServiceConfig.Read.All` unless noted

| Type | Notes |
|---|---|
| `Microsoft.Graph/windowsAutopilotDeploymentProfiles` | |
| `Microsoft.Graph/windowsAutopilotDeviceIdentities` | Registered device *data*, potentially large; never documented. Identities without a display name are named by serial number. |
| `Microsoft.Graph/deviceEnrollmentConfigurations` | Polymorphic: limits, restrictions, ESP, Windows Hello for Business, notifications, tenant defaults. |
| `Microsoft.Graph/applePushNotificationCertificate` | Tenant singleton named after the Apple ID; skipped when absent. |
| `Microsoft.Graph/depOnboardingSettings` | Apple ADE/DEP tokens; `enrollmentProfiles` child collection attached. |
| `Microsoft.Graph/appleUserInitiatedEnrollmentProfiles` | |
| `Microsoft.Graph/termsAndConditions` | Intune terms and conditions. |
| `Microsoft.Graph/notificationMessageTemplates` | `$expand=localizedNotificationMessages` (best-effort) so per-locale subject and body are inlined. Referenced by compliance policies. |
| `Microsoft.Graph/deviceManagement` | Intune tenant settings singleton. |
| `Microsoft.Graph/deviceCategories` | `DeviceManagementManagedDevices.Read.All`. |
| `Microsoft.Graph/roleScopeTags` | `DeviceManagementRBAC.Read.All`. Referenced from every scoped object. |
| `Microsoft.Graph/roleDefinitions` | `DeviceManagementRBAC.Read.All`. **Custom** roles only; built-ins are skipped at listing. |

**Assignments.** Every Intune policy, profile, app and script type fetches its `/{id}/assignments` child
collection and inlines it under `assignments`, so the export carries every assignment target (group id, filter
id and mode, or the built-in *all users* / *all devices*). Assignment reads are best-effort: on failure a
warning is logged and the resource is exported without them. Group and filter **names** are not written into
the YAML — the documentation step resolves them from the exported groups and filters (see
[Known limitations](#known-limitations)).

## Transformations

Fetched resources pass through a fixed pipeline before being written. Which transformers run is configured by
the `transformers` list in the config file (config-only; no flag). By default all four run; the list **replaces**
the default wholesale, so `transformers: []` exports raw Azure data. Regardless of the order in the file they
execute in this order:

```
cleaning → id-resolution → base64-decode → name-sanitization
```

| Transformer | What it does | Settings (defaults in `config.example.yaml`) |
|---|---|---|
| `cleaning` | Removes properties and empty values | `clean-empty` (default `true`), `remove-keys`, `remove-keys-by-type` (merged per type), `preserve-keys` (exceptions to the removals), `replace` (collapse an object to one of its fields) |
| `id-resolution` | Adds `<property>_name` beside every ARM resource id, parsed offline from the id itself (no lookup) | — |
| `base64-decode` | Decodes base64 payloads: macOS `payload` (`.mobileconfig` plist), Windows `omaSettings[].omaSettingStringXml`, script bodies | `mode` `inline` (replace in the YAML) or `file` (sidecar next to the YAML); `source-key`, `filename-key`, `extension`, `remove-source` for file mode |
| `name-sanitization` | Turns the display name into the file name (rules under [Output layout](#output-layout)) | — |

Independently of the configured pipeline, output is always made **deterministic**: map keys are emitted sorted
and every list of plain strings is sorted (Graph returns some multi-valued attributes such as `proxyAddresses`
in unstable order). Lists of objects — assignments, settings — keep their order, which can be significant.

The effective transformer configuration plus the resolve-secrets switch are hashed into
`run.transformConfigSha256` in `metadata.yaml`, so a later diff can attribute a mass movement of resource hashes
to a config change rather than to edits in the tenant. Note that spelling a transformer's default settings out
explicitly changes that hash even though the output is identical — add only the keys you override.

## Filters

`filters` (config-only) restricts which resources of a type are written, by matching properties of the **raw**
fetched resource against Go regular expressions:

```yaml
filters:
  Microsoft.Graph/deviceConfigurations:
    displayName: "^GBL_.*"          # only this naming prefix
  Microsoft.Graph/groups:
    displayName: "^IT-.*"
    mailEnabled: "true"             # all properties of a type must match (AND)
```

Types without a filter are unaffected. Filtering happens **after fetch** (the resource is read, then dropped),
so filtered resources cost API calls but are never written; they are reported as *filtered* in the summary,
recorded as `filtered: true` in `metadata.yaml` when a previous export wrote them, and do not affect the exit
code or completeness. An invalid regex is logged and skipped. Property paths are dot-separated and matched
case-insensitively.

## Concurrency, retries and timeouts

`download` is a three-stage pipeline (fetch → transform → write), each stage a worker pool connected by
channels. Type **listing** runs before it, concurrently across types.

- **Workers.** The right count depends on the API, not the type: Microsoft Graph throttles hard, ARM does not.
  Defaults are 5 for Graph and 20 for ARM (`workers-by-api`, config-only). Precedence: `--workers` flag →
  `workers-by-api` → `workers` (config/env, default 5) → API default. When a run covers exactly one API the API
  default wins over a configured `workers`; only the flag overrides it. More Graph workers usually make a run
  *slower* once throttling and backoff kick in.
- **Retries.** Transient failures (HTTP 429, 503, timeouts) are retried up to 5 times with exponential backoff.
  Permission errors (403, missing scopes) are never retried: the resource is skipped with a warning.
- **Timeout.** `--timeout` (default 300 s) applies **per operation** — around each resource fetch including its
  retries — not to the whole run, so a large tenant needs no larger value.
- **Accounting.** Every request produces exactly one result, including on cancellation (`Cancelled`). The
  pipeline fails loudly if the counts disagree, because a silently dropped request would later look like a
  resource deleted from the tenant.

## Dry-run

`--dry-run` is honoured by every command and writes nothing:

- `download --dry-run` **lists** instead of downloading: it performs the per-type listing (so it still signs in
  and talks to Azure), then reports the set of resources a real run would fetch, the types it could not list and
  the types that listed empty, and whether that set is complete. No resource is fetched, transformed or
  written, and `metadata.yaml` is not updated. With `--prune` it lists exactly the files a real run would
  delete, from the same selection.
- `docs generate-prompt --dry-run` runs the full comparison and reports the work list without writing
  `generate.md`; `docs generate-index --dry-run` reports the index counts without writing `index.yaml`.

## Logging

Structured key/value logging (charmbracelet/log). `--log-level` (or `log-level` in the config,
`AZURE_RD_LOG_LEVEL`, or the plain `LOG_LEVEL` env var) selects `debug`, `info` (default), `warn` or `error`.
`debug` shows per-resource fetch/transform/write lines, retry attempts, sanitised names and the full error behind
every summarised warning; `info` shows listing counts, progress every 10 %, pipeline metrics and the summary.

Secrets are never logged: tokens, client secrets and resolved OMA-URI values are redacted.

## Security notes

- **Delegated only.** Every read is performed as the signed-in user and audited as such. There is no way to run
  the tool unattended with an application secret.
- **Secrets in output.** ARM handlers drop `adminPassword`, access keys and connection strings before writing.
  Graph returns most secrets already masked (`****`, `encryptedValueToken`); the tool keeps them masked and the
  documentation prompts instruct the model to treat masked values as expected, not as findings.
- **`--resolve-secrets`** is the one deliberate exception: it resolves masked Intune OMA-URI secrets in
  `Microsoft.Graph/deviceConfigurations` through `getOmaSettingPlainTextValue` and writes them **in plaintext**.
  It is off by default, logs a warning when on, needs `DeviceManagementConfiguration.ReadWrite.All` in the token
  (delegated — the Intune backend rejects app-only tokens for this call), and degrades per setting: a value that
  cannot be resolved stays masked. Treat an export produced with it as sensitive material.
- **Credential-shaped free text.** Descriptions and decoded payloads sometimes contain pasted credentials the
  service does not mask. The prompts require the model to redact such values in the documentation and flag
  them under `Security`; the literal stays only in the YAML.
- **Files** are written `0644`, directories `0755`. The tool deletes only under `--prune`, only under
  `resources/`, and only what a complete run proved gone.

## Architecture

```
cmd/download → handlers.Registry.BuildFetchRequests   (concurrent per-type listing)
             → pipeline.Fetcher → pipeline.Transformer → pipeline.Writer   (worker pools, channels)
             → docs.WriteExportMetadata                (merge into metadata.yaml, optional prune)

cmd/docs generate-prompt → docs.GeneratePrompt   (metadata + document frontmatter → generate.md)
cmd/docs generate-index  → docs.GenerateIndex    (metadata + frontmatter + taxonomy → index.yaml)
```

```
go/
├── main.go                       entry point
├── cmd/
│   ├── root.go                   global flags, --debug, config/env wiring (Viper)
│   ├── download.go               the export command
│   ├── list.go                   offline type listing
│   ├── docs.go                   `docs` parent command
│   └── docs/                     `generate-prompt`, `generate-index` (own package; avoids an import cycle)
├── internal/
│   ├── azure/                    credentials (CLI / device code), client, identity, tenant domain,
│   │                             ARM list pagers, permission-error detection, resource-id parsing
│   ├── cmdutil/                  shared flag groups (auth / selection / pipeline), Viper binding,
│   │                             interactive app-registration prompt
│   ├── docs/                     metadata.yaml (merge, prune), generate-prompt engine and its embedded
│   │                             template, generate-index, taxonomy, assignment / notification indexes
│   ├── handlers/                 Registry, BuildFetchRequests, defaults.go (every handler registered here)
│   │   ├── arm/                  ARM handlers + arm_prompt.tmpl
│   │   └── graph/                GraphCollectionHandler base, one constructor per Graph type,
│   │                             *_prompt.tmpl overrides (credential, group, record, referenced, singleton)
│   ├── logger/                   charmbracelet/log wrapper
│   ├── models/                   ResourceHandler interface, config/result types, API detection,
│   │                             documentation prompt builder + default template, grouping vocabularies, filters
│   ├── pipeline/                 fetcher, transformer, writer (file-name planning, doc-prompt assembly), metrics
│   ├── retry/                    exponential backoff for transient Azure failures
│   └── transform/                cleaner, sanitizer, base64 decoding, scalar-list sorting
├── config.example.yaml           reference schema — every option at its default, documented
├── config-tailored-intune.yaml   worked Intune configuration incl. a full taxonomy
├── Makefile                      build / test / lint targets (the only supported way to run them)
├── .golangci.yml                 the linter set `make lint` and GoLand both run
├── CHANGELOG.md                  Keep a Changelog; released sections match go/vX.Y.Z tags
├── NEXT-ITERATIONS.md            outstanding work and parked ideas
└── .windsurf/rules/              editor / AI-assistant rules for this folder
```

Design points that everything else leans on:

- **Handler registry.** Every type is a `models.ResourceHandler` (`GetType`, `List`, `Fetch`, `Transform`,
  `GetDocumentationPrompt`) registered in `internal/handlers/defaults.go`. Graph types are thin constructors
  around the shared `GraphCollectionHandler` supplying list/fetch/name closures plus their documentation
  metadata; ARM types implement the interface directly. Handlers also declare whether they need a dedicated app
  and whether they have an assignments concept.
- **Pipeline stages share nothing but channels**, and every request yields exactly one result — the invariant
  that keeps `--prune` safe.
- **Facts vs decisions.** `metadata.yaml` and the resource YAML hold facts; classification (taxonomy),
  staleness and grouping are computed from them at `docs` time.
- **Deterministic output** at every layer: sorted keys and scalar lists, id-decided file names, wall-clock-free
  `index.yaml`, prompt hashes taken from the assembled bytes on disk.

## Extending

### Add a resource type

1. Create the handler in `internal/handlers/graph/<type>.go` (or `arm/`). For Graph, return a
   `*GraphCollectionHandler` from `New<Type>Handler(credential)` with `azureType`, the list/fetch closures, and a
   `documentation: models.ResourceDocumentation{...}` literal (purpose, key settings, required permissions,
   lifecycle notes, links, related types; pick a `Template` family if the default layout does not fit; set
   `hasAssignments: true` if the type has assignments and fetch them in `fetchItem`). Established `fetchItem`
   patterns: `$expand` query parameters, child-collection fetches attached before serialisation, post-fetch
   enrichment, singletons (probe in `listIDs`, ignore the id in `fetchItem`).
2. Register the constructor in `internal/handlers/defaults.go`.
3. Add unit tests beside it.
4. Add the type to the [Supported resource types](#supported-resource-types) table, with its permission — and
   the scope to the app-registration script if it is a new one.
5. Add a `CHANGELOG.md` entry under `## [Unreleased]`.

Type naming follows `Microsoft.Graph/<endpointName>` (the Graph collection segment, camelCase) or the ARM
provider type; `models.DetectAPIType` derives the API from the prefix.

### Add a transformation

Add the function under `internal/transform/`, call it from `internal/pipeline/transformer.go`, document the
behaviour, test it, and update `config.example.yaml` if it is configurable.

### Add a config option

Add the field to `models.PipelineConfig`; declare the flag on the command (or in the matching `cmdutil` group
when several commands share it) — local flags are bound to Viper automatically; give it a default constant; use
it; document it in `config.example.yaml` **at its default value** (loading that file unmodified must remain a
no-op, including every hash) and in this README; add a changelog entry.

### Change a documentation template

Editing `internal/models/documentation_prompt.tmpl` or any `*_prompt.tmpl` moves `promptSha256` for every type
that uses it, which makes every document of those types stale on the next `generate-prompt` — a full
regeneration for the default template. Batch such changes; `NEXT-ITERATIONS.md` tracks the ones waiting for a
regeneration to ride on.

## Development

Always use the Makefile targets, never the raw `go` commands:

```bash
make build           # binary with version stamp
make test            # go test ./...
make test-race       # with the race detector — required for any change touching goroutines, channels,
                     # sync primitives, the pipeline or the concurrent listing
make lint            # golangci-lint --fix: applies the fixes it can, rewrites files
make lint-check      # golangci-lint without --fix: reports only
make fmt             # rewrites files
make fmt-check       # reports unformatted files, rewrites nothing
make check           # fmt-check + lint-check + test — modifies nothing, so it can report on a commit as-is
make ci              # check + build (the default goal)
make deps            # download + tidy
make test-coverage   # coverage.html
make release-ready   # report whether a release can be cut (changes nothing); tag + publish via ../Makefile
make branch-ready    # gate: clean tree, ci, then is this feature/fix branch ready to ship? (changes nothing)
```

### Linting and the editor

`.golangci.yml` is the single lint truth: `make lint-check` and `make lint` run golangci-lint with it (both
verify the file against the v2 schema first, so a typo fails loudly instead of silently reverting to the
defaults), and GoLand runs the same binary with the same file — Settings | **Go | Linters** → *Use config* →
`.golangci.yml`, a one-time per-developer setting because `.idea/` is not committed. Note the path is resolved
from the working directory: pointing at it explicitly (or opening this folder rather than the repository root)
is required, since golangci-lint finds no config when started from the monorepo root. The file also declares
`gofmt` as the only formatter, so GoLand's golangci-lint-based format-on-save produces exactly what `make fmt`
does and cannot leave a diff that `make fmt-check` rejects.

The file enables golangci-lint's default set — `errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused` —
plus `unconvert`, `unparam` and govet's `nilness` pass, each mirroring a GoLand inspection that is on by
default, and each annotated in the file with the inspection it mirrors. Choices that would be stricter than
the IDE are deliberately left out: govet's `shadow` pass is off (it is not in vet's default suite, upstream has
proposed deprecating it, and here it reports only idiomatic `if err := f(); err != nil` blocks), and
`unparam`'s "parameter always receives the same argument" reports are excluded for `_test.go`, where they are
the normal shape of a test helper. What only GoLand's own bundled inspections report stays an editor hint, not
a merge gate.

Conventions that CI and review expect:

- Every exported symbol has a doc comment; `context.Context` is the first parameter of anything that does I/O;
  errors are returned, not logged and returned.
- Lint findings are fixed in the code, or silenced at the single site with a `//nolint:<linter>` carrying the
  reason. Dropping a linter from `.golangci.yml` to go green would also change what the editor reports.
- Tests use no network and no real export: fixtures are built in temp directories.
- `CHANGELOG.md` is updated in the same change for anything a user can notice, under `## [Unreleased]`; released
  sections are `## [X.Y.Z] - YYYY-MM-DD` matching a `go/vX.Y.Z` tag (procedure in the
  [monorepo README](../README.md#development-workflow)).
- `README.md` is the single source of truth for what the tool does today; `NEXT-ITERATIONS.md` holds
  outstanding work and parked ideas; no other documentation Markdown lives in this folder (the embedded
  `generate_prompt_template.md` is program input, not documentation).
- Delivered work is **struck through** in `NEXT-ITERATIONS.md` rather than deleted, so a branch can be
  reviewed against what its entries set out to do. Clearing them out and renumbering the rest is part of
  closing the branch, which is what `make branch-ready` gates; `make release-ready` repeats the strikeout check
  at release time as a backstop.
- `make branch-ready` (or `make branch-ready-go` from the repository root) reports whether a feature or fix
  branch is ready to ship: `make ci` passes, the struck-out `NEXT-ITERATIONS.md` entries have been cleared out
  and the rest renumbered contiguously, and `## [Unreleased]` records the work. It edits nothing, and unlike
  `release-ready` it reports every check and **exits non-zero if any of them failed**, so it can gate a merge.
  An empty `[Unreleased]` is reported, not failed: a branch with no user-visible effect legitimately has none.
  There is no version check — this project's version is the `go/vX.Y.Z` tag, not a file.
- It **refuses to run while `go/` has uncommitted changes**, before `make ci`, so the verdict describes the
  commit that will be merged rather than the editor's current state. That preflight is a read-only
  `git status --porcelain` scoped to `go/`, so an unrelated edit in `../web` cannot block it; outside a clone it
  says so and waves the run through. `release-ready` runs no git at all, and `ci` runs only the read-only
  `fmt-check`/`lint-check`, so nothing between the preflight and the verdict can change the tree.

Editor and AI-assistant rules live in `.windsurf/rules/` and apply to this folder only:

| File | Covers |
|---|---|
| `01-project.md` | Purpose, layout, architecture patterns, non-negotiables (handler contract, pipeline accounting, metadata) |
| `02-style-and-quality.md` | Go style, error handling, testing, changelog policy |
| `03-commands.md` | Makefile usage, recipes for new handlers / commands / transformations / config options |
| `04-security-and-ops.md` | Credentials, secrets, configuration precedence, output layout, metadata and prune rules |
| `05-azure-conventions.md` | Resource type naming, API versions, display names, Graph SDK usage |
| `06-next-iterations.md` | How `NEXT-ITERATIONS.md` entries and parked ideas are managed |

## Known limitations

- **Assignment targets in the YAML carry GUIDs only.** Group, filter and notification-template names are
  resolved in the *documentation* (from the exported groups/filters/templates), not in the resource files. A
  transformer that resolved them in the YAML is deliberately parked — see `NEXT-ITERATIONS.md` for why.
- **Graceful interrupt is not implemented**: Ctrl+C stops the process without cleaning up partial writes.
  Re-running is safe (writes are idempotent and `metadata.yaml` merges), but a resource file may be truncated
  until the next run rewrites it.
- **Writes are not atomic** (no temp-file-and-rename), so do not point another consumer at `resources/` while a
  download is running.
- **App-only authentication is out of scope** by design, so the tool cannot run unattended.

## License

[Add your license here]

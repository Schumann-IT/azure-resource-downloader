# azure-rd — tenant configuration export & documentation

`azure-rd` exports the configuration of an **Entra ID / Intune tenant** (plus a few Azure Resource Manager
types) as clean, reproducible YAML, and drives the **incremental, AI-generated documentation** of that export.

It is the CLI half of the [azure-resource-downloader](../README.md) monorepo; the sibling
[`web/`](../web/README.md) project browses what this tool and the documentation agent produce. The two share
nothing but the export tree on disk.

What it does, in one paragraph: `azure-rd resource download` signs in as *you* (delegated permissions only — never a
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
  - [`resource download`](#resource-download)
  - [`resource drift`](#resource-drift)
  - [`resource types`](#resource-types)
  - [`resource list`](#resource-list)
  - [`docs generate-prompt`](#docs-generate-prompt)
  - [`docs generate-index`](#docs-generate-index)
  - [`docs analyze-drift`](#docs-analyze-drift)
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
#    registration's client id (and tenant id); it prints the profile snippet to
#    save so the next run does not ask again.
az login
./azure-rd resource download --output ../output

# 2. Decide what to document (offline; --domain is the export folder name)
./azure-rd docs generate-prompt --output ../output --domain contoso.onmicrosoft.com

# 3. Paste output/<tenant>/docs/generate.md into an AI agent session and let it finish.

# 4. Build the navigation index the browser reads
./azure-rd docs generate-index --output ../output --domain contoso.onmicrosoft.com

# Later: has anything gone stale?
./azure-rd docs generate-prompt --output ../output --domain contoso.onmicrosoft.com --dry-run --exit-code
```

Put `output: ../output` in a base config file instead of repeating `--output`; the monorepo layout expects the
export tree at the repo root, which is also the browser's default `DOCS_ROOT`. With several tenants, keep one
profile per tenant and select it by domain — see [Configuration](#configuration--precedence).

## Commands

Commands are grouped by the noun they act on: `resource …` for a tenant's Azure resources, `docs …` for the
generated documentation of an export.

**Almost everything is configuration, not a flag** — see [Configuration](#configuration--precedence). The
command line carries only what belongs there: `--config` and `--config-dir` (where the configuration is),
`--domain` (which tenant), and the switches that change one invocation without changing what is produced
(`--dry-run`, `--log-level`, `--output`/`--out`, `--type`, `--resource-id`, `--resource-group`, `--prompt`,
`--exit-code`).

The global flags are `--config`, `--config-dir`, `--output`, `--dry-run` and `--log-level`. Everything else
belongs to a command or its group and must follow it: `azure-rd resource download --type X`, not
`azure-rd --type X resource download`. The selection flags (`--type`, `--resource-id`, `--resource-group`) and
`--domain` are declared once on the `resource` group, so every subcommand under it accepts them. `--help` on
any command lists its flags; this section explains what they do.

### `resource download`

Exports resources. With no selection flag it exports **every registered type** — a full export.

```bash
azure-rd resource download                                   # full export
azure-rd resource download --type Microsoft.Graph/deviceCompliancePolicies \
                           --type Microsoft.Graph/groups     # only these types
azure-rd resource download --resource-group my-rg            # one ARM resource group (the group itself)
azure-rd resource download --resource-id /subscriptions/…/storageAccounts/acct   # explicit ids
azure-rd resource download --dry-run                         # list what would be downloaded
azure-rd resource download --config-dir ~/.azure-rd --domain contoso.onmicrosoft.com   # a named tenant
```

| Flag | Meaning |
|---|---|
| `--type` (repeatable) | Restrict to these resource types for this run, replacing the configured `type` list. Selection precedence: `--resource-id` > `--resource-group` > `--type` > everything. |
| `--resource-id`, `--resource-group` | Explicit ARM selection. A run scoped this way covers no *type*, so it never marks anything absent (see [metadata](#export-metadata-metadatayaml)). Command-line only: a forgotten entry in a file would silently scope every later run. |
| `--domain` | Assert which tenant to act on, and select its configuration profile. Refused when it differs from the signed-in tenant, so a typo cannot write one tenant's export under another's name. |
| `--output` | Redirect this run's export root, e.g. to download elsewhere and compare. |

Everything else a download needs is configuration: `subscription`, `client-id` and `tenant-id` in the tenant's
profile; `workers`, `timeout`, `resolve-secrets`, `no-prompt` and `prune` in the base file. `resolve-secrets`
writes Intune OMA-URI secrets **in plaintext** (see [Security notes](#security-notes)) and `prune` is the tool's
only delete path (see [prune](#prune)) — both are settings rather than flags so the decision is recorded in a
file you can review, not typed ad-hoc.

**Exit codes:** `0` when no resource *failed* — resources skipped for missing permissions, filtered out, or
types that could not be listed do not fail the run; they are reported in the summary. `1` on any failed
resource, a configuration error (unknown type, unreadable `--config`), or no resources to download.
Completeness is reported separately from the exit code: a run is *complete* when every type in scope listed,
nothing was cancelled and every request produced a result — an incomplete run can still exit `0`, and
`metadata.yaml` records which.

**Interrupts:** Ctrl+C cancels the run cleanly — listing and fetching stop, in-flight requests are drained and
reported as cancelled in the summary, and the run is recorded as incomplete (so it can never mark a resource
absent or feed a prune). A second Ctrl+C force-quits.

**What a run prints:** per-type counts as listing finishes, progress every 10 %, then a summary with
successful / skipped / filtered / cancelled / failed counts, the types that could not be listed (with the
reason) and the types that listed empty, and finally whether the run is complete.

### `resource drift`

Answers **"has this tenant changed since the last download?"** — without re-baselining anything. It runs the
download's own listing, fetch and transform stages, then **compares instead of writing**: each resource's
freshly marshalled bytes are hashed exactly as a download would hash them and matched against the export's
recorded `sourceSha256`, by resource id first, so a renamed resource is reported as a *rename* rather than an
add plus a remove.

```bash
azure-rd resource drift                                   # compare the whole tenant against its export
azure-rd resource drift --type Microsoft.Graph/groups    # one type only
azure-rd resource drift --dry-run                         # report in full, write nothing
azure-rd resource drift --exit-code                       # exit 3 when drift was found, for CI
```

| Flag | Meaning |
|---|---|
| `--domain` | Assert which export folder to compare against. Refused when it differs from the signed-in tenant — drift against another tenant's export is all noise. |
| `--exit-code` | Exit `3` when drift was found (default: drift is a report, not a failure). |

The settings that must match how the baseline was written — `transformers`, `filters`, `resolve-secrets` — are
configuration precisely so they cannot be typed differently here than they were for the download; the
comparability preflight refuses when the recorded hashes disagree.

**The export is the baseline and stays untouched.** Drift writes nothing under `resources/` or `docs/` and
never prunes; its only output is the `<tenant>/drift/` tree (see [Output layout](#output-layout)): an
observation record `drift/metadata.yaml` plus the fetched bytes of every *added*, *changed* and *renamed*
resource at paths mirroring `resources/` exactly, so a payload is byte-comparable with the baseline file it
shadows. The tree is cleared and rebuilt on every run, so it always holds exactly the latest observation —
and a re-baselining `resource download` clears it too, because a new baseline supersedes the observation by
definition. Re-baselining is a normal `resource download`. To have an LLM judge what the findings mean,
follow up with [`docs analyze-drift`](#docs-analyze-drift) before re-baselining.

**Comparability is a precondition.** The command refuses (exit `2`) when there is no baseline, when the
export belongs to a different tenant, or when the export's recorded transform or filter configuration differs
from this run's — comparing across a different configuration would report every resource as drifted.
Individual entries written under an older configuration (or by a tool version predating the per-entry
attestation) are reported as **unattested** and excluded from the verdict counts, never counted as drift.

**Removals follow the prune rule.** A resource is only asserted *removed* when the run is complete and its
type was actually covered; an incomplete run suppresses removals and says so. Types that could not be listed
are reported as *unknown* and excluded from the totals. Verdict counts, per-finding lines and — for changed
resources — dotted-path `old → new` field deltas are printed (full delta values at `--log-level debug`), plus
how many documents the observed drift will make stale once re-baselined.

**Exit codes:** `0` on success whether or not drift was found; `2` when the question cannot be answered (no
baseline, wrong tenant, incomparable configuration); `1` only when resources failed to fetch; `3` with
`--exit-code` when drift was found.

### `resource types`

Shows what this build can handle: every registered resource type, the handler that implements it and the API
it speaks, grouped by API surface. The map itself is a property of the binary — it needs **no subscription, no
sign-in and no network** — and is the reference for `--type` values.

When a usable session happens to be available, the map is enriched with how many resources of each type the
tenant holds, counted by the same listing `resource list` and `resource download` perform. Whether the counts
appear is decided by the session, never by a flag — and the session is probed **without ever prompting**: a
sign-in that would require interaction (device-code, browser) counts as “no session”, and the command prints
the offline map with a note saying so. The output always states which of the two it printed. A type whose
listing was refused (missing permissions, no subscription) is counted as **unknown — never 0**, which would
mean “listed and found nothing”; unknown or omitted counts never make the command fail.

```bash
azure-rd resource types                                   # the full map (works offline)
azure-rd resource types --type Microsoft.Graph/groups    # one type, with its count when signed in
```

Without a session (offline map only):

```
INFO Supported Azure resource types count=53
INFO Tenant counts omitted (no usable session without prompting); showing the offline type map only reason=…
INFO Azure.ResourceManager (3 types)
INFO Microsoft.Resources/resourceGroups handler=arm.ResourceGroupHandler
…
INFO Microsoft.Graph (50 types)
INFO Microsoft.Graph/groups handler=graph.GraphCollectionHandler
…
```

With a session, the same lines gain a count column:

```
INFO Tenant counts included from a live listing types_unknown=1
…
INFO Microsoft.Graph/groups handler=graph.GraphCollectionHandler count=42
INFO Microsoft.Graph/deviceCompliancePolicies handler=graph.GraphCollectionHandler count=unknown reason=missing permissions …
```

### `resource list`

Lists what the tenant actually contains, per resource, **without downloading anything**. With no selection it
covers every registered type, exactly as a full download would; the enumeration is the same listing a download
uses to build its fetch requests, so the two can never disagree about what is in scope — only the framing
differs (“what is there” versus “what would be written”).

Listing yields resource ids. When an export for the tenant already exists under `--output`, the display names
recorded in its `resources/metadata.yaml` are joined in, and resources the export does not know yet are marked
`new=true` — nothing is ever fetched merely to prettify the listing. A type that could not be listed is
reported as **unknown**, never as empty, and does not fail the command. The command writes nothing, so
`--dry-run` changes nothing.

This command's question is online-only, so unlike `resource types` it requires a session and fails up front
without one.

```bash
azure-rd resource list                                    # everything the tenant contains
azure-rd resource list --type Microsoft.Graph/groups     # one type only
azure-rd resource list --config-dir ~/.azure-rd --domain contoso.onmicrosoft.com   # a named tenant
```

```
INFO Microsoft.Graph/groups count=3
INFO  id=07f9a17e-… name="All Staff"
INFO  id=2f1b4c9d-… name="Intune Admins"
INFO  id=9c3e51ab-… new=true
WARN Type could not be listed; its contents are unknown (not zero) type=Microsoft.Graph/deviceCompliancePolicies reason=…
INFO Tenant listing finished resources=3 types_unknown=1 types_empty=12
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

### `docs analyze-drift`

Renders `output/<tenant>/drift/analyze.md`: a prompt directing an LLM to analyze the **impact** of the latest
[`resource drift`](#resource-drift) observation — what each finding means for security posture, compliance,
lifecycle and who is affected — and to write **one drift document per in-scope finding** plus a summary index. Each
drift document lands at the payload's path with the extension swapped (`drift/<APIType>/<endpoint>/<name>.md`
beside `…/<name>.yaml`), so per resource the baseline YAML, the observed YAML, the documentation and the
judgment all share one key — a frontend can show the YAML diff and the drift document side by side without any
lookup. The index, `drift/index.md` at the tree root, carries the summary: findings ordered by severity with
links, and the cross-resource security / compliance / lifecycle view. Fully offline: it
reads the observation (`drift/metadata.yaml`) and the baseline (`resources/metadata.yaml`), and verifies every
payload against its recorded hash before directing an agent at it. It never fetches a resource and never
writes under `resources/` or `docs/`.

```bash
azure-rd docs analyze-drift --domain contoso.onmicrosoft.com     # offline
azure-rd docs analyze-drift                                      # resolve tenant via az login
azure-rd docs analyze-drift --domain … --dry-run                 # report only, write nothing
azure-rd docs analyze-drift --domain … --prompt my-template.md   # override the built-in template
```

Same `--domain`/`--out`/auth flags as `generate-prompt`, plus `--prompt` for a template override. There is no
`--exit-code`: `resource drift` already gates CI on drift being found.

**Prompt, drift documents and index live with the observation.** All of them sit inside the `drift/` tree and
are deleted with it — by the next `resource drift` run, and by a re-baselining `resource download`. There is
deliberately **no drift history**; archive them manually if they must be kept. The analysis agent writes
exactly the per-finding drift documents named by the worklist plus `drift/index.md` — nothing else — and never
touches `resources/` or `docs/`: updating the documentation remains the re-baseline → `docs generate-prompt`
loop.

**Scope.** The analysis applies the same documentation scope as `generate-prompt`:
`Microsoft.Graph/windowsAutopilotDeviceIdentities` findings are never analyzed, and group findings only when
the group is referenced by an assignment — in the baseline **or in an observed payload**, so a group a
drifted policy newly assigns is analyzed too. Out-of-scope **added/removed** findings are not lost: the
tool feeds them to the agent as **inventory rows**, listed in `drift/index.md`'s findings table with a fixed
`info` severity, no analysis and no drift document. Out-of-scope **changed/renamed** findings are only
counted, under the index's "Not analyzed" caveats. The observation itself (`drift/metadata.yaml`) always
records every finding — scope is applied when the prompt is rendered, never by `resource drift`.

There are no per-type drift templates. The prompt instructs the agent to read each finding type's
`doc-prompt.md` as the type-specific lens for judging impact, and flags types whose spec is missing (an
export run with `no-prompt` set) so reduced confidence is stated rather than hidden. The finished prompt —
like `docs/generate.md` — no longer carries the template's explanatory header comment or the ` (template)`
H1 suffix; a `--prompt` override template must now also carry the `inventory` marked block.

**Exit codes:** `0` on success — including a clean observation (nothing to analyze, no prompt written); `2`
when the question cannot be answered: no observation (run `resource drift` first), the export was
re-baselined after the observation (run `resource drift` again), tenant mismatch, a payload not matching the
observation, or an unreadable `--prompt` template.

### `--debug`

`azure-rd --debug` prints a diagnostic report of the session a `download` would use and exits without writing
anything: tool version, authentication method (CLI session or device-code app), config file in use, the
signed-in user (UPN, tenant id, object id), the resolved subscription (or that ARM types would be skipped), the
tenant's default domain and the resulting output directory, and the number of registered type handlers. Run it
when a type is unexpectedly skipped or the export lands in the wrong folder. It also reports which
configuration this invocation resolved — config directory, base file, profile, domain — so a tenant switch can
be confirmed without running anything that writes; pass `--config-dir` and `--domain` to inspect that tenant's
profile and the session it names.

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
| **Device-code sign-in to your app registration** | Any `Microsoft.Graph/*` type selected | `client-id` + `tenant-id` in the tenant's configuration profile. The tool prints a device code and URL; sign in in a browser as the same user. When a selected type needs an app and the profile has none, the run asks interactively and prints the snippet to save. |

Why the second path is unavoidable for Graph: the delegated Intune and policy scopes
(`DeviceManagementConfiguration.Read.All`, `DeviceManagementApps.Read.All`, `Policy.Read.All`, …) are not
consentable for Microsoft's first-party Azure CLI app, so a CLI token can never carry them. Every Graph handler
in this tool therefore declares that it needs a dedicated app. When you run `download` (or a full export) and
the flags are unset, the tool lists the affected types with the scopes each needs and **prompts for the client
id and tenant id** (the tenant defaults to the CLI session's tenant; press Enter to accept). Refusing the prompt
aborts the run — a partial export that silently skipped every Graph type would be worse than stopping.

Unless the profile sets `client-id`, `resource download` **verifies the CLI session before anything else — including
the dedicated-app prompt** — and fails immediately with `run 'az login' first` when there is none. Without that
check, a missing session would only compound into warnings — subscription resolution degrades deliberately for
permission-poor identities, and each type's listing is skipped individually — ending in "No resources to
download" instead of naming the cause, after prompting for an app registration the operator may not have been
asked about otherwise. A profile that names a `client-id` skips the check: the device-code sign-in needs no CLI
session, because its first token request *is* the sign-in.

Once signed in, the token's scopes decide what is read. A type whose scope the token lacks is **skipped with a
warning**, never a failure: its listing is recorded as "could not be listed", the run is marked incomplete,
and nothing is inferred about its resources. The token decoder under [`--debug`](#--debug) shows which scopes
a CLI token actually carries.

The tenant's **Entra default domain** (e.g. `contoso.onmicrosoft.com`) is resolved through the ARM Tenants API
for the signed-in identity (preferring the configured tenant, then the subscription's tenant) and becomes the
export folder name `output/<domain>/`. When `--domain` is also given, the two must agree — a mismatch aborts
before anything is written, so one tenant's export can never land under another's name. When neither can be
established the run **refuses**: there is deliberately no fallback to the bare output directory, because an
export written to `output/resources/` does not match the layout every other command looks for and would be
invisible from the moment it was written.

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
# DeviceManagementConfiguration.ReadWrite.All is only needed for resolve-secrets;
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

Then record it in the tenant's profile (`<config-dir>/<domain>.yaml`) and run:

```yaml
client-id: "<the APP_ID printed above>"
tenant-id: "<tenant-id>"
```

```bash
azure-rd resource download --config-dir ~/.azure-rd --domain contoso.onmicrosoft.com
```

> **Give the app to `azure-rd`, not to `az login`.** Tokens from `az account get-access-token` are always
> minted for the Azure CLI first-party app (`04b07795-8ddb-461a-bbee-02f9e1bf7b46`) — even after
> `az login --client-id <app>` — so the extra Graph scopes never appear in them. Only the tool's own
> device-code sign-in, driven by the profile's `client-id`/`tenant-id`, produces a token for your app. Also
> check the value is actually set: an empty one silently falls back to the CLI session and every Graph type is
> skipped.

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

**The configuration file is the single source of truth.** Every setting has exactly one home, and the command
line carries only what genuinely belongs there:

```
CLI flag  >  tenant profile  >  base config file  >  built-in default
```

There is **no environment layer**: `AZURE_RD_*` variables are not read at all. They used to outrank the config
file, so a variable exported for one tenant silently applied to the next — and a wrong value does not fail
loudly, it produces a confident, wrong result. (`LOG_LEVEL` still works, because the logger reads it directly.)

### Base file and tenant profiles

Configuration splits in two, and the split is **enforced**: a key on the wrong side is a fatal error naming it.

| | Where | Settings |
|---|---|---|
| **General** | base file | `output`, `type`, `workers`, `workers-by-api`, `timeout`, `resolve-secrets`, `no-prompt`, `prune`, `transformers`, `taxonomy` |
| **Tenant-scoped** | `<config-dir>/<domain>.yaml` | `subscription`, `client-id`, `tenant-id`, `filters` |

The split is not bookkeeping. `transformers` is hashed into `transformConfigSha256`, so a per-tenant override
would make an export non-comparable with its own baseline and with every other tenant; `output` is the export
root and the tenant is already a subdirectory of it; `filters` is hashed into `filtersSha256`, which gates
drift comparability, so it must be stable *per tenant* rather than shared.

- **`--config <path>`** names the base file. A mistyped path is a fatal error, never a silent fallback.
- **`--config-dir <dir>`** holds one `<domain>.yaml` per tenant plus an optional **`base.yaml`**, which is
  picked up as the base file by convention — so the everyday invocation is two flags. It **requires
  `--domain`**: configuration is read before authentication (it supplies the credentials authentication
  needs), so the profile cannot be chosen by the tenant a run later resolves to. A domain with no profile is a
  fatal error listing the ones that exist.
- [`config.example.yaml`](config.example.yaml) and
  [`config.example.domain.yaml`](config.example.domain.yaml) are the **reference schemas** for the two files.
  Both carry the same guarantee — loading either unmodified behaves byte-for-byte like running without it,
  including every hash in `metadata.yaml` — so copy and change only what you need.
- [`config-tailored-intune.yaml`](config-tailored-intune.yaml) is a worked, opinionated **base** file for an
  Intune-centred tenant: secrets resolved, the cleaning and id-resolution transformers dropped, and a full
  `taxonomy:` for `docs generate-index`. A starting point, not a default — it changes the recorded hashes.

### Working with several tenants

```
~/.azure-rd/
  base.yaml                      # general settings, shared by every tenant
  contoso.onmicrosoft.com.yaml   # one profile per tenant, named after its domain
  fabrikam.onmicrosoft.com.yaml
```

```bash
azure-rd resource download --config-dir ~/.azure-rd --domain contoso.onmicrosoft.com
azure-rd resource drift    --config-dir ~/.azure-rd --domain fabrikam.onmicrosoft.com --exit-code
```

The profile's file name **is** the tenant's Entra default domain, which is also the export directory name
(`<output>/<domain>/`). The tool cross-checks it against the tenant you are actually signed in to and refuses
on mismatch, so a typo can never write one tenant's data under another tenant's directory. `--domain`
completes from the profiles in `--config-dir` and the exports under `--output`, so `azure-rd resource download
--config-dir ~/.azure-rd --domain <Tab>` answers *which tenants do I have?* — install completions with
`azure-rd completion zsh` (or `bash`, `fish`, `powershell`).

### Where each setting went

If you have scripts or pipelines on an older version:

| Was | Now |
|---|---|
| `--subscription`, `--client-id`, `--tenant-id` | `subscription`, `client-id`, `tenant-id` in the **tenant profile** |
| `--workers`, `--timeout` | `workers`, `timeout` in the **base file** |
| `--resolve-secrets`, `--no-prompt`, `--prune` | same keys in the **base file** — the destructive and secret-writing switches are deliberately written down rather than typed |
| `AZURE_RD_*` (any) | the matching config key; the environment is no longer read |
| `--type`, `--resource-id`, `--resource-group`, `--output`, `--domain`, `--dry-run`, `--log-level`, `--out`, `--prompt`, `--exit-code` | unchanged — these stayed on the command line |

## Output layout

Everything lives under `<output>/<tenant>/` in sibling trees that mirror each other exactly (`drift/` exists
only after a `resource drift` run, and is cleared by the next drift run or a re-baselining download):

```
output/
└── contoso.onmicrosoft.com/                      the tenant's Entra default domain
    ├── drift/                                    owned by azure-rd resource drift — the latest observation only
    │   ├── metadata.yaml                         what was compared, against which baseline, and the findings
    │   ├── analyze.md                            written by azure-rd docs analyze-drift (agent input)
    │   ├── index.md                              written by the agent: the drift summary for this observation
    │   ├── Microsoft.Graph/…/….yaml              fetched bytes of added/changed/renamed resources, mirroring resources/
    │   └── Microsoft.Graph/…/….md                written by the agent: per-resource drift document, beside its payload
    ├── resources/                                written by azure-rd resource download — and only by it
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
  filtersSha256: c41…            # hash of the effective resource-filter config (drift refuses across a change)
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
    transformConfigSha256: 7f…   # the config that produced THIS entry's bytes — partial runs merge, so the
                                 # run-level hash attests only the last run; drift trusts entries per-entry
  Microsoft.Graph/groups/gbl_d_win_all.yaml:
    groupTypes: [DynamicMembership]       # group-only facts, so a referenced group's kind
    securityEnabled: true                 # can be rendered without reading its YAML
notListed:
  types: []                     # could not be listed this run (permissions) — count unknown
  empty: [Microsoft.Graph/vppTokens]     # listed successfully to zero resources
```

Rules the tool holds itself to, because pruning is built on them:

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

`prune` is the **only** delete path inside the export (`resources/`); the single other delete in the tool is
a `resource drift` run clearing its own `drift/` tree before rebuilding it. After the merge, prune deletes the
YAML (and sidecar artifacts) of every entry marked `presentInTenant: false` within a type this run covered, drops the entry, and
sets `run.pruned: true`. It refuses to run — and says why — unless the run is complete and had no failed
resources. It never leaves `resources/`, never deletes `metadata.yaml`, removes a type's `doc-prompt.md` only
when the type emptied out entirely, and **never touches `docs/`**: a pruned resource leaves its document behind
as an orphan, which `generate-prompt` reports and leaves in place. Every deletion is logged, with a total. With
`prune: true` configured, `--dry-run` lists exactly what a real run would delete, from the same selection.

## Per-type documentation prompts (`doc-prompt.md`)

Every resource type directory receives a `doc-prompt.md` — the **specification** an AI must follow to
document a resource of that type. It is written on every run unless `no-prompt` is configured, and its hash is
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
documented **only when referenced by an assignment**. A type whose `doc-prompt.md` is missing (an export run
with `no-prompt` set) is reported and skipped — no document of that type can be produced. Resources marked
`presentInTenant: false` are reported as orphans and never regenerated or deleted. The finished
`docs/generate.md` carries none of the template's self-description: the explanatory header comment and the
` (template)` H1 suffix are stripped at render time (marker comments stay).

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

53 types: 3 Azure Resource Manager, 50 Microsoft Graph. `azure-rd resource types` prints the same list. Graph types use the
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
| `Microsoft.Graph/deviceConfigurations` | Legacy profiles, polymorphic incl. Custom/OMA-URI. `resolve-secrets` additionally needs `DeviceManagementConfiguration.ReadWrite.All`. |
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
  Defaults are 5 for Graph and 20 for ARM (`workers-by-api`). Precedence: `workers-by-api` → `workers` → API
  default. Setting `workers` at all is the signal that you want one count for every API, which is why the
  example file leaves it commented out; when a run covers exactly one API the API-specific count wins. More
  Graph workers usually make a run *slower* once throttling and backoff kick in.
- **Retries.** Transient failures (HTTP 429, 503, timeouts) are retried up to 5 times with exponential backoff.
  Permission errors (403, missing scopes) are never retried: the resource is skipped with a warning.
- **Timeout.** `timeout` (default 300 s) applies **per operation** — around each resource fetch including its
  retries — not to the whole run, so a large tenant needs no larger value.
- **Accounting.** Every request produces exactly one result, including on cancellation (`Cancelled`). The
  pipeline fails loudly if the counts disagree, because a silently dropped request would later look like a
  resource deleted from the tenant.

## Dry-run

`--dry-run` is honoured by every command and writes nothing:

- `download --dry-run` **lists** instead of downloading: it performs the per-type listing (so it still signs in
  and talks to Azure), then reports the set of resources a real run would fetch, the types it could not list and
  the types that listed empty, and whether that set is complete. No resource is fetched, transformed or
  written, and `metadata.yaml` is not updated. With `prune: true` configured it lists exactly the files a real
  run would delete, from the same selection.
- `resource drift --dry-run` still **fetches and compares in full** — nothing about drift is answerable
  without the tenant's current bytes — but withholds the `drift/` tree entirely (clearing nothing): an
  observation from an earlier run stays on disk and is reported as not refreshed.
- `docs generate-prompt --dry-run` runs the full comparison and reports the work list without writing
  `generate.md`; `docs generate-index --dry-run` reports the index counts without writing `index.yaml`;
  `docs analyze-drift --dry-run` runs the full preflight and reports the observation's findings without
  writing `analyze.md` (an `analyze.md` from an earlier run stays on disk and is reported as not refreshed).

## Logging

Structured key/value logging (charmbracelet/log). `--log-level` (or the plain `LOG_LEVEL` environment
variable, which the logger reads directly) selects `debug`, `info` (default), `warn` or `error`. It is
flag-only: verbosity belongs to one invocation, so it has no config key.
`debug` shows per-resource fetch/transform/write lines, retry attempts, sanitised names and the full error behind
every summarised warning; `info` shows listing counts, progress every 10 %, pipeline metrics and the summary.

Secrets are never logged: tokens, client secrets and resolved OMA-URI values are redacted.

## Security notes

- **Delegated only.** Every read is performed as the signed-in user and audited as such. There is no way to run
  the tool unattended with an application secret.
- **Secrets in output.** ARM handlers drop `adminPassword`, access keys and connection strings before writing.
  Graph returns most secrets already masked (`****`, `encryptedValueToken`); the tool keeps them masked and the
  documentation prompts instruct the model to treat masked values as expected, not as findings.
- **`resolve-secrets`** is the one deliberate exception: it resolves masked Intune OMA-URI secrets in
  `Microsoft.Graph/deviceConfigurations` through `getOmaSettingPlainTextValue` and writes them **in plaintext**.
  It is off by default, logs a warning when on, needs `DeviceManagementConfiguration.ReadWrite.All` in the token
  (delegated — the Intune backend rejects app-only tokens for this call), and degrades per setting: a value that
  cannot be resolved stays masked. Treat an export produced with it as sensitive material.
- **Credential-shaped free text.** Descriptions and decoded payloads sometimes contain pasted credentials the
  service does not mask. The prompts require the model to redact such values in the documentation and flag
  them under `Security`; the literal stays only in the YAML.
- **Files** are written `0644`, directories `0755`. The tool deletes only with `prune` configured, only under
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
├── NEXT-ITERATIONS.md            outstanding work and parked ideas (done entries are archived to ../.claude/archive/go/)
└── .windsurf/rules/              editor / AI-assistant rules for this folder
```

Design points that everything else leans on:

- **Handler registry.** Every type is a `models.ResourceHandler` (`GetType`, `List`, `Fetch`, `Transform`,
  `GetDocumentationPrompt`) registered in `internal/handlers/defaults.go`. Graph types are thin constructors
  around the shared `GraphCollectionHandler` supplying list/fetch/name closures plus their documentation
  metadata; ARM types implement the interface directly. Handlers also declare whether they need a dedicated app
  and whether they have an assignments concept.
- **Pipeline stages share nothing but channels**, and every request yields exactly one result — the invariant
  that keeps pruning safe.
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
make check           # fmt-check + lint-check + test + test-scripts — modifies nothing, so it can report on a commit as-is
make ci              # check + build (the default goal)
make deps            # download + tidy
make test-coverage   # coverage.html
make test-scripts    # tests for the readers behind the readiness reports and the start gate
make start-item N=2  # gate: may entry 2 of NEXT-ITERATIONS.md be implemented? (branch, clean tree, entry committed, plan open)
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

GoLand is not the only source the file mirrors: `gocognit` is enabled for the SonarQube quality profile's
cognitive-complexity rule, at **that profile's threshold rather than the linter's own default**, so a finding
here is the finding the server reports and neither tool can be satisfied without satisfying the other — measured
against a full analysis it covers every function the server flags, plus a few whose scores straddle the limit
(the two implementations differ by about a point, in both directions). golangci-lint's output truncation is
switched off (`max-issues-per-linter`, `max-same-issues`) because the server truncates nothing, and with the
defaults `make lint-check` would report a strict subset of it. `goconst` is deliberately **not** enabled for the
duplicated-string-literal rule: that pair cannot be made to agree — the server counts occurrences per file and
ignores identifier-like literals, `goconst` counts per package and by default skips call arguments, which is
where all of this project's findings are — so it stays a server-only signal rather than an approximation. See the
**Static analysis** section of the [repository README](../README.md).

**The code that predates `gocognit` is baselined, not exempted.** The end of the file's `exclusions.rules` names
the 26 functions (in 20 files) that already exceeded the threshold when the linter was switched on, so
`make lint-check`, `make check`, `make ci` and `make branch-ready` pass while **every new function has to
comply**. Each entry matches one function by name rather than excluding its file: add a second complex function
to a listed file and it is reported, which a path exclusion would have hidden. Two properties follow from doing
it this way — a listed function can still get *worse* unnoticed, since the match is on its name and not its
score, and nothing detects a stale entry, because golangci-lint does not report an exclusion that matched
nothing; an entry is therefore deleted in the same commit as the refactor that fixes its function, and the block
can be commented out to re-measure. It is a debt ledger rather than a decision: nothing in it is a finding this
project disagrees with, Sonar keeps reporting all 26 (which is why, unlike the two path-scoped `gocognit`
exclusions, it is deliberately *not* mirrored in `sonar-project.properties`), and clearing it is a parked idea in
`NEXT-ITERATIONS.md`. A finding you judge genuinely wrong is still silenced at its own site with a
`//nolint:gocognit`, never by adding to the baseline.

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
- **Work starts from the backlog.** Every change is a numbered entry in `NEXT-ITERATIONS.md`, committed before
  it is implemented — a one-line fix included, as a tiny entry. `make start-item N=<n>` is the gate: it refuses
  on the release branch, on a dirty `go/`, when entry `N` is not in **`HEAD`'s** backlog (the working copy does
  not count) or when it has no outstanding plan item, and otherwise prints the entry's Goal and Plan. Exit `2`
  is a usage error, `1` a refusal.
- Delivered plan items are **struck through** in `NEXT-ITERATIONS.md` while the branch is open, so it can be
  reviewed against what its entries set out to do, and the `CHANGELOG.md` entry is written in the same edit.
  When an entry is done it is **archived, never deleted**: moved with its full plan to
  `../.claude/archive/go/<finished-date>-<slug>.md` (frontmatter: title, status `done` or `dropped`, dates,
  branch, and `changelog: Unreleased` until the release stamps the version), and the remaining entries are
  renumbered. The changelog records what shipped and why; the archive keeps how. Nothing under the archive is
  read unless asked for. An abandoned entry is archived as `dropped` with a one-line reason.
- `make branch-ready` (or `make branch-ready-go` from the repository root) reports whether a feature or fix
  branch is ready to ship: `make ci` passes, nothing is left struck out in `NEXT-ITERATIONS.md` (done entries
  archived) and the rest is numbered contiguously, `## [Unreleased]` records the work, the branch is not the
  release branch, `NEXT-ITERATIONS.md` changed on the branch, and every entry archived as done on the branch
  grew `## [Unreleased]`. It edits nothing, and unlike `release-ready` it reports every check and **exits
  non-zero if any of them failed**, so it can gate a merge. An empty `[Unreleased]` is reported, not failed: a
  branch with no user-visible effect legitimately has none. There is no version check — this project's version
  is the `go/vX.Y.Z` tag, not a file.
- It **refuses to run while `go/` has uncommitted changes**, before `make ci`, so the verdict describes the
  commit that will be merged rather than the editor's current state. That preflight is a read-only
  `git status --porcelain` scoped to `go/`, so an unrelated edit in `../web` cannot block it. The branch
  checks run further read-only git (`rev-parse`, `merge-base`, `diff`, `show`) against the merge-base with the
  release branch (`RELEASE_BRANCH`, default `main`); outside a clone every git-backed check says so and is
  skipped. `release-ready` runs no git at all — it only adds a line listing the archived entries the release
  will stamp — and `ci` runs only the read-only `fmt-check`/`lint-check`, so nothing between the preflight
  and the verdict can change the tree.

Editor and AI-assistant rules live in `.windsurf/rules/` and apply to this folder only (Claude Code reads the
same rules from `CLAUDE.md`, `../.claude/rules/` and the procedures in `../.claude/skills/`):

| File | Covers |
|---|---|
| `01-project.md` | Purpose, layout, architecture patterns, non-negotiables (handler contract, pipeline accounting, metadata) |
| `02-style-and-quality.md` | Go style, error handling, testing, changelog policy |
| `03-commands.md` | Makefile usage, recipes for new handlers / commands / transformations / config options |
| `04-security-and-ops.md` | Credentials, secrets, configuration precedence, output layout, metadata and prune rules |
| `05-azure-conventions.md` | Resource type naming, API versions, display names, Graph SDK usage |
| `06-next-iterations.md` | How `NEXT-ITERATIONS.md` entries and parked ideas are managed, and the workflow from entry to archive |

## Known limitations

- **Assignment targets in the YAML carry GUIDs only.** Group, filter and notification-template names are
  resolved in the *documentation* (from the exported groups/filters/templates), not in the resource files. A
  transformer that resolved them in the YAML is deliberately parked — see `NEXT-ITERATIONS.md` for why.
- **Resource file writes are not atomic** (no temp-file-and-rename), so do not point another consumer at
  `resources/` while a download is running. `metadata.yaml` is the exception: it is written to a temp file and
  renamed into place, so the export baseline can never be left truncated. Re-running is always safe (writes are
  idempotent and `metadata.yaml` merges).
- **App-only authentication is out of scope** by design, so the tool cannot run unattended.

## License

[Add your license here]

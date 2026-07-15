# Azure Resource Downloader

A powerful command-line tool that downloads Azure resources and transforms them into clean YAML format. Built with Go and following the async pipeline pattern for maximum performance.

## 🚀 Features

- **Async Pipeline Architecture**: Parallel processing with configurable worker pools
- **Resource Transformation**: Clean YAML output with unnecessary Azure metadata removed
- **ID Resolution**: Automatically resolves Azure resource IDs to friendly names
- **Extensible Design**: Easy to add support for new Azure resource types
- **Multiple Resource Types**: Support for Resource Groups, Storage Accounts, Virtual Machines, and more

## 📋 Architecture

The tool follows a three-stage async pipeline. The stages run concurrently, connected by Go channels, so resources stream through as soon as they're fetched:

```
[FetchRequest] → Fetcher → [FetchResult] → Transformer → [TransformResult] → Writer → [WriteResult]
```

1. **Fetcher** — retrieves resources from Azure with retry logic (5 attempts, exponential backoff).
2. **Transformer** — applies configurable transformations: cleaning (property removal), ID resolution, name sanitization, and base64 decoding.
3. **Writer** — writes one YAML file per resource. With `--write-prompts` (or `write-prompts: true`) it also writes a documentation LLM prompt (`doc-prompt.md`) per resource type.

Each stage uses its own worker pool; the worker count is configurable via the `--workers` flag or API-specific settings in the config file (see *Worker Count Optimization* below).

## 🛠️ Installation

### Prerequisites

- Go 1.24 or later
- Azure CLI (for authentication) or Azure credentials configured

### Build from Source

```bash
# Clone the repository
git clone <repository-url>
cd azure-resource-downloader

# Download dependencies
go mod download

# Build
go build -o azure-rd

# Install globally (optional)
go install
```

## 🔐 Authentication

The tool authenticates as the user signed in with the Azure CLI (`az login`) and reuses that session for both ARM and Microsoft Graph calls — there is no separate sign-in or stored credential. Service principal / app-only credentials are **not** supported; run the tool as a privileged user (e.g. a Global / Intune Administrator).

```bash
az login                                              # sign in once
az account set --subscription "your-subscription-id"  # optional; a default is auto-detected
azure-rd download --subscription "your-subscription-id"
```

`--subscription` is optional — when omitted the tool resolves a default subscription the signed-in user can access. No app registration, client ID, or tenant ID is required unless you need Graph scopes the Azure CLI app can't provide (see [dedicated app registration](#optional-dedicated-app-registration-device-code-sign-in)).

### Required directory / Intune roles for the signed-in user

| Resource | Role the user needs |
|---|---|
| Conditional Access / Authentication Strength policies | Security Reader (or higher) |
| Intune Settings Catalog / device configurations | Intune Administrator or Global Reader |
| OMA-URI secret resolution (`--resolve-secrets`) | Intune Administrator (read rights on the profile) |

> ⚠️ **ARM is separate from Entra roles.** Being a Global Administrator does **not** grant Azure RBAC. To download the ARM types the signed-in user must additionally hold a subscription role such as **Reader** (or use *elevate access*). Otherwise those types return `403 AuthorizationFailed` and are skipped.

> ⚠️ **The Azure CLI app cannot obtain some Graph scopes.** Intune Settings Catalog / device configurations require `DeviceManagementConfiguration.Read.All` and authentication-strength policies require `Policy.Read.All`. The Microsoft Azure CLI is a Microsoft first-party app that is **not pre-authorized** for these scopes, so `az login` tokens can never include them (you'll see `AADSTS65002` if you request them). For those resource types use a **dedicated app registration** (below). Without it, those types are skipped gracefully.

### Optional: dedicated app registration (device-code sign-in)

To download resource types that need scopes the Azure CLI app can't provide, register your own Entra app and sign in to it with `--client-id`. The tool then uses an interactive **device-code** flow (prints a URL + code) and acquires a token carrying those delegated scopes.

**One-time app setup** (requires a privileged admin):

This grants every delegated scope the tool can use, so **all** supported resource types download with a single consent. Scope IDs are resolved by name from the Microsoft Graph service principal, so the script stays correct as Graph evolves.

```bash
GRAPH="00000003-0000-0000-c000-000000000000"
ARM="797f4846-ba00-4fd7-ba43-dac1f8f63013"
ARM_USER_IMP="41094075-9dad-400e-a0bd-54e686782033"      # user_impersonation (delegated)

# Delegated Microsoft Graph scopes covering every supported resource type.
# DeviceManagementConfiguration.ReadWrite.All is only needed for --resolve-secrets;
# all other scopes are read-only. Drop any you don't need (those types are skipped).
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

# Add the ARM delegated permission (storageAccounts, virtualMachines, resourceGroups)
az ad app permission add --id "$APP_ID" --api "$ARM" \
  --api-permissions "$ARM_USER_IMP=Scope"

# Create the service principal, then admin-consent (allow ~60s for replication)
az ad sp create --id "$APP_ID"
az ad app permission admin-consent --id "$APP_ID"
```

**Run with the dedicated app:**

```bash
azure-rd download --client-id "$APP_ID" --tenant-id "<your-tenant-id>"
```

> ⚠️ **Pass `--client-id` to `azure-rd`, not to `az login`.** Tokens returned by `az account get-access-token` are always minted for the Azure CLI first-party app (`04b07795-8ddb-461a-bbee-02f9e1bf7b46`) — even after `az login --client-id <app>` — so the extra Graph scopes never appear in them. Only the tool's own `--client-id`/`--tenant-id` flags (device-code sign-in) produce a token for your app. Also verify `$APP_ID` is actually set (`echo "$APP_ID"`): an **empty** value silently falls back to the az login session and the Graph types are skipped with permission warnings.

To verify which app and scopes a CLI session token carries:

```bash
az account get-access-token --resource https://graph.microsoft.com -o tsv --query accessToken \
  | python3 -c "import sys,base64,json; t=sys.stdin.read().strip().split('.')[1]; t+='='*(-len(t)%4); c=json.loads(base64.urlsafe_b64decode(t)); print(json.dumps({k:c.get(k) for k in ['appid','app_displayname','scp']}, indent=2))"
```

| Resource type | Delegated permission |
|---|---|
| `Microsoft.Graph/conditionalAccessPolicies` | `Policy.Read.All` |
| `Microsoft.Graph/authenticationStrengthPolicies` | `Policy.Read.All` |
| `Microsoft.Graph/deviceManagementConfigurationPolicies` | `DeviceManagementConfiguration.Read.All` |
| `Microsoft.Graph/deviceConfigurations` | `DeviceManagementConfiguration.Read.All` |
| `Microsoft.Graph/deviceConfigurations` + `--resolve-secrets` | `DeviceManagementConfiguration.ReadWrite.All` |
| `Microsoft.Graph/assignmentFilters`, `windowsFeatureUpdateProfiles`, `windowsQualityUpdateProfiles`, `windowsDriverUpdateProfiles` | `DeviceManagementConfiguration.Read.All` |
| `Microsoft.Graph/deviceCompliancePolicies`, `compliancePolicies`, `groupPolicyConfigurations`, `deviceManagementIntents` | `DeviceManagementConfiguration.Read.All` |
| `Microsoft.Graph/deviceCategories` | `DeviceManagementManagedDevices.Read.All` |
| `Microsoft.Graph/deviceManagementScripts`, `deviceShellScripts`, `deviceCustomAttributeShellScripts`, `deviceHealthScripts` | `DeviceManagementScripts.Read.All` |
| `Microsoft.Graph/deviceComplianceScripts`, `reusablePolicySettings`, `mobileThreatDefenseConnectors`, `ndesConnectors` | `DeviceManagementConfiguration.Read.All` |
| `Microsoft.Graph/roleScopeTags`, `roleDefinitions` | `DeviceManagementRBAC.Read.All` |
| `Microsoft.Graph/deviceManagement` (tenant settings) | `DeviceManagementServiceConfig.Read.All` |
| `Microsoft.Graph/authenticationMethodsPolicy`, `authorizationPolicy` | `Policy.Read.All` |
| `Microsoft.Graph/onPremisesSynchronization` | `OnPremDirectorySynchronization.Read.All` |
| `Microsoft.Graph/organization` | `Organization.Read.All` |
| `Microsoft.Graph/organizationalBranding` | `OrganizationalBranding.Read.All` (+ `Organization.Read.All`) |
| `Microsoft.Graph/groups` | `Group.Read.All` |
| `Microsoft.Graph/termsAndConditions`, `notificationMessageTemplates` | `DeviceManagementServiceConfig.Read.All` |
| `Microsoft.Graph/windowsAutopilotDeploymentProfiles`, `windowsAutopilotDeviceIdentities`, `deviceEnrollmentConfigurations`, `applePushNotificationCertificate`, `depOnboardingSettings`, `appleUserInitiatedEnrollmentProfiles` | `DeviceManagementServiceConfig.Read.All` |
| `Microsoft.Graph/intuneBrandingProfiles` | `DeviceManagementApps.Read.All` |
| `Microsoft.Graph/mobileApps`, `iosManagedAppProtections`, `androidManagedAppProtections`, `windowsManagedAppProtections`, `mdmWindowsInformationProtectionPolicies`, `windowsInformationProtectionPolicies`, `mobileAppConfigurations`, `targetedManagedAppConfigurations`, `vppTokens` | `DeviceManagementApps.Read.All` |
| `Microsoft.Graph/namedLocations` | `Policy.Read.All` |
| `Microsoft.Graph/termsOfUseAgreements` | `Agreement.Read.All` |
| ARM types (`storageAccounts`, `virtualMachines`, `resourceGroups`) | `Azure Service Management/user_impersonation` (+ your Azure RBAC) |

> The one-time setup above already grants every scope in this table. To add a scope later (e.g. a new resource type, or one you dropped from `GRAPH_SCOPES`), look up its permission ID by name and grant it:
> ```bash
> SCOPE_ID=$(az ad sp show --id "$GRAPH" --query "oauth2PermissionScopes[?value=='DeviceManagementRBAC.Read.All'].id" -o tsv)
> az ad app permission add --id "$APP_ID" --api "$GRAPH" --api-permissions "$SCOPE_ID=Scope"
> az ad app permission admin-consent --id "$APP_ID"
> ```
> Types whose scope is missing are simply skipped (reported in the summary).

> These are **delegated** permissions — the token acts as the signed-in user, so the user still needs the matching directory / Intune / Azure RBAC roles.

> **Graph token scopes (az login path):** if you get `Request Authorization failed` / "required scopes are missing" for Graph types the CLI app *can* serve, refresh the session:
> ```bash
> az logout && az login --scope https://graph.microsoft.com/.default
> ```

## 📖 Usage

### Basic Commands

```bash
# Show help
azure-rd --help

# List supported resource types (uses the signed-in user's default subscription)
azure-rd list

# Download a specific resource (uses default subscription)
azure-rd download \
  --resource-id "/subscriptions/.../resourceGroups/my-rg"

# Download all resources in a resource group
azure-rd download \
  --resource-group "my-resource-group"

# Download all resources of a specific type
azure-rd download \
  --type "Microsoft.Resources/resourceGroups"

azure-rd download \
  --type "Microsoft.Storage/storageAccounts"

# --type is a repeatable filter: pass it multiple times to download several types
azure-rd download \
  --type "Microsoft.Graph/deviceConfigurations" \
  --type "Microsoft.Graph/deviceManagementConfigurationPolicies"

# Omit --type (and --resource-id/--resource-group) to download EVERY registered type
azure-rd download

# Download Microsoft Graph resources (tenant-level)
azure-rd download \
  --type "Microsoft.Graph/conditionalAccessPolicies"

azure-rd download \
  --type "Microsoft.Graph/authenticationStrengthPolicies"

# Download all Intune Settings Catalog policies (Microsoft Graph beta)
azure-rd download \
  --type "Microsoft.Graph/deviceManagementConfigurationPolicies"

# Download all legacy Intune device configuration profiles, incl. Custom/OMA-URI (Microsoft Graph beta)
azure-rd download \
  --type "Microsoft.Graph/deviceConfigurations"

# Download a specific conditional access policy by ID
azure-rd download \
  --resource-id "12345678-1234-1234-1234-123456789abc"

# Override default subscription with explicit subscription ID
azure-rd download \
  --subscription "your-subscription-id" \
  --resource-group "my-resource-group"

# Dry run (preview without writing files)
azure-rd download \
  --resource-group "my-rg" \
  --dry-run
```

> **Flags vs. configuration:** run `azure-rd --help` (or `download`/`list --help`)
> for the full, authoritative list of CLI flags and their defaults. Every flag can
> also be set in a config file loaded with `--config` under a config key of the same
> name; the flag overrides the configured value for a single run (see
> [Configuration File](#configuration-file) for precedence). Options with **no**
> CLI flag — `workers-by-api`, `transformers` (including property removal via the
> `cleaning` transformer's `remove-keys` / `remove-keys-by-type`), and `filters` —
> are config-only; see [`.azure-rd.example.yaml`](.azure-rd.example.yaml) for the
> fully-commented schema.

### Download Multiple Resources

```bash
# Download multiple specific resources
azure-rd download \
  --resource-id "/subscriptions/.../resourceGroups/rg1" \
  --resource-id "/subscriptions/.../resourceGroups/rg2" \
  --resource-id "/subscriptions/.../Microsoft.Storage/storageAccounts/mysa"

# Download all resources of a specific type across the entire subscription
azure-rd download --type "Microsoft.Compute/virtualMachines"
azure-rd download --type "Microsoft.Network/virtualNetworks"
```

### Environment Variables

You can use environment variables instead of flags:

```bash
export AZURE_RD_SUBSCRIPTION="your-subscription-id"  # Optional - overrides the signed-in user's default subscription
export AZURE_RD_OUTPUT="./output"
export AZURE_RD_WORKERS="5"
export LOG_LEVEL="info"  # or debug, warn, error

azure-rd download --resource-group "my-rg"
```

**Available environment variables:**
- `AZURE_RD_SUBSCRIPTION` - Azure subscription ID (optional, uses the signed-in user's default subscription if not set)
- `AZURE_RD_CLIENT_ID` - App registration (client) ID for device-code sign-in (optional; defaults to the az login session)
- `AZURE_RD_TENANT_ID` - Entra tenant ID for device-code sign-in (used with `AZURE_RD_CLIENT_ID`)
- `AZURE_RD_OUTPUT` - Output directory path
- `AZURE_RD_WORKERS` - Number of concurrent workers
- `AZURE_RD_TIMEOUT` - Timeout in seconds for the download operation (default 300)
- `AZURE_RD_TYPE` - Resource type filter (equivalent to `--type`)
- `AZURE_RD_LOG_LEVEL` - Logging verbosity (debug, info, warn, error)
- `LOG_LEVEL` - Legacy logging verbosity (still supported)

### Configuration File

**Every option this tool accepts can be set in a configuration file** that you load explicitly with `--config` (e.g. `--config ~/.azure-rd.yaml`). Most options also expose a CLI flag that *overrides* the configured value for a single run. **A config file is read only when you pass `--config`** — without it, the built-in defaults apply (a mistyped `--config` path is a fatal error rather than being silently ignored).

**Precedence (highest to lowest):** CLI flag → environment variable → configuration file → built-in default.

> **CLI flags are documented by the tool itself** — run `azure-rd --help`, `azure-rd download --help`, or `azure-rd list --help` for the full list with defaults. Each flag maps to a config key of the same name (e.g. `--resource-group` → `resource-group`). This section documents only the options that have **no** CLI flag and can therefore be set **only** in the configuration file.

#### Config-only options (no CLI flag)

| Config key | Purpose |
|---|---|
| `workers-by-api` | Per-API worker counts (`microsoft-graph`, `azure-resource-manager`); overridden by `--workers` / `workers` |
| `transformers` | Transformer pipeline and per-transformer settings, including property removal via the `cleaning` transformer's `remove-keys` / `remove-keys-by-type` |
| `filters` | Per-resource-type property regex filters |

See [`.azure-rd.example.yaml`](.azure-rd.example.yaml) for the fully-commented schema of these options.

Create a config file (e.g. `~/.azure-rd.yaml`) and load it with `--config`:

```yaml
# All fields are optional
# subscription: "your-subscription-id"  # Optional - uses the signed-in user's default subscription if not specified
output: "./azure-resources"
workers: 10

# Timeout in seconds for the download operation (default: 300)
# Equivalent to --timeout; the flag overrides this value.
timeout: 300

# Resource type filter (optional) - equivalent to repeating --type.
# The flag overrides this value. Omit to download every registered type.
# type:
#   - Microsoft.Resources/resourceGroups
#   - Microsoft.Storage/storageAccounts

# Log level - controls verbosity (default: info)
# Options: debug, info, warn, error
log-level: "info"

# Property removal is configured on the `cleaning` transformer (see Transformers).
# Global exclusions apply to all types; per-type exclusions are merged with them.
transformers:
  - name: cleaning
    remove-keys:
      - etag
      - provisioningState
    remove-keys-by-type:
      Microsoft.Resources/resourceGroups:
        - id
        - managedBy
      Microsoft.Storage/storageAccounts:
        - id
        - primaryEndpoints
  - name: id-resolution
  - name: name-sanitization
```

The repository ships a fully-commented [`.azure-rd.example.yaml`](.azure-rd.example.yaml) documenting **every** option (type/item filtering, workers, logging, transformers). Copy it as a starting point:

```bash
cp .azure-rd.example.yaml ~/.azure-rd.yaml
```

Then run, passing the file with `--config`:

```bash
azure-rd download --config ~/.azure-rd.yaml --resource-group "my-rg"
```

### Logging

Control verbosity with `log-level` (`debug`, `info` default, `warn`, `error`). Set it via the `--log-level` flag, the `log-level` config key, or the `AZURE_RD_LOG_LEVEL` / `LOG_LEVEL` environment variable (precedence: flag → env → config → default).

```bash
azure-rd download --resource-group "my-rg" --log-level debug   # verbose, per-resource detail
azure-rd download --resource-group "my-rg" --log-level error   # errors only (CI/cron)
```

| Level | What you see |
|-------|--------------|
| `debug` | All messages incl. per-resource fetch/transform/write detail |
| `info` | Progress (every 10%), retries, warnings, errors, summary (**default**) |
| `warn` | Warnings and errors only |
| `error` | Errors only |

## 🔍 Resource Filters

Restrict which resources are downloaded per resource type by matching one or more properties against a regular expression (Go [RE2](https://github.com/google/re2/wiki/Syntax) syntax). This is useful to export only a naming-convention subset, e.g. all Intune device configurations prefixed `GBL_`.

Add a `filters` block to `~/.azure-rd.yaml`:

```yaml
filters:
  # Only export device configurations whose displayName starts with "GBL_"
  Microsoft.Graph/deviceConfigurations:
    displayName: "GBL_.*"

  # Multiple properties on one type must ALL match (logical AND)
  Microsoft.Graph/groups:
    displayName: "^IT-.*"
    mailEnabled: "true"

  # Anchor a regex for an exact prefix on storage accounts
  Microsoft.Storage/storageAccounts:
    name: "^prod"
```

**How it works**

- **Structure:** `filters.<resourceType>.<propertyPath>: <regex>`.
- **`<resourceType>`** matches a registered handler type (case-insensitive).
- **`<propertyPath>`** is a dot-separated path into the resource's raw properties (e.g. `displayName` or `properties.subnet.id`); path segments are matched case-insensitively.
- **`<regex>`** is a Go regular expression matched against the property value (use `^`/`$` to anchor).
- A resource is kept only when **every** property regex configured for its type matches. Use `^X$` for an exact match.
- Resource types **without** a filter are unaffected and download normally.
- Filters are evaluated **after fetch**, against the raw Azure properties (before the cleaning transformer can rename or remove keys). Excluded resources are still read from Azure but never written to disk; they are reported as `filtered` in the execution summary and do not affect the exit code.
- Invalid regular expressions are logged and skipped so the run proceeds with the remaining valid filters.

## 🎛️ Transformers

Each transformer can be independently configured with its own settings. By default, all transformers are applied. Set `transformers: []` to disable all and get raw Azure data.

Listing a transformer in the config **enables** it; omitting it disables it. Regardless of the order in the config file, transformers always execute in this fixed pipeline order:

```
cleaning → id-resolution → base64-decode → name-sanitization
```

### Available Transformers

**`cleaning`** - Remove unwanted properties and transform data
- `remove-keys` - Keys to remove globally (recursive)
- `remove-keys-by-type` - Resource-type-specific removals  
- `preserve-keys` - Preserve specific paths (exceptions to remove-keys)
- `replace` - Replace complex objects with field values
- `clean-empty` - Remove empty values (default: true)

**`id-resolution`** - Convert Azure resource IDs to friendly names

**`name-sanitization`** - Sanitize names for files

**`base64-decode`** - Decode base64-encoded values, either in place or into sidecar files
- `mode` - `inline` (default) replaces the encoded value with the decoded text in the YAML; `file` writes the decoded value to a sidecar file alongside the YAML instead
- `source-key` - Top-level property holding the base64 value (default: `payload`)
- `filename-key` - (file mode) Property holding the target file name for the top-level payload (default: `payloadFileName`)
- `extension` - (file mode) Extension applied to the decoded payload file; the existing extension on the file name is replaced (default: `.mobileconfig`)
- `remove-source` - (file mode) Remove the encoded value from the YAML output after decoding (default: `false`)

> Handles two locations in Intune `Microsoft.Graph/deviceConfigurations` profiles:
> - **macOS `payload`** (`macOSCustomConfiguration`): base64-encoded `.mobileconfig` plist. `inline` replaces `payload` with the decoded XML; `file` writes e.g. `payloadFileName: WindowsDefenderATPOnboarding.xml` to `WindowsDefenderATPOnboarding.mobileconfig`.
> - **Windows `omaSettings[]`** (`windows10CustomConfiguration`): `omaSettingStringXml` values are base64-encoded XML. `inline` replaces each value with the decoded XML; `file` writes each to its own `fileName` (e.g. `CB_VPN_Profile.xml`) as-is. Plain `omaSettingString` values are left untouched.
>
> Note: inline-decoded values are no longer base64, so re-importing to Intune requires re-encoding.

#### Encrypted OMA-URI secrets (`--resolve-secrets`)

Some Windows OMA-URI settings are stored as secrets; Microsoft Graph returns their `value` masked as `****` (this is **not** decodable — it's redacted server-side, not encoded). By default the masked value is kept as-is.

Passing `--resolve-secrets` makes the `Microsoft.Graph/deviceConfigurations` handler resolve each masked value to plaintext via the Graph `getOmaSettingPlainTextValue(secretReferenceValueId=...)` function and write it into the output.

```bash
az login   # signed in as an Intune admin
azure-rd download --type "Microsoft.Graph/deviceConfigurations" --resolve-secrets
```

Secret resolution reuses the **same** `az login` session as everything else — no separate sign-in is needed.

- **Delegated auth:** the Intune backend rejects app-only (service principal) tokens for `getOmaSettingPlainTextValue`. The `az login` user token is delegated, so resolution can work — provided the token carries the `DeviceManagementConfiguration.ReadWrite.All` scope and the user has Intune read rights on the profile.
- **Token scopes:** the Azure CLI token may not include the Intune write scope by default. If resolution returns `Forbidden`, refresh with `az login --scope https://graph.microsoft.com/.default` (or a scope that grants `DeviceManagementConfiguration.ReadWrite.All`).
- **Graceful degradation:** per-setting resolution failures are logged and skipped; the masked value is kept.
- **Security:** this writes secrets to disk in plaintext. Disabled by default; a warning is logged when enabled.

### Transformer Configuration Examples

Add to `~/.azure-rd.yaml`:

```yaml
# Example 1: Typical workflow
transformers:
  - name: cleaning
    remove-keys:
      - provisioningState
      - etag
      - systemData
    clean-empty: true
  - name: id-resolution
  - name: name-sanitization

# Example 2: Remove ID everywhere except specific paths
transformers:
  - name: cleaning
    remove-keys:
      - id                      # Remove "id" recursively everywhere
    preserve-keys:
      - properties.subnet.id    # But keep this specific one
    clean-empty: true
  - name: id-resolution
  - name: name-sanitization
```

### Common Use Cases

| Configuration | Output | Use Case |
|---------------|--------|----------|
| All transformers (default) | Clean data, resolved IDs, sanitized names | **Default** |
| Only `id-resolution` | Raw Azure data with resolved IDs | Debugging, keeping all metadata |
| Custom `remove-keys` | Selective property filtering | Fine-tuned data export |


### Worker Count Optimization

Optimal worker count depends on the **API**, not the resource type. The tool auto-selects sensible defaults (5 for Microsoft Graph, 20 for ARM) and warns when `--workers` is too high for the target API — too many Graph workers actually *slows* downloads (rate limiting + backoff).

| API | Resource types | Recommended | Rate limits |
|-----|----------------|-------------|-------------|
| Microsoft Graph | `Microsoft.Graph/*` | 3–5 | Strict (~7 req/sec) |
| Azure Resource Manager | `Microsoft.Storage/*`, `Microsoft.Compute/*`, `Microsoft.Resources/*`, … | 10–20 | Generous (1000s/min) |

Override the defaults with `workers-by-api` (per API) or `workers` (global) in `~/.azure-rd.yaml`, or `--workers` for a single run. Precedence: `--workers` → `workers-by-api` → `workers` → auto-default. See [`.azure-rd.example.yaml`](.azure-rd.example.yaml) for the worker config block.

### Customizing Output

Which properties land in the YAML is controlled by the `cleaning` transformer (see [Transformers](#-transformers)):

- `remove-keys` drops keys globally (recursive); `remove-keys-by-type` adds per-type removals — the two lists are **merged** for each type.
- `preserve-keys` keeps specific nested paths even if their key is in a remove list.
- By default the transformer only removes empty values; nothing else is dropped unless you configure `remove-keys`.

Common keys to drop: `provisioningState`, `etag`, `creationTime`, `changedTime`, `correlationId`, `managedBy`, `sku.tier`. See [`.azure-rd.example.yaml`](.azure-rd.example.yaml) for ready-to-use `cleaning` examples.

## 📂 Output Structure

The tool creates the following directory structure (the `doc-prompt.md` files are written only when `--write-prompts` / `write-prompts: true` is set):

Resources are written under a per-tenant directory named after the tenant's
Entra default domain (e.g. `contoso.onmicrosoft.com`), so downloads from
different tenants never collide. The domain is resolved via the ARM Tenants API;
if it cannot be resolved (e.g. insufficient permissions), the tool logs a
warning and writes directly into the base `--output` directory.

```
output/
└── contoso.onmicrosoft.com/            # tenant default domain
    ├── Microsoft.Resources/
    │   └── resourceGroups/
    │       ├── my-resource-group.yaml
    │       ├── another-resource-group.yaml
    │       └── doc-prompt.md            # only with --write-prompts
    ├── Microsoft.Storage/
    │   └── storageAccounts/
    │       ├── mystorageaccount.yaml
    │       └── doc-prompt.md            # only with --write-prompts
    └── Microsoft.Compute/
        └── virtualMachines/
            ├── my_vm.yaml
            └── doc-prompt.md            # only with --write-prompts
```

### YAML File

Each resource gets its own YAML file with clean representation of the Azure resource:

```yaml
id: /subscriptions/.../resourceGroups/my-rg
name: my-rg
location: eastus
tags:
  environment: production
  owner: team-platform
```

### Documentation Prompt File

When enabled with `--write-prompts` (or `write-prompts: true` in the config file; **off by default**), each resource type directory also receives a `doc-prompt.md` documentation prompt. It is a ready-to-use LLM prompt that instructs a model to generate end-user documentation for any resource YAML in that directory. The prompt asks the model to:

- **Document every setting** — one row per YAML property (path, configured value, what it does, recommended value, reference).
- **Link best practices and Microsoft docs** — Microsoft Learn URLs plus hardening baselines (Microsoft security baselines, CIS) where relevant.
- **Expand embedded payloads** — decode and document encoded/embedded properties such as `configurationXml`, `omaSettings`, `payloadJson` and base64 `payload` blobs.
- **Flag security-sensitive settings** — secrets, certificates, encryption, conditional-access conditions, and deviations from baselines.

Each resource type produces its **own dedicated prompt** (not a single shared template): the prompt is tailored with that type's purpose, notable settings and embedded payloads to expand. It is produced by each handler's `GetDocumentationPrompt()` method via `models.BuildDocumentationPrompt(models.ResourceDocumentation{...})`. ARM handlers supply this metadata inline; Microsoft Graph types are tailored through the `graphResourceDocs` table in `internal/handlers/graph/documentation.go`. To use a prompt, paste it together with a resource YAML from the same directory into an LLM.

## 🎯 Supported Resource Types

Currently supported Azure resource types:

| Azure Resource Type | Handler |
|---------------------|---------|
| `Microsoft.Resources/resourceGroups` | ✅ |
| `Microsoft.Storage/storageAccounts` | ✅ |
| `Microsoft.Compute/virtualMachines` | ✅ |
| `Microsoft.Graph/conditionalAccessPolicies` | ✅ |
| `Microsoft.Graph/authenticationStrengthPolicies` | ✅ |
| `Microsoft.Graph/deviceManagementConfigurationPolicies` | ✅ |
| `Microsoft.Graph/deviceConfigurations` | ✅ |
| `Microsoft.Graph/assignmentFilters` | ✅ |
| `Microsoft.Graph/windowsFeatureUpdateProfiles` | ✅ |
| `Microsoft.Graph/windowsQualityUpdateProfiles` | ✅ |
| `Microsoft.Graph/windowsDriverUpdateProfiles` | ✅ |
| `Microsoft.Graph/deviceCategories` | ✅ |
| `Microsoft.Graph/roleScopeTags` | ✅ |
| `Microsoft.Graph/termsAndConditions` | ✅ |
| `Microsoft.Graph/intuneBrandingProfiles` | ✅ |
| `Microsoft.Graph/notificationMessageTemplates` | ✅ |
| `Microsoft.Graph/namedLocations` | ✅ |
| `Microsoft.Graph/termsOfUseAgreements` | ✅ |
| `Microsoft.Graph/deviceManagementScripts` | ✅ |
| `Microsoft.Graph/deviceShellScripts` | ✅ |
| `Microsoft.Graph/deviceCustomAttributeShellScripts` | ✅ |
| `Microsoft.Graph/deviceHealthScripts` | ✅ |
| `Microsoft.Graph/deviceComplianceScripts` | ✅ |
| `Microsoft.Graph/reusablePolicySettings` | ✅ |
| `Microsoft.Graph/vppTokens` | ✅ |
| `Microsoft.Graph/mobileThreatDefenseConnectors` | ✅ |
| `Microsoft.Graph/ndesConnectors` | ✅ |
| `Microsoft.Graph/deviceCompliancePolicies` | ✅ |
| `Microsoft.Graph/compliancePolicies` | ✅ |
| `Microsoft.Graph/groupPolicyConfigurations` | ✅ |
| `Microsoft.Graph/deviceManagementIntents` | ✅ |
| `Microsoft.Graph/mobileApps` | ✅ |
| `Microsoft.Graph/iosManagedAppProtections` | ✅ |
| `Microsoft.Graph/androidManagedAppProtections` | ✅ |
| `Microsoft.Graph/windowsManagedAppProtections` | ✅ |
| `Microsoft.Graph/mdmWindowsInformationProtectionPolicies` | ✅ |
| `Microsoft.Graph/windowsInformationProtectionPolicies` | ✅ |
| `Microsoft.Graph/mobileAppConfigurations` | ✅ |
| `Microsoft.Graph/targetedManagedAppConfigurations` | ✅ |
| `Microsoft.Graph/windowsAutopilotDeploymentProfiles` | ✅ |
| `Microsoft.Graph/windowsAutopilotDeviceIdentities` | ✅ |
| `Microsoft.Graph/deviceEnrollmentConfigurations` | ✅ |
| `Microsoft.Graph/applePushNotificationCertificate` | ✅ |
| `Microsoft.Graph/depOnboardingSettings` | ✅ |
| `Microsoft.Graph/appleUserInitiatedEnrollmentProfiles` | ✅ |
| `Microsoft.Graph/roleDefinitions` | ✅ |
| `Microsoft.Graph/deviceManagement` | ✅ |
| `Microsoft.Graph/authenticationMethodsPolicy` | ✅ |
| `Microsoft.Graph/authorizationPolicy` | ✅ |
| `Microsoft.Graph/onPremisesSynchronization` | ✅ |
| `Microsoft.Graph/organization` | ✅ |
| `Microsoft.Graph/organizationalBranding` | ✅ |
| `Microsoft.Graph/groups` | ✅ |

> **Note:** The 15 collection types above (assignment filters through remediation scripts) all use the Microsoft Graph **beta** API via the shared `GraphCollectionHandler` (simple GET collection + GET item, full generic serialization).
>
> **Note:** Script resources (`deviceManagementScripts`, `deviceShellScripts`, `deviceCustomAttributeShellScripts`, `deviceHealthScripts`) carry base64-encoded script bodies (`scriptContent`, or `detectionScriptContent`/`remediationScriptContent` for Remediations). The base64-decode transformer decodes them inline by default; in `file` mode the decoded script is written as a sidecar file named after the resource's `fileName` (`.ps1`/`.sh`), or `<display_name>_detection.ps1` / `<display_name>_remediation.ps1` for Remediations.

> **Note:** `Microsoft.Graph/deviceManagementConfigurationPolicies` (Intune Settings Catalog) uses the Microsoft Graph **beta** API and downloads the full settings tree via `$expand=settings`.
>
> **Note:** macOS/iOS **DDM (Declarative Device Management)** policies have no dedicated Graph endpoint — they are Settings Catalog policies and are exported by `Microsoft.Graph/deviceManagementConfigurationPolicies`. A policy delivered via DDM is identifiable in the exported YAML by its `technologies` field containing `appleRemoteManagement` (the Graph DDM delivery channel). No separate handler or resource type is required.
>
> **Note:** `Microsoft.Graph/deviceConfigurations` (legacy Intune device configuration profiles) uses the Microsoft Graph **beta** API and covers the polymorphic profile types, including Custom/OMA-URI profiles (`windows10CustomConfiguration`, `androidCustomConfiguration`, `iosCustomConfiguration`, `macOSCustomConfiguration`). This is distinct from the Settings Catalog endpoint above. Requires `DeviceManagementConfiguration.Read.All`.
>
> **Note:** Compliance, Administrative Templates and Endpoint Security intents need child fetches beyond a plain GET:
> - `Microsoft.Graph/deviceCompliancePolicies` (classic, platform-polymorphic) is fetched with `$expand=scheduledActionsForRule($expand=scheduledActionConfigurations)`. The provider's compliance resources are per-platform (`windows`/`macos`/`ios`/`android_device_owner`/`aosp` variants); the Windows variant is emitted by default — adjust the import for other platforms.
> - `Microsoft.Graph/compliancePolicies` (Settings Catalog based, currently Linux) is fetched with `$expand=settings,scheduledActionsForRule(...)` and named via its `name` field.
> - `Microsoft.Graph/groupPolicyConfigurations` (Administrative Templates) additionally downloads the `definitionValues?$expand=definition` child collection so each configured ADMX setting carries its definition metadata.
> - `Microsoft.Graph/deviceManagementIntents` (legacy Endpoint Security) additionally downloads the `settings` child collection.
> - `Microsoft.Graph/deviceComplianceScripts` (Windows **custom compliance** scripts) carry a single base64 `detectionScriptContent`, decoded by the base64-decode transformer (inline by default, or a `*_detection.ps1` sidecar in file mode). Assignments are inlined. Distinct from `deviceHealthScripts` (Remediations).
> - `Microsoft.Graph/reusablePolicySettings` are reusable settings (e.g. firewall rule groups, certificates) referenced **by ID** from Endpoint Security / Settings Catalog policies; exporting them keeps those references resolvable. A plain GET returns the full `settingInstance` tree.
> - `Microsoft.Graph/mobileThreatDefenseConnectors` configure MTD partner integrations (e.g. Microsoft Defender for Endpoint) across Windows/macOS/iOS/Android. Connectors have no display name, so the item ID (partner identifier) is used as the name.
> - `Microsoft.Graph/ndesConnectors` expose the on-premises NDES/SCEP certificate connector state/metadata (certificate-based Windows config). Named by friendly name, falling back to the item ID.
>
> **Note:** Application (`deviceAppManagement`) types:
> - `Microsoft.Graph/mobileApps` is highly polymorphic (`win32LobApp`, `winGetApp`, `macOSPkgApp`, `iosStoreApp`, `officeSuiteApp`, …) and includes Microsoft built-in apps.
> - App protection policies (`iosManagedAppProtections`, `androidManagedAppProtections`, `windowsManagedAppProtections`) and app configurations (`targetedManagedAppConfigurations`) are fetched with `$expand=apps` so the targeted app list is included.
> - WIP policies (`mdmWindowsInformationProtectionPolicies`, `windowsInformationProtectionPolicies`) are deprecated by Microsoft; downloaded for documentation/backup only.
> - `Microsoft.Graph/mobileAppConfigurations` (managed-device app config) is platform-polymorphic.
> - `Microsoft.Graph/vppTokens` are Apple Volume Purchase Program tokens used to license store apps to macOS/iOS; the token secret is masked by the service (metadata only). Named by friendly name, falling back to organization name then Apple ID.
>
> **Note:** Autopilot & enrollment types:
> - `Microsoft.Graph/windowsAutopilotDeviceIdentities` is registered device *data* rather than configuration and can be a large collection; identities without a display name are named by serial number.
> - `Microsoft.Graph/deviceEnrollmentConfigurations` is polymorphic (enrollment limits, platform restrictions, ESP, Windows Hello for Business, enrollment notifications, incl. tenant defaults).
> - `Microsoft.Graph/applePushNotificationCertificate` is a tenant **singleton**: at most one file, named after the Apple ID; tenants without a certificate are skipped.
> - `Microsoft.Graph/depOnboardingSettings` (Apple ADE/DEP tokens) additionally downloads the `enrollmentProfiles` child collection per token.
> - `Microsoft.Graph/appleUserInitiatedEnrollmentProfiles` has no provider resource for the profile itself (only its assignment).
>
> **Note:** Tenant admin & Entra types:
> - `Microsoft.Graph/roleDefinitions` exports only **custom** Intune RBAC roles (built-in definitions are skipped during listing).
> - `Microsoft.Graph/deviceManagement` (Intune tenant settings), `authenticationMethodsPolicy` (v1.0), `authorizationPolicy` (v1.0) are tenant **singletons** — one file each.
> - `Microsoft.Graph/onPremisesSynchronization` (Entra Connect, v1.0) yields one file in hybrid tenants and none in cloud-only tenants.
> - `Microsoft.Graph/organization` (v1.0) exports the tenant information object.
> - `Microsoft.Graph/organizationalBranding` (beta) is a tenant **singleton** under the organization (`organization/{id}/branding`); it exports the default Entra company-branding object (incl. per-locale `localizations` via `$expand`) and yields no file when branding has not been configured. Distinct from `intuneBrandingProfiles` (Intune company portal branding).
> - `Microsoft.Graph/groups` (v1.0) exports the **full** directory group list incl. dynamic membership rules — this can be very large in big tenants.

### Handler Implementation Notes

Every handler implements the `ResourceHandler` interface (`GetType`, `List`, `Fetch`, `Transform`). Listing is handler-driven: ARM handlers delegate to shared pagers in `internal/azure/list.go`, Graph handlers page their own collection via `@odata.nextLink`.

| Handler group | SDK | Transform strategy |
|---|---|---|
| ARM (`resourceGroups`, `storageAccounts`, `virtualMachines`) | Azure SDK (`armresources`, `armstorage`, `armcompute`) | Hand-picked property set; secrets (`adminPassword`, access keys, connection strings) are **never** written to output |
| Graph v1.0 (`conditionalAccessPolicies`, `authenticationStrengthPolicies`, `groups`, `organization`, `onPremisesSynchronization`, `authenticationMethodsPolicy` + `authorizationPolicy` singletons) | `msgraph-sdk-go` (stable) | `GraphCollectionHandler` base; full generic serialization of the model tree via the Kiota JSON writer |
| Graph beta with custom fetch (`deviceManagementConfigurationPolicies` + `compliancePolicies` + `deviceCompliancePolicies` via `$expand`, `groupPolicyConfigurations` + `deviceManagementIntents` + `depOnboardingSettings` via child-collection fetches, `deviceConfigurations` with optional OMA secret resolution, `applePushNotificationCertificate` as singleton) | `msgraph-beta-sdk-go` | `GraphCollectionHandler` base with custom `fetchItem` closures; full generic serialization of the polymorphic `@odata.type` tree — no setting is lost |
| Graph beta collections (assignment filters, update profiles, device categories, scope tags, T&C, branding, notification templates, named locations, ToU agreements, mobile apps, app protections, WIP policies, app configurations) | `msgraph-beta-sdk-go` | Shared `GraphCollectionHandler` base (`graph/collection.go`): per-resource constructors supply list/fetch/name closures; transform is full generic serialization |
| Graph beta scripts (Windows platform, macOS shell, macOS custom attribute, Remediations) | `msgraph-beta-sdk-go` | Same `GraphCollectionHandler` base; base64 script bodies are decoded by the base64-decode transformer (inline or `.ps1`/`.sh` sidecar files) |

Handlers are split into two subpackages: ARM handlers live in `internal/handlers/arm` (package `arm`) and implement the `ResourceHandler` interface directly; Microsoft Graph handlers live in `internal/handlers/graph` (package `graph`) as thin constructors around the shared `GraphCollectionHandler` (`internal/handlers/graph/collection.go`). The `Registry` itself lives in `internal/handlers` (`registry.go`, package `handlers`).

Established `fetchItem` patterns for types needing more than a plain GET:

- **`$expand`** — pass query parameters in the item request config (see `devicecompliancepolicy.go`).
- **Child-collection fetches** — page the child collection and attach it to the model before serialization (see `grouppolicyconfiguration.go`, `devicemanagementintent.go`, `deponboardingsetting.go`).
- **Post-fetch enrichment** — mutate the fetched model (see `deviceconfiguration.go` OMA secret resolution).
- **Singletons** — probe the object in `listIDs` (return at most one ID, empty when absent) and ignore the item ID in `fetchItem` (see `applepushnotificationcertificate.go`).
- **Assignments** — Intune policies/profiles/apps/scripts fetch the `/{id}/assignments` child collection in `fetchItem` and attach it to the model via `SetAssignments`, so assignments are inlined under `assignments` in the exported YAML. Exception: the Graph beta service has no `/assignments` route for `deviceShellScripts` and `deviceCustomAttributeShellScripts`, so those two handlers read assignments via a second item GET with `$expand=assignments` instead. Assignment reads are best-effort: on failure a warning is logged (`warnAssignmentsFetchFailed` in `graph/collection.go`) and the item is exported without assignments.

> **Known limitation:** group display names referenced by assignment targets are not resolved — assignments carry the target group IDs only (groups themselves are exported by `Microsoft.Graph/groups`).

## 📌 Backlog

Planned but not yet implemented:

- **ID-to-name resolution transformer** — a transformer that replaces attributes referencing object IDs (e.g. `groupId` in assignment targets) with the corresponding display name, resolving the known limitation above.

## 🔧 Adding New Resource Types

The tool is designed to be easily extensible. To add support for a new resource type:

### 1. Create a Handler

Create a new file in the appropriate subpackage — `internal/handlers/arm/` for ARM types or `internal/handlers/graph/` for Microsoft Graph types. For an ARM type (e.g., `internal/handlers/arm/keyvault.go`):

```go
package arm

import (
    "context"
    "azure-resource-downloader/internal/models"
    // Import Azure SDK for the resource
)

type KeyVaultHandler struct {
    credential     azcore.TokenCredential
    subscriptionID string
}

func NewKeyVaultHandler(credential azcore.TokenCredential, subscriptionID string) *KeyVaultHandler {
    return &KeyVaultHandler{
        credential:     credential,
        subscriptionID: subscriptionID,
    }
}

func (h *KeyVaultHandler) GetType() string {
    return "Microsoft.KeyVault/vaults"
}

// GetDocumentationPrompt returns the dedicated LLM documentation prompt for
// this type. Supply type-specific metadata (Purpose, KeySettings,
// EmbeddedPayloads) so the prompt is tailored to this resource type.
func (h *KeyVaultHandler) GetDocumentationPrompt() string {
    return models.BuildDocumentationPrompt(models.ResourceDocumentation{
        AzureType:     h.GetType(),
        Purpose:       "An Azure Key Vault that stores secrets, keys and certificates, with its access and network configuration.",
        KeySettings:   []string{"enableRbacAuthorization", "networkAcls", "enableSoftDelete", "enablePurgeProtection"},
    })
}

// List enumerates all resource IDs of this type. ARM handlers delegate to the
// shared azure pager; Microsoft Graph handlers page their own collection.
func (h *KeyVaultHandler) List(ctx context.Context) ([]string, error) {
    return azure.ListResourcesByType(ctx, h.credential, h.subscriptionID, h.GetType())
}

func (h *KeyVaultHandler) Fetch(ctx context.Context, resourceID string) (interface{}, error) {
    // Implement fetching logic using Azure SDK
}

func (h *KeyVaultHandler) Transform(resource interface{}) (*models.TransformedResource, error) {
    // Implement transformation logic
}
```

### 2. Register the Handler

Add the handler registration in `internal/handlers/defaults.go` → `registerDefaults()`. This is the single place where all handlers are wired; `handlers.NewRegistry(cred, subscriptionID, resolveSecrets)` builds a registry pre-populated by this function, so the `cmd` commands need no changes.

```go
func registerDefaults(r *Registry, cred azcore.TokenCredential, subscriptionID string, resolveSecrets bool) {
    // Existing ARM handlers (from the arm subpackage)
    r.Register("Microsoft.Resources/resourceGroups", arm.NewResourceGroupHandler(cred, subscriptionID))
    r.Register("Microsoft.Storage/storageAccounts", arm.NewStorageAccountHandler(cred, subscriptionID))

    // Add your new handler
    r.Register("Microsoft.KeyVault/vaults", arm.NewKeyVaultHandler(cred, subscriptionID))
}
```

### 3. Test

```bash
make build
./azure-rd list  # Uses the signed-in user's default subscription
```

That's it! Your new resource type is now supported.

## 🏗️ Project Structure

```
azure-resource-downloader/
├── cmd/                    # CLI commands (Cobra)
│   ├── root.go            # Root command and configuration
│   ├── download.go        # Download command
│   └── list.go            # List command
├── internal/
│   ├── models/            # Core types and interfaces
│   │   ├── types.go       # ResourceHandler interface, pipeline types
│   │   └── api.go         # API type detection (ARM vs Graph), worker defaults
│   ├── pipeline/          # Pipeline implementation
│   │   ├── pipeline.go    # Orchestrator
│   │   ├── fetcher.go     # Fetch stage (retry + permission skips)
│   │   ├── transformer.go # Transform stage
│   │   ├── writer.go      # Write stage
│   │   └── metrics.go     # Execution metrics
│   ├── handlers/          # Handler registry + handler subpackages
│   │   ├── registry.go    # Registry (package handlers)
│   │   ├── arm/           # ARM handlers (package arm)
│   │   │   ├── resourcegroup.go
│   │   │   ├── storageaccount.go
│   │   │   └── virtualmachine.go
│   │   └── graph/         # Microsoft Graph handlers (package graph)
│   │       ├── collection.go                          # Shared base for ALL Microsoft Graph handlers (v1.0 + beta)
│   │       ├── conditionalaccesspolicy.go             # Graph v1.0 (on the base)
│   │       ├── authenticationstrengthpolicy.go        # Graph v1.0 (on the base)
│   │       ├── devicemanagementconfigurationpolicy.go # Intune Settings Catalog, $expand=settings (Graph beta)
│   │       ├── deviceconfiguration.go                 # Legacy Intune profiles incl. OMA-URI secret resolution (Graph beta)
│   │       └── assignmentfilter.go, rolescopetag.go, … # collection handlers built on the base (incl. 4 script types)
│   ├── azure/             # Azure client wrappers
│   │   ├── client.go      # Auth (az login / device-code) + ARM client
│   │   ├── errors.go      # Permission-error detection (warn & skip)
│   │   ├── list.go        # Shared ARM listing pagers
│   │   └── resolver.go    # ID to name resolver
│   ├── logger/            # Structured logging (charmbracelet/log)
│   ├── retry/             # Exponential backoff for transient errors
│   └── transform/         # Transformation utilities
│       ├── cleaner.go     # Key removal / empty-value cleanup
│       ├── sanitizer.go   # Filename sanitizer
│       └── base64.go      # Base64 payload decoding (inline / sidecar files)
├── go.mod
├── main.go
└── README.md
```

## 🤖 Editor & AI Assistant Rules

This repo ships machine-readable coding conventions for AI pair-programming tools. The same rule set is maintained for both editors:

- **Cursor**: `.cursor/rules/*.md`
- **Windsurf**: `.windsurf/rules/*.md` (with activation frontmatter)

| File | Purpose | Windsurf activation |
|------|---------|---------------------|
| `01-project.md` | Project context, architecture & non-negotiables | `always_on` |
| `02-style-and-quality.md` | Go style, errors, testing philosophy, logging, docs policy | `glob` (`**/*.go`) |
| `03-commands.md` | Makefile-first workflow & generation recipes | `always_on` |
| `04-security-and-ops.md` | Secrets, config precedence, ops & production readiness | `always_on` |
| `05-azure-conventions.md` | Handler structure, Graph/Intune SDK usage, naming | `glob` (`internal/handlers/**`, `internal/azure/**`) |

When changing project conventions, update the rule files in **both** directories so Cursor and Windsurf stay in sync.

## 🤝 Contributing

Contributions are welcome! Here are some ways you can contribute:

1. Add support for new Azure resource types
2. Improve transformation logic
3. Add tests
4. Improve documentation
5. Report bugs

## 📝 License

[Add your license here]

## 🙏 Acknowledgments

- Built with [Azure SDK for Go](https://github.com/Azure/azure-sdk-for-go)
- CLI powered by [Cobra](https://github.com/spf13/cobra)
- Configuration with [Viper](https://github.com/spf13/viper)

## 📞 Support

For issues and questions:
- Open an issue on GitHub
- Check existing issues for solutions

## 🐛 Debug Logging

Run with `--log-level debug` to see detailed transformation operations:

**Cleaning transformer:**
```
DEBUG Removed key key=id path=id
DEBUG Removed empty array key=excludeApplications path=conditions.applications.excludeApplications type=[]string
DEBUG Preserving key (in preserve-keys list) key=id path=properties.subnet.id
DEBUG Replaced key value from=grantControls.authenticationStrength.displayName to=grantControls.authenticationStrength
DEBUG Removed excluded keys keys_removed=[id etag systemData] count=3
DEBUG Removed empty values keys_removed=[conditions.applications.excludeApplications ...] count=8
```

**ID resolution transformer:**
```
DEBUG Resolved resource IDs to names ids_resolved=[virtualNetworkId subnet.id] count=2
```

**Name sanitization transformer:**
```
DEBUG Sanitized name original="My-Resource@Group!" sanitized=my_resource_group
```

Debug logging shows exactly what each transformer does, which keys are removed/preserved/replaced, and why.


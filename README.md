# Azure Resource Downloader

A powerful command-line tool that downloads Azure resources, transforms them into clean YAML format, and generates Terraform import statements. Built with Go and following the async pipeline pattern for maximum performance.

## ToDo

- Add support for Intune Settings Catalog policies
- Add old and new device management policies (deviceManagementConfigurationPolicies are the new ones?)
- Add support for remediation/platform scripts (old and new)
- Add support for onboarding scrrens/profiles (OOBE settings/profiles, Mac deployment profiles etc) 
- Add support for Entra Groups incl. dynamic groups

## 🚀 Features

- **Async Pipeline Architecture**: Parallel processing with configurable worker pools
- **Resource Transformation**: Clean YAML output with unnecessary Azure metadata removed
- **ID Resolution**: Automatically resolves Azure resource IDs to friendly names
- **Terraform Integration**: Generates import statements for easy Terraform adoption
- **Extensible Design**: Easy to add support for new Azure resource types
- **Multiple Resource Types**: Support for Resource Groups, Storage Accounts, Virtual Machines, and more

## 📋 Architecture

The tool follows a three-stage async pipeline pattern:

```
Fetcher → Transformer → Writer
  ↓           ↓           ↓
Azure API   Clean Data   YAML + TF
```

Each stage runs concurrently with configurable worker pools for optimal performance.

### Pipeline Stages

1. **Fetcher**: Retrieves resources from Azure using the Azure SDK with retry logic
2. **Transformer**: 
   - Removes unnecessary properties (provisioningState, etag, etc.)
   - Resolves resource IDs to names
   - Sanitizes display names for filenames
   - Generates Terraform import statements
3. **Writer**: Saves resources as YAML files and consolidated Terraform import.tf files

### 📚 Pipeline Architecture

The pipeline uses a streaming architecture with three concurrent stages connected via Go channels for maximum parallelism.

**Pipeline Flow:**
```
[FetchRequest] → Fetcher (workers) → [FetchResult] → Transformer (workers) → [TransformResult] → Writer (workers) → [WriteResult]
```

All stages run **concurrently** - resources flow through as soon as they're fetched, enabling true parallelism.

**Stage Details:**

1. **Fetcher** - Retrieves resources from Azure with retry logic (5 attempts, exponential backoff)
2. **Transformer** - Applies configurable transformations (cleaning, ID resolution, name sanitization, Terraform import generation)
3. **Writer** - Writes YAML files and consolidated import.tf per resource type

Each stage uses a worker pool for parallel processing. Worker count is configurable via `--workers` flag or API-specific settings in config file.

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

The tool uses Azure's DefaultAzureCredential, which supports multiple authentication methods:

1. **Azure CLI**: `az login` (recommended)
2. **Managed Identity**: When running on Azure resources
3. **Environment Variables**: `AZURE_TENANT_ID`, `AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET`
4. **Service Principal**: Configure via environment variables

### Quick Setup

```bash

# Login via Azure CLI (easiest method)
az login

# The tool will automatically use the default subscription from your Azure CLI session
# Optionally, set a specific subscription (if you have multiple)
az account set --subscription "your-subscription-id"

# Or override the subscription using the --subscription flag
azure-rd download --subscription "different-subscription-id" --resource-group "my-rg"
```

**Note**: The subscription ID is optional. If not specified via CLI flag, config file, or environment variable, the tool will automatically use the default subscription from your `az login` session.

### Microsoft Graph Permissions

Some resources (like Conditional Access Policies and Authentication Strength Policies) are accessed via Microsoft Graph API and require additional permissions:

**For Azure CLI users (`az login`):**
```bash
# Your user account needs the appropriate Azure AD role assignments
# Required roles for Conditional Access Policies:
# - Security Reader (read-only)
# - Security Administrator (read/write)
# - Global Administrator (full access)

# Required roles for Authentication Strength Policies:
# - Security Reader (read-only)
# - Security Administrator (read/write)
# - Global Administrator (full access)

# Required roles for Intune Settings Catalog (deviceManagementConfigurationPolicies):
# - Intune Administrator
# - Global Reader (read-only)
# - Global Administrator (full access)
```

**For Service Principal authentication:**
```bash
# Your service principal needs Microsoft Graph API permissions
# Required API permissions for Conditional Access Policies:
# - Policy.Read.All (read-only)
# - Policy.ReadWrite.ConditionalAccess (read/write)

# Required API permissions for Authentication Strength Policies:
# - Policy.Read.All (read-only)
# - Policy.ReadWrite.AuthenticationMethod (read/write)

# Required API permissions for Intune Settings Catalog (deviceManagementConfigurationPolicies):
# - DeviceManagementConfiguration.Read.All (read-only)
# - DeviceManagementConfiguration.ReadWrite.All (read/write)

# To grant permissions:
# 1. Register an app in Azure AD
# 2. Add Microsoft Graph API permissions
# 3. Grant admin consent for the permissions
# 4. Set environment variables:
export AZURE_TENANT_ID="your-tenant-id"
export AZURE_CLIENT_ID="your-client-id"
export AZURE_CLIENT_SECRET="your-client-secret"
```

**Note**: If you receive permission errors when listing Graph resources, even as a Global Administrator, this is likely a **token scope issue**. See [Troubleshooting Graph Permissions](docs/TROUBLESHOOTING-GRAPH-PERMISSIONS.md) for solutions.

**Quick Fix** (if you're getting "Request Authorization failed"):
```bash
# Logout and re-login with proper scope
az logout
az login --scope https://graph.microsoft.com/.default

# Or create a service principal with proper permissions
./docs/grant-graph-permissions.sh
```

## 📖 Usage

### Basic Commands

```bash
# Show help
azure-rd --help

# List supported resource types (uses default subscription from az login)
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

# Specify output directory
azure-rd download \
  --resource-group "my-rg" \
  --output "./azure-resources"

# Adjust worker count for performance
# Recommended: 3-5 for Graph API, 10-20 for ARM
azure-rd download \
  --type "Microsoft.Graph/conditionalAccessPolicies" \
  --workers 5

azure-rd download \
  --type "Microsoft.Storage/storageAccounts" \
  --workers 15

# Control log verbosity
LOG_LEVEL=debug azure-rd download \
  --resource-group "my-rg"

# Remove specific keys from output (e.g., for Terraform imports)
azure-rd download \
  --resource-group "my-rg" \
  --remove-keys "id,etag,provisioningState"
```

### Log Levels

Control output verbosity with the `LOG_LEVEL` environment variable:

```bash
# Show only errors (quiet mode)
LOG_LEVEL=error azure-rd download --resource-group "my-rg"

# Show warnings and above
LOG_LEVEL=warn azure-rd download --resource-group "my-rg"

# Default: info level (recommended)
azure-rd download --resource-group "my-rg"

# Show debug information (verbose, includes file paths)
LOG_LEVEL=debug azure-rd download --resource-group "my-rg"
```

**Available levels:** `debug`, `info` (default), `warn`, `error`, `fatal`

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
export AZURE_RD_SUBSCRIPTION="your-subscription-id"  # Optional - overrides default from az login
export AZURE_RD_OUTPUT="./output"
export AZURE_RD_WORKERS="5"
export LOG_LEVEL="info"  # or debug, warn, error

azure-rd download --resource-group "my-rg"
```

**Available environment variables:**
- `AZURE_RD_SUBSCRIPTION` - Azure subscription ID (optional, uses default from az login if not set)
- `AZURE_RD_OUTPUT` - Output directory path
- `AZURE_RD_WORKERS` - Number of concurrent workers
- `AZURE_RD_REMOVE_KEYS` - Comma-separated list of keys to remove from output
- `AZURE_RD_LOG_LEVEL` - Logging verbosity (debug, info, warn, error)
- `LOG_LEVEL` - Legacy logging verbosity (still supported)

### Configuration File

Create `~/.azure-rd.yaml`:

```yaml
# All fields are optional
# subscription: "your-subscription-id"  # Optional - uses default from az login if not specified
output: "./azure-resources"
workers: 10

# Log level - controls verbosity (default: info)
# Options: debug, info, warn, error
log-level: "info"

# Global exclusions (apply to all resource types)
# Specify which keys to remove from output
remove-keys:
  - etag
  - provisioningState

# Resource-type-specific exclusions (merged with global)
# Useful for Terraform imports where different resources need different exclusions
remove-keys-by-type:
  Microsoft.Resources/resourceGroups:
    - id
    - managedBy
  Microsoft.Storage/storageAccounts:
    - id
    - primaryEndpoints
```

You can also copy the example configuration:

```bash
cp .azure-rd.example.yaml ~/.azure-rd.yaml
```

Then run:

```bash
azure-rd download --resource-group "my-rg"
```

### Logging Configuration

Control the verbosity of output with the `log-level` setting. Available in **three ways** (priority order):

#### **1. CLI Flag (Highest Priority)**
```bash
# Debug mode - see all details
azure-rd download --resource-group "my-rg" --log-level debug

# Quiet mode - errors only
azure-rd download --resource-group "my-rg" --log-level error

# Warnings and errors only
azure-rd download --resource-group "my-rg" --log-level warn
```

#### **2. Configuration File**
In `~/.azure-rd.yaml`:
```yaml
log-level: "info"  # debug, info, warn, or error
```

#### **3. Environment Variable (Lowest Priority)**
```bash
# Option 1: Use AZURE_RD_LOG_LEVEL (recommended)
export AZURE_RD_LOG_LEVEL=debug
azure-rd download --resource-group "my-rg"

# Option 2: Use legacy LOG_LEVEL (still supported)
export LOG_LEVEL=debug
azure-rd download --resource-group "my-rg"
```

#### **Available Log Levels**

| Level | What You See | Use Case |
|-------|--------------|----------|
| `debug` | All messages including detailed debug info | Troubleshooting, development |
| `info` | Progress, metrics, warnings, errors | **Default** - normal operation |
| `warn` | Warnings and errors only | Reduce noise, still see issues |
| `error` | Errors only | CI/CD, cron jobs, silent mode |

**Examples of what you'll see:**

**`debug` level:**
- DEBUG: Fetching resource X
- DEBUG: Resource fetched successfully
- DEBUG: Transforming resource X
- DEBUG: Resource transformed successfully
- DEBUG: Writing resource files
- DEBUG: Resource files written successfully
- INFO: Progress updates (every 10%)
- WARN: Retry attempts
- INFO: Performance metrics

**`info` level (default):**
- INFO: Progress updates (every 10%)
- INFO: Retry succeeded messages (when retries work)
- WARN: Retry attempts
- ERROR: Any errors
- INFO: Performance summary
- (No per-resource details - keeps output clean!)

**`warn` level:**
- WARN: Retry warnings
- ERROR: Errors
- (Progress updates hidden)

**`error` level:**
- ERROR: Only errors that occurred
- (Minimal output for automation)

## 🎛️ Transformers

Each transformer can be independently configured with its own settings. By default, all transformers are applied. Set `transformers: []` to disable all and get raw Azure data.

### Available Transformers

**`cleaning`** - Remove unwanted properties and transform data
- `remove-keys` - Keys to remove globally (recursive)
- `remove-keys-by-type` - Resource-type-specific removals  
- `preserve-keys` - Preserve specific paths (exceptions to remove-keys)
- `replace` - Replace complex objects with field values
- `clean-empty` - Remove empty values (default: true)

**`id-resolution`** - Convert Azure resource IDs to friendly names

**`name-sanitization`** - Sanitize names for files/Terraform

**`terraform-import`** - Generate Terraform import blocks
- `target-format` - Template for import address (default: `{resource_type}.{name}`)

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
> Note: inline-decoded values are no longer base64, so re-importing to Intune/Terraform requires re-encoding.

#### Encrypted OMA-URI secrets (`--resolve-secrets`)

Some Windows OMA-URI settings are stored as secrets; Microsoft Graph returns their `value` masked as `****` (this is **not** decodable — it's redacted server-side, not encoded). By default the masked value is kept as-is.

Passing `--resolve-secrets` makes the `Microsoft.Graph/deviceConfigurations` handler resolve each masked value to plaintext via the Graph `getOmaSettingPlainTextValue(secretReferenceValueId=...)` function and write it into the output.

```bash
azure-rd download --type "Microsoft.Graph/deviceConfigurations" \
  --resolve-secrets \
  --secrets-client-id "<public-client-app-id>"
# (optional) --secrets-tenant-id "<tenant-id>"   # defaults to AZURE_TENANT_ID
```

On start you'll be prompted with a device-code URL + code; sign in as an Intune admin. After consent, downloads proceed and encrypted values are resolved.

- **Delegated device-code auth required:** the Intune backend rejects **app-only** (service principal) tokens for `getOmaSettingPlainTextValue`, even with `DeviceManagementConfiguration.ReadWrite.All`. The Azure CLI's own token also can't carry that scope. Resolution therefore uses an interactive **device-code** sign-in against a public client app you supply via `--secrets-client-id`; normal fetching still uses your service principal.
- **Public client requirements:** the app registration needs delegated `DeviceManagementConfiguration.ReadWrite.All`, *Allow public client flows* enabled, and the signed-in user must be an Intune admin. Tenant defaults to `AZURE_TENANT_ID` (override with `--secrets-tenant-id`).
- **Graceful degradation:** if sign-in/consent fails, secret resolution is disabled with a warning and masked values are kept. Per-setting failures are logged and skipped.
- **Security:** this writes secrets to disk in plaintext. Disabled by default; a warning is logged when enabled.

### Configuration File

Add to `~/.azure-rd.yaml`:

```yaml
# Example 1: Typical Terraform workflow
transformers:
  - name: cleaning
    remove-keys:
      - provisioningState
      - etag
      - systemData
    clean-empty: true
  - name: id-resolution
  - name: name-sanitization
  - name: terraform-import

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
  - name: terraform-import

# Example 3: Documentation only (no Terraform)
transformers:
  - name: cleaning
  - name: id-resolution
  - name: name-sanitization

# Example 4: Module-based Terraform imports
transformers:
  - name: cleaning
  - name: id-resolution
  - name: name-sanitization
  - name: terraform-import
    target-format: 'module["{name}"].{resource_type}.this'
```

### Common Use Cases

| Configuration | Output | Use Case |
|---------------|--------|----------|
| All transformers (default) | Clean data, resolved IDs, sanitized names, Terraform imports | **Default** - Ready for Terraform import |
| Omit `terraform-import` | Clean data without Terraform files | Documentation generation |
| Only `id-resolution` | Raw Azure data with resolved IDs | Debugging, keeping all metadata |
| Custom `remove-keys` | Selective property filtering | Fine-tuned data export |


### Worker Count Optimization

The optimal number of workers depends on **which API** the resources use, not individual resource types.

#### **API-Based Recommendations**

| API Type | Resource Types | Recommended Workers | Rate Limits |
|----------|----------------|---------------------|-------------|
| **Microsoft Graph** | `Microsoft.Graph/*` | **3-5 workers** | Strict (~7 req/sec) |
| **Azure Resource Manager** | `Microsoft.Storage/*`<br>`Microsoft.Compute/*`<br>`Microsoft.Resources/*`<br>`Microsoft.Network/*`<br>etc. | **10-20 workers** | Generous (1000s/min) |

**Why this matters:**
- ✅ **5 workers + Graph API** = 33s for 54 resources (optimal!)
- ❌ **20 workers + Graph API** = 165s for 54 resources (5x slower!)
- ✅ **15 workers + ARM** = Fast downloads without rate limiting

**The tool will warn you if you use too many workers:**
```bash
./azure-rd download --type Microsoft.Graph/conditionalAccessPolicies --workers 20

WARN Worker count exceeds recommendation for this API
  workers=20
  api=Microsoft.Graph
  recommended_workers=5
  rate_limit=~2000 requests per 300 seconds (~6.67 req/sec)
  note=More workers can SLOW DOWN downloads due to rate limits
```

#### **Configuration Examples**

**Option 1: Automatic API-based defaults (RECOMMENDED)**
```yaml
# ~/.azure-rd.yaml
# Don't specify 'workers' - the tool automatically uses optimal counts:
#   - 5 workers for Microsoft Graph API
#   - 20 workers for Azure Resource Manager

log-level: "info"
output: "./azure-resources"
```

Then simply run:
```bash
# Automatically uses 5 workers (optimal for Graph API)
azure-rd download --type Microsoft.Graph/conditionalAccessPolicies

# Automatically uses 20 workers (optimal for ARM)
azure-rd download --type Microsoft.Storage/storageAccounts
```

**Option 2: Fine-tune per API (ADVANCED)**
```yaml
# ~/.azure-rd.yaml
workers-by-api:
  microsoft-graph: 3        # Custom: more conservative for Graph
  azure-resource-manager: 15  # Custom: moderate for ARM

log-level: "info"
```

**Option 3: Override all APIs globally**
```yaml
# ~/.azure-rd.yaml
workers: 10  # Use 10 workers for ALL APIs (overrides automatic detection)
```

**Option 4: CLI flag override (one-time)**
```bash
# Override for a specific command (highest priority)
azure-rd download --type Microsoft.Graph/conditionalAccessPolicies --workers 3
```

#### **Configuration Priority**

The tool uses this priority order (highest to lowest):

1. **`--workers` CLI flag** - Explicitly set for this command
2. **`workers-by-api`** config - API-specific settings in config file
3. **`workers`** config - General setting in config file
4. **Automatic defaults** - 5 for Graph, 20 for ARM (no config needed)

### Customizing Output for Different Use Cases

The tool allows you to customize which properties are included in the output YAML files:

#### Default Behavior

You can configure which keys to remove from the output using the cleaning transformer:
- `provisioningState` - Azure provisioning status
- `creationTime` - Resource creation timestamp
- `changedTime` - Last modification timestamp
- `correlationId` - Azure correlation ID
- `etag` - Entity tag for versioning
- `managedBy` - Management metadata
- `sku.tier` - Auto-derived SKU tier

#### For Terraform Imports

When generating resources for Terraform imports, you typically don't need the `id` property since Terraform will manage it. You can remove additional keys globally or per resource type:

**Global Exclusions** (apply to all resource types):
```bash
# Remove id and other Terraform-managed properties globally
azure-rd download \
  --type "Microsoft.Resources/resourceGroups" \
  --remove-keys "id,etag,provisioningState"
```

**Resource-Type-Specific Exclusions** (using config file):
```yaml
# Global exclusions (apply to all resources)
remove-keys:
  - etag
  - provisioningState
  - creationTime
  - changedTime

# Resource-type-specific exclusions
# These are merged with global exclusions
remove-keys-by-type:
  Microsoft.Resources/resourceGroups:
    - id
    - managedBy
  Microsoft.Storage/storageAccounts:
    - id
    - primaryEndpoints
    - secondaryEndpoints
  Microsoft.Compute/virtualMachines:
    - id
    - vmId
```

This allows you to fine-tune which properties are removed for each resource type while maintaining common removals globally.

**How It Works:**
- Global `remove-keys` apply to ALL resource types
- Type-specific keys in `remove-keys-by-type` are MERGED with global keys
- The final exclusion list for each resource type is: `global keys + type-specific keys`

**Example:** With the config above:
- Resource Groups will remove: `etag`, `provisioningState`, `id`, `managedBy`
- Storage Accounts will remove: `etag`, `provisioningState`, `id`, `primaryEndpoints`
- All other types will only remove: `etag`, `provisioningState`

#### For Documentation

If you want complete resource information for documentation purposes, you can remove fewer keys:

```bash
# Keep most properties
azure-rd download \
  --resource-group "my-rg" \
  --remove-keys "correlationId"
```

## 📂 Output Structure

The tool creates the following directory structure:

```
output/
├── Microsoft.Resources/
│   └── resourceGroups/
│       ├── my-resource-group.yaml
│       ├── another-resource-group.yaml
│       └── import.tf
├── Microsoft.Storage/
│   └── storageAccounts/
│       ├── mystorageaccount.yaml
│       └── import.tf
└── Microsoft.Compute/
    └── virtualMachines/
        ├── my_vm.yaml
        └── import.tf
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

### Terraform Import File

A single `import.tf` file per resource type containing all import blocks (Terraform 1.5+ format):

```hcl
# Terraform import statements
# Generated by azure-resource-downloader

# Import for my-rg
import {
  to = azurerm_resource_group.my_rg
  id = "/subscriptions/.../resourceGroups/my-rg"
}

# Import for another-rg
import {
  to = azurerm_resource_group.another_rg
  id = "/subscriptions/.../resourceGroups/another-rg"
}
```

#### Configurable Import Target Format

The `to` address in import blocks is configurable via the `--import-target-format` flag or `import-target-format` config option:

**Default format** (`{resource_type}.{name}`):
```hcl
import {
  to = azurerm_resource_group.my_rg
  id = "/subscriptions/.../resourceGroups/my-rg"
}
```

**Module format** (`module["{name}"].{resource_type}.this`):
```hcl
import {
  to = module["my_rg"].azurerm_resource_group.this
  id = "/subscriptions/.../resourceGroups/my-rg"
}
```

**Custom nested format** (`module.infrastructure.{resource_type}.{name}`):
```hcl
import {
  to = module.infrastructure.azurerm_resource_group.my_rg
  id = "/subscriptions/.../resourceGroups/my-rg"
}
```

Available template variables:
- `{resource_type}` - The Terraform resource type (e.g., `azurerm_resource_group`)
- `{name}` - The sanitized resource name (e.g., `my_rg`)

## 🎯 Supported Resource Types

Currently supported Azure resource types:

| Azure Resource Type | Terraform Resource Type | Handler |
|---------------------|-------------------------|---------|
| `Microsoft.Resources/resourceGroups` | `azurerm_resource_group` | ✅ |
| `Microsoft.Storage/storageAccounts` | `azurerm_storage_account` | ✅ |
| `Microsoft.Compute/virtualMachines` | `azurerm_virtual_machine` | ✅ |
| `Microsoft.Graph/conditionalAccessPolicies` | `azuread_conditional_access_policy` | ✅ |
| `Microsoft.Graph/authenticationStrengthPolicies` | `azuread_authentication_strength_policy` | ✅ |
| `Microsoft.Graph/deviceManagementConfigurationPolicies` | `microsoft365_graph_beta_device_management_settings_catalog_configuration_policy` | ✅ |
| `Microsoft.Graph/deviceConfigurations` | `microsoft365_graph_beta_device_management_device_configuration` | ✅ |

> **Note:** `Microsoft.Graph/deviceManagementConfigurationPolicies` (Intune Settings Catalog) uses the Microsoft Graph **beta** API and downloads the full settings tree via `$expand=settings`.
>
> **Note:** `Microsoft.Graph/deviceConfigurations` (legacy Intune device configuration profiles) uses the Microsoft Graph **beta** API and covers the polymorphic profile types, including Custom/OMA-URI profiles (`windows10CustomConfiguration`, `androidCustomConfiguration`, `iosCustomConfiguration`, `macOSCustomConfiguration`). This is distinct from the Settings Catalog endpoint above. Requires `DeviceManagementConfiguration.Read.All`. The Terraform resource type is polymorphic in practice; verify the emitted import against your provider/profile variant.

## 🔧 Adding New Resource Types

The tool is designed to be easily extensible. To add support for a new resource type:

### 1. Create a Handler

Create a new file in `internal/handlers/` (e.g., `keyvault.go`):

```go
package handlers

import (
    "context"
    "azure-resource-downloader/internal/models"
    // Import Azure SDK for the resource
)

type KeyVaultHandler struct {
    credential     *azidentity.DefaultAzureCredential
    subscriptionID string
}

func NewKeyVaultHandler(credential *azidentity.DefaultAzureCredential, subscriptionID string) *KeyVaultHandler {
    return &KeyVaultHandler{
        credential:     credential,
        subscriptionID: subscriptionID,
    }
}

func (h *KeyVaultHandler) GetType() string {
    return "Microsoft.KeyVault/vaults"
}

func (h *KeyVaultHandler) GetTerraformResourceType() string {
    return "azurerm_key_vault"
}

func (h *KeyVaultHandler) Fetch(ctx context.Context, resourceID string) (interface{}, error) {
    // Implement fetching logic using Azure SDK
}

func (h *KeyVaultHandler) Transform(resource interface{}) (*models.TransformedResource, error) {
    // Implement transformation logic
}
```

### 2. Register the Handler

Add the handler registration in `cmd/download.go`:

```go
func registerHandlers(registry *handlers.Registry, azureClient *azure.Client) {
    cred := azureClient.GetCredential()
    sub := azureClient.GetSubscriptionID()

    // Existing handlers
    registry.Register("Microsoft.Resources/resourceGroups", handlers.NewResourceGroupHandler(cred, sub))
    registry.Register("Microsoft.Storage/storageAccounts", handlers.NewStorageAccountHandler(cred, sub))
    
    // Add your new handler
    registry.Register("Microsoft.KeyVault/vaults", handlers.NewKeyVaultHandler(cred, sub))
}
```

### 3. Test

```bash
go build -o azure-rd
./azure-rd list  # Uses default subscription from az login
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
│   │   └── types.go
│   ├── pipeline/          # Pipeline implementation
│   │   ├── pipeline.go    # Orchestrator
│   │   ├── fetcher.go     # Fetch stage
│   │   ├── transformer.go # Transform stage
│   │   └── writer.go      # Write stage
│   ├── handlers/          # Resource handlers
│   │   ├── handler.go     # Registry
│   │   ├── resourcegroup.go
│   │   ├── storageaccount.go
│   │   └── virtualmachine.go
│   ├── azure/             # Azure client wrappers
│   │   ├── client.go
│   │   └── resolver.go    # ID to name resolver
│   └── transform/         # Transformation utilities
│       ├── cleaner.go     # YAML cleanup
│       ├── sanitizer.go   # Filename sanitizer
│       └── terraform.go   # Terraform generator
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

## 🗺️ Roadmap

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

**Terraform import transformer:**
```
DEBUG Generated Terraform import block resource_type=azurerm_resource_group target_address=azurerm_resource_group.my_rg
```

Debug logging shows exactly what each transformer does, which keys are removed/preserved/replaced, and why.

---

## 🔮 Roadmap

- [ ] Support for more Azure resource types
- [ ] Bulk download by subscription
- [ ] Resource filtering by tags
- [ ] Export to multiple formats (JSON, HCL)
- [ ] Interactive mode
- [ ] Unit and integration tests


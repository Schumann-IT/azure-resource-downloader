# Azure Resource Downloader

A powerful command-line tool that downloads Azure resources, transforms them into clean YAML format, and generates Terraform import statements. Built with Go and following the async pipeline pattern for maximum performance.

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

1. **Fetcher**: Retrieves resources from Azure using the Azure SDK
2. **Transformer**: 
   - Removes unnecessary properties (provisioningState, etag, etc.)
   - Resolves resource IDs to names
   - Sanitizes display names for filenames
   - Generates Terraform import statements
3. **Writer**: Saves resources as YAML files and Terraform import scripts

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

Some resources (like Conditional Access Policies) are accessed via Microsoft Graph API and require additional permissions:

**For Azure CLI users (`az login`):**
```bash
# Your user account needs the appropriate Azure AD role assignments
# Required roles for Conditional Access Policies:
# - Security Reader (read-only)
# - Security Administrator (read/write)
# - Global Administrator (full access)
```

**For Service Principal authentication:**
```bash
# Your service principal needs Microsoft Graph API permissions
# Required API permissions for Conditional Access Policies:
# - Policy.Read.All (read-only)
# - Policy.ReadWrite.ConditionalAccess (read/write)

# To grant permissions:
# 1. Register an app in Azure AD
# 2. Add Microsoft Graph API permissions
# 3. Grant admin consent for the permissions
# 4. Set environment variables:
export AZURE_TENANT_ID="your-tenant-id"
export AZURE_CLIENT_ID="your-client-id"
export AZURE_CLIENT_SECRET="your-client-secret"
```

**Note**: If you receive permission errors when listing Graph resources, contact your Azure AD administrator to grant the necessary permissions.

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
azure-rd download \
  --resource-group "my-rg" \
  --workers 10

# Control log verbosity
LOG_LEVEL=debug azure-rd download \
  --resource-group "my-rg"

# Exclude specific keys from output (e.g., for Terraform imports)
azure-rd download \
  --resource-group "my-rg" \
  --exclude-keys "id,etag,provisioningState"
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
- `AZURE_RD_EXCLUDE_KEYS` - Comma-separated list of keys to exclude from output
- `LOG_LEVEL` - Logging verbosity (debug, info, warn, error)

### Configuration File

Create `~/.azure-rd.yaml`:

```yaml
# All fields are optional
# subscription: "your-subscription-id"  # Optional - uses default from az login if not specified
output: "./azure-resources"
workers: 10

# Global exclusions (apply to all resource types)
# If not specified, default keys will be excluded (provisioningState, etag, etc.)
exclude-keys:
  - etag
  - provisioningState

# Resource-type-specific exclusions (merged with global)
# Useful for Terraform imports where different resources need different exclusions
exclude-keys-by-type:
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

### Customizing Output for Different Use Cases

The tool allows you to customize which properties are included in the output YAML files:

#### Default Behavior

By default, the following keys are automatically excluded from the output:
- `provisioningState` - Azure provisioning status
- `creationTime` - Resource creation timestamp
- `changedTime` - Last modification timestamp
- `correlationId` - Azure correlation ID
- `etag` - Entity tag for versioning
- `managedBy` - Management metadata
- `sku.tier` - Auto-derived SKU tier

#### For Terraform Imports

When generating resources for Terraform imports, you typically don't need the `id` property since Terraform will manage it. You can exclude additional keys globally or per resource type:

**Global Exclusions** (apply to all resource types):
```bash
# Exclude id and other Terraform-managed properties globally
azure-rd download \
  --type "Microsoft.Resources/resourceGroups" \
  --exclude-keys "id,etag,provisioningState"
```

**Resource-Type-Specific Exclusions** (using config file):
```yaml
# Global exclusions (apply to all resources)
exclude-keys:
  - etag
  - provisioningState
  - creationTime
  - changedTime

# Resource-type-specific exclusions
# These are merged with global exclusions
exclude-keys-by-type:
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

This allows you to fine-tune which properties are excluded for each resource type while maintaining common exclusions globally.

**How It Works:**
- Global `exclude-keys` apply to ALL resource types
- Type-specific keys in `exclude-keys-by-type` are MERGED with global keys
- The final exclusion list for each resource type is: `global keys + type-specific keys`

**Example:** With the config above:
- Resource Groups will exclude: `etag`, `provisioningState`, `id`, `managedBy`
- Storage Accounts will exclude: `etag`, `provisioningState`, `id`, `primaryEndpoints`
- All other types will only exclude: `etag`, `provisioningState`

#### For Documentation

If you want complete resource information for documentation purposes, you can exclude fewer keys:

```bash
# Keep most properties
azure-rd download \
  --resource-group "my-rg" \
  --exclude-keys "correlationId"
```

## 📂 Output Structure

The tool creates the following directory structure:

```
output/
├── Microsoft.Resources/
│   └── resourceGroups/
│       ├── my-resource-group.yaml
│       └── my-resource-group.tf
├── Microsoft.Storage/
│   └── storageAccounts/
│       ├── mystorageaccount.yaml
│       └── mystorageaccount.tf
└── Microsoft.Compute/
    └── virtualMachines/
        ├── my_vm.yaml
        └── my_vm.tf
```

### YAML File

Clean representation of the Azure resource:

```yaml
id: /subscriptions/.../resourceGroups/my-rg
name: my-rg
location: eastus
tags:
  environment: production
  owner: team-platform
```

### Terraform Import File

Ready-to-use Terraform import statement:

```hcl
# Terraform import statement for my-rg
# Generated by azure-resource-downloader

terraform import azurerm_resource_group.my_rg "/subscriptions/.../resourceGroups/my-rg"
```

## 🎯 Supported Resource Types

Currently supported Azure resource types:

| Azure Resource Type | Terraform Resource Type | Handler |
|---------------------|-------------------------|---------|
| `Microsoft.Resources/resourceGroups` | `azurerm_resource_group` | ✅ |
| `Microsoft.Storage/storageAccounts` | `azurerm_storage_account` | ✅ |
| `Microsoft.Compute/virtualMachines` | `azurerm_virtual_machine` | ✅ |
| `Microsoft.Graph/conditionalAccessPolicies` | `azuread_conditional_access_policy` | ✅ |

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

- [ ] Support for more Azure resource types
- [ ] Bulk download by subscription
- [ ] Resource filtering by tags
- [ ] Custom transformation rules
- [ ] Export to multiple formats (JSON, HCL)
- [ ] Interactive mode
- [ ] Progress bars and better output
- [ ] Unit and integration tests


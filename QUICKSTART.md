# Quick Start Guide

Get started with Azure Resource Downloader in 5 minutes!

## 1. Prerequisites

- Go 1.24+ installed
- Azure CLI or Azure credentials configured
- Azure subscription

## 2. Build

```bash
make build
# or
go build -o azure-rd
```

## 3. Authenticate with Azure

```bash
# Option 1: Azure CLI (easiest)
az login
az account set --subscription "your-subscription-id"

# Option 2: Environment variables
export AZURE_TENANT_ID="your-tenant-id"
export AZURE_CLIENT_ID="your-client-id"
export AZURE_CLIENT_SECRET="your-client-secret"
```

## 4. Get Your Subscription ID

```bash
# If using Azure CLI
az account show --query id -o tsv
```

## 5. Run Your First Download

```bash
# List supported resource types
./azure-rd list --subscription "YOUR_SUBSCRIPTION_ID"

# Download a resource group
./azure-rd download \
  --subscription "YOUR_SUBSCRIPTION_ID" \
  --resource-group "YOUR_RESOURCE_GROUP_NAME"

# Check the output
ls -la output/
```

## Example Output

After running the download command, you'll get:

```
output/
└── Microsoft.Resources/
    └── resourceGroups/
        ├── my-resource-group.yaml
        └── my-resource-group.tf
```

**YAML file** (`my-resource-group.yaml`):
```yaml
id: /subscriptions/.../resourceGroups/my-resource-group
name: my-resource-group
location: eastus
tags:
  environment: production
```

**Terraform file** (`my-resource-group.tf`):
```hcl
terraform import azurerm_resource_group.my_resource_group "/subscriptions/.../resourceGroups/my-resource-group"
```

## 6. Use with Terraform

```bash
# Copy the import statement from the .tf file
cd output/Microsoft.Resources/resourceGroups/

# Initialize Terraform (if needed)
terraform init

# Run the import
terraform import azurerm_resource_group.my_resource_group "/subscriptions/.../resourceGroups/my-resource-group"

# Verify
terraform plan
```

## Common Commands

```bash
# Download multiple resources
./azure-rd download \
  --subscription "SUB_ID" \
  --resource-id "/subscriptions/.../resourceGroups/rg1" \
  --resource-id "/subscriptions/.../resourceGroups/rg2"

# Dry run (preview without writing)
./azure-rd download \
  --subscription "SUB_ID" \
  --resource-group "my-rg" \
  --dry-run

# Custom output directory
./azure-rd download \
  --subscription "SUB_ID" \
  --resource-group "my-rg" \
  --output "./my-resources"

# Use more workers for faster processing
./azure-rd download \
  --subscription "SUB_ID" \
  --resource-group "my-rg" \
  --workers 10
```

## Configuration File (Optional)

Create `~/.azure-rd.yaml`:

```yaml
subscription: "your-subscription-id"
output: "./azure-resources"
workers: 10
```

Then simply run:

```bash
./azure-rd download --resource-group "my-rg"
```

## Troubleshooting

### Authentication Error

```
Error: failed to create Azure client: authentication failed
```

**Solution**: Run `az login` or set environment variables

### No Handler for Resource Type

```
Error: no handler registered for resource type: Microsoft.Network/virtualNetworks
```

**Solution**: This resource type is not yet supported. See README.md for instructions on adding new resource types.

### Permission Denied

```
Error: failed to get resource: authorization failed
```

**Solution**: Ensure your Azure account has Reader permissions on the subscription/resource group.

## Next Steps

- Read the full [README.md](README.md) for detailed documentation
- Add support for more resource types (see README.md)
- Integrate with your Terraform workflow
- Set up CI/CD automation

## Need Help?

- Check the [README.md](README.md) for detailed documentation
- Review existing issues on GitHub
- Open a new issue with details about your problem

---

Happy downloading! 🚀


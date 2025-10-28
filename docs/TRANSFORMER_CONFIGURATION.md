# Transformer Configuration - Per-Transformer Settings

Each transformer can be independently configured with its own settings, providing a modular and flexible approach to customizing the transformation pipeline.

## Overview

Instead of global configuration, each transformer is configured separately:

```yaml
transformers:
  - name: cleaning
    exclude-keys:
      - provisioningState
      - etag
    exclude-keys-by-type:
      Microsoft.Resources/resourceGroups:
        - id
  - name: id-resolution
  - name: name-sanitization  
  - name: terraform-import
    target-format: "{resource_type}.{name}"
```

## Available Transformers

### 1. **cleaning** - Property Cleaning

Removes Azure metadata and applies custom exclusions.

**Configuration Options:**

- `exclude-keys` (list) - Global keys to exclude from all resources
- `exclude-keys-by-type` (map) - Resource-type-specific exclusions
- `clean-empty` (boolean) - If true, remove keys with empty values (null, [], "", {})

📖 **See [CLEANING_TRANSFORMER.md](./CLEANING_TRANSFORMER.md) for complete reference.**

**Example:**

```yaml
transformers:
  - name: cleaning
    exclude-keys:
      - provisioningState
      - etag
      - systemData
      - createdTime
      - changedTime
    exclude-keys-by-type:
      Microsoft.Resources/resourceGroups:
        - id
        - managedBy
      Microsoft.Storage/storageAccounts:
        - primaryEndpoints
        - secondaryEndpoints
        - creationTime
      Microsoft.Compute/virtualMachines:
        - vmId
        - hardwareProfile
    clean-empty: true    # Remove empty values (null, [], "", {})
```

**Notes:**
- Keys are case-sensitive
- Resource types are normalized to lowercase for matching
- Type-specific exclusions are applied in addition to global ones

---

### 2. **id-resolution** - Resource ID Resolution

Converts Azure resource ID paths to friendly names for readability.

**Configuration Options:**

None - this transformer works automatically.

**Example:**

```yaml
transformers:
  - name: id-resolution
```

**What it does:**

Before:
```yaml
virtualNetworkId: /subscriptions/XXX/resourceGroups/my-rg/providers/Microsoft.Network/virtualNetworks/my-vnet
```

After:
```yaml
virtualNetworkId: my-vnet
```

---

### 3. **name-sanitization** - Name Sanitization

Sanitizes resource names for file system and Terraform compatibility.

**Configuration Options:**

None - uses built-in sanitization rules.

**Example:**

```yaml
transformers:
  - name: name-sanitization
```

**Sanitization Rules:**
- Lowercase conversion
- Spaces → underscores
- Remove special characters: `@!#$%^&*()`
- Remove leading/trailing underscores

Before: `My-Resource@123!`  
After: `my_resource_123`

---

### 4. **terraform-import** - Terraform Import Generation

Generates Terraform import blocks with configurable target format.

**Configuration Options:**

- `target-format` (string) - Template for the import 'to' address

**Template Variables:**
- `{resource_type}` - Terraform resource type (e.g., `azurerm_resource_group`)
- `{name}` - Sanitized resource name (e.g., `my_rg`)

**Examples:**

**Default format:**
```yaml
transformers:
  - name: terraform-import
    target-format: "{resource_type}.{name}"
```

Output:
```hcl
import {
  to = azurerm_resource_group.my_rg
  id = "/subscriptions/.../resourceGroups/my-rg"
}
```

**Module format:**
```yaml
transformers:
  - name: terraform-import
    target-format: 'module["{name}"].{resource_type}.this'
```

Output:
```hcl
import {
  to = module["my_rg"].azurerm_resource_group.this
  id = "/subscriptions/.../resourceGroups/my-rg"
}
```

**Nested module format:**
```yaml
transformers:
  - name: terraform-import
    target-format: "module.infrastructure.{resource_type}.{name}"
```

Output:
```hcl
import {
  to = module.infrastructure.azurerm_resource_group.my_rg
  id = "/subscriptions/.../resourceGroups/my-rg"
}
```

---

## Configuration Examples

### Example 1: Default Configuration

```yaml
# If not specified, all transformers use default settings
# This is equivalent to:
transformers:
  - name: cleaning
  - name: id-resolution
  - name: name-sanitization
  - name: terraform-import
```

### Example 2: Custom Cleaning Rules

```yaml
transformers:
  - name: cleaning
    exclude-keys:
      - provisioningState
      - etag
      - systemData
    exclude-keys-by-type:
      Microsoft.Storage/storageAccounts:
        - primaryEndpoints
        - secondaryEndpoints
        - networkRuleSet
  - name: id-resolution
  - name: name-sanitization
  - name: terraform-import
```

### Example 3: Documentation Only (No Terraform)

```yaml
transformers:
  - name: cleaning
  - name: id-resolution
  - name: name-sanitization
  # terraform-import omitted - no import files generated
```

### Example 4: Raw Data with ID Resolution

```yaml
transformers:
  - name: id-resolution
  # Only resolve IDs, keep all other data as-is
```

### Example 5: Module-Based Terraform Imports

```yaml
transformers:
  - name: cleaning
    exclude-keys:
      - etag
      - systemData
  - name: id-resolution
  - name: name-sanitization
  - name: terraform-import
    target-format: 'module["{name}"].{resource_type}.this'
```

### Example 6: Minimal Cleaning

```yaml
transformers:
  - name: cleaning
    exclude-keys:
      - etag  # Only exclude etag, keep everything else
  - name: id-resolution
  - name: name-sanitization
  - name: terraform-import
```

### Example 7: Simple List Format

For transformers without custom configuration:

```yaml
transformers:
  - cleaning
  - id-resolution
  - name-sanitization
  - terraform-import
```

This is equivalent to:

```yaml
transformers:
  - name: cleaning
  - name: id-resolution
  - name: name-sanitization
  - name: terraform-import
```

---

## Per-Transformer vs Global Configuration

### Old Approach (Global)

```yaml
exclude-keys:
  - etag
  - provisioningState

exclude-keys-by-type:
  Microsoft.Resources/resourceGroups:
    - id

import-target-format: "{resource_type}.{name}"
```

### New Approach (Per-Transformer)

```yaml
transformers:
  - name: cleaning
    exclude-keys:
      - etag
      - provisioningState
    exclude-keys-by-type:
      Microsoft.Resources/resourceGroups:
        - id
  - name: id-resolution
  - name: name-sanitization
  - name: terraform-import
    target-format: "{resource_type}.{name}"
```

### Benefits

✅ **Modular** - Each transformer is self-contained  
✅ **Flexible** - Easy to add/remove/configure transformers independently  
✅ **Clear** - Configuration is co-located with the transformer  
✅ **Extensible** - New transformers can have their own config schemas  

---

## Default Settings

### Cleaning Transformer

If no `exclude-keys` are specified, the cleaning transformer uses sensible defaults:

- No keys excluded by default
- Define your own exclusions based on your needs

### Terraform Import Transformer

Default `target-format`: `"{resource_type}.{name}"`

Generates standard Terraform resource addresses like:
- `azurerm_resource_group.my_rg`
- `azurerm_storage_account.mystorageaccount`

---

## Use Cases

### Use Case 1: Terraform Import Workflow

```yaml
transformers:
  - name: cleaning
    exclude-keys:
      - provisioningState
      - etag
      - systemData
  - name: id-resolution
  - name: name-sanitization
  - name: terraform-import
```

**Result:** Clean, readable YAML + Terraform import files

---

### Use Case 2: Documentation Generation

```yaml
transformers:
  - name: cleaning
    exclude-keys:
      - etag
      - systemData
  - name: id-resolution
  - name: name-sanitization
```

**Result:** Clean YAML files without Terraform artifacts

---

### Use Case 3: Debugging Azure Resources

```yaml
transformers:
  - name: id-resolution
```

**Result:** Raw Azure data with resolved IDs for readability

---

### Use Case 4: Module-Based Terraform Project

```yaml
transformers:
  - name: cleaning
  - name: id-resolution
  - name: name-sanitization
  - name: terraform-import
    target-format: 'module["{name}"].{resource_type}.this'
```

**Result:** Terraform imports targeting modules

---

### Use Case 5: Audit/Inventory

```yaml
transformers:
  - name: cleaning
    # Don't exclude anything - keep all metadata
    exclude-keys: []
  - name: id-resolution
```

**Result:** Complete Azure data for audit purposes

---

## Best Practices

1. **Start with defaults** - Only customize when needed
2. **Use type-specific exclusions** - Different resources need different filtering
3. **Keep ID resolution** - Makes output much more readable
4. **Use name sanitization with Terraform** - Prevents invalid identifiers
5. **Document your exclusions** - Add comments explaining why keys are excluded

---

## Configuration File Location

Configuration is read from:
1. `--config` flag (highest priority)
2. `~/.azure-rd.yaml`
3. `./.azure-rd.yaml` (current directory)

---

## Summary

| Transformer | Configuration | Purpose |
|-------------|---------------|---------|
| **cleaning** | `exclude-keys`, `exclude-keys-by-type` | Remove unwanted properties |
| **id-resolution** | None | Convert IDs to names |
| **name-sanitization** | None | Make names file/Terraform-safe |
| **terraform-import** | `target-format` | Generate import blocks |

Each transformer is independently configurable, giving you complete control over the transformation pipeline! 🎯


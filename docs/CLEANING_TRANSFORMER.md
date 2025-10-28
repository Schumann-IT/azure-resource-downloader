# Cleaning Transformer - Configuration Reference

The `cleaning` transformer removes Azure metadata and unwanted properties from resource data.

## Configuration Options

### `exclude-keys` (list of strings)

Global keys to exclude from all resources.

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
```

**Default behavior:** If no keys are specified, the transformer uses built-in defaults:
- `provisioningState`
- `creationTime`
- `changedTime`
- `correlationId`
- `etag`
- `managedBy`
- `sku.tier`

---

### `exclude-keys-by-type` (map)

Resource-type-specific exclusions. These are applied in addition to global exclusions.

**Format:**
```yaml
transformers:
  - name: cleaning
    exclude-keys-by-type:
      <ResourceType>:
        - <key1>
        - <key2>
```

**Example:**
```yaml
transformers:
  - name: cleaning
    exclude-keys:
      - etag
      - provisioningState
    exclude-keys-by-type:
      Microsoft.Resources/resourceGroups:
        - id
        - managedBy
      Microsoft.Storage/storageAccounts:
        - primaryEndpoints
        - secondaryEndpoints
        - networkRuleSet
      Microsoft.Compute/virtualMachines:
        - vmId
        - hardwareProfile
```

**Notes:**
- Resource type keys are case-insensitive (normalized to lowercase)
- Type-specific keys are merged with global keys
- Use nested paths with dots: `sku.tier`, `properties.subnet.id`

---

### `clean-empty` (boolean)

If `true`, removes keys with empty values including:
- `null`
- Empty strings: `""`
- Empty arrays: `[]`
- Empty maps: `{}`

**Default:** `true` (empty values are removed - maintains backward compatibility)

**Example:**
```yaml
transformers:
  - name: cleaning
    clean-empty: true
```

**Effect:**

With `clean-empty: false` (disabling the default):
```yaml
name: my-resource
location: eastus
tags: {}                    # Empty map kept
networkRules: []            # Empty array kept
description: ""             # Empty string kept
optionalField: null         # Null kept
```

With `clean-empty: true` (default behavior):
```yaml
name: my-resource
location: eastus
# All empty values removed
```

**When to use:**
- ✅ **Keep enabled (default)** for cleaner, more readable output
- ✅ **Keep enabled (default)** to reduce YAML file size
- ✅ **Keep enabled (default)** for Terraform import (cleaner resource definitions)
- ❌ **Set to false** if you need to distinguish between "not set" and "explicitly empty"
- ❌ **Set to false** for complete data preservation including empty structures

---

## Complete Examples

### Example 1: Default Cleaning

```yaml
transformers:
  - name: cleaning
    # Uses default exclude keys and clean-empty: true (default)
```

Output:
```yaml
name: my-rg
location: eastus
properties:
  someValue: test
  # Empty values removed by default
```

---

### Example 2: Aggressive Cleaning

```yaml
transformers:
  - name: cleaning
    exclude-keys:
      - provisioningState
      - etag
      - systemData
      - createdTime
      - changedTime
      - id
    clean-empty: true   # Remove all empty values
```

Output:
```yaml
name: my-rg
location: eastus
properties:
  someValue: test
  # All empty values removed
```

---

### Example 3: Keep Empty Values

```yaml
transformers:
  - name: cleaning
    exclude-keys:
      - etag           # Only remove etag
    clean-empty: false # Explicitly keep empty values
```

Output:
```yaml
id: /subscriptions/.../resourceGroups/my-rg
name: my-rg
location: eastus
provisioningState: Succeeded   # Kept
tags: {}                       # Kept
systemData:                    # Kept
  createdBy: user@example.com
```

---

### Example 4: Type-Specific Cleaning

```yaml
transformers:
  - name: cleaning
    exclude-keys:
      - etag
      - provisioningState
    exclude-keys-by-type:
      Microsoft.Resources/resourceGroups:
        - id
        - managedBy
      Microsoft.Storage/storageAccounts:
        - id
        - primaryEndpoints
        - secondaryEndpoints
        - creationTime
      Microsoft.Compute/virtualMachines:
        - vmId
        - instanceView
    clean-empty: true
```

**For Resource Groups:**
- Removes: `etag`, `provisioningState`, `id`, `managedBy`
- Removes empty values

**For Storage Accounts:**
- Removes: `etag`, `provisioningState`, `id`, `primaryEndpoints`, `secondaryEndpoints`, `creationTime`
- Removes empty values

**For Virtual Machines:**
- Removes: `etag`, `provisioningState`, `vmId`, `instanceView`
- Removes empty values

---

### Example 5: No Cleaning

```yaml
transformers:
  - name: id-resolution
  - name: name-sanitization
  # cleaning omitted - no cleaning applied
```

Output: Complete raw Azure data (except what the handler transforms)

---

## Nested Key Exclusions

Use dot notation to exclude nested properties:

```yaml
transformers:
  - name: cleaning
    exclude-keys:
      - sku.tier                    # Remove sku.tier
      - properties.subnet.id        # Remove nested property
      - networkRuleSet.defaultAction
```

Before:
```yaml
sku:
  name: Standard
  tier: Standard              # Will be removed
properties:
  subnet:
    id: /subscriptions/...    # Will be removed
    name: my-subnet           # Kept
```

After:
```yaml
sku:
  name: Standard
properties:
  subnet:
    name: my-subnet
```

---

## Empty Value Behavior

The `clean-empty` flag recursively removes empty values:

### Example: Nested Empty Structures

Input:
```yaml
name: my-resource
properties:
  validValue: test
  emptyString: ""
  emptyMap: {}
  items:
    - name: item1
    - {}                    # Empty map in array
    - name: item2
  nestedEmpty:
    level1:
      level2: {}
```

With `clean-empty: true`:
```yaml
name: my-resource
properties:
  validValue: test
  items:
    - name: item1
    - name: item2
  # All empty structures removed
```

With `clean-empty: false`:
```yaml
name: my-resource
properties:
  validValue: test
  emptyString: ""
  emptyMap: {}
  items:
    - name: item1
    - {}
    - name: item2
  nestedEmpty:
    level1:
      level2: {}
```

---

## Best Practices

1. **Start with defaults** - The built-in exclude keys handle common Azure metadata
2. **Add type-specific exclusions** - Different resource types have different unnecessary properties
3. **Enable `clean-empty` for Terraform** - Creates cleaner import-ready resources
4. **Disable `clean-empty` for debugging** - Keeps all data structure information
5. **Use nested paths sparingly** - Only when specific nested properties need removal

---

## Common Exclusion Patterns

### Resource Groups
```yaml
exclude-keys-by-type:
  Microsoft.Resources/resourceGroups:
    - id
    - managedBy
```

### Storage Accounts
```yaml
exclude-keys-by-type:
  Microsoft.Storage/storageAccounts:
    - id
    - primaryEndpoints
    - secondaryEndpoints
    - creationTime
    - statusOfPrimary
    - statusOfSecondary
```

### Virtual Machines
```yaml
exclude-keys-by-type:
  Microsoft.Compute/virtualMachines:
    - id
    - vmId
    - instanceView
    - provisioningState
```

### Virtual Networks
```yaml
exclude-keys-by-type:
  Microsoft.Network/virtualNetworks:
    - id
    - resourceGuid
    - subnets.*.id
```

---

## Summary

| Option | Type | Default | Purpose |
|--------|------|---------|---------|
| `exclude-keys` | list | Built-in defaults | Remove specific keys globally |
| `exclude-keys-by-type` | map | (none) | Remove keys for specific resource types |
| `clean-empty` | boolean | `true` | Remove empty values (null, [], "", {}) |

The cleaning transformer provides fine-grained control over which properties to include in the output! 🧹


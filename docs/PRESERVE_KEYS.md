# Preserve Keys - Fine-Grained Control

The `preserve-keys` option in the cleaning transformer allows you to make exceptions to the `remove-keys` rules.

## Problem It Solves

Sometimes you want to remove a key globally (like `id`) but keep it in specific locations:

**Without preserve-keys:**
```yaml
remove-keys:
  - id    # Removes ALL id keys everywhere
```

This removes:
- ✅ Root `id`
- ❌ `properties.subnet.id` (you might need this!)
- ❌ `metadata.id` (you might need this too!)

**With preserve-keys:**
```yaml
remove-keys:
  - id                      # Remove "id" everywhere
preserve-keys:
  - properties.subnet.id    # EXCEPT keep this one
  - metadata.id             # And this one
```

This removes:
- ✅ Root `id` 
- ✅ Other `id` keys
- ❌ `properties.subnet.id` (PRESERVED)
- ❌ `metadata.id` (PRESERVED)

---

## Configuration

### Basic Example

```yaml
transformers:
  - name: cleaning
    remove-keys:
      - id
      - etag
      - systemData
    preserve-keys:
      - properties.subnet.id
```

**Result:**
- Removes `id`, `etag`, `systemData` everywhere
- EXCEPT keeps `properties.subnet.id`

---

## How It Works

### Precedence Rule

**`preserve-keys` takes precedence over `remove-keys`**

1. First, all `remove-keys` are identified for removal
2. Then, `preserve-keys` are checked
3. If a key matches a preserve path, it's NOT removed

### Path Matching

**Exact path matching:**
- `preserve-keys: ["foo.id"]` preserves only `foo.id`
- Does NOT preserve root `id` or `bar.id`

**Paths use dot notation:**
- `properties.subnet.id` = `properties` → `subnet` → `id`
- Must match the exact nested structure

---

## Examples

### Example 1: Remove ID Everywhere Except Specific Locations

```yaml
transformers:
  - name: cleaning
    remove-keys:
      - id
    preserve-keys:
      - properties.subnet.id
      - networkProfile.id
```

**Input:**
```yaml
id: /subscriptions/.../my-rg              # Removed
name: my-resource
properties:
  subnet:
    id: /subscriptions/.../my-subnet      # PRESERVED
    name: my-subnet
networkProfile:
  id: /subscriptions/.../profile-123      # PRESERVED
metadata:
  id: abc-123                             # Removed
```

**Output:**
```yaml
name: my-resource
properties:
  subnet:
    id: /subscriptions/.../my-subnet      # KEPT!
    name: my-subnet
networkProfile:
  id: /subscriptions/.../profile-123      # KEPT!
```

---

### Example 2: Remove System Metadata But Keep User Metadata

```yaml
transformers:
  - name: cleaning
    remove-keys:
      - systemData
      - metadata
    preserve-keys:
      - metadata.userDefined
      - properties.metadata
```

**Input:**
```yaml
name: my-resource
systemData:
  createdBy: system@azure.com             # Removed
metadata:
  systemGenerated: true                   # Removed
  userDefined:                            # PRESERVED
    owner: john@example.com
properties:
  metadata:                               # PRESERVED
    description: "Important metadata"
```

**Output:**
```yaml
name: my-resource
metadata:
  userDefined:                            # KEPT!
    owner: john@example.com
properties:
  metadata:                               # KEPT!
    description: "Important metadata"
```

---

### Example 3: Complex Nested Preservation

```yaml
transformers:
  - name: cleaning
    remove-keys:
      - id
      - etag
      - provisioningState
    preserve-keys:
      - virtualNetwork.id
      - virtualNetwork.subnets.id          # Note: Won't work for array items
      - properties.primaryNetworkInterface.id
```

---

## Debug Logging

When using `--log-level debug`, you'll see which keys were preserved:

```
DEBUG Removed excluded keys keys_removed=[id etag provisioningState metadata.id] count=4
DEBUG Preserved keys (excluded from removal) keys_preserved=[properties.subnet.id networkProfile.id] count=2
```

This shows:
- 4 keys were removed (including `id` and `metadata.id`)
- 2 keys were preserved (even though they match removal patterns)

---

## Limitations

### Array Items

Preserve keys work with named paths but NOT array indices:

**Works:**
```yaml
preserve-keys:
  - subnets.id    # Preserves id in ALL items in subnets array
```

**Doesn't work (yet):**
```yaml
preserve-keys:
  - subnets[0].id  # Array index syntax not supported
```

---

## Common Patterns

### Pattern 1: Remove Root ID, Keep Nested IDs

```yaml
remove-keys:
  - id
preserve-keys:
  - properties.subnet.id
  - properties.virtualNetwork.id
  - networkProfile.networkInterfaces.id
```

### Pattern 2: Remove All Metadata Except Specific Fields

```yaml
remove-keys:
  - metadata
  - systemData
preserve-keys:
  - metadata.owner
  - metadata.cost-center
  - properties.metadata
```

### Pattern 3: Clean Azure Metadata, Keep Application Metadata

```yaml
remove-keys:
  - provisioningState
  - etag
  - systemData
  - tags              # Remove Azure tags
preserve-keys:
  - properties.tags   # But keep application-specific tags
```

---

## Backward Compatibility

The old names still work:

**Old style (still works):**
```yaml
transformers:
  - name: cleaning
    exclude-keys:
      - id
    exclude-keys-by-type:
      Microsoft.Resources/resourceGroups:
        - managedBy
```

**New style (preferred):**
```yaml
transformers:
  - name: cleaning
    remove-keys:
      - id
    remove-keys-by-type:
      Microsoft.Resources/resourceGroups:
        - managedBy
    preserve-keys:
      - properties.subnet.id
```

---

## Summary

| Option | Purpose | Example |
|--------|---------|---------|
| `remove-keys` | Remove keys globally (recursive) | `["id", "etag"]` |
| `remove-keys-by-type` | Type-specific removals | `Microsoft.Storage/*: ["primaryEndpoints"]` |
| `preserve-keys` | Exceptions to removal | `["properties.subnet.id"]` |
| `clean-empty` | Remove empty values | `true` (default) |

**Key insight:** Use `remove-keys` for broad patterns, `preserve-keys` for exceptions! 🎯


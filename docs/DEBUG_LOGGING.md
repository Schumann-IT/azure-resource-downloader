# Debug Logging Guide

All transformers now provide detailed debug logging when running with `--log-level debug`.

## Enable Debug Logging

```bash
azure-rd download --type "Microsoft.Resources/resourceGroups" --log-level debug
```

## Transformer Debug Output

### 1. Cleaning Transformer

**Shows which keys were removed:**

```
DEBUG Removed excluded keys keys_removed=[provisioningState etag systemData creationTime] count=4
DEBUG Removed empty values keys_removed=[conditions.applications.excludeApplications conditions.applications.includeUserActions conditions.users.excludeGroups conditions.users.excludeRoles conditions.users.includeGroups conditions.users.includeRoles grantControls.customAuthenticationFactors grantControls.termsOfUse] count=8
```

**Information provided:**
- `keys_removed` - List of all keys that were removed (includes nested paths)
- `count` - Total number of keys removed
- Separate logging for excluded keys vs empty values

**Key paths show nesting:**
- `conditions.applications.excludeApplications` - Nested structure
- `grantControls.termsOfUse` - Shows exact location

---

### 2. ID Resolution Transformer

**Shows which resource IDs were resolved:**

```
DEBUG Resolved resource IDs to names ids_resolved=[virtualNetworkId subnet.id properties.networkProfile.networkInterfaces[0]] count=3
```

**Information provided:**
- `ids_resolved` - List of property paths where IDs were found and resolved
- `count` - Number of IDs resolved
- Shows nested paths and array indices

**Example transformations:**
```yaml
# Before
virtualNetworkId: /subscriptions/XXX/resourceGroups/my-rg/providers/Microsoft.Network/virtualNetworks/my-vnet

# After (adds _name field)
virtualNetworkId: /subscriptions/XXX/resourceGroups/my-rg/providers/Microsoft.Network/virtualNetworks/my-vnet
virtualNetworkId_name: my-vnet
```

---

### 3. Name Sanitization Transformer

**Shows name transformations:**

```
DEBUG Sanitized name original="My-Resource@Group!" sanitized=my_resource_group
DEBUG Sanitized name original="VM-123" sanitized=vm_123
DEBUG Sanitized name original="Storage Account 2024" sanitized=storage_account_2024
```

**Information provided:**
- `original` - Original Azure resource name
- `sanitized` - Sanitized name for files/Terraform

**Only logs when name changed** - If original equals sanitized, no log (reduces noise).

**Transformations shown:**
- Spaces → underscores
- Special characters removed
- Lowercase conversion
- Leading numbers handled

---

### 4. Terraform Import Transformer

**Shows import block generation:**

```
DEBUG Generated Terraform import block resource_type=azurerm_resource_group resource_name="my-rg" sanitized_name=my_rg target_address=azurerm_resource_group.my_rg target_format={resource_type}.{name}
DEBUG Generated Terraform import block resource_type=azurerm_storage_account resource_name="myStorage" sanitized_name=mystorage target_address=module["mystorage"].azurerm_storage_account.this target_format=module["{name}"].{resource_type}.this
```

**Information provided:**
- `resource_type` - Terraform resource type
- `resource_name` - Original resource name
- `sanitized_name` - Sanitized version used in Terraform
- `target_address` - Final import target address
- `target_format` - Template used

---

## Complete Debug Output Example

### Full Resource Transformation

```
DEBUG Transforming resource resource_id=/subscriptions/.../resourceGroups/my-rg type=Microsoft.Resources/resourceGroups

DEBUG Removed excluded keys keys_removed=[provisioningState etag managedBy] count=3
DEBUG Removed empty values keys_removed=[tags properties.permissions] count=2

DEBUG Sanitized name original="my-rg" sanitized=my_rg

DEBUG Generated Terraform import block resource_type=azurerm_resource_group resource_name=my-rg sanitized_name=my_rg target_address=azurerm_resource_group.my_rg target_format={resource_type}.{name}

DEBUG Resource transformed successfully resource_id=/subscriptions/.../resourceGroups/my-rg name=my-rg sanitized_name=my_rg transformers="cleaning, id-resolution, name-sanitization, terraform-import"
```

### When Transformers Are Skipped

```
DEBUG Transforming resource resource_id=/subscriptions/.../resourceGroups/my-rg type=Microsoft.Resources/resourceGroups

DEBUG Skipping cleaning transformer (not configured)

DEBUG Resolved resource IDs to names ids_resolved=[parentResourceId] count=1

DEBUG Skipping name-sanitization transformer (not configured) using_original_name=my-rg

DEBUG Skipping terraform-import transformer (not configured)

DEBUG Resource transformed successfully resource_id=/subscriptions/.../resourceGroups/my-rg name=my-rg sanitized_name=my-rg transformers="id-resolution"
```

---

## Debugging Scenarios

### Scenario 1: Why was this key removed?

**Question:** "Why is `provisioningState` missing from my output?"

**Answer:** Run with debug logging:
```bash
azure-rd download --resource-id "..." --log-level debug
```

Look for:
```
DEBUG Removed excluded keys keys_removed=[...provisioningState...] count=N
```

This shows it was explicitly excluded by the cleaning transformer.

---

### Scenario 2: Why are there no empty arrays?

**Question:** "Where did all the empty arrays go?"

**Answer:** Debug output shows:
```
DEBUG Removed empty values keys_removed=[conditions.applications.excludeApplications ...] count=8
```

The `clean-empty: true` flag removed them.

---

### Scenario 3: Why did the resource name change?

**Question:** "Why is my file called `my_resource_group` instead of `My-Resource-Group`?"

**Answer:** Debug output shows:
```
DEBUG Sanitized name original="My-Resource-Group" sanitized=my_resource_group
```

The name-sanitization transformer made it file/Terraform compatible.

---

### Scenario 4: Which IDs were resolved?

**Question:** "Did my virtual network ID get resolved?"

**Answer:** Debug output shows:
```
DEBUG Resolved resource IDs to names ids_resolved=[virtualNetworkId subnet.id] count=2
```

Yes, both `virtualNetworkId` and `subnet.id` were resolved.

---

## Debug Log Levels

### DEBUG Level (Most Verbose)

- Every transformer operation
- Every key removed
- Every ID resolved
- Every name sanitized
- Every import generated

**Use for:**
- Troubleshooting
- Understanding transformations
- Verifying configuration

### INFO Level (Default)

- Pipeline progress
- Performance metrics
- Transformer configuration summary
- Consolidated file generation

**Use for:**
- Normal operation
- Production runs

### WARN Level

- Retry attempts
- Configuration warnings
- Missing handlers

### ERROR Level

- Failures only

---

## Example Debug Session

```bash
$ azure-rd download --resource-id "/subscriptions/.../resourceGroups/my-rg" --log-level debug

INFO Cleaning transformer config exclude_keys=[etag systemData] exclude_keys_by_type_count=1 clean_empty=true
INFO Starting pipeline resources=1 workers=5
DEBUG Fetching resource resource_id=/subscriptions/.../resourceGroups/my-rg type=Microsoft.Resources/resourceGroups
DEBUG Resource fetched successfully resource_id=/subscriptions/.../resourceGroups/my-rg
DEBUG Transforming resource resource_id=/subscriptions/.../resourceGroups/my-rg
DEBUG Removed excluded keys keys_removed=[etag systemData provisioningState] count=3
DEBUG Removed empty values keys_removed=[tags] count=1
DEBUG Sanitized name original="my-rg" sanitized=my_rg
DEBUG Generated Terraform import block resource_type=azurerm_resource_group target_address=azurerm_resource_group.my_rg
DEBUG Resource transformed successfully
DEBUG Writing resource files resource_id=/subscriptions/.../resourceGroups/my-rg
DEBUG Resource files written successfully yaml_path=output/Microsoft.Resources/resourceGroups/my_rg.yaml
INFO Progress completed=1 total=1 percentage=100.0%
INFO Files written count=1
```

---

## Summary

| Transformer | Debug Output | Shows |
|-------------|--------------|-------|
| **cleaning** | Removed excluded keys, Removed empty values | Which keys were removed and why |
| **id-resolution** | Resolved resource IDs to names | Which IDs were found and resolved |
| **name-sanitization** | Sanitized name | Original → sanitized transformation |
| **terraform-import** | Generated Terraform import block | Import details and format used |

All transformers provide detailed, actionable debug information! 🔍


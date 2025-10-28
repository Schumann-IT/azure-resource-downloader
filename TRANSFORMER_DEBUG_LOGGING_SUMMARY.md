# Transformer Debug Logging - Implementation Summary

## Overview

All transformers now provide detailed debug logging showing exactly what operations they perform on each resource.

## Implementation

### 1. Cleaning Transformer (`internal/transform/cleaner.go`)

**Added functions:**
- `findAndRemoveKeys()` - Returns list of removed keys
- `removeEmptyValuesWithTracking()` - Returns list of removed empty value paths  
- `removeEmptyValuesWithPath()` - Recursively removes and tracks paths

**Debug output:**
```go
if len(removedKeys) > 0 {
    log.Debug("Removed excluded keys",
        "keys_removed", removedKeys,
        "count", len(removedKeys))
}

if len(removedEmpty) > 0 {
    log.Debug("Removed empty values",
        "keys_removed", removedEmpty,
        "count", len(removedEmpty))
}
```

**Example:**
```
DEBUG Removed excluded keys keys_removed=[provisioningState etag systemData] count=3
DEBUG Removed empty values keys_removed=[conditions.applications.excludeApplications conditions.users.excludeGroups] count=2
```

---

### 2. ID Resolution Transformer (`internal/azure/resolver.go`)

**Added functions:**
- `resolveIDsWithTracking()` - Resolves IDs and tracks which ones
- `resolveIDsInSliceWithTracking()` - Handles arrays with tracking

**Debug output:**
```go
if len(resolvedIDs) > 0 {
    log.Debug("Resolved resource IDs to names",
        "ids_resolved", resolvedIDs,
        "count", len(resolvedIDs))
}
```

**Example:**
```
DEBUG Resolved resource IDs to names ids_resolved=[virtualNetworkId subnet.id networkProfile.networkInterfaces[0]] count=3
```

---

### 3. Name Sanitization Transformer (`internal/transform/sanitizer.go`)

**Debug output:**
```go
if sanitized != displayName {
    log.Debug("Sanitized name",
        "original", displayName,
        "sanitized", sanitized)
}
```

**Example:**
```
DEBUG Sanitized name original="My-Resource@Group!" sanitized=my_resource_group
DEBUG Sanitized name original="VM-2024" sanitized=vm_2024
```

**Only logs when name actually changed** - Reduces noise for resources that don't need sanitization.

---

### 4. Terraform Import Transformer (`internal/transform/terraform.go`)

**Debug output:**
```go
log.Debug("Generated Terraform import block",
    "resource_type", terraformResourceType,
    "resource_name", resourceName,
    "sanitized_name", sanitizedName,
    "target_address", targetAddress,
    "target_format", targetFormat)
```

**Example:**
```
DEBUG Generated Terraform import block resource_type=azurerm_resource_group resource_name=my-rg sanitized_name=my_rg target_address=azurerm_resource_group.my_rg target_format={resource_type}.{name}
DEBUG Generated Terraform import block resource_type=azurerm_storage_account resource_name=myStorage sanitized_name=mystorage target_address=module["mystorage"].azurerm_storage_account.this target_format=module["{name}"].{resource_type}.this
```

---

## Files Modified

1. **`internal/transform/cleaner.go`**
   - Added tracking functions for excluded keys
   - Added tracking functions for empty values
   - Both return lists of removed key paths

2. **`internal/azure/resolver.go`**
   - Added tracking functions for ID resolution
   - Tracks property paths where IDs were found
   - Shows nested structures and array indices

3. **`internal/transform/sanitizer.go`**
   - Added conditional logging for name changes
   - Only logs when sanitization actually changes the name

4. **`internal/transform/terraform.go`**
   - Added detailed logging for import block generation
   - Shows all template variables and final address

5. **`internal/pipeline/transformer.go`**
   - Simplified logging (removed duplicate logs)
   - Individual transformers now handle their own logging

6. **`docs/DEBUG_LOGGING.md`**
   - Complete guide to debug output
   - Examples and troubleshooting scenarios

---

## Usage

### Enable Debug Logging

```bash
# Full debug output
azure-rd download --type "Microsoft.Resources/resourceGroups" --log-level debug

# Save to file for analysis
azure-rd download --type "Microsoft.Resources/resourceGroups" --log-level debug 2>&1 | tee debug.log
```

### Filter Debug Output

```bash
# See only cleaning operations
azure-rd download --type "..." --log-level debug 2>&1 | grep "Removed"

# See only ID resolutions
azure-rd download --type "..." --log-level debug 2>&1 | grep "Resolved resource IDs"

# See only name sanitization
azure-rd download --type "..." --log-level debug 2>&1 | grep "Sanitized name"

# See only Terraform imports
azure-rd download --type "..." --log-level debug 2>&1 | grep "Generated Terraform"
```

---

## Benefits

1. **Transparency** - See exactly what each transformer does
2. **Debugging** - Quickly identify why keys are missing or changed
3. **Verification** - Confirm transformers are working as expected
4. **Learning** - Understand the transformation pipeline
5. **Troubleshooting** - Diagnose configuration issues

---

## Example Debugging Workflows

### Workflow 1: "Where did my property go?"

```bash
azure-rd download --resource-id "..." --log-level debug 2>&1 | grep -i "myProperty"
```

Check output for:
- `Removed excluded keys` - Was it explicitly excluded?
- `Removed empty values` - Was it empty?

---

### Workflow 2: "Is my ID being resolved?"

```bash
azure-rd download --resource-id "..." --log-level debug 2>&1 | grep "Resolved"
```

Check for your ID path in the `ids_resolved` list.

---

### Workflow 3: "Why is my resource name different?"

```bash
azure-rd download --resource-id "..." --log-level debug 2>&1 | grep "Sanitized"
```

Shows the original → sanitized transformation.

---

### Workflow 4: "Is my custom target format working?"

```bash
azure-rd download --resource-id "..." --log-level debug 2>&1 | grep "target_address"
```

Shows the final Terraform import address generated.

---

## Summary

All four transformers now provide comprehensive debug logging:

| Transformer | Logs | Useful For |
|-------------|------|------------|
| **cleaning** | Keys removed (excluded + empty) | Finding missing properties |
| **id-resolution** | IDs resolved with paths | Verifying ID transformations |
| **name-sanitization** | Name changes | Understanding filename differences |
| **terraform-import** | Import details | Verifying Terraform addresses |

Debug logging makes the transformation pipeline completely transparent! 🔍


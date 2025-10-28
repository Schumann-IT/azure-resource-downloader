# Replace Keys - Value Replacement Feature

The `replace` option in the cleaning transformer allows you to replace complex nested objects with specific field values, simplifying your output structure.

## Problem It Solves

Sometimes Azure returns complex nested objects when you only need a single value from them:

**Before replacement:**
```yaml
grantControls:
  authenticationStrength:
    id: "00000000-0000-0000-0000-000000000004"
    displayName: "Multifactor authentication"
    description: "Requires multifactor..."
    policyType: builtIn
    requirementsSatisfied: mfa
```

You only need the `displayName`, not the entire object.

**With replacement:**
```yaml
grantControls:
  authenticationStrength: "Multifactor authentication"
```

Much simpler!

---

## Configuration

```yaml
transformers:
  - name: cleaning
    replace:
      - from: <source.path>
        to: <destination.path>
```

---

## Examples

### Example 1: Simplify Authentication Strength

**Config:**
```yaml
transformers:
  - name: cleaning
    replace:
      - from: grantControls.authenticationStrength.displayName
        to: grantControls.authenticationStrength
```

**Before:**
```yaml
grantControls:
  authenticationStrength:
    id: "00000000-0000-0000-0000-000000000004"
    displayName: "Multifactor authentication"
    policyType: builtIn
```

**After:**
```yaml
grantControls:
  authenticationStrength: "Multifactor authentication"
```

---

### Example 2: Extract Nested ID to Top Level

**Config:**
```yaml
transformers:
  - name: cleaning
    replace:
      - from: properties.networkProfile.id
        to: networkProfileId
```

**Before:**
```yaml
name: my-vm
properties:
  networkProfile:
    id: "/subscriptions/.../networkProfiles/profile-123"
    name: "profile-123"
```

**After:**
```yaml
name: my-vm
networkProfileId: "/subscriptions/.../networkProfiles/profile-123"
properties:
  networkProfile:
    id: "/subscriptions/.../networkProfiles/profile-123"
    name: "profile-123"
```

---

### Example 3: Multiple Replacements

**Config:**
```yaml
transformers:
  - name: cleaning
    replace:
      - from: grantControls.authenticationStrength.displayName
        to: grantControls.authenticationStrength
      - from: conditions.users.includeUsers
        to: targetUsers
      - from: conditions.applications.includeApplications
        to: targetApplications
```

**Before:**
```yaml
grantControls:
  authenticationStrength:
    displayName: "MFA"
conditions:
  users:
    includeUsers: ["All"]
  applications:
    includeApplications: ["app-123"]
```

**After:**
```yaml
grantControls:
  authenticationStrength: "MFA"
targetUsers: ["All"]
targetApplications: ["app-123"]
conditions:
  users:
    includeUsers: ["All"]
  applications:
    includeApplications: ["app-123"]
```

**Note:** Original paths are preserved, new paths are added. Use `remove-keys` to remove originals if needed.

---

### Example 4: Replace Then Remove Original

**Config:**
```yaml
transformers:
  - name: cleaning
    replace:
      - from: grantControls.authenticationStrength.displayName
        to: grantControls.authenticationStrength
    remove-keys:
      - grantControls.authenticationStrength.id
      - grantControls.authenticationStrength.description
      - grantControls.authenticationStrength.policyType
```

**Before:**
```yaml
grantControls:
  authenticationStrength:
    id: "..."
    displayName: "MFA"
    description: "..."
    policyType: "builtIn"
```

**After replacement + removal:**
```yaml
grantControls:
  authenticationStrength: "MFA"
```

---

## How It Works

### Processing Order

1. **Copy data** - Deep copy to avoid mutations
2. **Apply replacements** - Copy values to new locations
3. **Remove keys** - Delete unwanted keys (respecting preserve-keys)
4. **Clean empty** - Remove empty values if enabled

### Path Resolution

Uses dot notation for nested paths:
- `foo` - Top-level key
- `foo.bar` - `foo` → `bar`
- `foo.bar.baz` - `foo` → `bar` → `baz`

### Value Types

Replacements work with any value type:
- Strings
- Numbers
- Booleans
- Objects (maps)
- Arrays

---

## Debug Logging

When running with `--log-level debug`:

**Successful replacement:**
```
DEBUG Replaced key value from=grantControls.authenticationStrength.displayName to=grantControls.authenticationStrength value_type=string
```

**Source not found:**
```
DEBUG Replacement source not found from=nonexistent.path to=destination
```

**Summary:**
```
DEBUG Applied key replacements replacements=3
```

---

## Common Use Cases

### Use Case 1: Simplify Complex Objects

When Azure returns complex objects but you only need one field:

```yaml
replace:
  - from: complexObject.id
    to: complexObject
  - from: anotherObject.name
    to: anotherObject
```

---

### Use Case 2: Flatten Nested Structure

Move deeply nested values to top level:

```yaml
replace:
  - from: properties.configuration.settings.value
    to: configValue
  - from: properties.metadata.owner
    to: owner
```

---

### Use Case 3: Normalize Field Names

Rename fields for consistency:

```yaml
replace:
  - from: properties.subnetId
    to: subnet_id
  - from: properties.vnetId
    to: virtual_network_id
```

---

## Combining with Other Features

### With remove-keys

```yaml
transformers:
  - name: cleaning
    remove-keys:
      - id
      - etag
    replace:
      - from: properties.resourceId
        to: resource_id
    clean-empty: true
```

### With preserve-keys

```yaml
transformers:
  - name: cleaning
    remove-keys:
      - id
    preserve-keys:
      - properties.subnet.id
    replace:
      - from: grantControls.authenticationStrength.displayName
        to: grantControls.authenticationStrength
```

---

## Limitations

1. **Path must exist** - Source path must have a value
2. **Overwrites destination** - If destination exists, it's replaced
3. **No wildcards** - Must specify exact paths
4. **No array indices** - Can't use `items[0].value` syntax (yet)

---

## Complete Example

**Config:**
```yaml
transformers:
  - name: cleaning
    remove-keys:
      - id
      - provisioningState
      - etag
      - systemData
    replace:
      - from: grantControls.authenticationStrength.displayName
        to: grantControls.authenticationStrength
      - from: sessionControls.signInFrequency.value
        to: signInFrequencyDays
    clean-empty: true
  - name: id-resolution
  - name: name-sanitization
  - name: terraform-import
```

**Before:**
```yaml
id: "policy-123"
provisioningState: "Succeeded"
etag: "W/..."
grantControls:
  authenticationStrength:
    id: "00000000-..."
    displayName: "Multifactor authentication"
    policyType: builtIn
sessionControls:
  signInFrequency:
    value: 7
    type: days
    isEnabled: true
tags: {}
```

**After:**
```yaml
grantControls:
  authenticationStrength: "Multifactor authentication"
sessionControls:
  signInFrequency:
    value: 7
    type: days
    isEnabled: true
signInFrequencyDays: 7
```

All cleaning, replacement, and empty value removal applied! ✨

---

## Summary

| Feature | Purpose | Example |
|---------|---------|---------|
| `replace` | Replace complex objects with specific values | Extract `displayName` from nested object |
| `from` | Source path | `grantControls.authenticationStrength.displayName` |
| `to` | Destination path | `grantControls.authenticationStrength` |

Replace operations provide powerful data transformation capabilities! 🔄


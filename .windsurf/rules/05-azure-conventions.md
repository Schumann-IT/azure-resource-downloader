---
trigger: glob
globs: internal/handlers/**/*.go,internal/azure/**/*.go
---

# Azure-Specific Conventions

## Resource Handlers

### Structure
Every ARM handler follows this pattern (credentials are ALWAYS the `azcore.TokenCredential` interface, never a concrete azidentity type):
```go
type XHandler struct {
    credential     azcore.TokenCredential
    subscriptionID string
}

func NewXHandler(credential azcore.TokenCredential, subscriptionID string) *XHandler {
    return &XHandler{
        credential:     credential,
        subscriptionID: subscriptionID,
    }
}
```
Microsoft Graph handlers do not define their own struct: their constructor `NewXHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error)` configures the shared `GraphCollectionHandler` base (`internal/handlers/graph/collection.go`) with closures (`listIDs`, `fetchItem`, `displayName`) and a `models.ResourceDocumentation{...}` literal for the `documentation` field (leave `AzureType` unset — it is filled in from the handler at prompt-build time).

### List Implementation
- Every handler implements `List(ctx) ([]string, error)` — listing is handler-driven, there is no central listing switch
- ARM: delegate to the shared pagers `azure.ListResourcesByType(ctx, cred, sub, type)` / `azure.ListResourceGroups(ctx, cred, sub)` in `internal/azure/list.go`
- Graph: page the collection in the `listIDs` closure, following `@odata.nextLink`; singletons probe the object and return at most one pseudo-ID

### Fetch Implementation
- Use appropriate Azure SDK client for the resource type
- Parse resource ID using `azure.ParseResourceID()`
- Handle errors with context: `fmt.Errorf("failed to get X: %w", err)`
- Return the raw Azure SDK type (e.g., `armresources.ResourceGroup`)

Example:
```go
func (h *XHandler) Fetch(ctx context.Context, resourceID string) (interface{}, error) {
    idInfo, err := azure.ParseResourceID(resourceID)
    if err != nil {
        return nil, fmt.Errorf("failed to parse resource ID: %w", err)
    }
    
    client, err := armX.NewXClient(h.subscriptionID, h.credential, nil)
    if err != nil {
        return nil, fmt.Errorf("failed to create client: %w", err)
    }
    
    resp, err := client.Get(ctx, idInfo.ResourceGroup, idInfo.ResourceName, nil)
    if err != nil {
        return nil, fmt.Errorf("failed to get resource: %w", err)
    }
    
    return resp.X, nil
}
```

### Transform Implementation
- Type assert the raw resource
- Build `map[string]interface{}` for properties
- Use helper function `safeString()` for pointer dereferences
- Include: id, name, location, type, tags, and resource-specific properties
- Exclude: timestamps, etags, provisioning states (cleaned by transform.CleanProperties)

Example:
```go
func (h *XHandler) Transform(resource interface{}) (*models.TransformedResource, error) {
    x, ok := resource.(armX.X)
    if !ok {
        return nil, fmt.Errorf("invalid resource type, expected X")
    }
    
    if x.Name == nil {
        return nil, fmt.Errorf("resource name is nil")
    }
    
    properties := make(map[string]interface{})
    if x.ID != nil {
        properties["id"] = *x.ID
    }
    // ... add other properties ...
    
    return &models.TransformedResource{
        ID:          safeString(x.ID),
        Type:        h.GetType(),
        Name:        safeString(x.Name),
        DisplayName: safeString(x.Name),
        Properties:  properties,
    }, nil
}
```

## Microsoft Graph Handlers
- Tenant-level resources use the `Microsoft.Graph/*` type prefix (detected as the Microsoft Graph API).
- Stable resources (e.g. conditional access, authentication strength) use the v1.0 SDK
  `github.com/microsoftgraph/msgraph-sdk-go`.
- Beta-only endpoints (e.g. Intune `deviceManagement/configurationPolicies` Settings Catalog) use the beta SDK
  `github.com/microsoftgraph/msgraph-beta-sdk-go`. Document why a handler needs the beta SDK in a doc comment.
- Graph handlers receive only the credential via constructor (no subscription ID) and create their own Graph client (`newGraphClient` / `newBetaGraphClient` in `internal/handlers/graph/collection.go`).
- For deeply nested / polymorphic Graph objects, prefer serializing the whole object to a generic map via the
  Kiota `JsonSerializationWriter` (`serializeParsableToMap`) rather than hand-coding every `@odata.type` variant.
- Established `fetchItem` patterns for types needing more than a plain GET:
  - `$expand` query parameters in the item request config (e.g. `devicecompliancepolicy.go`)
  - Child-collection fetches attached to the model before serialization (e.g. `grouppolicyconfiguration.go`, `devicemanagementintent.go`)
  - Post-fetch enrichment (e.g. `deviceconfiguration.go` OMA secret resolution)
  - Singletons: probe in `listIDs`, ignore the item ID in `fetchItem` (e.g. `applepushnotificationcertificate.go`)
  - Assignments: fetch `/{id}/assignments` and attach via `SetAssignments`; reads are best-effort — on failure call `warnAssignmentsFetchFailed` and export the item without assignments
- Permission errors (missing scopes, Forbidden) must never fail the run: detection via `azure.IsPermissionError`, the resource/type is warned about and skipped.

## Naming Conventions

### Resource Types
- Azure format: `Microsoft.Service/resourceType` (e.g., `Microsoft.Storage/storageAccounts`)
- Use exact Azure naming (case-sensitive)

### File Names
- Handler files: `<resourcetype>.go` (lowercase, no underscores)
  - ✅ `resourcegroup.go`
  - ✅ `storageaccount.go`
  - ✅ `keyvault.go`
  - ❌ `resource_group.go`
- Test files: `<resourcetype>_test.go`

### Handler Struct Names
- Pattern: `<ResourceType>Handler` (PascalCase)
  - `ResourceGroupHandler`
  - `StorageAccountHandler`
  - `KeyVaultHandler`

## Azure SDK Usage

### Client Creation
- Always pass subscription ID, credential, and nil options:
  ```go
  client, err := armX.NewXClient(subscriptionID, credential, nil)
  ```

### API Versions
- Let SDK use default API versions (don't specify manually)
- Exception: If using generic `resourcesClient.GetByID()`, specify stable API version

### Error Handling
- Always wrap Azure SDK errors with context
- Check for specific error types when needed:
  ```go
  if azErr, ok := err.(*azcore.ResponseError); ok {
      if azErr.StatusCode == 404 {
          return fmt.Errorf("resource not found: %w", err)
      }
  }
  ```

### Resource ID Parsing
- Use `azure.ParseResourceID()` utility
- Returns: SubscriptionID, ResourceGroup, Provider, ResourceType, ResourceName
- Handle parsing errors before making API calls

## Transformation Rules

### Properties to Include
- **Always**: id, name, type, location
- **Conditionally**: tags (only if not empty)
- **Resource-specific**: Critical configuration properties only

### Properties to Exclude (handled by cleaner)
- `provisioningState`
- `creationTime`, `changedTime`
- `correlationId`
- `etag`
- `managedBy`

### Nested Properties
- Flatten one level when sensible
- Keep complex objects as nested maps
- Example: `sku.name` and `sku.tier` as `sku: {name: "...", tier: "..."}`

### Resource ID Resolution
- Automatically done by `azure.ResolveIDsInProperties()`
- Adds `<property>_name` field for each resource ID found
- Example: `subnetId` → adds `subnetId_name`

## Registration

### Adding to Registry
In `internal/handlers/defaults.go` → `registerDefaults()` (package `handlers`). `handlers.NewRegistry(cred, subscriptionID, resolveSecrets)` builds a registry pre-populated by this function, so `cmd` needs no edits:
```go
func registerDefaults(r *Registry, cred azcore.TokenCredential, subscriptionID string, resolveSecrets bool) {
    // Existing ARM handlers (from the arm subpackage)
    r.Register("Microsoft.Resources/resourceGroups",
        arm.NewResourceGroupHandler(cred, subscriptionID))

    // Add new handler here
    r.Register("Microsoft.KeyVault/vaults",
        arm.NewKeyVaultHandler(cred, subscriptionID))
}
```

## Testing

### Unit Tests
- Mock Azure SDK responses
- Test error cases: nil names, failed API calls, invalid type assertions
- Test property mapping completeness

### Integration Tests (future)
- Use Azure SDK test recorder
- Test against real Azure resources in test subscription
- Validate end-to-end: Fetch → Transform → Write

## Documentation Updates

When adding a new handler, update:
1. `README.md` - "Supported Resource Types" table
2. `cmd/list.go` - automatically shows via registry

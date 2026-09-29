---
paths:
  - "go/internal/handlers/**"
  - "go/internal/azure/**"
---

# Azure and Microsoft Graph handler conventions (`go/internal/handlers`, `go/internal/azure`)

## Handler shape
- Credentials are always the `azcore.TokenCredential` interface, never a concrete `azidentity` type.
- **ARM** handlers own a struct and constructor:
  `NewXHandler(credential azcore.TokenCredential, subscriptionID string) *XHandler`; file
  `internal/handlers/arm/<resourcetype>.go`, package `arm`.
- **Graph** handlers define no struct: `NewXHandler(credential azcore.TokenCredential)
  (*GraphCollectionHandler, error)` configures the shared base in `internal/handlers/graph/collection.go`
  with closures `listIDs`, `fetchItem`, `displayName` and a `models.ResourceDocumentation{...}` literal for
  `documentation` (leave `AzureType` unset — filled at prompt-build time; pick `Template` for a non-default
  family; set `hasAssignments` when the type has assignments). They create their own client via
  `newGraphClient` / `newBetaGraphClient`. Type prefix `Microsoft.Graph/<endpointName>` (camelCase collection
  segment); stable objects use the v1.0 SDK, beta-only endpoints (Intune) the beta SDK — say why in a doc
  comment.
- Files: `<resourcetype>.go` lowercase, no underscores (`resourcegroup.go`, not `resource_group.go`); tests
  `<resourcetype>_test.go` beside them. Struct names `<ResourceType>Handler`.
- Register in `internal/handlers/defaults.go` → `registerDefaults()` only. `cmd` needs no change.

## List / Fetch / Transform
- `List(ctx)` is handler-driven; there is no central listing switch. ARM: the shared pagers
  `azure.ListResourcesByType` / `azure.ListResourceGroups` (`internal/azure/list.go`). Graph: page the
  collection following `@odata.nextLink`; singletons probe the object and return at most one pseudo-id.
- `Fetch(ctx, id)`: parse with `azure.ParseResourceID()` before any call; create clients as
  `armX.NewXClient(subscriptionID, credential, nil)`; let the SDK pick API versions (specify a stable one only
  for generic `GetByID`); wrap errors with context; return the raw SDK type. Established Graph `fetchItem`
  patterns: `$expand` in the item request config; child collections attached before serialisation; post-fetch
  enrichment (OMA secret resolution); singletons ignore the item id; assignments fetched from
  `/{id}/assignments` and attached via `SetAssignments`, best-effort — on failure call
  `warnAssignmentsFetchFailed` and export without them. Prefer serialising deeply polymorphic objects to a
  generic map (`serializeParsableToMap`) over hand-coding `@odata.type` variants.
- `Transform(resource)`: type-assert, nil-check the name, build `map[string]interface{}` with `safeString()`
  for pointers. Include id, name, type, location, tags (if non-empty) and the critical configuration
  properties; timestamps, etags, `provisioningState`, `correlationId`, `managedBy` are removed by the cleaner.
  Flatten one level when sensible, keep complex objects nested. Never write `adminPassword`, access keys,
  connection strings. **`DisplayName` must be meaningful** — it names the file and is recorded in
  `metadata.yaml`, and several Graph singletons carry no `displayName` at all: supply a fixed label.
- Resource-id resolution (`<prop>_name` fields) happens in `azure.ResolveIDsInProperties()`; do not duplicate.
- Permission errors (403, missing scopes, Forbidden) never fail the run: `azure.IsPermissionError` → warn +
  skip. `internal/retry` retries 429/503/timeouts (5 attempts, backoff); 403 is never retried.

## Adding a type — what must move together
Handler + tests, `registerDefaults()`, the **Supported resource types** table in `README.md` (type,
delegated permission, notes) and the app-registration scope list if the permission is new, and a
`CHANGELOG.md` entry. Full procedure: `/new-handler`.

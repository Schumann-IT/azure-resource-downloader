---
name: new-handler
description: Add a new Azure Resource Manager or Microsoft Graph resource type handler to the Go CLI (go/internal/handlers), register it, test it and document it. Use when asked to support, export or download a new resource type.
---

# Create a new resource handler

Target type: `$ARGUMENTS` (an Azure type such as `Microsoft.KeyVault/vaults` or a Graph collection such as
`Microsoft.Graph/deviceManagementScripts`). Work in `go/`; conventions in `go/CLAUDE.md` and
`.claude/rules/go-handlers.md` apply in full.

1. **Pick the subpackage.** ARM types → `go/internal/handlers/arm/<resourcetype>.go` (package `arm`,
   struct `<ResourceType>Handler`, constructor `NewXHandler(credential azcore.TokenCredential,
   subscriptionID string)`). Graph types → `go/internal/handlers/graph/<resourcetype>.go` (package `graph`,
   `NewXHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error)` configuring the shared
   base with `listIDs`, `fetchItem`, `displayName` closures). Read two existing neighbours of the same shape
   first (e.g. `storageaccount.go`; `devicecompliancepolicy.go` for `$expand`, `applepushnotificationcertificate.go`
   for a singleton, `grouppolicyconfiguration.go` for child collections).
2. **Implement the interface**: `GetType()` (exact Azure/Graph type name), `GetDocumentationPrompt()` via a
   `models.ResourceDocumentation{...}` literal (purpose, key settings, required permissions, lifecycle notes,
   Microsoft Learn links, related types; `Template` family if the default layout does not fit; `AzureType`
   left unset), `List(ctx)`, `Fetch(ctx, id)`, `Transform(resource)` with a meaningful `DisplayName` and no
   secrets. Beta SDK only when the endpoint is beta-only, with a doc comment saying so. Assignments, if the
   type has them: fetch `/{id}/assignments`, attach with `SetAssignments`, best-effort, `hasAssignments: true`.
3. **Register** the constructor in `go/internal/handlers/defaults.go` → `registerDefaults()`.
4. **Tests** in the same subpackage (`<resourcetype>_test.go`): nil names, failed API calls, invalid type
   assertions, property-mapping completeness, display name. No network.
5. **Dependencies**: if a new SDK module is needed, `make deps` (never `go mod tidy`).
6. **README**: add the type to the *Supported resource types* table in `go/README.md` with its delegated
   permission and notes; if the permission is new, add the scope to the app-registration script in the
   *Authentication* section. Update the type count in the README intro if it is stated.
7. **CHANGELOG**: entry under `## [Unreleased]` → `### Added` in `go/CHANGELOG.md`
   (`.claude/rules/changelog.md`). If the type needs a new delegated scope, say in bold that operators must
   re-consent the app registration.
8. **Verify**: `make check` then `make build`; `./azure-rd resource types` must list the type offline.

Finish with the checklist:

```
✅ Handler created in internal/handlers/{arm,graph}/<type>.go
✅ Registered in internal/handlers/defaults.go
✅ Tests added
✅ Dependencies updated: make deps (if any)
✅ make check + make build passed
✅ README.md supported-types table (and app-registration scopes) updated
✅ CHANGELOG.md updated under [Unreleased]
⚠️  Manual: export the type against a real tenant with --type and inspect the YAML and metadata entry
```

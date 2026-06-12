# Microsoft Graph / Intune Endpoint Roadmap

Tracking checklist for the resource types covered by the reference PowerShell
exporter (`Export-IntuneEntraDocumentation.ps1`) that are **not yet** implemented
as Go handlers in this package. Implement one at a time; each gets a real handler
(a `GraphCollectionHandler` constructor, no no-op stubs), is registered in
`cmd/download.go`, gets a unit test, and is added to the README "Supported
Resource Types" table.

Implemented types and their delegated permissions are **not** tracked here —
the README "Supported Resource Types" and permission tables are the single
source of truth. New granular Intune scopes discovered during implementation
(e.g. `DeviceManagementScripts.Read.All` for script types) must be recorded in
the README permission table.

## Legend

- Status: `[ ]` pending, `[~]` in progress, `[x]` done
- "Graph endpoint" is the beta path unless noted (`v1.0`).
- Terraform type = `terraform-provider-microsoft365` resource. Marked **TBD**
  where it must be confirmed against the provider before implementing.

Phases 1 (simple collections), 2 (scripts), 3 ($expand / child-fetch
policies), 4 (applications) and 5 (Autopilot & enrollment) are complete and
were removed from this backlog; see the README "Supported Resource Types"
table.

For new types needing more than a plain GET, the pattern is established:
supply a custom `fetchItem` closure to the `GraphCollectionHandler` base —
`$expand` (see `devicecompliancepolicy.go`), child-collection fetches attached
to the model before serialization (see `grouppolicyconfiguration.go`,
`devicemanagementintent.go`), or post-fetch enrichment
(`deviceconfiguration.go`). Types without a Terraform representation return an
empty `terraformType`; the transformer then skips the import block.
Singletons (see `applepushnotificationcertificate.go`) probe the object in
`listIDs` (returning at most one ID, empty when absent) and ignore the item ID
in `fetchItem`.

---

## Phase 6 — Tenant admin & Entra singletons

| Status | Resource | Graph endpoint | Azure type (proposed) | Notes |
| --- | --- | --- | --- | --- |
| [ ] | Intune RBAC role definitions | `deviceManagement/roleDefinitions` | `Microsoft.Graph/roleDefinitions` | filter custom (`isBuiltIn=false`) |
| [ ] | Tenant settings (deviceManagement) | `deviceManagement` (root) | `Microsoft.Graph/deviceManagement` | **singleton** |
| [ ] | Authentication methods policy | `policies/authenticationMethodsPolicy` (v1.0) | `Microsoft.Graph/authenticationMethodsPolicy` | **singleton** |
| [ ] | Authorization policy / SSPR | `policies/authorizationPolicy` | `Microsoft.Graph/authorizationPolicy` | **singleton**; collection-wrapped in some tenants |
| [ ] | Entra Connect sync | `directory/onPremisesSynchronization` | `Microsoft.Graph/onPremisesSynchronization` | **singleton** |
| [ ] | Organization info | `organization` (v1.0) | `Microsoft.Graph/organization` | **singleton** |
| [ ] | Groups (dynamic + referenced) | `groups` | `Microsoft.Graph/groups` | cross-cutting; dynamic membership rules |

---

## Cross-cutting considerations

- **Terraform types are TBD** for all pending rows and must be confirmed against
  `terraform-provider-microsoft365` before each handler is finalized (the README
  table requires an accurate Terraform type).
- **Singletons** need a different handler shape than the list-based ones (single
  object GET, no per-item ID iteration) — either a `listIDs` closure returning a
  fixed pseudo-ID or a dedicated singleton base.
- **`$expand` / child collections** (intents, compliance, group policy, DEP) need
  an extra fetch step inside the `fetchItem` closure (see Phase 3 note).
- **Assignments**: the exporter enriches most items with `assignments`. Decide
  whether handlers should also fetch `/{id}/assignments` and inline them.
- **Groups resolution**: in the exporter this resolves group display names used by
  assignments. As a standalone handler it would just export groups; the
  name-resolution behavior is out of scope unless we add it explicitly.
- **Secrets**: only `deviceConfigurations` (OMA-URI) supports plaintext read-back.
  Settings-catalog / app secrets remain write-only (see README).

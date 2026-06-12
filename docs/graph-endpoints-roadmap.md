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

Phases 1 (simple collections) and 2 (scripts) are complete and were removed
from this backlog; see the README "Supported Resource Types" table.

---

## Phase 3 — Policies needing `$expand` / child fetches

Pattern already established: supply a custom `fetchItem` closure to the
`GraphCollectionHandler` base (see `devicemanagementconfigurationpolicy.go`
for `$expand=settings` and `deviceconfiguration.go` for post-fetch
enrichment). Child-collection joins (e.g. intents `settings` + `templates`)
need an extra request inside the closure.

| Status | Resource | Graph endpoint | Azure type (proposed) | Notes |
| --- | --- | --- | --- | --- |
| [ ] | Compliance policies (classic) | `deviceManagement/deviceCompliancePolicies` | `Microsoft.Graph/deviceCompliancePolicies` | `$expand=scheduledActionsForRule(...)` |
| [ ] | Compliance policies (Settings Catalog) | `deviceManagement/compliancePolicies` | `Microsoft.Graph/compliancePolicies` | child `settings` |
| [ ] | Administrative Templates | `deviceManagement/groupPolicyConfigurations` | `Microsoft.Graph/groupPolicyConfigurations` | child `definitionValues?$expand=definition` |
| [ ] | Endpoint Security intents (legacy) | `deviceManagement/intents` | `Microsoft.Graph/deviceManagementIntents` | child `settings`; join `templates` |

## Phase 4 — Applications

| Status | Resource | Graph endpoint | Azure type (proposed) | Notes |
| --- | --- | --- | --- | --- |
| [ ] | Applications | `deviceAppManagement/mobileApps` | `Microsoft.Graph/mobileApps` | many polymorphic `@odata.type`s |
| [ ] | App protection (iOS) | `deviceAppManagement/iosManagedAppProtections` | `Microsoft.Graph/iosManagedAppProtections` | |
| [ ] | App protection (Android) | `deviceAppManagement/androidManagedAppProtections` | `Microsoft.Graph/androidManagedAppProtections` | |
| [ ] | App protection (Windows) | `deviceAppManagement/windowsManagedAppProtections` | `Microsoft.Graph/windowsManagedAppProtections` | |
| [ ] | WIP (MDM) | `deviceAppManagement/mdmWindowsInformationProtectionPolicies` | `Microsoft.Graph/mdmWindowsInformationProtectionPolicies` | |
| [ ] | WIP (no enrollment) | `deviceAppManagement/windowsInformationProtectionPolicies` | `Microsoft.Graph/windowsInformationProtectionPolicies` | |
| [ ] | App config (managed devices) | `deviceAppManagement/mobileAppConfigurations` | `Microsoft.Graph/mobileAppConfigurations` | |
| [ ] | App config (managed apps) | `deviceAppManagement/targetedManagedAppConfigurations` | `Microsoft.Graph/targetedManagedAppConfigurations` | |

## Phase 5 — Autopilot & Enrollment

| Status | Resource | Graph endpoint | Azure type (proposed) | Notes |
| --- | --- | --- | --- | --- |
| [ ] | Autopilot deployment profiles | `deviceManagement/windowsAutopilotDeploymentProfiles` | `Microsoft.Graph/windowsAutopilotDeploymentProfiles` | |
| [ ] | Autopilot device identities | `deviceManagement/windowsAutopilotDeviceIdentities` | `Microsoft.Graph/windowsAutopilotDeviceIdentities` | potentially large; data not config |
| [ ] | Enrollment configurations | `deviceManagement/deviceEnrollmentConfigurations` | `Microsoft.Graph/deviceEnrollmentConfigurations` | ESP, restrictions, WHfB |
| [ ] | Apple MDM push certificate | `deviceManagement/applePushNotificationCertificate` | `Microsoft.Graph/applePushNotificationCertificate` | **singleton** |
| [ ] | Apple ADE/DEP tokens + profiles | `deviceManagement/depOnboardingSettings` | `Microsoft.Graph/depOnboardingSettings` | child `enrollmentProfiles` |
| [ ] | Apple user-initiated enrollment | `deviceManagement/appleUserInitiatedEnrollmentProfiles` | `Microsoft.Graph/appleUserInitiatedEnrollmentProfiles` | |

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

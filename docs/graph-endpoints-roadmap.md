# Microsoft Graph / Intune Endpoint Roadmap

Tracking checklist for the resource types covered by the reference PowerShell
exporter (`Export-IntuneEntraDocumentation.ps1`) that are **not yet** implemented
as Go handlers in this package. Implement one at a time; each gets a real handler
(no no-op stubs), is registered in `cmd/download.go`, gets a unit test, and is
added to the README "Supported Resource Types" table.

## Legend

- Status: `[ ]` pending, `[~]` in progress, `[x]` done
- "Graph endpoint" is the beta path unless noted (`v1.0`).
- Terraform type = `terraform-provider-microsoft365` resource. Marked **TBD**
  where it must be confirmed against the provider before implementing.

## Already implemented (reference)

| Graph endpoint | Azure type | Handler |
| --- | --- | --- |
| `deviceManagement/deviceConfigurations` | `Microsoft.Graph/deviceConfigurations` | `deviceconfiguration.go` |
| `deviceManagement/configurationPolicies` | `Microsoft.Graph/deviceManagementConfigurationPolicies` | `devicemanagementconfigurationpolicy.go` |
| `identity/conditionalAccess/policies` | `Microsoft.Graph/conditionalAccessPolicies` | `conditionalaccesspolicy.go` |
| `policies/authenticationStrengthPolicies` | `Microsoft.Graph/authenticationStrengthPolicies` | `authenticationstrengthpolicy.go` |

## Common Graph scopes (delegated/app)

From the exporter; needed depending on which handlers are enabled:
`DeviceManagementConfiguration.Read.All`, `DeviceManagementApps.Read.All`,
`DeviceManagementServiceConfig.Read.All`, `DeviceManagementManagedDevices.Read.All`,
`DeviceManagementRBAC.Read.All`, `Policy.Read.All`, `Directory.Read.All`,
`Group.Read.All`, `Organization.Read.All`, `OnPremDirectorySynchronization.Read.All`,
`Agreement.Read.All`.

---

## Phase 1 — Simple collections (fetch + serialize)

Structurally identical to the existing Settings Catalog handler: GET collection,
serialize each item to a map, transform. Lowest effort, high coverage.

| Status | Resource | Graph endpoint | Azure type (proposed) | Notes |
| --- | --- | --- | --- | --- |
| [ ] | Assignment filters | `deviceManagement/assignmentFilters` | `Microsoft.Graph/assignmentFilters` | has `rule` syntax |
| [ ] | Feature update profiles | `deviceManagement/windowsFeatureUpdateProfiles` | `Microsoft.Graph/windowsFeatureUpdateProfiles` | |
| [ ] | Quality update profiles | `deviceManagement/windowsQualityUpdateProfiles` | `Microsoft.Graph/windowsQualityUpdateProfiles` | |
| [ ] | Driver update profiles | `deviceManagement/windowsDriverUpdateProfiles` | `Microsoft.Graph/windowsDriverUpdateProfiles` | |
| [ ] | Device categories | `deviceManagement/deviceCategories` | `Microsoft.Graph/deviceCategories` | |
| [ ] | Scope tags | `deviceManagement/roleScopeTags` | `Microsoft.Graph/roleScopeTags` | |
| [ ] | Terms & Conditions | `deviceManagement/termsAndConditions` | `Microsoft.Graph/termsAndConditions` | |
| [ ] | Branding profiles | `deviceManagement/intuneBrandingProfiles` | `Microsoft.Graph/intuneBrandingProfiles` | name field `profileName` |
| [ ] | Notification templates | `deviceManagement/notificationMessageTemplates` | `Microsoft.Graph/notificationMessageTemplates` | |
| [ ] | Named locations | `identity/conditionalAccess/namedLocations` | `Microsoft.Graph/namedLocations` | |
| [ ] | Terms of use agreements | `identityGovernance/termsOfUse/agreements` | `Microsoft.Graph/termsOfUseAgreements` | |

## Phase 2 — Scripts (base64-decode payloads)

Each returns a base64 `scriptContent` (or detection/remediation pair) that should
be decoded. Reuse the existing base64 transform.

| Status | Resource | Graph endpoint | Azure type (proposed) | Notes |
| --- | --- | --- | --- | --- |
| [ ] | Windows platform scripts | `deviceManagement/deviceManagementScripts` | `Microsoft.Graph/deviceManagementScripts` | `scriptContent` → `.ps1` |
| [ ] | macOS shell scripts | `deviceManagement/deviceShellScripts` | `Microsoft.Graph/deviceShellScripts` | `scriptContent` → `.sh` |
| [ ] | macOS custom attribute scripts | `deviceManagement/deviceCustomAttributeShellScripts` | `Microsoft.Graph/deviceCustomAttributeShellScripts` | `scriptContent` → `.sh` |
| [ ] | Remediations | `deviceManagement/deviceHealthScripts` | `Microsoft.Graph/deviceHealthScripts` | detection + remediation scripts |

## Phase 3 — Policies needing `$expand` / child fetches

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

- **Terraform types are TBD** for all rows and must be confirmed against
  `terraform-provider-microsoft365` before each handler is finalized (the README
  table requires an accurate Terraform type).
- **Singletons** need a different handler shape than the list-based ones (single
  object GET, no per-item ID iteration).
- **`$expand` / child collections** (intents, compliance, group policy, DEP) need
  an extra fetch step inside `Fetch`.
- **Assignments**: the exporter enriches most items with `assignments`. Decide
  whether handlers should also fetch `/{id}/assignments` and inline them.
- **Groups resolution**: in the exporter this resolves group display names used by
  assignments. As a standalone handler it would just export groups; the
  name-resolution behavior is out of scope unless we add it explicitly.
- **Secrets**: only `deviceConfigurations` (OMA-URI) supports plaintext read-back.
  Settings-catalog / app secrets remain write-only (see README).

---
title: Correct and complete the per-handler documentation metadata
project: go
status: done
started: 2026-10-01
finished: 2026-10-02
branch: feat/regeneration-batch
pr: 52
changelog: 0.4.0
---
## Correct and complete the per-handler documentation metadata

*Kind:* fix

**Goal.** Every handler's `models.ResourceDocumentation` is accurate and complete: the right read permission, an
API reference for the API version the handler actually calls, a link to the permissions page, a deep link to the
admin center blade where the resource is managed, and type-specific best-practice links — guarded by a test so
it cannot silently decay again.

> **Review findings (2026-10-01, 53 registered types).** `EndpointDocs` is set everywhere; `BestPractices` on 7;
> `SchemaReference` and `Links.Permissions` on none; there is no admin-center link kind. `namedLocations`
> (`namedlocation.go:33`) and `termsOfUseAgreements` (`termsofuseagreement.go:32`) link `?view=graph-rest-1.0`
> but call the beta client. `deviceManagement` (`devicemanagementsettings.go:47`) links the
> `deviceManagementSettings` complex type, not the root entity it exports. `mobileThreatDefenseConnectors`
> (`mobilethreatdefenseconnector.go:30`) and `intuneBrandingProfiles` (`intunebrandingprofile.go:28`) declare
> scopes Microsoft's "Get" pages do not list (`DeviceManagementServiceConfig.Read.All` is documented). All
> `mem/intune/...` links use the retired path; the best-practice URLs are generic and duplicated across types.
>
> **Runtime effect, not only docs.** `RequiredPermissions` feeds the dedicated-app scope message
> (`internal/handlers/registry.go:100-116`, `internal/runprep/runprep.go:178`) and the audit routing
> (`internal/audit/route.go`), and the access check probes once per permission group
> (`internal/runprep/access.go` `PlanRunProbes`), so the two permission fixes change what an operator is told to
> consent and which probe covers the type. Both move to `DeviceManagementServiceConfig.Read.All`, already in the
> README's dedicated-app scope list, so no new consent is needed for them.
>
> **Decision.** Link kinds: fill the existing `Links.Permissions` on every handler and add a new
> `Links.AdminCenter`; no PowerShell link.
>
> **Permission source rule (reviewer).** The authority is the Microsoft Learn page of the operation the handler
> calls (List for a collection, Get for a singleton), at the API version it calls, *Delegated (work or school
> account)* row. A declared scope that the page lists — as least privileged or as higher privileged — stays; only
> a scope the page does not list is corrected, to the page's least-privileged read scope. Tightening every type to
> its least-privileged scope would add new scopes an operator must consent to and is out of scope here.
>
> **Group template.** `group_prompt.tmpl` does not call `prompt-links` (it renders only `EndpointDocs` inline), so
> `groups` gets its `Links.Permissions` / `Links.AdminCenter` values here but shows them only once *Template
> content fixes and a Conditional Access template* gives the group template the full header — on this branch,
> before the one regeneration, so no generated document ever misses them.
>
> **Metadata review (2026-10-02).** A field-by-field audit of every handler's `ResourceDocumentation` against
> Microsoft Learn, built on `c2fba77`: 277 changes over the 53 handler files (71 wrong, 10 outdated, 99 missing, 97
> optional) — Lifecycle 53, KeySettings 48, RelatedTypes 47, BestPractices 41, EmbeddedPayloads 24, AdminCenter 17,
> Purpose 16, SchemaReference 15, SubtypeNote 8, Permissions 4, EndpointDocs 2, RequiredPermissions 1, Template 1.
> The ready-to-paste values are in the review's change plan (`Claude outputs/handler-metadata-change-plan.md`,
> untracked and git-ignored — the implementer's input); they are applied verbatim, so the commit diff is the
> record. Its eight handler-logic follow-ups are parked ideas under *export & metadata*.
>
> **Decision.** From the review: `organizationalBranding` moves to the singleton template; `Microsoft.Graph/roleScopeTags
> (roleScopeTagIds)` is added to `RelatedTypes` of the 32 scope-tagged Intune types, with the reverse entry on
> `roleScopeTags`; `EmbeddedPayloads` names only encoded or deeply nested content and says how the export decodes
> it (the five script types and `deviceConfigurations`); `AdminCenter` only where a Learn page cites the blade;
> no permission tightening for `authenticationMethodsPolicy`, `authenticationStrengthPolicies` or
> `deviceConfigurations` (consistent with the permission source rule above).
>
> **Overlap.** The `organizationalBranding` template switch is a metadata `Template` value and ships here; the
> template-selection bullet of *Template content fixes and a Conditional Access template* stays limited to
> Conditional Access, `reusablePolicySettings` and `roleScopeTags`.
>
> **Regeneration-gated.** Every changed value moves the type's `promptSha256`; the new `Links.Permissions` line moves
> it for every type. Batch with *Template content fixes and a Conditional Access template* and *Run-prompt fixes and
> the `summary:` frontmatter line* so they share the one scheduled regeneration. Builds on the shared prompt
> partials (done; the new link goes into `prompt-links`).
>
> **Contract.** No Go → web artefact changes shape: no frontmatter key, H2 heading, `doc-headings` /
> `doc-groups` marker, export path or `metadata.yaml` field is added or renamed. After the regeneration every
> document carries a new `promptSha256` (the browser shows it as is) and its References section may cite the
> permissions page and an admin-center link on `intune.microsoft.com`, `entra.microsoft.com` or
> `portal.azure.com` — plain Markdown links the browser already renders. `organizationalBranding` moving to the
> singleton template keeps the same `doc-headings` set (`References | Lifecycle and operations | Security |
> Settings`) as the default template, so its documents keep their H2 sections. `web/` needs no change.
>
> **Owner.** none — every file touched is under `go/`. Sequencing: first of the regeneration batch on this branch;
> *Template content fixes and a Conditional Access template* relies on the `Links.AdminCenter` field and the
> "Admin center:" line this entry adds.
>
> **Implementer.** sonnet

**Plan.**

- ✅ Accessor first, bytes unchanged: a `models.Documented` interface (`Documentation() models.ResourceDocumentation`,
  `AzureType` filled in) in `internal/models/types.go`, implemented by `GraphCollectionHandler` and the three ARM
  handlers (`internal/handlers/arm/*.go`: move the inline `ResourceDocumentation` literal into `Documentation()`);
  each `GetDocumentationPrompt` becomes `models.BuildDocumentationPrompt(h.Documentation())`. `make -C go test`
  must pass with the golden prompts untouched before any metadata changes.
- ✅ `Links.AdminCenter string` in `models.ResourceLinks` (`internal/models/documentation.go`, doc comment: the admin
  center blade where the resource is managed; empty when no verified deep link exists). `prompt-links` in
  `internal/models/prompt_partials.tmpl`: add `.Links.AdminCenter` to the block's `or` condition and render
  `- Admin center: <url>` after the `- Required permissions:` line; update the `Template` field's doc comment if it
  lists the partials' content. A `models` unit test renders a `ResourceDocumentation` with only `AdminCenter` set
  and with all link kinds set, and asserts the block appears with the lines in that order.
- ✅ Permissions: `mobileThreatDefenseConnectors` (`mobilethreatdefenseconnector.go`) and `intuneBrandingProfiles`
  (`intunebrandingprofile.go`) → `DeviceManagementServiceConfig.Read.All`, in `RequiredPermissions` and in every
  `hint: requires '…'` string of the file. Then check every other handler against the permission source rule in
  the Notes (fetch each operation's Learn page); correct only a declared scope the page does not list, together
  with its hints. A corrected scope outside the current set (`Policy.Read.All`, `Group.Read.All`,
  `Agreement.Read.All`, `Organization.Read.All`, `OrganizationalBranding.Read.All`,
  `OnPremDirectorySynchronization.Read.All`, `DeviceManagement*`) must also be added to `entraPermissions` in
  `internal/audit/route.go` (`TestRouteEveryRegisteredType` fails otherwise) and named in the implementation
  report as a new consent. A page that cannot be fetched leaves the value as is and is listed in the report as
  unverified — never guessed.
- ✅ Links, Graph `?view=` per the client the handler builds: `namedLocations` (`namedlocation.go`) and
  `termsOfUseAgreements` (`termsofuseagreement.go`) → `?view=graph-rest-beta` (both call `newBetaGraphClient`).
  Every `learn.microsoft.com/en-us/mem/intune/...` URL → its current `learn.microsoft.com/en-us/intune/
  intune-service/...` page (follow the redirect, use the final URL).
- ✅ `deviceManagement` (`devicemanagementsettings.go`): Microsoft Learn has no `deviceManagement` entity or Get page, so
  `EndpointDocs` stays on the `deviceManagementSettings` complex-type page and `SchemaReference` stays empty;
  `Links.Permissions` is the permissions-reference anchor
  (`https://learn.microsoft.com/en-us/graph/permissions-reference#devicemanagementserviceconfigreadall`); Purpose and
  KeySettings describe what is exported (identifiers and `maximumDepTokens` only).
- ✅ `BestPractices`: keep a link only where the page is about this type's feature (a planning, hardening or baseline
  guide for it); remove generic ones (e.g. `protect/security-baselines` on `deviceConfigurations`, which security
  baselines do not use); add one only where such a page exists. Empty is acceptable.
- ✅ `Links.Permissions` on all 53 types: for Graph, the Learn page of the List (collection) or Get (singleton)
  operation the handler calls, at its API version (`?view=` as above); for the three ARM types, the Azure built-in
  *Reader* role section on Learn.
- ✅ `Links.AdminCenter` where a deep link is verified: the blade URL cited by a Learn page for that feature, or one
  confirmed to open the blade, on `intune.microsoft.com` (Intune types), `entra.microsoft.com` (Entra types) or
  `portal.azure.com` (ARM types); empty otherwise. The implementation report lists the types left empty.
- ✅ Registry-wide test `internal/handlers/documentation_metadata_test.go` over `NewRegistry(stubCredential{}, …,
  false)`: every handler implements `models.Documented`; `EndpointDocs`, `RequiredPermissions` and
  `Links.Permissions` are non-empty; `EndpointDocs`, `SchemaReference`, `Permissions` and `BestPractices` are
  `https://learn.microsoft.com/en-us/…`; `AdminCenter` is empty or `https://` on one of the three portal hosts; no
  link contains `/mem/intune/`; no `BestPractices` entry repeats within a type. Source scan in the same test, per
  non-test file of `internal/handlers/graph/` holding an `azureType: "…"` literal: the file uses the beta client
  (`newBetaGraphClient(` or the `msgraph-beta-sdk-go` import) xor v1.0 (`newGraphClient(` or `msgraph-sdk-go`),
  and every `/graph/api/` link of that type carries the matching `?view=graph-rest-beta` / `?view=graph-rest-1.0`;
  every scope named in a `requires '…'` hint is in that type's `RequiredPermissions`. Every registered Graph type
  is matched by exactly one file.
- ✅ Golden prompts, deliberately: `make -C go golden-update`, then review the diff — only files under
  `internal/pipeline/testdata/golden/prompts/` change (the exported-YAML goldens stay byte-identical), and each
  type's diff is confined to its permission lines and its *Reference material* block (`groups`: only its
  permission lines, if any, since its template renders no links block yet).
- ✅ Apply the metadata review. Precondition: `git diff --quiet c2fba77 HEAD -- go/internal/handlers` holds (the
  change plan was built on `c2fba77`); if it does not, stop and report the changed handler files instead of
  pasting. Input: `Claude outputs/handler-metadata-change-plan.md` (untracked, git-ignored), section *Changes per
  file*. For each of its 53 per-file sections, replace exactly the fields of its `go` block inside the handler's
  `models.ResourceDocumentation{…}` literal (ARM handlers: the literal returned by `Documentation()`): add keys
  that do not exist yet, delete a field marked `// remove` (`organizationalBranding` loses `EmbeddedPayloads`),
  put the lines under `// in Links: models.ResourceLinks{…}` into the nested `Links` literal, leave every field
  the section does not list as it is. Values are pasted verbatim; `deviceConfigurations` keeps its
  `requiredPermission` variable; `deviceManagementScripts` (`windowsplatformscript.go`) gets
  `RequiredPermissions: {"DeviceManagementScripts.Read.All", "DeviceManagementConfiguration.Read.All"}` in that
  order (the first permission keeps the type in its access-probe group; the second is already in `GRAPH_SCOPES`,
  so no new consent, and audit routing stays on `IntuneAuditLogs`). The change plan's *Not applied* and *Code
  follow-ups* sections are not implemented (the follow-ups are parked ideas). Then `make -C go fmt` and
  `make -C go build`.
- ✅ Link check, before the tests: every URL added or changed by the change plan is fetched with redirects followed
  (`curl -sSL -o /dev/null -w '%{http_code} %{url_effective}'`) and must answer 200. A Learn link that redirects
  to another `learn.microsoft.com/en-us/` URL is replaced by its final URL; one that fails is reverted to its
  previous value (or left out when it is new) and listed in the report — never replaced by a guess. The 17
  `Links.AdminCenter` blades (login required, not fetchable) are checked only for host — `intune.microsoft.com`
  for Intune types, `entra.microsoft.com` for Entra types, `portal.azure.com` for ARM types — and for the blade
  path the change plan cites; the report lists any that do not match.
- ✅ Tests: `make -C go test` passes with `internal/handlers/documentation_metadata_test.go` unchanged (Learn-only
  links, admin-center hosts, `?view=` per client, no `/mem/intune/`, hint scopes declared, no repeated
  `BestPractices` link). Add an `organizationalBranding` case to `TestSharedPromptTemplateOverrides`
  (`internal/handlers/graph/prompt_templates_test.go`): `NewOrganizationalBrandingHandler`, marker
  `tenant-wide singleton`, description "organizationalBranding uses the singleton template" — the table already
  asserts the default template's assignments text is absent.
- ✅ Golden prompts, deliberately: `make -C go golden-update`, then review the diff — exactly the 53 files under
  `internal/pipeline/testdata/golden/prompts/` change and no `*.golden.yaml` does. Each type's diff is confined
  to the fields the change plan lists for it, as far as its template renders them: `organizationalBranding`'s
  prompt is rewritten as a whole (template switch); `groups` (`group_prompt.tmpl` renders only type/Purpose,
  permissions, Lifecycle, `EndpointDocs` and KeySettings) changes only its *About this resource type*,
  *Lifecycle notes* and *give particular attention to* lines — its new `RelatedTypes` and `Links.AdminCenter`
  stay unrendered until *Template content fixes and a Conditional Access template* gives it the full header;
  `EmbeddedPayloads` appears only in default-template prompts.
- ✅ Resolve the two open follow-ups of this entry: rewrite each to its outcome, then strike it with this work.
  `deviceManagement`: Microsoft Learn has no `deviceManagement` entity or Get page, so `EndpointDocs` stays on the
  `deviceManagementSettings` complex-type page and `SchemaReference` stays empty; `Links.Permissions` is the
  permissions-reference anchor (`https://learn.microsoft.com/en-us/graph/permissions-reference#devicemanagementserviceconfigreadall`);
  Purpose and KeySettings describe what is exported (identifiers and `maximumDepTokens` only — exporting the
  settings is the parked idea *export the tenant `deviceManagement.settings`*). `AdminCenter`: the 17 blades a
  Learn page cites are set, every other type stays empty by rule; `windowsAutopilotDeploymentProfiles`'
  `Links.Permissions` is now the List page of `azureADWindowsAutopilotDeploymentProfile`.
- ✅ Documentation at *done*: `README.md` "Supported resource types" — move the `mobileThreatDefenseConnectors` and
  `intuneBrandingProfiles` rows to the *enrollment, Autopilot and tenant* (`DeviceManagementServiceConfig.Read.All`)
  table and reflect any other corrected scope there and in the dedicated-app scope list; *Intune — scripts*:
  platform-script assignments (`…/deviceManagementScripts/{id}/assignments`) also need
  `DeviceManagementConfiguration.Read.All` (already in `GRAPH_SCOPES`, no new consent); the
  `onPremisesSynchronization` row reads "One file per tenant, cloud-only tenants included"; the template-family
  table moves `organizationalBranding` to the `singleton` row. `CHANGELOG.md`: `### Fixed` for the permissions,
  `### Changed` for the corrected and completed per-type documentation metadata, with **regenerate the
  documentation** in bold.
- ✅ `Links.AdminCenter`: the 17 blades a Microsoft Learn page cites are set (Intune and Entra blades and the Azure storage
  account list); every other type stays empty by rule. `windowsAutopilotDeploymentProfiles`' `Links.Permissions` is
  now the List page of `azureADWindowsAutopilotDeploymentProfile`.

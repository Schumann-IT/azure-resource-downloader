# Next iterations

Outstanding work and parked ideas for the Go CLI. `README.md` says what it does today and `CHANGELOG.md` what
shipped and why; neither is repeated here. How entries and ideas are written, promoted, implemented and
archived is `../.claude/rules/next-iterations.md`.

Numbered entries are scheduled work: committed here before they are implemented, struck through as they land,
and archived to `../.claude/archive/go/` once done. Parked ideas, grouped by area below, are
deliberately unscheduled; each says why it is parked and what would make it worth doing.

## 1. Fix the Firewall CSP bridge between custom OMA-URIs and the Settings Catalog

*Kind:* fix

**Goal.** A Windows Firewall setting configured through a custom OMA-URI profile and the same setting configured
through the Settings Catalog are recognised as one setting, so the consistency analysis reports their conflicts and
duplicates instead of silently missing them.

> **Why.** `normaliseOMAURI` (`internal/consistency/index.go`) reads `./Vendor/MSFT/…` as `device/vendor/msft/…`,
> so `./Vendor/MSFT/Firewall/MdmStore/PublicProfile/EnableFirewall` becomes
> `device_vendor_msft_firewall_mdmstore_publicprofile_enablefirewall`. The Settings Catalog id of that node is
> `vendor_msft_firewall_mdmstore_publicprofile_enablefirewall` — no `device_` prefix — so the two never join. Found
> by the Claude Cowork catalog review (2026-10-03); confirmed against the code.
>
> **Scope.** Only the CSPs whose Settings Catalog ids carry no `device_` / `user_` scope prefix. Changing a key form
> moves which members of a configured catalog match; the seed's firewall topic carries both forms today and keeps
> working.
>
> **Not regeneration-gated.**

**Plan.**

- `internal/consistency/index.go`: in `normaliseOMAURI`, a small table of CSP roots whose Settings Catalog ids
  have no scope prefix (Firewall first; any other found while doing this, each with its evidence) normalises
  `./Vendor/MSFT/<root>/…` to `vendor_msft_<root>_…` rather than `device_vendor_msft_…`. Member validation follows
  automatically (it shares the builder).
- Tests: a custom OMA-URI firewall setting and a Settings Catalog firewall setting with different values yield one
  `conflict`; `./Device/Vendor/MSFT/Policy/…` and `./Vendor/MSFT/Policy/…` keep normalising to the
  `device_vendor_msft_policy_…` form; a catalog member in the old firewall OMA-URI form is refused with the new
  canonical form in the message.
- Documentation at *done*: `README.md` (the *Joined on* row for custom OMA-URI settings); `CHANGELOG.md`
  `### Fixed`.

## 2. Replace the seed consistency catalog with the reviewed catalog

*Kind:* feat

**Goal.** The worked example ships the consistency catalog that was checked against Microsoft Learn and the two
real tenant exports, instead of the blind seed, so an operator who copies it gets correct members, current
references, documented enforcement and the rules Learn actually supports.

> **Source.** The Claude Cowork review of 2026-10-03: `Claude outputs/consistency-catalog-review.md` (the
> reasoning, per entry) and `Claude outputs/consistency-catalog-proposed.yaml` (the catalog), both git-ignored. The
> proposal: 37 equivalences (17 fixed seed entries, 20 additions; the three minimum-OS equivalences dropped as
> unmatchable), 13 topics (`assignment-filters` dropped; `windows-feature-update-policy`,
> `windows-driver-update-policy`, `conditional-access` added), 29 rules (R6 and R7 removed — they move to the
> scope model, see *R6 and R7 as assignment checks in the scope model*); 2 equivalences and 7 rules `verified`,
> the rest `verify`; every reference moved to the current Learn addresses.
>
> **Checked (Claude Code, 2026-10-03).** `go/azure-rd docs analyze-consistency` with the proposal against scratch
> copies of both exports: exit 0 for both; only `macos-password-complex-characters` matches in neither tenant
> (kept for future tenants, as the review says). An independent leak check of both files against both exports:
> clean (every raw hit a Microsoft-defined id, a product name or a Learn example name). The detector already
> handles the review's engine questions on shared members (no double reporting; scope rules out cross-platform
> pairs), so the iOS entries stay.
>
> **Value conditions** in rule `violation` text (e.g. "deferral > 0") are judged by *The consistency analysis job*;
> no rule schema change here.
>
> **Not regeneration-gated.** Operator action at done: copy the new section into the live base file under
> `go/.config/`.

**Plan.**

- `go/config-tailored-intune.yaml`: replace the `consistency:` section with `consistency-catalog-proposed.yaml`'s,
  keeping the file's comment style (a short comment per group naming the review as source); never touch
  `go/.config/`.
- Tests: the tracked-config tests still compile the section; extend them to pin the new shape — counts per
  section, R6/R7 and `assignment-filters` absent, the verified entries' ids — and keep the
  `macos-password-minimum-length` pin.
- Documentation at *done*: `README.md` (*The catalog* — the seed paragraph: counts, what is verified, R6/R7 moved
  to the scope model, the minimum-OS comparison blocked on indexing); `CHANGELOG.md` `### Changed` with the
  operator action in bold.

## 3. Index the inner payloads of Apple custom profiles

*Kind:* feat

**Goal.** The consistency catalog can select and relate what a macOS or iOS/iPadOS custom profile actually
configures: each payload inside the profile becomes a setting that topics, rules and equivalences can name, without
a payload string ever leaving the export.

> **Why.** Today a custom profile is indexed only by the root `PayloadIdentifier` of its property list
> (`internal/consistency/apple.go`), which already reports two profiles sharing an identifier with different
> payloads. What the profile configures — a passcode policy, a FileVault payload — is invisible to the catalog, so
> the same control set through a custom profile and a Settings Catalog or compliance policy (R4) cannot be named.
>
> **Decision.** Presence only: the setting's value is always unknown and the payload's keys are never read — its
> strings may carry secrets. Equivalences and rules can show a control set through several surfaces but cannot
> compare its values.
>
> **Sequencing.** Builds on *A consistency rule and topic catalog in the configuration* (shipped), whose member
> validation and seed equivalences this entry extends. Ships after *Replace the seed consistency catalog with the reviewed catalog*, so its
> payload member lands in the reviewed catalog. Not regeneration-gated.
>
> **Owner.** `go/config-tailored-intune.yaml` (inside `go/`); never `go/.config/`.

**Plan.**

- `internal/consistency/apple.go`: for each `PayloadContent` dict of a `macOSCustomConfiguration` or
  `iosCustomConfiguration` XML property list, one setting with key
  `#microsoft.graph.<macOS|ios>CustomConfiguration#<PayloadType>`, read from the same normalised payload bytes the
  identifier hash uses; one setting per distinct type, `Unknown: true`.
- A `Setting` flag, like `ListMember`, that keeps these keys out of same-key pairing: two profiles sharing a
  `PayloadType` are a finding only through a catalog equivalence or rule, never on their own.
- The catalog's member validation accepts the new key form (through the shared key builders).
- The seed catalog: `#microsoft.graph.<macOS|ios>CustomConfiguration#com.apple.mobiledevice.passwordpolicy` joins
  the macOS and iOS/iPadOS password equivalences, `status: verify`.
- Tests: a two-payload profile yields two keys; a signed or binary payload yields none; two profiles with the same
  `PayloadType` yield no finding without a catalog and an unknown-value pair through the seed equivalence; no
  payload string reaches `consistency/` (a sentinel string grepped in the written tree).
- Documentation at *done*: `README.md` (the indexed sources of `docs analyze-consistency`); `CHANGELOG.md`
  `### Added`.

## 4. R6 and R7 as assignment checks in the scope model

*Kind:* feat

**Goal.** The consistency analysis reports assignments that cannot work as intended — a filter that can never
match the policy it is attached to, an Autopilot profile or Enrollment Status Page aimed at the wrong kind of group,
include and exclude targets of incompatible kinds — each with the Microsoft Learn statement it rests on.

> **Decision.** R6 and R7 are checks in the mechanical scope model (`internal/consistency/scope.go`), not catalog
> rules: every condition joins one resource to its own assignment target, the referenced filter or group, and a
> fixed capability table from Learn — a shape the catalog's two-sided relations cannot express. The scope model
> already holds every input (filter id and mode, filter `platform` and `rule`, group `membershipRule` and kind,
> resource platforms). Recommended by the Claude Cowork review of 2026-10-03 (`Claude
> outputs/consistency-catalog-review.md`, section 2), taken by the user.
>
> **Scope: the error-level subset the export supports today.** R6.0 dangling filter reference; R6.1 filter platform
> family ≠ resource platform family; R6.4 rule property not valid for the filter platform (W only for
> `enrollmentProfileName` on macOS — Learn contradicts itself); R6.6 an ESP or platform restriction whose filter
> uses a property outside the enrollment subset; R6.7 a value outside a closed property domain; R7.1 an Autopilot
> profile included via a user-kind target; R7.3 an Autopilot profile on All devices with an exclusion; R7.6 an
> Autopilot group rule no Autopilot device can satisfy; R7.15 an Apple user-enrollment profile on a device target;
> R7.16 user-kind include with device-kind exclude, or the reverse; R7.17 device include with a dynamic device-group
> exclude, as one summary count (info — 39 hits in one tenant). The other conditions wait for export fields (parked
> idea *the remaining R6/R7 assignment checks*).
>
> **Evidence from the review (counts only).** Tenant A: 3 resources with a macOS filter using `enrollmentProfileName`
> (6 targets), 1 resource with R7.16; tenant B clean.
>
> **Contract.** The findings land in `consistency/mechanical.yaml`; whether the browser shows them is decided with
> *The consistency analysis job* and the web *consistency view*.
>
> **Not regeneration-gated.**

**Plan.**

- Embedded capability tables next to `scope.go` (Go tables or an embedded YAML), each row with its Learn
  `reference` and `status`: filter property × platform × operators × value domain; enrollment-workload property
  subsets; Autopilot attribute patterns; workload × allowed group kind. Rows from the review's section 2.1/2.3
  facts.
- A filter-rule tokenizer (atoms `entity.property op value`, `and`/`or`, parentheses, keywords case-insensitive
  with or without `-`) evaluated three-valued; shared with the dynamic-group classifier where it fits.
- The checks listed in the Scope note, over every indexed and non-indexed resource with assignments, as a new
  finding kind in `consistency/mechanical.yaml` (one finding per resource and assignment target: check id, severity
  `error | warning | info`, the Learn reference), counted in `consistency/metadata.yaml`; R7.17 as a count only.
  Never a group name, rule string or filter value in either file.
- Tests from synthetic fixtures: each check positive and negative; the undecidable cases the review lists
  (static groups, unknown platform, version thresholds alone) never flag; nothing tenant-identifying in the output.
- Documentation at *done*: `README.md` (*Consistency analysis* — the assignment checks, the finding shape, the
  capability tables and their sources); `CHANGELOG.md` `### Added`.

## 5. The consistency analysis job

*Kind:* feat

**Goal.** A documentation agent can judge the cross-type relations the mechanical step cannot decide — within
bounded topic clusters, against the rule catalog — and every finding it writes is checked by a script against the
export and the tool's overlap matrix before it is accepted, so the result is a verified list of contradictions an
operator can act on.

> **Mirrors** `docs analyze-drift`: the CLI writes a prompt file with marked blocks; a Claude session runs it as
> its own two-phase job. Precedent: `cmd/docs/analyze_drift.go` and `internal/drift/analyzeprompt.go` (embedded
> template, `docs.ValidateMarkers` / `SpliceMarker` / `StripTemplateHeader`, the `CheckCurrent` preflight,
> `--prompt`). Paired with the web entry *The consistency view* (the browser renders
> `consistency/index.md`).
>
> **Contract.** `consistency/` joins the Go → web contract: `consistency/index.md` (the human report, rendered by
> the browser) and `consistency/findings.yaml` (machine-readable); the browser never serves `analyze.md`,
> `metadata.yaml`, `mechanical.yaml`, `findings.yaml` or `chunks/`.
>
> **Owner.** `documentation-agent-instructions.md` (repository root) gets the third job; the root `CLAUDE.md`
> Go → web contract list gains `consistency/`.
>
> **Not regeneration-gated.** The new prompt template is not hashed.
>
> **Decision.** R6 and R7 are not catalog rules: they are checks in the scope model, delivered by *R6 and R7 as
> assignment checks in the scope model* (Claude Cowork review, 2026-10-03). This job judges only the catalog's
> rules; their value conditions (e.g. "deferral > 0") are read from the `violation` text.

**Plan.**

- `docs analyze-consistency` also writes `consistency/analyze.md` from an embedded template (`go:embed`, atomic
  write, header stripped) with marked blocks `export`, `mechanical`, `clusters` (deterministic per topic, split by
  expected output size like `generate.md` §3), `overlap` (a compact pair matrix per cluster) and `rules`; preflight
  like `drift.CheckCurrent` (`exportGeneratedAt` equals `resources/metadata.yaml` `generatedAt`); `--prompt`
  substitutes a template.
- The template's procedure: one agent per cluster writing `consistency/chunks/NN.json` against a fixed schema; a
  shipped verification script (stdlib Python ≥ 3.9, read-only, same conventions as the run prompt's §6/§7 scripts)
  that fails a finding whose cited source or key is missing, whose cited value differs from the YAML (masked
  excepted), whose overlap disagrees with the matrix, whose vocabulary is outside the closed sets (`rule` from the
  catalog or `mechanical` / `other` with a justification; `kind` conflict | duplicate | contradiction | gap |
  ineffective | precedence; `severity` critical | high | medium | low; `overlap` certain | possible), or that
  restates a mechanical finding without citing it; then the orchestrator writes `consistency/findings.yaml`,
  `consistency/index.md` and a report.
- `documentation-agent-instructions.md`: the third job "analyze consistency for `<tenant>`" — prompt file,
  preflight, the two-phase gate, the allow-list (`consistency/chunks/**`, `findings.yaml`, `index.md`,
  `report-*.md`) and never `analyze.md`, `metadata.yaml`, `mechanical.yaml`, `docs/`, `resources/`.
- Root `CLAUDE.md`: `consistency/` in the Go → web contract list.
- Tests: the template validates its markers; rendering on a synthetic tenant; the verification script run on
  fixtures rejects planted bad findings (wrong value, wrong overlap, bad vocabulary, duplicate of a mechanical
  finding) and accepts a correct one.
- Documentation at *done*: `README.md` (the job, the `consistency/` files); `CHANGELOG.md` `### Added`.

## 6. Feed the consistency findings into the tenant summary

*Kind:* feat

**Goal.** The tenant summary's *consistency* paragraph says something real: it judges from the verified consistency
findings when an analysis exists for this export, and says plainly that none was run when it does not.

> **From the code.** The gate follows `internal/drift/analyzeprompt_audit.go` (`loadAttributionState`: absent /
> unreadable / outdated / current; never fails the run). The summary contract — `# Tenant summary`, the four H2s,
> `### Findings` / `### Recommendations`, severity `critical | high | medium` — stays; the five-signal sweep stays
> untouched.
>
> **Not regeneration-gated.** The run prompt is not hashed.

**Plan.**

- `docs generate-prompt` reads `consistency/findings.yaml` and renders a new `consistency` marker block (added to
  `requiredMarkers`): counts by kind × severity, the top findings linked to `consistency/index.md`, and the state —
  rendered as findings only when its `exportGeneratedAt` equals `resources/metadata.yaml` `generatedAt`, otherwise
  as "absent", "unreadable" or "outdated — not used".
- Run prompt §7: four inputs instead of three; the *consistency* paragraph judges from the new block, or says
  "consistency analysis not run for this export"; appendix E explains the fourth input; the summary check script
  unchanged in contract.
- Tests: each state renders as specified; the block carries findings only for a current file.
- Documentation at *done*: `README.md` (the pipeline order: `resource download` → `docs analyze-consistency` → the
  consistency job → `docs generate-prompt` → the documentation job → `docs generate-index`); `CHANGELOG.md`
  `### Changed`.

## Parked ideas

**Legend.** *Area* — **contract** (Go → web data on disk: `index.yaml`, `drift/`, frontmatter, section
headings), **templates** (documentation and analysis prompts; *regen-gated* when it moves `promptSha256`),
**export & metadata** (what `resource download` fetches and records), **drift & compare**, **navigation**
(sidebar, breadcrumbs, landing pages, search, routing), **document view** (how one article renders), **export
formats**, **platform rule** (a non-negotiable itself), **dependencies**, **housekeeping** (lint ledgers,
caching internals). *Impact* — operator value: high / medium / low. *Effort* — S (a day or less), M (one
branch), L (several branches or a design change).

**Ships together.**

1. **The pre-regeneration batch** has shipped: the shared prompt partials, the per-handler metadata, the template
   content fixes with the CA template (paired with web *Style the Conditional Access `Conditions` section*) and the
   consistent templates with the 2026-10-02 prompt review. What is left is one documentation regeneration of every
   tenant. The two regen-gated ideas here (*per-finding severity*, *taxonomy bootstrap*) did not join it and wait
   for the next regeneration.
2. **The compare track** (cross-project, must): *`resource compare`* ships with web *Move the compare
   normalisation to the CLI*; web *manual pairing* and *one-sided resource* follow on the CLI's rule; *version the
   drift observation* rides the first drift contract change.
3. **Housekeeping** is opportunistic: a `gocognit` entry is paid off by whichever entry edits its function.
4. **Export & metadata follow-ups** from the metadata review are independent of each other; *export ADMX
   presentation values* and *Intune branding images* each need a re-baseline after shipping, so they pair well with
   a planned re-download; *mask dedicated secret properties* only joins a regeneration if it extends the prompt
   redaction rule instead of masking.
5. **The consistency track** (go-internal, in order): *Fix the Firewall CSP bridge*, *Replace the seed consistency
   catalog with the reviewed catalog*, *Index the inner payloads of Apple custom profiles*, *R6 and R7 as assignment
   checks in the scope model*, then *The consistency analysis job* (with web *The consistency view*) and *Feed the
   consistency findings into the tenant summary*. *Reject catalog members whose type is never indexed* rides the
   catalog replacement; *export the tenant `deviceManagement.settings`* rides the analysis job.

## Parked ideas — export & metadata

### Idea: ARM values as plain data with their REST names

*Area:* export & metadata · *Impact:* medium · *Effort:* S · *Ships with:* a re-baseline (every storage account and VM changes
once)

The ARM transforms put Azure SDK structs straight into the property map (`armstorage.NetworkRuleSet` and
`Encryption`, `armcompute.OSDisk` and `ImageReference`), so yaml.v3 writes untagged fields under lowercased Go
names and nil pointers as `null`: the export reads `properties.encryption.keysource`,
`properties.networkRuleSet.defaultaction`, `storageProfile.osDisk.deleteoption` instead of the REST names, and drift
deltas print pointer addresses. Round-trip each struct through `encoding/json` into a `map[string]any` (the SDK's
json tags carry the REST names and `omitempty`) in `internal/handlers/arm/storageaccount.go` and `virtualmachine.go`;
add a test asserting `keySource`, `defaultAction` and `deleteOption`, and an ARM golden YAML. Effect: ARM YAML keys
change once, so drift reports every storage account and VM as changed — **re-baseline with `resource download` right
after shipping**. **Parked** because it needs that re-baseline (code follow-up CF2 of the 2026-10-02 prompt review;
also the Go review's G2). **Revisit** with the next planned re-download.

### Idea: Edge for Windows settings in targeted managed app configurations

*Area:* export & metadata · *Impact:* low · *Effort:* S · *Ships with:* standalone; a re-baseline if the YAML grows

Learn builds the Windows (Microsoft Edge) managed-apps configuration in the settings catalog, and Graph models those
settings as the `settings` relationship of `targetedManagedAppConfiguration`, which a plain GET does not return; the
handler (`internal/handlers/graph/targetedmanagedappconfiguration.go`) expands only `apps`. Check with a tenant that has
an Edge for Windows managed-apps configuration; if the settings are missing, add `settings` to the `$expand` (or page
`/settings`) and a KeySettings note. Effect: those policies' YAML grows once. **Parked** because it needs a tenant with
such a configuration to confirm (code follow-up CF4 of the 2026-10-02 prompt review). **Revisit** when one is
available.

### Idea: export the tenant `deviceManagement.settings`

*Area:* export & metadata · *Impact:* medium · *Effort:* S · *Ships with:* standalone, or with *The consistency
analysis job* (it makes rule R3 decidable); then turn the type's metadata back into tenant-wide settings

`GET /deviceManagement` without `$select` returns only `id`, `intuneAccountId` and `maximumDepTokens`, so the
`deviceManagement` export carries no configurable setting. Pass `$select=id,intuneAccountId,maximumDepTokens,settings`
to `client.DeviceManagement().Get` in `getSingleton` (`internal/handlers/graph/devicemanagementsettings.go`) so the
tenant-wide Intune settings (`deviceManagementSettings`) are exported — notably `settings.secureByDefault` (the switch
behind "Mark devices with no compliance policy assigned as"), `deviceComplianceCheckinThresholdDays`,
`deviceInactivityBeforeRetirementInDay`, `enhancedJailBreak` and `androidDeviceAdministratorEnrollmentEnabled`. Then
the type's metadata becomes "tenant-wide settings" again: Purpose, KeySettings on those `settings.*` paths, and the
`deviceManagementSettings` page documenting real exported data. Effect: one nested map more per tenant, so drift
reports one change per tenant once. **Parked** because the metadata review made the type honestly informational and
nobody has asked for these settings in the export yet. **Revisit** when an operator wants tenant-wide Intune
settings documented or drift-tracked — or with *The consistency analysis job*: rule R3 (Conditional Access requires a
compliant device, but a platform has no compliance policy) resolves differently for the two values of
`secureByDefault` (the default **Compliant** lets the device pass), so it cannot be judged without this export
(Claude Cowork catalog review, 2026-10-03, section 4).

### Idea: export ADMX presentation values for group policy configurations

*Area:* export & metadata · *Impact:* medium · *Effort:* M · *Ships with:* standalone; a re-baseline after it
ships

`presentationValues` are never fetched, so the values entered for Administrative Templates settings with
presentations (text boxes, drop-downs, lists) are missing from the export. In `listGroupPolicyDefinitionValues`
(`internal/handlers/graph/grouppolicyconfiguration.go`) widen the existing `$expand` from `definition` to
`definition,presentationValues($expand=presentation)`; if the service rejects the nested expand, fall back to one
`GET …/definitionValues/{id}/presentationValues?$expand=presentation` per definition value (Learn lists
`DeviceManagementConfiguration.Read.All`, no new scope). Then add `presentationValues` back to the type's
`EmbeddedPayloads` and a handler test asserting the values are attached. Effect: every Administrative Templates
profile's YAML grows, so the first drift run reports them all as changed — **re-baseline with `resource download`
right after shipping**. **Parked** because it changes the export of every such profile and needs that re-baseline.
**Revisit** when an operator needs the configured ADMX values documented or drift-tracked.

### Idea: mask dedicated secret properties in the export

*Area:* export & metadata · *Impact:* medium · *Effort:* S–M · *Ships with:* the next regeneration, but only if
the prompt redaction rule is extended instead of masking (*regen-gated* then)

Check that no dedicated secret property reaches disk: `depMacOSEnrollmentProfile.adminAccountPassword` (inside
`depOnboardingSettings.enrollmentProfiles`), `windowsAutopilotDeviceIdentity.deviceAccountPassword` and
`vppToken.token`. None occurs in the 2026-10-01 exports (the vppTokens export is empty), so Graph probably never
returns them. If any can carry a value, mask it in the export with the cleaner's replacement mechanism
(`CleanPropertiesWithReplace`, `internal/transform/cleaner.go`) or a type-specific remove-key in the default
transform config, with a `cleaner_test.go` case per key. Only if masking is not done, extend `prompt-redaction-rule`
to "dedicated secret properties (passwords, tokens)" — that moves every `promptSha256` and must ride a regeneration.
Effect: YAML changes only where a secret was present. **Parked** because it needs one export from a tenant with VPP
tokens and a macOS ADE profile that creates a local admin account to confirm. **Revisit** when such a tenant is
available, or at once if any of the three properties appears in an export.
The prompt half is covered by the 2026-10-02 prompt review (rule T1 of the archived entry on consistent prompt templates: documents never reprint values
under credential-named keys); this idea is now the export half only — masking before the value reaches disk.

### Idea: Intune branding images as content summaries

*Area:* export & metadata · *Impact:* low · *Effort:* M · *Ships with:* standalone; a re-baseline or accepting
the drift

`themeColorLogo`, `lightBackgroundLogo` and `landingPageCustomizedImage` never appear in the
`intuneBrandingProfiles` export although Learn's GET example returns them. In `fetchItem`
(`internal/handlers/graph/intunebrandingprofile.go`) fetch each profile with a `$select` naming them and check whether
Graph then returns the mimeContent `{type, value}`; if so, replace each image in the handler's `normalize` hook with
`{type, sha256, sizeBytes}` (base64 bytes are large and useless in drift, the digest still shows a logo change) and add
the three keys to KeySettings. Effect: three small summaries per profile — a one-time re-baseline or accepted drift.
**Parked** because a logo change is rarely what a review is about. **Revisit** when branding changes need to be
tracked.

### Idea: organizationalBranding: the documented request shape

*Area:* export & metadata · *Impact:* low · *Effort:* S · *Ships with:* standalone; needs a tenant with default
branding to verify

The handler uses `$expand=localizations`, but the Learn Get page supports only `$select` (documented alternative:
`GET …/branding/localizations`) and marks `Accept-Language` as required. In `fetchItem`
(`internal/handlers/graph/organizationalbranding.go`) drop the expand, send `Accept-Language: 0` for the default
branding via `requestConfig.Headers`, read the per-locale overrides with a separate paged
`GET /organization/{id}/branding/localizations` and attach them with `SetLocalizations`; test the request shape with an
httptest fixture. Effect: none for the current exports, which have no branding (404). **Parked** because neither
export tenant has default branding, so the change cannot be observed. **Revisit** when a tenant with branding is
exported, or the current call starts failing.

### Idea: relax secret resolution to a read scope

*Area:* export & metadata · *Impact:* low · *Effort:* S · *Ships with:* standalone; needs one consent test

Secret resolution on `deviceConfigurations` declares `DeviceManagementConfiguration.ReadWrite.All`, but Learn lists
`DeviceManagementConfiguration.Read.All` as least privileged for `getOmaSettingPlainTextValue`. Test once with an app
registration consented only `Read.All` (export only deviceConfigurations with `resolve-secrets` on, against a custom
profile with an encrypted OMA-URI value). If plaintext comes back, remove the `requiredPermission` switch in
`internal/handlers/graph/deviceconfiguration.go`, fix its comment and the README (`GRAPH_SCOPES` loses
`DeviceManagementConfiguration.ReadWrite.All`) and update the golden prompt; on a 403 keep `ReadWrite.All` and note in
the comment that Learn's table is wrong. Effect: no export change; one write scope fewer to consent. **Parked** because
it needs that consent test against a real tenant. **Revisit** when such a test tenant is at hand.

### Idea: name the Global Administrator role for onPremisesSynchronization

*Area:* export & metadata · *Impact:* low · *Effort:* S · *Ships with:* standalone

Learn: Global Administrator is the only role supported for the delegated GET of
`onPremisesSynchronization`, so operators without it get a 403 for this type that does not explain itself. Append
" and the Global Administrator role" to the two hint strings in `internal/handlers/graph/onpremisessynchronization.go`
(outside the quoted scope, so the metadata test's `requires '…'` check is unaffected) and add the role to the README
row. Effect: messages and documentation only. **Parked** because nobody has hit the 403 yet. **Revisit** at the first
report of it, or with any other change to that handler.

### Idea: compliance and settings-catalog assignments through the documented Get

*Area:* export & metadata · *Impact:* low · *Effort:* S · *Ships with:* standalone

`compliancePolicies` and `deviceManagementConfigurationPolicies` read assignments through a separate
`/{id}/assignments` call that has no Learn operation page, so its permission is unverified. Both handlers already send
`$expand` on the item GET (`settings`, plus `scheduledActionsForRule` for compliance): add `assignments` and drop the
separate call, so assignments come from the documented Get (`DeviceManagementConfiguration.Read.All`); keep the
best-effort behaviour (`warnAssignmentsFetchFailed` when the expanded list is absent) and verify with one export or the
golden YAML that the assignment objects are identical. Effect: none expected; one request fewer per policy. **Parked**
because the current call works. **Revisit** if it starts failing, or with other work on these handlers.

## Parked ideas — drift & compare

### Idea: the remaining R6/R7 assignment checks

*Area:* drift & compare · *Impact:* low · *Effort:* M · *Ships with:* after *R6 and R7 as assignment checks in the scope model*

The Claude Cowork review (2026-10-03, `Claude outputs/consistency-catalog-review.md` section 2) specifies more
conditions than the first entry ships: R6.2/R6.3 management-type mismatches, R6.5 filters on unsupported workloads,
R6.8 contradictory atoms, R6.9–R6.13 (ownership vs Autopilot audiences, `enrollmentProfileName` values matching no
profile, deprecated `osVersion`, app intents), R7.4/R7.5 non-Autopilot or circular group rules, R7.7–R7.14
(group tags matching no device, one group on several profiles, ESP blocking lists on user-only targets,
pre-provisioning). **Parked** because they need fields the export may not carry (`assignmentFilterManagementType`,
the assignment `intent`, `createdDateTime`, `deploymentProfileAssignmentStatus`, group owners) or rule algebra
beyond simple atoms. **Revisit** once the first checks run on real tenants and the export questions in the review's
2.6 are answered.

### Idea: keep the macOS preference domain in Settings Catalog keys

*Area:* drift & compare · *Impact:* medium · *Effort:* M · *Ships with:* standalone; changes which catalog members match

Edge, Office, Microsoft AutoUpdate and Defender for Mac settings all index as
`com.apple.managedclient.preferences_<key>`, so a topic or an equivalence member cannot tell the apps apart (Cowork
review, open question 6.0.7). Fix: carry the preference domain (e.g. `com.microsoft.edge`) into the key or as a
fact the topic matcher reads. **Parked** because it changes a key form operators may already use in a catalog and
needs a look at how the Settings Catalog instance carries the domain. **Revisit** when a macOS browser, Office or
Defender rule needs to tell the apps apart.

### Idea: canonicalise the PassportForWork `tenantid` id pair

*Area:* drift & compare · *Impact:* low · *Effort:* S · *Ships with:* standalone

One tenant carries `device_vendor_msft_passportforwork_tenantid_policies_usecloudtrustforonpremauth` (bare
`tenantid`) next to the `{tenantid}` form of the same CSP node — two Settings Catalog definitions for one node, so
the same-key step misses the pair (Cowork review, open question 6.0.6). **Parked** until Learn or Graph confirms
both definitions write the same node. **Revisit** then: canonicalise the segment in `catalogKey`, or add a `same`
equivalence to the catalog.

### Idea: reject catalog members whose type is never indexed

*Area:* drift & compare · *Impact:* low · *Effort:* S · *Ships with:* standalone; best with *Replace the seed consistency catalog with the reviewed catalog*

The `consistency:` validation checks a member's *form* only (`validMemberKey`), so a well-formed typed key of a type
the setting index never reads passes and can never match — e.g. the seed's
`#microsoft.graph.windowsFeatureUpdateProfile#featureUpdateVersion` (feature update profiles are not among the six
indexed types). It is only visible later as an `unmatchedMembers` entry, indistinguishable from a key this export
merely does not carry. Fix: refuse a typed member or `keys:` selector whose `@odata.type` belongs to no indexed
source type, naming the type, with a test on that seed key. **Parked** because it needs a map from `@odata.type` to
source type that the index does not keep today (`deviceConfigurations` alone spans dozens of types), and the Cowork
review is told to flag unmatchable members itself. **Revisit** when the review's fixes land, or when a second
unmatchable member is found.

### Idea: per-document backlinks to consistency findings

*Area:* drift & compare · *Impact:* low · *Effort:* M · *Ships with:* after the consistency analysis job and its web view

Link each resource's document to the consistency findings that cite it (a marked block or a browser-side lookup in
`consistency/findings.yaml`), so a reader of one policy sees that it conflicts with another. **Parked** because the
summary and the consistency view already list the findings. **Revisit** when readers ask for it from a policy page.

### Idea: `resource compare` — an offline comparison of two exports, and the home of the cross-tenant identity rule

*Area:* drift & compare, contract · *Impact:* high · *Effort:* L · *Ships with:* **must** ship with web *Move the
compare normalisation to the CLI*; carries `version:` from day one

Compare two tenants' exports on disk — stage against prod — the way `resource drift` compares one tenant
against its own export: a verdict per resource key (only in A, only in B, same, different), dotted-path deltas
for the different ones, payloads, and an `analyze.md` so the drift-analysis agent can judge the **impact** of
each difference (security posture, compliance, lifecycle, who is affected) rather than only that bytes differ.
No Azure call: both inputs are export trees, so the command is offline like `docs analyze-drift`.

**Why this exists as a Go idea now.** The web project has shipped a **web-only proof of concept** of exactly
this comparison (the docs browser's tenant compare, described in `web/README.md`: two-click tenant
selection on the picker, a listing from the two `resources/metadata.yaml` files, and a per-resource YAML diff).
To make equal configuration compare equal across tenants, that PoC **normalises tenant-local identity in the browser**:
drops every `id` or `sourceId` at any depth whose value contains a GUID (ids without one — settings
ordinals, `all_users`, authentication method names, the all-zero sentinels — are content and stay), every
`*@odata.context` key at any depth, `createdDateTime`, `lastModifiedDateTime`, `version` and the group
identity fields (`mail`, `mailNickname`, `proxyAddresses`, `securityIdentifier`, `renewedDateTime`), and
resolves `assignments[].target.groupId`, `assignments[].target.deviceAndAppManagementAssignmentFilterId` and
`notificationTemplateId` to the display name of the matching `resourceId` in the same tenant's
`metadata.yaml` — a set-valued lookup, since neither `resourceId` nor `displayName` is unique in real exports:
a reference resolves only when every entry with that id agrees on the name, otherwise the GUID stays and is
flagged ambiguous or unresolved, and a zero-sentinel reference means *none* and passes through. Measured
against the two reference exports, that rule leaves 19 of 114 paired resources identical, 36 once
`assignments` is removed as well — the difference being the same policy targeting a differently named group
in each tenant, which the web diff page reports as *differs only in audience*. That rule is a **judgment,
not a fact**, and it was accepted on the web side under one condition, recorded there: it is provisional and
**moves to the CLI once it is stable**. The PoC's job is to find out what the rule is against real pairs;
this idea's job is to receive it. Until then the browser computes what the CLI should be emitting — the same
disagreement risk the taxonomy rules exist to prevent (a rule derived in one consumer can disagree with every
other), tolerated here only because there is no other consumer yet.

**Not planned — parked deliberately**, until the PoC has produced a rule that can be *stated and tested* —
a fixed drop list plus a reference-resolution table per field, with fixtures from a real stage/prod pair —
rather than guessed per resource type. Promoting it before that would freeze a guess into a contract the
browser then depends on.

**Shape when promoted.**

- **Engine** beside the drift engine (`internal/drift` or a sibling `internal/compare`), reusing the verdict
  and delta machinery over normalised documents; the normalisation is one exported, table-driven, unit-tested
  function — the *single* truth the browser stops duplicating.
- **Command** `azure-rd resource compare` taking two export domains (both resolved the way `--domain` is
  today), offline, `--dry-run` withholding only the writes like `resource drift`.
- **Output tree — placement is the open design question.** Every tree today lives under `<output>/<tenant>/`
  and a comparison belongs to neither tenant. Candidates: a sibling root `<output>/compare/<a>__<b>/`, or
  under the left tenant `<output>/<a>/compare/<b>/`. Whichever is chosen, the tree mirrors `resources/` keys
  the way `drift/` does (metadata at the root, payloads ≥ 2 levels deep, agent-written documents beside them),
  is cleared and rebuilt per run with no history, and carries a schema `version:` from day one — the field
  `drift/metadata.yaml` lacks and `index.yaml` learned to need.
- **Analysis prompt** as a third template beside `generate_prompt_template.md` and
  `analyze_drift_template.md`, with `doc-prompt.md` as the per-type lens again — **not regeneration-gated**,
  moving no `promptSha256`.
- The browser then renders the tree as it renders `drift/` and deletes its own normaliser.

The comparison reads names from `metadata.yaml`; the exported YAML keeps ids as facts and never embeds another
object's name.

## Parked ideas — contract

### Idea: resolve group references outside assignments in the documentation run

*Area:* contract · *Impact:* medium · *Effort:* M · *Ships with:* a go/web pair (a new marker class in the browser); the next
regeneration

The CA template keeps group, role and application GUIDs bare "unless the documentation run resolves it for you in a
block it splices in", but the run has no CA splice: `docs generate-prompt` builds the referenced groups and the
reference map from `assignments[].target.groupId` only, so a group used only by Conditional Access gets no document
and every CA document shows bare GUIDs. Collect `conditions.users.includeGroups` / `excludeGroups` into the
referenced groups and the reference map (`internal/docs/generateprompt.go`); give CA documents a forward hash and a
`<!-- ca-targets:start -->` block rendered by the run as `Direction | Kind | Target` at the end of *Conditions* (the
template asks for it with bare GUIDs, like the assignments block); register the marker class in
`web/src/docs/section-hooks.ts` (`MARKER_BLOCK_CLASSES`). Effect: new documents for CA-only groups; every CA document
is re-spliced once. **Parked** by decision C11 of the 2026-10-02 prompt review: a browser-contract change that should
not hold up the regeneration batch. **Revisit** when readers ask who a CA policy targets by name, or with the next
contract change on the web side.

The same gap exists outside Conditional Access: the authentication methods policy targets groups from its own
settings (`authenticationMethodConfigurations[].includeTargets[].id`, e.g. the X.509 certificate method), which the
reference map does not cover either. On the 2026-10-02 run of `iis.mitarbeiterangebote-staging.de` that document
showed a bare GUID for an exported group (`co_developers`), and the group got no document of its own. When picked
up, collect every non-assignment group reference — CA include/exclude groups and authentication-method include
targets — through one table of reference paths per type, so a new type with inline group targets is one row.

### Idea: version the drift observation, and name `drift/` a Go → web contract

*Area:* contract · *Impact:* low · *Effort:* S · *Ships with:* rides the next change to the drift schema or
per-finding frontmatter (web accepts `>= 1` in the same pair)

`drift/metadata.yaml`, the payloads at `drift/<key>.yaml`, the analysis prompt and the agent-written
`drift/<key>.md` and `drift/index.md` are read by the browser, joined by the shared `<type>/<name>` key, gated on
`baseline.generatedAt` and verified by hash — part of the Go → web contract the root `CLAUDE.md` lists, so a
change to the observation schema, to the per-finding frontmatter (`verdict`, `severity`, `observedAt`,
`baselineGeneratedAt`) or to the index's `severities:` line already ships as a go/web pair. The observation
carries a `toolVersion` but no schema `version:` — the field `index.yaml` learned to need. **Not planned — parked
deliberately**: nothing has broken, and adding the field alone is cheap but pointless until a consumer branches
on it.

**Revisit when** the observation schema or the per-finding frontmatter next changes for another reason: add
`version: 1` to `drift/metadata.yaml` in that same change, have the browser accept `>= 1`, and mark `drift/`
as versioned in the root `CLAUDE.md`'s contract list.

## Parked ideas — templates

### Idea: default objects are not "configured but unassigned"

*Area:* templates · *Impact:* low · *Effort:* S · *Ships with:* standalone; takes effect on the next `docs generate-prompt`

The tenant summary's *Assignment posture* (`summary-facts` block, `renderSummaryFacts` in
`internal/docs/generateprompt_render.go`) counts every resource of an assignment-capable type without targets as
*configured but unassigned*, and the run report lists them as "policies with no assignments". Built-in default
objects land there although they are never assigned the way a policy is: the 2026-10-02 run of
`iis.mitarbeiterangebote-staging.de` listed the *Default Branding profile* (`intuneBrandingProfiles`, applies to
everyone) and the *Default scope tag* (`roleScopeTags` id `0`, applies to everything). Exclude them by a fact the
export already records (the branding profile's `isDefaultProfile`, the scope tag's id `0`) — a one-line rule per
type in the summary facts, plus a run-prompt sentence that defaults are not findings. **Parked** because the signal
is noise, not wrong data, and readers see the names. **Revisit** when the summary's posture numbers are used for
reporting, or with the next change to the summary facts.

### Idea: per-finding severity in document `Security` sections

*Area:* templates (*regen-gated*) · *Impact:* low · *Effort:* M · *Ships with:* only worth it riding a
regeneration already scheduled; web colours or filters the tags

Tag every individual security callout *inside each resource's document* with `**[risk]**` / `**[review]**` /
`**[ok]**`, so the web side can colour or filter them. **Not planned — parked deliberately**, for three
reasons:

- **It is a subjective, model-only judgement.** Nothing in the export can compute or validate whether a given
  setting is risk / review / ok.
- **It is made 400+ times** (once per callout across every document), so a bad or inconsistent batch is
  likely — and the only fix is regenerating everything.
- **Low marginal benefit.** Section-level styling (the `Security` H2 slug) already gives the frontend most of
  the visual win without the per-item risk.

This is the opposite trade-off from the tenant-summary findings severity, which is decided once per tenant on
at most six findings — tiny blast radius, easy to eyeball — and was therefore done.

**Revisit only if both hold:** (1) the closed-heading contract has proven stable across a real regeneration,
with no drift observed in practice; and (2) the web side needs per-item severity that section-level styling
cannot deliver. If promoted, treat it as its own one-shot: extend the `Security:` instruction across all
seven templates and regenerate every document — and accept that it cannot be automatically validated.

### Idea: bootstrap the curated taxonomy from per-document LLM suggestions

*Area:* templates (*regen-gated*), contract · *Impact:* low · *Effort:* M–L · *Ships with:* only worth it riding
a regeneration already scheduled

Have the doc-generation model suggest, per resource, which programmes it belongs to (as *labels* with a short
rationale, never ids), then harvest those suggestions at index time into `docs/taxonomy-suggestions.yaml` — a
report that diffs the guesses against the curated rules into **coverage gaps** (a resource the model assigns to
an existing programme that no rule matches) and **new-programme candidates** (a label that is not a programme
yet). It would let an operator grow the `taxonomy:` rule set from evidence instead of authoring every regex
cold. **Not planned — parked deliberately**, for two reasons:

- **Its payoff scales with taxonomy size, which is small today.** The report earns its keep only once an
  operator is maintaining a large, drifting rule set across many tenants. Cold-authoring the handful of
  programmes in play now is cheaper than building and reviewing an advisory pipeline.
- **It cannot land cheaply on its own.** The suggestion instruction lives in the per-type templates, so adding
  it moves `promptSha256` for every non-`record` type and forces a full documentation regeneration. It is only
  free if it rides a regeneration already scheduled for another reason — otherwise it forces its own.

**Revisit when** an operator is maintaining programmes at a scale where cold-authoring rules is the bottleneck,
*and* a full documentation regeneration is already scheduled to absorb the template change. If promoted, the
shape is constrained — these are invariants that keep it safe, not open questions:

- **Advisory only, never authoritative.** `facets` stays rules-only and deterministic; suggestions live in a
  separate artifact and the only path from a guess to authoritative data is a human writing a rule. Promotion
  stays manual — auto-writing rules would re-inject non-determinism into the rules source and defeat the point.
- **Labels, not ids.** The operator mints the stable id (`programmeIDPattern`) at promotion time, so the model
  can never spawn `cis`/`cis-l1`/`cis-hardening` as three programmes.
- **Determinism preserved.** Because suggestions are harvested from written frontmatter bytes, both
  `index.yaml` and the suggestions artifact stay byte-identical over an unchanged export; the non-determinism
  is confined to doc-authoring time, exactly as `platformGroup`/`functionGroup` already are.
- **The hint vocabulary must not touch `promptSha256`.** Any hint of the operator's current labels rides the
  non-hashed `docs/generate.md`, never a per-type `doc-prompt.md`, so a cheap offline `taxonomy:` edit never
  couples to an expensive regeneration. Suggesting with no hint (pure bootstrap, no `taxonomy:` yet) must also
  work; label normalisation clusters the free output.
- **`docs/taxonomy-suggestions.yaml`** is the third and last file `azure-rd` writes under `docs/` (with
  `generate.md` and `index.yaml`), at the tree root where no document can be; `--dry-run` writes nothing and
  `--prune` never touches it.
- **`config.example.yaml` stays inert** — the feature needs no new config key (it reuses `taxonomy:` labels as
  an optional hint), so loading the example unmodified still produces byte-identical output including every
  hash in `resources/metadata.yaml`.

## Parked ideas — housekeeping

### Idea: clear the `gocognit` baseline

*Area:* housekeeping · *Impact:* low · *Effort:* S per function · *Ships with:* opportunistic: any entry that
edits a listed function (`GeneratePrompt`, `GenerateIndex`, `drift.Compare`, …)

Split up the 26 functions named in the baseline at the end of `.golangci.yml`'s `exclusions.rules` — the ones
that already exceeded the Sonar cognitive-complexity threshold when `gocognit` was switched on, across 20 files —
deleting each entry as its function is fixed, until the block and its explanation can go. The worst are
`GeneratePrompt` (58), `GenerateIndex` (57), `findAndRemoveKeysWithPreserve` (47), `ParseCleaningConfig` (39),
`drift.Compare` (38), `compileTaxonomy` (37) and `PrintSummary` (34); the rest sit between 16 and 30.
**Not planned — parked deliberately**, for three reasons:

- **The baseline already delivers what mattered.** The rule stays enabled at the server's threshold, the gates
  are green, and every function written from now on has to comply — including new functions in the listed files,
  because each entry names one function instead of excluding its file. Nothing is hidden either: Sonar keeps
  reporting all 26, which is deliberately why the baseline is not mirrored in `sonar-project.properties`.
- **A high score here is not a defect, and several of these functions are covered by invariants that a
  refactor must not disturb.** `PrintSummary`, `mergeMetadata`, `pruneCovered` and `Compare` are exactly the
  places where "an incomplete run may not mark anything absent", "covered means the listing succeeded" and the
  prune guards live; `Writer.Write` and `transformResource` sit on the every-request-produces-one-result
  accounting. Splitting them for a metric, without a reason a reader would recognise, risks a real regression in
  return for a number.
- **Go's explicit error handling inflates the metric**, so a mechanical fix — extracting each `if err != nil`
  ladder into a helper — would move the score without making anything clearer, which is the outcome the rule is
  supposed to prevent.

**Revisit when** one of these functions is being changed for another reason (split it then and delete its entry
in the same commit — that is how this shrinks without a campaign), when a function on the list becomes hard to
change safely in practice rather than merely scoring high, or if the baseline ever stops shrinking, which would
mean it has started collecting new debt instead of recording old. How to split, measure and record is the ledger
procedure in `.claude/rules/go-style.md`.

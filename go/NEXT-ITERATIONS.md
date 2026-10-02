# Next iterations

Outstanding work and parked ideas for the Go CLI. `README.md` says what it does today and `CHANGELOG.md` what
shipped and why; neither is repeated here. How entries and ideas are written, promoted, implemented and
archived is `../.claude/rules/next-iterations.md`.

Numbered entries are scheduled work: committed here before they are implemented, struck through as they land,
and archived to `../.claude/archive/go/` once done. Parked ideas, grouped by area below, are
deliberately unscheduled; each says why it is parked and what would make it worth doing.

## 1. Ship the section-6 reference check and the section-7 signal sweep as scripts in the run prompt

*Kind:* fix

**Goal.** Every documentation run checks its cross-document references and sweeps the export for the tenant
summary's signals the same way, with scripts the run prompt ships — as it already does for the structural checks
(section 4) and the summary check (section 7) — instead of each agent writing its own, so a run cannot fail, pass
or miss a finding because of a script it got wrong.

> **Why.** Section 6 of `internal/docs/generate_prompt_template.md` describes six checks in a table and tells the
> agent to "script them the same way", but ships no script. On the first documentation run with the harmonised
> templates (tenant `iis.mitarbeiterangebote-staging.de`, 2026-10-02) the agent's own script first reported a
> false failure: it counted the filter link in a group's `Targeted by` row (the *Filter* column) as a group
> back-reference. The agent corrected its script and the re-run was clean, but the next run writes a new script
> with new bugs.
>
> **Signal sweep (2026-10-02, tenant `cb-gmbh.com`).** Section 7 defines a five-signal sweep over `resources/` in
> prose and leaves the script to the agent. On that run the agent had to widen the credential-word list itself
> (it first missed `REMOTEOFFICEAUTHKEY`), the closed rules (a)–(d) miss a credential in a command line (a
> TeamViewer `APITOKEN=` in `mobileApps` `installCommandLine` — the document redacted it, the summary did not
> list it), and *Credentials near expiry* picked up update-ring pause expiries, which are not credentials. The
> agent excluded ISO timestamps and pause dates by hand. The same run's documents and section 6 were clean.
>
> **Not regeneration-gated.** The run prompt is not hashed; no document changes. Takes effect at the next
> `docs generate-prompt`. The summary's *configured but unassigned* count for default objects is a separate
> parked idea (*default objects are not "configured but unassigned"*). No other entry or parked idea needs to
> ride along.
>
> **Scope.** Update-ring pause windows (`featureUpdatesPauseExpiryDateTime`, `qualityUpdatesPauseExpiryDateTime`)
> and a Microsoft 365 group's `expirationDateTime` leave the sweep entirely: they are not credentials, the signal
> list stays closed at five (appendix E), and a paused ring is already described in its own document. The
> marker-pair check is shared by copy, not import: the agent pastes each script as its own file, so section 6
> carries a verbatim copy of the section-4 helper and a test keeps the two identical.
>
> **Contract.** No artefact the web project reads changes. Documents keep their frontmatter, their four marker
> blocks (`assignments`, `targeted-by`, `used-by`, `notifications`) and their table shapes; `docs/summary.md`
> keeps `# Tenant summary`, the four H2 headings, the `### Findings` / `### Recommendations` H3 pair and the
> `Severity | Finding | Affected | Documents` table with `critical` / `high` / `medium`. The new scripts only
> read: section 6 reads `docs/` (documents and the rendered `docs/generate.md`) and `chunks/mtimes.json`; the
> sweep reads `resources/` (YAML, `metadata.yaml`, `doc-prompt.md`, plist sidecars) and prints to stdout.
> Neither writes a file. The tightened section-4 marker check (order and nesting) fails only documents that
> already broke section 2's "never nest markers, never a start without its end" rule.
>
> **Owner.** none — every change is under `go/`. No sequencing constraint.
>
> **Implementer.** opus

**Plan.**

- ~~Constraints for both new scripts (state them in the template prose once, above the section-6 script): Python ≥
  3.9 standard library only (no PyYAML — like sections 4 and 7, the agent's environment is not guaranteed to have
  it); run from the tenant folder; read-only (write no file); never print a credential value (print the resource
  path, the key path or argument name, the rule and the value's length). The script text must not contain the
  literal start or end comment of any tool-filled block (`export`, `worklist`, `refmap`, `usedbymap`, `resplice`,
  `migrate`, `expected`, `summary-facts`): `ValidateMarkers` requires each exactly once in the template, so build
  such strings from the name (`f"<!-- {name}:start -->"`), as the section-4 script already does.~~
- ~~`internal/docs/generate_prompt_template.md` section 4: move the inline marker loop (`for marker in
  ("assignments", "targeted-by", "used-by", "notifications")`) into a function `def marker_problems(text):` that
  returns a list of messages and is called once per document with `fail(doc, msg)` for each. It keeps the
  existing *unbalanced* and *repeated* messages and adds *end before start* and *nested* (another block's start
  between a start and its end). The section-4 table's *Assignment markers* row and section 2's "never nest"
  sentence already state the rule; no prose change beyond that.~~
- ~~`internal/docs/generate_prompt_template.md` section 6: a Python block in a four-backtick fence (as in sections 4
  and 7) after the table, docstring
  `"""Section 6 reference checks. Run from the tenant folder after section 5. Exit 1 if anything failed."""`,
  the same `fail(doc, msg)` / `Counter` summary / exit-code shape as section 4, walking every document under
  `docs/` except the root files. It implements every row of the table:~~
  - ~~*Assignment resolution* — inside any of the four marked blocks, every GUID except the all-zero filter
    sentinel shares its table cell (or, outside a table, its line) with a Markdown link or `⚠️ not in export`.~~
  - ~~*Link symmetry* — tables are read by their header row. Policy → group: links in the assignments table's
    *Target* column only, resolved relative to the document; the group document's `targeted-by` table must have
    a *Resource*-column link resolving back to the policy. Group → policy: links in the `targeted-by` *Resource*
    column only; that document's assignments *Target* column must link back. Links in the *Filter* column (either
    table) are never group references. Compliance policy ↔ notification template: every link in a policy's
    `notifications` block must be answered by a *Resource*-column link in the template's `used-by` table, and
    every such *Resource* link by a link in that policy's `notifications` block.~~
  - ~~*Link targets exist* — every relative link inside a marked block (anchors and `http(s)` links ignored)
    resolves to a file under `docs/`.~~
  - ~~*Marker pairs survived* — a verbatim copy of section 4's `marker_problems`.~~
  - ~~*Hashes updated* — the script reads the rendered prompt (`docs/generate.md`, or the path given as its first
    argument) and, inside its `worklist`, `resplice` and `migrate` blocks, every table whose header has a
    `Document` column: for each row and each of `assignmentsSha256` / `notificationsSha256` / `usedBySha256` /
    `targetedBySha256` present as a column with a non-empty cell, the document's frontmatter must carry that key
    with that value. A missing prompt file fails.~~
  - ~~*Nothing else touched* — against `chunks/mtimes.json` (missing → fail: section 4 has not passed): a document
    whose mtime moved, or that is not in the snapshot, must be named in the `worklist`, `resplice` or `migrate`
    block; a snapshot document that no longer exists fails.~~
  ~~The table stays as the explanation; the sentence "Script them the same way" becomes: run the script after
  section 5, repair what it reports through the section-5 splice script (never by hand), and re-run until it
  exits 0. The *Marker pairs survived* row says the script carries the section-4 helper.~~
- ~~`internal/docs/generate_prompt_template.md` section 7: ship the signal sweep as a Python block in a four-backtick fence
  (as in sections 4 and 7) under *Where the facts come from*, docstring `"""Section 7 signal sweep. Run from the tenant folder before writing
  docs/summary.md. Prints each signal with the resources it names."""`. It reads YAML with a small
  indentation-aware walker over the `yaml.v3` output shape (mapping scalars with their key path, `|` / `>` block
  scalars joined, the keys of each list item grouped), measures time against `metadata.yaml`'s `generatedAt`
  (never the clock), prints one section per signal with a count and deduplicated, sorted lines, and exits 0
  (1 only when `resources/metadata.yaml` is missing). It sweeps `resources/**/*.yaml` except `metadata.yaml`, plus
  `.mobileconfig` / `.plist` / `.xml` sidecars for rule (a) only, so the result does not depend on the
  `base64-decode` transformer's inline or file mode. Within it:~~
  - ~~*Not in force* — top-level `state: enabledForReportingButNotEnforced`, `state: disabled` or
    `isEnabled: false`, counted per type with each resource's path.~~
  - ~~*Configured but unassigned* — from `metadata.yaml`: entries with `presentInTenant: true` whose type has
    `hasAssignments: true` and that carry no `assignmentTargets`, with display name and derived document path —
    the same set the `summary-facts` block counts.~~
  - ~~*Dangling targets* — from `metadata.yaml`: every `groupId` in a present entry's `assignmentTargets` that is
    not the `resourceId` of any `Microsoft.Graph/groups/…` entry, with the number of resources assigning it —
    the GUIDs the `refmap` block flags dangling.~~
  - ~~*Credentials near expiry* — only types whose `doc-prompt.md` `doc-headings` marker lists `Expiry and renewal`
    (the credential family: Apple push certificate, VPP tokens, DEP onboarding settings), and only their
    top-level `expirationDateTime` / `tokenExpirationDateTime`, quoted or not: past, or within 180 days of the
    export timestamp, printed with the date and the days left.~~
  - ~~one credential-word list, stated once and used by rules (a), (b), (c) and (e): `password`, `passwd`, `pwd`,
    `passphrase`, `secret`, `token`, `apikey`, `authkey`, `accesskey`, `privatekey`, `sharedkey`. A name matches
    when, lowercased with `_`, `-` and `.` removed and one trailing `value` / `text` / `string` dropped, it ends
    with a list word (`wifiPassword`, `preSharedKey`, `REMOTEOFFICEAUTHKEY`, `APITOKEN` match; `tokenName`,
    `passwordMinimumLength`, `tokenExpirationDateTime` do not). Rule (c) instead looks for a list word as a
    whole word in the free text.~~
  - ~~*Plaintext credentials*, rules (a)–(d) as today, plus rule (e): in `installCommandLine`,
    `uninstallCommandLine` and any key ending in `CommandLine` (case-insensitive), a `NAME=value`, `/NAME value`,
    `/NAME:value` or `-NAME value` argument (quotes stripped) whose name matches the list and whose value is
    credential-shaped.~~
  - ~~credential-shaped as today (≥ 10 characters; a hex run of 16+ first, else at least three of lowercase,
    uppercase, digit, other non-space), with these exclusions added to the existing ones: ISO-8601 dates and
    timestamps (`YYYY-MM-DD`, optionally `T…`) and any value containing whitespace are never credential-shaped.~~
  ~~The table stays as the explanation and is updated to match: the *Credentials near expiry* row names the
  credential types and their two fields and says pause windows and group expiry are not credentials; the
  *Plaintext credentials* row says "exactly five rules" and adds (e); the credential-shaped paragraph gains the
  two exclusions. The rule (c) caution (free-text fields only) stays. The sentence introducing the sweep says to
  run the shipped script, not to write one.~~
- ~~Tests (`internal/docs/generateprompt_test.go`), presence: the default template contains both docstrings,
  `def marker_problems(`, the *Target* / *Resource* column names the section-6 symmetry reads, the
  credential-word list, rule (e)'s `CommandLine` match and `Expiry and renewal`; and a `GeneratePrompt` run with
  the default template still succeeds (no tool-filled marker literal leaked into a script).~~
- ~~Tests, helper identity: extract the two `def marker_problems` bodies from the template (by the section-4 and
  section-6 docstrings) and assert they are byte-identical.~~
- ~~Tests, section-6 fixture run (skip with `t.Skip` when `exec.LookPath("python3")` fails; CI's
  `ubuntu-latest` has it): in `t.TempDir()`, a tenant with one compliance policy assigned to one group with an
  assignment filter and referencing one notification template; `docs/generate.md` produced by `GeneratePrompt`
  on that fixture so the parser is pinned to the real table shapes; hand-written documents whose blocks and
  frontmatter hashes match the work list; `chunks/mtimes.json` written from the files' mtimes. Extract the
  script from the rendered prompt, run it with the tenant as working directory. Clean → exit 0, including with
  the filter link in the group's `Targeted by` *Filter* column. Each planted defect → exit 1 with its message:
  the policy row removed from the group's `Targeted by`, a bare group GUID in an assignments block, a wrong
  `targetedBySha256`, a document outside the three lists touched (`os.Chtimes`), a `notifications` link with no
  `Used by` answer.~~
- ~~Tests, sweep fixture run (same skip): a `resources/` tree with `metadata.yaml` (`generatedAt`
  `2026-10-02T00:00:00Z`) holding a win32 app with `installCommandLine: setup.exe APITOKEN=<planted>`, an iOS
  custom profile whose plist carries `<key>RemoteOfficeAuthKey</key><string><planted></string>`, an update ring
  with both pause expiries inside 180 days, a Microsoft 365 group with `expirationDateTime` inside 180 days, a VPP
  token expiring inside 180 days (with its credential-family `doc-prompt.md`), a `tokenName` with a
  three-class value, and one unassigned assignment-capable resource. Assert the output reports the `APITOKEN`
  argument (rule e), the plist key (rule a), the VPP expiry and the unassigned resource; does not report the
  pause expiries, the group expiry or `tokenName`; and never contains either planted value.~~
- Documentation at *done*: `README.md` *Documentation generation*, items 6 and 7 of the agent's steps (the
  shipped reference-check and signal-sweep scripts; command-line credentials in the summary); `CHANGELOG.md`
  `### Changed`.
- Follow-up (found while implementing): `renderMigrate` prints only an `assignmentsSha256` column, so a document
  migrated for its noncompliance-notification markers never receives the `notificationsSha256` its work item
  carries — the agent cannot write it, the next run lists it as a notifications re-splice, and the section-6
  *Hashes updated* check has no column to verify. Render the `notificationsSha256` column in the `migrate`
  table (blank for assignments-only rows) and cover it in `generateprompt_test.go`.

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

*Area:* export & metadata · *Impact:* medium · *Effort:* S · *Ships with:* standalone; then turn the type's
metadata back into tenant-wide settings

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
settings documented or drift-tracked.

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
The prompt half is covered by the 2026-10-02 prompt review (rule T1 in go entry 1: documents never reprint values
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

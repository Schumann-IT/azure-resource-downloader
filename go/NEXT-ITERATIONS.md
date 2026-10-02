# Next iterations

Outstanding work and parked ideas for the Go CLI. `README.md` says what it does today and `CHANGELOG.md` what
shipped and why; neither is repeated here. How entries and ideas are written, promoted, implemented and
archived is `../.claude/rules/next-iterations.md`.

Numbered entries are scheduled work: committed here before they are implemented, struck through as they land,
and archived to `../.claude/archive/go/` once done. Parked ideas, grouped by area below, are
deliberately unscheduled; each says why it is parked and what would make it worth doing.

## 1. Consistent prompt templates, run-prompt fixes and the `summary:` frontmatter line

*Kind:* feat

**Goal.** Every documentation template follows one section order and one shape for its settings section, so a
reader finds the same thing in the same place for every resource type; the run prompt (`docs/generate.md`)
agrees with the templates, describes full and incremental runs alike, asks for the one frontmatter field the
browser is still missing and fixes link formatting — so the scheduled regeneration produces consistent documents
and lights up the sidebar's per-item context.

> Promoted from the parked idea *emit `summary:` in the generated document frontmatter*: the plumbing is complete
> on both sides (`docFrontmatter.Summary`, `GenerateIndex`, the browser's per-item context), the field is absent
> from every document only because the template never asks for it, and the web idea *per-item context in the sidebar*
> waits on it. `platformGroup` / `functionGroup`, already required but empty in the
> reference exports that predate them, fill in on the same regeneration.
>
> **Review findings (2026-10-01).** `generate_prompt_template.md:151-154` puts the source filename under the title
> while every type template puts the summary there (only 201 of 263 documents carry the line);
> `:143-145` says "a `Properties` or `Settings` section", forgetting `Definition`; documents mix bare URLs (~2800)
> and Markdown links (~1400) and do not link sibling documents.
>
> **Template review (2026-10-02, eight templates and the shared partials).** Section order differs (credential puts
> *Expiry and renewal* after *Lifecycle and operations*; every other type-specific section sits right after
> *References*); `group` has no *References* and no *Lifecycle and operations* although #53 gave it the curated
> links and lifecycle notes; `record` has no *Security* and therefore its own redaction variant. The settings
> section's bullets come in a different order everywhere and several are missing (complete-and-infer rule in
> credential, record and group; baseline rule in credential and record; referenced ids in group and arm; embedded
> payloads in singleton, record, group, arm and CA); the `data-setting` / `data-note` bullets are near-identical
> copies in all eight. No Lifecycle section tells the model to use the curated lifecycle notes; `prompt-related`
> says "cross-reference their YAML directories instead of guessing", contradicting the referenced-ids rule; CA
> repeats its no-assignments paragraph before the closed set; the metadata-table bullet is worded two ways. The run
> prompt is titled "Incremental documentation generation prompt" and calls every run incremental, though a run
> after a template change — like the coming regeneration — lists every resource.
>
> **Decision.** Section order: one order everywhere — *References* → type-specific section(s) → *Lifecycle and
> operations* → *Security* → settings section.
>
> **Decision.** Group: add both *References* and *Lifecycle and operations*.
>
> **Decision.** Record: add a *Security* section; the record-only redaction variant goes.
>
> **Decision.** Settings-section names: keep *Settings* (default, singleton, conditional access), *Properties*
> (credential, record, group, arm) and *Definition* (referenced).
>
> **Contract.** The heading sets after this entry (H2 names unchanged; only order and presence change):
> default `References | Lifecycle and operations | Security | Settings`; conditional access `References |
> Conditions | Lifecycle and operations | Security | Settings`; referenced `References | Usage and references |
> Lifecycle and operations | Security | Definition`; singleton `References | Lifecycle and operations | Security |
> Settings`; arm `References | Lifecycle and operations | Security | Properties`; credential `References | Expiry
> and renewal | Lifecycle and operations | Security | Properties`; record `References | Lifecycle and operations |
> Security | Properties`; group `References | Membership | Usage as assignment target | Lifecycle and operations |
> Security | Properties`. Every heading is one of the eleven the browser already styles (`References`,
> `Conditions`, `Membership`, `Usage as assignment target`, `Usage and references`, `Expiry and renewal`,
> `Lifecycle and operations`, `Security`, `Settings`, `Properties`, `Definition`); the browser keys on the text, not
> the order. Frontmatter: every document the run writes carries `summary: "<one sentence>"` — a double-quoted
> YAML string on one line, plain text (no Markdown, no links), taken from the summary paragraph; `docs
> generate-index` already copies it unchanged into `docs/index.yaml` `resources[].summary`, which the browser
> shows as plain, escaped text and treats absent or empty as "no summary". `data-setting` / `data-note`
> (`security` | `inert`) keep their meaning in every family, record included.
>
> **Regeneration-gated.** Every type's `promptSha256` moves; this entry rides the pending regeneration and is the
> last before it. The run prompt is not hashed, but no existing document gets `summary:` without regeneration.
> The run prompt's heading check reads each type's `<!-- doc-headings: … -->` line (`heading_contract`), so the new
> sets need no hard-coded change there. The other regeneration-gated parked ideas (*per-finding severity in
> document `Security` sections*, *bootstrap the curated taxonomy from per-document LLM suggestions*) stay out of
> this batch and wait for the next regeneration; *mask dedicated secret properties* joins one only if it
> extends the redaction rule.
>
> **Owner.** No tracked file outside `go/`. The last bullet writes the git-ignored
> `Claude outputs/prompt-review-instructions.md` at the repository root (never staged or committed). No
> sequencing with the web side: the paired web entry hard-codes the same heading sets and ships independently.
>
> **Implementer.** opus

**Plan.**

- Section order and presence per the Contract. `internal/handlers/graph/credential_prompt.tmpl`: move *Expiry and
  renewal* before *Lifecycle and operations*. `group_prompt.tmpl`: add *References* first (`prompt-references` +
  `prompt-url-rule`; drop the `prompt-url-rule` call from its *Properties* section) and *Lifecycle and
  operations* after *Usage as assignment target* (`prompt-lifecycle-rule`, plus: what deleting, restoring or
  converting the group between assigned and dynamic membership means). `record_prompt.tmpl`: add *Security*
  after *Lifecycle and operations* and move `prompt-change-role` and the exposed-credential call-out there,
  plus "call out security-sensitive properties". Update the `<!-- doc-headings: … -->` line of credential,
  group and record; the other five lines stay byte-identical.
- One settings-section shape in all eight templates (`internal/models/documentation_prompt.tmpl` and the seven
  `*_prompt.tmpl`), bullets in this order: (1) document every property the section covers (per-template scope
  sentence: CA "outside `conditions`", group "every remaining property"); (2) `prompt-key-settings` (CA keeps its
  "entries under `conditions`" bullet right after it); (3) the `<details>` format bullet, per template, ending
  with its existing example `<details data-setting="…">` path; (4) `prompt-details-attrs`; (5) type-specific bullets
  (default: the `@odata.type` subtype rule; referenced: the rule/filter-expression bullet); (6)
  `prompt-complete-rule`; (7) `prompt-baseline-rule`; (8) `prompt-settings-grouping`; (9) `prompt-referenced-ids`;
  (10) `prompt-embedded-payloads` (default keeps its notification-templates and sidecar-file bullets directly
  after it); (11) `prompt-present-rule`; (12) `prompt-masked-rule`; (13) `prompt-redaction-rule`.
- New partials in `internal/models/prompt_partials.tmpl`, each with neutral wording ("the block", "property"):
  `prompt-details-attrs` — open each block as `<details data-setting="<exact YAML path>">`, the same string the
  `<summary>` shows, never invented or abbreviated; `data-note="security"` when the property is one called out in
  the Security section (this includes a value redacted under the redaction rule), `data-note="inert"` when present
  but without effect because a gating setting is off, otherwise no attribute, no other value.
  `prompt-complete-rule` — "Do not omit a property this section covers; if one is unfamiliar, infer its meaning
  from the type's API schema (the schema reference listed above, where given) and say so explicitly."
  `prompt-present-rule` — "Only describe what is actually present; never invent values." `prompt-lifecycle-rule`
  — with `.Lifecycle`: build on the lifecycle notes listed above and add only what the YAML itself shows; without:
  state only what the YAML shows and say that deprecation or migration status is not documented here. Fold
  `prompt-redaction-lead` into `prompt-redaction-rule` (one partial, rendered text unchanged) and delete
  `prompt-redaction-rule-record`.
- `prompt-lifecycle-rule` is the first bullet of every *Lifecycle and operations* section (all eight templates);
  the type-specific bullets after it stay.
- `prompt-related`: the lead line becomes "Related resource types exported alongside this one (context only; a
  reference to one of them follows the referenced-object rule below):" — the "cross-reference their YAML
  directories instead of guessing" wording goes.
- `conditional_access_prompt.tmpl`: delete the "Targeting — the users, groups, roles, applications and conditions
  under `conditions` — belongs in the `Conditions` section; this document has no assignments block." paragraph
  before the closed set; the intro already says it.
- One metadata-table wording: credential, group, record, referenced, singleton and arm change "A metadata table
  stating …" to "Directly after the summary paragraph, a metadata table stating …" (default and CA already do).
- `internal/docs/generate_prompt_template.md`, run-wide wording: title "# Documentation generation prompt
  (template)"; the intro replaces "This is an **incremental** run" with: the work list is closed and covers
  either every resource (a first run, or a run after a template change moved every `promptSha256`) or only
  what changed — document exactly what it names; the frontmatter passage says "the next run" instead of "the next
  incremental run"; appendix D says the work list may be only the documents that changed. In
  `internal/docs/generateprompt.go` the two doc comments say "the documentation prompt" instead of "the
  incremental documentation prompt".
- `internal/docs/generate_prompt_template.md`, section 2: the spec paragraph reads "a `Settings`, `Properties` or
  `Definition` section"; *Headings* drops the "source YAML filename under the title" rule — the summary paragraph
  sits directly under the title as every spec says, and the source filename becomes a `Source` row of the
  metadata table; a *Links* rule: Markdown links `[label](url)` only, never a bare URL, and a link to another
  document is relative and only to a document whose path the run gives you (the work list, the reference maps);
  *Frontmatter*: the example gains `summary: "…"` after `functionGroup`, and a paragraph requires it — one
  sentence taken from the document's summary paragraph, plain text without Markdown or links, always a
  double-quoted YAML string on one line (`\"` for an inner quote), because an unquoted `: ` breaks the whole
  frontmatter and the document is then regenerated every run.
- `internal/docs/generate_prompt_template.md`, section 4: the *Frontmatter* row of the checks table adds "and a
  non-empty one-line double-quoted `summary`"; the Python check, in the frontmatter branch, fails a work-list
  document with "frontmatter missing summary" when there is no `^summary:` line, and with "frontmatter summary
  not a non-empty double-quoted line" when the value is not `"…"` with non-empty content or contains a Markdown
  link or backticks. Only work-list documents are checked, so documents retained from older runs are not failed.
- Tests, `internal/handlers/prompt_rules_test.go` (every registered type): parse the `doc-headings` line —
  first `References`; last one of `Settings`/`Properties`/`Definition`; second to last `Security`; third to last
  `Lifecycle and operations`; every heading in a Go list of the eleven headings named in the Contract (the
  browser's vocabulary, copied, not imported). Every prompt contains the `prompt-details-attrs`,
  `prompt-complete-rule`, `prompt-present-rule`, masked and redaction texts, exactly one of the two baseline-rule
  variants, and the `prompt-lifecycle-rule` text; none contains "cross-reference their YAML directories" or
  "(a record has no Security section)".
- Tests, intended assertion changes (planned, not weakened): `internal/models/documentation_test.go` —
  `promptPartialNames` drops `prompt-redaction-lead` and `prompt-redaction-rule-record` and adds the four new
  partials; a lifecycle-rule case with and without `Lifecycle`. `internal/handlers/graph/prompt_templates_test.go`
  — `TestRecordPromptTemplateRedaction` becomes a record *Security* test (asserts "call it out in the **Security**
  section", the standard `data-note` wording and the record heading set; asserts the Lifecycle-section wording is
  gone); `TestConditionalAccessPromptTemplate` replaces "belongs in the `Conditions` section" with the intro's
  "its targeting is documented in the `Conditions` section" and asserts "Targeting — the users" is absent;
  `TestGroupPromptTemplateHeader` also asserts "References:" and "Lifecycle and operations:" and the group heading
  set; a credential test asserts its heading set.
- Tests, run prompt: in `internal/docs/generateprompt_test.go` a test over `DefaultGeneratePromptTemplate()`
  asserts the "# Documentation generation prompt" title, no "This is an **incremental** run", the `summary:`
  requirement and its double-quote rule, the "`Settings`, `Properties` or `Definition`" wording, the bare-URL
  ban and the section-4 "frontmatter missing summary" check; `TestParseFrontmatter` gains a case where
  `summary: "Enforces X: requires Y"` parses into `Summary`.
- Goldens: `make -C go golden-update`, then review the diff per template family — only the 53
  `prompts/<type>/doc-prompt.md` files change, the exported-YAML goldens stay byte-identical; `make -C go test`.
- Documentation at *done*: `README.md` (the template-family table and the closed-H2 contract with the new sets, the
  run prompt described as full or incremental, the `summary:` frontmatter field written by the agent);
  `CHANGELOG.md` `### Changed` (consistent templates, the run prompt) and `### Added` (`summary:`, noting that it
  and `platformGroup` / `functionGroup` appear on the next regeneration), with **regenerate the documentation** in
  bold.
- Last, after every other bullet: write the Claude Cowork review brief
  `Claude outputs/prompt-review-instructions.md` at the repository root (git-ignored, never committed — input for
  a later Cowork session, not repository documentation). It tells Cowork to read the eight templates
  (`internal/models/documentation_prompt.tmpl` and the seven `*_prompt.tmpl`),
  `internal/models/prompt_partials.tmpl`, the run prompt
  `internal/docs/generate_prompt_template.md` and the 53 assembled prompts under
  `go/internal/pipeline/testdata/golden/prompts/<type>/doc-prompt.md` (byte-for-byte what an export's
  `resources/<type>/doc-prompt.md` holds, without customer data — nothing under `output/` is read or uploaded);
  to review consistency across templates and against the run prompt, **per resource type whether an aspect a
  reader needs is missing** (licensing or prerequisites, platform or OS limits, conflicts with other policy types,
  reporting and monitoring, service limits, migration or deprecation, settings that only act together), each
  checked against the type's Microsoft Learn pages with the source cited, instructions that invite guessing or
  boilerplate, and anything that would break the browser contract (closed H2 sets, markers, `data-setting` /
  `data-note`); and to return a change plan shaped like the metadata review (findings with severity wrong /
  missing / optional and a Learn source; ready-to-paste text per template and per type; *regeneration-gated*
  marked; a "your choices" table; no repository edits). Its result enters the backlog later, like the metadata
  review did. No README or CHANGELOG mention.

## Parked ideas

**Legend.** *Area* — **contract** (Go → web data on disk: `index.yaml`, `drift/`, frontmatter, section
headings), **templates** (documentation and analysis prompts; *regen-gated* when it moves `promptSha256`),
**export & metadata** (what `resource download` fetches and records), **drift & compare**, **navigation**
(sidebar, breadcrumbs, landing pages, search, routing), **document view** (how one article renders), **export
formats**, **platform rule** (a non-negotiable itself), **dependencies**, **housekeeping** (lint ledgers,
caching internals). *Impact* — operator value: high / medium / low. *Effort* — S (a day or less), M (one
branch), L (several branches or a design change).

**Ships together.**

1. **The pre-regeneration batch** (scheduled; the shared prompt partials, the per-handler metadata and the
   template content fixes with the CA template — paired with web *Style the Conditional Access `Conditions`
   section* — already shipped): *run-prompt fixes and `summary:`* is the last one, then one documentation
   regeneration for all of them. The two regen-gated ideas here (*per-finding severity*, *taxonomy bootstrap*)
   did not join it and wait for the next regeneration.
2. **The compare track** (cross-project, must): *`resource compare`* ships with web *Move the compare
   normalisation to the CLI*; web *manual pairing* and *one-sided resource* follow on the CLI's rule; *version the
   drift observation* rides the first drift contract change.
3. **Housekeeping** is opportunistic: a `gocognit` entry is paid off by whichever entry edits its function.
4. **Export & metadata follow-ups** from the metadata review are independent of each other; *export ADMX
   presentation values* and *Intune branding images* each need a re-baseline after shipping, so they pair well with
   a planned re-download; *mask dedicated secret properties* only joins a regeneration if it extends the prompt
   redaction rule instead of masking.

## Parked ideas — export & metadata

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

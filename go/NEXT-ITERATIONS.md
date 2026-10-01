# Next iterations

Outstanding work and parked ideas for the Go CLI. `README.md` says what it does today and `CHANGELOG.md` what
shipped and why; neither is repeated here. How entries and ideas are written, promoted, implemented and
archived is `../.claude/rules/next-iterations.md`.

Numbered entries are scheduled work: committed here before they are implemented, struck through as they land,
and archived to `../.claude/archive/go/` once done. Parked ideas, grouped by area below, are
deliberately unscheduled; each says why it is parked and what would make it worth doing.

## 1. Shared prompt partials, without changing a byte

*Kind:* refactor

**Goal.** The seven documentation prompt templates repeat the same blocks six or seven times (header, reference
links, `<details>` rules, redaction rule, closed-set paragraph), and they have already drifted apart (the group
template renders only one link). Move the shared text into partials so later template work changes one place —
while proving that this refactor moves no `promptSha256` and therefore forces no regeneration.

> **Why first.** The two regeneration-gated entries below (per-handler metadata, template content) both edit the
> shared blocks; doing it once in partials keeps them small and consistent. The golden test this entry adds is
> what makes their intended hash moves explicit and reviewable.
>
> **Not regeneration-gated** by construction: the rendered bytes stay identical, which the golden test proves.
>
> **Contract.** Every registered type's assembled `doc-prompt.md` — and therefore its `promptSha256` in
> `resources/metadata.yaml` — is byte-identical before and after this entry; the `doc-headings` and `doc-groups`
> marker lines that `docs generate-prompt` and the generation checker read are unchanged. Nothing the web
> browser reads (generated documents, `docs/index.yaml`, the H2 vocabulary) changes. The new partials are an
> internal Go contract with the entries that follow on this branch: *Correct and complete the per-handler
> documentation metadata* adds the "Admin center:" line to the shared links partial, and *Template content
> fixes and a Conditional Access template* edits the redaction, URL, key-settings and closed-set partials and
> gives the group template the full shared header.
>
> **Owner.** none — every file is under `go/`. Sequencing: first of the four go entries on
> `feat/pre-regeneration-batch`; the per-handler metadata and template content entries build on its partials
> and its golden prompt files, so it is implemented and committed before either starts. It is closed together
> with them on this branch.
>
> **Implementer.** opus

**Plan.**

- ~~Golden prompt test first, against the untouched templates: new `internal/pipeline/golden_prompt_test.go`
  (`TestGoldenDocPrompts`) builds `handlers.NewRegistry(offlineCredential{}, "00000000-0000-0000-0000-000000000000",
  false)`, iterates `GetAllTypes()` **sorted**, and compares
  `assembleDocPrompt(resourceType, h.GetDocumentationPrompt())` — the exact bytes the writer hashes into `promptSha256` — with
  `internal/pipeline/testdata/golden/prompts/<resource type>/doc-prompt.md` (the type path mirrors the export
  tree, e.g. `prompts/Microsoft.Graph/groups/doc-prompt.md`). Reuse `updateGoldenEnv`, `offlineCredential` and
  `firstDifference` from `golden_test.go`: with `UPDATE_GOLDEN` set it deletes and rewrites the `prompts/` tree;
  otherwise a missing golden file, a golden file for an unregistered type, or any byte difference fails with
  the type and first differing line. Generate the files with `make golden-update` **before any template edit**
  and keep them in the same diff; after the refactor `make test` must pass with no change under
  `testdata/golden/prompts/`. Full text, not hashes, so the later entries' deliberate changes are reviewable
  as diffs.~~
- ~~`go/Makefile`: the `golden-update` comment, echo lines and `help` text name both golden sets (exported YAML
  and documentation prompts); the recipe already runs `./internal/pipeline/` and needs no other change.~~
- ~~New `internal/models/prompt_partials.tmpl`, embedded in `internal/models/documentation.go` and parsed by
  `parsePromptTemplate` into every prompt template (default and each `Template` override) as a separate
  associated template (`tmpl.New("prompt-partials").Parse(…)`, after the main text) so the main body is never
  replaced; partial-only text stays out of the rendered output. Partials, each holding exactly the text the
  templates repeat today, with the call sites' `{{-` trimming kept so the output is unchanged:~~
  - ~~`prompt-type` — the `Azure resource type:` line and the optional `About this resource type:` block;~~
  - ~~`prompt-subtype`, `prompt-permissions`, `prompt-lifecycle`, `prompt-related` — the `SubtypeNote`,
    `RequiredPermissions`, `Lifecycle` and `RelatedTypes` blocks;~~
  - ~~`prompt-links` — the full `Reference material …` block (`EndpointDocs`, `SchemaReference`, `Permissions`,
    `BestPractices`), the one place a later link kind is added;~~
  - ~~`prompt-header` — `prompt-type`, `prompt-subtype`, `prompt-permissions`, `prompt-lifecycle`,
    `prompt-links`, `prompt-related` in that order (used by the default, ARM, credential, record, referenced
    and singleton templates);~~
  - ~~`prompt-key-settings` — the `{{- if .KeySettings }}` "give particular attention to" bullet;~~
  - ~~`prompt-url-rule` — the "Use real, verifiable URLs …" bullet (six templates);~~
  - ~~`prompt-masked-rule` — the "Where a value is masked or redacted by the service …" bullet (all seven);~~
  - ~~`prompt-redaction-rule` — the credential-shaped redaction bullet (six templates; record has none today);~~
  - ~~`prompt-closed-set` — the "These H2 headings are a closed set …" paragraph (all seven).~~
- ~~Rewrite `internal/models/documentation_prompt.tmpl`, `internal/handlers/graph/{credential,group,record,
  referenced,singleton}_prompt.tmpl` and `internal/handlers/arm/arm_prompt.tmpl` to call those partials. The
  group template calls `prompt-type`, `prompt-permissions` and `prompt-lifecycle` and keeps its own one-link
  `EndpointDocs` block inline (it has no `SubtypeNote`, `RelatedTypes` or other links today; adopting
  `prompt-header` there is a content change for the template content entry, not this one). Stay per template:
  the persona line, the "The configuration is provided …" layout paragraph, section headings and bodies, the
  `<details>` intro, `data-setting` example and `data-note` lines (their wording differs per type, so sharing
  them would move bytes), and the `doc-headings` line. No helper is added to the `FuncMap`.~~
- ~~Doc comments in `internal/models/documentation.go` (`parsePromptTemplate`, `DefaultDocumentationPromptTemplate`,
  `ResourceDocumentation.Template`) and in `internal/handlers/graph/prompt_templates.go` /
  `internal/handlers/arm/prompt_templates.go` state that every prompt template is parsed with the shared
  partials and may call them; the default template's text now contains `{{ template … }}` calls and is only
  usable through the `Template` field.~~
- ~~Models tests (`internal/models/documentation_test.go`): an override that calls every partial executes without
  error; an override that calls none renders exactly as before; a `Template` override cannot break the partials
  for the next type (each parse starts from a fresh template set). Existing per-template tests stay green
  unchanged.~~
- Documentation at *done*: `CHANGELOG.md` gets nothing of its own — the entry is closed together with the
  per-handler metadata, template content and run-prompt entries on `feat/pre-regeneration-batch`, whose
  `[Unreleased]` lines satisfy the branch gate. `README.md`: the developer section's `make golden-update` line
  and the "Dependency updates are proven byte-neutral" paragraph also name the documentation-prompt golden
  files (`testdata/golden/prompts/`) and that a deliberate template change updates them in the same diff.

## 2. Correct and complete the per-handler documentation metadata

*Kind:* fix

**Goal.** Every handler's `models.ResourceDocumentation` is accurate and complete: the right read permission, an
API reference for the API version the handler actually calls, a link to the permissions page, a deep link to the
admin center blade where the resource is managed, and type-specific best-practice links — guarded by a test so
it cannot silently decay again.

> **Review findings (2026-10-01, 51 handlers).** `EndpointDocs` is set everywhere; `BestPractices` on 7;
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
> (`internal/audit/route.go`), so the two permission fixes change what an operator is told to consent.
>
> **Decision.** Link kinds: fill the existing `Links.Permissions` on every handler and add a new
> `Links.AdminCenter`; no PowerShell link.
>
> **Regeneration-gated.** Every changed value moves the type's `promptSha256`. Batch with *Template content fixes
> and a Conditional Access template* and *Run-prompt fixes and the `summary:` frontmatter line* so they share the
> one scheduled regeneration. Depends on *Shared prompt partials* (the new link renders from the shared partial).
>
> **Implementer.** sonnet

**Plan.**

- Permissions: `mobileThreatDefenseConnectors` and `intuneBrandingProfiles` → `DeviceManagementServiceConfig.Read.All`,
  including their error-hint text; verify every other handler's `RequiredPermissions` against its Learn "Get"/"List"
  page and correct what differs.
- Links: `namedLocations`, `termsOfUseAgreements` → `?view=graph-rest-beta`; `deviceManagement` → the root
  `deviceManagement` entity page (the settings complex type may stay as a second reference); every
  `learn.microsoft.com/en-us/mem/intune/...` → its current `/intune/intune-service/...` path; `BestPractices` made
  type-specific, generic duplicates removed rather than kept.
- `Links.Permissions` on all handlers: the Learn page of the "Get"/"List" operation, which lists scopes and roles.
- New `Links.AdminCenter` field (`internal/models/documentation.go`), rendered by the shared links partial as
  "Admin center:" so every template shows it; set only where a stable Intune / Entra / Azure portal blade URL
  exists, empty otherwise — never guessed.
- Registry-wide test over `NewRegistry`: every handler has `EndpointDocs`, `RequiredPermissions` and
  `Links.Permissions`; every link is `https://` on `learn.microsoft.com` or an allowed portal host
  (`intune.microsoft.com`, `entra.microsoft.com`, `portal.azure.com`); a Graph link's `?view=` matches the SDK
  client the handler uses (beta vs v1.0). Update the golden prompt test deliberately.
- Documentation at *done*: `README.md` "Supported resource types" permissions column and the dedicated-app scope
  list; `CHANGELOG.md` (`### Fixed` for the permissions, `### Changed` for the richer per-type references).

## 3. Template content fixes and a Conditional Access template

*Kind:* feat

**Goal.** The documentation prompts describe each type as it really is and stop inviting guesses: Conditional
Access gets its own template built around its conditions, mismatched types get the right template, the shared
rules are consistent, and the instructions that produced invented links, recommendations and boilerplate are
replaced by evidence-bound ones — so the scheduled regeneration yields better documents, not just new ones.

> **Review findings (2026-10-01, 7 templates, 411 generated documents in two tenants).** The heading contract and
> frontmatter hold everywhere. Problems:
> - *Fact mismatches.* Conditional Access uses the generic template's group-only assignments table although CA
>   targets users, roles and apps through `conditions.*`; its 37 documents carry assignments markers that are
>   never resolved or re-spliced. `roleScopeTags` has `hasAssignments: true` but gets the referenced template
>   ("no assignments of its own"), so the block landed under an H2. `reusablePolicySettings` is a referenced
>   object but gets the policy template.
> - *Inconsistencies.* Six overrides say "assignment information belongs in the assignments block above" without
>   having one; `KeySettings` lands in Settings, Security or Lifecycle depending on the template; the group
>   template drops `SubtypeNote`, `RelatedTypes` and all links but `EndpointDocs`; the record template lacks the
>   credential-redaction rule; the credential template's Lifecycle duplicates its Expiry section.
> - *Guess- and boilerplate-inducing instructions.* A mandatory link per setting plus the "flag as approximate"
>   escape (116 of 263 documents carry approximate URLs); a "recommended value" for every setting without a cited
>   baseline; a forced review cadence (178 of 263); purpose inferred from names; "search sibling directories"
>   prose the spliced *Targeted by* / *Used by* blocks have replaced.
> - *Missing guidance.* Nothing tells the model what to do with `RequiredPermissions` (2 documents mention a
>   scope); no change-role / least-privilege statement; assignment-table columns and intent unspecified (tables
>   differ); dependencies (filters, scope tags, named locations, strengths) not asked for by ID with links; large
>   settings-catalog payloads not grouped (largest document 2394 lines, ~27 H3s across 263 documents).
>
> **Decision.** Conditional Access: a dedicated template with a `Conditions` section (not a splice fix inside the
> generic template).
>
> **Contract.** The new H2 is exactly `Conditions` (slug `conditions`); the web entry *Style the Conditional Access
> `Conditions` section* adds it to the browser's section vocabulary. No other H2 is added or renamed.
>
> **Regeneration-gated.** Moves `promptSha256` for every type whose template changes (all but record types where
> nothing changes). Batch with *Correct and complete the per-handler documentation metadata* and *Run-prompt fixes
> and the `summary:` frontmatter line*; ship together with the web entry; depends on *Shared prompt partials*.
>
> **Implementer.** opus

**Plan.**

- New `internal/handlers/graph/conditional_access_prompt.tmpl` for `conditionalAccessPolicies`: targeting is
  documented from `conditions.*` (users, groups, roles, applications, locations, platforms, client apps, risk) in a
  `Conditions` section, with no group-only assignments block; IDs are resolved only through the existing reference
  maps and an unresolved role or application id is stated as such, never named. `doc-headings: References |
  Conditions | Lifecycle and operations | Security | Settings`. Check how the CA handler's `hasAssignments` and the
  assignments splice interact so CA documents no longer carry unresolved markers.
- Template selection: `reusablePolicySettings` → referenced template; `roleScopeTags` keeps the referenced template
  but, having `hasAssignments: true`, gets the assignments block above the first H2 (a referenced-template variant
  or a template flag, whichever keeps the partials simple).
- Closed-set paragraph per template names only the blocks that template defines.
- Consistency: `KeySettings` always in the settings-like section; group template gets the full header (links,
  `SubtypeNote`, `RelatedTypes`); record template gets the redaction rule; credential Lifecycle drops what Expiry
  and renewal covers.
- Evidence-bound instructions: a reference link per setting only when a specific page is known, otherwise none —
  no "approximate" links; the curated `Links` (incl. Admin center) must appear under References; a recommended
  value only where a cited `BestPractices` baseline covers the setting; review cadence only for types with expiry or
  renewal; the summary states purpose from the resource's own `description` and settings, or says it is not
  documented — never inferred from the name; drop the "search sibling directories" usage prose in favour of the
  spliced *Targeted by* / *Used by* blocks.
- New guidance: Security names the read permission (`RequiredPermissions`) and the admin role needed to change the
  resource (least privilege); assignment tables use the fixed columns `Direction | Target | Intent | Filter
  (mode)`; referenced objects (assignment filters, scope tags, named locations, authentication strengths,
  notification templates) are documented by id with a relative link to their document; large payloads are grouped
  under H3s by category.
- Tests: golden prompt test updated deliberately; template selection for CA, `roleScopeTags`,
  `reusablePolicySettings`; the CA `doc-headings` line; existing per-template tests kept green.
- Documentation at *done*: `README.md` (documentation section: the CA template and its `Conditions` section, the
  templates' evidence rules); `CHANGELOG.md` `### Changed`, with **regenerate the documentation** in bold.

## 4. Run-prompt fixes and the `summary:` frontmatter line

*Kind:* feat

**Goal.** The run prompt (`docs/generate.md`) agrees with the type templates, asks for the one frontmatter field the
browser is still missing, and fixes link formatting — so the scheduled regeneration also lights up the sidebar's
per-item context.

> Promoted from the parked idea *emit `summary:` in the generated document frontmatter*: the plumbing is complete
> on both sides (`docFrontmatter.Summary`, `GenerateIndex`, the browser's per-item context), the field is absent
> from every document only because the template never asks for it, and the web idea *a name filter and per-item
> context in the sidebar* waits on it. `platformGroup` / `functionGroup`, already required but empty in the
> reference exports that predate them, fill in on the same regeneration.
>
> **Review findings (2026-10-01).** `generate_prompt_template.md:151-154` puts the source filename under the title
> while every type template puts the summary there (only 201 of 263 documents carry the line);
> `:143-145` says "a `Properties` or `Settings` section", forgetting `Definition`; documents mix bare URLs (~2800)
> and Markdown links (~1400) and do not link sibling documents.
>
> **Regeneration-gated in effect.** `generate_prompt_template.md` is not hashed, but no existing document gets the
> new frontmatter without regeneration. Batch with *Correct and complete the per-handler documentation metadata*
> and *Template content fixes and a Conditional Access template*.
>
> **Implementer.** sonnet

**Plan.**

- `internal/docs/generate_prompt_template.md`: require `summary:` in the frontmatter — one sentence, taken from the
  document's summary paragraph, plain text; Markdown links only, and relative links to sibling documents under
  `docs/`; the source filename goes into the metadata table, not under the title; the settings-like section reads
  "`Settings`, `Properties` or `Definition`"; the Python heading check includes the Conditional Access heading set.
- Tests: the rendered `generate.md` contains the `summary:` rule and the CA heading set (existing generate-prompt
  render tests).
- Documentation at *done*: `README.md` (frontmatter fields written by the agent); `CHANGELOG.md` `### Added`
  (`summary:`), noting that it and `platformGroup` / `functionGroup` appear on the next regeneration.

## Parked ideas

**Legend.** *Area* — **contract** (Go → web data on disk: `index.yaml`, `drift/`, frontmatter, section
headings), **templates** (documentation and analysis prompts; *regen-gated* when it moves `promptSha256`),
**export & metadata** (what `resource download` fetches and records), **drift & compare**, **navigation**
(sidebar, breadcrumbs, landing pages, search, routing), **document view** (how one article renders), **export
formats**, **platform rule** (a non-negotiable itself), **dependencies**, **housekeeping** (lint ledgers,
caching internals). *Impact* — operator value: high / medium / low. *Effort* — S (a day or less), M (one
branch), L (several branches or a design change).

**Ships together.**

1. **The pre-regeneration batch** (scheduled): *Shared prompt partials* first, then *per-handler metadata*,
   *template content fixes and a CA template* and *run-prompt fixes and `summary:`* together with web *Style the
   Conditional Access `Conditions` section* — one documentation regeneration for all of them. The two
   regen-gated ideas here (*per-finding severity*, *taxonomy bootstrap*) either join that batch, decided before
   *per-handler metadata* starts, or wait for the next regeneration.
2. **The compare track** (cross-project, must): *`resource compare`* ships with web *Move the compare
   normalisation to the CLI*; web *manual pairing* and *one-sided resource* follow on the CLI's rule; *version the
   drift observation* rides the first drift contract change.
3. **Housekeeping** is opportunistic: a `gocognit` entry is paid off by whichever entry edits its function.

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

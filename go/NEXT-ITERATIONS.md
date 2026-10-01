# Next iterations

Outstanding work and parked ideas for the Go CLI. `README.md` says what it does today and `CHANGELOG.md` what
shipped and why; neither is repeated here. How entries and ideas are written, promoted, implemented and
archived is `../.claude/rules/next-iterations.md`.

Numbered entries are scheduled work: committed here before they are implemented, struck through as they land,
and archived to `../.claude/archive/go/` once done. Parked ideas, grouped by area below, are
deliberately unscheduled; each says why it is parked and what would make it worth doing.

## 1. Correct and complete the per-handler documentation metadata

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
> **Regeneration-gated.** Every changed value moves the type's `promptSha256`; the new `Links.Permissions` line moves
> it for every type. Batch with *Template content fixes and a Conditional Access template* and *Run-prompt fixes and
> the `summary:` frontmatter line* so they share the one scheduled regeneration. Builds on the shared prompt
> partials (done; the new link goes into `prompt-links`).
>
> **Contract.** No Go → web artefact changes shape: no frontmatter key, H2 heading, `doc-headings` /
> `doc-groups` marker, export path or `metadata.yaml` field is added or renamed. After the regeneration every
> document carries a new `promptSha256` (the browser shows it as is) and its References section may cite the
> permissions page and an admin-center link on `intune.microsoft.com`, `entra.microsoft.com` or
> `portal.azure.com` — plain Markdown links the browser already renders. `web/` needs no change.
>
> **Owner.** none — every file touched is under `go/`. Sequencing: first of the regeneration batch on this branch;
> *Template content fixes and a Conditional Access template* relies on the `Links.AdminCenter` field and the
> "Admin center:" line this entry adds.
>
> **Implementer.** sonnet

**Plan.**

- ~~Accessor first, bytes unchanged: a `models.Documented` interface (`Documentation() models.ResourceDocumentation`,
  `AzureType` filled in) in `internal/models/types.go`, implemented by `GraphCollectionHandler` and the three ARM
  handlers (`internal/handlers/arm/*.go`: move the inline `ResourceDocumentation` literal into `Documentation()`);
  each `GetDocumentationPrompt` becomes `models.BuildDocumentationPrompt(h.Documentation())`. `make -C go test`
  must pass with the golden prompts untouched before any metadata changes.~~
- ~~`Links.AdminCenter string` in `models.ResourceLinks` (`internal/models/documentation.go`, doc comment: the admin
  center blade where the resource is managed; empty when no verified deep link exists). `prompt-links` in
  `internal/models/prompt_partials.tmpl`: add `.Links.AdminCenter` to the block's `or` condition and render
  `- Admin center: <url>` after the `- Required permissions:` line; update the `Template` field's doc comment if it
  lists the partials' content. A `models` unit test renders a `ResourceDocumentation` with only `AdminCenter` set
  and with all link kinds set, and asserts the block appears with the lines in that order.~~
- ~~Permissions: `mobileThreatDefenseConnectors` (`mobilethreatdefenseconnector.go`) and `intuneBrandingProfiles`
  (`intunebrandingprofile.go`) → `DeviceManagementServiceConfig.Read.All`, in `RequiredPermissions` and in every
  `hint: requires '…'` string of the file. Then check every other handler against the permission source rule in
  the Notes (fetch each operation's Learn page); correct only a declared scope the page does not list, together
  with its hints. A corrected scope outside the current set (`Policy.Read.All`, `Group.Read.All`,
  `Agreement.Read.All`, `Organization.Read.All`, `OrganizationalBranding.Read.All`,
  `OnPremDirectorySynchronization.Read.All`, `DeviceManagement*`) must also be added to `entraPermissions` in
  `internal/audit/route.go` (`TestRouteEveryRegisteredType` fails otherwise) and named in the implementation
  report as a new consent. A page that cannot be fetched leaves the value as is and is listed in the report as
  unverified — never guessed.~~
- ~~Links, Graph `?view=` per the client the handler builds: `namedLocations` (`namedlocation.go`) and
  `termsOfUseAgreements` (`termsofuseagreement.go`) → `?view=graph-rest-beta` (both call `newBetaGraphClient`).
  Every `learn.microsoft.com/en-us/mem/intune/...` URL → its current `learn.microsoft.com/en-us/intune/
  intune-service/...` page (follow the redirect, use the final URL).~~
- `deviceManagement` (`devicemanagementsettings.go`): `EndpointDocs` → the beta `deviceManagement` entity resource
  page; the `deviceManagementSettings` complex-type page moves to `SchemaReference`. No other `SchemaReference` is added. Open: Microsoft Learn has no
  `deviceManagement` entity page (the Graph docs hold no `resources/…devicemanagement` and no `GET /deviceManagement`
  page, and the guessed names return 404), so `EndpointDocs` still points at the complex-type page and
  `SchemaReference` stays empty; its `Links.Permissions` is the Graph permissions reference. Revisit when such a page
  appears, or decide to keep it as is.
- ~~`BestPractices`: keep a link only where the page is about this type's feature (a planning, hardening or baseline
  guide for it); remove generic ones (e.g. `protect/security-baselines` on `deviceConfigurations`, which security
  baselines do not use); add one only where such a page exists. Empty is acceptable.~~
- ~~`Links.Permissions` on all 53 types: for Graph, the Learn page of the List (collection) or Get (singleton)
  operation the handler calls, at its API version (`?view=` as above); for the three ARM types, the Azure built-in
  *Reader* role section on Learn.~~
- ~~`Links.AdminCenter` where a deep link is verified: the blade URL cited by a Learn page for that feature, or one
  confirmed to open the blade, on `intune.microsoft.com` (Intune types), `entra.microsoft.com` (Entra types) or
  `portal.azure.com` (ARM types); empty otherwise. The implementation report lists the types left empty.~~
- ~~Registry-wide test `internal/handlers/documentation_metadata_test.go` over `NewRegistry(stubCredential{}, …,
  false)`: every handler implements `models.Documented`; `EndpointDocs`, `RequiredPermissions` and
  `Links.Permissions` are non-empty; `EndpointDocs`, `SchemaReference`, `Permissions` and `BestPractices` are
  `https://learn.microsoft.com/en-us/…`; `AdminCenter` is empty or `https://` on one of the three portal hosts; no
  link contains `/mem/intune/`; no `BestPractices` entry repeats within a type. Source scan in the same test, per
  non-test file of `internal/handlers/graph/` holding an `azureType: "…"` literal: the file uses the beta client
  (`newBetaGraphClient(` or the `msgraph-beta-sdk-go` import) xor v1.0 (`newGraphClient(` or `msgraph-sdk-go`),
  and every `/graph/api/` link of that type carries the matching `?view=graph-rest-beta` / `?view=graph-rest-1.0`;
  every scope named in a `requires '…'` hint is in that type's `RequiredPermissions`. Every registered Graph type
  is matched by exactly one file.~~
- ~~Golden prompts, deliberately: `make -C go golden-update`, then review the diff — only files under
  `internal/pipeline/testdata/golden/prompts/` change (the exported-YAML goldens stay byte-identical), and each
  type's diff is confined to its permission lines and its *Reference material* block (`groups`: only its
  permission lines, if any, since its template renders no links block yet).~~
- Documentation at *done*: `README.md` "Supported resource types" — move the `mobileThreatDefenseConnectors` and
  `intuneBrandingProfiles` rows to the *enrollment, Autopilot and tenant* (`DeviceManagementServiceConfig.Read.All`)
  table and reflect any other corrected scope there and in the dedicated-app scope list; `CHANGELOG.md`
  (`### Fixed` for the permissions, `### Changed` for the richer per-type references).
- Open follow-up: no verified `Links.AdminCenter` deep link exists for any of the 53 types. The Microsoft Learn
  Intune, Entra and Azure documentation cite only the portal roots (and a few blades for other features), so
  every `AdminCenter` stays empty and no "Admin center:" line is rendered yet. Add values per type when a Learn
  page or a confirmed blade URL is available. `windowsAutopilotDeploymentProfiles` has no List page on Learn (its
  Permissions link is the Get page).

## 2. Template content fixes and a Conditional Access template

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
> and the `summary:` frontmatter line*; ship together with the web entry; builds on the shared prompt partials (done).
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

## 3. Run-prompt fixes and the `summary:` frontmatter line

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

1. **The pre-regeneration batch** (scheduled, on the shared prompt partials already shipped): *per-handler metadata*,
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

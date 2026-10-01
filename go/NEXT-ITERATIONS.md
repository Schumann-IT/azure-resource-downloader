# Next iterations — deliberately out of scope

Outstanding work and parked ideas for the Go CLI. Each numbered entry is a unit of planned work: it is written
and committed here before it is implemented, its plan items are struck through as they land, and **once it is
done it is archived** — moved with its full plan to `../.claude/archive/go/`, so the *how* survives for later
review while `CHANGELOG.md` records the what and why. Ideas that are deliberately not scheduled collect under
*Parked ideas* at the end, so they persist as the entries around them ship. `README.md` stays the single source
of truth for what the tool *does today*.

## 1. Shared prompt partials, without changing a byte

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
> **Implementer.** sonnet

**Plan.**

- Move the duplicated blocks of `internal/models/documentation_prompt.tmpl` and the overrides
  (`internal/handlers/graph/*_prompt.tmpl`, `internal/handlers/arm/arm_prompt.tmpl`) into `{{define}}` partials,
  parsed together with each template in `parsePromptTemplate` (`internal/models/documentation.go:82`). Per-type
  differences (persona, layout, section bodies, `doc-headings`) stay in each template.
- A golden test over every handler registered by `NewRegistry` pins the assembled `doc-prompt.md` bytes (or their
  SHA-256) per type, captured before the refactor; the refactor must leave all of them unchanged. The golden file
  stays, so a later deliberate change updates it in the same diff.
- Documentation at *done*: none in `README.md` (no user-visible change). `CHANGELOG.md`: nothing of its own — but
  the branch gate refuses an entry archived as done unless `[Unreleased]` grew on the branch, so close it on the
  same branch as a regeneration-gated entry whose changelog line covers it, or add one line under the release
  workflow area.

## 2. Correct and complete the per-handler documentation metadata

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

Deliberately not scheduled — kept here rather than in a work entry so they survive as the entries around them
ship and are archived. Each records why it is parked and what would make it worth doing.

### Idea: version the drift observation, and name `drift/` a Go → web contract

`drift/metadata.yaml`, the payloads at `drift/<key>.yaml`, the analysis prompt and the agent-written
`drift/<key>.md` and `drift/index.md` are read by the browser, joined by the shared `<type>/<name>` key,
gated on `baseline.generatedAt` and verified by hash — exactly the shape of the `index.yaml` contract, with
the same obligations: a change to the observation schema, to the per-finding frontmatter (`verdict`,
`severity`, `observedAt`, `baselineGeneratedAt`) or to the index's `severities:` line is a cross-project
change. The observation carries a `toolVersion` but no schema `version:` — the field `index.yaml` learned to
need. **Not planned — parked deliberately**: nothing has broken, and adding the field alone is cheap but
pointless until a consumer branches on it.

**Revisit when** the observation schema or the per-finding frontmatter next changes for another reason: add
`version: 1` to `drift/metadata.yaml` in that same change, have the browser accept `>= 1`, and name `drift/`
beside `index.yaml` in both projects' rules as a versioned contract. Related, web-only: the drift index
table uses `high / medium / low / info` while the tenant summary's Findings table uses
`critical / high / medium` (see the web backlog's *Fixes*).

### Idea: routine dependency updates, and consolidating on one Microsoft Graph SDK

Two related pieces of dependency hygiene. First, a routine update pass (`go get -u`, `make deps`, `make check`)
over the direct dependencies. Second, investigate whether the module can carry **one** Microsoft Graph SDK
instead of two. The direction is the opposite of the intuitive one: `msgraph-beta-sdk-go` is imported by 53
handler files and serves the Intune/device-management endpoints that **do not exist on v1.0**, so it can never
be the one removed. The candidate for removal is `msgraph-sdk-go` (v1.0), used by one shared client constructor
and seven handlers (conditional access, groups, organization, authentication methods/strengths, authorization
policy, on-premises synchronization) — the beta endpoint is a superset, so those seven could move. The prize is
real: the two generated SDKs dominate compile time and binary size, and one of them is nearly gone already.
**Not planned — parked deliberately**, for three reasons:

- **A dependency bump is not hash-neutral by construction.** A Graph SDK update can change which properties a
  model carries and therefore the YAML bytes the export writes — moving `sourceSha256` for resources that did
  not change in the tenant, which reads as mass drift and forces documentation regeneration. `resource drift`
  is the verification tool: "update, run drift against a fixture tenant, it must report zero findings" is the
  standard check, so a bump is provable content-neutral before it lands.
- **Moving the seven v1.0 types to beta trades a stability contract for uniformity.** v1.0 responses are
  contractually stable; beta responses may change shape at Microsoft's discretion. Today the most stable,
  most-referenced types (groups, conditional access) deliberately sit on the stable endpoint. Consolidation
  buys shorter builds but makes every type's bytes hostage to beta churn.
- **The switch itself moves hashes once.** Re-fetching those seven types through the beta endpoint will change
  their YAML (beta models carry extra properties), so their `sourceSha256` values move and their documents
  regenerate — a one-time cost that should ride a regeneration scheduled for another reason, not force its own.

**Revisit when** a security advisory forces an SDK bump anyway, or build time becomes a felt cost. If
consolidation is picked up:
move the seven handlers one at a time, verify each with a fixture-tenant drift run, expect and batch the
one-time hash movement, and only then drop the v1.0 module. If beta churn is the worry instead, the same
investigation can conclude the opposite consolidation is wiser once Microsoft ports the Intune endpoints to
v1.0 — check that first; it would remove the *beta* SDK and the churn with it.

### Idea: per-finding severity in document `Security` sections

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

### Idea: resolve Graph object ids to names inside the exported YAML

Add a transformer that resolves Microsoft Graph object ids that appear in a resource — assignment `groupId`s,
filter ids, `notificationTemplateId`s — to their display names at export time, as the `id-resolution`
transformer already does offline for ARM resource ids (which carry their name in the id itself). The YAML
would then read `groupId: 8964516b-… (GBL_D_WIN_...)` instead of a bare GUID. **Not planned — parked
deliberately**, for three reasons:

- **The documentation already resolves them, and does so incrementally.** `docs generate-prompt` builds the
  group, filter and template reference maps from `metadata.yaml`, renders every assignments / "Targeted by" /
  "Used by" block from them, and re-splices exactly those blocks when a referenced object is renamed — without
  touching the resource's own document or its YAML. Resolving in the YAML would duplicate that with a worse
  failure mode.
- **It would put a decision into a fact.** A resource's YAML and its `sourceSha256` are meant to move only when
  the resource itself changes. Embedding another object's *current* name makes every policy's hash move when a
  group is renamed, which forces regenerating every document that assigns it — the exact cascade the marked
  splice blocks exist to avoid.
- **It costs one extra Graph read per referenced id**, on every run, for information the export already holds
  once (in the group's own YAML).

**Revisit only if** a consumer other than the documentation pipeline needs names inside the YAML itself — e.g.
a diff/review workflow on `resources/` that cannot read `metadata.yaml`. If promoted, resolve from the export
(the already-downloaded groups/filters/templates), never from a live lookup, and write the name into a sidecar
`_name` key the way `id-resolution` does — never in place of the id.

### Idea: `resource compare` — an offline comparison of two exports, and the home of the cross-tenant identity rule

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

**Relation to the idea above.** This does **not** trigger *resolve Graph object ids to names inside the
exported YAML*: the comparison reads names from `metadata.yaml`, which is exactly the consumer that idea says
does not need them in the YAML. The facts stay facts.

### Idea: bootstrap the curated taxonomy from per-document LLM suggestions

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

### Idea: review the sign-in surface — can a scoped `az login` replace the dedicated-app device-code path?

The tool carries two sign-in paths. The default reuses the `az login` session; `--client-id`/`--tenant-id`
starts a device-code sign-in against a dedicated app registration. The second path exists for exactly one
reason: `az account get-access-token` always mints tokens for the Azure CLI *first-party* app — regardless of
how the user logged in — and that app's Graph token has lacked the delegated scopes most Graph handlers
declare (`DeviceManagementConfiguration.*`, `DeviceManagementApps.*`, `DeviceManagementScripts.*`,
`Policy.Read.All`, …). But `az login` accepts `--scope`: signing in with
`az login --scope https://graph.microsoft.com/.default` (or explicit scopes) asks Entra to add delegated
Graph scopes to that same CLI session — the README's own troubleshooting hint already leans on it for the
"required scopes are missing" failure. If a scoped login reliably lands every declared scope in the session's
Graph token, the entire second path becomes removable: the device-code credential branch, the two flags and
their env/config equivalents, the `PermissionScoped` probe (`RequiresDedicatedApp` /
`DedicatedAppRequirements`), the interactive dedicated-app prompt, and the app-registration setup in
`README.md` — collapsing authentication to one path and one instruction. Even a partial "yes" has value: the
dedicated-app prompt could recommend the exact scoped re-login first and fall back to device code only when
the CLI app genuinely cannot obtain a scope. **Not planned — parked deliberately**, for three reasons:

- **The answer is not in this repository.** Whether a scope lands in the CLI token's `scp` claim depends on
  the tenant's consent policy and on which scopes Microsoft lets its first-party app request — both outside
  the tool's control and changeable by Microsoft without notice. Only a live-tenant experiment settles it:
  perform a scoped `az login`, decode the token's `scp`, verify that azidentity's CLI credential (which
  shells out to `az account get-access-token` per request) actually surfaces the scopes granted at login,
  then run a full download of every dedicated-app-gated type — including `--resolve-secrets`, which needs
  `DeviceManagementConfiguration.ReadWrite.All`.
- **A positive result on one tenant does not generalize.** Consent policies differ per tenant, Microsoft has
  been progressively hardening what the first-party CLI app may do, and some services may gate on the calling
  application rather than the token's scopes alone — only a live call against each gated endpoint settles
  that. The dedicated app registration is the escape hatch the operator controls; deleting it trades
  resilience for a smaller surface.
- **Removal is a breaking change to the auth surface** — the flags, their `AZURE_RD_*` variables and config
  keys — so it should ride a major, not a hygiene pass.

**Revisit when** an operator confirms on a representative tenant that a scoped `az login` yields every
permission the handlers declare, or the next time the authentication surface is reworked anyway. If promoted,
soften before deleting: first teach the dedicated-app prompt to recommend the exact `az login --scope …`
command derived from the selected types' declared permissions, keeping device code as the fallback; only
retire the flags once the CLI path has covered every `PermissionScoped` type against a live tenant, and
record the removal under `Breaking`.

### Idea: clear the `gocognit` baseline

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
mean it has started collecting new debt instead of recording old. If picked up, work one function at a time with
`make test-race` where the function touches the pipeline, keep the tests that pin the invariants above unchanged
(a refactor that needs a test edited is a redesign, not a split), and remember that a stale entry is invisible —
golangci-lint does not report an exclusion that matched nothing, so re-measure by commenting the block out. Each
split is internal and needs no `CHANGELOG.md` entry; deleting the block at the end does.


---
title: Template content fixes and a Conditional Access template
project: go
status: done
started: 2026-10-01
finished: 2026-10-02
branch: feat/ca-template-and-conditions
pr: 53
changelog: Unreleased
---
## Template content fixes and a Conditional Access template

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
> **Already addressed on main (per-handler metadata, shared partials).** `Links.Permissions` on every type and
> `Links.AdminCenter` on 17 (rendered as `- Admin center:` by `prompt-links`), corrected Purpose / KeySettings /
> Lifecycle / RelatedTypes, `organizationalBranding` already on the singleton template, `EmbeddedPayloads` now
> saying which payloads the export decodes. Still open in the code (checked 2026-10-02): `group_prompt.tmpl`
> renders only `prompt-type`, `-permissions`, `-lifecycle` and the `EndpointDocs` link; `prompt-key-settings` sits
> in Settings (default), Security (credential, group, referenced, singleton, arm) or Lifecycle (record);
> `record_prompt.tmpl` lacks `prompt-redaction-rule`; `prompt-closed-set` names "the assignments block above" in all
> seven templates; `prompt-url-rule` still allows "approximate" links; `EmbeddedPayloads` is rendered only by the
> default template (so `depOnboardingSettings` on the credential template never sees its payload line).
>
> **Scope choices (plan review, 2026-10-02).** (1) The default template's "decode and pretty-print" line is
> rewritten to match the metadata: a payload the export already decoded (inline or a sidecar file) is documented
> as decoded; one still encoded is decoded only when it is text (XML, JSON, script), while binary content
> (certificates, images) is stated as present, never reprinted — it rides this batch at no extra regeneration
> cost. (2) Referenced objects in a document are named and linked only through a map the run provides (today:
> groups via the refmap, notification templates via the usedbymap); everything else stays a bare id — a general
> id → document map for filters, scope tags, named locations and strengths is out of scope. (3) Assignment tables
> use the run's splice columns, not new ones. (4) The group template keeps its four H2s; with no `References`
> section its curated links are model input only.
>
> **Contract.** The Go → web artefact is the H2 heading set a document carries, declared per type in the
> `<!-- doc-headings: … -->` line of its `doc-prompt.md` and written verbatim by the agent. This entry adds one
> heading set, for `Microsoft.Graph/conditionalAccessPolicies` only:
> `References | Conditions | Lifecycle and operations | Security | Settings`. The new H2 text is exactly
> `Conditions` (the browser slugs it `conditions`); it holds targeting prose and tables (`conditions.*`) and no
> `<details data-setting>` blocks, which stay in `Settings`. CA documents carry no `<!-- assignments:… -->`
> markers. Every other type keeps its heading set unchanged (default / singleton `References | Lifecycle and
> operations | Security | Settings`, arm `… | Properties`, group `Membership | Usage as assignment target |
> Security | Properties`, credential adds `Expiry and renewal`, record `References | Lifecycle and operations |
> Properties`, referenced `References | Usage and references | Lifecycle and operations | Security | Definition`),
> including `roleScopeTags` and `reusablePolicySettings`, which move to the referenced set. Marker names and the
> `data-setting` / `data-note` attributes are unchanged.
>
> **Regeneration-gated.** Moves `promptSha256` for every type: the shared `prompt-url-rule` and `prompt-closed-set`
> change in all seven templates, record types included (only three are documented — `deviceCategories`,
> `mobileThreatDefenseConnectors`, `ndesConnectors`; Autopilot device identities never are, so no bulk
> regeneration). Rides one regeneration with *Run-prompt fixes and the `summary:` frontmatter line* (implemented
> after this entry: its Python heading check needs the CA heading set above). The parked regen-gated ideas
> *per-finding severity* and *taxonomy bootstrap* do not ride along — neither revisit condition holds (the heading
> contract has not yet survived a real regeneration; no taxonomy at a scale where cold-authoring is the
> bottleneck). Ships with the web entry *Style the Conditional Access `Conditions` section*.
>
> **Owner.** none — every file is under `go/`. No sequencing constraint with the web side (the web change is
> harmless before the regeneration); within go, the run-prompt entry follows this one.
>
> **Implementer.** opus

**Plan.**

- ✅ `models.ResourceDocumentation` gains `HasAssignments bool`, filled by `GraphCollectionHandler.Documentation()`
  from `h.hasAssignments` (ARM handlers leave it false), so templates branch on the same fact the export records
  in `metadata.yaml` instead of a second per-handler flag. Extract the default template's assignments bullet into
  a partial `prompt-assignments` (the explanation, the table built with the run's splice columns
  `Direction | Target | Filter | Intent` — `Intent` only for apps, empty columns dropped — bare group GUIDs, never
  invented names) and call it from the default template and, under `{{ if .HasAssignments }}`, the referenced
  template.
- ✅ New `internal/handlers/graph/conditional_access_prompt.tmpl`, embedded in `conditionalaccesspolicy.go` the way
  `group.go` embeds its template, wired through `Template`; `hasAssignments` stays false. Layout: title, summary,
  metadata table (type, id, `state`, created / modified) and no assignments block or markers. `Conditions`:
  a `Condition | Include | Exclude` table with one row per `conditions.*` dimension present (users, groups, roles,
  guests or external users, applications / user actions / authentication contexts, platforms, locations, client
  app types, device filter, sign-in / user / service-principal risk, authentication flows), well-known values
  (`All`, `GuestsOrExternalUsers`, `Office365`, `AllTrusted`) verbatim, every GUID bare unless a map the run
  provides resolves it — a role, application, named location or authentication strength id is never named from
  memory — then a short prose statement of who and what is in scope. `Settings`: every property outside
  `conditions.*` (`state`, `grantControls`, `sessionControls`, timestamps …) as `<details data-setting>` blocks,
  with the KeySettings bullet. `Security`: report-only vs enforced, whether any user or group is excluded (without
  claiming an exclusion is an emergency-access account), grant and session controls that weaken or strengthen
  access. `<!-- doc-headings: References | Conditions | Lifecycle and operations | Security | Settings -->`.
- ✅ Template selection: `reusablePolicySettings` → `referencedPromptTemplateText` (no `hasAssignments`);
  `roleScopeTags` keeps the referenced template and, through `HasAssignments`, gets the `prompt-assignments` block
  above the first H2 and an intro that no longer claims "no assignments of its own".
- ✅ `prompt-closed-set` mentions "assignment information belongs in the assignments block above" only under
  `{{ if .HasAssignments }}`; the CA template says targeting belongs in `Conditions`; every template keeps the
  forbidden-heading list.
- ✅ Consistency: `prompt-key-settings` called from the settings-like section of every template (`Settings`,
  `Properties` or `Definition`); `group_prompt.tmpl` uses `{{ template "prompt-header" . }}` (subtype, links incl.
  Admin center, related types); `record_prompt.tmpl` gets the redaction rule in a record wording — the exposed
  credential is called out in `Lifecycle and operations` (record has no `Security` H2) and its data-note bullet
  allows `security` for the redacted block only; a new `prompt-embedded-payloads` partial (the rewritten line from
  scope choice 1) is called from the settings-like section of the default, referenced and credential templates;
  credential `Lifecycle and operations` keeps deprecation / migration / deletion and leaves expiry, renewal and
  cadence to `Expiry and renewal`.
- ✅ Evidence-bound instructions, in every template: `prompt-url-rule` becomes "link a setting only to a specific page
  you know; otherwise give no link — never an approximate or guessed URL"; `References` must list the curated
  `Links` from the header (API reference, schema, permissions, Admin center, baselines) where the template has that
  section; a recommended / best-practice value only where a listed `BestPractices` baseline covers the setting
  (otherwise the body documents the configured value only); "recommended review cadence" removed everywhere except
  `Expiry and renewal`; the summary states purpose from the resource's own `description` and settings or says the
  purpose is not documented — never inferred from the display name (group: drop "likely purpose").
- ✅ Reverse references: the group `Usage as assignment target` section says the tool-spliced `Targeted by` block
  lists every assigning resource and asks only for the rename / delete impact; the referenced `Usage and
  references` section explains how referencing policies use the object and what breaks on deletion — no "search
  sibling directories" prose and no hand-made list of referencing resources in either.
- ✅ New guidance: `Security` (record: `Lifecycle and operations`) names the read permission from the header's
  `RequiredPermissions`, and the least-privileged role able to change the resource only when the curated metadata
  or the linked permissions page supports it — otherwise it says the change role is not documented here; referenced
  objects (assignment filters, `roleScopeTagIds`, named locations, authentication strengths, notification
  templates, reusable settings) are documented by id, named and linked only per scope choice 2; when `Settings`
  would hold more than 30 top-level blocks, group them under H3s by the leading segment of the setting path
  (settings catalog: the `settingDefinitionId` category prefix), never by invented themes.
- ✅ Tests: `prompt_templates_test.go` — CA renders the `Conditions:` section, the exact CA `doc-headings` line and no
  assignments instruction; `reusablePolicySettings` and `roleScopeTags` render `Usage and references:`, and only
  `roleScopeTags` the assignments instruction; group renders `- Admin center:` and `Related resource types`; record
  renders the redaction placeholder. A registry-wide test in `internal/handlers`: the assignments instruction
  appears iff `HasAssignments()`, every type with `EmbeddedPayloads` renders them, no prompt contains
  "approximate" or "search sibling", and only credential prompts contain "review cadence". Golden prompts rewritten with
  `make -C go golden-update` and the diff reviewed type by type; `internal/handlers/arm/prompt_templates_test.go`
  and `internal/models/documentation_test.go` kept green.
- ✅ Documentation at *done*: `README.md` (documentation section: the CA template and its `Conditions` section, the
  templates' evidence rules); `CHANGELOG.md` `### Changed`, with **regenerate the documentation** in bold.

---
title: Consistent prompt templates, run-prompt fixes and the `summary:` frontmatter line
project: go
status: done
started: 2026-10-01
finished: 2026-10-02
branch: feat/prompt-consistency
pr: 54
changelog: 0.4.0
---
## Consistent prompt templates, run-prompt fixes and the `summary:` frontmatter line

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
> (`security` | `inert`) keep their meaning in every family, record included. After the prompt review:
> `deviceComplianceScripts` documents use the referenced set and their `docs/index.yaml` rows carry no
> `assignments` summary (`hasAssignments: false`); `deviceCategories`, `mobileThreatDefenseConnectors` and
> `ndesConnectors` documents gain `platformGroup` / `functionGroup`; no heading or marker name changes.
>
> **Regeneration-gated.** Every type's `promptSha256` moves; this entry rides the pending regeneration and is the
> last before it. The run prompt is not hashed, but no existing document gets `summary:` without regeneration.
> The run prompt's heading check reads each type's `<!-- doc-headings: … -->` line (`heading_contract`), so the new
> sets need no hard-coded change there. The other regeneration-gated parked ideas (*per-finding severity in
> document `Security` sections*, *bootstrap the curated taxonomy from per-document LLM suggestions*) stay out of
> this batch and wait for the next regeneration; *mask dedicated secret properties* joins one only if it
> extends the redaction rule.
>
> **Prompt review (2026-10-02).** A Claude Cowork review of the harmonised templates, the shared partials, the run
> prompt and the 53 assembled prompts against Microsoft Learn (`Claude outputs/documentation-prompt-review.md`,
> git-ignored; built on `5a0ad8d`). Four problems matter before the regeneration: credential-shaped values under
> credential-named keys and the plaintext OMA-URI values `resolve-secrets` writes fall outside the redaction rule
> (T1, R7); the assignments explanation can land inside the splice markers and is lost, the CA *Conditions* table
> gets wrapped too, and nothing checks for missing markers (T4, R1, R2, R4); every *Lifecycle and operations*
> section asks questions the curated notes cannot answer (T2); decoded scripts are never required verbatim (T6,
> R12). Three templates invite false claims (credential renewal pitfall and lead time T7/T8; ARM locks, backup and
> identities the export never carries T13; group `department`/`jobTitle` called user-editable T9). Per type: 227
> Learn-backed findings (13 wrong, 141 missing, 73 optional). Its four patches —
> `documentation-prompt-review-{templates,run-prompt,metadata,tests}.patch` in `Claude outputs/` — apply together to
> `5a0ad8d` and are the implementer's input; the commit diff is the record. The prompts grow by about 25 %.
>
> **Decision.** C1 when the gated changes ship: with this entry, before the one regeneration.
>
> **Decision.** C2 plaintext OMA-URI secrets: redact them in the documents (in the patches); the export keeps them.
>
> **Decision.** C3 the `deviceConfigurations` permission line: a static line naming both scopes and when each
> applies, now; the parked idea *relax secret resolution to a read scope* stays for later.
>
> **Decision.** C4 optional per-type findings: apply all 227.
>
> **Decision.** C5 `deviceComplianceScripts`: the referenced template with `hasAssignments: false`, still fetching
> `/assignments` and warning when it returns entries.
>
> **Decision.** C6 doc-groups for record types: drop `OmitGroupAxes` for `deviceCategories`,
> `mobileThreatDefenseConnectors` and `ndesConnectors`; keep it for `windowsAutopilotDeviceIdentities`.
>
> **Decision.** C7 procedural links: label them *Microsoft guidance*; the baseline rule applies only where a listed
> page states a recommended value.
>
> **Decision.** C8 per-setting links: only to pages listed under *References*, anchors included.
>
> **Decision.** C9 `windowsAutopilotDeviceIdentities`: stays out of documentation runs; its metadata stays current.
>
> **Decision.** C10 change roles: type by type, named only where the type's notes give one.
>
> **Decision.** C11 Conditional Access targets: resolved in the run later — the parked idea *resolve Conditional
> Access targets in the documentation run*.
>
> **Owner.** No tracked file outside `go/`. The last bullet writes the git-ignored
> `Claude outputs/prompt-review-instructions.md` at the repository root (never staged or committed). No
> sequencing with the web side: the paired web entry hard-codes the same heading sets and ships independently.
>
> **Implementer.** opus

**Plan.**

- ✅ Section order and presence per the Contract. `internal/handlers/graph/credential_prompt.tmpl`: move *Expiry and
  renewal* before *Lifecycle and operations*. `group_prompt.tmpl`: add *References* first (`prompt-references` +
  `prompt-url-rule`; drop the `prompt-url-rule` call from its *Properties* section) and *Lifecycle and
  operations* after *Usage as assignment target* (`prompt-lifecycle-rule`, plus: what deleting, restoring or
  converting the group between assigned and dynamic membership means). `record_prompt.tmpl`: add *Security*
  after *Lifecycle and operations* and move `prompt-change-role` and the exposed-credential call-out there,
  plus "call out security-sensitive properties". Update the `<!-- doc-headings: … -->` line of credential,
  group and record; the other five lines stay byte-identical.
- ✅ One settings-section shape in all eight templates (`internal/models/documentation_prompt.tmpl` and the seven
  `*_prompt.tmpl`), bullets in this order: (1) document every property the section covers (per-template scope
  sentence: CA "outside `conditions`", group "every remaining property"); (2) `prompt-key-settings` (CA keeps its
  "entries under `conditions`" bullet right after it); (3) the `<details>` format bullet, per template, ending
  with its existing example `<details data-setting="…">` path; (4) `prompt-details-attrs`; (5) type-specific bullets
  (default: the `@odata.type` subtype rule; referenced: the rule/filter-expression bullet); (6)
  `prompt-complete-rule`; (7) `prompt-baseline-rule`; (8) `prompt-settings-grouping`; (9) `prompt-referenced-ids`;
  (10) `prompt-embedded-payloads` (default keeps its notification-templates and sidecar-file bullets directly
  after it); (11) `prompt-present-rule`; (12) `prompt-masked-rule`; (13) `prompt-redaction-rule`.
- ✅ New partials in `internal/models/prompt_partials.tmpl`, each with neutral wording ("the block", "property"):
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
- ✅ `prompt-lifecycle-rule` is the first bullet of every *Lifecycle and operations* section (all eight templates);
  the type-specific bullets after it stay.
- ✅ `prompt-related`: the lead line becomes "Related resource types exported alongside this one (context only; a
  reference to one of them follows the referenced-object rule below):" — the "cross-reference their YAML
  directories instead of guessing" wording goes.
- ✅ `conditional_access_prompt.tmpl`: delete the "Targeting — the users, groups, roles, applications and conditions
  under `conditions` — belongs in the `Conditions` section; this document has no assignments block." paragraph
  before the closed set; the intro already says it.
- ✅ One metadata-table wording: credential, group, record, referenced, singleton and arm change "A metadata table
  stating …" to "Directly after the summary paragraph, a metadata table stating …" (default and CA already do).
- ✅ `internal/docs/generate_prompt_template.md`, run-wide wording: title "# Documentation generation prompt
  (template)"; the intro replaces "This is an **incremental** run" with: the work list is closed and covers
  either every resource (a first run, or a run after a template change moved every `promptSha256`) or only
  what changed — document exactly what it names; the frontmatter passage says "the next run" instead of "the next
  incremental run"; appendix D says the work list may be only the documents that changed. In
  `internal/docs/generateprompt.go` the two doc comments say "the documentation prompt" instead of "the
  incremental documentation prompt".
- ✅ `internal/docs/generate_prompt_template.md`, section 2: the spec paragraph reads "a `Settings`, `Properties` or
  `Definition` section"; *Headings* drops the "source YAML filename under the title" rule — the summary paragraph
  sits directly under the title as every spec says, and the source filename becomes a `Source` row of the
  metadata table; a *Links* rule: Markdown links `[label](url)` only, never a bare URL, and a link to another
  document is relative and only to a document whose path the run gives you (the work list, the reference maps);
  *Frontmatter*: the example gains `summary: "…"` after `functionGroup`, and a paragraph requires it — one
  sentence taken from the document's summary paragraph, plain text without Markdown or links, always a
  double-quoted YAML string on one line (`\"` for an inner quote), because an unquoted `: ` breaks the whole
  frontmatter and the document is then regenerated every run.
- ✅ `internal/docs/generate_prompt_template.md`, section 4: the *Frontmatter* row of the checks table adds "and a
  non-empty one-line double-quoted `summary`"; the Python check, in the frontmatter branch, fails a work-list
  document with "frontmatter missing summary" when there is no `^summary:` line, and with "frontmatter summary
  not a non-empty double-quoted line" when the value is not `"…"` with non-empty content or contains a Markdown
  link or backticks. Only work-list documents are checked, so documents retained from older runs are not failed.
- ✅ Tests, `internal/handlers/prompt_rules_test.go` (every registered type): parse the `doc-headings` line —
  first `References`; last one of `Settings`/`Properties`/`Definition`; second to last `Security`; third to last
  `Lifecycle and operations`; every heading in a Go list of the eleven headings named in the Contract (the
  browser's vocabulary, copied, not imported). Every prompt contains the `prompt-details-attrs`,
  `prompt-complete-rule`, `prompt-present-rule`, masked and redaction texts, exactly one of the two baseline-rule
  variants, and the `prompt-lifecycle-rule` text; none contains "cross-reference their YAML directories" or
  "(a record has no Security section)".
- ✅ Tests, intended assertion changes (planned, not weakened): `internal/models/documentation_test.go` —
  `promptPartialNames` drops `prompt-redaction-lead` and `prompt-redaction-rule-record` and adds the four new
  partials; a lifecycle-rule case with and without `Lifecycle`. `internal/handlers/graph/prompt_templates_test.go`
  — `TestRecordPromptTemplateRedaction` becomes a record *Security* test (asserts "call it out in the **Security**
  section", the standard `data-note` wording and the record heading set; asserts the Lifecycle-section wording is
  gone); `TestConditionalAccessPromptTemplate` replaces "belongs in the `Conditions` section" with the intro's
  "its targeting is documented in the `Conditions` section" and asserts "Targeting — the users" is absent;
  `TestGroupPromptTemplateHeader` also asserts "References:" and "Lifecycle and operations:" and the group heading
  set; a credential test asserts its heading set.
- ✅ Tests, run prompt: in `internal/docs/generateprompt_test.go` a test over `DefaultGeneratePromptTemplate()`
  asserts the "# Documentation generation prompt" title, no "This is an **incremental** run", the `summary:`
  requirement and its double-quote rule, the "`Settings`, `Properties` or `Definition`" wording, the bare-URL
  ban and the section-4 "frontmatter missing summary" check; `TestParseFrontmatter` gains a case where
  `summary: "Enforces X: requires Y"` parses into `Summary`.
- ✅ Goldens: `make -C go golden-update`, then review the diff per template family — only the 53
  `prompts/<type>/doc-prompt.md` files change, the exported-YAML goldens stay byte-identical; `make -C go test`.
- ✅ Apply the prompt review: `git apply` the four patches in `Claude outputs/` —
  `documentation-prompt-review-templates.patch`, `-run-prompt.patch`, `-metadata.patch`, `-tests.patch` (templates
  and partials T1–T16 and T20–T23, run prompt R1–R10, R12–R13 with five new §4 checks, the per-type metadata of all
  53 handlers, the intended test-assertion changes plus eleven new assertions). They were built on `5a0ad8d`
  (`HEAD` differs from it only in this file); apply all four from the repository root, before every other bullet
  below, and stop and report if `git apply --check` fails. Read the test changes like any other test edit: each keeps its
  assertion's intent.
- ✅ After the patches (every snippet anchor below was checked against the patched tree), C5 —
  `internal/handlers/graph/devicecompliancescript.go`: `hasAssignments: false` and `Template:
  referencedPromptTemplateText` in its `documentation` literal. Keep the `/assignments` fetch and keep
  `item.SetAssignments(…)`, so the exported YAML and drift stay byte-identical; when the fetch returns one or more
  entries, log a warning through a new helper in `collection.go`, `warnUnexpectedAssignments(resourceType, itemID
  string, count int)` (`logger.Default.Warn`, "assignments returned for a type without an assignments concept;
  exported but not documented", fields `type`, `id`, `count`). Nothing else changes for it: `metadata.yaml` records
  `hasAssignments: false` on the next download, `docs generate-prompt` stops emitting its `assignmentsSha256` and
  migrate rows, the summary drops scripts from the assignment posture, `docs generate-index` writes no `assignments`
  for them, and `prompt_rules_test.go` follows `HasAssignments()` by itself. Tests: in `devicecompliancescript_test.go`
  `HasAssignments()` is false and the prompt carries the referenced heading set (`References | Usage and references |
  Lifecycle and operations | Security | Definition`); `TestReferencedPromptTemplateAssignments` gains a
  `deviceComplianceScripts` case with `hasAssignments: false`; a helper test captures `logger.Default.SetOutput`
  (restored in `t.Cleanup`, not parallel) and asserts the warning for `count` 2 and silence for 0.
- ✅ C6: delete the `OmitGroupAxes: true` line in `devicecategory.go`, `mobilethreatdefenseconnector.go` and
  `ndesconnector.go` (`windowsautopilotdeviceidentity.go` keeps it); reword the `OmitGroupAxes` field comment and the
  "Record types opt out" comment in `internal/models/documentation.go` to the one remaining case (a type kept out of
  documentation runs). Test in `internal/handlers/graph/prompt_templates_test.go`: the three prompts contain
  `DocumentationGroupsMarker()`, the `windowsAutopilotDeviceIdentities` prompt does not.
- ✅ C7: in `internal/models/prompt_partials.tmpl` `prompt-links` "- Best-practice baseline: {{ . }}" → "- Microsoft
  guidance: {{ . }}"; `prompt-references` "best-practice baselines)" → "Microsoft guidance)"; `prompt-baseline-rule`
  "…only where a best-practice baseline listed above covers the setting, and name that baseline;" → "…only where a
  Microsoft guidance page listed above states one for the setting, and name that page;" and "- No best-practice
  baseline is listed for this type:" → "- No Microsoft guidance is listed for this type:". "deviations from the
  best-practice baselines listed above" → "deviations from a value that a Microsoft guidance page listed above
  recommends" in `documentation_prompt.tmpl`, `conditional_access_prompt.tmpl`, `referenced_prompt.tmpl`,
  `singleton_prompt.tmpl` and `arm_prompt.tmpl`; and, so no template still names the old label, "the best-practice
  pages listed above" / "a best-practice page listed above" → "the Microsoft guidance pages listed above" / "a
  Microsoft guidance page listed above" in the two *Expiry and renewal* bullets of `credential_prompt.tmpl`. After
  this no `*.tmpl` contains "best-practice". Tests (intended assertion changes, same intent): in
  `internal/models/documentation_test.go` the three expected "- Best-practice baseline: " lines and both baseline
  cases of the lifecycle/baseline table; in `internal/handlers/prompt_rules_test.go` the `baselineListed` /
  `baselineUnlisted` constants; and `TestDocumentationPromptSectionShape` asserts no prompt contains
  "best-practice" (case-insensitive).
- ✅ C8: `prompt-url-rule` becomes "- Use real, verifiable URLs; link a setting only to a page listed under References
  (an anchor on one is fine), otherwise give it no link — never a recalled, guessed or merely nearby URL."; add
  "link a setting only to a page listed under References" to the `required` list of
  `TestDocumentationPromptSectionShape`.
- ✅ The optional snippets: T17 — `documentation_prompt.tmpl` *Security*: drop "conditional-access conditions, " from
  the example list; T18 — `prompt-closed-set` last sentence: "The comment lines below record this heading list (and,
  where present, the grouping vocabulary) for the documentation pipeline; do not copy them into the document.";
  T19 — `credential_prompt.tmpl`, `referenced_prompt.tmpl`, `singleton_prompt.tmpl`: directly after `{{ template
  "prompt-details-attrs" . }}` the bullet "- If the YAML carries an `@odata.type`, first identify the concrete
  subtype and document against that subtype's schema." (a test in `prompt_templates_test.go` asserts it in one prompt
  of each of the three families); R11 — `generate_prompt_template.md` §2 *Grouping*: "(its `@odata.type`,
  `platforms`, what the settings do, and the scope token in its name only where those don't decide it)", with an
  assertion in `generateprompt_test.go`.
- ✅ C3: the documented permission line becomes static while the run's own scope keeps following `resolve-secrets` —
  `RequiredPermissions()` also feeds the dedicated-app consent prompt (`DedicatedAppRequirements`) and `debug token`'s
  coverage report, which must not ask for `ReadWrite.All` when secrets are not resolved. `GraphCollectionHandler`
  gains `runtimePermissions []string` (doc comment: the delegated scopes a run needs when they differ from the
  documented list); `RequiredPermissions()` returns it when set, else `documentation.RequiredPermissions`.
  `deviceconfiguration.go` sets `runtimePermissions: []string{requiredPermission}` (unchanged logic) and
  `RequiredPermissions: []string{"DeviceManagementConfiguration.Read.All", "DeviceManagementConfiguration.ReadWrite.All
  (only with resolve-secrets: true, to resolve encrypted OMA-URI values to plaintext)"}` — `Read.All` first and
  verbatim, so the metadata test's `requires '…'` hint check, probe grouping (`PermissionGroup` of the first scope)
  and audit routing (Intune prefix) are unaffected. Tests in `deviceconfiguration_test.go`: the existing runtime cases
  stay; new: `Documentation().RequiredPermissions` and `GetDocumentationPrompt()` are identical for `resolveSecrets`
  false and true.
- ✅ `make -C go fmt`, `make -C go golden-update` (exactly the 53 prompt goldens change, no `*.golden.yaml`), review the
  per-type golden diff for wording slips (the review expects a few), then `make -C go test` and `make -C go check`.
- ✅ Documentation at *done*: `README.md` (the template-family table and the closed-H2 contract with the new sets, the
  run prompt described as full or incremental, the `summary:` frontmatter field written by the agent);
  `CHANGELOG.md` `### Changed` (consistent templates, the run prompt) and `### Added` (`summary:`, noting that it
  and `platformGroup` / `functionGroup` appear on the next regeneration), with **regenerate the documentation** in
  bold.
  Also from the prompt review: `README.md` *Security notes* — the credential-shaped free-text bullet now also covers
  values under credential-named keys and, with `resolve-secrets`, the resolved OMA-URI values (documents never
  reprint them; the export still holds them); the template-family table moves `deviceComplianceScripts` to the
  referenced family; the run prompt's new §4 checks. `CHANGELOG.md` `### Changed` names the redaction, the
  assignment-marker checks and the Learn-checked per-type notes, under the same **regenerate the documentation**.
- ✅ Last, after every other bullet: write the Claude Cowork review brief
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

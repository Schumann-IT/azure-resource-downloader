---
title: Shared prompt partials, without changing a byte
project: go
status: done
started: 2026-10-01
finished: 2026-10-01
branch: feat/pre-regeneration-batch
pr: 51
changelog: Unreleased
---
## Shared prompt partials, without changing a byte

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

- ✅ Golden prompt test first, against the untouched templates: new `internal/pipeline/golden_prompt_test.go`
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
  as diffs.
- ✅ `go/Makefile`: the `golden-update` comment, echo lines and `help` text name both golden sets (exported YAML
  and documentation prompts); the recipe already runs `./internal/pipeline/` and needs no other change.
- ✅ New `internal/models/prompt_partials.tmpl`, embedded in `internal/models/documentation.go` and parsed by
  `parsePromptTemplate` into every prompt template (default and each `Template` override) as a separate
  associated template (`tmpl.New("prompt-partials").Parse(…)`, after the main text) so the main body is never
  replaced; partial-only text stays out of the rendered output. Partials, each holding exactly the text the
  templates repeat today, with the call sites' `{{-` trimming kept so the output is unchanged:
  - `prompt-type` — the `Azure resource type:` line and the optional `About this resource type:` block;
  - `prompt-subtype`, `prompt-permissions`, `prompt-lifecycle`, `prompt-related` — the `SubtypeNote`,
    `RequiredPermissions`, `Lifecycle` and `RelatedTypes` blocks;
  - `prompt-links` — the full `Reference material …` block (`EndpointDocs`, `SchemaReference`, `Permissions`,
    `BestPractices`), the one place a later link kind is added;
  - `prompt-header` — `prompt-type`, `prompt-subtype`, `prompt-permissions`, `prompt-lifecycle`,
    `prompt-links`, `prompt-related` in that order (used by the default, ARM, credential, record, referenced
    and singleton templates);
  - `prompt-key-settings` — the `{{- if .KeySettings }}` "give particular attention to" bullet;
  - `prompt-url-rule` — the "Use real, verifiable URLs …" bullet (six templates);
  - `prompt-masked-rule` — the "Where a value is masked or redacted by the service …" bullet (all seven);
  - `prompt-redaction-rule` — the credential-shaped redaction bullet (six templates; record has none today);
  - `prompt-closed-set` — the "These H2 headings are a closed set …" paragraph (all seven).
- ✅ Rewrite `internal/models/documentation_prompt.tmpl`, `internal/handlers/graph/{credential,group,record,
  referenced,singleton}_prompt.tmpl` and `internal/handlers/arm/arm_prompt.tmpl` to call those partials. The
  group template calls `prompt-type`, `prompt-permissions` and `prompt-lifecycle` and keeps its own one-link
  `EndpointDocs` block inline (it has no `SubtypeNote`, `RelatedTypes` or other links today; adopting
  `prompt-header` there is a content change for the template content entry, not this one). Stay per template:
  the persona line, the "The configuration is provided …" layout paragraph, section headings and bodies, the
  `<details>` intro, `data-setting` example and `data-note` lines (their wording differs per type, so sharing
  them would move bytes), and the `doc-headings` line. No helper is added to the `FuncMap`.
- ✅ Doc comments in `internal/models/documentation.go` (`parsePromptTemplate`, `DefaultDocumentationPromptTemplate`,
  `ResourceDocumentation.Template`) and in `internal/handlers/graph/prompt_templates.go` /
  `internal/handlers/arm/prompt_templates.go` state that every prompt template is parsed with the shared
  partials and may call them; the default template's text now contains `{{ template … }}` calls and is only
  usable through the `Template` field.
- ✅ Models tests (`internal/models/documentation_test.go`): an override that calls every partial executes without
  error; an override that calls none renders exactly as before; a `Template` override cannot break the partials
  for the next type (each parse starts from a fresh template set). Existing per-template tests stay green
  unchanged.
- ✅ Documentation at *done*: `README.md` developer section — the `make golden-update` line and the byte-neutral
  paragraph name the documentation-prompt golden files (`testdata/golden/prompts/`) and the shared partials;
  `CHANGELOG.md` `### Added` (Release workflow): documentation prompts pinned byte for byte, no regeneration
  needed. Closed on its own (not with the regeneration-gated entries), so it carries its own changelog line.

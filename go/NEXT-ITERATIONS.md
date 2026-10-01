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

## 5. Routine dependency updates with a byte-neutrality guard, and Dependabot

**Goal.** Every direct Go dependency is brought up to date — proven not to change a single exported byte — and
every later update becomes the same non-event: Dependabot proposes it, CI proves it, the operator merges it. Along
the way, find out whether one of the two Microsoft Graph SDKs could be dropped, without dropping it yet.

> **Why.** Dependencies were last updated by hand on 2026-09-18 (`73daed9`); nothing automates updates and nothing
> proves one is safe. A Graph SDK bump can change the exported YAML: `serializeParsableToMap`
> (`internal/handlers/graph/collection.go:180-198`) writes the SDK model with the Kiota JSON writer, the pipeline
> cleans, decodes and sorts it, `MarshalResourceYAML` (`internal/pipeline/nameplanner.go:21`) writes it, and
> `sourceSha256` is the hash of those bytes (`internal/pipeline/writer.go:285-296`) — so a changed model moves the
> hash of resources that did not change in the tenant, which reads as mass drift and forces a regeneration. There
> are no golden or fixture tests today (handler tests build models with setters), so neutrality could only be shown
> by diffing two live exports.
>
> **What is true now (2026-10-01).** Go `1.26.0`; `msgraph-sdk-go v1.103.0`, `msgraph-beta-sdk-go v0.166.0`,
> `kiota-abstractions-go v1.11.0`, `kiota-serialization-json-go v1.1.4`, `azcore v1.23.1`, `azidentity v1.14.1`,
> `azlogs v1.2.0`, `armcompute/v5 v5.7.0`, `armresources v1.2.0`, `armsubscriptions v1.3.0`, `armstorage v1.8.1`,
> `cobra v1.10.2`, `pflag v1.0.10`, `viper v1.21.0`, `charmbracelet/log v1.0.0`, `yaml.v3 v3.0.1`. `make deps` only
> downloads and tidies; no Makefile target upgrades, and raw `go get` is off-limits (Makefile targets only). The
> v1.0 SDK serves exactly seven handlers (conditional access, groups, organization, authentication methods and
> strengths, authorization policy, on-premises sync) plus `GraphErrorCode` in `internal/azure/errors.go`. There is
> no `dependabot.yml`, and both branch gates refuse a branch that touches the project without a backlog change — so
> a Dependabot pull request could never pass the required `branch-ready-*` checks. `go/README.md:90` and the root
> `README.md:58,109` still say Go 1.24.
>
> **How production parses and transforms (what the golden test reuses).** Building any Graph client registers the
> Kiota JSON parse-node factory in `serialization.DefaultParseNodeFactoryInstance` and wraps it for the backing store
> (`abstractions.EnableBackingStoreForParseNodeFactory`, idempotent); the request adapter then calls
> `GetRootParseNode("application/json", body)` and `GetObjectValue(<model>.Create…FromDiscriminatorValue)` with the
> factory its request builder passes. The pipeline's transform stage is `(*Transformer).transformResource`
> (`internal/pipeline/transformer.go:111`): handler `Transform`, filters, cleaning, id resolution, base64 decode,
> name sanitisation, `transform.SortScalarSlices`. The zero-config transformer set is
> `models.DefaultTransformerConfigs()` (base64 decode inline).
>
> **Decision.** Scope: the routine update plus an investigation only — no SDK consolidation this time.
>
> **Decision.** Neutrality proof: an offline golden test, so every future bump is checkable in CI.
>
> **Decision.** Dependabot for go, web and GitHub Actions, with a dependency-only exemption in both branch gates.
>
> **Not regeneration-gated, by guard.** Not batched with the scheduled regeneration: the golden test is the guard —
> a module whose bump changes a golden file is held back. Its follow-up is regeneration-gated and rides the one
> scheduled regeneration with *Correct and complete the per-handler documentation metadata*, *Template content fixes
> and a Conditional Access template* and *Run-prompt fixes and the `summary:` frontmatter line*. The prompt golden
> test of *Shared prompt partials, without changing a byte* is a separate test; it should reuse this entry's
> `UPDATE_GOLDEN=1` / `make golden-update` switch.
>
> **Contract.** Dependency-only, in the go gate's terms: the branch changes something under `go/` and every changed
> path under `go/` (merge base with `main` to `HEAD`) is `go/go.mod` or `go/go.sum`; paths outside `go/`
> (`.github/…`, `.claude/archive/…`, `web/…`) count neither way. The web gate applies the same rule to
> `web/package.json` and `web/package-lock.json` (and keeps its `version` check). On a dependency-only branch only
> the backlog-changed check is replaced, by an ok line identical in both gates:
> `dependency-only branch: backlog check not required`; every other check (strikeouts, numbering, `[Unreleased]`,
> not on `main`, Conventional Commits, archived-as-done recorded) runs unchanged. Dependabot subjects need no gate
> exception: `commit-message.prefix` `build(go)` / `build(web)` / `ci`, without `include: scope`, makes Dependabot
> write `build(go): bump …`, `build(web): bump …`, `ci: bump …`, which match the shared `CONVENTIONAL_SUBJECT`
> regex of both gates. `.github/dependabot.yml` is one file holding all three ecosystems. Until the web gate ships
> its half, npm Dependabot pull requests fail `branch-ready-web` and wait.
>
> **Owner.** go owns `.github/dependabot.yml` (written by the implementer). At *done*, written by the session: the
> root `README.md`, the root `CLAUDE.md`, `.claude/rules/next-iterations.md` and its Windsurf twins
> `go/.windsurf/rules/06-next-iterations.md` and `web/.windsurf/rules/06-next-iterations.md`. Sequencing: the golden
> test lands and passes on the current versions before any module is bumped; the npm block of `dependabot.yml`
> ships now, ahead of the web entry *Accept a dependency-only branch in the branch gate*, which must match the
> Contract above.
>
> **Implementer.** opus

**Plan.**

- ~~Golden test first, on the current versions: `internal/pipeline/golden_test.go`, package `pipeline`, so it runs
  the production transform stage rather than a copy of it. Setup: `handlers.NewRegistry(<test-local fake
  azcore.TokenCredential>, "00000000-0000-0000-0000-000000000000", false)` — the production registry, whose
  construction builds the Graph clients and so registers the SDK's JSON parse-node factory with the backing store
  enabled — and `NewTransformer(registry, 1, models.DefaultTransformerConfigs(), nil)`. Per case: parse the fixture
  with `serialization.DefaultParseNodeFactoryInstance.GetRootParseNode("application/json", body)` and
  `GetObjectValue(<models>.Create<Type>FromDiscriminatorValue)` — v1.0 `msgraph-sdk-go/models` for conditional
  access and groups, beta `msgraph-beta-sdk-go/models` otherwise; a case table in the test maps resource type →
  fixture → factory (no parse hook is added to the handlers). Then `transformResource(&models.FetchResult{…,
  RawData: parsed})` must succeed and `MarshalResourceYAML(result.CleanedData)` must equal the checked-in
  `<case>.golden.yaml` byte for byte; on mismatch the failure names the case and the first differing line. Runs in
  `make test`, offline.~~
- ~~Fixtures under `internal/pipeline/testdata/golden/`: `<case>.json` + `<case>.golden.yaml`, synthetic only
  (made-up GUIDs, names and domains, no tenant data). Cases: a conditional access policy (v1.0; conditions, grant
  and session controls), a group (v1.0; unsorted `proxyAddresses`), a settings-catalog policy
  (`deviceManagementConfigurationPolicies`, beta) with expanded `settings` holding nested choice and
  group-collection setting instances plus `assignments`, a `deviceConfigurations` subtype with a base64 `payload`
  (e.g. `#microsoft.graph.iosCustomConfiguration`), and a `mobileApps` subtype with `assignments` (e.g.
  `#microsoft.graph.win32LobApp`). Across the set the fixtures exercise an `@odata.type` discriminator, a
  `DateTimeOffset`, an enum and a flags enum, an explicit `null`, an integer, a property unknown to the model (lands
  in `AdditionalData`) and nested collections.~~
- ~~Golden regeneration is a deliberate manual step: a missing golden file fails the test (never auto-created); with
  `UPDATE_GOLDEN=1` set the test rewrites the golden files instead of comparing. New `go/Makefile` target
  `golden-update` runs the `internal/pipeline` tests with `UPDATE_GOLDEN=1` and is listed in `make help`; `make test`
  and CI never set it.~~
- ~~New `go/Makefile` target `deps-update`: refuses with a usage message when `MODULES` is empty, otherwise runs
  `$(GOCMD) get -u $(MODULES)` and then `$(GOMOD) tidy`; listed in `make help`. It is how the update below runs
  (raw `go get` is off-limits; `make -C go …` is on the agents' allow list and the agent guard does not block it).
  It needs network to `proxy.golang.org`, which was reachable from the agent sandbox at review time; if it is not,
  report it under *Could not do* and the session runs the same targets.~~
- ~~The update, one module group at a time, each followed by `make -C go test` and `make -C go build`: Azure SDK
  (`azcore`, `azidentity`, `azlogs`, `armcompute/v5`, `armresources`, `armsubscriptions`, `armstorage`),
  Graph/Kiota (`msgraph-sdk-go`, `msgraph-beta-sdk-go` and every direct `kiota-*` / `msgraph-sdk-go-core`
  requirement in `go.mod`), CLI libraries (`cobra`, `pflag`, `viper`, `charmbracelet/log`, `yaml.v3`), each via
  `make -C go deps-update MODULES="<module paths>"`. The golden files must stay byte-identical and the `go` line
  must stay `1.26.x`. A module whose bump changes a golden file or needs a newer Go minor is held back: restore its
  previous version in `go.mod` by hand (git writes are blocked), run `make -C go deps`, rerun the group without it,
  and add an unstruck follow-up bullet to this entry naming the module, the version tried and the golden case that
  changed with its first differing lines. API breaks are fixed at their call sites, with tests.~~
- Follow-up: `github.com/Azure/azure-sdk-for-go/sdk/azidentity/cache` (`v0.4.0`) is a direct requirement in `go.mod`
  but was not in the Azure SDK group above, so it was not updated; run
  `make -C go deps-update MODULES="github.com/Azure/azure-sdk-for-go/sdk/azidentity/cache"`, then `make -C go test` and
  `make -C go build` (the golden files must stay byte-identical).
- ~~Investigation, no code change, offline: for each beta endpoint the handlers call (the `msgraphbeta` request
  builders in `internal/handlers/graph/`), check whether the updated v1.0 SDK in the module cache (default
  `~/go/pkg/mod/github.com/microsoftgraph/msgraph-sdk-go@<version>/`) has the matching request builder and model —
  the v1.0 SDK is generated from the v1.0 OpenAPI description, so it is the evidence; a Microsoft Learn cross-check
  is optional and the session's (the implementer has no web tool). Conclude which consolidation direction is viable —
  drop v1.0 (seven handlers to beta), drop beta (only if every Intune endpoint exists on v1.0), or neither — and
  write the dated result, with a compact endpoint → v1.0 yes/no list, into the parked idea *consolidate on one
  Microsoft Graph SDK* as its revisit evidence.~~
- ~~`.github/dependabot.yml` (`version: 2`), three `updates` entries, each with `schedule.interval: weekly` and one
  `groups` entry with `patterns: ["*"]`, so version updates arrive as one pull request per ecosystem: `gomod` on
  `/go` with `commit-message.prefix: "build(go)"`, `npm` on `/web` with `"build(web)"`, `github-actions` on `/` with
  `"ci"`; no `include: scope` (it would write `build(go)(deps):`, which the gates refuse). Every module held back
  above gets an `ignore` entry (`dependency-name`) with a comment naming its follow-up bullet, so the grouped pull
  request is not permanently red. Resulting subjects: `build(go): bump …`, `build(web): bump …`, `ci: bump …`.~~
- ~~Dependency-only exemption in the go gate: `scripts/lib/branch.sh` gains `dependency_only <base>`, which succeeds
  when `git diff --name-only --relative "$base" HEAD -- .` (run from `go/`; without `--relative` git prints
  repository-root paths) is non-empty and every line is `go.mod` or `go.sum`. In the gate script's check 5, when
  `backlog_state` is empty and `dependency_only` holds, report `ok "dependency-only branch: backlog check not
  required"` instead of the failure; checks 1–4, 6 and 7 stay unchanged. Tests in `scripts/lib/branch_test.sh`
  (`make -C go test-scripts`): `go.mod` + `go.sum` → dependency-only; `go.sum` alone → dependency-only; `go.mod` + a
  `.go` file → not (and `backlog_state` empty, so the gate fails as before); `go.mod` + `NEXT-ITERATIONS.md` → not
  (`backlog_state` is `changed`); a change only outside `go/` (`.github/dependabot.yml`) → not.~~
- Stale Go version (documentation, at *done*): `go/README.md` and the root `README.md` say Go 1.26+ (from
  `go.mod`); drop the mismatch note in the root `CLAUDE.md`.
- Rules wording (documentation, at *done*): the root `CLAUDE.md` gates paragraph, `.claude/rules/next-iterations.md`
  and both Windsurf twins state the one exception — a dependency-only branch (Dependabot or manual) needs no backlog
  entry; `ci-*` and the golden test prove it. Until the web entry ships, the wording names the go gate only.
- Documentation at *done*: `CHANGELOG.md` `### Changed` (the updated modules with versions) and *Release workflow*
  (Dependabot, the dependency-only exemption); `README.md` toolchain, the golden test in the testing section and the
  `deps-update` / `golden-update` targets; `go/CLAUDE.md` commands table.

## Parked ideas

Deliberately not scheduled — kept here rather than in a work entry so they survive as the entries around them
ship and are archived. Each records why it is parked and what would make it worth doing.

### Idea: recommend a scoped `az login` in the dedicated-app prompt

Let the dedicated-app prompt (`PromptForDedicatedApp`, `internal/cmdutil/prompt.go`) first print the exact
`az login --scope …` command derived from the selected types' declared permissions, with device code as the
fallback — the softening step the sign-in review proposed. **Parked** because the measurement says it cannot work:
on cb-gmbh.com (2026-10-01) a token from `az login --scope https://graph.microsoft.com/.default` belonged to the
first-party Azure CLI app (`04b07795-…`) with its fixed scope set and covered 1 of 51 declared permissions
(`Group.Read.All` only) — every `DeviceManagement*`, `Policy.Read.All`, `Agreement.Read.All`,
`OnPremDirectorySynchronization.Read.All` and `Organization*` permission was missing. Recommending the command would
send operators down a path that fails. The `--debug` Graph-token section stays the instrument to re-measure.

**Revisit when** a re-measurement with `azure-rd --debug` on a representative tenant shows the Azure CLI app's token
covering the declared permissions (Microsoft changes what its first-party app may request), or Microsoft documents
that it does. Removing the dedicated-app path entirely would additionally be breaking (two tenant-scoped keys).

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

### Idea: consolidate on one Microsoft Graph SDK

The module carries two generated Graph SDKs: `msgraph-beta-sdk-go` for the Intune / device-management endpoints that
do not exist on v1.0 (about 50 handlers), and `msgraph-sdk-go` (v1.0) for seven handlers (conditional access, groups,
organization, authentication methods and strengths, authorization policy, on-premises sync) plus `GraphErrorCode`.
The two SDKs dominate compile time and binary size. **Parked** because both directions cost something real: moving
the seven to beta trades v1.0's stability contract for beta churn on the most-referenced types, and re-fetching them
through beta changes their YAML once (beta models carry extra properties), which moves their `sourceSha256` and
regenerates their documents; dropping beta instead is only possible once every Intune endpoint exists on v1.0.

**Revisit when** the investigation recorded by the routine-dependency-update entry shows a viable direction, build
time becomes a felt cost, or a security advisory forces an SDK change anyway. **Regeneration-gated** when picked up:
batch it with a scheduled regeneration; move the handlers one at a time, each proven with the golden test, and drop
the module only at the end.

**Evidence (2026-10-01, `msgraph-sdk-go v1.103.0`, `msgraph-beta-sdk-go v0.166.0`, read from the module cache —
request builders only; a Microsoft Learn cross-check is still open).** *Drop beta* is **not viable**: 21 of the beta
endpoints the handlers call have no v1.0 request builder, among them the Settings Catalog. *Drop v1.0* is
**technically viable**: all seven v1.0 endpoints (`identity/conditionalAccess/policies`,
`policies/authenticationStrengthPolicies`, `policies/authenticationMethodsPolicy`, `policies/authorizationPolicy`,
`directory/onPremisesSynchronization`, `groups`, `organization`) exist on beta, and beta has its own `odataerrors`
for `GraphErrorCode`; the cost stays the one stated above (beta churn, one YAML/hash move per type). Beta endpoint →
on v1.0:
- `deviceManagement`: `applePushNotificationCertificate` yes; `appleUserInitiatedEnrollmentProfiles` no;
  `assignmentFilters` no; `compliancePolicies` no; `configurationPolicies` no; `depOnboardingSettings` no;
  `deviceCategories` yes; `deviceCompliancePolicies` yes (+ assignments); `deviceComplianceScripts` no;
  `deviceConfigurations` yes (+ assignments, `getOmaSettingPlainTextValue`); `deviceCustomAttributeShellScripts` no;
  `deviceEnrollmentConfigurations` yes (+ assignments); `deviceHealthScripts` no; `deviceManagementScripts` no;
  `deviceShellScripts` no; `groupPolicyConfigurations` no; `intents` no; `intuneBrandingProfiles` no;
  `mobileThreatDefenseConnectors` yes; `ndesConnectors` no; `notificationMessageTemplates` yes;
  `reusablePolicySettings` no; `roleDefinitions` yes; `roleScopeTags` no; `termsAndConditions` yes (+ assignments);
  `windowsAutopilotDeploymentProfiles` no; `windowsAutopilotDeviceIdentities` yes; `windowsDriverUpdateProfiles` no;
  `windowsFeatureUpdateProfiles` no; `windowsQualityUpdateProfiles` no; the `deviceManagement` singleton (settings)
  yes.
- `deviceAppManagement` (each with its assignments where the handler reads them): `androidManagedAppProtections`,
  `iosManagedAppProtections`, `mdmWindowsInformationProtectionPolicies`, `mobileAppConfigurations`, `mobileApps`,
  `targetedManagedAppConfigurations`, `vppTokens`, `windowsInformationProtectionPolicies` yes;
  `windowsManagedAppProtections` no.
- `identity/conditionalAccess/namedLocations` yes; `identityGovernance/termsOfUse/agreements` yes;
  `organization` and `organization/{id}/branding` yes.

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


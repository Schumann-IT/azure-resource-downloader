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
> downloads and tidies. The v1.0 SDK serves exactly seven handlers (conditional access, groups, organization,
> authentication methods and strengths, authorization policy, on-premises sync) plus `GraphErrorCode` in
> `internal/azure/errors.go`. There is no `dependabot.yml`, and both branch gates refuse a branch that touches the
> project without a backlog change — so a Dependabot pull request could never pass the required `branch-ready-*`
> checks. `go/README.md:88` and the root `README.md:58,109` still say Go 1.24.
>
> **Decision.** Scope: the routine update plus an investigation only — no SDK consolidation this time.
>
> **Decision.** Neutrality proof: an offline golden test, so every future bump is checkable in CI.
>
> **Decision.** Dependabot for go, web and GitHub Actions, with a dependency-only exemption in both branch gates.
>
> **Not regeneration-gated, by guard.** Not batched with the scheduled regeneration: the golden test is the guard —
> a module whose bump changes a golden file is held back and left for a regeneration-batched follow-up.
>
> **Contract.** The dependency-only rule and its ok-line wording are identical in both gates (web entry *Accept a
> dependency-only branch in the branch gate*).
>
> **Owner.** Outside `go/`: `.github/dependabot.yml`, the root `README.md`, the root `CLAUDE.md`,
> `.claude/rules/next-iterations.md` and its Windsurf twins `go/.windsurf/rules/06-next-iterations.md`,
> `web/.windsurf/rules/06-next-iterations.md`. Sequencing: the golden test lands before the bump.
>
> **Implementer.** opus

**Plan.**

- Golden test first, on the current versions: synthetic Graph JSON fixtures under
  `internal/handlers/graph/testdata/golden/` — hand-written or sanitised, no tenant data — for one or two v1.0
  types (conditional access, groups) and several beta types (a settings-catalog policy with nested settings, a
  `deviceConfigurations` subtype with a base64 payload, `mobileApps`). The test parses each fixture into the
  handler's model through the SDK's Kiota JSON parse node (the factory the client uses), runs the handler's
  `Transform` and the pipeline transforms, marshals with `MarshalResourceYAML` and compares against checked-in
  `.golden.yaml` files. Regenerating the goldens is a deliberate manual step, never in CI. Runs in `make test`.
- The update: `go get -u` per module group (Azure SDK, Graph/Kiota, CLI libraries), then `make deps` and
  `make check`. The golden test must stay byte-identical; a module whose bump changes a golden file is held back at
  its current version, its diff recorded in this entry for a regeneration-batched follow-up. API breaks are fixed at
  their call sites.
- Investigation, no code change: for each Intune endpoint the beta handlers use, record whether a v1.0 equivalent
  exists today (Microsoft Learn); conclude which consolidation direction is viable — drop v1.0 (seven handlers to
  beta), drop beta (only if every Intune endpoint exists on v1.0), or neither — and write the result into the parked
  idea *consolidate on one Microsoft Graph SDK* as its revisit evidence.
- `.github/dependabot.yml`: `gomod` on `/go`, `npm` on `/web`, `github-actions` on `/`; weekly; one grouped pull
  request per ecosystem; a commit-message prefix whose subjects pass the gates' Conventional Commits check
  (`build(go): …` / `build(web): …` / `ci: …` if Dependabot's options allow it — otherwise the exemption below also
  accepts Dependabot's own subject form). The exact subject is stated in this entry.
- Dependency-only exemption in the go gate (`scripts/branch-ready.sh`, `scripts/lib/branch.sh`): a branch whose
  changes under `go/` are only `go.mod` / `go.sum` skips the backlog-changed check and the archive checks; it still
  needs Conventional Commits and no strikeouts. Ok line: "dependency-only branch: backlog check not required". Script
  tests in `scripts/lib/branch_test.sh`: deps-only passes; deps plus a code file fails as before.
- Stale Go version: `go/README.md` and the root `README.md` say Go 1.26+ (from `go.mod`); drop the mismatch note in
  the root `CLAUDE.md`.
- Rules wording: the root `CLAUDE.md` gates paragraph, `.claude/rules/next-iterations.md` and both Windsurf twins
  state the one exception — a dependency-only branch (Dependabot or manual) needs no backlog entry; `ci-*` and the
  golden test prove it.
- Documentation at *done*: `CHANGELOG.md` `### Changed` (the updated modules with versions) and *Release workflow*
  (Dependabot, the dependency-only exemption); `README.md` toolchain and the golden test in the testing section.

## 6. Sign in once: a cached device-code session, and a measured answer on scoped az login

**Goal.** Signing in stops being a per-run chore, by two routes pursued together: the dedicated-app sign-in is
done once with device code and later runs get their tokens silently from a persistent, OS-protected token cache
(until the refresh token expires or is revoked); and the tool shows, for the selected handlers, which declared
Graph permissions the `az login` session's token actually carries — so one recorded experiment settles whether a
scoped `az login` could stand in for the dedicated app. The documentation stops contradicting itself and stops
naming flags that no longer exist.

> **Why.** Two sign-in paths exist: the `az login` session (`AzureCLICredential`, used when the tenant profile has no
> `client-id`) and a device-code sign-in to a dedicated app registration (`client-id` + `tenant-id` in the profile;
> `newCredential`, `internal/azure/client.go:156-185`). The device-code credential has no persistent cache, so every
> run prompts again. Every Graph type is treated as needing the dedicated app — `worksWithCLICredential` is never set
> (`internal/handlers/graph/collection.go:35-42, 101-103`), `DedicatedAppRequirements` is static metadata
> (`internal/handlers/registry.go:100-116`), `VerifySession` only mints a token (`client.go:51-59`) — and nothing
> decodes a token's `scp`; `--debug` (`cmd/root.go:204-283`) prints identity claims only. The README contradicts
> itself: `README.md:540-546` says the Intune and policy scopes are not consentable for the first-party CLI app, so a
> CLI token can never carry them, while `:679-681` advises `az login --scope https://graph.microsoft.com/.default`
> for "required scopes are missing" on an ARM-only run, where no Graph call happens.
>
> **Reconciled from the parked idea** *review the sign-in surface — can a scoped `az login` replace the dedicated-app
> device-code path?*: the `--client-id` / `--tenant-id` flags and `AZURE_RD_*` variables it names are gone —
> `client-id` / `tenant-id` are tenant-scoped config keys only (`internal/config/keys.go:62-63`), and tests assert
> the flags and variables are absent; and azidentity's CLI credential calls `az account get-access-token
> --resource …`, never `--scope`, so whatever a scoped login granted must appear in the token minted for the Graph
> resource. Stale wording is still live: the user-facing error at `client.go:168` (names the removed flags and
> `AZURE_RD_TENANT_ID`), the `cmd/root.go:52` help text, comments in `models/types.go:174`, `registry.go:99`,
> `collection.go:41`, `cmd/resource/list.go:97`, and `README.md:544` ("the flags are unset").
>
> **Decision.** How far the review goes: instrument, run the experiment, then decide — the prompt's scoped-login
> recommendation is built only if the experiment says yes.
>
> **Decision.** The user added a cached device-code session (sign in once, later runs silent), independent of the
> experiment's outcome.
>
> **Out of scope, revisit note.** Removing the dedicated-app path would be breaking (two tenant-scoped keys go) and
> needs the experiment's "yes" on a representative tenant first; record it under `Breaking` if it ever ships.
>
> Not regeneration-gated: no template and no `promptSha256` moves, so nothing needs to ride along.
>
> **Token cache, verified at review (2026-10-01).** azidentity `v1.14.1` (in `go.mod`) has everything the session
> needs: `DeviceCodeCredentialOptions.Cache` (type `azidentity.Cache`), `.AuthenticationRecord`,
> `.DisableAutomaticAuthentication`, `(*DeviceCodeCredential).Authenticate` → `azidentity.AuthenticationRecord`, and
> `*azidentity.AuthenticationRequiredError`. The persistent store is the separate module
> `github.com/Azure/azure-sdk-for-go/sdk/azidentity/cache` (import path the same, package `cache`; latest `v0.4.0`,
> requires azidentity ≥ v1.13.1, azcore ≥ v1.21.0 — satisfied), constructor `cache.New(&cache.Options{Name:
> "azure-rd"})`, which round-trips test data once and returns an error when no secure store works. New indirect
> modules: `github.com/AzureAD/microsoft-authentication-extensions-for-go/cache v0.1.1` and
> `github.com/keybase/go-keychain v0.0.1`. Storage per platform: macOS — the login Keychain (via go-keychain,
> **cgo required**: the extensions' darwin accessor is `//go:build darwin && cgo`, so a `CGO_ENABLED=0` darwin build
> of the package fails to compile); Windows — a DPAPI-encrypted file under `%LOCALAPPDATA%\.IdentityService\`;
> Linux — **not libsecret**: an AES-encrypted file under `$XDG_CACHE_HOME` (or `~/.cache`)`/.IdentityService/` whose
> key lives in the kernel persistent/user keyring (`keyctl`, pure Go via `golang.org/x/sys/unix`, no cgo), so on
> Linux the session survives across runs and logins but **not a reboot** (or keyring expiry); where `keyctl` is
> unavailable (some containers) `cache.New` errors. The module compiles only for darwin, linux and windows. CI
> (`ubuntu-latest`, `make -C go ci`, golangci-lint) is unaffected: the Linux path needs no cgo or system library,
> and tests never call `cache.New`.
>
> **Contract.** None with `web/`: nothing under the export tree (`output/<tenant>/{resources,docs,drift}/`), no
> `metadata.yaml` or `index.yaml` field, file name or exit code changes. The only new on-disk artefact is the
> authentication record at `os.UserConfigDir()/azure-rd/auth/<tenant-id>-<client-id>.json` (mode 0600, directory
> 0700; fields per `azidentity.AuthenticationRecord`, no token or secret) plus the SDK-owned token cache named
> `azure-rd` — both outside the repository, the `--config-dir` and `output/`, and neither read by `web/`.
>
> **Owner.** none — no file outside `go/`. Sequencing: no dependency on `web/`. Entry 5 (routine dependency
> updates) also edits `go/go.mod` / `go/go.sum`; whichever lands second on a shared branch reruns `make deps`.
>
> **Who does what.** The implementer delivers the token-claims decoder, the coverage helper, the `--debug`
> sections, the cached device-code session and the stale-wording fixes, with tests, and strikes those bullets.
> The cache check and the scoped-login experiment need a live tenant and a human sign-in: they stay unstruck for the
> operator, who records the results in those bullets. The conditional follow-up stays unstruck and unbuilt until the
> experiment is recorded; the documentation bullet stays unstruck until *done*.
>
> **Implementer.** opus

**Plan.**

- ~~Token claims: `internal/azure/identity.go` gains a pure decoder for a Graph token's `appid`, `app_displayname`
  and `scp`, beside `parseIdentityClaims`; tested with synthetic JWT payloads. The token itself is never logged.~~
- ~~Coverage helper (pure): given declared permissions and a scope list, return covered and missing; a `ReadWrite`
  scope covers its `Read` counterpart, matching is case-insensitive; table-tested.~~
- ~~`--debug` (`runDebugReport`): a "Graph token" section with `appid` / `app_displayname` and the sorted `scp`, and
  for the effective type selection (`runprep.SelectTypesFromConfig` plus `DedicatedAppRequirements`) every declared
  permission marked covered or missing. It runs on the CLI credential when the profile has no `client-id`, otherwise
  on the dedicated app's token, and writes nothing. The section is built by a pure function from the decoded claims
  and the coverage result (tested with fixed inputs, sorted output); `runDebugReport` only fetches the Graph token
  and logs the result.~~
- ~~Dependency: add `github.com/Azure/azure-sdk-for-go/sdk/azidentity/cache` (latest release, `v0.4.0` at review) as
  a direct requirement, then `make deps`; `go.sum` gains the module and its two new indirects (Keychain accessor,
  go-keychain). No other module version moves.~~
- ~~Platform split, so a build without the store still compiles: the only file importing `azidentity/cache` is
  `internal/azure/tokencache_supported.go` (`//go:build (darwin && cgo) || linux || windows`), exposing
  `newPersistentCache() (azidentity.Cache, error)`; `internal/azure/tokencache_other.go` (the negated constraint)
  returns an error "persistent token cache unavailable on this platform/build" — the "no secure store" path below.
  The factory is a package variable so tests replace it and never touch a real store.~~
- ~~Authentication record store (`internal/azure/authrecord.go`): `authRecordPath(tenantID, clientID)` →
  `os.UserConfigDir()/azure-rd/auth/<tenant-id>-<client-id>.json` (directory created 0700, file written 0600 via a
  temp file and rename); `loadAuthRecord` returns the record, or none when the file is missing, unparsable, or its
  `TenantID` / `ClientID` differ from the profile (case-insensitive) — a mismatched or corrupt file is treated as
  absent and overwritten on the next sign-in, never trusted. Never under the repository, `--config-dir` or
  `output/`; it holds no token or secret.~~
- ~~Cached device-code session in `newCredential` (only when the profile names a `client-id`; no new config key):
  build the persistent cache (`Name: "azure-rd"`); on error, warn once ("token cache unavailable: <reason>; signing
  in on every run") and construct today's credential unchanged — never store tokens unencrypted. With a cache, wrap
  an `azidentity.DeviceCodeCredential` built with `Cache`, the loaded `AuthenticationRecord` (if any) and
  `DisableAutomaticAuthentication: true` in a `cachedDeviceCodeCredential` implementing `azcore.TokenCredential`:
  `GetToken` tries the inner credential silently; on `*azidentity.AuthenticationRequiredError` (no record, refresh
  token expired or revoked) the interactive variant (`NewCredential`) calls `Authenticate` with the Graph
  `.default` scope — the device-code prompt — under a mutex so concurrent `GetToken` callers trigger exactly one
  prompt, saves the returned record (replacing the old one), and retries `GetToken`; the non-interactive variant
  (`NewNonInteractiveCredential`) returns the error unchanged and never prompts, but now succeeds silently whenever
  a cached session exists. Construction stays network-free (`NewCredential`'s contract). Any other error passes
  through unchanged.~~
- ~~`--debug` token-cache line: "Token cache" `active (record from <file mtime, date>)`, `no session yet (the next run
  signs in once)` or `unavailable (<reason>)`; printed only when the profile names a `client-id`.~~
- ~~Tests (all with the cache factory and the inner credential stubbed; no real tokens, no real store): record
  round-trip; path shape, directory 0700 and file 0600 (skipped on Windows); tenant mismatch, client mismatch and
  corrupt file load as absent; cache-unavailable falls back to the plain credential and warns once; silent success
  never calls `Authenticate`; `AuthenticationRequiredError` → one `Authenticate`, record saved, retry succeeds;
  non-interactive variant never calls `Authenticate`; N concurrent `GetToken` calls on a fresh session call
  `Authenticate` once (run under `make test-race`); other errors pass through.~~
- Operator step, not for the implementer — stays unstruck until the result is recorded here: on cb-gmbh.com, with
  the dedicated app in the profile, sign in once, then a second `resource download --dry-run` must not prompt;
  record the platform and whether it prompted.
- The experiment, an operator step not for the implementer — stays unstruck until recorded here: on cb-gmbh.com
  `az logout && az login --scope https://graph.microsoft.com/.default` (explicit scopes if that is refused), then `azure-rd --debug --config-dir …
  --domain cb-gmbh.com` with no `client-id` in the profile; record `appid`, the `scp` list and the covered/missing
  table. If everything is covered, also run one `resource download --dry-run` per dedicated-app type without
  `client-id` and record the outcome — a service may gate on the calling application, not only on the scopes.
- Follow-up, conditional — not built in the first implementation run, stays unstruck until the experiment above is
  recorded; built only if the experiment covers every declared permission: `PromptForDedicatedApp`
  (`internal/cmdutil/prompt.go`) first prints the exact `az login --scope …` command derived from the selected types
  and offers device code as the fallback. If the experiment says no, this bullet goes back to the parked ideas,
  rewritten with the evidence, and the README states the measured answer.
- ~~Stale wording: the `client.go:168` error reads "tenant-id is required when client-id is set in the tenant profile";
  the `cmd/root.go:52` help text and the comments in `models/types.go`, `registry.go`, `collection.go`,
  `cmd/resource/list.go` name the profile keys, not flags. A test asserts the new error text and that neither it nor
  the root help mentions `--client-id`, `--tenant-id` or `AZURE_RD_`.~~
- Documentation at *done*: `README.md` — resolve the contradiction (`:540-546` vs `:679-681`) with the measured
  result; `:544` "the profile sets no `client-id`"; the `--debug` section documents the Graph token section and
  replaces the shell token-decoder one-liner (`:518-523`); the cached device-code session (where the record and the
  tokens live, how to forget a session by deleting the record, the fallback without a secure store, that on Linux
  the session lasts until reboot, and that a macOS build needs cgo — the default with Xcode's command-line tools —
  to get the Keychain cache). `CHANGELOG.md`:
  `### Added` (the cached device-code session; the `--debug` Graph token and coverage section), `### Fixed` (the
  error message and help text naming removed flags).
- Follow-up, found while implementing — operator check first: azidentity keeps CAE and non-CAE tokens in separate
  cache partitions (`azure-rd` and `azure-rd.cae`, separate MSAL clients), and the sign-in authenticates only the
  partition of the request that needed it (the request's `EnableCAE` is carried into `Authenticate`). A run whose
  token requests mix both (the Graph SDK and azcore's bearer policy request CAE tokens; `VerifySession` and
  `SignedInIdentity` do not) may therefore prompt twice on its first sign-in, and silently after that. The cache
  check above should record how many prompts the first run showed; if two, sign in once and seed the other
  partition from the same session, or drop the mixed request.

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


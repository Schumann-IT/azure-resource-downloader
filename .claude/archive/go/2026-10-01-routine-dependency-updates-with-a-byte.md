---
title: Routine dependency updates with a byte-neutrality guard, and Dependabot
project: go
status: done
started: 2026-10-01
finished: 2026-10-01
branch: build/dependency-updates
pr: 40
changelog: Unreleased
---
## Routine dependency updates with a byte-neutrality guard, and Dependabot

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

- ✅ Golden test first, on the current versions: `internal/pipeline/golden_test.go`, package `pipeline`, so it runs
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
  `make test`, offline.
- ✅ Fixtures under `internal/pipeline/testdata/golden/`: `<case>.json` + `<case>.golden.yaml`, synthetic only
  (made-up GUIDs, names and domains, no tenant data). Cases: a conditional access policy (v1.0; conditions, grant
  and session controls), a group (v1.0; unsorted `proxyAddresses`), a settings-catalog policy
  (`deviceManagementConfigurationPolicies`, beta) with expanded `settings` holding nested choice and
  group-collection setting instances plus `assignments`, a `deviceConfigurations` subtype with a base64 `payload`
  (e.g. `#microsoft.graph.iosCustomConfiguration`), and a `mobileApps` subtype with `assignments` (e.g.
  `#microsoft.graph.win32LobApp`). Across the set the fixtures exercise an `@odata.type` discriminator, a
  `DateTimeOffset`, an enum and a flags enum, an explicit `null`, an integer, a property unknown to the model (lands
  in `AdditionalData`) and nested collections.
- ✅ Golden regeneration is a deliberate manual step: a missing golden file fails the test (never auto-created); with
  `UPDATE_GOLDEN=1` set the test rewrites the golden files instead of comparing. New `go/Makefile` target
  `golden-update` runs the `internal/pipeline` tests with `UPDATE_GOLDEN=1` and is listed in `make help`; `make test`
  and CI never set it.
- ✅ New `go/Makefile` target `deps-update`: refuses with a usage message when `MODULES` is empty, otherwise runs
  `$(GOCMD) get -u $(MODULES)` and then `$(GOMOD) tidy`; listed in `make help`. It is how the update below runs
  (raw `go get` is off-limits; `make -C go …` is on the agents' allow list and the agent guard does not block it).
  It needs network to `proxy.golang.org`, which was reachable from the agent sandbox at review time; if it is not,
  report it under *Could not do* and the session runs the same targets.
- ✅ The update, one module group at a time, each followed by `make -C go test` and `make -C go build`: Azure SDK
  (`azcore`, `azidentity`, `azlogs`, `armcompute/v5`, `armresources`, `armsubscriptions`, `armstorage`),
  Graph/Kiota (`msgraph-sdk-go`, `msgraph-beta-sdk-go` and every direct `kiota-*` / `msgraph-sdk-go-core`
  requirement in `go.mod`), CLI libraries (`cobra`, `pflag`, `viper`, `charmbracelet/log`, `yaml.v3`), each via
  `make -C go deps-update MODULES="<module paths>"`. The golden files must stay byte-identical and the `go` line
  must stay `1.26.x`. A module whose bump changes a golden file or needs a newer Go minor is held back: restore its
  previous version in `go.mod` by hand (git writes are blocked), run `make -C go deps`, rerun the group without it,
  and add an unstruck follow-up bullet to this entry naming the module, the version tried and the golden case that
  changed with its first differing lines. API breaks are fixed at their call sites, with tests.
- ✅ Follow-up: `github.com/Azure/azure-sdk-for-go/sdk/azidentity/cache` (`v0.4.0`) is a direct requirement in `go.mod`
  but was not in the Azure SDK group above, so it was not updated; run
  `make -C go deps-update MODULES="github.com/Azure/azure-sdk-for-go/sdk/azidentity/cache"`, then `make -C go test` and
  `make -C go build` (the golden files must stay byte-identical). Outcome: already newest at v0.4.0.
- ✅ Investigation, no code change, offline: for each beta endpoint the handlers call (the `msgraphbeta` request
  builders in `internal/handlers/graph/`), check whether the updated v1.0 SDK in the module cache (default
  `~/go/pkg/mod/github.com/microsoftgraph/msgraph-sdk-go@<version>/`) has the matching request builder and model —
  the v1.0 SDK is generated from the v1.0 OpenAPI description, so it is the evidence; a Microsoft Learn cross-check
  is optional and the session's (the implementer has no web tool). Conclude which consolidation direction is viable —
  drop v1.0 (seven handlers to beta), drop beta (only if every Intune endpoint exists on v1.0), or neither — and
  write the dated result, with a compact endpoint → v1.0 yes/no list, into the parked idea *consolidate on one
  Microsoft Graph SDK* as its revisit evidence.
- ✅ `.github/dependabot.yml` (`version: 2`), three `updates` entries, each with `schedule.interval: weekly` and one
  `groups` entry with `patterns: ["*"]`, so version updates arrive as one pull request per ecosystem: `gomod` on
  `/go` with `commit-message.prefix: "build(go)"`, `npm` on `/web` with `"build(web)"`, `github-actions` on `/` with
  `"ci"`; no `include: scope` (it would write `build(go)(deps):`, which the gates refuse). Every module held back
  above gets an `ignore` entry (`dependency-name`) with a comment naming its follow-up bullet, so the grouped pull
  request is not permanently red. Resulting subjects: `build(go): bump …`, `build(web): bump …`, `ci: bump …`.
- ✅ Dependency-only exemption in the go gate: `scripts/lib/branch.sh` gains `dependency_only <base>`, which succeeds
  when `git diff --name-only --relative "$base" HEAD -- .` (run from `go/`; without `--relative` git prints
  repository-root paths) is non-empty and every line is `go.mod` or `go.sum`. In the gate script's check 5, when
  `backlog_state` is empty and `dependency_only` holds, report `ok "dependency-only branch: backlog check not
  required"` instead of the failure; checks 1–4, 6 and 7 stay unchanged. Tests in `scripts/lib/branch_test.sh`
  (`make -C go test-scripts`): `go.mod` + `go.sum` → dependency-only; `go.sum` alone → dependency-only; `go.mod` + a
  `.go` file → not (and `backlog_state` empty, so the gate fails as before); `go.mod` + `NEXT-ITERATIONS.md` → not
  (`backlog_state` is `changed`); a change only outside `go/` (`.github/dependabot.yml`) → not.
- ✅ Stale Go version (documentation, at *done*): `go/README.md` and the root `README.md` say Go 1.26+ (from
  `go.mod`); drop the mismatch note in the root `CLAUDE.md`.
- ✅ Rules wording (documentation, at *done*): the root `CLAUDE.md` gates paragraph, `.claude/rules/next-iterations.md`
  and both Windsurf twins state the one exception — a dependency-only branch (Dependabot or manual) needs no backlog
  entry; `ci-*` and the golden test prove it. Until the web entry ships, the wording names the go gate only.
- ✅ Documentation at *done*: `CHANGELOG.md` `### Changed` (the updated modules with versions) and *Release workflow*
  (Dependabot, the dependency-only exemption); `README.md` toolchain, the golden test in the testing section and the
  `deps-update` / `golden-update` targets; `go/CLAUDE.md` commands table.

## Appendix: Graph SDK investigation (moved from the dropped parked idea, 2026-10-01)

The investigation bullet above wrote its result into the parked idea *consolidate on one Microsoft Graph SDK*.
That idea was dropped on 2026-10-01 — dropping beta is not viable, dropping v1.0 trades the stable contract of
the most-referenced types for beta churn and a one-time hash move, and build time was not a felt cost — so its
evidence is kept here, verbatim:

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

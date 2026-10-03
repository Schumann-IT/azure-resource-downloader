---
title: Index settings and assignment scopes and report same-setting conflicts
project: go
status: done
started: 2026-10-02
finished: 2026-10-03
branch: feat/consistency-analysis
changelog: Unreleased
---
## Index settings and assignment scopes and report same-setting conflicts

*Kind:* feat

**Goal.** An operator can find out, from the export alone and without choosing what to compare, where two
resources configure the same setting differently — or redundantly — for scopes that can reach the same device or
user. A new command, `docs analyze-consistency`, indexes every configured setting, models every resource's
assignment scope, and writes the same-setting conflicts and duplicates it finds into a new `consistency/` tree.

> **Why.** The tenant summary has a *consistency* paragraph ("does any setting contradict another",
> `internal/docs/generate_prompt_template.md` §7) that nothing feeds: the summary may judge only from
> `summary-facts`, the reference map and the five-signal sweep. A Claude Cowork draft
> (`Claude outputs/consistency-analysis-plan.md`, git-ignored, written without the code) proposed the feature;
> this entry is its deterministic first step, reconciled with the code. The later steps are the entries
> *A consistency rule and topic catalog in the configuration*, *The consistency analysis job*, and *Feed the
> consistency findings into the tenant summary*, plus the web entry *The consistency view*.
>
> **Principles.** Deterministic first: Go indexes, scopes and compares same-setting values; no LLM in this entry.
> Independent of the documentation: everything comes from `resources/` (YAML and `metadata.yaml`), so the command
> can run right after `resource download`. Facts stay in `resources/metadata.yaml`; the analysis is a `docs`
> command's decision and lives in its own tree.
>
> **Decision.** Scope v1: mechanical first; the rule catalog, the LLM job, the summary signal and the web view are
> the following entries.
>
> **Decision.** macOS custom profiles: their identity and payload properties are non-settings; each profile is
> keyed by its top-level `PayloadIdentifier` (`bundleId` for app configurations) — the device replaces a profile
> with the same identifier, a mechanical conflict. The inner payloads (`PayloadContent`, several per profile, each
> with its own `PayloadType` and identifier) are semantic overlap and go to *A consistency rule and topic catalog
> in the configuration*.
>
> **Decision.** `false` as not configured: it holds for typed properties, Settings Catalog simple values and intents;
> OMA-URI and ADMX keep `false` / disabled as a real value (recorded at done, as implemented).
>
> **Decision.** `<tenant>/consistency/` describes one export: a re-baselining `resource download` clears it, like
> `drift/`. The web browser renders it in v1 (the web entry *The consistency view*).
>
> **Decision.** Shapes: before writing normalisers, the implementer may read one real export's ADMX, intent,
> assignment-filter and collection-valued Settings Catalog YAML **read-only, for key structure only**; no value is
> copied into the repository, every fixture is synthetic and built in a temp directory.
>
> **From the code.** The model is `docs generate-index` (`cmd/docs/generate_index.go`,
> `internal/docs/generateindex.go`): an offline, deterministic `docs` command over `resources/metadata.yaml` and
> the exported data (plus a config key — `taxonomy:`, which the catalog entry follows), with `--domain` / `--out`,
> `cmdutil.ResolveExportDir`, exit 2 for "cannot answer" and a dry-run that writes nothing. Unlike its
> `writeIndexFile` (plain `os.WriteFile`), write with `docs.WriteFileAtomic`. Tree ownership follows `resource
> drift`: `drift.ClearTree`, `WriteObservation` and `rebaselineClearDrift` in `cmd/resource/download.go`.
> Assignment parsing exists in `internal/docs/assignments.go` (`parseAssignments`, `buildGroupInfo`,
> `buildFilterInfo`, the zero-GUID filter sentinel). `metadata.yaml` carries raw `assignmentTargets`,
> `groupTypes`, `platforms`, `odataType` — not a group's `membershipRule` nor a filter's `platform`/`rule`, which
> come from the groups' and filters' YAML. ADMX presentation values are not exported (parked idea *export ADMX
> presentation values*). No repository fixtures exist for ADMX `definitionValues`, intent `settings`,
> `assignmentFilters` or collection-valued Settings Catalog settings.
>
> **Compliance is not only a check.** On macOS (and iOS/iPadOS) several compliance settings — the password ones in
> particular — are applied to the device, not only evaluated. A compliance policy and a configuration profile can
> then enforce the same control with different values, or a configuration can enforce a value the compliance check
> rejects (devices non-compliant by design). Both are mechanically decidable once the catalog says which compliance
> property and which configuration keys describe the same control: the `equivalences` of the entry *A consistency
> rule and topic catalog in the configuration*. This entry builds the alias hook and the comparison; the catalog
> entry supplies the real mappings. Without a catalog the detector works on exact keys only.
>
> **Not regeneration-gated.** No `*_prompt.tmpl` changes; no document changes. Nothing regeneration-gated needs to
> ride along.
>
> **Contract.** No Go → web contract in this entry. It creates `<tenant>/consistency/` with two Go-internal files the
> browser never serves: `metadata.yaml` (`version: 1`, `tenant`, `exportGeneratedAt`, `exportComplete`,
> `toolVersion`, counts) and `mechanical.yaml` (`version: 1`, `findings`, `unknownValues`). The browser's tenant
> discovery keys on `docs/` only, so a new sibling tree changes nothing for the current web code. The
> browser-facing `consistency/index.md` and the root `CLAUDE.md` contract line arrive with the entry *The
> consistency analysis job*, whose preflight relies on `consistency/metadata.yaml` `exportGeneratedAt` equalling
> `resources/metadata.yaml` `generatedAt` and on the finding shape of `mechanical.yaml`; an incompatible shape change
> bumps that file's `version`. A re-baselining `resource download` removes the tree. Apple custom profiles appear
> under the keys `#microsoft.graph.<macOS|ios>CustomConfiguration#payload:<PayloadIdentifier>` and
> `#microsoft.graph.macOSCustomAppConfiguration#configurationXml:<bundleId>`, valued `sha256:<hex>` of the
> normalised payload text (inline or sidecar alike); no payload content ever reaches `consistency/`. A nested Apple
> Settings Catalog collection key only yields a `duplicate` valued by the JSON list of the shared members; the
> counts carry `ruledOutByScope` (distinct same-key resource pairs whose overlap is `none`).
> Typed Graph defaults listed per property (`unavailable` threat levels, `deviceDefault` password and passcode
> types) are not configured and never indexed: fewer findings, the same shape, both `version`s stay 1.
>
> **Owner.** `.claude/rules/go-export-safety.md` (go). Root `CLAUDE.md` is not touched here. No sequencing: no web
> side.
>
> **Implementer.** sonnet

**Plan.**

- ✅ New package `internal/consistency` and command `docs analyze-consistency` (`cmd/docs/analyze_consistency.go`,
  registered in `cmd/docs.go`), offline with `--domain` like `docs generate-index` (`exportCredential`,
  `cmdutil.ResolveExportDir`). Flags: `--domain` only — split `addExportFlags` in `cmd/docs/flags.go` so the domain
  flag and its completion can be declared alone; the command writes a fixed tree, so `--out` (and `--prompt`)
  wait for the analysis prompt of the entry *The consistency analysis job* (a flag the command ignores is
  forbidden). Surface tests: `cmd/docs_test.go` (offers `--domain`; does not offer `--out`, `--prompt`,
  `--exit-code`) and the docs subcommand list in `cmd/resource_test.go`. Exit 2 (`exitCannotAnswer`) without
  `resources/metadata.yaml` (`docs.ErrNoMetadata`), on a tenant mismatch (`docs.ErrTenantMismatch`) or when a write
  fails; findings never change the exit code.
- ✅ Inputs: every `resources/metadata.yaml` entry with `presentInTenant: true` and neither `skipped` nor `filtered`, of
  the indexed types below, read from `resources/<metadata key>`; a file that is missing or does not parse is warned
  about by key, counted as `unreadable` and skipped — the run continues. Other types are not indexed in v1.
- ✅ Setting index `(resource key, source type, canonicalKey, value, class)`:
  `Microsoft.Graph/deviceManagementConfigurationPolicies` and `Microsoft.Graph/compliancePolicies` by
  `settingDefinitionId`, walking choice children and group collections, choice option ids, simple and collection
  values in one comparable form; `Microsoft.Graph/deviceConfigurations` custom profiles by each `omaSettings[]`
  `omaUri`, normalised (lowercase, leading `./` dropped, `vendor/msft/…` read as `device/vendor/msft/…`, `/` → `_`)
  so a Policy CSP path lands on the Settings Catalog id `device_vendor_msft_policy_config_…` (or `user_vendor_msft_…`)
  — the bridge, without splitting area and setting (ADMX-backed ids contain underscores); across the bridge a choice
  compares by its option suffix against the OMA-URI value as a string, a non-scalar side (an ADMX `<enabled/>`
  payload) goes to `unknownValues`; a value the export moved to a sidecar artifact compares by the artifact bytes'
  SHA-256; other `deviceConfigurations` and all `Microsoft.Graph/deviceCompliancePolicies` by
  `@odata.type#property` (so only within one `@odata.type`), nested objects flattened to dotted paths;
  `Microsoft.Graph/groupPolicyConfigurations` by `definitionValues[].definition.id` with value `enabled`
  (presentation values are not exported); `Microsoft.Graph/deviceManagementIntents` by `settings[].definitionId` with
  the parsed `valueJson` in canonical JSON. Class `requirement` for `deviceCompliancePolicies` and
  `compliancePolicies`, `configuration` for the rest.
- ✅ Typed properties that are not settings: `id`, `displayName`, `description`, `version`, `createdDateTime`,
  `lastModifiedDateTime`, `roleScopeTagIds`, `assignments`, `scheduledActionsForRule`, `omaSettings` and every key
  starting with or containing `@odata.`. Not configured, never indexed: null, empty string or list, the enum value
  `notConfigured`, and boolean `false` (the legacy profiles render it "Not configured"; an explicit `false` is missed
  rather than every default reported as a duplicate).
- ✅ Secrets never leave the export: a Settings Catalog secret (`valueState` present), an `isEncrypted` or masked
  OMA-URI value is indexed as `set (value unknown)` even when `resolve-secrets` wrote it in plaintext — never written
  under `consistency/`, never logged, never compared. A pair with an unknown value is never a value finding; it is
  listed under `unknownValues`.
- ✅ Alias hook for equivalences: an `Equivalence` type in `internal/consistency` joins canonical keys through an
  equivalence table (the catalog entry's `equivalences`; the command passes an empty table here, the tests a fixture
  table). Each equivalence names the members (a compliance property and/or configuration keys, in the index's
  canonical form), a relation — `same` (both sides enforce the control) or a check operator `>=`, `<=`, `=`,
  `required` with the compliance side as the requirement — whether the compliance side is enforced on the device
  per platform, and a `status` (`verified` | `verify`).
- ✅ Scope model per resource from its `assignmentTargets`, built on `internal/docs/assignments.go` — export what the
  package needs (`parseAssignments`, the row fields, the zero-GUID sentinel handling) instead of copying it: include
  and exclude sets (groups, All devices, All users), filters with mode, and a target kind per group (`device.` vs
  `user.` in a dynamic group's `membershipRule`, read from the group's YAML; assigned groups `unknown`); a filter's
  `platform` and `rule` from the `Microsoft.Graph/assignmentFilters` YAML; the resource's platform from metadata
  `platforms`, else the `@odata.type` prefix (`windows…`, `macOS…`, `ios…`, `android…`), else `unknown`.
- ✅ Overlap verdict per resource pair: `none` when both platforms are known and differ; when either side has no
  include; when one side's include groups all sit in the other side's exclude set and that exclusion is honoured
  (Intune ignores an exclusion that mixes kinds — a known `device.` group excluded from a user include or All users,
  or the reverse — so a known mismatch stays `possible`; an `unknown` kind counts as matching); or when every include
  on one side carries a filter in `include` mode that every include on the other side carries in `exclude` mode.
  Target kind alone never decides `none`: a user assignment reaches the devices its users sign in to. `certain` when
  the same group, or All devices on both, or All users on both, is included without a filter and excluded on neither
  side, on the same known platform; `possible` otherwise. Each pair also records the target kinds and filter ids.
- ✅ Mechanical findings, one per resource pair: a canonical key (or an equivalence group) set in two or more resources
  whose overlap is not `none` is a `conflict` (two enforcing sides with different values — two configurations, or an
  enforcing compliance setting and a configuration), a `duplicate` (same value), or a `contradiction` (a
  configuration value that fails a compliance requirement under the equivalence's operator, e.g. an enforced minimum
  password length of 8 against a required 12), each carrying its overlap verdict, the operator and both values, and
  `confidence: firm | possible` — `possible` when it rests on an equivalence with `status: verify`, never a hard
  conflict. A compliance requirement with no in-scope configuration is not a mechanical finding (rule R1 in the
  analysis job).
- ✅ Output, both via `docs.WriteFileAtomic` (directory `0755`, files `0644`), `mechanical.yaml` first and
  `metadata.yaml` last: `<tenant>/consistency/mechanical.yaml` — `version: 1`, `findings` sorted by kind, key, then
  the two resource keys (lower first), each with `kind`, `key` (the canonical key or `equivalence:<id>`), `overlap`,
  `confidence`, `operator` (equivalences only) and `a` / `b` (`resource` metadata key, `sourceKey`, `value`); and
  `unknownValues` — the pairs on one key with overlap other than `none` and an unknown value, without values.
  `consistency/metadata.yaml` — `version: 1`, `tenant`, `exportGeneratedAt` (the export's `generatedAt`),
  `exportComplete`, `toolVersion`, counts (indexed resources and settings per source type, `unreadable`, findings per
  kind × overlap, `unknownValues`); no wall-clock time, so reruns are byte-equal. Other files in `consistency/` are
  left alone. `--dry-run` writes and clears nothing and reports the counts; a summary line per kind ends the run.
- ✅ Tree ownership: `consistency.ClearTree` (its own `DirName` constant and a constructed path, like `drift.ClearTree`),
  called by `resource download` next to `rebaselineClearDrift` — after a successful metadata write, never on dry-run,
  a failure only warns. `.claude/rules/go-export-safety.md` (and its Windsurf twin
  `go/.windsurf/rules/04-security-and-ops.md`): a `consistency/` bullet under *Output layout*, the "only other
  deletes" sentence and the prune exclusion list.
- ✅ Tests (`internal/consistency`, synthetic fixtures in temp directories only): the normalisers per source type,
  the not-configured and non-setting exclusions, the OMA-URI ↔ Settings Catalog bridge (including an ADMX-backed id
  with underscores and a non-scalar side landing in `unknownValues`), a resolved secret value never appearing in
  either output file, the overlap truth table (including a mixed-kind exclusion staying `possible` and a user-only vs
  device-only pair staying `possible`), the alias hook with a fixture equivalence table (a macOS compliance password
  length against a configuration profile's: `conflict` when both enforce, `contradiction` when the configuration
  fails the requirement, confidence `possible` for a `verify` equivalence), mechanical detection on a synthetic
  tenant (including an L1/Admin complement pair — one policy excludes an assigned admin group, its twin includes only
  it — that must come out `none`), an unreadable resource warned and counted, a `presentInTenant: false` entry not
  indexed, byte-equal reruns, dry-run writing nothing, the refusals; `ClearTree` and the re-baseline call in
  `cmd/resource/download_test.go` (dry run clears nothing, a real run clears only `consistency/`).
- ✅ Compare Apple Settings Catalog collections as additive lists, not as one folded value (found in a real export:
  every certain new↔new conflict was a collection entry — managed login item rules
  `com.apple.servicemanagement_rules_item_*`, allowed system extensions
  `com.apple.system-extension-policy_allowedsystemextensions_*`, privacy entries
  `com.apple.tcc.configuration-profile-policy_services_*_item_*` — where each policy adds its own entries and macOS
  installs every profile's entries side by side):
  - Today `walkCatalogInstance` recurses into each `groupSettingCollectionValue` element and the collector folds
    every child occurrence into one sorted list per child key, so two policies with different entries conflict on
    each child key. Additive applies when **every** platform family of the resource is `macos` or `ios` —
    `resourcePlatforms(entry)` from `scope.go`, computed in `buildIndex` and passed into `indexResource` /
    `indexSettingsCatalog` (both `deviceManagementConfigurationPolicies` and `compliancePolicies`); an `unknown`,
    mixed or non-Apple platform keeps today's folding.
  - Only **nested** collections are additive: the `settingInstance` of a `settings[]` item itself (the Apple payload
    instance, a group-setting collection with one element per payload) is never a member list — its children stay
    keys as today, so two macOS policies setting the same payload key (`com.apple.loginwindow_…`,
    `com.apple.applicationaccess_…`) differently still `conflict`. Below that level a `groupSettingCollectionValue`
    is indexed as list members under the collection's lowercased `settingDefinitionId`: one member per element,
    valued by the canonical JSON of the element's children indexed into a fresh collector of the same family
    (`{key: value}` of its folded settings, so option ids, simple values, the not-configured and secret rules apply
    unchanged); a collection nested inside an element is itself member-ised under its own id and left out of the
    outer element's value (the TCC services → per-service items shape); an element with no configured child adds
    no member; an element whose fresh collector holds an unknown setting is an **unknown member** (no value
    kept). The element's children are not indexed as separate keys. A nested `simpleSettingCollectionValue` or
    `choiceSettingCollectionValue` becomes members the same way, one per value (a secret simple value → unknown
    member; a choice option's children are still walked as their own keys).
  - `Setting` gains `Members []string` (sorted, distinct known member values) and `UnknownMembers bool`, and a
    list-member flag; such a setting has an empty `Value`/`Scalar`, never crosses the OMA-URI bridge or an
    equivalence, and counts as one setting per key and resource in `counts.indexed`.
  - Detection for a list-member key (in `judge`, after the scope check, which runs first as today): shared known
    members → one `duplicate` per pair, `a.value` and `b.value` both the canonical JSON list of the shared
    members; no shared known member and no unknown member on either side → no finding; no shared known member and
    an unknown member on either side → listed under `unknownValues`; never a `conflict`. A pair where only one side
    is a list-member setting (one key, one Apple-only and one folded resource) → `unknownValues`. An unknown member
    is never compared and never written. Other platforms keep today's folded-list comparison (Windows Settings
    Catalog merges only some collections; the catalog entry *A consistency rule and topic catalog in the
    configuration* can mark more collections additive later).
  - Tests (`index_test.go`, `consistency_test.go`, synthetic catalog policies): two macOS policies with different
    login-item rules → no finding; one shared rule → `duplicate` whose values are the shared rule only; an iOS pair
    behaves alike; two macOS policies setting one payload-level key differently → still `conflict`; a Windows
    policy pair with different nested collection values stays a `conflict`; a TCC-shaped nested collection yields
    members at the inner level only; a collection element holding a secret is never compared or written (and a
    disjoint pair with it lands in `unknownValues`); the children of an additive collection are no longer separate
    keys; a policy with platforms `macOS, windows10` keeps folding.
- ✅ Count the pairs the scope check ruled out: `counts.ruledOutByScope` in `consistency/metadata.yaml` (field after
  `unknownValues` in `Counts`, always written, `0` included) — the number of **distinct resource pairs** that
  reached `evaluate` on at least one shared key or equivalence but whose overlap verdict is `none`, so no finding
  or unknown value was written for them; a pair ruled out on several keys counts once. The detector records them
  in a set beside `findings` / `unknowns` and `detect` returns the count to `Analyze` for `countAll`. Logged as
  `ruled_out_by_scope` on the `Consistency summary` line in `cmd/docs/analyze_consistency.go`. `version` stays 1
  (the file has not shipped). Tests: a fixture with an excluded pair sharing two keys counts 1; a `none` pair that
  shares no key counts 0; a dry run reports the same count; reruns stay byte-equal.
- ✅ Treat typed enum defaults as not configured (found in a real export: 27 of the 28 certain duplicates between
  split compliance policies were Graph defaults that every legacy compliance policy returns whether or not the
  operator set them — `deviceThreatProtectionRequiredSecurityLevel: unavailable`,
  `advancedThreatProtectionRequiredSecurityLevel: unavailable`, `passwordRequiredType: deviceDefault`):
  - A `typedEnumDefaults map[string][]string` in `internal/consistency/index.go`, next to `typedNonSettings`,
    keyed by the bare top-level property name (any `@odata.type`), listing the value(s) that mean "not
    configured": `deviceThreatProtectionRequiredSecurityLevel` and `advancedThreatProtectionRequiredSecurityLevel`
    → `unavailable`; `passwordRequiredType` → `deviceDefault`; plus `passcodeRequiredType` → `deviceDefault`, the
    iOS/iPadOS compliance spelling of the same Graph default. An explicit per-property list, never a blanket rule on
    the strings (`unavailable` or `deviceDefault` may be a real choice on another property).
  - In `walkTyped`, next to the existing `notConfigured(v, true)` return: a leaf whose `path` has no `.` (a
    top-level property; a nested path such as `x.passwordRequiredType` is never matched) and whose value is a string
    equal under `strings.EqualFold` to one of its table entries is skipped — not indexed, so it yields no finding,
    no `unknownValues` pair and no equivalence member. `notConfigured` itself and its exact `notConfigured`
    comparison stay as they are; `typedLeaf` and `typedNonSettings` are unchanged. Settings Catalog, OMA-URI, ADMX
    and intents are unchanged.
  - Tests, `internal/consistency/index_test.go` (index level, beside `TestIndexTypedProperties`): the four defaults
    are not indexed, also as `Unavailable` / `DeviceDefault`; `unavailable` on a property not in the table and
    `passwordRequiredType: alphanumeric` are still indexed; a nested `deviceDefault` under a table name is still
    indexed. `internal/consistency/consistency_test.go` (run level, synthetic fixtures in a temp directory): two
    compliance policies of one `@odata.type` on one scope that both carry the defaults → no finding; one
    `passwordRequiredType: alphanumeric`, the other `deviceDefault` → no finding (the default side is not
    configured); both `alphanumeric` → `duplicate`.
- ✅ Documentation at *done*: `README.md` (the command, the `consistency/` tree, where it sits in the pipeline);
  `CHANGELOG.md` `### Added`.
- ✅ Key typed macOS custom profiles by their payload identifier, not by their identity properties (found by a dry
  run over a real export: `payload`, `payloadName`, `payloadFileName`, `bundleId`, `configurationXml` and `fileName`
  were indexed as settings, so any two unrelated custom profiles on overlapping scopes came out as a `conflict` —
  the bulk of the mechanical conflicts in that export):
  - Dispatch in `indexResource`'s typed `deviceConfigurations` branch on `@odata.type`: for
    `#microsoft.graph.macOSCustomConfiguration`, `#microsoft.graph.iosCustomConfiguration` (same Graph shape —
    `payloadName`, `payloadFileName`, `payload` — and the same replace-by-identifier semantics) and
    `#microsoft.graph.macOSCustomAppConfiguration`, a new `indexAppleCustomProfile(resource, doc, artifacts,
    typeDir, c)` replaces `indexTypedProperties` entirely, so no other property of these types (`payloadName`,
    `payloadFileName`, `fileName`, `bundleId`, `deploymentChannel`, the bare `payload` / `configurationXml`) is
    ever a key. Delete `indexMacOSPayload` and the `payload` / `configurationXml` branch of `typedLeaf`.
  - Payload bytes, one helper for both properties: the resource's single sidecar artifact when `entry.Artifacts`
    has exactly one (file mode: `payload` → `<payloadFileName stem>.mobileconfig`, `configurationXml` → named
    after `fileName`); else the inline string; an inline string that does not start with `<` after trimming is
    base64-decoded first (file mode without `remove-source`, or the `base64-decode` transformer off, leaves the
    Graph base64 in the YAML); a decode failure or an unreadable sidecar indexes nothing.
  - Value: `sha256:<hex>` of the bytes after the export's inline normalisation — export
    `transform.normalizeInlineText` as `NormalizeInlineText` (one definition, used by both) and hash its result
    with trailing newlines trimmed — so an inline copy (BOM stripped, CRLF → LF, trailing blanks trimmed) and a
    byte-exact sidecar copy of one payload hash equal.
  - Custom configuration (macOS, iOS): parse the bytes with `encoding/xml` (no new dependency) as an XML property
    list and read `PayloadIdentifier` from the root `<dict>` only — not from the inner `PayloadContent` payloads,
    which the catalog entry indexes; index one setting `<@odata.type>#payload:<PayloadIdentifier>` (identifier
    trimmed, case kept). Same identifier and different hash → `conflict` (the device keeps one profile per
    identifier); same hash → `duplicate`. Bytes that are not an XML plist (a signed CMS profile, a binary
    `bplist`) or carry no root `PayloadIdentifier` index nothing.
  - Custom app configuration (macOS): the app's preference domain is its `bundleId`; index one setting
    `#microsoft.graph.macOSCustomAppConfiguration#configurationXml:<bundleId>` with the hash above; an empty
    `bundleId` or no payload indexes nothing.
  - Add `supportsScopeTags` to `typedNonSettings` (a read-only Graph capability flag on every typed profile, `true`
    throughout, which otherwise makes every same-type pair a `duplicate`).
  - Whatever indexes nothing logs at DEBUG through `logger.Default` with the resource key and a fixed reason
    only — never payload bytes, identifiers or names.
  - Tests (synthetic plists built in the test, temp directories only): two macOS profiles with one identifier
    and different bytes → `conflict`; different identifiers → no finding; an inline copy and a CRLF sidecar copy
    of one payload → `duplicate`; a still-base64 inline payload indexes like its decoded form; an inner
    `PayloadContent` `PayloadIdentifier` is not the key; an iOS pair behaves like the macOS one; two app
    configurations on one `bundleId` with different XML → `conflict`; a signed/binary payload and a plist
    without `PayloadIdentifier` index nothing; `payloadName`, `payloadFileName`, `fileName`, `bundleId`,
    `deploymentChannel` and `supportsScopeTags` are no longer keys; no payload string appears in either output
    file.

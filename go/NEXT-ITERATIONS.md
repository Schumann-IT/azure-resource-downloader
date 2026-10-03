# Next iterations

Outstanding work and parked ideas for the Go CLI. `README.md` says what it does today and `CHANGELOG.md` what
shipped and why; neither is repeated here. How entries and ideas are written, promoted, implemented and
archived is `../.claude/rules/next-iterations.md`.

Numbered entries are scheduled work: committed here before they are implemented, struck through as they land,
and archived to `../.claude/archive/go/` once done. Parked ideas, grouped by area below, are
deliberately unscheduled; each says why it is parked and what would make it worth doing.

## 1. Index settings and assignment scopes and report same-setting conflicts

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
> **Decision.** macOS custom profiles: key them by their payload identifier (`PayloadIdentifier`; `bundleId` for
> app configurations) rather than treating them as non-settings or leaving them to the catalog.
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
> bumps that file's `version`. A re-baselining `resource download` removes the tree.
>
> **Owner.** `.claude/rules/go-export-safety.md` (go). Root `CLAUDE.md` is not touched here. No sequencing: no web
> side.
>
> **Implementer.** opus

**Plan.**

- ~~New package `internal/consistency` and command `docs analyze-consistency` (`cmd/docs/analyze_consistency.go`,
  registered in `cmd/docs.go`), offline with `--domain` like `docs generate-index` (`exportCredential`,
  `cmdutil.ResolveExportDir`). Flags: `--domain` only — split `addExportFlags` in `cmd/docs/flags.go` so the domain
  flag and its completion can be declared alone; the command writes a fixed tree, so `--out` (and `--prompt`)
  wait for the analysis prompt of the entry *The consistency analysis job* (a flag the command ignores is
  forbidden). Surface tests: `cmd/docs_test.go` (offers `--domain`; does not offer `--out`, `--prompt`,
  `--exit-code`) and the docs subcommand list in `cmd/resource_test.go`. Exit 2 (`exitCannotAnswer`) without
  `resources/metadata.yaml` (`docs.ErrNoMetadata`), on a tenant mismatch (`docs.ErrTenantMismatch`) or when a write
  fails; findings never change the exit code.~~
- ~~Inputs: every `resources/metadata.yaml` entry with `presentInTenant: true` and neither `skipped` nor `filtered`, of
  the indexed types below, read from `resources/<metadata key>`; a file that is missing or does not parse is warned
  about by key, counted as `unreadable` and skipped — the run continues. Other types are not indexed in v1.~~
- ~~Setting index `(resource key, source type, canonicalKey, value, class)`:
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
  `compliancePolicies`, `configuration` for the rest.~~
- ~~Typed properties that are not settings: `id`, `displayName`, `description`, `version`, `createdDateTime`,
  `lastModifiedDateTime`, `roleScopeTagIds`, `assignments`, `scheduledActionsForRule`, `omaSettings` and every key
  starting with or containing `@odata.`. Not configured, never indexed: null, empty string or list, the enum value
  `notConfigured`, and boolean `false` (the legacy profiles render it "Not configured"; an explicit `false` is missed
  rather than every default reported as a duplicate).~~
- ~~Secrets never leave the export: a Settings Catalog secret (`valueState` present), an `isEncrypted` or masked
  OMA-URI value is indexed as `set (value unknown)` even when `resolve-secrets` wrote it in plaintext — never written
  under `consistency/`, never logged, never compared. A pair with an unknown value is never a value finding; it is
  listed under `unknownValues`.~~
- ~~Alias hook for equivalences: an `Equivalence` type in `internal/consistency` joins canonical keys through an
  equivalence table (the catalog entry's `equivalences`; the command passes an empty table here, the tests a fixture
  table). Each equivalence names the members (a compliance property and/or configuration keys, in the index's
  canonical form), a relation — `same` (both sides enforce the control) or a check operator `>=`, `<=`, `=`,
  `required` with the compliance side as the requirement — whether the compliance side is enforced on the device
  per platform, and a `status` (`verified` | `verify`).~~
- ~~Scope model per resource from its `assignmentTargets`, built on `internal/docs/assignments.go` — export what the
  package needs (`parseAssignments`, the row fields, the zero-GUID sentinel handling) instead of copying it: include
  and exclude sets (groups, All devices, All users), filters with mode, and a target kind per group (`device.` vs
  `user.` in a dynamic group's `membershipRule`, read from the group's YAML; assigned groups `unknown`); a filter's
  `platform` and `rule` from the `Microsoft.Graph/assignmentFilters` YAML; the resource's platform from metadata
  `platforms`, else the `@odata.type` prefix (`windows…`, `macOS…`, `ios…`, `android…`), else `unknown`.~~
- ~~Overlap verdict per resource pair: `none` when both platforms are known and differ; when either side has no
  include; when one side's include groups all sit in the other side's exclude set and that exclusion is honoured
  (Intune ignores an exclusion that mixes kinds — a known `device.` group excluded from a user include or All users,
  or the reverse — so a known mismatch stays `possible`; an `unknown` kind counts as matching); or when every include
  on one side carries a filter in `include` mode that every include on the other side carries in `exclude` mode.
  Target kind alone never decides `none`: a user assignment reaches the devices its users sign in to. `certain` when
  the same group, or All devices on both, or All users on both, is included without a filter and excluded on neither
  side, on the same known platform; `possible` otherwise. Each pair also records the target kinds and filter ids.~~
- ~~Mechanical findings, one per resource pair: a canonical key (or an equivalence group) set in two or more resources
  whose overlap is not `none` is a `conflict` (two enforcing sides with different values — two configurations, or an
  enforcing compliance setting and a configuration), a `duplicate` (same value), or a `contradiction` (a
  configuration value that fails a compliance requirement under the equivalence's operator, e.g. an enforced minimum
  password length of 8 against a required 12), each carrying its overlap verdict, the operator and both values, and
  `confidence: firm | possible` — `possible` when it rests on an equivalence with `status: verify`, never a hard
  conflict. A compliance requirement with no in-scope configuration is not a mechanical finding (rule R1 in the
  analysis job).~~
- ~~Output, both via `docs.WriteFileAtomic` (directory `0755`, files `0644`), `mechanical.yaml` first and
  `metadata.yaml` last: `<tenant>/consistency/mechanical.yaml` — `version: 1`, `findings` sorted by kind, key, then
  the two resource keys (lower first), each with `kind`, `key` (the canonical key or `equivalence:<id>`), `overlap`,
  `confidence`, `operator` (equivalences only) and `a` / `b` (`resource` metadata key, `sourceKey`, `value`); and
  `unknownValues` — the pairs on one key with overlap other than `none` and an unknown value, without values.
  `consistency/metadata.yaml` — `version: 1`, `tenant`, `exportGeneratedAt` (the export's `generatedAt`),
  `exportComplete`, `toolVersion`, counts (indexed resources and settings per source type, `unreadable`, findings per
  kind × overlap, `unknownValues`); no wall-clock time, so reruns are byte-equal. Other files in `consistency/` are
  left alone. `--dry-run` writes and clears nothing and reports the counts; a summary line per kind ends the run.~~
- ~~Tree ownership: `consistency.ClearTree` (its own `DirName` constant and a constructed path, like `drift.ClearTree`),
  called by `resource download` next to `rebaselineClearDrift` — after a successful metadata write, never on dry-run,
  a failure only warns. `.claude/rules/go-export-safety.md` (and its Windsurf twin
  `go/.windsurf/rules/04-security-and-ops.md`): a `consistency/` bullet under *Output layout*, the "only other
  deletes" sentence and the prune exclusion list.~~
- ~~Tests (`internal/consistency`, synthetic fixtures in temp directories only): the normalisers per source type,
  the not-configured and non-setting exclusions, the OMA-URI ↔ Settings Catalog bridge (including an ADMX-backed id
  with underscores and a non-scalar side landing in `unknownValues`), a resolved secret value never appearing in
  either output file, the overlap truth table (including a mixed-kind exclusion staying `possible` and a user-only vs
  device-only pair staying `possible`), the alias hook with a fixture equivalence table (a macOS compliance password
  length against a configuration profile's: `conflict` when both enforce, `contradiction` when the configuration
  fails the requirement, confidence `possible` for a `verify` equivalence), mechanical detection on a synthetic
  tenant (including an L1/Admin complement pair — one policy excludes an assigned admin group, its twin includes only
  it — that must come out `none`), an unreadable resource warned and counted, a `presentInTenant: false` entry not
  indexed, byte-equal reruns, dry-run writing nothing, the refusals; `ClearTree` and the re-baseline call in
  `cmd/resource/download_test.go` (dry run clears nothing, a real run clears only `consistency/`).~~
- Documentation at *done*: `README.md` (the command, the `consistency/` tree, where it sits in the pipeline);
  `CHANGELOG.md` `### Added`.
- Key typed macOS custom profiles by their payload identifier, not by their identity properties (found by a dry
  run over a real export: `payload`, `payloadName`, `payloadFileName`, `bundleId`, `configurationXml` and `fileName`
  were indexed as settings, so any two unrelated custom profiles on overlapping scopes came out as a `conflict` —
  the bulk of the mechanical conflicts in that export):
  - `macOSCustomConfiguration`: read the top-level `PayloadIdentifier` of the XML property list (inline `payload`,
    or the sidecar artifact the export moved it to) with `encoding/xml` — no new dependency; index one setting
    `#microsoft.graph.macOSCustomConfiguration#payload:<PayloadIdentifier>` whose value stays the `sha256:<hex>` of
    the payload bytes (inline and sidecar hash alike). Same identifier and different bytes → `conflict` (the device
    keeps one profile per identifier); same bytes → `duplicate`.
  - `macOSCustomAppConfiguration`: the app's preference domain is its `bundleId`; index one setting
    `#microsoft.graph.macOSCustomAppConfiguration#configurationXml:<bundleId>` with the `sha256:<hex>` of
    `configurationXml`.
  - `payloadName`, `payloadFileName`, `fileName`, `bundleId` and the bare `payload` / `configurationXml` become
    non-settings for these two types.
  - A payload that is not an XML property list (signed or binary) or has no `PayloadIdentifier` indexes nothing;
    log it at DEBUG with the resource key only, never payload content.
  - Tests: two profiles with one identifier and different bytes → `conflict`; different identifiers → no finding;
    inline and sidecar copies of the same payload → `duplicate`; a signed/binary payload indexes nothing; the
    identity properties are no longer keys.

## 2. A consistency rule and topic catalog in the configuration

*Kind:* feat

**Goal.** The operator can tell the consistency analysis which settings describe the same control across policy
types (compliance included), which cross-type relations to check, and how to group resources into topics — each
with its resolution semantics and its Microsoft Learn source written down — so the mechanical step can compare
compliance against configuration and the LLM step judges against a reviewed catalog instead of recall.

> **Decision.** The catalog is a configuration key like `taxonomy:` (operator-editable, validated), not embedded
> data.
>
> **Shape.** A general key `consistency:` with `equivalences` (id, members — a compliance property and/or
> configuration keys: Settings Catalog `settingDefinitionId`s, OMA-URI paths, legacy `@odata.type#property` —, a
> relation `same | >= | <= | = | required`, `enforced` per platform for the compliance side, a Microsoft Learn
> reference, `status: verified | verify`), `topics` (id, label, match patterns over setting keys,
> `@odata.type`s and type names; a resource may sit in several topics) and `rules` (id, description, left and right
> selectors, what counts as a violation, resolution semantics — which setting wins —, a Microsoft Learn reference,
> `status: verified | verify`). Seed rules from the Cowork draft, all to be verified: R1 compliance requires
> encryption/Defender/firewall but no in-scope configuration enables it; R2 compliance minimum OS above what the
> update rings / feature-update profiles deliver; R3 a CA policy requires a compliant device but the targeted users
> have no compliance policy for a covered platform; R4 the same control configured through several surfaces; R5
> update-ring feature deferral with a feature-update profile on an overlapping scope; R6 a filter property that
> contradicts the policy's platform or enrollment type; R7 enrollment/ESP/Autopilot targeting inconsistent with its
> dynamic group rule; R8 a macOS Platform SSO / account setting conflicting with password compliance.
>
> **Equivalences replace the parked mapping idea.** Joining a legacy typed property and a Settings Catalog / CSP
> setting is the same problem as joining a compliance property and a configuration setting, so the former parked
> idea *a curated legacy-property → CSP mapping* is part of `equivalences`.
>
> **Microsoft Learn check by Claude Cowork.** The seed catalog's semantics — which compliance settings are enforced
> on which platform, which setting wins, which keys describe one control — need sourced verification, which a Cowork
> session with web access does best (as for the metadata and prompt reviews). The entry's last bullet writes its
> brief; the result enters the backlog later like those reviews did.
>
> **Not regeneration-gated.**

**Plan.**

- The `consistency:` key through `/add-config-option`: a general key in `internal/config` (`keyScopes`), read by
  `docs analyze-consistency` like `generate-index` reads `taxonomy:`; types, compilation and validation in
  `internal/consistency` (id pattern, selectors and equivalence members resolve against the index's source types,
  relations from the closed set, references are `https://learn.microsoft.com/…`), with errors naming the offending
  entry; the compiled `equivalences` feed the mechanical detector's alias hook.
- The compiled catalog's hash in `consistency/metadata.yaml`; an absent catalog means mechanical findings only.
- Both example files stay no-ops (the key illustrated in comments only); the worked example
  `config-tailored-intune.yaml` carries a seed catalog — equivalences: the macOS and iOS/iPadOS password and
  passcode family first (compliance ↔ Settings Catalog ↔ legacy device restrictions), then encryption
  (FileVault/BitLocker), firewall, minimum OS version; topics: encryption, Defender/EDR, firewall, Windows Update,
  identity & sign-in, browsers, Office/OneDrive, enrollment, macOS accounts/SSO; rules R1–R8 — each entry's
  semantics and reference checked against Microsoft Learn where the implementer can; anything not settled stays
  `status: verify`, never guessed.
- Tests: validation errors (unknown relation, unresolvable member, non-Learn reference), compilation, the catalog
  hash, the equivalences reaching the detector (a seed macOS password equivalence turns a fixture pair into a
  `contradiction`), `cmd/config_test.go` partition coverage.
- Documentation at *done*: `README.md` (the key, its three sections, the seed catalog); `CHANGELOG.md` `### Added`.
- Last, after every other bullet: write the Claude Cowork review brief `Claude
  outputs/consistency-catalog-review-instructions.md` (git-ignored, never committed — input for a Cowork session,
  not repository documentation). It tells Cowork to read the seed catalog in `go/config-tailored-intune.yaml`
  (`consistency:` — equivalences, topics, rules), the README's description of the key and its closed vocabularies,
  and the indexed source types; to check every entry against Microsoft Learn — that the members of each
  equivalence describe the same control, the relation and operator, which compliance settings are enforced on
  which platform (macOS, iOS/iPadOS, Windows, Android), each rule's violation and resolution semantics ("which
  setting wins"), and that every reference URL resolves and supports the claim; to name missing high-value
  equivalences and rules; and to return a change plan in the shape of the earlier reviews — per entry a verdict
  (keep / fix / drop / add), ready-to-paste YAML, `status: verified` only with a cited Learn source, open
  questions where Learn is silent — with no repository edits and nothing read under `output/`.

## 3. The consistency analysis job

*Kind:* feat

**Goal.** A documentation agent can judge the cross-type relations the mechanical step cannot decide — within
bounded topic clusters, against the rule catalog — and every finding it writes is checked by a script against the
export and the tool's overlap matrix before it is accepted, so the result is a verified list of contradictions an
operator can act on.

> **Mirrors** `docs analyze-drift`: the CLI writes a prompt file with marked blocks; a Claude session runs it as
> its own two-phase job. Precedent: `cmd/docs/analyze_drift.go` and `internal/drift/analyzeprompt.go` (embedded
> template, `docs.ValidateMarkers` / `SpliceMarker` / `StripTemplateHeader`, the `CheckCurrent` preflight,
> `--prompt`). Paired with the web entry *The consistency view* (the browser renders
> `consistency/index.md`).
>
> **Contract.** `consistency/` joins the Go → web contract: `consistency/index.md` (the human report, rendered by
> the browser) and `consistency/findings.yaml` (machine-readable); the browser never serves `analyze.md`,
> `metadata.yaml`, `mechanical.yaml`, `findings.yaml` or `chunks/`.
>
> **Owner.** `documentation-agent-instructions.md` (repository root) gets the third job; the root `CLAUDE.md`
> Go → web contract list gains `consistency/`.
>
> **Not regeneration-gated.** The new prompt template is not hashed.

**Plan.**

- `docs analyze-consistency` also writes `consistency/analyze.md` from an embedded template (`go:embed`, atomic
  write, header stripped) with marked blocks `export`, `mechanical`, `clusters` (deterministic per topic, split by
  expected output size like `generate.md` §3), `overlap` (a compact pair matrix per cluster) and `rules`; preflight
  like `drift.CheckCurrent` (`exportGeneratedAt` equals `resources/metadata.yaml` `generatedAt`); `--prompt`
  substitutes a template.
- The template's procedure: one agent per cluster writing `consistency/chunks/NN.json` against a fixed schema; a
  shipped verification script (stdlib Python ≥ 3.9, read-only, same conventions as the run prompt's §6/§7 scripts)
  that fails a finding whose cited source or key is missing, whose cited value differs from the YAML (masked
  excepted), whose overlap disagrees with the matrix, whose vocabulary is outside the closed sets (`rule` from the
  catalog or `mechanical` / `other` with a justification; `kind` conflict | duplicate | contradiction | gap |
  ineffective | precedence; `severity` critical | high | medium | low; `overlap` certain | possible), or that
  restates a mechanical finding without citing it; then the orchestrator writes `consistency/findings.yaml`,
  `consistency/index.md` and a report.
- `documentation-agent-instructions.md`: the third job "analyze consistency for `<tenant>`" — prompt file,
  preflight, the two-phase gate, the allow-list (`consistency/chunks/**`, `findings.yaml`, `index.md`,
  `report-*.md`) and never `analyze.md`, `metadata.yaml`, `mechanical.yaml`, `docs/`, `resources/`.
- Root `CLAUDE.md`: `consistency/` in the Go → web contract list.
- Tests: the template validates its markers; rendering on a synthetic tenant; the verification script run on
  fixtures rejects planted bad findings (wrong value, wrong overlap, bad vocabulary, duplicate of a mechanical
  finding) and accepts a correct one.
- Documentation at *done*: `README.md` (the job, the `consistency/` files); `CHANGELOG.md` `### Added`.

## 4. Feed the consistency findings into the tenant summary

*Kind:* feat

**Goal.** The tenant summary's *consistency* paragraph says something real: it judges from the verified consistency
findings when an analysis exists for this export, and says plainly that none was run when it does not.

> **From the code.** The gate follows `internal/drift/analyzeprompt_audit.go` (`loadAttributionState`: absent /
> unreadable / outdated / current; never fails the run). The summary contract — `# Tenant summary`, the four H2s,
> `### Findings` / `### Recommendations`, severity `critical | high | medium` — stays; the five-signal sweep stays
> untouched.
>
> **Not regeneration-gated.** The run prompt is not hashed.

**Plan.**

- `docs generate-prompt` reads `consistency/findings.yaml` and renders a new `consistency` marker block (added to
  `requiredMarkers`): counts by kind × severity, the top findings linked to `consistency/index.md`, and the state —
  rendered as findings only when its `exportGeneratedAt` equals `resources/metadata.yaml` `generatedAt`, otherwise
  as "absent", "unreadable" or "outdated — not used".
- Run prompt §7: four inputs instead of three; the *consistency* paragraph judges from the new block, or says
  "consistency analysis not run for this export"; appendix E explains the fourth input; the summary check script
  unchanged in contract.
- Tests: each state renders as specified; the block carries findings only for a current file.
- Documentation at *done*: `README.md` (the pipeline order: `resource download` → `docs analyze-consistency` → the
  consistency job → `docs generate-prompt` → the documentation job → `docs generate-index`); `CHANGELOG.md`
  `### Changed`.

## Parked ideas

**Legend.** *Area* — **contract** (Go → web data on disk: `index.yaml`, `drift/`, frontmatter, section
headings), **templates** (documentation and analysis prompts; *regen-gated* when it moves `promptSha256`),
**export & metadata** (what `resource download` fetches and records), **drift & compare**, **navigation**
(sidebar, breadcrumbs, landing pages, search, routing), **document view** (how one article renders), **export
formats**, **platform rule** (a non-negotiable itself), **dependencies**, **housekeeping** (lint ledgers,
caching internals). *Impact* — operator value: high / medium / low. *Effort* — S (a day or less), M (one
branch), L (several branches or a design change).

**Ships together.**

1. **The pre-regeneration batch** has shipped: the shared prompt partials, the per-handler metadata, the template
   content fixes with the CA template (paired with web *Style the Conditional Access `Conditions` section*) and the
   consistent templates with the 2026-10-02 prompt review. What is left is one documentation regeneration of every
   tenant. The two regen-gated ideas here (*per-finding severity*, *taxonomy bootstrap*) did not join it and wait
   for the next regeneration.
2. **The compare track** (cross-project, must): *`resource compare`* ships with web *Move the compare
   normalisation to the CLI*; web *manual pairing* and *one-sided resource* follow on the CLI's rule; *version the
   drift observation* rides the first drift contract change.
3. **Housekeeping** is opportunistic: a `gocognit` entry is paid off by whichever entry edits its function.
4. **Export & metadata follow-ups** from the metadata review are independent of each other; *export ADMX
   presentation values* and *Intune branding images* each need a re-baseline after shipping, so they pair well with
   a planned re-download; *mask dedicated secret properties* only joins a regeneration if it extends the prompt
   redaction rule instead of masking.

## Parked ideas — export & metadata

### Idea: ARM values as plain data with their REST names

*Area:* export & metadata · *Impact:* medium · *Effort:* S · *Ships with:* a re-baseline (every storage account and VM changes
once)

The ARM transforms put Azure SDK structs straight into the property map (`armstorage.NetworkRuleSet` and
`Encryption`, `armcompute.OSDisk` and `ImageReference`), so yaml.v3 writes untagged fields under lowercased Go
names and nil pointers as `null`: the export reads `properties.encryption.keysource`,
`properties.networkRuleSet.defaultaction`, `storageProfile.osDisk.deleteoption` instead of the REST names, and drift
deltas print pointer addresses. Round-trip each struct through `encoding/json` into a `map[string]any` (the SDK's
json tags carry the REST names and `omitempty`) in `internal/handlers/arm/storageaccount.go` and `virtualmachine.go`;
add a test asserting `keySource`, `defaultAction` and `deleteOption`, and an ARM golden YAML. Effect: ARM YAML keys
change once, so drift reports every storage account and VM as changed — **re-baseline with `resource download` right
after shipping**. **Parked** because it needs that re-baseline (code follow-up CF2 of the 2026-10-02 prompt review;
also the Go review's G2). **Revisit** with the next planned re-download.

### Idea: Edge for Windows settings in targeted managed app configurations

*Area:* export & metadata · *Impact:* low · *Effort:* S · *Ships with:* standalone; a re-baseline if the YAML grows

Learn builds the Windows (Microsoft Edge) managed-apps configuration in the settings catalog, and Graph models those
settings as the `settings` relationship of `targetedManagedAppConfiguration`, which a plain GET does not return; the
handler (`internal/handlers/graph/targetedmanagedappconfiguration.go`) expands only `apps`. Check with a tenant that has
an Edge for Windows managed-apps configuration; if the settings are missing, add `settings` to the `$expand` (or page
`/settings`) and a KeySettings note. Effect: those policies' YAML grows once. **Parked** because it needs a tenant with
such a configuration to confirm (code follow-up CF4 of the 2026-10-02 prompt review). **Revisit** when one is
available.

### Idea: export the tenant `deviceManagement.settings`

*Area:* export & metadata · *Impact:* medium · *Effort:* S · *Ships with:* standalone; then turn the type's
metadata back into tenant-wide settings

`GET /deviceManagement` without `$select` returns only `id`, `intuneAccountId` and `maximumDepTokens`, so the
`deviceManagement` export carries no configurable setting. Pass `$select=id,intuneAccountId,maximumDepTokens,settings`
to `client.DeviceManagement().Get` in `getSingleton` (`internal/handlers/graph/devicemanagementsettings.go`) so the
tenant-wide Intune settings (`deviceManagementSettings`) are exported — notably `settings.secureByDefault` (the switch
behind "Mark devices with no compliance policy assigned as"), `deviceComplianceCheckinThresholdDays`,
`deviceInactivityBeforeRetirementInDay`, `enhancedJailBreak` and `androidDeviceAdministratorEnrollmentEnabled`. Then
the type's metadata becomes "tenant-wide settings" again: Purpose, KeySettings on those `settings.*` paths, and the
`deviceManagementSettings` page documenting real exported data. Effect: one nested map more per tenant, so drift
reports one change per tenant once. **Parked** because the metadata review made the type honestly informational and
nobody has asked for these settings in the export yet. **Revisit** when an operator wants tenant-wide Intune
settings documented or drift-tracked.

### Idea: export ADMX presentation values for group policy configurations

*Area:* export & metadata · *Impact:* medium · *Effort:* M · *Ships with:* standalone; a re-baseline after it
ships

`presentationValues` are never fetched, so the values entered for Administrative Templates settings with
presentations (text boxes, drop-downs, lists) are missing from the export. In `listGroupPolicyDefinitionValues`
(`internal/handlers/graph/grouppolicyconfiguration.go`) widen the existing `$expand` from `definition` to
`definition,presentationValues($expand=presentation)`; if the service rejects the nested expand, fall back to one
`GET …/definitionValues/{id}/presentationValues?$expand=presentation` per definition value (Learn lists
`DeviceManagementConfiguration.Read.All`, no new scope). Then add `presentationValues` back to the type's
`EmbeddedPayloads` and a handler test asserting the values are attached. Effect: every Administrative Templates
profile's YAML grows, so the first drift run reports them all as changed — **re-baseline with `resource download`
right after shipping**. **Parked** because it changes the export of every such profile and needs that re-baseline.
**Revisit** when an operator needs the configured ADMX values documented or drift-tracked.

### Idea: mask dedicated secret properties in the export

*Area:* export & metadata · *Impact:* medium · *Effort:* S–M · *Ships with:* the next regeneration, but only if
the prompt redaction rule is extended instead of masking (*regen-gated* then)

Check that no dedicated secret property reaches disk: `depMacOSEnrollmentProfile.adminAccountPassword` (inside
`depOnboardingSettings.enrollmentProfiles`), `windowsAutopilotDeviceIdentity.deviceAccountPassword` and
`vppToken.token`. None occurs in the 2026-10-01 exports (the vppTokens export is empty), so Graph probably never
returns them. If any can carry a value, mask it in the export with the cleaner's replacement mechanism
(`CleanPropertiesWithReplace`, `internal/transform/cleaner.go`) or a type-specific remove-key in the default
transform config, with a `cleaner_test.go` case per key. Only if masking is not done, extend `prompt-redaction-rule`
to "dedicated secret properties (passwords, tokens)" — that moves every `promptSha256` and must ride a regeneration.
Effect: YAML changes only where a secret was present. **Parked** because it needs one export from a tenant with VPP
tokens and a macOS ADE profile that creates a local admin account to confirm. **Revisit** when such a tenant is
available, or at once if any of the three properties appears in an export.
The prompt half is covered by the 2026-10-02 prompt review (rule T1 in go entry 1: documents never reprint values
under credential-named keys); this idea is now the export half only — masking before the value reaches disk.

### Idea: Intune branding images as content summaries

*Area:* export & metadata · *Impact:* low · *Effort:* M · *Ships with:* standalone; a re-baseline or accepting
the drift

`themeColorLogo`, `lightBackgroundLogo` and `landingPageCustomizedImage` never appear in the
`intuneBrandingProfiles` export although Learn's GET example returns them. In `fetchItem`
(`internal/handlers/graph/intunebrandingprofile.go`) fetch each profile with a `$select` naming them and check whether
Graph then returns the mimeContent `{type, value}`; if so, replace each image in the handler's `normalize` hook with
`{type, sha256, sizeBytes}` (base64 bytes are large and useless in drift, the digest still shows a logo change) and add
the three keys to KeySettings. Effect: three small summaries per profile — a one-time re-baseline or accepted drift.
**Parked** because a logo change is rarely what a review is about. **Revisit** when branding changes need to be
tracked.

### Idea: organizationalBranding: the documented request shape

*Area:* export & metadata · *Impact:* low · *Effort:* S · *Ships with:* standalone; needs a tenant with default
branding to verify

The handler uses `$expand=localizations`, but the Learn Get page supports only `$select` (documented alternative:
`GET …/branding/localizations`) and marks `Accept-Language` as required. In `fetchItem`
(`internal/handlers/graph/organizationalbranding.go`) drop the expand, send `Accept-Language: 0` for the default
branding via `requestConfig.Headers`, read the per-locale overrides with a separate paged
`GET /organization/{id}/branding/localizations` and attach them with `SetLocalizations`; test the request shape with an
httptest fixture. Effect: none for the current exports, which have no branding (404). **Parked** because neither
export tenant has default branding, so the change cannot be observed. **Revisit** when a tenant with branding is
exported, or the current call starts failing.

### Idea: relax secret resolution to a read scope

*Area:* export & metadata · *Impact:* low · *Effort:* S · *Ships with:* standalone; needs one consent test

Secret resolution on `deviceConfigurations` declares `DeviceManagementConfiguration.ReadWrite.All`, but Learn lists
`DeviceManagementConfiguration.Read.All` as least privileged for `getOmaSettingPlainTextValue`. Test once with an app
registration consented only `Read.All` (export only deviceConfigurations with `resolve-secrets` on, against a custom
profile with an encrypted OMA-URI value). If plaintext comes back, remove the `requiredPermission` switch in
`internal/handlers/graph/deviceconfiguration.go`, fix its comment and the README (`GRAPH_SCOPES` loses
`DeviceManagementConfiguration.ReadWrite.All`) and update the golden prompt; on a 403 keep `ReadWrite.All` and note in
the comment that Learn's table is wrong. Effect: no export change; one write scope fewer to consent. **Parked** because
it needs that consent test against a real tenant. **Revisit** when such a test tenant is at hand.

### Idea: name the Global Administrator role for onPremisesSynchronization

*Area:* export & metadata · *Impact:* low · *Effort:* S · *Ships with:* standalone

Learn: Global Administrator is the only role supported for the delegated GET of
`onPremisesSynchronization`, so operators without it get a 403 for this type that does not explain itself. Append
" and the Global Administrator role" to the two hint strings in `internal/handlers/graph/onpremisessynchronization.go`
(outside the quoted scope, so the metadata test's `requires '…'` check is unaffected) and add the role to the README
row. Effect: messages and documentation only. **Parked** because nobody has hit the 403 yet. **Revisit** at the first
report of it, or with any other change to that handler.

### Idea: compliance and settings-catalog assignments through the documented Get

*Area:* export & metadata · *Impact:* low · *Effort:* S · *Ships with:* standalone

`compliancePolicies` and `deviceManagementConfigurationPolicies` read assignments through a separate
`/{id}/assignments` call that has no Learn operation page, so its permission is unverified. Both handlers already send
`$expand` on the item GET (`settings`, plus `scheduledActionsForRule` for compliance): add `assignments` and drop the
separate call, so assignments come from the documented Get (`DeviceManagementConfiguration.Read.All`); keep the
best-effort behaviour (`warnAssignmentsFetchFailed` when the expanded list is absent) and verify with one export or the
golden YAML that the assignment objects are identical. Effect: none expected; one request fewer per policy. **Parked**
because the current call works. **Revisit** if it starts failing, or with other work on these handlers.

## Parked ideas — drift & compare

### Idea: per-document backlinks to consistency findings

*Area:* drift & compare · *Impact:* low · *Effort:* M · *Ships with:* after the consistency analysis job and its web view

Link each resource's document to the consistency findings that cite it (a marked block or a browser-side lookup in
`consistency/findings.yaml`), so a reader of one policy sees that it conflicts with another. **Parked** because the
summary and the consistency view already list the findings. **Revisit** when readers ask for it from a policy page.

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

### Idea: resolve group references outside assignments in the documentation run

*Area:* contract · *Impact:* medium · *Effort:* M · *Ships with:* a go/web pair (a new marker class in the browser); the next
regeneration

The CA template keeps group, role and application GUIDs bare "unless the documentation run resolves it for you in a
block it splices in", but the run has no CA splice: `docs generate-prompt` builds the referenced groups and the
reference map from `assignments[].target.groupId` only, so a group used only by Conditional Access gets no document
and every CA document shows bare GUIDs. Collect `conditions.users.includeGroups` / `excludeGroups` into the
referenced groups and the reference map (`internal/docs/generateprompt.go`); give CA documents a forward hash and a
`<!-- ca-targets:start -->` block rendered by the run as `Direction | Kind | Target` at the end of *Conditions* (the
template asks for it with bare GUIDs, like the assignments block); register the marker class in
`web/src/docs/section-hooks.ts` (`MARKER_BLOCK_CLASSES`). Effect: new documents for CA-only groups; every CA document
is re-spliced once. **Parked** by decision C11 of the 2026-10-02 prompt review: a browser-contract change that should
not hold up the regeneration batch. **Revisit** when readers ask who a CA policy targets by name, or with the next
contract change on the web side.

The same gap exists outside Conditional Access: the authentication methods policy targets groups from its own
settings (`authenticationMethodConfigurations[].includeTargets[].id`, e.g. the X.509 certificate method), which the
reference map does not cover either. On the 2026-10-02 run of `iis.mitarbeiterangebote-staging.de` that document
showed a bare GUID for an exported group (`co_developers`), and the group got no document of its own. When picked
up, collect every non-assignment group reference — CA include/exclude groups and authentication-method include
targets — through one table of reference paths per type, so a new type with inline group targets is one row.

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

### Idea: default objects are not "configured but unassigned"

*Area:* templates · *Impact:* low · *Effort:* S · *Ships with:* standalone; takes effect on the next `docs generate-prompt`

The tenant summary's *Assignment posture* (`summary-facts` block, `renderSummaryFacts` in
`internal/docs/generateprompt_render.go`) counts every resource of an assignment-capable type without targets as
*configured but unassigned*, and the run report lists them as "policies with no assignments". Built-in default
objects land there although they are never assigned the way a policy is: the 2026-10-02 run of
`iis.mitarbeiterangebote-staging.de` listed the *Default Branding profile* (`intuneBrandingProfiles`, applies to
everyone) and the *Default scope tag* (`roleScopeTags` id `0`, applies to everything). Exclude them by a fact the
export already records (the branding profile's `isDefaultProfile`, the scope tag's id `0`) — a one-line rule per
type in the summary facts, plus a run-prompt sentence that defaults are not findings. **Parked** because the signal
is noise, not wrong data, and readers see the names. **Revisit** when the summary's posture numbers are used for
reporting, or with the next change to the summary facts.

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

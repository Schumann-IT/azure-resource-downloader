# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- **The generated documentation prompts now declare a closed, machine-readable set of H2 section headings.** The
  seven prompt templates each ended with a section list that was described but never binding, and in real exports
  the model invented extra headings (`## Metadata` in 99 documents, `## Assignments` in 4), which breaks a docs
  browser that styles and deep-links sections by their heading slug. Each template now states that its headings
  are a closed contract to be written verbatim, in order, with no others, and records the exact ordered list in a
  `<!-- doc-headings: … -->` comment that carries into every type's `doc-prompt.md`. Three headings were renamed to
  produce clean anchor slugs — `Lifecycle & operations` → `Lifecycle and operations`, `Expiry & renewal` →
  `Expiry and renewal`, `Usage & references` → `Usage and references`. Each setting `<details>` block now carries a
  `data-setting="<exact YAML path>"` deep-link target and an optional `data-note="security"|"inert"` hint. The
  incremental generation prompt (`internal/docs/generate_prompt_template.md`) gained a **Heading vocabulary** check
  (section 4) that validates every document's H2s against its type's `doc-headings` list, ignoring headings inside
  `<!-- …:start -->`/`<!-- …:end -->` marker pairs so the tool-spliced `## Targeted by` block is exempt.
  The tenant summary (`docs/summary.md`, section 7) now declares the same closed contract for its four H2
  headings — `## Management summary`, `## At a glance`, `## Assignment posture`, `## Coverage caveats` — written
  verbatim, unnumbered and in order, so the landing page slugs the same way as every document. Its Findings and
  Recommendations are now declared as verbatim `### Findings` / `### Recommendations` H3 headings, and the
  summary's H3 vocabulary is closed to exactly those two, so the browser can style and deep-link them
  consistently (they previously varied between exports). Findings are now rendered as a table sorted by
  severity (most serious first) with a closed `Severity` column — `critical`, `high` or `medium` — so the
  landing page can rank and filter them. The summary's preamble is also fixed: a verbatim `# Tenant summary`
  H1 followed by a single orientation sentence, so everything above the first H2 has a known shape. A new
  mandatory section-7 check validates `docs/summary.md` against all of the above — preamble, H2 set and order,
  H3 sub-vocabulary, and the findings table's columns, closed `Severity` values and severity ordering — since
  the section-4 sweep runs before the summary is written and skips the `docs/` root.
  **This changes every type's `promptSha256`, so a full regeneration of all existing documentation is required.**
- **Documentation runs now persist their report to disk.** Section 8 of the generation prompt writes the
  run report to `docs/report-<UTC-date>-<UTC-time>.md` (one file per run, never overwritten, not hash-tracked)
  next to `docs/summary.md`, in addition to printing it. Like `summary.md` it lives at the `docs/` root and is
  exempt from the section-4 stray-document sweep.
- **Tests** for the closed-heading contract: the prompt-template tests now assert the renamed headings and the
  `doc-headings` marker for the default and ARM templates.

### Security

- **Per-resource documents no longer reprint credential-shaped secrets found in free-text.** Real tenant runs
  surfaced unmasked secrets (a plist `REMOTEOFFICEAUTHKEY`, a profile-removal password in a `description`)
  being copied verbatim into the generated documents, widening exposure from Intune readers to anyone with
  read access to the docs. All six property-documenting prompt templates (default, singleton, group,
  credential, referenced, ARM) now redact a credential-shaped value — one found in a free-text field
  (`description`/`notes`) or a decoded embedded payload (plist/XML/base64) that the service did **not**
  already mask — rendering `«redacted — secret present in source»` in its place, still documenting the field,
  marking the block `data-note="security"`, and flagging it in the `Security` section as a credential to
  rotate. The literal value remains only in the source YAML. Structured setting values are left verbatim, and
  the `record` template (inventory types with no free-text fields) is unchanged.
  **This changes those types' `promptSha256`, contributing to the full regeneration already required.**
- **Tests** asserting the redaction rule renders in the default and ARM prompts.

### Fixed

- **Documentation-run check script (`generate_prompt_template.md` §4) hardened against three false failures
  real tenant runs hit.** The `<details>`/`</details>` balance check now ignores tags inside fenced code
  blocks and inline code, so a document that merely *mentions* `<details>` (or embeds a shell script) is no
  longer reported as imbalanced. A document that exists but is not in the work list is no longer flagged as a
  stray — an incremental run legitimately leaves most documents untouched, and a document is retained when its
  type was not regenerated (e.g. a type that could not be listed); it is now failed only when it is genuinely
  a stray (no resource frontmatter) or misplaced (its `source` maps to a different derived path). The section-6
  mtime baseline (`chunks/mtimes.json`) now covers the whole document tree rather than only the work list, and
  is written once, so re-running §4 after the section-5 splice can no longer clobber the pre-splice snapshot.
- **Exports are now reproducible: an unchanged tenant re-exports byte-for-byte identically.** Three sources of
  run-to-run noise were eliminated:
  - **Colliding display names.** A resource's file name is its sanitized display name, and two resources of the
    same type can share one — several default Intune `deviceEnrollmentConfigurations` are all named *"All users
    and all devices"*, and a `winGetApp` and a `macOSLobApp` can both be named *"3CX"*. The writer used the
    display name directly, so colliding siblings overwrote one another on disk **and** collapsed into a single
    `metadata.yaml` entry (the key is the file path); because the pipeline writes concurrently, *which* sibling
    survived varied run to run, silently dropping the rest. The writer now buffers its input and assigns names
    in a deterministic order (sorted by type, sanitized name, resource ID): the lowest resource ID keeps the
    bare `<name>.yaml` and every other collision is written to `<name>_<disc>.yaml`, where `<disc>` is a short,
    stable token derived from its resource ID. No resource is lost, each keeps its own metadata entry, the
    bare-name owner no longer depends on processing order, and every disambiguation is logged as a warning.
  - **Unstable scalar array order.** Microsoft Graph returns some multivalued attributes (e.g. `proxyAddresses`)
    in a different order per read. All-string arrays are now sorted during transformation (the array analogue of
    the YAML marshaller already sorting map keys); arrays containing objects keep their order, which can be
    significant.
  - **Volatile server-generated identifiers.** The `Microsoft.Graph/applePushNotificationCertificate` singleton
    returns a fresh `id` GUID on every read. That `id` is now dropped from the exported YAML via a new per-type
    normalization hook, and the singleton is identified by its stable `appleIdentifier` (which the file is named
    after) instead of the GUID, so its `resourceId` in `resources/metadata.yaml` is also stable across runs.
    After these three fixes, two runs of an unchanged tenant produce identical resource YAML and `doc-prompt.md`
    files, and a `metadata.yaml` that differs only in its timestamps.

- **`config.example.yaml` is now a true no-op when loaded unmodified.** The example's own header promises that
  loading it behaves exactly like running with no config file, but its `transformers` block spelled out each
  transformer's settings (`clean-empty: true`, the full `base64-decode` block). Those values matched the runtime
  defaults functionally, yet writing them changed the transform-config hash recorded in `resources/metadata.yaml`
  (`transformConfigSha256` is a byte hash of the transformer config), so a run that loaded the example diverged
  from one that did not. The default pipeline is now written as bare transformer names (empty settings, identical
  hash) and every per-transformer option is illustrated in comments instead — restoring the no-op guarantee while
  still documenting every setting.

### Added

- **`docs generate-prompt` now instructs the agent to write a tenant summary (`docs/summary.md`).** The
  emitted prompt gained a final step: after every document is written and verified, the agent writes one
  narrative landing-page overview of the tenant's Intune/Entra management posture, which the documentation
  frontend renders as the tenant's index page. To keep it correct on an incremental run — where the work list
  is only the changed documents — the summary is built from a new tool-injected **`summary-facts`** block
  computed from `resources/metadata.yaml` (complete every run, so the summary is right even when the work list
  was empty): per-type counts of every resource present in the tenant (all types, groups and Autopilot
  included), the platforms each type covers, whether it has an assignments concept, the assignment posture
  (assigned vs configured-but-unassigned; group targets split into dynamic/assigned/dangling; All users / All
  devices), and coverage caveats (types not listed, types that listed empty, resources retained but gone from
  the tenant). On top of those facts the prompt has the agent produce a fixed-length (~600–900 word)
  management summary — findings and recommendations drawn only from the fact block, the reference map and a
  bounded, mechanically-decidable signal sweep over `resources/` (not-in-force resources, unassigned
  resources, dangling targets, credentials near expiry, unmasked plaintext credentials) — so a no-op run and a
  full run yield the same page. Deeper per-resource analysis stays in the individual documents.
  `docs/summary.md` is written by the agent, not by `azure-rd`, and the prompt's structural check now
  tolerates tool-/agent-owned files at the `docs/` root (`generate.md`, `summary.md`).

- **Tests** for `renderSummaryFacts`: per-type counts over present resources (groups and Autopilot included),
  platform aggregation, the assignment posture math (assigned/unassigned, dynamic/assigned/dangling group
  targets, All users/All devices), coverage caveats, dangling-group counting, determinism, and that the block
  is spliced into `generate.md` between surviving markers.

- **Tests** for the reproducibility fixes: colliding resources are written to distinct files with the
  lowest resource ID keeping the bare name, the collision naming is order-independent (feeding the same
  resources in reverse yields an identical id→name mapping) and lossless under a concurrent worker pool, the
  name discriminator is stable per resource ID, `SortScalarSlices` sorts all-string arrays while leaving
  object arrays and mixed lists untouched, and the `applePushNotificationCertificate` handler strips its
  volatile `id`.

- **Tests** guarding the `config.example.yaml` no-op promise: a run that loads the example unmodified now has a
  regression test asserting every effective value equals the built-in default a plain `azure-rd download` uses —
  every scalar flag key, the config-only `workers-by-api`/`transformers`/`filters` sections, and the
  `transformConfigSha256` recorded in `resources/metadata.yaml` — so any future drift in the file (a re-defaulted
  key or a spelled-out transformer setting) fails the build.

- **`docs generate-index` command.** A second `docs` subcommand emits `docs/index.yaml`, the machine-readable
  navigation index the documentation frontend reads to render a tenant's index (so no `index.md` is
  generated). Like `generate-prompt` it **never fetches a resource** and never writes into `resources/`; it
  authenticates only to resolve the tenant's Entra default domain (`--domain` skips sign-in and runs offline)
  and writes exactly one file, `output/<tenant>/docs/index.yaml` (override with `--out`) — the only file
  besides `generate.md` that `azure-rd` writes under `docs/`. The index is fully derived from
  `resources/metadata.yaml` (facts: `displayName`, `scope`, `odataType`, `platforms`, count-only assignment
  summaries) enriched with each document's frontmatter (the LLM-authored `summary`, `platformGroup` and
  `functionGroup`). An in-scope resource with no document yet is listed with `documented: false` and blank
  summary/grouping so counts stay honest and the frontend can show it as pending; excluded bulk types
  (unreferenced groups, autopilot device identities) are reported as counts only, never listed; orphans
  (resources gone from the tenant) are counted but leave their document in place. It shares
  `metadata.yaml`, the in-scope rule and the assignment resolution with `generate-prompt`, so the index can
  never describe a different set of resources than the prompt documents. It carries no wall-clock time
  (`generatedAt` mirrors the export), so re-running over an unchanged export is byte-identical. Under
  `--dry-run` nothing is written and the command reports what the index would contain. Exit codes mirror
  `generate-prompt`: `0` on success, `2` when the command cannot answer (no metadata, tenant mismatch).

- **Tests** for the index engine (in-scope classification and excluded-type counts, pending vs documented
  resources, orphans, assignment count summaries, tenant mismatch, determinism and dry-run).

- **`docs generate-prompt` now computes list 2 (re-splice + migrate), not just the documents to generate.**
  In one pass over the export the engine hashes each assignment-capable resource's *resolved* assignment rows —
  group name, group kind (assigned/dynamic · security/Microsoft 365), filter name and include/exclude — into a
  forward `assignmentsSha256`, and hashes the resources targeting a referenced group into a reverse
  `targetedBySha256`. A current document whose recorded hash no longer matches is reported as a **re-splice**
  (only its marked block needs re-rendering); a current document that predates the assignment markers is
  reported as a **migrate** (markers must be inserted first). A document already in list 1 is never also in
  list 2. Assignment target filters with no export entry are now flagged **dangling** alongside groups.

- **Reference map carries group kind.** The spliced `refmap` block now resolves each referenced group GUID to
  its display name, document path **and** kind, so the agent renders assignment tables from given facts
  without re-reading each group's YAML. The work list's `assignmentsSha256` column is now populated, and the
  emitted prompt gains real `resplice` and `migrate` blocks.

- **`AssignmentCapable` capability.** Handlers with an assignments concept declare it; the registry surfaces it
  and `download` records it per covered type as `hasAssignments` in `metadata.yaml`, so `docs generate-prompt`
  can tell a type with no assignments concept from one whose resources merely have none.

- **Tests** for assignment parsing and the forward/reverse hashes (order-independence, group/filter rename,
  dangling, group-kind change), list-2 detection (forward re-splice, reverse re-splice, migrate,
  list-1-excluded-from-list-2), dangling filters, the env-key replacer, and `metadata.yaml` determinism.

### Changed

- **`--exit-code` now gates on any pending work.** It returns `3` when documents need generating **or**
  re-splicing **or** migrating, not merely when documents are missing, so a clean CI run means every document
  on disk matches the export.

- **`download --dry-run` is list-only.** A dry run stops after building the fetch requests and reports what it
  would download without calling the fetch/transform/write pipeline; it writes no resource files and does not
  touch `metadata.yaml`.

- **`list` runs offline.** It builds the handler registry with a lazily-resolved credential so supported
  resource types can be listed without authenticating.

- **Version is injected at build time.** `rootCmd.Version` resolves from an ldflags-injected string, falling
  back to `debug.ReadBuildInfo()`; the `Makefile` injects the version on `build`/`install`.

## [RC2]

### Added

- **`docs generate-prompt` command.** A new `docs` command group whose first subcommand turns an export into
  a single, ready-to-paste incremental documentation prompt covering exactly the resources whose
  documentation is missing or out of date. It **never fetches a resource** and never writes into
  `resources/`: it reads `resources/metadata.yaml` and the documents already under `docs/`, then writes one
  file, `output/<tenant>/docs/generate.md` (override with `--out`). A document is listed for (re)generation
  when it is missing, its resource's `sourceSha256` changed, its type's `doc-prompt.md` (`promptSha256`)
  changed, or its frontmatter is unreadable; `presentInTenant: false` entries are reported as orphaned
  documents (never deleted) and types with no `doc-prompt.md` are reported as ungeneratable. Two Graph types
  are excluded as bulk records — `windowsAutopilotDeviceIdentities`, and `groups` **except those referenced
  by an assignment** (computed from `assignmentTargets`, with dangling GUIDs flagged). The `export`,
  `worklist` and `refmap` blocks are spliced into a built-in template embedded in the binary
  (`internal/docs/generate_prompt_template.md`; override with `--prompt`); every marker is validated before
  anything is written.

  Tenant resolution mirrors `download` (Azure CLI sign-in → tenant default domain), with `--domain` to skip
  authentication and run offline, and a single-export-directory default when neither is available; the
  resolved domain is cross-checked against `metadata.yaml`'s `tenant:`. Exit codes: `0` on success, `2` when
  the command cannot answer (no metadata, ambiguous tenant, unreadable template), and — with `--exit-code` —
  `3` when stale documents exist. `docs/generate.md` is the only file `azure-rd` writes under `docs/`, and
  `--prune` still never touches `docs/`.

  Under `--dry-run` nothing is written, and an existing `generate.md` left by an earlier run is reported
  with its path and age — a dry run cannot be mistaken for having refreshed the prompt someone is about to
  paste.

  This is the first iteration: it decides the list of documents to *generate*. It does not yet detect
  documents needing only a marked block re-rendered (a renamed group, a re-pointed assignment) — the
  `resplice` and `migrate` blocks state that plainly rather than claiming "none". For the same reason the
  work list's `assignmentsSha256` column is emitted empty: the hash is not computed yet, and nothing reads
  the column back. See `NEXT-ITERATIONS.md` §4.

- **Group facts in `metadata.yaml`.** Group entries now record `groupTypes` and `securityEnabled` as raw
  facts (`ResourceFacts`/`ResourceMeta`), so a downstream step can resolve a referenced group's kind without
  re-reading its YAML. These feed the future assignment-resolution hash.

- **Tests** for the documentation-prompt engine (staleness classification, referenced groups and reference
  map, tenant mismatch, missing metadata, missing per-type prompt, determinism and dry-run, template marker
  validation and splicing, frontmatter parsing), the embedded-template/root sync guard, and the group-facts
  extraction.

### Changed

- **Shared CLI flag helpers moved to `internal/cmdutil`.** `AddAzureAuthFlags`, `AddSelectionFlags`,
  `AddPipelineFlags`, `BindFlags` and the default constants moved out of `cmd/flags.go` into a new
  `internal/cmdutil` package so a subcommand living in its own directory can reuse them without an import cycle
  back into package `cmd`.

- **`docs` subcommands live in their own directory (`cmd/docs/`).** The `docs` parent command stays in
  package `cmd`; each subcommand is a file under `cmd/docs/` exposing an exported constructor (e.g.
  `NewGeneratePromptCommand`) that the parent attaches.

## [RC1]

### Added

- **Export metadata (`resources/metadata.yaml`).** Every download now records a facts-only description of
  the export: per resource its `sourceSha256`, `resourceId`, `displayName`, `@odata.type`, `platforms`,
  `technologies`, sidecar artifact names and raw assignment targets; per type the SHA-256 of its assembled
  `doc-prompt.md` and when it was last covered; and per run the scope, completeness verdict, tool version and
  a hash of the effective transform configuration. Facts are captured in the pipeline from the marshalled
  bytes and the already-parsed resource, so nothing is re-read or re-parsed from disk.

  The file deliberately holds **facts only** — no grouping, classification or change buckets. Those are
  decisions that depend on rules rather than on the tenant, so they belong to a later post-processing step
  and can be revised without re-downloading anything.

- **`download --prune`.** Deletes files under `resources/` for resources the run proves are no longer in the
  tenant, so an export stops accumulating. It runs *after* the download, never before: the delete set is
  exactly what the metadata merge marked absent. Guards — it refuses on an incomplete run or any fetch
  failure, only touches types whose listing succeeded, never leaves `resources/`, never removes
  `resources/metadata.yaml`, and removes a type's `doc-prompt.md` only when that type empties out entirely.
  Every deletion is logged, with a total.

- **`--dry-run --prune` preview.** Lists exactly which files a real prune would delete, sharing one
  eligibility decision with the real path so the preview cannot drift from the outcome.

- **Run completeness.** `ExecutionSummary` gained `Complete`, `IncompleteReason` and `CancelledResources`.
  A run is complete only when every request produced a result, nothing was cancelled, and every selected type
  listed successfully. Reported in the run summary and recorded in `metadata.yaml`.

- **Per-command flag groups (`cmd/flags.go`).** `addAzureAuthFlags`, `addSelectionFlags` and
  `addPipelineFlags`, with local flags bound to viper per execution. Commands now opt into the flags they
  actually use instead of inheriting every persistent flag. `--client-id` and `--tenant-id` are marked
  required-together, enforcing what their help text already promised.

- **Interactive dedicated-app sign-in.** When selected resource types need Microsoft Graph permissions the
  Azure CLI first-party app cannot provide, the run reports which types require it and prompts for the app
  registration client and tenant ID, pre-filled from config or environment. Non-interactive runs fall back to
  the defaults and fail fast when a required value has neither.

- **Tests** for pipeline cancellation accounting and stage status propagation, metadata merge and absence
  detection, prune and prune-refusal, dry-run prune preview, worker-count resolution, and the interactive
  prompt.

### Changed

- **Export layout.** Everything the tool writes now lives under a single `resources/` subdirectory:

  ```
  output/<domain>/resources/metadata.yaml
  output/<domain>/resources/<APIType>/<endpoint>/<resource>.yaml
  output/<domain>/resources/<APIType>/<endpoint>/doc-prompt.md
  ```

  The boundary is by owner: `resources/` is the only tree `azure-rd` writes to, which is what makes `--prune`
  safe to reason about.

- **`--timeout` is now a per-operation deadline**, applied around each individual resource fetch (including
  its retries), matching what its help text always claimed. It was previously applied as a single deadline
  around the entire pipeline, so a large tenant could exhaust the 300-second default mid-run.

- **`--write-prompts` replaced by `--no-prompt`.** Per-type `doc-prompt.md` files are now written by default;
  pass `--no-prompt` (or `no-prompt: true`) to skip them.

- **`list` no longer advertises download flags.** It previously inherited `--type` and `--resource-group`,
  which it silently ignored.

- **`config.example.yaml` now ships every option set to its built-in default** rather than fully commented
  out, so the file doubles as a reference for what the defaults actually are and loading it unmodified
  behaves exactly like running with no config. It also documents `resource-id`, `no-prompt` and `prune`, and
  describes `timeout` as per-operation. `filters` remains commented out deliberately — the default is its
  absence, and a valueless `filters:` key is not a valid filter map.

- **`download --help` gained examples** for `--config`, `--prune` and `--prune --dry-run`.

- **Documentation corrections.** The README now states that `transformers` replaces the default pipeline
  rather than merging with it, and that a configured `workers` value applies to full or multi-type exports
  while a single `--type` run uses the API-specific count unless the `--workers` flag is passed. The
  `.windsurf/rules` were updated to match the new flag structure, export layout, prune guards and pipeline
  invariants.

### Fixed

- **Cancelled requests produced no result at all.** All three pipeline stages emitted a single error result
  on cancellation and then abandoned their input channel, so queued requests vanished silently:
  `len(summary.Results)` could be smaller than the request count with nothing comparing them, and the run
  still reported success. Each stage now drains its input and emits exactly one `Cancelled` result per
  request, and the pipeline fails loudly if the counts disagree.

  This was latent before but is load-bearing now: a request that produces no result is indistinguishable
  from a resource deleted in the tenant, so with `--prune` a timeout could have deleted live resources.

- **Environment variable overrides did not work for any hyphenated setting.** Without an env key replacer,
  viper looked for names like `AZURE_RD_LOG-LEVEL`, which a shell cannot export — so `log-level`, `dry-run`,
  `client-id`, `tenant-id`, `resource-group`, `resolve-secrets` and `write-prompts` had no working override
  despite the documented `flag > env > config > default` precedence.

- **`--workers 5` was indistinguishable from not passing the flag.** Explicitness was inferred by comparing
  the value against the default literal, so an explicit `--workers 5` was discarded in favour of the
  API-specific default (20 for ARM). Explicitness is now taken from the flag itself, and the defaults are
  named constants rather than literals duplicated across files.

- **Every documented `transformers` example silently disabled base64 decoding.** `config.example.yaml` and
  the README both listed `cleaning`, `id-resolution` and `name-sanitization`, but the built-in default
  pipeline has a fourth entry, `base64-decode`. Because the key replaces the defaults wholesale instead of
  merging, anyone copying an example lost decoding of embedded Intune payloads (e.g. macOS `.mobileconfig`
  profiles) without any warning. All examples now list the full pipeline, and the replace-not-merge behaviour
  is called out where the key is documented.

- **`config-tailored-intune.yaml` still set the removed `write-prompts` key**, which viper no longer reads.
  Replaced with the equivalent `no-prompt: false` so the intent survives instead of sitting dead in the file.

### Breaking

- **Flags must now follow the subcommand.** `--type`, `--resource-group`, `--workers`, `--subscription`,
  `--client-id` and `--tenant-id` moved from persistent root flags to per-command flags, so
  `azure-rd --type X download` no longer parses. Use `azure-rd download --type X`. `--config`, `--output`,
  `--dry-run` and `--log-level` remain global and may still be given in either position.
- **`--write-prompts` no longer exists**; prompts are written by default and `--no-prompt` opts out.
- **Downloaded files moved into `resources/`.** Exports produced by earlier versions need their
  `<APIType>/` trees moved under `resources/`, or regenerating.

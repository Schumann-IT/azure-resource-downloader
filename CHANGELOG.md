# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

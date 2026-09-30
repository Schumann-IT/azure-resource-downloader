---
trigger: always_on
description: 
globs: 
---

# Security & Ops

## Security
- **Never log secrets**: Redact Azure tokens, client secrets, and resolved OMA-URI secret values in logs
- **Azure credentials**: delegated user auth only — app-only / service principal credentials are NOT supported
  - Default: `azidentity.NewAzureCLICredential` reusing the `az login` session (same token for ARM + Microsoft Graph)
  - With `client-id`/`tenant-id` in the tenant's configuration profile: `azidentity.NewDeviceCodeCredential` against a dedicated app registration (for Graph scopes the Azure CLI app cannot obtain)
  - All credential fields/params are typed `azcore.TokenCredential` (never a concrete azidentity type)
- **Secret resolution (`resolve-secrets`, config-only)**: off by default; when enabled it writes decrypted Intune OMA-URI secrets to disk in PLAINTEXT and must log a warning. Requires `DeviceManagementConfiguration.ReadWrite.All` in the token.
- **Sensitive data in output**:
  - Don't include `adminPassword`, `connectionStrings`, `keys` in YAML
  - Filter these in handler's `Transform()` method
- **File permissions**: Write files with 0644 (readable), directories 0755

## Configuration
**The configuration file is the single source of truth.** Every setting has exactly one home; the command line carries only what locates the configuration, selects the tenant, or changes one invocation without changing what is produced.
- **Config precedence**: flag > tenant profile > base config file > built-in default. **There is NO environment layer** — `viper.AutomaticEnv`, `SetEnvPrefix` and `SetEnvKeyReplacer` are gone and no `AZURE_RD_*` variable is read. Do not reintroduce them: an environment value outranked the file, so one left over from another tenant's run applied silently, and a wrong value here does not fail loudly — it produces a confident, wrong result. `LOG_LEVEL` still works (the logger reads it directly, not through viper).
- **Two files, one enforced partition** (`internal/config`, `keyScopes`): general settings (`output`, `type`, `workers`, `workers-by-api`, `timeout`, `resolve-secrets`, `no-prompt`, `prune`, `transformers`, `taxonomy`) live in the base file; tenant-scoped settings (`subscription`, `client-id`, `tenant-id`, `filters`) live in `<config-dir>/<domain>.yaml`. A key on the wrong side, an unknown key, or a key that is really a flag is a **fatal error naming it**. Each file is validated BEFORE merging — the merged state no longer knows which file a key came from. Reference schemas: `config.example.yaml` and `config.example.domain.yaml`, both no-ops when loaded unmodified.
- **Why the partition is enforced, not advised**: `transformers` is hashed into `transformConfigSha256`, so a per-tenant value makes an export non-comparable with its own baseline and every other tenant; `filters` is hashed into `filtersSha256` and gates drift comparability, so it must be stable *per tenant* rather than shared; `output` is the export root and the tenant is already a subdirectory of it.
- **Config location**: `--config <path>` names the base file (a mistyped path is fatal, never a silent fallback); `--config-dir <dir>` holds the profiles plus an optional `base.yaml` picked up by convention, and **requires `--domain`** — configuration is read before authentication and supplies the credentials it needs, so the profile cannot be chosen by the tenant a run later resolves to. A domain with no profile is fatal, listing the ones that exist. The config directory is deliberately NOT defaulted.
- **Flag placement**: global are `--config`, `--config-dir`, `--output`, `--dry-run`, `--log-level`. `--domain` and the selection flags (`--type`, `--resource-id`, `--resource-group`) are persistent on the `resource` group; `--domain`/`--out`/`--prompt` are per `docs` subcommand; `--exit-code` sits on the commands that decide it; root declares `--debug` and `--domain` locally for its diagnostic report. Flags follow the command or its group (`azure-rd resource download --type X`, not `azure-rd --type X resource download`). Only `--output` and `--type` are viper-bound; any flag declared in more than one place is read from the running command (`cmdutil.DeclaredDomain`, `cmd.Flags().Get*`), because a global binding could resolve to a sibling's copy. There is no `BindFlags` helper.
- **Everything is optional**: with no configuration and no flags a run performs a full export with the built-in defaults. `subscription` is auto-detected from the signed-in user's default subscription; with none at all, Graph types still download and ARM types are skipped with a warning.
- **A run must know which tenant it acts on.** `internal/tenantdir` is the one resolver: the declared `--domain` is intent, the signed-in tenant is ground truth, a mismatch refuses before anything is written, and when neither is known the run refuses — there is deliberately **no fallback to the bare output directory**, because an export at `<output>/resources/` does not match the `<output>/<domain>/` layout every other command looks for and is invisible from the moment it is written.

## Operations
- **Graceful shutdown** (implemented):
  - `Execute` wraps the run context with `signal.NotifyContext` (SIGINT/SIGTERM) and every command uses `cmd.Context()`, so Ctrl+C cancels listing and fetching cleanly: the pipeline drains, every request still produces one (Cancelled) result, and the run is recorded incomplete — so it can never mark a resource absent or feed a prune. A second Ctrl+C force-quits (the first signal restores default disposition).
  - Per-operation timeout via context in the pipeline (implemented)
- **Error handling**:
  - Commands NEVER call `os.Exit` inline: `RunE` returns the error (a distinct exit code rides `cmdutil.WithExitCode`; `cmdutil.ExitCode` unwraps it) and the ONLY `os.Exit` lives in `cmd.Execute`. Root sets `SilenceErrors` and `execute()` is the single error print site — exactly one print per failure; usage is shown for invocation mistakes only (SilenceUsage is set once flags parsed, in root's `PersistentPreRunE`, which also loads the config — `cobra.EnableTraverseRunHooks` keeps it running under group hooks)
  - Continue processing other resources if one fails
  - Permission errors (ARM 403, Graph missing scopes/Forbidden) NEVER fail the run: warn + skip via `azure.IsPermissionError`, reported as skipped in the summary
  - Collect errors in `ExecutionSummary`
  - Return non-zero exit code only when `FailedResources > 0` (skipped/filtered resources don't affect it)
  - **Completeness is separate from the exit code.** `ExecutionSummary.Complete` is false when any request was cancelled, any result went missing, or any type failed to list — an incomplete run can still exit 0. Anything that infers absence (metadata, prune) must gate on `Complete`, never on the exit code.
- **Resource limits**:
  - API-specific worker defaults: Microsoft Graph 5, ARM 20 (configurable via `workers-by-api`, or a single `workers` count whose mere presence overrides them — detected with `viper.IsSet`, which is why that key has no registered default)
  - Default timeout: 300 seconds, applied **per operation** (around each resource fetch including its retries), not as a whole-run budget
  - Rate limiting: `internal/retry` retries transient failures (429/503/timeouts allowlist) with exponential backoff, 5 attempts; 403 is never retried
- **Dry-run mode**: Always support `--dry-run`. Under it the tool writes **no** resource files and neither writes nor updates `resources/metadata.yaml` — and nothing that depends on those writes is recalculated to compensate (e.g. `Writer.writeResource` builds `ResourceFacts` only when not in dry-run, which is correct: no file is written, so there is no hash to record). For destructive operations it must list exactly what would be removed.
  - A command whose real work is a download may **skip the download entirely** under `--dry-run` and answer only the part of its question that can be answered offline. When it does, the output is a subset of a real run, not a preview, and must be worded so it cannot be mistaken for one. The inverse also exists: `resource drift` cannot answer anything offline, so its `--dry-run` still fetches and reports in full and only withholds its writes (the `drift/` tree — clearing nothing, and saying an earlier observation was not refreshed).
- **Output layout**: everything lives under `<output>/<tenant>/`, where `<tenant>` is the tenant's Entra default domain — resolved by `internal/tenantdir`, which refuses the run rather than falling back to the bare output directory — in sibling trees:
  - `resources/` — **the tree `download` writes to exclusively.** Holds `metadata.yaml` at its root and `<APIType>/<endpoint>/` directories containing each resource YAML, its sidecar artifacts and the type's `doc-prompt.md`
  - `docs/` — generated documentation, written by the documentation run and NOT by this tool, **with one exception**: `docs generate-prompt` writes `docs/generate.md` (the incremental documentation prompt) at the tree root, where no document can ever be (documents are always `<APIType>/<endpoint>/<name>.md`, at least two levels deep). That single file is the only thing `azure-rd` writes under `docs/`. It mirrors `resources/` exactly, so a document's path is its resource's path with the tree root and extension swapped (`resources/Microsoft.Graph/x/y.yaml` → `docs/Microsoft.Graph/x/y.md`). Do not add a `doc:` field to `metadata.yaml` — the path is derived
  - `drift/` — **the tree `resource drift` owns**: an observation *about* the export (neither a resource nor a document). `drift/metadata.yaml` sits at its root and the payloads of added/changed/renamed resources mirror `resources/` exactly (`drift/<APIType>/<endpoint>/<name>.yaml`), so a payload joins to its baseline file, finding and document by the same key. `resource audit` — and a `resource drift` run whose tenant profile sets `audit-workspace-id`, after the observation — writes the attribution `drift/audit.yaml` atomically at the tree root (facts only, never folded into the observation, so `findingsSha256` and the observation's determinism survive); `docs analyze-drift` writes `drift/analyze.md` at the tree root, where no payload can be (payloads are at least two levels deep); the analysis agent then writes one **drift document per finding** at the payload's path with the extension swapped (`drift/<key>.md` beside `drift/<key>.yaml`, so per resource the baseline YAML, observed YAML, documentation and judgment share one key) plus the summary `drift/index.md` at the root — those agent-written files are the only ones under `drift/` that `azure-rd` itself never produces. Each drift run clears and rebuilds the tree, and a **re-baselining `resource download` clears it too** (`drift.ClearTree`, after a successful metadata write, never under dry-run) — `audit.yaml` included: it is swept with the tree, never pruned, and has no delete path of its own. The tree holds exactly the latest observation and its analysis artifacts, deliberately no history. `docs analyze-drift` refuses (exit 2, `ErrObservationSuperseded`) when the observation predates the current baseline, and verifies every payload against its recorded hash before directing an agent at it. Drift never writes under `resources/` or `docs/`, never updates the export's `metadata.yaml` and never prunes; re-baselining is a normal `resource download`
  - Pruning must never reach into `docs/` or `drift/`. A pruned resource leaves its document behind as an orphan: report it, never delete it

## Export Metadata and Prune

`prune` (config-only) is the ONLY delete path inside the export (`resources/`); the only other deletes in the codebase both go through `drift.ClearTree` and touch only the `drift/` tree: a `resource drift` run clearing it before rebuilding, and a re-baselining `resource download` clearing it after a successful metadata write (a new baseline supersedes the observation by definition). These rules are what make them safe; do not relax them.

- **`resources/metadata.yaml` describes the export directory, not the tenant.** Never remove an entry for a file that still exists on disk — a resource gone from the tenant is recorded as `presentInTenant: false` with its facts and hash retained. Removing the entry instead makes the next run find an undescribed YAML and treat it as new, forever. Only a prune, having actually deleted the file, removes an entry.
- **An incomplete run may not mark anything `presentInTenant: false`** — it cannot tell a deleted resource from one it never reached.
- **"Covered" means a type's listing succeeded**, not that it returned resources. `EmptyTypes` is covered (absence there is real); `SkippedTypes` is not (the count is unknown). Collapsing the two turns a missing permission into a deletion.
- **Prune guards**: refuses unless the run is `Complete` and `FailedResources == 0`; only deletes within covered types; never leaves `resources/`; never deletes `resources/metadata.yaml`; removes a type's `doc-prompt.md` only when that type empties out entirely. Every deletion is logged, with a total.
- **Partial runs merge, never truncate**: a `--type`-scoped run must retain entries for types it did not cover and leave their `lastCoveredAt` alone.
- **Metadata records facts, never decisions.** A value belongs in `metadata.yaml` only if it is read from the resource or computed from its bytes (hashes, display name, `@odata.type`, assignment targets, artifact names). Anything derived from a rule you might revise — grouping, classification, change buckets, counts — belongs to a post-processing step, or revising the rule means re-downloading every tenant.
- **`promptSha256` hashes the ASSEMBLED `doc-prompt.md` bytes** (`content.String()` in `Writer.writePromptFiles` — the generated header and trailing newline included), never `TransformResult.DocumentationPrompt`. Hash the raw prompt string instead and the recorded hash never matches the file on disk, so every later comparison reports every type as changed.
- **Per-entry config attestation**: every successfully written entry records the `transformConfigSha256` of the run that produced its bytes (partial runs merge, so the run-level hash attests only the last run's covered types), and the run records `filtersSha256`. `resource drift` refuses (exit 2) across a transform/filter config change, treats entries whose per-entry hash is missing or different as *unattested* (reported, never counted as drift), gates removals on the same complete-and-covered rule as a prune, and hashes the same marshalled bytes the writer hashes (`pipeline.MarshalResourceYAML` + the shared `NamePlanner`), so a verdict can never disagree with what a download would record.

## Observability
- **User-friendly output**: Use emojis and clear progress messages
- **Summary reporting**: Show success/failure/skipped/filtered counts after execution
- **Error context**: Always include resource ID in error messages
- **Log verbosity**: `--log-level` flag, or the plain `LOG_LEVEL` env var the logger reads directly (debug, info, warn, error). It is flag-only — verbosity belongs to one invocation, so there is no config key
  ```bash
  ./azure-rd resource download --log-level debug ...
  ```

## Production Readiness
- **Idempotent**: Re-running should be safe (overwrites existing files)
- **Atomic writes**: `resources/metadata.yaml` is written atomically (temp file in the same directory, then rename — `writeMetadata`), so the export baseline can never be left truncated. Resource YAML/artifact writes are deliberately not atomic; extend the pattern only if a consumer needs it
- **Validation**: Validate resource IDs before processing
- **Azure API versions**: Use stable API versions in handlers
- **Retries**: implemented in `internal/retry` (exponential backoff, retryable-error allowlist) and used by the fetcher
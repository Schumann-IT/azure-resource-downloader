---
trigger: always_on
---

# Security & Ops

## Security
- **Never log secrets**: Redact Azure tokens, client secrets, and resolved OMA-URI secret values in logs
- **Azure credentials**: delegated user auth only — app-only / service principal credentials are NOT supported
  - Default: `azidentity.NewAzureCLICredential` reusing the `az login` session (same token for ARM + Microsoft Graph)
  - With `--client-id`/`--tenant-id`: `azidentity.NewDeviceCodeCredential` against a dedicated app registration (for Graph scopes the Azure CLI app cannot obtain)
  - All credential fields/params are typed `azcore.TokenCredential` (never a concrete azidentity type)
- **Secret resolution (`--resolve-secrets`)**: off by default; when enabled it writes decrypted Intune OMA-URI secrets to disk in PLAINTEXT and must log a warning. Requires `DeviceManagementConfiguration.ReadWrite.All` in the token.
- **Sensitive data in output**:
  - Don't include `adminPassword`, `connectionStrings`, `keys` in YAML
  - Filter these in handler's `Transform()` method
- **File permissions**: Write files with 0644 (readable), directories 0755

## Configuration
- **Config precedence**: flags > env vars > config file > defaults
- **Environment variables**: Prefix with `AZURE_RD_*`
  ```bash
  AZURE_RD_SUBSCRIPTION="..."
  AZURE_RD_OUTPUT="./output"
  AZURE_RD_WORKERS="10"
  ```
- **Config file**: read ONLY when `--config <path>` is passed (no auto-discovery of `~/.azure-rd.yaml`); a mistyped `--config` path is a fatal error. Reference schema: `config.example.yaml`.
- **Env key replacer**: `initConfig` sets `viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))`. Removing it silently breaks every hyphenated override (`AZURE_RD_LOG_LEVEL` would have to be spelled `AZURE_RD_LOG-LEVEL`).
- **Flag placement**: only `--config`, `--output`, `--dry-run` and `--log-level` are global. Auth, selection and pipeline-tuning flags are per-command groups in `../../internal/cmdutil` (exported `AddAzureAuthFlags`/`AddSelectionFlags`/`AddPipelineFlags`/`BindFlags`), so they must follow the subcommand (`azure-rd download --type X`, not `azure-rd --type X download`).
- **All flags are optional**:
  - `--subscription` is auto-detected from the signed-in user's default subscription; with no subscription at all, Graph types still download and ARM types are skipped with a warning
  - `--output`, `--workers`, `--dry-run`, `--timeout`, `--resolve-secrets`, `--no-prompt`, `--prune`, `--log-level`, `--client-id`/`--tenant-id`
  - Selection: `--resource-id`, `--type`, `--resource-group` (all repeatable/config-backed; none given = every registered type)

## Operations
- **Graceful shutdown**:
  - Use context with timeout in pipeline (implemented)
  - TARGET (not yet implemented): cancel operations on interrupt (Ctrl+C) and clean up partial downloads
- **Error handling**:
  - Continue processing other resources if one fails
  - Permission errors (ARM 403, Graph missing scopes/Forbidden) NEVER fail the run: warn + skip via `azure.IsPermissionError`, reported as skipped in the summary
  - Collect errors in `ExecutionSummary`
  - Return non-zero exit code only when `FailedResources > 0` (skipped/filtered resources don't affect it)
  - **Completeness is separate from the exit code.** `ExecutionSummary.Complete` is false when any request was cancelled, any result went missing, or any type failed to list — an incomplete run can still exit 0. Anything that infers absence (metadata, prune) must gate on `Complete`, never on the exit code.
- **Resource limits**:
  - API-specific worker defaults: Microsoft Graph 5, ARM 20 (configurable via `--workers` / `workers` / `workers-by-api`)
  - Default timeout: 300 seconds, applied **per operation** (around each resource fetch including its retries), not as a whole-run budget
  - Rate limiting: `internal/retry` retries transient failures (429/503/timeouts allowlist) with exponential backoff, 5 attempts; 403 is never retried
- **Dry-run mode**: Always support `--dry-run`. Under it the tool writes **no** resource files and neither writes nor updates `resources/metadata.yaml` — and nothing that depends on those writes is recalculated to compensate (e.g. `Writer.writeResource` builds `ResourceFacts` only when not in dry-run, which is correct: no file is written, so there is no hash to record). For destructive operations it must list exactly what would be removed.
  - A command whose real work is a download may **skip the download entirely** under `--dry-run` and answer only the part of its question that can be answered offline. When it does, the output is a subset of a real run, not a preview, and must be worded so it cannot be mistaken for one.
- **Output layout**: everything lives under `<output>/<tenant>/`, where `<tenant>` is the tenant's Entra default domain (falls back to the base output dir with a warning if it cannot be resolved), in two sibling trees:
  - `resources/` — **the tree `download` writes to exclusively.** Holds `metadata.yaml` at its root and `<APIType>/<endpoint>/` directories containing each resource YAML, its sidecar artifacts and the type's `doc-prompt.md`
  - `docs/` — generated documentation, written by the documentation run and NOT by this tool, **with one exception**: `docs generate-prompt` writes `docs/generate.md` (the incremental documentation prompt) at the tree root, where no document can ever be (documents are always `<APIType>/<endpoint>/<name>.md`, at least two levels deep). That single file is the only thing `azure-rd` writes under `docs/`. It mirrors `resources/` exactly, so a document's path is its resource's path with the tree root and extension swapped (`resources/Microsoft.Graph/x/y.yaml` → `docs/Microsoft.Graph/x/y.md`). Do not add a `doc:` field to `metadata.yaml` — the path is derived
  - `--prune` must never reach into `docs/`. A pruned resource leaves its document behind as an orphan: report it, never delete it

## Export Metadata and Prune

`--prune` is the ONLY delete path in the codebase. These rules are what make it safe; do not relax them.

- **`resources/metadata.yaml` describes the export directory, not the tenant.** Never remove an entry for a file that still exists on disk — a resource gone from the tenant is recorded as `presentInTenant: false` with its facts and hash retained. Removing the entry instead makes the next run find an undescribed YAML and treat it as new, forever. Only `--prune`, having actually deleted the file, removes an entry.
- **An incomplete run may not mark anything `presentInTenant: false`** — it cannot tell a deleted resource from one it never reached.
- **"Covered" means a type's listing succeeded**, not that it returned resources. `EmptyTypes` is covered (absence there is real); `SkippedTypes` is not (the count is unknown). Collapsing the two turns a missing permission into a deletion.
- **Prune guards**: refuses unless the run is `Complete` and `FailedResources == 0`; only deletes within covered types; never leaves `resources/`; never deletes `resources/metadata.yaml`; removes a type's `doc-prompt.md` only when that type empties out entirely. Every deletion is logged, with a total.
- **Partial runs merge, never truncate**: a `--type`-scoped run must retain entries for types it did not cover and leave their `lastCoveredAt` alone.
- **Metadata records facts, never decisions.** A value belongs in `metadata.yaml` only if it is read from the resource or computed from its bytes (hashes, display name, `@odata.type`, assignment targets, artifact names). Anything derived from a rule you might revise — grouping, classification, change buckets, counts — belongs to a post-processing step, or revising the rule means re-downloading every tenant.
- **`promptSha256` hashes the ASSEMBLED `doc-prompt.md` bytes** (`content.String()` in `Writer.writePromptFiles` — the generated header and trailing newline included), never `TransformResult.DocumentationPrompt`. Hash the raw prompt string instead and the recorded hash never matches the file on disk, so every later comparison reports every type as changed.

## Observability
- **User-friendly output**: Use emojis and clear progress messages
- **Summary reporting**: Show success/failure/skipped/filtered counts after execution
- **Error context**: Always include resource ID in error messages
- **Log verbosity**: `--log-level` flag / `log-level` config / `AZURE_RD_LOG_LEVEL` or `LOG_LEVEL` env (debug, info, warn, error)
  ```bash
  ./azure-rd download --log-level debug ...
  ```

## Production Readiness
- **Idempotent**: Re-running should be safe (overwrites existing files)
- **Atomic writes**: TARGET (not yet implemented) — write to temp file, then rename
- **Validation**: Validate resource IDs before processing
- **Azure API versions**: Use stable API versions in handlers
- **Retries**: implemented in `internal/retry` (exponential backoff, retryable-error allowlist) and used by the fetcher

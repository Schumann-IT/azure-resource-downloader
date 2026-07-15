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
- **All flags are optional**:
  - `--subscription` is auto-detected from the signed-in user's default subscription; with no subscription at all, Graph types still download and ARM types are skipped with a warning
  - `--output`, `--workers`, `--dry-run`, `--timeout`, `--resolve-secrets`, `--write-prompts`, `--log-level`, `--client-id`/`--tenant-id`

## Operations
- **Graceful shutdown**:
  - Use context with timeout in pipeline (implemented)
  - TARGET (not yet implemented): cancel operations on interrupt (Ctrl+C) and clean up partial downloads
- **Error handling**:
  - Continue processing other resources if one fails
  - Permission errors (ARM 403, Graph missing scopes/Forbidden) NEVER fail the run: warn + skip via `azure.IsPermissionError`, reported as skipped in the summary
  - Collect errors in `ExecutionSummary`
  - Return non-zero exit code only when `FailedResources > 0` (skipped/filtered resources don't affect it)
- **Resource limits**:
  - API-specific worker defaults: Microsoft Graph 5, ARM 20 (configurable via `--workers` / `workers` / `workers-by-api`)
  - Default timeout: 300 seconds
  - Rate limiting: `internal/retry` retries transient failures (429/503/timeouts allowlist) with exponential backoff, 5 attempts; 403 is never retried
- **Dry-run mode**: Always support `--dry-run` to preview without writes
- **Output layout**: resources are written under a per-tenant directory named after the tenant's Entra default domain; falls back to the base output dir with a warning if it cannot be resolved

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

# Security & Ops

## Security
- **Never log secrets**: Redact Azure tokens, subscription IDs in logs, client secrets
- **Azure credentials**: Use DefaultAzureCredential (supports multiple auth methods)
  - Azure CLI (`az login`) - preferred for local dev
  - Managed Identity - for Azure resources
  - Environment variables - for CI/CD
  - Service Principal - for production
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
- **Config file**: `~/.azure-rd.yaml` (don't commit with real credentials)
- **Required vs Optional**:
  - Required: `--subscription` (or env/config)
  - Optional: `--output`, `--workers`, `--dry-run`, `--timeout`

## Operations
- **Graceful shutdown**: 
  - Use context with timeout in pipeline
  - Cancel operations on interrupt (Ctrl+C)
  - Clean up partial downloads
- **Error handling**:
  - Continue processing other resources if one fails
  - Collect errors in `ExecutionSummary`
  - Return non-zero exit code on failures
- **Resource limits**:
  - Default worker count: 5 (configurable)
  - Default timeout: 300 seconds
  - Handle Azure rate limiting gracefully
- **Dry-run mode**: Always support `--dry-run` to preview without writes

## Observability
- **User-friendly output**: Use emojis and clear progress messages
- **Summary reporting**: Show success/failure counts after execution
- **Error context**: Always include resource ID in error messages
- **Debugging**: Support verbose mode via environment variable (future)
  ```bash
  LOG_LEVEL=debug ./azure-rd download ...
  ```

## Production Readiness
- **Idempotent**: Re-running should be safe (overwrites existing files)
- **Atomic writes**: Write to temp file, then rename
- **Validation**: Validate resource IDs before processing
- **Azure API versions**: Use stable API versions in handlers
- **Retries**: Consider adding retry logic for transient Azure API failures

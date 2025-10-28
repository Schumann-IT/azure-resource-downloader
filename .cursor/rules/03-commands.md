# Makefile Usage Policy

**ALWAYS use Makefile targets instead of running commands directly.**

This project uses a Makefile to standardize development workflows. All instructions, documentation, and generated code must reference make targets, not raw commands.

## Required Make Targets

When providing instructions or examples:
- ✅ `make lint` — NOT `golangci-lint run`
- ✅ `make test` — NOT `go test ./...`
- ✅ `make build` — NOT `go build`
- ✅ `make fmt` — NOT `go fmt ./...`
- ✅ `make deps` — NOT `go mod tidy`
- ✅ `make check` — Run fmt + lint + test
- ✅ `make all` — Run fmt + lint + test + build
- ✅ `make ci` — For CI/CD pipelines

## In Documentation and Instructions

When writing README updates, commit messages, or instructions:
- Always reference `make <target>` 
- Never show raw `go` or `golangci-lint` commands
- Exception: Internal Makefile implementation may use raw commands

## Examples

**Bad:**
```bash
go build -o azure-rd
golangci-lint run ./...
go test -v ./...
```

**Good:**
```bash
make build
make lint
make test
# Or run all checks at once
make check
```

# What to generate on request

## "Create a new resource handler"
1. Create `internal/handlers/<resource>.go` implementing `ResourceHandler` interface:
   - `GetType()` - Return Azure resource type (e.g., "Microsoft.KeyVault/vaults")
   - `GetTerraformResourceType()` - Return Terraform type (e.g., "azurerm_key_vault")
   - `Fetch(ctx, resourceID)` - Use Azure SDK to fetch resource
   - `Transform(resource)` - Convert to `*models.TransformedResource`
2. Add constructor: `NewXHandler(credential, subscriptionID)`
3. Register in `cmd/download.go` → `registerHandlers()` function
4. Add unit tests in `internal/handlers/<resource>_test.go`
5. Update README.md "Supported Resource Types" table

## "Add a CLI command"
1. Create `cmd/<command>.go` with Cobra structure
2. Define command-specific flags
3. Add to `init()` function: `rootCmd.AddCommand(<command>Cmd)`
4. Implement `RunE` function with:
   - Configuration loading via Viper
   - Azure client initialization
   - Handler registry setup
   - Pipeline execution
   - Error handling and user-friendly output
5. Add examples in command's `Long` description
6. Update README.md with new command usage

## "Add a transformation"
1. Add function to `internal/transform/<transformation>.go`
2. Use in `internal/pipeline/transformer.go` → `transformResource()`
3. Add unit tests
4. Document behavior in function comment

## "Add config option"
1. Add field to `models.PipelineConfig` struct
2. Add flag in `cmd/root.go` → `PersistentFlags`
3. Bind to Viper: `viper.BindPFlag()`
4. Use in pipeline/command
5. Update `.azure-rd.example.yaml`
6. Document in README.md

# Output shape
- Provide full file paths and complete code blocks
- Include `make deps` if new dependencies added (NOT `go mod tidy`)
- Show registration/wiring steps
- Provide example usage using make targets
- End with checklist of manual steps:
  ```
  ✅ Handler created
  ✅ Registered in cmd/download.go
  ✅ Dependencies updated: make deps
  ✅ Built successfully: make build
  ✅ All checks passed: make check
  ⚠️  Manual: Add to README.md supported types table
  ⚠️  Manual: Test with: ./azure-rd list --subscription "SUB_ID"
  ```

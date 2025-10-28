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
- Include `go mod tidy` if new dependencies added
- Show registration/wiring steps
- Provide example usage
- End with checklist of manual steps:
  ```
  ✅ Handler created
  ✅ Registered in cmd/download.go
  ⚠️  Manual: Add to README.md supported types table
  ⚠️  Manual: Test with: ./azure-rd list --subscription "SUB_ID"
  ```

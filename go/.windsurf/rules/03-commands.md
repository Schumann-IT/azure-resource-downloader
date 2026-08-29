---
trigger: always_on
---

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
- ✅ `make test-race` — Run tests with the Go race detector

## When to run `make test-race`

`make test-race` MUST be run (in addition to `make test`) whenever a change
touches concurrent code. Detect this by checking whether the diff adds or
modifies any of the following:

- The `go` keyword (new or changed goroutines).
- Channel operations: `chan`, `<-`, `close(`, or `select` blocks.
- Synchronization primitives from `sync`/`sync/atomic`: `sync.WaitGroup`,
  `sync.Mutex`, `sync.RWMutex`, `sync.Once`, `atomic.*`, or a semaphore
  pattern (`chan struct{}{}`).
- Worker-pool or pipeline code: anything under `internal/pipeline/` (fetcher,
  transformer, writer, metrics) or the concurrent listing in
  `internal/handlers/requests.go` → `Registry.BuildFetchRequests`.
- Shared state written from more than one goroutine (maps, slices, struct
  fields, package-level vars), or changes to the handler `Registry`'s locking.

If none of the above appear in the diff, `make test` is sufficient. When in
doubt, run `make test-race`. CI for concurrency-touching changes should run it
too.

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
1. Create the handler in the right subpackage — `internal/handlers/arm/<resource>.go` (package `arm`) for ARM types or `internal/handlers/graph/<resource>.go` (package `graph`) for Microsoft Graph types — implementing `ResourceHandler` interface:
   - `GetType()` - Return Azure resource type (e.g., "Microsoft.KeyVault/vaults")
   - `GetDocumentationPrompt()` - Dedicated per-type LLM documentation prompt (ARM: inline `models.ResourceDocumentation`; Graph: `documentation` field set to a `models.ResourceDocumentation{...}` literal, `AzureType` left unset)
   - `List(ctx)` - Enumerate all resource IDs of this type (ARM: shared pagers in `internal/azure/list.go`; Graph: page the collection via `@odata.nextLink`)
   - `Fetch(ctx, resourceID)` - Use Azure SDK to fetch resource
   - `Transform(resource)` - Convert to `*models.TransformedResource`
2. Add constructor: ARM `NewXHandler(credential, subscriptionID)`; Graph `NewXHandler(credential)` (Graph handlers build on the shared `GraphCollectionHandler`)
3. Register in `internal/handlers/defaults.go` → `registerDefaults()` function
4. Add unit tests in the same subpackage (`internal/handlers/{arm,graph}/<resource>_test.go`)
5. Update README.md "Supported Resource Types" table
6. Add a `CHANGELOG.md` entry under `## [Unreleased]` (see Changelog Policy in `02-style-and-quality.md`)

## "Add a CLI command"
1. Create `cmd/<command>.go` with Cobra structure. For a command **group with subcommands**, keep the parent in package `cmd` (like `docs`) and put each subcommand in its own directory `cmd/<command>/` as a separate package exposing an exported constructor (e.g. `NewGeneratePromptCommand`) that the parent attaches — a subcommand package must NOT import package `cmd` (import cycle), so it takes shared helpers from `../../internal/cmdutil`.
2. In `init()` (flat command) or the constructor (directory subcommand): `rootCmd.AddCommand(<command>Cmd)`, then **opt into the flag groups the command actually uses** from `../../internal/cmdutil` — `cmdutil.AddAzureAuthFlags`, `cmdutil.AddSelectionFlags`, `cmdutil.AddPipelineFlags` — plus any command-specific flags declared on the command's `Flags()` (NOT on `rootCmd.PersistentFlags()`; see "Add config option")
3. **Call `cmdutil.BindFlags(cmd)` as the first statement of `RunE`**, before reading any value. Local flags are bound to Viper per-execution so the global Viper singleton cannot pick up a sibling command's identically named flag. Skipping this silently breaks the flag > env > config > default precedence for that command.
4. Implement `RunE` function with:
   - Configuration loading via Viper (after `cmdutil.BindFlags`)
   - Azure client initialization
   - Handler registry setup
   - Pipeline execution
   - Error handling and user-friendly output
5. Add examples in command's `Long` description
6. Update README.md with new command usage
7. Add a `CHANGELOG.md` entry under `## [Unreleased]` (see Changelog Policy in `02-style-and-quality.md`)

**Do not add a flag to a command that ignores it.** Persistent flags were deliberately narrowed for this reason: `list` previously advertised `--type` and `--resource-group` and silently ignored them.

**Destructive commands**: if the command deletes anything, it must respect `--dry-run` by listing what it would remove, and share one eligibility decision between the preview and the real path (see `prunableKeys` in `internal/docs/metadata.go`) so the two cannot diverge.

## "Add a transformation"
1. Add function to `internal/transform/<transformation>.go`
2. Use in `internal/pipeline/transformer.go` → `transformResource()`
3. Add unit tests
4. Document behavior in function comment
5. Add a `CHANGELOG.md` entry under `## [Unreleased]` (see Changelog Policy in `02-style-and-quality.md`)

## "Add config option"
1. Add field to `models.PipelineConfig` struct
2. Declare the flag in the right place:
   - **Command-specific** (the normal case) → on that command's `Flags()`, or in the matching group helper in `../../internal/cmdutil` if more than one command needs it
   - **Global** → `cmd/root.go` → `PersistentFlags`, and only if *every* command genuinely needs it. Root currently holds only `--config`, `--output`, `--dry-run` and `--log-level`; adding a fifth needs a reason
3. Binding: local flags are bound automatically by `cmdutil.BindFlags(cmd)` in the command's `RunE` — do NOT add a manual `viper.BindPFlag()` for them. Only the four global flags are bound explicitly in `root.go`'s `init()`
4. Give the flag a default via a named constant if any logic branches on "was it set explicitly" — never compare a value against a duplicated literal; use `cmd.Flags().Changed("<name>")` (see `cmdutil.DefaultWorkerCount` in `../../internal/cmdutil`)
5. Use in pipeline/command
6. Update `config.example.yaml` (see the invariant below — this is mandatory for ANY option change)
7. Document in README.md
8. Add a `CHANGELOG.md` entry under `## [Unreleased]` (see Changelog Policy in `02-style-and-quality.md`)

**`config.example.yaml` must be updated on ANY change to a config option — not only when adding one.** This includes adding, renaming, removing, or re-defaulting an option, or changing what an existing value does. The file is the single reference schema for configuration, and it carries an explicit promise in its own header: **loading it unmodified must behave byte-for-byte identically to running with no config file at all.** Therefore:
- Every active key sets the tool's **built-in default**, so loading the file is a true no-op. Add the key with its default value; if a key has no usable default (credentials, selection filters) leave it empty; if a value is dangerous to leave enabled, document it **commented out**.
- **Never write an active value that changes observable output OR any recorded fact** versus running with no config. In particular, do not spell out sub-settings whose only effect is to change a hash — e.g. the `transformers` entries are bare names (empty settings) because writing their defaults explicitly would change `transformConfigSha256` in `resources/metadata.yaml` even though the transformation is identical. Illustrate such options in **comments** instead.
- Every option must still be **illustrated by a comment** describing what it does, its non-default alternatives, and the equivalent CLI flag / `AZURE_RD_*` env var where one exists.
- When in doubt, verify the no-op guarantee: a `download`/`docs` run with `--config config.example.yaml` must produce the same files and the same `resources/metadata.yaml` (including all hashes) as the same run without `--config`.

Hyphenated keys work as `AZURE_RD_*` env vars only because of `viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))` in `initConfig`. Do not remove it — without it `log-level` resolves to `AZURE_RD_LOG-LEVEL`, which no shell can export, and every hyphenated override silently stops working.

# Output shape
- Provide full file paths and complete code blocks
- Include `make deps` if new dependencies added (NOT `go mod tidy`)
- Show registration/wiring steps
- Provide example usage using make targets
- End with checklist of manual steps:
  ```
  ✅ Handler created
  ✅ Registered in internal/handlers/defaults.go
  ✅ Dependencies updated: make deps
  ✅ Built successfully: make build
  ✅ All checks passed: make check
  ✅ CHANGELOG.md updated under [Unreleased]
  ⚠️  Manual: Add to README.md supported types table
  ⚠️  Manual: Test with: ./azure-rd list
  ```

---
trigger: always_on
description: 
globs: 
---

# Makefile Usage Policy

**ALWAYS use Makefile targets instead of running commands directly.**

This project uses a Makefile to standardize development workflows. All instructions, documentation, and generated code must reference make targets, not raw commands.

## Required Make Targets

When providing instructions or examples:
- ✅ `make lint` — NOT `golangci-lint run --fix` (applies fixes, rewrites files)
- ✅ `make lint-check` — NOT `golangci-lint run` (reports only; what `check`/`ci` run)
- ✅ `make test` — NOT `go test ./...`
- ✅ `make build` — NOT `go build`
- ✅ `make fmt` — NOT `go fmt ./...` (rewrites files)
- ✅ `make fmt-check` — NOT `gofmt -l` (reports only; what `check`/`ci` run)
- ✅ `make deps` — NOT `go mod tidy`
- ✅ `make check` — Run fmt-check + lint-check + test; modifies nothing
- ✅ `make ci` — Run check + build (default goal; for CI/CD pipelines)
- ✅ `make test-race` — Run tests with the Go race detector
- ✅ `make test-scripts` — Test the readers behind the readiness reports and the start gate (part of `check`)
- ✅ `make start-item N=<n>` — Gate: may entry N of `NEXT-ITERATIONS.md` be implemented? (branch, clean tree, entry committed, plan open)
- ✅ `make branch-ready` — Clean-tree preflight, `ci`, then report whether this feature/fix branch is ready to ship (gate: fails on any ❌)
- ✅ `make release-ready` — `ci`, then report whether a release can be cut (report: fails only if all ❌)

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
1. Prefer adding the command to an existing group over a new top-level verb. The surface is grouped by noun: `resource` (commands acting on a tenant's Azure resources) and `docs` (commands acting on an export's documentation). Both parents live in package `cmd` (`cmd/resource.go`, `cmd/docs.go`) and each subcommand is a separate package in its own directory (`cmd/resource/`, `cmd/docs/`) exposing an exported constructor (e.g. `NewDownloadCommand`, `NewGeneratePromptCommand`) that the parent attaches — a subcommand package must NOT import package `cmd` (import cycle), so it takes shared helpers from `../../internal/cmdutil`, and anything else it shares with root (e.g. the tool version) lives in its own `internal/` package.
2. **Default to a configuration setting, not a flag.** The config file is the single source of truth; a new flag has to earn its place by being one of these:
   - **Bootstrap** — it says where the configuration is (`--config`, `--config-dir`). There should never be a third.
   - **Selector** — `--domain`, which picks the tenant and its profile. Declare it where the command needs it (persistently on `resource`, per subcommand on `docs`, locally on root for `--debug`) and register completion with `cmdutil.RegisterDomainCompletion`.
   - **Invocation-scoped** — it changes this run's verbosity, side effects or destination without changing *what* is produced (`--dry-run`, `--log-level`, `--output`/`--out`), or its meaning is **per command** so one flat config key could not carry it (`--prompt`, `--exit-code`), or it is ad-hoc selection whose persistence would be a trap (`--type`, `--resource-id`, `--resource-group`).
   Anything else is a config key — see "Add config option".
3. Register a flag where it is honoured: on the group parent (via the persistent variants in `../../internal/cmdutil`) only when *every* subcommand honours it, otherwise on the command's own `Flags()`.
4. **Read flag values from the command, not from Viper**, unless the flag is config-backed. Only `--output` and `--type` are bound (`viper.BindPFlag`, at their single declaration site); a flag declared in more than one place — `--domain`, `--out`, `--prompt`, `--exit-code` — MUST be read with `cmd.Flags().Get*` or `cmdutil.DeclaredDomain(cmd)`, because a global binding could resolve to a sibling command's copy. There is no `BindFlags` helper any more and none should be reintroduced: it existed only because every flag was config- and env-backed.
5. Implement `RunE` function with:
   - Configuration read from Viper (already loaded by root's `PersistentPreRunE`)
   - Azure client initialization
   - Handler registry setup
   - Pipeline execution
   - Error handling and user-friendly output
6. Add examples in command's `Long` description
7. Update README.md with new command usage
8. Add a `CHANGELOG.md` entry under `## [Unreleased]` (see Changelog Policy in `02-style-and-quality.md`)

**A command that acts on a tenant takes `--domain` and resolves its directory through `internal/tenantdir`.** Never join `<output>` and a domain by hand: the resolver is the one place that cross-checks the declared domain against the signed-in tenant, refuses a mismatch, refuses when neither is known (there is deliberately no flat-output fallback) and reports whether the domain was verified.

**Do not add a flag to a command that ignores it.** Persistent flags were deliberately narrowed for this reason: `list` previously advertised `--type` and `--resource-group` and silently ignored them. Hoisting a flag onto a group parent is the same mechanism and can reintroduce the same defect, so the test in `cmd/resource_test.go` asserts both directions: every subcommand sees the group's shared flags, and no subcommand offers one it ignores.

**Destructive commands**: if the command deletes anything, it must respect `--dry-run` by listing what it would remove, and share one eligibility decision between the preview and the real path (see `prunableKeys` in `internal/docs/metadata.go`) so the two cannot diverge.

## "Add a transformation"
1. Add function to `internal/transform/<transformation>.go`
2. Use in `internal/pipeline/transformer.go` → `transformResource()`
3. Add unit tests
4. Document behavior in function comment
5. Add a `CHANGELOG.md` entry under `## [Unreleased]` (see Changelog Policy in `02-style-and-quality.md`)

## "Add config option"
1. Add field to `models.PipelineConfig` struct
2. **Register the key in `internal/config`'s `keyScopes` table** — the single truth for the configuration partition — on the correct side:
   - **`ScopeTenant`** (profile only, `<config-dir>/<domain>.yaml`): it names something that exists inside one tenant, so a value from another tenant is meaningless or harmful (`subscription`, `client-id`, `tenant-id`, `filters`).
   - **`ScopeGeneral`** (base file only): it describes behaviour, tuning or the export format (`output`, `type`, `workers`, `timeout`, `transformers`, `taxonomy`, …).
   - **`ScopeFlagOnly`**: it is not configuration at all. List it anyway, so a file still carrying it gets a migration message naming the flag instead of "not a known setting".
   A key absent from the table is rejected as unknown, which is what turns a typo into an error. Placement is not cosmetic: a setting hashed into `transformConfigSha256` or `filtersSha256`, or one that decides the export layout, breaks comparability or invisibly relocates the export if it lands on the wrong side.
3. Give it a default: add it to `config.SetDefaults` if the zero value is wrong. **Do not register a default for a key whose mere presence is meaningful** — `viper.IsSet` is how that is detected (see `workers`, where a default would silently flatten the per-API counts).
4. Read it with `viper.Get*`. If logic branches on "was it set explicitly", use `viper.IsSet` — never compare against a duplicated literal; the named defaults live in `internal/config` (re-exported by `cmdutil`).
5. Use in pipeline/command
6. Update **both** example files (see the invariant below — mandatory for ANY option change)
7. Document in README.md, including the partition table and the migration table if a setting moved
8. Add a `CHANGELOG.md` entry under `## [Unreleased]` (see Changelog Policy in `02-style-and-quality.md`)

**`config.example.yaml` (general) and `config.example.domain.yaml` (tenant-scoped) must be updated on ANY change to a config option — not only when adding one.** This includes adding, renaming, removing, re-scoping or re-defaulting an option, or changing what an existing value does. They are the reference schemas, and each carries an explicit promise in its own header: **loading it unmodified must behave byte-for-byte identically to running without it.** Therefore:
- Every active key sets the tool's **built-in default**, so loading the file is a true no-op. Add the key with its default value; if a key has no usable default (credentials, filters) leave it empty; if a value is dangerous to leave enabled, or if its mere presence changes behaviour (`workers`), document it **commented out**.
- **Never write an active value that changes observable output OR any recorded fact** versus running with no config. In particular, do not spell out sub-settings whose only effect is to change a hash — e.g. the `transformers` entries are bare names (empty settings) because writing their defaults explicitly would change `transformConfigSha256` in `resources/metadata.yaml` even though the transformation is identical. Illustrate such options in **comments** instead.
- Every option must still be **illustrated by a comment** describing what it does, its non-default alternatives, and — for the two config-backed flags — the flag that overrides it for one run.
- A key must appear in the file matching its scope; `cmd/config_test.go` asserts that every key in the partition table is mentioned in the right example file, so a new option cannot ship undocumented.
- When in doubt, verify the no-op guarantee: a `download`/`docs` run with `--config config.example.yaml` must produce the same files and the same `resources/metadata.yaml` (including all hashes) as the same run without `--config`.

**There is no environment layer.** `viper.AutomaticEnv`, `SetEnvPrefix` and `SetEnvKeyReplacer` were removed, and no `AZURE_RD_*` variable is read: an environment value outranked the config file, so one left over from another tenant's run applied silently — and a wrong value here does not fail loudly, it produces a confident, wrong result. Do not reintroduce them. `LOG_LEVEL` is unaffected; the logger reads it directly.

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
  ⚠️  Manual: Test with: ./azure-rd resource types
  ```
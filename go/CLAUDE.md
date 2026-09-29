# azure-rd — Go CLI

`README.md` in this folder is the single source of truth for what the tool does today; update it in the same
change whenever a command, flag, setting, output file or supported type changes. Workflow, gates and
repo-wide rules: `../CLAUDE.md`. Path-scoped detail loads automatically from `../.claude/rules/`:
`go-style.md` (every `.go` file), `go-handlers.md` (`internal/handlers`, `internal/azure`),
`go-export-safety.md` (metadata, prune, drift tree, dry-run, secrets, error handling).

## Context

- Go module `azure-resource-downloader` (Go version per `go.mod`), Cobra + Viper, charmbracelet/log,
  gopkg.in/yaml.v3, Azure SDK (azcore, azidentity, armresources, armcompute, armstorage, armsubscriptions),
  Microsoft Graph SDK **v1.0 and beta** (Intune endpoints exist only on beta) + Kiota.
- **The version is the git tag.** `make build` stamps `git describe --match 'go/v*'` into `--version` and into
  every `resources/metadata.yaml` (`toolVersion`). There is no version file to bump.
- **Delegated user authentication only**: the `az login` session (`azidentity.AzureCLICredential`) or a
  device-code sign-in to a dedicated app registration named by `client-id`/`tenant-id` in the tenant's profile.
  Credential fields are always typed `azcore.TokenCredential`. App-only / service-principal auth is out of
  scope by design — never add it.

## Layout

- `cmd/` — Cobra commands. Root (`root.go`: global flags, `--debug`, config loading in `PersistentPreRunE`);
  `resource` parent (`resource.go`) with `download`/`drift`/`types`/`list` in `cmd/resource/`; `docs` parent
  (`docs.go`) with `generate-prompt`/`generate-index`/`analyze-drift` in `cmd/docs/`. Subcommand packages
  must not import package `cmd` (cycle); shared helpers live in `internal/cmdutil`.
- `internal/config/` — the configuration surface: the `keyScopes` partition (general / tenant-scoped /
  flag-only), base + profile resolution and merging, built-in defaults.
- `internal/tenantdir/` — the one resolver for "which tenant directory does this run act on".
- `internal/runprep/` — run preparation shared by `resource download` and `resource drift` (config, session
  verify, dedicated-app probe/prompt, auth, tenant/output resolution, registry, fetch requests).
- `internal/models/` — `ResourceHandler` interface, config/result types, API detection, documentation prompt
  builder + default template (`documentation_prompt.tmpl`), grouping vocabularies, filters.
- `internal/pipeline/` — 3-stage async pipeline: fetcher → transformer → writer (worker pools, channels),
  name planning, `doc-prompt.md` assembly, metrics.
- `internal/handlers/` — `Registry`, `BuildFetchRequests`, `defaults.go` (every handler registered here);
  `arm/` (ARM handlers + `arm_prompt.tmpl`), `graph/` (`GraphCollectionHandler` base, one constructor per
  Graph type, `*_prompt.tmpl` families: credential, group, record, referenced, singleton).
- `internal/azure/` — credentials, identity, tenant domain, ARM list pagers, permission-error detection,
  resource-id parsing and resolution.
- `internal/transform/` — cleaner, sanitizer, base64 decoding, scalar-list sorting.
- `internal/docs/` — `resources/metadata.yaml` (facts, partial-run merge, prune); `docs generate-prompt`
  engine (staleness vs frontmatter, referenced groups, template splice → `docs/generate.md`);
  `docs generate-index` (taxonomy → `docs/index.yaml`).
- `internal/drift/` — `resource drift` engine (comparability preflight, verdicts, field deltas, the `drift/`
  tree) and `docs analyze-drift` (observation preflight, payload verification, template → `drift/analyze.md`).
- `internal/logger/`, `internal/retry/`, `internal/version/`, `main.go`, `Makefile`, `.golangci.yml`
  (single lint truth), `config.example.yaml` + `config.example.domain.yaml` (reference schemas, must stay
  no-ops), `config-tailored-intune.yaml` (worked example), `scripts/` (readiness reports).

## Commands — always the Makefile

| Use | Never |
|---|---|
| `make build` (version-stamped `./azure-rd`) | `go build` |
| `make test`, `make test-race`, `make test-coverage` | `go test …` |
| `make lint-check` (reports), `make lint` (rewrites) | `golangci-lint run` |
| `make fmt-check` (reports), `make fmt` (rewrites) | `gofmt`, `go fmt` |
| `make deps` | `go mod tidy` |
| `make check` = fmt-check + lint-check + test + test-scripts; `make ci` = check + build (default goal) | |
| `make start-item N=<n>` (gate before implementing entry n), `make branch-ready` (gate), `make release-ready` (report) | editing versions or tags by hand |

Run `make test-race` in addition to `make test` whenever the diff touches goroutines, channels, `select`,
`sync`/`atomic`, `internal/pipeline/`, `Registry.BuildFetchRequests` or any state shared across goroutines.
When in doubt, run it. Run the binary as `./azure-rd resource download …` etc.; flags follow the command or
its group, never precede it.

## Architecture patterns

- **Pipeline**: fetcher → transformer → writer, each a worker pool joined by channels only (no shared state);
  worker counts per API (Graph 5, ARM 20; `workers-by-api`, or a single `workers` whose mere presence
  overrides them — detected with `viper.IsSet`, so that key has no registered default).
- **Handler registry**: every type is a `models.ResourceHandler` (`GetType`, `GetDocumentationPrompt`, `List`,
  `Fetch`, `Transform`) registered in `internal/handlers/defaults.go` → `registerDefaults()`; registry access
  guarded by `sync.RWMutex`; handlers get dependencies via constructor (ARM: credential + subscription id;
  Graph: credential only). Handlers also declare whether they need a dedicated app and have assignments.
- **Facts vs decisions**: `resources/metadata.yaml` records only what is read from a resource or computed
  from its bytes (hashes, display names, `@odata.type`, assignment targets, artifact names, presence). Anything
  derived from a rule you might revise (grouping, classification, counts) is computed later by the `docs`
  commands, so revising a rule never requires re-downloading a tenant.
- **Deterministic output everywhere**: sorted keys and scalar lists, id-decided file names, wall-clock-free
  `index.yaml`, `promptSha256` taken from the assembled `doc-prompt.md` bytes on disk.

## Non-negotiables

- Every exported symbol has a doc comment; `context.Context` is the first parameter of anything that does I/O;
  errors are returned (wrapped with `%w`), never logged *and* returned.
- **Every request produces exactly one result.** A stage that stops on `ctx.Done()` keeps draining its input
  and emits one `Cancelled` result per remaining item; `Pipeline.Execute` fails loudly when
  `len(Results) != TotalResources`. A dropped request is indistinguishable from a resource deleted in the
  tenant, so with `prune` it would delete a live file. `internal/pipeline/pipeline_test.go` covers it.
- `Transform()` must set a meaningful `DisplayName`: it names the file and is the only source for that name.
- Register handlers only in `defaults.go`; never in `cmd`. Adding or removing a type **must** update the
  "Supported resource types" table in `README.md` (type, permission, notes) and `CHANGELOG.md`.
- Commands never call `os.Exit`; `RunE` returns the error (exit codes via `cmdutil.WithExitCode`) and the single
  `os.Exit` lives in `cmd.Execute`, which also installs `signal.NotifyContext` so Ctrl+C drains cleanly.
- Permission errors (ARM 403, Graph missing scopes) never fail a run: warn and skip via
  `azure.IsPermissionError`. Non-zero exit only when `FailedResources > 0`; completeness (`Complete`) is
  tracked separately and is what anything inferring absence must gate on.
- `--dry-run` is honoured by every command and writes nothing (details in `go-export-safety.md`).
- Structured logging through `internal/logger` (charmbracelet/log), never `fmt.Println`/`log`; secrets are
  redacted; `LOG_LEVEL` is read directly by the logger.

## Command-line and configuration surface

**The configuration file is the single source of truth.** Precedence: `flag > tenant profile > base config
file > built-in default`. **There is no environment layer** — `viper.AutomaticEnv`/`SetEnvPrefix`/
`SetEnvKeyReplacer` were removed deliberately; never reintroduce them or any `AZURE_RD_*` variable.

- Two files, one **enforced** partition (`internal/config` `keyScopes`, validated per file *before* merging):
  general keys (`output`, `type`, `workers`, `workers-by-api`, `timeout`, `resolve-secrets`, `no-prompt`,
  `prune`, `transformers`, `taxonomy`) in the base file; tenant-scoped keys (`subscription`, `client-id`,
  `tenant-id`, `filters`) in `<config-dir>/<domain>.yaml`. A key on the wrong side, an unknown key or a key
  that is really a flag is a fatal error naming it. `transformers` is hashed into `transformConfigSha256`,
  `filters` into `filtersSha256`; both gate drift comparability, which is why the split is enforced.
- `--config <path>` names the base file (mistyped path = fatal); `--config-dir <dir>` holds profiles plus an
  optional `base.yaml` and **requires `--domain`** (config is read before authentication). The directory is
  deliberately not defaulted. Missing profile = fatal, listing the profiles that exist.
- A flag must earn its place: **bootstrap** (`--config`, `--config-dir`), **selector** (`--domain`), or
  **invocation-scoped** (`--dry-run`, `--log-level`, `--output`/`--out`, `--prompt`, `--exit-code`, `--type`,
  `--resource-id`, `--resource-group`, `--debug`). Everything else is a config key. Only `--output` and
  `--type` are viper-bound; a flag declared in more than one place (`--domain`, `--out`, `--prompt`,
  `--exit-code`) is read from the running command (`cmd.Flags().Get*`, `cmdutil.DeclaredDomain`). There is
  no `BindFlags` helper and none may return.
- Never add a flag to a command that ignores it (`cmd/resource_test.go` asserts both directions).
- **A run must know its tenant.** `internal/tenantdir` is the one resolver: the declared `--domain` is intent,
  the signed-in tenant is ground truth, a mismatch refuses before anything is written, and when neither is
  known the run refuses — there is deliberately no fallback to the bare output directory. Never join
  `<output>` and a domain by hand.
- With no configuration and no flags a run still performs a full export with the built-in defaults; keep that
  zero-config path working.

Procedures: `/new-handler`, `/add-command`, `/add-config-option`; workflow: `/promote-idea`, `/implement-item`,
`/implement-pair` (a go/web pair through the agent pipeline), `/item-done`, `/close-branch`. Finished
entries: `../.claude/archive/go/` (`/archive`).

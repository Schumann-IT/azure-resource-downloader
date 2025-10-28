# Style & Quality
- Formatting: run `go fmt` and `goimports` on all generated code
- Lint: write code that passes `golangci-lint run` with default linters
- Errors:
    - Wrap with `%w` and `fmt.Errorf` (no `%v`).
    - Sentinel errors via `var ErrX = errors.New("x")` in package scope.
- Concurrency:
    - Use `sync.WaitGroup` + channels for pipeline worker pools.
    - Always propagate context cancellation via select statements.
    - Guard shared state with `sync.RWMutex` (e.g., handler registry).

# Testing
- Table-driven tests: `_test.go`, `t.Run` subtests.
- Use `testing` + `require/assert` from `testify` when helpful.
- Deterministic I/O: inject dependencies; no network in unit tests.
- Aim ≥70% coverage for new packages; include one example test per package.

## Logging & Output
- Always use a structured logger framework, not `fmt.Println` or `log`.
- Default choice: [`github.com/charmbracelet/log`](https://github.com/charmbracelet/log)
    - Rich text, icons, and colorized levels (Info = 💡, Warn = ⚠️, Error = 🔥, Debug = 🐞).
    - Uses `log.NewWithOptions(os.Stderr, log.Options{ReportCaller: false})` for new instances.
    - Respect log level via `LOG_LEVEL` env variable (debug, info, warn, error).
- Each subsystem (HTTP, CLI, scheduler, etc.) should have its own logger instance with `With("component", "<name>")`.
- Prefer:
  ```go
  logger.Info("Starting server", "addr", cfg.Addr)
  logger.Warn("Cache miss", "key", key)
  logger.Error("Failed to fetch user", "err", err)

## Documentation Policy

- All documentation must live in a single, central **README.md** at the repository root.
- Do **not** create multiple docs (e.g., `CONTRIBUTING.md`, `USAGE.md`, `docs/*.md`) unless explicitly requested.
- The README must include:
  - Overview and purpose of the project.
  - Setup instructions (build, run, test, release).
  - Configuration and environment variables.
  - Example CLI usage or API examples.
  - Development conventions and coding guidelines.
- Internal packages may contain short inline Go doc comments, but **no separate Markdown files**.
- When generating new features or commands, Cursor should **update the main README.md** with concise usage notes instead of adding a new file.
- The README should be treated as the *single source of truth* for onboarding, configuration, and developer reference.
- Documentation: Never create new markdown files or docs folders. Append all relevant information to the root README.md.
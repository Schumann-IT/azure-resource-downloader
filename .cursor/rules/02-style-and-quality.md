# Style & Quality
- Formatting: run `go fmt` and `goimports` on all generated code.
- Lint: write code that passes `golangci-lint run` with default linters.
- Errors:
    - Wrap with `%w` and `fmt.Errorf` (no `%v`).
    - Sentinel errors via `var ErrX = errors.New("x")` in package scope.
- Concurrency:
    - Prefer `errgroup` for fan-out; always propagate context cancellation.
    - Guard shared state; avoid unnecessary goroutines.

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
# Style & Quality
- Formatting: run `go fmt` and `goimports` on all generated code
- Lint: write code that passes `golangci-lint run` with default linters
- Errors:
    - Wrap with `%w` and `fmt.Errorf` (no `%v`).
    - Sentinel errors via `var ErrX = errors.New("x")` in package scope.
    - Error strings must NOT end with punctuation (`.`, `!`, `?`) or newlines (`\n`).
    - Use lowercase for error messages unless starting with proper nouns or acronyms.
    - For multi-part error messages, use parentheses or commas, not newlines:
      ```go
      // ❌ BAD
      return fmt.Errorf("failed to connect: %w\nHint: check network.", err)
      
      // ✅ GOOD
      return fmt.Errorf("failed to connect: %w (hint: check network)", err)
      ```
- Concurrency:
    - Use `sync.WaitGroup` + channels for pipeline worker pools.
    - Always propagate context cancellation via select statements.
    - Guard shared state with `sync.RWMutex` (e.g., handler registry).

# Testing
- Table-driven tests: `_test.go`, `t.Run` subtests.
- Use `testing` + `require/assert` from `testify` when helpful.
- Deterministic I/O: inject dependencies; no network in unit tests.
- Never try to fix tests automatically; always 
- Aim ≥70% coverage for new packages; include one example test per package.

## Testing Philosophy: Analyze, Don't Auto-Fix

- When tests fail, **assume tests are correct by default** and that the implementation may require changes.
- **Do not automatically modify tests or implementation.** Cursor must first produce an analysis and refactoring plan.
- The assistant’s output should be a **Failure Handling Report** (no code edits applied), containing:

  1) **Summary** — brief description of failures and affected packages.
  2) **Failing Tests** — list of test names with error messages, stack frames, and likely root causes.
  3) **Hypotheses** — what changed or is missing (contracts, invariants, edge cases, concurrency, IO).
  4) **Option A: Refactor Implementation** — precise plan to make code satisfy existing tests.
     - Outline affected files/functions, API contract changes (if any), and data flows.
     - Risks and side-effects (perf, concurrency, error semantics, public API).
  5) **Option B: Refactor Tests** — if tests are outdated or incorrect, explain why and how to adjust them to the *current* intended behavior.
     - Call out flaky patterns, brittle timing, network reliance, and propose isolation strategies.
  6) **Migration Steps** — step-by-step order of changes (small commits), with **proposed patch blocks** only (no automatic edits).
  7) **Verification Plan** — exact commands to re-run (`go test ./...`, targeted packages), plus additional checks (race detector, `-run`, `-bench`, coverage).

- **Output format requirements**
  - Provide **unapplied** diffs in fenced `diff` blocks (unified format) for each option:
    - Label them clearly: `### Option A – Implementation patch (proposed)` / `### Option B – Test patch (proposed)`.
    - Keep patches minimal and focused on the described plan.
  - Include any interface or contract changes in bullet form before the patch.

- **Go specifics**
  - Favor table-driven tests, `httptest`, deterministic fakes, and `context.Context`.
  - Avoid network, time, or randomness in unit tests; inject dependencies instead.
  - If data races are suspected, propose running `go test -race ./...` and include suggested synchronization or design changes (e.g., guard shared state, use `errgroup`).
  - For logging in tests, default to no-color, buffer-based logger per our logging rule.

- **Important safeguards**
  - If the user explicitly asks to “fix tests” or “make tests pass”, first deliver the **Failure Handling Report** with both options and trade-offs. Do not apply either without explicit confirmation.
  - Never suppress or rewrite assertions to “green” the build without addressing semantics.

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
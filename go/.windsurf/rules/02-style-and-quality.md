---
trigger: glob
description: 
globs: **/*.go
---

# Style & Quality
- Formatting: run `go fmt` and `goimports` on all generated code
- Lint: write code that passes `make lint-check`, i.e. the linters configured in `.golangci.yml` (the default
  set plus `unconvert`, `unparam` and govet's `nilness`). That file is the single lint truth and GoLand runs it
  too, so the editor mirrors the config, never the other way round: fix a finding in the code, or silence it at
  the one site with a `//nolint:<linter>` stating why — never by dropping a linter to go green. Adding or
  removing a linter is a deliberate change to what every developer's editor reports, and belongs in
  `CHANGELOG.md`.
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
- Never try to fix tests automatically; always produce a Failure Handling Report first (see below).
- Aim ≥70% coverage for new packages; include one example test per package.

## Testing Philosophy: Analyze, Don't Auto-Fix

- When tests fail, **assume tests are correct by default** and that the implementation may require changes.
- **Do not automatically modify tests or implementation.** Cascade must first produce an analysis and refactoring plan.
- The assistant's output should be a **Failure Handling Report** (no code edits applied), containing:

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
  - If the user explicitly asks to "fix tests" or "make tests pass", first deliver the **Failure Handling Report** with both options and trade-offs. Do not apply either without explicit confirmation.
  - Never suppress or rewrite assertions to "green" the build without addressing semantics.

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
  ```

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
- When generating new features or commands, Cascade should **update the main README.md** with concise usage notes instead of adding a new file.
- The README should be treated as the *single source of truth* for onboarding, configuration, and developer reference.
- Documentation: Never create new markdown files or docs folders. Append all relevant information to the root README.md.
- `CHANGELOG.md` is the ONLY sanctioned Markdown file besides `README.md` (see Changelog Policy); do not treat it as a violation of the "no separate Markdown files" rule.

## Changelog Policy

- **Every change must be reflected in `CHANGELOG.md`.** No code, flag, behavior, config, or dependency change is complete until its user-visible effect is recorded — treat the changelog update as part of the change, not a follow-up.
- Add entries under the `## [Unreleased]` section, in the appropriate Keep a Changelog group: `Added`, `Changed`, `Fixed`, or `Breaking`. Create the group under `Unreleased` if it does not exist yet.
- Entries describe **behavior and intent** from the user's perspective (what changed and why it matters), not the commit or the files touched. Match the existing prose style: a bolded lead sentence, then the rationale.
- The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html); a breaking change goes under `Breaking` and drives the next major.
- Pure repo housekeeping with zero user-visible effect (e.g. moving files without an import or behavior change, formatting-only diffs) does not need an entry. When in doubt, add one.
- **Tests get no changelog entry.** Testing is assumed, not announced — a feature is only done when it is tested, so listing tests adds nothing a reader can act on. Test coverage requirements live in the Testing section above, not here.
- **Keep entries short: a bolded lead-in plus a few sentences.** State what changed, why it matters and which invariant it preserves. Leave out implementation detail (internal symbol, file and package names, per-defect narratives, counts) and configuration detail (flag/option enumerations, syntax, defaults) — **configuration is documented in `README.md`**, which is the single source of truth for it; the changelog says a config section exists and what it is for, and points there.
- Within a large `Added`/`Changed` group, entries are grouped by feature area under `####` subheadings (as in 0.1.0: *Downloading a tenant's configuration*, *Export metadata and pruning*, *Command-line surface and configuration*, *Per-type documentation prompts*, *Incremental documentation*, *Documentation index*). Put a new entry in the subheading it belongs to; add a subheading only when a genuinely new area appears.
- One entry per user-visible feature, the way a squash-merged feature branch would read. A fix or refactor that only completes an existing feature belongs **inside that feature's entry**, not as its own bullet; an internal-only change (a package move, an interface flag) gets no entry at all.
- An entry that requires operator action — a re-download, a one-time documentation regeneration, moving files in an existing export — must say so explicitly, in bold. That is the one detail never to trim.
- Released sections are SemVer versions with a date (`## [0.1.0] - 2026-09-06`), newest first, each matching a `go/vX.Y.Z` git tag. This project's version line is its own and unrelated to `web/`'s. Cutting a release means renaming `[Unreleased]` to a bare, **undated** `## [X.Y.Z]` and starting a fresh empty `[Unreleased]` — by hand, and only when asked; the date is stamped by the root release script when it publishes, never by hand. `make release-ready` never edits this file: it only reports whether a release can be cut (empty `[Unreleased]`, no struck-out `NEXT-ITERATIONS.md` entries, newest heading undated) and runs no git command. `make branch-ready` is its counterpart for a feature/fix branch and asks the opposite questions (`[Unreleased]` **written**, strikeouts cleared and entries renumbered; there is no version file to check — the version is the tag); it also changes nothing, but it reports every check and exits non-zero if **any** of them failed, so it can gate a merge. It is the one place in this project's readiness tooling that runs git: a read-only `git status --porcelain` scoped to `go/`, as a **preflight** that refuses to report on uncommitted changes (so the verdict describes the commit that will be merged) and degrades to a skip outside a clone. The repository-wide branch and working-tree checks, date stamping, tagging and the GitHub release happen from the repository root; the procedure lives in the **Development workflow** section of `../README.md`.
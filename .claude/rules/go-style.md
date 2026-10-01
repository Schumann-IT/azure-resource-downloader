---
paths:
  - "go/**/*.go"
---

# Go style, lint and testing (`go/`)

## Formatting and lint
- Run `make fmt` (`gofmt` is the only formatter, declared in `.golangci.yml`); `make fmt-check` reports.
- Code must pass `make lint-check`: golangci-lint's default set plus `unconvert`, `unparam`, `gocognit`
  and govet's `nilness`, per `.golangci.yml` — the single lint truth that GoLand runs too. Fix the finding,
  or silence it at the one site with `//nolint:<linter>` stating why. Never drop or weaken a linter to go
  green; adding or removing one changes every developer's editor and belongs in `CHANGELOG.md`.
- The **`gocognit` baseline** at the end of `exclusions.rules` is a **debt ledger, not an escape hatch**: it
  names the functions that already exceeded the Sonar threshold when the linter was enabled. **Never add your
  own function to it** — split the function, or `//nolint:gocognit` with a reason if the finding is genuinely
  wrong. It only shrinks: when editing a listed function anyway, split it and delete its entry in the same
  commit (golangci-lint does not report an exclusion that matched nothing, so a stale entry is invisible).
  Work one function at a time, with `make test-race` when it touches the pipeline; a split that needs a test
  edited is a redesign, not a split — the tests pinning the absence, coverage, prune and
  one-result-per-request invariants stay unchanged. Re-measure by commenting the block out. Splits are
  internal (no changelog); deleting the block at the end is not. Growing it needs the user's agreement and a
  changelog entry. The two path-scoped `gocognit` exclusions above it are permanent parity decisions mirrored
  in `sonar-project.properties`; the baseline is not mirrored on purpose.

## Errors
- Wrap with `fmt.Errorf("…: %w", err)` — `%w`, never `%v`. Sentinels: `var ErrX = errors.New("x")` at package
  scope.
- Messages: lowercase unless starting with a proper noun or acronym; no trailing punctuation or newline;
  multi-part messages use parentheses or commas, never `\n`.
- Always include the resource id in a resource error. Continue with other resources when one fails.

## Concurrency
- Worker pools: `sync.WaitGroup` + channels; always propagate cancellation through `select` on `ctx.Done()`;
  shared state behind `sync.RWMutex` (e.g. the handler registry). Every request still produces exactly one
  result on cancellation (see `go/CLAUDE.md`).
- Any diff touching `go` statements, channels, `select`, `sync`/`atomic`, `internal/pipeline/`,
  `Registry.BuildFetchRequests` or state written from more than one goroutine must also run `make test-race`.

## Logging
- `internal/logger` (charmbracelet/log) only — never `fmt.Println` or `log`. Key/value style:
  `logger.Info("Starting", "addr", addr)`; per-subsystem instances via `With("component", "<name>")`.
  Levels honour `--log-level` / `LOG_LEVEL`. Redact tokens, client secrets and resolved OMA-URI values.
- User-facing output uses emojis and clear progress lines; every run ends with a summary
  (successful / skipped / filtered / cancelled / failed, unlistable and empty types, completeness).

## Testing
- Table-driven `_test.go` with `t.Run` subtests; `testify` `require`/`assert` where helpful.
- Deterministic: inject dependencies, `httptest` and fakes; **no network, no time, no randomness, no real
  export tree** — fixtures live in temp directories.
- Aim for ≥ 70 % coverage on new packages, with an example test per package.
- Tests get **no** changelog entry.
- Failing tests: do not edit tests or implementation on your own — produce the Failure Handling Report
  (`/test-failure-report`) with unapplied `diff` blocks for both options and wait for confirmation.

## Documentation
- Doc comment on every exported symbol; explain *why* where a constraint is non-obvious.
- The only Markdown in `go/` is `README.md`, `CHANGELOG.md`, `NEXT-ITERATIONS.md`. New features and
  commands update `README.md` (usage, flags, settings, supported-types table); never add a new `.md`.
- Every user-visible change records a `CHANGELOG.md` entry, written when the entry is declared done
  (`.claude/rules/changelog.md`).

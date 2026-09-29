---
name: test-failure-report
description: Analyse failing Go (make test) or web (npm test) tests and produce a Failure Handling Report with proposed, unapplied patches instead of auto-fixing. Use whenever tests fail, or when the user asks to "fix the tests" or "make the tests pass".
---

# Failure Handling Report — analyse, don't auto-fix

Assume the tests are correct and the implementation is wrong. **Do not edit tests or implementation.**
Run the project's own target (`make test` / `make test-race` in `go/`; `npm test` in `web/`), then produce
this report and stop. Apply neither option without explicit confirmation from the user; never weaken, delete
or rewrite an assertion to green the build without addressing the semantics.

1. **Summary** — what fails, which packages/suites are affected.
2. **Failing tests** — each name with its error message, relevant stack frames and the likely root cause.
3. **Hypotheses** — what changed or is missing: contracts, invariants (one result per request, facts-only
   metadata, prune guards, path safety, one renderer instance, no-restart freshness), edge cases, concurrency,
   I/O.
4. **Option A: refactor the implementation** — precise plan to satisfy the existing tests: affected
   files/functions, contract changes, data flow, risks (performance, concurrency, error semantics, public
   surface). Then `### Option A – Implementation patch (proposed)` as an unapplied unified `diff` block.
5. **Option B: refactor the tests** — only if a test genuinely encodes outdated behaviour: why, and how to
   adjust it to the *current* intended behaviour; call out flaky patterns (timing, network, real export tree)
   and propose isolation. Then `### Option B – Test patch (proposed)` as an unapplied `diff` block.
6. **Migration steps** — small ordered commits.
7. **Verification plan** — exact commands to rerun (`make test`, `make test-race`, a targeted `-run`; `npm
   test`, a single spec), plus coverage or race checks.

Go specifics: favour table-driven tests, `httptest`, deterministic fakes, injected dependencies; propose
`make test-race` and synchronisation changes when a race is suspected; buffer-based, no-colour logger in tests.
Web specifics: fixtures in `fs.mkdtemp`, e2e through `configureViews(app)`, no real `output/`.

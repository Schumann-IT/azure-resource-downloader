---
title: One severity vocabulary for the drift index table
project: web
status: done
started: 2026-09-29
finished: 2026-09-30
branch: fix/drift-severity-vocabulary
changelog: Unreleased
---
## One severity vocabulary for the drift index table

**Goal.** Give every severity value in a drift index table the same treatment the tenant summary's Findings
table gets, so a reader is not left guessing why `high` rows carry an icon and `low` rows do not.

> **Why.** Two vocabularies exist by design: the tenant summary uses `critical / high / medium`, the drift
> analysis `high / medium / low / info` (`info` reserved for tool-fed inventory rows). The table tagging in
> `findings-table.ts` knows only the summary's set, so in `drift/index.md` the `low` and `info` rows fall
> back to the plain word — graceful (an unknown value must never become a wrong icon) but visibly uneven in
> the one table where all four appear. The drift page header in `drift-view.ts` already maps all four.
> Web-only: unifying on the Go side would touch the summary template (regeneration-gated) for no gain.
>
> **Contract.** Nothing changes on the Go side and nothing is regenerated; the web reads what the CLI's
> current `go/internal/drift/analyze_drift_template.md` asks the analysing agent to write. `drift/index.md`
> carries a Findings table "severity · verdict · resource · one-liner", rendered today as
> `| Severity | Verdict | Resource | Judgment |`, ordered high first; analysed rows take `high`, `medium` or
> `low`, and the inventory rows are appended verbatim with severity `info`, the one-liner "inventory change —
> not analyzed" and no link. A drift document's frontmatter `severity:` is `high|medium|low` (already mapped
> by `drift-view.ts`, untouched here). The summary's Findings table (`docs/summary.md`,
> `Severity | Finding | Affected | Documents`, `critical|high|medium`) is untouched. The template calls the
> index advisory prose ("a table fits well; prose is fine too"), so the browser keys the drift set on the
> table's shape — a Severity first column **and** a Verdict column, the test `findings-table.ts` already uses
> for `.findings-drift` — and a drift index written as prose or without a Verdict column simply gets no drift
> severity icons. The `data-severity` value stays the lowercase word in both tables (the hook the parked
> actionable-findings idea relies on).
>
> **Owner.** none — every file is under `web/`. No sequencing constraint.
>
> **Implementer.** sonnet
>
> **Decided in review.** Each table keeps its own closed set: a summary table leaves `low`/`info` plain, a
> drift table leaves `critical` plain. The drift table's colours follow its own scale — the tones
> `drift-view.ts`'s `SEVERITY_TONE` already gives the drift page badge — not the summary's word: `high` →
> the danger hue and octagon icon the summary uses for `critical`, `medium` → the amber triangle, `low` → the
> blue info circle, `info` → a neutral slate hue with its own quiet icon. A drift `high` drawn amber in the
> table but red in the page badge would be the unevenness this entry removes.

**Plan.**

- ✅ `src/docs/findings-table.ts`: add `DRIFT_SEVERITIES = ['high', 'medium', 'low', 'info'] as const` and its
  type beside `SEVERITIES`; make the severity normaliser take the closed set to check against, and have
  `applyFindingsTable` decide the table kind (Verdict column present or not) **before** tagging rows, so
  `annotateRows` gets the drift set for a `.findings-drift` table and the summary set otherwise. Row and
  cell attributes (`data-severity`, `title`) keep today's shape; a value outside the table's own set stays
  untagged plain text. Update the file's header and exported-function comments to name both sets.
- ✅ `src/styles.css`: scoped under `.findings.findings-drift` (so it outranks the summary rules), set
  `--sev-color` / `--sev-icon` for `high`, `medium`, `low` and `info` per the mapping in the Notes, reusing
  the summary's SVGs and hues for the first three and adding one neutral icon for `info`; add the matching
  dark-mode lifts in the existing `prefers-color-scheme: dark` block. Without these rules a tagged `low` or
  `info` cell would lose its word to the `text-indent` and draw no icon — the CSS is not optional. Update the
  drift-table header comment to name the severity set.
- ✅ `test/docs.e2e.spec.ts`, the drift index case: extend the `DRIFT_INDEX` fixture with a `low` row, an `info`
  inventory row (no link, "inventory change — not analyzed") and a `critical` row, and assert
  `<tr data-severity="…">` plus `<td data-severity="…" title="…">` for `high`, `medium`, `low` and `info`,
  and that `critical` stays untagged in the drift table; keep the existing `medium | shifted` row working.
- ✅ `test/docs.e2e.spec.ts`, the summary Findings case: add a `low` row to the summary fixture and assert it
  stays untagged (the summary set stays closed).
- ✅ `test/styles-build.spec.ts`: assert the compiled CSS carries `.findings-drift` rules with
  `[data-severity="low"]` and `[data-severity="info"]`.
- ✅ `CHANGELOG.md` under `[Unreleased]` → `### Fixed`.

---
title: Align tables by content, not by column position
project: web
status: done
started: 2026-10-03
finished: 2026-10-03
branch: fix/web-table-alignment
changelog: Unreleased
---
## Align tables by content, not by column position

*Kind:* fix

**Goal.** The tenant summary's *At a glance* table — and any table after it — reads cleanly whatever column order
the documentation agent chose: counts right-aligned and compact, text wrapping at full width. A test fails whenever
a stylesheet rule styles a table column by its position.

> **Why.** `src/styles.css` forces the at-a-glance table's *last* column to `width: 5rem; text-align: right`,
> assuming a count sits there. The go template leaves the table free ("as prose or one small table"), and real
> exports differ: `Area | Resources | Types` squeezes the type list into the 5rem right-aligned column and leaves
> the count left-aligned, while `Area | Types (count) | Resources` happens to look right. `styles-build.spec.ts`
> only checks that a rule survives compilation, not which cells it reaches, so nothing caught it.
>
> **Scope.** Web-only: no contract change, no regeneration; existing exports are fixed on the next request. The
> rule it enforces (tables styled by content, never by position; counts right-aligned) is already in
> `.claude/rules/web-style.md` and its Windsurf twin. The right-alignment is scoped to the at-a-glance section;
> widening it to every document table is not part of this entry. The Confluence export drops `data-*`
> attributes (`html-allowlist.ts`), so it is unaffected, as it was by the positional rule.
>
> **Contract.** Read from the go CLI's current output, unchanged: `docs/summary.md` carries the H2
> `## At a glance` (slug `at-a-glance`, wrapped by `wrapSections` as `[data-section="at-a-glance"]`), whose
> table — if any — has no fixed column order or column names. Positional styling stays only where the order is
> a CLI contract: the summary's Findings table (`Severity | Finding | Affected | Documents`, tagged `.findings`)
> and the drift index's findings table led by Severity (`.findings.findings-drift`); and the label | value
> metadata table every document opens with (`.doc-metadata`). `data-numeric` is a browser-internal attribute,
> not part of the contract.
>
> **Owner.** none — every file is under `web/`. No sequencing.
>
> **Implementer.** sonnet

**Plan.**

- ✅ `src/docs/section-hooks.ts`: a pure, exported pass `applyNumericColumns(tokens)` — per table, a column is
  numeric when it has at least one body cell holding an integer and every other body cell is empty or a dash
  placeholder (`-`, `–`, `—`). Integer means `^\d+$` or `^\d{1,3}(,\d{3})+$` after trimming and stripping
  surrounding `*`, `_` and backticks — export `normalise` from `findings-table.ts` (its lowercasing is
  harmless for digits) rather than duplicating it. Every `th` and `td` of such a column gets
  `data-numeric` (attribute present, no value needed). Called from the `doc_sections` rule in
  `markdown-renderer.service.ts`, after `applyMetadataTable` and before `wrapSections`. Every table is tagged;
  the visual effect is scoped in CSS. A short "why" comment on the export, as for the other passes.
- ✅ `src/styles.css`: replace the at-a-glance `td:last-child` / `th:last-child` rule with
  `.prose .doc-section[data-section="at-a-glance"] [data-numeric]` (`text-align: right`, `white-space: nowrap`,
  `font-variant-numeric: tabular-nums`, `width: 1%`); at-a-glance `th`/`td` get `vertical-align: top` and
  `overflow-wrap: anywhere`; correct the stale comment ("a sentence per row in its first column") to say the
  column order is the agent's and counts are found by content. Extend the `.doc-metadata` block comment so it
  names the label | value contract its `:first-child` rule relies on (the Findings block already names its
  column contract).
- ✅ `test/section-hooks.spec.ts` (inline fixtures, never `output/`): `Area | Resources | Types` and
  `Area | Types (count) | Resources` each tag exactly the count column, header and body; `1,234`, `**12**` and
  `` `7` `` count as integers; a dash placeholder cell keeps the column numeric; a column with one non-numeric
  cell (`12 (3 disabled)`, `1.5`) is not tagged; an all-empty or all-dash column is not tagged; header-only and
  empty tables are untouched.
- ✅ `test/docs.e2e.spec.ts`: a rendered `summary.md` fixture in each column order carries `data-numeric` on the
  count cells inside `[data-section="at-a-glance"]` and on no other cell of that table.
- ✅ `test/styles-build.spec.ts`: a guard over the compiled CSS. Split each rule's selector list on top-level
  commas (commas inside `:where(…)` / `:not(…)` stay together); a selector is a violation when a `td` or `th`
  compound carries `:first-child`, `:last-child`, `:nth-child(`, `:nth-last-child(`, `:first-of-type`,
  `:last-of-type` or `:nth-of-type(`, unless the selector also carries the class `findings` or `doc-metadata`
  (the contract tables; `findings-drift` only ever appears with `findings`). Two non-violations by
  construction: `@tailwindcss/typography`'s zero-specificity edge-padding defaults, i.e. a positional cell
  selector that sits entirely inside `.prose :where(…)` (`thead th:first-child`, `tbody td:last-child`, …), and
  row or container positions (`tbody tr:last-child`, `.doc-assignments > :first-child`). The test reports every
  offending selector. The matcher is a helper in the spec with its own cases: the removed
  `.prose .doc-section[data-section="at-a-glance"] td:last-child` is a violation; the typography `:where`
  defaults, `.prose table.findings td:nth-child(3)`, `.prose table.doc-metadata th:first-child` and
  `.doc-assignments>:first-child` are not. Plus an assertion that the at-a-glance `[data-numeric]` rule
  survives with `text-align: right`.
- ✅ Documentation at *done*: `CHANGELOG.md` `### Fixed`. No README change (no route, flag or setting).

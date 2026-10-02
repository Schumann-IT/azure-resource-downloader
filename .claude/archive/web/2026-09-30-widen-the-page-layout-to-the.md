---
title: Widen the page layout to the 2xl breakpoint
project: web
status: done
started: 2026-09-30
finished: 2026-09-30
branch: feat/drift-attribution
pr: 33
changelog: 0.4.0
---
## Widen the page layout to the 2xl breakpoint

**Goal.** Give the documentation, drift and compare pages more room on wide screens: the page layout and the
top bar share one width, 96rem (Tailwind's `2xl` breakpoint) instead of 80rem, and stay centred.

> Tailwind 4 has no `max-w-screen-*` utilities; the class is `max-w-(--breakpoint-2xl)`. The tenant picker
> and the error page keep their narrower `max-w-5xl`. A first attempt (`fix(web): widen the viewport`) lost
> the space before the class (`mx-automax-w-…`), so the layout was neither centred nor bounded.
>
> **Implementer.** sonnet

**Plan.**

- ✅ Every page layout (`views/tenant.hbs`, `resource.hbs`, `page.hbs`, `drift.hbs`, `drift-tenant.hbs`,
  `drift-diff.hbs`, `compare.hbs`, `compare-diff.hbs`) uses `mx-auto max-w-(--breakpoint-2xl)`, with the space.
- ✅ The top bar's default width in `views/partials/header.hbs` is the same class, so the breadcrumb lines up
  with the sidebar and the document.
- ✅ `test/docs.e2e.spec.ts`: the top-bar width assertions expect the new class; a new assertion pins the page
  layout's class, so a lost space fails a test.
- ✅ `CHANGELOG.md`: under `[Unreleased]` → `### Changed`, a layout entry.

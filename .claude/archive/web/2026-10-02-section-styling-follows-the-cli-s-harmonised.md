---
title: Section styling follows the CLI's harmonised heading sets
project: web
status: done
started: 2026-10-02
finished: 2026-10-02
branch: feat/prompt-consistency
pr: 54
changelog: Unreleased
---
## Section styling follows the CLI's harmonised heading sets

*Kind:* refactor

**Goal.** When the CLI's templates are harmonised, every section a regenerated document can carry is styled like
every other contract section — group documents' new *References* and *Lifecycle and operations*, and record
documents' new *Security* included — and a test keeps the browser's vocabulary in step with the CLI's heading
sets.

> **Contract.** The go entry *Consistent prompt templates, run-prompt fixes and the `summary:` frontmatter line*
> fixes the heading sets: default `References | Lifecycle and operations | Security | Settings`; conditional access
> `References | Conditions | Lifecycle and operations | Security | Settings`; referenced `References | Usage and
> references | Lifecycle and operations | Security | Definition`; singleton `References | Lifecycle and operations |
> Security | Settings`; arm `References | Lifecycle and operations | Security | Properties`; credential `References |
> Expiry and renewal | Lifecycle and operations | Security | Properties`; record `References | Lifecycle and
> operations | Security | Properties`; group `References | Membership | Usage as assignment target | Lifecycle and
> operations | Security | Properties`. H2 names are unchanged and every one is already in `SECTION_VOCABULARY`;
> styling keys on the heading text, never its position, so rendering needs no change and documents written
> before and after the regeneration render alike. The browser never reads these sets: the spec copies them (never
> importing from `go/`), so a CLI heading change has to update that table.
>
> **Owner.** none — every file is under `web/`. No sequencing: ships as a pair with that go entry, independent of
> it, harmless before the regeneration.
>
> **Implementer.** sonnet

**Plan.**

- ✅ `src/docs/section-hooks.ts`: rewrite the heading-set comment above `SECTION_VOCABULARY` to the eight families in
  the Contract ("the union of the eight per-template heading sets"), one line per family — default, singleton,
  arm, conditional-access, group, credential, record, referenced — plus the existing `summary.md` and
  `targeted-by` / `used-by` lines. `SECTION_VOCABULARY` itself is unchanged.
- ✅ `test/section-hooks.spec.ts`: a `HEADING_SETS` table (family → the Contract's heading texts, verbatim) and an
  `it.each` over it under `applySectionHeadings`: each heading, as an H2, gets `data-section` equal to
  `slugifyHeading(heading)` and the `SECTION_HEADING_CLASS` class; plus one test that the union of the sets'
  slugs is a subset of `SECTION_VOCABULARY`. Nothing in `src/` changes beyond the comment; `npm test` and
  `npm run build` pass.
- ✅ Documentation at *done*: `CHANGELOG.md` `### Changed` (Views and navigation): group documents' new *References*
  and *Lifecycle and operations* sections and record documents' *Security* section are styled like every other
  contract section, once regenerated.

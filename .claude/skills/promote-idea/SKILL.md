---
name: promote-idea
description: Promote a parked idea from NEXT-ITERATIONS.md into a numbered, planned entry through plan mode, or refine an existing entry's plan. Triggered by "promote idea <title>", "plan idea <title>" or "plan item N". Ends with a committed backlog entry, never with code.
disable-model-invocation: true
---

# Promote an idea (or refine an entry) through plan mode

Argument: `$ARGUMENTS` — an idea title (fuzzy match against `### Idea: …` headings), or `item N` for an
existing entry. The rule `.claude/rules/next-iterations.md` applies throughout.

1. **Locate.** Decide the project from the request or the idea's home (`go/NEXT-ITERATIONS.md` or
   `web/NEXT-ITERATIONS.md`); ask when ambiguous. Find the `### Idea: <title>` block (or entry N). Quote its
   text back in one line to confirm you have the right one.
2. **Plan mode, seeded with the block.** Call `EnterPlanMode`. The plan file starts from the idea's
   description, its *why parked* reasons and its *revisit conditions*, reconciled against what is true
   **now** (code, README, changelog) — a promotion is a review, not a copy. Explore, design, and ask the user
   the open questions. If the work is regeneration-gated (edits `documentation_prompt.tmpl` or a
   `*_prompt.tmpl`, or moves `promptSha256`), survey the other regeneration-gated entries and ideas and ask
   whether to batch them.
3. **On approval (`ExitPlanMode`), transcribe — do not paste.** The entry anatomy is fixed:
   - `**Goal.**` ← the plan's Context, in user/intent terms (one paragraph).
   - Notes blockquote ← rationale, scope, hash impact, cross-references, the reconciled revisit conditions,
     and "not regeneration-gated" / "regeneration-gated, batched with …" as applicable.
   - `**Plan.**` ← the work steps and the verification as concrete, implementable bullets, tests and
     documentation included (`README.md`, `CHANGELOG.md`, rule files if they change).
   Write it as the next number (`## N.` in `go/`; `### N.` under `## Features` or `## Fixes` in `web/`),
   delete the `### Idea` block (for `plan item N`: replace the entry in place), and keep `1..N` contiguous.
4. **Commit** `docs(<project>): plan <title>` (only the backlog file). Stop: implementation is a separate
   `implement item N`, and the start gate needs the entry in `HEAD`.

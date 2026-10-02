---
title: Mirror the CI-monitor wording in the Windsurf twin
project: web
status: done
started: 2026-10-01
finished: 2026-10-01
branch: docs/plan-exclude-type
pr: 38
changelog: 0.4.0
---
## Mirror the CI-monitor wording in the Windsurf twin

**Goal.** The Windsurf twin of the backlog protocol says what the Claude workflow now does after a push: start a
background CI monitor that reports back, instead of waiting for CI — so the two copies of the rule do not
disagree while the Windsurf files are kept.

> The web half of the go entry *Monitor CI in the background instead of waiting for it*, which changed the skills,
> the root `CLAUDE.md`, `.claude/rules/next-iterations.md` and both Windsurf twins in one commit (`7a6fbd5`). This
> entry exists because that commit touched `web/`, so the web branch gate needs a web backlog change; it adds no
> work of its own. Not regeneration-gated.
>
> **Owner.** none.

**Plan.**

- ✅ `web/.windsurf/rules/06-next-iterations.md`: the `implement pair` row reads "starts a CI monitor that reports
  back" (delivered in `7a6fbd5`).
- ✅ Documentation at *done*: `CHANGELOG.md` `### Changed` under *Release workflow* — the session monitors CI in the
  background instead of waiting; merging stays manual. No README change.

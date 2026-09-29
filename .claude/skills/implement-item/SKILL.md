---
name: implement-item
description: "Implement a numbered NEXT-ITERATIONS.md entry after its start gate passes — by default through the agent pipeline (/implement-pair with one side), or inline in this session with 'implement item N inline'. Triggered by 'implement item N'."
disable-model-invocation: true
---

# Implement entry N

Argument: `$ARGUMENTS` = N, optionally prefixed by the project (`go 2`, `web 1`) and optionally followed by
`inline`; ask when the project is ambiguous. `.claude/rules/next-iterations.md` and the project's
`CLAUDE.md` apply.

**Default: the agent pipeline.** Follow `.claude/skills/implement-pair/SKILL.md` with the single side
`<project> N` — plan review, the refinement checkpoint, one implementer, implementation review, one QA
agent; you orchestrate and commit, agents never do. Use the inline procedure below only when the user says
`inline` (or when agents are unavailable).

## Inline procedure

1. **Start gate first, and obey it.** `make -C go start-item N=<n>` or `npm --prefix web run start-item -- <n>`
   from the repository root. On ❌ stop and report the reason (on `main` → create a branch; dirty tree → the
   user commits or stashes; entry not in `HEAD` → commit the backlog; nothing open → `item N is done`).
   On ✅ the printed Goal and Plan are the work; nothing outside them is in scope.
2. **Work the plan bullet by bullet.** For each bullet: implement, test (`make -C go check`, `make test-race`
   when concurrency is touched; `npm test`, `npm run lint`, `npm run build` in `web/`), update `README.md` if
   the bullet changes a command, flag, setting, route or variable, write the `CHANGELOG.md` entry under
   `## [Unreleased]`, and strike the bullet (`- ~~…~~`) — all in the same edit. Strike the title once every
   bullet is struck. Never add scope; a discovery becomes a follow-up bullet (unstruck) or a note to the user.
3. **Commit as you go** in coherent commits (`feat(<project>): …`, `fix(<project>): …`), never on `main`.
4. **When the plan is delivered, stop.** Report what shipped and ask the user for follow-ups. Do not archive
   (`item N is done` is the user's call), do not run the branch gate, do not merge.

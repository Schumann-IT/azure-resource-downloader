---
name: item-done
description: Archive a finished (or dropped) NEXT-ITERATIONS.md entry to .claude/archive/<project>/ with its full plan, renumber the backlog and commit. Triggered by "item N is done" or "drop item N <reason>".
disable-model-invocation: true
---

# Archive entry N

Argument: `$ARGUMENTS` = N, optionally with the project (`go 2`) and, for a drop, `dropped <reason>`.
`.claude/rules/next-iterations.md` applies.

1. **Read the entry** from the project's `NEXT-ITERATIONS.md`. For `done`: every plan bullet struck, or the
   user explicitly accepts the unstruck ones as follow-ups that stay. For `dropped`: no code shipped from it
   (if some did, it is `done` with follow-ups, not dropped).
2. **Write the archive file** `.claude/archive/<project>/<finished-date>-<slug>.md` (`slug`: title in
   lowercase, `[a-z0-9]+` joined by `-`, at most ~6 words):
   ```
   ---
   title: <title, no ~~>
   project: go | web
   status: done | dropped
   started: <date the entry first appeared: git log --reverse -S'<title>' -- <project>/NEXT-ITERATIONS.md>
   finished: <today>
   branch: <current branch>
   changelog: Unreleased      # done; `none` for dropped
   ---
   ## <title>                  # number dropped
   <Goal, Notes and Plan verbatim, ~~ removed; delivered bullets `- ✅ …`;
    follow-ups that stay in the backlog `- ↪ … (now entry M)`; for dropped: `**Dropped.** <reason>` before the Goal>
   ```
   A partially delivered entry: archive only the struck bullets into the file (append if it already exists
   from an earlier branch), leave the entry with its open bullets in the backlog.
3. **Update the backlog**: remove the archived entry (or its struck bullets), renumber the remaining entries
   `1..N` in file order (`web/`: within `## Features` then `## Fixes`), and for `done` confirm the
   `CHANGELOG.md` `[Unreleased]` entry exists — write it now if it was missed.
4. **Commit** `chore(<project>): archive <title>` with the archive file and the backlog (and changelog if
   touched). Report the archive path. The branch gate (`/close-branch`) checks the rest.

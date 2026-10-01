---
name: item-done
description: Close a finished (or dropped) NEXT-ITERATIONS.md entry — write its README and CHANGELOG changes, archive it to .claude/archive/<project>/ with its full plan, renumber the backlog and commit. Triggered by "item N is done" or "drop item N <reason>".
disable-model-invocation: true
---

# Close entry N: documentation, then archive

Argument: `$ARGUMENTS` = N, optionally with the project (`go 2`) and, for a drop, `dropped <reason>`.
`.claude/rules/next-iterations.md` and `.claude/rules/changelog.md` apply. The user has verified the
implementation by hand before saying this; nothing here changes code.

1. **Read the entry** from the project's `NEXT-ITERATIONS.md`. For `done`: every code bullet struck, or the
   user explicitly accepts the unstruck ones as follow-ups that stay; documentation-only bullets may still be
   unstruck — they are done in step 2. For `dropped`: no code shipped from it (if some did, it is `done`
   with follow-ups, not dropped); skip step 2.
2. **Write the documentation now** — this is the one place it is written:
   - Gather the facts: the entry's Goal and Notes (the why), `git diff <base>..HEAD -- <project>` where
     `<base>` is the merge-base with `main` (the what), and the implementer / reviewer reports if they are
     in this conversation — their *Surface changes* and *Deferred to done* sections in particular.
   - `README.md` of the project: every new or changed command, flag, setting, route, environment variable,
     script, output file or supported type, in the section that owns it. Say what exists today; the README
     is the single source of truth.
   - `CHANGELOG.md` under `## [Unreleased]`: one entry per user-visible feature in the matching
     `### Added` / `### Changed` / `### Fixed` / `### Breaking` subsection (and `####` area), bolded lead-in,
     the why and the invariant that now holds, no implementation detail, operator action in bold. Purely
     internal work gets none.
   - Strike the documentation bullets this satisfied.
3. **Write the archive file** `.claude/archive/<project>/<finished-date>-<slug>.md` (`slug`: title in
   lowercase, `[a-z0-9]+` joined by `-`, at most ~6 words):
   ```
   ---
   title: <title, no ~~>
   project: go | web
   status: done | dropped
   started: <date the entry first appeared: git log --reverse -S'<title>' -- <project>/NEXT-ITERATIONS.md>
   finished: <today>
   branch: <current branch>
   pr: <left out; /pull-request adds it>
   changelog: Unreleased      # done; `none` for dropped
   ---
   ## <title>                  # number dropped
   <Goal, Notes and Plan verbatim, ~~ removed; delivered bullets `- ✅ …`;
    follow-ups that stay in the backlog `- ↪ … (now entry M)`; for dropped: `**Dropped.** <reason>` before the Goal>
   ```
   A partially delivered entry: archive only the struck bullets into the file (append if it already exists
   from an earlier branch), leave the entry with its open bullets in the backlog.
4. **Update the backlog**: remove the archived entry (or its struck bullets) and renumber the remaining
   entries `1..N` in file order.
5. **Commit** `docs(<project>): close <title>` with the README, the changelog, the archive file and the
   backlog together — the gate requires an archived entry's changelog line in the same branch. Report the
   archive path. `/close-branch` runs the gate.

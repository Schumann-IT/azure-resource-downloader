---
name: close-branch
description: Close a feature or fix branch — clear struck-out NEXT-ITERATIONS entries, renumber, confirm the changelog and README, commit, then run the branch-ready gate for each project the branch touched.
disable-model-invocation: true
---

# Close the branch

Run only when the user asks. Branch: `$ARGUMENTS` (or the current one). Follow the root README's
*Development workflow*, step 2.

1. `git status` must be clean and the branch must not be `main`. Determine which projects the branch touched:
   `git diff --stat main...HEAD -- go web`.
2. For each touched project's `NEXT-ITERATIONS.md`: delete every fully struck entry (`~~title~~` with all plan
   items struck); in a partially delivered entry delete only the struck plan items and keep the rest;
   renumber the remaining entries `1..N` in file order. Do not touch parked ideas or standing decisions.
   Keep the two projects' files independent.
3. Confirm `## [Unreleased]` in that project's `CHANGELOG.md` records every user-visible effect of the branch
   (compare against the diff); confirm `README.md` documents any new command, flag, setting, route or
   variable. Confirm `web/package.json` `version` and the changelog version headings were not touched.
4. Commit (`chore(<project>): close branch` or fold into the last feature commit if the user prefers).
5. From the repository root run `make branch-ready-go` and/or `make branch-ready-web`. Each refuses on a dirty
   folder, runs the pipeline, and exits non-zero on any ❌. Fix, commit, rerun until green. Report the final
   output verbatim.
6. Do **not** merge, push or open a pull request unless the user asks; when asked, use `gh`.

---
name: close-branch
description: Close a feature or fix branch — run the done step (README, CHANGELOG, archive) for every entry with struck bullets, confirm nothing is left struck out, commit, run the branch report locally, push and start a CI monitor that reports back.
disable-model-invocation: true
---

# Close the branch

Run only when the user asks. Branch: `$ARGUMENTS` (or the current one). Follow the root README's
*Development workflow*, step 3.

1. `git status` must be clean and the branch must not be `main`. Determine which projects the branch touched:
   `git diff --stat main...HEAD -- go web`.
2. For each touched project's `NEXT-ITERATIONS.md`: every entry with struck bullets goes through
   `/item-done N` now — that step writes the README and changelog from the plan, the diff and the reports,
   then archives (a fully struck entry whole; a partially delivered one only its struck bullets, keeping the
   open ones). Ask the user before treating an entry with open code bullets as done. Never delete an entry.
   Afterwards nothing is struck out and the remaining entries are numbered `1..N`. Do not touch parked ideas
   or standing decisions. Keep the two projects' files independent.
3. Confirm `## [Unreleased]` in that project's `CHANGELOG.md` records every user-visible effect of the branch
   (compare against the diff) and `README.md` documents every new command, flag, setting, route or
   variable. Confirm `web/package.json` `version` and the changelog version headings were not touched.
4. Commit anything left (`docs(<project>): …`, per `.claude/rules/commits.md`).
5. From the repository root run `make branch-ready-report-go` and/or `make branch-ready-report-web`: the
   clean-tree preflight and the branch report without the pipeline (no strikeouts, numbering, `[Unreleased]`
   written, not on `main`, the backlog changed on the branch, Conventional Commits, every entry archived as
   done grew `[Unreleased]`; web: `version` untouched). Exit non-zero on any ❌. Fix, commit, rerun until
   green. Report the final output verbatim.
6. Push (`git push -u origin <branch>`) and start the **CI monitor** of `/implement-pair` §6 on the push run.
   The branch counts as closed once the local reports are green and the push is done; the report says CI is
   pending. A red monitor report for `HEAD` offers the same user-confirmed fix round as `/implement-pair`
   (at most twice); a red run of an older commit is reported as stale. `gh auth status` must succeed for
   this; without `gh`, run the full local gates `make branch-ready-go` / `-web` instead and say so.
7. Do **not** merge or open a pull request here; that is `/pull-request`, on the user's request.

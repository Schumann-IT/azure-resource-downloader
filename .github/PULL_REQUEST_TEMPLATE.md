<!--
Title = the squash commit subject: Conventional Commits, `type(go|web): description`, lowercase, no trailing
period. One backlog entry → that entry's title behind its type; several → a short summary.
The /pull-request skill fills every section below from the branch; a hand-opened pull request fills them
the same way. Keep the headings.
-->

## Summary

<!-- Two or three sentences: what changes for a user or operator, and why. -->

## Backlog entries

<!-- Per project, one line per entry closed or advanced on this branch: title, then its Goal in one sentence.
     `go/` — … / `web/` — … ; "none" for a project the branch does not touch. -->

## Changes

<!-- The `## [Unreleased]` lines this branch added to go/CHANGELOG.md and web/CHANGELOG.md, verbatim.
     "no user-visible change" when a project's changelog did not grow. -->

## User actions

<!-- Anything an operator must do after merging (a GitHub setting, a re-download, a consent). "none" otherwise. -->

## Verification

<!-- The last lines of `make branch-ready-go` / `make branch-ready-web` on the final commit. -->

## Plans

<!-- One collapsible block per entry archived on this branch, containing the archive file verbatim
     (.claude/archive/<project>/<date>-<slug>.md) — the how, kept with the pull request as well as in the repo. -->

<details><summary>go: &lt;title&gt;</summary>

</details>

---
title: Monitor CI in the background instead of waiting for it
project: go
status: done
started: 2026-10-01
finished: 2026-10-01
branch: docs/plan-exclude-type
pr: 38
changelog: 0.4.0
---
## Monitor CI in the background instead of waiting for it

**Goal.** After the session pushes a branch (end of `/implement-pair` or `/implement-item`, `/close-branch`) or
opens a pull request (`/pull-request`), it no longer blocks until GitHub Actions finish. It starts a background
monitor on the run or the pull request's checks and reports back when they finish — green with the run link, or
red with the failing jobs and their log excerpt — so the operator can keep working, and reviews and merges on
GitHub once the monitor has reported.

> **Why.** The blocking waits buy no safety: branch protection on `main` already requires `ci-go`, `ci-web`,
> `branch-ready-go` and `branch-ready-web`, so nothing red can be merged whether or not the session waited. They
> only hold the session for minutes per push. Tried by hand on 2026-10-01 (pull requests #37 and the push before
> it): pushing without waiting and reading the result later worked.
>
> **What changes in meaning.** A step that used to end on "CI green" now ends on "gates green locally, pushed, CI
> pending, monitor running", and the session must say *pending*, never *green*, until the monitor reports. The
> red-CI fix round of `/implement-pair` becomes asynchronous: it applies only when the failed run is still for the
> branch's current `HEAD`; a failure of an older commit is reported as stale. The monitor reports only while the
> session is alive — GitHub's page stays the source of truth.
>
> **Contract.** No export-tree artefact changes: `go/` code, `web/` code, schemas, file names and exit codes
> stay as they are. The shared surface is the workflow text both projects follow, and it must say the same thing
> everywhere it appears: after a push the session starts a background CI monitor and reports *pending*; the
> monitor reports green (run or pull-request URL) or red (failing jobs plus a `--log-failed` excerpt); a red
> result is acted on only when its head commit is still `HEAD`, otherwise reported as stale; any fix round runs
> only after the user confirms; merging stays the user's action on GitHub. The status-check names the monitor
> reads are unchanged: `ci-go`, `ci-web` (push run; `success` or `skipped` is green) and `branch-ready-go`,
> `branch-ready-web` (pull request; the merge gate). The two `06-next-iterations.md` Windsurf twins carry the
> same `implement pair` row text as `.claude/rules/next-iterations.md`.
>
> **Owner.** go owns every file this entry touches, all outside `go/` except the go twin:
> `.claude/skills/implement-pair/SKILL.md`, `.claude/skills/close-branch/SKILL.md` (body and `description:`
> frontmatter), `.claude/skills/pull-request/SKILL.md`, `.claude/agents/qa.md` (its "then pushes and waits for
> CI" sentence), the root `CLAUDE.md`, `.claude/rules/next-iterations.md`, `go/.windsurf/rules/06-next-iterations.md`
> and `web/.windsurf/rules/06-next-iterations.md`. No code, no `web/` code change; no sequencing.
>
> Not regeneration-gated.
>
> **Implementer.** sonnet

**Plan.**

- ✅ The monitor, one procedure shared by the three skills (written once, as a "CI monitor" subsection of
  `implement-pair` §6, and referenced by name from `/close-branch` and `/pull-request`): after the push, record
  `SHA=$(git rev-parse HEAD)` and start **one** Bash command with `run_in_background: true` and an explicit
  `timeout` of 3600000 ms that (a) polls `gh run list --branch <branch> --event push --commit $SHA --json
  databaseId -q '.[0].databaseId'` every 5 s, at most 24 times, until the run registers (the poll lives inside
  the background command because a foreground `sleep` is blocked), exiting non-zero with "no push run for
  <sha>" otherwise, then (b) runs `gh run watch <id> --exit-status`. The session is notified when the command
  exits. On the notification: read `gh run view <id> --json jobs,url,headSha`; `ci-go` / `ci-web` `success`
  or `skipped` → report green with the run URL; otherwise report the failing jobs with a `gh run view <id>
  --log-failed` excerpt (tail, at most ~60 lines per job). No run registered or the background timeout hit →
  report "CI still pending/unknown" with the branch's Actions URL; never report green without having read the
  jobs. Without a background capability, fall back to the blocking wait and say so.
- ✅ `/implement-pair` §6 and §7: push, start the monitor, and finish the final report with "CI pending (monitor
  running)" and the branch's Actions URL instead of the result (the run URL arrives with the monitor's report;
  the "CI run URL and result" field of §7 becomes that report, posted when it arrives). When a red report arrives and the run's `headSha` is still
  `HEAD`, offer the existing fix round (one QA round with the log excerpt, commit, push, new monitor; at most
  twice) — the user confirms, nothing is fixed unprompted; a red run of an older commit is reported as stale.
- ✅ `/close-branch` §6: push and start the monitor; the branch counts as closed once the local reports are green and
  the push is done — the report says CI is pending. Remove "Only when both are green is the branch closed". A red
  monitor report for `HEAD` offers the same user-confirmed fix round as `/implement-pair` (at most twice); a red
  run of an older commit is reported as stale. The `description:` frontmatter drops "wait for the CI pipelines
  to be green" for "push and start a CI monitor that reports back". The no-`gh` fallback (full local
  `make branch-ready-go` / `-web`) stays.
- ✅ `/pull-request`: the preflight accepts a pending push run (it says so in the Verification section instead of
  quoting a green run) but refuses a red one for `HEAD`; after the link-back commit is pushed, start the monitor on
  the pull request — one background command (same `run_in_background` / 3600000 ms timeout) that polls every
  5 s, at most 24 times, until `gh pr view <N> --json headRefOid -q .headRefOid` equals the pushed `HEAD` and
  `gh pr checks <N>` lists checks, then runs `gh pr checks <N> --watch` — and on the notification read `gh pr
  checks <N> --json name,state,link` and report the merge-gate checks (`branch-ready-*`) and `ci-*`
  (`ci-*` skipped counts as green). The step ends with the URL and "checks pending (monitor running)"; when the
  monitor reports, the session tells the user the pull request is ready for review and merge on GitHub (or what
  is red). Merging stays the user's action.
- ✅ Wording: root `CLAUDE.md` (the agent-pipeline line "pushes once after QA and waits for CI" and "`/close-branch`
  … pushes and waits for CI to be green") and the `implement pair` row of `.claude/rules/next-iterations.md`
  ("pushes once, and waits for the `ci-*` jobs") say "pushes and starts a CI monitor that reports back"; the two
  Windsurf twins get the same row; `.claude/agents/qa.md` ("then pushes and waits for CI") says "then pushes and
  monitors CI".
- ✅ Check: `grep -rn -i "wait for the push run\|waits for CI\|waits for the .ci-\|wait for the CI\|--watch"
  CLAUDE.md .claude go/.windsurf web/.windsurf` finds nothing outside the monitor procedure itself, and the three
  copies of the `implement pair` row agree (the two twins identical to each other, and to the
  `.claude/rules/next-iterations.md` row apart from their trailing "This pipeline exists in Claude Code only…"
  sentence).
- ✅ Verification (session, after the implementation is committed — agents cannot push): run this branch's own
  `/close-branch` and `/pull-request` with the new procedure — both return immediately with "pending", and each
  monitor reports once its run or checks finish.
- ✅ Documentation at *done*: `CHANGELOG.md` (both projects, *Release workflow* area, `### Changed`): the session
  monitors CI in the background instead of waiting; merging stays manual. No README change (the workflow lives in
  `CLAUDE.md` and the skills).

> **Verification result.** The push half ran on this branch: after pushing `a498652` the monitor ran in the
> background and reported `ci-go` / `ci-web` green (run 36876444579) without blocking the session. The
> pull-request half runs with this branch's own `/pull-request`.

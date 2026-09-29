---
name: implement-pair
description: "Implement two backlog entries that ship together (go N and web M) — or a single one — through the agent pipeline: an Opus plan review that refines and commits the backlog (asking the user only for real decisions), parallel Sonnet/Opus implementation, per-side Opus review, parallel Sonnet QA, one push with CI as the authority. The main session orchestrates and owns git. Triggered by 'implement pair go N web M', 'implement pair go N' or 'implement item N'."
disable-model-invocation: true
---

# Implement a pair (or one entry) through the agent pipeline

Argument: `$ARGUMENTS` = `go N web M` | `web M go N` | `go N` | `web M`. You are the **orchestrator**: you
launch the agents defined in `.claude/agents/`, relay their fixed-shape reports, ask the user the decisions
the plan reviewer raises, and run git — the plan reviewer's backlog commits are the only exception.
`.claude/rules/next-iterations.md` and `.claude/rules/commits.md` apply throughout. Every command below
runs from the repository root. Single side: the same steps with one agent per stage.

## 0. Preflight (no agents)
- `gh auth status` succeeds (CI is the authority; without it stop: "run `gh auth login`").
- Current branch is not `main` (or `RELEASE_BRANCH`); `git status --porcelain -- go web` is empty.
- Each named entry exists in its `NEXT-ITERATIONS.md` (working copy). Record the sides and titles.

## 1. Plan review — `plan-reviewer` (opus), the decision loop
`Agent(subagent_type: "plan-reviewer", name: "plan-review", model: "opus")` with the sides and the branch.
- `refined` / `consistent` → confirm with `git log --oneline -3` that its commits touch only
  `NEXT-ITERATIONS.md` files and carry `docs(<project>): refine …` subjects. Continue.
- `needs-decision` → one `AskUserQuestion` call, one question per `D<n>`: the reviewer's options with the
  recommended one first and marked, plus "Stop the pipeline". Every answer a choice → `SendMessage(to:
  "plan-review", <the answers, verbatim>)` and wait for its next report, which is either another
  `needs-decision` (ask again, only the new questions) or `refined` / `consistent`. Repeat until a pass
  raises no decision. Any "stop", or no answer → print the open decisions and **stop**: the pipeline failed
  by design; the user answers in the backlog and reruns.
No other checkpoint exists.

## 2. Gates (orchestrator)
`make -C go start-item N=<n>` and/or `npm --prefix web run start-item -- <m>`; any ❌ → stop with the output
verbatim. Read from each entry's Notes: `Implementer` (tier), `Owner` (root files, sequencing).
`BASE=$(git rev-parse HEAD)`.

## 3. Implement in parallel — `implementer` ×1–2
In **one** turn launch both: `Agent(subagent_type: "implementer", name: "implementer-go", model: <the
entry's Implementer note>)` and `…"implementer-web"…`, each prompt carrying only the project, the entry
number and `BASE` (the entry holds the rest). If the `Owner` note sequences one side after the other's root
file, launch the dependent side after the owner's report.
- `partial` (turn budget) → one `SendMessage(to: "implementer-<side>", "continue, then report")`.
- `blocked` → relay the Failure Handling Report; commit only a *delivered* other side; stop.
- `delivered` → commit that side: `git add <project>/ <root files its Owner note lists>` then
  `feat(<project>): <title>` (`fix(web): …` for an entry under `## Fixes`). `git status --porcelain` must
  be empty afterwards; anything left is a scope leak — report it, do not commit it. **Do not push.**

## 4. Review in parallel — `impl-reviewer` ×1–2 (opus)
One turn: `Agent(subagent_type: "impl-reviewer", name: "review-go", model: "opus")` and `…"review-web"…`,
each with its project, entry number, `BASE` and that side's implementer report verbatim.
- A side `not-ready` → **one** fix loop on that side only: `SendMessage(to: "implementer-<side>", <its
  must-findings>)`, commit `refactor(<side>): review fixes for <title>`, re-run that side's reviewer once.
  Still `not-ready` → stop with the findings. The other side proceeds regardless.

## 5. QA in parallel — `qa` ×1–2 (sonnet)
One turn: `Agent(subagent_type: "qa", name: "qa-go", model: "sonnet")` and `…"qa-web"…`, each with its
project, entry number, its side's `should` findings and `BASE`. No git in the agents.
- Any `blocked` → relay, commit nothing, leave the tree for the user, stop.
- All `fixed` / `nothing-to-fix` → `git diff BASE..HEAD -- web/eslint-suppressions.json go/.golangci.yml`
  must be empty and `git status --porcelain` must list only files inside the sides' folders (a grown ledger
  or a stray file is a finding to report, not to commit). Commit per side that has changes:
  `refactor(<side>): review and ci fixes for <title>`.

## 6. Push and CI
`git push -u origin <branch>`; find the push run (`gh run list --branch <branch> --event push --commit
$(git rev-parse HEAD) --json databaseId,status,conclusion --limit 1`), wait for it (`gh run watch <id>
--exit-status`), read the jobs (`gh run view <id> --json jobs`): `ci-go` / `ci-web` must be `success` or
`skipped`. A failed job → `gh run view <id> --log-failed`, one more QA round on that side with the log
excerpt, commit, push, wait. Still red → stop with the log.

## 7. Final report, then stop
Per side: entry title, commit SHAs, bullets struck/open, **Surface changes** and **Deferred to done** from
the implementer's report (verbatim — the done step writes `README.md` and `CHANGELOG.md` from them), review
verdict, QA state, the CI run URL and result, user actions still owed (from the plan review), follow-ups
added to the entry. End with: "Verify the work; then `item N is done` writes the README and changelog and
archives the entry, and `/close-branch` runs the gate." Do **not** write documentation, do not archive, do
not run `branch-ready`, do not merge.

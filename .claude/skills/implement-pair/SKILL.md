---
name: implement-pair
description: "Implement two backlog entries that ship together (go N and web M) — or a single one — through the agent pipeline: Opus plan review, one refinement checkpoint, parallel Sonnet implementation, Opus implementation review, parallel Sonnet QA. The main session orchestrates and owns git; agents never commit. Triggered by 'implement pair go N web M', 'implement pair go N' or 'implement item N'."
disable-model-invocation: true
---

# Implement a pair (or one entry) through the agent pipeline

Argument: `$ARGUMENTS` = `go N web M` | `web M go N` | `go N` | `web M`. You are the **orchestrator**: you
launch the agents defined in `.claude/agents/`, relay their fixed-shape reports, and you alone run git.
`.claude/rules/next-iterations.md` and `.claude/rules/commits.md` apply throughout. Every command below runs
from the repository root.

## 0. Preflight (no agents)
- Current branch is not `main` (or `RELEASE_BRANCH`); `git status --porcelain -- go web` is empty.
- Each named entry exists in its `NEXT-ITERATIONS.md` (working copy). Record the sides and titles.

## 1. Plan review — `plan-reviewer` (opus)
`Agent(subagent_type: "plan-reviewer", name: "plan-review", model: "opus")` with: the sides (`go N`,
`web M`), the branch, and "produce the fixed report". Foreground; wait for it.

## 2. Checkpoint — the only pause
Show the user the report verbatim: Contract summary, Root-file ownership, Sequencing, Refinements, User
actions, Open questions. Wait for **go**, amendments, or stop. Do not continue on your own.

## 3. Refine and gate (orchestrator)
- Apply the approved refinements with Edit — the reviewer supplied exact replacement text; keep the entry
  anatomy and numbering; touch nothing but the named bullets. `git diff --stat` must list only
  `NEXT-ITERATIONS.md` files. Commit per project: `docs(go): refine <title>` / `docs(web): refine <title>`.
  Skip when the verdict was `consistent` and the user changed nothing.
- Start gates: `make -C go start-item N=<n>` and/or `npm --prefix web run start-item -- <m>`. Any ❌ →
  stop and show the gate output verbatim. Keep each gate's printed Goal + Plan.
- `BASE=$(git rev-parse HEAD)`.

## 4. Implement in parallel — `implementer` (sonnet) ×1–2
In **one** turn launch both (foreground): `Agent(subagent_type: "implementer", name: "implementer-go",
model: "sonnet")` and `…name: "implementer-web"…`. Each prompt carries: the project, the entry number, the
gate's Goal + Plan verbatim, the Contract summary verbatim, the root files that side owns (from
Root-file ownership), the sequencing notes, and `BASE`.
- If Sequencing makes one side depend on a root file the other side creates, launch the dependent side
  after the owner's report (or `SendMessage` it the "verify/extend" bullet once the owner is done).
- `Status: partial` (turn budget) → one `SendMessage(to: "implementer-<side>", "continue, then report")`.
- `Status: blocked` → relay its Failure Handling Report to the user; commit only a *delivered* other side
  (below); stop.
- `Status: delivered` → commit that side: `git add <project>/ <root files it owns>` then
  `feat(<project>): <title>` (`fix(web): …` for an entry under `## Fixes`). Afterwards `git status
  --porcelain` must be empty; anything left is a scope leak — report it, do not commit it.

## 5. Implementation review — `impl-reviewer` (opus)
`Agent(subagent_type: "impl-reviewer", name: "impl-review", model: "opus")` with `BASE`, the entries, the
Contract summary and both implementer reports verbatim.
- A side `not-ready` → **one** fix loop: `SendMessage(to: "implementer-<side>", <its must-findings plus
  the cross-side findings marked for it>)`; commit `refactor(<side>): review fixes for <title>`; re-run the
  reviewer once against the new HEAD. Still `not-ready` → stop and show the findings; QA is the user's call.

## 6. QA in parallel — `qa` (sonnet) ×1–2
One turn, both: `Agent(subagent_type: "qa", name: "qa-go", model: "sonnet")` and `…"qa-web"…`, each with
its project, entry number, the findings for its side (own + cross-side marked for it) and `BASE`.
- `blocked` → relay, no commit for that side.
- `green` → `git diff BASE..HEAD -- web/eslint-suppressions.json go/.golangci.yml` must be empty (a grown
  ledger is a finding to report, not to commit); commit `refactor(<side>): review and lint fixes for <title>`
  (skip when the diff is empty).

## 7. Final report, then stop
Per side: entry title, commit SHAs, bullets struck/open, **Surface changes** and **Deferred to done** from
the implementer's report (verbatim — the done step writes `README.md` and `CHANGELOG.md` from them), review
verdict, lint state, tests / build / race, user actions still owed (from the plan review), follow-ups added
to the entry. End with: "Verify the work; then `item N is done` writes the README and changelog and
archives the entry, and `/close-branch` runs the gate." Do **not** write documentation, do not archive, do
not run `branch-ready`, do not merge or push.

Single side (`go N` or `web M` alone, or `/implement-item N`): the same steps with one implementer and one
QA agent; the plan review's other-side sections read `n/a`.

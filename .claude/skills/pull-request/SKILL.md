---
name: pull-request
description: "Open the pull request for the current branch: build a Conventional Commits title and a description from the branch's backlog entries, changelog lines, gate output and archived plans (.github/PULL_REQUEST_TEMPLATE.md), let the user edit the title and body, then push and create it with gh and link the PR number back into the changelog and archive. Triggered by 'create the pull request' / 'open a PR'."
disable-model-invocation: true
---

# Open the pull request

Run only when the user asks, and only after `/close-branch` (the gates must be green on the final commit).
Merges are squash-and-merge, so the **title becomes the commit on `main`** and must follow
`.claude/rules/commits.md`.

1. **Preflight, fail fast.** `gh auth status` must succeed — otherwise stop with "run `gh auth login`".
   Branch ≠ `main`; `git status --porcelain` empty; `origin` exists. Base = `git merge-base HEAD main`.
   `/close-branch` must have run on this commit (report green, CI green); otherwise run
   `make branch-ready-report-go` / `-web` now and check the push run with `gh`; a ❌ stops here.
2. **Gather the facts** (all read-only): the projects touched (`git diff --stat <base>..HEAD -- go web`);
   the archive files added on the branch (`git diff --name-only --diff-filter=A <base>..HEAD --
   .claude/archive`) and, from their frontmatter, the entries closed; entries still open but advanced
   (`git diff <base>..HEAD -- go/NEXT-ITERATIONS.md web/NEXT-ITERATIONS.md`); the `[Unreleased]` lines added
   to each changelog (`git diff <base>..HEAD -- go/CHANGELOG.md web/CHANGELOG.md`, added lines only); user
   actions named in the entries; the gate output.
3. **Title.** One closed entry → `<type>(<scope>): <entry title in lowercase, imperative, ≤ 70 chars>`
   (`feat` for a Features entry, `fix` for a Fixes entry, `chore` for tooling or setup, `docs` for
   documentation only; scope `go` or `web`, none when both). Several entries → a short summary in the same
   shape. Breaking changes: `!` and a `BREAKING CHANGE:` line in the body's Summary.
4. **Body** from `.github/PULL_REQUEST_TEMPLATE.md`, every section filled: Summary; Backlog entries (per
   project, title + one-sentence goal); Changes (the added changelog lines verbatim); User actions;
   Verification (the gates' last lines); Plans (one `<details>` per archived entry, the archive file
   verbatim). End the body with the attribution footer the session is configured with.
5. **Review with the user.** Show the title and the full body. Wait for edits or a go. Apply their edits
   verbatim; re-check the title against the commit rule.
6. **Create.** `git push -u origin <branch>`, then `gh pr create --base main --head <branch> --title
   "<title>" --body-file <scratchpad file>`. Record the number `N` from the output.
7. **Link back.** Add `pr: N` to the frontmatter of each archive file added on the branch (after
   `branch:`), and append ` (#N)` to the last line of each changelog bullet the branch added under
   `[Unreleased]`. Commit `docs: link pull request #N`, push, then wait for the pull request's checks
   (`gh pr checks N --watch`): `branch-ready-go` / `branch-ready-web` are the merge gate and must be green,
   `ci-*` must be green or skipped.
8. Report the URL, the title, and what was linked. Do not merge; merging is the user's action on GitHub.

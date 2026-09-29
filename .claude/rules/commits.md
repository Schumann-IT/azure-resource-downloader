# Commit messages (every commit, every skill)

Agents never commit; the session does, and the branch gate checks every subject on the branch. Follow
[Conventional Commits 1.0.0](https://www.conventionalcommits.org/en/v1.0.0/#specification) strictly:

```
<type>(<scope>)!: <description>

<body — why, and which invariant now holds; wrapped at 100 columns>

BREAKING CHANGE: <what an operator must change>      (only for a breaking change; `!` on the subject too)
Co-Authored-By: …                                    (the footers the session is configured with)
```

- **type** ∈ `feat` (new capability for a user or operator), `fix` (a defect), `docs` (documentation,
  backlog, archive only), `refactor` (no behaviour change), `test`, `build` (dependencies, build files),
  `ci`, `chore` (tooling, setup, housekeeping), `revert`. Nothing else.
- **scope** is `go` or `web` when the commit touches one project, `release` for the release commit, and
  **omitted** for repository-level commits (root files, `.claude/`, both projects at once).
- **description**: lowercase first letter, imperative mood, no trailing period, ≤ 72 characters.
- The workflow commits are: `docs(<project>): plan <title>` (promotion or refinement), `feat(<project>): <title>`
  or `fix(<project>): <title>` (implementation), `refactor(<project>): review and lint fixes for <title>`,
  `docs(<project>): close <title>` (README, changelog, archive), `docs: link pull request #N`,
  `chore(release): go vX.Y.Z, web vX.Y.Z` (the release script).
- The pull request title is the squash commit on `main` and follows the same rule (`/pull-request`).
- `make branch-ready-*` fails on any subject that does not match; merge commits are ignored.

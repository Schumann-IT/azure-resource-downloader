---
name: release
description: Cut a release of go/ and/or web/ — close the changelog by hand, bump the web version, report readiness with make release-status, and only on explicit instruction run make release to tag, push and publish.
disable-model-invocation: true
---

# Release

Run only when the user asks. Target: `$ARGUMENTS` (e.g. `go 0.4.0`, `web 0.3.1`, or both). Follow the root
README's *Development workflow*, step 4. Releases are cut from `main` with a clean tree.

1. **Preconditions**: on `main` (`RELEASE_BRANCH` overrides), clean tree, `gh auth status` succeeds. Both
   `NEXT-ITERATIONS.md` files have no strikeouts (done entries are archived); `## [Unreleased]` of each
   project to release is non-empty. A project with an empty `[Unreleased]` is not released.
2. **Close each changelog by hand**: rename `## [Unreleased]` to a bare, **undated** `## [X.Y.Z]`, start a
   fresh empty `## [Unreleased]` above it. The version follows SemVer from the section's content (`Breaking`
   → major once past 1.0; today's 0.x line: breaking → minor). Never write a date — the script stamps it.
3. **web only**: `cd web && npm version X.Y.Z --no-git-tag-version` (updates `package.json` and
   `package-lock.json`).
4. Commit: `chore: bump versions` (or `chore(<project>): close changelog for vX.Y.Z`).
5. **Report**: `make release-status` from the root. It runs both `release-ready` reports (each runs the
   project pipeline and lists the archived entries still marked `changelog: Unreleased`) and lists which
   projects have an undated heading. Show the output; stop here unless the user explicitly said to publish.
6. **Publish** (explicit instruction only): `make release`. It stamps today's date into the changelog and the
   version into those archived entries, commits `release: go vX.Y.Z, web vX.Y.Z`, tags `<project>/vX.Y.Z`,
   pushes branch and tags, and creates the GitHub releases with the changelog section as notes. Nothing in
   the archive is edited by hand. Report the output verbatim, including the tags created.
7. Afterwards `make -C go build` on the tagged commit reports `vX.Y.Z`.

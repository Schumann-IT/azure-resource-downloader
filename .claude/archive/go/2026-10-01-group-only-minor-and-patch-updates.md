---
title: Group only minor and patch updates in Dependabot
project: go
status: done
started: 2026-10-01
finished: 2026-10-01
branch: docs/dependabot-majors
pr: 44
changelog: Unreleased
---
## Group only minor and patch updates in Dependabot

**Goal.** Dependabot's weekly grouped pull requests carry only minor and patch updates, so they stay green and
mergeable; a major version arrives as its own single-dependency pull request that can be judged — and, when it
needs work, planned — on its own, instead of one breaking major blocking every routine update.

> **Why.** `.github/dependabot.yml` groups every update (`patterns: ["*"]`). The first grouped npm pull request
> (#42, 2026-10-01) bundled 26 updates including about 16 majors — NestJS 11 → 12, TypeScript 5 → 7, Jest 29 → 30,
> `markdown-it` 14 → 15, `markdown-it-anchor` 9 → 10, `shiki` 3 → 4, `htmlparser2` 10 → 12, `js-yaml` 4 → 5,
> `diff` 8 → 9, `@types/node` 20 → 26 — and `ci-web` failed before any test ran: `npm ci` hit `ERESOLVE`
> (`ts-jest@29.4.14` needs `typescript <7`). The pull request was closed; the majors are parked as web ideas
> (toolchain, NestJS 12, the rendering stack).
>
> **Behaviour that stays.** A major-version pull request still changes only the dependency files, so it remains
> dependency-only for the branch gates and needs no backlog entry to *pass* them; whether it is *merged* is the
> operator's call — a red one is closed and, when the upgrade is wanted, planned as an entry.
>
> **Dependabot semantics (checked against the `groups` options).** `update-types` inside a group restricts the
> group by SemVer level (`major`, `minor`, `patch`); an update that matches no group is raised as an ordinary
> individual pull request. `commit-message` belongs to the ecosystem block, not to the group, so the individual
> pull requests keep `build(go)` / `build(web)` — subjects such as
> `build(go): bump github.com/spf13/cobra from 1.8.1 to 2.0.0 in /go`, next to the grouped
> `build(go): bump the go-dependencies group in /go with 3 updates`. For `gomod` a true major is usually a new
> module path (`…/v2`) Dependabot does not propose at all, so in practice the restriction bites on `v0` → `v1`
> and `+incompatible` jumps; the npm block is where it matters.
>
> **github-actions stays unrestricted.** Action majors are tag moves (`actions/checkout@v4` → `@v5`) that touch
> only `.github/workflows/`, have been green (#41, four updates in one group), and a red one is visible in the
> same pull request's checks; splitting them would only multiply `ci:` pull requests.
>
> Not regeneration-gated.
>
> **Contract.** No go ↔ web artefact contract: nothing under `output/<tenant>/` changes. The shared surface is
> `.github/dependabot.yml`, which configures both ecosystems; this entry edits the `gomod` and the `npm` block
> identically (`update-types: ["minor", "patch"]` in the group) and leaves each `commit-message.prefix`
> (`build(go)`, `build(web)`, `ci`) and the absence of `include: scope` untouched. The web side relies on its
> dependency-only rule (`web/scripts/lib/branch.js`: only `package.json` / `package-lock.json` changed under
> `web/`), which looks at changed paths, not at grouping, and is not edited.
>
> **Owner.** `.github/dependabot.yml` (root, the whole file including the `npm` block); `README.md` (root) and
> `go/README.md` at done. No file under `web/` changes. No sequencing constraint.
>
> **Implementer.** sonnet

**Plan.**

- ✅ `.github/dependabot.yml`: in the `gomod` and `npm` groups add `update-types: ["minor", "patch"]`, so majors fall
  out of the group and Dependabot opens them as individual pull requests with the same commit-message prefix; keep
  the `github-actions` group as it is (action majors are tag moves and have been green). Keep the header comment
  accurate: grouping covers minor and patch, majors come one per dependency, and `include: scope` stays out.
- ✅ Test: in `go/scripts/lib/changelog_test.sh`, extend the "all conventional" assertion of
  `unconventional_subjects_in` with the three Dependabot subject shapes —
  `build(go): bump the go-dependencies group in /go with 3 updates`,
  `build(go): bump github.com/spf13/cobra from 1.8.1 to 2.0.0 in /go` and
  `ci: bump actions/checkout from 4 to 5` — so a grouped and an individual pull request are both proven to pass
  the Conventional Commits check.
- ✅ Verify, without editing them, that the dependency-only exemption is independent of grouping: `make -C go
  test-scripts` (`go/scripts/lib/branch_test.sh`, cases 6–11: `go.mod` + `go.sum`, `go.sum` alone, plus a `.go`
  file, plus the backlog, outside `go/` only, plus a path outside `go/`) and `npm --prefix web test --
  readiness-git` (`web/test/readiness-git.spec.ts`, `describe('dependency-only branches')`: `package.json` +
  `package-lock.json`, the lock file alone, plus a source file, a nested `package.json`, a changed backlog, paths
  outside `web/`) both pass; every case fixes changed paths only, none a pull request or a group.
- ✅ User action (no agent can do it): after the merge to `main`, open the repository's Dependabot page (Insights → (handed to the operator in the pull request's User actions)
  Dependency graph → Dependabot) and confirm the three ecosystems show no configuration error; on the next weekly
  run, confirm the npm majors arrive as individual `build(web): bump … in /web` pull requests.
- ✅ Documentation at *done*: the root `README.md` and `go/README.md` passages on Dependabot say that weekly groups
  carry minor and patch updates and that majors arrive one per dependency and are merged only when green or planned
  as an entry; `go/CHANGELOG.md` *Release workflow*, `### Changed`.

---
paths:
  - "go/CHANGELOG.md"
  - "web/CHANGELOG.md"
---

# Changelog policy (both projects)

`CHANGELOG.md` is part of the change, not something left to a release. **Every** change a user or operator
can notice — commands, flags, settings, routes, views, behaviour, environment variables, scripts,
dependencies, security boundaries, bug fixes — gets an entry under `## [Unreleased]`. It is written **when
the entry is declared done** (`item N is done`, `/item-done`), after the user has verified the work: from
the entry's Goal and Notes, the diff and the implementation reports, in the same commit as the entry's
archive file — never during implementation, and never later than the branch close (the gate refuses an
archived entry without it). Purely internal edits (a rename, a comment, a package move, a test-only
addition) get none; when in doubt, add one.

## Entries
- Keep a Changelog + SemVer. Place new entries under `## [Unreleased]` in `### Added` / `### Changed` /
  `### Fixed` / `### Breaking` (create the subsection if missing). Never edit an already-released section.
- Within a long section, group by feature area under `####` subheadings (e.g. Go: *Command-line surface and
  configuration*, *Drift analysis*; web: *Views and navigation*, *Drift view*, *Confluence export*). Put an
  entry in the subheading it belongs to; add one only for a genuinely new area.
- **One entry per user-visible feature**, the way a squash-merged branch would read. A fix that only
  completes an existing feature goes **inside** that feature's entry.
- **Short: a bolded lead-in plus a few sentences** describing behaviour and intent from the user's
  perspective — what changed, why it matters, which invariant now holds (read-only, path safety, one
  `markdown-it` instance, no client-side JS, no-restart freshness, one-result-per-request, facts-only
  metadata…). Leave out implementation detail (symbols, files, counts, defect-by-defect narratives) and
  configuration detail (option enumerations, defaults, syntax) — those live in `README.md`; say the option
  exists and point there.
- An entry that requires operator action — a re-download, a documentation regeneration, moving files in an
  existing export, moving flags into a config file — says so **in bold**. That is the one detail never to trim.
- **Tests get no entry.** Testing is assumed.
- Deliberate scope cuts go in `NEXT-ITERATIONS.md` and are referenced, not duplicated.
- A breaking change goes under `### Breaking` and drives the next major.

## Released sections and versions
- Released sections are `## [X.Y.Z] - YYYY-MM-DD`, newest first, each matching a `go/vX.Y.Z` or
  `web/vX.Y.Z` tag. The two version lines are unrelated.
- Cutting a release means renaming `[Unreleased]` to a bare, **undated** `## [X.Y.Z]`, starting a fresh empty
  `## [Unreleased]` above it and, for `web/`, bumping `version` in `package.json` and `package-lock.json` —
  **by hand, and only when the user asks** (`/release`). The date is stamped by the root release script,
  never by hand. The old `RC1`/`RC2` naming is retired.
- `make release-ready` / `npm run release-ready` only report (empty `[Unreleased]`, no strikeouts, newest
  heading undated, web `package.json` matching; they list the archived entries the release will stamp).
  `make branch-ready` / `npm run branch-ready` ask the opposite (`[Unreleased]` written, done entries
  archived, the backlog touched, not on `main`, `version` untouched) and exit non-zero on any failure.
- The release commit also stamps `changelog: X.Y.Z` into every archived entry of that project still marked
  `changelog: Unreleased` — never by hand.
- When the branch's pull request exists, `/pull-request` appends ` (#N)` to each entry the branch added, so
  a released changelog links every feature to the pull request that carried its plan.

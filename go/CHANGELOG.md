# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

This project is released independently of the documentation browser in `web/`: its releases are tagged
`go/vX.Y.Z` in the monorepo and the versions below are its own, unrelated to `web/`'s. See the
**Development workflow** section of the [repository README](../README.md) for the procedure.

## [Unreleased]

### Added

#### Release workflow

- **Work starts from the backlog, and the tooling now says so.** Every change begins as a numbered entry in
  `NEXT-ITERATIONS.md`, committed before it is implemented: `make start-item N=<n>` is the gate that checks
  this — not on the release branch, a clean `go/`, entry `N` present in `HEAD`'s backlog with an outstanding
  plan item — and prints the entry's Goal and Plan. When an entry is done it is no longer deleted but
  **archived** with its full plan to `.claude/archive/go/` at the repository root, so how something was built
  stays reviewable while this changelog keeps the what and why; the release stamps each archived entry with
  the version that shipped it. `make branch-ready` gained three read-only git checks on top of the existing
  ones: the branch is not the release branch, the backlog changed on it, and every entry archived as done on
  it is recorded under `[Unreleased]`. **A branch that changes `go/` without touching `NEXT-ITERATIONS.md` now
  fails `make branch-ready`** — a small fix gets a small entry — **and so does a commit whose subject does
  not follow Conventional Commits** (`type(go|web|release)!: description`), since the pull request title
  becomes the squash commit on `main`. The readers behind the reports are tested by
  `make test-scripts`, part of `make check`. Outside a git clone the new checks skip with a note, so the
  tooling stays usable there. See the **Development workflow** section of the
  [repository README](../README.md). (#30)
- **The branch gate now runs on GitHub, split into two rhythms.** The `branch-ready` workflow runs the
  pipeline — `make -C go ci`: format, lint, tests, build — as the status check `ci-go` on **every push**, so a
  red result arrives while the work is still in hand, and the branch report — strikeouts, numbering,
  changelog, archive, commit subjects — as the status check `branch-ready-go` on the **pull request**, in
  seconds, through the new `make branch-ready-report` (the gate without its pipeline; `make branch-ready`
  with the pipeline stays for offline use). Each job runs only when the pull request touches `go/`, its
  archive, the root `Makefile` or the workflow; a skipped job counts as passed, so the checks always report.
  The pull request's branch is checked out by name with full history, Go comes from `go.mod` and
  golangci-lint is pinned to the version used locally. **Branch protection on `main` requires `ci-go`,
  `ci-web`, `branch-ready-go` and `branch-ready-web`, with branches up to date and no bypass for
  administrators; merges are squash only** — a repository setting, made by hand. See the **Development
  workflow** section of the [repository README](../README.md). (#31)

#### Drift analysis

- **Drift findings now say who changed the resource, and when.** A drift observation says a resource's
  bytes moved between the baseline and the observation; the new `resource audit` command joins every
  finding against the tenant's Log Analytics audit tables (Intune's and Entra ID's) over exactly that window
  and records the actor, activity, result and time of each change in `drift/audit.yaml` beside the
  observation — so a deliberate administrative change can be told from an unexplained one. When a tenant
  profile sets the new `audit-workspace-id` key, every `resource drift` run attributes its own observation
  as well, and `docs analyze-drift` hands the attribution to the analysis agent as a `Changed by:` line per
  event, with the instruction that an actor is a fact about *who*, never proof of intent. Attribution is
  facts only and never alters the observation or a verdict: every finding gets exactly one status, so *no
  event found*, *no audit target*, *before the table's retention* and *could not look* never read the same,
  and a failed lookup is a warning — the drift run's exit code stays the comparison's. Audit records arrive
  with an ingestion lag, so `resource audit` can be rerun later without fetching the tenant again. The key is
  configuration only — a workspace GUID in the tenant profile, with no flag and no environment variable, since
  a workspace typed for one tenant would silently answer "nobody changed it" for the next. **To use it: send
  Intune's and Entra ID's audit logs to one Log Analytics workspace through diagnostic settings, grant the
  signed-in user Log Analytics Reader on it, set `audit-workspace-id` in the tenant profile, and — when you
  sign in through a dedicated app registration — add the Log Analytics API delegated permission `Data.Read`
  to that app and grant admin consent again**; without it the sign-in fails with `AADSTS650057`. See
  **resource audit** and **Create the app registration** in the [README](README.md). (#33)

#### Command-line surface and configuration

- **A tenant profile can exclude resource types instead of the base file listing every wanted one.** The new
  `exclude-type` key (tenant profile only) names types that are never listed for that tenant — typically the
  ARM types of an Intune/Entra-only tenant whose account has no subscription role, which otherwise fail with a
  403 on every run and keep every export and drift run incomplete. The only alternative was a base-file `type`
  allow-list of every wanted type, shared by all tenants of the config directory and silently missing any type a
  later release adds. Excluded types are never requested, so the run stays complete and absence, prune and
  removals keep working; their earlier files are left alone. A `--type`, `--resource-id` or `--resource-group`
  that targets an excluded type is refused rather than quietly overriding the profile, an unknown name is a
  fatal error, `resource types` marks excluded types instead of querying them, and the exclusion is recorded with
  the export (`run.scope.excludedTypes`) so drift cannot compare across a changed one. **After adding or changing
  `exclude-type`, run `resource download` before the next `resource drift`** — drift refuses until the baseline
  carries the same exclusion. See `README.md` (*Excluding types per tenant*). (#38)

### Changed

#### Release workflow

- **The session no longer blocks on CI after a push; it monitors the run and reports back.** After
  `/implement-pair` (or `/implement-item`), `/close-branch` and `/pull-request` push, the session starts a
  background monitor on that commit's push run — or, for a pull request, on its checks — and says *pending*
  until the monitor reports: green with the run URL, or red with the failing jobs and a log excerpt. A red result
  is acted on only while it is still for the branch's `HEAD`, and a fix round runs only after the user confirms;
  merging stays the user's action on GitHub. Waiting bought no safety — branch protection already requires all
  four checks before a merge — it only held the session. The app itself is unchanged. See the root `CLAUDE.md`
  and the workflow skills. (#38)

#### Documentation and drift analysis

- **Both management summaries now lead with the verdict.** The tenant summary the documentation agent writes
  opens with short bold-led paragraphs on what is managed, whether the settings are consistent and whether the
  configuration is actually in force, and closes with a **bottom line**: does anything demand action before the
  next export. The drift analysis index opens with the same bold bottom line, then one short section per theme
  (a group of findings that share a cause, not a resource type), most severe first, naming the actor where the
  change is attributed and the recommended action; the remaining low-severity findings go into a final
  *Everything else*. A reader who reads only the summary now learns what to do; the evidence stays in the parts
  below. Neither template is hashed, so no regeneration is required: the next `docs generate-prompt` or `docs
  analyze-drift` run hands the agent the new structure. (#33)

### Fixed

#### Release workflow

- **`make branch-ready-go` no longer refuses a branch that planned an entry and closed it on the same branch.**
  The gate requires every branch that changes `go/` to touch the backlog; it judged that by the net change to
  `NEXT-ITERATIONS.md`, so an entry added and archived on one branch left the file as it was on `main` and the
  gate failed with *NEXT-ITERATIONS.md is unchanged on this branch* although the branch had visibly delivered
  its backlog. An entry archived under `.claude/archive/go/` on the branch now counts as well, and the ok line
  says which applied: *changed on this branch* or *delivered on this branch (N archived entry(ies))*. An
  archive file of the web project never counts, and the failure message is unchanged. (#36)

#### Downloading and export metadata

- **A tenant without organizational branding no longer makes every export incomplete.** Graph answers 404 when
  no default branding is configured; the CLI read that as *could not be listed* and blamed a missing permission,
  so every download and drift run of such a tenant was incomplete — which suppresses absence and removals —
  although nothing was wrong. A 404 now lists the type as empty; any other error still fails it. Error
  summaries read `HTTP <status> <code>: <first line>` and add the permission hint only to a 401/403, so a 404,
  throttling or server error no longer reads as a permission problem; a Graph 401/403 while fetching one
  resource skips it with a warning. `resources/metadata.yaml` now records why each type could not be listed
  (`notListed.reasons`), and `docs/generate.md` shows that reason. A download whose types all list empty — for
  example `--type Microsoft.Graph/organizationalBranding` on such a tenant — records the metadata and exits `0`
  instead of failing with *no resources to download*; that error is now reserved for a run where nothing could
  be listed at all. No action needed: the next download records the branding as empty. (#37)

#### Command-line surface and configuration

- **`--type` help no longer claims to narrow the configured types.** It replaces the base file's `type` list for
  that run, as the README always said; the help text now says so, and that the tenant profile's `exclude-type`
  still applies. (#38)

### Breaking

#### Command-line surface and configuration

- **The configuration file is now the single source of truth, and the command line carries only what belongs
  there.** Every setting used to be reachable from a flag, an `AZURE_RD_*` variable and the config file at
  once, with the flag winning silently — so one tenant's credentials could be paired with another tenant's
  export, and the destructive and secret-writing switches could be typed instead of reviewed. Identity,
  tuning and the write-path switches moved into the file; the flags that duplicated them and the entire
  environment layer are gone. What stays on the command line is what locates the configuration, selects the
  tenant, or changes one invocation without changing what is produced. **Scripts and pipelines that passed
  the removed flags or exported `AZURE_RD_*` must move those values into a configuration file** — the
  README's configuration section lists where each one went. A run with no configuration at all still behaves
  exactly as before.
- **Per-tenant configuration profiles, selected by domain.** A config directory holds one `<domain>.yaml` per
  tenant plus an optional shared `base.yaml`, so switching tenants is `--config-dir <dir> --domain <domain>`
  instead of a long path or a private shell wrapper. The split between general and tenant-scoped settings is
  **enforced**: a key on the wrong side is a fatal error naming it, which is what keeps a per-tenant
  transformer or output root — each of which silently breaks comparability or the export layout — from being
  expressible at all. `--domain` completes from the profiles and the existing exports, so tab completion
  answers which tenants are configured. See the **Configuration** section of `README.md`.
- **A run now refuses when it cannot establish which tenant it acts on.** The declared `--domain` is intent
  and the signed-in tenant is ground truth; they are cross-checked, a mismatch aborts before anything is
  written, and `--domain` is accepted by every resource command rather than drift alone. The previous
  fallback — writing the export to the bare output directory when the tenant domain could not be resolved —
  is removed: that layout is invisible to every other command, so the export was lost from the moment it was
  written.

## [0.3.0] - 2026-09-28

### Added

#### Drift analysis

- **`docs analyze-drift` turns a drift observation into an impact analysis.** After `resource drift` has
  found changes, the new command renders a ready-to-paste prompt (`drift/analyze.md`) directing an LLM to
  judge each finding's impact — security posture, compliance, lifecycle and who is affected — and to write
  **one drift document per in-scope finding** (next bullet) beside the payload it judges (the payload's
  path with the extension swapped, so per resource the baseline YAML, observed YAML, documentation and
  judgment share one key and a frontend can render the YAML diff and the drift document side by side) plus
  a summary index at `drift/index.md` (findings ordered by severity with links, and the cross-resource
  security / compliance / lifecycle view). It is fully offline, verifies every payload against the
  observation's recorded hashes before directing an agent at it, and refuses when there is no observation
  or when the export was re-baselined after it (re-run `resource drift` in both cases). There are no
  per-type drift templates: the prompt uses each type's existing `doc-prompt.md` as the type-specific lens,
  so the feature moves no hash and forces no documentation regeneration. Prompt, drift documents and index
  live and die with the observation — the next drift run sweeps them, and a **re-baselining
  `resource download` now clears the `drift/` tree too**, since a new baseline supersedes the observation by
  definition. There is deliberately no drift history; archive them before re-baselining if they must be
  kept. See the **`docs analyze-drift`** section of `README.md`.
- **The drift analysis applies the documentation's scope, with an inventory for what it skips.** Findings on
  the two bulk directory types the documentation already excludes — autopilot device identities always,
  groups unless referenced by an assignment — are no longer analyzed per resource. The referenced-groups set
  is payload-aware: a group a drifted policy newly assigns counts as referenced and is analyzed in full.
  Out-of-scope added/removed findings become tool-fed inventory rows in the drift index (fixed `info`
  severity, no drift document); out-of-scope changed/renamed findings are only counted under the index's
  caveats. The observation itself keeps recording every finding — scope is a render-time decision, so
  revising it never requires re-running drift. **A custom `--prompt` template for `analyze-drift` must now
  carry the `inventory` marked block**; a template without it fails loudly (exit 2).
- **Generated prompts no longer carry their template headers.** `docs/generate.md` and `drift/analyze.md` are
  finished prompts, not templates: the explanatory header comment and the ` (template)` heading suffix are
  stripped at render time, so an agent never reads template-editing instructions. Marker comments stay, and a
  custom `--prompt` template without a header passes through unchanged.

#### Static analysis

- **This project can be analysed by a local SonarQube server.** `sonar-project.properties` describes it as its
  own Sonar project, analysed from this folder with the coverage report `make test-coverage` produces, and the
  server, the scan and the findings download are driven from the repository root. A tenant export under
  `output/` is excluded from the analysis: it is customer configuration and must never be uploaded to a
  code-quality server. The whole setup is optional and gates nothing — see the **Static analysis** section of
  the [repository README](../README.md).

### Changed

#### Static analysis

- **`.golangci.yml` now also mirrors the Sonar quality profile, so a Sonar finding shows up in the editor
  instead of waiting for a scan.** `gocognit` is enabled at the profile's own cognitive-complexity threshold
  rather than the linter's default, which was measured to report every function the server reports plus a few
  whose scores straddle the limit — the linter can no longer let a server finding through. golangci-lint's
  output truncation is switched off for the same reason: with the defaults it showed a strict subset of what the
  server reports, so the two could not be compared at all. `goconst` is deliberately **not** enabled; the file
  records why it cannot agree with the corresponding Sonar rule. Two shapes the metric misreads are excluded and
  mirrored on the server so the two tools keep agreeing: table-driven tests, and the Microsoft Graph handlers,
  whose score belongs to the paging pattern all of them share rather than to the individual handler. **The code
  that predates the rule is baselined rather than exempted**: the functions that already exceeded the threshold
  are named one by one, so `make lint-check` — and with it `make check`, `make ci` and `make branch-ready` —
  passes, while **every new function has to comply**; a second complex function in a listed file is still
  reported, which excluding the file would have hidden. The server keeps reporting all of them, so the deferred
  work stays counted where it is visible, and clearing it is tracked in `NEXT-ITERATIONS.md`. Nothing about what
  the tool does changed.

## [0.2.0] - 2026-09-18

### Breaking

- **The resource-facing commands moved under one `resource` noun.** `azure-rd download` is now
  `azure-rd resource download` and `azure-rd list` is now `azure-rd resource list`; the old spellings are gone
  and are not kept as aliases, so **any script, CI job or alias invoking them must be updated**. The surface now
  reads consistently with the existing `docs` group, and the flags the resource commands share are declared once
  on the group rather than per command, so a later resource verb inherits one definition instead of restating it.
  Nothing about `download` — what it does, which flags it accepts or what it writes — changed in that move;
  `resource list` is repurposed separately (next bullet). Configuration and `AZURE_RD_*` overrides are
  unaffected.
- **`resource list` now lists the tenant, not the tool.** The supported-type map moved to the new
  `resource types`; `resource list` keeps its spelling and changes its meaning to what the tenant actually
  contains. This is the more dangerous break of the two: a caller that kept working after the regrouping now
  gets the tenant's resources where it used to get the supported types — no error, just different output — and
  it now requires a signed-in session where the old command worked offline. **Anything invoking
  `azure-rd resource list` (or the pre-grouping `azure-rd list`) for the supported types must switch to
  `azure-rd resource types`.**

### Added

#### Commands 

- **`azure-rd resource types` — the map of what this build supports, with tenant counts when a session allows.**
  Prints every registered resource type with the handler that implements it and the API it speaks, grouped by
  API surface; this half is a property of the binary and works offline, with no subscription and no sign-in.
  Whenever a usable session happens to be available the map gains per-type tenant counts, rolled up from the
  same listing a download performs — whether they appear is decided by the session, never by a flag, and the
  session is probed **without ever prompting**: the device-code credential is built with automatic
  authentication disabled, so a sign-in that would require interaction reads as “no session” and the command
  degrades to the offline map with a note. The output always states which of the two it printed; a type whose
  listing was refused is counted as unknown — never 0 — and unknown or omitted counts never fail the command.
- **`azure-rd resource list` — what the tenant contains, per resource, without downloading.** Enumerates the
  selected types (all registered types when unselected, exactly as a full download would) through the same
  listing path a download uses to build its fetch requests, so the two can never disagree about scope. Listing
  yields ids; when an export for the tenant exists, display names recorded in its `resources/metadata.yaml` are
  joined in and resources the export does not know yet are marked new — nothing is fetched merely to prettify
  the listing. Unlistable types are reported as unknown, never as empty, and do not fail the command.
- **`azure-rd resource drift` — has the tenant changed since the last download?** Fetches the tenant's current
  state through the download's own listing, fetch and transform stages, compares it against the export on disk
  and records what was added, changed, renamed or removed — without re-baselining anything. The export is the
  baseline and stays untouched; the only output is the `<tenant>/drift/` tree (an observation record plus the
  fetched bytes of drifted resources at paths mirroring `resources/`), cleared and rebuilt on every run.
  Comparability is a precondition: the command refuses rather than report noise when the export's recorded
  transform or filter configuration differs from the run's, reports entries written under another configuration
  as unattested instead of drifted, and asserts removals only under the same complete-and-covered rule as
  `--prune`. Changed resources additionally get dotted-path old → new field deltas, the report names how many
  documents the drift will make stale once re-baselined, and an opt-in flag turns found drift into a distinct
  exit code for CI. `--dry-run` still fetches and reports in full — nothing about drift is answerable offline —
  and only withholds the `drift/` tree. See the README's `resource drift` section for usage.

#### Downloading a tenant's configuration

- **Ctrl+C now stops a run cleanly, and the export metadata can never be left half-written.** An interrupt
  cancels the run through the pipeline's normal cancellation path: every request is still accounted for, the
  summary reports the cancellations and the run is recorded as incomplete — so an interrupted run can never
  mark a resource absent or feed `--prune`. A second Ctrl+C force-quits. `resources/metadata.yaml` — the
  baseline every later comparison trusts — is now replaced atomically (temp file, then rename), so a run
  killed at any moment leaves either the previous file or the new one, never a truncated mix.

#### Release Workflow

- **The linter set is now committed, so the editor and the command line agree.** A `.golangci.yml` pins which
  linters run instead of leaving it to whichever golangci-lint version happens to be installed, and GoLand can
  be pointed at the same file, so an IDE warning and a `make lint-check` finding are the same thing. Beyond the
  default set it enables only checks that mirror an inspection GoLand has on by default, each annotated with
  the one it mirrors; checks that would be stricter than the IDE are deliberately left out. `make lint` and
  `make lint-check` validate the file before linting, so a typo in it fails the run rather than silently
  reverting to the defaults. How to point the IDE at it is documented in `README.md`.
- **`make branch-ready` reports whether a feature or fix branch is ready to ship.** It runs `make ci`, then checks
  that the work is recorded under `[Unreleased]` and that the `NEXT-ITERATIONS.md` entries the branch delivered
  have been cleared out and the rest renumbered — struck-out entries now stay in place while a branch is in
  progress and are deleted when it closes, so a reviewer can see what the branch set out to do beside what it
  did. Like the release report it edits nothing, but it reports every check and exits non-zero if any of them
  failed, so it can gate a merge. It refuses to run while this folder has uncommitted changes, so the verdict
  describes the commit that will be merged; that read-only, folder-scoped check is the only git either report
  runs. Also available as `make branch-ready-go` from the repository root; what it checks is documented in
  `README.md`.

### Changed

- **The selection flags (`--type`, `--resource-id`, `--resource-group`) and `--workers` moved from
  `resource download` onto the `resource` group**, now that every subcommand honours them: `types` narrows its
  map offline and its counts online, `list` selects what it enumerates, and `--workers` bounds the listing
  concurrency they all share. `--timeout` stays on `download` and `drift`, the commands that fetch individual
  resources. Invocations are unaffected — the flags follow the subcommand either way — as are configuration
  keys and `AZURE_RD_*` overrides.
- **The run preparation is shared code, not download-private.** Everything a resource-fetching command does
  before its real work — configuration reading, worker/transformer/filter construction, session verification,
  the dedicated-app probe and prompt, authentication, tenant and output resolution, the registry and the fetch
  requests — moved into one shared preparation that `download` and `drift` both run, so the two cannot diverge
  in authentication or selection semantics. No user-visible behaviour changed in the move.
- **An explicit `--workers` now also bounds the per-type listing concurrency.** Previously it only sized the
  per-resource fetch workers while listing always ran at the Microsoft Graph worker count; the one derivation
  is now shared by `download`, `list` and `types`, so the flag means the same thing wherever listing happens.
- **The tool now describes itself as what it became: tenant configuration export and documentation.** The
  "Azure Resource Downloader" expansion predates the documentation pipeline and misnames the content — nearly
  all exported types are Entra ID / Intune configuration, not ARM resources, and downloading is one verb among
  several. `--help`, the README title and the example config header now lead with the Entra ID / Intune tenant
  and the documentation half; the `azure-rd` binary name, module path and `AZURE_RD_*` prefix are unchanged.
- **The export now attests the configuration that produced each resource's bytes.** Every entry in
  `resources/metadata.yaml` records the transform-config hash of the run that wrote it, and the run records a
  hash of its resource-filter configuration. Partial runs merge, so the run-level hash alone cannot attest a
  mixed baseline; the per-entry fact is what lets a drift check trust exactly the comparable entries and report
  the rest as unattested instead of drifted. **No re-download is required**: entries written by earlier versions
  simply carry no attestation yet and backfill as resources are rewritten.

### Fixed

#### Downloading a tenant's configuration

- **A download without an `az login` session now fails immediately, naming the cause.** Previously a missing
  session never failed loudly: every step degrades deliberately for permission-poor identities (subscription and
  tenant resolution warn and continue, unlistable types are skipped one by one), so a full authentication
  failure compounded into warnings ending in "No resources to download" — after prompting for an app
  registration. The download now verifies the Azure CLI session before anything else, including that prompt, and
  stops with a clear `run 'az login' first` error; explicit `--client-id`/`--tenant-id` skips the check, since
  the device-code sign-in needs no CLI session.

#### Command-line surface and configuration

- **Errors now surface once, through one path, with the right exit code.** Commands return errors instead of
  exiting mid-run, so deferred cleanup always runs and the `docs` subcommands' distinct exit codes ("cannot
  answer", "stale found") are preserved end-to-end; previously a failure could be printed twice — once by the
  command framework and once by the tool. Usage is still shown for an invocation mistake, but no longer for a
  runtime failure, and the `--exit-code` gate of `docs generate-prompt` now prints one line naming why it exits
  non-zero instead of exiting silently. The `--workers` help text now states that the per-API defaults apply
  when the flag is not set, and a latent trap was closed where the flag's default could silently overwrite the
  general worker default from configuration.

#### Release Workflow

- **`make ci` no longer runs `go mod download` and `go mod tidy` first.** Doing so before every lint defeated the
  lint cache, so `make ci` — and `make release-ready`, which runs it — took minutes; it now runs `check` and
  `build` only and finishes in seconds. Refreshing dependencies stays an explicit `make deps`.
- **`make check` no longer rewrites files.** It ran `go fmt`, so a readiness report could describe a working
  tree that `check` itself had just changed. Formatting and linting now each come as a pair: `make fmt` and
  `make lint` fix what they can (`lint` now runs `golangci-lint --fix`), while the new `make fmt-check` and
  `make lint-check` only report and fail. `check` — and therefore `ci` and `release-ready` — runs the `-check`
  variants, so it leaves the working tree exactly as it found it.

## [0.1.0] - 2026-09-07

### Added

#### Release Workflow

- **`make release-ready` reports whether a release can be cut.** It runs the full `make ci` pipeline first, then
  looks at this changelog: when `[Unreleased]` has not been closed into a new, still undated version section it
  reports that no release is needed and succeeds; otherwise it checks that `NEXT-ITERATIONS.md` has no
  struck-out entries (work that shipped but was never recorded here) and that `[Unreleased]` is empty. Every
  check is run and reported with what to do about it; the goal only fails when all of them fail. It edits
  nothing and runs no git command: closing the changelog is done by hand, and branch and working-tree checks,
  stamping the release date, tagging and the GitHub release are a separate step at the repository root that runs
  this report first and acts on every project whose newest heading is undated, so the CLI and the browser stay
  independently versioned. Procedure in the monorepo README.

#### Downloading a tenant's configuration

- **`download` streams a tenant's configuration through a three-stage async pipeline.** Fetch, transform and
  write run concurrently over channels, each backed by its own worker pool, so a resource is written as soon
  as it is fetched rather than in batches. Concurrency defaults per API because ARM and Microsoft Graph
  throttle differently, and transient failures are retried with exponential backoff. One failing resource
  never stops the others, and a permission error is not a failure: a missing ARM role or Graph scope warns and
  skips, so a partially privileged operator still gets everything they are allowed to read.

- **53 resource types across ARM and Microsoft Graph, behind one handler interface.** Three ARM types and 50
  Entra and Intune types — conditional access, configuration and compliance policies, apps and app protection,
  Autopilot, enrollment, updates, scripts, groups and more. Adding a type means implementing the handler
  interface and registering it; the README lists every type with the permissions it needs.

- **Selection by resource id, type or resource group, and property filters per type.** The selection flags are
  repeatable, and with none of them given every registered type is downloaded. A config-only `filters:` block
  narrows a type further by matching regexes against properties of the raw Azure response, evaluated before
  transformation so they see the original property names; excluded resources are reported separately and do
  not affect the exit code.

- **Clean YAML through four transformers in a fixed pipeline.** Whatever order a config lists them in, they
  run as cleaning, id-resolution, base64-decode, name-sanitization; listing a transformer enables it and
  omitting it disables it, so an empty list yields raw Azure data. Decoding matters because Intune hides real
  configuration in encoded payloads — a macOS `.mobileconfig` profile, the per-setting XML of a Windows custom
  profile — which is otherwise unreadable in the export. `--resolve-secrets` additionally resolves masked
  Windows OMA-URI secrets; it is **off by default and writes secrets to disk in plaintext**.

- **Delegated user authentication, with a device-code fallback for scopes the Azure CLI app cannot get.** By
  default the run reuses the existing `az login` session, and the same token serves both ARM and Microsoft
  Graph. Passing a client and tenant id switches to a device-code flow against a dedicated app registration,
  the only way to obtain the Graph scopes the first-party CLI application does not carry. App-only
  service-principal credentials are deliberately **not** supported: the Intune backend rejects them for secret
  resolution, and delegated auth keeps every read attributable to a person. `--subscription` is optional —
  with no subscription at all the Graph types still download and the ARM types are skipped.

- **Interactive dedicated-app sign-in.** When selected types need Graph permissions the Azure CLI app cannot
  provide, the run reports which types require it and prompts for the app registration's client and tenant id.
  Non-interactive runs fall back to the configured values and fail fast when one is missing.

- **Run completeness, and the accounting invariant behind it.** A run is complete only when every request
  produced a result, nothing was cancelled and every selected type listed successfully; the verdict is
  reported and recorded in the export metadata. What makes it trustworthy is that every request produces
  exactly one result — a stage that stops early still emits one cancelled result per remaining item, and the
  pipeline fails loudly if the counts disagree. A request that produced no result is indistinguishable from a
  resource deleted in the tenant, so without this a timeout could make `--prune` delete live resources.

- **`--timeout` is a per-operation deadline**, applied around each individual resource fetch including its
  retries rather than around the whole pipeline, so a large tenant cannot exhaust it mid-run.

- **Exports are reproducible: an unchanged tenant re-exports byte-for-byte identically.** Three sources of
  run-to-run noise are eliminated. Resources of one type that share a display name — several default Intune
  enrollment configurations are all *"All users and all devices"* — used to overwrite each other on disk and
  collapse into a single metadata entry, with the survivor varying per run; names are now assigned in a
  deterministic order, the lowest resource id keeping the bare name while every collision gets a stable
  suffix, and each disambiguation is logged. All-string arrays are sorted, since Graph returns some
  multivalued attributes in a different order per read. And server-regenerated identifiers are dropped from
  the export instead of handing a resource a new hash on every run. Two runs of an unchanged tenant now
  produce identical YAML and prompt files, and metadata that differs only in its timestamps.

- **Everything the tool writes lives under one `resources/` tree per tenant** — `output/<domain>/resources/`,
  where `<domain>` is the tenant's Entra default domain resolved from the signed-in session, so several
  tenants can share an output directory without colliding. The boundary below it is by owner: `resources/` is
  the only tree the tool writes to, which is what makes `--prune` safe to reason about, while the sibling
  `docs/` tree belongs to the documentation run. An export produced during the release candidates needs its
  type directories moved under `resources/`, or regenerating.

#### Export metadata and pruning

- **Export metadata (`resources/metadata.yaml`).** Every download records a description of the export: per
  resource its content hash, id, display name, platforms, sidecar artifacts and assignment targets; per type
  the hash of its documentation prompt and when it was last covered; per run the scope, completeness verdict
  and tool version. It holds **facts only** — no grouping, classification or change buckets. Those depend on
  rules rather than on the tenant, so they belong to a later step and can be revised without re-downloading
  anything.

- **`download --prune`, with a `--dry-run` preview.** Deletes exported files for resources the run proves are
  no longer in the tenant, so an export stops accumulating. It runs after the download, on exactly what the
  metadata merge marked absent, and refuses on an incomplete run or any fetch failure; it only touches types
  whose listing succeeded, never leaves `resources/`, never removes the metadata file, and logs every
  deletion. The dry run lists exactly what a real prune would delete, sharing one eligibility decision with
  the real path so the preview cannot drift from the outcome.

#### Command-line surface and configuration

- **Flags follow the subcommand, and a command only advertises the flags it uses.** Commands opt into the
  auth, selection and pipeline groups they actually read instead of inheriting every persistent flag, so it is
  `azure-rd download --type X`, not `azure-rd --type X download`; only `--config`, `--output`, `--dry-run` and
  `--log-level` are global. An explicitly passed `--workers` is honoured rather than being discarded in favour
  of the API-specific default.

- **`config.example.yaml` is the reference schema, and loading it unmodified is a true no-op.** Every option
  is present at its built-in default rather than commented out, so the file doubles as a reference for what
  the defaults are, and a run that loads it produces the same files **and the same recorded hashes** as one
  that does not. Options with no usable default, or that are dangerous to enable, are illustrated in comments
  instead. Configuration is documented in the README; precedence is flag, environment variable, config file,
  default.

- **`list` works offline.** The supported resource types can be listed without authenticating, and the command
  does not accept the selection flags it would silently ignore.

- **`azure-rd --debug` prints a diagnostic report of the current Azure session.** Run with no subcommand, it
  authenticates exactly as `download` would and reports how it is authenticated, the signed-in identity, the
  resolved tenant, subscription and output directory, and how many handlers are registered — so you can
  confirm which identity and target a download would use. It writes nothing.

- **The version is injected at build time, from this project's own release tags.** `--version` and the tool
  version recorded in the export metadata track the binary. In this monorepo the tag lookup is restricted to
  this project's tag line: the newest tag of *any* project would otherwise be stamped into every export,
  attributing it to a version of the CLI that never existed.

#### Per-type documentation prompts

- **Per-type documentation prompts, from a family of seven templates, written by default.** Each covered type
  gets a `doc-prompt.md` alongside its resources: the instructions an LLM needs to document *that* type. There
  are seven templates because the policy-with-settings-and-assignments framing of the default one suits
  neither a tenant-wide singleton, a credential whose story is expiry and renewal, an inventory record with no
  settings, a supporting object other policies point at by id, a group, nor an ARM resource. Pass
  `--no-prompt` to skip writing them.

- **The prompts declare a closed, machine-readable set of section headings.** A described but non-binding
  section list is not enough: in real exports the model invented extra headings, which breaks a browser that
  styles and deep-links sections by their slug. Every template now fixes its headings as a contract to be
  written verbatim and records them in a marker the generation prompt validates each document against, with
  settings blocks carrying a deep-link target for their exact property path. The tenant summary declares the
  same contract for its own headings and a severity-ranked findings table. **Documents written before this
  carry a different prompt hash for every type, so one full regeneration is required.**

- **Documents declare their platform and function group in frontmatter.** The taxonomy resolved at index time
  answers which initiative a resource belongs to; these two axes add the complementary per-document judgement
  of which platform it targets and what management function it performs — the one classification only the
  document generation can make. Each is a single value from a closed vocabulary carried in the type's prompt,
  validated during generation and harvested into the index, with an uncategorised count reported so an axis
  the model stops filling is visible. Inventory record types opt out, keeping their prompt — and their
  documents — unchanged.

- **Per-resource documents never reprint credential-shaped secrets found in free-text.** Real tenant runs
  surfaced unmasked secrets in a description and a decoded payload being copied verbatim into the generated
  documents, widening exposure from Intune readers to anyone who can read the docs. Every property-documenting
  template now redacts such a value, still documenting the field and flagging it as a credential to rotate;
  the literal value remains only in the source YAML.

#### Incremental documentation (`docs generate-prompt`)

- **`docs generate-prompt` command.** Turns an export into a single, ready-to-paste prompt covering exactly
  the resources whose documentation is missing or out of date. It **never fetches a resource** and never
  writes into `resources/`: it reads the export metadata and the existing documents, then writes one file,
  `docs/generate.md`. A document is listed when it is missing, its resource changed, its type's prompt
  changed, or its frontmatter is unreadable; documents for resources gone from the tenant are reported as
  orphans and never deleted. Tenant resolution mirrors `download`, with an offline mode that skips sign-in,
  and an optional exit code makes a clean CI run mean every document on disk matches the export.

- **A renamed group re-splices one block instead of regenerating a page.** Assignments are resolved once and
  hashed in both directions — forward per resource, reverse per referenced group — so a document whose
  assignment facts changed is listed for a **re-splice** of its marked block rather than a rewrite, and a
  document predating the markers is listed for a one-time **migrate**. The prompt carries a reference map
  resolving each group id to its name, document and kind, so assignment tables are rendered from given facts
  without re-reading each group's YAML, and assignment targets with no export entry are flagged as dangling.

- **The incremental prompt is hash-complete, and its own checks can fail loudly.** Freshly generated documents
  are stamped with every block hash by the orchestrator rather than by the fan-out agent, which only ever sees
  its own chunk — previously they landed without them and were re-spliced on every future run, for ever.
  Coverage is checked against a set the tool emits rather than against the orchestrator's own chunks, so a
  chunking mistake that drops a whole type fails instead of passing, and the freshness baseline is recorded
  only once the checks pass, so a partial run cannot poison it.

- **Notification message templates are cross-referenced with the compliance policies that use them, in both
  directions.** A template's document gains a *Used by* block listing the policies that reference it, and a
  policy's document names the template it notifies through and re-renders that reference when the template is
  renamed, without regenerating the page. **The reference is a new recorded fact, so existing exports must be
  re-downloaded to populate it, and the two policy types' documents regenerate once to pick up the marker.**

- **`notificationMessageTemplates` export their actual content, and export it stably.** A plain read omits the
  per-locale subject and body that are the template's real payload, so each template is fetched with them
  expanded. The API stamps those messages with the response time rather than the tenant's modification time,
  which would hand every template a new content hash on every run, so that volatile field is stripped before
  serialization.

- **The prompt has the agent write a tenant summary (`docs/summary.md`)**, the narrative overview of the
  tenant's management posture that the documentation frontend renders as its landing page. Because an
  incremental run's work list is only the changed documents, the summary is built from a fact block the tool
  computes from the export metadata — per-type counts, platforms, assignment posture, coverage caveats — so a
  no-op run and a full run yield the same page. Findings and recommendations may draw only on those facts and
  a bounded signal sweep; deeper per-resource analysis stays in the individual documents.

- **Documentation runs persist their report to disk.** Each run writes a timestamped report next to the tenant
  summary — one file per run, never overwritten — and reproduces the work list it set out to do, so the report
  stands alone as the record of what happened rather than only being printed.

#### Documentation index (`docs generate-index`)

- **`docs generate-index` command.** Emits `docs/index.yaml`, the machine-readable navigation index the
  documentation frontend reads to render a tenant, replacing a generated `index.md`. Like `generate-prompt` it
  never fetches a resource and never writes into `resources/`, and it can run offline. The index is derived
  from the export metadata and enriched with each document's frontmatter; an in-scope resource with no
  document yet is listed as pending so counts stay honest, and bulk types are reported as counts only. It
  shares the in-scope rule and assignment resolution with `generate-prompt`, so the index can never describe a
  different set of resources than the prompt documents, and it carries no wall-clock time, so re-running over
  an unchanged export is byte-identical.

- **`docs generate-index` classifies resources along multiple independent facet axes.** A `taxonomy:` config
  section declares axes — *Programme*, *Platform*, *Environment*, whatever fits the tenant — each with an
  ordered list of values matched against facts the export already carries, so a tenant can be sliced several
  ways at once. The index carries the axis registry in display order plus each resource's memberships, which
  makes it the single grouping surface both the docs browser and the Confluence export read, so the two can
  never classify differently. Resources matching nothing on an axis are reported as uncategorised for that
  axis. The taxonomy records rules, never facts, so revising it never requires re-downloading a tenant.

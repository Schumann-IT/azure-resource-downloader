# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

This project is released independently of the documentation browser in `web/`: its releases are tagged
`go/vX.Y.Z` in the monorepo and the versions below are its own, unrelated to `web/`'s. See the **Releasing**
section of the [repository README](../README.md) for the procedure.

## [Unreleased]

### Added

#### Release Workflow

- **`make branch-ready` reports whether a feature or fix branch is ready to ship.** It runs `make ci`, then checks
  that the work is recorded under `[Unreleased]` and that the `NEXT-ITERATIONS.md` entries the branch delivered
  have been cleared out and the rest renumbered — struck-out entries now stay in place while a branch is in
  progress and are deleted when it closes, so a reviewer can see what the branch set out to do beside what it
  did. Like the release report it edits nothing, but it reports every check and exits non-zero if any of them
  failed, so it can gate a merge. It refuses to run while this folder has uncommitted changes, so the verdict
  describes the commit that will be merged; that read-only, folder-scoped check is the only git either report
  runs. Also available as `make branch-ready-go` from the repository root; what it checks is documented in
  `README.md`.

### Fixed

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

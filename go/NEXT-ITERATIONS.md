# Next iterations — deliberately out of scope

Outstanding work and parked ideas for the Go CLI. Each numbered entry is a unit of planned work; **once its
plan ships in full, the entry is removed** and its history lives in `CHANGELOG.md`. Ideas that are
deliberately not scheduled collect under *Parked ideas* at the end, so they persist as the entries around them
ship. `README.md` stays the single source of truth for what the tool *does today*.

## 1. ~~Make configuration the single source of truth, with per-tenant profiles selected by domain~~

**Goal.** Give every setting exactly one home, and make switching between tenants a single visible argument.
Today a value can arrive from a flag, an `AZURE_RD_*` variable or a config file, and the tenant-scoped ones
(`client-id`, `tenant-id`, `subscription`, the selection filters) are spread across all three — so nothing
prevents one tenant's app registration being paired with another tenant's export, and working across several
tenants means long command lines or a private shell wrapper. Reduce the command line to the few flags that
genuinely belong there, move everything else into the config file, split that file into a shared **base** and
per-tenant **profiles** selected by `--domain`, and delete the environment layer entirely.

> **The option model.** Every option lands in exactly one of four places, and the rule for which is
> statable rather than case-by-case:
>
> - **Bootstrap flags** — `--config`, `--config-dir`. Not configurable, because they say where the
>   configuration is.
> - **Selector flag** — `--domain`. Not a setting: the thing that picks which profile applies.
> - **Invocation flags, absent from the config file** — `--dry-run`, `--log-level`, `--debug`, plus the three
>   whose meaning is **per command**: `--out`, `--prompt` and `--exit-code`. Viper's namespace is flat, so a
>   single `out:` key cannot carry three different defaults (`docs/generate.md`, `docs/index.yaml`,
>   `drift/analyze.md`), `prompt:` cannot name three different templates, and `exit-code:` cannot mean both
>   "drift was found" and "documentation is stale". They stay on the command line, where each command gives
>   them one unambiguous meaning. `--resource-id` and `--resource-group` are also invocation flags with **no
>   config key at all**: they are ad-hoc selection ("re-download this one resource"), and a persistent
>   profile entry would silently scope every future run of that tenant — the stale-entry footgun outweighs
>   the partition purity, and the wrong-tenant risk is minimal since an ARM resource id embeds its
>   subscription and fails as not-found. `dry-run` and `log-level` are **removed from `config.example.yaml`**
>   so every option keeps exactly one home.
> - **Config-backed options.** Everything else. Two keep an invocation override — `--output` (redirect a run
>   elsewhere to compare; it changes *where* bytes land, never *which*) and `--type` (scope one run without
>   editing a file, the common ad-hoc case). The flag **replaces** the config value wholesale for that run —
>   never an intersection with it, which could silently select nothing — exactly the flag-beats-config
>   semantics both have today. The rest are
>   config-only: `workers`, `workers-by-api`, `timeout`, `resolve-secrets`, `no-prompt`, `prune`,
>   `transformers`, `taxonomy`, plus the tenant-scoped set below.
>
> **Moving `--prune` and `--resolve-secrets` into the file is a safety improvement, not tidying.** They are
> the tool's only destructive switch and its only write-plaintext-secrets switch; requiring them to be
> written down — reviewable, diffable, attributable — rather than typed ad-hoc is strictly better.
>
> **The base/profile partition is enforced, not documented.** Config-backed options split in two:
>
> - **Tenant-scoped — profile only**: `tenant-id`, `client-id`, `subscription`, `filters`, and the audit
>   workspace once that entry lands. These name things that exist inside one tenant, so a value from one
>   tenant is meaningless or harmful in another.
> - **General — base only**: `output`, `type`, `transformers`, `taxonomy`, `workers`, `workers-by-api`,
>   `timeout`, `resolve-secrets`, `no-prompt`, `prune`.
>
> A key on the wrong side is a **fatal error naming the offender**, in both directions.
>
> **Three placements carry real risk and are why this is enforced rather than advised.** `transformers` is
> hashed into `transformConfigSha256`: a per-tenant override silently makes an export non-comparable with
> its own baseline and with every other tenant, the failure this codebase is least able to detect. `output`
> is the export root and the tenant is already a subdirectory of it (`<output>/<tenant>/`), so a per-profile
> root breaks the layout every `docs` command and `detectSingleExportDomain` assume. `filters` sits on the
> tenant side for the mirror-image reason: it is hashed into `filtersSha256`, which gates drift
> comparability, so it must be stable *per tenant* — in a shared file, editing one tenant's naming
> convention invalidates every tenant's baseline at once.
>
> **The environment layer goes away completely.** `viper.AutomaticEnv`, `SetEnvPrefix`, the
> `SetEnvKeyReplacer` and every `AZURE_RD_*` variable are removed; the rule text in `.windsurf/rules/` that
> currently forbids removing the replacer is deleted as part of this change rather than violated by it.
> `LOG_LEVEL` is unaffected — the logger reads it directly, not through viper. Precedence collapses from
> four layers to two: flag > config > default for the two config-backed flags, config > default for
> everything else. **`cmdutil.BindFlags` disappears with it**, and with it the standing requirement to call
> it as the first statement of every `RunE` — a rule that can no longer be forgotten because there is
> nothing to call. The cost is real and belongs in the migration note: a CI pipeline can no longer inject
> configuration through the environment and must write a config file.
>
> **This is a breaking change to the command-line surface**, in two respects — flags removed, and
> environment variables no longer read. It belongs under `Breaking` in `CHANGELOG.md` and drives the next
> major. The zero-config path must keep working exactly as it does now: `azure-rd resource download` with no
> file, no flags and an `az login` session still performs a full export with the built-in defaults.
>
> **The chicken-and-egg that shapes the profile design.** Config must be read *before* authentication,
> because it supplies the credentials used to authenticate — but for `resource download` the tenant domain
> is only known *after* authentication resolves it. So a profile can only ever be selected by an
> **explicitly passed** `--domain`, never by the auth-resolved tenant. That is a statable rule
> (`--config-dir` requires `--domain`), and it means the commands that lack `--domain` today must gain it —
> including root, whose `--debug` report must be able to show the profile a run would use.
>
> **A missing profile is fatal, never a fallback.** `--config-dir` with a domain that has no file must fail
> the way a mistyped `--config` already does. Falling back to defaults would run with the wrong (empty)
> configuration while looking like it worked.
>
> **Implement this before the drift-attribution work** (the entry that adds per-tenant Log Analytics audit
> attribution): that entry's workspace setting is tenant-scoped and is far simpler to express once profiles
> exist — a plain key in the tenant's own file rather than a domain-keyed map inside a shared one.

**Plan.**

*The flag surface (breaking)*

- ~~Reduce the command line to the model above: keep `--config`, `--config-dir`, `--domain`, `--dry-run`,
  `--log-level`, `--debug`, `--output`, `--out`, `--prompt`, `--exit-code`, `--type`, `--resource-id` and
  `--resource-group`; remove `--subscription`, `--client-id`, `--tenant-id`, `--workers`, `--timeout`,
  `--resolve-secrets`, `--no-prompt` and `--prune`. Drop the `resource-id` and `resource-group` **config
  keys** in the same pass — they become flag-only.~~
- ~~Delete what the removal makes dead: `cmdutil.AddAzureAuthFlags` / `AddPersistentAzureAuthFlags`,
  `AddWorkersFlag` / `AddPersistentWorkersFlag`, `AddTimeoutFlag` and `RequireAuthFlagPair`
  (`AddSelectionFlags` survives whole — all three selection flags stay). The client-id/tenant-id pairing
  becomes profile validation, which is the only place it can be checked against the tenant it belongs to.~~
- ~~Bind only the two config-backed flags explicitly (`viper.BindPFlag` for `output` and `type`), and delete
  `cmdutil.BindFlags` along with every `BindFlags(cmd)` call in a `RunE`. The remaining flags are read from
  the command's own flag set, never through viper — which is what stops a sibling command's identically
  named flag leaking through the global singleton, the defect `BindFlags` existed to work around.~~
- ~~Replace the "was it set explicitly" checks that `cmd.Flags().Changed(...)` provided for removed flags with
  `viper.IsSet(...)`. `--workers` is the one that matters: the per-API defaults apply unless the operator
  set a count deliberately (`WorkersExplicit`), so losing that distinction would silently override the
  Microsoft Graph and ARM defaults with a single number.~~
- ~~Keep the interactive dedicated-app prompt working when a selected type needs one and no profile supplies
  it, and have it print the **profile snippet to save** rather than only the values, so the answer lands in
  the single source of truth instead of a shell history entry.~~

*Removing the environment layer (breaking)*

- ~~Remove `viper.SetEnvPrefix`, `viper.AutomaticEnv` and `viper.SetEnvKeyReplacer` from `initConfig`, and
  delete every `AZURE_RD_*` reference from `config.example.yaml`, `README.md` and the rule files — including
  the invariant that currently forbids removing the replacer, which this change retires.~~
- ~~Leave `LOG_LEVEL` alone: it is read directly by the logger and is not part of the viper surface.~~

*Profiles and layering*

- ~~Add `--config-dir` as a root persistent flag beside `--config`. It is the fifth global flag and needs its
  justification recorded: it is part of the config-loading mechanism root already owns, and it must be
  resolved in `initConfig` before any command runs, so it cannot live on a group.~~
- ~~Resolve and layer in `initConfig`. The base is `<config-dir>/base.yaml` **by convention** when the
  directory holds one, or the file named by `--config`, which wins when both are present. Then read the leaf
  command's explicitly passed `--domain` (root's hook already receives the leaf command, and flags are
  parsed by the time it runs), join it to the config directory as `<domain>.yaml`, and merge it over the
  base with `MergeInConfig`. Log both files and the domain that selected the profile, extending the existing
  "Using config file" line. The convention exists so the everyday invocation is two flags
  (`--config-dir ~/.azure-rd --domain cb-gmbh.com`) rather than three; with the environment layer gone there
  is no way to set the directory once per shell, so this is what keeps the command line short.~~
- ~~Exclude `base.yaml` from profile resolution and from the `--domain` completion candidates. A real Entra
  default domain always contains a dot, so the name cannot collide with a tenant, but the exclusion should
  be explicit rather than incidental.~~
- ~~Do **not** default the config directory (e.g. to `~/.azure-rd`). Auto-discovery was deliberately removed
  from this tool once already; a run must state which configuration it used.~~
- ~~Define the key partition in **one** exported table (tenant-scoped vs general), and validate both files
  against it: a general key in a profile and a tenant-scoped key in a base file are each a fatal error
  naming the key and the file. That table is the single truth the documentation, the stub file and the tests
  all read from — never a second hand-maintained list. Validate each file **before** merging: after
  `MergeInConfig` the merged state no longer records which file a key came from, so validation on the merge
  result cannot name the offender — or catch a key present legally in one file and illegally in the other.~~
- ~~Validate the domain as a **single path segment** before joining it — no separator, no `..`, not empty — so
  a flag value can never escape the config directory.~~
- ~~Fail loudly and helpfully when the profile is absent: name the path that was looked for and list the
  `*.yaml` files the directory does contain, which doubles as the answer to "which tenants do I have?".~~
- ~~Fail when `--config-dir` is given without `--domain`, stating the reason (the tenant is not known until
  after authentication, and the profile supplies the credentials that authentication needs).~~
- ~~Ship `config.example.domain.yaml`: a commented stub listing **every** tenant-scoped key
  (`tenant-id`, `client-id`, `subscription`, `filters`) with empty values, carrying the same no-op promise
  `config.example.yaml` does — copying it to `<config-dir>/<domain>.yaml` unmodified must behave exactly
  like having no profile.~~
- ~~Rework `config.example.yaml` in the same change: drop the tenant-scoped keys (`subscription`, `client-id`,
  `tenant-id`, `filters`) and the now flag-only ones (`dry-run`, `log-level`, `resource-id`,
  `resource-group`), add the options that lost their flags, replace every "Equivalent to --x / AZURE_RD_X"
  note with the new home, and point at the domain stub. The no-op promise still holds and gets easier to
  verify.~~
- ~~Ship `workers` **commented out** in the example, unlike today's active `workers: 5`. It is now the one
  option whose mere presence changes behaviour — setting it is the explicit choice that overrides the
  per-API defaults (Graph 5, ARM 20) — so an active value in the example would silently opt every copier out
  of ARM's 20. This is a real difference from the flag, which had to be passed to count as explicit.~~

*Tenant resolution cleanup*

- ~~Replace the three divergent policies with one shared resolver: `runprep.Prepare` resolves and on failure
  **keeps the flat base output path**, `cmd/docs`' `resolveExportDir` treats `--domain` as offline-or-detect,
  and `resource drift` does its own declared-vs-resolved cross-check. Only the third is right. One resolver
  returns the tenant directory, the domain to cross-check against, and an explicit state: *declared and
  verified*, *declared but unverifiable* (offline, or resolution failed), or *resolved only*.~~
- ~~Delete the flat fallback in `runprep`. Writing `<output>/resources/` in a layout where everything else
  looks for `<output>/<domain>/resources/` produces an export that `docs` and `detectSingleExportDomain` can
  never find again. With a declared domain, use it and mark it unverified; with neither, refuse — exactly as
  `resource drift` already does.~~
- ~~Keep the auth-resolved tenant as **validation, not as the source**: the declared domain is intent, the
  resolved domain is ground truth, and a mismatch aborts before anything is written. Removing resolution
  would let a typo silently create a second export directory for the same tenant, which nothing would ever
  notice.~~
- ~~Add `--domain` to `resource download`, `resource types` and `resource list` with those same semantics, and
  to root for the `--debug` report, so the resolver has one input everywhere and profiles are usable on a
  download.~~

*Discovery*

- ~~Register a completion function for `--domain` (`cmd.RegisterFlagCompletionFunc`, returning
  `ShellCompDirectiveNoFileComp`) offering the config directory's `*.yaml` basenames and the export
  directories under `--output`. It must stay local and cheap — never an Azure call on a Tab press — and must
  not assume config has been loaded, since the hidden `__complete` invocation is not guaranteed to run the
  hooks; read `--config-dir` and `--output` from the flag values directly.~~
- ~~Report the resolved selection in `azure-rd --debug`: config directory, domain, base file, profile file —
  beside the existing config-file line, so an operator can confirm a switch without running anything that
  writes.~~

*Verification and documentation*

- ~~Tests: the `base.yaml` convention, including `--config` overriding it and a directory without one;
  `base.yaml` excluded from profile resolution and completion; base+profile merge order and precedence (the
  surviving flags still beat both); partition validation
  rejecting a key on either wrong side; the missing-profile error including the candidate listing;
  path-segment validation rejecting separators and traversal; `--config-dir` without `--domain`; the stub
  file loading as a true no-op; the resolver's three states including the removed flat fallback; the
  `--domain` cross-check aborting a download against the wrong tenant; completion candidates from a fixture
  directory; `viper.IsSet`-based explicit-worker detection preserving the per-API defaults; and a test that
  a set `AZURE_RD_*` variable is **ignored**, so the env layer cannot creep back in unnoticed.~~
- ~~Invert the flag-surface tests in `cmd/resource_test.go` and `cmd/docs_test.go`: they currently assert the
  removed flags are present. They must assert the full surviving set and that every removed flag is **gone**
  from every command — the same both-directions check they already make, retargeted.~~
- ~~Pin the zero-config path with a test: no config file, no flags, defaults only must still resolve a full
  export as today **when tenant resolution succeeds**. The failure edge changes deliberately (refuse instead
  of writing flat — see the resolver cleanup), so the test pins the promise that actually survives. It is
  the one most at risk in a refactor this wide.~~
- ~~Documentation: a README section on working with several tenants (directory layout, base vs profile, the
  partition and why it is enforced, the `--domain` switch and its cross-check, installing shell completion),
  a configuration-reference section replacing the flag-centric one, and a migration table mapping every
  removed flag and `AZURE_RD_*` variable to its new home.~~
- ~~Update the rule files this change contradicts: the flag-placement and `cmdutil.BindFlags` requirements in
  `.windsurf/rules/03-commands.md`, and the config-precedence, env-variable and env-key-replacer invariants
  in `.windsurf/rules/04-security-and-ops.md`. Leaving them in place would keep describing a surface that no
  longer exists.~~
- ~~`CHANGELOG.md` under **`Breaking`**, stating in bold that existing scripts and pipelines must move the
  removed flags and environment variables into a config file.~~

## 2. Attribute each drift finding to an actor and a time, from the tenant's Log Analytics audit tables

**Goal.** Answer *who changed this, and when* for every drifted resource. A drift observation today says a
resource's bytes moved somewhere between the baseline and the observation; the change record that names the
actor already exists in the operator's Log Analytics workspace, keyed by the same object GUID the finding
carries. Join the two and record the result as a new, separate artifact at the drift tree root — so the
drift-analysis agent, and a human reading the tree, can tell a deliberate administrative change from an
unexplained one.

> **Why a separate `drift/audit.yaml` and not `drift/metadata.yaml`.** Three structural reasons, not
> presentation: (1) **provenance differs** — the observation is computed from bytes this run fetched and can be
> verified against them, while audit rows are copied from an external system with its own ingestion latency,
> retention and completeness, so folding them in makes the observation partly unverifiable; (2) it would
> **break determinism** — `findingsSha256` and the "identical bytes over an unchanged tenant except the
> timestamp" property would be lost, because a re-run sees whatever has been ingested since; (3) it **must be
> allowed to fail** — no workspace, no permission or a window past retention may not invalidate the
> observation, and a separate artifact can simply be absent. Lifecycle is free: the file sits at the drift tree
> root beside `metadata.yaml` and `analyze.md`, where no payload can be, so it is swept by the drift run's
> existing clear-and-rebuild and by a re-baselining `resource download`. **No new delete path.**
>
> **The join key and the window already exist.** `Finding.ResourceID` is the Graph object GUID the audit
> tables record as the target, and `Observation.Baseline.GeneratedAt` → `Observation.ObservedAt` is exactly the
> interval the verdict claims the change happened in — so the query needs no heuristic bounds.
>
> **Facts only, and the gaps are facts too.** Record the actor (UPN / application name), the activity, its
> result, the event timestamp and the correlation id — never a judgment such as "authorized" or "expected".
> Where the join cannot be made the entry says so explicitly with a distinct status, because *no event found*
> and *could not look* must never render as the same thing: singleton types (`organization`,
> `authorizationPolicy`, `deviceManagementSettings`, …) have no GUID target to join on; a window starting
> before the workspace's retention is *unknown*, not *unchanged*; ingestion latency means a change observed
> seconds ago may not be queryable yet; and several events in one window are a list, never a single "who".
>
> **Failure semantics (settled).** The lookup is enrichment and **never fails a run** — in either entry point
> it warns, records the per-finding status, writes whatever it could answer, and leaves the exit code to the
> drift comparison alone. This mirrors how a permission error skips a type instead of failing a download.
>
> **Not regeneration-gated.** It touches no `doc-prompt.md` and no per-type template, so no `promptSha256`
> moves and no documentation regeneration is forced. The drift-analysis template is not hashed either.
>
> **Coverage (settled).** `IntuneAuditLogs` for the Intune/device-management types plus `AuditLogs` (Entra
> directory audit) for conditional access, groups, named locations and the authentication policies. The three
> ARM types are out of scope for this entry and report their status as not queried; `AzureActivity` can be
> added later behind the same per-finding shape.
>
> **Where the workspace is configured (settled), and the ordering that follows.** The workspace is a
> **per-tenant fact**, like the tenant domain itself — not a per-invocation choice — so it is one plain,
> config-only key in **that tenant's own configuration file**. This entry therefore depends on the
> per-tenant configuration profiles planned ahead of it (a config directory of `<domain>.yaml` files selected
> by `--domain`): with profiles, the setting is a single scalar in the file that already describes the
> tenant; without them it would have to be a domain-keyed map inside a shared file, which is strictly worse
> and would have to be migrated afterwards. **Do not implement this entry first.**
>
> Two consequences are deliberate. The key holds the workspace **id** (the GUID `azquery` queries by; a full
> ARM resource id may be accepted as an alternative spelling), never a display name — resolving a name would
> need a subscription, Reader on the workspace's resource group and disambiguation across subscriptions, a
> whole second permission surface to save pasting a GUID once per tenant. And there is deliberately **no
> flag and no `AZURE_RD_*` variable**: both beat the config file in the precedence order, so a value passed or
> exported for one tenant and forgotten would silently apply to the next — and a wrong workspace does not fail
> loudly, it returns no matching rows, which reads as *nobody changed it*. Binding the setting to the tenant's
> own profile makes that mistake unrepresentable.

**Plan.**

- Add an `internal/audit` engine (imported by `internal/drift`, keeping the dependency direction that already
  holds for `drift` → `docs`) that takes an `Observation` plus a workspace id and returns one attribution per
  finding. Query `IntuneAuditLogs` and `AuditLogs` through `sdk/monitor/query/azquery` (new direct dependency,
  added with `make deps`) over the observation's window, and map both schemas onto **one** finding-shaped
  record — the two tables' field names must not leak into the artifact.
- Route each finding to a table by its `APIType`/resource type, not by trying both: Intune types to
  `IntuneAuditLogs` (joining `Properties.TargetObjectIds` against the finding's `resourceId`), Entra types to
  `AuditLogs` (joining `TargetResources[].id`), everything else straight to a stated not-queried status.
- Batch the lookup: one query per table for the whole finding set (a GUID `in (…)` set), not one query per
  finding — a drift observation can carry hundreds.
- Write `drift/audit.yaml` atomically, last, the way `WriteObservation` writes the observation. It is
  self-describing: a schema `version:` from day one (the field `drift/metadata.yaml` lacks and `index.yaml`
  learned to need), the workspace queried, the exact window, the tables consulted, the `observedAt` and
  baseline `generatedAt` it belongs to, and a per-finding status of `matched` / `no-event-in-window` /
  `no-join-key` / `retention-exceeded` / `query-failed` / `not-queried`.
- Refuse to attribute a superseded observation: reuse the `docs analyze-drift` preflight rule (baseline
  `generatedAt` mismatch) and the tenant cross-check, so an audit file can never describe an observation the
  current baseline has replaced.
- Expose it twice, over the one engine: a standalone `azure-rd resource audit` that enriches the observation
  already on disk (re-runnable without re-fetching the tenant — the usual case, since ingestion lags), and a
  drift run that calls the same engine after comparing. Per the option model established by the
  configuration entry above, the latter is enabled by a config key, not a flag — it changes what a run
  produces — while the standalone command remains the explicit, visible entry point. Both honour `--dry-run`
  by withholding only the write and saying an earlier `audit.yaml` was not refreshed.
- Add one config-only option, `audit-workspace-id:`, read from the tenant's configuration profile and
  registered on the tenant-scoped side of the key partition (so it is rejected in a base file). Empty or
  absent means the lookup is off and every finding's status says so — never a silent no-op. Add it to
  `config.example.domain.yaml` **empty and commented**, preserving that file's no-op promise.
- Record the workspace actually queried in `drift/audit.yaml`, so an attribution can always be traced back to
  the source it came from and a profile mix-up is visible after the fact rather than only at the time.
- Note the new audience in the auth surface: the workspace token is issued for `api.loganalytics.io`, so the
  operator needs *Log Analytics Reader* on the workspace, and the dedicated-app path needs the Log Analytics
  API permission added. Detect a missing permission through `azure.IsPermissionError` and degrade to
  `query-failed` with a warning naming the grant required.
- Splice the attribution into `docs analyze-drift`: each finding's section gains the actor and timestamp when
  one is known, and an explicit "attribution unavailable (<status>)" line when it is not, so the analysis
  agent can weigh a change against who made it instead of reading it as anonymous.
- Tests: schema mapping for both tables from recorded fixtures (no network), routing by resource type, the
  batching query construction, the preflight refusals, every status path including retention and permission
  failure, determinism of the written bytes for a fixed input, and the dry-run withholding. Cover the
  off-by-default case (no workspace configured) reporting a stated status rather than an empty result. Extend
  the drift command's flag-surface test so `--audit` is offered only where honoured.
- Documentation: a README section on attributing drift (prerequisites — Intune and Entra diagnostic settings
  shipping to one workspace, the RBAC grant — the two entry points, the profile key that enables it, the
  artifact and its statuses, and the ephemerality it shares with the rest of the drift tree), plus the
  output-layout list gaining `drift/audit.yaml`;
  `CHANGELOG.md` under `[Unreleased]`; and the drift-tree lifecycle rule in
  `.windsurf/rules/04-security-and-ops.md` naming the new root file as swept, never pruned.

## Parked ideas

Deliberately not scheduled — kept here rather than in a work entry so they survive as the entries around them
ship and are removed. Each records why it is parked and what would make it worth doing.

### Idea: routine dependency updates, and consolidating on one Microsoft Graph SDK

Two related pieces of dependency hygiene. First, a routine update pass (`go get -u`, `make deps`, `make check`)
over the direct dependencies. Second, investigate whether the module can carry **one** Microsoft Graph SDK
instead of two. The direction is the opposite of the intuitive one: `msgraph-beta-sdk-go` is imported by 53
handler files and serves the Intune/device-management endpoints that **do not exist on v1.0**, so it can never
be the one removed. The candidate for removal is `msgraph-sdk-go` (v1.0), used by one shared client constructor
and seven handlers (conditional access, groups, organization, authentication methods/strengths, authorization
policy, on-premises synchronization) — the beta endpoint is a superset, so those seven could move. The prize is
real: the two generated SDKs dominate compile time and binary size, and one of them is nearly gone already.
**Not planned — parked deliberately**, for three reasons:

- **A dependency bump is not hash-neutral by construction.** A Graph SDK update can change which properties a
  model carries and therefore the YAML bytes the export writes — moving `sourceSha256` for resources that did
  not change in the tenant, which reads as mass drift and forces documentation regeneration. Until the drift
  command exists there is no cheap way to *prove* a bump content-neutral; after it ships, "update, re-download
  a fixture tenant, drift must report zero findings" becomes the standard verification, and updates get cheap.
- **Moving the seven v1.0 types to beta trades a stability contract for uniformity.** v1.0 responses are
  contractually stable; beta responses may change shape at Microsoft's discretion. Today the most stable,
  most-referenced types (groups, conditional access) deliberately sit on the stable endpoint. Consolidation
  buys shorter builds but makes every type's bytes hostage to beta churn.
- **The switch itself moves hashes once.** Re-fetching those seven types through the beta endpoint will change
  their YAML (beta models carry extra properties), so their `sourceSha256` values move and their documents
  regenerate — a one-time cost that should ride a regeneration scheduled for another reason, not force its own.

**Revisit when** the drift command has shipped (it is the verification tool this work lacks), and either a
security advisory forces an SDK bump anyway or build time becomes a felt cost. If consolidation is picked up:
move the seven handlers one at a time, verify each with a fixture-tenant drift run, expect and batch the
one-time hash movement, and only then drop the v1.0 module. If beta churn is the worry instead, the same
investigation can conclude the opposite consolidation is wiser once Microsoft ports the Intune endpoints to
v1.0 — check that first; it would remove the *beta* SDK and the churn with it.

### Idea: per-finding severity in document `Security` sections

Tag every individual security callout *inside each resource's document* with `**[risk]**` / `**[review]**` /
`**[ok]**`, so the web side can colour or filter them. **Not planned — parked deliberately**, for three
reasons:

- **It is a subjective, model-only judgement.** Nothing in the export can compute or validate whether a given
  setting is risk / review / ok.
- **It is made 400+ times** (once per callout across every document), so a bad or inconsistent batch is
  likely — and the only fix is regenerating everything.
- **Low marginal benefit.** Section-level styling (the `Security` H2 slug) already gives the frontend most of
  the visual win without the per-item risk.

This is the opposite trade-off from the tenant-summary findings severity, which is decided once per tenant on
at most six findings — tiny blast radius, easy to eyeball — and was therefore done.

**Revisit only if both hold:** (1) the closed-heading contract has proven stable across a real regeneration,
with no drift observed in practice; and (2) the web side needs per-item severity that section-level styling
cannot deliver. If promoted, treat it as its own one-shot: extend the `Security:` instruction across all
seven templates and regenerate every document — and accept that it cannot be automatically validated.

### Idea: resolve Graph object ids to names inside the exported YAML

Add a transformer that resolves Microsoft Graph object ids that appear in a resource — assignment `groupId`s,
filter ids, `notificationTemplateId`s — to their display names at export time, as the `id-resolution`
transformer already does offline for ARM resource ids (which carry their name in the id itself). The YAML
would then read `groupId: 8964516b-… (GBL_D_WIN_...)` instead of a bare GUID. **Not planned — parked
deliberately**, for three reasons:

- **The documentation already resolves them, and does so incrementally.** `docs generate-prompt` builds the
  group, filter and template reference maps from `metadata.yaml`, renders every assignments / "Targeted by" /
  "Used by" block from them, and re-splices exactly those blocks when a referenced object is renamed — without
  touching the resource's own document or its YAML. Resolving in the YAML would duplicate that with a worse
  failure mode.
- **It would put a decision into a fact.** A resource's YAML and its `sourceSha256` are meant to move only when
  the resource itself changes. Embedding another object's *current* name makes every policy's hash move when a
  group is renamed, which forces regenerating every document that assigns it — the exact cascade the marked
  splice blocks exist to avoid.
- **It costs one extra Graph read per referenced id**, on every run, for information the export already holds
  once (in the group's own YAML).

**Revisit only if** a consumer other than the documentation pipeline needs names inside the YAML itself — e.g.
a diff/review workflow on `resources/` that cannot read `metadata.yaml`. If promoted, resolve from the export
(the already-downloaded groups/filters/templates), never from a live lookup, and write the name into a sidecar
`_name` key the way `id-resolution` does — never in place of the id.

### Idea: `resource compare` — an offline comparison of two exports, and the home of the cross-tenant identity rule

Compare two tenants' exports on disk — stage against prod — the way `resource drift` compares one tenant
against its own export: a verdict per resource key (only in A, only in B, same, different), dotted-path deltas
for the different ones, payloads, and an `analyze.md` so the drift-analysis agent can judge the **impact** of
each difference (security posture, compliance, lifecycle, who is affected) rather than only that bytes differ.
No Azure call: both inputs are export trees, so the command is offline like `docs analyze-drift`.

**Why this exists as a Go idea now.** The web project has shipped a **web-only proof of concept** of exactly
this comparison (the docs browser's tenant compare, described in `web/README.md`: two-click tenant
selection on the picker, a listing from the two `resources/metadata.yaml` files, and a per-resource YAML diff).
To make equal configuration compare equal across tenants, that PoC **normalises tenant-local identity in the browser**:
drops every `id` or `sourceId` at any depth whose value contains a GUID (ids without one — settings
ordinals, `all_users`, authentication method names, the all-zero sentinels — are content and stay), every
`*@odata.context` key at any depth, `createdDateTime`, `lastModifiedDateTime`, `version` and the group
identity fields (`mail`, `mailNickname`, `proxyAddresses`, `securityIdentifier`, `renewedDateTime`), and
resolves `assignments[].target.groupId`, `assignments[].target.deviceAndAppManagementAssignmentFilterId` and
`notificationTemplateId` to the display name of the matching `resourceId` in the same tenant's
`metadata.yaml` — a set-valued lookup, since neither `resourceId` nor `displayName` is unique in real exports:
a reference resolves only when every entry with that id agrees on the name, otherwise the GUID stays and is
flagged ambiguous or unresolved, and a zero-sentinel reference means *none* and passes through. Measured
against the two reference exports, that rule leaves 19 of 114 paired resources identical, 36 once
`assignments` is removed as well — the difference being the same policy targeting a differently named group
in each tenant, which the web diff page reports as *differs only in audience*. That rule is a **judgment,
not a fact**, and it was accepted on the web side under one condition, recorded there: it is provisional and
**moves to the CLI once it is stable**. The PoC's job is to find out what the rule is against real pairs;
this idea's job is to receive it. Until then the browser computes what the CLI should be emitting — the same
disagreement risk the taxonomy rules exist to prevent (a rule derived in one consumer can disagree with every
other), tolerated here only because there is no other consumer yet.

**Not planned — parked deliberately**, until the PoC has produced a rule that can be *stated and tested* —
a fixed drop list plus a reference-resolution table per field, with fixtures from a real stage/prod pair —
rather than guessed per resource type. Promoting it before that would freeze a guess into a contract the
browser then depends on.

**Shape when promoted.**

- **Engine** beside the drift engine (`internal/drift` or a sibling `internal/compare`), reusing the verdict
  and delta machinery over normalised documents; the normalisation is one exported, table-driven, unit-tested
  function — the *single* truth the browser stops duplicating.
- **Command** `azure-rd resource compare` taking two export domains (both resolved the way `--domain` is
  today), offline, `--dry-run` withholding only the writes like `resource drift`.
- **Output tree — placement is the open design question.** Every tree today lives under `<output>/<tenant>/`
  and a comparison belongs to neither tenant. Candidates: a sibling root `<output>/compare/<a>__<b>/`, or
  under the left tenant `<output>/<a>/compare/<b>/`. Whichever is chosen, the tree mirrors `resources/` keys
  the way `drift/` does (metadata at the root, payloads ≥ 2 levels deep, agent-written documents beside them),
  is cleared and rebuilt per run with no history, and carries a schema `version:` from day one — the field
  `drift/metadata.yaml` lacks and `index.yaml` learned to need.
- **Analysis prompt** as a third template beside `generate_prompt_template.md` and
  `analyze_drift_template.md`, with `doc-prompt.md` as the per-type lens again — **not regeneration-gated**,
  moving no `promptSha256`.
- The browser then renders the tree as it renders `drift/` and deletes its own normaliser.

**Relation to the idea above.** This does **not** trigger *resolve Graph object ids to names inside the
exported YAML*: the comparison reads names from `metadata.yaml`, which is exactly the consumer that idea says
does not need them in the YAML. The facts stay facts.

### Idea: bootstrap the curated taxonomy from per-document LLM suggestions

Have the doc-generation model suggest, per resource, which programmes it belongs to (as *labels* with a short
rationale, never ids), then harvest those suggestions at index time into `docs/taxonomy-suggestions.yaml` — a
report that diffs the guesses against the curated rules into **coverage gaps** (a resource the model assigns to
an existing programme that no rule matches) and **new-programme candidates** (a label that is not a programme
yet). It would let an operator grow the `taxonomy:` rule set from evidence instead of authoring every regex
cold. **Not planned — parked deliberately**, for two reasons:

- **Its payoff scales with taxonomy size, which is small today.** The report earns its keep only once an
  operator is maintaining a large, drifting rule set across many tenants. Cold-authoring the handful of
  programmes in play now is cheaper than building and reviewing an advisory pipeline.
- **It cannot land cheaply on its own.** The suggestion instruction lives in the per-type templates, so adding
  it moves `promptSha256` for every non-`record` type and forces a full documentation regeneration. It is only
  free if it rides a regeneration already scheduled for another reason — otherwise it forces its own.

**Revisit when** an operator is maintaining programmes at a scale where cold-authoring rules is the bottleneck,
*and* a full documentation regeneration is already scheduled to absorb the template change. If promoted, the
shape is constrained — these are invariants that keep it safe, not open questions:

- **Advisory only, never authoritative.** `facets` stays rules-only and deterministic; suggestions live in a
  separate artifact and the only path from a guess to authoritative data is a human writing a rule. Promotion
  stays manual — auto-writing rules would re-inject non-determinism into the rules source and defeat the point.
- **Labels, not ids.** The operator mints the stable id (`programmeIDPattern`) at promotion time, so the model
  can never spawn `cis`/`cis-l1`/`cis-hardening` as three programmes.
- **Determinism preserved.** Because suggestions are harvested from written frontmatter bytes, both
  `index.yaml` and the suggestions artifact stay byte-identical over an unchanged export; the non-determinism
  is confined to doc-authoring time, exactly as `platformGroup`/`functionGroup` already are.
- **The hint vocabulary must not touch `promptSha256`.** Any hint of the operator's current labels rides the
  non-hashed `docs/generate.md`, never a per-type `doc-prompt.md`, so a cheap offline `taxonomy:` edit never
  couples to an expensive regeneration. Suggesting with no hint (pure bootstrap, no `taxonomy:` yet) must also
  work; label normalisation clusters the free output.
- **`docs/taxonomy-suggestions.yaml`** is the third and last file `azure-rd` writes under `docs/` (with
  `generate.md` and `index.yaml`), at the tree root where no document can be; `--dry-run` writes nothing and
  `--prune` never touches it.
- **`config.example.yaml` stays inert** — the feature needs no new config key (it reuses `taxonomy:` labels as
  an optional hint), so loading the example unmodified still produces byte-identical output including every
  hash in `resources/metadata.yaml`.

### Idea: review the sign-in surface — can a scoped `az login` replace the dedicated-app device-code path?

The tool carries two sign-in paths. The default reuses the `az login` session; `--client-id`/`--tenant-id`
starts a device-code sign-in against a dedicated app registration. The second path exists for exactly one
reason: `az account get-access-token` always mints tokens for the Azure CLI *first-party* app — regardless of
how the user logged in — and that app's Graph token has lacked the delegated scopes most Graph handlers
declare (`DeviceManagementConfiguration.*`, `DeviceManagementApps.*`, `DeviceManagementScripts.*`,
`Policy.Read.All`, …). But `az login` accepts `--scope`: signing in with
`az login --scope https://graph.microsoft.com/.default` (or explicit scopes) asks Entra to add delegated
Graph scopes to that same CLI session — the README's own troubleshooting hint already leans on it for the
"required scopes are missing" failure. If a scoped login reliably lands every declared scope in the session's
Graph token, the entire second path becomes removable: the device-code credential branch, the two flags and
their env/config equivalents, the `PermissionScoped` probe (`RequiresDedicatedApp` /
`DedicatedAppRequirements`), the interactive dedicated-app prompt, and the app-registration setup in
`README.md` — collapsing authentication to one path and one instruction. Even a partial "yes" has value: the
dedicated-app prompt could recommend the exact scoped re-login first and fall back to device code only when
the CLI app genuinely cannot obtain a scope. **Not planned — parked deliberately**, for three reasons:

- **The answer is not in this repository.** Whether a scope lands in the CLI token's `scp` claim depends on
  the tenant's consent policy and on which scopes Microsoft lets its first-party app request — both outside
  the tool's control and changeable by Microsoft without notice. Only a live-tenant experiment settles it:
  perform a scoped `az login`, decode the token's `scp`, verify that azidentity's CLI credential (which
  shells out to `az account get-access-token` per request) actually surfaces the scopes granted at login,
  then run a full download of every dedicated-app-gated type — including `--resolve-secrets`, which needs
  `DeviceManagementConfiguration.ReadWrite.All`.
- **A positive result on one tenant does not generalize.** Consent policies differ per tenant, Microsoft has
  been progressively hardening what the first-party CLI app may do, and some services may gate on the calling
  application rather than the token's scopes alone — only a live call against each gated endpoint settles
  that. The dedicated app registration is the escape hatch the operator controls; deleting it trades
  resilience for a smaller surface.
- **Removal is a breaking change to the auth surface** — the flags, their `AZURE_RD_*` variables and config
  keys — so it should ride a major, not a hygiene pass.

**Revisit when** an operator confirms on a representative tenant that a scoped `az login` yields every
permission the handlers declare, or the next time the authentication surface is reworked anyway. If promoted,
soften before deleting: first teach the dedicated-app prompt to recommend the exact `az login --scope …`
command derived from the selected types' declared permissions, keeping device code as the fallback; only
retire the flags once the CLI path has covered every `PermissionScoped` type against a live tenant, and
record the removal under `Breaking`.

### Idea: clear the `gocognit` baseline

Split up the 26 functions named in the baseline at the end of `.golangci.yml`'s `exclusions.rules` — the ones
that already exceeded the Sonar cognitive-complexity threshold when `gocognit` was switched on, across 20 files —
deleting each entry as its function is fixed, until the block and its explanation can go. The worst are
`GeneratePrompt` (58), `GenerateIndex` (57), `findAndRemoveKeysWithPreserve` (47), `ParseCleaningConfig` (39),
`drift.Compare` (38), `compileTaxonomy` (37) and `PrintSummary` (34); the rest sit between 16 and 30.
**Not planned — parked deliberately**, for three reasons:

- **The baseline already delivers what mattered.** The rule stays enabled at the server's threshold, the gates
  are green, and every function written from now on has to comply — including new functions in the listed files,
  because each entry names one function instead of excluding its file. Nothing is hidden either: Sonar keeps
  reporting all 26, which is deliberately why the baseline is not mirrored in `sonar-project.properties`.
- **A high score here is not a defect, and several of these functions are covered by invariants that a
  refactor must not disturb.** `PrintSummary`, `mergeMetadata`, `pruneCovered` and `Compare` are exactly the
  places where "an incomplete run may not mark anything absent", "covered means the listing succeeded" and the
  prune guards live; `Writer.Write` and `transformResource` sit on the every-request-produces-one-result
  accounting. Splitting them for a metric, without a reason a reader would recognise, risks a real regression in
  return for a number.
- **Go's explicit error handling inflates the metric**, so a mechanical fix — extracting each `if err != nil`
  ladder into a helper — would move the score without making anything clearer, which is the outcome the rule is
  supposed to prevent.

**Revisit when** one of these functions is being changed for another reason (split it then and delete its entry
in the same commit — that is how this shrinks without a campaign), when a function on the list becomes hard to
change safely in practice rather than merely scoring high, or if the baseline ever stops shrinking, which would
mean it has started collecting new debt instead of recording old. If picked up, work one function at a time with
`make test-race` where the function touches the pipeline, keep the tests that pin the invariants above unchanged
(a refactor that needs a test edited is a redesign, not a split), and remember that a stale entry is invisible —
golangci-lint does not report an exclusion that matched nothing, so re-measure by commenting the block out. Each
split is internal and needs no `CHANGELOG.md` entry; deleting the block at the end does.

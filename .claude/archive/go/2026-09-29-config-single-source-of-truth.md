---
title: Make configuration the single source of truth, with per-tenant profiles selected by domain
project: go
status: done
started: 2026-09-29
finished: 2026-09-29
branch: chore/refacor-command-line-surface
pr: 30
changelog: 0.4.0
---
## Make configuration the single source of truth, with per-tenant profiles selected by domain

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

- ✅ Reduce the command line to the model above: keep `--config`, `--config-dir`, `--domain`, `--dry-run`,
  `--log-level`, `--debug`, `--output`, `--out`, `--prompt`, `--exit-code`, `--type`, `--resource-id` and
  `--resource-group`; remove `--subscription`, `--client-id`, `--tenant-id`, `--workers`, `--timeout`,
  `--resolve-secrets`, `--no-prompt` and `--prune`. Drop the `resource-id` and `resource-group` **config
  keys** in the same pass — they become flag-only.
- ✅ Delete what the removal makes dead: `cmdutil.AddAzureAuthFlags` / `AddPersistentAzureAuthFlags`,
  `AddWorkersFlag` / `AddPersistentWorkersFlag`, `AddTimeoutFlag` and `RequireAuthFlagPair`
  (`AddSelectionFlags` survives whole — all three selection flags stay). The client-id/tenant-id pairing
  becomes profile validation, which is the only place it can be checked against the tenant it belongs to.
- ✅ Bind only the two config-backed flags explicitly (`viper.BindPFlag` for `output` and `type`), and delete
  `cmdutil.BindFlags` along with every `BindFlags(cmd)` call in a `RunE`. The remaining flags are read from
  the command's own flag set, never through viper — which is what stops a sibling command's identically
  named flag leaking through the global singleton, the defect `BindFlags` existed to work around.
- ✅ Replace the "was it set explicitly" checks that `cmd.Flags().Changed(...)` provided for removed flags with
  `viper.IsSet(...)`. `--workers` is the one that matters: the per-API defaults apply unless the operator
  set a count deliberately (`WorkersExplicit`), so losing that distinction would silently override the
  Microsoft Graph and ARM defaults with a single number.
- ✅ Keep the interactive dedicated-app prompt working when a selected type needs one and no profile supplies
  it, and have it print the **profile snippet to save** rather than only the values, so the answer lands in
  the single source of truth instead of a shell history entry.

*Removing the environment layer (breaking)*

- ✅ Remove `viper.SetEnvPrefix`, `viper.AutomaticEnv` and `viper.SetEnvKeyReplacer` from `initConfig`, and
  delete every `AZURE_RD_*` reference from `config.example.yaml`, `README.md` and the rule files — including
  the invariant that currently forbids removing the replacer, which this change retires.
- ✅ Leave `LOG_LEVEL` alone: it is read directly by the logger and is not part of the viper surface.

*Profiles and layering*

- ✅ Add `--config-dir` as a root persistent flag beside `--config`. It is the fifth global flag and needs its
  justification recorded: it is part of the config-loading mechanism root already owns, and it must be
  resolved in `initConfig` before any command runs, so it cannot live on a group.
- ✅ Resolve and layer in `initConfig`. The base is `<config-dir>/base.yaml` **by convention** when the
  directory holds one, or the file named by `--config`, which wins when both are present. Then read the leaf
  command's explicitly passed `--domain` (root's hook already receives the leaf command, and flags are
  parsed by the time it runs), join it to the config directory as `<domain>.yaml`, and merge it over the
  base with `MergeInConfig`. Log both files and the domain that selected the profile, extending the existing
  "Using config file" line. The convention exists so the everyday invocation is two flags
  (`--config-dir ~/.azure-rd --domain cb-gmbh.com`) rather than three; with the environment layer gone there
  is no way to set the directory once per shell, so this is what keeps the command line short.
- ✅ Exclude `base.yaml` from profile resolution and from the `--domain` completion candidates. A real Entra
  default domain always contains a dot, so the name cannot collide with a tenant, but the exclusion should
  be explicit rather than incidental.
- ✅ Do **not** default the config directory (e.g. to `~/.azure-rd`). Auto-discovery was deliberately removed
  from this tool once already; a run must state which configuration it used.
- ✅ Define the key partition in **one** exported table (tenant-scoped vs general), and validate both files
  against it: a general key in a profile and a tenant-scoped key in a base file are each a fatal error
  naming the key and the file. That table is the single truth the documentation, the stub file and the tests
  all read from — never a second hand-maintained list. Validate each file **before** merging: after
  `MergeInConfig` the merged state no longer records which file a key came from, so validation on the merge
  result cannot name the offender — or catch a key present legally in one file and illegally in the other.
- ✅ Validate the domain as a **single path segment** before joining it — no separator, no `..`, not empty — so
  a flag value can never escape the config directory.
- ✅ Fail loudly and helpfully when the profile is absent: name the path that was looked for and list the
  `*.yaml` files the directory does contain, which doubles as the answer to "which tenants do I have?".
- ✅ Fail when `--config-dir` is given without `--domain`, stating the reason (the tenant is not known until
  after authentication, and the profile supplies the credentials that authentication needs).
- ✅ Ship `config.example.domain.yaml`: a commented stub listing **every** tenant-scoped key
  (`tenant-id`, `client-id`, `subscription`, `filters`) with empty values, carrying the same no-op promise
  `config.example.yaml` does — copying it to `<config-dir>/<domain>.yaml` unmodified must behave exactly
  like having no profile.
- ✅ Rework `config.example.yaml` in the same change: drop the tenant-scoped keys (`subscription`, `client-id`,
  `tenant-id`, `filters`) and the now flag-only ones (`dry-run`, `log-level`, `resource-id`,
  `resource-group`), add the options that lost their flags, replace every "Equivalent to --x / AZURE_RD_X"
  note with the new home, and point at the domain stub. The no-op promise still holds and gets easier to
  verify.
- ✅ Ship `workers` **commented out** in the example, unlike today's active `workers: 5`. It is now the one
  option whose mere presence changes behaviour — setting it is the explicit choice that overrides the
  per-API defaults (Graph 5, ARM 20) — so an active value in the example would silently opt every copier out
  of ARM's 20. This is a real difference from the flag, which had to be passed to count as explicit.

*Tenant resolution cleanup*

- ✅ Replace the three divergent policies with one shared resolver: `runprep.Prepare` resolves and on failure
  **keeps the flat base output path**, `cmd/docs`' `resolveExportDir` treats `--domain` as offline-or-detect,
  and `resource drift` does its own declared-vs-resolved cross-check. Only the third is right. One resolver
  returns the tenant directory, the domain to cross-check against, and an explicit state: *declared and
  verified*, *declared but unverifiable* (offline, or resolution failed), or *resolved only*.
- ✅ Delete the flat fallback in `runprep`. Writing `<output>/resources/` in a layout where everything else
  looks for `<output>/<domain>/resources/` produces an export that `docs` and `detectSingleExportDomain` can
  never find again. With a declared domain, use it and mark it unverified; with neither, refuse — exactly as
  `resource drift` already does.
- ✅ Keep the auth-resolved tenant as **validation, not as the source**: the declared domain is intent, the
  resolved domain is ground truth, and a mismatch aborts before anything is written. Removing resolution
  would let a typo silently create a second export directory for the same tenant, which nothing would ever
  notice.
- ✅ Add `--domain` to `resource download`, `resource types` and `resource list` with those same semantics, and
  to root for the `--debug` report, so the resolver has one input everywhere and profiles are usable on a
  download.

*Discovery*

- ✅ Register a completion function for `--domain` (`cmd.RegisterFlagCompletionFunc`, returning
  `ShellCompDirectiveNoFileComp`) offering the config directory's `*.yaml` basenames and the export
  directories under `--output`. It must stay local and cheap — never an Azure call on a Tab press — and must
  not assume config has been loaded, since the hidden `__complete` invocation is not guaranteed to run the
  hooks; read `--config-dir` and `--output` from the flag values directly.
- ✅ Report the resolved selection in `azure-rd --debug`: config directory, domain, base file, profile file —
  beside the existing config-file line, so an operator can confirm a switch without running anything that
  writes.

*Verification and documentation*

- ✅ Tests: the `base.yaml` convention, including `--config` overriding it and a directory without one;
  `base.yaml` excluded from profile resolution and completion; base+profile merge order and precedence (the
  surviving flags still beat both); partition validation
  rejecting a key on either wrong side; the missing-profile error including the candidate listing;
  path-segment validation rejecting separators and traversal; `--config-dir` without `--domain`; the stub
  file loading as a true no-op; the resolver's three states including the removed flat fallback; the
  `--domain` cross-check aborting a download against the wrong tenant; completion candidates from a fixture
  directory; `viper.IsSet`-based explicit-worker detection preserving the per-API defaults; and a test that
  a set `AZURE_RD_*` variable is **ignored**, so the env layer cannot creep back in unnoticed.
- ✅ Invert the flag-surface tests in `cmd/resource_test.go` and `cmd/docs_test.go`: they currently assert the
  removed flags are present. They must assert the full surviving set and that every removed flag is **gone**
  from every command — the same both-directions check they already make, retargeted.
- ✅ Pin the zero-config path with a test: no config file, no flags, defaults only must still resolve a full
  export as today **when tenant resolution succeeds**. The failure edge changes deliberately (refuse instead
  of writing flat — see the resolver cleanup), so the test pins the promise that actually survives. It is
  the one most at risk in a refactor this wide.
- ✅ Documentation: a README section on working with several tenants (directory layout, base vs profile, the
  partition and why it is enforced, the `--domain` switch and its cross-check, installing shell completion),
  a configuration-reference section replacing the flag-centric one, and a migration table mapping every
  removed flag and `AZURE_RD_*` variable to its new home.
- ✅ Update the rule files this change contradicts: the flag-placement and `cmdutil.BindFlags` requirements in
  `.windsurf/rules/03-commands.md`, and the config-precedence, env-variable and env-key-replacer invariants
  in `.windsurf/rules/04-security-and-ops.md`. Leaving them in place would keep describing a surface that no
  longer exists.
- ✅ `CHANGELOG.md` under **`Breaking`**, stating in bold that existing scripts and pipelines must move the
  removed flags and environment variables into a config file.

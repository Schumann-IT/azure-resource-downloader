# Next iterations — deliberately out of scope

Outstanding work and parked ideas for the Go CLI. Each numbered entry is a unit of planned work; **once its
plan ships in full, the entry is removed** and its history lives in `CHANGELOG.md`. Ideas that are
deliberately not scheduled collect under *Parked ideas* at the end, so they persist as the entries around them
ship. `README.md` stays the single source of truth for what the tool *does today*.

## 1. Commit a golangci-lint configuration that GoLand runs unchanged

**Goal.** `make lint-check` and the GoLand editor report the same findings from one committed configuration,
and the linter set the style rule leans on ("passes with default linters") is pinned in the repository instead
of being whatever the installed golangci-lint happens to default to.

> **Today.** There is no `.golangci.yml`. `make lint` / `make lint-check` run golangci-lint 2.x with its
> `standard` set — `errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused` — and nothing in the repo says
> so, so a developer whose IDE shows a different set of warnings has no file to compare against.
>
> **How parity with GoLand works.** GoLand bundles the *Go Linter* plugin (its golangci-lint integration,
> enabled by default since 2025.3), and it reads a golangci-lint config file: Settings | Go | Linters | *Use
> config* disables the IDE's own linter table and runs whatever the file says. That is the direction in which
> identity is achievable — the editor runs the same binary with the same file — so the committed config is the
> single lint truth and the IDE mirrors it, not the other way round. GoLand's **bundled** Go inspections are a
> separate engine and cannot be exported into a golangci-lint config; the mapping below makes the two agree
> where they overlap, and anything only the bundled inspections report stays an editor hint, not a gate.
> `.idea/` is gitignored, so the *Use config* path cannot be committed: the README documents it as a one-time
> per-developer setting. The file lives in `go/`, which golangci-lint finds when run from `go/` (the Makefile
> does); whether GoLand opened at the repository root resolves it on its own has to be verified, and the
> explicit path is the fallback.
>
> **Mapping GoLand's default inspections onto linters** — each row is checked against the default inspection
> profile before its linter is enabled, so the file mirrors what GoLand reports out of the box, not a wish
> list: *Unhandled error* → `errcheck` (standard); *Go vet* → `govet` (standard; `printf`, `structtag`,
> `unreachable` and the rest of vet's default passes are already in); *Unused constant / variable / function /
> type* → `unused` (standard); *Nilness analyzer* → govet's `nilness` analyzer, which is **not** in govet's
> default pass set and must be enabled in `settings.govet`; *Shadowing variable* → govet's `shadow` analyzer,
> same caveat, and only if the default profile actually has that inspection on; *Redundant type conversion* →
> `unconvert`; *Deprecated element*, *Self assignment*, *Unreachable code*, the *Redundant …* family →
> `staticcheck` (standard; its `ST` style checks stay at golangci-lint's own default subset); *Unused
> parameter* → `unparam` is the candidate, decided after a trial run on the volume of findings, because
> GoLand's inspection is a weak warning and `unparam` is stricter. *Exported element should have comment* is
> **off** in GoLand's default profile, so `revive`'s `exported` rule is deliberately not enabled here even
> though the project's "every exported function gets a doc comment" rule would like it — that would be a
> stricter-than-the-IDE choice and belongs to its own decision, not to a parity entry.
>
> **Formatting.** golangci-lint v2 separates `formatters` from `linters`; GoLand 2025.3 can format on save
> through `golangci-lint fmt`. Declaring `gofmt` as the only formatter makes the IDE's format-on-save identical
> to `make fmt`, and `make fmt-check` keeps guarding it independently. No `goimports`/`gofumpt` — that would
> change bytes `make fmt` does not.
>
> **Schema.** `version: "2"`, validated with `golangci-lint config verify`, so a typo in the file fails loudly
> in CI instead of silently reverting to defaults.
>
> **What the trial run settled.** `unconvert` and govet's `nilness` report nothing on this codebase, so both
> are enabled at zero cost. `unparam` found one dead parameter (removed) and six "always receives the same
> argument" reports — five in test helpers, excluded by path, and one on a fact accessor whose signature is
> deliberately symmetric with its siblings, silenced at the site. govet's `shadow` reported only idiomatic
> `if err := f(); err != nil` blocks and is **not** enabled; the reason is recorded in the config. Note that
> gofmt moves a `//nolint` directive to the end of its comment block, so the prose reason goes *above* it and
> the directive line stays short. Also confirmed: golangci-lint finds no config when started from the
> repository root, so the explicit path in GoLand is required rather than optional.

**Plan.**

- ~~Add `go/.golangci.yml` (`version: "2"`): `linters.default: standard`, an explicit `enable` list for the
  additions the mapping settles on (`unconvert`; `unparam` pending the trial run), `settings.govet.enable`
  with `nilness` (and `shadow` if the profile check says so), `formatters.enable: [gofmt]`. Each enabled
  linter carries a comment naming the GoLand inspection it mirrors, so the file *is* the mapping and a later
  reader can tell a parity choice from a project preference.~~
- ~~Run `make lint-check` on the new config and triage: a finding is fixed in the code or excluded with a
  written reason under `linters.exclusions`, never by dropping the linter to go green. Run `make test-race`
  if any fix touches concurrent code.~~
- ~~Prepend `golangci-lint config verify` to the `lint-check` and `lint` targets in the `Makefile`, so
  `check`/`ci`/`branch-ready`/`release-ready` all fail on a broken config.~~
- GoLand: Settings | Go | Linters → *Use config* → `go/.golangci.yml`. Verify the plugin's findings in the
  editor match `make lint-check` on a file with a known finding. **Outstanding — needs the IDE**; the config,
  the docs and the verification that the path must be explicit are done.
- ~~Docs: README *Development* section (the config file, that it is the single lint truth, the one-time GoLand
  setting and the verification above); `02-style-and-quality.md` — "passes `golangci-lint run` with default
  linters" becomes "passes `make lint-check` with the linters configured in `.golangci.yml`, which GoLand runs
  too"; `01-project.md` repo layout gains the `.golangci.yml` line; `03-commands.md` is unchanged (the make
  targets keep their names and their read-only / rewriting split).~~
- ~~`CHANGELOG.md` entry under `[Unreleased]` *Added* (the config and the IDE parity), plus a note under
  *Changed* if the triage step changes any exported behaviour — otherwise the code fixes ride the same entry.~~

## Parked ideas

Deliberately not scheduled — kept here rather than in a work entry so they survive as the entries around them
ship and are removed. Each records why it is parked and what would make it worth doing.

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

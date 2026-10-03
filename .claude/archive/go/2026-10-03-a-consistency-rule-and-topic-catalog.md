---
title: A consistency rule and topic catalog in the configuration
project: go
status: done
started: 2026-10-03
finished: 2026-10-03
branch: feat/go-consistency-catalog
pr: 58
changelog: Unreleased
---
## A consistency rule and topic catalog in the configuration

*Kind:* feat

**Goal.** The operator can tell the consistency analysis which settings describe the same control across policy
types (compliance included), which cross-type relations to check, and how to group resources into topics — each
with its resolution semantics and its Microsoft Learn source written down — so the mechanical step can compare
compliance against configuration and the LLM step judges against a reviewed catalog instead of recall.

> **Decision.** The catalog is a configuration key like `taxonomy:` (operator-editable, validated), not embedded
> data.
>
> **Shape.** A general key `consistency:` with `equivalences` (id, members — a compliance property and/or
> configuration keys: Settings Catalog `settingDefinitionId`s, OMA-URI paths, legacy `@odata.type#property` —, a
> relation `same | >= | <= | = | required`, `enforced` per platform for the compliance side, a Microsoft Learn
> reference, `status: verified | verify`), `topics` (id, label, match patterns over setting keys,
> `@odata.type`s and type names; a resource may sit in several topics) and `rules` (id, description, left and right
> selectors, what counts as a violation, resolution semantics — which setting wins —, a Microsoft Learn reference,
> `status: verified | verify`). Seed rules from the Cowork draft, all to be verified: R1 compliance requires
> encryption/Defender/firewall but no in-scope configuration enables it; R2 compliance minimum OS above what the
> update rings / feature-update profiles deliver; R3 a CA policy requires a compliant device but the targeted users
> have no compliance policy for a covered platform; R4 the same control configured through several surfaces; R5
> update-ring feature deferral with a feature-update profile on an overlapping scope; R6 a filter property that
> contradicts the policy's platform or enrollment type; R7 enrollment/ESP/Autopilot targeting inconsistent with its
> dynamic group rule; R8 a macOS Platform SSO / account setting conflicting with password compliance.
>
> **Equivalences replace the parked mapping idea.** Joining a legacy typed property and a Settings Catalog / CSP
> setting is the same problem as joining a compliance property and a configuration setting, so the former parked
> idea *a curated legacy-property → CSP mapping* is part of `equivalences`.
>
> **Microsoft Learn check by Claude Cowork.** The seed catalog's semantics — which compliance settings are enforced
> on which platform, which setting wins, which keys describe one control — need sourced verification, which a Cowork
> session with web access does best (as for the metadata and prompt reviews). The entry's last bullet writes its
> brief; the result enters the backlog later like those reviews did.
>
> **Not regeneration-gated.**
>
> **From the code.** `docs analyze-consistency` (`cmd/docs/analyze_consistency.go`) and `internal/consistency/`
> shipped the mechanical half: the setting index, the scope model and the detector. The detector already takes
> `consistency.Equivalence` (`equivalence.go`: ID, Members, Relation, Enforced per platform, Status) through
> `Options.Equivalences`, which the command passes as `nil` today; an equivalence with `status: verify` already
> makes its findings `possible`, and `contradiction` already exists. This entry fills that hook. Member keys are
> validated in two steps: at load, every member must be a canonical key form the index emits (shared with the
> `index.go` key builders, never restated); at analysis, a member that indexes nothing in this export is counted,
> not an error — the config is per tenant, existence is per export. Topic match rules copy the taxonomy rule
> shape (`internal/docs/taxonomy.go`: OR of rules, AND within one, case-insensitive regexes) plus a `key:` regex
> over canonical setting keys. A rule's `left` / `right` selector is `topic: <id>` or `keys: [...]`, with an
> optional `class: requirement | configuration` and a `platforms:` regex.
>
> **Where the configuration lives.** The seed catalog ships in the tracked worked example
> `go/config-tailored-intune.yaml`. The operator's live configuration is `go/.config/` (the `--config-dir`,
> e.g. `go/.config/intune/base.yaml` and its per-domain profiles), git-ignored as operator data; nothing in this
> entry reads or writes it — tests build their own config fixtures, and the catalog reaches a tenant only when the
> operator copies the section into their base file.
>
> **Decision.** Indexing the inner payloads of Apple custom profiles is split out of this entry: it ships as the
> next entry, *Index the inner payloads of Apple custom profiles*, which extends this entry's seed equivalences.
>
> **Contract.** none — the `consistency:` key and the `catalog:` block this entry adds to
> `consistency/metadata.yaml` are Go-internal: the browser never serves `consistency/metadata.yaml` or
> `mechanical.yaml`, and the web consistency view reads only `consistency/index.md`, which this entry does not
> write. The block is additive; the metadata `version` stays as it is.
>
> **Owner.** Outside `go/`: only `Claude outputs/consistency-catalog-review-instructions.md` (git-ignored,
> written by the last bullet, never committed). Inside `go/`: `config-tailored-intune.yaml` and both example
> files; never `go/.config/`. No sequencing; *Index the inner payloads of Apple custom profiles* follows this
> entry.
>
> **Implementer.** opus
>
> **Decision.** Scope of the Claude Cowork review brief: verify **and extend** the seed catalog — read the real
> exports locally (`output/<tenant>/resources/` and `consistency/` only), research per resource type what Microsoft
> Learn documents as conflicting or taking precedence, and propose additions from the keys the tenants actually
> configure; no tenant value, name or id leaves the machine or appears in the returned plan. Widened by the user on
> 2026-10-03; supersedes "nothing read under `output/`" in the delivered brief bullet.

**Plan.**

- ✅ The `consistency:` key through `/add-config-option`: a general key (`internal/config/keys.go` `keyScopes`,
  `ScopeGeneral`, no default — absent means no catalog). `cmd/docs/analyze_consistency.go` reads it like
  `generate-index` reads `taxonomy:` (`viper.IsSet`, then `viper.UnmarshalKey` into a `mapstructure`-tagged
  `consistency.CatalogConfig`) and compiles it **before** resolving the export, so a typo fails offline and before
  any sign-in; an unparsable or invalid section exits with `exitCannotAnswer` and `invalid 'consistency' config
  section: …`. The compiled catalog reaches `consistency.Analyze` through a new `Options.Catalog *Compiled`, whose
  equivalences replace today's `nil` (the existing `Options.Equivalences` stays for the package tests).
  `config.example.yaml` carries the section commented out (it must stay a no-op); `config.example.domain.yaml`
  names `consistency` among the base-file-only keys in its header comment. `cmd/config_test.go`: the key is
  accepted in a base file and rejected in a profile, the per-file mention check covers it, and the no-op test
  asserts `viper.IsSet("consistency")` is false for `config.example.yaml`.
- ✅ `internal/consistency/catalog.go`: `CatalogConfig{Version, Equivalences, Topics, Rules}` compiled to a
  `Compiled`. Validation, each error naming the section and the id: `version >= 1`; ids match the taxonomy id
  pattern (`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`) and are unique per section; an equivalence has at least two distinct
  members; relations from the closed set (`same | >= | <= | = | required`); `enforced` keys are the platform
  families of `scope.go` (`windows | macos | ios | android | linux`); `reference` starts with
  `https://learn.microsoft.com/`; `status` is `verified | verify`; every topic has a label and at least one
  match rule with a non-empty field, and every regex compiles; a rule has a description, its `left` / `right`
  each name an existing topic or a non-empty key list (each key validated like a member), `class` is
  `requirement | configuration` when set, `platforms` compiles, and `violation` and `resolution` are non-empty.
  The compiled equivalences are the existing `consistency.Equivalence` values; reference and description stay in
  the catalog.
- ✅ Member validation shares the index's key construction, never a restatement: `index.go` gains small key
  builders the indexers call — `catalogKey(id)` (lowercase; Settings Catalog, compliance and ADMX ids),
  the existing `normaliseOMAURI`, `typedKey(odataType, path)` (`#microsoft.graph.<Type>#<dotted.path>`, also the
  Apple custom `…#payload:<identifier>` form), and the intent id verbatim — and `validMemberKey(key)` accepts a
  key exactly when it is already in one of those forms: a `#microsoft.graph.<Type>#<path>` key with a
  non-empty path, an intent id (contains `--`), or otherwise a key equal to its `normaliseOMAURI` form
  (lowercase, no `/`, no leading `./`). A raw OMA-URI or a mixed-case Settings Catalog id is rejected with the
  canonical form in the error, so the operator can paste it.
- ✅ A pure topic resolver `(*Compiled).Topics(r TopicFacts)`: `TopicFacts` carries the resource's name, type,
  `@odata.type`, platforms and the canonical keys of its indexed settings; the semantics are the taxonomy's (OR of
  rules, AND within one rule, `name` / `odataType` / `platforms` / `key` case-insensitive regexes, `type` exact,
  `key` matching when any of the resource's keys matches). It returns every matching topic id (a resource may sit
  in several) sorted by id; exported for *The consistency analysis job*, tested here.
- ✅ A `catalog:` block in `consistency/metadata.yaml`: `sha256` of the catalog's canonical JSON — every field
  (descriptions, references and statuses included, since the LLM step judges against them), entries sorted by id
  per section, member and key lists sorted, map keys sorted — so neither YAML key order nor entry order moves it;
  the counts per section; and `unmatchedMembers`, the sorted unique member keys no indexed setting of this export
  carries. Absent without a catalog, which means mechanical findings only. Additive, the metadata `version`
  unchanged; the command logs one catalog line (counts, unmatched members, `verify` entries), also under
  `--dry-run`.
- ✅ The seed catalog in `go/config-tailored-intune.yaml` — equivalences: the macOS and iOS/iPadOS password and
  passcode family first (compliance ↔ Settings Catalog ↔ legacy device restrictions), then encryption
  (FileVault/BitLocker), firewall, minimum OS version; topics: encryption, Defender/EDR, firewall, Windows Update,
  identity & sign-in, browsers, Office/OneDrive, enrollment, macOS accounts/SSO; rules R1–R8 — each entry's
  semantics and reference checked against Microsoft Learn where the implementer can; anything not settled stays
  `status: verify`, never guessed.
- ✅ Tests: validation errors (unknown relation, a one-member equivalence, a raw OMA-URI member with the canonical
  form in the message, an unknown `enforced` platform, non-Learn reference, duplicate id, a rule naming a missing
  topic, an uncompilable regex); every key the index fixtures produce passes member validation; compilation; hash
  stability (reordered YAML keys and entries hash the same, a changed reference does not); the metadata block
  present and absent; `unmatchedMembers`; a seed macOS password equivalence turns a fixture compliance /
  configuration pair into a `contradiction`, `possible` under `status: verify`; topic resolution (several topics,
  a `key:` rule, no match); the command refuses an invalid section with exit 2 before resolving the export; the
  tracked `config-tailored-intune.yaml` `consistency:` section compiles without error (read by relative path like
  the example-file tests). Every fixture is built in a temp directory; no test reads `go/.config/` or `output/`.
- ✅ Documentation at *done*: `README.md` — the key, its three sections, the closed vocabularies, the seed catalog in
  the worked example, and under *Working with several tenants* one sentence that inside a checkout `go/.config/` is
  the git-ignored `--config-dir`; `CHANGELOG.md` `### Added` with the operator action in bold: copy the
  `consistency:` section from `config-tailored-intune.yaml` into your base file (the catalog is opt-in).
- ✅ Last, after every other bullet: write the Claude Cowork review brief `Claude
  outputs/consistency-catalog-review-instructions.md` (git-ignored, never committed — input for a Cowork session,
  not repository documentation). It tells Cowork to read the seed catalog in `go/config-tailored-intune.yaml`
  (`consistency:` — equivalences, topics, rules), the README's description of the key and its closed vocabularies,
  and the indexed source types; to check every entry against Microsoft Learn — that the members of each
  equivalence describe the same control, the relation and operator, which compliance settings are enforced on
  which platform (macOS, iOS/iPadOS, Windows, Android), each rule's violation and resolution semantics ("which
  setting wins"), and that every reference URL resolves and supports the claim; to name missing high-value
  equivalences and rules; and to return a change plan in the shape of the earlier reviews — per entry a verdict
  (keep / fix / drop / add), ready-to-paste YAML, `status: verified` only with a cited Learn source, open
  questions where Learn is silent — with no repository edits and nothing read under `output/`.
- ↪ Follow-up (found while seeding R6 and R7): a rule selector can name only a topic or a key list, so it cannot
  say "any resource the filter is attached to" (R6) or "the dynamic group an assignment targets" (R7); decide
  between an assignment-relative selector and the mechanical scope model (now the first bullet of entry 2, *The
  consistency analysis job*).

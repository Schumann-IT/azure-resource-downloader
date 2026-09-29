---
name: add-config-option
description: Add, rename, re-scope or change a configuration key of the Go CLI (internal/config key partition, defaults, both example files, README). Use for any azure-rd setting change, including ones that first looked like a flag.
---

# Add or change a config option

Request: `$ARGUMENTS`. Work in `go/`; the configuration rules in `go/CLAUDE.md` and the example-file
no-op promise in `.claude/rules/go-export-safety.md` apply.

1. Add the field to `models.PipelineConfig` (or the struct the option belongs to).
2. **Register the key in `internal/config`'s `keyScopes` table** — the single truth for the partition — on
   the correct side:
   - `ScopeTenant` (profile `<config-dir>/<domain>.yaml` only): names something inside one tenant, so another
     tenant's value is meaningless or harmful (`subscription`, `client-id`, `tenant-id`, `filters`).
   - `ScopeGeneral` (base file only): behaviour, tuning, export format (`output`, `type`, `workers`,
     `timeout`, `transformers`, `taxonomy`, …).
   - `ScopeFlagOnly`: not configuration; listed so a file still carrying it gets a migration message.
   Placement is not cosmetic: a key hashed into `transformConfigSha256`/`filtersSha256` or deciding the export
   layout breaks comparability or relocates the export if it lands on the wrong side.
3. Default in `config.SetDefaults` if the zero value is wrong — but **no default for a key whose mere presence
   is meaningful** (`viper.IsSet` is how that is detected; see `workers`).
4. Read it with `viper.Get*`; branch on `viper.IsSet` for "was it set explicitly", never on a duplicated
   literal (named defaults live in `internal/config`, re-exported by `cmdutil`).
5. Use it in the pipeline or command.
6. **Update both example files** — `config.example.yaml` (general) and `config.example.domain.yaml`
   (tenant-scoped) — on *any* option change. Every active key sets the built-in default so loading the file is
   a true no-op (every hash in `metadata.yaml` included); dangerous or presence-sensitive keys are commented
   out; every option is illustrated by a comment (what it does, alternatives, the flag that overrides it if
   any). `cmd/config_test.go` asserts each partition key is mentioned in the right file — extend it.
7. `README.md`: the *Configuration & precedence* section, the partition table, and the *Where each setting
   went* migration table if a setting moved or a flag was removed.
8. `CHANGELOG.md` under `## [Unreleased]`; a removed flag or moved key is `### Breaking` and states in bold
   what pipelines must change.
9. Tests: partition validation for the new key (rejected on the wrong side), default behaviour, explicit-set
   detection where relevant. Verify the no-op promise when in doubt: a run with `--config config.example.yaml`
   must produce the same files and the same `metadata.yaml` hashes as a run without it. `make check`.

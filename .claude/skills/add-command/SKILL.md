---
name: add-command
description: Add or change a Cobra command or flag in the Go CLI (go/cmd), following the noun-grouped surface, the flag-placement rules and the tenant resolver. Use when asked for a new azure-rd command, subcommand or flag.
---

# Add a CLI command or flag

Request: `$ARGUMENTS`. Work in `go/`; `go/CLAUDE.md` (command-line and configuration surface) applies.

1. **Prefer an existing group** over a new top-level verb: `resource` (acts on a tenant's Azure resources)
   or `docs` (acts on an export's documentation). Parents live in package `cmd` (`cmd/resource.go`,
   `cmd/docs.go`); each subcommand is its own package in `cmd/resource/` or `cmd/docs/` exposing an exported
   constructor (`NewXCommand`) that the parent attaches. A subcommand package must **not** import package
   `cmd`; shared helpers come from `internal/cmdutil`, the version from `internal/version`.
2. **Default to a configuration setting, not a flag.** A flag must be bootstrap (`--config`, `--config-dir` —
   there is never a third), selector (`--domain`, registered with `cmdutil.RegisterDomainCompletion`), or
   invocation-scoped (changes verbosity, side effects or destination without changing *what* is produced, or
   has a per-command meaning, or is ad-hoc selection). Anything else → `/add-config-option`.
3. **Declare the flag where it is honoured**: on the group parent (persistent variants in `cmdutil`) only
   when every subcommand honours it, otherwise on the command's own `Flags()`. Never add a flag a command
   ignores; `cmd/resource_test.go` asserts both directions — extend it.
4. **Read flags from the command**, not from Viper, unless config-backed (`--output`, `--type` only):
   `cmd.Flags().Get*`, `cmdutil.DeclaredDomain(cmd)`. Do not reintroduce a `BindFlags` helper.
5. **`RunE`**: configuration from Viper (loaded by root's `PersistentPreRunE`); a command acting on a tenant
   takes `--domain` and resolves its directory through `internal/tenantdir` (never join `<output>` and a
   domain by hand); downloads and drift go through `internal/runprep`; return errors (`cmdutil.WithExitCode`
   for a distinct code), never `os.Exit`. Honour `--dry-run`; a destructive command lists exactly what it
   would remove and shares one eligibility decision between preview and real path.
6. Examples in the command's `Long` description; `README.md` *Commands* section (flags table, exit codes,
   what a run prints); `CHANGELOG.md` under `## [Unreleased]` (a renamed or removed command or flag is
   `### Breaking` and says in bold what scripts must change).
7. Tests for the flag surface and the command's decisions (no network; fixtures in temp dirs).
   `make check`, `make build`, run `./azure-rd <group> <command> --help`.

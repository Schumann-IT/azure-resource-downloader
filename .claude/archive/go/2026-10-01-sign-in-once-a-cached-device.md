---
title: Sign in once: a cached device-code session, and a measured answer on scoped az login
project: go
status: done
started: 2026-10-01
finished: 2026-10-01
branch: docs/plan-dependency-updates
pr: 39
changelog: Unreleased
---
## Sign in once: a cached device-code session, and a measured answer on scoped az login

**Goal.** Signing in stops being a per-run chore, by two routes pursued together: the dedicated-app sign-in is
done once with device code and later runs get their tokens silently from a persistent, OS-protected token cache
(until the refresh token expires or is revoked); and the tool shows, for the selected handlers, which declared
Graph permissions the `az login` session's token actually carries — so one recorded experiment settles whether a
scoped `az login` could stand in for the dedicated app. The documentation stops contradicting itself and stops
naming flags that no longer exist.

> **Why.** Two sign-in paths exist: the `az login` session (`AzureCLICredential`, used when the tenant profile has no
> `client-id`) and a device-code sign-in to a dedicated app registration (`client-id` + `tenant-id` in the profile;
> `newCredential`, `internal/azure/client.go:156-185`). The device-code credential has no persistent cache, so every
> run prompts again. Every Graph type is treated as needing the dedicated app — `worksWithCLICredential` is never set
> (`internal/handlers/graph/collection.go:35-42, 101-103`), `DedicatedAppRequirements` is static metadata
> (`internal/handlers/registry.go:100-116`), `VerifySession` only mints a token (`client.go:51-59`) — and nothing
> decodes a token's `scp`; `--debug` (`cmd/root.go:204-283`) prints identity claims only. The README contradicts
> itself: `README.md:540-546` says the Intune and policy scopes are not consentable for the first-party CLI app, so a
> CLI token can never carry them, while `:679-681` advises `az login --scope https://graph.microsoft.com/.default`
> for "required scopes are missing" on an ARM-only run, where no Graph call happens.
>
> **Reconciled from the parked idea** *review the sign-in surface — can a scoped `az login` replace the dedicated-app
> device-code path?*: the `--client-id` / `--tenant-id` flags and `AZURE_RD_*` variables it names are gone —
> `client-id` / `tenant-id` are tenant-scoped config keys only (`internal/config/keys.go:62-63`), and tests assert
> the flags and variables are absent; and azidentity's CLI credential calls `az account get-access-token
> --resource …`, never `--scope`, so whatever a scoped login granted must appear in the token minted for the Graph
> resource. Stale wording is still live: the user-facing error at `client.go:168` (names the removed flags and
> `AZURE_RD_TENANT_ID`), the `cmd/root.go:52` help text, comments in `models/types.go:174`, `registry.go:99`,
> `collection.go:41`, `cmd/resource/list.go:97`, and `README.md:544` ("the flags are unset").
>
> **Decision.** How far the review goes: instrument, run the experiment, then decide — the prompt's scoped-login
> recommendation is built only if the experiment says yes.
>
> **Decision.** The user added a cached device-code session (sign in once, later runs silent), independent of the
> experiment's outcome.
>
> **Out of scope, revisit note.** Removing the dedicated-app path would be breaking (two tenant-scoped keys go) and
> needs the experiment's "yes" on a representative tenant first; record it under `Breaking` if it ever ships.
>
> Not regeneration-gated: no template and no `promptSha256` moves, so nothing needs to ride along.
>
> **Token cache, verified at review (2026-10-01).** azidentity `v1.14.1` (in `go.mod`) has everything the session
> needs: `DeviceCodeCredentialOptions.Cache` (type `azidentity.Cache`), `.AuthenticationRecord`,
> `.DisableAutomaticAuthentication`, `(*DeviceCodeCredential).Authenticate` → `azidentity.AuthenticationRecord`, and
> `*azidentity.AuthenticationRequiredError`. The persistent store is the separate module
> `github.com/Azure/azure-sdk-for-go/sdk/azidentity/cache` (import path the same, package `cache`; latest `v0.4.0`,
> requires azidentity ≥ v1.13.1, azcore ≥ v1.21.0 — satisfied), constructor `cache.New(&cache.Options{Name:
> "azure-rd"})`, which round-trips test data once and returns an error when no secure store works. New indirect
> modules: `github.com/AzureAD/microsoft-authentication-extensions-for-go/cache v0.1.1` and
> `github.com/keybase/go-keychain v0.0.1`. Storage per platform: macOS — the login Keychain (via go-keychain,
> **cgo required**: the extensions' darwin accessor is `//go:build darwin && cgo`, so a `CGO_ENABLED=0` darwin build
> of the package fails to compile); Windows — a DPAPI-encrypted file under `%LOCALAPPDATA%\.IdentityService\`;
> Linux — **not libsecret**: an AES-encrypted file under `$XDG_CACHE_HOME` (or `~/.cache`)`/.IdentityService/` whose
> key lives in the kernel persistent/user keyring (`keyctl`, pure Go via `golang.org/x/sys/unix`, no cgo), so on
> Linux the session survives across runs and logins but **not a reboot** (or keyring expiry); where `keyctl` is
> unavailable (some containers) `cache.New` errors. The module compiles only for darwin, linux and windows. CI
> (`ubuntu-latest`, `make -C go ci`, golangci-lint) is unaffected: the Linux path needs no cgo or system library,
> and tests never call `cache.New`.
>
> **Contract.** None with `web/`: nothing under the export tree (`output/<tenant>/{resources,docs,drift}/`), no
> `metadata.yaml` or `index.yaml` field, file name or exit code changes. The only new on-disk artefact is the
> authentication record at `os.UserConfigDir()/azure-rd/auth/<tenant-id>-<client-id>.json` (mode 0600, directory
> 0700; fields per `azidentity.AuthenticationRecord`, no token or secret) plus the SDK-owned token cache named
> `azure-rd` — both outside the repository, the `--config-dir` and `output/`, and neither read by `web/`.
>
> **Owner.** none — no file outside `go/`. Sequencing: no dependency on `web/`. Entry 5 (routine dependency
> updates) also edits `go/go.mod` / `go/go.sum`; whichever lands second on a shared branch reruns `make deps`.
>
> **Who does what.** The implementer delivers the token-claims decoder, the coverage helper, the `--debug`
> sections, the cached device-code session and the stale-wording fixes, with tests, and strikes those bullets.
> The cache check and the scoped-login experiment need a live tenant and a human sign-in: they stay unstruck for the
> operator, who records the results in those bullets. The conditional follow-up stays unstruck and unbuilt until the
> experiment is recorded; the documentation bullet stays unstruck until *done*.
>
> **Implementer.** opus

**Plan.**

- ✅ Token claims: `internal/azure/identity.go` gains a pure decoder for a Graph token's `appid`, `app_displayname`
  and `scp`, beside `parseIdentityClaims`; tested with synthetic JWT payloads. The token itself is never logged.
- ✅ Coverage helper (pure): given declared permissions and a scope list, return covered and missing; a `ReadWrite`
  scope covers its `Read` counterpart, matching is case-insensitive; table-tested.
- ✅ `--debug` (`runDebugReport`): a "Graph token" section with `appid` / `app_displayname` and the sorted `scp`, and
  for the effective type selection (`runprep.SelectTypesFromConfig` plus `DedicatedAppRequirements`) every declared
  permission marked covered or missing. It runs on the CLI credential when the profile has no `client-id`, otherwise
  on the dedicated app's token, and writes nothing. The section is built by a pure function from the decoded claims
  and the coverage result (tested with fixed inputs, sorted output); `runDebugReport` only fetches the Graph token
  and logs the result.
- ✅ Dependency: add `github.com/Azure/azure-sdk-for-go/sdk/azidentity/cache` (latest release, `v0.4.0` at review) as
  a direct requirement, then `make deps`; `go.sum` gains the module and its two new indirects (Keychain accessor,
  go-keychain). No other module version moves.
- ✅ Platform split, so a build without the store still compiles: the only file importing `azidentity/cache` is
  `internal/azure/tokencache_supported.go` (`//go:build (darwin && cgo) || linux || windows`), exposing
  `newPersistentCache() (azidentity.Cache, error)`; `internal/azure/tokencache_other.go` (the negated constraint)
  returns an error "persistent token cache unavailable on this platform/build" — the "no secure store" path below.
  The factory is a package variable so tests replace it and never touch a real store.
- ✅ Authentication record store (`internal/azure/authrecord.go`): `authRecordPath(tenantID, clientID)` →
  `os.UserConfigDir()/azure-rd/auth/<tenant-id>-<client-id>.json` (directory created 0700, file written 0600 via a
  temp file and rename); `loadAuthRecord` returns the record, or none when the file is missing, unparsable, or its
  `TenantID` / `ClientID` differ from the profile (case-insensitive) — a mismatched or corrupt file is treated as
  absent and overwritten on the next sign-in, never trusted. Never under the repository, `--config-dir` or
  `output/`; it holds no token or secret.
- ✅ Cached device-code session in `newCredential` (only when the profile names a `client-id`; no new config key):
  build the persistent cache (`Name: "azure-rd"`); on error, warn once ("token cache unavailable: <reason>; signing
  in on every run") and construct today's credential unchanged — never store tokens unencrypted. With a cache, wrap
  an `azidentity.DeviceCodeCredential` built with `Cache`, the loaded `AuthenticationRecord` (if any) and
  `DisableAutomaticAuthentication: true` in a `cachedDeviceCodeCredential` implementing `azcore.TokenCredential`:
  `GetToken` tries the inner credential silently; on `*azidentity.AuthenticationRequiredError` (no record, refresh
  token expired or revoked) the interactive variant (`NewCredential`) calls `Authenticate` with the Graph
  `.default` scope — the device-code prompt — under a mutex so concurrent `GetToken` callers trigger exactly one
  prompt, saves the returned record (replacing the old one), and retries `GetToken`; the non-interactive variant
  (`NewNonInteractiveCredential`) returns the error unchanged and never prompts, but now succeeds silently whenever
  a cached session exists. Construction stays network-free (`NewCredential`'s contract). Any other error passes
  through unchanged.
- ✅ `--debug` token-cache line: "Token cache" `active (record from <file mtime, date>)`, `no session yet (the next run
  signs in once)` or `unavailable (<reason>)`; printed only when the profile names a `client-id`.
- ✅ Tests (all with the cache factory and the inner credential stubbed; no real tokens, no real store): record
  round-trip; path shape, directory 0700 and file 0600 (skipped on Windows); tenant mismatch, client mismatch and
  corrupt file load as absent; cache-unavailable falls back to the plain credential and warns once; silent success
  never calls `Authenticate`; `AuthenticationRequiredError` → one `Authenticate`, record saved, retry succeeds;
  non-interactive variant never calls `Authenticate`; N concurrent `GetToken` calls on a fresh session call
  `Authenticate` once (run under `make test-race`); other errors pass through.
- ✅ Operator step, not for the implementer — stays unstruck until the result is recorded here: on cb-gmbh.com, with
  the dedicated app in the profile, sign in once, then a second `resource download --dry-run` must not prompt;
  record the platform and whether it prompted. The profile's `tenant-id` must be the tenant GUID: the stored record
  carries the GUID, so a domain never matches it and every run would prompt.
  **Result (2026-10-01, cb-gmbh.com, macOS):** the cache works — after the one device-code sign-in, later runs got
  their tokens silently. The number of prompts on the very first run was not recorded (see the CAE follow-up).
- ✅ The experiment, an operator step not for the implementer — stays unstruck until recorded here: on cb-gmbh.com
  `az logout && az login --scope https://graph.microsoft.com/.default` (explicit scopes if that is refused), then `azure-rd --debug --config-dir …
  --domain cb-gmbh.com` with no `client-id` in the profile; record `appid`, the `scp` list and the covered/missing
  table. If everything is covered, also run one `resource download --dry-run` per dedicated-app type without
  `client-id` and record the outcome — a service may gate on the calling application, not only on the scopes.
  **Result (2026-10-01, cb-gmbh.com):** after `az logout && az login --tenant <tenant GUID> --scope
  https://graph.microsoft.com/.default`, `azure-rd --debug` (no `client-id` in the profile) reported the Graph token
  as `app_id=04b07795-8ddb-461a-bbee-02f9e1bf7b46` ("Microsoft Azure CLI") with the CLI app's fixed scope set —
  `AppRoleAssignment.ReadWrite.All Application.ReadWrite.All AuditLog.Read.All DelegatedPermissionGrant.ReadWrite.All
  Directory.AccessAsUser.All Group.ReadWrite.All SubjectNameRegistration.ReadWrite User.Read.All User.ReadWrite.All`
  plus `email openid profile` — and `Graph permission coverage covered=1 missing=50`: only `Group.Read.All` (groups)
  is covered; every `DeviceManagement*`, `Policy.Read.All`, `Agreement.Read.All`,
  `OnPremDirectorySynchronization.Read.All`, `Organization.Read.All` and `OrganizationalBranding.Read.All` is missing.
  The scoped login added nothing to the CLI app's token. **Answer: no** — a scoped `az login` cannot stand in for
  the dedicated app on this tenant; the per-type dry runs are not needed. (`Directory.AccessAsUser.All` might let a
  few Entra reads through in practice; that does not change the answer for the Intune and policy types.)
- ✅ Stale wording: the `client.go:168` error reads "tenant-id is required when client-id is set in the tenant profile";
  the `cmd/root.go:52` help text and the comments in `models/types.go`, `registry.go`, `collection.go`,
  `cmd/resource/list.go` name the profile keys, not flags. A test asserts the new error text and that neither it nor
  the root help mentions `--client-id`, `--tenant-id` or `AZURE_RD_`.
- ✅ Documentation at *done*: `README.md` — resolve the contradiction (`:540-546` vs `:679-681`) with the measured
  result; `:544` "the profile sets no `client-id`"; the `--debug` section documents the Graph token section and
  replaces the shell token-decoder one-liner (`:518-523`); the cached device-code session (where the record and the
  tokens live, how to forget a session by deleting the record, the fallback without a secure store, that on Linux
  the session lasts until reboot, and that a macOS build needs cgo — the default with Xcode's command-line tools —
  to get the Keychain cache). `CHANGELOG.md`:
  `### Added` (the cached device-code session; the `--debug` Graph token and coverage section), `### Fixed` (the
  error message and help text naming removed flags).
- ✅ Follow-up, found while implementing — operator check first: azidentity keeps CAE and non-CAE tokens in separate
  cache partitions (`azure-rd` and `azure-rd.cae`, separate MSAL clients), and the sign-in authenticates only the
  partition of the request that needed it (the request's `EnableCAE` is carried into `Authenticate`). A run whose
  token requests mix both (the Graph SDK and azcore's bearer policy request CAE tokens; `VerifySession` and
  `SignedInIdentity` do not) may therefore prompt twice on its first sign-in, and silently after that. The cache
  check above should record how many prompts the first run showed; if two, sign in once and seed the other
  partition from the same session, or drop the mixed request.
  **Result (2026-10-01, cb-gmbh.com, macOS):** with the session forgotten (sign-in record and both Keychain items
  deleted), the first `resource download --dry-run --log-level debug` showed exactly **one** device-code prompt and
  listed all 50 types; the second run did not prompt at all. The two cache partitions do not cause a second prompt
  in practice; nothing to change.

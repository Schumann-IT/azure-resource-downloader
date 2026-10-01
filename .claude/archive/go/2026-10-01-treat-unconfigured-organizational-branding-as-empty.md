---
title: Treat unconfigured organizational branding as empty, and keep why a type could not be listed
project: go
status: done
started: 2026-10-01
finished: 2026-10-01
branch: docs/pre-regeneration-review
pr: 37
changelog: Unreleased
---
## Treat unconfigured organizational branding as empty, and keep why a type could not be listed

**Goal.** `Microsoft.Graph/organizationalBranding` is listed as "could not be listed" in every run of both
reference tenants, which marks every export and every drift run **incomplete**. A tenant without a configured
default branding must export as *empty*, and whenever a type really cannot be listed, the reason must survive
the run instead of being lost after one debug line.

> **Why.** `organization` exports fine with the same token, so `Organization.Read.All` is present. The branding
> handler assumes Graph answers an empty body when no branding is configured
> (`internal/handlers/graph/organizationalbranding.go:21-25, :59-75`); Graph documents a **404** for that case.
> Every listing error lands in the "could not be listed" path (`internal/handlers/requests.go:115-123`) with a hint
> that blames the permission; `azure.ErrorSummary` extracts no HTTP status from Graph SDK errors
> (`internal/azure/errors.go:65-105`); and `notListed` keeps type names only (`internal/docs/metadata.go:159-163`),
> so the real error was never visible. The debug run recorded in the Plan confirmed the 404.
>
> Not regeneration-gated: no `*_prompt.tmpl` or `documentation_prompt.tmpl` changes, no `promptSha256` moves.
> The one prompt-adjacent change is the "Types not listed" line of the non-hashed `docs/generate.md` render
> (`internal/docs/generateprompt_render.go`), which the run-prompt entry also touches — whichever lands second
> rebases. Independent of the other entries; can ship first.
>
> **Scope.** The `--type` run that lists only empty types belongs here: the debug run showed that a tenant with
> unconfigured branding, downloaded with `--type Microsoft.Graph/organizationalBranding`, ends in
> `Error: no resources to download` (exit 1) and writes no metadata — so the fix would still not give a clean,
> complete run, and a branding that was configured earlier and then removed could never be marked absent.
> The drift observation's `unknownTypes` stays a plain list (no reasons): the browser reads it as `string[]`,
> and drift prints the improved summary in its own warning.
>
> **Contract.** `resources/metadata.yaml` gains one key, additive: `notListed.reasons`, a map from each type in
> `notListed.types` to its error summary (always written, `{}` when nothing failed; keys sorted). Shape:
> `notListed: {types: [<type>…], empty: [<type>…], reasons: {<type>: "<summary>"}}`. `notListed.types` and
> `notListed.empty` keep their names and their `[]string` shape, so older readers and older files keep
> working (a file without `reasons` reads as an empty map). The summary is deterministic for the same error:
> `HTTP <status> <code>: <message first line>` for typed Graph/ARM errors, the hint appended only for a 401/403.
> An unconfigured branding now appears under `notListed.empty`, not `notListed.types`, and its run is
> `complete: true`. The browser (`web/src/docs/resources-metadata.ts`) reads only the `resources:` map and the
> drift observation's `unknownTypes`; neither changes, so no web change follows.
>
> **Decision.** Pre-existing TestErrorSummary assertions that encode the old format: adjust them to the new format (Option B).
>
> **Owner.** none — every file is under `go/`. No sequencing constraint.
>
> **Implementer.** opus — error classification, the metadata schema, and a change to when a download writes
> metadata (and so can mark resources absent and prune) are invariant-bearing.

**Plan.**

- ✅ **User action, first:** run `./azure-rd resource download --type Microsoft.Graph/organizationalBranding
  --log-level debug --dry-run` against one reference tenant and record the "Listing failed" line in this entry.
  **Result (2026-10-01, cb-gmbh.com):** `failed to get organizational branding: Resource '<organization id>' does
  not exist or one of its queried reference-property objects are not present. (hint: requires
  'OrganizationalBranding.Read.All' permission in Microsoft Graph)` — Graph's `Request_ResourceNotFound` (404)
  text: no default branding is configured. Not a permission problem; the bullets below apply as written. The
  same run ended with `Error: no resources to download` — handled by the download bullet below.
- ✅ `internal/azure`: add `HTTPStatus(err error) (int, bool)` — the status of an `*azcore.ResponseError` or of
  any error matching Kiota's `abstractions.ApiErrorable` via `errors.As` (covers both the v1.0 and the beta
  `odataerrors.ODataError`, which embed `abstractions.ApiError`) — and `GraphErrorCode(err error) string`,
  reading `GetErrorEscaped().GetCode()` from either SDK's `*odataerrors.ODataError` (empty when absent).
  `IsPermissionError` also returns true for a typed 401/403 from `HTTPStatus`.
- ✅ `azure.ErrorSummary`: for a typed error (azcore or Graph) return `HTTP <status>`, then ` <code>` when known,
  then `: <first line of err.Error()>` (the handler's wrapping context included) with any `(hint: …)`
  stripped from it; append the handler's
  `(hint: …)` only when the status is 401 or 403. For an untyped error keep the first-line / JSON-`Message`
  fallback and append the hint only when `IsPermissionError(err)` is true. Update the doc comment.
- ✅ Branding handler (`organizationalbranding.go`): move the list decision into a pure function
  `brandingListIDs(branding betamodels.OrganizationalBrandingable, err error) ([]string, error)`: a 404
  (`azure.HTTPStatus`) → `nil, nil` (no default branding configured — the type lists as *empty*); any other
  error → wrapped as today, with the permission hint; `nil` body → `nil, nil`; otherwise the ID or the
  `organizationalBranding` fallback. In `fetchItem` a 404 stays an error but reads
  `organizational branding is not configured` (no permission hint) — the branding vanished between list and
  fetch. Correct the handler's doc comment (404, not an empty body).
- ✅ `resource download` (`cmd/resource/download.go`): return `no resources to download` only when the listing
  produced no requests **and** no type listed as empty (every type in scope could not be listed, or a
  `--resource-group` run without a subscription). When only empty types were listed, skip the pipeline
  (`summary := &pipeline.ExecutionSummary{Results: []*models.WriteResult{}}`; under `--dry-run`
  `pipeline.DryRunSummary(nil)`), attach skipped/empty types, mark completeness, print the summary, write the
  metadata (the empty types are covered, so their earlier entries are marked absent and — with `prune` on a
  complete run — pruned, exactly as in a full run) and exit 0. Put the decision in a small pure helper
  (e.g. `nothingListed(requests, emptyTypes)`) so it is table-testable.
- ✅ `resources/metadata.yaml` (`internal/docs/metadata.go`): `NotListedMeta` gains
  `Reasons map[string]string \`yaml:"reasons"\`` filled from `SkippedType.Reason` (already
  `azure.ErrorSummary(err)`; "no subscription available" for ARM types without a subscription), never nil so
  it marshals as `{}`. Reading a file without the key must not fail.
- ✅ `docs generate-prompt` render (`internal/docs/generateprompt_render.go`): the line becomes
  `Types not listed: <type> (<reason>), …` (drop the "(permissions)" claim); `none` when empty. Adjust the
  existing assertion in `internal/docs/generateprompt_test.go`.
- ✅ Tests: `brandingListIDs` with a constructed beta `ODataError` at 404 (→ no IDs, no error), at 403 (→ error
  whose `ErrorSummary` carries `HTTP 403` and the hint), a nil body and a body with an ID; `HTTPStatus`,
  `GraphErrorCode` and `ErrorSummary` with v1.0 and beta `ODataError`s (404 without hint, 403 with hint), an
  azcore 404 (no hint) and 403 (hint), and the untyped fallback; `IsPermissionError` with a typed Graph 403;
  `nothingListed` table (requests only / empty types only / both empty / neither); metadata round-trip of
  `notListed` with `reasons`, `reasons: {}` when nothing failed, and an old file without `reasons` loading.
- ✅ Follow-up (found while implementing): four pre-existing `TestErrorSummary` subtests in
  `internal/azure/errors_test.go` encode the old summary format and fail against the `ErrorSummary` bullet —
  the two azcore cases expect a bare `HTTP 403 AuthorizationFailed` (no `: <first line>`), and the two
  multi-line Intune cases expect the hint although their synthetic messages match no permission marker. The
  user decides: adjust those assertions to the new format (and give the Intune fixtures a real
  permission message), or change the bullet's format.
- ✅ Documentation at *done*: `README.md` (the branding row's "no file when unconfigured" note, troubleshooting a
  not-listed type) and a `CHANGELOG.md` `### Fixed` entry.

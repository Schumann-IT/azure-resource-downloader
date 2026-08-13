# Next iterations

Open work only. Everything delivered so far is recorded in `CHANGELOG.md`.

`DOC-GENERATION-PROMPT.md` remains untouched, and where generated documentation lives is still deliberately
undecided.

---

## 1. Two missing tests

Both were specified with the work they cover and are the only gaps left in it.

**The env-key-replacer guarantee.** `initConfig` maps hyphens to underscores so `log-level` resolves to
`AZURE_RD_LOG_LEVEL`. Nothing tests it, so the fix can silently regress into the exact bug it replaced — an
override that is documented, believed, and inert. Set the variable, assert it is read.

**Determinism of `metadata.yaml`.** Two runs over an unchanged export must produce byte-identical output.
`summary.Results` arrives in worker-completion order, and slices are sorted on the way in, but nothing pins
that down. Without the test, a future field added as an unsorted slice churns the file in git on every run
and nobody notices until the diffs are unreadable.

## 2. `list` requires authentication to print a compile-time registry

`runList` constructs an Azure client purely to "ensure authentication works", then prints
`registry.GetAllTypes()` — a list baked into the binary. So `azure-rd list` fails without `az login`, or
offline, to answer a question that needs neither. If `handlers.NewRegistry` tolerates zero-value credentials
for enumeration, `list` becomes usable as documentation.

## 3. `rootCmd.Version` is hardcoded

`toolVersion()` correctly makes the root command's version the single source of truth for what
`metadata.yaml` records — but that version is the literal `"1.0.0"` and never moves when the binary does.

This defeats a specific thing the metadata was built for. `promptSha256` comes from
`handler.GetDocumentationPrompt()`, a compile-time constant, so a tool upgrade changes the prompt hash for
every affected type at once. `toolVersion` is what lets a later step attribute that to an upgrade rather than
report it as content drift — and it can only do that if it actually changes. Wire it to
`debug.ReadBuildInfo()` or an `-ldflags -X` value.

---

## Later, deliberately not decided here

Where generated documentation lives; how a post-processing step consumes `metadata.yaml` to emit a
documentation prompt covering only changed resources; and any resulting amendments to
`DOC-GENERATION-PROMPT.md`.

---
trigger: always_on
---

# Project Context
- **Language**: Go 1.24, Module mode
- **Target**: CLI tool for downloading and transforming Azure resources to YAML
- **Architecture**: Async pipeline pattern with worker pools
- **Repo layout**:
    - `cmd/`                    → Cobra CLI commands (root, download, list)
    - `internal/models/`        → Core types, interfaces, config structs
    - `internal/pipeline/`      → 3-stage async pipeline (fetcher, transformer, writer)
    - `internal/handlers/`      → Handler registry (package handlers); ARM handlers in `arm/`, Microsoft Graph handlers in `graph/`
    - `internal/azure/`         → Azure SDK wrappers and utilities (auth, listing pagers, permission-error detection, ID resolver)
    - `internal/transform/`     → Transformation utilities (cleaner, sanitizer, base64 decoding)
    - `internal/logger/`        → Structured logging (charmbracelet/log wrapper)
    - `internal/retry/`         → Exponential backoff for transient Azure API failures
    - `main.go`                 → Entry point (calls cmd.Execute())
    - `Makefile`                → Build automation
- **External dependencies**:
    - Azure SDK for Go (azcore, azidentity, armresources, armcompute, armstorage, armsubscriptions)
    - Microsoft Graph SDK for Go (stable v1.0 + beta for Intune endpoints) + Kiota (abstractions, JSON serialization)
    - Cobra (CLI framework)
    - Viper (configuration management)
    - charmbracelet/log (structured logging)
    - gopkg.in/yaml.v3 (YAML marshaling)

# Architecture Patterns

## Pipeline Pattern
```
Input → Fetcher Stage → Transformer Stage → Writer Stage → Output
        (Worker Pool)    (Worker Pool)       (Worker Pool)
             ↓                 ↓                  ↓
         Channels          Channels           Channels
```

## Handler Registry Pattern
- Interface: `ResourceHandler` (GetType, List, Fetch, Transform, GetDocumentationPrompt)
- Registry: Central handler registry with `Register()` and `Get()` methods
- Extensibility: Add new resource types by implementing interface + registering

## Worker Pool Pattern
- Configurable concurrency with API-specific defaults (Microsoft Graph: 5, ARM: 20; see `internal/models/api.go`), overridable via `--workers` / `workers` / `workers-by-api`
- Uses `sync.WaitGroup` + goroutines + channels
- Context-aware cancellation

# Non-negotiables
- Every exported function gets a doc comment
- Use `context.Context` as first param for operations (Fetch, pipeline methods)
- Return errors, don't log+return the same error
- Use interfaces for extensibility (`ResourceHandler`)
- New handlers MUST implement all interface methods:
  - `GetType() string` - Azure resource type
  - `GetDocumentationPrompt() string` - dedicated per-type LLM documentation prompt; build via `models.BuildDocumentationPrompt(models.ResourceDocumentation{...})` (ARM: inline metadata; Graph: pass metadata via the `docMeta` helper from `internal/handlers/graph/documentation.go` in the constructor)
  - `List(ctx context.Context) ([]string, error)`
  - `Fetch(ctx context.Context, resourceID string) (interface{}, error)`
  - `Transform(resource interface{}) (*models.TransformedResource, error)`
- Register new handlers in `internal/handlers/defaults.go` → `registerDefaults()` function (NOT in `cmd`; `handlers.NewRegistry` pre-populates from there)
- ALWAYS update the "Supported Resource Types" table in `README.md` when adding (or removing) a resource type handler — this is mandatory, not optional. Include the Azure type and any required permissions/API notes. The README is the single source of truth.
- Avoid global state; handlers get their dependencies via constructor (ARM: credential + subscriptionID; Graph: credential only)
- Pipeline stages communicate via channels only (no shared state)
- Use `sync.RWMutex` for thread-safe registry access

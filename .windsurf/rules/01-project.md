---
trigger: always_on
---

# Project Context
- **Language**: Go 1.24, Module mode
- **Target**: CLI tool for downloading and transforming Azure resources to YAML + Terraform imports
- **Architecture**: Async pipeline pattern with worker pools
- **Repo layout**:
    - `cmd/`                    → Cobra CLI commands (root, download, list)
    - `internal/models/`        → Core types, interfaces, config structs
    - `internal/pipeline/`      → 3-stage async pipeline (fetcher, transformer, writer)
    - `internal/handlers/`      → Handler registry (package handlers); ARM handlers in `arm/`, Microsoft Graph handlers in `graph/`
    - `internal/azure/`         → Azure SDK wrappers and utilities
    - `internal/transform/`     → Transformation utilities (cleaner, sanitizer, terraform)
    - `main.go`                 → Entry point (calls cmd.Execute())
    - `Makefile`                → Build automation
- **External dependencies**:
    - Azure SDK for Go (azidentity, armresources, armcompute, armstorage)
    - Microsoft Graph SDK for Go (stable v1.0 + beta for Intune endpoints)
    - Cobra (CLI framework)
    - Viper (configuration management)
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
- Interface: `ResourceHandler` (GetType, Fetch, Transform, GetTerraformResourceType)
- Registry: Central handler registry with `Register()` and `Get()` methods
- Extensibility: Add new resource types by implementing interface + registering

## Worker Pool Pattern
- Configurable concurrency (default: 5 workers per stage)
- Uses `sync.WaitGroup` + goroutines + channels
- Context-aware cancellation

# Non-negotiables
- Every exported function gets a doc comment
- Use `context.Context` as first param for operations (Fetch, pipeline methods)
- Return errors, don't log+return the same error
- Use interfaces for extensibility (`ResourceHandler`)
- New handlers MUST implement all interface methods:
  - `GetType() string` - Azure resource type
  - `GetTerraformResourceType() string` - Terraform resource type
  - `Fetch(ctx context.Context, resourceID string) (interface{}, error)`
  - `Transform(resource interface{}) (*models.TransformedResource, error)`
- Register new handlers in `internal/handlers/defaults.go` → `registerDefaults()` function (NOT in `cmd`; `handlers.NewRegistry` pre-populates from there)
- ALWAYS update the "Supported Resource Types" table in `README.md` when adding (or removing) a resource type handler — this is mandatory, not optional. Include the Azure type, the Terraform resource type, and any required permissions/API notes. The README is the single source of truth.
- Avoid global state; handlers get credential + subscriptionID via constructor
- Pipeline stages communicate via channels only (no shared state)
- Use `sync.RWMutex` for thread-safe registry access

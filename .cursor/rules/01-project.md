# Project Context
- Language: Go (>=1.22), Module mode.
- Target: CLI + library with clean pkg layout.
- Repo layout:
    - cmd/root.go  → thin CLI entrypoint
    - internal/...           → app/business logic
    - pkg/...                → reusable public packages
    - configs/...            → example YAML/TOML/JSON
    - tools/...              → dev tools (lint, goreleaser)
- External deps: keep minimal; prefer stdlib.

# Non-negotiables
- Every new function gets a short doc comment.
- Use `context.Context` as first param where it makes sense.
- Return errors, don’t log+return the same error.
- Avoid global state; use small structs with interfaces.

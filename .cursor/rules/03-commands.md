# What to generate on request
- “Create a handler”: scaffold interface in `internal/handlers/handler`, implementation, constructor, and unit tests.
- “Add a CLI command”: add Cobra subcommand under `cmd/` with flags, validation, and wiring.
- “Add config”: define struct in `internal/config`, load from env + file, with defaults and validation.

# Output shape
- Provide full file paths and complete code blocks.
- Include `go mod tidy` changes when new deps are introduced.
- End with a short checklist of manual steps (if any).
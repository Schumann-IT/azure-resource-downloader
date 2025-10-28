# Security & Ops
- Never log secrets. Redact tokens, keys, passwords.
- Config precedence: flags > env > file.
- Graceful shutdown: context + `http.Server.Shutdown` with timeout.
- Observability: structured logs (`slog` preferred), include request id and latency in HTTP logs.

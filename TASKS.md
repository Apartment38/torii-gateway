# Tasks

## IN PROGRESS

### TASK-001 — Bootstrap Go project

Goal:
Create the initial Go project and repository structure.

Requirements:
- Go module
- cmd/gateway/main.go
- internal package directories required for Milestone 1
- docs directory
- project builds
- no unnecessary dependencies

Acceptance:
- `go build ./...` succeeds
- `go test ./...` succeeds

---

## TODO

### TASK-002 — YAML configuration loader

Implement:
- YAML config loading
- strict unknown-field rejection
- configuration validation

Do not yet implement provider-specific configuration beyond the minimum
structures needed for Ollama.

---

### TASK-003 — Secret references

Implement:
- environment-variable secrets
- file-backed secrets
- validation requiring exactly one source
- whitespace trimming for files

---

### TASK-004 — Gateway bearer authentication

Implement:
- Authorization: Bearer
- constant-time comparison
- OpenAI-style unauthorized response

---

### TASK-005 — Provider/model ID parsing

Implement:
- `<provider>/<model>`
- split on first slash only
- provider ID validation
- tests for OpenRouter-style nested model IDs

---

### TASK-006 — Generic OpenAI-compatible transport

Implement transparent passthrough transport.

Do not implement routing or classifiers.

---

### TASK-007 — Ollama provider

Use the generic OpenAI-compatible transport where possible.

---

### TASK-008 — SSE streaming relay

Implement low-overhead streaming passthrough and cancellation.

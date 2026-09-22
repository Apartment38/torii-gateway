# Tasks

## IN PROGRESS

No task is currently in progress.

---

## DONE

### TASK-002 — YAML configuration loader

Goal:
Load and validate the initial gateway configuration.

Completed:
- added strict single-document YAML decoding with unknown-field rejection
- added file-based configuration loading
- validated listener and state settings plus the minimal Ollama provider schema
- validated provider IDs, uniqueness, supported type, and HTTP(S) base URLs
- added focused loader and validation tests

Decisions:
- used `gopkg.in/yaml.v3`, because the Go standard library has no YAML decoder
- intentionally reject configuration for providers and subsystems that are not
  implemented yet

---

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

Completed:
- initialized the Go module and gateway command
- added the Milestone 1 package skeleton for API, configuration, providers,
  and upstream transports
- kept the module dependency-free
- documented and verified the repository-wide build and test commands

---

## TODO

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

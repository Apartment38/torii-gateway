# Tasks

## IN PROGRESS

No task is currently in progress.

---

## DONE

### TASK-004 — Gateway bearer authentication

Goal:
Protect gateway API handlers with the configured bearer API key.

Completed:
- added the extensible client authenticator and identity contracts
- added static `Authorization: Bearer` authentication for the v1 API key
- compared fixed-size API-key digests in constant time
- added OpenAI-style JSON authentication errors and bearer challenges
- added focused middleware, credential rejection, and identity tests

Decisions:
- all missing, malformed, duplicate, and invalid credentials return the same
  response to avoid exposing authentication details
- store and compare SHA-256 digests so comparisons remain fixed-length even
  when a presented credential has a different length

---

### TASK-003 — Secret references

Goal:
Load secrets referenced by environment variables or files without allowing
secret values in YAML.

Completed:
- added a reusable `SecretRef` with strict exactly-one-source validation
- added environment-variable resolution without altering the stored value
- added file resolution with surrounding whitespace trimming
- added optional server API-key secret configuration for strict YAML loading
- added focused validation, resolution, and configuration decoding tests

Decisions:
- environment values are preserved exactly; only file-backed values are
  whitespace-trimmed as required
- missing environment variables and unreadable files fail resolution without
  exposing secret values in errors

---

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

# Project Instructions

This project is a lightweight, resource-efficient OpenAI-compatible
LLM gateway written in Go.

## Primary goals

In priority order:

1. Correct OpenAI API compatibility.
2. Minimal memory and CPU usage.
3. Minimal dependencies and operational complexity.
4. Low proxy latency.
5. Simple, understandable architecture.

## Source of truth

Before architectural work, read:

- docs/implementation-plan.md
- docs/architecture.md
- TASKS.md

For provider work, also read:

- docs/provider-notes.md

## Architectural rules

- One binary and one process.
- No database in v1.
- No Redis, queues, sidecars, or microservices.
- Prefer the Go standard library.
- Preserve unknown OpenAI request fields for passthrough transports.
- Do not normalize or rebuild requests unless translation is required.
- Never buffer an entire SSE response unnecessarily.
- Never switch upstream models after streamed output has begun.
- Keep classification, model selection, and transport separate.
- Explicit provider/model requests must use exactly that model.
- Automatic rerouting is only allowed through the smart virtual model.
- Provider credentials must never be sent to another provider's endpoint.
- Do not silently drop unsupported request features.
- Do not implement future-version features unless required by the current task.
- Avoid new dependencies unless they provide a clear correctness or maintenance benefit.

## Development workflow

Before starting:

1. Read TASKS.md.
2. Identify the specific assigned task.
3. Inspect existing related code and tests.

Before finishing:

1. Run relevant tests.
2. Run `go test ./...` when practical.
3. Update TASKS.md.
4. Update documentation when the task changes architecture, public behavior,
   configuration, developer workflow, significant design decisions, or another
   durable project convention. Do not add narrow task-specific test commands to
   `docs/development.md` unless they are useful as a reusable development
   procedure.
5. Leave the repository in a buildable, resumable state.
6. Make one focused commit if Git commits are permitted.

If a shared interface appears inadequate, do not casually redesign it inside
a provider-specific task. Document the issue first.

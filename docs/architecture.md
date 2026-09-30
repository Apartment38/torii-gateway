# Architecture

## Purpose

This document distinguishes the architecture currently implemented in the
gateway from the approved Version 1 target. The repository is pre-v1: only the
explicit Ollama Chat Completions path is currently usable. Sections labeled
"planned" are contracts, not claims about the running product.

For the complete target and ordered implementation work, see:

- `docs/implementation-plan.md`
- `TASKS.md`

## Design Goals

The project prioritizes:

1. Correct OpenAI API compatibility.
2. Low CPU and memory usage.
3. Low proxy latency.
4. Minimal dependencies.
5. Minimal operational complexity.
6. One process and one binary.

## Process Model

The gateway runs as a single Go process.

The `gateway serve --config <path>` command loads and validates configuration,
resolves the gateway API key, constructs each configured Ollama provider, and
serves the API on the configured listener. It shuts down gracefully on SIGINT
or SIGTERM. The health endpoint remains unauthenticated; all other currently
served routes require bearer authentication. On SIGINT or SIGTERM, the process
stops accepting new connections and waits up to 10 seconds for active requests
to finish before exiting. Idle client connections are closed after 60 seconds.

No external database, Redis instance, message queue, sidecar, or worker
service is required.

## Request Flow

Current request flow:

    Client
      |
      v
    Request ID and structured request logging
      |
      v
    API authentication
      |
      v
    OpenAI-compatible HTTP handler
      |
      v
    Provider/model resolution
      |
      v
    Provider transport
      |
      v
    Upstream provider

The Version 1 planned path inserts session resolution, request-requirement
extraction, classification, deterministic selection, quota/cost policy, and
failure handling between authentication and the provider registry. `smart`
requests use that path; explicit `provider/model` requests continue to resolve
directly and can never be substituted.

## Transport Model

There are two transport categories.

### Transparent transport

Used when an upstream provider accepts an OpenAI-compatible request.

The gateway:

1. inspects fields required for routing
2. rewrites the public model ID to the upstream model ID
3. preserves unknown request fields
4. forwards the request

The implemented transport accepts a raw JSON request, validates its top-level
structure, and replaces only the encoded value of the top-level model field.
All other request bytes, including unknown fields, retain their original
representation. The transport reuses the provider's HTTP client and supports
Chat Completions, Completions, Embeddings, and stateless Responses. JSON and
SSE bodies are relayed without interpretation. Stateful Responses fields
(`previous_response_id` and `conversation`) are rejected at the public API.

### Translated transport

Used when the upstream protocol differs from the public OpenAI-compatible
API.

Examples may include:

- OpenAI Codex Responses
- Anthropic Messages

Translation must be isolated from generic request routing.

## Provider Model

Providers are identified by a unique provider ID.

Public model IDs use:

    provider/model

The provider ID ends at the first `/`.

Provider IDs must match `[a-z0-9][a-z0-9._-]*`. Public model IDs are
rejected when the separator is absent, the provider ID is invalid, or the
upstream model ID is empty.

For example:

    openrouter/google/gemini-2.5-pro

means:

    provider = openrouter
    model    = google/gemini-2.5-pro

## Provider Registry

The provider registry owns configured provider instances and maps public
model IDs to physical upstream providers.

Provider credentials are scoped to their owning provider.

Credentials must never be reused for another upstream endpoint.

The current OpenAI HTTP handler maintains an ordered collection of configured
inference providers plus an ID lookup. Explicit `provider/model` requests are
sent only to the named provider. Routing fields use exact, case-sensitive JSON
key matching; differently cased keys remain unknown passthrough fields. Routing
inspection does not copy unrelated values, keeping memory overhead bounded for
requests containing large message or image payloads.

## Model Catalog

**Current:** model discovery is aggregated directly by the HTTP handler. Ollama
models are read from the OpenAI-compatible model endpoint, prefixed, and
returned; one provider failure fails the request. There is no capability merge,
refresh cache, or partial result policy yet.

**Version 1 target:** a catalog component combines:

1. provider-discovered metadata
2. bundled capability metadata

Live authoritative provider metadata takes precedence over bundled values.

Unknown capability values remain unknown rather than being guessed.


## Streaming

Streaming uses SSE passthrough wherever possible.

The gateway relays upstream stream bytes without parsing or rebuilding events
and flushes each copied chunk promptly. This preserves event ordering and
unknown payload fields, including tool-call and reasoning deltas, without
buffering the entire response. The downstream request context controls the
upstream request, so a disconnected client cancels upstream work.

Once downstream response bytes have been emitted, the request cannot be
rerouted to another model.

## Errors, Request IDs, and Logging

Internal component failures use a shared typed taxonomy, allowing the API to
map client, authentication, routing, provider, quota, capability, classifier,
and streaming failures without inspecting error-message text. Public failures
use one OpenAI-style JSON envelope and deliberately omit internal error detail.

Every request receives an `X-Request-ID`. A single safe client value (at most
128 characters from a conservative ASCII alphabet) is retained; otherwise the
gateway generates a cryptographically random value. The same value is returned
to the client, carried in request context, and sent to the selected upstream.
An upstream response cannot replace the gateway's correlation ID.

The request middleware writes a structured `slog` completion record with the
request ID, method, URL path, status, and duration. It does not log query
values, headers, request or response bodies, prompts, tokens, or secrets.

## Smart Routing

**Not implemented.** The Version 1 smart model is delivered by TASK-024 through
TASK-029 and consists of:

    request requirements
        |
        v
    session resolution
        |
        v
    classifier
        |
        v
    capability filtering
        |
        v
    model selection
        |
        v
    physical provider/model

Classification and model selection are separate concerns.

## Session State

**Not implemented.** TASK-030 through TASK-033 add identification, bounded
in-memory affinity, tool-loop pinning, and pre-stream failure handoff. Version 1
session state is stored in memory only and is deliberately lost on restart.

## Persistent State

**Current:** `state.directory` is validated and reserved, but the gateway does
not write persistent state.

**Version 1 target:** persistent state lives beneath the configured state
directory (normally `/state`). Expected state includes:

    /state/auth/
    /state/usage.json

Configuration is not stored in the state directory.

## Configuration

Configuration is loaded from one YAML document at startup. Unknown fields,
duplicate provider IDs, malformed listener addresses, invalid provider URLs,
and unsupported provider types cause loading to fail. The currently
implemented schema supports the server listener and optional API-key secret
reference, state directory, and the common fields required for Ollama
providers. Secret references select exactly one environment variable or file;
file values are trimmed of surrounding whitespace when resolved. API keys
containing whitespace are rejected at startup because they cannot be represented
by the supported Bearer syntax. Configuration for later providers and
subsystems is intentionally rejected until implemented.

## Security Invariants

- Provider credentials are bound to provider endpoints.
- Secrets are never logged.
- Client API keys are compared using constant-time comparison.
- Unknown API fields are not silently discarded.
- Unsupported features produce explicit errors.
- OpenAI API credentials and OpenAI Codex OAuth credentials are separate.

## Package Layout

The implemented packages are `cmd/gateway`, `internal/api`, `internal/config`,
`internal/provider`, and `internal/transport`. Planned components are added only
by their corresponding task. The intended Version 1 layout is:

    cmd/gateway/
    internal/api/
    internal/config/
    internal/provider/
    internal/transport/
    internal/catalog/
    internal/classifier/
    internal/router/
    internal/session/
    internal/quota/
    internal/state/

## Architectural References

See:

- `docs/implementation-plan.md`
- `docs/decisions.md`

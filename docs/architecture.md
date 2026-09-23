# Architecture

## Purpose

This document describes the architecture currently implemented in the
gateway.

For planned functionality that has not yet been implemented, see:

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
to finish before exiting.

No external database, Redis instance, message queue, sidecar, or worker
service is required.

## Request Flow

Current request flow:

    Client
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

As intelligent routing is implemented, this section should be updated to
reflect the actual request path.

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
representation. The transport reuses the provider's HTTP client. It currently
supports non-streaming requests only; `stream: true` is rejected explicitly
until the SSE relay is implemented.

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
sent only to the named provider.

## Model Catalog

The model catalog combines:

1. provider-discovered metadata
2. bundled capability metadata

Live authoritative provider metadata takes precedence over bundled values.

Unknown capability values remain unknown rather than being guessed.

Ollama models are currently discovered from its OpenAI-compatible
`GET /v1/models` endpoint. The gateway prefixes each upstream ID with the
configured provider ID and exposes the result through `GET /v1/models` and
`GET /v1/models/{model}`. A provider discovery failure currently fails the
catalog request rather than returning a partial result.

## Streaming

Streaming uses SSE passthrough wherever possible.

The gateway must not buffer an entire streaming response.

Once downstream response bytes have been emitted, the request cannot be
rerouted to another model.

## Smart Routing

[Update this section when implemented.]

The planned smart model consists of:

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

[Update when implemented.]

Version 1 session state is stored in memory only.

## Persistent State

Persistent state lives beneath:

    /state

Expected v1 state includes:

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
file values are trimmed of surrounding whitespace when resolved. Configuration
for later providers and subsystems is intentionally rejected until implemented.

## Security Invariants

- Provider credentials are bound to provider endpoints.
- Secrets are never logged.
- Client API keys are compared using constant-time comparison.
- Unknown API fields are not silently discarded.
- Unsupported features produce explicit errors.
- OpenAI API credentials and OpenAI Codex OAuth credentials are separate.

## Package Layout

Keep this section synchronized with the repository.

Example:

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

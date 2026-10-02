# Architecture

## Purpose

This document distinguishes the architecture currently implemented in the
gateway from the approved Version 1 target. Version 1 supports local Ollama,
standard OpenAI-compatible endpoints, and ChatGPT Codex subscription access;
dedicated integrations for other providers are Version 2 work. The repository
implements the Version 1 route, including semantic selection, quota-aware
ranking, session affinity, and endpoint-bound Codex transport.
Sections labeled "planned" are contracts, not claims about the running product.

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
opens encrypted SQLite credentials, imports legacy secrets, prompts for initial
administrator setup, constructs each configured provider, and
serves the API on the configured listener. It shuts down gracefully on SIGINT
or SIGTERM. Health/static console/setup status remain public; inference uses
bearer authentication and management uses administrator Basic login. On SIGINT or SIGTERM, the process
stops accepting new connections and waits up to 10 seconds for active requests
to finish before exiting. Idle client connections are closed after 60 seconds.

No external database, Redis instance, message queue, sidecar, or worker
service is required.

All credential payloads, including OAuth records, use versioned encrypted SQLite
storage at `/data/torii.db` (configurable). AES-256-GCM encryption uses an external
master key; WAL, busy timeout and foreign keys are enabled. Schema 4 is the
fresh database-only baseline; future transactional upgrades preserve its data. See
[credential storage](credential-storage.md) for key management and recovery.

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

The request-requirement and classification contracts feed session resolution,
deterministic selection, quota/cost policy, and pre-stream failure handling
between authentication and the provider registry. `smart` requests use that
path; explicit `provider/model` requests continue to resolve directly and can
never be substituted.

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

Provider-specific translation such as Anthropic Messages is deferred to
Version 2.

Translation must be isolated from generic request routing.

The implemented canonical translation contract represents instructions,
system/developer/user/assistant/tool messages, text and image content, tools and
results, structured output, reasoning controls, token limits, normalized usage,
finish reasons, and stream deltas. Fields not understood by the shared contract
are retained as validated raw JSON extensions for an adapter to preserve or
explicitly reject. Transparent providers never instantiate this model.

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

Generic compatible providers use independent HTTP clients and do not follow
redirects. Optional bearer credentials and custom-header values are resolved
from secret references and attached only by the owning provider transport.
Per-provider TLS trust, TLS server name, connection timeout, request timeout,
and discovery timeout settings do not affect other providers.

The current OpenAI HTTP handler maintains an ordered collection of configured
inference providers plus an ID lookup. Explicit `provider/model` requests are
sent only to the named provider. Routing fields use exact, case-sensitive JSON
key matching; differently cased keys remain unknown passthrough fields. Routing
inspection does not copy unrelated values, keeping memory overhead bounded for
requests containing large message or image payloads.

## Model Catalog

The model catalog discovers configured providers at startup and combines:

1. provider-discovered metadata
2. bundled capability metadata

Live authoritative provider metadata takes precedence over bundled values.
Configuration overrides take precedence over both. Published snapshots are
sorted and immutable, and duplicate public model IDs reject the refresh.

Unknown capability values remain unknown rather than being guessed.
The implemented pure selector filters unavailable, exhausted, incompatible,
insufficient-context, and disallowed paid candidates before considering quality.
It then deterministically orders eligible models by estimated cost, quota,
location preference, measured latency, context headroom, and public ID, retaining
structured rejection and selection reasons. Unknown quality is admitted only
when no known-quality candidate meets the configured threshold.
Endpoint and feature support use explicit supported, unsupported, and unknown
states; descriptors also carry context, pricing, quality, locality,
availability, staleness, and metadata provenance for later routing policy.

Providers refresh independently. A failed provider retains its last-known-good
entries marked stale, or uses matching bundled entries when no live snapshot
has succeeded. Other providers remain visible. Startup fails only when no
configured provider has a usable live, cached, or bundled catalog.


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

The Version 1 smart model has deterministic capability fallback and may use a
configured semantic classifier. Its classifier may use a self-hosted Laya model
through a physical Ollama or standard OpenAI-compatible provider; Laya never
acts as the virtual model and cannot recursively select `smart`. The flow is:

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

Session identification and bounded in-memory affinity are implemented. Tool-loop
pinning and pre-stream failure handoff are supported for smart requests; session
state is stored in memory only and is deliberately lost on restart. Full
multi-turn compatibility coverage remains part of the release audit.

## Persistent State

All durable application state uses `state.database_path`: credentials, public
benchmark observations, aggregate usage and the stable Codex host ID. Only
credential payloads are encrypted. Benchmark and usage persistence own separate
SQLite pools so credential reads are not queued behind their writes. Routing uses
in-memory views; usage additions perform no SQLite I/O during inference. A shared
worker flushes usage totals every second and after HTTP shutdown drains requests.

Persistent state normally includes:

    /data/torii.db

SQLite schema 5 (supported baseline 4) contains credential, named gateway
client-key metadata/digests, source/status, dataset generation,
observation, usage and application-setting tables. It contains no legacy import
metadata. Only each source's latest published generation is retained.
Staged generations become visible through one short pointer-publication
transaction; failed publication preserves active data.

Nonsensitive configuration remains in YAML outside the database.
See [runtime-state baseline and durability](runtime-state.md).

## Configuration

Configuration is loaded from one YAML document at startup. Unknown fields,
duplicate provider IDs, malformed listener addresses, invalid provider URLs,
and unsupported provider types cause loading to fail. The currently
implemented schema supports the server listener, database path, configured
providers, and routing controls. Secret references select encrypted database
IDs; persisted inline/env/file inputs are rejected. Gateway admin
authentication and the inference key are stored only in SQLite. Unsupported provider families and unknown
configuration fields remain rejected.

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

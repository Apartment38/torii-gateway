# Tasks

## Project status

The repository is a **pre-v1 implementation**. The completed work provides the
first usable Ollama-backed Chat Completions slice, but it does not satisfy the
Version 1 definition of done in `docs/implementation-plan.md`.

## IN PROGRESS

No task is currently in progress. Start the first unblocked task in the ordered
TODO list below and move only that task here while implementing it.

---

## DONE

### TASK-009 — Complete transparent OpenAI endpoint coverage

Goal:
Expose the complete transparent OpenAI-compatible endpoint set supported by
the Ollama MVP.

Completed:
- added transparent Completions, Embeddings, and stateless Responses handlers
  alongside Chat Completions
- preserved unknown request fields and the existing 32 MiB body limit across
  every inference endpoint
- relayed JSON and supported streaming responses through the shared transport
- rejected `previous_response_id` and `conversation` with an OpenAI-style
  `unsupported_parameter` error
- added transport, provider, handler, and end-to-end endpoint coverage

Decisions:
- use one provider forwarding contract for the compatible endpoint set so all
  endpoints retain identical byte-preservation and cancellation behavior
- treat the presence of a stateful Responses field as unsupported, including a
  null value, rather than silently changing request semantics

### Pre-v1 usage documentation

Prepared public-facing documentation for the currently implemented pre-v1
feature set and a configuration example.

Completed:
- documented the project's purpose, implemented feature set, limitations,
  source installation, configuration schema, API usage, and operational notes
- added a safe example configuration using an environment-backed client API
  key and a local Ollama provider
- added a test that loads the committed example through the strict production
  configuration decoder to prevent it from becoming stale or invalid

### TASK-008 — SSE streaming relay

Goal:
Relay streaming Chat Completions with low overhead and prompt cancellation.

Completed:
- enabled `stream: true` on the transparent OpenAI-compatible transport
- advertised `text/event-stream` to upstreams for streaming requests
- relayed upstream stream bytes without parsing or rebuilding SSE events
- flushed each copied upstream chunk promptly instead of buffering the response
- retained request-context cancellation through the upstream request
- added transport and end-to-end prompt-flush coverage

Decisions:
- preserve SSE payloads byte-for-byte so unknown, tool-call, and reasoning
  deltas remain compatible
- terminate a failed stream rather than attempting a mid-stream provider switch

### OpenAI-compatible transport hardening

Hardened the shared OpenAI-compatible path by preserving escaped trailing
base-path slashes, bounding Ollama model discovery, validating model-catalog
arrays, rejecting top-level trailing commas, and stripping all applicable
hop-by-hop response headers, including `Proxy-Connection`.
### TASK-006 and TASK-007 — Transparent transport and Ollama provider

Goal:
Forward OpenAI-compatible requests without dropping unknown fields, and support
model discovery plus non-streaming Chat Completions through Ollama.

Completed:
- added a reusable OpenAI-compatible transport with byte-targeted top-level
  model rewriting, context cancellation, shared HTTP-client reuse, and exact
  preservation of every other request field
- added Ollama model discovery through its OpenAI-compatible model endpoint
- exposed discovered models under provider-qualified public IDs
- added OpenAI-compatible model-list, model-retrieval, and non-streaming Chat
  Completions handlers
- explicitly reject streaming requests until TASK-008
- added transport, provider, and end-to-end handler contract tests
- wired configuration loading, Ollama providers, bearer authentication, health
  checks, and the OpenAI handler into the `gateway serve` executable
- added a process-level test that builds and starts the gateway and requests its
  authenticated model catalog
- ensured graceful shutdown waits for active requests to finish, up to the
  configured 10-second shutdown deadline
- made routing-field extraction case-sensitive so unknown case variants cannot
  override the requested provider or model
- extract routing fields without copying unrelated large request values, return
  413 for bodies over 32 MiB, and distinguish client validation failures from
  upstream failures
- validate configured bearer keys at startup and bound idle HTTP connections
- preserve escaped provider base paths and remove response headers named by
  upstream `Connection` tokens
- preserve escaped trailing base-path slashes, strictly reject trailing commas,
  strip proxy connection headers, validate model catalogs, and bound model
  discovery calls without limiting inference
- validate complete, size-bounded model-catalog documents and require every
  catalog entry to be an object

Decisions:
- gateway client authorization is never forwarded to Ollama
- upstream status, body, and end-to-end response headers are relayed for Chat
  Completions while hop-by-hop headers are removed
- model discovery currently fails the aggregate request when a configured
  provider is unavailable; partial-catalog policy remains future work

---

### Continuous integration workflow

Added a minimal GitHub Actions workflow for pushes to `main` and pull requests
targeting `main`. CI verifies formatting and module tidiness, runs `go vet`, the
standard and race-enabled test suites, and builds all packages using the Go
version declared in `go.mod`.

---

### Documentation workflow clarification

Clarified that documentation updates are required for durable changes to
architecture, public behavior, configuration, developer workflow, and
significant design decisions. Narrow task-specific test commands no longer need
to be added to `docs/development.md` unless they are reusable development
procedures.

---

### TASK-005 — Provider/model ID parsing

Goal:
Parse public model IDs into validated provider namespaces and upstream model
IDs.

Completed:
- added provider ID validation shared by configuration and model ID parsing
- split public model IDs on the first slash only
- rejected missing separators and empty upstream model IDs
- added tests for simple and OpenRouter-style nested model IDs

Decisions:
- parsing returns no partial provider or model value when validation fails
- upstream model IDs are otherwise preserved verbatim, including nested slashes

---

### TASK-004 — Gateway bearer authentication

Goal:
Protect gateway API handlers with the configured bearer API key.

Completed:
- added the extensible client authenticator and identity contracts
- added static `Authorization: Bearer` authentication for the v1 API key
- compared fixed-size API-key digests in constant time
- added OpenAI-style JSON authentication errors and bearer challenges
- added focused middleware, credential rejection, and identity tests
- made the middleware fail closed for missing authenticators and identities

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

## TODO — Ordered Version 1 roadmap

Version 1 is intentionally limited to local Ollama, standard OpenAI-compatible
endpoints (including the normal OpenAI API), and ChatGPT Codex subscription
access. Dedicated OpenRouter, Nous Portal, Gemini, xAI, and Anthropic adapters,
along with Jev classifiers, are tracked in the Version 2 backlog below.

Each numbered item is one focused implementation milestone. Tasks are ordered
by dependency and must include focused tests, repository checks, task updates,
and durable documentation updates. Provider and classifier protocol work must
first add dated findings from primary sources to `docs/provider-notes.md`.

### Next steps — Version 1

#### 1. Stabilize the public API and catalog

##### TASK-010 — Normalize public errors and request IDs

**Depends on:** TASK-009

Define shared OpenAI error envelopes and typed internal failures. Accept or
generate request IDs, propagate them safely, and add secret-safe structured
logging. Test mappings, propagation, and redaction.

##### TASK-011 — Build the model catalog and capability metadata core

**Depends on:** TASK-010

Add immutable model descriptors, tri-state capabilities, deterministic metadata
merging, refresh and last-known-good caching, collision detection, and partial
provider availability. Unknown capabilities must remain unknown.

##### TASK-012 — Add configurable standard OpenAI-compatible endpoints

**Depends on:** TASK-011

Extend configuration and the provider registry for arbitrary standard
OpenAI-compatible endpoints, including local self-hosted servers. Support
optional bearer secrets or no auth, safe custom headers and TLS configuration,
static or discovered models, transport selection, model-name rewriting, and
separate connection/request timeouts. Preserve unknown request fields and bind
all credentials to their configured origin. Add fake-server, isolation,
streaming, and configuration tests.

##### TASK-014 — Verify the normal OpenAI API profile

**Depends on:** TASK-012

Document current official OpenAI API behavior and provide a tested `openai`
configuration/profile over the standard compatible provider. Keep API-key
credentials, endpoints, and clients strictly separate from Codex subscription
OAuth credentials.

#### 2. Build the Codex subscription path

##### TASK-017 — Define the canonical translated request/response model

**Depends on:** TASK-010

Add the translation-only representation needed by Codex for instructions,
multimodal messages, tools and results, structured output, reasoning controls,
usage, finish reasons, and streaming deltas. Define lossless/error rules and
round-trip tests; compatible traffic must continue to bypass translation.

##### TASK-019 — Add secure persistent credential storage

**Depends on:** TASK-010

Store provider-scoped credentials below `state.directory/auth` with restrictive
permissions, atomic replacement, corruption-safe reads, and secret-safe errors.
Test restart and cross-provider isolation behavior.

##### TASK-020 — Add authentication administration CLI

**Depends on:** TASK-019

Implement config validation plus provider login, logout, and status commands.
Support non-interactive operation, never print tokens, and test exit behavior.

##### TASK-021 — Verify and implement ChatGPT Codex subscription authentication

**Depends on:** TASK-019, TASK-020

Document the current verified ChatGPT OAuth flow, refresh lifecycle, audience,
account identifiers, and persisted fields. Implement the supported login and
refresh flow with endpoint binding and tests proving API keys and OAuth tokens
cannot cross endpoints.

##### TASK-022 — Implement the ChatGPT Codex subscription transport

**Depends on:** TASK-017, TASK-021

Implement the verified Codex Responses protocol and catalog, translate supported
public requests and streams, preserve tools/reasoning/usage/errors, reject
unsupported features explicitly, and expose `openai-codex/<model>` IDs. Add
fixtures, fake-server tests, and opt-in live tests.

#### 3. Implement classification and smart selection

##### TASK-024 — Implement request requirements and classifier contracts

**Depends on:** TASK-011, TASK-017

Define objective request requirements, the fixed classification schema,
classifier interface, validation, recursion prevention, timeouts, and failure
policies. Extract hard requirements without losing unknown request fields.

##### TASK-025 — Implement the generic connected-model classifier

**Depends on:** TASK-024

Invoke a configured physical model through the provider registry, require and
validate structured classification output, prevent `smart` recursion, bound the
call, and test malformed output, cancellation, and fallback behavior.

##### TASK-026 — Verify and implement the self-hosted Laya classifier

**Depends on:** TASK-012, TASK-024

Document the Laya model's current input/output contract from primary sources.
Run a configured provider-qualified Laya model through local Ollama or a
self-hosted standard OpenAI-compatible endpoint; do not add a separate cloud
provider or credential path. Validate output against the common schema, forbid
`smart` recursion, and test malformed output, timeouts, cancellation, and
failure policy with a fake local endpoint.

##### TASK-027 — Implement the Ollama embedding classifier

**Depends on:** TASK-024

Add deterministic in-memory prototype similarity over a configured physical
Ollama embedding model. Validate vector shape and finite values and test
thresholds, cancellation, and stability.

##### TASK-028 — Implement deterministic model selection

**Depends on:** TASK-011, TASK-024

Implement pure selection policy: capability and endpoint filtering, context
headroom, quality, paid/free and known-price rules, estimated cost, quota, local
preference, latency, and stable tie-breaking. Return structured debug reasons
and add table/fuzz tests.

##### TASK-029 — Expose and route the `smart` virtual model

**Depends on:** TASK-025, TASK-026, TASK-027, TASK-028

Add `smart` catalog behavior and route requirements through classification and
selection to one physical model. Explicit model requests must bypass semantic
routing. Add end-to-end deterministic-choice and failure-policy tests.

#### 4. Add sessions, handoff, quota, and usage

##### TASK-030 — Implement session identification

**Depends on:** TASK-029

Resolve bounded session identity from explicit header, metadata, `user`, then a
conversation fingerprint; avoid logging raw identifiers or content.

##### TASK-031 — Implement in-memory affinity lifecycle

**Depends on:** TASK-030

Add concurrency-safe pinning, expiry, context reset, cleanup, and authenticated
session deletion with bounded memory and race coverage.

##### TASK-032 — Preserve affinity through agent/tool loops

**Depends on:** TASK-031

Keep smart sessions pinned across assistant tool calls and results, including
parallel and incomplete tool exchanges.

##### TASK-033 — Implement typed provider failures and pre-stream handoff

**Depends on:** TASK-029, TASK-031

Classify retryable failures and support configured fail/reroute behavior only
before downstream bytes are committed. Preserve request/tool context, cap
attempts, and never substitute an explicit model.

##### TASK-034 — Implement quota and allowance state

**Depends on:** TASK-011, TASK-019

Add known/unknown quota semantics, local allowances, normalized snapshots, TTL
caching, concurrency control, and exhausted-model filtering.

##### TASK-035 — Persist minimal aggregate usage

**Depends on:** TASK-034

Persist only routing-required aggregate usage with versioning, restrictive
permissions, atomic replacement, recovery, and streaming-safe collection.

##### TASK-036 — Integrate quota, usage, cost, and latency with smart routing

**Depends on:** TASK-028, TASK-033, TASK-034, TASK-035

Feed quota, aggregate usage, verified pricing, and latency into selection and
test budget, unknown-data, local-preference, and reroute boundaries.

#### 5. Harden and release Version 1

##### TASK-037 — Complete Version 1 configuration and operational CLI

**Depends on:** TASK-012, TASK-014, TASK-022, TASK-024–TASK-036

Finalize the strict schema for only the Version 1 provider families and routing
components, finish model/version commands, validate combinations, update the
example configuration, and add migration/error tests.

##### TASK-038 — Add compatibility, security, and agent-loop suites

**Depends on:** TASK-037

Cover every public endpoint, compatible provider, Codex translation, self-hosted
Laya classification, unknown fields, tools, multimodality, SSE, cancellation,
credential isolation, malformed inputs, and multi-turn handoff.

##### TASK-039 — Add the production container and shutdown hardening

**Depends on:** TASK-037

Build a reproducible non-root `linux/amd64` image with read-only configuration,
persistent `/state`, health checks, cancellation, and graceful shutdown tests.

##### TASK-040 — Establish performance and resource baselines

**Depends on:** TASK-038, TASK-039

Benchmark startup, idle RSS, catalogs, transparent proxy overhead, SSE first
byte, classifier latency, and concurrent sessions using a reproducible method.

##### TASK-041 — Validate and publish Version 1

**Depends on:** TASK-038, TASK-039, TASK-040

Audit the narrowed Version 1 definition of done, run all formatting, tidy, vet,
unit/race/integration/security checks, build artifacts, exercise documented
Ollama and opt-in OpenAI/Codex/Laya smoke tests, and publish limitations and
checksums only after all required behavior passes.

---

## Version 2 backlog (not required for Version 1)

These tasks are deliberately unordered until Version 1 is complete. Before
implementation, refresh their requirements and dependencies against current
primary documentation.

- **TASK-013 — OpenRouter adapter:** discovery, authentication, metadata,
  quota/credit behavior, and contract tests without duplicating OpenRouter's
  internal routing.
- **TASK-015 — Gemini adapter:** verified compatible surface plus any required
  provider-specific behavior.
- **TASK-016 — xAI adapter:** verified compatible surface plus explicit
  unsupported-feature behavior.
- **TASK-018 — Anthropic adapter:** native Messages translation with tools,
  vision, reasoning, usage, and streaming.
- **TASK-023 — Nous Portal adapter:** verified subscription authentication,
  refresh, catalog, inference, quota, and pricing behavior.
- **Jev classifier adapters:** direct and provider-transport modes may be added
  behind the Version 1 classifier contract; they must not replace or complicate
  the self-hosted Laya path.

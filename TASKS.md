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

Each numbered item below is one implementation milestone and should produce one
focused commit. Tasks are ordered by dependency; a task may start only after
all IDs in its `Depends on` field are done. Every task must add focused tests,
run the repository checks, update this file, and update durable documentation
when public behavior or architecture changes. Provider tasks must first update
`docs/provider-notes.md` with dated, primary-source protocol findings. Optional
upstream metadata must not block the transparent gateway path.

### Foundation and complete public API

#### TASK-009 — Complete transparent OpenAI endpoint coverage

**Depends on:** TASK-008

Add transparent `POST /v1/completions`, `POST /v1/embeddings`, and stateless
`POST /v1/responses` forwarding, including streaming where the endpoint permits
it. Preserve unknown fields, enforce the existing body limit, reject stateful
Responses fields such as `previous_response_id` with an OpenAI-style
unsupported-parameter error, and add handler/transport/end-to-end tests.

#### TASK-010 — Normalize public errors and request IDs

**Depends on:** TASK-009

Define the shared OpenAI error envelope and typed internal errors for client,
authentication, routing, provider, and upstream failures. Accept or generate a
request ID, return it in responses, propagate it upstream where safe, and add
structured `slog` records that never include request bodies, tokens, or secrets.
Test status/code mappings, request-ID propagation, and redaction invariants.

#### TASK-011 — Build the model catalog and capability metadata core

**Depends on:** TASK-010

Introduce immutable model descriptors, tri-state capabilities, bundled
capability data, deterministic live/bundled/config merge rules, catalog refresh
and last-known-good caching, collision detection, and partial-provider refresh
behavior. Include context, modalities, endpoints, tool/structured-output,
reasoning, price, quality, and availability fields without guessing unknowns.
Expose deterministic model list and retrieval views and test stale/partial data.

#### TASK-012 — Add configurable generic OpenAI-compatible providers

**Depends on:** TASK-011

Extend strict configuration and the registry for arbitrary compatible
endpoints. Support optional bearer API-key secret references and explicit
no-auth mode; arbitrary literal or secret-backed request headers; custom CA
bundles and an explicit TLS-verification toggle; startup model discovery or a
static model list; `auto`, `chat_completions`, and `responses` transport
selection; configured upstream model-name rewriting; and separate request and
connect timeouts. Make `auto` probe conservatively instead of inferring a
transport from model names. Reuse the transparent transport where selected,
bind credentials and secret headers to the configured origin, reject unsafe
URL, header, and TLS configuration, and do not silently discard features that
the selected transport cannot represent.

Test multiple providers, authenticated and no-auth operation, literal and
secret-backed headers, nested and rewritten model IDs, static catalogs and
discovery failures, transport selection and conservative `auto` probing,
custom-CA trust and disabled-verification modes, independent connect/request
timeouts (without imposing a streaming response deadline), credential and
secret-header isolation across origins, unsupported-feature errors, and
byte-preserving streaming through compatible transports.

### Provider verification and adapters

#### TASK-013 — Verify and implement OpenRouter

**Depends on:** TASK-012

Document current official model, inference, pricing, capability, credit/quota,
and error behavior; then add strict configuration, authentication, discovery,
metadata ingestion, transparent endpoint support, and fake-server contract
tests. Treat OpenRouter as one provider and do not reproduce its internal
provider selection.

#### TASK-014 — Verify and implement the OpenAI API provider

**Depends on:** TASK-012

Document current official behavior and implement the API-key-backed `openai`
namespace with discovery and supported transparent endpoints. Keep its client,
credentials, and configuration distinct from Codex subscription access and add
credential-isolation and contract tests.

#### TASK-015 — Verify and implement the Gemini API provider

**Depends on:** TASK-012

Verify Gemini's current OpenAI-compatible surface, authentication, catalog,
capabilities, streaming, quota, and pricing. Implement compatible operations
through the transparent path, explicitly reject unsupported operations or
features, ingest verified metadata, and add contract tests.

#### TASK-016 — Verify and implement the xAI provider

**Depends on:** TASK-012

Verify xAI's current OpenAI-compatible surface, authentication, catalog,
capabilities, streaming, quota, and pricing. Implement the supported transparent
operations with explicit unsupported-feature errors and contract tests.

#### TASK-017 — Define the canonical translated request/response model

**Depends on:** TASK-010

Add the translation-only canonical representation for instructions, multimodal
messages, tools/calls/results, structured output, reasoning controls, token
limits, usage, finish reasons, and streaming deltas. Define capability checks
and lossless/error rules so adapters never silently discard a feature. Add
round-trip fixtures and streaming state-machine tests without routing compatible
traffic through this layer.

#### TASK-018 — Verify and implement Anthropic

**Depends on:** TASK-011, TASK-017

Document current official Messages API authentication, models, capabilities,
streaming events, errors, quota, and pricing. Implement OpenAI-to-Anthropic
translation for supported public endpoints, including tools, vision, structured
output where supported, reasoning, usage, and SSE; explicitly reject every
untranslatable feature and add fixture/fake-server contract tests.

### Subscription authentication and providers

#### TASK-019 — Add secure persistent credential storage

**Depends on:** TASK-010

Implement provider-scoped JSON credential files beneath `state.directory/auth`
using restrictive permissions, atomic replacement, corruption-safe reads, and
secret-safe errors/logging. Define a small credential-store interface for tests
and future migration; test permissions, interrupted writes, malformed files,
and cross-provider isolation.

#### TASK-020 — Add authentication administration CLI

**Depends on:** TASK-019

Implement `gateway config validate`, `gateway auth login <provider>`, `logout`,
and `status` with the same config/state resolution as `serve`. Make commands
non-interactive where flags/stdin are supplied, never print tokens, use useful
exit codes, and add command tests. Implement `gateway models` after the registry
can be constructed without serving.

#### TASK-021 — Verify and implement OpenAI Codex subscription authentication

**Depends on:** TASK-019, TASK-020

Document the current official/observed ChatGPT OAuth flow, refresh lifecycle,
account identifiers, scopes, and persisted fields. Implement PKCE/device or
other verified login flow, refresh with atomic persistence, logout/status, and
strict endpoint/audience binding. Test expiry, refresh races, invalid grants,
restart persistence, and prove API keys and OAuth tokens cannot cross endpoints.

#### TASK-022 — Implement the OpenAI Codex subscription transport

**Depends on:** TASK-017, TASK-021

Document and implement the verified Codex Responses protocol and model catalog,
translate supported OpenAI requests and streaming events, preserve tools,
reasoning, usage, and errors, and explicitly reject unsupported features. Add
recorded fixtures, fake-server tests, opt-in live tests, and ensure the public
namespace is `openai-codex`.

#### TASK-023 — Verify and implement Nous Portal

**Depends on:** TASK-017, TASK-019, TASK-020

Document current OAuth/API-key, refresh, inference-token, catalog, inference,
streaming, quota, and pricing behavior. Implement verified login persistence,
refresh, model discovery, and transparent or translated transport as required,
with contract tests, opt-in live tests, and provider-scoped credentials.

### Classification and smart selection

#### TASK-024 — Implement request requirements and classifier contracts

**Depends on:** TASK-011, TASK-017

Define objective request requirements independently of semantic classification,
the fixed classification schema, classifier interface, validation, recursion
prevention, timeouts, and configured failure policies. Extract hard endpoint,
modality, tool, structured-output, reasoning, and context requirements without
losing unknown request fields; add exhaustive validation tests.

#### TASK-025 — Implement the connected-model classifier

**Depends on:** TASK-024

Invoke one configured physical model through the provider registry, require and
validate structured classification output, prevent `smart` recursion, bound the
call, and implement configured fallback behavior. Add malformed output,
timeout, cancellation, and provider-failure tests.

#### TASK-026 — Verify and implement Jev classifiers

**Depends on:** TASK-024

Verify and document the current Jev API/schema. Implement direct Jev and
provider-transport Jev modes, strict output validation, timeout/cancellation,
recursion prevention, and failure policy with fake-server tests.

#### TASK-027 — Implement the Ollama embedding classifier

**Depends on:** TASK-024

Define the bundled labels/reference embeddings and deterministic similarity
algorithm, call the configured physical Ollama embedding model, validate vector
shape and finite values, and test malformed vectors, cancellation, thresholds,
and classification stability.

#### TASK-028 — Implement deterministic model selection

**Depends on:** TASK-011, TASK-024

Implement the selector as a pure policy component: hard-capability and endpoint
filtering, context headroom, quality thresholds, paid/free policy, known-price
rules, estimated input/output cost, quota eligibility, local/cloud preference,
latency EWMA, and stable tie-breaking. Return structured debug reasons and add
table/fuzz tests for every stage, including no-eligible-model errors.

#### TASK-029 — Expose and route the `smart` virtual model

**Depends on:** TASK-025, TASK-026, TASK-027, TASK-028

Add configured `smart` catalog/list/retrieval behavior and orchestration from
requirements through classification and selection to exactly one physical
model. Explicit model requests must bypass classification and substitution.
Keep explanations in debug logs only, propagate cancellation, prevent recursive
selection, and add end-to-end tests for deterministic choices and failure
policies.

### Sessions, failure, quota, and usage

#### TASK-030 — Implement session identification

**Depends on:** TASK-029

Resolve session identity in the documented precedence order from
`X-Session-ID`, request metadata, OpenAI `user`, and a bounded deterministic
conversation fingerprint. Validate and hash untrusted identifiers where needed,
avoid logging raw identifiers/content, and test conflicts, stability, and
fingerprint limits.

#### TASK-031 — Implement in-memory affinity lifecycle

**Depends on:** TASK-030

Add concurrency-safe session pinning, last-seen state, inactivity expiration,
context-reset detection, background cleanup, and authenticated
`DELETE /gateway/v1/sessions/{session_id}`. Bound memory and identifier sizes,
make deletion idempotent, and test races, expiry, reset, shutdown, and explicit
physical requests without requiring persistence.

#### TASK-032 — Preserve affinity through agent/tool loops

**Depends on:** TASK-031

Recognize assistant tool calls and matching tool results, keep a smart session
on its pinned physical model across multi-turn loops, and avoid false resets
while a tool exchange is active. Add realistic multi-turn, parallel-tool,
missing-result, and context-boundary tests.

#### TASK-033 — Implement typed provider failures and pre-stream handoff

**Depends on:** TASK-029, TASK-031

Classify retryable failures, implement configured `fail` or `reroute` behavior,
exclude the failed model when selecting a replacement, and retry only before
any downstream bytes are committed. Preserve the complete request/tool context,
add model-facing handoff notice and client-facing headers, cap attempts, and
test cancellation, loops, explicit-model no-failover, and the no-mid-stream
invariant.

#### TASK-034 — Implement quota and allowance state

**Depends on:** TASK-011, TASK-019

Define known/unknown quota semantics, configured local allowances, normalized
provider quota snapshots, TTL caching, concurrency control, and exhausted-model
filtering. Ingest verified provider queries and response headers when available,
keep unknown quota neutral, and test stale data, refresh failures, clock edges,
and concurrent refreshes.

#### TASK-035 — Persist minimal aggregate usage

**Depends on:** TASK-034

Store only routing-required aggregate token/cost totals in
`state.directory/usage.json` with versioning, restrictive permissions, atomic
replacement, and corruption recovery. Capture non-streaming and terminal SSE
usage without buffering streams; document estimation when upstream usage is
absent and test restart recovery, concurrent updates, and malformed state.

#### TASK-036 — Integrate quota, usage, cost, and latency with smart routing

**Depends on:** TASK-028, TASK-033, TASK-034, TASK-035

Feed fresh quota, aggregate usage/allowances, verified pricing, and latency EWMA
into selection; enforce global paid disablement and known-zero-cost free mode;
update state after success/failure without delaying streamed output. Add
end-to-end selection tests for exhaustion, unknown quota/price, budget edges,
local preference, and reroute replacement.

### Product hardening and release

#### TASK-037 — Complete configuration and operational CLI

**Depends on:** TASK-013–TASK-036

Finalize the strict v1 schema for providers, classifier, router, sessions,
quota, selection, logging, and state; implement cross-field validation and safe
defaults. Finish `gateway models`, add build/version output, validate usable
provider/router combinations at startup, update `config.example.yaml`, and add
configuration migration/error and CLI tests. Hot reload remains out of scope.

#### TASK-038 — Add compatibility, security, and agent-loop test suites

**Depends on:** TASK-037

Add reusable fake providers plus OpenAI SDK compatibility tests for every public
endpoint, unknown fields, tools, vision, reasoning, structured output, SSE,
errors, and cancellation. Add complete smart multi-turn tool-loop/handoff tests,
credential-leak assertions across all providers, malformed-input/fuzz coverage,
and opt-in live-provider tests that skip safely without credentials.

#### TASK-039 — Add the production container and shutdown hardening

**Depends on:** TASK-037

Create a reproducible multi-stage `linux/amd64` image containing one binary,
run as non-root with read-only config and writable `/state`, add healthcheck and
signal handling, and document volume/network/TLS-boundary operation. Test image
build, non-root startup, read-only filesystem behavior, state persistence,
health, cancellation, and graceful shutdown with active streams.

#### TASK-040 — Establish performance and resource baselines

**Depends on:** TASK-038, TASK-039

Benchmark startup, idle RSS, catalog size, transparent proxy allocations and
throughput, time to first SSE byte, classification overhead, and concurrent
session/catalog behavior. Record reproducible methodology and acceptance
thresholds, fix regressions that violate project priorities, and add lightweight
regression benchmarks suitable for development/CI where stable.

#### TASK-041 — Validate and publish Version 1

**Depends on:** TASK-038, TASK-039, TASK-040

Audit every item in the implementation plan's Version 1 definition of done;
run formatting, tidy, vet, unit/race/integration/compatibility/security tests,
build the release binary and container, and execute documented Ollama plus
opt-in provider smoke tests. Resolve all gaps, update README/config/architecture/
provider notes/decisions with final behavior and limitations, produce versioned
artifacts/checksums and release notes, and only then label the product Version 1.

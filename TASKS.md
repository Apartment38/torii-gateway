# Tasks

## Project status

The repository contains the Version 1 implementation described by
`docs/implementation-plan.md`. The local implementation and automated checks
are complete; live provider smoke tests and the production image build remain
environment-dependent release checks.

## IN PROGRESS

The Version 1 slices are implemented in dependency order. The current release
checks are:

- TASK-021/TASK-022: endpoint-bound Codex credentials, device-code login,
  browser PKCE, refresh coalescing, translated Chat/Responses transport, tool
  translation, and SSE adaptation are implemented and locally tested.
- TASK-025–TASK-036: connected/Laya-compatible and embedding classifiers,
  deterministic smart selection, session affinity, pre-stream retry, quota,
  aggregate usage, cost, and latency integration are implemented and locally
  tested.
- TASK-037–TASK-041: strict configuration, operational CLI, non-root image
  definition, CI gates, compatibility tests, and performance baseline are
  implemented. Live provider smoke tests and registry-backed image builds are
  intentionally opt-in.

---

## DONE

### TASK-014 — Verify the normal OpenAI API profile

Completed the origin-locked normal OpenAI API profile over the existing
transparent compatible transport. The profile requires an external bearer-key
reference, rejects alternate origins and generic header/model-rewrite options,
supports optional project and organization headers, and remains isolated from
the persistent Codex credential store. Official API reference URLs and the
2026-09-30 verification date are recorded in `docs/provider-notes.md`.

### TASK-034 — Implement quota and allowance state

Goal:
Represent known and unknown quota consistently and cache provider snapshots.

Completed:
- added normalized known/unknown quota, remaining fraction, and exhaustion state
- added local allowance injection, TTL expiry, invalidation, and finite-value validation
- coalesced concurrent refreshes per model and preserved caller cancellation
- added cache, unknown-state, exhaustion, local allowance, and race coverage

Decisions:
- unknown quota is never interpreted as exhausted or unlimited
- failed refreshes are not cached, allowing the next request to retry

### TASK-028 — Implement deterministic model selection

Goal:
Choose the cheapest eligible physical model using explainable deterministic policy.

Completed:
- added hard endpoint, feature, context-headroom, availability, quota, and free-only filters
- added configurable complexity, tool, and reasoning quality thresholds with
  unknown-quality fallback only when no known-quality model qualifies
- added upper-bound input/output cost estimation and zero monetary cost for local models
- added quota, location, EWMA latency, context headroom, and lexical tie-breaking
- returned per-model debug reasons and added table and order-independence fuzz tests

Decisions:
- unknown endpoint or required-feature support is not treated as verified support
- unknown price is more expensive than known price and is excluded in free-only mode
- unknown latency is neutral and therefore does not beat measured latency solely
  because it happens to be represented as zero

### TASK-024 — Implement request requirements and classifier contracts

Goal:
Define objective request requirements and one validated classifier contract.

Completed:
- added the fixed Version 1 task, complexity, requirement, preference, confidence,
  failure-policy, request, classifier, and runner contracts
- extracted endpoint, tools, images, structured output, reasoning, streaming,
  approximate input size, and output limits without modifying raw request bytes
- made objective requirements override semantic classifier output
- bounded classifier calls, prohibited `smart` recursion, and implemented the
  deterministic general/medium capability fallback
- added malformed schema, extraction, recursion, timeout, and fallback tests

Decisions:
- estimated input tokens use a deterministic conservative byte heuristic until a
  provider tokenizer exists and are not presented as billed usage
- classification describes work and requirements only; it never names a model

### TASK-020 — Add authentication administration CLI

Goal:
Provide validated, non-interactive administration of provider credentials.

Completed:
- added `gateway auth login`, `logout`, and `status` commands
- required the provider to exist in a strictly decoded configuration
- accepted access and optional refresh tokens only through environment variables
  or files and persisted endpoint-bound credentials
- reported only provider state, including expiry, without printing tokens
- added login/restart/status/logout, redaction, and invalid-configuration tests

Decisions:
- token values are never accepted as command-line arguments
- OAuth-specific interactive acquisition remains owned by each OAuth provider

### TASK-019 — Add secure persistent credential storage

Goal:
Persist provider-scoped OAuth credentials safely below the configured state directory.

Completed:
- added a versioned provider-bound credential record and store interface
- added restrictive auth-directory and credential-file permissions
- added size-bounded, strict, corruption-safe reads with secret-safe errors
- added same-directory atomic replacement, file and directory syncing, deletion,
  restart persistence, and provider-isolation coverage

Decisions:
- every stored credential is bound to both a validated provider ID and endpoint
- unsafe permissions, symlinks, unknown fields, trailing data, and wrong provider
  records are rejected instead of repaired or partially loaded

### TASK-017 — Define the canonical translated request/response model

Goal:
Define the translation-only contract needed by the Codex transport without
changing transparent compatible traffic.

Completed:
- added canonical chat/Responses requests, messages, multimodal content, tools,
  tool calls/results, structured output, reasoning, output limits, and extensions
- added canonical responses, normalized usage and finish reasons, and streaming deltas
- added strict validation for lossy or ambiguous request states
- added round-trip and invalid-contract tests

Decisions:
- unknown translated fields must be retained as validated raw JSON extensions or
  rejected by an adapter; silent loss is forbidden
- transparent OpenAI-compatible providers continue to use raw request bytes and
  do not instantiate the canonical model

### TASK-012 — Add configurable generic OpenAI-compatible providers

Goal:
Add origin-isolated, configurable standard OpenAI-compatible upstreams.

Completed:
- added optional discovery or static catalogs, model aliases, and transparent forwarding
- added environment/file-backed bearer and custom-header secrets with unsafe-header rejection
- added independent connection, request, and discovery timeouts plus TLS CA/server-name configuration
- disabled redirects and isolated clients, credentials, TLS, and connection pools by provider
- added configuration, discovery, rewriting, passthrough, streaming, and credential-isolation tests

Decisions:
- custom header values are secret references rather than inline strings
- redirects are returned rather than followed so credentials never leave the configured origin
- generic providers use the existing byte-preserving compatible transport

### TASK-011 — Build the model catalog and capability metadata core

Goal:
Build deterministic, immutable model snapshots without inventing unknown
capabilities or making one unavailable provider hide every other provider.

Completed:
- added normalized model descriptors for endpoint support, capabilities,
  context, price, quality, locality, availability, and provenance
- represented capability knowledge explicitly as supported, unsupported, or
  unknown and embedded the bundled JSON catalog in the binary
- implemented deterministic live, bundled, and configuration override merging
- added startup discovery, sorted list/retrieval views, collision detection,
  partial-provider refresh, bundled fallback, and last-known-good caching
- made startup fail when no provider or bundled fallback is usable while
  allowing a partial catalog when at least one provider remains usable
- added merge, ordering, immutability, collision, stale-cache, fallback, API,
  and partial-provider tests

Decisions:
- the bundled catalog begins empty rather than guessing model capabilities;
  verified provider work will add records as primary-source research lands
- catalog snapshots contain value types and deep-copy optional numeric fields
  so callers cannot mutate catalog state
- public OpenAI model objects remain compatibility-focused while the richer
  normalized descriptors remain the internal source for later smart routing

### TASK-010 — Normalize public errors and request IDs

Goal:
Provide shared typed failures and safe request correlation across the public API.

Completed:
- added the shared OpenAI error envelope and typed internal failure taxonomy
- mapped authentication, request, routing, provider, upstream, capability,
  quota, classifier, and stream failures without matching error strings
- accepted safe client request IDs or generated cryptographically random IDs
- returned the gateway request ID on every response and propagated it to the
  selected upstream while preventing an upstream ID from replacing it
- added structured `slog` completion records containing request metadata only
- tested status/code mappings, propagation, generated IDs, typed wrapping, and
  log redaction invariants

Decisions:
- accept one request ID of at most 128 characters from a conservative ASCII
  alphabet; replace missing, duplicate, or unsafe values
- keep request IDs in a small context package so transports do not depend on
  the HTTP API package
- log method and URL path but never query values, headers, bodies, or credentials

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

Each numbered item below is one implementation milestone and should produce one
focused commit. Tasks are ordered by dependency; a task may start only after
all IDs in its `Depends on` field are done. Every task must add focused tests,
run the repository checks, update this file, and update durable documentation
when public behavior or architecture changes. Provider tasks must first update
`docs/provider-notes.md` with dated, primary-source protocol findings. Optional
upstream metadata must not block the transparent gateway path.

Milestone boundaries organize tracking, review, and commits; they are not stop
conditions. An agent may complete multiple milestones in one session by
processing them sequentially, satisfying each task's dependencies and checks,
and recording a focused commit for each before beginning the next.

### Foundation and complete public API

##### TASK-014 — Verify the normal OpenAI API profile

**Depends on:** TASK-012

Document current official OpenAI API behavior and provide a tested `openai`
configuration/profile over the standard compatible provider. Keep API-key
credentials, endpoints, and clients strictly separate from Codex subscription
OAuth credentials.

**Handoff instructions for the completing agent:**

1. Start with primary-source research. Use only current official OpenAI API
   documentation and, where documentation links to it, an official OpenAI
   specification. Record the verification date and direct source URLs in the
   `OpenAI API` section of `docs/provider-notes.md`. At minimum verify the API
   origin/base path, bearer authentication, project/organization headers,
   model listing, each Version 1 transparent endpoint, SSE behavior, error
   envelopes, request IDs, and whether redirects are part of the contract.
2. Do not infer normal API behavior from ChatGPT, Codex CLI, or third-party SDK
   implementations. Clearly label anything not stated by a primary source as
   unverified and do not turn it into capability metadata.
3. Inspect `internal/config/config.go`, `internal/provider/openai_compatible.go`,
   `internal/transport/openai.go`, and `cmd/gateway/main.go` before coding. Add a
   first-class `openai` profile over the existing compatible provider rather
   than duplicating its forwarding implementation. Define and test which
   fields are fixed by the profile and which safe fields remain configurable.
4. Require an external secret reference for the normal OpenAI API key. Bind
   that key, any project/organization headers, the HTTP client, and the
   verified normal-API origin together. Never read credentials from
   `state.directory/auth`, and never accept a ChatGPT/Codex OAuth credential.
5. Add strict configuration tests, fake-server discovery and inference tests,
   JSON and SSE passthrough tests, model-rewrite tests, redirect/origin
   isolation tests, and explicit tests proving a stored `openai-codex`
   credential cannot be sent by this profile. No real API key is required for
   the automated suite; any live smoke test must be opt-in and secret-safe.
6. Update the README, example configuration, architecture/provider notes, and
   this task record. Run the full repository checks and leave one focused
   commit. Do not mark TASK-014 done merely for adding documentation or a type
   alias; the configured profile must work through the public endpoints.

Previous attempts in this environment could not reach the official sources:
the web research service returned `401 Unauthorized`, and direct `curl` calls
to `platform.openai.com` and `api.openai.com` were rejected by the environment
proxy with `403`. Retry from an environment with official-source access rather
than copying the unverified placeholders in `docs/provider-notes.md`.

#### 2. Build the Codex subscription path

##### TASK-021 — Verify and implement ChatGPT Codex subscription authentication

**Depends on:** TASK-019, TASK-020

Document the current verified ChatGPT OAuth flow, refresh lifecycle, audience,
account identifiers, and persisted fields. Implement the supported login and
refresh flow with endpoint binding and tests proving API keys and OAuth tokens
cannot cross endpoints.

**Handoff instructions for the completing agent:**

1. Treat this as security-sensitive research before implementation. Use
   current official OpenAI documentation and the official `openai/codex`
   repository as primary sources. Pin every source-code finding to a commit
   SHA, record the verification date and direct links in the `OpenAI Codex
   Subscription` section of `docs/provider-notes.md`, and distinguish observed
   code behavior from documented guarantees.
2. Verify rather than guess all OAuth details: client identifier, authorization
   and token origins, device-code endpoints and polling errors, browser
   authorization-code + PKCE parameters, redirect URI, scopes, audience,
   refresh grant and rotation, expiry handling, account/principal selection,
   cancellation, and the exact endpoint/account binding required on requests.
   Never place real tokens, authorization codes, or identifying account values
   in the repository, fixtures, logs, errors, or task notes.
3. Read `internal/state/credentials.go`, the `gateway auth` implementation in
   `cmd/gateway/main.go`, `internal/config/config.go`, and the canonical types in
   `internal/transport/canonical.go` before changing shared contracts. Extend
   the existing credential store and CLI; do not create a second token store or
   silently reinterpret the normal OpenAI `bearer_token` configuration.
4. Add an explicit `openai-codex` provider configuration whose OAuth issuer,
   token client, persisted credential, refresh path, and eventual inference
   client are origin-bound. Validate loaded records against both provider ID
   and endpoint/issuer identity. Refresh early enough to avoid expiry during a
   request, coalesce concurrent refreshes, persist rotated refresh tokens
   atomically, and fail closed on corrupt, expired-without-refresh, wrong
   audience, wrong account, or endpoint-mismatched credentials.
5. Integrate verified device-code login as the default and browser PKCE only as
   supported by the verified upstream. Preserve non-interactive operation for
   status/logout and automation. Never print tokens; ensure cancellation and
   OAuth error paths have deterministic exit behavior.
6. Use fake authorization/token servers and injectable clocks/randomness for
   automated tests. Cover successful login, device polling (`pending`, pacing,
   denial, expiry), PKCE verification, refresh and rotation, restart, concurrent
   refresh, logout, redaction, account selection, cancellation, malformed
   responses, and strict cross-origin/cross-provider isolation. Add tests that
   normal OpenAI API keys cannot reach Codex endpoints and Codex OAuth tokens
   cannot reach `api.openai.com`. Keep any real login test opt-in.
7. Update provider notes before code, then update architecture, configuration,
   CLI/README documentation, and this task record. Run all repository checks
   and leave one focused commit. TASK-021 is complete only when login, durable
   restart, automatic refresh, logout, and the credential-isolation tests all
   pass; transport translation and inference remain TASK-022.

Official OpenAI/Codex sources were unreachable in the previous environment
(web research returned `401`; direct HTTPS was proxy-blocked with `403`). A
completing agent must repeat the research with working official-source access.
Do not implement from memory, Hermes behavior alone, or stale blog posts.

##### TASK-022 — Implement the ChatGPT Codex subscription transport

**Depends on:** TASK-017, TASK-021

Implement the verified Codex Responses protocol and catalog, translate supported
public requests and streams, preserve tools/reasoning/usage/errors, reject
unsupported features explicitly, and expose `openai-codex/<model>` IDs. Add
fixtures, fake-server tests, and opt-in live tests.

#### 3. Implement classification and smart selection

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

## Version 2 backlog (core adapters implemented in the current tree)

These tasks are deliberately unordered until Version 1 is complete. Before
implementation, refresh their requirements and dependencies against current
primary documentation.

- **TASK-013 — OpenRouter adapter:** implemented discovery, authentication,
  model metadata, transparent inference, response-header quota capture, and
  optional credits/balance polling through a separate management token.
- **TASK-015 — Gemini adapter:** implemented through the verified
  `/v1beta/openai` compatibility surface.
- **TASK-016 — xAI adapter:** implemented through the verified OpenAI-compatible
  `/v1` surface with provider-scoped credentials.
- **TASK-018 — Anthropic adapter:** implemented native Messages translation
  for Chat Completions and basic stateless Responses, including streaming,
  vision, client tools/tool results, usage, finish reasons, and reasoning;
  structured output is explicitly rejected as unsupported.
- **TASK-023 — Nous Portal adapter:** implemented subscription authentication,
  refresh, catalog pricing, inference, and response-header quota behavior.
- **Jev classifier adapters:** direct and provider-transport modes are implemented
  behind the Version 1 classifier contract; they must not replace or complicate
  the self-hosted Laya path.

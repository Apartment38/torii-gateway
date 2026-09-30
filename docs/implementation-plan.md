# Torii Router
A Personal Lightweight Intelligent LLM Gateway & Router.

## Version 1 Implementation Plan

### Status

This document defines the initial architecture and implementation plan for a lightweight, opinionated LLM gateway.

The intended audience is a coding agent implementing the project from scratch.

**Current status (2026-09-28): pre-v1.** The repository currently implements
an Ollama-backed transparent Chat Completions slice with authentication, model
discovery, and SSE relay. It is not Version 1 because the required endpoint
surface, providers, smart model, sessions, and quota/cost-aware routing are not
complete. `TASKS.md` is the authoritative ordered execution roadmap; this
document remains the Version 1 product and architecture contract. A build may
be called Version 1 only after TASK-041 verifies the definition of done.

### Version 1 scope boundary

Version 1 supports exactly these upstream families:

1. local Ollama instances;
2. standard OpenAI-compatible HTTP endpoints, including the normal API-key-
   billed OpenAI API and self-hosted compatible servers; and
3. ChatGPT Codex access backed by a user's subscription and isolated OAuth
   credentials.

Provider-specific OpenRouter, Nous Portal, Gemini, xAI, and Anthropic adapters
are Version 2 work. Jev is also deferred to Version 2. Version 1 instead allows
a self-hosted Laya model, addressed through a configured physical provider, to
produce the semantic classification used by `smart` routing. This boundary is
normative: later provider sections retained for design context do not expand
the Version 1 definition of done.

The design priorities are, in order:

1. Correct OpenAI-client compatibility.
2. Low resource consumption.
3. Low request-routing overhead.
4. Minimal operational complexity.
5. Few dependencies and moving parts.
6. Clear extension points for explicitly planned future features.
7. Predictable and explainable routing behavior.

The project name has intentionally not been specified. Use `gateway` as a temporary binary/module name until the project is named.

---

# 1. Core Requirements

Version 1 must:

- run as a single Go process
- run in one container
- target `linux/amd64`
- use YAML configuration
- use flat files for persistent state
- expose an OpenAI-compatible API
- protect that API with a bearer API key
- aggregate model catalogs from configured providers
- expose provider models as `provider/model`
- expose a virtual intelligent-routing model
- support streaming
- support tools/function calling
- support vision
- support reasoning-capable models
- support structured output / JSON schema
- support model affinity across an agent/tool session
- support quota-aware and cost-aware routing
- require no database
- require no Redis
- require no sidecar
- require no external message queue
- require no web interface
- require no Kubernetes infrastructure

Version 1 should remain useful even if the intelligent router is disabled.

A direct request such as:

```text
ollama-ziz/qwen3
```

must remain a simple, low-overhead proxy operation.

---

# 2. Non-Goals for Version 1

Do not implement these features yet:

- dedicated OpenRouter, Nous Portal, Gemini, xAI, or Anthropic adapters
- Jev classifier transports
- full accounting ledger
- historical accounting reports
- SQLite configuration
- SQLite accounting
- multiple client API keys
- users
- per-user spend
- per-user quotas
- custom user-defined routing policies
- model aliases
- model allow/deny filters
- manual capability overrides
- web UI
- Prometheus
- distributed deployment
- Redis
- Kubernetes-specific functionality
- MCP gateway
- tool gateway
- skill repository/gateway

The v1 architecture must make these possible later without requiring a rewrite.

---

# 3. Process Architecture

Use a **modular monolith**.

Do not split the gateway into microservices.

```text
                     OpenAI-compatible client
                  Hermes / Pi / OpenCode / SDK
                              │
                              ▼
                  ┌──────────────────────┐
                  │ Client API Auth      │
                  └──────────┬───────────┘
                             │
                             ▼
                  ┌──────────────────────┐
                  │ OpenAI HTTP API      │
                  └──────────┬───────────┘
                             │
                    requested model
                             │
              ┌──────────────┴─────────────┐
              │                            │
       explicit model                    smart
              │                            │
              │                            ▼
              │                      Session Resolver
              │                            │
              │                ┌───────────┴───────────┐
              │                │                       │
              │             pinned                  unpinned
              │                │                       │
              │                │                       ▼
              │                │                  Classifier
              │                │                       │
              │                │                       ▼
              │                │                Requirement Filter
              │                │                       │
              │                │                       ▼
              │                │                 Model Selector
              │                │                       │
              └────────────────┴───────────┬───────────┘
                                           │
                                           ▼
                                    Provider Registry
                                           │
                           ┌───────────────┼──────────────┐
                           ▼               ▼              ▼
                  OpenAI-compatible   OpenAI Codex     Ollama
                  (cloud or local)    subscription     (local)
```

All components run in one address space.

---

# 4. Transport Architecture

This is an important architectural distinction.

There must be **two upstream transport paths**.

## 4.1 Transparent OpenAI-compatible path

Use this whenever the upstream accepts the same OpenAI request format being presented by the gateway.

Examples include:

- the standard OpenAI API
- self-hosted OpenAI-compatible endpoints
- Ollama

Flow:

```text
client JSON
    │
    ├─ inspect required routing fields
    │
    ├─ replace public model ID with upstream model ID
    │
    ▼
upstream
```

Unknown request fields must be preserved.

Do not deserialize and recreate the entire request unnecessarily.

This minimizes:

- CPU
- allocations
- compatibility regressions
- maintenance burden

## 4.2 Translated transport path

Use a canonical internal representation only where protocol translation is actually required.

Examples:

```text
OpenAI Chat/Responses
       ↓
OpenAI Codex Responses

```

The canonical representation must support:

- system/developer instructions
- user messages
- assistant messages
- text content
- image content
- tools
- tool calls
- tool results
- structured-output requirements
- reasoning parameters
- token/output limits

Do not force transparent OpenAI-compatible traffic through the translation layer.

---

# 5. Public API Surface

Version 1 targets the same OpenAI-compatible endpoint set currently exposed by Ollama:

```text
GET  /v1/models
GET  /v1/models/{model}

POST /v1/chat/completions
POST /v1/completions
POST /v1/embeddings
POST /v1/responses
```

Ollama currently supports this subset, including streaming, tools, vision, structured output fields, reasoning controls, and stateless Responses.

Version 1 of this gateway should likewise treat Responses as **stateless**.

Do not implement:

```text
previous_response_id
conversation state storage
```

as actual server-side Responses state in v1.

If unsupported stateful Responses fields are supplied, return a clear OpenAI-style unsupported-parameter error rather than silently ignoring them.

---

# 6. Gateway-Specific Endpoints

Keep gateway extensions outside `/v1`.

Required:

```text
GET /healthz
```

This should return process health only.

It does not need to verify every upstream provider.

Also provide:

```text
DELETE /gateway/v1/sessions/{session_id}
```

for explicit session-affinity termination.

All `/v1/*` and `/gateway/*` endpoints require gateway authentication.

`/healthz` may remain unauthenticated.

---

# 7. Provider Scope

Only Sections 7.2, 7.3, 7.8, and the compatible-endpoint configuration in
Section 7.9 are required for Version 1. All other provider-specific sections
are retained as a Version 2 backlog.

Provider IDs must:

- be unique
- contain no `/`
- preferably match:

```text
[a-z0-9][a-z0-9._-]*
```

Public model names are:

```text
<provider-id>/<upstream-model-id>
```

Split public IDs on the **first slash only**.

This is necessary because many upstream model IDs themselves contain slashes.

Example:

```text
openrouter/google/gemini-2.5-pro
```

parses as:

```text
provider:
openrouter

upstream model:
google/gemini-2.5-pro
```

---

## 7.1 OpenRouter (Version 2)

Authentication:

```text
API key
```

Use the OpenRouter OpenAI-compatible API.

Responsibilities:

- model discovery
- inference
- pricing metadata ingestion when available
- context/capability metadata ingestion when available
- quota/credit information when available

Public model IDs:

```text
openrouter/<openrouter-model-id>
```

Example:

```text
openrouter/google/gemini-2.5-pro
```

Do not duplicate OpenRouter's internal provider-routing logic inside this gateway.

Treat OpenRouter as one upstream provider.

---

## 7.2 Standard OpenAI-Compatible Endpoints (Version 1)

This family covers normal API-billed OpenAI access and arbitrary cloud or
self-hosted endpoints that implement the standard OpenAI HTTP surface. The
official API uses the `openai` namespace; custom endpoint IDs are configured by
the operator. Provider-specific behavior for OpenRouter, Gemini, and xAI is not
part of Version 1 even when those services offer partly compatible APIs.

Authentication:

```text
OPENAI_API_KEY
```

Public namespace:

```text
openai/<model>
```

Example:

```text
openai/gpt-5.6
```

Keep this provider strictly distinct from OpenAI Codex subscription access.

---

## 7.3 OpenAI Codex Subscription

This provider represents ChatGPT Plus/Pro subscription-backed Codex access.

It is a normal v1 provider, not a future experiment.

Hermes currently implements this as a distinct `openai-codex` provider using ChatGPT OAuth and a `codex_responses` transport. Its current runtime points the provider at the ChatGPT Codex backend rather than the normal OpenAI API.

Public namespace:

```text
openai-codex/<model>
```

### Authentication

Implement ChatGPT/Codex OAuth based on the current Codex/Hermes authentication flow.

Default:

```text
device-code authentication
```

Also support browser authorization-code + PKCE where required by the upstream flow.

Hermes currently supports device-code login by default and browser PKCE as an alternative.

CLI:

```text
gateway auth login openai-codex
gateway auth logout openai-codex
```

Persist:

- access token
- refresh token
- expiry data
- account/principal data required by the protocol

under:

```text
/state/auth/
```

Refresh automatically.

### Transport

Implement a dedicated:

```text
CodexResponsesTransport
```

Do not send ChatGPT OAuth tokens to:

```text
api.openai.com
```

Do not send normal `OPENAI_API_KEY` credentials to the Codex subscription endpoint.

Credentials and endpoint identity must remain bound together.

This is a hard security invariant.

### Protocol adaptation

The public gateway may receive:

```text
/v1/chat/completions
```

while the upstream Codex provider uses the Responses-style transport.

Translate internally:

```text
Chat Completions
      ↓
canonical request
      ↓
Codex Responses
      ↓
canonical response
      ↓
Chat Completions
```

Likewise, `/v1/responses` should map more directly.

### Model discovery

Use the live Codex-accessible catalog when an upstream mechanism exists.

If live discovery is unavailable, use the bundled model database for this provider and mark entries as:

```text
catalog_source: bundled
```

Do not assume models available through the normal OpenAI API are automatically available through the Codex subscription.

---

## 7.4 Nous Portal (Version 2)

Support Nous Portal as a subscription/OAuth provider.

Public namespace:

```text
nous/<model>
```

Hermes currently stores a Portal OAuth refresh token separately from configuration and derives short-lived inference credentials from it.

### OAuth

CLI:

```text
gateway auth login nous
gateway auth logout nous
```

Use the OAuth method required by the current Portal implementation.

Do not hardcode a particular login mechanism if Nous changes between:

- browser callback
- device code
- another OAuth grant

The provider adapter owns this behavior.

Persist the long-lived refresh credential under:

```text
/state/auth/
```

Use short-lived inference credentials as required by Portal.

Refresh them lazily before use.

Do not require a background refresh worker in v1.

### API-key authentication

If the current Nous inference API officially supports a reusable API key, expose:

```yaml
authentication:
  method: api_key
```

If it does not, do not emulate one or scrape browser/session credentials.

OAuth support is mandatory.

API-key support is conditional on confirmed upstream support.

---

## 7.5 Google Gemini API (Version 2)

Authentication:

```text
Gemini API key
```

Public namespace:

```text
gemini/<model>
```

Prefer Google's OpenAI-compatible interface wherever it provides the required functionality.

Reuse the transparent OpenAI transport where possible.

Add provider-specific translation only for features that cannot be represented by the compatible interface.

---

## 7.6 xAI / Grok API (Version 2)

Authentication:

```text
xAI API key
```

Public namespace:

```text
xai/<model>
```

Use OpenAI-compatible xAI interfaces wherever possible.

Do not implement subscription/SuperGrok OAuth in v1 unless explicitly added to the project scope later.

This requirement refers to the normal Grok API.

---

## 7.7 Anthropic API (Version 2)

Authentication:

```text
Anthropic API key
```

Public namespace:

```text
anthropic/<model>
```

Use Anthropic's native Messages protocol internally when required.

Implement an explicit translation layer between the gateway's canonical request and Anthropic.

Must preserve:

- system instructions
- user/assistant content
- image blocks
- tool schemas
- tool calls
- tool results
- streaming
- token limits
- reasoning/thinking fields where compatible

Unsupported feature combinations must return a capability error rather than being silently discarded.

---

## 7.8 Local Ollama

Support any number of Ollama instances.

Example:

```yaml
providers:
  - id: ollama-ziz
    type: ollama
    base_url: http://ziz:18081
```

Public models:

```text
ollama-ziz/qwen3
ollama-ziz/qwen3-coder
```

Another server may simultaneously exist:

```text
ollama-laptop/qwen3:8b
```

Use Ollama's OpenAI-compatible API for inference where possible.

Use Ollama's native model metadata endpoints where they provide richer capability information than `/v1/models`.

---

## 7.9 Custom OpenAI-Compatible Providers (Version 1)

Support an arbitrary number.

Example:

```yaml
providers:
  - id: ziz-vllm
    type: openai-compatible
    base_url: http://ziz:8000/v1

    authentication:
      api_key:
        env: VLLM_API_KEY
```

Required v1 options:

```text
base URL
optional bearer API key
no-auth mode
arbitrary request headers
secret-backed headers
custom CA bundle
TLS verification toggle
model discovery endpoint
static model list
transport selection
model-name rewriting
request timeout
connect timeout
```

Example:

```yaml
providers:
  - id: custom-ai
    type: openai-compatible

    base_url: https://example.internal/v1

    transport: auto

    authentication:
      api_key:
        file: /run/secrets/custom-ai

    headers:
      X-Tenant:
        value: homelab

      X-Private-Key:
        env: CUSTOM_PRIVATE_KEY

    tls:
      verify: true
      ca_file: /config/internal-ca.pem

    models:
      discovery: true
```

Supported transport values:

```text
auto
chat_completions
responses
```

`auto` should probe/discover conservatively rather than guessing from model names.

---

# 8. Model Catalog

At startup:

```text
configured providers
        ↓
provider authentication resolution
        ↓
model discovery
        ↓
normalization
        ↓
capability enrichment
        ↓
global catalog
```

A provider discovery failure must **not** terminate the entire gateway unless no provider remains usable.

Behavior:

```text
invalid provider configuration
    → startup failure

configured provider unreachable
    → warning + provider unavailable

bad provider credentials
    → warning + provider unavailable

zero usable providers
    → startup failure
```

`GET /v1/models` exposes every successfully discovered/enabled v1 model plus the smart virtual model.

There is no model filtering system in v1.

---

# 9. Public Model IDs

Every physical model uses:

```text
provider/model
```

The virtual router uses:

```text
smart
```

The router name should be configurable:

```yaml
router:
  model_name: smart
```

Provider IDs may not contain `/`.

Upstream model IDs may contain any number of `/`.

For:

```text
GET /v1/models/{model}
```

support slash-containing IDs correctly.

The handler must treat the remainder of the path as the model ID or accept the URL-encoded equivalent.

Do not assume model IDs contain one path segment.

---

# 10. Internal Model Descriptor

Use a normalized model descriptor.

Conceptually:

```go
type ModelDescriptor struct {
    PublicID       string
    ProviderID     string
    UpstreamID     string

    EndpointSupport EndpointCapabilities
    Capabilities    ModelCapabilities

    ContextWindow   int64

    Pricing         PricingInfo
    Quality         QualityProfile
    Quota           QuotaStatus

    Local           bool

    Metadata        MetadataProvenance
}
```

Capabilities include:

```text
chat
completions
responses
embeddings
streaming
tools
vision
structured_output
reasoning
```

Unknown values should remain explicitly unknown rather than being guessed.

---

# 11. Bundled Capability Database

Embed a capability/model database in the binary using:

```go
embed.FS
```

Use JSON internally for the embedded database.

Do not fetch a remote capability database during normal gateway startup.

A maintainer/build tool may regenerate the bundled data from reliable model/provider catalogs.

Each record should contain:

```text
model family / exact match patterns
capabilities
context window
pricing when known
task-quality scores
tool-use quality
reasoning quality
metadata source
metadata timestamp
```

Example conceptual record:

```json
{
  "provider": "openrouter",
  "model": "vendor/model",
  "capabilities": {
    "tools": true,
    "vision": true,
    "reasoning": true,
    "structured_output": true
  },
  "context_window": 262144,
  "quality": {
    "general": 0.88,
    "coding": 0.94,
    "reasoning": 0.91,
    "tool_use": 0.89
  }
}
```

---

# 12. Metadata Merge Rules

Merge metadata field-by-field.

Priority:

```text
1. authoritative live provider metadata
2. bundled capability database
3. unknown
```

Examples:

If a provider explicitly says:

```text
context_window = 128000
```

that wins over a stale bundled value.

If the provider does not expose tool support but the bundled database knows it:

```text
tools = true
```

use the bundled value.

If neither source knows:

```text
tools = unknown
```

Do not infer critical capabilities from model names.

Manual overrides are deferred to a later version.

---

# 13. Smart Virtual Model

From the client's perspective, `smart` behaves like any other model.

Example:

```json
{
  "model": "smart",
  "messages": [
    {
      "role": "user",
      "content": "Debug this Go race condition."
    }
  ]
}
```

Internal flow:

```text
request
   ↓
extract objective requirements
   ↓
resolve session
   ↓
existing valid affinity?
   │
   ├─ yes → pinned model
   │
   └─ no
        ↓
      classifier
        ↓
     Classification
        ↓
     candidate filtering
        ↓
     quality eligibility
        ↓
     deterministic selection
        ↓
     session pin
        ↓
     provider request
```

The classifier does **not** return a provider/model.

The classifier describes the task.

The selector chooses the provider/model.

---

# 14. Classification Schema

All classifiers produce the same internal structure.

Example:

```yaml
task_type: coding
complexity: high
confidence: 0.93

requirements:
  tools: true
  vision: false
  reasoning: true
  structured_output: false

preferences:
  latency_sensitive: false
```

Initial task vocabulary:

```text
general
conversation
coding
sysadmin
reasoning
math
research
summarization
extraction
creative
translation
```

Initial complexity vocabulary:

```text
low
medium
high
```

Do not continuously expand this taxonomy without a demonstrated routing need.

---

# 15. Objective Request Requirements

Do not rely on the classifier for facts visible directly in the request.

Determine requirements before classification.

Examples:

```text
tools[] present
    → tools required

image content present
    → vision required

response_format / JSON schema present
    → structured output required

reasoning parameter present
    → reasoning support required

/v1/embeddings
    → embeddings required
```

Also determine:

```text
estimated context size
requested output limit
streaming requested
```

Objective request requirements override classifier guesses.

---

# 16. Classifier Interface

Conceptual interface:

```go
type Classifier interface {
    ID() string

    Classify(
        context.Context,
        ClassificationRequest,
    ) (Classification, error)
}
```

A classifier must not be allowed to call the virtual `smart` model recursively.

If a connected model is used for classification, invoke the physical provider/model directly through the internal provider registry.

---

# 17. Classifier Implementations

Version 1 supports three classifier families.

---

## 17.1 Self-hosted Laya

Support a self-hosted Laya routing model through a configured physical
OpenAI-compatible or Ollama provider. Laya is a classifier, not an inference
provider selected to answer the user's request.

```text
LayaClassifier
     |
     +-- ProviderRegistry
             |
             +-- configured local provider/model
```

Configuration identifies the physical provider-qualified model. It must never
resolve to `smart`, and the gateway must validate Laya's output against the
common classification schema before using it. No cloud Laya service or special
credential flow is required in Version 1.

Example:

```yaml
router:
  classifier:
    type: laya
    model: local/laya
```

Keep classification semantics separate from provider transport so Laya can run
on any supported local compatible runtime. Jev transports may be added in
Version 2 without changing the router contract.

---

## 17.2 Ollama Embedding Classifier

Do not introduce a vector database.

Use:

```text
incoming request text
       ↓
local embedding model
       ↓
embedding vector
       ↓
cosine similarity
       ↓
in-memory prototype embeddings
```

Maintain small prototype sets for:

```text
task type
complexity
```

Generate prototype embeddings once at process startup.

Store them only in memory.

Configuration:

```yaml
router:
  classifier:
    type: embedding
    provider: ollama-ziz
    model: nomic-embed-text
```

Hard requirements such as tools/vision still come from request inspection rather than the embedding classifier.

---

## 17.3 Connected Model Classifier

Allow any suitable physical model from the catalog to classify requests.

Example:

```yaml
router:
  classifier:
    type: model
    model: gemini/gemini-flash
```

Send a compact classification prompt.

Request structured output matching the internal schema when supported.

If the model does not support structured output:

1. request JSON-only output
2. parse strictly
3. retry malformed classifier output once
4. if still invalid, invoke classifier-failure behavior

Do not pass raw classifier prose to routing logic.

---

# 18. Classifier Failure Behavior

Classifier failure must be deterministic.

Configuration:

```yaml
router:
  classifier:
    on_failure: capability_fallback
```

For v1, support:

```text
capability_fallback
fail
```

`capability_fallback` means:

1. extract objective requirements
2. skip semantic task classification
3. choose among models capable of satisfying those hard requirements
4. use `general` + `medium` as routing classification
5. follow the normal cost-selection algorithm

Do not select a model lacking required capabilities merely because classification failed.

---

# 19. Session Identification

Model affinity must work without requiring a gateway-specific feature from every client.

Resolve sessions using the following hierarchy.

## 19.1 Explicit header

Highest priority:

```http
X-Session-ID: abc123
```

This is optional.

## 19.2 Standard request metadata

If available, inspect recognized metadata fields such as:

```text
metadata.session_id
metadata.conversation_id
metadata.thread_id
```

Do not treat arbitrary metadata as session identity.

## 19.3 OpenAI `user`

`user` alone is **not** a session ID.

It may be used as part of a derived identity.

## 19.4 Conversation fingerprint

For stateless Chat Completions clients that resend conversation context, create a best-effort fingerprint from stable initial context:

```text
API family
+
user identifier if present
+
system/developer prompt hash
+
first user-turn hash
```

Use SHA-256.

Store only the hash.

Do not store prompt content solely for session matching.

If the request has insufficient stable context to distinguish a conversation, treat it as a new session rather than risking affinity to an unrelated conversation.

---

# 20. Session State

Keep session state in memory only for v1.

Conceptual structure:

```go
type Session struct {
    ID              string
    Model           ModelRef
    Classification  Classification
    CreatedAt       time.Time
    LastSeen        time.Time
    ContextSignature string
    ToolLoopActive  bool
}
```

Use a concurrency-safe in-memory map.

No Redis.

No disk persistence.

---

# 21. Session Expiration

Affinity ends under three conditions.

## 21.1 Explicit termination

```text
DELETE /gateway/v1/sessions/{session}
```

## 21.2 Inactivity

Configurable:

```yaml
router:
  sessions:
    inactivity_timeout: 30m
```

Use lazy expiry on access plus one low-frequency cleanup goroutine.

This is not a separate worker service.

## 21.3 Context reset

If a previously contextual request suddenly no longer supplies the conversation context needed to represent the same conversation, invalidate affinity.

Be conservative.

When ambiguous:

```text
preserve affinity
```

When clearly a new context:

```text
discard affinity
reclassify
```

An explicit `X-Session-ID` does not override a clearly detected context reset.

---

# 22. Agent and Tool-Call Affinity

Tool loops are a first-class case.

Detect:

```text
assistant.tool_calls
role=tool
tool_call_id
Responses function-call items
Responses function-call output items
```

Once a physical model begins a tool loop, keep that model pinned.

Do not reclassify after every tool result.

Desired behavior:

```text
user
  ↓
smart chooses model A
  ↓
model A tool call
  ↓
tool result
  ↓
model A
  ↓
second tool call
  ↓
tool result
  ↓
model A final response
```

---

# 23. Model Failure During a Session

Configuration:

```yaml
router:
  sessions:
    on_model_failure: reroute
```

Allowed:

```text
fail
reroute
```

## `fail`

Return the normalized upstream error.

## `reroute`

Only reroute when the current request has not already emitted response content to the client.

Eligible pre-response failures include:

```text
quota exhausted
rate limit
connection failure
timeout before response
upstream 5xx
model unavailable
provider authentication failure
```

Do not reroute for:

```text
invalid user request
unsupported parameter
malformed tool schema
context-too-large caused by request itself
```

---

# 24. Streaming Failure Rule

This must be explicit.

Once any streamed response bytes have been sent to the client:

```text
DO NOT switch upstream models inside that HTTP response
```

Doing so would produce an invalid or semantically corrupted stream.

If a stream fails after output begins:

1. terminate the stream cleanly where possible
2. mark the session's current model as failed/unavailable as appropriate
3. allow the next client request to invoke configured rerouting behavior

Automatic handoff applies only before response streaming begins or between agent turns.

---

# 25. Handoff Notification

When a model handoff occurs:

## Model-facing notice

Inject a synthetic developer/system-level context message for the replacement model:

```text
A model handoff occurred during this conversation.
The previous model became unavailable because: <reason>.
Continue from the supplied conversation and tool context.
```

Do not present this as a user message.

## Client-facing notice

Return headers:

```http
X-Gateway-Handoff: true
X-Gateway-Previous-Model: provider/model
X-Gateway-New-Model: provider/model
X-Gateway-Handoff-Reason: quota_exhausted
```

Do not prepend human-readable warning text to the model's response body.

That would break:

- JSON-schema responses
- machine-readable clients
- tool outputs
- structured output

The response headers are the client-facing warning mechanism.

---

# 26. Quota Model

Define:

```go
type QuotaStatus struct {
    State      QuotaState
    Remaining  *float64
    Limit      *float64
    Unit       string
    ResetAt    *time.Time
}
```

States:

```text
known
unknown
exhausted
```

Sources:

```text
provider live quota information
provider response headers
locally configured allowance
locally tracked usage
```

If neither the provider nor local configuration can determine quota:

```text
unknown
```

Per the v1 requirement, unknown quota remains normally eligible.

---

# 27. Minimal V1 Usage State

Full accounting remains deferred, but configured quota enforcement requires minimal persistent counters.

Implement only:

```text
input tokens
output tokens
estimated spend
```

aggregated per provider/model.

Store:

```text
/state/usage.json
```

Use:

```text
temporary file
fsync where appropriate
atomic rename
```

Do not store one historical row per request in v1.

This file is quota state, not the future accounting ledger.

Expose storage behind:

```go
type UsageStore interface {
    Add(UsageDelta) error
    Get(ModelRef) (UsageSummary, error)
}
```

Future implementations can add:

```text
CSV ledger
SQLite
```

without altering routing logic.

---

# 28. Quota Refresh Strategy

Do not poll all providers continuously.

Use lazy refresh.

Maintain a small in-memory quota cache.

Refresh when:

```text
cache missing
or
cache expired
```

Default TTL:

```text
60 seconds
```

Make the TTL configurable.

Also update quota state opportunistically from provider response headers.

This avoids a permanent background quota-polling subsystem.

---

# 29. Smart Model Selection

Use a deterministic, explainable selection algorithm.

Do not use a vague weighted “AI score” in v1.

Selection happens in stages.

---

## 29.1 Stage 1 — Hard Capability Filter

Remove any candidate that cannot satisfy the request.

Examples:

```text
wrong API endpoint
missing tool support
missing vision
missing structured output
missing reasoning support
missing embedding support
insufficient context window
quota exhausted
disallowed paid model
```

A cheaper incapable model may never beat a capable model.

---

## 29.2 Context Headroom

Do not select a model whose context window would be nearly exhausted by the current request.

Use an estimated-token safety margin.

Default:

```text
maximum estimated input usage = 85% of context window
```

Make this configurable.

Implement token estimation behind:

```go
type TokenEstimator interface {
    Estimate(RequestContent) int64
}
```

The first implementation may use a lightweight conservative heuristic.

Do not introduce large tokenizer runtimes solely for v1 routing.

Actual provider usage data should replace estimates after completion.

---

## 29.3 Stage 2 — Quality Eligibility

The bundled model database supplies normalized quality values from:

```text
0.0 → 1.0
```

for relevant categories.

Map classifier complexity to minimum task quality.

Initial defaults:

```text
low:    0.55
medium: 0.70
high:   0.82
```

Keep these in configuration defaults so they can be changed without altering code.

For requests containing tools, additionally enforce a minimum tool-use quality when known.

For reasoning-heavy requests, enforce reasoning-quality minimums when known.

### Unknown quality

Unknown-quality models are not immediately excluded.

Selection policy:

1. prefer known-quality models meeting the requirement
2. if none exist, admit unknown-quality capable models
3. prefer known quality over unknown quality when other factors are comparable

---

## 29.4 Stage 3 — Paid Model Policy

Configuration:

```yaml
router:
  allow_paid_models: true
```

Default:

```text
true
```

If:

```text
allow_paid_models: false
```

only models positively known to be:

```text
local
or
zero-cost
```

are eligible.

A model with unknown pricing must be excluded in free-only mode.

This prevents accidentally spending money.

---

## 29.5 Stage 4 — Estimated Cost

For each remaining candidate, estimate request cost.

Use:

```text
estimated input tokens
×
input-token price

+

estimated output tokens
×
output-token price
```

If the request supplies an output limit, use it as the upper-bound output estimate.

Otherwise use configurable:

```yaml
router:
  selection:
    default_expected_output_tokens: 1024
```

This estimate exists only for routing.

Actual usage replaces estimated usage after the request completes.

Local models have monetary inference cost:

```text
0
```

for v1 routing purposes.

Electricity/hardware amortization is outside v1 scope.

---

## 29.6 Stage 5 — Deterministic Ordering

Among quality-eligible candidates, order by:

1. lowest estimated monetary cost
2. greater remaining quota fraction, when known
3. configured local/cloud preference
4. lower EWMA request latency
5. greater context-window headroom
6. stable lexical public model ID

This implements the desired principle:

> choose the cheapest model expected to perform the job well

rather than:

> choose the cheapest model available

---

# 30. Local-vs-Cloud Preference

Support one simple v1 setting:

```yaml
router:
  selection:
    location_preference: neutral
```

Allowed:

```text
neutral
local
cloud
```

This is a tie-break preference, not a hard rule.

Default:

```text
neutral
```

Custom routing policies are still deferred.

---

# 31. Latency Tracking

Maintain an in-memory EWMA per physical model.

Example:

```go
type RuntimeStats struct {
    LatencyEWMA time.Duration
    Samples     uint64
}
```

Unknown latency is neutral.

Do not persist latency data.

Do not implement provider-reliability scoring in v1.

---

# 32. Explicit Model Requests

A physical model request must be deterministic.

If the client requests:

```text
openrouter/vendor/model
```

the gateway must use exactly that physical model.

Do not silently substitute another physical model.

Automatic provider/model substitution occurs only for:

```text
smart
```

This keeps debugging predictable.

---

# 33. Client API Authentication

Protect `/v1/*` using:

```http
Authorization: Bearer <gateway-api-key>
```

Secrets may come only from:

```text
environment variable
file
```

Example:

```yaml
server:
  authentication:
    api_key:
      env: GATEWAY_API_KEY
```

or:

```yaml
server:
  authentication:
    api_key:
      file: /run/secrets/gateway-api-key
```

Do not allow plaintext gateway secrets directly in YAML in v1.

Use constant-time comparison.

---

# 34. Authentication Extension Point

Define:

```go
type ClientAuthenticator interface {
    Authenticate(*http.Request) (*ClientIdentity, error)
}
```

V1 implementation:

```text
StaticBearerAuthenticator
```

V1 identity:

```go
ClientIdentity{
    ID: "default",
}
```

Later this supports:

```text
multiple API keys
users
permissions
per-user quota
per-user accounting
```

without replacing HTTP handlers.

---

# 35. Secret References

Generic structure:

```go
type SecretRef struct {
    Env  string `yaml:"env,omitempty"`
    File string `yaml:"file,omitempty"`
}
```

Exactly one may be configured.

Secrets loaded from files should have surrounding whitespace/newline trimmed.

Never include secret values in:

- logs
- errors
- debug dumps
- model metadata

---

# 36. Credential Persistence

OAuth credentials live under:

```text
/state/auth/
```

Use a storage abstraction:

```go
type CredentialStore interface {
    Load(provider string) (*Credential, error)
    Save(provider string, *Credential) error
    Delete(provider string) error
}
```

V1 implementation:

```text
JSON flat files
0600 permissions
atomic replacement
```

One provider per file is preferable:

```text
/state/auth/openai-codex.json
/state/auth/nous.json
```

This limits corruption scope and simplifies future migration.

Future implementation:

```text
SQLiteCredentialStore
```

---

# 37. Configuration

Use YAML.

Suggested structure:

```yaml
server:
  listen: 0.0.0.0:8080

  authentication:
    api_key:
      env: GATEWAY_API_KEY

state:
  directory: /state

providers:
  - id: ollama-ziz
    type: ollama
    base_url: http://ziz:18081

  - id: openrouter
    type: openrouter
    authentication:
      api_key:
        env: OPENROUTER_API_KEY

  - id: openai
    type: openai
    authentication:
      api_key:
        env: OPENAI_API_KEY

  - id: openai-codex
    type: openai-codex

  - id: nous
    type: nous
    authentication:
      method: oauth

router:
  model_name: smart

  allow_paid_models: true

  classifier:
    type: jev

    transport:
      type: provider
      provider: openrouter
      model: typesafe/jev

    on_failure: capability_fallback

  sessions:
    inactivity_timeout: 30m
    on_model_failure: reroute

  quota:
    cache_ttl: 60s

  selection:
    context_utilization_limit: 0.85
    default_expected_output_tokens: 1024
    location_preference: neutral

    minimum_quality:
      low: 0.55
      medium: 0.70
      high: 0.82
```

Validate all configuration at startup.

Unknown YAML keys should produce an error rather than being silently ignored.

This prevents configuration typos from becoming routing behavior.

---

# 38. Configuration Reload

Do not implement hot reload in v1.

Configuration changes require process restart.

OAuth token refresh does not count as configuration reload.

---

# 39. CLI

Use the same binary for server and administrative commands.

Required commands:

```text
gateway serve

gateway config validate

gateway models

gateway auth login <provider>

gateway auth logout <provider>

gateway auth status
```

`serve` should be the default container command.

Authentication commands should work against the same mounted `/state` directory.

Recommended container workflow:

```text
run auth command with /state mounted
then
start/restart gateway container
```

Do not require a separate admin daemon.

---

# 40. HTTP Implementation

Prefer the Go standard library:

```text
net/http
context
encoding/json
crypto/subtle
crypto/sha256
embed
log/slog
```

Use external dependencies only where they materially simplify correctness.

Expected external dependencies:

```text
YAML parser
OAuth2 support where useful
```

Do not introduce a large HTTP framework without a concrete requirement.

---

# 41. HTTP Client Reuse

Maintain one shared `http.Client` / transport per upstream provider configuration.

Do not construct clients per inference request.

Configure:

```text
keep-alive
connection pooling
HTTP/2 where supported
TLS
connect timeouts
idle timeouts
proxy environment support
```

Streaming response timeouts must not terminate legitimate long-running generations.

Use request context cancellation.

When the downstream client disconnects:

```text
cancel upstream immediately
```

---

# 42. SSE Streaming

Streaming is mandatory.

For transparent transports:

```text
upstream SSE
    ↓
minimal inspection
    ↓
immediate downstream flush
```

Do not buffer full responses.

Requirements:

- preserve event order
- preserve streamed tool-call deltas
- preserve reasoning fields where supported
- flush promptly
- capture usage events if available
- cancel on downstream disconnect
- never replay a partial stream through another model

---

# 43. Error Model

Normalize public errors into OpenAI-style error objects.

Example:

```json
{
  "error": {
    "message": "Model does not support vision",
    "type": "invalid_request_error",
    "code": "model_capability_mismatch"
  }
}
```

Internal typed errors should include:

```text
invalid_request
authentication_failure
provider_authentication_failure
rate_limited
quota_exhausted
provider_unavailable
model_unavailable
model_not_found
unsupported_endpoint
unsupported_capability
context_too_large
classifier_failure
stream_interrupted
```

Routing logic acts on typed errors, not error-message string matching.

---

# 44. Provider Credential Isolation

This is a hard requirement.

Provider credentials must be scoped to the provider configuration that owns them.

Never infer:

```text
"this looks OpenAI-compatible, so use OPENAI_API_KEY"
```

for an arbitrary endpoint.

A credential object should carry or be associated with:

```text
provider ID
allowed host/base URL
authentication type
```

This prevents accidentally sending:

```text
OpenAI API key → Codex endpoint
OpenRouter key → custom endpoint
Nous token → unrelated endpoint
```

Provider switches must switch:

```text
model
transport
base URL
credentials
```

as one logical route.

Never retain stale route state from the previous provider.

---

# 45. Model Discovery Refresh

V1 performs catalog discovery:

```text
at startup
```

No periodic background catalog refresh is required.

Restart the gateway to refresh provider catalogs.

This is consistent with the v1 no-hot-reload design.

A manual refresh API may be added later.

---

# 46. Logging

Use:

```go
log/slog
```

Support:

```text
error
warn
info
debug
```

Default:

```text
info
```

Never log:

- provider API keys
- OAuth access tokens
- refresh tokens
- full Authorization headers
- image contents
- full user prompts at normal log levels

Debug logging may include:

```text
provider ID
model ID
endpoint family
routing decision
classification
quota state
handoff reason
request ID
```

but not secrets.

---

# 47. Request IDs

Generate or propagate a request identifier.

Accept a suitable existing client request ID if present.

Otherwise generate one.

Return:

```http
X-Request-ID: ...
```

Use it consistently in gateway logs.

Do not use request ID as session ID.

---

# 48. Suggested Go Package Layout

Keep package count moderate.

```text
cmd/
  gateway/
    main.go

internal/
  api/
    server.go
    auth.go
    openai.go
    streaming.go
    errors.go

  config/
    config.go
    secrets.go
    validate.go

  provider/
    provider.go
    registry.go
    openai_compatible.go
    openrouter.go
    openai.go
    openai_codex.go
    nous.go
    gemini.go
    xai.go
    anthropic.go
    ollama.go
    custom.go

  transport/
    passthrough.go
    canonical.go
    codex_responses.go
    anthropic_messages.go

  catalog/
    catalog.go
    descriptor.go
    capabilities.go
    models.json

  classifier/
    classifier.go
    jev.go
    embedding.go
    model.go

  router/
    router.go
    requirements.go
    selector.go
    cost.go

  session/
    store.go
    identity.go

  quota/
    quota.go
    usage.go

  state/
    credentials.go
    flatfile.go
```

Do not create interfaces solely because an implementation might theoretically exist someday.

Create them where:

- multiple implementations already exist, or
- a specifically planned future implementation exists.

---

# 49. Provider Interfaces

Avoid one giant provider interface.

Prefer small capability interfaces.

Conceptually:

```go
type Provider interface {
    ID() string
    Models(context.Context) ([]ModelDescriptor, error)
}
```

Inference:

```go
type InferenceProvider interface {
    Execute(context.Context, RequestEnvelope) (*ProviderResponse, error)
}
```

Quota:

```go
type QuotaProvider interface {
    Quota(context.Context) (QuotaStatus, error)
}
```

OAuth providers may additionally implement:

```go
type OAuthProvider interface {
    Login(context.Context, LoginOptions) error
    Refresh(context.Context) error
}
```

This avoids meaningless stub methods for providers that cannot expose quota or OAuth.

---

# 50. Request Envelope

Use a request wrapper that preserves both raw and parsed information.

Conceptually:

```go
type RequestEnvelope struct {
    Endpoint EndpointType

    RawBody []byte

    ModelPublicID   string
    ModelUpstreamID string

    Requirements RequestRequirements

    SessionHint SessionHint

    Stream bool
}
```

Transparent transports primarily use:

```text
RawBody
```

Translated transports use a lazily decoded canonical request.

Do not decode twice.

---

# 51. Future Accounting Extension

Full accounting should consume events emitted after inference.

Define now:

```go
type UsageEvent struct {
    Timestamp time.Time

    ClientID   string
    ProviderID string
    ModelID    string

    InputTokens  int64
    OutputTokens int64

    EstimatedCost float64
    ActualCost    *float64
}
```

V1 uses these events only to update aggregate quota state.

Future storage:

```text
CSV accounting ledger
SQLite
```

must not require changes to provider code.

---

# 52. Future Configuration Storage

Hide config loading behind:

```go
type ConfigSource interface {
    Load(context.Context) (*Config, error)
}
```

V1:

```text
YAMLConfigSource
```

Future:

```text
SQLiteConfigSource
```

Do not build an ORM or repository layer in v1.

---

# 53. Future Routing Policies

Keep these concepts separate now:

```text
request requirements
classification
model catalog
selection policy
```

Future routing policies should be able to consume:

```text
Classification
RequestRequirements
ModelDescriptor[]
QuotaStatus
RuntimeStats
ClientIdentity
```

and return:

```text
ModelRef
```

Potential future policies:

```text
cheapest-capable
highest-quality
free-only
local-first
lowest-latency
privacy-first
custom weighted policy
```

Do not merge policy logic into classifier implementations.

---

# 54. Future Users and API Keys

The v1 `ClientAuthenticator` and `ClientIdentity` abstractions should support later addition of:

```text
managed API keys
users
groups
per-user quota
per-user spend
permissions
```

without changing inference routing.

---

# 55. Future MCP / Tool / Skill Gateways

Keep these future systems logically separate from model providers.

Do not create one generic "provider" interface for everything.

Future top-level systems may be:

```text
Model Gateway
MCP Gateway
Tool Gateway
Skill Repository
```

They may share:

```text
authentication
user identity
quota/accounting
configuration
policy
```

but should not share model-specific transport abstractions.

---

# 56. Container Design

Use a multi-stage build.

```text
Go builder
    ↓
linux/amd64 binary
    ↓
minimal runtime
```

Prefer a minimal runtime image containing:

- CA certificates
- timezone data only if actually needed
- no compiler
- no shell unless required

Run as non-root.

Mount:

```text
/config/config.yaml  read-only
/state               read-write
```

Expose one application port.

Do not require:

```text
privileged mode
Docker socket
host networking
sidecars
```

---

# 57. Testing Strategy

Testing must focus heavily on protocol behavior.

## Unit tests

Cover:

- config parsing
- unknown config-key rejection
- secret loading
- provider/model parsing
- model IDs containing multiple slashes
- model metadata merging
- capability filtering
- classification parsing
- cost estimation
- quota state
- session fingerprints
- context reset detection
- session expiry
- error normalization
- routing order
- free-only behavior
- unknown pricing behavior
- unknown quota behavior

## Fake-provider tests

Use:

```go
httptest.Server
```

Simulate:

- successful completion
- SSE streaming
- tool-call streaming
- vision request
- structured output
- reasoning fields
- 400
- 401
- 403
- 404
- 429
- 500
- timeout
- connection reset
- malformed SSE
- stream interruption after partial output

---

# 58. Agent-Loop Test

This is a release-blocking integration test.

```text
user
  ↓
smart
  ↓
classifier
  ↓
model A selected
  ↓
model A tool call
  ↓
tool result
  ↓
same session detected
  ↓
model A reused
  ↓
second tool call
  ↓
tool result
  ↓
model A final answer
```

Then simulate failure before the next model response.

Test:

```text
on_model_failure=fail
```

and:

```text
on_model_failure=reroute
```

When rerouting:

- model B receives full supplied conversation context
- model B receives handoff notice
- client receives handoff headers
- prior tool-call IDs remain intact

---

# 59. Streaming Handoff Test

Explicitly test:

```text
model A emits 3 SSE chunks
then disconnects
```

Expected:

```text
gateway DOES NOT switch to model B mid-stream
```

The stream terminates.

Model B may be selected on the next request if rerouting is enabled.

This behavior must not regress.

---

# 60. Client Compatibility Tests

Before v1 release, smoke-test against:

```text
Hermes Agent
Pi
OpenCode
official OpenAI SDK
curl
```

Required scenarios:

- `/v1/models`
- direct model
- smart model
- streaming chat
- tools
- multiple tool turns
- vision
- structured output
- reasoning
- embeddings
- Responses API
- model handoff
- explicit session header
- inferred session without header

---

# 61. OpenAI Codex Integration Tests

Treat this as its own transport test suite.

Verify:

- OAuth login
- access-token refresh
- refresh-token persistence
- ChatGPT-plan request succeeds
- Chat Completions → Codex Responses translation
- Responses → Codex Responses translation
- tool calls
- tool outputs
- streaming
- reasoning
- vision where model supports it
- model discovery/catalog
- credential isolation

Hermes's current implementation is a useful behavioral reference for the Codex Responses transport and OAuth model.

---

# 62. Nous Portal Integration Tests (Version 2)

Verify:

- OAuth login
- refresh-token persistence
- short-lived inference credential generation/refresh
- model discovery
- inference
- streaming
- tool calls
- authentication expiry
- revoked refresh-token behavior

Hermes currently stores the Portal refresh token independently and obtains short-lived inference credentials automatically, which should be treated as the reference behavior rather than storing a long-lived inference bearer token.

---

# 63. Performance Targets

These are targets, not correctness requirements.

Direct transparent proxying should add:

```text
single-digit milliseconds
```

of gateway-side LAN latency under normal load.

Idle memory target:

```text
tens of MB
```

rather than hundreds.

Idle CPU should be effectively negligible.

Benchmark:

```text
startup time
idle RSS
request throughput
transparent-proxy overhead
SSE time-to-first-byte overhead
classification latency
catalog memory
```

Do not optimize the classifier before measuring it.

---

# 64. Implementation Milestones

Implementation is tracked as single-task milestones in `TASKS.md`. The task
list, rather than this section, owns detailed dependencies and acceptance
criteria. The phases below define ordering and map the original architectural
milestones to executable tasks:

| Phase | Tasks | Exit condition |
| --- | --- | --- |
| Current transparent foundation | TASK-001–TASK-008 (done) | Authenticated Ollama model discovery and Chat Completions, including SSE, work end to end. |
| Complete API and catalog foundation | TASK-009–TASK-012, TASK-014 | The promised stateless OpenAI endpoint surface, errors/request IDs, catalog metadata, and standard OpenAI-compatible endpoints are complete. |
| Codex translation and authentication | TASK-017, TASK-019–TASK-022 | Secure credential state and CLI flows support verified ChatGPT Codex subscription access. |
| Classification and smart routing | TASK-024–TASK-029 | The `smart` model classifies requirements and deterministically selects a capable physical model. |
| Sessions, handoff, and quota | TASK-030–TASK-036 | Agent/tool sessions remain pinned; pre-stream rerouting, quota, aggregate usage, cost, and latency influence selection. |
| Product hardening and release | TASK-037–TASK-041 | Configuration, compatibility/security suites, container, and performance baselines pass, followed by a complete Version 1 audit. |

Protocol verification is part of each provider/classifier task so findings are
fresh when code is written. Each such task must update `docs/provider-notes.md`
before implementation. This replaces the earlier broad milestones that allowed
multiple unrelated features to appear complete as one unit.

TASK-013, TASK-015, TASK-016, TASK-018, and TASK-023 are explicitly excluded
from these Version 1 phases and are listed as Version 2 work in `TASKS.md`.

No phase changes the v1 constraints: one binary/process, flat-file persistence,
no database/Redis/queue/sidecar, transparent forwarding where possible, and no
mid-stream handoff. Optional metadata unavailable from an upstream must not
block explicit transparent routing, but unsupported request features must still
produce explicit errors.

---

# 65. Definition of Done

Version 1 is complete when:

- one Go binary provides the service
- one container runs it
- no database is required
- `/v1/models` aggregates configured providers
- all public physical models use `provider/model`
- the `smart` virtual model appears in `/v1/models`
- bearer authentication protects the inference API
- Chat Completions work
- Completions work
- Embeddings work
- stateless Responses work
- streaming works
- tools work
- multi-turn tool loops work
- vision works
- reasoning controls work where supported
- structured output works where supported
- standard OpenAI-compatible endpoints work, including the OpenAI API and
  self-hosted compatible servers
- OpenAI Codex subscription works through ChatGPT OAuth
- local Ollama works
- a configured self-hosted Laya model can classify `smart` requests
- the smart router classifies requests
- hard capabilities are enforced
- context window is considered
- model task quality is considered
- tool quality is considered
- reasoning strength is considered
- quota is considered
- cost is considered
- latency is considered
- local/cloud preference is considered
- paid-model routing can be globally disabled
- free-only mode cannot select unknown-cost cloud models
- sessions remain pinned
- tool loops remain pinned
- context reset removes affinity
- inactivity removes affinity
- explicit session deletion works
- model failure can fail or reroute
- handoff is never attempted after stream output begins
- handoff preserves conversation/tool context
- OAuth credentials survive container restart
- session affinity does not need to survive restart
- provider secrets never appear in logs
- OpenAI API credentials can never be sent to the Codex subscription endpoint or vice versa

---

# 66. Coding Rules

The implementation must follow these rules:

1. Keep one process and one binary.
2. Prefer Go standard-library functionality.
3. Do not add infrastructure to solve a problem that can be handled in memory or a small flat file.
4. Preserve unknown OpenAI fields on transparent transports.
5. Do not normalize/rebuild requests unless protocol translation requires it.
6. Never buffer an entire SSE response unnecessarily.
7. Never switch models after streamed output begins.
8. Keep classification separate from selection.
9. Keep selection separate from transport.
10. Keep credentials bound to their provider and endpoint.
11. Treat capability requirements as hard constraints.
12. Treat explicit physical model selection as exact.
13. Permit automatic fallback only through `smart`.
14. Keep tool-loop sessions pinned.
15. Unknown quota remains eligible in v1.
16. Unknown cloud pricing is excluded when `allow_paid_models=false`.
17. Do not silently discard unsupported request features.
18. Fail clearly rather than pretending a provider supports a feature.
19. Persist OAuth credentials atomically.
20. Never store session prompt text solely for affinity tracking.
21. Do not implement future features early.
22. Add abstractions only where a current or explicitly planned second implementation justifies them.
23. Prefer deterministic routing behavior over opaque heuristics.
24. Keep the direct proxy path fast even if classifiers or routing become more sophisticated later.

---

# 67. First Coding Task

Do **not** start by implementing intelligent routing.

The first coding task is to prove the transparent proxy architecture.

Implement, in order:

```text
Go module
    ↓
YAML configuration
    ↓
strict config validation
    ↓
secret resolution
    ↓
gateway bearer auth
    ↓
provider/model ID parsing
    ↓
generic OpenAI-compatible provider
    ↓
Ollama provider
    ↓
model discovery
    ↓
/v1/models
/v1/models/{model}
    ↓
/v1/chat/completions
/v1/completions
/v1/embeddings
/v1/responses
    ↓
SSE relay
```

Then test:

```text
official OpenAI SDK
        ↓
gateway
        ↓
local Ollama
```

with:

```text
plain text
streaming
tool calls
multi-turn tool results
vision
structured output
reasoning
embeddings
Responses
```

Only after this transport path is reliable should implementation continue to cloud providers, translated transports, classification, and smart routing.

The architectural invariant is:

> The gateway must remain a fast and transparent OpenAI-compatible proxy even if every intelligent-routing feature is disabled.

# 68. Quota-Constrained Multi-Agent Development Strategy

Development may need to be distributed across multiple coding agents and multiple sessions because individual providers or subscriptions may have limited usage quotas.

The project must therefore be organized so that:

- work can stop cleanly at any point
- another agent can resume without reconstructing prior reasoning
- multiple agents can work on independent components
- duplicate work is minimized
- partially completed code remains buildable whenever practical
- no agent needs the entire project history in its context window

The repository itself is the authoritative source of project state.

Do not rely on conversational memory from any individual coding agent.

---

## 68.1 Development Model

Treat each implementation milestone as a collection of small, independently handoffable work units.

Preferred unit size:

```text
one focused feature
or
one provider adapter
or
one testable subsystem
```

Avoid handing an agent tasks such as:

```text
"Implement Milestones 1 through 4."
```

Prefer:

```text
"Implement strict YAML configuration validation."

"Implement provider/model ID parsing."

"Implement the generic OpenAI-compatible passthrough transport."

"Implement the Ollama provider using the existing passthrough transport."

"Implement SSE relay and tests."

"Implement the OpenRouter provider."

"Implement OpenAI Codex OAuth login only."
```

Each task should ideally fit within one coding-agent session.

---

## 68.2 Repository State Is the Handoff Mechanism

Maintain these files from the beginning:

```text
docs/
  architecture.md
  provider-notes.md
  development.md
  decisions.md

TASKS.md
CHANGELOG.md
```

Their purposes are:

### `docs/architecture.md`

Contains the current implemented architecture.

It should describe what the code **actually does**, not merely repeat this design document.

Update it when architectural behavior changes.

### `docs/provider-notes.md`

Contains protocol research for each provider:

```text
authentication
base URLs
model discovery
supported endpoints
streaming behavior
quota APIs
pricing APIs
known incompatibilities
implementation references
```

This is particularly important for:

```text
OpenAI Codex
Nous Portal
Anthropic
Jev
```

because later agents should not have to repeat protocol research.

### `docs/development.md`

Contains:

```text
build commands
test commands
container commands
repository conventions
how to add a provider
how to add a classifier
how to add a transport
```

### `docs/decisions.md`

Maintain short architecture decision records.

Example:

```text
ADR-001
Decision: Public model IDs split on first slash only.

Reason:
OpenRouter and other upstream model IDs contain slashes.
```

Do not require elaborate formal ADR files unless the project becomes large.

### `TASKS.md`

This is the primary agent handoff document.

It must clearly show:

```text
TODO
IN PROGRESS
BLOCKED
DONE
```

for implementation units.

---

## 68.3 Task Record Format

Every coding task should have enough information for another agent to continue it without access to previous conversations.

Recommended format:

```markdown
## TASK-014 — Ollama model discovery

Status: DONE
Milestone: 2
Owner: Codex
Commit: abc1234

### Goal

Discover models from an Ollama provider and convert them into
ModelDescriptor records.

### Relevant files

- internal/provider/ollama.go
- internal/catalog/descriptor.go

### Requirements

- Use Ollama native model metadata where useful.
- Public IDs use `<provider>/<model>`.
- Do not infer unsupported capabilities from names.

### Tests

- discovers multiple models
- handles unreachable Ollama server
- preserves model names containing `:`
- converts metadata correctly

### Result

Implemented discovery through `/api/tags`.
Additional model details are retrieved through `/api/show`.

### Remaining issues

Vision/tool capability extraction is incomplete because Ollama does not
expose all capabilities consistently. Bundled metadata fills those fields.
```

The `Result` and `Remaining issues` sections are particularly important for cross-agent work.

---

## 68.4 Require Clean Handoffs

Before an agent stops because of quota, context limit, or task completion, it should:

1. ensure modified code is saved
2. run the narrowest relevant tests
3. run `go test ./...` when practical
4. update `TASKS.md`
5. update documentation when the task changes architecture, public behavior,
   configuration, developer workflow, significant design decisions, or another
   durable project convention; do not add narrow task-specific test commands to
   `docs/development.md` unless they are useful as a reusable procedure
6. record unresolved problems
7. make a focused Git commit

A partially completed task should be committed only if:

```text
the repository still builds
or
the commit is clearly marked as incomplete
```

Prefer not to leave uncommitted work as the only record of progress.

---

## 68.5 Git Workflow

Use short-lived feature branches if multiple agents may work concurrently.

Recommended:

```text
main
  │
  ├── feature/config
  ├── feature/openai-transport
  ├── feature/ollama
  ├── feature/openrouter
  ├── feature/codex-auth
  └── feature/smart-router
```

Task branches should be narrowly scoped.

Example:

```text
agent/TASK-014-ollama-discovery
```

Do not assign two agents overlapping files unless explicitly coordinating them.

Prefer integration through:

```text
small commits
+
small merges
```

rather than large long-running branches.

---

## 68.6 Commit Requirements

Commits should represent one understandable change.

Good:

```text
provider: add Ollama model discovery
transport: add SSE passthrough relay
router: implement hard capability filtering
auth: add gateway bearer-key middleware
```

Avoid:

```text
implement a bunch of gateway features
```

When practical, include the task ID:

```text
TASK-014 provider: add Ollama model discovery
```

This allows another agent to correlate Git history with `TASKS.md`.

---

## 68.7 Agent Assignment Strategy

Different agents can be used opportunistically according to remaining quota and task type.

Do not make any provider-specific agent mandatory.

Potential allocation:

### Codex

Good candidate for:

```text
Go implementation
refactoring
tests
protocol adapters
repository-wide code changes
```

### Gemini

Good candidate for:

```text
independent code review
protocol/documentation research
test-case generation
architecture review
provider compatibility review
```

### Hermes / Pi / OpenCode

Good candidates for:

```text
interactive development
small focused patches
local testing
integration testing
repository exploration
provider experiments
```

These are recommendations only.

Tasks should remain agent-neutral.

Any capable coding agent must be able to perform any task using repository documentation alone.

---

## 68.8 Separate Research Tasks from Coding Tasks

Protocol research can consume substantial model quota.

Do not repeatedly research the same provider.

Create explicit research tasks such as:

```text
RESEARCH-001 OpenAI Codex OAuth
RESEARCH-002 Nous Portal authentication
RESEARCH-003 Jev API
RESEARCH-004 OpenRouter quota interfaces
```

Research output goes into:

```text
docs/provider-notes.md
```

A later coding agent should implement from those notes rather than starting research again unless the notes are incomplete or outdated.

Record:

```text
source
date checked
endpoint
observed behavior
uncertainty
```

for information likely to change.

---

## 68.9 Use Interfaces to Enable Parallel Work

Parallel work should follow package boundaries already defined in the architecture.

For example:

```text
Agent A:
generic OpenAI passthrough transport

Agent B:
catalog metadata database

Agent C:
OpenAI Codex protocol research

Agent D:
Jev classifier research
```

can proceed independently.

Once the provider interfaces stabilize:

```text
Agent A → OpenRouter
Agent B → Gemini
Agent C → xAI
Agent D → Anthropic
```

can also proceed mostly independently.

Do not begin broad parallel provider implementation until the following shared contracts are stable:

```text
Provider
RequestEnvelope
ModelDescriptor
typed errors
transport interface
secret handling
```

Otherwise multiple agents are likely to invent incompatible abstractions.

---

## 68.10 Establish Shared Contracts Early

Before distributing implementation widely, complete and freeze the first practical version of these definitions:

```text
ModelDescriptor
RequestEnvelope
Provider interfaces
Transport interfaces
Classification
RequestRequirements
QuotaStatus
typed provider errors
SecretRef
CredentialStore
```

"Freeze" does not mean they can never change.

It means:

> Do not casually redesign them inside provider-specific tasks.

If an agent discovers that a shared contract is inadequate:

1. stop provider-specific implementation
2. document the problem
3. create a separate architecture task
4. update the shared interface
5. update affected tests
6. resume provider work

This prevents architecture drift between agents.

---

## 68.11 Integration Owner

At any given development phase, designate one task or agent as the **integration owner**.

This is a role, not a permanent person or model.

The integration owner is responsible for:

```text
merging completed work
running repository-wide tests
resolving interface conflicts
updating TASKS.md
checking architectural consistency
```

The integration owner should not simultaneously redesign multiple subsystems.

If agent quota changes, another agent can assume the role using repository state.

---

## 68.12 Checkpoint Milestones

Each major milestone should end in a stable checkpoint.

A checkpoint requires:

```text
go build ./...
go test ./...
container build succeeds
relevant integration test succeeds
TASKS.md updated
architecture docs updated
```

Tag useful checkpoints:

```text
checkpoint/m1-proxy
checkpoint/m2-catalog
checkpoint/m3-cloud-providers
checkpoint/m4-subscription-providers
checkpoint/m5-classifiers
checkpoint/m6-smart-router
```

Release tags can use normal semantic versioning later.

Checkpoint tags make it easy to recover if later multi-agent work causes regressions.

---

## 68.13 Recommended Quota-Conscious Build Order

When coding quota is limited, prioritize work that unlocks the greatest amount of subsequent parallel development.

### Phase A — Shared foundation

Prefer one agent or tightly coordinated sequential work.

Implement:

```text
project skeleton
configuration
secret handling
typed errors
request envelope
provider interfaces
transport interfaces
model descriptor
gateway authentication
```

Do not parallelize this heavily.

These are shared contracts.

---

### Phase B — Minimal vertical slice

Complete one end-to-end implementation:

```text
OpenAI-compatible client
        ↓
gateway
        ↓
Ollama
```

including:

```text
models
chat
streaming
tools
structured output
vision
reasoning
embeddings
Responses
```

This establishes a known-working reference implementation.

Do not spend quota implementing every provider before this works.

---

### Phase C — Version 1 providers

After the reference path is stable, complete standard OpenAI-compatible endpoint
support and the separate ChatGPT Codex subscription transport. Local Ollama is
the reference implementation. Do not implement the Version 2 adapters as part
of this phase.

For example:

```text
Agent/session 1:
standard OpenAI-compatible endpoints and normal OpenAI API profile

Agent/session 2:
OpenAI Codex
```

Provider work should include tests in the same task, plus documentation when
the work changes public behavior, configuration, architecture, significant
design decisions, or reusable development procedures.

---

### Phase D — Classifiers

Classifiers can likewise be distributed:

```text
self-hosted Laya classifier
embedding classifier
connected-model classifier
```

All must conform to the same fixed `Classification` contract.
Jev classifiers and provider-specific OpenRouter, Gemini, xAI, Anthropic, and
Nous work belong to Version 2.

---

### Phase E — Smart routing

Keep the core model-selection algorithm primarily under one implementation owner.

It contains shared policy and should not be independently implemented by several agents.

Parallel assistance is appropriate for:

```text
test generation
review
benchmarking
quality-data preparation
edge-case analysis
```

---

### Phase F — Sessions and handoff

Implement session affinity first.

Then implement handoff behavior as a separate task.

Do not combine:

```text
session inference
routing
provider fallback
stream recovery
```

into one large coding assignment.

---

## 68.14 Use Independent Agents for Review

When quota permits, use a different model/agent to review important changes than the one that wrote them.

Particularly valuable review targets:

```text
OAuth implementation
credential isolation
SSE streaming
tool-call preservation
protocol translation
session fingerprinting
routing algorithm
quota calculations
```

The reviewing agent should be given:

```text
requirements
relevant code diff
tests
```

rather than the entire conversational history.

Ask it to identify:

```text
correctness bugs
security issues
protocol incompatibilities
resource inefficiencies
unnecessary complexity
missing tests
```

Do not ask only:

```text
"Does this look good?"
```

---

## 68.15 Avoid Repeated Full-Repository Context

To conserve model quota, tasks should provide agents only the context they require.

Use repository files as references.

For a provider task, relevant context might be:

```text
implementation-plan.md
docs/development.md
docs/provider-notes.md
internal/provider/provider.go
internal/transport/*
one existing provider implementation
relevant tests
```

Do not repeatedly paste the entire project specification into every agent session.

The root repository should contain a concise:

```text
AGENTS.md
```

with universal coding instructions.

Suggested contents:

```text
project goals
resource-efficiency requirements
build/test commands
architectural invariants
files to read before modifying shared interfaces
task tracking requirements
handoff requirements
```

Keep `AGENTS.md` concise enough that coding agents can load it routinely.

---

## 68.16 Suggested `AGENTS.md` Rules

At minimum:

```text
1. Read TASKS.md before beginning work.
2. Read docs/architecture.md before modifying shared abstractions.
3. Do not introduce new infrastructure or dependencies without justification.
4. Preserve unknown OpenAI fields on passthrough transports.
5. Never send credentials to an endpoint other than their configured provider.
6. Never silently discard unsupported API features.
7. Do not change shared interfaces inside a provider task without documenting why.
8. Add or update tests for every behavioral change.
9. Run the relevant tests before finishing.
10. Update TASKS.md with result and remaining issues.
11. Update documentation when architecture, public behavior, configuration,
    developer workflow, significant design decisions, or another durable
    project convention changes. Do not add narrow task-specific test commands
    to `docs/development.md` unless they are useful as a reusable procedure.
12. Leave the repository in a resumable state.
```

---

## 68.17 Dependency Discipline Across Agents

Coding agents must not independently add libraries merely for convenience.

Before introducing a dependency, record:

```text
dependency
purpose
why standard library is insufficient
runtime impact
binary-size impact where material
```

Prefer one library solving a shared problem over different agents introducing competing libraries.

Particularly scrutinize dependencies for:

```text
HTTP routing
OAuth
JSON manipulation
SSE
tokenization
embedding math
logging
CLI handling
```

Given the project's resource-efficiency goal, standard-library implementations are preferred when straightforward and maintainable.

---

## 68.18 Testing Ownership

Every implementation task owns its tests.

Do not create a later "write all the tests" milestone.

Example:

```text
TASK-020 — OpenRouter provider

must include:
- model discovery tests
- authentication tests
- streaming tests
- error normalization tests
- slash-containing model ID tests
```

Repository-wide integration tests may be added separately, but component behavior must be tested with the component.

---

## 68.19 Work That Must Not Be Performed Concurrently

Avoid concurrent implementation of:

```text
shared provider interfaces
canonical request format
routing algorithm
session identity algorithm
config schema
public model naming rules
typed error taxonomy
```

These define contracts used by the rest of the application.

Have one agent make the change, then allow other tasks to rebase onto that stable contract.

---

## 68.20 Work That Can Be Parallelized Safely

Once shared contracts exist, good parallel tasks include:

```text
different provider adapters
different classifier implementations
provider research
capability database entries
provider contract tests
documentation
container packaging
performance benchmarking
client compatibility testing
```

---

## 68.21 Blocked Tasks

If an agent cannot complete a task because:

```text
provider documentation is unclear
OAuth flow cannot be verified
required API access is unavailable
another shared component is incomplete
```

do not guess.

Mark it:

```text
BLOCKED
```

in `TASKS.md` and document:

```text
what is known
what was attempted
what evidence is missing
what would unblock implementation
```

Then move to another independent task.

Limited agent quota should not be wasted repeatedly attempting the same unresolved problem.

---

## 68.22 Context-Safe Continuation Prompt

When handing unfinished work to another coding agent, use a short standardized prompt.

Example:

```text
Continue TASK-027 from this repository.

Read:
- AGENTS.md
- TASKS.md
- docs/architecture.md
- the TASK-027 entry
- relevant files listed in that entry

Do not redesign shared interfaces unless the task documents a blocking
problem with them.

Run the existing relevant tests before making changes.

Complete the task, add/update tests, update TASKS.md and relevant docs,
and leave a focused Git commit.
```

The repository should contain all additional context required.

---

# 68.23 Quota Exhaustion During a Task

If an agent is approaching its usage/context limit, prioritize in this order:

```text
1. preserve correct code already written
2. document current implementation state
3. document known failing tests/errors
4. update TASKS.md
5. commit resumable work
6. perform additional cleanup only if quota remains
```

Do not use remaining quota to polish prose while leaving undocumented partial code.

---

# 68.24 Cross-Agent Verification

For security-sensitive and protocol-sensitive code, prefer:

```text
Agent A implements
        ↓
Agent B reviews
        ↓
integration owner merges
```

High-priority examples:

```text
OpenAI Codex OAuth
Nous OAuth
credential persistence
provider credential isolation
Anthropic translation
SSE parsing/relay
tool-call translation
```

This is more valuable than having multiple agents independently implement the same feature.

---

# 68.25 Definition of a Handoffable Task

A task is ready to hand to another agent when another agent can determine all of the following from repository contents:

```text
what was requested
what has been implemented
what files changed
what tests exist
which tests pass
which tests fail
why work stopped
what remains to do
whether any architecture decision was made
```

If any of those exist only in the previous agent's conversation, the handoff is incomplete.

---

# 68.26 Core Principle

The project must be developed as though **no individual coding agent will be available for the next session**.

Therefore:

> Git, tests, task records, and repository documentation are the project's memory.

Individual agents are interchangeable implementation resources.

This allows development to move between Codex, Gemini, Hermes, Pi, OpenCode, or other coding agents according to available quota without coupling the project to any one model or provider.

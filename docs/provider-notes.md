# Provider Notes

This document records verified provider behavior needed to implement provider
adapters.

It is research documentation, not gateway configuration.

For each provider, record:

- date verified
- authentication
- API base URL
- model discovery
- supported inference endpoints
- streaming behavior
- tool calling
- vision
- structured output
- reasoning
- quota information
- pricing information
- known incompatibilities
- source/reference material

Do not record credentials here.

The repository's provider research covers local Ollama, standard
OpenAI-compatible endpoints, ChatGPT Codex subscription access, Nous Portal,
Jev, Gemini, xAI, and Anthropic. Before changing an adapter, record dated
findings from primary documentation, identify which fields are verified versus
inferred, and record unsupported behavior explicitly. A provider task is not
complete until its notes and contract tests agree.

## Bundled core-model catalog

The embedded `internal/catalog/capabilities.json` contains the active core
text models verified on 2026-09-30 for the configured provider IDs
`openai-codex`, `anthropic`, `gemini`, and `xai`. Specialized media, audio,
embedding, moderation, and retired models are excluded. The records describe
what Torii's current adapters expose, not every capability available through a
provider's native API.

Quality scores are intentionally absent. Provider documentation does not
provide a comparable normalized benchmark scale, so the selector treats these
models as unknown-quality candidates until benchmark-backed data is added.

Catalog sources:

- https://developers.openai.com/api/docs/models/gpt-5.3-codex
- https://platform.claude.com/docs/en/models/overview
- https://platform.claude.com/docs/en/about-claude/pricing
- https://ai.google.dev/gemini-api/docs/models
- https://ai.google.dev/gemini-api/docs/pricing
- https://docs.x.ai/developers/models
- https://docs.x.ai/developers/pricing

The Version 1 router may use a self-hosted Laya model through an already
configured local physical provider. Laya protocol and schema findings belong in
this document; no dedicated hosted-Laya provider
or credential flow is in scope. Jev classifier transports are Version 2 work.

## Self-hosted Laya classifier

### Status

Implementation status: complete through the standard OpenAI-compatible
transport. The classifier is a configured physical `provider/model` selected by
`router.classifier.type: laya`; it does not introduce a hosted Laya endpoint,
credential flow, or provider-specific wire protocol. Its response is required
to match the shared strict classification schema, and `smart` recursion is
rejected.

Last verified: 2026-09-30

This deliberately documents the supported gateway contract rather than
claiming a particular self-hosted Laya distribution's private prompt or model
behavior. The configured local server remains responsible for serving the
standard Chat Completions contract.

---

## OpenRouter

### Status

Research status: verified from OpenRouter documentation
Implementation status: complete through the dedicated OpenAI-compatible
provider profile. OpenRouter uses the transparent transport, model discovery,
model rewrites, and provider-scoped bearer credentials.
Last verified: 2026-09-30

### Authentication

API key:

    Authorization: Bearer <token>

### API

Base URL:

    https://openrouter.ai/api/v1

The documented inference surface is OpenAI-compatible. Chat Completions is
available at `/chat/completions`; the model catalog is available at
`GET /models`.

### Model Discovery

Endpoint:

    GET /models

Record:

- model IDs
- context windows
- capabilities
- pricing fields
- provider metadata

The model response includes architecture/modalities, context length, pricing,
per-request limits, and a details link for endpoint-level information. Model
IDs may contain `/`, so the gateway must preserve the complete upstream ID
after the `openrouter/` public namespace.

### Quotas

Document whether remaining credits/quota can be queried and through which
endpoint or headers.

`GET /credits` returns total credits purchased and total usage, and requires a
management key according to the current documentation. Torii accepts this as
the separate `management_token` secret on the `openrouter` provider. Ordinary
inference keys are never reused for management operations. If the management
key is absent or the endpoint is unavailable, quota remains unknown and normal
inference continues.

### OpenAI Compatibility

Document support for:

- Chat Completions
- Responses
- Completions
- Embeddings
- streaming
- tools
- vision
- structured output
- reasoning

### Known Quirks

- OpenRouter performs its own provider selection and fallback; Torii must not
  duplicate that routing.
- Optional `HTTP-Referer` and `X-OpenRouter-Title` headers are attribution
  headers, not credentials.
- The OpenAI compatibility layer is the documented path, but provider-specific
  features and parameters may not be uniform across models.

### Sources

- https://openrouter.ai/docs/quickstart
- https://openrouter.ai/docs/api/api-reference/models/get-models
- https://openrouter.ai/docs/api/api-reference/credits/get-credits
- https://openrouter.ai/docs/guides/overview/auth/management-api-keys

---

## OpenAI API

### Status

Research status: verified from official API reference
Implementation status: complete for the transparent Version 1 endpoint set
Last verified: 2026-09-30

### Authentication

OpenAI API keys are sent as `Authorization: Bearer <key>`. The configured
profile is bound to `https://api.openai.com` and never reads the credential
store used by the Codex subscription provider.

This provider must remain separate from OpenAI Codex subscription auth.

### Notes

- Model listing is `GET https://api.openai.com/v1/models`.
- The gateway transparently forwards Chat Completions, Completions,
  Embeddings, and stateless Responses requests and preserves SSE bytes.
- Optional `OpenAI-Organization` and `OpenAI-Project` headers are configured
  as non-secret values on the profile; arbitrary headers and model rewrites are
  rejected.
- Redirects are returned rather than followed by the isolated HTTP client.

### Sources

- https://platform.openai.com/docs/api-reference/authentication
- https://platform.openai.com/docs/api-reference/models
- https://platform.openai.com/docs/api-reference/chat
- https://platform.openai.com/docs/api-reference/responses
- https://platform.openai.com/docs/api-reference/embeddings

---

## OpenAI Codex Subscription

### Status

Implemented as an endpoint-bound provider using the configured ChatGPT Codex
backend and the encrypted SQLite credential store. The current transport
discovers the authenticated subscription model catalog from the Codex backend,
supports Chat/Responses request translation, response translation for
non-streaming Chat Completions, SSE event adaptation, and refreshes credentials
near expiry. Device-code login is the default CLI flow; browser
authorization-code + PKCE is available with `--browser`.

Last verified: 2026-09-30

Official implementation findings used here: the OpenAI Codex repository uses
`https://auth.openai.com/oauth/token` for refresh, the client identifier
`app_EMoamEEZ73f0CkXaXp7hrann`, stores access/refresh/id-token data with an
account identifier, and targets `https://chatgpt.com/backend-api/codex/responses`.
These are observed implementation details, not a public API guarantee.

Sources:

- https://github.com/openai/codex/blob/main/codex-rs/login/src/auth/manager.rs
- https://github.com/openai/codex/blob/main/codex-rs/login/src/device_code_auth.rs
- https://github.com/openai/codex/blob/main/codex-rs/cli/src/doctor.rs

The authenticated model catalog is requested from the Codex backend's
`/models` endpoint. Torii exposes entries whose visibility is `list` and whose
`supported_in_api` flag is true. If the account is not logged in or discovery
is temporarily unavailable, the configured fallback model list remains
available; a successful login refreshes the live catalog without restarting
the gateway.

## Nous Portal

### Status

Research status: partially verified from Nous-maintained Hermes documentation
and adapter source; public API definition unavailable during this review
Implementation status: complete through the Portal device-code login,
endpoint-bound refresh flow, subscription-aware model metadata, and standard
OpenAI-compatible inference transport. Quota is learned from gateway response
rate-limit headers; no stable public balance endpoint was found.
Last verified: 2026-09-30

### Authentication

Verify:

- OAuth flow
- refresh-token behavior
- short-lived inference tokens
- API-key availability, if any

The maintained Hermes integration documents a browser OAuth login against
`portal.nousresearch.com`, stores a refresh token separately from configuration,
and obtains short-lived runtime credentials for the inference API. The current
implementation uses the Portal OAuth token endpoint
`https://portal.nousresearch.com/api/oauth/token`; the runtime inference base
is `https://inference-api.nousresearch.com/v1`. The maintained adapter uses
bearer credentials and refreshes them lazily, retrying once after a 401.

The current source also documents rotating refresh tokens and a device-code to
runtime-credential path for inference. Token values must remain outside YAML
and must be persisted atomically in the gateway credential store.

### Model Discovery

The maintained proxy exposes `GET /models`, but the public Portal API
definition was not retrievable on 2026-09-30. Do not infer catalog shape,
pricing, or quota semantics from the subscription marketing page. Before
implementation, capture a real authenticated response and document model IDs,
capabilities, context limits, pricing, and subscription/quota fields.

The inference adapter currently allows `/chat/completions`, `/completions`,
`/embeddings`, and `/models`; a maintained issue reports that the upstream
also serves `/responses`, so Responses support should be verified and added or
explicitly rejected in the Torii adapter.

The live `/v1/models` response is the pricing source when it includes
`pricing.prompt` and `pricing.completion` values. Torii converts those values
from per-token dollars to its per-million-token catalog units. Rows marked
`billing_mode: subscription` are represented as zero marginal cost for routing
purposes. The maintained Hermes implementation also treats the Portal account
as subscription-billed and uses live model pricing for paid/free-tier display.

No stable Portal quota-balance endpoint was found. Portal tier RPM/TPM limits
apply across the bearer and are surfaced through inference response headers or
provider errors; Torii therefore records standard rate-limit headers through
its existing quota cache and treats absent headers as unknown rather than
inventing a balance.

### Sources

- https://github.com/NousResearch/hermes-agent/blob/main/website/docs/integrations/nous-portal.md
- https://github.com/NousResearch/hermes-agent/blob/main/hermes_cli/proxy/adapters/nous_portal.py
- https://github.com/NousResearch/hermes-agent/blob/main/plugins/dashboard_auth/nous/__init__.py
- https://portal.nousresearch.com/api-docs (definition failed to load during review)
- https://github.com/NousResearch/hermes-agent/issues/104505
- https://github.com/NousResearch/hermes-agent/blob/main/website/docs/reference/model-catalog.md
- https://github.com/NousResearch/hermes-agent/blob/main/hermes_cli/models.py

---

## Google Gemini

### Status

Research status: verified from Google AI for Developers documentation
Implementation status: complete through the dedicated Gemini profile using
the documented `/v1beta/openai` compatibility root and provider-scoped bearer
credentials.
Last verified: 2026-09-30

### Authentication and API

Use a Gemini API key. The documented OpenAI-compatible base URL is:

    https://generativelanguage.googleapis.com/v1beta/openai/

The compatibility surface supports `GET /models`, model retrieval, and Chat
Completions. The public namespace is `gemini/<model>` and the existing
transparent transport can be reused for compatible requests.

### Capabilities and limitations

Gemini supports multimodal content, tools/function calling, streaming, and
thinking controls. OpenAI `reasoning_effort` is mapped to Gemini thinking
settings for supported models; Gemini-specific thinking settings can be sent
through the documented extension fields. The compatibility layer is beta and
does not expose all Gemini features, notably some server-side tools such as
Google Search grounding and File API workflows. Unsupported combinations must
be rejected or routed through a future native Gemini transport rather than
silently discarded.

Pricing and rate limits are model/project dependent. Catalog ingestion should
retain model lifecycle state (stable, preview, experimental, deprecated) and
pricing where available, but must not treat aliases such as `latest` as
immutable model identities.

### Sources

- https://ai.google.dev/gemini-api/docs/openai
- https://ai.google.dev/gemini-api/docs/models
- https://ai.google.dev/gemini-api/docs/partner-integration
- https://ai.google.dev/gemini-api/docs/pricing
- https://ai.google.dev/gemini-api/docs/rate-limits

## xAI

### Status

Research status: verified from xAI REST API documentation
Implementation status: complete through the dedicated xAI profile using the
documented OpenAI-compatible `/v1` surface and provider-scoped bearer
credentials.
Last verified: 2026-09-30

### Authentication and API

xAI inference uses `Authorization: Bearer <xAI API key>` against the inference
base `https://api.x.ai`. The REST API is OpenAI-compatible and documents
Responses, Chat Completions, model listing, streaming, images, files, batches,
and other resources. The public namespace is `xai/<model>`.

Use the transparent transport for the documented Chat Completions and
Responses paths. Model discovery should use the inference model-list endpoint
and preserve the provider's model identifiers. Do not implement Management API
operations as part of the inference adapter.

### Capabilities and limitations

The Chat Completions reference documents tool/function-call fields, streaming,
usage including reasoning tokens, and multimodal request support where the
selected model permits it. Capability metadata must therefore remain
model-specific. xAI's compatibility statement does not make every OpenAI
parameter universally supported; provider errors and documented unsupported
features must be surfaced as capability errors. This task covers normal xAI
API keys only, not SuperGrok or consumer-subscription OAuth.

### Sources

- https://docs.x.ai/developers/rest-api-reference/inference
- https://docs.x.ai/developers/rest-api-reference/inference/chat-completions
- https://docs.x.ai/developers/rest-api-reference/inference/models

## Anthropic

### Status

Research status: verified from Anthropic Claude Platform documentation
Implementation status: complete through a native Messages API adapter for
Chat Completions and basic stateless Responses requests, including system
messages, streaming SSE adaptation, vision, client tools/tool results, usage,
finish-reason translation, and reasoning-effort mapping. Structured output
remains explicitly rejected because Anthropic's native contract cannot be
translated without changing its guarantees.
Last verified: 2026-09-30

### Authentication and API

Use an Anthropic API key in `x-api-key` and the required `anthropic-version`
header. The native endpoint is:

    POST https://api.anthropic.com/v1/messages

The public namespace is `anthropic/<model>`. This provider requires an
explicit canonical-to-Messages translation; it must not be treated as a
transparent OpenAI-compatible endpoint.

### Request translation

Translate system instructions to the top-level `system` field and preserve
user/assistant turns as Messages content blocks. Map text, base64/HTTPS
images, tool definitions (`input_schema`), tool calls (`tool_use`), and tool
results (`tool_result`). Preserve max token controls and model-specific
thinking/reasoning controls. Client tools return `stop_reason: tool_use` and
the next request carries the assistant content plus a user `tool_result`.

### Responses, streaming, and limits

Anthropic streaming is an SSE event protocol with message/content-block
events, not OpenAI chat-delta bytes. The adapter translates text deltas,
tool-input deltas, stop reasons, and usage into the gateway's canonical Chat
stream; Responses streams are adapted to the corresponding output-text and
completion events. Non-streaming usage retains input/output token counts;
tool schemas and tool results contribute to input usage. Vision accepts
base64 or HTTPS image sources on the direct API.

Anthropic's current thinking model families use model-dependent adaptive or
extended thinking controls. Unsupported combinations of thinking, tools,
structured output, or content types must return a capability error instead of
being dropped. Server-side Anthropic tools are a separate capability and must
not be confused with client-defined gateway tools.

### Sources

- https://platform.claude.com/docs/en/api/messages
- https://platform.claude.com/docs/en/agents-and-tools/tool-use/overview
- https://platform.claude.com/docs/en/build-with-claude/vision
- https://platform.claude.com/docs/en/build-with-claude/extended-thinking
- https://platform.claude.com/docs/en/about-claude/models/overview

## Jev classifier adapters

### Status

Research status: verified from Jev API documentation
Implementation status: direct and provider-transport classifiers implemented
Last verified: 2026-09-30

Jev is a typed-decision API, not a chat-generation provider. The direct
classifier sends one state plus typed `choice` and `noul` questions to
`POST https://jevmodel.org/v1/systemone/` with a bearer API key. Torii maps
the returned task-type and complexity choices plus boolean noul probabilities
into the existing classifier contract. The request asks seven questions,
within Jev's documented eight-question limit.

The gateway configuration is:

    router:
      classifier:
        type: jev
        model: jev-latest
        base_url: https://jevmodel.org
        api_key:
          env: JEVMODEL_API_KEY

The implementation also supports provider transport for gateways that expose
the Jev System One contract at their own origin. For example, OpenRouter
documents `POST /api/v1/systemone`; configure a provider-backed classifier with
`transport: provider`, `provider: openrouter`, and model
`typesafe/jev-1.13`. The provider owns the origin and bearer credential, while
the classifier owns only the typed-decision request/response mapping.

Jev's documented limits include an 8,000-character serialized state, up to
eight questions, and 120 requests per minute per key. The API bills input
tokens only.

### Sources

- https://jevmodel.org/docs/
- https://openrouter.ai/blog/insights/what-is-jev/

## Ollama

### Status

Implementation status: model discovery and Chat Completions, including SSE
streaming
Last implementation update: 2026-09-24

### API

The configured base URL is used with Ollama's OpenAI-compatible endpoints:

    GET  /v1/models
    POST /v1/chat/completions

Discovered upstream model IDs are exposed publicly as
`<configured-provider-id>/<upstream-model-id>`. Chat Completion requests have
that prefix removed before forwarding. Unknown JSON request fields are
preserved by the generic compatible transport. Streaming responses are
relayed byte-for-byte and flushed as upstream bytes arrive.

### Current Limitations

- Native Ollama metadata endpoints are not queried yet.
- Ollama authentication is not currently configured or sent.
- Model-discovery calls have a 10-second deadline; inference uses the caller's
  context without that discovery-specific limit.

## Custom OpenAI-Compatible Providers

### Status

Implementation status: complete for the transparent Version 1 endpoint set
Last verified: 2026-09-30

### Protocol profile

The generic provider treats the configured base URL as an origin-bound OpenAI
HTTP API and uses `GET /v1/models` for optional discovery. It transparently
forwards Chat Completions, Completions, Embeddings, and stateless Responses,
rewriting only the top-level model. The model list is expected to be an object
with a `data` array whose entries contain non-empty string `id` fields.

The profile can instead use a static model list for servers that do not expose
model discovery. Public aliases can map to different upstream model names.
Streaming remains byte-for-byte SSE passthrough; compatibility therefore does
not depend on the gateway knowing every event or request field.

### Authentication and transport

Each provider may have no authentication, one bearer secret reference, and
secret-reference-backed custom headers. Hop-by-hop headers, request framing
headers, `Host`, `Authorization`, `X-Request-ID`, and browser `Sec-*` headers
cannot be configured as custom headers. Redirects are not followed, ensuring
credentials cannot leave the exact configured provider origin. Each provider
gets an independent HTTP client, connection pool, TLS roots/server name, dial
 timeout, request timeout, and discovery timeout.

### Primary sources

- OpenAI API authentication: https://platform.openai.com/docs/api-reference/authentication
- OpenAI Models list operation: https://platform.openai.com/docs/api-reference/models/list
- OpenAI Chat Completions: https://platform.openai.com/docs/api-reference/chat
- OpenAI Responses: https://platform.openai.com/docs/api-reference/responses
- OpenAI Embeddings: https://platform.openai.com/docs/api-reference/embeddings
- Go HTTP client redirect policy: https://pkg.go.dev/net/http#Client
- Go TLS client configuration: https://pkg.go.dev/crypto/tls#Config

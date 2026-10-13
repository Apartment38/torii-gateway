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
inferred, and record unsupported behavior explicitly. Provider protocol changes must keep these notes and contract tests consistent.

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

The smart router can use a configured Jev-compatible physical provider for
self-hosted Laya typed decisions. These models use System One for public typed-decision passthrough and judge
selection; they are never advertised as chat-capable models.

## Self-hosted Laya classifier

### Status

Research status: verified from the Laya System One server and agent source
Last verified: 2026-10-07

The current `laya-system-one` server exposes `POST /v1/systemone`, with an
optional `LAYA_API_KEY` bearer check, and `GET /health`. It does not implement
Chat Completions or model discovery. Configure its physical provider with the
`jev_compatible` profile. Discovery tries `/v1/models`, then uses the health
response's `loaded` checkpoint names only on HTTP 404 or 405; authentication
failures and malformed catalogs remain errors. Explicit static models are also
supported when discovery is disabled.

Laya accepts named checkpoints such as `english`, `multilingual`, and
`typed-decisions`; unknown Jev model names permit server-side auto-selection.
Torii sends the same seven typed questions as Jev and validates every returned
answer. Laya's `action` and `routing` metadata remains compatible; choice
probability validation allows the aggregate error from rounding each option to
four decimal places. `output_tokens: 0` is valid for these decisions.

This describes protocol support. No accuracy, latency, or cost improvement is
inferred from synthetic contract fixtures. A connected generative classifier
must use the separate Chat Completions classifier contract.

### Sources

- https://github.com/LeonaDavinci/laya-system-one/blob/main/laya/serve.py
- https://github.com/LeonaDavinci/laya-system-one/blob/main/laya/agent.py

---

## Decision API discovery and passthrough

Verified 2026-10-12 against primary API documentation and read-only public catalogs.
OpenRouter's default `/api/v1/models` excludes decision output. Querying
`output_modalities=decisions` returns Jev (`typesafe/jev-1.13`, latest aliases) and
other decision families. Their `architecture.output_modalities` contains
`decisions`; the similarly named `typesafe/jev-router` emits text and must not be
classified by name. Input prices are USD per token, output prices can be zero.
`POST /api/v1/systemone` accepts the TypeSafe `model`, `state`, `questions` shape
and bearer inference key, with provider-qualified IDs or upstream bare aliases.
Torii forwards this protocol without normalizing questions, answers or extensions.
The TypeSafe model-list response uses `models` entries containing `name`,
`description`, `release_date`; Torii adds this array beside OpenAI `data`.
No authenticated or billable live decision requests were used for validation.

Sources:

- https://openrouter.ai/docs/guides/community/typesafe-sdk
- https://openrouter.ai/docs/api/api-reference/models/list-all-models-and-their-properties
- https://openrouter.ai/api/v1/models?output_modalities=decisions
- https://docs.typesafe.ai/sdk/python/api/types/responses

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

Verified 2026-10-10: `GET /key` accepts the inference bearer and returns key
spend, `limit_remaining`, `limit_reset`, and optional effective-budget/free-model
daily-request fields. Torii retains upstream daily/weekly/monthly USD spend,
key/effective-budget windows, and free-request limits only for `:free` models.
Daily resets use midnight UTC, weekly resets Monday UTC, and monthly resets the
first day UTC; explicit effective-budget reset timestamps take precedence for
that window. Null limits remain unknown, independently of reported spend.

Updated 2026-10-11: `GET /credits` accepts ordinary API keys as described in
the maintained Hermes account-usage implementation and subscription meter.
Torii uses the ordinary key by default and a separate `management_token` when
configured; management credentials never reach inference. Missing or failed quota reads keep inference available
with unknown quota; successful account credits remain usable if key lookup fails.
Live `/models` prompt/completion prices are USD per token and are converted to
USD per million tokens. Contract tests use synthetic upstreams, not live billing.

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

- https://openrouter.ai/docs/api/api-reference/api-keys/get-current-api-key
- https://openrouter.ai/docs/api/reference/limits
- https://openrouter.ai/docs/quickstart
- https://openrouter.ai/docs/api/api-reference/models/get-models
- https://openrouter.ai/docs/api/api-reference/credits/get-credits
- https://openrouter.ai/docs/guides/overview/auth/management-api-keys

---

## OpenAI API

### Status

Research status: verified from official API reference
Implementation status: complete for the transparent alpha (v0.1.0) endpoint set
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
authorization-code + PKCE is available with `--browser`. The console uses
server-side device authorization for local and remote gateways, opening OpenAI
in another browser tab with a one-time code and no helper download.

Device-flow source reviewed 2026-10-12: OpenAI returns `device_auth_id`,
`user_code` (or `usercode`) and a string polling interval, without a verification
URL. The browser URL is `/codex/device`; polling `/api/accounts/deviceauth/token`
returns 403/404 while pending, then an authorization code and PKCE verifier.
Torii exchanges them at `/oauth/token` with the fixed Codex client and
`{issuer}/deviceauth/callback`. Device credentials retain account/client/issuer
context and use the Codex backend. Existing plan-usage/helper tokens retain their
separate endpoint and refresh behavior. Tests use synthetic upstream fixtures;
this change has not been verified with a live OpenAI account.

Last source review: 2026-10-12

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
`/models` endpoint. Torii exposes visible entries; hidden entries are omitted. The backend
`supported_in_api` flag does not filter subscription models. If the account is not logged in or discovery
is temporarily unavailable, the configured fallback model list remains
available; a successful login refreshes the live catalog without restarting
the gateway.

Quota source verification, 2026-10-11 (source inspection and mocked integration,
not authenticated live validation): official `backend-client` selects
`/backend-api/wham/usage` for ChatGPT bases and `/api/codex/usage` for Codex API
bases. Torii follows that split using the same endpoint-bound authentication
manager as inference. Its dynamic plan-usage OAuth scope selects the public API
origin; whether the affected saved token is authorized for that usage endpoint
remains unverified. A working inference call does not establish usage permission.

Before this change, polling already loaded the inference credential and refreshed
known expirations, but did not retry a 401. Missing expiry could also hide an
expired JWT. Refresh now accepts optional expires_in, clears the old expiry and
uses the new token expiry hint when available; it does not persist expired tokens. Source inspection found no independent quota credential copy. No
credential or request trace for the original 401 is available, so expiry, account
selection, audience/permission mismatch and upstream changes remain possible.

Refresh uses the existing OAuth grant/client-specific path, re-reads under a
per-account lock shared across handler replacement, and securely persists rotated
credentials only if the login still matches. A 401 permits one refresh and one
retry; failures never include response bodies or token values. JWT expiry/account
claims are routing hints, not verified identity evidence.

Official app-server `rateLimitsByLimitId` and legacy `rateLimits` are separate
schemas from direct `rate_limit`/`additional_rate_limits`. Torii preserves map
keys, provider IDs, names and independent windows. Direct ordinary limits have
account scope; additional names never imply model routing authority. Durations
classify 5h/weekly/custom/unknown windows. Missing data remains unavailable.
Credits and banked reset counts are read-only; no reset is redeemed.

An optional configured `codex app-server` performs initialize/initialized,
account/read and account/rateLimits/read over bounded JSON lines. Quota accountId
and email must both match Torii. A missing stored email cannot prove user
identity within a shared workspace, so the HTTP collector is used instead. Older snapshots without
accountId, notifications without account proof, mismatched/unauthenticated CLI
logins and unavailable executables fall back to HTTP. Torii neither copies nor
refreshes CLI credential files itself. Inference headers are separate supplementary
snapshots and cannot overwrite polled pools or a different account.

Reference implementations inspected: codex-session's quota service refreshes
and retries a 401; provider-usage prefers Hermes account helpers but its fallback
omits the account header; ai-usage-tracker delegates to Hermes account usage;
resetwatch separates read-only catalog probes from refresh-capable vendor probes;
rarf's Codex reader groups credential contexts and normalizes extra pools.
These are source observations, not evidence that Torii's affected login works.

Sources:

- https://github.com/openai/codex/blob/main/codex-rs/backend-client/src/client.rs
- https://github.com/openai/codex/blob/main/codex-rs/backend-client/src/client/rate_limit_resets.rs
- https://github.com/openai/codex/blob/main/codex-rs/app-server/tests/suite/v2/rate_limits.rs
- https://github.com/openai/codex/blob/main/codex-rs/app-server-protocol/schema/typescript/v2/GetAccountRateLimitsResponse.ts
- https://github.com/openai/codex/blob/main/codex-rs/app-server-protocol/schema/typescript/v2/RateLimitSnapshot.ts
- https://github.com/gubasso/codex-session/blob/develop/src/services/account/quota.rs
- https://github.com/PhoeniXAbhisheK/hermes-plugin-provider-usage/blob/main/dashboard/plugin_api.py
- https://github.com/Adolanium/hermes-resetwatch/blob/main/probe.py
- https://github.com/rarf/hermes-quota-plugin/blob/main/quota_providers/codex.py
- https://github.com/lvabarajithan/hermes-ai-usage-tracker/blob/main/dashboard/plugin_api.py

## Nous Portal

### API-key connection (verified 2026-10-11)

Live `GET https://inference-api.nousresearch.com/v1/models` accepts the operator's API key as a bearer credential. The response contains context lengths and per-token `pricing.prompt`/`pricing.completion`; Torii converts prices to per-million units. The service returns Cloudflare HTTP 403 for the default `Go-http-client/1.1` user agent; `ToriiGateway` succeeds. Torii identifies itself on discovery and inference requests. OAuth login is no longer exposed for Nous by the gateway or console. Existing OAuth credentials must be replaced with a provider API key.

The inference origin returned HTTP 404 for `/openapi.json`, `/v1/credits`, `/v1/balance`, `/v1/usage`, `/v1/account`, and `/v1/user`. The Portal API documentation was login-gated. No account balance or quota contract was verified for API keys. Absent provider telemetry remains unknown; inference response headers can still supply rate limits.

Sources: [Nous API documentation](https://portal.nousresearch.com/api-docs), [Nous model catalog](https://portal.nousresearch.com/models), and live authenticated discovery (credentials omitted).

Updated 2026-10-11: maintained Hermes `hermes_cli/nous_account.py` reads
`GET {portal}/api/oauth/account` with its OAuth bearer. The account collector supports that
endpoint for bearers with account access; API-key access remains unverified. `subscription` and
`paid_service_access` supply remaining subscription/top-up/rollover/total usable
USD credits and renewal times. A valid monthly denominator yields a plan gauge;
rollover above the cap never yields an invented percentage. Plan/member-spend
gauges are informational for routing because free models and top-ups have
different entitlements. Inference headers still supply request/token constraints.
No browser-cookie scraping or charge/top-up/reset mutations are performed.
Fixtures cover bearer headers, balances and renewal; no live billing credentials
were available for validation.



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

Research status: verified from current TypeSafe API documentation
Implementation status: direct and provider-transport classifiers implemented
Last verified: 2026-10-07

Jev returns typed decisions rather than chat completions. The current direct
endpoint is `POST https://api.typesafe.ai/v1/systemone`, with bearer
authentication. Torii accepts a configured service root or `/v1` API root,
retaining deployment prefixes, and sends `model`, `state`, and seven typed
questions. The request contains two `choice` questions for task and complexity
and five `noul` questions for requirements and latency preference.

Choice answers must match their requested type and option, contain confidence
in [0, 1], and include the complete valid probability distribution. The choice
must be a highest-probability option. Each requested noul answer must contain
a numeric probability in [0, 1]; omitted and null values are errors. Unknown
extension metadata is accepted. Confidence is the minimum of the two choice
confidences, including zero. Objective request capabilities are preserved on
success and by the configured capability fallback.

The direct configuration is:

    router:
      classifier:
        type: jev
        model: jev-latest
        base_url: https://api.typesafe.ai
        api_key:
          env: JEVMODEL_API_KEY

Provider transport uses a configured decision provider's origin, headers,
model aliases, and `/systemone` endpoint. The classifier owns the typed
request/response mapping. Both the direct client and dedicated typed provider
refuse redirects even with caller-supplied HTTP clients, keeping credentials
and request state at the configured endpoint. Upstream diagnostic bodies are
not included in classification errors.

### Sources

- https://docs.typesafe.ai/api
- https://github.com/LeonaDavinci/laya-system-one/blob/main/laya/serve.py
- https://github.com/LeonaDavinci/laya-system-one/blob/main/laya/agent.py

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

- Native `/api/show` discovery supplies capabilities, context and locality.
  Verified local completion models support schema-constrained `response_format`;
  cloud aliases and incomplete metadata keep structured-output support unknown.
  See [Ollama structured outputs](https://github.com/ollama/ollama/blob/main/docs/capabilities/structured-outputs.mdx)
  and [OpenAI compatibility](https://github.com/ollama/ollama/blob/main/docs/api/openai-compatibility.mdx)
  (checked 2026-10-12).
- Ollama authentication is not currently configured or sent.
- Model discovery has a 10-second deadline. Inference defaults to a configurable
  five-minute whole-request deadline and propagates caller cancellation.

## Custom OpenAI-Compatible Providers

### Status

Implementation status: complete for the transparent alpha (v0.1.0) endpoint set
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


## Extended account tracking — 2026-10-11

The following user-provided implementations were inspected as protocol/schema
references, not executed or copied as plugin code:

- [hermes-ai-usage-tracker](https://github.com/lvabarajithan/hermes-ai-usage-tracker): Go/Zen, Z AI, Kimi, MiniMax and DeepSeek probes.
- [billing](https://github.com/anotherchu/billing): Nous plan/top-up balance presentation and renewal semantics.
- [hermes-codex-resets](https://github.com/sxuff/hermes-codex-resets): public reset-analysis presentation; its third-party schedule is not an authoritative account quota and is not queried by Torii.
- [hermes-ledgerline](https://github.com/Adolanium/hermes-ledgerline): gateway estimates versus provider billing and catalog policy.
- [hermes-nous-prices](https://github.com/Adolanium/hermes-nous-prices): live Nous pricing/catalog propagation.
- [hermes-resetwatch](https://github.com/Adolanium/hermes-resetwatch): endpoint path styles, credential isolation and optional-read failure behavior.
- [hermes-subscription-meter](https://github.com/NealZhouPanda/hermes-subscription-meter): xAI sign/cent conversion, optional spend, OpenRouter ordinary-key credits and raw Nous balance fields.
- [hermes-quota-plugin](https://github.com/rarf/hermes-quota-plugin): Go window aliases, Z AI unit buckets, Kimi ratios, modern MiniMax token-plan used counters, DeepSeek currency preservation and scoped Anthropic/Codex limits.

Primary implementation/docs checked alongside those references:

- [Hermes account usage](https://github.com/NousResearch/hermes-agent/blob/main/agent/account_usage.py) and [Nous account parser](https://github.com/NousResearch/hermes-agent/blob/main/hermes_cli/nous_account.py).
- [xAI billing Management API](https://docs.x.ai/developers/rest-api-reference/management/billing): separate management bearer/team, negative cent `total.val` means prepaid credit; usage analytics POST is a read-only query. Zero prepaid credit alone does not prove API exhaustion because postpaid billing can continue.
- [Anthropic Usage and Cost API](https://platform.claude.com/docs/en/manage-claude/usage-cost-api): admin key, daily buckets, pagination, organization scope and cost decimal strings in cents. Priority-tier charges are excluded upstream. Torii requests complete reports with bounded pagination; incomplete totals are omitted. OAuth utilization is a percentage, not inferred tokens or dollars, and subscription windows do not gate API-key inference.
- [Standard Compute budgets](https://docs.standardcompute.com/budgets-and-limits): `x-sc-budget-renews-at`, budget exhaustion and dashboard accounting; no documented public bearer balance endpoint was verified. Torii observes the renewal header and supports an explicitly configured private tracking URL.
- [OpenCode Zen](https://opencode.ai/docs/zen/) and [upstream wallet API issue](https://github.com/anomalyco/opencode/issues/44189): wallet balance is not reliably exposed publicly. Torii reports this limitation explicitly and never assumes opaque integer wallet units are USD.

Text-model base prices are bundled from the MIT-licensed models.dev provider
registry at commit `095fbd1b6aed9876b98c3d983c6b03a3a49ece0e`, dated 2026-10-11.
The extracted input/output pairs and license are in `internal/provider`.
Live model prices and operator overrides take precedence. These are estimates;
DeepSeek peak/off-peak differences, cache discounts, context tiers and non-token
charges are outside the two-rate estimator. No provider credential is sent to
models.dev or any third-party reference service.

HTTP fixtures verify endpoint paths, credential types, string/number balances,
independent reset windows, incomplete-data handling and redirect refusal. They
establish adapter behavior, not live endpoint availability for every account/tier.

## Codex quota windows (verified 2026-10-11)

The [upstream backend client](https://github.com/openai/codex/blob/main/codex-rs/backend-client/src/client/rate_limit_resets.rs) polls `/wham/usage` beneath the ChatGPT backend API root. Torii uses the same account-bound credential headers as model discovery. Primary and secondary windows carry used percentages, window lengths, and reset timestamps. The most depleted window governs eligibility, and snapshots expire no later than the next window reset. Telemetry failures remain distinct from exhaustion.

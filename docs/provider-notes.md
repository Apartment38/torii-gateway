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

The repository is currently pre-v1. Version 1 provider research is limited to
local Ollama, standard OpenAI-compatible endpoints (including the normal OpenAI
API), and ChatGPT Codex subscription access. OpenRouter, Nous Portal, Gemini,
xAI, and Anthropic sections are retained for the Version 2 backlog. Before
changing an adapter, replace its placeholders with dated findings from primary
documentation, identify which fields are verified versus inferred, and record
unsupported behavior explicitly. A provider task is not complete until its
notes and contract tests agree.

The Version 1 router may use a self-hosted Laya model through an already
configured local physical provider. Laya protocol and schema findings belong in
this document before TASK-026 is implemented; no dedicated hosted-Laya provider
or credential flow is in scope. Jev classifier transports are Version 2 work.

---

## OpenRouter

### Status

Research status: TODO
Implementation status: TODO
Last verified: YYYY-MM-DD

### Authentication

API key:

    Authorization: Bearer <token>

### API

Base URL:

    <verified URL>

### Model Discovery

Endpoint:

    <endpoint>

Record:

- model IDs
- context windows
- capabilities
- pricing fields
- provider metadata

### Quotas

Document whether remaining credits/quota can be queried and through which
endpoint or headers.

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

Add implementation-relevant behavior here.

### Sources

- <source>
- <source>

---

## OpenAI API

### Status

Research status: TODO
Implementation status: TODO
Last verified: YYYY-MM-DD

### Authentication

OpenAI API key.

This provider must remain separate from OpenAI Codex subscription auth.

### Notes

...

---

## OpenAI Codex Subscription

### Status

Research status: TODO
Implementation status: TODO
Last verified: YYYY-MM-DD

### Authentication

ChatGPT/Codex OAuth.

Investigate and document:

- device-code flow
- authorization-code + PKCE
- refresh behavior
- persisted credential structure
- account identifiers required by requests

### Transport

Document the current Codex Responses protocol.

Hermes Agent and Codex may be used as implementation references.

### Important Security Constraint

Never send:

    OPENAI_API_KEY

to the Codex subscription endpoint.

Never send Codex OAuth credentials to the normal OpenAI API endpoint.

### Model Discovery

Document how Codex-accessible models are discovered.

Do not assume the normal OpenAI API model catalog is equivalent.

### Sources

...

---

## Nous Portal

### Status

Research status: TODO
Implementation status: TODO
Last verified: YYYY-MM-DD

### Authentication

Verify:

- OAuth flow
- refresh-token behavior
- short-lived inference tokens
- API-key availability, if any

### Model Discovery

...

### Sources

...

---

## Google Gemini

...

## xAI

...

## Anthropic

...

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

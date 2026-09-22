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

...

## Custom OpenAI-Compatible Providers

Record generic assumptions and compatibility requirements here.

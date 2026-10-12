# Features and limitations

This inventory describes implemented repository behavior. External-service access
still depends on your account, model capabilities, quotas, and upstream availability.

| Area | Supported behavior | Main limits |
| --- | --- | --- |
| Public API | Model list/detail; Chat Completions, Completions, Embeddings, stateless Responses, System One decisions | Endpoint support varies by provider; no stored Responses conversations |
| Multiple providers | Unique `provider/model` IDs, discovered/static models, aliases | Explicit model requests never change providers |
| Transparent transport | Unknown-field preservation; JSON and byte-preserving SSE | Upstream must support requested fields/endpoint |
| Translated transport | Codex Responses and Anthropic Messages adapters | Explicit protocol restrictions; not all native features exposed |
| Console | Embedded light/dark UI, connection tests, live validated save/reload | Management requires one admin; listener/database changes require restart |
| Credentials | AES-256-GCM encrypted SQLite payloads, provider references, default/named client keys | Master key required; saved secrets cannot be retrieved; no automatic key rotation |
| Subscription access | Codex local/remote login/helper; Nous Portal API-key access | Account eligibility and provider token scopes required |
| Smart routing | Deterministic selection; optional connected, embedding, typed Jev/Laya classification | Only eligible candidates; decision-only models never answer chat |
| Routing policies | Capabilities/context, quality floors, paid-model gate, estimated cost, location, quotas, latency | Unknown measurements remain unknown; estimates are not bills |
| Sessions | In-memory bounded affinity, explicit deletion, safe tool pinning, pre-stream rerouting | Lost on restart; tool continuation cannot move to an unpinned model; no retry after bytes sent |
| Benchmarks | Five source families, background/manual refresh, cache, aliases, scores, raw inspection, progress | Opt-in; source terms/schema/availability apply; approximate display matches never route |
| Runtime | Request IDs, typed errors, structured safe logs, graceful shutdown, healthcheck | TLS needs reverse proxy; body limit 32 MiB |
| Persistence | Embedded SQLite credentials, benchmark evidence, usage, host identity | One gateway per local data volume; sessions/quota/login flows remain in memory |
| Release tooling | Versioned gateway/helper builds, checksums, multiarch container workflow | Workflow existence does not guarantee a published release |

Not implemented: multi-user administration, administrator password reset,
Authentik/OIDC, automatic master-key rotation, automated update installation,
stateful Responses storage and a provider billing ledger. JSON and SSE usage are tracked as operator accounting, with conservative estimates when usage is missing.
Native provider audio/media endpoints and MCP/tool/skill gateways are not part
of the public API. Current pending checks and future work live only in
[Project status](../development/status.md).

[Provider matrix](providers.md) · [API](api.md) · [User guides](../README.md)

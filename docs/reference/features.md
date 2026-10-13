# Features and limitations

This inventory describes implemented repository behavior. External-service access
still depends on your account, model capabilities, quotas, and upstream availability.

| Area | Supported behavior | Main limits |
| --- | --- | --- |
| Public API | Model list/detail; Chat Completions, Completions, Embeddings, stateless Responses, System One decisions | Endpoint support varies by provider; no stored Responses conversations |
| Multiple providers | Unique `provider/model` IDs, discovered/static models, aliases | Explicit model requests never change providers |
| Transparent transport | Unknown-field preservation; JSON and byte-preserving SSE | Upstream must support requested fields/endpoint |
| Translated transport | Codex Responses and Anthropic Messages adapters | Explicit protocol restrictions; not all native features exposed |
| Console | Embedded light/dark UI, connection tests, revision-protected live saves, Activity and provider balances | One administrator; listener/file logging changes require restart |
| Administrator account | Username/email/password updates and single-use email password recovery | Current password required for account edits; recovery needs a saved email and configured HTTPS/SMTP |
| Client accounting | Persistent per-key usage and daily/monthly request, token and estimated-cost budgets | Named keys are not users; estimates are not provider billing |
| Credentials | AES-256-GCM encrypted SQLite payloads, provider references, default/named client keys | Master key required; saved secrets cannot be retrieved; no automatic key rotation |
| Subscription access | Codex device/browser OAuth; Nous API-key access | Account eligibility and provider token scopes required |
| Smart routing | Default `smart` and additional named routers with independent pools/policies; optional connected, embedding, typed Jev/Laya classification | Only eligible candidates; decision-only models never answer chat |
| Routing policies | Capabilities/context, quality floors, paid-model gate, estimated cost, location, quotas, latency | Unknown measurements remain unknown; estimates are not bills |
| Sessions | In-memory bounded affinity, explicit deletion, safe tool pinning, pre-stream rerouting | Lost on restart; tool continuation cannot move to an unpinned model; no retry after bytes sent |
| Benchmarks | Five source families, background/manual refresh, cache, aliases, scores, raw inspection, progress | Opt-in; source terms/schema/availability apply; approximate display matches never route |
| Runtime | Request IDs, typed errors, structured logs, bounded Activity history with opt-in body capture, graceful shutdown, healthcheck | TLS needs reverse proxy; body limit 32 MiB |
| Persistence | Embedded SQLite settings/revisions, credentials, administrator sessions, accounting, Activity, benchmark evidence and host identity | One gateway per local data volume; conversation affinity, provider quota caches and pending login flows remain in memory |
| Release tooling | Versioned gateway builds, checksums, multiarch container workflow | Workflow existence does not guarantee a published release |

## Current limitations

Not implemented: multi-user administration,
Authentik/OIDC, automatic master-key rotation, automated update installation,
stateful Responses storage and a provider billing ledger. JSON and SSE usage are tracked as operator accounting, with conservative estimates when usage is missing.
Native provider audio/media endpoints and MCP/tool/skill gateways are not part
of the public API.

## Planned features

The [README feature table](../../README.md#features) lists the following future
features. None is available in the current codebase.

| Planned feature | Intended behavior | Current boundary |
| --- | --- | --- |
| TUI | Terminal user interface with web-console feature parity | Use the existing CLI, HTTP API or browser console |
| Skill Gateway | Centralized, version-controlled skill registry | Torii does not store, version or serve client skills |
| MCP Gateway | Aggregate Model Context Protocol servers behind a gateway | No MCP server aggregation or gateway endpoints |
| User Accounts | Separate users with their own accounting and settings | One administrator; named client keys provide usage/budgets, not user accounts or per-user settings |
| Authentik Integration | Authentik login for the web console | Administrator cookie sessions and Basic authentication only; no OIDC login |

The development roadmap additionally lists dedicated Hermes and Pi plugins.
These plans do not define supported configuration fields or endpoints. Existing tool-call
forwarding is not an MCP server; named API keys and budgets are not multi-user
accounts. Hermes can already connect through the
[OpenAI-compatible client interface](../guides/clients.md#configure-an-openai-compatible-client).
Scheduling and pending validation remain in
[Project status](../development/status.md#pending-release-checks-and-future-work).

[Provider matrix](providers.md) · [API](api.md) · [User guides](../README.md)

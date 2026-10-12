# HTTP API reference

The base origin in examples is `http://127.0.0.1:8080`. Use HTTPS remotely.
For client setup see [Clients](../guides/clients.md); supported endpoint/model
combinations are in [Provider support](providers.md).

## Authentication

| Boundary | Authorization |
| --- | --- |
| `/v1/*` inference and model routes | `Authorization: Bearer GATEWAY_KEY` |
| Config, provider tests/login/status/helper download, credential/client-key management | Administrator cookie session (CSRF required for writes) or HTTP Basic (`curl -u USERNAME`, prompts for password) |
| Intelligence and session deletion | Administrator session/Basic or gateway bearer key |
| Static console, `/healthz`, setup status | Public |
| Initial setup write | Unused 30-minute setup code, only while no administrator exists |
| Remote login session/import | One-time pairing bearer token (10-minute pending session), not an inference key |
| OAuth callback | Pending authorization state validation |

Management responses use `Cache-Control: no-store` and enforce same-origin
boundaries when an Origin header is present. Static console availability does
not imply public access to configuration. Basic and bearer keys have different
permissions. There is one administrator; named client keys do not create users.

## Inference and models

| Method/path | Input | Successful result |
| --- | --- | --- |
| `GET /v1/models` | No body | OpenAI-style `object: list`, `data` model objects |
| `GET /v1/models/{provider/model}` | Public ID in path | One model object; 404 if unknown |
| `POST /v1/chat/completions` | `model`, `messages`, optional `stream` and supported upstream fields | Chat JSON or SSE |
| `POST /v1/completions` | `model`, `prompt`, supported upstream fields | Completion JSON or SSE if upstream supports it |
| `POST /v1/embeddings` | `model`, `input`, supported upstream fields | Embedding JSON |
| `POST /v1/responses` | `model`, `input`, optional instructions/tools/stream | Stateless response JSON or adapted SSE |

Bodies are JSON objects and limited to 32 MiB. Transparent forwarding replaces
only the top-level model value, preserving other fields/bytes. The model may be
`smart`, or a provider-qualified ID. `previous_response_id` and `conversation`
on Responses are rejected with `unsupported_parameter`. Context must be supplied
in each request. Provider-specific endpoint, tool, vision, and format limits apply.

### Model cost, usage and provider quota metadata

Physical models in both discovery routes include additive metadata for harnesses
and selectors. IDs remain unchanged; `smart` stays first in the list.

| Field | Meaning |
| --- | --- |
| `pricing.input_per_million`, `pricing.output_per_million` | Known USD rates per million tokens; missing rates are unknown. Top-level rate fields are also returned for compatible consumers. |
| `pricing.source` | Price provenance: live provider catalog, dated bundled base-rate snapshot, subscription inclusion or operator override. Cached tokens, variable rates and extra charges can differ. |
| `context_window`, `subscription` | Known token capacity and explicit subscription billing; subscription prices represent zero marginal inference cost. |
| `quota.known`, `quota.exhausted`, `quota.blocked` | Current provider allowance state; unknown does not imply zero balance or unlimited usage. |
| `quota.windows[]` | Independent constraints with `name`, `unit` (`USD`, `percent`, `requests`, `tokens`), `limit`, `used`, `remaining`, `fraction_remaining`, optional `reset_at` (UTC RFC3339), and optional `window_seconds`, `model` scope, and `informational` flag. Informational windows describe another model or a plan gauge that does not constrain this model. |
| `quota.source`, `checked_at`, `expires_at` | Snapshot provenance/freshness. The summary uses the window with the smallest remaining fraction; inspect all windows for their reset times. |
| `quota.usage_usd` | Provider-reported dollar spend when available, with optional total/daily/weekly/monthly fields; separate from gateway estimates. |
| `quota.balances[]` | Provider balances with `name`, native `unit`, optional numeric `remaining`, `granted`, `purchased`, `unlimited` and UTC `reset_at`. Multiple currencies and plan/top-up components are never added together. A balance alone does not make `quota.known` true. |
| `quota.reported_usage[]` | Provider-reported `period`, `unit`, `used`, and optional `scope`; organization reports are labeled and separate from client-key gateway usage. |
| `quota.plan`, `quota.reset_credits` | Reported plan and Codex banked reset count; displaying resets never redeems them. |
| `quota.partial`, `quota.unavailable_reason` | Partial provider reads and stable reasons such as `management_credentials_required`, `admin_credentials_required`, `credential_rejected`, `endpoint_unavailable`, `wallet_balance_not_exposed`, `response_headers_only`, `no_subscription`, `spend_unavailable`, `report_incomplete` or `no_reported_data`. Missing data stays unknown. |
| `usage_available`, `usage[]` | Current UTC day/month gateway usage for the authenticated client key, including reported/estimated tokens, estimated cost and unknown-cost attempts. Empty usage with availability true means no recorded usage; false means unavailable. Key IDs/names are omitted. |

`GET /gateway/v1/models` uses the same tracking fields with gateway-wide usage
and its existing capabilities/benchmark metadata. Quota failures keep discovery
available and return an unknown snapshot. Account snapshots are shared across
models, with a separate OpenRouter free-model bucket. Refreshes are bounded by
a shared ten-second picker deadline and quota TTL; known reset times shorten cache
life. Observed response headers preserve billing data without extending its
original expiration. Smart uses the same pricing and quota cache for eligibility/ranking.
External harnesses must consume these additive fields to render them; clients
that ignore extended model metadata retain their existing selectors.

### Request examples

Set `GATEWAY_API_KEY` to your saved Torii key. Substitute actual discovered models
with the needed endpoint capability:

```sh
curl --fail http://127.0.0.1:8080/v1/chat/completions   -H "Authorization: Bearer $GATEWAY_API_KEY"   -H 'Content-Type: application/json'   -d '{"model":"local/llama3.2","messages":[{"role":"user","content":"Hello!"}]}'
curl --fail http://127.0.0.1:8080/v1/completions   -H "Authorization: Bearer $GATEWAY_API_KEY"   -H 'Content-Type: application/json'   -d '{"model":"local/llama3.2","prompt":"Complete this: Hello"}'
curl --fail http://127.0.0.1:8080/v1/embeddings   -H "Authorization: Bearer $GATEWAY_API_KEY"   -H 'Content-Type: application/json'   -d '{"model":"local/nomic-embed-text","input":"Text to embed"}'
curl --fail http://127.0.0.1:8080/v1/responses   -H "Authorization: Bearer $GATEWAY_API_KEY"   -H 'Content-Type: application/json'   -d '{"model":"openai/RESPONSES_CAPABLE_MODEL","input":"Hello!"}'
```

The embedding example requires that model installed in Ollama; the Responses
placeholder must be replaced with an actual supported model. JSON payload shape
follows the selected OpenAI-compatible endpoint. Streaming uses `stream: true`
and `curl --no-buffer`; after bytes are sent, errors cannot become a replacement
JSON response and no cross-model retry is attempted.

## Health, setup, configuration, and providers

`GET /v1/models` lists `smart` first, followed by the deterministically sorted
physical `provider/model` IDs. `smart` stays visible in clients that truncate a
provider's initial model list; the endpoint returns the complete catalog.

| Method/path | Request | Response / limits |
| --- | --- | --- |
| `GET /healthz` | None | HTTP 200, `status: ok`; server liveness, not all-provider readiness |
| `GET /gateway/v1/setup` | None | `setup_required` boolean |
| `POST /gateway/v1/setup` | `username`, `password`, `setup_code` | 201 with username and newly generated `api_key` once; existing admin gives 409, bad/expired code 403 |
| `GET /gateway/v1/config` | Admin | Config JSON with references, never secret values; `ETag` identifies its revision |
| `PUT /gateway/v1/config` (also `POST`) | Full config JSON, admin, `If-Match` | `reloaded: true` on successful validated atomic save; revision checks described below |
| `POST /gateway/v1/providers/test` | `{"provider": PROVIDER_CONFIG}` JSON, admin | Actual discovery status/models, including classifier-only metadata; no inference test |
| `POST /gateway/v1/providers/login` | `{"provider_id":"ID"}` or `{"provider":"ID"}`, admin | Pending login instructions/URL or helper session details |
| `GET /gateway/v1/providers/login?provider=ID` | Admin | Login status without stored tokens |
| `GET /gateway/v1/providers/login/helper/{asset}` | Admin | OS/architecture-matching binary; unavailable assets fail |
| `GET /gateway/v1/providers/login/session` | Pairing bearer token | Pending session metadata used by helper |
| `POST /gateway/v1/providers/login/import` | Pairing bearer token and helper-generated OAuth payload | Validated single-use credential import; token/state/endpoint/scopes checked |
| `GET /gateway/v1/providers/login/callback` or `/auth/callback` | Authorization code/state query | Complete matching pending login; use generated flow rather than constructing callback |
| `DELETE /gateway/v1/sessions/{id}` | Bearer/admin | End in-memory affinity for that identity |

Configuration saves replace the full document. First read it, retain the exact
quoted `ETag` response header, and send it as `If-Match` when submitting your
edited document. Missing revisions return **428 Precondition Required**;
outdated revisions return **412 Precondition Failed**, before credentials or
configuration are written. One exact strong tag is required; wildcard, weak,
and multiple tags are rejected. This is required for API scripts as well as the
console, including the POST alias.

Every successful save advances the revision, including an unchanged document.
Read again before preparing another save. Revisions persist across restarts; successful changes advance them. Normal operation never reads external YAML edits. On conflict, reload and reconcile
your draft; do not fetch a new revision merely to retry the old full document.
The console preserves conflicted drafts. Copy your edits before choosing
**Refresh config**, which discards the local draft.

Use the console/helper to orchestrate OAuth; do not hand-author import payloads.
The helper URL/token comes from the pending login, is short-lived, and must not be
logged or shared. See [Providers](../guides/providers.md).

## Credentials and client keys

| Method/path | Request | Result |
| --- | --- | --- |
| `GET /gateway/v1/credentials` | Admin | `credentials` metadata list; no values |
| `POST /gateway/v1/credentials` | `value`, `service`, `purpose`, admin | HTTP 200, `credential` ID and `status: configured` |
| `PUT /gateway/v1/credentials/{id}` | `value`, `service`, `purpose`, admin | Replace generic secret; preserve existing service/purpose |
| `DELETE /gateway/v1/credentials/{id}` | Admin | Revoke generic credential; protected system/OAuth/client-key records denied |
| `POST /gateway/v1/admin/api-key` | Admin | New default `api_key` once; prior default invalid immediately |
| `GET /gateway/v1/admin/api-keys` | Admin | `api_keys` metadata list |
| `POST /gateway/v1/admin/api-keys` | `{"name":"Client"}`, admin | 201, `key` metadata and newly generated `api_key` once |
| `DELETE /gateway/v1/admin/api-keys/{id}` | Admin | Delete/revoke named or default key |

Example creating a client key, with a password prompt:

```sh
curl --fail -u YOUR_ADMIN_USERNAME http://127.0.0.1:8080/gateway/v1/admin/api-keys   -H 'Content-Type: application/json' -d '{"name":"My client"}'
```

Save the returned key securely. Responses containing it must not be shared.
See [Credentials](../guides/credentials.md) for consequences of rotation/deletion.

## Benchmark intelligence

All routes below use the `/gateway/v1/intelligence` prefix and accept admin Basic
or gateway bearer authentication for reads/refresh. Uploads and edits require
administrator authentication. Disabled intelligence returns 503, except for the
CSV template download.

| Method/suffix | Result and parameters |
| --- | --- |
| `GET` base path | Full snapshot, sources/observations, models, unmatched |
| `GET /sources` | Source status, safe errors, last attempt/success, count, age, metadata |
| `GET /models` | Matched summaries without raw observations; `include_unmatched=true` adds display-only summaries; `configured_only=true` filters by catalog associations |
| `GET /observations` | Raw paginated observations; `offset` ≥0, `limit` 1–500 (default 100), optional `q` ≤256 characters; `include_metadata=true` includes captured metadata |
| `GET /models/{id}` | Canonical/physical model details with raw observations/provenance |
| `GET /unmatched` | Unmatched/ambiguous IDs and candidate identities |
| `GET /template.csv` | Download CSV header and sample; also available when intelligence is disabled |
| `POST /import` | Administrator-only CSV body, ≤8 MiB/10,000 rows; atomically merge into reserved `custom` source |
| `GET /records` | Paginated editable records using observation pagination/filter parameters; each has `source`, `index`, `revision`, `observation` |
| `PUT /observations/edit` | Administrator-only JSON `{source,index,revision,observation}` from `/records`; validates and saves one row, HTTP 409 for stale revisions |
| `POST /refresh` | JSON `updated`, `partial_failure`, `sources`, model/unmatched counts; failed sources retain cache |

```sh
curl --fail http://127.0.0.1:8080/gateway/v1/intelligence/sources   -H "Authorization: Bearer $GATEWAY_API_KEY"
curl --fail --no-buffer -X POST http://127.0.0.1:8080/gateway/v1/intelligence/refresh   -H "Authorization: Bearer $GATEWAY_API_KEY" -H 'Accept: text/event-stream'
```

SSE refresh emits `progress` events with phase/source/completed/total/failed/
observations/message, 15-second heartbeats, and a final `result` summary.
An existing worker is observed rather than duplicated. Reverse proxies need
streaming and sufficiently long read timeouts (`X-Accel-Buffering: no` is set).
Both JSON and SSE callers must check partial-source failure, not HTTP 200 alone.

CSV fields and edit behavior are documented in [Custom benchmarks](../guides/benchmarks.md#upload-custom-benchmarks).
Upload/edit success returns `{"updated":true}`. Invalid data returns HTTP 400;
cookie mutations require the CSRF token. Editing a publisher source can be
replaced by its next refresh; custom measurements survive refresh and restart.

## Errors and correlation

Inference errors use an OpenAI-style envelope, for example:

```json
{"error":{"message":"The model does not exist","type":"invalid_request_error","param":null,"code":"model_not_found"}}
```

| Status | Typical cause |
| --- | --- |
| 400 | Invalid JSON/config/parameter or unsupported endpoint/capability |
| 401 | Invalid/missing credentials for this authentication boundary |
| 403 | Cross-origin/unauthorized management or invalid setup/pairing flow |
| 404 | Unknown model/path/resource |
| 409 | Setup already complete or conflicting management state |
| 413 | Inference body exceeds 32 MiB |
| 429 | Upstream rate limit/quota exhaustion or login failure limiting |
| 502 | Upstream/auth/classifier/stream failure |
| 503 | No eligible model, missing tool pin, disabled intelligence, unavailable state |

Management routes also return safe validation messages; some boundary failures
use plain HTTP text. Do not assume every route has the inference error envelope.
`X-Request-ID` is returned on responses; a single safe supplied ID ≤128 characters
is retained, otherwise generated. Upstream headers cannot replace it.

Smart response evidence includes `X-Torii-Model`, `X-Torii-Task`,
`X-Torii-Complexity`, `X-Torii-Routing-Ms`, `X-Torii-Classifier-Ms`,
`X-Torii-Classifier-Source`, and `X-Torii-Estimated-Inference-Cost-Usd`.
Classifier version and input/output token headers appear only when reported and
valid. `X-Torii-Classifier-Cost-Usd` remains `unknown`; inference cost is an
estimate, not a bill. Cached tool continuation is not a fresh measured classifier
call. These headers expose safe evidence, not prompts or full classifier context.

## Administrator browser sessions

`POST /gateway/v1/session` returns HTTP 201, accepts `{username,password}` and sets an opaque HttpOnly, SameSite=Lax cookie. Only its hash is stored in SQLite; expiry defaults to seven days (`server.admin_session_ttl`, positive Go duration up to `720h`). `GET` returns username, expiry and `csrf_token`; `DELETE` revokes the session and clears its cookie. Browser mutations must send `X-CSRF-Token` from the session response and a matching Origin when present. Cross-site requests are denied. Logout and administrator record changes invalidate sessions. Basic administrator authentication remains available for scripts; inference requires a Bearer key.

Cookies are Secure on direct HTTPS and when the immediate peer belongs to an explicitly configured `server.trusted_proxies` CIDR and sends `X-Forwarded-Proto: https`. Untrusted forwarding headers have no effect. Direct HTTP remains supported. `/login` is the dedicated login page; first-run setup stays at the console root.

## Model management

Administrator `GET /gateway/v1/models` returns discovered physical models, enabled state, catalog capabilities/pricing, and safely matched benchmark intelligence with source, freshness and missing-score metadata. It uses the existing identity/alias resolver; approximate display matches supply no scores. `PUT` accepts `{id,enabled}` and requires the configuration `If-Match` revision; its successful `ETag` is the newly committed settings revision. Full config changes can also set `model_enabled`.

## Usage and quotas

Administrator `GET /gateway/v1/usage` and `/gateway/v1/quotas` return current UTC day/month totals by key/provider/physical model, reported versus estimated tokens, unknown-cost counts, configured budgets, remaining limits and reset times, plus a `provider_quotas` map keyed by physical model (bounded to 10,000 usage rows). `PUT /gateway/v1/quotas` accepts the `budgets` settings object and requires `If-Match`; full configuration saves can also edit budgets. Named inference keys resolve to their stable credential IDs and names. Quota failures return OpenAI-compatible HTTP 429 (`insufficient_quota`, `quota_exceeded`) with `Retry-After` and `X-Quota-Reset`. Quota storage failures return HTTP 503 before dispatch for requests subject to limits. Smart filters candidates against budgets and may retry a different eligible model before response commitment.

## Live activity

Administrator `GET /gateway/v1/activity?limit=100&after_id=ID` returns recent metadata and a gap indicator; limits range from 1 to 1000. `GET /gateway/v1/activity/events` streams `activity` and `gap` SSE events with persistent IDs. Reconnect with `Last-Event-ID` or `after_id`. Initial replay is bounded to 1000 events; retention, restored histories, recording overflow and slow subscribers report gaps. Recording queues hold 1024 events; each viewer has 64 queued events, with at most 128 viewers. Slow viewers disconnect and can replay retained history. Session validity is rechecked on keepalives. Events include request start, routing/selected provider and completion outcome, duration, API-key name, tokens and estimated cost qualifications; content and credentials are excluded.

Provider quota snapshots are included in management model records and public `/v1/models` records as `quota`: `known`, `limit`, `used`, `remaining`, `fraction_remaining`, `exhausted`, `checked_at`, `expires_at`, `reset_at`, and `windows`. Each window supplies `name`, `unit`, `limit`, `used`, `remaining`, `fraction_remaining`, and optional `window_seconds` and `reset_at`. Percent windows use a limit of 100. Clients can show reset countdowns from UTC timestamps. Failed telemetry has `unavailable_reason`; unknown is distinct from exhausted. Usage responses additionally include `provider_quotas`, keyed by physical model. Harness model pickers can consume these optional fields; displaying them requires client support.

Administrator `POST /gateway/v1/providers/ollama-detect` accepts `{"port":55242}` or `{"port":0}` for detection. It returns `base_url`, `port`, and `model_count`, or HTTP 422 when no local service is reachable. It probes local and container-host addresses only.

Codex inference `X-Codex-Primary-*` and `X-Codex-Secondary-*` headers, when present, supply shared account-window telemetry to router eligibility and the model catalog if polling is unavailable. Header snapshots are bounded by cache/reset expiry; no data is fabricated when neither channel supplies it.

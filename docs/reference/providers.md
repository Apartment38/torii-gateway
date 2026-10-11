# Provider support

Add providers with the [provider guide](../guides/providers.md). Fields and profile
restrictions are in [Configuration](configuration.md). Model-level support is
verified independently; accepting a route does not guarantee every discovered
model supports it. Use an upstream model suited to the requested task.

| `type` | API base URL | Authentication/discovery | Inference surface |
| --- | --- | --- | --- |
| `ollama` | Your Ollama origin, e.g. `http://127.0.0.1:11434` | Local service; installed model discovery and native capability metadata | Transparent Chat, Completions, Embeddings, stateless Responses, subject to Ollama/model support |
| `openai` | Exactly `https://api.openai.com` | Required API key; discovery always enabled; optional organization/project | Transparent OpenAI endpoints, subject to model eligibility |
| `openai_compatible` | Your compatible HTTP(S) origin/path prefix | Optional bearer/custom headers, static/discovered models | Transparent Chat, Completions, Embeddings, stateless Responses if upstream supports them |
| `openai-codex` | Usually `https://chatgpt.com/backend-api/codex`; token-scoped plan-usage flow can use `https://api.openai.com/v1` | Gateway-managed subscription OAuth; live discovery; configured models only fallback | Translated Chat and stateless Responses; JSON/SSE; no Completions or Embeddings |
| `nous` | `https://inference-api.nousresearch.com/v1` | Portal subscription/device login and refresh; discovered/static models | Compatible Chat, Completions, Embeddings, stateless Responses where supported by service/model |
| `openrouter` | `https://openrouter.ai/api` | Required inference key; optional separate management key for credits | Compatible `/v1` endpoints; native metadata and quota support |
| `gemini` | `https://generativelanguage.googleapis.com` | Required bearer key; discovery/static models | Compatible `/v1beta/openai` endpoints; not a native Gemini API adapter |
| `xai` | `https://api.x.ai` | Required bearer key; discovery/static models | Compatible `/v1` endpoints, subject to upstream/model support |
| `anthropic` | Exactly `https://api.anthropic.com` | Required API key; discovered/static models | Native Messages translation for Chat and basic stateless Responses, including SSE; no Completions/Embeddings |
| `jev` | `https://api.typesafe.ai`, default `api_path: /v1` | Required key; discovered/static classifier models | Typed System One classification only; excluded from inference catalog |
| `jev_compatible` | Your compatible decision-service origin | Optional key; `/models` discovery, `/health` fallback on 404/405 | Typed System One classification only; excluded from inference catalog |

## Compatibility details

- Codex credentials are endpoint-bound and separate from normal OpenAI API keys.
  Direct-scope helper tokens and legacy Codex OAuth tokens use their appropriate
  endpoint/refresh flow. Configured model fallbacks are not an allowlist.
- Anthropic translates instructions, text/images, tools/results, usage, finish
  reasons, and reasoning controls. Structured output is explicitly rejected.
  Responses support is basic and stateless, not native Responses equivalence.
- Transparent providers preserve unknown request fields. Translated adapters
  must preserve supported extensions or reject unsupported behavior explicitly.
- Compatible providers isolate credentials, headers, TLS, and timeouts in their
  own clients; no redirect credential forwarding. OpenRouter's management key
  is never substituted with the inference key for credit polling.
- Laya typed decisions use `jev_compatible`; a generative model emitting the
  classifier JSON contract uses the separate `model`/`laya` classifier types.
- Model catalog metadata merges bundled values, live authoritative values, then
  operator overrides. Unknown tool/vision/endpoint support is never guessed.
  Failed discovery retains last-known-good models marked stale or uses bundled
  fallback where available. The catalog refreshes on requests after one minute
  and after successful console login. An empty setup can run with only `smart`;
  inference still needs an eligible physical model.

## Costs, usage and remaining allowances

Nous Portal and OpenRouter discovery ingest live model prices. Configured/static
models receive live metadata when discovery is enabled; explicit subscription
billing has zero marginal cost. Unknown prices remain unknown.

OpenRouter polls key spend/remaining limits with the inference key and optionally
account credits with the separate management key. Free daily-request limits
apply only to `:free` models. Codex polls primary/secondary subscription usage
and reset times for legacy ChatGPT-bound OAuth accounts. Direct-scope public API
tokens use header observations when available. Portal tracks account-scoped
request/token quota headers and reset times; its dollar balance remains unknown
without a verified bearer-accessible balance endpoint.

These snapshots appear in [model discovery](api.md#model-cost-usage-and-provider-quota-metadata)
and the console alongside persistent gateway usage estimates. Provider balances
are separate from administrator-configured operator budgets. Quota snapshots
are in memory and refresh on demand; gateway accounting survives restarts.

Protocol research and dated upstream sources are in
[Development provider notes](../development/provider-notes.md). Those dated
observations are not promises about current account tiers or upstream availability.

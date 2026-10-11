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
| `nous` | `https://inference-api.nousresearch.com/v1` | Bearer API key; discovered/static models and live prices | Compatible Chat, Completions, Embeddings, stateless Responses where supported by service/model |
| `openrouter` | `https://openrouter.ai/api` | Required inference key; optional separate management key for credits | Compatible `/v1` endpoints; native metadata and quota support |
| `gemini` | `https://generativelanguage.googleapis.com` | Required bearer key; discovery/static models | Compatible `/v1beta/openai` endpoints; not a native Gemini API adapter |
| `xai` | `https://api.x.ai` | Required bearer key; discovery/static models | Compatible `/v1` endpoints, subject to upstream/model support |
| `opencode-go` | `https://opencode.ai`, `/zen/go/v1` | Required bearer key; discovered/static models | Go subscription usage windows |
| `opencode-zen` | `https://opencode.ai`, `/zen/v1` | Required bearer key; discovered/static models | Zen API, optional wallet tracking endpoint |
| `standard-compute` | `https://api.stdcmpt.com`, `/v1` | Required bearer key; discovered/static models | Included compute plan, budget-renewal headers |
| `zai` | `https://api.z.ai`, `/api/coding/paas/v4` | Required bearer key; discovered/static models | Coding-plan quota monitoring; override API path for ordinary API use |
| `kimi-coding` | `https://api.kimi.com`, `/coding/v1` | Required bearer key; discovered/static models | Coding-plan allowance windows |
| `minimax` | `https://api.minimax.io`, `/v1` | Required bearer key; discovered/static models | Token-plan remaining usage; subscription mode optional |
| `deepseek` | `https://api.deepseek.com`, `/v1` | Required bearer key; discovered/static models | Native multi-currency account balances |
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
  own clients; no redirect credential forwarding. Billing-only credentials never
  reach inference. OpenRouter uses an ordinary key for credits unless a separate
  management key is configured.
- Laya typed decisions use `jev_compatible`; a generative model emitting the
  classifier JSON contract uses the separate `model`/`laya` classifier types.
- Model catalog metadata merges bundled values, live authoritative values, then
  operator overrides. Unknown tool/vision/endpoint support is never guessed.
  Failed discovery retains last-known-good models marked stale or uses bundled
  fallback where available. The catalog refreshes on requests after one minute
  and after successful console login. An empty setup can run with only `smart`;
  inference still needs an eligible physical model.

## Costs, usage and remaining allowances

All supported providers expose prices, persistent gateway usage, and available
provider data through model discovery and the console picker. Live catalog prices
win over the bundled base-rate snapshot; operator catalog overrides win over both.
The dated models.dev fallback covers text models on DeepSeek, MiniMax, Z AI,
OpenCode Zen, xAI and Anthropic, including custom provider IDs and aliases. Prices
are estimates: cached tokens, peak pricing, long-context tiers and non-token
charges can change the bill. Unknown models keep unknown prices. Go, Standard
Compute and Kimi Coding models have included subscription pricing; explicit
`tracking.mode: subscription` also marks included billing for compatible coding
plans; Anthropic OAuth tracking is informational alongside API inference pricing.

| Provider | Tracking behavior |
| --- | --- |
| OpenRouter | `/key` spend/caps and `/credits` account balance; ordinary key by default, separate management key when supplied; free request limits apply only to `:free` models, whose dollar balances/caps stay informational |
| OpenCode Go | Rolling, weekly and monthly `/zen/go/v1/usage` windows and resets |
| OpenCode Zen | Usage/explicitly denominated wallet fields when returned; public wallet balance is not exposed reliably; optional `tracking.usage_url` supports an authenticated private credits endpoint |
| Standard Compute | `x-sc-budget-renews-at` reset header; optional private usage endpoint for reported `budget_usd`, `used_usd`, `remaining_usd` and renewal time |
| Z AI | Session/weekly limits and informational web-tool allowance from `/api/monitor/usage/quota/limit` |
| Kimi Coding | `/coding/v1/usages` ratios, absolute allowances and independent resets |
| MiniMax | `/v1/token_plan/remains` session/weekly token-plan limits, including model-specific buckets; ordinary API accounts may report no subscription |
| DeepSeek | `/user/balance` currencies and granted/purchased components kept separate; availability flag is authoritative |
| xAI | Separate management key plus `tracking.team_id`; prepaid balance and best-effort calendar UTC dollar spend; inference-key rate limits come from headers |
| Anthropic API | Optional admin key for paginated organization token usage/cost reports; request/token allowance/reset headers; organization totals are labeled separately from gateway per-key usage |
| Anthropic subscription | `tracking.mode: subscription` and an explicit OAuth tracking token read `/api/oauth/usage`; model-scoped windows and extra usage are retained as informational account data; API inference keeps its API rates and header limits |
| Nous | OAuth `/api/oauth/account`: subscription, purchased, rollover and total usable USD credits, monthly gauge when its denominator is valid, and renewal; plan gauges are informational because free models/top-up credit use can continue |
| Codex | ChatGPT `/backend-api/wham/usage` or public `/api/codex/usage`; quota/reset windows, credit balance/unlimited state, model-scoped limits and banked reset credits; headers supplement polling |

Missing credentials/endpoints/fields produce an explicit unavailable reason,
not an invented zero or unlimited allowance. Optional spend failures preserve
available balances. Billing reads are read-only, reject redirects and share an
account cache with a ten-second deadline. Picker requests warm accounts
concurrently. Independent model-scoped/informational windows do not exhaust
unrelated models. Reset credits are displayed; Torii never redeems them.

These snapshots appear in [model discovery](api.md#model-cost-usage-and-provider-quota-metadata)
and console selectors. Provider balances are separate from operator budgets.
Snapshots are in memory and refresh on demand; gateway accounting survives
restarts. External harnesses must consume the discovery metadata in their picker.

Protocol research and dated upstream sources are in
[Development provider notes](../development/provider-notes.md). Those dated
observations are not promises about current account tiers or upstream availability.

Codex quota polling uses the account-bound `/backend-api/wham/usage` endpoint, including primary and secondary window percentages and reset times. Failed polling reports unavailable telemetry; it does not invent a remaining allowance.

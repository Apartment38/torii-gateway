# Configuration reference

Settings and their revisions are stored in SQLite. Use `gateway config import --config PATH --database PATH` once to migrate a legacy YAML installation before starting this version. Import refuses an established settings document and keeps existing credential references and model IDs. Normal commands use `--database` or `TORII_DB_PATH` (default `/data/torii.db`), never a YAML file. Fresh databases use an empty provider list and first-run administrator setup. `serve --listen` / `TORII_LISTEN` overrides the listener; listener changes require restart. The master key still comes from `TORII_DB_KEY_FILE` / `TORII_DB_KEY`.

`router.catalog_overrides` stores the same descriptor array previously read from `router.catalog_file`; import reads and embeds that file. Optional CA files remain external resources. Configuration mutations validate a candidate runtime and atomically commit its settings, revision, and submitted encrypted credentials before publishing it. Failed changes preserve persisted and active settings.

The import and configuration-management interfaces accept the schema below. Unknown fields and multiple documents are rejected. Normal startup loads the stored settings document from SQLite.
The [complete example](../../config.example.yaml) is a starting point; a native
installation must change its `/data/torii.db` path to writable local storage.
[Console](../guides/console.md) saves use these same validation rules.

Validate with `./bin/gateway config validate --config ./config.yaml`. This checks
syntax and supported combinations, not provider connectivity or stored secret
availability. Credential placeholders below are illustrative, not usable IDs.
Durations use Go syntax, such as `500ms`, `30s`, `10m`, `24h`; `7d` is invalid.
Optional defaults refer to code defaults when omitted, not enabled example values.

## Server and state

| Field | Type/default | Rules and purpose |
| --- | --- | --- |
| `server.listen` | Required string | TCP `host:port`; `127.0.0.1:8080` local, `0.0.0.0:8080` inside container |
| `state.database_path` | String, `/data/torii.db` | Legacy import bootstrap path; normal commands select `--database`, then `TORII_DB_PATH`, then `/data/torii.db`; SQLite URIs/in-memory paths are not supported |
| `providers` | List, empty allowed | Unique provider IDs; inference needs an eligible physical model |

Listener changes require a restart; database changes require a startup-option change and restart. Master keys,
administrator login, and gateway API keys are never YAML settings.
`server.authentication` and `state.directory` are obsolete and rejected.

## Providers

| Field | Type/default | Rules and purpose |
| --- | --- | --- |
| `id` | Required string | Unique `[a-z0-9][a-z0-9._-]*`; becomes public model prefix |
| `type` | Required string | One of the types in [Provider support](providers.md) |
| `base_url` | Required HTTP(S) URL | No userinfo/query/fragment; path prefix permitted except origin-locked profiles |
| `api_path` | String, `/v1` compatible; `/v1beta/openai` Gemini | Absolute path without query/fragment; profile restrictions apply |
| `portal_url` | String, `https://portal.nousresearch.com` | Nous HTTPS Portal origin/path |
| `client_id` | String, `hermes-agent` | Nous device-login client identifier |
| `bearer_token` | Secret reference, absent | Required for OpenAI/OpenRouter/Gemini/xAI/Anthropic/hosted Jev; optional generic/compatible Jev; not subscription OAuth |
| `management_token` | Secret reference, absent | OpenRouter-only separate key for credits |
| `headers` | Map of secret references, empty | Compatible custom headers; reserved routing/auth/hop-by-hop headers forbidden |
| `tls.ca_file` | String, system roots | Compatible provider custom PEM trust file; path readable by gateway |
| `tls.server_name` | String, URL host | Compatible provider certificate server-name override; TLS section must set at least one field |
| `models` | String list, empty | Static/fallback upstream IDs; nonempty unique IDs; required when discovery disabled |
| `discover_models` | Boolean, true | Codex always discovers; its configured models are fallback only |
| `transport` | String, implicit profile transport | Compatible profiles permit only `openai_compatible`; subscriptions/native adapters choose transport themselves |
| `organization` / `project` | Strings, absent | OpenAI profile headers; no newlines |
| `model_rewrites` | Alias → upstream string map, empty | Compatible public aliases; distinct nonempty target per alias |
| `connect_timeout` | Positive duration, transport default | Connection deadline for compatible clients |
| `request_timeout` | Positive duration, `5m` Ollama; no completion timeout for compatible profiles | Whole inference HTTP timeout, including loading and reading JSON/SSE; Ollama accepts overrides such as `10m`. Zero/negative/invalid durations are rejected; omit to use the profile default |
| `discovery_timeout` | Positive duration, 10s compatible | Separate catalog-discovery deadline |

Persisted secrets must have exactly the stored reference form:

```yaml
# Provider excerpt: replace ID with an existing stored credential.
bearer_token: {credential: ID}
headers:
  X-Custom-Key: {credential: ANOTHER_ID}
```

Authenticated transient console values are encrypted before saving. Persisted
`value`, `env`, or `file` credential references fail loading. Custom headers may
not override Authorization, Host, Content-Type, Accept, Connection,
Proxy-Connection, Transfer-Encoding, Upgrade, or X-Request-Id.

### Profile restrictions

- Ollama accepts its ID/type/base URL without generic provider tuning.
- `openai` locks to `https://api.openai.com`, requires its bearer key, always
  discovers, and disallows static models, custom headers/TLS/rewrites/transport.
- `openai-codex` requires HTTPS; OAuth is managed in SQLite, not bearer/custom
  headers/TLS. `models` is an optional discovery fallback, not an allowlist.
- `nous` requires HTTPS and provider-managed authentication/transport; it
  disallows bearer/custom headers/TLS/rewrites/organization/project options.
- `anthropic` locks to `https://api.anthropic.com`, requires its key, and disallows
  custom headers/TLS/api_path/transport/organization/project/rewrites.
- `openrouter`, `gemini`, and `xai` use compatible options with required keys.
- `jev` locks to the HTTPS `api.typesafe.ai` host and requires a key;
  `jev_compatible` supports alternate origins. Both are classifier-only.

## Router

| Field under `router` | Type/default | Rules and purpose |
| --- | --- | --- |
| `model_name` | String, `smart` | Only `smart` accepted |
| `catalog_file` | String, absent | Legacy import only: strict JSON descriptor array imported into `catalog_overrides` |
| `catalog_overrides` | Descriptor array, empty | Persistent provider-qualified metadata and pricing overrides, applied after bundled/live metadata |
| `allow_paid_models` | Boolean, true | Hard gate for smart paid candidates; explicit model requests bypass smart |
| `classifier.type` | String, absent | `disabled`, `model`, `laya`, `embedding`, `jev`; absent means no configured classifier |
| `classifier.model` | String, conditional | Physical `provider/model` for connected `model`/`laya`; provider-local model for typed/embedding transport; never `smart` |
| `classifier.provider` | String, conditional | Required for embedding and Jev provider transport |
| `classifier.transport` | String, direct Jev default | Jev `direct` or `provider`; provider mode needs a DecisionProvider |
| `classifier.base_url` | HTTPS URL, conditional | Required for direct Jev; forbidden with provider transport |
| `classifier.api_key` | Secret reference, conditional | Required for direct Jev; provider transport uses its provider key |
| `classifier.on_failure` | String, `capability_fallback` | `capability_fallback` or `fail` |
| `classifier.timeout` | Duration, `10s` | 1ms–5m inclusive |
| `classifier.minimum_confidence` | Number, 0 | Finite 0–1; below threshold uses on_failure |
| `classifier.questions` | Map, built-in rubrics | Typed Jev question instruction/criteria overrides; allowed with `jev`/`disabled` |
| `classifier.noul_thresholds` | Map, each 0.5 | Per yes/no question threshold in 0–1, equality accepted |
| `sessions.inactivity_timeout` | Positive duration, `30m` | In-memory affinity expiry |
| `sessions.on_model_failure` | String, `reroute` | `fail` or `reroute`; no replay after downstream bytes/tool-pin loss |
| `quota.cache_ttl` | Positive duration, `60s` | Provider quota snapshot/failure lifetime; account models share snapshots, and known reset times shorten cache life |
| `quota.allowances` | Model → number map, empty | Finite nonnegative estimated-cost allowances in USD, compared with accumulated usage cost |
| `selection.context_utilization_limit` | Number, 0.85 | 0–1; zero/omitted uses default; output reserve included |
| `selection.default_expected_output_tokens` | Integer, 1024 | Positive values override estimate; zero/nonpositive uses default |
| `selection.location_preference` | String, `neutral` | `neutral`, `local`, `cloud` |
| `selection.minimum_quality` | Complexity → number map | Default `low: 0.55`, `medium: 0.70`, `high: 0.82`; finite 0–1. Supply all needed complexity labels when replacing map |

Disabled classifiers must not specify connection/model fields; tuning can remain.
For embedding type, `provider` identifies an embedding-capable provider and
`model` its upstream embedding model. Connected types require a generative model
implementing the shared JSON classification contract, not just any chat model.

### Typed question overrides

Question names: `task_type`, `complexity`, `tools`, `vision`, `reasoning`,
`structured_output`, `latency_sensitive`. Task labels: `general`, `conversation`,
`coding`, `sysadmin`, `reasoning`, `math`, `research`, `summarization`, `extraction`,
`creative`, `translation`. Complexity labels: `low`, `medium`, `high`.
Yes/no criteria use quoted `"true"` and `"false"`; thresholds only address the
five yes/no questions. Overrides merge with defaults without removing labels.
Each instruction/criterion is at most 4,000 characters; serialized overrides
are at most 32 KiB. Unknown labels are rejected.

```yaml
# Router excerpt; laya must be an existing compatible decision provider.
router:
  classifier:
    type: jev
    transport: provider
    provider: laya
    model: english
    on_failure: capability_fallback
    timeout: 3s
    minimum_confidence: 0.4
    questions:
      task_type:
        instructions: Which task type best describes this request?
        criteria:
          coding: Developing, debugging, or reviewing software
      tools:
        criteria:
          "true": Requires external tool or function actions
          "false": Can be answered directly
    noul_thresholds:
      tools: 0.7
```

See [Smart routing](../guides/smart-routing.md) for setup and session behavior.

## Intelligence

| Field under `intelligence` | Type/default | Rules and purpose |
| --- | --- | --- |
| `enabled` | Boolean, false | Enables worker/cache APIs; does not enable individual sources |
| `refresh_interval` | Positive duration, `24h` | Periodic imports |
| `http_timeout` | Positive duration, `30s` | Individual upstream HTTP deadline |
| `source_timeout` | Positive duration, `30m` | Complete source import deadline |
| `request_interval` | Positive duration, `1s` | Minimum shared per-host request start spacing |
| `retries` | Integer, 2 | 0–5; network/server/429 retries |
| `stale_after` | Positive duration, `168h` | Retrieval freshness limit |
| `max_age` | Positive duration, `4320h` | Evaluation age limit |
| `freshness_half_life` | Positive duration, `2160h` | Evidence age decay |
| `sources` | List, empty | At most 32; valid unique source IDs |
| `identities` | List, catalog-derived | Explicit model grouping and aliases |
| `aggregation.source_weights` | Source ID → number | Finite nonnegative weights, default 1 per source; zero disables contribution |
| `aggregation.benchmark_weights` | Benchmark ID → number | Finite nonnegative, default 1 |
| `aggregation.overall_weights` | Category → number | Available categories default 1, preference 0; no `overall` recursion |
| `routing.capabilities` | Category → number | Nonnegative category utility weights; all zero/absent disables this term |
| `routing.task_weights` | Task → category → number | Task-specific capability utility weights |
| `routing.price`, `latency`, `ttft`, `throughput`, `context` | Numbers, 0 | Finite nonnegative utility weights |
| `routing.cost_reference_usd` | Number, 0.01 effective | Positive request-cost normalization scale; zero uses default |
| `routing.latency_reference_seconds` | Number, 2 effective | Positive latency normalization scale; zero uses default |
| `routing.throughput_reference` | Number, 100 effective | Positive tokens/second normalization scale; zero uses default |
| `routing.missing_score` | Number, 0.5 | 0–1 for unknown utility components, not an observed score |

Categories: `general_intelligence`, `coding`, `agentic`, `reasoning`, `math`,
`knowledge`, `instruction_following`, `multimodal`, `preference`; routing also
permits `overall`. Import mappings can describe `operational` measurements.
No ranking change occurs without enabled weights and relevant usable evidence.

### Source settings

| Field in each `sources` entry | Type/default | Rules and purpose |
| --- | --- | --- |
| `id` | Required string | Valid unique ID; aliases/source weights use this ID |
| `type` | Required string | `artificial_analysis`, `benchlm`, `huggingface`, `lmarena`, `livebench` |
| `enabled` | Boolean, false | Contact this source only when intelligence also enabled |
| `api_key` | Secret reference, absent | Required for enabled Artificial Analysis; optional HF token for public dataset sources |
| `url` | URL, built-in preset | Optional trusted compatible endpoint; no embedded credentials/secret queries |
| `transport` | String, `auto` | Dataset sources only: `auto`, `rows`, `parquet`; built-in Arena/LiveBench auto uses native Parquet, generic/custom uses rows |
| `datasets` | List, preset where available | At most eight selected datasets; generic HF requires metric mappings after defaults applied |
| `max_rows` | Integer, 100000 | 1–100000; zero/omitted uses default; oversize import rejected |
| `include_estimated` | Boolean, false | BenchLM estimated evidence eligible for normalization when true |
| `metrics` | Column/benchmark → mapping map | Source-specific declared scales/categories |

Presets live in `internal/intelligence/presets.json`; explicit fields override
them. Parquet imports require native declared config/split files and do not
fall back to rows on failure. Download and row-group size limits are 64 MiB;
the logical cache limit is 128 MiB. See [Benchmarks](../guides/benchmarks.md).

### Dataset and metric fields

Each dataset has required `dataset`, `config`, `split`; optional `model_field`,
`date_field`, `version`, and `metrics`. Column paths may use dot notation.
`model_field`/`date_field` select published columns; inspect the dataset schema
and supply them for a custom dataset. Missing dates remain explicitly limited.
Each metrics map key identifies a published column. Mapping fields are `category`,
optional canonical `metric`, `min` (default 0), required `max` for scores,
`lower_is_better` (default false), and `unit`. Numeric scores need finite bounds
with `max > min`; no scale guessing. Operational mappings require the units below.

| Operational `metric` | Required `unit` |
| --- | --- |
| `input_price`, `output_price` | `USD_per_million_tokens` |
| `latency`, `ttft` | `seconds` |
| `throughput` | `tokens_per_second` |
| `context_length` | `tokens` |

```yaml
# Intelligence excerpt for a custom dataset; replace publisher/columns.
intelligence:
  enabled: true
  sources:
    - id: custom
      type: huggingface
      enabled: true
      datasets:
        - dataset: organization/benchmark-results
          config: default
          split: train
          model_field: model_id
          date_field: evaluation_date
          version: evaluation-v2
          metrics:
            pass_at_1: {category: coding, min: 0, max: 1}
            math_accuracy: {category: math, min: 0, max: 100}
            runtime_seconds: {category: operational, metric: latency, unit: seconds}
```

### Identity fields

Each identity has a nonempty unique `canonical_id`, optional `provider_ids`
(public model IDs, each assigned once), `aliases` (external IDs), and `source_ids`
(source ID → list of external IDs). Source-specific aliases take precedence.
Only group IDs that represent the same evaluated model; collisions remain
ambiguous and do not route. Unavailable provider IDs cannot grant a match.

```yaml
# Merge under intelligence; replace IDs with actual equal-model variants.
identities:
  - canonical_id: evaluated-model
    provider_ids: [local/evaluated-model, private/evaluated-model]
    aliases: [Published Model Name]
    source_ids:
      custom: [published-model-id]
```

See [Intelligence internals](../development/intelligence.md) for weighting and
freshness equations, raw-data retention, identity safeguards, and provenance.

## Model controls

`model_enabled` maps provider-qualified model IDs to booleans; omitted IDs are enabled. Disabled IDs survive rediscovery/restarts, disappear from inference discovery, and cannot be dispatched directly or selected by Smart. Change an active classifier before disabling its model. `providers[].enabled` defaults to true. `providers[].treat_models_as_free` defaults to true for native Ollama and false otherwise. When true, routing and quotas use explicit zero estimated cost; false retains individual catalog pricing overrides. Locality alone does not make a model free or override its prices.

## Operator budgets

All budgets default to disabled. `budgets.gateway` applies to every client together; `budgets.keys` maps stable credential IDs (from the API key list) to per-key limits. Each has `daily` and `monthly` objects with `requests`, `tokens` (input plus output), and `estimated_cost` (USD). Nonnegative zero disables a limit; request/token limits are integers at most 10^15. UTC midnight and the first of each UTC month reset periods.

Example settings excerpt:

```yaml
budgets:
  gateway:
    monthly: {estimated_cost: 100}
  keys:
    gateway/api-key:
      daily: {requests: 1000, tokens: 1000000}
```

Reservations count one client request and every upstream attempt, including classifier calls and retries. Text input reserves one token per request-body byte; multimodal input conservatively reserves the known context capacity. An explicit positive output limit is reserved; omitted limits reserve `max_output_tokens` from a reliable catalog override or the known context capacity as an upper bound. Token/cost budgets reject requests without a supported bound. Embeddings reserve zero generation tokens. Classifier calls reserve supported output capacity because typed-decision transports can report upstream output usage too; token/cost budgets require a reliable bound for those calls. Missing/cancelled usage retains estimates; complete reported JSON/SSE usage replaces them. Embeddings have no generation tokens, so reported input or total tokens can settle their usage. For generative endpoints, a total plus one component determines the other; totals without a reliable split or inconsistent totals retain conservative monetary reservations. Streams also require a completion marker and usage after the last generated content; truncated streams and stale initial snapshots retain estimates. Unknown prices remain unknown, and monetary limits reject them unless the provider is marked free or reliable pricing overrides exist. Outstanding reservations and committed totals survive restarts. Storage failures deny limited requests; unlimited inference can continue with an accounting warning. These figures are operator accounting, not provider billing.

## Activity and structured logs

`activity.retention_days` defaults to 7 (1–3650); `activity.max_bytes` defaults to 104857600 (100 MiB; minimum 8192). The cap includes serialized metadata plus a per-row storage allowance; deleted SQLite pages are reused. Retention runs at startup, on writes and every minute. `activity.log_file` optionally duplicates JSON stdout logs to a rotating file. `log_max_bytes` defaults to 10485760 (minimum 4096) and `log_backups` to 3 (1–20). The log directory must exist and be writable. File logging and listener changes require restart; a successful configuration response reports `restart_required`. Activity retains request metadata, never prompts, responses, credential values or authorization headers.

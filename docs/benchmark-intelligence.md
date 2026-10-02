# Benchmark ingestion and model intelligence

Benchmark intelligence is optional. It updates in the background and enriches
the existing smart selector; explicit `provider/model` requests do not change.
No benchmark HTTP calls, disk reads, or aggregation occur during inference.
Existing routing remains in effect when disabled, when weights are zero, or
when no usable evidence exists.

## Architecture

`internal/intelligence` contains a registered `Source` interface, independent
adapters, a shared identity resolver, a pure aggregation function, and a manager
with a context-aware `SnapshotStore` interface backed by the existing application
SQLite database. It reuses `catalog.Descriptor` for known models and
`state.database_path` for storage. `config.SecretRef` remains the only
credential configuration type. `internal/router` owns ranking weights and policy;
the benchmark subsystem does not choose models or change capability flags.

The manager retains all imported observations, including unnormalized and
unmatched measurements, with original score, benchmark, source, model identifier,
unit, version, evaluation date if published, retrieval timestamp, and metadata.
Missing scores are absent, never manufactured as zero. A new adapter implements
`Source.Fetch`, registers a factory in `Builtins`, maps categories, and adds
fixture tests; aggregation and routing need no source-specific changes.

## Sources

| Type | Published interface | Notes |
| --- | --- | --- |
| `artificial_analysis` | [Official data API](https://artificialanalysis.ai/data-api/docs), `/api/v2/language/models/free` | API key via `x-api-key`; pagination; capability indices, prices and performance. Set `url` to the Pro endpoint if your account supports it. The legacy `/api/v2/data/llms/models` JSON is also accepted. Credit Artificial Analysis and observe its data terms. |
| `benchlm` | [Published model JSON](https://benchlm.ai/data), `https://benchlm.ai/data/models.json` | Raw benchmarks plus published composite/category scores and provenance. The leaderboard JSON export is also accepted via `url`. CC BY-NC 4.0: commercial use requires a BenchLM licence. Estimates stay raw-only unless `include_estimated: true`. |
| `huggingface` | [Dataset Viewer rows API](https://huggingface.co/docs/dataset-viewer/rows) | Configure dataset, subset, split, model/date columns and each score's scale/category. No single leaderboard is hard-coded. Tokens use bearer authentication when needed. Dataset licences remain the operator's responsibility. |
| `lmarena` | [Arena's official leaderboard dataset](https://huggingface.co/datasets/lmarena-ai/leaderboard-dataset) through Dataset Viewer | Defaults to `text_style_control`, `latest`. Every Arena subset remains human-preference evidence, including coding/vision categories. Confidence bounds and vote counts are retained. |
| `livebench` | [Official published judgments](https://huggingface.co/datasets/livebench/model_judgment) through Dataset Viewer | Defaults to `default`, `leaderboard`; per-question scores, tasks and evaluation timestamps are retained. No HTML scraping. The artifact can lag the current website; old evidence is excluded by `max_age`. |

Sources are explicitly enabled; no vendor is contacted by default. All support
an optional `url` for a trusted mirror or compatible published endpoint. URLs
cannot contain embedded credentials or secret query parameters. Credentials
are never forwarded across redirects. Use HTTPS for remote authenticated sources.

## Configuration

Add this top-level section to `config.yaml`, or edit the same object as JSON on
the console's **Benchmarks** page and select **Save & reload**:

```yaml
intelligence:
  enabled: true
  refresh_interval: 24h
  http_timeout: 30s
  source_timeout: 10m
  retries: 2
  stale_after: 168h
  max_age: 4320h
  freshness_half_life: 2160h
  sources:
    - id: aa
      type: artificial_analysis
      enabled: true
      api_key: {credential: YOUR_STORED_CREDENTIAL_ID}
    - id: arena
      type: lmarena
      enabled: true
  aggregation:
    source_weights: {aa: 1, arena: 1}
    benchmark_weights: {aa/artificial_analysis_coding_index: 2}
    overall_weights:
      general_intelligence: 1
      coding: 1
      agentic: 1
      reasoning: 1
      math: 1
      knowledge: 1
      instruction_following: 1
      multimodal: 1
      preference: 0
  routing:
    capabilities: {overall: 1}
    task_weights:
      coding: {coding: 3, agentic: 1}
      reasoning: {reasoning: 3, math: 1}
      research: {knowledge: 3, reasoning: 1}
    price: 0.2
    latency: 0.1
    ttft: 0.1
    throughput: 0.1
    context: 0.1
    cost_reference_usd: 0.01
    latency_reference_seconds: 2
    throughput_reference: 100
    missing_score: 0.5
```

The durations above are defaults (Go duration syntax: hours, not `7d`); retries
default to two and are limited to five. Refreshes retry network/server errors
and 429 with a bounded delay, not authentication or parsing failures. Individual
HTTP requests time out separately from the complete source import. Sources run
independently and concurrently. A failed source retains its previous successful
dataset, with a failure status; successful sources still update.

Source keys use `{credential: ID}` references from the authenticated provider
credential API, or transient `{value: "key"}` input encrypted on console save.
The Credentials page's gateway client keys must not be used as source keys.
In the console, check **Benchmarks → Enable benchmark intelligence** to reveal
the individual source toggles and settings. Enable the sources you want, configure
their options, and click **Save & reload** before **Refresh benchmarks**. The
master toggle does not enable individual sources automatically. Unsaved source
changes must be saved before refreshing; ingestion always uses saved settings.

Each source card shows its status, observation count, last successful update and
cache age. Artificial Analysis requires a source API key. Other adapters accept
an optional key/token for their configured endpoint. Entering a key replaces it
on save; leaving the field blank keeps the encrypted credential reference.
**Reveal** only reveals a newly entered value, never a previously saved key.
Keys are held only in memory until submitted and are cleared on save/sign-out.

Source-specific controls include BenchLM's estimated-evidence toggle and dataset
ID, configuration, split, model/date columns and version for Hugging Face,
LMArena and LiveBench. Hugging Face requires at least one numeric metric mapping
per dataset: specify the upstream column, capability category, score range and
whether lower is better. Operational mappings use explicit canonical metrics
and units. Arena ratings always remain preference data; LiveBench uses the
structured judgment schema described below. Optional category/scale overrides
are available for Artificial Analysis and BenchLM. Refresh interval, timeouts,
retries and freshness thresholds have their own advanced controls. Existing
aliases, aggregation and routing weights are preserved when saving these forms.

The **Current model benchmark data** table loads cached data on page entry.
Choose aggregate capability scores and operational measurements, or raw source
observations with original/normalized scores, dates, versions and match status.
Raw results include unmatched models and retained disabled-source data; only
enabled, matched evidence contributes to aggregates. Both views are filterable
and paginated. Missing values are unknown, not zero, and preference stays separate.

**Refresh activity** streams real fetch/parse, persistence, publication and failure
milestones. The progress bar counts completed sources, including failed sources;
it is not a download-byte percentage. Each source is published only after its
validated data is saved atomically, and the table reloads after each publication
or failure without waiting for slower sources. Closing the connection cancels
the manual refresh; already published results remain available. Scheduled
refreshes also publish incrementally, but their activity is not attached to
this manual-refresh stream.
Authenticated direct submissions are encrypted before saving; configuration
reads return references only. YAML value/env/file credentials are not supported.
See [Encrypted credential storage](credential-storage.md).

## Importing a Hugging Face leaderboard

Choose a dataset with published per-model results and available Parquet exports
(the Dataset Viewer requirement), not a dataset containing only test questions.
Replace the illustrative dataset/field names below with its real schema:

```yaml
    - id: hf-code
      type: huggingface
      enabled: true
      max_rows: 100000
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
            runtime_seconds:
              category: operational
              metric: latency
              unit: seconds
```

Nested columns can use dot paths. Null/missing measurements are skipped;
non-numeric values, out-of-range scores, truncated cells, missing pages, changed
row counts, and exceeded limits reject the update instead of publishing a
partial leaderboard. Pages use at most eight concurrent requests; `max_rows`
defaults to 100,000 and the importer rejects larger datasets instead of silently
truncating them. At most eight datasets and 32 sources may be configured.

Operational mapping names are `input_price`, `output_price` (unit
`USD_per_million_tokens`), `latency`, `ttft` (`seconds`), `throughput`
(`tokens_per_second`), and `context_length` (`tokens`). Convert milliseconds or
non-USD prices in the published dataset before importing; no exchange-rate or
unit guessing occurs. Source-specific `metrics` can override declared score
scales/categories for Artificial Analysis and BenchLM. BenchLM raw benchmark
keys are `category.benchmark`, such as `coding.sweVerified`; unknown scales
remain inspectable but do not contribute to aggregates without a mapping.

## Model identity and aliases

Each live catalog ID is a canonical identity by default. Matching recognizes
public IDs, upstream IDs and explicit aliases, normalizing case, whitespace and
underscores without removing version dates, decimal points, quantization, or
reasoning-effort suffixes. It does not fuzzy-match model families. An upstream
ID shared by several providers is ambiguous unless explicitly grouped:

```yaml
  identities:
    - canonical_id: gpt-5.3-codex
      provider_ids:
        - openai-codex/gpt-5.3-codex
        - private/gpt-5.3-codex
      aliases: [GPT-5.3-Codex]
      source_ids:
        aa: [gpt-53-codex]
        arena: [gpt-5.3-codex]
```

Only include provider IDs that actually represent the same evaluated model.
Source IDs are scoped to the configured source **id**, not its type. They take
precedence over unscoped aliases. Aliases for unavailable catalog models are
not accepted. Colliding aliases remain ambiguous and are logged/inspectable;
duplicate physical-ID assignments are rejected by configuration validation.
Effort-labelled Artificial Analysis results retain the label instead of being
attached silently to the unqualified model. New model IDs or credentials that
change the provider catalog trigger rematching of cached evidence, not a
benchmark fetch.

No universal alias list is guessed or downloaded. Inspect `/unmatched`, add
explicit identities, then save/reload or inspect the snapshot to rematch the
cache. An identifier with no match is retained, never discarded.

## Aggregation and routing

The isolated aggregation algorithm is a configurable hierarchical weighted mean:

1. Keep the latest observation for each source/model/benchmark/category and
   question ID, retaining all raw observations separately. Average per-question
   results before treating a benchmark as one signal.
2. Normalize only declared bounded scores as `(score - min) / (max - min)`;
   `lower_is_better` reverses the scale. Unknown scales remain raw-only. Arena
   ratings use a separately labelled logistic preference transform, reference
   1000 and scale 400; Agent Arena IPS scores stay unnormalized unless an explicit
   dataset `score` min/max is configured. They are not objective accuracy.
3. Exclude observations older than `max_age` and retrievals older than
   `stale_after`. Weight usable evidence by `2^(-age / freshness_half_life)`;
   average benchmarks within each source/category, then average sources. More
   rows from one source do not give it more influence. Source and benchmark
   weights default to one; zero disables their contribution.
4. Derive `overall` from available categories and `overall_weights`. Preference
   defaults to weight zero and joins overall only through an explicit weight.
   No score is produced for a category with no usable measurements.

Operational measurements use the latest non-expired published observation,
with deterministic source-ID tie-breaking. They do not enter objective quality
aggregates. Unknown evaluation dates use retrieval time as an explicitly
limited age basis; model release and website build dates are not silently used
as evaluation dates. Raw timestamps and source build/version metadata remain
inspectable.

The selector's existing availability, quota, verified endpoint/features,
quality floors, and context checks remain hard gates. Benchmark scores never
grant tool or vision support or overwrite existing static quality floors.
When routing weights are enabled, intelligence may fill unknown pricing/context
metadata; a smaller reported context limit tightens, never relaxes, an existing
limit. Known local/free/subscription pricing remains authoritative.

Eligible candidates get a weighted utility from task capability scores, lower
estimated request cost, lower latency/TTFT, higher throughput and context
headroom. Cost and lower-latency utilities are `reference / (reference + value)`;
throughput uses `value / (reference + value)`. Observed gateway latency takes
precedence over external latency. Missing measurements use `missing_score`
(default 0.5), not a fake observed zero. With no relevant usable intelligence,
ranking reverts to existing cost/quota/location/latency/context/ID ordering.
Ties always use that existing deterministic ordering. Existing session affinity
remains intact; a benchmark refresh does not switch an ongoing pinned session.

## Cache, management and refresh

Benchmark data lives in the existing `state.database_path` database (default
`/data/torii.db`), in `benchmark_sources`, `benchmark_datasets`,
`benchmark_observations`, and `benchmark_storage` tables. No new volume or
configuration setting is needed. Benchmark observations and provenance are public
data and **not encrypted**; credential payloads remain encrypted with AES-256-GCM.
Only the latest published dataset per source is retained, including duplicate,
unnormalized and unmatched observations. Aliases, aggregate scores and freshness
are recalculated in memory, not stored as another authoritative dataset.

The benchmark store owns a separate connection pool using WAL, foreign keys and
a 5-second busy timeout. It validates and serializes before writing, stages at
most 250 observations per transaction, verifies staged datasets, then atomically
publishes source pointers/statuses and the update timestamp. Refresh publishes
one completed source at a time, independently of slower sources. Unchanged
evidence is reused. Failed staging/publication leaves previous datasets and the
routing snapshot intact. Abandoned and superseded datasets are cleaned in bounded
batches at open/refresh. Credential reads can continue while benchmark writes
are active; competing writes may wait for SQLite's write lock. Pools are closed
when handlers are replaced or discarded. The logical cache remains limited to
128 MiB, in addition to the existing upstream response and row limits.

A fresh database starts with an empty benchmark store. There are no JSON-cache
imports. Corrupt cached observations are logged and ignored so inference
fallback remains available.

The database-only baseline is schema 4. See
[runtime state and releases](runtime-state.md) for the one-time reset, future
database-preserving upgrades, consistent backups and rollback instructions.

Cached data stays in use during refresh. Freshness is recalculated in the
background at least once a minute (or the shorter configured refresh interval),
and cache-only routing lookups reject expired derived views immediately.
Stopped/replaced handlers cancel their workers and manual refreshes before a
new handler writes the same database. One persistent database volume belongs
to one gateway process: do not run replicas against the same database. Staging
coordination is process-local, not a multi-process benchmark writer protocol.

Management routes use the gateway's existing API authentication:

| Endpoint | Result |
| --- | --- |
| `GET /gateway/v1/intelligence` | Full snapshot, source observations, model scores and unmatched IDs |
| `GET /gateway/v1/intelligence/sources` | Source status, last attempt/success, age, staleness, safe errors and metadata |
| `GET /gateway/v1/intelligence/models` | Models with aggregates and operational metadata |
| `GET /gateway/v1/intelligence/observations` | Paginated raw table rows including unmatched IDs; `offset`, `limit` (1–500, default 100), optional `q` filter; metadata omitted, original observations retained |
| `GET /gateway/v1/intelligence/models/{id}` | Raw observations, provenance and aggregates for a canonical or physical ID |
| `GET /gateway/v1/intelligence/unmatched` | Unmatched/ambiguous source IDs and candidate canonical IDs |
| `POST /gateway/v1/intelligence/refresh` | Refresh all enabled sources; summary includes `partial_failure` and per-source status |

With `Accept: text/event-stream`, the refresh endpoint returns `progress` events
(`phase`, `source`, `completed`, `total`, `failed`, `observations`, `message`),
15-second heartbeats, and a final `result` summary. Existing JSON callers remain
compatible. Proxies must allow streaming and sufficiently long read timeouts;
the gateway sets `X-Accel-Buffering: no`. Source/model summaries include counts
without transmitting every raw observation.

Run a manual refresh from the console or against the running gateway:

```sh
./gateway benchmarks refresh --config ./config.yaml
podman compose exec torii-gateway /gateway benchmarks refresh --config /config/config.yaml
# Use a reverse-proxy origin when needed:
./gateway benchmarks refresh --config ./config.yaml --url https://gateway.example.com
```

The CLI loads the inference API key from the credential database and calls the running gateway rather
than creating a competing cache writer. A partial-source failure returns an
inspectable summary; check `partial_failure`, not only the command's exit code.
Disabled intelligence returns HTTP 503. New APIs do not add authentication to
the existing public console; keep it behind trusted access controls.

Refresh starts, successes, parsing/upstream failures, cache errors and matching
failures are structured `slog` events. Logs/status omit arbitrary upstream error
bodies and credentials. Upstream HTTP status and safe parsing stages are exposed.

## Limitations

- Source scores are evidence, not calibrated probabilities of successful task
  completion. Different evaluation settings, correlated benchmarks and provider
  aggregates can require operator weights. BenchLM includes aggregated evidence
  from other sources; reduce its weight if you also import those publishers.
- Missing evaluation dates cannot establish true benchmark age. Retrieval-only
  freshness is a fallback, not proof that a benchmark was rerun recently.
- Arena preference and task accuracy are deliberately separate; logistic Arena
  normalization is a configurable overall-policy input, not accuracy equivalence.
- LiveBench's structured public dataset can lag recent releases; stale rows
  stay inspectable but do not affect routing. No promise of website parity.
- Dataset Viewer availability, upstream schema changes, provider/API tiers and
  licences can affect ingestion. Tests use fixtures/mocks, not live credentials.
- Row-count checking cannot guarantee snapshot isolation if an upstream edits
  values without changing the row count. Prefer versioned/pinned published datasets.
- The cache keeps the current successful source import and all its observations,
  not an unbounded history of every periodic response. Logical snapshot size is capped at
  128 MiB; oversized replacements are rejected without deleting good data.
- Unresolved variants and aliases require explicit operator mapping; the API
  reports the actual unmatched list for that installation. No model aliases were
  asserted against a live subscription as part of this implementation.

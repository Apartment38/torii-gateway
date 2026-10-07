# Benchmark intelligence internals

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

## Retrieval, transport, and pacing

The durations above are defaults (Go duration syntax: hours, not `7d`); retries
default to two and are limited to five. Refreshes retry network/server errors
and 429, not authentication or parsing failures. Individual
HTTP requests time out separately from the complete source import. Sources run
independently and concurrently. A failed source retains its previous successful
dataset, with a failure status; successful sources still update.

LiveBench and Arena default to downloading the official native Parquet files,
not Dataset Viewer `/rows` pagination. Torii discovers the selected config/split
in the Hub repository manifest, pins its commit revision, and downloads every
matching shard before publishing any results. It never downloads Arena's complete
repository or historical `full` split when `latest` is selected. Downloads follow
at most five HTTPS redirects to trusted Hugging Face CDN hosts; bearer tokens
are stripped when leaving the Hub origin. Signed download URLs are never logged.

`transport` accepts `auto` (the default), `rows`, or `parquet` for dataset sources.
`auto` selects file downloads for the built-in LiveBench/Arena `/rows` URL,
including existing saved configurations. Custom URLs and generic Hugging Face
sources retain rows ingestion. Explicit `rows` preserves the legacy transport;
explicit `parquet` uses the Hub dataset manifest, independently of `url`, and
requires native Parquet files with declared config/split paths (or the standard
`data/<split>-*.parquet` layout for a default config). It does not depend on
Viewer-generated conversions. A file failure never triggers a pagination fallback.

File imports use temporary storage, a **64 MiB total download limit per source**,
the existing `max_rows` limit across its selected files, and batched decoding.
Each row group's declared uncompressed size is also limited to 64 MiB. Temporary
files are removed after success, failure or cancellation. Malformed/incomplete
files retain the previous SQLite observations; raw files are not durable state.
A Go Parquet decoder is included in the gateway, with no Python runtime or sidecar.

Benchmark requests share one in-flight lane per host/port with at least
`request_interval` between starts (default **1 second**). Discovery, file
requests and legacy rows ingestion use the same retry and quota handling.
A 429 pauses sources using that host, honors full `Retry-After` seconds or HTTP
dates, and temporarily slows subsequent requests. Torii reads structured
`RateLimit` remaining/reset fields, spaces requests across the advertised budget,
and waits for exhausted windows to reset. Without headers, 429 retries back off
from 30 seconds to 5 minutes with jitter; network/server failures use shorter
backoff. All waits are cancellable and bounded by the source deadline. Cooldowns
survive configuration reloads in the running process, not gateway restarts.

### Published-interface verification — 2026-10-07

The public native files were downloaded without authentication and decoded in Go:
LiveBench `default/leaderboard` contains 60,372 rows in a 737,444-byte file;
Arena `text_style_control/latest` contains 10,923 rows in a 606,130-byte file.
This replaces 604 and 110 Viewer page requests respectively with one file each,
plus manifest discovery and CDN redirects. See the official
[LiveBench metadata](https://huggingface.co/datasets/livebench/model_judgment/blob/main/README.md)
and [Arena metadata](https://huggingface.co/datasets/lmarena-ai/leaderboard-dataset/blob/main/README.md).

Hugging Face publishes [Hub rate limits](https://huggingface.co/docs/hub/rate-limits)
in five-minute windows (currently 500 anonymous API requests per IP and 3,000
resolver requests; free authenticated users have 1,000 and 5,000 respectively).
These are Hub quotas, **not a verified fixed Dataset Viewer ceiling**; anonymous
and free quotas may change with platform health. No fixed Viewer request limit
was found in the reviewed official documentation. Tokens are optional for these
public datasets, not a guarantee of unlimited requests. No separate LiveBench
or Arena API key is required for their published Hugging Face files.

The LiveBench repository reports its last update as **2025-04-07**; Arena's
repository was updated on **2026-10-07**. File downloads improve retrieval, not
freshness. Preserve actual evaluation dates; expired evidence remains inspectable
but cannot influence routing. Verify publication dates when comparing the dataset
with the current benchmark website.

The default complete-source timeout is **30 minutes** to allow large datasets
to import politely. Previously saved explicit `source_timeout` values remain
unchanged; if an installation still specifies `10m`, increase it to `30m` in
`config.yaml` for combined large imports. These protections reduce avoidable
429s, but cannot guarantee access if the upstream quota is exhausted by other
applications or the service is overloaded. Inference requests are unaffected.
Rate-limit pauses are logged safely and appear in manual refresh activity.
Startup uses a recently updated cached snapshot without repeating its imports;
newly enabled sources still trigger an initial import, and manual refresh remains
available regardless of cache age.


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
[backup and upgrade](../guides/backup-and-upgrade.md) for the one-time reset, future
database-preserving upgrades, consistent backups and rollback instructions.

Cached data stays in use during refresh. Freshness is recalculated in the
background at least once a minute (or the shorter configured refresh interval),
and cache-only routing lookups reject expired derived views immediately.
Stopped/replaced handlers cancel their workers and manual refreshes before a
new handler writes the same database. One persistent database volume belongs
to one gateway process: do not run replicas against the same database. Staging
coordination is process-local, not a multi-process benchmark writer protocol.


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

To run the opt-in combined public-file smoke test:

```sh
TORII_LIVE_BENCHMARK_SMOKE=1 go test ./internal/intelligence -run '^TestLivePublishedParquetBenchmarks$' -v
```

The test refreshes both sources through the manager, reports observation counts
and request counts, and rejects `/rows` requests. It uses no provider credentials.

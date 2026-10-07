# Use benchmark intelligence

Import public model evaluations to inspect scores and optionally inform smart
routing. You need administrator access; this feature is disabled by default.
Imported scores do not grant unverified tool, vision, or endpoint support.

## Enable and refresh sources

1. Open **Benchmarks** and select **Enable benchmark intelligence**.
2. Enable the individual sources you want; the master switch does not enable
   every source automatically.
3. Enter an Artificial Analysis key if using that source. Hugging Face,
   LiveBench, and LMArena accept an optional Hugging Face token; public datasets
   need no separate LiveBench or Arena key.
4. Choose **Save & reload** before **Refresh benchmarks**. Refresh uses saved settings.
5. Watch **Refresh activity** and the source cards. The progress bar counts
   completed sources (including failures), then table loading, not download bytes.

Saved keys are encrypted. Leaving a key field blank retains its reference;
**Reveal** shows only a newly entered value. Keys are cleared from browser memory
on save/sign-out. A failed source retains its previous data independently of
other sources. Refresh may take up to the configured source deadline (30 minutes
by default) while respecting shared host rate limits. Inference continues.

## Choose a source

| Type | Published interface | Notes |
| --- | --- | --- |
| `artificial_analysis` | [Official data API](https://artificialanalysis.ai/api-reference), `/api/v2/data/llms/models` | API key via `x-api-key`; capability indices, prices and performance. Explicit compatible endpoints remain supported. The obsolete built-in `/api/v2/language/models/free` URL is replaced with the documented endpoint. Credit Artificial Analysis and observe its data terms. |
| `benchlm` | [Published model JSON](https://benchlm.ai/data), `https://benchlm.ai/data/models.json` | Raw benchmarks plus published composite/category scores and provenance. The leaderboard JSON export is also accepted via `url`. CC BY-NC 4.0: commercial use requires a BenchLM licence. Estimates stay raw-only unless `include_estimated: true`. |
| `huggingface` | [Dataset Viewer rows API](https://huggingface.co/docs/dataset-viewer/rows) | Defaults to the public [Open LLM Leaderboard v2 results](https://huggingface.co/datasets/open-llm-leaderboard/contents): `default` / `train`, model ID `fullname`, six raw scores on a 0–1 scale. Explicit datasets and mappings override the preset. These are historical evaluations, not a current ranking of every model; unavailable evaluation dates stay unknown. Dataset licences remain the operator's responsibility. |
| `lmarena` | [Arena's official leaderboard dataset](https://huggingface.co/datasets/lmarena-ai/leaderboard-dataset) as native Parquet files | Defaults to `text_style_control`, `latest`. Every Arena subset remains human-preference evidence, including coding/vision categories. Confidence bounds and vote counts are retained. |
| `livebench` | [Official published judgments](https://huggingface.co/datasets/livebench/model_judgment) as native Parquet files | Defaults to `default`, `leaderboard`; per-question scores, tasks and evaluation timestamps are retained. No HTML scraping. The artifact can lag the current website; old evidence is excluded by `max_age`. |

Sources are explicitly enabled; no vendor is contacted by default. All support
an optional `url` for a trusted mirror or compatible published endpoint. URLs
cannot contain embedded credentials or secret query parameters. Credentials
are never forwarded across redirects. Use HTTPS for remote authenticated sources.

## Read the table

**Current model benchmark data** shows one row per model with capability scores,
operational measurements, provenance, dates, age, and validity. Choose **All
benchmark models** or **Configured models only**, filter rows, and select
25, 50, or 100 models per page (default 25).

Missing scores are unknown, not zero. Exact identities and explicit aliases
can supply routing evidence. **Approximate** associations are for display only:
variants remain separate and never supply smart-routing scores. Ambiguous matches
are also ineligible for routing. Associations follow the current provider catalog.
Arena preference ratings stay separate from objective accuracy; they do not
contribute to overall quality unless you explicitly change the weights.

The table uses cached data. Source cards show status, observation count, last
successful update, and cache age. Table updates occur as each source finishes.
A running refresh is followed instead of duplicated; disconnecting that observer
does not cancel the worker. Rate-limit pauses/heartbeats can indicate active work.

## Refresh from a terminal

On the gateway host with its master-key environment loaded:

```sh
./bin/gateway benchmarks refresh --config ./config.yaml
```

For the file-secret Podman setup:

```sh
podman compose -f compose.yaml -f compose.secrets.yaml exec torii-gateway /gateway benchmarks refresh --config /config/config.yaml
```

The command calls the running gateway using its stored default API key. It needs
that key to exist and intelligence to be enabled. For a reverse-proxy origin add
`--url https://gateway.example.com`. Inspect `partial_failure` in the returned
summary even if the command succeeds. [API reference](../reference/api.md#benchmark-intelligence)
shows cached-data and streamed-refresh endpoints.

## Customize imports and weights

The console presets configure URLs, datasets, and score mappings. Advanced
settings are retained during saves but edited in YAML: aliases, dataset columns,
metrics/scales, freshness thresholds, transport, and aggregation/routing weights.
Use [Configuration](../reference/configuration.md#intelligence) for every field.

LiveBench/Arena normally download selected native Parquet files. Generic/custom
Hugging Face sources use rows unless explicitly configured otherwise. Files must
fit a 64 MiB per-source download limit; datasets are limited to 100,000 rows by
default. A failed file import does not silently switch to pagination.

Only group aliases that identify the same evaluated model/version. Inspect
unmatched IDs before mapping them. Do not attach effort-labelled or different-size
results to an unqualified model. New catalog entries rematch cached evidence.

## Interpret limitations

Scores are evidence, not calibrated success probabilities. Dataset artifacts may
lag a publisher's website; retrieval time does not prove a new evaluation.
Stale/expired evidence remains inspectable but is excluded from routing according
to freshness limits. Source availability, schema, quotas, and licences can change.
BenchLM aggregates other publishers: reduce weights if importing overlapping
sources. Review source data terms before commercial use.

See [Intelligence internals](../development/intelligence.md) for normalization,
identity safety, storage, and dated source verification.

# Use benchmark intelligence

Import public model evaluations to inspect scores and optionally inform smart
routing. You need administrator access; this feature is disabled by default.
Imported scores do not grant unverified tool, vision, or endpoint support.

## Enable and refresh sources

1. Open **Benchmarks** and select **Enable benchmark intelligence**.
2. Select **Configure Ingestion** to open the source card. Enable the individual
   sources you want; the master switch does not enable
   every source automatically.
3. Enter an Artificial Analysis key if using that source. Hugging Face,
   LiveBench, and LMArena accept an optional Hugging Face token; public datasets
   need no separate LiveBench or Arena key.
4. Wait for **Saved** before **Refresh benchmarks**. Refresh uses saved settings.
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

**Current model benchmark data** shows a compact scorecard per model with overall
and capability scores, sources, match safety, and freshness. **Inspect** opens all
available scores, operational measurements, provenance, dates, age, and validity.
Choose **All
benchmark models** or **Configured models only**, filter rows, and select
5, 10, 25, 50, or 100 models per page (default 25 on desktop, 5 on mobile).
Select **Display Fields**, beside **Rows per page**, to choose table columns.
Double-click fields between **Available fields** and **Enabled fields**, or use
**Enable →** / **← Remove**. Keep at least one field enabled. Summary and editing
views retain separate field choices until sign-out; no gateway settings change.

Evidence is grouped into capability scores, operating metrics, and provenance;
dates display in UTC with exact timestamps available in tooltips.

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

## Upload custom benchmarks

1. Enable benchmark intelligence and wait for **Saved**.
2. Select **Upload Data**, then **Download CSV template** in the overlay card.
3. Replace the sample with your measurements, keeping the header and column order.
4. Choose the file under **Benchmark CSV**, then select **Upload CSV**.
5. Inspect matched scores in the benchmark table. Use exact provider/model IDs
   or explicit configured aliases so the data can inform routing.

Uploads are limited to 8 MiB and 10,000 rows. The entire CSV is validated before
saving; invalid rows leave the previous data intact. Measurements are stored in
SQLite under the reserved source `custom` and survive restart and source refresh.
Import merges by model ID, benchmark, category, and version, replacing matching
custom rows and retaining others. Duplicate identities within an upload are
rejected. At most 100,000 custom rows can be stored.

| CSV field | Meaning |
| --- | --- |
| `external_model_id` | Required exact model ID or configured benchmark alias. Approximate matches remain display-only. |
| `benchmark` | Required evaluation name; for example `my-coding-eval`. |
| `category` | Required `coding`, `agentic`, `knowledge`, `reasoning`, `math`, `instruction_following`, `multimodal`, `preference`, `general_intelligence`, `operational`, or `unknown`. |
| `original_score` | Required finite measurement in its original scale. Operational values must be nonnegative. |
| `normalized_score` | Optional finite 0–1 score, higher is better. Blank means unknown. For percentages divide by 100; for lower-is-better scales invert the normalized value. Operational measurements use the original score. |
| `unit` | Optional original unit, such as `percent`. Operational benchmark names/units: `input_price`/`output_price` use `USD_per_million_tokens`, `latency`/`ttft` use `seconds`, `throughput` uses `tokens_per_second`, `context_length` uses `tokens`. |
| `version` | Optional evaluation version; part of the custom row identity. |
| `evaluated_at` | Optional RFC3339 evaluation timestamp, such as `2026-10-01T00:00:00Z`. Blank uses retrieval time as the date basis; upload time is recorded separately. |

Each field is limited to 512 bytes. Quoted CSV values and UTF-8 headers with a
byte-order mark are supported. Freshness and aggregation rules also apply to
custom data; uploading an old evaluation does not make it new evidence.

## Edit loaded measurements

Select **Edit data** at the top of **Benchmarks**. The current data table switches
from calculated model summaries to individual measurements. Edit the model ID,
benchmark, category, original/normalized scores, unit, version or evaluation time
inline, then select **Save** on that row. **Cancel** discards that row's draft;
**Done editing** returns to summaries and asks before discarding unsaved drafts.
**Display Fields** selects measurement columns while editing. Source, matched ID,
retrieval time and date basis are calculated provenance and remain read-only.

The same filter, model scope and page-size controls apply. Drafts survive page and
field changes. Failed validation or concurrent changes retain your edits; a stale
revision requires cancelling that row and reloading cached data before retrying.
Saving rematches and aggregates enabled sources for routing. Disabled source
edits stay stored but do not affect routing. Original source metadata is retained
and marked as manually edited. Normalized scores use 0–1 in the editor, while
summary scores display percentages.

Refreshing a downloaded source can replace its manual edits. Keep independent
measurements in custom CSV data when they must survive publisher refreshes.
Upload and editing require administrator authentication; cookie mutations also
require the console's CSRF token.

## Refresh from a terminal

On the gateway host with its master-key environment loaded:

```sh
./bin/gateway benchmarks refresh --database ./state/torii.db
```

For the file-secret Podman setup:

```sh
podman compose -f compose.yaml -f compose.secrets.yaml exec torii-gateway /gateway benchmarks refresh --database /data/torii.db
```

The command calls the running gateway using its stored default API key. It needs
that key to exist and intelligence to be enabled. For a reverse-proxy origin add
`--url https://gateway.example.com`. Inspect `partial_failure` in the returned
summary even if the command succeeds. [API reference](../reference/api.md#benchmark-intelligence)
shows cached-data and streamed-refresh endpoints.

## Customize imports and weights

The console presets configure URLs, datasets, and score mappings. Advanced
settings are retained during saves but edited through the revision-protected configuration API: aliases, dataset columns,
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

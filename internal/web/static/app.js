// Generated from benchmarks.ts and app.ts by scripts/build-console.mjs.
// Source forms and cached evidence views. No browser persistence or dependencies.

let benchmarkConfiguration                      = {};
let benchmarkSources                         = [];
let benchmarkSourceStatus                      = {};
let benchmarkSourceActivity                         = {};
let benchmarkModels                      = {};
let benchmarkRawRows                        = [];
let benchmarkRawTotal = 0;
let benchmarkPage = 0;
let benchmarkTableView = "models";
let benchmarkFilter = "";
let benchmarkDirty = false;
let benchmarkRefreshRunning = false;
let benchmarkAbort                         = null;
let benchmarkReloadSequence = 0;
let benchmarkProgressLog           = [];
let benchmarkProgressCounts = {total:0,completed:0,failed:0};
const benchmarkPageSize = 100;
const benchmarkDefinitions = [
  { type:"artificial_analysis", name:"Artificial Analysis", url:"https://artificialanalysis.ai/api/v2/language/models/free", note:"Requires an Artificial Analysis API key. Published capability, pricing, and speed measurements are imported." },
  { type:"benchlm", name:"BenchLM.ai", url:"https://benchlm.ai/data/models.json", note:"Imports supported benchmark evidence by default. Estimated evidence is opt-in." },
  { type:"huggingface", name:"Hugging Face leaderboards", url:"https://datasets-server.huggingface.co/rows", note:"Choose published leaderboard datasets and map their numeric columns to explicit categories and score scales." },
  { type:"lmarena", name:"LMArena", url:"https://datasets-server.huggingface.co/rows", note:"Arena ratings remain human-preference evidence, not objective intelligence. Dataset fields can be customized." },
  { type:"livebench", name:"LiveBench", url:"https://datasets-server.huggingface.co/rows", note:"Imports structured published judgments. If the artifact is unavailable or its schema changes, cached data is retained; no HTML scraping is used." },
];
const benchmarkCategories = ["general_intelligence", "coding", "agentic", "reasoning", "math", "knowledge", "instruction_following", "multimodal", "preference", "operational"];
const benchmarkOperations                         = { input_price:"USD_per_million_tokens", output_price:"USD_per_million_tokens", latency:"seconds", ttft:"seconds", throughput:"tokens_per_second", context_length:"tokens" };

function benchmarkMetricDrafts(metrics                      = {})                        {
  return Object.entries(metrics).map(([column, mapping]) => ({ column, ...mapping }));
}
function benchmarkDataset(type        )                      {
  if (type === "lmarena") return { dataset:"lmarena-ai/leaderboard-dataset", config:"text_style_control", split:"latest", model_field:"model_name", date_field:"leaderboard_publish_date", _metrics:[] };
  if (type === "livebench") return { dataset:"livebench/model_judgment", config:"default", split:"leaderboard", model_field:"model", date_field:"tstamp", _metrics:[] };
  return { dataset:"", config:"default", split:"train", model_field:"model", date_field:"", _metrics:[] };
}
function benchmarkDraft(source                     , fresh = false)                       {
  return { ...source, _new:fresh, api_key_input:"", _metrics:benchmarkMetricDrafts(source.metrics),
    datasets:(source.datasets || (["huggingface", "lmarena", "livebench"].includes(source.type) ? [benchmarkDataset(source.type)] : []))
      .map((dataset                     ) => ({ ...dataset, _metrics:dataset._metrics || benchmarkMetricDrafts(dataset.metrics) })) };
}
function applyBenchmarkConfig(config                      = {})       {
  benchmarkConfiguration = structuredClone(config);
  benchmarkSources = (config.sources || []).map((source                     ) => benchmarkDraft(source));
  for (const definition of benchmarkDefinitions) {
    if (benchmarkSources.some(source => source.type === definition.type)) continue;
    let id = definition.type.replace(/_/g, "-");
    const base = id;
    for (let suffix = 2; benchmarkSources.some(source => source.id === id); suffix++) id = `${base}-${suffix}`;
    benchmarkSources.push(benchmarkDraft({ id, type:definition.type, enabled:false }, true));
  }
  benchmarkDirty = false;
  benchmarkSourceActivity = {};
  benchmarksEnabledSaved = config.enabled === true;
  document.querySelector                  ("#intelligence-enabled") .checked = benchmarksEnabledSaved;
  document.querySelectorAll                  ("[data-benchmark-global]").forEach(input => { input.value = String(config[input.dataset.benchmarkGlobal ] ?? ""); });
  renderBenchmarkSources();
  updateBenchmarkControls();
}
function clearBenchmarkConsole()       {
  benchmarkAbort?.abort(); benchmarkAbort = null; benchmarkReloadSequence++;
  benchmarkConfiguration = {}; benchmarkSources = []; benchmarkSourceStatus = {}; benchmarkSourceActivity = {}; benchmarkModels = {}; benchmarkRawRows = [];
  benchmarkRawTotal = 0; benchmarkPage = 0; benchmarkFilter = ""; benchmarkTableView = "models";
  benchmarkDirty = false; benchmarkRefreshRunning = false; benchmarkProgressLog = [];
  benchmarkProgressCounts = {total:0,completed:0,failed:0};
  document.querySelector                  ("#intelligence-enabled") .checked = false;
  document.querySelector                ("#benchmark-refresh-panel") .hidden = true;
  document.querySelector("#benchmark-refresh-log") .textContent = "";
  document.querySelector                  ("#benchmark-table-filter") .value = "";
  document.querySelector                   ("#benchmark-table-view") .value = "models";
  document.querySelectorAll                  ("[data-benchmark-global]").forEach(input => { input.value = ""; });
  renderBenchmarkSources(); renderBenchmarkTable();
}
function benchmarkAttributes(source        , field        , dataset         , mapping         )         {
  return `data-benchmark-source="${source}" data-benchmark-field="${field}"${dataset === undefined ? "" : ` data-benchmark-dataset="${dataset}"`}${mapping === undefined ? "" : ` data-benchmark-mapping="${mapping}"`}`;
}
function benchmarkInput(source        , field        , label        , raw     , help        , dataset         , mapping         , type = "text")         {
  return `<label data-tooltip="${escapeHTML(help)}">${escapeHTML(label)}<input ${benchmarkAttributes(source, field, dataset, mapping)} type="${type}" value="${escapeHTML(String(raw ?? ""))}" /></label>`;
}
function benchmarkMappings(source        , rows                       , dataset         )         {
  return rows.map((row, index) => `<div class="benchmark-mapping"><div class="section-heading"><h4>Metric mapping ${index + 1}</h4><button type="button" class="button quiet" data-benchmark-action="remove-metric" ${benchmarkAttributes(source, "", dataset, index)}>Remove</button></div><div class="grid three">
    ${benchmarkInput(source, "column", "Upstream column / benchmark", row.column, "The numeric column or benchmark key in the upstream data. Nested dataset columns can use dot paths.", dataset, index)}
    <label data-tooltip="The capability this measurement describes. Arena measurements always remain preference evidence.">Category<select ${benchmarkAttributes(source, "category", dataset, index)}>${benchmarkCategories.map(category => `<option value="${category}" ${row.category === category ? "selected" : ""}>${category.replace(/_/g, " ")}</option>`).join("")}</select></label>
    ${benchmarkInput(source, "min", "Minimum score", row.min ?? 0, "The declared lower end of the score scale. This is not inferred from observed values.", dataset, index, "number")}
    ${benchmarkInput(source, "max", "Maximum score", row.max ?? 1, "The declared upper end of the score scale. Scores outside this range reject the source update.", dataset, index, "number")}
    <label class="checkbox" data-tooltip="Invert normalized scores for measurements where a smaller value is better. Operational measurements keep their original units."><input type="checkbox" ${benchmarkAttributes(source, "lower_is_better", dataset, index)} ${row.lower_is_better ? "checked" : ""} /> Lower is better</label>
    <label data-tooltip="For operational mappings, choose a canonical measurement; its unit is set automatically. Leave empty for capability scores.">Operational metric<select ${benchmarkAttributes(source, "metric", dataset, index)}><option value="">Not operational</option>${Object.entries(benchmarkOperations).map(([metric, unit]) => `<option value="${metric}" ${row.metric === metric ? "selected" : ""}>${metric} (${unit})</option>`).join("")}</select></label>
  </div></div>`).join("") + `<button type="button" class="button quiet" data-benchmark-action="add-metric" ${benchmarkAttributes(source, "", dataset)}>＋ Add metric mapping</button>`;
}
function renderBenchmarkSources()       {
  const list = document.querySelector                ("#benchmark-sources-list") ;
  list.innerHTML = benchmarkSources.map((source, index) => {
    const definition = benchmarkDefinitions.find(item => item.type === source.type) ;
    const datasets = ["huggingface", "lmarena", "livebench"].includes(source.type);
    return `<article class="provider-card"><div class="provider-card-head"><h3>${escapeHTML(definition?.name || source.type)}${source._new ? "" : ` · ${escapeHTML(source.id)}`}</h3><label class="checkbox" data-tooltip="Enable this source for background and manual refreshes. Save and reload before ingestion starts."><input type="checkbox" ${benchmarkAttributes(index, "enabled")} ${source.enabled ? "checked" : ""} /> Enabled</label></div>
      <div class="provider-result" data-benchmark-status="${index}" role="status" aria-live="polite"></div>
      <div class="benchmark-source-options" data-benchmark-options="${index}" ${source.enabled ? "" : "hidden"}>
      <p>${escapeHTML(definition?.note || "")}</p><div class="grid two">
      ${benchmarkInput(index, "id", "Source ID", source.id, "The stable source identifier for cache, provenance, and weights. Changing it starts a separate source history.")}
      ${benchmarkInput(index, "url", "Data API URL", source.url || "", `Leave empty to use ${definition?.url || "the adapter default"}. Use a machine-readable endpoint, not an HTML page.`)}
      <label data-tooltip="Enter a source API key or token; saved values cannot be revealed. Save encrypts new values in SQLite and keeps only a reference in YAML.">API key${source.type === "artificial_analysis" ? " (required)" : " (optional)"}<span class="password-field"><input type="password" autocomplete="new-password" ${benchmarkAttributes(index, "api_key_input")} placeholder="${source.api_key?.credential ? "Configured · leave blank to keep" : "Enter source API key"}" /><button type="button" class="reveal-button" data-benchmark-action="reveal-key" ${benchmarkAttributes(index, "")}>Reveal</button></span></label>
      ${datasets ? benchmarkInput(index, "max_rows", "Maximum rows", source.max_rows, "Reject datasets exceeding this limit rather than silently truncating them. The default and maximum are 100000.", undefined, undefined, "number") : ""}
      ${source.type === "benchlm" ? `<label class="checkbox" data-tooltip="Allow estimated BenchLM evidence to influence aggregation. Uncertain measurements are still retained with provenance."><input type="checkbox" ${benchmarkAttributes(index, "include_estimated")} ${source.include_estimated ? "checked" : ""} /> Include estimated evidence</label>` : ""}
      </div>
      ${datasets ? (source.datasets || []).map((dataset                     , datasetIndex        ) => `<div class="benchmark-dataset"><div class="section-heading"><h4>Dataset ${datasetIndex + 1}</h4><button type="button" class="button quiet" data-benchmark-action="remove-dataset" ${benchmarkAttributes(index, "", datasetIndex)}>Remove</button></div><div class="grid three">
        ${[ ["dataset", "Dataset ID", "The published Hugging Face dataset identifier, such as organization/leaderboard. It must contain published model results."], ["config", "Dataset configuration", "The Dataset Viewer configuration or subset name. This must match the published artifact."], ["split", "Split", "The published split containing benchmark rows, such as train or leaderboard."], ["model_field", "Model ID column", "The column containing each upstream model identifier. Nested fields may use dot paths."], ["date_field", "Evaluation date column", "The column containing the evaluation date. Leave empty when unavailable; retrieval-only freshness will be marked."], ["version", "Benchmark version", "An optional version label retained with each observation. This does not replace the evaluation date."] ].map(([field, label, help]) => benchmarkInput(index, field, label, dataset[field], help, datasetIndex)).join("")}
      </div>${source.type === "livebench" ? "<p>LiveBench uses published score, category and task columns with a 0–1 scale.</p>" : benchmarkMappings(index, dataset._metrics || [], datasetIndex)}</div>`).join("") + `<button type="button" class="button quiet" data-benchmark-action="add-dataset" ${benchmarkAttributes(index, "")} ${(source.datasets || []).length >= 8 ? "disabled" : ""}>＋ Add dataset</button>`
        : `<details class="advanced-provider"><summary>Benchmark category / scale overrides</summary>${benchmarkMappings(index, source._metrics || [])}</details>`}
      </div></article>`;
  }).join("");
  list.querySelectorAll                  ('[data-benchmark-field="api_key_input"]').forEach(input => { input.value = benchmarkSources[Number(input.dataset.benchmarkSource)].api_key_input || ""; });
  renderBenchmarkSourceStatus(); initializeTooltips();
}
function renderBenchmarkSourceStatus()       {
  benchmarkSources.forEach((source, index) => {
    const node = document.querySelector             (`[data-benchmark-status="${index}"]`);
    if (!node) return;
    const status = benchmarkSourceStatus[source.id];
    const success = status?.last_success && !String(status.last_success).startsWith("0001-");
    const activity = benchmarkSourceActivity[source.id];
    node.textContent = (activity ? `${activity} · ` : "") + (!status ? (source.enabled ? "Not refreshed yet" : "Disabled · no cached results")
      : `${status.enabled ? (status.error ? "Refresh failed" : success ? (status.stale ? "Cached data is stale" : "Working") : "Not refreshed yet") : "Disabled"} · ${status.observation_count || 0} observations${success ? ` · Last successful update ${status.last_success} · Cache age ${benchmarkNumber(status.age_seconds)} s` : ""}${status.error ? ` · ${status.error}` : ""}`);
    node.className = `provider-result ${status?.error || activity === "Update failed; previous cache retained" ? "failure" : success && !status.stale && status.enabled ? "success" : ""}`;
  });
}
function updateBenchmarkSource(event       )       {
  const input = event.target                                        ;
  if (input.dataset.benchmarkSource === undefined || !input.dataset.benchmarkField) return;
  const source = benchmarkSources[Number(input.dataset.benchmarkSource)];
  const dataset = input.dataset.benchmarkDataset === undefined ? undefined : source.datasets[Number(input.dataset.benchmarkDataset)];
  const target = input.dataset.benchmarkMapping === undefined ? (dataset || source) : (dataset || source)._metrics[Number(input.dataset.benchmarkMapping)];
  const field = input.dataset.benchmarkField;
  target[field] = input.type === "checkbox" ? (input                    ).checked : input.value;
  if (field === "metric" && input.value) { target.category = "operational"; target.unit = benchmarkOperations[input.value]; }
  benchmarkDirty = true; markDirty(); updateBenchmarkControls();
  if (field === "enabled" || field === "metric") renderBenchmarkSources();
}
function benchmarkSourceAction(event       )       {
  const button = (event.target               ).closest                   ("[data-benchmark-action]");
  if (!button || button.disabled) return;
  const source = benchmarkSources[Number(button.dataset.benchmarkSource)];
  const datasetIndex = button.dataset.benchmarkDataset === undefined ? undefined : Number(button.dataset.benchmarkDataset);
  const target = datasetIndex === undefined ? source : source.datasets[datasetIndex];
  const action = button.dataset.benchmarkAction;
  if (action === "reveal-key") {
    const input = document.querySelector                  (`[data-benchmark-source="${button.dataset.benchmarkSource}"][data-benchmark-field="api_key_input"]`) ;
    input.type = input.type === "password" ? "text" : "password";
    button.textContent = input.type === "text" ? "Hide" : "Reveal";
    button.setAttribute("aria-pressed", String(input.type === "text")); return;
  }
  if (action === "add-dataset" && source.datasets.length < 8) source.datasets.push(benchmarkDataset(source.type));
  if (action === "remove-dataset") source.datasets.splice(datasetIndex, 1);
  if (action === "add-metric") target._metrics.push({ column:"", category:source.type === "lmarena" ? "preference" : "coding", min:0, max:1 });
  if (action === "remove-metric") target._metrics.splice(Number(button.dataset.benchmarkMapping), 1);
  benchmarkDirty = true; renderBenchmarkSources(); markDirty(); updateBenchmarkControls();
}
function readBenchmarkMappings(rows                       )                      {
  const mappings                      = Object.create(null);
  for (const row of rows) {
    const column = String(row.column || "").trim();
    if (!column || Object.hasOwn(mappings, column)) throw new Error("Benchmark metric columns must be nonempty and unique");
    const { column:ignored, ...mapping } = row;
    mapping.min = Number(mapping.min || 0); mapping.max = Number(mapping.max ?? 1);
    if (!Number.isFinite(mapping.min) || !Number.isFinite(mapping.max)) throw new Error("Benchmark score scales must be finite numbers");
    mappings[column] = mapping;
  }
  return mappings;
}
function readBenchmarkConfiguration()                      {
  const config = structuredClone(benchmarkConfiguration);
  config.enabled = document.querySelector                  ("#intelligence-enabled") .checked;
  config.sources = benchmarkSources.filter(source => !source._new || source.enabled).map(source => {
    const { _new, _metrics, api_key_input, datasets, ...output } = source;
    output.id = String(output.id || "").trim();
    if (!output.id) throw new Error("Each benchmark source needs a source ID");
    if (output.url === "") delete output.url;
    if (output.max_rows === "" || output.max_rows === undefined) delete output.max_rows; else output.max_rows = Number(output.max_rows);
    output.metrics = readBenchmarkMappings(_metrics || []);
    if (datasets?.length) output.datasets = datasets.map((dataset                     ) => {
      const { _metrics:rows, ...saved } = dataset;
      saved.metrics = readBenchmarkMappings(rows || []);
      return saved;
    });
    if (api_key_input) output.api_key = { value:api_key_input };
    if (output.enabled && output.type === "artificial_analysis" && !output.api_key) throw new Error("Artificial Analysis requires an API key");
    if (output.type === "huggingface" && (!output.datasets?.length || output.datasets.some((dataset     ) => !dataset.dataset || Object.keys(dataset.metrics).length === 0))) throw new Error("Hugging Face requires a dataset ID and at least one metric mapping per dataset");
    return output;
  });
  document.querySelectorAll                  ("[data-benchmark-global]").forEach(input => {
    const field = input.dataset.benchmarkGlobal ;
    if (input.value.trim() === "") delete config[field]; else config[field] = field === "retries" ? Number(input.value) : input.value.trim();
  });
  return config;
}
function clearSubmittedBenchmarkKeys()       { benchmarkSources.forEach(source => { source.api_key_input = ""; }); renderBenchmarkSources(); }
function benchmarkSafeMessage(message        )         {
  for (const source of benchmarkSources) { if (source.api_key_input) message = message.split(source.api_key_input).join("[redacted]"); }
  return message;
}
function benchmarkNumber(raw     , score = false)         { return typeof raw === "number" && Number.isFinite(raw) ? (score ? (raw * 100).toFixed(1) : String(Number(raw.toFixed(4)))) : "—"; }
function renderBenchmarkTable()       {
  const raw = benchmarkTableView === "observations";
  const scores = ["overall", ...benchmarkCategories.filter(category => category !== "operational")];
  const operations = Object.keys(benchmarkOperations);
  const headers = raw ? ["Model / external ID", "Source", "Benchmark", "Category", "Raw score", "Normalized %", "Unit", "Version", "Evaluation date", "Retrieved", "Match"]
    : ["Model", "Providers", ...scores.map(score => `${score.replace(/_/g," ")} %`), "Input $ / 1M", "Output $ / 1M", "Latency s", "TTFT s", "Tokens / s", "Context tokens", "Sources", "Observations", "Data status", "Last retrieval"];
  let rows       ;
  let total        ;
  if (raw) { rows = benchmarkRawRows; total = benchmarkRawTotal; }
  else {
    const query = benchmarkFilter.toLowerCase();
    const models = Object.entries(benchmarkModels).sort(([a],[b]) => a.localeCompare(b)).filter(([id, model]) => `${id} ${(model.provider_ids || []).join(" ")} ${(model.sources || []).join(" ")}`.toLowerCase().includes(query));
    total = models.length;
    benchmarkPage = Math.min(benchmarkPage, Math.max(0, Math.ceil(total / benchmarkPageSize) - 1));
    rows = models.slice(benchmarkPage * benchmarkPageSize, (benchmarkPage + 1) * benchmarkPageSize).map(([id, model]) => ({...model,id}));
  }
  document.querySelector("#benchmark-table-head") .innerHTML = `<tr>${headers.map(header => `<th scope="col">${escapeHTML(header)}</th>`).join("")}</tr>`;
  document.querySelector("#benchmark-table-body") .innerHTML = rows.length ? rows.map(row => {
    const values = raw ? [row.model_id || row.external_model_id, row.source, row.benchmark, row.category, benchmarkNumber(row.original_score), benchmarkNumber(row.normalized_score,true), row.unit || "—", row.version || "—", benchmarkDate(row.evaluated_at), benchmarkDate(row.retrieved_at), row.model_id ? "Matched" : "Unmatched / ambiguous"]
      : [row.id, (row.provider_ids || []).join(", "), ...scores.map(score => benchmarkNumber(row.scores?.[score],true)), ...operations.map(metric => benchmarkNumber(row.operations?.[metric])), (row.sources || []).join(", "), String(row.observation_count || 0), row.stale ? "Stale" : "Current", benchmarkDate(row.latest_retrieval)];
    return `<tr>${values.map(value => `<td>${escapeHTML(String(value))}</td>`).join("")}</tr>`;
  }).join("") : `<tr><td colspan="${headers.length}">No cached benchmark data matches this view. Enable sources, save, and refresh to import results.</td></tr>`;
  const pages = Math.max(1, Math.ceil(total / benchmarkPageSize));
  document.querySelector("#benchmark-data-count") .textContent = `${total} ${raw ? "observations" : "models"}`;
  document.querySelector("#benchmark-page-label") .textContent = `Page ${benchmarkPage + 1} of ${pages}`;
  document.querySelector                   ("#benchmark-previous") .disabled = benchmarkPage === 0;
  document.querySelector                   ("#benchmark-next") .disabled = (benchmarkPage + 1) * benchmarkPageSize >= total;
}
function benchmarkDate(value     )         { return value && !String(value).startsWith("0001-") ? String(value) : "Unknown"; }
async function loadBenchmarkData()                {
  if (!benchmarksEnabledSaved || !adminAuthorization) return;
  const sequence = ++benchmarkReloadSequence;
  const login = adminAuthorization;
  try {
    const headers = await authProvider.headers();
    const paths = ["sources", "models", ...(benchmarkTableView === "observations" ? [`observations?offset=${benchmarkPage * benchmarkPageSize}&limit=${benchmarkPageSize}&q=${encodeURIComponent(benchmarkFilter)}`] : [])];
    const replies = await Promise.all(paths.map(async path => {
      const response = await gatewayFetch(`/gateway/v1/intelligence/${path}`, {headers});
      const body = await response.json();
      if (!response.ok) throw new Error(body.error?.message || body.error || "Unable to load benchmark data");
      return body;
    }));
    if (sequence !== benchmarkReloadSequence || login !== adminAuthorization) return;
    [benchmarkSourceStatus, benchmarkModels] = replies;
    if (replies[2]) { benchmarkRawRows = replies[2].observations || []; benchmarkRawTotal = replies[2].total || 0; }
    renderBenchmarkSourceStatus(); renderBenchmarkTable();
    if (!benchmarkRefreshRunning) document.querySelector("#benchmark-status") .textContent = "Showing saved benchmark evidence. Missing scores are unknown, not zero; human preference remains a separate signal.";
  } catch (error) { if (sequence === benchmarkReloadSequence) document.querySelector("#benchmark-status") .textContent = benchmarkSafeMessage(error instanceof Error ? error.message : "Unable to load benchmark data"); }
}
function benchmarkProgress(event                     )       {
  if (event.source) {
    const labels                         = {fetching:"Fetching and parsing",persisting:"Saving validated data",failed:"Update failed; previous cache retained"};
    if (labels[event.phase]) benchmarkSourceActivity[event.source] = labels[event.phase];
    if (event.phase === "published") delete benchmarkSourceActivity[event.source];
    renderBenchmarkSourceStatus();
  }
  for (const field of ["total", "completed", "failed"]         ) {
    if (event[field] !== undefined) benchmarkProgressCounts[field] = Number(event[field]);
  }
  const {total, completed, failed} = benchmarkProgressCounts;
  const bar = document.querySelector                     ("#benchmark-progress") ;
  bar.max = Math.max(1, total); bar.value = total ? Math.min(total, completed) : event.phase === "complete" ? 1 : 0;
  document.querySelector("#benchmark-progress-label") .textContent = `${completed} of ${total} sources finished${failed ? ` · ${failed} failed` : ""} · ${benchmarkSafeMessage(String(event.message || ""))}`;
  benchmarkProgressLog.push(benchmarkSafeMessage(`${event.source ? `${event.source}: ` : ""}${event.message || event.phase}${event.observations ? ` (${event.observations} observations)` : ""}`));
  benchmarkProgressLog = benchmarkProgressLog.slice(-256);
  const log = document.querySelector             ("#benchmark-refresh-log") ;
  log.textContent = benchmarkProgressLog.join("\n"); log.scrollTop = log.scrollHeight;
}
async function inspectBenchmarks(refresh         )                {
  if (!refresh) { await loadBenchmarkData(); return; }
  const output = document.querySelector("#benchmark-status") ;
  if (!benchmarksEnabledSaved || !document.querySelector                  ("#intelligence-enabled") .checked || benchmarkDirty) { output.textContent = "Enable benchmark intelligence and click Save & reload before refreshing changed source settings."; return; }
  if (benchmarkRefreshRunning) return;
  const login = adminAuthorization;
  const abort = new AbortController();
  benchmarkRefreshRunning = true; benchmarkAbort = abort; benchmarkProgressLog = []; benchmarkSourceActivity = {};
  benchmarkProgressCounts = {total:0,completed:0,failed:0};
  document.querySelector                ("#benchmark-refresh-panel") .hidden = false;
  benchmarkProgress({phase:"connecting",total:0,message:"Connecting to the refresh worker"}); updateBenchmarkControls();
  try {
    const response = await gatewayFetch("/gateway/v1/intelligence/refresh", {method:"POST",headers:{Accept:"text/event-stream",...await authProvider.headers()},signal:abort.signal});
    if (!response.ok) { const body = await response.json(); throw new Error(body.error?.message || body.error || "Refresh failed"); }
    if (!response.body?.getReader || !response.headers?.get("Content-Type")?.includes("text/event-stream")) {
      const body = await response.json();
      if (login !== adminAuthorization || abort.signal.aborted) return;
      benchmarkSourceStatus = body.sources || {}; renderBenchmarkSourceStatus();
      output.textContent = body.partial_failure ? "Refresh finished with source failures; previous data retained." : "Benchmark refresh finished.";
      await loadBenchmarkData(); return;
    }
    const reader = response.body.getReader(); const decoder = new TextDecoder(); let buffer = ""; let finished = false;
    try {
      while (true) {
        const chunk = await reader.read();
        buffer = (buffer + decoder.decode(chunk.value || new Uint8Array(), {stream:!chunk.done})).replace(/\r\n/g,"\n");
        if (buffer.length > 4*1024*1024) throw new Error("Benchmark progress frame is too large");
        let boundary        ;
        while ((boundary = buffer.indexOf("\n\n")) >= 0) {
          const frame = buffer.slice(0,boundary); buffer = buffer.slice(boundary+2);
          const lines = frame.split("\n"); const kind = lines.find(line => line.startsWith("event:"))?.slice(6).trim();
          const data = lines.filter(line => line.startsWith("data:")).map(line => line.slice(5).trimStart()).join("\n");
          if (!data || login !== adminAuthorization || abort.signal.aborted) continue;
          const event = JSON.parse(data);
          if (kind === "progress") { benchmarkProgress(event); if (event.phase === "published" || event.phase === "failed") await loadBenchmarkData(); }
          if (kind === "result") { finished = true; benchmarkSourceStatus = event.sources || {}; renderBenchmarkSourceStatus(); output.textContent = event.partial_failure ? "Refresh finished with source failures; previous good data retained." : "Benchmark refresh finished."; await loadBenchmarkData(); }
        }
        if (chunk.done) break;
      }
      if (!finished && login === adminAuthorization && !abort.signal.aborted) throw new Error("Refresh connection ended before completion. Reload cached data to inspect published results.");
    } finally { await reader.cancel(); }
  } catch (error) { if (login === adminAuthorization && !abort.signal.aborted) { const message = benchmarkSafeMessage(error instanceof Error ? error.message : "Refresh failed"); output.textContent = message; benchmarkProgress({phase:"failed",message}); } }
  finally { if (benchmarkAbort === abort) { benchmarkRefreshRunning = false; benchmarkAbort = null; updateBenchmarkControls(); } }
}
function initializeBenchmarks()       {
  const list = document.querySelector("#benchmark-sources-list") ;
  list.addEventListener("input", updateBenchmarkSource); list.addEventListener("click", benchmarkSourceAction);
  document.querySelectorAll                  ("[data-benchmark-global]").forEach(input => input.addEventListener("input",()=>{benchmarkDirty=true;markDirty();updateBenchmarkControls();}));
  document.querySelector("#benchmark-table-view") .addEventListener("change",()=>{benchmarkTableView=document.querySelector                   ("#benchmark-table-view") .value;benchmarkPage=0;void loadBenchmarkData();});
  document.querySelector("#benchmark-table-filter") .addEventListener("input",()=>{benchmarkFilter=document.querySelector                  ("#benchmark-table-filter") .value;benchmarkPage=0;if(benchmarkTableView==="models")renderBenchmarkTable();else void loadBenchmarkData();});
  document.querySelector("#benchmark-previous") .addEventListener("click",()=>{if(benchmarkPage>0){benchmarkPage--;if(benchmarkTableView==="models")renderBenchmarkTable();else void loadBenchmarkData();}});
  document.querySelector("#benchmark-next") .addEventListener("click",()=>{benchmarkPage++;if(benchmarkTableView==="models")renderBenchmarkTable();else void loadBenchmarkData();});
  renderBenchmarkTable();
}



// Administrator authentication remains in memory, never browser storage.




class MemoryBasicProvider                         {
  async headers()                       { return adminAuthorization ? { Authorization:adminAuthorization } : {}; }
}

const authProvider               = new MemoryBasicProvider();
async function gatewayFetch(url       ,options             )                  {
  const login = adminAuthorization;
  const response=await fetch(url,options);
  if(response.status===401 && login===adminAuthorization){signOutConsole();document.querySelector                   ("#login-dialog")?.showModal?.();}
  return response;
}
const form = document.querySelector                 ("#config-form") ;
const providerList = document.querySelector                ("#providers-list") ;
const saveState = document.querySelector                 ("#save-state") ;
let adminAuthorization = "";
let signedInUsername = "";
let benchmarksEnabledSaved = false;
function updateConsoleIdentity()       {
  const signedIn = Boolean(adminAuthorization && signedInUsername);
  document.querySelector                   ("#console-login") .hidden = signedIn;
  document.querySelector                   ("#console-logout") .hidden = !signedIn;
  document.querySelector("#signed-in-user") .textContent = signedIn ? signedInUsername : "Not signed in";
}
function signOutConsole()       {
  adminAuthorization = "";
  signedInUsername = "";
  clearNewAPIKey();
  document.querySelector                   ("#new-key-dialog") .close();
  document.querySelector                   ("#create-key-dialog") .close();
  document.querySelector                  ("#client-key-name") .value = "";
  providers = [];
  configurationLoaded = false;
  form.reset();
  clearBenchmarkConsole();
  benchmarksEnabledSaved = false;
  document.querySelector("#credential-list") .textContent = "";
  document.querySelector("#benchmark-status") .textContent = "";
  updateConsoleIdentity();
  updateBenchmarkControls();
  renderProviders();
  saveState.textContent = "Signed out";
}
function basicAuthorization(username       ,password       )        {return "Basic "+btoa(Array.from(new TextEncoder().encode(`${username}:${password}`),byte=>String.fromCharCode(byte)).join(""));}
const providerDefaults = { id: "", type: "openai_compatible", api_url: "", api_key: "", api_key_env: "", api_key_file: "", management_env: "", portal_url: "", client_id: "", headers: "", models: "", discover: true, transport: "", organization: "", project: "", rewrites: "", tls_ca: "", tls_server: "", connect_timeout: "", request_timeout: "", discovery_timeout: "" };
let providers                                     = [];




const providerStatuses = new WeakMap                                                  ();
const classifierCatalogs = new WeakMap                                            ();
let classifierDiscoveryPending = false;
let configurationLoaded = false;
let requestedClassifier = "";
let legacyClassifier                             = null;
const classifierSelect = document.querySelector                   ("#classifier-model") ;

function classifierProvider(provider                                  )          {
  return provider.type === "jev" || provider.type === "jev_compatible";
}
function availableClassifiers()           {
  return providers.filter(provider => classifierProvider(provider) && providerStatuses.get(provider)?.state === "success")
    .flatMap(provider => classifierCatalogs.get(provider) || []);
}
function updateClassifierOptions()       {
  const models = availableClassifiers();
  const pending = classifierDiscoveryPending || providers.some(provider => classifierProvider(provider) && providerStatuses.get(provider)?.state === "pending");
  const existing = requestedClassifier && requestedClassifier !== "disabled" && !models.includes(requestedClassifier);
  classifierSelect.innerHTML = (models.length === 0 ? `<option value="">${pending ? "Discovering classifier models…" : "No classifier models available"}</option>` : "") + `<option value="disabled">Disabled · deterministic routing only</option>` +
    (existing ? `<option value="${escapeHTML(requestedClassifier)}">${legacyClassifier ? "Existing classifier" : "Selected classifier unavailable"} · ${escapeHTML(requestedClassifier)}</option>` : "") +
    models.map(model => `<option value="${escapeHTML(model)}">${escapeHTML(model)}</option>`).join("");
  classifierSelect.value = requestedClassifier || models[0] || "";
  classifierSelect.disabled = pending && models.length === 0;
  const notice = document.querySelector                ("#classifier-notice") ;
  const message = document.querySelector                 ("#classifier-notice-message") ;
  notice.hidden = models.length > 0 && !existing;
  document.querySelector                ("#classifier-provider-notice") .hidden = pending || models.length > 0;
  message.textContent = pending ? "Checking classifier providers…" : models.length === 0
    ? "No classifier models are available. Add a TypeSafe Jev or Jev-compatible provider (such as Laya) on the Providers page, then test its connection. Deterministic routing remains available."
    : "The selected classifier is unavailable. Check its provider or choose another discovered model.";
  if (configurationLoaded && !pending && models.length === 0 && location.hash === "#routing") {
    toast("No classifier models available. Add a Jev or Jev-compatible provider on the Providers page.");
    location.hash = "#providers";
    navigate();
  }
}

function setProviderStatus(provider                                  , status                )       {
  status.message = redactProviderKeys(status.message);
  providerStatuses.set(provider, status);
  const index = providers.indexOf(provider);
  if (index < 0) return;
  const result = providerList.querySelector                (`[data-result="${index}"]`);
  if (result) {
    result.className = `provider-result ${status.state}`;
    result.textContent = status.message;
  }
  const button = providerList.querySelector                   (`[data-test="${index}"]`);
  if (button) button.disabled = status.state === "pending";
  updateClassifierOptions();
}

function redactProviderKeys(message        )         {
  providers.forEach(provider => {
    const key = String(provider.api_key || "");
    if (key) message = message.split(key).join("[redacted]");
  });
  return benchmarkSafeMessage(message);
}

function escapeHTML(value        )         { return value.replace(/[&<>"]/g, ch => ({"&":"&amp;", "<":"&lt;", ">":"&gt;", '"':"&quot;"}[ch] )); }
function value(provider                                  , key        )         { return escapeHTML(String(provider[key] ?? "")); }
function checked(provider                                  , key        )         { return provider[key] === false ? "" : "checked"; }
function subscriptionProvider(provider                                  )          { return provider.type === "openai-codex" || provider.type === "nous"; }
const knownProviderIDs                         = { openai: "openai", "openai-codex": "openai-codex", anthropic: "anthropic", gemini: "gemini", xai: "xai", openrouter: "openrouter", ollama: "ollama", nous: "nous", jev: "jev" };
function knownProviderID(type        , index        )         {
  const base = knownProviderIDs[type];
  if (!base) return "";
  const used = providers.filter((provider, providerIndex) => providerIndex !== index && String(provider.id || "").startsWith(base)).map(provider => String(provider.id));
  if (!used.includes(base)) return base;
  let suffix = 2;
  while (used.includes(`${base}-${suffix}`)) suffix += 1;
  return `${base}-${suffix}`;
}
function isKnownProvider(provider                                  )          { return Boolean(knownProviderIDs[String(provider.type)]); }
function initializeTooltips()       {
  document.querySelectorAll                  ("label[data-tooltip]").forEach(label => {
    if (label.querySelector(".tooltip-trigger")) return;
    const trigger = document.createElement("button");
    trigger.type = "button";
    trigger.className = "tooltip-trigger";
    trigger.textContent = "?";
    trigger.setAttribute("aria-label", "Show help");
    trigger.setAttribute("aria-expanded", "false");
    const content = document.createElement("span");
    content.className = "tooltip-content";
    content.setAttribute("role", "tooltip");
    content.textContent = label.dataset.tooltip || "";
    trigger.addEventListener("click", event => {
      event.stopPropagation();
      document.querySelectorAll("label.tooltip-open").forEach(open => open.classList.remove("tooltip-open"));
      const open = !label.classList.contains("tooltip-open");
      label.classList.toggle("tooltip-open", open);
      trigger.setAttribute("aria-expanded", String(open));
    });
    label.append(trigger, content);
  });
}

function renderProviders()       {
  document.querySelector                ("#providers-empty-notice") .hidden = providers.length > 0;
  providers.forEach((provider, index) => { if (!provider.id && isKnownProvider(provider)) provider.id = knownProviderID(String(provider.type), index); });
  providerList.innerHTML = providers.map((provider, index) => `
    <article class="provider-card">
      <div class="provider-card-head"><div class="provider-card-title"><span class="provider-index">${index + 1}</span> <span data-provider-title="${index}">${value(provider,"id") || `Provider ${index + 1}`}</span></div><div class="provider-card-actions"><button class="test-provider button secondary" type="button" data-test="${index}">${subscriptionProvider(provider) ? "Check status" : "Test connection"}</button>${subscriptionProvider(provider) ? `<button class="login-provider button secondary" type="button" data-login="${index}">Log in</button>` : ""}<button class="remove-provider" type="button" data-remove="${index}">Remove</button></div></div>
      <div class="provider-result" data-result="${index}" role="status" aria-live="polite">Not checked yet</div>
      ${subscriptionProvider(provider) ? `<div class="grid two"><label data-tooltip="The stable ID used as the prefix of public model names. Built-in providers receive this automatically; custom API endpoints must define it.">Provider ID<input data-provider="${index}" data-key="id" value="${value(provider,"id")}" readonly required /></label><label data-tooltip="The subscription integration used by Torii. OAuth credentials are encrypted in the gateway database.">Type<select data-provider="${index}" data-key="type"><option ${provider.type === "openai-codex" ? "selected" : ""} value="openai-codex">OpenAI Codex</option><option ${provider.type === "nous" ? "selected" : ""} value="nous">Nous</option></select></label></div><p class="provider-subscription-note">This subscription provider uses its OAuth login. API URL, API keys, and discovery controls are not used.</p>` : `<div class="grid three">
        <label data-tooltip="The stable ID used as the prefix of public model names. Built-in providers receive this automatically; custom API endpoints must define it.">Provider ID<input data-provider="${index}" data-key="id" value="${value(provider,"id")}" ${isKnownProvider(provider) ? "readonly" : ""} required /></label>
        <label data-tooltip="The provider integration Torii should use. Jev and Jev-compatible providers supply classifier models, not chat models.">Type<select data-provider="${index}" data-key="type"><option ${provider.type === "openai_compatible" ? "selected" : ""} value="openai_compatible">OpenAI compatible</option><option ${provider.type === "jev" ? "selected" : ""} value="jev">TypeSafe AI · Jev</option><option ${provider.type === "jev_compatible" ? "selected" : ""} value="jev_compatible">Jev compatible · Laya / custom</option><option ${provider.type === "openrouter" ? "selected" : ""} value="openrouter">OpenRouter</option><option ${provider.type === "gemini" ? "selected" : ""} value="gemini">Gemini</option><option ${provider.type === "xai" ? "selected" : ""} value="xai">xAI</option><option ${provider.type === "anthropic" ? "selected" : ""} value="anthropic">Anthropic</option><option ${provider.type === "openai" ? "selected" : ""} value="openai">OpenAI</option><option ${provider.type === "openai-codex" ? "selected" : ""} value="openai-codex">OpenAI Codex</option><option ${provider.type === "ollama" ? "selected" : ""} value="ollama">Ollama</option><option ${provider.type === "nous" ? "selected" : ""} value="nous">Nous</option></select></label>
        <label data-tooltip="The complete upstream URL Torii should call. Include any required API path yourself; Torii does not add /v1 to this field."><span>API URL</span><input data-provider="${index}" data-key="api_url" value="${value(provider,"api_url")}" placeholder="https://api.example.com" /></label>
        ${provider.type === "ollama" ? "" : `<label data-tooltip="Enter an upstream API key to replace the configured credential. Save encrypts it in SQLite; saved keys cannot be revealed.">API key<span class="password-field"><input id="provider-api-key-${index}" type="password" autocomplete="new-password" data-provider="${index}" data-key="api_key" placeholder="${provider.bearer_token ? "Configured · leave blank to keep" : "Enter provider API key"}" /><button class="reveal-button" type="button" data-reveal-provider-key="${index}" aria-controls="provider-api-key-${index}" aria-label="Reveal entered provider API key" aria-pressed="false">Reveal</button></span></label>`}
      </div>
      <details class="advanced-provider"><summary>Advanced provider options</summary><div class="grid three">
        <label data-tooltip="An existing encrypted provider credential reference. Gateway client API keys cannot be used here.">Stored API credential<input data-provider="${index}" data-key="credential_id" value="${escapeHTML(String((provider.bearer_token       )?.credential || provider.credential_id || ""))}" placeholder="Credential ID" /></label>
        <label data-tooltip="An existing encrypted management credential used for quota lookup. This is separate from gateway client API keys.">Management credential<input data-provider="${index}" data-key="management_id" value="${escapeHTML(String((provider.management_token       )?.credential || provider.management_id || ""))}" placeholder="Credential ID" /></label>
        <label data-tooltip="The portal URL used for subscription or account management. It is relevant to providers with a separate portal.">Portal URL<input data-provider="${index}" data-key="portal_url" value="${value(provider,"portal_url")}" placeholder="https://portal.example.com" /></label>
        <label data-tooltip="The OAuth client identifier used by a provider portal. Leave empty to use the provider default.">Client ID<input data-provider="${index}" data-key="client_id" value="${value(provider,"client_id")}" /></label>
        <label data-tooltip="A static fallback model list used when discovery is disabled or unavailable. Separate model IDs with commas.">Models (comma separated)<input data-provider="${index}" data-key="models" value="${value(provider,"models")}" placeholder="model-a, model-b" /></label>
        <label data-tooltip="The transport mode for provider-specific integrations. Leave empty for the provider default.">Transport<input data-provider="${index}" data-key="transport" value="${value(provider,"transport")}" placeholder="direct or provider" /></label>
        <label data-tooltip="Optional organization identifier sent to providers that support organization scoping.">Organization<input data-provider="${index}" data-key="organization" value="${value(provider,"organization")}" /></label>
        <label data-tooltip="Optional project identifier sent to providers that support project scoping.">Project<input data-provider="${index}" data-key="project" value="${value(provider,"project")}" /></label>
        <label data-tooltip="JSON map of custom header names to secret references. Headers are added to upstream requests.">Headers JSON<input data-provider="${index}" data-key="headers" value='${value(provider,"headers")}' placeholder='{"X-Header":{"env":"ENV_VAR"}}' /></label>
        <label data-tooltip="JSON map of public model IDs to upstream model IDs. Use this to expose friendly aliases.">Model rewrites JSON<input data-provider="${index}" data-key="rewrites" value='${value(provider,"rewrites")}' placeholder='{"public":"upstream"}' /></label>
        <label data-tooltip="A custom CA bundle for TLS verification. Leave empty to use the system trust store.">CA file<input data-provider="${index}" data-key="tls_ca" value="${value(provider,"tls_ca")}" /></label>
        <label data-tooltip="Overrides the TLS server name used during certificate verification. Leave empty for the URL hostname.">TLS server name<input data-provider="${index}" data-key="tls_server" value="${value(provider,"tls_server")}" /></label>
        <label data-tooltip="Maximum time allowed to establish an upstream connection. Use a Go duration such as 5s.">Connect timeout<input data-provider="${index}" data-key="connect_timeout" value="${value(provider,"connect_timeout")}" placeholder="5s" /></label>
        <label data-tooltip="Maximum time allowed for an upstream request. Use a Go duration such as 60s.">Request timeout<input data-provider="${index}" data-key="request_timeout" value="${value(provider,"request_timeout")}" placeholder="60s" /></label>
        <label data-tooltip="Maximum time allowed while discovering the provider model catalog. Use a Go duration such as 10s.">Discovery timeout<input data-provider="${index}" data-key="discovery_timeout" value="${value(provider,"discovery_timeout")}" placeholder="10s" /></label>
        <label class="checkbox" data-tooltip="When enabled, Torii queries the provider for its available models. Disable this only when you provide a static model list."><input data-provider="${index}" data-key="discover" type="checkbox" ${checked(provider,"discover")} /> Discover models</label>
      </div></details>`}
    </article>`).join("");
  providerList.querySelectorAll                   ("[data-remove]").forEach(button => button.addEventListener("click", () => { providers.splice(Number(button.dataset.remove), 1); renderProviders(); markDirty(); }));
  providerList.querySelectorAll                   ("[data-test]").forEach(button => button.addEventListener("click", () => { void testProvider(Number(button.dataset.test)); }));
  providerList.querySelectorAll                   ("[data-login]").forEach(button => button.addEventListener("click", () => { void loginProvider(Number(button.dataset.login), button); }));
  providerList.querySelectorAll                                      ("[data-provider]").forEach(control => control.addEventListener("input", updateProvider));
  providerList.querySelectorAll                  ('[data-key="api_key"]').forEach(input => {
    // Assign the property rather than including credentials in HTML attributes.
    input.value = String(providers[Number(input.dataset.provider)].api_key || "");
  });
  providerList.querySelectorAll                   ("[data-reveal-provider-key]").forEach(button => button.addEventListener("click", () => {
    const input = providerList.querySelector                  (`#provider-api-key-${button.dataset.revealProviderKey}`) ;
    const visible = input.type === "password";
    input.type = visible ? "text" : "password";
    button.textContent = visible ? "Hide" : "Reveal";
    button.setAttribute("aria-label", visible ? "Hide entered provider API key" : "Reveal entered provider API key");
    button.setAttribute("aria-pressed", String(visible));
  }));
  providers.forEach(provider => {
    const status = providerStatuses.get(provider);
    if (status) setProviderStatus(provider, status);
  });
  initializeTooltips();
  updateClassifierOptions();
}

function updateProvider(event       )       {
  const control = event.target                                        ;
  const index = Number(control.dataset.provider);
  const key = control.dataset.key ;
  const previousType = providers[index].type;
  providers[index][key] = control instanceof HTMLInputElement && control.type === "checkbox" ? control.checked : control.value;
  if (["api_key", "api_key_env", "api_key_file"].includes(key)) {
    delete providers[index].bearer_token;
    ["api_key", "api_key_env", "api_key_file"].filter(source => source !== key).forEach(source => {
      providers[index][source] = "";
      const input = providerList.querySelector                  (`[data-provider="${index}"][data-key="${source}"]`);
      if (input) input.value = "";
    });
  }
  if (key === "management_env") delete providers[index].management_token;
  if(key==="credential_id"){providers[index].api_key="";providers[index].api_key_env="";providers[index].api_key_file="";providers[index].bearer_token=control.value.trim()?{credential:control.value.trim()}       :undefined       ;}
  if(key==="management_id"){providers[index].management_token=control.value.trim()?{credential:control.value.trim()}       :undefined       ;}
  setProviderStatus(providers[index], { state: "unchecked", message: "Settings changed · check connection again" });
  if (key === "id") {
    const title = providerList.querySelector             (`[data-provider-title="${index}"]`);
    if (title) title.textContent = control.value || `Provider ${index + 1}`;
  }
  if (key === "type") {
    const previousID = String(providers[index].id || "");
    const previousBase = knownProviderIDs[String(previousType)] || "";
    if (previousBase && (previousID === previousBase || /^.+-\d+$/.test(previousID))) providers[index].id = "";
    if (control.value === "openai-codex" && (!providers[index].api_url || providers[index].models === "setup")) {
      providers[index].api_url = "https://chatgpt.com/backend-api/codex";
      providers[index].models = "gpt-5.3-codex";
    }
    if (control.value === "nous" && !providers[index].api_url) providers[index].api_url = "https://inference-api.nousresearch.com/v1";
    if (classifierProvider(providers[index])) {
      providers[index].api_url = control.value === "jev" ? "https://api.typesafe.ai/v1" : "";
      providers[index].api_key = "";
      providers[index].api_key_env = "";
      providers[index].api_key_file = "";
      providers[index].models = "";
      providers[index].management_env = "";
      providers[index].portal_url = "";
      providers[index].client_id = "";
      providers[index].headers = "";
      providers[index].discover = true;
      delete providers[index].bearer_token;
      delete providers[index].management_token;
      if (control.value === "jev_compatible") providers[index].id = "";
    }
    if (knownProviderIDs[control.value]) providers[index].id = knownProviderID(control.value, index);
    renderProviders();
    void testProvider(index);
  }
  markDirty();
}

function markDirty()       { saveState.textContent = "Unsaved changes"; }

// Recognize only the exact inert provider generated by earlier console bootstraps.
// Real providers (even those named setup) must remain visible and testable.
function isSetupPlaceholder(provider                     )          {
  const fields = ["id", "type", "base_url", "api_path", "discover_models", "models"];
  return provider.id === "setup" && provider.type === "openai_compatible"
    && provider.base_url === "https://example.invalid" && provider.api_path === "/v1"
    && provider.discover_models === false && Array.isArray(provider.models)
    && provider.models.length === 1 && provider.models[0] === "setup"
    && Object.keys(provider).every(key => fields.includes(key));
}

function applyConfig(config                     )       {
  classifierDiscoveryPending = true;
  configurationLoaded = true;
  legacyClassifier = null;
  const classifier = config.router?.classifier || {};
  requestedClassifier = classifier.type === "disabled" ? "disabled" : classifier.type === "jev" && classifier.transport === "provider"
    ? `${classifier.provider}/${classifier.model}` : "";
  applyBenchmarkConfig(config.intelligence || {});
  providers = (config.providers || []).filter((provider                     ) => !isSetupPlaceholder(provider)).map((provider                     ) => ({
    ...providerDefaults,
    ...provider,
    api_key: "",
    api_key_env: provider.bearer_token?.env || "",
    api_key_file: provider.bearer_token?.file || "",
    management_env: provider.management_token?.env || "",
    tls_ca: provider.tls?.ca_file || "",
    tls_server: provider.tls?.server_name || "",
    api_url: `${String(provider.base_url || "").replace(/\/$/, "")}${provider.api_path || ""}`,
    headers: provider.headers ? JSON.stringify(provider.headers) : "",
    rewrites: provider.model_rewrites ? JSON.stringify(provider.model_rewrites) : "",
    models: Array.isArray(provider.models) ? provider.models.join(", ") : "",
    discover: provider.discover_models !== false,
  }));
  // Move legacy direct Jev connections into provider drafts, retaining secret
  // references (including key files) rather than dropping them on the next save.
  if (classifier.type === "jev" && classifier.transport !== "provider") {
    const type = new URL(classifier.base_url).hostname === "api.typesafe.ai" ? "jev" : "jev_compatible";
    const id = type === "jev" ? knownProviderID("jev", -1) : `classifier-${providers.length + 1}`;
    providers.push({ ...providerDefaults, id, type, api_url:classifier.base_url, bearer_token:classifier.api_key, api_key:classifier.api_key?.value || "", api_key_env:classifier.api_key?.env || "", api_key_file:classifier.api_key?.file || "", models:classifier.model || "jev-latest", discover:true });
    requestedClassifier = `${id}/${classifier.model || "jev-latest"}`;
  } else if (classifier.type && classifier.type !== "jev" && classifier.type !== "disabled") {
    legacyClassifier = { ...classifier };
    requestedClassifier = classifier.model || "existing-classifier";
  }
  providers.forEach(provider => {
    if (provider.type === "openai-codex") {
      if (provider.models === "setup") provider.models = "gpt-5.3-codex";
      if (String(provider.api_url).startsWith("https://example.invalid")) provider.api_url = "https://chatgpt.com/backend-api/codex";
    }
  });
  renderProviders();
  const fill = (value     , prefix = "")       => Object.entries(value || {}).forEach(([key, raw]) => {
    const name = prefix ? `${prefix}.${key}` : key;
    if (raw && typeof raw === "object" && !Array.isArray(raw)) fill(raw, name);
    else {
      const control = form.elements.namedItem(name)                                                                     ;
      if (control) control.type === "checkbox" ? control.checked = Boolean(raw) : control.value = String(raw);
    }
  });
  fill(config);
  const allowances = config.router?.quota?.allowances || {};
  (form.elements.namedItem("router.quota.allowances")                       ).value = Object.entries(allowances).map(([key, raw]) => `${key} = ${raw}`).join("\n");
  const quality = config.router?.selection?.minimum_quality;
  if (quality) (form.elements.namedItem("router.selection.minimum_quality")                    ).value = JSON.stringify(quality);
  updateClassifierOptions();
  if (location.hash === "#intelligence" && benchmarksEnabledSaved) void loadBenchmarkData();
}

async function loadServerConfig()                {
  const login = adminAuthorization;
  try {
    const response = await gatewayFetch("/gateway/v1/config", { headers: await authProvider.headers() });
    if (login !== adminAuthorization) return;
    if (!response.ok) { if (response.status === 401 || response.status === 403) {signOutConsole();saveState.textContent="Sign in to configure the gateway";document.querySelector                   ("#login-dialog")?.showModal?.();} return; }
    const configuration = await response.json();
    if (login !== adminAuthorization) return;
    applyConfig(configuration);
    saveState.textContent = "Loaded from gateway";
    void testAllProviders();
  } catch { saveState.textContent = "Gateway config unavailable"; }
}

async function saveConfig()                {
  let configuration                     ;
  try { configuration = readConfig(); }
  catch (error) { toast(error instanceof Error ? error.message : "Invalid benchmark configuration JSON"); return; }
  const response = await gatewayFetch("/gateway/v1/config", { method: "PUT", headers: { "Content-Type": "application/json", ...await authProvider.headers() }, body: JSON.stringify(configuration) });
  if (!response.ok) { toast(redactProviderKeys(`Save failed: ${await response.text()}`)); return; }
  providers.forEach(provider=>{provider.api_key="";});
  clearSubmittedBenchmarkKeys();
  saveState.textContent = "Saved and reloaded";
  await loadServerConfig();
  toast("Configuration saved and reloaded");
}

function navigate()       {
  const current = location.hash.replace(/^#/, "") || "overview";
  document.querySelectorAll             (".page").forEach(page => {
    const active = page.id === current;
    page.classList.toggle("active", active);
    page.hidden = !active;
    page.setAttribute("aria-hidden", String(!active));
  });
  document.querySelectorAll                   (".nav-link").forEach(link => link.classList.toggle("active", link.getAttribute("href") === `#${current}`));
  if (current === "routing") updateClassifierOptions();
  if (current === "intelligence" && configurationLoaded && benchmarksEnabledSaved) void loadBenchmarkData();
}

function setNested(target                     , path        , raw         )       {
  const parts = path.split("."); let cursor = target;
  parts.forEach((part, index) => { if (index === parts.length - 1) cursor[part] = raw; else cursor = cursor[part] ||= {}; });
}
function readProviderConfig(provider                                  )                      {
  if (provider.type === "ollama") {
    // Ollama does not accept the generic advanced fields or discover_models.
    // Keep an existing Ollama alongside a newly added classifier saveable.
    const url = new URL(String(provider.api_url));
    return { id:provider.id, type:provider.type, base_url:`${url.protocol}//${url.host}${url.pathname === "/" ? "" : url.pathname}` };
  }
  const output                      = {};
  Object.entries(provider).forEach(([key, raw]) => {
    // The API URL is the source of truth; do not overwrite it with fields
    // retained from the loaded YAML after converting it for the form.
    if (key === "base_url" || key === "api_path" || key === "discover_models") return;
    if (["api_key", "api_key_env", "api_key_file", "bearer_token","credential_id","management_id"].includes(key)) return;
    if (key === "discover") { output.discover_models = Boolean(raw); return; }
    if (String(raw).trim() === "") return;
    const map                         = { management_env: "management_token", tls_ca: "tls.ca_file", tls_server: "tls.server_name" };
    const mapped = map[key] || key;
    if (key === "api_url") {
      const url = new URL(String(raw));
      if (subscriptionProvider(provider) || provider.type === "ollama" || provider.type === "anthropic") {
        output.base_url = `${url.protocol}//${url.host}${url.pathname === "/" ? "" : url.pathname}`;
      } else {
        output.base_url = `${url.protocol}//${url.host}`;
        output.api_path = url.pathname === "/" ? "" : url.pathname;
      }
      return;
    }
    if (key === "models") output.models = String(raw).split(",").map(v => v.trim()).filter(Boolean);
    else if (key === "headers" || key === "rewrites") {
      try { output[key === "headers" ? "headers" : "model_rewrites"] = JSON.parse(String(raw)); }
      catch { output[key === "headers" ? "headers" : "model_rewrites"] = {}; }
    } else if (key.endsWith("_env")) setNested(output, mapped, { env: raw });
    else setNested(output, mapped, raw);
  });
  if (!subscriptionProvider(provider)) {
    if (provider.api_key) output.bearer_token = { value:provider.api_key };
    else if (provider.api_key_env) output.bearer_token = { env:provider.api_key_env };
    else if (provider.api_key_file) output.bearer_token = { file:provider.api_key_file };
    else if (provider.credential_id) output.bearer_token={credential:provider.credential_id};
    else if (provider.bearer_token) output.bearer_token=provider.bearer_token;
  }
  if (provider.management_id) output.management_token={credential:provider.management_id};
  return output;
}
function readConfig()                      {
  const config                      = { server: {}, state: {}, providers: [], router: { classifier: {}, sessions: {}, quota: {}, selection: {} } };
  new FormData(form).forEach((raw, name) => { if (name === "router.quota.allowances") return; if (name === "router.allow_paid_models") setNested(config, name, true); else if (name === "router.selection.minimum_quality") { try { setNested(config, name, JSON.parse(String(raw))); } catch { /* keep invalid drafts editable */ } } else if (String(raw).trim() !== "") setNested(config, name, raw); });
  config.providers = providers.map(readProviderConfig);
  const selected = classifierSelect.value;
  const onFailure = config.router.classifier.on_failure;
  if (legacyClassifier && selected === requestedClassifier) {
    config.router.classifier = { ...legacyClassifier, ...(onFailure ? { on_failure:onFailure } : {}) };
  } else if (selected && selected !== "disabled") {
    const slash = selected.indexOf("/");
    config.router.classifier = { type:"jev", transport:"provider", provider:selected.slice(0, slash), model:selected.slice(slash + 1), ...(onFailure ? { on_failure:onFailure } : {}) };
  } else if (selected === "disabled") {
    config.router.classifier = { type:"disabled", ...(onFailure ? { on_failure:onFailure } : {}) };
  } else {
    config.router.classifier = onFailure ? { on_failure:onFailure } : {};
  }
  const allowances = String((form.elements.namedItem("router.quota.allowances")                       )?.value || "").split("\n").map(line => line.split("=").map(v => v.trim())).filter(pair => pair.length === 2 && pair[0]).reduce((all, [key, raw]) => { const value = Number(raw); if (Number.isFinite(value)) all[key] = value; return all; }, {}                          );
  if (Object.keys(allowances).length) config.router.quota.allowances = allowances;
  config.intelligence = readBenchmarkConfiguration();
  return config;
}

function updateBenchmarkControls()       {
  const enabled = document.querySelector                  ("#intelligence-enabled") .checked;
  document.querySelector                   ("#refresh-benchmarks") .disabled = !benchmarksEnabledSaved || !enabled || benchmarkDirty || benchmarkRefreshRunning;
  document.querySelector                ("#benchmark-configuration") .hidden = !enabled;
  document.querySelector                ("#benchmark-data") .hidden = !benchmarksEnabledSaved;
  document.querySelector("#benchmark-enable-notice") .textContent = enabled !== benchmarksEnabledSaved || benchmarkDirty
    ? "Click Save & reload to apply your benchmark intelligence setting before refreshing."
    : enabled ? "Benchmark intelligence is enabled. Choose sources below, then Save & reload to apply changes. Refresh uses saved settings."
    : "Benchmark intelligence is disabled. Enable it and click Save & reload before refreshing.";
}

async function testProvider(index        )                {
  const provider = providers[index];
  if (!provider) return;
  const pending                 = { state: "pending", message: "Checking connection and discovering models…" };
  setProviderStatus(provider, pending);
  try {
    if (!provider.id || !provider.api_url) throw new Error("Configure the provider before checking its connection");
    // Snapshot the draft before awaiting, so a re-render cannot change which
    // provider this request tests. Ignore responses for edited or removed drafts.
    const configuration = readProviderConfig(provider);
    const response = await gatewayFetch("/gateway/v1/providers/test", { method: "POST", headers: { "Content-Type": "application/json", ...await authProvider.headers() }, body: JSON.stringify({ provider: configuration }) });
    const body = await response.json();
    if (!response.ok) throw new Error(body.error || "connection failed");
    if (body.models !== null && !Array.isArray(body.models)) throw new Error("Invalid model discovery response");
    const count = body.models?.length || 0;
    if (providerStatuses.get(provider) !== pending || !providers.includes(provider)) return;
    const connection = subscriptionProvider(provider) ? "Logged in" : "Connected";
    if (classifierProvider(provider)) {
      const ids = (body.models || []).map((model             ) => model.id);
      if (ids.some((id         ) => typeof id !== "string" || !id.startsWith(`${provider.id}/`))) throw new Error("Invalid classifier model IDs");
      classifierCatalogs.set(provider, ids);
    }
    setProviderStatus(provider, {
      state: count > 0 ? "success" : "failure",
      message: `${connection} · ${count > 0 ? "Working" : "Not working"} · ${count} model${count === 1 ? "" : "s"} discovered${count === 0 ? " (no models available)" : ""}`,
    });
  } catch (error) {
    if (providerStatuses.get(provider) !== pending || !providers.includes(provider)) return;
    setProviderStatus(provider, { state: "failure", message: `Not working · 0 models discovered · ${error instanceof Error ? error.message : "connection failed"}` });
  }
}
async function loginProvider(index        , button                   )                {
  const result = providerList.querySelector                (`[data-result="${index}"]`) ;
  const provider = providers[index];
  button.disabled = true;
  setProviderStatus(provider, { state: "pending", message: "Starting login…" });
  try {
    const configuration = readProviderConfig(provider);
    const response = await gatewayFetch("/gateway/v1/providers/login", { method: "POST", headers: { "Content-Type": "application/json", ...await authProvider.headers() }, body: JSON.stringify({ provider: configuration }) });
    const body = await response.json();
    if (!response.ok) throw new Error(body.error || "login could not start");
    if (body.helper_url && body.helper_token) {
      renderHelperInstructions(result, body.helper_url, body.helper_token);
      await pollProviderLogin(provider, result, button, "", true);
      return;
    }
    if (body.verification_url) window.open(body.verification_url, "_blank", "noopener,noreferrer");
    result.textContent = body.user_code ? `${body.message}. Code: ${body.user_code}` : body.message || "Waiting for login…";
    await pollProviderLogin(provider, result, button, body.verification_url || "");
  } catch (error) {
    setProviderStatus(provider, { state: "failure", message: `Not working · 0 models discovered · ${error instanceof Error ? error.message : "login failed"}` });
    button.disabled = false;
  }
}
function renderHelperInstructions(result                , sessionURL        , pairingToken        )       {
  const command = `torii-auth --session-url ${sessionURL} --pairing-token ${pairingToken}`;
  result.className = "provider-result pending provider-helper";
  const downloadURL = helperDownloadURL();
  result.innerHTML = `<strong>Download and run the one-shot helper on the computer with your browser.</strong><a href="${downloadURL}" download>Download torii-auth for this computer</a><span>${escapeHTML(command)}</span><button class="copy-helper button quiet" type="button">Copy command</button><small>Browsers cannot launch executables automatically. Run the downloaded helper once; it opens OpenAI login locally, imports the credential, then exits. This pairing expires in 10 minutes.</small>`;
  result.querySelector                   ("a")?.addEventListener("click",async event=>{if(!downloadURL.startsWith("/"))return;event.preventDefault();try{const response=await fetch(downloadURL,{headers:await authProvider.headers()});if(!response.ok)throw new Error();const url=URL.createObjectURL(await response.blob());const a=document.createElement("a");a.href=url;a.download=downloadURL.split("/").pop() ;a.click();URL.revokeObjectURL(url);}catch{toast("Helper download failed; sign in and retry");}});
  result.querySelector                   (".copy-helper")?.addEventListener("click", async event => {
    const button = event.currentTarget                     ;
    try {
      await navigator.clipboard.writeText(command);
      button.textContent = "Copied";
    } catch {
      button.textContent = "Copy unavailable";
    }
  });
}
function helperDownloadURL()         {
  const platform = navigator.platform.toLowerCase();
  const agent = navigator.userAgent.toLowerCase();
  let asset = "";
  if (platform.includes("win")) asset = "torii-auth-windows-amd64.exe";
  else if (platform.includes("mac") || agent.includes("macintosh")) asset = agent.includes("arm") ? "torii-auth-darwin-arm64" : "torii-auth-darwin-amd64";
  else if (agent.includes("aarch64") || agent.includes("arm64")) asset = "torii-auth-linux-arm64";
  else if (platform.includes("linux")) asset = "torii-auth-linux-amd64";
  return asset ? `/gateway/v1/providers/login/helper/${asset}` : "https://github.com/Apartment38/torii-router/releases/latest";
}
async function pollProviderLogin(provider                                  , result                , button                   , openedURL = "", helperPending = false)                {
  const providerID = String(provider.id);
  for (let attempt = 0; attempt < 180; attempt += 1) {
    await new Promise(resolve => window.setTimeout(resolve, 2000));
    const response = await gatewayFetch(`/gateway/v1/providers/login?provider=${encodeURIComponent(providerID)}`, { headers: await authProvider.headers() });
    const body = await response.json();
    if (body.verification_url && body.verification_url !== openedURL) { openedURL = body.verification_url; window.open(openedURL, "_blank", "noopener,noreferrer"); }
    if (body.status === "success") {
      button.disabled = false;
      await testProvider(providers.indexOf(provider));
      return;
    }
    if (body.status === "failure") { setProviderStatus(provider, { state: "failure", message: `Not working · 0 models discovered · ${body.message || "login failed"}` }); button.disabled = false; return; }
    if (!helperPending) {
      setProviderStatus(provider, { state: "pending", message: body.user_code ? `${body.message}. Code: ${body.user_code}` : body.message || "Waiting for login…" });
    }
  }
  setProviderStatus(provider, { state: "failure", message: "Not working · 0 models discovered · login timed out" });
  button.disabled = false;
}
async function testAllProviders()                {
  classifierDiscoveryPending = true;
  updateClassifierOptions();
  try { await Promise.all(providers.map((_, index) => testProvider(index))); }
  finally { classifierDiscoveryPending = false; updateClassifierOptions(); }
}
function toast(message        )       { const node = document.querySelector                ("#toast") ; node.textContent = message; node.classList.add("show"); window.setTimeout(() => node.classList.remove("show"), 2400); }

document.querySelector("#add-provider") .addEventListener("click", () => { providers.push({ ...providerDefaults }); renderProviders(); markDirty(); });
document.querySelector("#refresh-benchmarks") .addEventListener("click", () => { void inspectBenchmarks(true); });
document.querySelector("#inspect-benchmarks") .addEventListener("click", () => { void inspectBenchmarks(false); });
document.querySelector("#save-button") .addEventListener("click", () => { void saveConfig(); });
document.querySelector("#reset-button") .addEventListener("click", () => { location.reload(); });
document.querySelector("#theme-toggle") .addEventListener("click", () => { const dark = document.documentElement.dataset.theme !== "dark"; document.documentElement.dataset.theme = dark ? "dark" : "light"; localStorage.setItem("torii-theme", dark ? "dark" : "light"); });
form.addEventListener("input", markDirty);
window.addEventListener("hashchange", navigate);
document.documentElement.dataset.theme = localStorage.getItem("torii-theme") || (matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light");
classifierSelect.addEventListener("change", () => { requestedClassifier = classifierSelect.value; legacyClassifier = null; markDirty(); });
document.addEventListener("click", () => { document.querySelectorAll("label.tooltip-open").forEach(label => label.classList.remove("tooltip-open")); });
initializeBenchmarks(); renderProviders(); updateConsoleIdentity(); updateBenchmarkControls(); navigate(); void checkSetup();

document.querySelector("#console-login") .addEventListener("click",()=>document.querySelector                   ("#login-dialog") .showModal());
document.querySelector("#console-logout") .addEventListener("click",signOutConsole);
document.querySelector("#login-form") .addEventListener("submit",async event=>{
  event.preventDefault();const username=document.querySelector                  ("#login-username") .value;const input=document.querySelector                  ("#login-password") ;adminAuthorization=basicAuthorization(username,input.value);input.value="";
  try{const response=await gatewayFetch("/gateway/v1/config",{headers:await authProvider.headers()});if(!response.ok)throw new Error();applyConfig(await response.json());signedInUsername=username;updateConsoleIdentity();document.querySelector("#login-error") .textContent="";document.querySelector                   ("#login-dialog") .close();saveState.textContent="Signed in";void testAllProviders();void loadCredentialMetadata();}
  catch{signOutConsole();document.querySelector("#login-error") .textContent="Sign-in failed. Check your administrator username and password, then try again in one minute if rate limited.";}
});
async function loadCredentialMetadata()              {
  const list=document.querySelector             ("#credential-list") ;
  const login=adminAuthorization;
  try{const response=await gatewayFetch("/gateway/v1/admin/api-keys",{headers:await authProvider.headers()});if(!response.ok)throw new Error();const body=await response.json();if(login!==adminAuthorization)return;list.innerHTML=(body.api_keys||[]).map((key    )=>`<div class="card"><strong>${escapeHTML(key.name)}</strong><p>Created ${escapeHTML(key.created_at)}</p><button type="button" class="button quiet" data-delete-key="${escapeHTML(key.id)}">Delete</button></div>`).join("") || "No gateway API keys have been created. Click Add credential to generate one.";
    list.querySelectorAll                   ("[data-delete-key]").forEach(button=>button.addEventListener("click",async()=>{if(confirm("Delete this API key? Clients using it will immediately lose access on their next request."))await deleteClientKey(button.dataset.deleteKey );}));
  }catch{list.textContent="Sign in to manage gateway API keys.";}
}
async function deleteClientKey(id       )              {
  try{const response=await gatewayFetch(`/gateway/v1/admin/api-keys/${encodeURIComponent(id)}`,{method:"DELETE",headers:await authProvider.headers()});if(!response.ok)throw new Error();toast("API key deleted and invalidated");await loadCredentialMetadata();}catch{toast("API key deletion failed. Sign in as the administrator.");}
}
document.querySelector("#refresh-credentials") .addEventListener("click",()=>{void loadCredentialMetadata();});
document.querySelector("#create-credential") .addEventListener("click",()=>{document.querySelector                  ("#client-key-name") .value="";document.querySelector("#create-key-error") .textContent="";document.querySelector                   ("#create-key-dialog") .showModal();});
document.querySelector("#cancel-create-key") .addEventListener("click",()=>document.querySelector                   ("#create-key-dialog") .close());
document.querySelector("#create-key-form") .addEventListener("submit",async event=>{
  event.preventDefault();
  const button=document.querySelector                   ("#generate-client-key") ;
  if(button.disabled)return;
  button.disabled=true;
  const login=adminAuthorization;
  const name=document.querySelector                  ("#client-key-name") .value.trim();
  try{const response=await gatewayFetch("/gateway/v1/admin/api-keys",{method:"POST",headers:{"Content-Type":"application/json",...await authProvider.headers()},body:JSON.stringify({name})});const result=await response.json();if(!response.ok)throw new Error(result.error||"API key creation failed");if(login!==adminAuthorization)return;document.querySelector                  ("#client-key-name") .value="";document.querySelector                   ("#create-key-dialog") .close();showNewAPIKey(result.api_key,result.warning);await loadCredentialMetadata();}
  catch(error){document.querySelector("#create-key-error") .textContent=error instanceof Error?error.message:"API key creation failed";}
  finally{button.disabled=false;}
});
document.querySelector("#intelligence-enabled") .addEventListener("change",()=>{
  benchmarkDirty = true; updateBenchmarkControls();markDirty();
});

async function checkSetup()              {
  try{const response=await fetch("/gateway/v1/setup",{cache:"no-store"});if(!response.ok)throw new Error();const status=await response.json();document.querySelector                   (status.setup_required?"#setup-dialog":"#login-dialog") .showModal();}
  catch{saveState.textContent="Unable to check administrator setup; refresh to retry.";}
}
function showNewAPIKey(key       ,warning        )     {
  if(!key){toast("Your existing API key was imported and preserved. Generate a replacement on the Server page if needed.");return;}
  document.querySelector                     ("#new-api-key") .value=key;
  document.querySelector("#new-key-warning") .textContent=warning||"This API key is displayed only once. Save it securely before closing.";
  document.querySelector                   ("#new-key-dialog") .showModal();
}
document.querySelector("#setup-form") .addEventListener("submit",async event=>{
  event.preventDefault();const field=(id       )=>document.querySelector                  (id) ;
  const username=field("#setup-username").value,password=field("#setup-password").value,code=field("#setup-code").value;
  if(password!==field("#setup-confirm").value){document.querySelector("#setup-error") .textContent="Passwords do not match.";return;}
  field("#setup-password").value="";field("#setup-confirm").value="";field("#setup-code").value="";
  try{const response=await fetch("/gateway/v1/setup",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({username,password,setup_code:code})});const result=await response.json();if(!response.ok)throw new Error(result.error||"Setup failed");adminAuthorization=basicAuthorization(username,password);signedInUsername=username;updateConsoleIdentity();document.querySelector                   ("#setup-dialog") .close();await loadServerConfig();void loadCredentialMetadata();showNewAPIKey(result.api_key,result.warning);}
  catch(error){document.querySelector("#setup-error") .textContent=error instanceof Error?error.message:"Administrator setup failed.";}
});
document.querySelector("#generate-api-key") .addEventListener("click",async()=>{
  if(!confirm("Generate a new API key? All clients using the current key will need updating."))return;
  const login=adminAuthorization;
  try{const response=await gatewayFetch("/gateway/v1/admin/api-key",{method:"POST",headers:await authProvider.headers()});if(!response.ok)throw new Error();const result=await response.json();if(login!==adminAuthorization)return;showNewAPIKey(result.api_key,result.warning);}
  catch{toast("API key generation failed. Sign in as the administrator.");}
});
function clearNewAPIKey()     {document.querySelector                     ("#new-api-key") .value="";}
document.querySelector("#new-key-dialog") .addEventListener("close",clearNewAPIKey);
document.querySelector("#new-key-dialog") .addEventListener("cancel",clearNewAPIKey);
document.querySelector("#close-new-key") .addEventListener("click",()=>{clearNewAPIKey();document.querySelector                   ("#new-key-dialog") .close();});

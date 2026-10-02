// Source forms and cached evidence views. No browser persistence or dependencies.
type BenchmarkSourceDraft = Record<string, any>;
let benchmarkConfiguration: Record<string, any> = {};
let benchmarkSources: BenchmarkSourceDraft[] = [];
let benchmarkSourceStatus: Record<string, any> = {};
let benchmarkSourceActivity: Record<string, string> = {};
let benchmarkModels: Record<string, any> = {};
let benchmarkRawRows: Record<string, any>[] = [];
let benchmarkRawTotal = 0;
let benchmarkPage = 0;
let benchmarkTableView = "models";
let benchmarkFilter = "";
let benchmarkDirty = false;
let benchmarkRefreshRunning = false;
let benchmarkAbort: AbortController | null = null;
let benchmarkReloadSequence = 0;
let benchmarkProgressLog: string[] = [];
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
const benchmarkOperations: Record<string, string> = { input_price:"USD_per_million_tokens", output_price:"USD_per_million_tokens", latency:"seconds", ttft:"seconds", throughput:"tokens_per_second", context_length:"tokens" };

function benchmarkMetricDrafts(metrics: Record<string, any> = {}): Record<string, any>[] {
  return Object.entries(metrics).map(([column, mapping]) => ({ column, ...mapping }));
}
function benchmarkDataset(type: string): Record<string, any> {
  if (type === "lmarena") return { dataset:"lmarena-ai/leaderboard-dataset", config:"text_style_control", split:"latest", model_field:"model_name", date_field:"leaderboard_publish_date", _metrics:[] };
  if (type === "livebench") return { dataset:"livebench/model_judgment", config:"default", split:"leaderboard", model_field:"model", date_field:"tstamp", _metrics:[] };
  return { dataset:"", config:"default", split:"train", model_field:"model", date_field:"", _metrics:[] };
}
function benchmarkDraft(source: Record<string, any>, fresh = false): BenchmarkSourceDraft {
  return { ...source, _new:fresh, api_key_input:"", _metrics:benchmarkMetricDrafts(source.metrics),
    datasets:(source.datasets || (["huggingface", "lmarena", "livebench"].includes(source.type) ? [benchmarkDataset(source.type)] : []))
      .map((dataset: Record<string, any>) => ({ ...dataset, _metrics:dataset._metrics || benchmarkMetricDrafts(dataset.metrics) })) };
}
function applyBenchmarkConfig(config: Record<string, any> = {}): void {
  benchmarkConfiguration = structuredClone(config);
  benchmarkSources = (config.sources || []).map((source: Record<string, any>) => benchmarkDraft(source));
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
  document.querySelector<HTMLInputElement>("#intelligence-enabled")!.checked = benchmarksEnabledSaved;
  document.querySelectorAll<HTMLInputElement>("[data-benchmark-global]").forEach(input => { input.value = String(config[input.dataset.benchmarkGlobal!] ?? ""); });
  renderBenchmarkSources();
  updateBenchmarkControls();
}
function clearBenchmarkConsole(): void {
  benchmarkAbort?.abort(); benchmarkAbort = null; benchmarkReloadSequence++;
  benchmarkConfiguration = {}; benchmarkSources = []; benchmarkSourceStatus = {}; benchmarkSourceActivity = {}; benchmarkModels = {}; benchmarkRawRows = [];
  benchmarkRawTotal = 0; benchmarkPage = 0; benchmarkFilter = ""; benchmarkTableView = "models";
  benchmarkDirty = false; benchmarkRefreshRunning = false; benchmarkProgressLog = [];
  benchmarkProgressCounts = {total:0,completed:0,failed:0};
  document.querySelector<HTMLInputElement>("#intelligence-enabled")!.checked = false;
  document.querySelector<HTMLDivElement>("#benchmark-refresh-panel")!.hidden = true;
  document.querySelector("#benchmark-refresh-log")!.textContent = "";
  document.querySelector<HTMLInputElement>("#benchmark-table-filter")!.value = "";
  document.querySelector<HTMLSelectElement>("#benchmark-table-view")!.value = "models";
  document.querySelectorAll<HTMLInputElement>("[data-benchmark-global]").forEach(input => { input.value = ""; });
  renderBenchmarkSources(); renderBenchmarkTable();
}
function benchmarkAttributes(source: number, field: string, dataset?: number, mapping?: number): string {
  return `data-benchmark-source="${source}" data-benchmark-field="${field}"${dataset === undefined ? "" : ` data-benchmark-dataset="${dataset}"`}${mapping === undefined ? "" : ` data-benchmark-mapping="${mapping}"`}`;
}
function benchmarkInput(source: number, field: string, label: string, raw: any, help: string, dataset?: number, mapping?: number, type = "text"): string {
  return `<label data-tooltip="${escapeHTML(help)}">${escapeHTML(label)}<input ${benchmarkAttributes(source, field, dataset, mapping)} type="${type}" value="${escapeHTML(String(raw ?? ""))}" /></label>`;
}
function benchmarkMappings(source: number, rows: Record<string, any>[], dataset?: number): string {
  return rows.map((row, index) => `<div class="benchmark-mapping"><div class="section-heading"><h4>Metric mapping ${index + 1}</h4><button type="button" class="button quiet" data-benchmark-action="remove-metric" ${benchmarkAttributes(source, "", dataset, index)}>Remove</button></div><div class="grid three">
    ${benchmarkInput(source, "column", "Upstream column / benchmark", row.column, "The numeric column or benchmark key in the upstream data. Nested dataset columns can use dot paths.", dataset, index)}
    <label data-tooltip="The capability this measurement describes. Arena measurements always remain preference evidence.">Category<select ${benchmarkAttributes(source, "category", dataset, index)}>${benchmarkCategories.map(category => `<option value="${category}" ${row.category === category ? "selected" : ""}>${category.replace(/_/g, " ")}</option>`).join("")}</select></label>
    ${benchmarkInput(source, "min", "Minimum score", row.min ?? 0, "The declared lower end of the score scale. This is not inferred from observed values.", dataset, index, "number")}
    ${benchmarkInput(source, "max", "Maximum score", row.max ?? 1, "The declared upper end of the score scale. Scores outside this range reject the source update.", dataset, index, "number")}
    <label class="checkbox" data-tooltip="Invert normalized scores for measurements where a smaller value is better. Operational measurements keep their original units."><input type="checkbox" ${benchmarkAttributes(source, "lower_is_better", dataset, index)} ${row.lower_is_better ? "checked" : ""} /> Lower is better</label>
    <label data-tooltip="For operational mappings, choose a canonical measurement; its unit is set automatically. Leave empty for capability scores.">Operational metric<select ${benchmarkAttributes(source, "metric", dataset, index)}><option value="">Not operational</option>${Object.entries(benchmarkOperations).map(([metric, unit]) => `<option value="${metric}" ${row.metric === metric ? "selected" : ""}>${metric} (${unit})</option>`).join("")}</select></label>
  </div></div>`).join("") + `<button type="button" class="button quiet" data-benchmark-action="add-metric" ${benchmarkAttributes(source, "", dataset)}>＋ Add metric mapping</button>`;
}
function renderBenchmarkSources(): void {
  const list = document.querySelector<HTMLDivElement>("#benchmark-sources-list")!;
  list.innerHTML = benchmarkSources.map((source, index) => {
    const definition = benchmarkDefinitions.find(item => item.type === source.type)!;
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
      ${datasets ? (source.datasets || []).map((dataset: Record<string, any>, datasetIndex: number) => `<div class="benchmark-dataset"><div class="section-heading"><h4>Dataset ${datasetIndex + 1}</h4><button type="button" class="button quiet" data-benchmark-action="remove-dataset" ${benchmarkAttributes(index, "", datasetIndex)}>Remove</button></div><div class="grid three">
        ${[ ["dataset", "Dataset ID", "The published Hugging Face dataset identifier, such as organization/leaderboard. It must contain published model results."], ["config", "Dataset configuration", "The Dataset Viewer configuration or subset name. This must match the published artifact."], ["split", "Split", "The published split containing benchmark rows, such as train or leaderboard."], ["model_field", "Model ID column", "The column containing each upstream model identifier. Nested fields may use dot paths."], ["date_field", "Evaluation date column", "The column containing the evaluation date. Leave empty when unavailable; retrieval-only freshness will be marked."], ["version", "Benchmark version", "An optional version label retained with each observation. This does not replace the evaluation date."] ].map(([field, label, help]) => benchmarkInput(index, field, label, dataset[field], help, datasetIndex)).join("")}
      </div>${source.type === "livebench" ? "<p>LiveBench uses published score, category and task columns with a 0–1 scale.</p>" : benchmarkMappings(index, dataset._metrics || [], datasetIndex)}</div>`).join("") + `<button type="button" class="button quiet" data-benchmark-action="add-dataset" ${benchmarkAttributes(index, "")} ${(source.datasets || []).length >= 8 ? "disabled" : ""}>＋ Add dataset</button>`
        : `<details class="advanced-provider"><summary>Benchmark category / scale overrides</summary>${benchmarkMappings(index, source._metrics || [])}</details>`}
      </div></article>`;
  }).join("");
  list.querySelectorAll<HTMLInputElement>('[data-benchmark-field="api_key_input"]').forEach(input => { input.value = benchmarkSources[Number(input.dataset.benchmarkSource)].api_key_input || ""; });
  renderBenchmarkSourceStatus(); initializeTooltips();
}
function renderBenchmarkSourceStatus(): void {
  benchmarkSources.forEach((source, index) => {
    const node = document.querySelector<HTMLElement>(`[data-benchmark-status="${index}"]`);
    if (!node) return;
    const status = benchmarkSourceStatus[source.id];
    const success = status?.last_success && !String(status.last_success).startsWith("0001-");
    const activity = benchmarkSourceActivity[source.id];
    node.textContent = (activity ? `${activity} · ` : "") + (!status ? (source.enabled ? "Not refreshed yet" : "Disabled · no cached results")
      : `${status.enabled ? (status.error ? "Refresh failed" : success ? (status.stale ? "Cached data is stale" : "Working") : "Not refreshed yet") : "Disabled"} · ${status.observation_count || 0} observations${success ? ` · Last successful update ${status.last_success} · Cache age ${benchmarkNumber(status.age_seconds)} s` : ""}${status.error ? ` · ${status.error}` : ""}`);
    node.className = `provider-result ${status?.error || activity === "Update failed; previous cache retained" ? "failure" : success && !status.stale && status.enabled ? "success" : ""}`;
  });
}
function updateBenchmarkSource(event: Event): void {
  const input = event.target as HTMLInputElement | HTMLSelectElement;
  if (input.dataset.benchmarkSource === undefined || !input.dataset.benchmarkField) return;
  const source = benchmarkSources[Number(input.dataset.benchmarkSource)];
  const dataset = input.dataset.benchmarkDataset === undefined ? undefined : source.datasets[Number(input.dataset.benchmarkDataset)];
  const target = input.dataset.benchmarkMapping === undefined ? (dataset || source) : (dataset || source)._metrics[Number(input.dataset.benchmarkMapping)];
  const field = input.dataset.benchmarkField;
  target[field] = input.type === "checkbox" ? (input as HTMLInputElement).checked : input.value;
  if (field === "metric" && input.value) { target.category = "operational"; target.unit = benchmarkOperations[input.value]; }
  benchmarkDirty = true; markDirty(); updateBenchmarkControls();
  if (field === "enabled" || field === "metric") renderBenchmarkSources();
}
function benchmarkSourceAction(event: Event): void {
  const button = (event.target as HTMLElement).closest<HTMLButtonElement>("[data-benchmark-action]");
  if (!button || button.disabled) return;
  const source = benchmarkSources[Number(button.dataset.benchmarkSource)];
  const datasetIndex = button.dataset.benchmarkDataset === undefined ? undefined : Number(button.dataset.benchmarkDataset);
  const target = datasetIndex === undefined ? source : source.datasets[datasetIndex];
  const action = button.dataset.benchmarkAction;
  if (action === "reveal-key") {
    const input = document.querySelector<HTMLInputElement>(`[data-benchmark-source="${button.dataset.benchmarkSource}"][data-benchmark-field="api_key_input"]`)!;
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
function readBenchmarkMappings(rows: Record<string, any>[]): Record<string, any> {
  const mappings: Record<string, any> = Object.create(null);
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
function readBenchmarkConfiguration(): Record<string, any> {
  const config = structuredClone(benchmarkConfiguration);
  config.enabled = document.querySelector<HTMLInputElement>("#intelligence-enabled")!.checked;
  config.sources = benchmarkSources.filter(source => !source._new || source.enabled).map(source => {
    const { _new, _metrics, api_key_input, datasets, ...output } = source;
    output.id = String(output.id || "").trim();
    if (!output.id) throw new Error("Each benchmark source needs a source ID");
    if (output.url === "") delete output.url;
    if (output.max_rows === "" || output.max_rows === undefined) delete output.max_rows; else output.max_rows = Number(output.max_rows);
    output.metrics = readBenchmarkMappings(_metrics || []);
    if (datasets?.length) output.datasets = datasets.map((dataset: Record<string, any>) => {
      const { _metrics:rows, ...saved } = dataset;
      saved.metrics = readBenchmarkMappings(rows || []);
      return saved;
    });
    if (api_key_input) output.api_key = { value:api_key_input };
    if (output.enabled && output.type === "artificial_analysis" && !output.api_key) throw new Error("Artificial Analysis requires an API key");
    if (output.type === "huggingface" && (!output.datasets?.length || output.datasets.some((dataset: any) => !dataset.dataset || Object.keys(dataset.metrics).length === 0))) throw new Error("Hugging Face requires a dataset ID and at least one metric mapping per dataset");
    return output;
  });
  document.querySelectorAll<HTMLInputElement>("[data-benchmark-global]").forEach(input => {
    const field = input.dataset.benchmarkGlobal!;
    if (input.value.trim() === "") delete config[field]; else config[field] = field === "retries" ? Number(input.value) : input.value.trim();
  });
  return config;
}
function clearSubmittedBenchmarkKeys(): void { benchmarkSources.forEach(source => { source.api_key_input = ""; }); renderBenchmarkSources(); }
function benchmarkSafeMessage(message: string): string {
  for (const source of benchmarkSources) { if (source.api_key_input) message = message.split(source.api_key_input).join("[redacted]"); }
  return message;
}
function benchmarkNumber(raw: any, score = false): string { return typeof raw === "number" && Number.isFinite(raw) ? (score ? (raw * 100).toFixed(1) : String(Number(raw.toFixed(4)))) : "—"; }
function renderBenchmarkTable(): void {
  const raw = benchmarkTableView === "observations";
  const scores = ["overall", ...benchmarkCategories.filter(category => category !== "operational")];
  const operations = Object.keys(benchmarkOperations);
  const headers = raw ? ["Model / external ID", "Source", "Benchmark", "Category", "Raw score", "Normalized %", "Unit", "Version", "Evaluation date", "Retrieved", "Match"]
    : ["Model", "Providers", ...scores.map(score => `${score.replace(/_/g," ")} %`), "Input $ / 1M", "Output $ / 1M", "Latency s", "TTFT s", "Tokens / s", "Context tokens", "Sources", "Observations", "Data status", "Last retrieval"];
  let rows: any[];
  let total: number;
  if (raw) { rows = benchmarkRawRows; total = benchmarkRawTotal; }
  else {
    const query = benchmarkFilter.toLowerCase();
    const models = Object.entries(benchmarkModels).sort(([a],[b]) => a.localeCompare(b)).filter(([id, model]) => `${id} ${(model.provider_ids || []).join(" ")} ${(model.sources || []).join(" ")}`.toLowerCase().includes(query));
    total = models.length;
    benchmarkPage = Math.min(benchmarkPage, Math.max(0, Math.ceil(total / benchmarkPageSize) - 1));
    rows = models.slice(benchmarkPage * benchmarkPageSize, (benchmarkPage + 1) * benchmarkPageSize).map(([id, model]) => ({...model,id}));
  }
  document.querySelector("#benchmark-table-head")!.innerHTML = `<tr>${headers.map(header => `<th scope="col">${escapeHTML(header)}</th>`).join("")}</tr>`;
  document.querySelector("#benchmark-table-body")!.innerHTML = rows.length ? rows.map(row => {
    const values = raw ? [row.model_id || row.external_model_id, row.source, row.benchmark, row.category, benchmarkNumber(row.original_score), benchmarkNumber(row.normalized_score,true), row.unit || "—", row.version || "—", benchmarkDate(row.evaluated_at), benchmarkDate(row.retrieved_at), row.model_id ? "Matched" : "Unmatched / ambiguous"]
      : [row.id, (row.provider_ids || []).join(", "), ...scores.map(score => benchmarkNumber(row.scores?.[score],true)), ...operations.map(metric => benchmarkNumber(row.operations?.[metric])), (row.sources || []).join(", "), String(row.observation_count || 0), row.stale ? "Stale" : "Current", benchmarkDate(row.latest_retrieval)];
    return `<tr>${values.map(value => `<td>${escapeHTML(String(value))}</td>`).join("")}</tr>`;
  }).join("") : `<tr><td colspan="${headers.length}">No cached benchmark data matches this view. Enable sources, save, and refresh to import results.</td></tr>`;
  const pages = Math.max(1, Math.ceil(total / benchmarkPageSize));
  document.querySelector("#benchmark-data-count")!.textContent = `${total} ${raw ? "observations" : "models"}`;
  document.querySelector("#benchmark-page-label")!.textContent = `Page ${benchmarkPage + 1} of ${pages}`;
  document.querySelector<HTMLButtonElement>("#benchmark-previous")!.disabled = benchmarkPage === 0;
  document.querySelector<HTMLButtonElement>("#benchmark-next")!.disabled = (benchmarkPage + 1) * benchmarkPageSize >= total;
}
function benchmarkDate(value: any): string { return value && !String(value).startsWith("0001-") ? String(value) : "Unknown"; }
async function loadBenchmarkData(): Promise<void> {
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
    if (!benchmarkRefreshRunning) document.querySelector("#benchmark-status")!.textContent = "Showing saved benchmark evidence. Missing scores are unknown, not zero; human preference remains a separate signal.";
  } catch (error) { if (sequence === benchmarkReloadSequence) document.querySelector("#benchmark-status")!.textContent = benchmarkSafeMessage(error instanceof Error ? error.message : "Unable to load benchmark data"); }
}
function benchmarkProgress(event: Record<string, any>): void {
  if (event.source) {
    const labels: Record<string, string> = {fetching:"Fetching and parsing",persisting:"Saving validated data",failed:"Update failed; previous cache retained"};
    if (labels[event.phase]) benchmarkSourceActivity[event.source] = labels[event.phase];
    if (event.phase === "published") delete benchmarkSourceActivity[event.source];
    renderBenchmarkSourceStatus();
  }
  for (const field of ["total", "completed", "failed"] as const) {
    if (event[field] !== undefined) benchmarkProgressCounts[field] = Number(event[field]);
  }
  const {total, completed, failed} = benchmarkProgressCounts;
  const bar = document.querySelector<HTMLProgressElement>("#benchmark-progress")!;
  bar.max = Math.max(1, total); bar.value = total ? Math.min(total, completed) : event.phase === "complete" ? 1 : 0;
  document.querySelector("#benchmark-progress-label")!.textContent = `${completed} of ${total} sources finished${failed ? ` · ${failed} failed` : ""} · ${benchmarkSafeMessage(String(event.message || ""))}`;
  benchmarkProgressLog.push(benchmarkSafeMessage(`${event.source ? `${event.source}: ` : ""}${event.message || event.phase}${event.observations ? ` (${event.observations} observations)` : ""}`));
  benchmarkProgressLog = benchmarkProgressLog.slice(-256);
  const log = document.querySelector<HTMLElement>("#benchmark-refresh-log")!;
  log.textContent = benchmarkProgressLog.join("\n"); log.scrollTop = log.scrollHeight;
}
async function inspectBenchmarks(refresh: boolean): Promise<void> {
  if (!refresh) { await loadBenchmarkData(); return; }
  const output = document.querySelector("#benchmark-status")!;
  if (!benchmarksEnabledSaved || !document.querySelector<HTMLInputElement>("#intelligence-enabled")!.checked || benchmarkDirty) { output.textContent = "Enable benchmark intelligence and click Save & reload before refreshing changed source settings."; return; }
  if (benchmarkRefreshRunning) return;
  const login = adminAuthorization;
  const abort = new AbortController();
  benchmarkRefreshRunning = true; benchmarkAbort = abort; benchmarkProgressLog = []; benchmarkSourceActivity = {};
  benchmarkProgressCounts = {total:0,completed:0,failed:0};
  document.querySelector<HTMLDivElement>("#benchmark-refresh-panel")!.hidden = false;
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
        let boundary: number;
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
function initializeBenchmarks(): void {
  const list = document.querySelector("#benchmark-sources-list")!;
  list.addEventListener("input", updateBenchmarkSource); list.addEventListener("click", benchmarkSourceAction);
  document.querySelectorAll<HTMLInputElement>("[data-benchmark-global]").forEach(input => input.addEventListener("input",()=>{benchmarkDirty=true;markDirty();updateBenchmarkControls();}));
  document.querySelector("#benchmark-table-view")!.addEventListener("change",()=>{benchmarkTableView=document.querySelector<HTMLSelectElement>("#benchmark-table-view")!.value;benchmarkPage=0;void loadBenchmarkData();});
  document.querySelector("#benchmark-table-filter")!.addEventListener("input",()=>{benchmarkFilter=document.querySelector<HTMLInputElement>("#benchmark-table-filter")!.value;benchmarkPage=0;if(benchmarkTableView==="models")renderBenchmarkTable();else void loadBenchmarkData();});
  document.querySelector("#benchmark-previous")!.addEventListener("click",()=>{if(benchmarkPage>0){benchmarkPage--;if(benchmarkTableView==="models")renderBenchmarkTable();else void loadBenchmarkData();}});
  document.querySelector("#benchmark-next")!.addEventListener("click",()=>{benchmarkPage++;if(benchmarkTableView==="models")renderBenchmarkTable();else void loadBenchmarkData();});
  renderBenchmarkTable();
}

// Source forms and cached evidence views. No browser persistence or dependencies.
type BenchmarkSourceDraft = Record<string, any>;
let benchmarkConfiguration: Record<string, any> = {};
let benchmarkSources: BenchmarkSourceDraft[] = [];
let benchmarkSourceStatus: Record<string, any> = {};
let benchmarkSourceActivity: Record<string, string> = {};
let benchmarkModels: Record<string, any> = {};
let benchmarkPage = 0;
let benchmarkTableView = "models";
let benchmarkFilter = "";
let benchmarkDirty = false;
let benchmarkRefreshRunning = false;
let benchmarkAbort: AbortController | null = null;
let benchmarkReloadSequence = 0;
let benchmarkProgressLog: string[] = [];
let benchmarkProgressCounts = {total:0,completed:0,failed:0};
let benchmarkPageSize = 25;
// Injected by build-console.mjs from the same presets.json used by the backend.
declare const benchmarkDefinitions: {name:string; requires_api_key:boolean; note:string; settings:Record<string,any>}[];
const benchmarkCapabilities = ["coding", "agentic", "knowledge", "reasoning", "math", "instruction_following", "multimodal", "preference", "general_intelligence"];
const benchmarkOperations: Record<string, string> = { input_price:"USD_per_million_tokens", output_price:"USD_per_million_tokens", latency:"seconds", ttft:"seconds", throughput:"tokens_per_second", context_length:"tokens" };

function benchmarkDraft(source: Record<string, any>, fresh = false): BenchmarkSourceDraft {
  const defaults = structuredClone(benchmarkDefinitions.find(preset => preset.settings.type === source.type)?.settings || {});
  const configured = { ...defaults, ...structuredClone(source) };
  if (!source.url || (source.type === "artificial_analysis" && source.url === "https://artificialanalysis.ai/api/v2/language/models/free")) configured.url = defaults.url;
  if (!source.datasets?.length || (source.type === "huggingface" && source.datasets.length === 1 && ["", "org/your-leaderboard"].includes(String(source.datasets[0].dataset || "").trim()))) configured.datasets = defaults.datasets;
  return { ...configured, _new:fresh, api_key_input:"" };
}
function applyBenchmarkConfig(config: Record<string, any> = {}): void {
  benchmarkConfiguration = structuredClone(config);
  benchmarkSources = (config.sources || []).map((source: Record<string, any>) => benchmarkDraft(source));
  for (const definition of benchmarkDefinitions) {
    if (benchmarkSources.some(source => source.type === definition.settings.type)) continue;
    let id = definition.settings.id;
    const base = id;
    for (let suffix = 2; benchmarkSources.some(source => source.id === id); suffix++) id = `${base}-${suffix}`;
    benchmarkSources.push(benchmarkDraft({ ...definition.settings, id }, true));
  }
  benchmarkDirty = false;
  benchmarkSourceActivity = {};
  benchmarksEnabledSaved = config.enabled === true;
  document.querySelector<HTMLInputElement>("#intelligence-enabled")!.checked = benchmarksEnabledSaved;
  renderBenchmarkSources();
  updateBenchmarkControls();
}
function clearBenchmarkConsole(): void {
  benchmarkAbort?.abort(); benchmarkAbort = null; benchmarkReloadSequence++;
  benchmarkConfiguration = {}; benchmarkSources = []; benchmarkSourceStatus = {}; benchmarkSourceActivity = {}; benchmarkModels = {};
  benchmarkPage = 0; benchmarkFilter = ""; benchmarkTableView = "models";
  benchmarkPageSize = 25;
  benchmarkDirty = false; benchmarkRefreshRunning = false; benchmarkProgressLog = [];
  benchmarkProgressCounts = {total:0,completed:0,failed:0};
  document.querySelector<HTMLInputElement>("#intelligence-enabled")!.checked = false;
  document.querySelector<HTMLDivElement>("#benchmark-refresh-panel")!.hidden = true;
  document.querySelector("#benchmark-refresh-log")!.textContent = "";
  document.querySelector<HTMLInputElement>("#benchmark-table-filter")!.value = "";
  document.querySelector<HTMLSelectElement>("#benchmark-table-view")!.value = "models";
  document.querySelector<HTMLSelectElement>("#benchmark-page-size")!.value = "25";
  renderBenchmarkSources(); renderBenchmarkTable();
}
function benchmarkAttributes(source: number, field: string): string {
  return `data-benchmark-source="${source}" data-benchmark-field="${field}"`;
}
function renderBenchmarkSources(): void {
  const list = document.querySelector<HTMLDivElement>("#benchmark-sources-list")!;
  list.innerHTML = benchmarkSources.map((source, index) => {
    const definition = benchmarkDefinitions.find(item => item.settings.type === source.type);
    const huggingFaceToken = ["huggingface", "livebench", "lmarena"].includes(source.type);
    const needsKey = definition?.requires_api_key || source.api_key || huggingFaceToken;
    return `<article class="provider-card"><div class="provider-card-head"><h3>${escapeHTML(definition?.name || source.type)}${source._new ? "" : ` · ${escapeHTML(source.id)}`}</h3><label class="checkbox" data-tooltip="Enable this preconfigured source for background and manual refreshes. Save and reload before ingestion starts."><input type="checkbox" ${benchmarkAttributes(index, "enabled")} ${source.enabled ? "checked" : ""} /> Enabled</label></div>
      <div class="provider-result" data-benchmark-status="${index}" role="status" aria-live="polite"></div>
      <div class="benchmark-source-options" ${source.enabled ? "" : "hidden"}>
      <p>${escapeHTML(definition?.note || "Uses your saved source configuration.")}</p>
      ${needsKey ? `<label data-tooltip="Save encrypts new values in SQLite; saved credentials cannot be revealed. Public Hugging Face datasets allow anonymous access; an optional token authenticates requests to Hugging Face.">${huggingFaceToken ? "Hugging Face token (optional)" : "API key"}<span class="password-field"><input type="password" autocomplete="new-password" ${benchmarkAttributes(index, "api_key_input")} placeholder="${source.api_key?.credential ? "Configured · leave blank to keep" : huggingFaceToken ? "Optional Hugging Face token" : "Enter source API key"}" /><button type="button" class="reveal-button" data-benchmark-action="reveal-key" ${benchmarkAttributes(index, "")}>Reveal</button></span></label>` : ""}
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
  const input = event.target as HTMLInputElement;
  if (input.dataset.benchmarkSource === undefined || !["enabled", "api_key_input"].includes(input.dataset.benchmarkField || "")) return;
  const source = benchmarkSources[Number(input.dataset.benchmarkSource)];
  const field = input.dataset.benchmarkField!;
  source[field] = input.type === "checkbox" ? input.checked : input.value;
  benchmarkDirty = true; markDirty(); updateBenchmarkControls();
  if (field === "enabled") renderBenchmarkSources();
}
function benchmarkSourceAction(event: Event): void {
  const button = (event.target as HTMLElement).closest<HTMLButtonElement>("[data-benchmark-action]");
  if (!button || button.disabled || button.dataset.benchmarkAction !== "reveal-key") return;
  const input = document.querySelector<HTMLInputElement>(`[data-benchmark-source="${button.dataset.benchmarkSource}"][data-benchmark-field="api_key_input"]`)!;
  input.type = input.type === "password" ? "text" : "password";
  button.textContent = input.type === "text" ? "Hide" : "Reveal";
  button.setAttribute("aria-pressed", String(input.type === "text"));
}
function readBenchmarkConfiguration(): Record<string, any> {
  const config = structuredClone(benchmarkConfiguration);
  config.enabled = document.querySelector<HTMLInputElement>("#intelligence-enabled")!.checked;
  config.sources = benchmarkSources.filter(source => !source._new || source.enabled).map(source => {
    const { _new, api_key_input, ...fields } = source;
    const output = structuredClone(fields);
    if (api_key_input) output.api_key = { value:api_key_input };
    if (output.enabled && output.type === "artificial_analysis" && !output.api_key) throw new Error("Artificial Analysis requires an API key");
    return output;
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
  const scores = ["overall", ...benchmarkCapabilities];
  const scoreHeaders = scores.map(score => `${score.replace(/_/g," ").replace(/^./, letter => letter.toUpperCase())} %`);
  const operations = Object.keys(benchmarkOperations);
  const headers = ["Model", ...scoreHeaders, "Providers", "Input $ / 1M", "Output $ / 1M", "Latency s", "TTFT s", "Tokens / s", "Context tokens", "Sources", "Observations", "Match", "Data status", "Latest evaluation", "Last retrieval", "Age seconds", "Valid until"];
  const query = benchmarkFilter.toLowerCase();
  const models = Object.entries(benchmarkModels).sort(([a,left],[b,right]) => String(left.display_name || a).localeCompare(String(right.display_name || b)) || a.localeCompare(b))
    .filter(([id, model]) => `${id} ${model.display_name || ""} ${(model.external_ids || []).join(" ")} ${(model.provider_ids || []).join(" ")} ${(model.sources || []).join(" ")}`.toLowerCase().includes(query));
  const total = models.length;
  benchmarkPage = Math.min(benchmarkPage, Math.max(0, Math.ceil(total / benchmarkPageSize) - 1));
  const rows = models.slice(benchmarkPage * benchmarkPageSize, (benchmarkPage + 1) * benchmarkPageSize).map(([id, model]) => ({...model,id}));
  document.querySelector("#benchmark-table-head")!.innerHTML = `<tr>${headers.map(header => `<th scope="col">${escapeHTML(header)}</th>`).join("")}</tr>`;
  document.querySelector("#benchmark-table-body")!.innerHTML = rows.length ? rows.map(row => {
    const match = row.match === "approximate" ? "Approximate" : row.match === "ambiguous" ? "Ambiguous" : row.match === "unmatched" ? "Unmatched" : "Matched";
    const values = [row.display_name || row.id, ...scores.map(score => benchmarkNumber(row.scores?.[score],true)), (row.provider_ids || []).join(", "), ...operations.map(metric => benchmarkNumber(row.operations?.[metric])), (row.sources || []).join(", "), String(row.observation_count || 0), match, row.stale ? "Stale" : "Current", benchmarkDate(row.latest_evaluation), benchmarkDate(row.latest_retrieval), benchmarkNumber(row.age_seconds), benchmarkDate(row.valid_until)];
    return `<tr>${values.map(value => `<td>${escapeHTML(String(value))}</td>`).join("")}</tr>`;
  }).join("") : `<tr><td colspan="${headers.length}">No cached benchmark data matches this view. Enable sources, save, and refresh to import results.</td></tr>`;
  const pages = Math.max(1, Math.ceil(total / benchmarkPageSize));
  document.querySelector("#benchmark-data-count")!.textContent = `${total} models`;
  document.querySelector("#benchmark-page-label")!.textContent = `Page ${benchmarkPage + 1} of ${pages}`;
  document.querySelector<HTMLButtonElement>("#benchmark-previous")!.disabled = benchmarkPage === 0;
  document.querySelector<HTMLButtonElement>("#benchmark-next")!.disabled = (benchmarkPage + 1) * benchmarkPageSize >= total;
}
function benchmarkDate(value: any): string { return value && !String(value).startsWith("0001-") ? String(value) : "Unknown"; }
async function loadBenchmarkData(): Promise<boolean> {
  if (!benchmarksEnabledSaved || !adminAuthorization) return false;
  const sequence = ++benchmarkReloadSequence;
  const login = adminAuthorization;
  try {
    const headers = await authProvider.headers();
    // Request full model summaries, not a page of per-score observations.
    // Grouping happens on the backend before filtering or client pagination.
    const paths = ["sources", benchmarkTableView === "routable" ? "models?configured_only=true" : "models?include_unmatched=true"];
    const replies = await Promise.all(paths.map(async path => {
      const response = await gatewayFetch(`/gateway/v1/intelligence/${path}`, {headers});
      const body = await response.json();
      if (!response.ok) throw new Error(body.error?.message || body.error || "Unable to load benchmark data");
      return body;
    }));
    if (sequence !== benchmarkReloadSequence || login !== adminAuthorization) return false;
    [benchmarkSourceStatus, benchmarkModels] = replies;
    renderBenchmarkSourceStatus(); renderBenchmarkTable();
    if (!benchmarkRefreshRunning) document.querySelector("#benchmark-status")!.textContent = "Showing saved benchmark evidence. Missing scores are unknown, not zero; human preference remains a separate signal.";
    return true;
  } catch (error) { if (sequence === benchmarkReloadSequence) document.querySelector("#benchmark-status")!.textContent = benchmarkSafeMessage(error instanceof Error ? error.message : "Unable to load benchmark data");return false; }
}
function benchmarkProgress(event: Record<string, any>): void {
  if (event.source) {
    const labels: Record<string, string> = {discovering:"Discovering dataset files",downloading:"Downloading dataset files",fetching:"Fetching and parsing",parsing:"Parsing dataset rows",rate_limited:"Waiting for upstream rate limit",persisting:"Saving validated data",failed:"Update failed; previous cache retained"};
    if (labels[event.phase]) benchmarkSourceActivity[event.source] = event.phase === "failed" ? labels.failed : String(event.message || labels[event.phase]);
    if (event.phase === "published") delete benchmarkSourceActivity[event.source];
    renderBenchmarkSourceStatus();
  }
  for (const field of ["total", "completed", "failed"] as const) {
    if (event[field] !== undefined) benchmarkProgressCounts[field] = Number(event[field]);
  }
  const {total, completed, failed} = benchmarkProgressCounts;
  const bar = document.querySelector<HTMLProgressElement>("#benchmark-progress")!;
  bar.max = total + 1; bar.value = Math.min(total, completed) + (event.phase === "table_loaded" ? 1 : 0);
  const message = event.phase === "complete" ? "Source ingestion complete; preparing the table update" : String(event.message || "");
  document.querySelector("#benchmark-progress-label")!.textContent = `${completed} of ${total} sources finished${failed ? ` · ${failed} failed` : ""} · ${benchmarkSafeMessage(message)}`;
  benchmarkProgressLog.push(benchmarkSafeMessage(`${event.source ? `${event.source}: ` : ""}${message || event.phase}${event.observations ? ` (${event.observations} observations)` : ""}`));
  benchmarkProgressLog = benchmarkProgressLog.slice(-256);
  const log = document.querySelector<HTMLElement>("#benchmark-refresh-log")!;
  log.textContent = benchmarkProgressLog.join("\n"); log.scrollTop = log.scrollHeight;
}
async function finishBenchmarkRefresh(result: Record<string,any>, login: string, abort: AbortController): Promise<void> {
  if (login !== adminAuthorization || abort.signal.aborted) return;
  document.querySelector("#benchmark-status")!.textContent = "Loading benchmark results into the table…";
  benchmarkProgress({phase:"loading_table",message:"Loading results into the table"});
  while (login === adminAuthorization && !abort.signal.aborted) {
    const expectedSequence = benchmarkReloadSequence + 1;
    if (!await loadBenchmarkData()) {
      if (login !== adminAuthorization || abort.signal.aborted) return;
      if (benchmarkReloadSequence > expectedSequence) continue; // View changed during loading.
      throw new Error("Sources ingested, but the table could not finish updating. Reload cached data to try again.");
    }
    const loadedSequence = benchmarkReloadSequence;
    // Let the browser paint the new rows before announcing completion.
    await new Promise<void>(resolve => window.requestAnimationFrame && document.visibilityState !== "hidden" ? window.requestAnimationFrame(()=>window.requestAnimationFrame(()=>resolve())) : window.setTimeout(resolve,0));
    if (loadedSequence === benchmarkReloadSequence) break;
  }
  if (login !== adminAuthorization || abort.signal.aborted) return;
  const message = result.partial_failure ? "Benchmark refresh finished with source failures; table updated with available data." : "Benchmark refresh finished; table updated.";
  benchmarkProgress({phase:"table_loaded",message});
  document.querySelector("#benchmark-status")!.textContent = message;
}
async function inspectBenchmarks(refresh: boolean): Promise<void> {
  if (!refresh) { await loadBenchmarkData(); return; }
  const output = document.querySelector("#benchmark-status")!;
  if (!benchmarksEnabledSaved || !document.querySelector<HTMLInputElement>("#intelligence-enabled")!.checked || benchmarkDirty) { output.textContent = "Enable benchmark intelligence and click Save & reload before refreshing changed source settings."; return; }
  if (benchmarkRefreshRunning) return;
  const login = adminAuthorization;
  const abort = new AbortController();
  output.textContent = "Refreshing sources; cached results remain available…";
  benchmarkRefreshRunning = true; benchmarkAbort = abort; benchmarkProgressLog = []; benchmarkSourceActivity = {};
  benchmarkProgressCounts = {total:0,completed:0,failed:0};
  document.querySelector<HTMLDivElement>("#benchmark-refresh-panel")!.hidden = false;
  benchmarkProgress({phase:"connecting",total:0,message:"Connecting to the refresh worker"}); updateBenchmarkControls();
  void loadBenchmarkData();
  try {
    const response = await gatewayFetch("/gateway/v1/intelligence/refresh", {method:"POST",headers:{Accept:"text/event-stream",...await authProvider.headers()},signal:abort.signal});
    if (!response.ok) { const body = await response.json(); throw new Error(body.error?.message || body.error || "Refresh failed"); }
    if (!response.body?.getReader || !response.headers?.get("Content-Type")?.includes("text/event-stream")) {
      const body = await response.json();
      if (login !== adminAuthorization || abort.signal.aborted) return;
      benchmarkSourceStatus = body.sources || {}; renderBenchmarkSourceStatus();
      benchmarkProgress({phase:"complete",total:Object.values(body.sources || {}).filter((source:any)=>source.enabled).length,completed:Object.values(body.sources || {}).filter((source:any)=>source.enabled).length});
      await finishBenchmarkRefresh(body,login,abort);return;
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
          // Table requests must not block reading activity from a long import.
          // The final table step below still waits for the latest view to render.
          if (kind === "progress") { benchmarkProgress(event); if (event.phase === "published" || event.phase === "failed") void loadBenchmarkData(); }
          if (kind === "heartbeat") document.querySelector("#benchmark-progress-label")!.textContent = `${benchmarkProgressCounts.completed} of ${benchmarkProgressCounts.total} sources finished · Refresh is still running; waiting for upstream data or cooldown`;
          if (kind === "result") { finished = true; benchmarkSourceStatus = event.sources || {}; renderBenchmarkSourceStatus(); await finishBenchmarkRefresh(event,login,abort); }
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
  document.querySelector("#benchmark-page-size")!.addEventListener("change",()=>{const size=Number(document.querySelector<HTMLSelectElement>("#benchmark-page-size")!.value);if(![25,50,100].includes(size))return;benchmarkPageSize=size;benchmarkPage=0;renderBenchmarkTable();});
  document.querySelector("#benchmark-table-view")!.addEventListener("change",()=>{benchmarkTableView=document.querySelector<HTMLSelectElement>("#benchmark-table-view")!.value;benchmarkPage=0;void loadBenchmarkData();});
  document.querySelector("#benchmark-table-filter")!.addEventListener("input",()=>{benchmarkFilter=document.querySelector<HTMLInputElement>("#benchmark-table-filter")!.value;benchmarkPage=0;renderBenchmarkTable();});
  document.querySelector("#benchmark-previous")!.addEventListener("click",()=>{if(benchmarkPage>0){benchmarkPage--;renderBenchmarkTable();}});
  document.querySelector("#benchmark-next")!.addEventListener("click",()=>{benchmarkPage++;renderBenchmarkTable();});
  renderBenchmarkTable();
}

// Generated from benchmarks.ts, models.ts, accounting.ts, activity.ts and app.ts by scripts/build-console.mjs.
const benchmarkDefinitions = [{"name":"Artificial Analysis","requires_api_key":true,"note":"Imports published capability, pricing and speed measurements. Requires an Artificial Analysis API key.","settings":{"id":"artificial-analysis","type":"artificial_analysis","enabled":false,"url":"https://artificialanalysis.ai/api/v2/data/llms/models"}},{"name":"BenchLM.ai","requires_api_key":false,"note":"Imports public supported benchmark evidence; estimated scores do not influence capabilities. Commercial use requires a BenchLM licence.","settings":{"id":"benchlm","type":"benchlm","enabled":false,"url":"https://benchlm.ai/data/models.json"}},{"name":"Hugging Face leaderboards","requires_api_key":false,"note":"Imports the public Open LLM Leaderboard v2 results with predefined instruction-following, reasoning, math and knowledge mappings. This published dataset contains historical evaluations, not a current ranking of all models. Public data permits anonymous access. An optional Hugging Face token authenticates downloads; no separate LiveBench or Arena API key is required.","settings":{"id":"huggingface","type":"huggingface","enabled":false,"url":"https://datasets-server.huggingface.co/rows","datasets":[{"dataset":"open-llm-leaderboard/contents","config":"default","split":"train","model_field":"fullname","version":"Open LLM Leaderboard v2","metrics":{"IFEval Raw":{"category":"instruction_following","max":1},"BBH Raw":{"category":"reasoning","max":1},"MATH Lvl 5 Raw":{"category":"math","max":1},"GPQA Raw":{"category":"knowledge","max":1},"MUSR Raw":{"category":"reasoning","max":1},"MMLU-PRO Raw":{"category":"knowledge","max":1}}}]}},{"name":"LMArena","requires_api_key":false,"note":"Imports the latest public text style-controlled Arena leaderboard. Ratings remain human-preference evidence, separate from objective benchmarks. Public data permits anonymous access. An optional Hugging Face token authenticates downloads; no separate LiveBench or Arena API key is required. Downloads the selected published Parquet files instead of paginating Dataset Viewer rows.","settings":{"id":"lmarena","type":"lmarena","enabled":false,"url":"https://datasets-server.huggingface.co/rows","datasets":[{"dataset":"lmarena-ai/leaderboard-dataset","config":"text_style_control","split":"latest","model_field":"model_name","date_field":"leaderboard_publish_date"}]}},{"name":"LiveBench","requires_api_key":false,"note":"Imports public structured model judgments. The artifact can lag the website; expired evaluations remain inspectable but do not influence routing. Public data permits anonymous access. An optional Hugging Face token authenticates downloads; no separate LiveBench or Arena API key is required. Downloads the selected published Parquet files instead of paginating Dataset Viewer rows.","settings":{"id":"livebench","type":"livebench","enabled":false,"url":"https://datasets-server.huggingface.co/rows","datasets":[{"dataset":"livebench/model_judgment","config":"default","split":"leaderboard","model_field":"model","date_field":"tstamp"}]}}];
// Source forms and cached evidence views. No browser persistence or dependencies.

let benchmarkConfiguration                      = {};
let benchmarkSources                         = [];
let benchmarkSourceStatus                      = {};
let benchmarkSourceActivity                         = {};
let benchmarkModels                      = {};
let benchmarkPage = 0;
let benchmarkTableView = "models";
let benchmarkFilter = "";
let benchmarkDirty = false;
let benchmarkRefreshRunning = false;
let benchmarkAbort                         = null;
let benchmarkReloadSequence = 0;
let benchmarkProgressLog           = [];
let benchmarkProgressCounts = {total:0,completed:0,failed:0};
let benchmarkPageSize = 25;
// Injected by build-console.mjs from the same presets.json used by the backend.

const benchmarkCapabilities = ["coding", "agentic", "knowledge", "reasoning", "math", "instruction_following", "multimodal", "preference", "general_intelligence"];
const benchmarkOperations                         = { input_price:"USD_per_million_tokens", output_price:"USD_per_million_tokens", latency:"seconds", ttft:"seconds", throughput:"tokens_per_second", context_length:"tokens" };

function benchmarkDraft(source                     , fresh = false)                       {
  const defaults = structuredClone(benchmarkDefinitions.find(preset => preset.settings.type === source.type)?.settings || {});
  const configured = { ...defaults, ...structuredClone(source) };
  if (!source.url || (source.type === "artificial_analysis" && source.url === "https://artificialanalysis.ai/api/v2/language/models/free")) configured.url = defaults.url;
  if (!source.datasets?.length || (source.type === "huggingface" && source.datasets.length === 1 && ["", "org/your-leaderboard"].includes(String(source.datasets[0].dataset || "").trim()))) configured.datasets = defaults.datasets;
  return { ...configured, _new:fresh, api_key_input:"" };
}
function applyBenchmarkConfig(config                      = {})       {
  benchmarkConfiguration = structuredClone(config);
  benchmarkSources = (config.sources || []).map((source                     ) => benchmarkDraft(source));
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
  document.querySelector                  ("#intelligence-enabled") .checked = benchmarksEnabledSaved;
  renderBenchmarkSources();
  updateBenchmarkControls();
}
function clearBenchmarkConsole()       {
  benchmarkAbort?.abort(); benchmarkAbort = null; benchmarkReloadSequence++;
  benchmarkConfiguration = {}; benchmarkSources = []; benchmarkSourceStatus = {}; benchmarkSourceActivity = {}; benchmarkModels = {};
  benchmarkPage = 0; benchmarkFilter = ""; benchmarkTableView = "models";
  benchmarkPageSize = 25;
  benchmarkDirty = false; benchmarkRefreshRunning = false; benchmarkProgressLog = [];
  benchmarkProgressCounts = {total:0,completed:0,failed:0};
  document.querySelector                  ("#intelligence-enabled") .checked = false;
  document.querySelector                ("#benchmark-refresh-panel") .hidden = true;
  document.querySelector("#benchmark-refresh-log") .textContent = "";
  document.querySelector                  ("#benchmark-table-filter") .value = "";
  document.querySelector                   ("#benchmark-table-view") .value = "models";
  document.querySelector                   ("#benchmark-page-size") .value = "25";
  renderBenchmarkSources(); renderBenchmarkTable();
}
function benchmarkAttributes(source        , field        )         {
  return `data-benchmark-source="${source}" data-benchmark-field="${field}"`;
}
function renderBenchmarkSources()       {
  const list = document.querySelector                ("#benchmark-sources-list") ;
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
  const input = event.target                    ;
  if (input.dataset.benchmarkSource === undefined || !["enabled", "api_key_input"].includes(input.dataset.benchmarkField || "")) return;
  const source = benchmarkSources[Number(input.dataset.benchmarkSource)];
  const field = input.dataset.benchmarkField ;
  source[field] = input.type === "checkbox" ? input.checked : input.value;
  benchmarkDirty = true; markDirty(); updateBenchmarkControls();
  if (field === "enabled") renderBenchmarkSources();
}
function benchmarkSourceAction(event       )       {
  const button = (event.target               ).closest                   ("[data-benchmark-action]");
  if (!button || button.disabled || button.dataset.benchmarkAction !== "reveal-key") return;
  const input = document.querySelector                  (`[data-benchmark-source="${button.dataset.benchmarkSource}"][data-benchmark-field="api_key_input"]`) ;
  input.type = input.type === "password" ? "text" : "password";
  button.textContent = input.type === "text" ? "Hide" : "Reveal";
  button.setAttribute("aria-pressed", String(input.type === "text"));
}
function readBenchmarkConfiguration()                      {
  const config = structuredClone(benchmarkConfiguration);
  config.enabled = document.querySelector                  ("#intelligence-enabled") .checked;
  config.sources = benchmarkSources.filter(source => !source._new || source.enabled).map(source => {
    const { _new, api_key_input, ...fields } = source;
    const output = structuredClone(fields);
    if (api_key_input) output.api_key = { value:api_key_input };
    if (output.enabled && output.type === "artificial_analysis" && !output.api_key) throw new Error("Artificial Analysis requires an API key");
    return output;
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
  document.querySelector("#benchmark-table-head") .innerHTML = `<tr>${headers.map(header => `<th scope="col">${escapeHTML(header)}</th>`).join("")}</tr>`;
  document.querySelector("#benchmark-table-body") .innerHTML = rows.length ? rows.map(row => {
    const match = row.match === "approximate" ? "Approximate" : row.match === "ambiguous" ? "Ambiguous" : row.match === "unmatched" ? "Unmatched" : "Matched";
    const values = [row.display_name || row.id, ...scores.map(score => benchmarkNumber(row.scores?.[score],true)), (row.provider_ids || []).join(", "), ...operations.map(metric => benchmarkNumber(row.operations?.[metric])), (row.sources || []).join(", "), String(row.observation_count || 0), match, row.stale ? "Stale" : "Current", benchmarkDate(row.latest_evaluation), benchmarkDate(row.latest_retrieval), benchmarkNumber(row.age_seconds), benchmarkDate(row.valid_until)];
    return `<tr>${values.map(value => `<td>${escapeHTML(String(value))}</td>`).join("")}</tr>`;
  }).join("") : `<tr><td colspan="${headers.length}">No cached benchmark data matches this view. Enable sources, save, and refresh to import results.</td></tr>`;
  const pages = Math.max(1, Math.ceil(total / benchmarkPageSize));
  document.querySelector("#benchmark-data-count") .textContent = `${total} models`;
  document.querySelector("#benchmark-page-label") .textContent = `Page ${benchmarkPage + 1} of ${pages}`;
  document.querySelector                   ("#benchmark-previous") .disabled = benchmarkPage === 0;
  document.querySelector                   ("#benchmark-next") .disabled = (benchmarkPage + 1) * benchmarkPageSize >= total;
}
function benchmarkDate(value     )         { return value && !String(value).startsWith("0001-") ? String(value) : "Unknown"; }
async function loadBenchmarkData()                   {
  if (!benchmarksEnabledSaved || !consoleSession) return false;
  const sequence = ++benchmarkReloadSequence;
  const login = consoleSession;
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
    if (sequence !== benchmarkReloadSequence || login !== consoleSession) return false;
    [benchmarkSourceStatus, benchmarkModels] = replies;
    renderBenchmarkSourceStatus(); renderBenchmarkTable();
    if (!benchmarkRefreshRunning) document.querySelector("#benchmark-status") .textContent = "Showing saved benchmark evidence. Missing scores are unknown, not zero; human preference remains a separate signal.";
    return true;
  } catch (error) { if (sequence === benchmarkReloadSequence) document.querySelector("#benchmark-status") .textContent = benchmarkSafeMessage(error instanceof Error ? error.message : "Unable to load benchmark data");return false; }
}
function benchmarkProgress(event                     )       {
  if (event.source) {
    const labels                         = {discovering:"Discovering dataset files",downloading:"Downloading dataset files",fetching:"Fetching and parsing",parsing:"Parsing dataset rows",rate_limited:"Waiting for upstream rate limit",persisting:"Saving validated data",failed:"Update failed; previous cache retained"};
    if (labels[event.phase]) benchmarkSourceActivity[event.source] = event.phase === "failed" ? labels.failed : String(event.message || labels[event.phase]);
    if (event.phase === "published") delete benchmarkSourceActivity[event.source];
    renderBenchmarkSourceStatus();
  }
  for (const field of ["total", "completed", "failed"]         ) {
    if (event[field] !== undefined) benchmarkProgressCounts[field] = Number(event[field]);
  }
  const {total, completed, failed} = benchmarkProgressCounts;
  const bar = document.querySelector                     ("#benchmark-progress") ;
  bar.max = total + 1; bar.value = Math.min(total, completed) + (event.phase === "table_loaded" ? 1 : 0);
  const message = event.phase === "complete" ? "Source ingestion complete; preparing the table update" : String(event.message || "");
  document.querySelector("#benchmark-progress-label") .textContent = `${completed} of ${total} sources finished${failed ? ` · ${failed} failed` : ""} · ${benchmarkSafeMessage(message)}`;
  benchmarkProgressLog.push(benchmarkSafeMessage(`${event.source ? `${event.source}: ` : ""}${message || event.phase}${event.observations ? ` (${event.observations} observations)` : ""}`));
  benchmarkProgressLog = benchmarkProgressLog.slice(-256);
  const log = document.querySelector             ("#benchmark-refresh-log") ;
  log.textContent = benchmarkProgressLog.join("\n"); log.scrollTop = log.scrollHeight;
}
async function finishBenchmarkRefresh(result                    , login        , abort                 )                {
  if (login !== consoleSession || abort.signal.aborted) return;
  document.querySelector("#benchmark-status") .textContent = "Loading benchmark results into the table…";
  benchmarkProgress({phase:"loading_table",message:"Loading results into the table"});
  while (login === consoleSession && !abort.signal.aborted) {
    const expectedSequence = benchmarkReloadSequence + 1;
    if (!await loadBenchmarkData()) {
      if (login !== consoleSession || abort.signal.aborted) return;
      if (benchmarkReloadSequence > expectedSequence) continue; // View changed during loading.
      throw new Error("Sources ingested, but the table could not finish updating. Reload cached data to try again.");
    }
    const loadedSequence = benchmarkReloadSequence;
    // Let the browser paint the new rows before announcing completion.
    await new Promise      (resolve => window.requestAnimationFrame && document.visibilityState !== "hidden" ? window.requestAnimationFrame(()=>window.requestAnimationFrame(()=>resolve())) : window.setTimeout(resolve,0));
    if (loadedSequence === benchmarkReloadSequence) break;
  }
  if (login !== consoleSession || abort.signal.aborted) return;
  const message = result.partial_failure ? "Benchmark refresh finished with source failures; table updated with available data." : "Benchmark refresh finished; table updated.";
  benchmarkProgress({phase:"table_loaded",message});
  document.querySelector("#benchmark-status") .textContent = message;
}
async function inspectBenchmarks(refresh         )                {
  if (!refresh) { await loadBenchmarkData(); return; }
  const output = document.querySelector("#benchmark-status") ;
  if (!benchmarksEnabledSaved || !document.querySelector                  ("#intelligence-enabled") .checked || benchmarkDirty) { output.textContent = "Enable benchmark intelligence and wait for Saved before refreshing changed source settings."; return; }
  if (benchmarkRefreshRunning) return;
  const login = consoleSession;
  const abort = new AbortController();
  output.textContent = "Refreshing sources; cached results remain available…";
  benchmarkRefreshRunning = true; benchmarkAbort = abort; benchmarkProgressLog = []; benchmarkSourceActivity = {};
  benchmarkProgressCounts = {total:0,completed:0,failed:0};
  document.querySelector                ("#benchmark-refresh-panel") .hidden = false;
  benchmarkProgress({phase:"connecting",total:0,message:"Connecting to the refresh worker"}); updateBenchmarkControls();
  void loadBenchmarkData();
  try {
    const response = await gatewayFetch("/gateway/v1/intelligence/refresh", {method:"POST",headers:{Accept:"text/event-stream",...await authProvider.headers()},signal:abort.signal});
    if (!response.ok) { const body = await response.json(); throw new Error(body.error?.message || body.error || "Refresh failed"); }
    if (!response.body?.getReader || !response.headers?.get("Content-Type")?.includes("text/event-stream")) {
      const body = await response.json();
      if (login !== consoleSession || abort.signal.aborted) return;
      benchmarkSourceStatus = body.sources || {}; renderBenchmarkSourceStatus();
      benchmarkProgress({phase:"complete",total:Object.values(body.sources || {}).filter((source    )=>source.enabled).length,completed:Object.values(body.sources || {}).filter((source    )=>source.enabled).length});
      await finishBenchmarkRefresh(body,login,abort);return;
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
          if (!data || login !== consoleSession || abort.signal.aborted) continue;
          const event = JSON.parse(data);
          // Table requests must not block reading activity from a long import.
          // The final table step below still waits for the latest view to render.
          if (kind === "progress") { benchmarkProgress(event); if (event.phase === "published" || event.phase === "failed") void loadBenchmarkData(); }
          if (kind === "heartbeat") document.querySelector("#benchmark-progress-label") .textContent = `${benchmarkProgressCounts.completed} of ${benchmarkProgressCounts.total} sources finished · Refresh is still running; waiting for upstream data or cooldown`;
          if (kind === "result") { finished = true; benchmarkSourceStatus = event.sources || {}; renderBenchmarkSourceStatus(); await finishBenchmarkRefresh(event,login,abort); }
        }
        if (chunk.done) break;
      }
      if (!finished && login === consoleSession && !abort.signal.aborted) throw new Error("Refresh connection ended before completion. Reload cached data to inspect published results.");
    } finally { await reader.cancel(); }
  } catch (error) { if (login === consoleSession && !abort.signal.aborted) { const message = benchmarkSafeMessage(error instanceof Error ? error.message : "Refresh failed"); output.textContent = message; benchmarkProgress({phase:"failed",message}); } }
  finally { if (benchmarkAbort === abort) { benchmarkRefreshRunning = false; benchmarkAbort = null; updateBenchmarkControls(); } }
}
function initializeBenchmarks()       {
  const list = document.querySelector("#benchmark-sources-list") ;
  list.addEventListener("input", updateBenchmarkSource); list.addEventListener("click", benchmarkSourceAction);
  document.querySelector("#benchmark-page-size") .addEventListener("change",()=>{const size=Number(document.querySelector                   ("#benchmark-page-size") .value);if(![25,50,100].includes(size))return;benchmarkPageSize=size;benchmarkPage=0;renderBenchmarkTable();});
  document.querySelector("#benchmark-table-view") .addEventListener("change",()=>{benchmarkTableView=document.querySelector                   ("#benchmark-table-view") .value;benchmarkPage=0;void loadBenchmarkData();});
  document.querySelector("#benchmark-table-filter") .addEventListener("input",()=>{benchmarkFilter=document.querySelector                  ("#benchmark-table-filter") .value;benchmarkPage=0;renderBenchmarkTable();});
  document.querySelector("#benchmark-previous") .addEventListener("click",()=>{if(benchmarkPage>0){benchmarkPage--;renderBenchmarkTable();}});
  document.querySelector("#benchmark-next") .addEventListener("click",()=>{benchmarkPage++;renderBenchmarkTable();});
  renderBenchmarkTable();
}

let modelEnabledDraft                       ={};
async function loadModelsPage()              {
 const output=document.querySelector             ("#models-list") ;
 try{const response=await gatewayFetch("/gateway/v1/models",{headers:await authProvider.headers()});if(!response.ok)throw new Error("Model discovery unavailable");const data=await response.json();
 output.innerHTML=(data.models||[]).map((model    )=>{
  const benchmark=model.benchmark;
  const scores=benchmark?Object.entries(benchmark.scores||{}).map(([name,value])=>`${escapeHTML(name)}: ${(Number(value)*100).toFixed(1)}%`).join(" · "):"";
  const pricing=model.pricing||{};
  return `<article class="subcard"><h3>${escapeHTML(model.id)}</h3><p>Provider: ${escapeHTML(model.provider_id)}</p><label class="checkbox"><input type="checkbox" data-model-enabled="${escapeHTML(model.id)}" ${model.enabled?"checked":""}> Enabled</label><p>Capabilities: ${Object.entries(model.capabilities||{}).map(([name,value])=>`${escapeHTML(name)}: ${escapeHTML(String(value))}`).join(" · ")}</p><p>Estimated USD per million tokens · input: ${pricing.input_per_million??"unknown"} · output: ${pricing.output_per_million??"unknown"}</p><p>${model.benchmark_missing?"No safely matched benchmark scores available":scores}</p>${benchmark?`<p>Sources: ${(benchmark.sources||[]).map(escapeHTML).join(", ")} · ${benchmark.stale?"Stale":"Current"} · retrieved ${escapeHTML(benchmark.latest_retrieval||"unknown")} · evaluated ${escapeHTML(benchmark.latest_evaluation||"unknown")}</p>`:""}</article>`;
 }).join("")||"No physical models discovered. Add a provider first.";
 output.querySelectorAll                  ("[data-model-enabled]").forEach(input=>input.addEventListener("change",async()=>{modelEnabledDraft[input.dataset.modelEnabled ]=input.checked;markDirty();if(await saveConfig())void loadModelsPage();}));
 }catch(error){output.textContent=error instanceof Error?error.message:"Models unavailable";}
}

document.querySelector("#refresh-models") .addEventListener("click",()=>{void loadModelsPage();});

let budgetsDraft                   ={};
function renderBudgetEditor()      {
 const scope=document.querySelector                   ("#budget-scope") .value||"*";
 const budgets=scope==="*"?budgetsDraft.gateway:budgetsDraft.keys?.[scope];
 for(const period of ["daily","monthly"])for(const metric of ["requests","tokens","estimated_cost"]){document.querySelector                  (`#budget-${period}-${metric}`) .value=String(budgets?.[period]?.[metric]||0);}
}
async function loadUsagePage()              {
 const output=document.querySelector             ("#usage-list") ;
 try{
 const keys=await gatewayFetch("/gateway/v1/admin/api-keys",{headers:await authProvider.headers()});if(!keys.ok)throw new Error("Keys unavailable");const metadata=await keys.json();const selector=document.querySelector                   ("#budget-scope") ;const chosen=selector.value;
 selector.innerHTML='<option value="*">Entire gateway</option>'+(metadata.api_keys||[]).map((key    )=>`<option value="${escapeHTML(key.id)}">${escapeHTML(key.name)}</option>`).join("");selector.value=chosen||"*";renderBudgetEditor();
 const response=await gatewayFetch("/gateway/v1/usage",{headers:await authProvider.headers()});if(!response.ok)throw new Error("Usage accounting unavailable");const data=await response.json();
 output.innerHTML=`<p>${escapeHTML(data.qualification||"")}</p><h3>Budgets and resets</h3>`+(data.budgets||[]).filter((b    )=>b.limit.requests||b.limit.tokens||b.limit.estimated_cost).map((b    )=>`<p>${escapeHTML(b.key_id==="*"?"Gateway":((metadata.api_keys||[]).find((k    )=>k.id===b.key_id)?.name||b.key_id))} · ${escapeHTML(b.period)} · resets ${escapeHTML(b.reset_at)} · remaining requests ${b.limit.requests?b.remaining.requests:"unlimited"}, tokens ${b.limit.tokens?b.remaining.tokens:"unlimited"}, estimated USD ${b.limit.estimated_cost?Number(b.remaining.estimated_cost).toFixed(6):"unlimited"}</p>`).join("")+`<h3>Current UTC usage</h3><div class="benchmark-table-scroll"><table><thead><tr><th>Key</th><th>Period</th><th>Model</th><th>Client requests</th><th>Input / output tokens</th><th>Estimated tokens</th><th>Estimated USD</th></tr></thead><tbody>`+(data.usage||[]).filter((r    )=>r.key_id!=="*").map((r    )=>`<tr><td>${escapeHTML(r.key_name||r.key_id)}</td><td>${escapeHTML(r.period)}</td><td>${escapeHTML(r.model||"Client requests")}</td><td>${r.requests}</td><td>${r.input_tokens} / ${r.output_tokens}</td><td>${r.estimated_tokens}</td><td>${Number(r.estimated_cost).toFixed(6)}${r.unknown_cost_attempts?" + unknown costs":""}</td></tr>`).join("")+`</tbody></table></div>`;
 }catch(error){output.textContent=error instanceof Error?error.message:"Usage unavailable";}
}
document.querySelector("#budget-scope") .addEventListener("change",renderBudgetEditor);
for(const period of ["daily","monthly"])for(const metric of ["requests","tokens","estimated_cost"]){document.querySelector                  (`#budget-${period}-${metric}`) .addEventListener("change",()=>{
 const scope=document.querySelector                   ("#budget-scope") .value||"*";const value=Number(document.querySelector                  (`#budget-${period}-${metric}`) .value);if(!Number.isFinite(value)||value<0){saveState.textContent="Budgets must be nonnegative";return;}
 let budgets                   ;if(scope==="*")budgets=budgetsDraft.gateway ||= {};else{budgetsDraft.keys ||= {};budgets=budgetsDraft.keys[scope] ||= {};}
 (budgets[period] ||= {})[metric]=value;markDirty();void saveConfig();
});}
document.querySelector("#refresh-usage") .addEventListener("click",()=>{void loadUsagePage();});

let activityStream                 =null;
let activityRows      =[];
function stopActivity()     {activityStream?.close();activityStream=null;activityRows=[];document.querySelector("#activity-list") .textContent="";}
function renderActivity()     {
 document.querySelector("#activity-list") .innerHTML=`<div class="benchmark-table-scroll"><table><thead><tr><th>Time / request</th><th>Event</th><th>API key</th><th>Requested → selected</th><th>Outcome</th><th>Duration</th><th>Tokens</th><th>Estimated USD</th></tr></thead><tbody>`+activityRows.slice(-200).reverse().map((e    )=>`<tr><td>${escapeHTML(e.at||"")}<br>${escapeHTML(e.request_id||"")}</td><td>${escapeHTML(e.kind)}</td><td>${escapeHTML(e.key_name||e.key_id||"")}</td><td>${escapeHTML(e.requested_model||"")} → ${escapeHTML(e.selected_model||"")}<br>${escapeHTML(e.provider||"")}</td><td>${escapeHTML(e.outcome||"")} ${e.status||""}</td><td>${e.duration_ms??""} ms</td><td>${e.input_tokens||0} / ${e.output_tokens||0}${e.estimated_tokens?" (estimated)":""}</td><td>${e.unknown_cost?"unknown":e.estimated_cost==null?"—":Number(e.estimated_cost).toFixed(6)}</td></tr>`).join("")+`</tbody></table></div>`;
}
async function startActivity()              {
 if(activityStream || !consoleSession)return;
 const status=document.querySelector("#activity-status") ;
 try{const response=await gatewayFetch("/gateway/v1/activity?limit=200",{headers:await authProvider.headers()});if(!response.ok)throw new Error();const history=await response.json();activityRows=history.events||[];renderActivity();
 if(location.hash!=="#activity" || !consoleSession)return;
 const last=activityRows.at(-1)?.id||0;const stream=new EventSource(`/gateway/v1/activity/events?after_id=${last}`);activityStream=stream;
 stream.onopen=()=>{status.textContent="Live · metadata only";};stream.onerror=()=>{status.textContent="Disconnected · reconnecting and replaying available events";};
 stream.addEventListener("activity",event=>{const e=JSON.parse((event                ).data);if(!activityRows.some(row=>row.id===e.id)){activityRows.push(e);activityRows=activityRows.slice(-200);renderActivity();}});
 stream.addEventListener("gap",()=>{status.textContent="History gap · events were pruned or a viewer/recording queue overflowed";});
 }catch{status.textContent="Activity unavailable. Sign in and retry.";}
}



// Browser authentication uses an HttpOnly cookie. Only the CSRF token is in memory.




class CookieSessionProvider                         {
  async headers()                       { return csrfToken ? { "X-CSRF-Token":csrfToken } : {}; }
}

const authProvider               = new CookieSessionProvider();
async function gatewayFetch(url       ,options             )                  {
  const login = consoleSession;
  const response=await fetch(url,options);
  if(response.status===401 && login===consoleSession){signOutConsole();document.querySelector                   ("#login-dialog")?.showModal?.();}
  return response;
}
const form = document.querySelector                 ("#config-form") ;
const providerList = document.querySelector                ("#providers-list") ;
const saveState = document.querySelector                 ("#save-state") ;
let consoleSession = "";
let csrfToken="";
let configurationRevision = "";
let loadedConfiguration                    = {};
let editGeneration=0;
let saveRunning=false;
let saveQueued=false;
let saveConflict=false;
let editingProvider            =null;
let editingOriginal                        =null;
function openProviderEditor(index       ,adding=false)     {editingProvider=index;editingOriginal=adding?null:structuredClone(providers[index]);providerList.querySelector                   (`[data-provider-dialog="${index}"]`)?.showModal?.();}
function cancelProviderEditor()     {if(editingProvider===null)return;if(editingOriginal)providers[editingProvider]=editingOriginal;else providers.splice(editingProvider,1);editingProvider=null;editingOriginal=null;renderProviders();}
let signedInUsername = "";
let benchmarksEnabledSaved = false;
function updateConsoleIdentity()       {
  const signedIn = Boolean(consoleSession && signedInUsername);
  document.querySelector                   ("#console-login") .hidden = signedIn;
  document.querySelector                   ("#console-logout") .hidden = !signedIn;
  document.querySelector("#signed-in-user") .textContent = signedIn ? signedInUsername : "Not signed in";
}
function signOutConsole()       {
 stopActivity();
  consoleSession = "";csrfToken="";loadedConfiguration={};editingProvider=null;editingOriginal=null;
  configurationRevision = "";
  signedInUsername = "";
  clearNewAPIKey();
  document.querySelector                   ("#new-key-dialog") .close();
  document.querySelector                   ("#create-key-dialog") .close();
  document.querySelector                  ("#client-key-name") .value = "";
  providers = [];
  configurationLoaded = false;
  classifierQuestionOverrides = {};
  classifierNoulThresholds = {};
  renderClassifierQuestion();
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
async function createBrowserSession(username       ,password       )              {
 const response=await fetch("/gateway/v1/session",{method:"POST",credentials:"same-origin",headers:{"Content-Type":"application/json"},body:JSON.stringify({username,password})});
 if(!response.ok)throw new Error("Sign-in failed");const session=await response.json();csrfToken=session.csrf_token;consoleSession=session.expires_at;signedInUsername=session.username;updateConsoleIdentity();
}

const providerDefaults = { id: "", type: "openai_compatible", api_url: "", api_key: "", api_key_env: "", api_key_file: "", management_env: "", portal_url: "", client_id: "", headers: "", models: "", discover: true, transport: "", organization: "", project: "", rewrites: "", tls_ca: "", tls_server: "", connect_timeout: "", request_timeout: "", discovery_timeout: "", enabled:true, treat_models_as_free:false };
let providers                                     = [];




const providerStatuses = new WeakMap                                                  ();
const classifierCatalogs = new WeakMap                                            ();
let classifierDiscoveryPending = false;
let configurationLoaded = false;
let requestedClassifier = "";
let legacyClassifier                             = null;
const classifierSelect = document.querySelector                   ("#classifier-model") ;
// Mirrors the typed protocol's defaults. Only edited entries are saved.
const classifierQuestionDefaults                                                                          = {
  task_type: { instructions:"Which task type best describes this request?", criteria:{ general:"General knowledge or other task", conversation:"Open-ended conversation", coding:"Writing or debugging code", sysadmin:"Operating systems, infrastructure, or deployment", reasoning:"Multi-step logical reasoning", math:"Mathematics or quantitative calculation", research:"Research or source synthesis", summarization:"Summarizing provided content", extraction:"Extracting structured facts", creative:"Creative writing or ideation", translation:"Translating between languages" } },
  complexity: { instructions:"How complex is this request?", criteria:{ low:"Simple and direct", medium:"Requires several steps or moderate judgment", high:"Requires deep reasoning, extensive context, or difficult execution" } },
  tools: { instructions:"Does this request require tool or function calling?", criteria:{ true:"", false:"" } },
  vision: { instructions:"Does this request require understanding image content?", criteria:{ true:"", false:"" } },
  reasoning: { instructions:"Does this request require a reasoning-capable model?", criteria:{ true:"", false:"" } },
  structured_output: { instructions:"Does this request require structured or schema-constrained output?", criteria:{ true:"", false:"" } },
  latency_sensitive: { instructions:"Is low latency especially important for this request?", criteria:{ true:"", false:"" } },
};
let classifierQuestionOverrides                                                                          = {};
let classifierNoulThresholds                         = {};
const classifierQuestion = document.querySelector                   ("#classifier-question") ;
const classifierInstructions = document.querySelector                     ("#classifier-question-instructions") ;
const classifierOption = document.querySelector                   ("#classifier-question-option") ;
const classifierRubric = document.querySelector                     ("#classifier-question-rubric") ;
const classifierThreshold = document.querySelector                  ("#classifier-question-threshold") ;
function renderClassifierRubric()       {
  const name = classifierQuestion.value || "task_type";
  classifierRubric.value = classifierQuestionOverrides[name]?.criteria?.[classifierOption.value] ?? classifierQuestionDefaults[name].criteria[classifierOption.value] ?? "";
  classifierRubric.placeholder = "Use the built-in rubric";
}
function renderClassifierQuestion()       {
  const name = classifierQuestion.value || "task_type";
  const defaults = classifierQuestionDefaults[name];
  classifierInstructions.value = classifierQuestionOverrides[name]?.instructions ?? defaults.instructions;
  classifierOption.innerHTML = Object.keys(defaults.criteria).map(option => `<option value="${option}">${option}</option>`).join("");
  classifierOption.value = Object.keys(defaults.criteria)[0];
  const isNoul = name !== "task_type" && name !== "complexity";
  document.querySelector                  ("#classifier-threshold-label") .hidden = !isNoul;
  classifierThreshold.value = name in classifierNoulThresholds ? String(classifierNoulThresholds[name]) : "";
  renderClassifierRubric();
}
function editClassifierQuestion(field                          , value       )       {
  const name = classifierQuestion.value || "task_type";
  const override = classifierQuestionOverrides[name] ||= {};
  const defaults = classifierQuestionDefaults[name];
  if (field === "instructions") {
    if (!value || value === defaults.instructions) delete override.instructions;
    else override.instructions = value;
  } else {
    const option = classifierOption.value;
    const criteria = override.criteria ||= {};
    if (!value || value === defaults.criteria[option]) delete criteria[option];
    else criteria[option] = value;
    if (!Object.keys(criteria).length) delete override.criteria;
  }
  if (!Object.keys(override).length) delete classifierQuestionOverrides[name];
  markDirty();
}

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
      <div class="provider-card-head"><div class="provider-card-title"><span class="provider-index">${index + 1}</span> <span data-provider-title="${index}">${value(provider,"id") || `Provider ${index + 1}`}</span></div><div class="provider-card-actions"><button class="test-provider button secondary" type="button" data-test="${index}">${subscriptionProvider(provider) ? "Check status" : "Test connection"}</button>${subscriptionProvider(provider) ? `<button class="login-provider button secondary" type="button" data-login="${index}">Log in</button>` : ""}<button class="button secondary" type="button" data-edit="${index}">Edit</button><button class="remove-provider" type="button" data-remove="${index}">Remove</button></div></div>
      <div class="provider-result" data-result="${index}" role="status" aria-live="polite">Not checked yet</div>
      <dialog data-provider-dialog="${index}"><h3>${value(provider,"id") || "Add provider"}</h3><label class="checkbox"><input type="checkbox" data-provider="${index}" data-key="enabled" ${provider.enabled!==false?"checked":""}> Provider enabled</label><label class="checkbox"><input type="checkbox" data-provider="${index}" data-key="treat_models_as_free" ${provider.treat_models_as_free?"checked":""}> Treat models as free</label>${subscriptionProvider(provider) ? `<div class="grid two"><label data-tooltip="The stable ID used as the prefix of public model names. Built-in providers receive this automatically; custom API endpoints must define it.">Provider ID<input data-provider="${index}" data-key="id" value="${value(provider,"id")}" readonly required /></label><label data-tooltip="The subscription integration used by Torii. OAuth credentials are encrypted in the gateway database.">Type<select data-provider="${index}" data-key="type"><option ${provider.type === "openai-codex" ? "selected" : ""} value="openai-codex">OpenAI Codex</option><option ${provider.type === "nous" ? "selected" : ""} value="nous">Nous</option></select></label></div><p class="provider-subscription-note">This subscription provider uses its OAuth login. API URL, API keys, and discovery controls are not used.</p>` : `<div class="grid three">
        <label data-tooltip="The stable ID used as the prefix of public model names. Built-in providers receive this automatically; custom API endpoints must define it.">Provider ID<input data-provider="${index}" data-key="id" value="${value(provider,"id")}" ${isKnownProvider(provider) ? "readonly" : ""} required /></label>
        <label data-tooltip="The provider integration Torii should use. Jev and Jev-compatible providers supply classifier models, not chat models.">Type<select data-provider="${index}" data-key="type"><option ${provider.type === "openai_compatible" ? "selected" : ""} value="openai_compatible">OpenAI compatible</option><option ${provider.type === "jev" ? "selected" : ""} value="jev">TypeSafe AI · Jev</option><option ${provider.type === "jev_compatible" ? "selected" : ""} value="jev_compatible">Jev compatible · Laya / custom</option><option ${provider.type === "openrouter" ? "selected" : ""} value="openrouter">OpenRouter</option><option ${provider.type === "gemini" ? "selected" : ""} value="gemini">Gemini</option><option ${provider.type === "xai" ? "selected" : ""} value="xai">xAI</option><option ${provider.type === "anthropic" ? "selected" : ""} value="anthropic">Anthropic</option><option ${provider.type === "openai" ? "selected" : ""} value="openai">OpenAI</option><option ${provider.type === "openai-codex" ? "selected" : ""} value="openai-codex">OpenAI Codex</option><option ${provider.type === "ollama" ? "selected" : ""} value="ollama">Ollama</option><option ${provider.type === "nous" ? "selected" : ""} value="nous">Nous</option></select></label>
        <label data-tooltip="The complete upstream URL Torii should call. Include any required API path yourself; Torii does not add /v1 to this field."><span>API URL</span><input data-provider="${index}" data-key="api_url" value="${value(provider,"api_url")}" placeholder="https://api.example.com" /></label>
        ${provider.type === "ollama" ? "" : `<label data-tooltip="Enter an upstream API key to replace the configured credential. Save encrypts it in SQLite; saved keys cannot be revealed.">API key<span class="password-field"><input id="provider-api-key-${index}" type="password" autocomplete="new-password" data-provider="${index}" data-key="api_key" placeholder="${provider.bearer_token ? "Configured · leave blank to keep" : "Enter provider API key"}" /><button class="reveal-button" type="button" data-reveal-provider-key="${index}" aria-controls="provider-api-key-${index}" aria-label="Reveal entered provider API key" aria-pressed="false">Reveal</button></span></label>`}
        ${provider.type === "ollama" ? `<label data-tooltip="Time allowed for model loading and the complete response, including streaming. Leave blank for 5 minutes. Client cancellation still applies.">Request timeout<input data-provider="${index}" data-key="request_timeout" value="${value(provider,"request_timeout")}" placeholder="5m" /></label>` : ""}
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
        ${provider.type === "ollama" ? "" : `<label data-tooltip="Maximum time allowed for an upstream request. Use a Go duration such as 60s.">Request timeout<input data-provider="${index}" data-key="request_timeout" value="${value(provider,"request_timeout")}" placeholder="60s" /></label>`}
        <label data-tooltip="Maximum time allowed while discovering the provider model catalog. Use a Go duration such as 10s.">Discovery timeout<input data-provider="${index}" data-key="discovery_timeout" value="${value(provider,"discovery_timeout")}" placeholder="10s" /></label>
        <label class="checkbox" data-tooltip="When enabled, Torii queries the provider for its available models. Disable this only when you provide a static model list."><input data-provider="${index}" data-key="discover" type="checkbox" ${checked(provider,"discover")} /> Discover models</label>
      </div></details>`}
      <button class="button primary" type="button" data-confirm-provider="${index}">Save provider</button><button class="button quiet" type="button" data-cancel-provider="${index}">Cancel</button></dialog>
    </article>`).join("");
  providerList.querySelectorAll                   ("[data-edit]").forEach(button=>button.addEventListener("click",()=>{openProviderEditor(Number(button.dataset.edit));}));
  providerList.querySelectorAll                   ("[data-confirm-provider]").forEach(button=>button.addEventListener("click",async()=>{const dialog=button.closest("dialog") ;const index=editingProvider;editingProvider=null;markDirty();if(await saveConfig()){editingOriginal=null;dialog.close();}else{editingProvider=index;}}));
  providerList.querySelectorAll                   ("[data-cancel-provider]").forEach(button=>button.addEventListener("click",cancelProviderEditor));
  providerList.querySelectorAll                   ("[data-provider-dialog]").forEach(dialog=>dialog.addEventListener("cancel",cancelProviderEditor));
  providerList.querySelectorAll                   ("[data-remove]").forEach(button => button.addEventListener("click", () => { providers.splice(Number(button.dataset.remove), 1); renderProviders(); markDirty();void saveConfig(); }));
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
  if(editingProvider!==null)providerList.querySelector                   (`[data-provider-dialog="${editingProvider}"]`)?.showModal?.();
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
 providers[index].treat_models_as_free=control.value==="ollama";
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

function markDirty()       { editGeneration++; saveState.textContent = "Draft changes"; }

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

function applyConfig(config                     , revision = "")       {
  configurationRevision = revision;
  loadedConfiguration=structuredClone(config);modelEnabledDraft={...config.model_enabled};budgetsDraft=structuredClone(config.budgets||{});renderBudgetEditor();saveConflict=false;
  classifierDiscoveryPending = true;
  configurationLoaded = true;
  legacyClassifier = null;
  const classifier = config.router?.classifier || {};
  (form.elements.namedItem("router.allow_paid_models")                    ).checked = config.router?.allow_paid_models !== false;
  classifierQuestionOverrides = structuredClone(classifier.questions || {});
  classifierNoulThresholds = { ...(classifier.noul_thresholds || {}) };
  (form.elements.namedItem("router.classifier.timeout")                    ).value = "";
  (form.elements.namedItem("router.classifier.minimum_confidence")                    ).value = "";
  (form.elements.namedItem("router.classifier.on_failure")                     ).value = "";
  renderClassifierQuestion();
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
    enabled:provider.enabled!==false,
    treat_models_as_free:provider.treat_models_as_free??provider.type==="ollama",
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
  const login = consoleSession;
  try {
    const response = await gatewayFetch("/gateway/v1/config", { headers: await authProvider.headers() });
    if (login !== consoleSession) return;
    if (!response.ok) { if (response.status === 401 || response.status === 403) {signOutConsole();saveState.textContent="Sign in to configure the gateway";document.querySelector                   ("#login-dialog")?.showModal?.();} return; }
    const configuration = await response.json();
    if (login !== consoleSession) return;
    applyConfig(configuration, response.headers.get("ETag") || "");
    saveState.textContent = "Loaded from gateway";
    void testAllProviders();
  } catch { saveState.textContent = "Gateway config unavailable"; }
}

async function saveConfig()                   {
  saveQueued=true;
  if(saveRunning)return false;
  if(saveConflict){saveState.textContent="Conflict · reload and reconcile your draft";return false;}
  saveRunning=true;
  let success=false;
  try {
    while(saveQueued){
      saveQueued=false;
      if(!configurationRevision)throw new Error("Reload configuration before saving");
      const generation=editGeneration,login=consoleSession;
      const configuration=readConfig();
      saveState.textContent="Saving…";
      const response=await gatewayFetch("/gateway/v1/config",{method:"PUT",headers:{"Content-Type":"application/json","If-Match":configurationRevision,...await authProvider.headers()},body:JSON.stringify(configuration)});
      if(login!==consoleSession)return false;
      if(response.status===412 || response.status===428){saveConflict=true;throw new Error("Configuration changed · your draft is still here. Copy your edits, reload, and reconcile them.");}
      if(!response.ok)throw new Error(redactProviderKeys(`Save failed: ${await response.text()}`));
      const result=await response.json();
      configurationRevision=response.headers.get("ETag")||"";
      const view=await gatewayFetch("/gateway/v1/config",{headers:await authProvider.headers()});
      if(!view.ok)throw new Error("Saved; reload failed. Refresh configuration before editing again.");
      if(view.headers.get("ETag")!==configurationRevision){saveConflict=true;throw new Error("Another tab changed the saved configuration. Your draft is preserved.");}
      const committed=await view.json();
      if(generation===editGeneration){applyConfig(committed,configurationRevision);void testAllProviders();}
      else {
        loadedConfiguration=structuredClone(committed);
        providers.forEach(p=>{const submitted=configuration.providers.find((q    )=>q.id===p.id);const stored=committed.providers.find((q    )=>q.id===p.id);if(stored && submitted?.bearer_token?.value===p.api_key){p.bearer_token=stored.bearer_token;p.api_key="";}});
      }
      saveState.textContent=saveQueued?"Saving…":result.restart_required?"Saved · restart required for listener or file logging changes":"Saved";
      document.querySelector                   ("#save-button") .hidden=true;
      success=true;
    }
  } catch(error){saveQueued=false;success=false;saveState.textContent=error instanceof Error?error.message:"Save failed · draft preserved";toast(saveState.textContent||"Save failed");document.querySelector                   ("#save-button") .hidden=false;}
  finally{saveRunning=false;}
  return success;
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
  if(current==="activity")void startActivity();else if(activityStream)stopActivity();
  if(current==="state" && configurationLoaded)void loadUsagePage();
  if(current==="models" && configurationLoaded)void loadModelsPage();
  if (current === "routing") updateClassifierOptions();
  if (current === "intelligence" && configurationLoaded && benchmarksEnabledSaved) void loadBenchmarkData();
}

function setNested(target                     , path        , raw         )       {
  const parts = path.split("."); let cursor = target;
  parts.forEach((part, index) => { if (index === parts.length - 1) cursor[part] = raw; else cursor = cursor[part] ||= {}; });
}
function readProviderConfig(provider                                  )                      {
  if (provider.type === "ollama") {
    // Preserve the native Ollama inference deadline without generic options.
    const url = new URL(String(provider.api_url));
    const output                      = { enabled:provider.enabled!==false,treat_models_as_free:provider.treat_models_as_free??true,id:provider.id, type:provider.type, base_url:`${url.protocol}//${url.host}${url.pathname === "/" ? "" : url.pathname}` };
    if (String(provider.request_timeout || "").trim()) output.request_timeout = String(provider.request_timeout).trim();
    return output;
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
  const config                      = structuredClone(loadedConfiguration);
 config.server={...config.server};config.state={...config.state};config.router={...config.router,classifier:{},sessions:{...config.router?.sessions},quota:{...config.router?.quota},selection:{...config.router?.selection}};
  new FormData(form).forEach((raw, name) => { if (name === "router.quota.allowances") return; if (name === "router.allow_paid_models") setNested(config, name, true); else if (name === "router.selection.minimum_quality") { try { setNested(config, name, JSON.parse(String(raw))); } catch { /* keep invalid drafts editable */ } } else if (String(raw).trim() !== "") setNested(config, name, raw); });
  config.model_enabled={...modelEnabledDraft};config.budgets=structuredClone(budgetsDraft);
 config.providers = providers.flatMap((p,index)=>editingProvider===index?(editingOriginal?[readProviderConfig(editingOriginal)]:[]):[readProviderConfig(p)]);
  // Unchecked checkboxes are absent from FormData. Emit the policy explicitly
  // so saving "false" cannot restore the backend's default of allowing paid models.
  config.router.allow_paid_models = Boolean((form.elements.namedItem("router.allow_paid_models")                    )?.checked);
  const selected = classifierSelect.value;
  const onFailure = String((form.elements.namedItem("router.classifier.on_failure")                     )?.value || "");
  const controls                      = {};
  const timeout = String((form.elements.namedItem("router.classifier.timeout")                    )?.value || "").trim();
  const minimumConfidence = String((form.elements.namedItem("router.classifier.minimum_confidence")                    )?.value || "").trim();
  if (timeout) controls.timeout = timeout;
  if (minimumConfidence) {
    const value = Number(minimumConfidence);
    if (!Number.isFinite(value) || value < 0 || value > 1) throw new Error("Minimum confidence must be between 0 and 1");
    controls.minimum_confidence = value;
  }
  if (Object.keys(classifierQuestionOverrides).length) controls.questions = structuredClone(classifierQuestionOverrides);
  if (Object.keys(classifierNoulThresholds).length) controls.noul_thresholds = { ...classifierNoulThresholds };
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
  for (const name of ["timeout", "minimum_confidence", "questions", "noul_thresholds"]) delete config.router.classifier[name];
  config.router.classifier = { ...config.router.classifier, ...controls };
  const allowances = String((form.elements.namedItem("router.quota.allowances")                       )?.value || "").split("\n").map(line => line.split("=").map(v => v.trim())).filter(pair => pair.length === 2 && pair[0]).reduce((all, [key, raw]) => { const value = Number(raw); if (Number.isFinite(value)) all[key] = value; return all; }, {}                          );
  if (Object.keys(allowances).length) config.router.quota.allowances = allowances;
  config.intelligence = readBenchmarkConfiguration();
  return config;
}

function updateBenchmarkControls()       {
  const enabled = document.querySelector                  ("#intelligence-enabled") .checked;
  document.querySelector                   ("#refresh-benchmarks") .disabled = !benchmarksEnabledSaved || !enabled || benchmarkDirty || benchmarkRefreshRunning;
  document.querySelector("#refresh-benchmarks") .textContent = benchmarkRefreshRunning ? "Refreshing…" : "Refresh benchmarks";
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
      message: `${connection} · ${count > 0 ? "Working" : "Not working"} · ${count} model${count === 1 ? "" : "s"} discovered${count === 0 ? " (no models available)" : ""}${configuration.discover_models === false ? " · Discovery disabled: Save & reload uses the static model list" : ""}`,
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

document.querySelector("#add-provider") .addEventListener("click", () => { providers.push({ ...providerDefaults }); renderProviders(); openProviderEditor(providers.length-1,true); });
document.querySelector("#refresh-benchmarks") .addEventListener("click", () => inspectBenchmarks(true));
document.querySelector("#inspect-benchmarks") .addEventListener("click", () => { void inspectBenchmarks(false); });
document.querySelector("#save-button") .addEventListener("click", () => { void saveConfig(); });
document.querySelector("#reset-button") .addEventListener("click", () => { location.reload(); });
document.querySelector("#theme-toggle") .addEventListener("click", () => { const dark = document.documentElement.dataset.theme !== "dark"; document.documentElement.dataset.theme = dark ? "dark" : "light"; localStorage.setItem("torii-theme", dark ? "dark" : "light"); });
form.addEventListener("input", markDirty);
form.addEventListener("change",event=>{const target=event.target               ;if(!target.closest("dialog") && !["classifier-question","classifier-question-option","budget-scope"].includes(target.id) && !target.id.startsWith("budget-")){void saveConfig();}});
window.addEventListener("hashchange", navigate);
document.documentElement.dataset.theme = localStorage.getItem("torii-theme") || (matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light");
classifierSelect.addEventListener("change", () => { requestedClassifier = classifierSelect.value; legacyClassifier = null; markDirty(); });
classifierQuestion.addEventListener("change", renderClassifierQuestion);
classifierOption.addEventListener("change", renderClassifierRubric);
classifierInstructions.addEventListener("input", () => editClassifierQuestion("instructions", classifierInstructions.value));
classifierRubric.addEventListener("input", () => editClassifierQuestion("criteria", classifierRubric.value));
classifierThreshold.addEventListener("input", () => {
  const name = classifierQuestion.value;
  if (classifierThreshold.value === "") delete classifierNoulThresholds[name];
  else classifierNoulThresholds[name] = Number(classifierThreshold.value);
  markDirty();
});
renderClassifierQuestion();
document.addEventListener("click", () => { document.querySelectorAll("label.tooltip-open").forEach(label => label.classList.remove("tooltip-open")); });
initializeBenchmarks(); renderProviders(); updateConsoleIdentity(); updateBenchmarkControls(); navigate(); void checkSetup();

document.querySelector("#console-login") .addEventListener("click",()=>location.assign("/login"));
document.querySelector("#console-logout") .addEventListener("click",async()=>{try{const response=await gatewayFetch("/gateway/v1/session",{method:"DELETE",headers:await authProvider.headers()});if(!response.ok)throw new Error();signOutConsole();location.assign("/login");}catch{toast("Sign-out failed. Retry to revoke the session.");}});
document.querySelector("#login-form") .addEventListener("submit",async event=>{
  event.preventDefault();const username=document.querySelector                  ("#login-username") .value;const input=document.querySelector                  ("#login-password") ;const password=input.value;input.value="";
  try{await createBrowserSession(username,password);const response=await gatewayFetch("/gateway/v1/config",{headers:await authProvider.headers()});if(!response.ok)throw new Error();applyConfig(await response.json(),response.headers.get("ETag")||"");signedInUsername=username;updateConsoleIdentity();document.querySelector("#login-error") .textContent="";document.querySelector                   ("#login-dialog") .close();saveState.textContent="Signed in";void testAllProviders();void loadCredentialMetadata();}
  catch{signOutConsole();document.querySelector("#login-error") .textContent="Sign-in failed. Check your administrator username and password, then try again in one minute if rate limited.";}
});
async function loadCredentialMetadata()              {
  const list=document.querySelector             ("#credential-list") ;
  const login=consoleSession;
  try{const response=await gatewayFetch("/gateway/v1/admin/api-keys",{headers:await authProvider.headers()});if(!response.ok)throw new Error();const body=await response.json();if(login!==consoleSession)return;list.innerHTML=(body.api_keys||[]).map((key    )=>`<div class="credential-card"><div class="credential-card-content"><h3 class="credential-card-name">${escapeHTML(key.name)}</h3><p class="credential-card-meta">Created ${escapeHTML(key.created_at)}</p></div><button type="button" class="button quiet credential-card-delete" data-delete-key="${escapeHTML(key.id)}" aria-label="Delete API key ${escapeHTML(key.name)}">Delete</button></div>`).join("") || "No gateway API keys have been created. Click Add credential to generate one.";
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
  const login=consoleSession;
  const name=document.querySelector                  ("#client-key-name") .value.trim();
  try{const response=await gatewayFetch("/gateway/v1/admin/api-keys",{method:"POST",headers:{"Content-Type":"application/json",...await authProvider.headers()},body:JSON.stringify({name})});const result=await response.json();if(!response.ok)throw new Error(result.error||"API key creation failed");if(login!==consoleSession)return;document.querySelector                  ("#client-key-name") .value="";document.querySelector                   ("#create-key-dialog") .close();showNewAPIKey(result.api_key,result.warning);await loadCredentialMetadata();}
  catch(error){document.querySelector("#create-key-error") .textContent=error instanceof Error?error.message:"API key creation failed";}
  finally{button.disabled=false;}
});
document.querySelector("#intelligence-enabled") .addEventListener("change",()=>{
  benchmarkDirty = true; updateBenchmarkControls();markDirty();
});

async function checkSetup()              {
 try{const response=await fetch("/gateway/v1/setup",{cache:"no-store"});if(!response.ok)throw new Error();const status=await response.json();if(status.setup_required){document.querySelector                   ("#setup-dialog") .showModal();return;}
 const view=await fetch("/gateway/v1/session",{credentials:"same-origin",cache:"no-store"});if(!view.ok){location.assign("/login");return;}const session=await view.json();csrfToken=session.csrf_token;consoleSession=session.expires_at;signedInUsername=session.username;updateConsoleIdentity();await loadServerConfig();void loadCredentialMetadata();
 }catch{saveState.textContent="Unable to check administrator setup; refresh to retry.";}
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
  try{const response=await fetch("/gateway/v1/setup",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({username,password,setup_code:code})});const result=await response.json();if(!response.ok)throw new Error(result.error||"Setup failed");await createBrowserSession(username,password);document.querySelector                   ("#setup-dialog") .close();await loadServerConfig();void loadCredentialMetadata();showNewAPIKey(result.api_key,result.warning);}
  catch(error){document.querySelector("#setup-error") .textContent=error instanceof Error?error.message:"Administrator setup failed.";}
});
document.querySelector("#generate-api-key") .addEventListener("click",async()=>{
  if(!confirm("Generate a new API key? All clients using the current key will need updating."))return;
  const login=consoleSession;
  try{const response=await gatewayFetch("/gateway/v1/admin/api-key",{method:"POST",headers:await authProvider.headers()});if(!response.ok)throw new Error();const result=await response.json();if(login!==consoleSession)return;showNewAPIKey(result.api_key,result.warning);}
  catch{toast("API key generation failed. Sign in as the administrator.");}
});
function clearNewAPIKey()     {document.querySelector                     ("#new-api-key") .value="";}
document.querySelector("#new-key-dialog") .addEventListener("close",clearNewAPIKey);
document.querySelector("#new-key-dialog") .addEventListener("cancel",clearNewAPIKey);
document.querySelector("#close-new-key") .addEventListener("click",()=>{clearNewAPIKey();document.querySelector                   ("#new-key-dialog") .close();});

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
let benchmarkPageSize = matchMedia("(max-width: 760px)").matches?5:25;
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
  clearBenchmarkRecords();closeBenchmarkEvidence();
  benchmarkEditing=false;benchmarkInlineDrafts={};benchmarkInlineSaving=false;benchmarkInlineSaveSequence++;benchmarkSummaryEnabled=["model","overall","capabilities","sources","freshness","evidence"];benchmarkMeasurementEnabled=benchmarkMeasurementFields.slice(0,8).map(field=>field.key);
  for(const id of ["benchmark-configuration","benchmark-upload-dialog","benchmark-fields-dialog"])document.querySelector                   (`#${id}`)?.close?.();
  document.querySelector("#benchmark-edit-data") .innerHTML=consoleEditButtonMarkup("Edit Data");document.querySelector("#benchmark-edit-data") .setAttribute("aria-pressed","false");
  document.querySelector("#benchmark-edit-data") .setAttribute("aria-label","Edit Data");
  document.querySelector("#benchmark-edit-data") .setAttribute("title","Edit Data");
  document.querySelector("#benchmark-inline-status") .textContent="";inspectedBenchmarkModel="";document.querySelector("#benchmark-evidence-content") .textContent="";
  benchmarkAbort?.abort(); benchmarkAbort = null; benchmarkReloadSequence++;
  benchmarkConfiguration = {}; benchmarkSources = []; benchmarkSourceStatus = {}; benchmarkSourceActivity = {}; benchmarkModels = {};
  benchmarkPage = 0; benchmarkFilter = ""; benchmarkTableView = "models";
  benchmarkPageSize = matchMedia("(max-width: 760px)").matches?5:25;
  benchmarkDirty = false; benchmarkRefreshRunning = false; benchmarkProgressLog = [];
  benchmarkProgressCounts = {total:0,completed:0,failed:0};
  document.querySelector                  ("#intelligence-enabled") .checked = false;
  document.querySelector                ("#benchmark-refresh-panel") .hidden = true;
  document.querySelector("#benchmark-refresh-log") .textContent = "";
  document.querySelector                  ("#benchmark-table-filter") .value = "";
  document.querySelector                   ("#benchmark-table-view") .value = "models";
  document.querySelector                   ("#benchmark-page-size") .value = String(benchmarkPageSize);
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
    return `<article class="provider-card"><div class="provider-card-head"><h2>${escapeHTML(definition?.name || source.type)}${source._new ? "" : ` · ${escapeHTML(source.id)}`}</h2><label class="checkbox" data-tooltip="Enable this preconfigured source for background and manual refreshes. Save and reload before ingestion starts."><input type="checkbox" ${benchmarkAttributes(index, "enabled")} ${source.enabled ? "checked" : ""} /> Enabled</label></div>
      <div class="provider-result" data-benchmark-status="${index}" role="status" aria-live="polite"></div>
      <div class="benchmark-source-options" ${source.enabled ? "" : "hidden"}>
      <p>${escapeHTML(definition?.note || "Uses your saved source configuration.")}</p>
      ${needsKey ? `<label data-tooltip="Save encrypts new values in SQLite; saved credentials cannot be revealed. Public Hugging Face datasets allow anonymous access; an optional token authenticates requests to Hugging Face.">${huggingFaceToken ? "Hugging Face Token (Optional)" : "API Key"}<span class="password-field"><input type="password" autocomplete="new-password" ${benchmarkAttributes(index, "api_key_input")} placeholder="${source.api_key?.credential ? "Configured · leave blank to keep" : huggingFaceToken ? "Optional Hugging Face token" : "Enter source API key"}" /><button type="button" class="reveal-button" data-benchmark-action="reveal-key" ${benchmarkAttributes(index, "")}>Reveal</button></span></label>` : ""}
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
function benchmarkLabel(name       )       {return name.replace(/_/g," ").replace(/\b[a-z]/g,letter=>letter.toUpperCase());}
function benchmarkEvidence(row    )       {
 const scores=["overall",...benchmarkCapabilities];
 const operations                      ={input_price:"Input USD / 1M",output_price:"Output USD / 1M",latency:"Latency Seconds",ttft:"First Token Seconds",throughput:"Tokens / Second",context_length:"Context Tokens"};
 const scoreFields=scores.map(name=>[`${benchmarkLabel(name)} %`,benchmarkNumber(row.scores?.[name],true)]);
 const operationFields=Object.entries(operations).map(([name,label])=>[label,benchmarkNumber(row.operations?.[name])]);
 const provenanceFields=[["Model ID",row.id],["Providers",(row.provider_ids||[]).join(", ")||"—"],["Sources",(row.sources||[]).join(", ")||"—"],["Match",["approximate","ambiguous","unmatched"].includes(row.match)?`${benchmarkLabel(row.match)} · display only`:"Matched"],["Data Status",row.stale?"Stale":"Current"],["Observations",String(row.observation_count||0)],["Latest Evaluation",benchmarkDate(row.latest_evaluation),String(row.latest_evaluation||"")],["Last Retrieval",benchmarkDate(row.latest_retrieval),String(row.latest_retrieval||"")],["Age Seconds",typeof row.age_seconds==="number"&&Number.isFinite(row.age_seconds)?String(Math.round(row.age_seconds)):"—",String(row.age_seconds??"")],["Valid Until",benchmarkDate(row.valid_until),String(row.valid_until||"")]];
 const group=(title       ,fields        )       =>`<section class="benchmark-evidence-section"><h2>${title}</h2><dl class="benchmark-evidence-grid">${fields.map(([label,value,exact])=>`<div><dt>${escapeHTML(label)}</dt><dd${exact?` title="${escapeHTML(exact)}"`:""}>${escapeHTML(String(value))}</dd></div>`).join("")}</dl></section>`;
 return `<div class="benchmark-evidence-content">${group("Capability Scores",scoreFields)}${group("Operating Metrics",operationFields)}${group("Provenance & Freshness",provenanceFields)}</div>`;
}

let inspectedBenchmarkModel="";
function openBenchmarkEvidence(id       )     {
 const model=benchmarkModels[id];if(!model)return;inspectedBenchmarkModel=id;
 document.querySelector("#benchmark-evidence-title") .textContent=model.display_name||id;
 document.querySelector("#benchmark-evidence-content") .innerHTML=benchmarkEvidence({...model,id});
 const dialog=document.querySelector                   ("#benchmark-evidence-dialog");dialog?.showModal?.();
 document.querySelector             ("#benchmark-evidence-title")?.focus?.({preventScroll:true});
 if(dialog)dialog.scrollTop=0;
}
function closeBenchmarkEvidence()     {document.querySelector                   ("#benchmark-evidence-dialog")?.close?.();}

const benchmarkSummaryFields                        = [
 {key:"model",label:"Model"},{key:"overall",label:"Overall %"},{key:"capabilities",label:"Capability Scores"},{key:"sources",label:"Sources"},{key:"freshness",label:"Match / Freshness"},{key:"evidence",label:"Evidence"},
 {key:"id",label:"Model ID"},{key:"display_name",label:"Display Name"},{key:"provider_ids",label:"Providers"},{key:"external_ids",label:"External Model IDs"},
 ...benchmarkCapabilities.map(key=>({key:`score:${key}`,label:`${benchmarkLabel(key)} %`})),
 ...Object.keys(benchmarkOperations).map(key=>({key:`operation:${key}`,label:benchmarkLabel(key)})),
 {key:"match",label:"Match"},{key:"stale",label:"Data Status"},{key:"observation_count",label:"Observations"},{key:"latest_evaluation",label:"Latest Evaluation"},{key:"latest_retrieval",label:"Last Retrieval"},{key:"age_seconds",label:"Age Seconds"},{key:"valid_until",label:"Valid Until"}
];
const benchmarkMeasurementFields                        = [
 {key:"external_model_id",label:"Model ID"},{key:"benchmark",label:"Benchmark"},{key:"category",label:"Category"},{key:"original_score",label:"Original Score"},{key:"normalized_score",label:"Normalized Score (0–1)"},{key:"unit",label:"Unit"},{key:"version",label:"Version"},{key:"evaluated_at",label:"Evaluation Time"},
 {key:"source",label:"Source"},{key:"model_id",label:"Matched Model ID"},{key:"retrieved_at",label:"Retrieved At"},{key:"date_basis",label:"Date Basis"}
];
let benchmarkEditing = false;
let benchmarkSummaryEnabled = ["model","overall","capabilities","sources","freshness","evidence"];
let benchmarkMeasurementEnabled = benchmarkMeasurementFields.slice(0,8).map(field=>field.key);
let benchmarkInlineDrafts                                                                                          = {};
let benchmarkInlineSaving = false;
let benchmarkInlineSaveSequence = 0;
function benchmarkActiveFields()                      {return benchmarkEditing?benchmarkMeasurementFields:benchmarkSummaryFields;}
function benchmarkEnabledFields()         {return benchmarkEditing?benchmarkMeasurementEnabled:benchmarkSummaryEnabled;}
function renderBenchmarkFields()     {
 const enabled=benchmarkEnabledFields(),fields=benchmarkActiveFields();
 const option=(key       )       =>`<option value="${escapeHTML(key)}">${escapeHTML(fields.find(field=>field.key===key) .label)}</option>`;
 document.querySelector("#benchmark-fields-available") .innerHTML=fields.filter(field=>!enabled.includes(field.key)).map(field=>option(field.key)).join("");
 document.querySelector("#benchmark-fields-enabled") .innerHTML=enabled.map(option).join("");
 document.querySelector("#benchmark-fields-status") .textContent=`${enabled.length} fields enabled`;
}
function transferBenchmarkFields(add        ,one        )     {
 const select=document.querySelector                   (add?"#benchmark-fields-available":"#benchmark-fields-enabled") ;
 const selected=one?[one]:Array.from(select.selectedOptions).map(option=>option.value);
 let enabled=benchmarkEnabledFields().slice();
 for(const key of selected){if(!benchmarkActiveFields().some(field=>field.key===key))continue;if(add){if(!enabled.includes(key))enabled.push(key);}else enabled=enabled.filter(field=>field!==key);}
 if(!enabled.length){document.querySelector("#benchmark-fields-status") .textContent="Keep at least one field enabled.";return;}
 if(benchmarkEditing)benchmarkMeasurementEnabled=enabled;else benchmarkSummaryEnabled=enabled;
 renderBenchmarkFields();renderBenchmarkTable();
}
function benchmarkSummaryCell(row    ,key       )       {
 const text=(raw    )=>escapeHTML(String(raw??"—"));
 if(key==="model")return text(row.display_name||row.id);
 if(key==="overall")return benchmarkNumber(row.scores?.overall,true);
 if(key.startsWith("score:"))return benchmarkNumber(row.scores?.[key.slice(6)],true);
 if(key.startsWith("operation:"))return benchmarkNumber(row.operations?.[key.slice(10)]);
 if(key==="capabilities"){
  const known=benchmarkCapabilities.filter(name=>typeof row.scores?.[name]==="number"&&Number.isFinite(row.scores[name]));
  const pills=known.slice(0,3).map(name=>`<span class="benchmark-score-pill"><span>${escapeHTML(benchmarkLabel(name))}</span><strong>${benchmarkNumber(row.scores[name],true)}%</strong></span>`).join("");
  return `<div class="benchmark-capability-summary">${pills||'<span class="data-secondary">No comparable scores</span>'}</div>${known.length>3?`<span class="data-secondary">${known.length-3} more in evidence</span>`:""}`;
 }
 if(key==="freshness"||key==="match"){
  const match=["approximate","ambiguous","unmatched"].includes(row.match)?benchmarkLabel(row.match):"Matched";
  return `<span class="status-badge ${match==="Matched"?"success":"neutral"}">${match}</span>${key==="freshness"?`<span class="data-secondary">${row.stale?"Stale":"Current"}${match!=="Matched"?" · display only":""}</span>`:""}`;
 }
 if(key==="stale")return row.stale?"Stale":"Current";
 if(["sources","provider_ids","external_ids"].includes(key))return text((row[key]||[]).join(", ")||"—");
 if(["latest_evaluation","latest_retrieval","valid_until"].includes(key))return `<span title="${text(row[key]||"")}">${benchmarkDate(row[key])}</span>`;
 if(key==="age_seconds")return benchmarkNumber(row[key]);
 if(key==="evidence")return `<button type="button" class="button quiet benchmark-inspect" data-benchmark-inspect="${text(row.id)}" aria-label="Inspect evidence for ${text(row.display_name||row.id)}">Inspect →</button>`;
 return text(row[key]);
}
function benchmarkRecordKey(record                   )       {return `${record.source}:${record.index}`;}
function benchmarkRecordValues(record                   )                      {
 return Object.fromEntries(benchmarkMeasurementFields.slice(0,8).map(({key})=>[key,key==="evaluated_at"&&String(record.observation[key]).startsWith("0001-")?"":String(record.observation[key]??"")]));
}
function renderBenchmarkMeasurements()     {
 const fields=benchmarkEnabledFields().map(key=>benchmarkMeasurementFields.find(field=>field.key===key) );
 document.querySelector("#benchmark-table-head") .innerHTML=`<tr>${fields.map(field=>`<th scope="col">${escapeHTML(field.label)}</th>`).join("")}<th scope="col">Actions</th></tr>`;
 document.querySelector("#benchmark-table-body") .innerHTML=benchmarkRecords.map((record,index)=>{
  const draft=benchmarkInlineDrafts[benchmarkRecordKey(record)];const values=draft?.values||benchmarkRecordValues(record);
  const cells=fields.map(field=>{
   const attrs=`data-benchmark-record="${index}" data-benchmark-cell="${field.key}" aria-label="${escapeHTML(field.label)} for ${escapeHTML(record.observation.external_model_id)}" ${benchmarkInlineSaving?"disabled":""}`;
   let cell       ;
   if(field.key==="category")cell=`<select class="benchmark-inline-input" ${attrs}>${[...benchmarkCapabilities,"operational","unknown"].map(category=>`<option ${values.category===category?"selected":""}>${category}</option>`).join("")}</select>`;
   else if(field.key in values)cell=`<input class="benchmark-inline-input" ${attrs} value="${escapeHTML(values[field.key])}" ${["original_score","normalized_score"].includes(field.key)?`type="number" step="any" ${field.key==="normalized_score"?'min="0" max="1"':""}`:'type="text" maxlength="512"'} ${["external_model_id","benchmark","original_score"].includes(field.key)?"required":""} />`;
   else cell=escapeHTML(String(record.observation[field.key]||record[field.key]||"—"));
   return `<td data-label="${escapeHTML(field.label)}">${cell}</td>`;
  }).join("");
  return `<tr>${cells}<td data-label="Actions"><button class="button quiet" type="button" data-benchmark-save-row="${index}" ${!draft||benchmarkInlineSaving?"disabled":""}>Save</button><button class="button quiet" type="button" data-benchmark-cancel-row="${index}" ${!draft||benchmarkInlineSaving?"disabled":""}>Cancel</button><span class="data-secondary" role="status">${escapeHTML(draft?.error||"")}</span></td></tr>`;
 }).join("")||`<tr><td colspan="${fields.length+1}">No measurements found.</td></tr>`;
 document.querySelector("#benchmark-data-count") .textContent=`${benchmarkRecordTotal} measurements`;
 const pages=Math.max(1,Math.ceil(benchmarkRecordTotal/benchmarkPageSize));
 document.querySelector("#benchmark-page-label") .textContent=`Page ${benchmarkPage+1} of ${pages}`;
 document.querySelector                   ("#benchmark-previous") .disabled=benchmarkPage===0||benchmarkInlineSaving;
 document.querySelector                   ("#benchmark-next") .disabled=(benchmarkPage+1)*benchmarkPageSize>=benchmarkRecordTotal||benchmarkInlineSaving;
}
function updateBenchmarkInlineDraft(event      )     {
 const input=event.target                    ;const index=Number(input.dataset.benchmarkRecord),field=input.dataset.benchmarkCell;
 const record=benchmarkRecords[index];if(!benchmarkEditing||benchmarkInlineSaving||!record||!field)return;
 const key=benchmarkRecordKey(record);
 const draft=benchmarkInlineDrafts[key]||(benchmarkInlineDrafts[key]={record:structuredClone(record),values:benchmarkRecordValues(record),error:""});
 draft.values[field]=input.value;draft.error="";
 const row=input.closest("tr");row?.querySelectorAll                   ("button").forEach(button=>button.disabled=false);
}
async function saveBenchmarkInlineRow(index       )              {
 const record=benchmarkRecords[index];if(!record||benchmarkInlineSaving)return;
 const key=benchmarkRecordKey(record),draft=benchmarkInlineDrafts[key];if(!draft)return;
 const values=draft.values;const observation                   ={};
 try{
  for(const {key:field} of benchmarkMeasurementFields.slice(0,8)){
   const value=values[field].trim();
   if(["external_model_id","benchmark","original_score"].includes(field)&&!value)throw new Error(`${benchmarkLabel(field)} is required.`);
   if(["original_score","normalized_score"].includes(field)){if(value){const n=Number(value);if(!Number.isFinite(n)||(field==="normalized_score"&&(n<0||n>1)))throw new Error(`${benchmarkLabel(field)} must be ${field==="normalized_score"?"between 0 and 1":"finite"}.`);observation[field]=n;}}
   else if(field==="evaluated_at"){if(value){if(!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})$/.test(value)||Number.isNaN(new Date(value).getTime()))throw new Error("Enter a valid RFC3339 evaluation time.");observation[field]=new Date(value).toISOString();}}
   else observation[field]=value;
  }
 }catch(error){draft.error=error instanceof Error?error.message:"Invalid measurement";renderBenchmarkTable();return;}
 const login=consoleSession,sequence=benchmarkRecordSequence,saveSequence=++benchmarkInlineSaveSequence;benchmarkInlineSaving=true;renderBenchmarkTable();
 try{
  const response=await gatewayFetch("/gateway/v1/intelligence/observations/edit",{method:"PUT",headers:{"Content-Type":"application/json",...await authProvider.headers()},body:JSON.stringify({...draft.record,observation})});
  const result=await response.json();if(login!==consoleSession||sequence!==benchmarkRecordSequence)return;
  if(!response.ok)throw new Error(result.error?.message||"Unable to save measurement");
  delete benchmarkInlineDrafts[key];document.querySelector("#benchmark-inline-status") .textContent="Measurement saved.";
  await loadBenchmarkData();
 }catch(error){if(login===consoleSession&&sequence===benchmarkRecordSequence)draft.error=benchmarkSafeMessage(error instanceof Error?error.message:"Unable to save measurement");}
 finally{if(saveSequence===benchmarkInlineSaveSequence){benchmarkInlineSaving=false;renderBenchmarkTable();}}
}

function renderBenchmarkTable()       {
 const table=document.querySelector("#benchmark-table");
 table?.classList.add("benchmark-scorecard");
 table?.classList.toggle("benchmark-custom-fields",benchmarkEditing||benchmarkSummaryEnabled.join(",")!=="model,overall,capabilities,sources,freshness,evidence");
 table?.classList.toggle("benchmark-measurements",benchmarkEditing);
 if(benchmarkEditing){renderBenchmarkMeasurements();return;}
 const fields=benchmarkEnabledFields().map(key=>benchmarkSummaryFields.find(field=>field.key===key) );
 const headers=fields.map(field=>field.label);
 const query=benchmarkFilter.toLowerCase();
 const models=Object.entries(benchmarkModels).sort(([a,left],[b,right])=>String(left.display_name||a).localeCompare(String(right.display_name||b))||a.localeCompare(b))
   .filter(([id,model])=>`${id} ${model.display_name||""} ${(model.external_ids||[]).join(" ")} ${(model.provider_ids||[]).join(" ")} ${(model.sources||[]).join(" ")}`.toLowerCase().includes(query));
 const total=models.length;const focused=document.activeElement?.id;
 benchmarkPage=Math.min(Math.max(0,benchmarkPage),Math.max(0,Math.ceil(total/benchmarkPageSize)-1));
 const rows=models.slice(benchmarkPage*benchmarkPageSize,(benchmarkPage+1)*benchmarkPageSize).map(([id,model])=>({...model,id}));
 const body=document.querySelector("#benchmark-table-body") ;
 document.querySelector("#benchmark-table")?.classList.add("benchmark-scorecard");
 document.querySelector("#benchmark-table-head") .innerHTML=`<tr>${headers.map(header=>`<th scope="col">${escapeHTML(header)}</th>`).join("")}</tr>`;
 body.innerHTML=rows.length?rows.map(row=>`<tr>${fields.map(field=>`<td data-label="${escapeHTML(field.label)}"${field.key==="overall"?' class="benchmark-overall"':""}>${benchmarkSummaryCell(row,field.key)}</td>`).join("")}</tr>`).join(""):`<tr><td colspan="${headers.length}"><div class="empty-state"><strong>No benchmark evidence in this view</strong><p>Enable a source, then refresh to import published results. Try another filter if data is already cached.</p></div></td></tr>`;
 body.querySelectorAll                   ("[data-benchmark-inspect]").forEach(button=>button.addEventListener("click",()=>openBenchmarkEvidence(button.dataset.benchmarkInspect )));
 const pages=Math.max(1,Math.ceil(total/benchmarkPageSize));
 document.querySelector("#benchmark-data-count") .textContent=`${total} models`;
 document.querySelector("#benchmark-page-label") .textContent=`Page ${benchmarkPage+1} of ${pages}`;
 const previous=document.querySelector                   ("#benchmark-previous") ,next=document.querySelector                   ("#benchmark-next") ;
 previous.disabled=benchmarkPage===0;next.disabled=(benchmarkPage+1)*benchmarkPageSize>=total;
 if(focused==="benchmark-next"&&next.disabled)(previous.disabled?document.querySelector                  ("#benchmark-table-filter"):previous)?.focus?.();
 if(focused==="benchmark-previous"&&previous.disabled)(next.disabled?document.querySelector                  ("#benchmark-table-filter"):next)?.focus?.();
}
function benchmarkDate(value    )       {
 if(!value||String(value).startsWith("0001-"))return "Unknown";
 const date=new Date(String(value));if(Number.isNaN(date.getTime()))return "Unknown";
 const display=date.toLocaleDateString("en-US",{timeZone:/^\d{4}-\d{2}-\d{2}$/.test(String(value))?"UTC":consoleTimezone||undefined,month:"short",day:"numeric",year:"numeric"});
 return /^\d{4}-\d{2}-\d{2}$/.test(String(value))?display:`${display} · ${date.toLocaleTimeString("en-US",{timeZone:consoleTimezone||undefined,hour:"2-digit",minute:"2-digit",hour12:false} )} ${escapeHTML(consoleTimezoneLabel())}`;
}
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
    if(benchmarkEditing)await loadBenchmarkRecords();
    renderBenchmarkSourceStatus(); renderBenchmarkTable(); updateBenchmarkControls();
    if (!benchmarkRefreshRunning) document.querySelector("#benchmark-status") .textContent = "Saved evidence · missing scores are unknown.";
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
 document.querySelector                   ("#benchmark-page-size") .value=String(benchmarkPageSize);
 document.querySelector("#close-benchmark-evidence")?.addEventListener("click",closeBenchmarkEvidence);
 const evidence=document.querySelector                   ("#benchmark-evidence-dialog");
 evidence?.addEventListener("close",()=>{const buttons=document.querySelectorAll                   ("[data-benchmark-inspect]");const opener=Array.from(buttons).find(button=>button.dataset.benchmarkInspect===inspectedBenchmarkModel);(opener||document.querySelector                  ("#benchmark-table-filter"))?.focus?.();});
 evidence?.addEventListener("keydown",event=>{if(event.key==="Tab"){event.preventDefault();document.querySelector                   ("#close-benchmark-evidence")?.focus?.();}});
  const list = document.querySelector("#benchmark-sources-list") ;
  list.addEventListener("input", updateBenchmarkSource); list.addEventListener("click", benchmarkSourceAction);
  document.querySelector("#benchmark-page-size") .addEventListener("change",()=>{const size=Number(document.querySelector                   ("#benchmark-page-size") .value);if(![5,10,25,50,100].includes(size))return;benchmarkPageSize=size;benchmarkPage=0;if(benchmarkEditing)void loadBenchmarkRecords();else renderBenchmarkTable();});
  document.querySelector("#benchmark-table-view") .addEventListener("change",()=>{benchmarkTableView=document.querySelector                   ("#benchmark-table-view") .value;benchmarkPage=0;void loadBenchmarkData();});
  document.querySelector("#benchmark-table-filter") .addEventListener("input",()=>{benchmarkFilter=document.querySelector                  ("#benchmark-table-filter") .value;benchmarkPage=0;if(benchmarkEditing)void loadBenchmarkRecords();else renderBenchmarkTable();});
  document.querySelector("#benchmark-previous") .addEventListener("click",()=>{if(benchmarkPage>0){benchmarkPage--;if(benchmarkEditing)void loadBenchmarkRecords();else renderBenchmarkTable();}});
  document.querySelector("#benchmark-next") .addEventListener("click",()=>{benchmarkPage++;if(benchmarkEditing)void loadBenchmarkRecords();else renderBenchmarkTable();});
  renderBenchmarkTable();
}

let benchmarkRecords                     =[];
let benchmarkRecordTotal=0;
let benchmarkRecordSequence=0;
async function loadBenchmarkRecords()              {
 if(!benchmarksEnabledSaved||!consoleSession)return;
 const sequence=++benchmarkRecordSequence,login=consoleSession;
 try{
  const response=await gatewayFetch(`/gateway/v1/intelligence/records?limit=${benchmarkPageSize}&offset=${benchmarkPage*benchmarkPageSize}&q=${encodeURIComponent(benchmarkFilter)}${benchmarkTableView==="routable"?"&configured_only=true":""}`,{headers:await authProvider.headers()});
  const result=await response.json();if(!response.ok)throw new Error(result.error?.message||"Unable to load measurements");
  if(sequence!==benchmarkRecordSequence||login!==consoleSession||!benchmarkEditing)return;
  benchmarkRecords=result.records||[];benchmarkRecordTotal=result.total||0;
  const last=Math.max(0,Math.ceil(benchmarkRecordTotal/benchmarkPageSize)-1);
  if(benchmarkPage>last){benchmarkPage=last;await loadBenchmarkRecords();return;}
  // Own row saves change a source revision. Rebase untouched row drafts only
  // when their original measurement still agrees with the fresh server row.
  for(const record of benchmarkRecords){const draft=benchmarkInlineDrafts[benchmarkRecordKey(record)];if(draft&&JSON.stringify(draft.record.observation)===JSON.stringify(record.observation))draft.record.revision=record.revision;}
  renderBenchmarkTable();
 }catch(error){if(sequence===benchmarkRecordSequence&&login===consoleSession)document.querySelector("#benchmark-inline-status") .textContent=benchmarkSafeMessage(error instanceof Error?error.message:"Unable to load measurements");}
}
function clearBenchmarkRecords()     {
 benchmarkRecords=[];benchmarkInlineDrafts={};benchmarkRecordTotal=0;benchmarkRecordSequence++;
 for(const id of ["benchmark-custom-status","benchmark-inline-status"]){const node=document.querySelector(`#${id}`);if(node)node.textContent="";}
 const file=document.querySelector                  ("#benchmark-csv-file");if(file)file.value="";
}
async function uploadBenchmarkCSV()              {
 const file=document.querySelector                  ("#benchmark-csv-file")?.files?.[0];const status=document.querySelector("#benchmark-custom-status") ;
 if(!file){status.textContent="Choose a CSV file first.";return;}
 if(file.size>8*1024*1024){status.textContent="CSV files must be at most 8 MiB.";return;}
 const login=consoleSession;const button=document.querySelector                   ("#benchmark-csv-upload") ;button.disabled=true;status.textContent="Validating and saving CSV…";
 try{
  const response=await gatewayFetch("/gateway/v1/intelligence/import",{method:"POST",headers:{"Content-Type":"text/csv",...await authProvider.headers()},body:file});
  const result=await response.json();if(login!==consoleSession)return;if(!response.ok)throw new Error(result.error?.message||"CSV upload failed");
  status.textContent="Custom benchmarks saved.";benchmarkPage=0;await loadBenchmarkData();
 }catch(error){status.textContent=benchmarkSafeMessage(error instanceof Error?error.message:"CSV upload failed");}finally{button.disabled=false;}
}
document.querySelector("#benchmark-csv-upload")?.addEventListener("click",()=>void uploadBenchmarkCSV());
for(const [button,id] of [["benchmark-configure","benchmark-configuration"],["benchmark-upload","benchmark-upload-dialog"],["benchmark-display-fields","benchmark-fields-dialog"]]){
 document.querySelector(`#${button}`)?.addEventListener("click",()=>{if(id==="benchmark-fields-dialog")renderBenchmarkFields();document.querySelector                   (`#${id}`)?.showModal?.();});
 document.querySelector(`#${id}-close`)?.addEventListener("click",()=>document.querySelector                   (`#${id}`)?.close?.());
}
document.querySelector("#benchmark-sources-list")?.addEventListener("change",()=>void saveConfig());
document.querySelector("#benchmark-field-add")?.addEventListener("click",()=>transferBenchmarkFields(true));
document.querySelector("#benchmark-field-remove")?.addEventListener("click",()=>transferBenchmarkFields(false));
for(const [id,add] of [["benchmark-fields-available",true],["benchmark-fields-enabled",false]]         ){
 document.querySelector(`#${id}`)?.addEventListener("dblclick",event=>{const option=event.target                     ;if(option.tagName==="OPTION")transferBenchmarkFields(add,option.value);});
 document.querySelector(`#${id}`)?.addEventListener("keydown",event=>{if((event                 ).key==="Enter"){event.preventDefault();transferBenchmarkFields(add);}});
}
document.querySelector("#benchmark-edit-data")?.addEventListener("click",async()=>{
 if(benchmarkInlineSaving)return;
 if(benchmarkEditing&&Object.keys(benchmarkInlineDrafts).length&&!confirm("Discard unsaved measurement edits?"))return;
 benchmarkEditing=!benchmarkEditing;benchmarkInlineDrafts={};benchmarkPage=0;
 document.querySelector("#benchmark-edit-data") .innerHTML=consoleEditButtonMarkup(benchmarkEditing?"Done Editing":"Edit Data");
 document.querySelector("#benchmark-edit-data") .setAttribute("aria-pressed",String(benchmarkEditing));
 document.querySelector("#benchmark-edit-data") .setAttribute("aria-label",benchmarkEditing?"Done Editing":"Edit Data");
 document.querySelector("#benchmark-edit-data") .setAttribute("title",benchmarkEditing?"Done Editing":"Edit Data");
 document.querySelector("#benchmark-inline-status") .textContent=benchmarkEditing?"Edit measurement fields, then Save each row. A source refresh can replace downloaded measurements; custom uploads are retained.":"";
 renderBenchmarkFields();if(benchmarkEditing)await loadBenchmarkRecords();renderBenchmarkTable();
});
document.querySelector("#benchmark-table-body")?.addEventListener("input",updateBenchmarkInlineDraft);
document.querySelector("#benchmark-table-body")?.addEventListener("change",updateBenchmarkInlineDraft);
document.querySelector("#benchmark-table-body")?.addEventListener("click",event=>{
 const button=(event.target               ).closest                   ("[data-benchmark-save-row],[data-benchmark-cancel-row]");if(!button||benchmarkInlineSaving)return;
 if(button.dataset.benchmarkSaveRow!==undefined)void saveBenchmarkInlineRow(Number(button.dataset.benchmarkSaveRow));
 else{const record=benchmarkRecords[Number(button.dataset.benchmarkCancelRow)];if(record){delete benchmarkInlineDrafts[benchmarkRecordKey(record)];renderBenchmarkTable();}}
});

document.querySelector("#benchmark-table-body")?.addEventListener("keydown",event=>{
 const input=event.target                    ;
 if((event                 ).key==="Enter"&&input.dataset.benchmarkCell){event.preventDefault();void saveBenchmarkInlineRow(Number(input.dataset.benchmarkRecord));}
});

const modelLoadingMarkup='<div class="empty-state" role="status"><strong>Loading model catalog…</strong><p>Discovering models from configured providers.</p></div>';
let modelEnabledDraft                       ={};
let modelPricingDraft                   ={};
let editingModelPricingID="";
let physicalModels      =[];
let modelSearch="";
let modelProviderFilter="";
let modelStatusFilter="";
let modelSort="name";
let modelPage=0;
let modelPageSize=25;
function modelQuotaSummary(model    )       {const q=model.quota;return q?.known?`${Number(q.fraction_remaining*100).toFixed(1)}% quota remaining${q.windows?.length?` · ${q.windows.map((w    )=>`${w.name}: ${quotaResetLabel(w.reset_at)}`).join(" · ")}`:""}`:model.quota_error||"Provider quota unknown";}
function modelPrice(raw       )       {return raw>0&&raw<.000001?`$${raw.toPrecision(3)}`:`$${raw.toLocaleString(undefined,{minimumFractionDigits:2,maximumFractionDigits:6})}`;}
function modelPricingText(model    )       {
 const p=model.pricing||{};
 if(model.subscription&&p.source!=="operator")return "Subscription included";
 return `${typeof p.input_per_million==="number"?modelPrice(p.input_per_million):"Unknown"} in / ${typeof p.output_per_million==="number"?modelPrice(p.output_per_million):"Unknown"} out per 1M${String(p.source||"").startsWith("models.dev")?" (estimate)":""}`;
}
function modelPricingStatus(model    ,now=Date.now())                            {
 const p=model.pricing||{};const hasPrice=typeof p.input_per_million==="number"||typeof p.output_per_million==="number";
 if(p.source==="operator"||p.source==="config"||modelPricingDraft[model.id])return {color:"yellow",label:"Manually entered pricing"};
 if(!hasPrice)return {color:"gray",label:"Pricing has never been pulled or set"};
 const updated=Date.parse(p.updated_at||"");const age=now-updated;
 const detail=Number.isFinite(updated)?` · Last price update: ${consoleDateTime(p.updated_at)}`:"";
 if(p.refresh_failed)return {color:"yellow",label:`Pricing refresh failed; using last known prices${detail}`};
 if(age>7*24*60*60*1000)return {color:"yellow",label:`Pricing is more than one week old${detail}`};
 if(age>24*60*60*1000)return {color:"yellow",label:`Pricing is more than 24 hours old${detail}`};
 if(String(p.source||"").startsWith("models.dev"))return {color:"yellow",label:`Bundled pricing estimate${detail}`};
 if(!Number.isFinite(updated))return {color:"yellow",label:"Pricing available; last price update unknown"};
 return {color:"green",label:`Pricing successfully pulled from provider${detail}`};
}
function modelPricingIndicator(model    )       {
 const status=modelPricingStatus(model);const id=`model-price-status-${physicalModels.indexOf(model)}`;
 return `<span class="pricing-status"><button type="button" class="pricing-status-dot ${status.color}" aria-label="${escapeHTML(status.label)}" aria-describedby="${id}"></button><span class="pricing-status-tooltip" role="tooltip" id="${id}">${escapeHTML(status.label)}</span></span>`;
}
function modelPageNumbers(total       )         {
 if(total<=5)return Array.from({length:total},(_,index)=>index);
 return [...new Set([0,Math.max(0,modelPage-1),modelPage,Math.min(total-1,modelPage+1),total-1])].sort((a,b)=>a-b);
}
function renderModelPagination(total       )     {
 const pages=Math.max(1,modelPageSize?Math.ceil(total/modelPageSize):1);
 for(const position of ["top","bottom"]){
  const suffix=position==="top"?"-top":"";
  const container=document.querySelector(`#models-pagination-${position}`);if(!container)continue;
  const numbers=modelPageNumbers(pages);
  container.innerHTML=`<label for="model-page-size${suffix}" class="inline-filter">Rows<select id="model-page-size${suffix}" aria-label="Models per page (${position})">${[25,50,100,0].map(size=>`<option value="${size}" ${size===modelPageSize?"selected":""}>${size||"All"}</option>`).join("")}</select></label><nav aria-label="Model pages (${position})"><button id="model-previous${suffix}" class="button quiet" type="button" ${modelPage===0?"disabled":""}>Previous</button>${numbers.map((page,index)=>`${index&&page-numbers[index-1]>1?'<span class="pagination-ellipsis" aria-hidden="true">…</span>':""}<button type="button" class="button quiet" data-model-page="${page}" ${page===modelPage?'aria-current="page"':""} aria-label="Page ${page+1}">${pages>5&&page===0?"First":pages>5&&page===pages-1?"Last":page+1}</button>`).join("")}<button id="model-next${suffix}" class="button quiet" type="button" ${modelPage===pages-1?"disabled":""}>Next</button></nav>`;
  const size=document.querySelector                   (`#model-page-size${suffix}`) ;size.value=String(modelPageSize);size.addEventListener("change",()=>{modelPageSize=[25,50,100,0].includes(Number(size.value))?Number(size.value):25;modelPage=0;renderModelRows();});
  const previous=document.querySelector                   (`#model-previous${suffix}`) ;previous.disabled=modelPage===0;previous.addEventListener("click",()=>{modelPage--;renderModelRows();});
  const next=document.querySelector                   (`#model-next${suffix}`) ;next.disabled=modelPage===pages-1;next.addEventListener("click",()=>{modelPage++;renderModelRows();});
  container.querySelectorAll                   ("[data-model-page]").forEach(button=>button.addEventListener("click",()=>{modelPage=Number(button.dataset.modelPage);renderModelRows();}));
 }
}
function quotaWindowText(window    )       {
 const remaining=Number(window.remaining);
 const value=window.unit==="USD"?modelPrice(remaining):window.unit==="percent"?`${remaining.toFixed(1)}%`:`${consoleNumber(remaining)} ${window.unit||"units"}`;
 const reset=window.reset_at?new Date(window.reset_at):null;
 return `${window.name?`${window.name.replace(/_/g," ")}: `:""}${value} remaining${window.informational?" (informational)":""}${reset&&!Number.isNaN(reset.getTime())?` · resets ${consoleDateTime(window.reset_at)}`:" · reset unknown"}`;
}
function balanceText(q    )        {
 return (q?.balances||[]).map((b    )=>`${b.name.replace(/_/g," ")}: ${b.unlimited?"Unlimited":typeof b.remaining==="number"?(b.unit==="USD"?modelPrice(b.remaining):`${consoleNumber(b.remaining)} ${b.unit}`):"Unknown"}${b.reset_at?` · renews ${consoleDateTime(b.reset_at)}`:""}`).join("; ");
}
function trackingReasonText(reason       )       {return ({management_credentials_required:"Billing management credentials required",admin_credentials_required:"Billing admin credential required",oauth_credentials_required:"Subscription OAuth credential required",wallet_balance_not_exposed:"Provider does not expose wallet balance",response_headers_only:"Quota available from response headers",credential_rejected:"Tracking credential rejected",endpoint_unavailable:"Tracking endpoint unavailable",rate_limited:"Tracking rate limited",tracking_unavailable:"Tracking unavailable",no_reported_data:"No reported provider data",spend_unavailable:"Provider spend unavailable",report_incomplete:"Provider report incomplete",no_subscription:"No subscription reported",oauth_login_or_refresh_required:"OAuth login expired or refresh unavailable; reconnect the account",oauth_required_api_key_supplied:"ChatGPT OAuth required; an API key was supplied",account_context_mismatch:"Quota account does not match the configured OAuth account",usage_auth_rejected_after_refresh_check_account_and_oauth_context:"Usage endpoint rejected OAuth authentication; verify account and token permissions",usage_auth_rejected_refresh_unavailable:"Usage authentication rejected and refresh unavailable; reconnect OAuth",usage_permission_denied:"OAuth account lacks usage permission",oauth_refresh_failed:"OAuth refresh failed; reconnect the account",usage_request_failed:"Quota request failed; cached values may be stale",usage_endpoint_failed:"Usage endpoint failed",invalid_usage_response:"Usage response could not be parsed",account_changed_during_collection:"Account changed during quota collection"}                         )[reason]||"Provider data unavailable";}
function codexWindowText(w    )        {
 const name=w.window_type==="5h"?"5-hour":w.window_type==="weekly"?"Weekly":w.window_seconds?`${w.window_seconds/60} minute window`:"Unknown duration";
 if(w.status==="unavailable")return `${name}: usage unavailable · ${quotaResetLabel(w.reset_at)}`;
 return `${name}: ${Number(w.used).toFixed(1)}% used · ${Number(w.remaining).toFixed(1)}% remaining · ${quotaResetLabel(w.reset_at)}`;
}
function codexGroupLabel(g    )       {return g.limit_name||(g.map_key&&g.limit_id&&g.map_key!==g.limit_id?`${g.limit_id} (${g.map_key})`:g.limit_id||g.map_key)||"Unidentified quota pool";}
function codexGroupText(g    )        {
 const windows=g.windows||[];
 return [codexGroupLabel(g),...(g.status==="exhausted"?["Exhausted"]:[]),...windows.map(codexWindowText),...['5h','weekly'].filter(type=>!windows.some((w    )=>w.window_type===type)).map(type=>`${type==="5h"?"5-hour":"Weekly"}: unavailable`)].join(" · ");
}
function codexQuotaMarkup(q    )        {
 if(q.provider!=="openai-codex")return "";
 return `<p>Account: ${escapeHTML(q.account_identity||q.account_id||"Unknown identity")} · ${escapeHTML(q.plan||"Unknown plan")}<br>Status: ${escapeHTML(q.status||"unavailable")}${q.collection_status&&q.collection_status!==q.status?` · Collection: ${escapeHTML(q.collection_status.replace(/_/g," "))}`:""}${q.unavailable_reason?` · ${escapeHTML(trackingReasonText(q.unavailable_reason))}`:""}<br>Last successful update: ${q.collected_at&&q.collected_at!=="0001-01-01T00:00:00Z"?escapeHTML(consoleDateTime(q.collected_at)):"Unavailable"} · ${escapeHTML(q.source||"Unknown source")}</p>${(q.limits||[]).map((g    )=>`<details class="quota-group"><summary>${escapeHTML(codexGroupLabel(g))} · ${escapeHTML(g.status||"unavailable")}</summary><p>${escapeHTML(codexGroupText(g))}</p><small>Scope: ${escapeHTML(g.scope||"unknown")}${g.limit_id?` · ID: ${escapeHTML(g.limit_id)}`:""}</small></details>`).join("")}`;
}
function modelQuotaText(model    )       {
 const q=model.quota;
 const extras=[balanceText(q),typeof q?.reset_credits==="number"?`${q.reset_credits} banked resets`:""].filter(Boolean).join(" · ");
 if(q?.provider==="openai-codex") {
  const groups=q.limits||[];
  return `${q.status==="stale"?"Stale · ":""}${groups.length?groups.map(codexGroupText).join("; "):"5-hour: unavailable · Weekly: unavailable"}${extras?` · ${extras}`:""}${q.unavailable_reason?` · ${trackingReasonText(q.unavailable_reason)}`:""}`;
 }

 if(extras&&!q?.known&&!q?.windows?.length&&!q?.reset_at)return `${q.blocked?"Quota exhausted · ":""}${extras}${q?.unavailable_reason?` · ${trackingReasonText(q.unavailable_reason)}`:""}`;
 if(!q?.known&&q?.windows?.length)return q.windows.map(quotaWindowText).join("; ")+`${extras?` · ${extras}`:""}`;
 if(!q?.known&&q?.reset_at)return `Quota unknown · resets ${consoleDateTime(q.reset_at)}${extras?` · ${extras}`:""}`;
 if(!q?.known&&q?.unavailable_reason)return trackingReasonText(q.unavailable_reason);
 if(!q?.known)return q?.blocked?"Quota exhausted":q?.source==="unavailable"?"Quota unavailable":"Quota unknown";
 const windows=q.windows?.length?q.windows:[q];
 return `${q.exhausted?"Exhausted · ":""}${windows.map(quotaWindowText).join("; ")}${extras?` · ${extras}`:""}`;
}
function modelUsageText(model    )       {
 if(!model.usage_available)return "Usage unavailable";
 const row=(model.usage||[]).find((r    )=>/^\d{4}-\d{2}$/.test(r.period));
 if(!row)return "No gateway usage this month";
 return `${consoleNumber(Number(row.input_tokens||0)+Number(row.output_tokens||0))} tokens · ${modelPrice(Number(row.estimated_cost||0))} est. this month${row.unknown_cost_attempts?" + unknown costs":""}`;
}
function modelProviderUsageText(model    )       {
 const q=model.quota||{};const monthly=(q.reported_usage||[]).filter((u    )=>u.period==="monthly");
 const parts=monthly.map((u    )=>`${u.unit==="USD"?modelPrice(u.used):`${consoleNumber(u.used)} ${u.unit}`} (${u.scope||"account"})`);
 if(typeof q.usage_usd?.monthly==="number"&&!monthly.some((u    )=>u.unit==="USD"))parts.unshift(`${modelPrice(q.usage_usd.monthly)} (account)`);
 return parts.length?`Provider this month: ${parts.join("; ")}`:"";
}
function modelSelectorLabel(model    )       {return [model.id,modelPricingText(model),modelQuotaText(model),modelProviderUsageText(model),modelUsageText(model)].filter(Boolean).join(" · ");}
function modelTrackingMarkup(model    )       {
 const q=model.quota||{};const reported=q.usage_usd;
 const usage=(q.reported_usage||[]).map((u    )=>`${u.period}: ${u.unit==="USD"?modelPrice(u.used):`${consoleNumber(u.used)} ${u.unit}`} (${u.scope||"account"})`).join(" · ");
 return `${codexQuotaMarkup(q)}<p><strong>Provider quota</strong><br>${escapeHTML(modelQuotaText(model))}</p>${model.pricing?.source?`<p>Price source: ${escapeHTML(model.pricing.source)}</p>`:""}${q.plan?`<p>Plan: ${escapeHTML(q.plan)}</p>`:""}${q.partial?`<p>${escapeHTML(trackingReasonText(q.unavailable_reason||"tracking_unavailable"))} · Partial provider data</p>`:""}${usage?`<p><strong>Provider-reported usage</strong><br>${escapeHTML(usage)}</p>`:""}${q.checked_at&&q.checked_at!=="0001-01-01T00:00:00Z"?`<p>Checked ${escapeHTML(consoleDateTime(q.checked_at))} · ${escapeHTML(q.source||"unknown source")}</p>`:""}${reported?`<p><strong>Provider-reported spend</strong><br>${["total","daily","weekly","monthly"].filter(period=>typeof reported[period]==="number").map(period=>`${period}: ${modelPrice(reported[period])}`).map(escapeHTML).join(" · ")}</p>`:""}${model.usage_available!==undefined?`<p><strong>Gateway usage</strong><br>${escapeHTML(modelUsageText(model))}</p>`:""}`;
}
function filteredPhysicalModels()      {const query=modelSearch.toLowerCase();return physicalModels.filter(model=>(!query||`${model.id} ${model.provider_id}`.toLowerCase().includes(query))&&(!modelProviderFilter||model.provider_id===modelProviderFilter)&&(!modelStatusFilter||Boolean(model.enabled)===(modelStatusFilter==="enabled"))).sort((a,b)=>modelSort==="provider"?String(a.provider_id).localeCompare(String(b.provider_id))||String(a.id).localeCompare(String(b.id)):modelSort==="enabled"?Number(Boolean(b.enabled))-Number(Boolean(a.enabled))||String(a.id).localeCompare(String(b.id)):String(a.id).localeCompare(String(b.id)));}
function renderModelRows()     {
 const rows=document.querySelector("#model-table-body");if(!rows)return;const active=document.activeElement                    ;const focused=active?.id;const focusedPage=active?.dataset?.modelPage;const focusedPagination=active?.closest?.(".models-pagination")?.id;const filtered=filteredPhysicalModels();const pageSize=modelPageSize||Math.max(1,filtered.length);modelPage=Math.min(Math.max(0,modelPage),Math.max(0,Math.ceil(filtered.length/pageSize)-1));const models=filtered.slice(modelPage*pageSize,(modelPage+1)*pageSize);
 rows.innerHTML=models.length?models.map((model    )=>{
  const pricing=model.pricing||{};const supported=Object.entries(model.capabilities||{}).filter(([,v])=>v===true||v==="supported").map(([name])=>name.replace(/_/g," "));
  const capabilities=supported.map(name=>`<span class="capability-pill">${escapeHTML(name)}</span>`).join("")||'<span class="data-secondary">Unknown</span>';
  const scores=model.benchmark?Object.entries(model.benchmark.scores||{}).map(([name,score])=>`${name.replace(/_/g," ")} ${(Number(score)*100).toFixed(1)}%`).join(" · "):"";
  const details=`<details class="model-details"><summary>Inspect Metadata</summary><div class="model-detail-content"><p><strong>Capabilities</strong><br>${Object.entries(model.capabilities||{}).map(([name,v])=>`${escapeHTML(name.replace(/_/g," "))}: ${escapeHTML(String(v))}`).join(" · ")||"Unknown"}</p><p><strong>Context window</strong> ${consoleNumber(model.context_window)} tokens</p>${modelTrackingMarkup(model)}${model.benchmark?`<p><strong>Benchmark scores</strong><br>${escapeHTML(scores||"No scores")}</p><p><strong>Sources</strong> ${(model.benchmark.sources||[]).map(escapeHTML).join(", ")||"Unknown"}<br>Retrieved ${escapeHTML(model.benchmark.latest_retrieval||"unknown")}<br>Evaluated ${escapeHTML(model.benchmark.latest_evaluation||"unknown")}</p>`:""}</div></details>`;
  return `<tr><td data-label="Model">${modelPricingIndicator(model)}<strong class="model-name">${escapeHTML(model.id)}</strong><span class="data-secondary">${escapeHTML(model.provider_id)}</span></td><td data-label="Capabilities"><div class="capability-list">${capabilities}</div></td><td class="numeric" data-label="Input / 1M Tokens">${model.subscription?'<span class="data-secondary">Subscription included</span>':""}${typeof pricing.input_per_million==="number"?modelPrice(pricing.input_per_million):"—"}<span class="data-secondary">${typeof pricing.output_per_million==="number"?`${modelPrice(pricing.output_per_million)} output`:"Output unknown"}</span>${pricing.source==="operator"?'<span class="data-secondary">Manual pricing</span>':""}<button type="button" class="action-icon-only button quiet model-price-edit" data-model-price="${escapeHTML(model.id)}" aria-label="Edit Pricing" title="Edit Pricing"><svg class="action-icon" viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path d="m16 3 5 5-12 12H4v-5L16 3ZM13 6l5 5"/></svg></button></td><td data-label="Benchmark Evidence"><span class="data-secondary">${escapeHTML(model.benchmark?scores.split(" · ").slice(0,2).join(" · ")||"No scores":"No matched benchmark data")}</span>${model.benchmark?`<span class="status-badge ${model.benchmark.stale?"neutral":"success"}">${model.benchmark.stale?"Stale":"Current"}</span>`:""}${details}</td><td data-label="Access"><label class="checkbox model-enable"><input type="checkbox" data-model-enabled="${escapeHTML(model.id)}" aria-label="Enable ${escapeHTML(model.id)}" ${model.enabled?"checked":""}><span>${model.enabled?"Enabled":"Disabled"}</span></label></td><td class="model-tracking" data-label="Usage &amp; Provider Quota"><span class="data-secondary">${escapeHTML(modelQuotaText(model))}</span><span class="data-secondary">${escapeHTML(modelUsageText(model))}</span></td></tr>`;
 }).join(""):`<tr><td colspan="6"><div class="empty-state"><strong>No models match these filters</strong><p>Try another model name or clear your filters.</p></div></td></tr>`;
 document.querySelector("#model-count") .textContent=`${filtered.length?modelPage*pageSize+1:0}–${Math.min((modelPage+1)*pageSize,filtered.length)} of ${filtered.length} models${filtered.length!==physicalModels.length?` · ${physicalModels.length} total`:""}`;
 renderModelPagination(filtered.length);
 if(focused?.startsWith("model-previous")||focused?.startsWith("model-next")){const replacement=document.querySelector                   (`#${focused}`);if(replacement&&!replacement.disabled)replacement.focus?.();}
 if(focusedPage!==undefined&&focusedPagination)document.querySelector                   (`#${focusedPagination} [data-model-page="${focusedPage}"]`)?.focus?.();
 for(const suffix of ["","-top"]){
  if(focused===`model-next${suffix}`&&document.querySelector                   (`#model-next${suffix}`) .disabled)(document.querySelector                   (`#model-previous${suffix}`) .disabled?document.querySelector                  ("#model-search"):document.querySelector                   (`#model-previous${suffix}`))?.focus?.();
  if(focused===`model-previous${suffix}`&&document.querySelector                   (`#model-previous${suffix}`) .disabled)(document.querySelector                   (`#model-next${suffix}`) .disabled?document.querySelector                  ("#model-search"):document.querySelector                   (`#model-next${suffix}`))?.focus?.();
 }
 rows.querySelectorAll                   (".pricing-status-dot").forEach(button=>{
  const show=()=>{const tooltip=button.nextElementSibling               ;const rect=button.getBoundingClientRect();tooltip.style.left=`${Math.max(8,Math.min(rect.left,window.innerWidth-256))}px`;tooltip.style.top=`${Math.max(8,Math.min(rect.bottom+6,window.innerHeight-tooltip.offsetHeight-8))}px`;};
  button.addEventListener("mouseenter",show);button.addEventListener("focus",show);button.addEventListener("click",show);button.addEventListener("keydown",event=>{if(event.key==="Escape")button.blur();});
 });
 rows.querySelectorAll                   ("[data-model-price]").forEach(button=>button.addEventListener("click",()=>openModelPricing(button.dataset.modelPrice )));
 rows.querySelectorAll                  ("[data-model-enabled]").forEach(input=>input.addEventListener("change",async()=>{modelEnabledDraft[input.dataset.modelEnabled ]=input.checked;markDirty();if(await saveConfig())void loadModelsPage();}));
}
function renderModelsPage()     {
 const output=document.querySelector             ("#models-list") ;
 if(!physicalModels.length){output.innerHTML='<div class="empty-state"><span class="empty-state-icon">◇</span><strong>Your model catalog is empty</strong><p>Connect and test a provider to discover the models available to your gateway.</p><a href="#providers" class="button primary">Connect A Provider</a></div>';return;}
 const providerIDs=[...new Set(physicalModels.map(model=>String(model.provider_id)))].sort();
 output.innerHTML=`<div class="models-toolbar"><label class="search-field" for="model-search">Search Models<input aria-label="Search models" id="model-search" type="search" placeholder="Search by model or provider…" maxlength="256" value="${escapeHTML(modelSearch)}"></label><label for="model-provider-filter">Provider<select id="model-provider-filter" aria-label="Provider"><option value="">All Providers</option>${providerIDs.map(id=>`<option value="${escapeHTML(id)}">${escapeHTML(id)}</option>`).join("")}</select></label><label for="model-status-filter">Status<select id="model-status-filter" aria-label="Status"><option value="">All Models</option><option value="enabled">Enabled</option><option value="disabled">Disabled</option></select></label><label for="model-sort">Sort<select id="model-sort" aria-label="Sort models"><option value="name">Model Name</option><option value="provider">Provider</option><option value="enabled">Enabled First</option></select></label><span id="model-count" class="section-note" role="status"></span></div><div id="models-pagination-top" class="benchmark-pagination models-pagination"></div><div class="benchmark-table-scroll"><table class="model-table"><thead><tr><th scope="col">Model / Provider</th><th scope="col">Capabilities</th><th scope="col">Input / 1M Tokens</th><th scope="col">Benchmark Evidence</th><th scope="col">Access</th><th scope="col">Usage &amp; Provider Quota</th></tr></thead><tbody id="model-table-body"></tbody></table></div><div id="models-pagination-bottom" class="benchmark-pagination models-pagination"></div><p class="section-note">Prices are USD per million tokens. Manual rates apply to routing and estimated spend. Providers configured to treat models as free retain zero effective cost. Gateway spend is estimated. Provider quota snapshots share account limits across models where applicable. — means unknown. Disabled models are unavailable to client discovery and routing.</p>`;
 document.querySelector                  ("#model-search") .addEventListener("input",event=>{modelSearch=(event.target                    ).value;modelPage=0;renderModelRows();});
 const provider=document.querySelector                   ("#model-provider-filter") ;provider.value=modelProviderFilter;provider.addEventListener("change",()=>{modelProviderFilter=provider.value;modelPage=0;renderModelRows();});
 const status=document.querySelector                   ("#model-status-filter") ;status.value=modelStatusFilter;status.addEventListener("change",()=>{modelStatusFilter=status.value;modelPage=0;renderModelRows();});const sort=document.querySelector                   ("#model-sort") ;sort.value=modelSort;sort.addEventListener("change",()=>{modelSort=sort.value;modelPage=0;renderModelRows();});
 renderModelRows();
}
async function loadModelsPage()              {
 const output=document.querySelector             ("#models-list") ;const login=consoleSession;const button=document.querySelector                   ("#refresh-models") ;button.disabled=true;if(!physicalModels.length)output.innerHTML=modelLoadingMarkup;
 try{const response=await gatewayFetch("/gateway/v1/models",{headers:await authProvider.headers()});if(login!==consoleSession)return;if(!response.ok)throw new Error("Model discovery unavailable. Refresh to retry.");const data=await response.json();if(login!==consoleSession)return;physicalModels=data.models||[];overviewModels=physicalModels;renderModelsPage();renderOverview();
 }catch(error){output.innerHTML=`<div class="empty-state error-state" role="alert"><strong>Unable to load models</strong><p>${escapeHTML(error instanceof Error?error.message:"Models unavailable")}</p></div>`;}finally{button.disabled=false;}
}
document.querySelector("#refresh-models") .addEventListener("click",()=>{void loadModelsPage();});

function clearModelPricing()     {
 modelPricingDraft={};editingModelPricingID="";
 document.querySelector                   ("#model-pricing-dialog")?.close?.();
 document.querySelector                   ("#model-pricing-import-dialog")?.close?.();
 document.querySelector                 ("#model-pricing-form")?.reset?.();
 for(const id of ["model-pricing-status","model-pricing-current","model-pricing-import-status"]){const node=document.querySelector(`#${id}`);if(node)node.textContent="";}
 const file=document.querySelector                  ("#model-pricing-csv");if(file)file.value="";
}
function openModelPricing(id       )     {
 editingModelPricingID=id;const manual=modelPricingDraft[id]||{};const model=physicalModels.find(model=>model.id===id);
 document.querySelector("#model-pricing-model") .textContent=id;
 document.querySelector("#model-pricing-current") .textContent=`Current effective rates: ${model?modelPricingText(model):"Unknown"}. Provider free-model policy takes precedence over entered rates.`;
 for(const [id,key] of [["model-pricing-input","input_per_million"],["model-pricing-output","output_per_million"]]){const input=document.querySelector                  (`#${id}`) ;input.value=manual[key]===undefined?"":String(manual[key]);input.placeholder=typeof model?.pricing?.[key]==="number"?String(model.pricing[key]):"Unknown automatic rate";}
 document.querySelector("#model-pricing-status") .textContent="";
 document.querySelector                   ("#model-pricing-dialog")?.showModal?.();
}
async function saveModelPricing(event       )              {
 event?.preventDefault();if(!editingModelPricingID)return;
 const form=document.querySelector                 ("#model-pricing-form") ;if(!form.reportValidity())return;
 const pricing                      ={};const status=document.querySelector("#model-pricing-status") ;
 for(const [id,key] of [["model-pricing-input","input_per_million"],["model-pricing-output","output_per_million"]]){const raw=document.querySelector                  (`#${id}`) .value.trim();if(raw==="")continue;const value=Number(raw);if(!Number.isFinite(value)||value<0){status.textContent="Rates must be finite, nonnegative USD amounts per million tokens.";return;}pricing[key]=value;}
 if(Object.keys(pricing).length)modelPricingDraft[editingModelPricingID]=pricing;else delete modelPricingDraft[editingModelPricingID];
 const login=consoleSession;const button=document.querySelector                   ("#model-pricing-save") ;button.disabled=true;markDirty();status.textContent="Saving pricing…";
 try{if(await saveConfig()){if(login!==consoleSession)return;document.querySelector                   ("#model-pricing-dialog")?.close?.();editingModelPricingID="";await loadModelsPage();}else if(login===consoleSession)status.textContent=saveState.textContent||"Pricing draft retained. Wait for any active save, then retry.";}finally{button.disabled=false;}
}
async function uploadModelPricingCSV()              {
 const status=document.querySelector("#model-pricing-import-status") ;const file=document.querySelector                  ("#model-pricing-csv")?.files?.[0];
 if(!file){status.textContent="Choose a pricing CSV first.";return;}if(file.size>1024*1024){status.textContent="Pricing CSV files must be at most 1 MiB.";return;}
 const login=consoleSession;const button=document.querySelector                   ("#model-pricing-upload") ;button.disabled=true;status.textContent="Validating pricing CSV…";
 try{
  const response=await gatewayFetch("/gateway/v1/models/pricing/parse",{method:"POST",headers:{"Content-Type":"text/csv",...await authProvider.headers()},body:file});const result=await response.json();if(login!==consoleSession)return;if(!response.ok)throw new Error(result.error||"Unable to validate pricing CSV");
  for(const [id,pricing] of Object.entries(result.model_pricing||{})                  ){if(pricing.input_per_million===undefined&&pricing.output_per_million===undefined)delete modelPricingDraft[id];else modelPricingDraft[id]=pricing;}
  markDirty();if(await saveConfig()){if(login!==consoleSession)return;status.textContent="Model pricing saved.";await loadModelsPage();}else status.textContent=saveState.textContent||"Pricing draft retained; retry saving.";
 }catch(error){if(login===consoleSession)status.textContent=error instanceof Error?error.message:"Unable to import pricing";}finally{button.disabled=false;}
}
document.querySelector("#model-pricing-form")?.addEventListener("submit",event=>void saveModelPricing(event));
document.querySelector("#model-pricing-cancel")?.addEventListener("click",()=>{document.querySelector                   ("#model-pricing-dialog")?.close?.();editingModelPricingID="";});
document.querySelector("#model-pricing-clear")?.addEventListener("click",()=>{document.querySelector                  ("#model-pricing-input") .value="";document.querySelector                  ("#model-pricing-output") .value="";void saveModelPricing();});
document.querySelector("#model-pricing-upload")?.addEventListener("click",()=>void uploadModelPricingCSV());

document.querySelector("#refresh-quotas")?.addEventListener("click",async()=>{
 const login=consoleSession;
 const status=document.querySelector             ("#quota-refresh-status");if(status)status.textContent="Refreshing quotas…";
 const button=document.querySelector                   ("#refresh-quotas") ;button.disabled=true;
 try {
  const response=await gatewayFetch("/gateway/v1/quotas/refresh",{method:"POST",headers:await authProvider.headers()});
  if(login!==consoleSession)return;
  if(response.ok) { const data=await response.json();if(login!==consoleSession)return;if(status)status.textContent="Quota check completed";physicalModels=data.models||[];overviewModels=physicalModels;renderModelsPage();renderOverview(); }
  else if(status)status.textContent=response.status===429?"Quota refresh is cooling down. Try again after the next poll.":"Quota refresh unavailable.";
 }catch {if(login===consoleSession&&status)status.textContent="Quota refresh unavailable.";}finally {button.disabled=false;}
});

document.querySelector("#import-pricing")?.addEventListener("click",()=>document.querySelector                   ("#model-pricing-import-dialog")?.showModal?.());
document.querySelector("#model-pricing-import-close")?.addEventListener("click",()=>document.querySelector                   ("#model-pricing-import-dialog")?.close?.());
document.querySelector("#refresh-pricing")?.addEventListener("click",async()=>{
 const login=consoleSession;const button=document.querySelector                   ("#refresh-pricing") ;const status=document.querySelector("#quota-refresh-status") ;
 button.disabled=true;status.textContent="Refreshing pricing…";
 try{
  const response=await gatewayFetch("/gateway/v1/models/pricing/refresh",{method:"POST",headers:await authProvider.headers()});if(login!==consoleSession)return;
  if(!response.ok)throw new Error("Pricing refresh unavailable. Last known prices retained.");
  const data=await response.json();if(login!==consoleSession)return;physicalModels=data.models||[];overviewModels=physicalModels;renderModelsPage();renderOverview();
  status.textContent=physicalModels.some(model=>model.pricing?.refresh_failed)?"Pricing refresh completed with provider failures. Last known prices retained.":"Pricing refresh completed. Providers without live pricing retain available estimates; manual rates remain in effect.";
 }catch(error){if(login===consoleSession)status.textContent=error instanceof Error?error.message:"Pricing refresh unavailable.";}finally{button.disabled=false;}
});

let budgetsDraft                   ={};
function renderBudgetEditor()      {
 const scope=document.querySelector                   ("#budget-scope") .value||"*";
 const budgets=scope==="*"?budgetsDraft.gateway:budgetsDraft.keys?.[scope];
 for(const period of ["daily","monthly"])for(const metric of ["requests","tokens","estimated_cost"]){document.querySelector                  (`#budget-${period}-${metric}`) .value=String(budgets?.[period]?.[metric]||0);}
}
let usageData    =null;
let usageKeyMetadata      =[];
let usagePeriod="daily";
function renderUsagePage()     {
 const output=document.querySelector             ("#usage-list") ;if(!usageData)return;
 const scope=document.querySelector                   ("#budget-scope") .value||"*";
 const rows=(usageData.usage||[]).filter((r    )=>r.key_id!=="*" && (scope==="*"||r.key_id===scope) && (usagePeriod==="daily"?/^\d{4}-\d{2}-\d{2}$/.test(r.period):/^\d{4}-\d{2}$/.test(r.period)));
 const requests=rows.filter((r    )=>!r.model).reduce((sum       ,r    )=>sum+Number(r.requests||0),0);
 const input=rows.reduce((sum       ,r    )=>sum+Number(r.input_tokens||0),0);const out=rows.reduce((sum       ,r    )=>sum+Number(r.output_tokens||0),0);
 const cost=rows.reduce((sum       ,r    )=>sum+Number(r.estimated_cost||0),0);const unknown=rows.reduce((sum       ,r    )=>sum+Number(r.unknown_cost_attempts||0),0);
 const budgets=(usageData.budgets||[]).filter((b    )=>(scope==="*"||b.key_id===scope)&&(b.limit.requests||b.limit.tokens||b.limit.estimated_cost));
 output.innerHTML=`<div class="section-heading"><h2>Usage Overview</h2><label class="inline-filter">Period<select id="usage-period"><option value="daily">Today · UTC</option><option value="monthly">This Month · UTC</option></select></label></div><div class="metric-grid usage-metrics"><div class="metric-card"><span class="metric-label">Client requests</span><strong class="metric-value">${consoleNumber(requests)}</strong><span class="metric-detail">${usagePeriod==="daily"?"Today":"This month"} · current scope</span></div><div class="metric-card"><span class="metric-label">Input tokens</span><strong class="metric-value">${consoleNumber(input)}</strong><span class="metric-detail">Includes classifiers and retries</span></div><div class="metric-card"><span class="metric-label">Output tokens</span><strong class="metric-value">${consoleNumber(out)}</strong><span class="metric-detail">Reported and conservative estimates</span></div><div class="metric-card"><span class="metric-label">Estimated cost</span><strong class="metric-value">$${cost.toFixed(4)}</strong><span class="metric-detail">${unknown?`${unknown} attempts with unknown cost`:"Operator estimate · USD"}</span></div></div>${budgets.length?`<details class="advanced-fields"><summary>Active Limits &amp; Resets · ${budgets.length}</summary><div class="budget-summary-list">${budgets.map((b    )=>`<div class="budget-summary"><strong>${escapeHTML(b.key_id==="*"?"Gateway":usageKeyMetadata.find(k=>k.id===b.key_id)?.name||b.key_id)} · ${escapeHTML(b.period)}</strong><span class="data-secondary">Resets ${escapeHTML(b.reset_at)}</span><span>${b.limit.requests?`${consoleNumber(b.remaining.requests)} requests remaining`:"Unlimited requests"} · ${b.limit.tokens?`${consoleNumber(b.remaining.tokens)} tokens remaining`:"Unlimited tokens"} · ${b.limit.estimated_cost?`$${Number(b.remaining.estimated_cost).toFixed(4)} remaining`:"Unlimited estimated cost"}</span></div>`).join("")}</div></details>`:""}<div class="benchmark-table-scroll"><table class="usage-table"><caption class="usage-table-caption">${usagePeriod==="daily"?"Current UTC day":"Current UTC month"}${rows[0]?.period?` · ${escapeHTML(rows[0].period)}`:""} · selected usage scope</caption><thead><tr><th scope="col">API Key</th><th scope="col">Model</th><th scope="col">Client Requests</th><th scope="col">Input / Output Tokens</th><th scope="col">Est. Tokens</th><th scope="col">Est. USD</th></tr></thead><tbody>${rows.length?rows.map((r    )=>`<tr><td><strong>${escapeHTML(r.key_name||r.key_id)}</strong></td><td>${escapeHTML(r.model||"Client request total")}${r.provider?`<span class="data-secondary">${escapeHTML(r.provider)}</span>`:""}</td><td class="numeric">${r.model?"—":consoleNumber(r.requests||0)}</td><td class="numeric">${consoleNumber(r.input_tokens||0)} / ${consoleNumber(r.output_tokens||0)}</td><td class="numeric">${consoleNumber(r.estimated_tokens||0)}</td><td class="numeric">${Number(r.estimated_cost||0).toFixed(6)}${r.unknown_cost_attempts?'<span class="data-secondary">Plus unknown costs</span>':""}</td></tr>`).join(""):'<tr><td colspan="6"><div class="empty-state"><strong>No usage in this period</strong><p>Request and model totals appear after clients use the gateway.</p></div></td></tr>'}</tbody></table></div><p class="section-note">${escapeHTML(usageData.qualification||"Operator accounting; estimated values are not provider billing.")}</p>`;
 const period=document.querySelector                   ("#usage-period") ;period.value=usagePeriod;period.addEventListener("change",()=>{usagePeriod=period.value;renderUsagePage();});
}
let providerBalancesData                        =null;
function combineProviderSnapshots(snapshots      )    {
 const ordered=[...snapshots].sort((a,b)=>(Date.parse(a.checked_at)||0)-(Date.parse(b.checked_at)||0));
 const quota=Object.assign({},...ordered);
 const merge=(rows      ,key                  ,prefer                         )=>{
  const entries=new Map            ();
  for(const row of rows){const id=key(row),old=entries.get(id);entries.set(id,old&&prefer?prefer(old,row):row);}
  return [...entries.values()];
 };
 const windowKey=(w    )=>JSON.stringify([w.name,w.unit,w.model,w.slot,w.window_type,w.window_seconds,w.source]);
 const preferWindow=(old    ,next    )=>{
  // A model may report the same account cap as informational. Keep one cap,
  // preserving its enforcement evidence without adding the balances together.
  return {...next,informational:Boolean(old.informational&&next.informational)};
 };
 quota.windows=merge(ordered.flatMap(q=>q.windows||[]),windowKey,preferWindow).map(w=>w.model?{...w,name:`${w.name} (${w.model})`}:w);
 quota.balances=merge(ordered.flatMap(q=>q.balances||[]),b=>JSON.stringify([b.name,b.unit]));
 quota.reported_usage=merge(ordered.flatMap(q=>q.reported_usage||[]),u=>JSON.stringify([u.period,u.unit,u.scope]));
 quota.limits=merge(ordered.flatMap(q=>q.limits||[]),g=>JSON.stringify([g.limit_id,g.map_key,g.scope,g.model]),(old,next)=>({...next,windows:merge([...(old.windows||[]),...(next.windows||[])],windowKey,preferWindow)}));
 if(ordered.some(q=>q.usage_usd))quota.usage_usd=Object.assign({},...ordered.map(q=>q.usage_usd||{}));
 quota.source=[...new Set(ordered.map(q=>q.source).filter(Boolean))].join(" · ");
 quota.known=ordered.some(q=>q.known);
 // One exhausted model does not establish provider-wide exhaustion.
 quota.exhausted=ordered.every(q=>q.exhausted);quota.blocked=ordered.every(q=>q.blocked);
 return quota;
}
function renderProviderBalancesPage()     {
 const output=document.querySelector             ("#provider-balances-list") ;
 if(!providerBalancesData){output.innerHTML='<p class="section-note">Provider balances have not loaded yet.</p>';return;}
 const providerSnapshots=new Map            ();
 for(const [id,status] of Object.entries(providerBalancesData)){
  const provider=id.split("/")[0];
  if(!providerSnapshots.has(provider))providerSnapshots.set(provider,{id:provider,snapshots:[]});
  providerSnapshots.get(provider).snapshots.push(status);
 }
 for(const model of providerSnapshots.values())model.quota=combineProviderSnapshots(model.snapshots);
 output.innerHTML=providerSnapshots.size?`<div class="budget-summary-list">${[...providerSnapshots.values()].map(model=>`<div class="budget-summary"><strong>${escapeHTML(model.id)}</strong>${modelTrackingMarkup(model)}</div>`).join("")}</div>`:'<div class="empty-state"><strong>No provider telemetry available</strong><p>Balances and subscription limits appear when providers report account telemetry.</p></div>';
}
async function loadProviderBalancesPage()              {
 const output=document.querySelector             ("#provider-balances-list") ;const login=consoleSession;
 const button=document.querySelector                   ("#refresh-provider-balances") ;button.disabled=true;
 if(!providerBalancesData)output.innerHTML='<p class="section-note" role="status">Loading provider balances…</p>';
 try{
  const response=await gatewayFetch("/gateway/v1/usage",{headers:await authProvider.headers()});if(login!==consoleSession)return;
  if(!response.ok)throw new Error("Provider telemetry unavailable. Refresh to retry.");
  const data=await response.json();if(login!==consoleSession)return;
  providerBalancesData=data.provider_quotas||{};renderProviderBalancesPage();
 }catch(error){if(login===consoleSession)output.innerHTML=`<div class="empty-state error-state" role="alert"><strong>Unable to load provider balances</strong><p>${escapeHTML(error instanceof Error?error.message:"Provider telemetry unavailable")}</p></div>`;}
 finally{button.disabled=false;}
}
document.querySelector("#refresh-provider-balances") .addEventListener("click",()=>{void loadProviderBalancesPage();});
async function loadUsagePage()              {
 const output=document.querySelector             ("#usage-list") ;const login=consoleSession;const button=document.querySelector                   ("#refresh-usage") ;button.disabled=true;
 try{
 const keys=await gatewayFetch("/gateway/v1/admin/api-keys",{headers:await authProvider.headers()});if(login!==consoleSession)return;if(!keys.ok)throw new Error("Keys unavailable");const metadata=await keys.json();if(login!==consoleSession)return;usageKeyMetadata=metadata.api_keys||[];const selector=document.querySelector                   ("#budget-scope") ;const chosen=selector.value;
 selector.innerHTML='<option value="*">Entire gateway</option>'+usageKeyMetadata.map((key    )=>`<option value="${escapeHTML(key.id)}">${escapeHTML(key.name)}</option>`).join("");selector.value=usageKeyMetadata.some(k=>k.id===chosen)?chosen:"*";renderBudgetEditor();
 const response=await gatewayFetch("/gateway/v1/usage",{headers:await authProvider.headers()});if(login!==consoleSession)return;if(!response.ok)throw new Error("Usage accounting unavailable. Refresh to retry.");const data=await response.json();if(login!==consoleSession)return;usageData=data;overviewUsage=data.usage||[];renderUsagePage();renderOverview();
 }catch(error){output.innerHTML=`<div class="empty-state error-state" role="alert"><strong>Unable to load usage</strong><p>${escapeHTML(error instanceof Error?error.message:"Usage unavailable")}</p></div>`;}finally{button.disabled=false;}
}
document.querySelector("#budget-scope") .addEventListener("change",()=>{renderBudgetEditor();renderUsagePage();});
for(const period of ["daily","monthly"])for(const metric of ["requests","tokens","estimated_cost"]){document.querySelector                  (`#budget-${period}-${metric}`) .addEventListener("change",()=>{
 const scope=document.querySelector                   ("#budget-scope") .value||"*";const value=Number(document.querySelector                  (`#budget-${period}-${metric}`) .value);if(!Number.isFinite(value)||value<0){saveState.textContent="Budgets must be nonnegative";return;}
 let budgets                   ;if(scope==="*")budgets=budgetsDraft.gateway ||= {};else{budgetsDraft.keys ||= {};budgets=budgetsDraft.keys[scope] ||= {};}
 (budgets[period] ||= {})[metric]=value;markDirty();void saveConfig();
});}
document.querySelector("#refresh-usage") .addEventListener("click",()=>{void loadUsagePage();});

function quotaResetLabel(raw        )       {const date=new Date(String(raw||""));if(Number.isNaN(date.getTime())||date.getUTCFullYear()<2000)return "Reset time unavailable";const seconds=Math.max(0,Math.ceil((date.getTime()-Date.now())/1000));if(seconds===0)return `Reported reset passed · awaiting quota refresh · ${consoleDateTime(raw)}`;const hours=Math.floor(seconds/3600),minutes=Math.floor(seconds%3600/60);return `Resets in ${hours}h ${minutes}m · ${consoleDateTime(raw)}`;}
function renderProviderQuotas(quotas                   )       {return `<section class="overview-panel"><h2>Provider Quotas</h2>${Object.entries(quotas).map(([id,q])=>`<div class="budget-summary"><strong>${escapeHTML(id)}</strong>${q.known?`<span>${Number(q.fraction_remaining*100).toFixed(1)}% remaining${q.exhausted?" · Exhausted":""}</span>${(q.windows||[]).map((w    )=>`<span class="data-secondary">${escapeHTML(w.name)} · ${Number(100-w.used_percent).toFixed(1)}% remaining · <span data-quota-reset="${escapeHTML(w.reset_at)}">${escapeHTML(quotaResetLabel(w.reset_at))}</span></span>`).join("")}`:`<span class="data-secondary">${escapeHTML(q.error||"Provider account telemetry unavailable")}</span>`}</div>`).join("")}</section>`;}
window.setInterval?.(()=>{document.querySelectorAll             ("[data-quota-reset]").forEach(element=>{element.textContent=quotaResetLabel(element.dataset.quotaReset);});},30000);

let activityStream                 =null;
let activityRows      =[];
let activityView="completed";
let activitySearch="";
let activityOutcomeFilter="";
let activityPage=0;
let activityPageSize=matchMedia("(max-width: 760px)").matches?5:20;
function visibleActivityRows()      {const query=activitySearch.toLowerCase();return activityRows.slice(-200).reverse().filter(e=>(activityView!=="completed"||e.kind==="request_complete")&&(!query||`${e.request_id||""} ${e.key_name||""} ${e.key_id||""} ${e.requested_model||""} ${e.selected_model||""} ${e.provider||""}`.toLowerCase().includes(query))&&(!activityOutcomeFilter||e.outcome===activityOutcomeFilter||activityOutcomeFilter==="error"&&e.status>=400));}
function activityOutcome(e    )       {return e.outcome||String(e.status||({request_start:"Started",routing:"Model selected",gap:"History gap"}                         )[e.kind]||"Recorded");}
function stopActivity()     {closeActivityInspector();activityStream?.close();activityStream=null;activityRows=[];document.querySelector("#activity-list") .textContent="";}
function renderActivity()     {
 const output=document.querySelector("#activity-list") ;const focused=document.activeElement?.id;const focusedRequest=(document.activeElement                          )?.dataset?.copyRequest;
 if(!activityRows.length){output.innerHTML='<div class="empty-state"><span class="empty-state-icon">↗</span><strong>Ready for your first request</strong><p>Request activity appears here as clients use the gateway. Debug details are captured only when enabled in Server settings.</p><a class="button secondary" href="#credentials">Manage Client Keys</a></div>';return;}
 // Keep view controls mounted during streaming updates so typing and focus survive.
 if(!output.querySelector("#activity-search")){
 output.innerHTML=`<div class="activity-toolbar console-filter"><label class="search-field" for="activity-search">Search Requests<input id="activity-search" type="search" aria-label="Search activity" maxlength="256" placeholder="Model, request ID, or API key" value="${escapeHTML(activitySearch)}"></label><label for="activity-view">View<select id="activity-view" aria-label="Activity view"><option value="completed">Completed Requests</option><option value="all">All Events</option></select></label><label for="activity-outcome">Outcome<select id="activity-outcome" aria-label="Activity outcome"><option value="">All Outcomes</option><option value="success">Success</option><option value="error">Error</option><option value="interrupted">Interrupted</option></select></label><span id="activity-count" class="section-note" role="status"></span></div><div id="activity-results"></div><div class="benchmark-pagination activity-pagination"><label for="activity-page-size" class="inline-filter">Rows<select id="activity-page-size" aria-label="Activity rows per page"><option value="5">5</option><option value="20">20</option><option value="50">50</option><option value="100">100</option></select></label><div><button id="activity-previous" class="button quiet" type="button">Previous</button><button id="activity-next" class="button quiet" type="button">Next</button></div></div>`;
 const search=document.querySelector                  ("#activity-search") ;search.addEventListener("input",()=>{activitySearch=search.value;activityPage=0;renderActivity();});
 const view=document.querySelector                   ("#activity-view") ;view.value=activityView;view.addEventListener("change",()=>{activityView=view.value;activityPage=0;renderActivity();});
 const outcome=document.querySelector                   ("#activity-outcome") ;outcome.value=activityOutcomeFilter;outcome.addEventListener("change",()=>{activityOutcomeFilter=outcome.value;activityPage=0;renderActivity();});
 const size=document.querySelector                   ("#activity-page-size") ;size.value=String(activityPageSize);size.addEventListener("change",()=>{activityPageSize=[5,20,50,100].includes(Number(size.value))?Number(size.value):20;activityPage=0;renderActivity();});
 document.querySelector("#activity-previous") .addEventListener("click",()=>{activityPage--;renderActivity();});document.querySelector("#activity-next") .addEventListener("click",()=>{activityPage++;renderActivity();});
 }
 const filtered=visibleActivityRows();activityPage=Math.min(Math.max(0,activityPage),Math.max(0,Math.ceil(filtered.length/activityPageSize)-1));const rows=filtered.slice(activityPage*activityPageSize,(activityPage+1)*activityPageSize);
 const results=document.querySelector("#activity-results") ;
 results.innerHTML=`<div class="benchmark-table-scroll"><table class="activity-table"><thead><tr><th scope="col">Time / Request</th><th scope="col">Event</th><th scope="col">API Key</th><th scope="col">Model Selection</th><th scope="col">Outcome</th><th scope="col">Duration</th><th scope="col">Input / Output</th><th scope="col">Est. USD</th>${activityDebugColumns()}</tr></thead><tbody>`+rows.map((e    )=>`<tr><td data-label="Time / Request"><time title="${escapeHTML(e.at||"")}">${consoleTime(e.at)}</time><span class="data-secondary">${consoleDate(e.at)}</span><button class="request-id-copy" type="button" data-copy-request="${escapeHTML(e.request_id||"")}" title="Copy request ID: ${escapeHTML(e.request_id||"")}" aria-label="Copy request ID ${escapeHTML(e.request_id||"")}">${escapeHTML(e.request_id?e.request_id.slice(0,12)+"…":"—")}</button></td><td data-label="Event">${escapeHTML(String(e.kind||"—").replace(/_/g," "))}</td><td data-label="API Key">${escapeHTML(e.key_name||e.key_id||"—")}</td><td data-label="Model Selection"><strong>${escapeHTML(e.selected_model||e.requested_model||"—")}</strong><span class="data-secondary">${e.requested_model&&e.selected_model&&e.requested_model!==e.selected_model?`From ${escapeHTML(e.requested_model)} · `:""}${escapeHTML(e.provider||"")}</span></td><td data-label="Outcome"><span class="status-badge ${e.outcome==="success"||e.status&&e.status<400?"success":e.outcome==="error"||e.outcome==="failure"||e.outcome==="interrupted"||e.status>=400?"failure":"neutral"}">${escapeHTML(activityOutcome(e))}</span>${e.outcome&&e.status?`<span class="data-secondary">HTTP ${escapeHTML(String(e.status))}</span>`:""}</td><td class="numeric" data-label="Duration">${typeof e.duration_ms==="number"?`${consoleNumber(e.duration_ms)} ms`:"—"}</td><td class="numeric" data-label="Input / Output">${consoleNumber(e.input_tokens||0)} / ${consoleNumber(e.output_tokens||0)}${e.estimated_tokens?'<span class="data-secondary">Estimated</span>':""}</td><td class="numeric" data-label="Estimated USD">${e.unknown_cost?'<span class="data-secondary">Unknown</span>':e.estimated_cost==null?"—":Number(e.estimated_cost).toFixed(6)}</td>${activityDebugButtons(e)}</tr>`).join("")+`${rows.length?"":`<tr><td colspan="${8+(loadedConfiguration.activity?.prompt_logging?1:0)+(loadedConfiguration.activity?.jev_decision_logging?1:0)}"><div class="empty-state"><strong>No requests match this view</strong><p>Try another search or clear your outcome filter. All events includes starts and routing decisions.</p></div></td></tr>`}</tbody></table></div><p class="section-note">${activityView==="completed"?"Completed requests from":"Showing"} the latest ${activityRows.length} recorded events. Timestamps use ${escapeHTML(consoleTimezoneLabel())}; cost is estimated.</p>`;
 results.querySelectorAll                   ("[data-inspect-activity]").forEach(button=>button.addEventListener("click",()=>openActivityInspector(Number(button.dataset.inspectActivity),button.dataset.inspectKind )));
 document.querySelector("#activity-count") .textContent=`${filtered.length?activityPage*activityPageSize+1:0}–${Math.min((activityPage+1)*activityPageSize,filtered.length)} of ${filtered.length} ${activityView==="completed"?"requests":"events"}`;
 document.querySelector                   ("#activity-previous") .disabled=activityPage===0;document.querySelector                   ("#activity-next") .disabled=(activityPage+1)*activityPageSize>=filtered.length;
 if(focused==="activity-next"&&document.querySelector                   ("#activity-next") .disabled)(document.querySelector                   ("#activity-previous") .disabled?document.querySelector                  ("#activity-search"):document.querySelector                   ("#activity-previous"))?.focus?.();
 if(focused==="activity-previous"&&document.querySelector                   ("#activity-previous") .disabled)(document.querySelector                   ("#activity-next") .disabled?document.querySelector                  ("#activity-search"):document.querySelector                   ("#activity-next"))?.focus?.();
 if(focusedRequest)Array.from(results.querySelectorAll                   ("[data-copy-request]")).find(button=>button.dataset.copyRequest===focusedRequest)?.focus?.();
 results.querySelectorAll                   ("[data-copy-request]").forEach(button=>button.addEventListener("click",async()=>{try{await navigator.clipboard.writeText(button.dataset.copyRequest||"");toast("Request ID copied");}catch{toast("Copy unavailable. The complete request ID is shown in the button tooltip.");}}));
}

async function startActivity()              {
 if(activityStream || !consoleSession)return;
 const status=document.querySelector("#activity-status") ;const login=consoleSession;
 try{const response=await gatewayFetch("/gateway/v1/activity?limit=200",{headers:await authProvider.headers()});if(login!==consoleSession)return;if(!response.ok)throw new Error();const history=await response.json();if(login!==consoleSession)return;activityRows=history.events||[];renderActivity();
 if(location.hash!=="#activity" || !consoleSession || activityStream)return;
 const last=activityRows.at(-1)?.id||0;const stream=new EventSource(`/gateway/v1/activity/events?after_id=${last}`);activityStream=stream;
 stream.onopen=()=>{status.textContent="Live · request activity";status.className="status-badge success";};stream.onerror=()=>{status.textContent="Reconnecting · replaying available events";status.className="status-badge neutral";};
 stream.addEventListener("activity",event=>{const e=JSON.parse((event                ).data);if(e.kind==="history_reset"){closeActivityInspector();void refreshActivityHistory();return;}if(!activityRows.some(row=>row.id===e.id)){activityRows.push(e);activityRows=activityRows.slice(-200);renderActivity();}});
 stream.addEventListener("gap",()=>{status.textContent="History gap · some older events are unavailable";status.className="status-badge neutral";});
 }catch{status.textContent="Activity unavailable. Sign in and reopen this page to retry.";status.className="status-badge failure";}
}

function activityDebugColumns()       {return `${loadedConfiguration.activity?.prompt_logging?'<th scope="col">Prompt</th>':""}${loadedConfiguration.activity?.jev_decision_logging?'<th scope="col">Jev Decision</th>':""}`;}
function activityDebugButtons(e    )       {
 const button=(kind       ,label       ,available        )=>`<td data-label="${label}"><button type="button" class="button quiet" data-inspect-activity="${e.id}" data-inspect-kind="${kind}" ${available?"":"disabled"} title="${available?`Inspect ${label.toLowerCase()}`:"Details were not recorded for this event"}">Inspect ${label}</button></td>`;
 return `${loadedConfiguration.activity?.prompt_logging?button("prompt","Prompt",Boolean(e.prompt)):""}${loadedConfiguration.activity?.jev_decision_logging?button("jev","Jev Decision",Boolean(e.jev_request)):""}`;
}
function readableActivityBody(body       )       {
 try{
  const formatted=JSON.stringify(JSON.parse(body),null,2);
  // Expand newlines inside prompt text as well as indenting the JSON structure.
  return formatted.split("\n").map(line=>line.replace(/"(?:\\.|[^"\\])*"/g,token=>{
   const value=JSON.parse(token);if(!value.includes("\n"))return token;
   const indent=line.match(/^\s*/)?.[0]||"";
   return '"'+value.split("\n").map((part       )=>JSON.stringify(part).slice(1,-1)).join("\n"+indent+"  ")+'"';
  })).join("\n");
 }catch{return body;}
}
function openActivityInspector(id       ,kind       )     {
 const e=activityRows.find(row=>row.id===id);if(!e)return;
 if(kind==="prompt"&&!loadedConfiguration.activity?.prompt_logging||kind==="jev"&&!loadedConfiguration.activity?.jev_decision_logging)return;
 const dialog=document.querySelector                   ("#activity-inspector-dialog") ;
 const title=document.querySelector             ("#activity-inspector-title") ;
 title.textContent=kind==="prompt"?"API Prompt":"Jev Decision";
 document.querySelector("#activity-inspector-note") .textContent=e.debug_truncated?"Debug capture was truncated at 64 KiB per body.":"Captured API body formatted for reading. HTTP authorization headers are not recorded.";
 const block=(label       ,body       )=>`<section><h2>${label}</h2><pre class="activity-debug-body">${escapeHTML(readableActivityBody(body))}</pre></section>`;
 document.querySelector("#activity-inspector-content") .innerHTML=kind==="prompt"?block("API Request",e.prompt||"Prompt was not recorded."):block("Request Sent To Jev",e.jev_request||"No Jev request was recorded.")+block("Jev Response",e.jev_response||"No response was received.");
 dialog.showModal();title.focus?.();
}
function closeActivityInspector()     {document.querySelector                   ("#activity-inspector-dialog")?.close?.();document.querySelector("#activity-inspector-content") .textContent="";}
document.querySelector("#close-activity-inspector") .addEventListener("click",closeActivityInspector);
document.querySelector("#activity-inspector-dialog") .addEventListener("close",()=>{document.querySelector("#activity-inspector-content") .textContent="";});

let activityHistorySequence=0;
async function refreshActivityHistory()              {
 const sequence=++activityHistorySequence,login=consoleSession,stream=activityStream;
 try{const response=await gatewayFetch("/gateway/v1/activity?limit=200",{headers:await authProvider.headers()});if(!response.ok)return;const history=await response.json();if(sequence!==activityHistorySequence||login!==consoleSession||stream!==activityStream)return;activityRows=history.events||[];renderActivity();}catch{document.querySelector("#activity-status") .textContent="Activity refresh unavailable. Reopen this page to retry.";}
}

function consoleEditButtonMarkup(label       )        { return `<svg class="action-icon" viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path d="m16 3 5 5-12 12H4v-5L16 3ZM13 6l5 5"/></svg><span class="action-label">${escapeHTML(label)}</span>`; }



// Browser authentication uses an HttpOnly cookie. Only the CSRF token is in memory.




class CookieSessionProvider                         {
  async headers()                       { return csrfToken ? { "X-CSRF-Token":csrfToken } : {}; }
}

const authProvider               = new CookieSessionProvider();
async function gatewayFetch(url       ,options             )                  {
  const login = consoleSession;
  const response=await fetch(url,options);
  if(response.status===401 && login===consoleSession){expireConsoleSession();document.querySelector                   ("#login-dialog")?.showModal?.();}
  return response;
}
const form = document.querySelector                 ("#config-form") ;
const providerList = document.querySelector                ("#providers-list") ;
const saveState = document.querySelector                 ("#save-state") ;
let consoleSession = "";
let csrfToken="";
let configurationRevision = "";
let loadedConfiguration                    = {};
let smartRouterDrafts                   ={};
let selectedSmartRouter="smart";
let editGeneration=0;
let saveRunning=false;
let saveQueued=false;
let saveConflict=false;
let editingProvider            =null;
let editingOriginal                        =null;
let editingGeneration=0;
let editingSaveLabel="";
let editingStatus                         ;
let editingCatalog                   ;
function openProviderEditor(index       ,adding=false)     {editingProvider=index;editingOriginal=adding?null:structuredClone(providers[index]);editingGeneration=editGeneration;editingSaveLabel=saveState.textContent||"";editingStatus=providerStatuses.get(providers[index]);editingCatalog=classifierCatalogs.get(providers[index]);const dialog=providerList.querySelector                   (`[data-provider-dialog="${index}"]`);dialog?.showModal?.();dialog?.querySelector                   ('[data-key="type"]')?.focus?.();}
function cancelProviderEditor()     {if(editingProvider===null)return;const returnIndex=editingProvider;const adding=!editingOriginal;if(editingOriginal){const provider=providers[editingProvider];Object.keys(provider).forEach(key=>delete provider[key]);Object.assign(provider,editingOriginal);if(editingStatus)providerStatuses.set(provider,editingStatus);else providerStatuses.delete(provider);if(editingCatalog)classifierCatalogs.set(provider,editingCatalog);else classifierCatalogs.delete(provider);}else providers.splice(editingProvider,1);editGeneration=editingGeneration;saveState.textContent=editingSaveLabel;editingProvider=null;editingOriginal=null;editingStatus=undefined;editingCatalog=undefined;renderProviders();(adding?document.querySelector                   ("#add-provider"):providerList.querySelector                   (`[data-edit="${returnIndex}"]`))?.focus?.();}
let signedInUsername = "";
let benchmarksEnabledSaved = false;
let overviewModels           =null;
let overviewUsage           =null;
let overviewRecent           =null;
let overviewClientModel="";
let overviewPeriod="daily";
let overviewLoading=false;
let overviewError="";
let consoleTimezone="";
function consoleTimezoneLabel()       {return consoleTimezone||"browser timezone";}
function consoleDateTime(raw        )       {const date=new Date(String(raw||""));return Number.isNaN(date.getTime())?"—":date.toLocaleString([], {timeZone:consoleTimezone||undefined});}
function consoleNumber(raw        )       {return typeof raw==="number" && Number.isFinite(raw)?raw.toLocaleString():"—";}
function consoleDate(raw        )       {const date=new Date(String(raw||""));return Number.isNaN(date.getTime())?"—":date.toLocaleDateString([], {timeZone:consoleTimezone||undefined,month:"short",day:"numeric",year:"numeric"});}
function consoleTime(raw        )       {const date=new Date(String(raw||""));return Number.isNaN(date.getTime())?"—":date.toLocaleTimeString([], {timeZone:consoleTimezone||undefined,hour:"2-digit",minute:"2-digit",second:"2-digit"});}
function overviewPeriodMatches(period       )        {return overviewPeriod==="daily"?/^\d{4}-\d{2}-\d{2}$/.test(period):/^\d{4}-\d{2}$/.test(period);}
function overviewModelUsage()      {
 const grouped=new Map            ();
 for(const row of overviewUsage||[]){if(row.key_id!=="*"||!row.model||!overviewPeriodMatches(row.period))continue;const key=`${row.provider||""}/${row.model}`;const previous=grouped.get(key)||{model:row.model,provider:row.provider,tokens:0,cost:0,unknown:0};previous.tokens+=Number(row.input_tokens||0)+Number(row.output_tokens||0);previous.cost+=Number(row.estimated_cost||0);previous.unknown+=Number(row.unknown_cost_attempts||0);grouped.set(key,previous);}
 return [...grouped.values()].sort((a,b)=>b.tokens-a.tokens||a.model.localeCompare(b.model));
}
function overviewCompletedRequests()      {return (overviewRecent||[]).filter(event=>event.kind==="request_complete");}
function overviewRequestSucceeded(event    )        {return event.outcome==="success"||!event.outcome&&event.status>=200&&event.status<400;}
function renderOverviewMonitoring()       {
 const usage=overviewModelUsage();const tokens=usage.reduce((sum,row)=>sum+row.tokens,0);const cost=usage.reduce((sum,row)=>sum+row.cost,0);const unknown=usage.reduce((sum,row)=>sum+row.unknown,0);
 const completed=overviewCompletedRequests();const successes=completed.filter(overviewRequestSucceeded).length;const errors=completed.filter(e=>e.outcome==="error"||e.status>=400).length;const recent=completed.slice(-5).reverse();
 return `<div class="overview-monitor-grid"><section class="overview-panel"><div class="section-heading"><div><h2>Model Usage ${overviewPeriod==="daily"?"Today":"This Month"}</h2><span class="data-secondary">Current UTC ${overviewPeriod==="daily"?"day":"month"} · input + output tokens</span></div><a href="#state">View Usage →</a></div><div class="usage-distribution-summary"><strong>${overviewUsage?consoleNumber(tokens):"—"}<span> tokens</span></strong><span>${overviewUsage?`$${cost.toFixed(4)} estimated${unknown?" + unknown costs":""}`:"Usage not loaded"}</span></div><div class="usage-distribution">${usage.length?usage.slice(0,5).map((row,index)=>`<div class="distribution-row"><div class="distribution-row-label"><strong>${escapeHTML(row.model)}</strong><span>${consoleNumber(row.tokens)} tokens</span></div><div class="distribution-bar"><span style="width:${tokens?row.tokens/tokens*100:0}%;--bar-index:${index}" aria-label="${escapeHTML(row.model)}: ${consoleNumber(row.tokens)} of ${consoleNumber(tokens)} tokens"></span></div></div>`).join(""):`<div class="monitor-empty"><strong>${overviewUsage?"No model usage yet":"Usage data not loaded"}</strong><p>${overviewUsage?"Send a request through Torii to see your model distribution.":"Gateway accounting will appear when it is available."}</p></div>`}</div></section><section class="overview-panel"><div class="section-heading"><div><h2>Recent Requests</h2><span class="data-secondary">Latest completed requests · ${escapeHTML(consoleTimezoneLabel())}</span>${overviewRecent?`<span class="recent-request-summary">${completed.length} completed · ${successes} successful · ${errors} errors in last ${overviewRecent.length} recorded events</span>`:""}</div><a href="#activity">View Activity →</a></div>${recent.length?recent.map(e=>`<div class="recent-request-row"><div><strong>${escapeHTML(e.selected_model||e.requested_model||"Unknown model")}</strong><span class="data-secondary">${consoleTime(e.at)} · ${escapeHTML(e.key_name||"Client request")}</span></div><span class="status-badge ${overviewRequestSucceeded(e)?"success":"failure"}">${escapeHTML(e.outcome||"Completed")}</span><span class="data-secondary">${consoleNumber(e.duration_ms)} ms</span></div>`).join(""):`<div class="monitor-empty"><strong>${overviewRecent?"Waiting for your first request":"Activity data not loaded"}</strong><p>Completed request outcomes will appear here.</p></div>`}</section></div>`;
}
function renderClientConnection()       {
 const baseURL=location.origin?`${location.origin}/v1`:"/v1";const smart=loadedConfiguration.router?.model_name||"smart";const enabled=(overviewModels||[]).filter(model=>model.enabled);
 const models=[smart,...(loadedConfiguration.smart_routers||[]).map((r    )=>r.model_name),...enabled.map(model=>model.id)];if(!models.includes(overviewClientModel))overviewClientModel=smart;
 return `<section class="overview-panel connect-client"><div class="section-heading"><div><h2>Connect A Client</h2><span class="data-secondary">Use an OpenAI-compatible application or SDK</span></div><a href="#credentials">API Keys →</a></div><label>Base URL<div class="copy-field"><input id="overview-base-url" value="${escapeHTML(baseURL)}" readonly aria-label="Client base URL"><button id="copy-client-url" class="button secondary" type="button">Copy</button></div></label><label>Model<div class="copy-field"><select id="overview-client-model">${models.map(id=>`<option value="${escapeHTML(id)}" ${id===overviewClientModel?"selected":""}>${escapeHTML(!id.includes("/")?`${id} · gateway routing`:modelSelectorLabel(enabled.find(model=>model.id===id)))}</option>`).join("")}</select><button id="copy-client-model" class="button secondary" type="button">Copy</button></div></label><p class="section-note">Create a named <a href="#credentials">Gateway API Key</a> for each application. Choose ${escapeHTML(smart)} to use your routing policy.</p></section>`;
}
function initializeClientConnection()     {
 document.querySelector                   ("#overview-client-model")?.addEventListener("change",event=>{overviewClientModel=(event.target                     ).value;});
 for(const [button,field,label] of [["#copy-client-url","#overview-base-url","Base URL"],["#copy-client-model","#overview-client-model","Model"]])document.querySelector(button)?.addEventListener("click",async()=>{try{const input=document.querySelector                  (field) ;await navigator.clipboard.writeText(input.value);toast(`${label} copied`);}catch{const input=document.querySelector                  (field);input?.focus?.();input?.select?.();toast(`Select and copy the ${label.toLowerCase()} from the field.`);}});
}
function renderOverview()     {
 const summary=document.querySelector("#overview-summary");const workspace=document.querySelector("#overview-workspace");if(!summary||!workspace)return;
 const period=document.querySelector                   ("#overview-period");if(period)period.value=overviewPeriod;
 const enabled=providers.filter(p=>p.enabled!==false).length;
 const models=overviewModels?.filter(m=>m.enabled);
 // Client request totals are stored in gateway rows with an empty model. Model
 // attempt rows include classifier/retries and must never be added to requests.
 const daily=overviewUsage?.filter(r=>r.key_id==="*" && !r.model && overviewPeriodMatches(r.period));
 const requests=daily?.reduce((sum,r)=>sum+Number(r.requests||0),0);
 const routing=loadedConfiguration.router?.classifier?.type;
 const usage=overviewModelUsage();const tokens=usage.reduce((sum,row)=>sum+row.tokens,0);const cost=usage.reduce((sum,row)=>sum+row.cost,0);const unknown=usage.reduce((sum,row)=>sum+row.unknown,0);
 const completed=overviewCompletedRequests();const successful=completed.filter(overviewRequestSucceeded).length;const periodLabel=`current UTC ${overviewPeriod==="daily"?"day":"month"}`;
 const metrics=[{label:overviewPeriod==="daily"?"Requests Today":"Requests This Month",value:overviewUsage?consoleNumber(requests):"—",detail:`Client requests · ${periodLabel}`,href:"state"},{label:"Processed Tokens",value:overviewUsage?consoleNumber(tokens):"—",detail:`Input + output · ${periodLabel}`,href:"state"},{label:"Estimated Cost",value:overviewUsage?`$${cost.toFixed(4)}`:"—",detail:overviewUsage?(unknown?`Partial USD total · ${unknown} unknown-cost attempts`:`Operator estimate · USD · ${periodLabel}`):"Usage data not loaded",href:"state"},{label:"Recent Success",value:completed.length?`${(successful/completed.length*100).toFixed(1)}%`:"—",detail:overviewRecent?`${successful} of ${completed.length} completed · last ${overviewRecent.length} recorded events`:"Activity data not loaded",href:"activity"}];
 summary.className="metric-grid";summary.innerHTML=metrics.map(m=>`<a class="metric-card" href="#${m.href}"><span class="metric-label">${m.label}</span><strong class="metric-value">${escapeHTML(m.value)}</strong><span class="metric-detail">${escapeHTML(m.detail)}</span></a>`).join("")+`<div class="overview-context" aria-label="Current gateway configuration"><a href="#providers"><strong>${configurationLoaded?enabled:"—"}</strong> Enabled Providers${configurationLoaded?` · ${providers.length} configured`:""}</a><a href="#models"><strong>${overviewModels?models?.length||0:"—"}</strong> Enabled Models${overviewModels?` · ${overviewModels.length} discovered`:""}</a><a href="#routing"><strong>${configurationLoaded?(routing&&routing!=="disabled"?"Smart":"Deterministic"):"—"}</strong> Routing</a></div>`;
 // Usage drill-down follows the selected overview period and gateway scope.
 summary.querySelectorAll                   ('a[href="#state"]').forEach(link=>link.addEventListener("click",()=>{usagePeriod=overviewPeriod;const scope=document.querySelector                   ("#budget-scope");if(scope)scope.value="*";}));
 workspace.className="overview-grid";
 workspace.innerHTML=`${providers.length?renderOverviewMonitoring():""}<section class="overview-panel"><div class="section-heading"><h2>Connected Providers</h2><a href="#providers">Manage Providers →</a></div>${providers.length?providers.slice(0,6).map(p=>{const status=providerStatuses.get(p);const count=classifierProvider(p)?classifierCatalogs.get(p)?.length:overviewModels?.filter(m=>m.provider_id===p.id).length;return `<div class="overview-provider-row"><span class="provider-avatar">${escapeHTML(String(p.id||"P").slice(0,1).toUpperCase())}</span><div><strong>${value(p,"id")}</strong><span class="data-secondary">${escapeHTML(providerTypeLabel(String(p.type)))}</span></div><span class="status-badge ${p.enabled===false?"neutral":status?.state||"neutral"}">${p.enabled===false?"Disabled":status?.state==="success"?"Connected":status?.state==="failure"?"Needs attention":status?.state==="pending"?"Checking…":"Not checked"}</span>${count===undefined?"":`<span class="data-secondary">${count} models</span>`}</div>`;}).join(""):`<div class="empty-state"><strong>${configurationLoaded?"Your gateway starts here":"Welcome to your control plane"}</strong><p>${configurationLoaded?"Connect an upstream to discover models and start routing requests.":"Sign in to manage providers, model access and routing."}</p><a class="button primary" href="${configurationLoaded?"#providers":"/login"}">${configurationLoaded?"Connect A Provider":"Sign In"}</a></div>`}</section>${providers.length?renderClientConnection():`<section class="overview-panel"><h2>Your Next Step</h2><a class="workflow-link" href="#providers"><span class="workflow-number">01</span><span><strong>Connect Your Providers</strong><span class="data-secondary">API Endpoints And Subscription Accounts</span></span><span>→</span></a><a class="workflow-link" href="#models"><span class="workflow-number">02</span><span><strong>Choose Your Model Catalog</strong><span class="data-secondary">Inspect Capabilities And Control Model Access</span></span><span>→</span></a><a class="workflow-link" href="#routing"><span class="workflow-number">03</span><span><strong>Shape Your Routing Policy</strong><span class="data-secondary">Choose Intelligent Or Deterministic Selection</span></span><span>→</span></a><a class="workflow-link" href="#credentials"><span class="workflow-number">04</span><span><strong>Create A Client API Key</strong><span class="data-secondary">Give Each Application Its Own Gateway Access</span></span><span>→</span></a></section>`}${overviewError?`<p class="notice" role="status">${escapeHTML(overviewError)}</p>`:""}`;
 initializeClientConnection();renderSmartRouterScope();
}
function providerTypeLabel(type       )       {return ({"opencode-go":"OpenCode Go","opencode-zen":"OpenCode Zen","standard-compute":"Standard Compute",zai:"Z AI","kimi-coding":"Kimi Coding",minimax:"MiniMax",deepseek:"DeepSeek",openai_compatible:"OpenAI Compatible",openai:"OpenAI", "openai-codex":"OpenAI Codex",anthropic:"Anthropic",gemini:"Google Gemini",xai:"xAI",openrouter:"OpenRouter",ollama:"Ollama",nous:"Nous",typesafe:"TypeSafe",jev:"TypeSafe",jev_compatible:"Jev Compatible"}                         )[type]||type;}
function providerTypeOptions(provider                              ,subscriptionOnly=false)       {
 const groups=[{label:"API Providers",types:["openai_compatible","openai","anthropic","gemini","xai","openrouter","opencode-go","opencode-zen","standard-compute","zai","kimi-coding","minimax","deepseek","nous"]},{label:"Subscriptions",types:["openai-codex"]},{label:"Self-Hosted",types:["ollama"]},{label:"Classifiers",types:["typesafe","jev_compatible"]}];
 return groups.filter(group=>!subscriptionOnly||group.label==="Subscriptions").map(group=>`<optgroup label="${group.label}">${group.types.map(type=>`<option value="${type}" ${provider.type===type?"selected":""}>${escapeHTML(providerTypeLabel(type))}</option>`).join("")}</optgroup>`).join("");
}
function providerTypeDescription(type       )       {
 return ({openai_compatible:"Connect any API that supports OpenAI-compatible endpoints.",openai:"Connect OpenAI with your API key.",anthropic:"Connect Anthropic with your API key.",gemini:"Connect Google Gemini with your API key.",xai:"Connect xAI with your API key.",openrouter:"Connect OpenRouter with your API key.","openai-codex":"Connect your OpenAI Codex subscription using OAuth login.",nous:"Connect Nous Portal with your API key.",ollama:"Connect a self-hosted Ollama server and discover its model catalog.",typesafe:"Connect TypeSafe decision models for passthrough and Smart routing.",jev:"Connect TypeSafe decision models for passthrough and Smart routing.",jev_compatible:"Connect a Jev-compatible classifier, such as Laya, for Smart routing."}                         )[type]||"Configure this provider integration.";
}
async function loadOverview()              {
 if(!consoleSession||!configurationLoaded||overviewLoading)return;const login=consoleSession;overviewLoading=true;overviewError="";renderOverview();
 const results=await Promise.allSettled([gatewayFetch("/gateway/v1/models",{headers:await authProvider.headers()}),gatewayFetch("/gateway/v1/usage",{headers:await authProvider.headers()}),gatewayFetch("/gateway/v1/activity?limit=30",{headers:await authProvider.headers()})]);
 if(login!==consoleSession){overviewLoading=false;return;}
 for(let i=0;i<results.length;i++){const result=results[i];try{if(result.status!=="fulfilled"||!result.value.ok)throw new Error();const data=await result.value.json();if(login!==consoleSession)return;if(i===0)overviewModels=data.models||[];else if(i===1)overviewUsage=data.usage||[];else overviewRecent=data.events||[];}catch{overviewError="Some gateway data could not be loaded. Open Models, Usage & quota, or Activity to retry.";}}
 overviewLoading=false;renderOverview();
}
function updateConsoleIdentity()       {
  const signedIn = Boolean(consoleSession && signedInUsername);
  document.querySelector                   ("#console-login") .hidden = signedIn;
  document.querySelector                   ("#console-logout") .hidden = !signedIn;
  document.querySelector("#signed-in-user") .textContent = signedIn ? signedInUsername : "Not signed in";
}
function expireConsoleSession()     {
 stopActivity();consoleSession="";csrfToken="";signedInUsername="";clearNewAPIKey();if(configurationLoaded)document.querySelector                   ("#save-button") .hidden=false;updateConsoleIdentity();updateBenchmarkControls();saveState.textContent="Session expired. Sign in to retry; your draft is retained.";
}
function signOutConsole()       {
 stopActivity();
  consoleSession = "";csrfToken="";loadedConfiguration={};smartRouterDrafts={};selectedSmartRouter="smart";editingProvider=null;editingOriginal=null;
  configurationRevision = "";
  signedInUsername = "";
  clearNewAPIKey();
  document.querySelector                   ("#new-key-dialog") .close();
  document.querySelector                   ("#create-key-dialog") .close();
  document.querySelector                  ("#client-key-name") .value = "";
  providers = [];
  overviewModels=null;overviewUsage=null;overviewRecent=null;overviewClientModel="";overviewLoading=false;overviewError="";physicalModels=[];clearModelPricing();usageData=null;usageKeyMetadata=[];providerBalancesData=null;document.querySelector("#provider-balances-list") .textContent="";document.querySelector("#models-list") .textContent="";document.querySelector("#usage-list") .textContent="";
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
const providerOriginalIDs = new WeakMap                                      ();
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
  return provider.type === "typesafe" || provider.type === "jev" || provider.type === "jev_compatible";
}
function availableClassifiers()           {
  return providers.filter(provider => provider.enabled!==false && providerStatuses.get(provider)?.state === "success")
    .flatMap(provider => classifierCatalogs.get(provider) || []).filter(id => modelEnabledDraft[id] !== false);
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
    ? "No classifier models are available. Connect a provider with decision models (TypeSafe, OpenRouter, or a Jev-compatible service such as Laya) on the Providers page, then test its connection. Deterministic routing remains available."
    : "The selected classifier is unavailable. Check its provider or choose another discovered model.";

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
  const editor=providerList.querySelector             (`[data-editor-status="${index}"]`);if(editor)editor.textContent=status.message;
  const button = providerList.querySelector                   (`[data-test="${index}"]`);
  if (button) button.disabled = status.state === "pending";
  updateClassifierOptions();
  renderOverview();
}

function redactProviderKeys(message        )         {
  providers.forEach(provider => {
    for(const key of [provider.api_key,provider.tracking_token]) if(key) message=message.split(String(key)).join("[redacted]");
  });
  return benchmarkSafeMessage(message);
}

function escapeHTML(value        )         { return value.replace(/[&<>"]/g, ch => ({"&":"&amp;", "<":"&lt;", ">":"&gt;", '"':"&quot;"}[ch] )); }
function value(provider                                  , key        )         { return escapeHTML(String(provider[key] ?? "")); }
function checked(provider                                  , key        )         { return provider[key] === false ? "" : "checked"; }
function subscriptionProvider(provider                                  )          { return provider.type === "openai-codex"; }
const knownProviderIDs                         = { "opencode-go":"opencode-go","opencode-zen":"opencode-zen","standard-compute":"standard-compute",zai:"zai","kimi-coding":"kimi-coding",minimax:"minimax",deepseek:"deepseek", openai: "openai", "openai-codex": "openai-codex", anthropic: "anthropic", gemini: "gemini", xai: "xai", openrouter: "openrouter", ollama: "ollama", nous: "nous", typesafe: "typesafe", jev: "typesafe" };
// Complete API URLs shown when selecting a hosted integration. Custom services
// require an operator URL; Ollama uses the existing local/remote detection flow.
const knownProviderURLs                         = {
  openai: "https://api.openai.com/v1",
  anthropic: "https://api.anthropic.com",
  gemini: "https://generativelanguage.googleapis.com/v1beta/openai",
  xai: "https://api.x.ai/v1",
  openrouter: "https://openrouter.ai/api/v1",
  "opencode-go": "https://opencode.ai/zen/go/v1",
  "opencode-zen": "https://opencode.ai/zen/v1",
  "standard-compute": "https://api.stdcmpt.com/v1",
  zai: "https://api.z.ai/api/coding/paas/v4",
  "kimi-coding": "https://api.kimi.com/coding/v1",
  minimax: "https://api.minimax.io/v1",
  deepseek: "https://api.deepseek.com/v1",
  nous: "https://inference-api.nousresearch.com/v1",
  "openai-codex": "https://chatgpt.com/backend-api/codex",
  typesafe: "https://api.typesafe.ai/v1",
  jev: "https://api.typesafe.ai/v1",
};
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
      <div class="provider-card-head"><div class="provider-card-title"><span class="provider-avatar">${escapeHTML(String(provider.id||"P").slice(0,1).toUpperCase())}</span><div><span data-provider-title="${index}">${value(provider,"id") || `Provider ${index + 1}`}</span><span class="data-secondary">${escapeHTML(providerTypeLabel(String(provider.type)))}${provider.enabled===false?" · Disabled":""}</span>${provider.api_url&&!subscriptionProvider(provider)?`<span class="provider-endpoint" title="${value(provider,"api_url")}">${value(provider,"api_url")}</span>`:""}</div></div><div class="provider-card-actions"><button class="button quiet" type="button" data-provider-models="${index}">Models →</button><button class="test-provider button secondary" type="button" data-test="${index}">${subscriptionProvider(provider) ? "Check Status" : "Test Connection"}</button>${subscriptionProvider(provider) ? `<button class="login-provider button secondary" type="button" data-login="${index}">Log In</button>` : ""}<button class="button secondary" type="button" data-edit="${index}"><svg class="action-icon" viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path d="m16 3 5 5-12 12H4v-5L16 3ZM13 6l5 5"/></svg><span class="action-label">Edit</span></button><button class="action-icon-only button danger" type="button" data-remove="${index}" aria-label="Remove Provider" title="Remove Provider"><svg class="action-icon" viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path d="M3 6h18M9 6V4h6v2M5 6l1 14h12l1-14M10 10v6M14 10v6"/></svg></button></div></div>
      <div class="provider-result" data-result="${index}" role="status" aria-live="polite">Not checked yet</div>
      <dialog class="provider-editor" data-provider-dialog="${index}" role="dialog" aria-modal="true" aria-labelledby="provider-editor-title-${index}"><div class="dialog-heading"><span class="eyebrow">Upstream Connection</span><h2 id="provider-editor-title-${index}">${editingProvider===index&&!editingOriginal?"Add Provider":provider.id?`Edit ${value(provider,"id")}`:"Add Provider"}</h2><p>${escapeHTML(providerTypeDescription(String(provider.type)))}</p></div><div data-editor-status="${index}" role="status" aria-live="polite"></div><div class="check-row"><label class="checkbox"><input type="checkbox" data-provider="${index}" data-key="enabled" ${provider.enabled!==false?"checked":""}> Provider Enabled</label><label class="checkbox"><input type="checkbox" data-provider="${index}" data-key="treat_models_as_free" ${provider.treat_models_as_free?"checked":""}> Treat Models As Free</label></div>${subscriptionProvider(provider) ? `<div class="grid two"><label data-tooltip="The subscription integration used by Torii. OAuth credentials are encrypted in the gateway database.">Type<select data-provider="${index}" data-key="type">${providerTypeOptions(provider,editingProvider!==index||editingOriginal!==null)}</select></label><label data-tooltip="The stable ID used as the prefix of public model names. Edit this namespace to choose the prefix of public model names. Saved router references follow ID changes.">Provider ID<input data-provider="${index}" data-key="id" value="${value(provider,"id")}" required /></label></div><p class="provider-subscription-note">This subscription provider uses its OAuth login. API URL, API keys, and discovery controls are not used.</p><button class="button secondary" type="button" data-login="${index}">Log In</button>` : `<div class="grid three">
        <label data-tooltip="The provider integration Torii should use. TypeSafe and Jev-compatible providers supply decision models for System One and Smart routing.">Type<select data-provider="${index}" data-key="type">${providerTypeOptions(provider,false)}</select></label>
        <label data-tooltip="The stable ID used as the prefix of public model names. Edit this namespace to choose the prefix of public model names. Saved router references follow ID changes.">Provider ID<input data-provider="${index}" data-key="id" value="${value(provider,"id")}" required /></label>
        ${provider.type==="ollama"?`<label>Port<input type="number" min="1" max="65535" data-provider="${index}" data-key="ollama_port" value="${value(provider,"ollama_port")}" placeholder="Auto-detect" /></label><button type="button" class="button secondary" data-detect-ollama="${index}">Detect Ollama</button>`:`<label data-tooltip="The complete upstream URL Torii should call. Include any required API path yourself; Torii does not add /v1 to this field."><span>API URL</span><input data-provider="${index}" data-key="api_url" value="${value(provider,"api_url")}" placeholder="https://api.example.com" /></label>`}
        ${provider.type === "ollama" ? "" : `<label data-tooltip="Enter an upstream API key to replace the configured credential. Save encrypts it in SQLite; saved keys cannot be revealed.">API Key<span class="password-field"><input id="provider-api-key-${index}" type="password" autocomplete="new-password" data-provider="${index}" data-key="api_key" placeholder="${provider.bearer_token ? "Configured · leave blank to keep" : "Enter provider API key"}" /><button class="reveal-button" type="button" data-reveal-provider-key="${index}" aria-controls="provider-api-key-${index}" aria-label="Reveal entered provider API key" aria-pressed="false">Reveal</button></span></label>`}
        ${provider.type === "ollama" ? `<label data-tooltip="Time allowed for model loading and the complete response, including streaming. Leave blank for 5 minutes. Client cancellation still applies.">Request Timeout<input data-provider="${index}" data-key="request_timeout" value="${value(provider,"request_timeout")}" placeholder="5m" /></label>` : ""}
      </div>
      <details class="advanced-provider"><summary>Advanced Provider Options</summary><div class="grid three">${provider.type==="ollama"?`<label>Remote API URL<input data-provider="${index}" data-key="api_url" value="${value(provider,"api_url")}" placeholder="http://server:11434" /></label>`:""}
        <label data-tooltip="An existing encrypted provider credential reference. Gateway client API keys cannot be used here.">Stored API Credential<input data-provider="${index}" data-key="credential_id" value="${escapeHTML(String((provider.bearer_token       )?.credential || provider.credential_id || ""))}" placeholder="Credential ID" /></label>
        <label data-tooltip="An existing encrypted management credential used for quota lookup. This is separate from gateway client API keys.">Management Credential<input data-provider="${index}" data-key="management_id" value="${escapeHTML(String((provider.management_token       )?.credential || provider.management_id || ""))}" placeholder="Credential ID" /></label>
        <label data-tooltip="Optional tracking integration for a custom OpenAI-compatible provider, such as deepseek or opencode-go.">Tracking Provider<input data-provider="${index}" data-key="tracking_provider" value="${value(provider,"tracking_provider")}" /></label>
        <label data-tooltip="Optional separate credential for billing or subscription usage. Saving encrypts it; leave blank to retain it.">Tracking Token<input type="password" autocomplete="new-password" data-provider="${index}" data-key="tracking_token" placeholder="${(provider.tracking       )?.token ? "Configured · leave blank to keep" : "Optional billing token"}" /></label>
        <label data-tooltip="An existing encrypted billing credential. It is used only by tracking requests.">Tracking Credential<input data-provider="${index}" data-key="tracking_credential_id" value="${value(provider,"tracking_credential_id")}" /></label>
        <label data-tooltip="xAI team identifier for the Management API.">Billing Team ID<input data-provider="${index}" data-key="tracking_team_id" value="${value(provider,"tracking_team_id")}" /></label>
        <label data-tooltip="Choose subscription for Anthropic OAuth usage or an included coding plan; API uses billing admin reports.">Tracking Mode<select data-provider="${index}" data-key="tracking_mode"><option value="" ${!provider.tracking_mode?"selected":""}>Provider Default</option><option value="api" ${provider.tracking_mode==="api"?"selected":""}>API</option><option value="subscription" ${provider.tracking_mode==="subscription"?"selected":""}>Subscription</option></select></label>
        <label data-tooltip="Optional billing origin override. A separate tracking token is required when changing the origin.">Tracking Base URL<input data-provider="${index}" data-key="tracking_base_url" value="${value(provider,"tracking_base_url")}" /></label>
        <label data-tooltip="Optional exact usage endpoint for a private billing API. A separate tracking token is required for a different origin.">Tracking Usage URL<input data-provider="${index}" data-key="tracking_usage_url" value="${value(provider,"tracking_usage_url")}" /></label>
        <label data-tooltip="The portal URL used for subscription or account management. It is relevant to providers with a separate portal.">Portal URL<input data-provider="${index}" data-key="portal_url" value="${value(provider,"portal_url")}" placeholder="https://portal.example.com" /></label>
        <label data-tooltip="The OAuth client identifier used by a provider portal. Leave empty to use the provider default.">Client ID<input data-provider="${index}" data-key="client_id" value="${value(provider,"client_id")}" /></label>
        <label data-tooltip="A static fallback model list used when discovery is disabled or unavailable. Separate model IDs with commas.">Models (Comma Separated)<input data-provider="${index}" data-key="models" value="${value(provider,"models")}" placeholder="model-a, model-b" /></label>
        <label data-tooltip="The transport mode for provider-specific integrations. Leave empty for the provider default.">Transport<input data-provider="${index}" data-key="transport" value="${value(provider,"transport")}" placeholder="direct or provider" /></label>
        <label data-tooltip="Optional organization identifier sent to providers that support organization scoping.">Organization<input data-provider="${index}" data-key="organization" value="${value(provider,"organization")}" /></label>
        <label data-tooltip="Optional project identifier sent to providers that support project scoping.">Project<input data-provider="${index}" data-key="project" value="${value(provider,"project")}" /></label>
        <label data-tooltip="JSON map of custom header names to secret references. Headers are added to upstream requests.">Headers JSON<input data-provider="${index}" data-key="headers" value='${value(provider,"headers")}' placeholder='{"X-Header":{"env":"ENV_VAR"}}' /></label>
        <label data-tooltip="JSON map of public model IDs to upstream model IDs. Use this to expose friendly aliases.">Model Rewrites JSON<input data-provider="${index}" data-key="rewrites" value='${value(provider,"rewrites")}' placeholder='{"public":"upstream"}' /></label>
        <label data-tooltip="A custom CA bundle for TLS verification. Leave empty to use the system trust store.">CA File<input data-provider="${index}" data-key="tls_ca" value="${value(provider,"tls_ca")}" /></label>
        <label data-tooltip="Overrides the TLS server name used during certificate verification. Leave empty for the URL hostname.">TLS Server Name<input data-provider="${index}" data-key="tls_server" value="${value(provider,"tls_server")}" /></label>
        <label data-tooltip="Maximum time allowed to establish an upstream connection. Use a Go duration such as 5s.">Connect Timeout<input data-provider="${index}" data-key="connect_timeout" value="${value(provider,"connect_timeout")}" placeholder="5s" /></label>
        ${provider.type === "ollama" ? "" : `<label data-tooltip="Maximum time allowed for an upstream request. Use a Go duration such as 60s.">Request Timeout<input data-provider="${index}" data-key="request_timeout" value="${value(provider,"request_timeout")}" placeholder="60s" /></label>`}
        <label data-tooltip="Maximum time allowed while discovering the provider model catalog. Use a Go duration such as 10s.">Discovery Timeout<input data-provider="${index}" data-key="discovery_timeout" value="${value(provider,"discovery_timeout")}" placeholder="10s" /></label>
        <label class="checkbox" data-tooltip="When enabled, Torii queries the provider for its available models. Disable this only when you provide a static model list."><input data-provider="${index}" data-key="discover" type="checkbox" ${checked(provider,"discover")} /> Discover Models</label>
      </div></details>`}
      <div class="dialog-actions"><button class="button quiet" type="button" data-cancel-provider="${index}">Cancel</button><button class="button primary" type="button" data-confirm-provider="${index}">Save Provider</button></div></dialog>
    </article>`).join("");
  providerList.querySelectorAll                   ("[data-provider-models]").forEach(button=>button.addEventListener("click",()=>{modelProviderFilter=String(providers[Number(button.dataset.providerModels)].id||"");modelSearch="";modelStatusFilter="";modelPage=0;location.hash="#models";}));
  providerList.querySelectorAll                   ("[data-edit]").forEach(button=>button.addEventListener("click",()=>{openProviderEditor(Number(button.dataset.edit));}));
  providerList.querySelectorAll                   ("[data-confirm-provider]").forEach(button=>button.addEventListener("click",async()=>{const dialog=button.closest("dialog") ;const index=editingProvider;if(index===null||saveRunning)return;const status=dialog.querySelector             ("[data-editor-status]") ;button.disabled=true;status.textContent="Checking connection and discovering models…";try{await testProvider(index);const result=providerStatuses.get(providers[index]);if(result?.state!=="success"){status.textContent=result?.message||"Connection failed. Check the settings and try again.";return;}status.textContent="Connection verified · saving provider…";editingProvider=null;markDirty();if(await saveConfig()){editingOriginal=null;dialog.close();}else{editingProvider=index;status.textContent=saveState.textContent||"Save failed. Your draft is preserved.";}}finally{button.disabled=false;}}));
  providerList.querySelectorAll                   ("[data-cancel-provider]").forEach(button=>button.addEventListener("click",cancelProviderEditor));
  providerList.querySelectorAll                   ("[data-provider-dialog]").forEach(dialog=>{
    dialog.addEventListener("cancel",event=>{event.preventDefault();cancelProviderEditor();});
    dialog.addEventListener("keydown",event=>{if(event.key!=="Tab")return;const controls=Array.from(dialog.querySelectorAll             ('button:not([disabled]),input:not([disabled]),select:not([disabled]),textarea:not([disabled]),summary,[tabindex]:not([tabindex="-1"])')).filter(node=>node.getClientRects().length>0);const first=controls[0],last=controls.at(-1);if(event.shiftKey&&document.activeElement===first){event.preventDefault();last?.focus();}else if(!event.shiftKey&&document.activeElement===last){event.preventDefault();first?.focus();}});
  });
  providerList.querySelectorAll                   ("[data-remove]").forEach(button => button.addEventListener("click", () => { providers.splice(Number(button.dataset.remove), 1); renderProviders(); markDirty();void saveConfig(); }));
  providerList.querySelectorAll                   ("[data-detect-ollama]").forEach(button=>button.addEventListener("click",async()=>{button.disabled=true;const provider=providers[Number(button.dataset.detectOllama)];try{await detectOllama(provider);await testProvider(providers.indexOf(provider));}catch(error){setProviderStatus(provider,{state:"failure",message:error instanceof Error?error.message:"Ollama detection failed"});}finally{button.disabled=false;}}));
  providerList.querySelectorAll                   ("[data-test]").forEach(button => button.addEventListener("click", () => { void testProvider(Number(button.dataset.test)); }));
  providerList.querySelectorAll                   ("[data-login]").forEach(button => button.addEventListener("click", () => { void loginProvider(Number(button.dataset.login), button); }));
  providerList.querySelectorAll                                      ("[data-provider]").forEach(control => {control.setAttribute("aria-label",({id:"Provider ID",type:"Provider Type",api_url:"API URL",api_key:"API Key",enabled:"Provider Enabled",treat_models_as_free:"Treat Models As Free",discover:"Discover Models"}                         )[control.dataset.key ]||control.dataset.key .replace(/_/g," "));control.addEventListener("input", updateProvider);});
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
  renderOverview();
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
  if(key==="tracking_token"&&control.value)providers[index].tracking_credential_id="";
  if(key==="tracking_credential_id"&&control.value)providers[index].tracking_token="";
  if (key === "management_env") delete providers[index].management_token;
  if(key==="ollama_port") providers[index].api_url="";
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
      providers[index].models = "gpt-5.3-codex";
    }
    const defaultURL = knownProviderURLs[control.value];
    if (defaultURL && (previousType !== control.value || !providers[index].api_url)) providers[index].api_url = defaultURL;
    if(previousType!==control.value){delete providers[index].tracking;Object.keys(providers[index]).filter(k=>k.startsWith("tracking_")).forEach(k=>delete providers[index][k]);}
    if(control.value==="ollama") { providers[index].api_url=""; providers[index].ollama_port=""; }
    if (classifierProvider(providers[index])) {
      if (control.value === "jev_compatible") providers[index].api_url = "";
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
    if(editingProvider===index)providerList.querySelector                   (`[data-provider="${index}"][data-key="type"]`)?.focus?.({preventScroll:true});
    void testProvider(index);
  }
  if(key==="enabled")renderSmartRouterScope();
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
  consoleTimezone=config.server?.timezone||"";
  for(const name of ["prompt_logging","jev_decision_logging"])(form.elements.namedItem(`activity.${name}`)                    ).checked=Boolean(config.activity?.[name]);
  (form.elements.namedItem("activity.retention_days")                    ).value=String(config.activity?.retention_days||7);
  (form.elements.namedItem("server.timezone")                    ).value=consoleTimezone;
  const providerCatalogChanged=JSON.stringify(loadedConfiguration.providers)!==JSON.stringify(config.providers);
  loadedConfiguration=structuredClone(config);modelPricingDraft=structuredClone(config.model_pricing||{});modelEnabledDraft={...config.model_enabled};budgetsDraft=structuredClone(config.budgets||{});renderBudgetEditor();saveConflict=false;
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
    type: provider.type === "jev" ? "typesafe" : provider.type,
    api_key: "",
    api_key_env: provider.bearer_token?.env || "",
    api_key_file: provider.bearer_token?.file || "",
    management_env: provider.management_token?.env || "",
    tracking_credential_id: provider.tracking?.token?.credential || "",
    tracking_provider: provider.tracking?.provider || "",
    tracking_team_id: provider.tracking?.team_id || "",
    tracking_base_url: provider.tracking?.base_url || "",
    tracking_usage_url: provider.tracking?.usage_url || "",
    tracking_mode: provider.tracking?.mode || "",
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
  providers.forEach(provider => providerOriginalIDs.set(provider, String(provider.id)));
  // Move legacy direct Jev connections into provider drafts, retaining secret
  // references (including key files) rather than dropping them on the next save.
  if (classifier.type === "jev" && classifier.transport !== "provider") {
    const type = new URL(classifier.base_url).hostname === "api.typesafe.ai" ? "typesafe" : "jev_compatible";
    const id = type === "typesafe" ? knownProviderID("typesafe", -1) : `classifier-${providers.length + 1}`;
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
    if (name === "activity.max_bytes") return;
    if (raw && typeof raw === "object" && !Array.isArray(raw)) fill(raw, name);
    else {
      const control = form.elements.namedItem(name)                                                                     ;
      if (control) control.type === "checkbox" ? control.checked = Boolean(raw) : control.value = String(raw);
    }
  });
  fill(config);
  (form.elements.namedItem("activity.max_bytes")                    ).value=String((config.activity?.max_bytes||104857600)/1048576);
  renderActivity();
  if(overviewRecent)renderOverview();
  if(physicalModels.length)renderModelsPage();
  const allowances = config.router?.quota?.allowances || {};
  (form.elements.namedItem("router.quota.allowances")                       ).value = Object.entries(allowances).map(([key, raw]) => `${key} = ${raw}`).join("\n");
  const quality = config.router?.selection?.minimum_quality;
  if (quality) (form.elements.namedItem("router.selection.minimum_quality")                    ).value = JSON.stringify(quality);
  smartRouterDrafts={smart:structuredClone(config.router||{})};
  for(const profile of config.smart_routers||[])smartRouterDrafts[profile.model_name]=structuredClone(profile);
  if(!smartRouterDrafts[selectedSmartRouter])selectedSmartRouter="smart";
  renderSmartRouterEditor();
  if(providerCatalogChanged && (overviewModels!==null||location.hash==="#routing") && consoleSession)void loadSmartRouterModels();
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
    navigate();
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

function syncConsoleNavigation(current       )     {
 let activeLink                            ;
 document.querySelectorAll                   (".nav-link").forEach(link=>{const active=link.getAttribute("href")===`#${current}`;link.classList.toggle("active",active);if(active){link.setAttribute("aria-current","page");activeLink=link;}else link.removeAttribute("aria-current");});
 if(!activeLink||!window.matchMedia?.("(max-width: 760px)").matches)return;
 const nav=document.querySelector             ("#nav");if(!nav)return;
 const bounds=nav.getBoundingClientRect();const linkBounds=activeLink.getBoundingClientRect();
 // Move only the horizontal navigation strip; never scroll the workspace.
 if(linkBounds.left<bounds.left||linkBounds.right>bounds.right)nav.scrollLeft+=linkBounds.left-bounds.left-(bounds.width-linkBounds.width)/2;
}

function navigate()       {
  const current = location.hash.replace(/^#/, "") || "overview";
  document.querySelectorAll             (".page").forEach(page => {
    const active = page.id === current;
    page.classList.toggle("active", active);
    page.hidden = !active;
    page.setAttribute("aria-hidden", String(!active));
  });
  const title=document.querySelector("#current-page-title");if(title)title.textContent=({overview:"Overview",providers:"Providers",models:"Models",routing:"Smart Routing",intelligence:"Benchmarks",credentials:"API Keys",activity:"Activity","provider-balances":"Provider Balances",state:"Usage & Quota",server:"Server"}                         )[current]||"Overview";
  syncConsoleNavigation(current);
  if(current==="overview"){renderOverview();if(configurationLoaded)void loadOverview();}
  if(current==="activity")void startActivity();else if(activityStream)stopActivity();
  if(current==="provider-balances" && configurationLoaded)void loadProviderBalancesPage();
  if(current==="server" && configurationLoaded)void loadAdminAccount();
  if(current==="state" && configurationLoaded)void loadUsagePage();
  if(current==="models"){if(configurationLoaded)void loadModelsPage();else document.querySelector("#models-list") .innerHTML=modelLoadingMarkup;}
  if (current === "routing") {updateClassifierOptions();renderSmartRouterScope();if(configurationLoaded)void loadSmartRouterModels();}
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
    if(key.startsWith("tracking_"))return;
    if (key === "base_url" || key === "api_path" || key === "discover_models") return;
    if (["api_key", "api_key_env", "api_key_file", "bearer_token","credential_id","management_id"].includes(key)) return;
    if (key === "discover") { output.discover_models = Boolean(raw); return; }
    if (String(raw).trim() === "") return;
    const map                         = { management_env: "management_token", tls_ca: "tls.ca_file", tls_server: "tls.server_name" };
    const mapped = map[key] || key;
    if (key === "api_url") {
      const url = new URL(String(raw));
      if (subscriptionProvider(provider) || provider.type === "nous" || provider.type === "ollama" || provider.type === "anthropic") {
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
  const tracking    ={...(provider.tracking       ||{})};
  for(const field of ["provider","base_url","usage_url","team_id","mode"]){const key="tracking_"+field;if(provider[key]!==undefined){if(provider[key])tracking[field]=provider[key];else delete tracking[field];}}
  if(provider.tracking_token)tracking.token={value:provider.tracking_token};else if(provider.tracking_credential_id)tracking.token={credential:provider.tracking_credential_id};
  if(Object.keys(tracking).length)output.tracking=tracking;else delete output.tracking;
  if (provider.management_id) output.management_token={credential:provider.management_id};
  return output;
}
function readConfig()                      {
  const config                      = structuredClone(loadedConfiguration);
 config.server={...config.server};config.state={...config.state};config.router=structuredClone(smartRouterDrafts[selectedSmartRouter]||config.router||{});config.router={...config.router,classifier:{},sessions:{...config.router?.sessions},quota:{...config.router?.quota},selection:{...config.router?.selection}};
  config.activity={...config.activity};
  new FormData(form).forEach((raw, name) => { if (name === "router.quota.allowances") return; if (name === "router.allow_paid_models") setNested(config, name, true); else if (name === "router.selection.minimum_quality" || name==="router.intelligence_weights") { try { setNested(config, name, JSON.parse(String(raw))); } catch { /* keep invalid drafts editable */ } } else if (String(raw).trim() !== "") setNested(config, name, raw); else if(name.startsWith("router.")){const parts=name.split(".");let parent    =config;for(const part of parts.slice(0,-1))parent=parent?.[part];if(parent)delete parent[parts[parts.length-1]];} });
  config.server.timezone=String((form.elements.namedItem("server.timezone")                    )?.value||"").trim();
  if(config.server.timezone){try{new Intl.DateTimeFormat("en-US",{timeZone:config.server.timezone});}catch{throw new Error("Enter a valid IANA timezone, such as America/New_York or UTC");}}
  for(const name of ["prompt_logging","jev_decision_logging"])config.activity[name]=Boolean((form.elements.namedItem(`activity.${name}`)                    )?.checked);
  for(const name of ["max_bytes","retention_days"]){const raw=String((form.elements.namedItem(`activity.${name}`)                    )?.value||"").trim();const value=raw?Number(raw)*(name==="max_bytes"?1048576:1):({max_bytes:104857600,retention_days:7}                         )[name];if(!Number.isSafeInteger(value)||value<({max_bytes:8192,retention_days:1}                         )[name]||name==="retention_days"&&value>3650)throw new Error("Enter valid Activity size and retention limits");config.activity[name]=value;}
  config.model_pricing=structuredClone(modelPricingDraft);config.model_enabled={...modelEnabledDraft};config.budgets=structuredClone(budgetsDraft);
 config.providers = providers.flatMap((p,index)=>editingProvider===index?(editingOriginal?[readProviderConfig(editingOriginal)]:[]):[readProviderConfig(p)]);
  // Unchecked checkboxes are absent from FormData. Emit the policy explicitly
  // so saving "false" cannot restore the backend's default of allowing paid models.
  config.router.allow_paid_models = Boolean((form.elements.namedItem("router.allow_paid_models")                    )?.checked);
 config.router.allow_free_models = (form.elements.namedItem("router.allow_free_models")                    )?.checked !== false;
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
  if (Object.keys(allowances).length) config.router.quota.allowances = allowances;else delete config.router.quota.allowances;
  config.intelligence = readBenchmarkConfiguration();
  if(!String((form.elements.namedItem("router.intelligence_weights")                    )?.value||"").trim())delete config.router.intelligence_weights;
  // Router names change only through Save, which validates uniqueness and rekeys the draft.
  if(selectedSmartRouter)config.router.model_name=smartRouterDrafts[selectedSmartRouter]?.model_name||selectedSmartRouter;
  smartRouterDrafts[selectedSmartRouter]=structuredClone(config.router);
  config.router=structuredClone(smartRouterDrafts.smart||{});
  config.smart_routers=Object.entries(smartRouterDrafts).filter(([name])=>name!=="smart"&&name!=="").map(([,draft])=>structuredClone(draft));
  providers.forEach((provider,index) => {
    const original=providerOriginalIDs.get(provider), next=String(provider.id);
    if(original && original!==next && editingProvider!==index)renameProviderReferences(config,original,next);
  });
  return config;
}
function renameProviderReferences(config                   ,original       ,next       )      {
  const model=(id       )=>id.startsWith(original+"/")?next+id.slice(original.length):id;
  const keys=(map                             ,providerKeys=false)=>map?Object.fromEntries(Object.entries(map).map(([id,value])=>[providerKeys&&id===original?next:model(id),value])):map;
  config.model_enabled=keys(config.model_enabled);config.model_pricing=keys(config.model_pricing);
  for(const router of [config.router,...(config.smart_routers||[])]) {
    if(!router)continue;
    if(router.providers)router.providers=router.providers.map((id       )=>id===original?next:id);
    if(router.models)router.models=router.models.map(model);
    if(router.classifier?.provider===original)router.classifier.provider=next;
    if(router.classifier?.model && !(router.classifier.type==="jev" && router.classifier.transport==="provider"))router.classifier.model=model(router.classifier.model);
    if(router.quota?.allowances)router.quota.allowances=keys(router.quota.allowances,true);
    for(const descriptor of router.catalog_overrides||[]) {
      descriptor.id=model(descriptor.id);
      if(descriptor.provider_id===original)descriptor.provider_id=next;
    }
  }
}

function updateBenchmarkControls()       {
  const enabled = document.querySelector                  ("#intelligence-enabled") .checked;
  document.querySelector                   ("#refresh-benchmarks") .disabled = !benchmarksEnabledSaved || !enabled || benchmarkDirty || benchmarkRefreshRunning;
  document.querySelector("#refresh-benchmarks") .textContent = benchmarkRefreshRunning ? "Refreshing…" : "Refresh Benchmarks";
  document.querySelector                   ("#benchmark-configure") .disabled=!enabled||!consoleSession;
  for(const id of ["benchmark-upload","benchmark-edit-data","benchmark-display-fields"])document.querySelector                   (`#${id}`) .disabled=!benchmarksEnabledSaved||!enabled||!consoleSession;
  if(!enabled||!consoleSession)for(const id of ["benchmark-configuration","benchmark-upload-dialog","benchmark-fields-dialog"])document.querySelector                   (`#${id}`)?.close?.();
  document.querySelector                ("#benchmark-data") .hidden = !benchmarksEnabledSaved;
  document.querySelector             ("#benchmark-status") .hidden=!enabled;
  const sourceNotice=document.querySelector             ("#benchmark-source-notice");if(sourceNotice)sourceNotice.hidden=enabled&&benchmarksEnabledSaved&&!benchmarkDirty&&Object.keys(benchmarkModels).length>0;
  document.querySelector("#benchmark-enable-notice") .textContent = enabled !== benchmarksEnabledSaved || benchmarkDirty
    ? "Your benchmark setting is saving. Wait for Saved before refreshing."
    : enabled ? "Choose the sources you trust in Benchmark sources, then refresh to import published results. Source changes save automatically."
    : "Published benchmarks are optional. Enable intelligence to import model evidence from your selected sources.";
}

async function testProvider(index        )                {
  const provider = providers[index];
  if (!provider) return;
  const pending                 = { state: "pending", message: "Checking connection and discovering models…" };
  setProviderStatus(provider, pending);
  try {
    if(provider.type==="ollama" && !provider.api_url) { await detectOllama(provider);setProviderStatus(provider,pending); }
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
    {
      const ids = (body.models || []).filter((model                     ) => classifierProvider(provider) || model.decision === true).map((model             ) => model.id);
      if (ids.some((id         ) => typeof id !== "string" || !id.startsWith(`${provider.id}/`))) throw new Error("Invalid classifier model IDs");
      classifierCatalogs.set(provider, ids);
    }
    setProviderStatus(provider, {
      state: count > 0 ? "success" : "failure",
      message: `${connection} · ${count > 0 ? "Working" : "Not working"} · ${count} model${count === 1 ? "" : "s"} discovered${count === 0 ? " (no models available)" : ""}${configuration.discover_models === false ? " · Discovery disabled: saved configuration uses the static model list" : ""}`,
    });
  } catch (error) {
    if (providerStatuses.get(provider) !== pending || !providers.includes(provider)) return;
    setProviderStatus(provider, { state: "failure", message: `Not working · 0 models discovered · ${error instanceof Error ? error.message : "connection failed"}` });
  }
}

async function loginProvider(index        , button                   )                {
  const provider = providers[index];
  // Reserve the tab during the click, before any asynchronous request, so
  // browsers do not treat the later authorization URL as an unsolicited popup.
  const loginTab = window.open("about:blank", "_blank");
  if (loginTab) loginTab.opener = null;
  button.disabled = true;
  setProviderStatus(provider, { state: "pending", message: "Starting login…" });
  try {
    const configuration = readProviderConfig(provider);
    const response = await gatewayFetch("/gateway/v1/providers/login", { method: "POST", headers: { "Content-Type": "application/json", ...await authProvider.headers() }, body: JSON.stringify({ provider: configuration, login_method: "device" }) });
    const body = await response.json();
    if (!response.ok) throw new Error(body.error || "login could not start");
    await pollProviderLogin(provider, button, loginTab, body);
  } catch (error) {
    loginTab?.close();
    setProviderStatus(provider, { state: "failure", message: `Not working · 0 models discovered · ${error instanceof Error ? error.message : "login failed"}` });
  } finally {
    button.disabled = false;
  }
}
function renderLoginInstructions(provider                                  , body                     )       {
  const message = body.message || "Waiting for login…";
  const index = providers.indexOf(provider);
  const nodes = [`[data-result="${index}"]`, `[data-editor-status="${index}"]`]
    .map(selector => providerList.querySelector             (selector)).filter((node)                      => node !== null);
  const instructions = JSON.stringify([message, body.verification_url, body.user_code]);
  // Preserve selection and copy feedback while polling unchanged instructions.
  if (body.user_code && providerStatuses.get(provider)?.state === "pending" && nodes.length && nodes.every(node => node.dataset.codexLogin === instructions && node.querySelector                  (".codex-login-code")?.value === body.user_code)) return;
  setProviderStatus(provider, { state: "pending", message });
  if (!body.verification_url) return;
  const url = new URL(body.verification_url);
  if (url.origin !== "https://auth.openai.com") throw new Error("Invalid OpenAI login URL");
  const codeControls = body.user_code ? `<div class="codex-code-controls"><label>One-Time Code<input class="codex-login-code" type="text" readonly autocomplete="off" spellcheck="false"></label><button class="codex-copy-code button secondary" type="button" aria-label="Copy one-time code">Copy</button></div><span class="codex-copy-status" role="status" aria-live="polite"></span>` : "";
  const content = `${escapeHTML(redactProviderKeys(message))}${codeControls} <a href="${escapeHTML(url.href)}" target="_blank" rel="noopener noreferrer">Open OpenAI Login</a>`;
  for (const node of nodes) {
    node.innerHTML = content;
    node.dataset.codexLogin = instructions;
    const input = node.querySelector                  (".codex-login-code");
    const copy = node.querySelector                   (".codex-copy-code");
    const feedback = node.querySelector             (".codex-copy-status");
    if (!input || !copy || !feedback || !body.user_code) continue;
    input.value = body.user_code;
    copy.addEventListener("click", async () => {
      try {
        await navigator.clipboard.writeText(input.value);
        copy.textContent = "Copied";
        feedback.textContent = "Code copied";
      } catch {
        copy.textContent = "Copy";
        input.focus();
        input.select();
        feedback.textContent = "Copy unavailable. Copy the selected code.";
      }
    });
  }
}
async function pollProviderLogin(provider                                  , button                   , loginTab               , initial                     )                {
  const providerID = String(provider.id);
  let body = initial;
  let openedURL = "";
  // Match OpenAI's 15-minute device authorization lifetime.
  for (let attempt = 0; attempt < 450; attempt += 1) {
    if (!providers.includes(provider) || String(provider.id) !== providerID) {
      if (!openedURL) loginTab?.close();
      return;
    }
    if (body.status === "success") {
      if (!openedURL) loginTab?.close();
      await testProvider(providers.indexOf(provider));
      return;
    }
    if (body.status === "failure") throw new Error(body.message || "login failed");
    renderLoginInstructions(provider, body);
    if (body.verification_url && !openedURL) {
      openedURL = body.verification_url;
      if (loginTab && !loginTab.closed) loginTab.location.replace(openedURL);
    }
    await new Promise(resolve => window.setTimeout(resolve, 2000));
    const response = await gatewayFetch(`/gateway/v1/providers/login?provider=${encodeURIComponent(providerID)}`, { headers: await authProvider.headers() });
    body = await response.json();
    if (!response.ok) throw new Error(body.message || "Could not check login status; click Log in to try again");
  }
  throw new Error("Login timed out; click Log in for a new code");
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
function isConsoleViewControl(target            )        {return !!target.closest?.("#admin-user") || ["smart-router-select","smart-router-new-name","smart-router-providers","smart-router-providers-available","smart-router-models","smart-router-models-available","benchmark-record-filter","benchmark-csv-file","budget-scope","classifier-question","classifier-question-option","model-search","model-provider-filter","model-status-filter","model-sort","model-page-size","model-page-size-top","model-pricing-csv","activity-search","activity-outcome","activity-page-size","usage-period","activity-view","benchmark-table-view","benchmark-table-filter","benchmark-page-size","benchmark-fields-available","benchmark-fields-enabled"].includes(target.id)||Boolean(target.dataset?.benchmarkCell);}
form.addEventListener("input",event=>{if(!isConsoleViewControl(event.target               ))markDirty();});
form.addEventListener("change",event=>{const target=event.target               ;if(!(selectedSmartRouter===""&&(target.closest("#routing")||(target                    ).name?.startsWith("router."))) && !isConsoleViewControl(target) && !target.closest("dialog") && !target.dataset.modelEnabled && !["classifier-question","classifier-question-option","budget-scope"].includes(target.id) && !target.id.startsWith("budget-")){void saveConfig();}});
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
document.querySelector                   ("#overview-period")?.addEventListener("change",event=>{overviewPeriod=(event.target                     ).value==="monthly"?"monthly":"daily";renderOverview();});
initializeBenchmarks(); renderProviders(); updateConsoleIdentity(); updateBenchmarkControls(); navigate(); void checkSetup();

document.querySelector("#console-login") .addEventListener("click",()=>location.assign("/login"));
document.querySelector("#console-logout") .addEventListener("click",async()=>{try{const response=await gatewayFetch("/gateway/v1/session",{method:"DELETE",headers:await authProvider.headers()});if(!response.ok)throw new Error();signOutConsole();location.assign("/login");}catch{toast("Sign-out failed. Retry to revoke the session.");}});
document.querySelector("#login-form") .addEventListener("submit",async event=>{
  event.preventDefault();const username=document.querySelector                  ("#login-username") .value;const input=document.querySelector                  ("#login-password") ;const password=input.value;input.value="";
  try{await createBrowserSession(username,password);const response=await gatewayFetch("/gateway/v1/config",{headers:await authProvider.headers()});if(!response.ok)throw new Error();const committed=await response.json();const revision=response.headers.get("ETag")||"";if(configurationLoaded){if(providerConfig!==JSON.stringify(loadedConfiguration.providers)){saveConflict=true;saveState.textContent="Settings changed while signed out. Your draft is retained; reload to resolve the conflict.";}}else applyConfig(committed,revision);signedInUsername=username;updateConsoleIdentity();document.querySelector("#login-error") .textContent="";document.querySelector                   ("#login-dialog") .close();if(!saveConflict)saveState.textContent="Signed in. Your draft is ready to save.";void testAllProviders();void loadCredentialMetadata();}
  catch{expireConsoleSession();document.querySelector("#login-error") .textContent="Sign-in failed. Check your administrator username and password, then try again in one minute if rate limited.";}
});
async function loadCredentialMetadata()              {
  const list=document.querySelector             ("#credential-list") ;
  const login=consoleSession;
  try{const response=await gatewayFetch("/gateway/v1/admin/api-keys",{headers:await authProvider.headers()});if(!response.ok)throw new Error();const body=await response.json();if(login!==consoleSession)return;list.innerHTML=(body.api_keys||[]).map((key    )=>`<div class="credential-card"><div class="credential-card-content"><h2 class="credential-card-name">${escapeHTML(key.name)}</h2><p class="credential-card-meta">Created <time title="${escapeHTML(key.created_at)}">${consoleDate(key.created_at)}</time></p></div><button type="button" class="action-icon-only button credential-card-delete danger" data-delete-key="${escapeHTML(key.id)}" aria-label="Delete API key ${escapeHTML(key.name)}" title="Delete"><svg class="action-icon" viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path d="M3 6h18M9 6V4h6v2M5 6l1 14h12l1-14M10 10v6M14 10v6"/></svg></button></div>`).join("") || "No gateway API keys have been created. Create an API key for your first application.";
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
document.querySelector("#setup-dialog") .addEventListener("cancel",event=>event.preventDefault());

function showNewAPIKey(key       ,warning        )     {
  if(!key){toast("Your existing API key was imported and preserved. Generate a replacement on the Server page if needed.");return;}
  document.querySelector                     ("#new-api-key") .value=key;
  document.querySelector("#new-key-warning") .textContent=warning||"This API key is displayed only once. Save it securely before closing.";
  document.querySelector                   ("#new-key-dialog") .showModal();
}
document.querySelector("#setup-form") .addEventListener("submit",async event=>{
  event.preventDefault();const field=(id       )=>document.querySelector                  (id) ;
  const email=field("#setup-email").value,username=field("#setup-username").value,password=field("#setup-password").value,code=field("#setup-code").value;
  if(password!==field("#setup-confirm").value){document.querySelector("#setup-error") .textContent="Passwords do not match.";return;}
  field("#setup-password").value="";field("#setup-confirm").value="";field("#setup-code").value="";
  try{const response=await fetch("/gateway/v1/setup",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({username,email,password,setup_code:code})});const result=await response.json();if(!response.ok)throw new Error(result.error||"Setup failed");await createBrowserSession(username,password);document.querySelector                   ("#setup-dialog") .close();await loadServerConfig();void loadCredentialMetadata();showNewAPIKey(result.api_key,result.warning);}
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

async function detectOllama(provider                   )              {
 setProviderStatus(provider,{state:"pending",message:"Looking for Ollama on the gateway host…"});
 const response=await gatewayFetch("/gateway/v1/providers/ollama-detect",{method:"POST",headers:{"Content-Type":"application/json",...await authProvider.headers()},body:JSON.stringify({port:Number(provider.ollama_port||0)})});
 const result=await response.json();if(!response.ok)throw new Error(result.error||"Ollama detection failed");provider.api_url=result.base_url;provider.ollama_port=String(result.port);
 const index=providers.indexOf(provider);const input=providerList.querySelector                  (`[data-provider="${index}"][data-key="ollama_port"]`);if(input)input.value=provider.ollama_port;
}

let smartRouterModelsSequence=0;
async function loadSmartRouterModels()              {
 const sequence=++smartRouterModelsSequence,login=consoleSession,providerConfig=JSON.stringify(loadedConfiguration.providers);
 if(!login)return;
 const status=document.querySelector             ("#smart-router-model-status") ;
 status.textContent="Loading available models…";
 try{
  const response=await gatewayFetch("/gateway/v1/models",{headers:await authProvider.headers()});
  if(sequence!==smartRouterModelsSequence||login!==consoleSession||providerConfig!==JSON.stringify(loadedConfiguration.providers))return;
  if(!response.ok)throw new Error("Model discovery unavailable. Reopen Smart routing to retry.");
  const data=await response.json();
  if(sequence!==smartRouterModelsSequence||login!==consoleSession||providerConfig!==JSON.stringify(loadedConfiguration.providers))return;
  overviewModels=data.models||[];renderSmartRouterScope();
  status.textContent=overviewModels .length?"":"No models discovered from enabled providers.";
 }catch(error){if(sequence===smartRouterModelsSequence&&login===consoleSession&&providerConfig===JSON.stringify(loadedConfiguration.providers))status.textContent=error instanceof Error?error.message:"Model discovery unavailable. Reopen Smart routing to retry.";}
}

function renderSmartRouterScope()     {
 const draft=smartRouterDrafts[selectedSmartRouter]||{};
 const enabled         =draft.providers||[];
 const providerIDs=[...new Set([...providers.filter(p=>!classifierProvider(p)).map(p=>String(p.id)),...enabled])].sort();
 for(const [selector,ids] of [["#smart-router-providers-available",providerIDs.filter(id=>!enabled.includes(id))],["#smart-router-providers",[...enabled].sort()]]                       ){
  const control=document.querySelector                   (selector);if(control)control.innerHTML=ids.map(id=>`<option value="${escapeHTML(id)}">${escapeHTML(id)}</option>`).join("");
 }
 const chosen         =draft.models||[];
 const available=[...new Set((overviewModels||[]).filter(m=>providers.some(p=>p.id===m.provider_id&&p.enabled!==false&&!classifierProvider(p))&&m.endpoints?.systemone!=="supported"&&!chosen.includes(m.id)&&(!(draft.providers||[]).length||draft.providers.includes(m.provider_id))).map(m=>m.id))].sort();
 for(const [selector,ids] of [["#smart-router-models-available",available],["#smart-router-models",[...chosen].sort()]]                       ){
  const control=document.querySelector                   (selector);if(control)control.innerHTML=ids.map(id=>`<option value="${escapeHTML(id)}">${escapeHTML(id)}</option>`).join("");
 }
 const count=document.querySelector("#smart-router-model-count");if(count)count.textContent=`${chosen.length} / 250`;
}
async function transferSmartModels(enable        , single        )              {
 const control=document.querySelector                   (enable?"#smart-router-models-available":"#smart-router-models");
 const ids=single?[single]:Array.from(control?.selectedOptions||[]).map(option=>option.value);if(!ids.length)return;
 readConfig();const draft=smartRouterDrafts[selectedSmartRouter];const current         =draft.models||[];
 const next=enable?[...new Set([...current,...ids])]:current.filter(id=>!ids.includes(id));
 if(next.length>250){toast("Smart routers support at most 250 enabled models.");return;}
 draft.models=next;renderSmartRouterScope();markDirty();if(selectedSmartRouter!=="")await saveConfig();
}
function renderSmartRouterEditor()     {
 const selector=document.querySelector                   ("#smart-router-select");if(selector){selector.innerHTML=Object.keys(smartRouterDrafts).map(name=>`<option value="${escapeHTML(name)}">${escapeHTML(name||"New router")}${name==="smart"?" · default":""}</option>`).join("");selector.value=selectedSmartRouter;}
 const draft=smartRouterDrafts[selectedSmartRouter]||{};
 const fields=["model_name","allow_paid_models","allow_free_models","sessions.on_model_failure","sessions.inactivity_timeout","quota.cache_ttl","quota.allowances","selection.context_utilization_limit","selection.default_expected_output_tokens","selection.location_preference","selection.minimum_quality","intelligence_weights","classifier.on_failure","classifier.timeout","classifier.minimum_confidence"];
 for(const field of fields){const control=form.elements.namedItem(`router.${field}`)                         ;if(!control)continue;let value    =draft;for(const part of field.split("."))value=value?.[part];
 if(field.startsWith("allow_"))control.checked=value!==false;else if(field==="model_name"){control.value=draft.model_name||selectedSmartRouter;control.readOnly=selectedSmartRouter==="smart";}else if(field==="quota.allowances")control.value=Object.entries(value||{}).map(([id,v])=>`${id} = ${v}`).join("\n");else if(field==="selection.minimum_quality" || field==="intelligence_weights")control.value=value?JSON.stringify(value):"";else control.value=value===undefined?"":String(value);
 }
 const c=draft.classifier||{};classifierQuestionOverrides=structuredClone(c.questions||{});classifierNoulThresholds={...(c.noul_thresholds||{})};
 requestedClassifier=c.type==="disabled"?"disabled":c.type==="jev"&&c.transport==="provider"?`${c.provider}/${c.model}`:c.model||"";
 legacyClassifier=c.type&&!["jev","disabled"].includes(c.type)?structuredClone(c):null;
 renderClassifierQuestion();updateClassifierOptions();renderSmartRouterScope();
 const remove=document.querySelector                   ("#remove-smart-router");if(remove)remove.disabled=selectedSmartRouter==="smart"||selectedSmartRouter==="";
}
document.querySelector                   ("#smart-router-select")?.addEventListener("change",event=>{readConfig();selectedSmartRouter=(event.target                     ).value;renderSmartRouterEditor();});
document.querySelector("#add-smart-router")?.addEventListener("click",()=>{
 readConfig();smartRouterDrafts[""]={};selectedSmartRouter="";renderSmartRouterEditor();
 (form.elements.namedItem("router.model_name")                    )?.focus?.();
});
async function saveSmartRouter()              {
 if(saveRunning)return;
 const name=String((form.elements.namedItem("router.model_name")                    ).value).trim();
 const oldName=selectedSmartRouter;
 if(!/^[a-z0-9][a-z0-9._-]*$/.test(name)||(name!==oldName&&(name==="smart"||smartRouterDrafts[name]))){toast("Enter a unique router name using lowercase letters, numbers, dots, underscores or hyphens.");return;}
 readConfig();smartRouterDrafts[oldName].model_name=name;
 if(name!==oldName){smartRouterDrafts[name]=smartRouterDrafts[oldName];delete smartRouterDrafts[oldName];selectedSmartRouter=name;}
 renderSmartRouterEditor();markDirty();await saveConfig();
}
document.querySelector("#save-smart-router")?.addEventListener("click",()=>void saveSmartRouter());
document.querySelector("#remove-smart-router")?.addEventListener("click",async()=>{if(selectedSmartRouter==="smart"||selectedSmartRouter==="")return;delete smartRouterDrafts[selectedSmartRouter];selectedSmartRouter="smart";renderSmartRouterEditor();markDirty();await saveConfig();});
async function transferSmartProviders(enable        ,single        )              {
 const control=document.querySelector                   (enable?"#smart-router-providers-available":"#smart-router-providers");
 const ids=single?[single]:Array.from(control?.selectedOptions||[]).map(option=>option.value);if(!ids.length)return;
 readConfig();const draft=smartRouterDrafts[selectedSmartRouter];const current         =draft.providers||[];
 draft.providers=enable?[...new Set([...current,...ids])]:current.filter(id=>!ids.includes(id));
 // Explicit model pools must remain inside the selected provider pool.
 if(draft.providers.length)draft.models=(draft.models||[]).filter((id       )=>draft.providers.includes(id.split("/")[0]));
 renderSmartRouterScope();markDirty();if(selectedSmartRouter!=="")await saveConfig();
}
document.querySelector("#smart-router-provider-add")?.addEventListener("click",()=>void transferSmartProviders(true));
document.querySelector("#smart-router-provider-remove")?.addEventListener("click",()=>void transferSmartProviders(false));
for(const [id,enable] of [["smart-router-providers-available",true],["smart-router-providers",false]]                      ){
 document.querySelector(`#${id}`)?.addEventListener("dblclick",event=>{const target=event.target                     ;return transferSmartProviders(enable,target.tagName==="OPTION"?target.value:undefined);});
}

document.querySelector("#smart-router-model-add")?.addEventListener("click",()=>void transferSmartModels(true));
document.querySelector("#smart-router-model-remove")?.addEventListener("click",()=>void transferSmartModels(false));
for(const [id,enable] of [["smart-router-models-available",true],["smart-router-models",false]]                      ){
 document.querySelector(id.startsWith("#")?id:`#${id}`)?.addEventListener("dblclick",event=>{const target=event.target                     ;void transferSmartModels(enable,target.tagName==="OPTION"?target.value:undefined);});
}

async function loadAdminAccount()              {
 const login=consoleSession;
 try{
  const response=await gatewayFetch("/gateway/v1/admin/account",{headers:await authProvider.headers()});
  if(!response.ok)throw new Error("Unable to load admin user.");const account=await response.json();if(login!==consoleSession)return;
  document.querySelector                  ("#admin-username") .value=account.username||"";
  document.querySelector                  ("#admin-email") .value=account.email||"";
 }catch{document.querySelector("#admin-account-status") .textContent="Unable to load admin user. Reopen the Server page to retry.";}
}
document.querySelector("#save-admin-user") .addEventListener("click",async()=>{
 const field=(id       )=>document.querySelector                  (id) ;
 const status=document.querySelector("#admin-account-status") ;
 const username=field("#admin-username").value.trim(),email=field("#admin-email").value.trim(),password=field("#admin-password").value,current_password=field("#admin-current-password").value;
 if(!username||!current_password){status.textContent="Enter your username and current password.";return;}
 for(const id of ["#admin-username","#admin-email","#admin-password"]){if(!field(id).reportValidity())return;}
 if(password!==field("#admin-confirm").value){status.textContent="New passwords do not match.";return;}
 const button=document.querySelector                   ("#save-admin-user") ;button.disabled=true;
 try{
  const response=await gatewayFetch("/gateway/v1/admin/account",{method:"PUT",headers:{...await authProvider.headers(),"Content-Type":"application/json"},body:JSON.stringify({username,email,password,current_password})});
  const result=await response.json();if(!response.ok)throw new Error(result.error||"Account update failed.");
  signOutConsole();location.assign("/login?updated=1");
 }catch(error){status.textContent=error instanceof Error?error.message:"Account update failed.";}
 finally{for(const id of ["#admin-password","#admin-confirm","#admin-current-password"])field(id).value="";button.disabled=false;}
});

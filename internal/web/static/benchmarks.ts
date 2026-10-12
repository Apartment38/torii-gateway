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
let benchmarkPageSize = matchMedia("(max-width: 760px)").matches?5:25;
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
  clearBenchmarkRecords();closeBenchmarkEvidence();
  benchmarkEditing=false;benchmarkInlineDrafts={};benchmarkInlineSaving=false;benchmarkInlineSaveSequence++;benchmarkSummaryEnabled=["model","overall","capabilities","sources","freshness","evidence"];benchmarkMeasurementEnabled=benchmarkMeasurementFields.slice(0,8).map(field=>field.key);
  for(const id of ["benchmark-configuration","benchmark-upload-dialog","benchmark-fields-dialog"])document.querySelector<HTMLDialogElement>(`#${id}`)?.close?.();
  document.querySelector("#benchmark-edit-data")!.textContent="Edit data";document.querySelector("#benchmark-edit-data")!.setAttribute("aria-pressed","false");
  document.querySelector("#benchmark-inline-status")!.textContent="";inspectedBenchmarkModel="";document.querySelector("#benchmark-evidence-content")!.textContent="";
  benchmarkAbort?.abort(); benchmarkAbort = null; benchmarkReloadSequence++;
  benchmarkConfiguration = {}; benchmarkSources = []; benchmarkSourceStatus = {}; benchmarkSourceActivity = {}; benchmarkModels = {};
  benchmarkPage = 0; benchmarkFilter = ""; benchmarkTableView = "models";
  benchmarkPageSize = matchMedia("(max-width: 760px)").matches?5:25;
  benchmarkDirty = false; benchmarkRefreshRunning = false; benchmarkProgressLog = [];
  benchmarkProgressCounts = {total:0,completed:0,failed:0};
  document.querySelector<HTMLInputElement>("#intelligence-enabled")!.checked = false;
  document.querySelector<HTMLDivElement>("#benchmark-refresh-panel")!.hidden = true;
  document.querySelector("#benchmark-refresh-log")!.textContent = "";
  document.querySelector<HTMLInputElement>("#benchmark-table-filter")!.value = "";
  document.querySelector<HTMLSelectElement>("#benchmark-table-view")!.value = "models";
  document.querySelector<HTMLSelectElement>("#benchmark-page-size")!.value = String(benchmarkPageSize);
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
function benchmarkLabel(name:string):string{return name.replace(/_/g," ").replace(/^./,letter=>letter.toUpperCase());}
function benchmarkEvidence(row:any):string{
 const scores=["overall",...benchmarkCapabilities];
 const operations:Record<string,string>={input_price:"Input USD / 1M",output_price:"Output USD / 1M",latency:"Latency seconds",ttft:"First token seconds",throughput:"Tokens / second",context_length:"Context tokens"};
 const scoreFields=scores.map(name=>[`${benchmarkLabel(name)} %`,benchmarkNumber(row.scores?.[name],true)]);
 const operationFields=Object.entries(operations).map(([name,label])=>[label,benchmarkNumber(row.operations?.[name])]);
 const provenanceFields=[["Model ID",row.id],["Providers",(row.provider_ids||[]).join(", ")||"—"],["Sources",(row.sources||[]).join(", ")||"—"],["Match",["approximate","ambiguous","unmatched"].includes(row.match)?`${benchmarkLabel(row.match)} · display only`:"Matched"],["Data status",row.stale?"Stale":"Current"],["Observations",String(row.observation_count||0)],["Latest evaluation",benchmarkDate(row.latest_evaluation),String(row.latest_evaluation||"")],["Last retrieval",benchmarkDate(row.latest_retrieval),String(row.latest_retrieval||"")],["Age seconds",typeof row.age_seconds==="number"&&Number.isFinite(row.age_seconds)?String(Math.round(row.age_seconds)):"—",String(row.age_seconds??"")],["Valid until",benchmarkDate(row.valid_until),String(row.valid_until||"")]];
 const group=(title:string,fields:any[][]):string=>`<section class="benchmark-evidence-section"><h3>${title}</h3><dl class="benchmark-evidence-grid">${fields.map(([label,value,exact])=>`<div><dt>${escapeHTML(label)}</dt><dd${exact?` title="${escapeHTML(exact)}"`:""}>${escapeHTML(String(value))}</dd></div>`).join("")}</dl></section>`;
 return `<div class="benchmark-evidence-content">${group("Capability scores",scoreFields)}${group("Operating metrics",operationFields)}${group("Provenance & freshness",provenanceFields)}</div>`;
}

let inspectedBenchmarkModel="";
function openBenchmarkEvidence(id:string):void{
 const model=benchmarkModels[id];if(!model)return;inspectedBenchmarkModel=id;
 document.querySelector("#benchmark-evidence-title")!.textContent=model.display_name||id;
 document.querySelector("#benchmark-evidence-content")!.innerHTML=benchmarkEvidence({...model,id});
 const dialog=document.querySelector<HTMLDialogElement>("#benchmark-evidence-dialog");dialog?.showModal?.();
 document.querySelector<HTMLElement>("#benchmark-evidence-title")?.focus?.({preventScroll:true});
 if(dialog)dialog.scrollTop=0;
}
function closeBenchmarkEvidence():void{document.querySelector<HTMLDialogElement>("#benchmark-evidence-dialog")?.close?.();}
type BenchmarkTableField = { key:string; label:string };
const benchmarkSummaryFields: BenchmarkTableField[] = [
 {key:"model",label:"Model"},{key:"overall",label:"Overall %"},{key:"capabilities",label:"Capability scores"},{key:"sources",label:"Sources"},{key:"freshness",label:"Match / freshness"},{key:"evidence",label:"Evidence"},
 {key:"id",label:"Model ID"},{key:"display_name",label:"Display name"},{key:"provider_ids",label:"Providers"},{key:"external_ids",label:"External model IDs"},
 ...benchmarkCapabilities.map(key=>({key:`score:${key}`,label:`${benchmarkLabel(key)} %`})),
 ...Object.keys(benchmarkOperations).map(key=>({key:`operation:${key}`,label:benchmarkLabel(key)})),
 {key:"match",label:"Match"},{key:"stale",label:"Data status"},{key:"observation_count",label:"Observations"},{key:"latest_evaluation",label:"Latest evaluation"},{key:"latest_retrieval",label:"Last retrieval"},{key:"age_seconds",label:"Age seconds"},{key:"valid_until",label:"Valid until"}
];
const benchmarkMeasurementFields: BenchmarkTableField[] = [
 {key:"external_model_id",label:"Model ID"},{key:"benchmark",label:"Benchmark"},{key:"category",label:"Category"},{key:"original_score",label:"Original score"},{key:"normalized_score",label:"Normalized score (0–1)"},{key:"unit",label:"Unit"},{key:"version",label:"Version"},{key:"evaluated_at",label:"Evaluation time"},
 {key:"source",label:"Source"},{key:"model_id",label:"Matched model ID"},{key:"retrieved_at",label:"Retrieved at"},{key:"date_basis",label:"Date basis"}
];
let benchmarkEditing = false;
let benchmarkSummaryEnabled = ["model","overall","capabilities","sources","freshness","evidence"];
let benchmarkMeasurementEnabled = benchmarkMeasurementFields.slice(0,8).map(field=>field.key);
let benchmarkInlineDrafts: Record<string, {record:Record<string,any>; values:Record<string,string>; error:string}> = {};
let benchmarkInlineSaving = false;
let benchmarkInlineSaveSequence = 0;
function benchmarkActiveFields():BenchmarkTableField[]{return benchmarkEditing?benchmarkMeasurementFields:benchmarkSummaryFields;}
function benchmarkEnabledFields():string[]{return benchmarkEditing?benchmarkMeasurementEnabled:benchmarkSummaryEnabled;}
function renderBenchmarkFields():void{
 const enabled=benchmarkEnabledFields(),fields=benchmarkActiveFields();
 const option=(key:string):string=>`<option value="${escapeHTML(key)}">${escapeHTML(fields.find(field=>field.key===key)!.label)}</option>`;
 document.querySelector("#benchmark-fields-available")!.innerHTML=fields.filter(field=>!enabled.includes(field.key)).map(field=>option(field.key)).join("");
 document.querySelector("#benchmark-fields-enabled")!.innerHTML=enabled.map(option).join("");
 document.querySelector("#benchmark-fields-status")!.textContent=`${enabled.length} fields enabled`;
}
function transferBenchmarkFields(add:boolean,one?:string):void{
 const select=document.querySelector<HTMLSelectElement>(add?"#benchmark-fields-available":"#benchmark-fields-enabled")!;
 const selected=one?[one]:Array.from(select.selectedOptions).map(option=>option.value);
 let enabled=benchmarkEnabledFields().slice();
 for(const key of selected){if(!benchmarkActiveFields().some(field=>field.key===key))continue;if(add){if(!enabled.includes(key))enabled.push(key);}else enabled=enabled.filter(field=>field!==key);}
 if(!enabled.length){document.querySelector("#benchmark-fields-status")!.textContent="Keep at least one field enabled.";return;}
 if(benchmarkEditing)benchmarkMeasurementEnabled=enabled;else benchmarkSummaryEnabled=enabled;
 renderBenchmarkFields();renderBenchmarkTable();
}
function benchmarkSummaryCell(row:any,key:string):string{
 const text=(raw:any)=>escapeHTML(String(raw??"—"));
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
function benchmarkRecordKey(record:Record<string,any>):string{return `${record.source}:${record.index}`;}
function benchmarkRecordValues(record:Record<string,any>):Record<string,string>{
 return Object.fromEntries(benchmarkMeasurementFields.slice(0,8).map(({key})=>[key,key==="evaluated_at"&&String(record.observation[key]).startsWith("0001-")?"":String(record.observation[key]??"")]));
}
function renderBenchmarkMeasurements():void{
 const fields=benchmarkEnabledFields().map(key=>benchmarkMeasurementFields.find(field=>field.key===key)!);
 document.querySelector("#benchmark-table-head")!.innerHTML=`<tr>${fields.map(field=>`<th scope="col">${escapeHTML(field.label)}</th>`).join("")}<th scope="col">Actions</th></tr>`;
 document.querySelector("#benchmark-table-body")!.innerHTML=benchmarkRecords.map((record,index)=>{
  const draft=benchmarkInlineDrafts[benchmarkRecordKey(record)];const values=draft?.values||benchmarkRecordValues(record);
  const cells=fields.map(field=>{
   const attrs=`data-benchmark-record="${index}" data-benchmark-cell="${field.key}" aria-label="${escapeHTML(field.label)} for ${escapeHTML(record.observation.external_model_id)}" ${benchmarkInlineSaving?"disabled":""}`;
   let cell:string;
   if(field.key==="category")cell=`<select class="benchmark-inline-input" ${attrs}>${[...benchmarkCapabilities,"operational","unknown"].map(category=>`<option ${values.category===category?"selected":""}>${category}</option>`).join("")}</select>`;
   else if(field.key in values)cell=`<input class="benchmark-inline-input" ${attrs} value="${escapeHTML(values[field.key])}" ${["original_score","normalized_score"].includes(field.key)?`type="number" step="any" ${field.key==="normalized_score"?'min="0" max="1"':""}`:'type="text" maxlength="512"'} ${["external_model_id","benchmark","original_score"].includes(field.key)?"required":""} />`;
   else cell=escapeHTML(String(record.observation[field.key]||record[field.key]||"—"));
   return `<td data-label="${escapeHTML(field.label)}">${cell}</td>`;
  }).join("");
  return `<tr>${cells}<td data-label="Actions"><button class="button quiet" type="button" data-benchmark-save-row="${index}" ${!draft||benchmarkInlineSaving?"disabled":""}>Save</button><button class="button quiet" type="button" data-benchmark-cancel-row="${index}" ${!draft||benchmarkInlineSaving?"disabled":""}>Cancel</button><span class="data-secondary" role="status">${escapeHTML(draft?.error||"")}</span></td></tr>`;
 }).join("")||`<tr><td colspan="${fields.length+1}">No measurements found.</td></tr>`;
 document.querySelector("#benchmark-data-count")!.textContent=`${benchmarkRecordTotal} measurements`;
 const pages=Math.max(1,Math.ceil(benchmarkRecordTotal/benchmarkPageSize));
 document.querySelector("#benchmark-page-label")!.textContent=`Page ${benchmarkPage+1} of ${pages}`;
 document.querySelector<HTMLButtonElement>("#benchmark-previous")!.disabled=benchmarkPage===0||benchmarkInlineSaving;
 document.querySelector<HTMLButtonElement>("#benchmark-next")!.disabled=(benchmarkPage+1)*benchmarkPageSize>=benchmarkRecordTotal||benchmarkInlineSaving;
}
function updateBenchmarkInlineDraft(event:Event):void{
 const input=event.target as HTMLInputElement;const index=Number(input.dataset.benchmarkRecord),field=input.dataset.benchmarkCell;
 const record=benchmarkRecords[index];if(!benchmarkEditing||benchmarkInlineSaving||!record||!field)return;
 const key=benchmarkRecordKey(record);
 const draft=benchmarkInlineDrafts[key]||(benchmarkInlineDrafts[key]={record:structuredClone(record),values:benchmarkRecordValues(record),error:""});
 draft.values[field]=input.value;draft.error="";
 const row=input.closest("tr");row?.querySelectorAll<HTMLButtonElement>("button").forEach(button=>button.disabled=false);
}
async function saveBenchmarkInlineRow(index:number):Promise<void>{
 const record=benchmarkRecords[index];if(!record||benchmarkInlineSaving)return;
 const key=benchmarkRecordKey(record),draft=benchmarkInlineDrafts[key];if(!draft)return;
 const values=draft.values;const observation:Record<string,any>={};
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
  delete benchmarkInlineDrafts[key];document.querySelector("#benchmark-inline-status")!.textContent="Measurement saved.";
  await loadBenchmarkData();
 }catch(error){if(login===consoleSession&&sequence===benchmarkRecordSequence)draft.error=benchmarkSafeMessage(error instanceof Error?error.message:"Unable to save measurement");}
 finally{if(saveSequence===benchmarkInlineSaveSequence){benchmarkInlineSaving=false;renderBenchmarkTable();}}
}

function renderBenchmarkTable(): void {
 const table=document.querySelector("#benchmark-table");
 table?.classList.add("benchmark-scorecard");
 table?.classList.toggle("benchmark-custom-fields",benchmarkEditing||benchmarkSummaryEnabled.join(",")!=="model,overall,capabilities,sources,freshness,evidence");
 table?.classList.toggle("benchmark-measurements",benchmarkEditing);
 if(benchmarkEditing){renderBenchmarkMeasurements();return;}
 const fields=benchmarkEnabledFields().map(key=>benchmarkSummaryFields.find(field=>field.key===key)!);
 const headers=fields.map(field=>field.label);
 const query=benchmarkFilter.toLowerCase();
 const models=Object.entries(benchmarkModels).sort(([a,left],[b,right])=>String(left.display_name||a).localeCompare(String(right.display_name||b))||a.localeCompare(b))
   .filter(([id,model])=>`${id} ${model.display_name||""} ${(model.external_ids||[]).join(" ")} ${(model.provider_ids||[]).join(" ")} ${(model.sources||[]).join(" ")}`.toLowerCase().includes(query));
 const total=models.length;const focused=document.activeElement?.id;
 benchmarkPage=Math.min(Math.max(0,benchmarkPage),Math.max(0,Math.ceil(total/benchmarkPageSize)-1));
 const rows=models.slice(benchmarkPage*benchmarkPageSize,(benchmarkPage+1)*benchmarkPageSize).map(([id,model])=>({...model,id}));
 const body=document.querySelector("#benchmark-table-body")!;
 document.querySelector("#benchmark-table")?.classList.add("benchmark-scorecard");
 document.querySelector("#benchmark-table-head")!.innerHTML=`<tr>${headers.map(header=>`<th scope="col">${escapeHTML(header)}</th>`).join("")}</tr>`;
 body.innerHTML=rows.length?rows.map(row=>`<tr>${fields.map(field=>`<td data-label="${escapeHTML(field.label)}"${field.key==="overall"?' class="benchmark-overall"':""}>${benchmarkSummaryCell(row,field.key)}</td>`).join("")}</tr>`).join(""):`<tr><td colspan="${headers.length}"><div class="empty-state"><strong>No benchmark evidence in this view</strong><p>Enable a source, then refresh to import published results. Try another filter if data is already cached.</p></div></td></tr>`;
 body.querySelectorAll<HTMLButtonElement>("[data-benchmark-inspect]").forEach(button=>button.addEventListener("click",()=>openBenchmarkEvidence(button.dataset.benchmarkInspect!)));
 const pages=Math.max(1,Math.ceil(total/benchmarkPageSize));
 document.querySelector("#benchmark-data-count")!.textContent=`${total} models`;
 document.querySelector("#benchmark-page-label")!.textContent=`Page ${benchmarkPage+1} of ${pages}`;
 const previous=document.querySelector<HTMLButtonElement>("#benchmark-previous")!,next=document.querySelector<HTMLButtonElement>("#benchmark-next")!;
 previous.disabled=benchmarkPage===0;next.disabled=(benchmarkPage+1)*benchmarkPageSize>=total;
 if(focused==="benchmark-next"&&next.disabled)(previous.disabled?document.querySelector<HTMLInputElement>("#benchmark-table-filter"):previous)?.focus?.();
 if(focused==="benchmark-previous"&&previous.disabled)(next.disabled?document.querySelector<HTMLInputElement>("#benchmark-table-filter"):next)?.focus?.();
}
function benchmarkDate(value:any):string{
 if(!value||String(value).startsWith("0001-"))return "Unknown";
 const date=new Date(String(value));if(Number.isNaN(date.getTime()))return "Unknown";
 const display=date.toLocaleDateString("en-US",{timeZone:/^\d{4}-\d{2}-\d{2}$/.test(String(value))?"UTC":consoleTimezone||undefined,month:"short",day:"numeric",year:"numeric"});
 return /^\d{4}-\d{2}-\d{2}$/.test(String(value))?display:`${display} · ${date.toLocaleTimeString("en-US",{timeZone:consoleTimezone||undefined,hour:"2-digit",minute:"2-digit",hour12:false} )} ${escapeHTML(consoleTimezoneLabel())}`;
}
async function loadBenchmarkData(): Promise<boolean> {
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
    if (!benchmarkRefreshRunning) document.querySelector("#benchmark-status")!.textContent = "Saved evidence · missing scores are unknown.";
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
  if (login !== consoleSession || abort.signal.aborted) return;
  document.querySelector("#benchmark-status")!.textContent = "Loading benchmark results into the table…";
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
    await new Promise<void>(resolve => window.requestAnimationFrame && document.visibilityState !== "hidden" ? window.requestAnimationFrame(()=>window.requestAnimationFrame(()=>resolve())) : window.setTimeout(resolve,0));
    if (loadedSequence === benchmarkReloadSequence) break;
  }
  if (login !== consoleSession || abort.signal.aborted) return;
  const message = result.partial_failure ? "Benchmark refresh finished with source failures; table updated with available data." : "Benchmark refresh finished; table updated.";
  benchmarkProgress({phase:"table_loaded",message});
  document.querySelector("#benchmark-status")!.textContent = message;
}
async function inspectBenchmarks(refresh: boolean): Promise<void> {
  if (!refresh) { await loadBenchmarkData(); return; }
  const output = document.querySelector("#benchmark-status")!;
  if (!benchmarksEnabledSaved || !document.querySelector<HTMLInputElement>("#intelligence-enabled")!.checked || benchmarkDirty) { output.textContent = "Enable benchmark intelligence and wait for Saved before refreshing changed source settings."; return; }
  if (benchmarkRefreshRunning) return;
  const login = consoleSession;
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
      if (login !== consoleSession || abort.signal.aborted) return;
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
          if (!data || login !== consoleSession || abort.signal.aborted) continue;
          const event = JSON.parse(data);
          // Table requests must not block reading activity from a long import.
          // The final table step below still waits for the latest view to render.
          if (kind === "progress") { benchmarkProgress(event); if (event.phase === "published" || event.phase === "failed") void loadBenchmarkData(); }
          if (kind === "heartbeat") document.querySelector("#benchmark-progress-label")!.textContent = `${benchmarkProgressCounts.completed} of ${benchmarkProgressCounts.total} sources finished · Refresh is still running; waiting for upstream data or cooldown`;
          if (kind === "result") { finished = true; benchmarkSourceStatus = event.sources || {}; renderBenchmarkSourceStatus(); await finishBenchmarkRefresh(event,login,abort); }
        }
        if (chunk.done) break;
      }
      if (!finished && login === consoleSession && !abort.signal.aborted) throw new Error("Refresh connection ended before completion. Reload cached data to inspect published results.");
    } finally { await reader.cancel(); }
  } catch (error) { if (login === consoleSession && !abort.signal.aborted) { const message = benchmarkSafeMessage(error instanceof Error ? error.message : "Refresh failed"); output.textContent = message; benchmarkProgress({phase:"failed",message}); } }
  finally { if (benchmarkAbort === abort) { benchmarkRefreshRunning = false; benchmarkAbort = null; updateBenchmarkControls(); } }
}
function initializeBenchmarks(): void {
 document.querySelector<HTMLSelectElement>("#benchmark-page-size")!.value=String(benchmarkPageSize);
 document.querySelector("#close-benchmark-evidence")?.addEventListener("click",closeBenchmarkEvidence);
 const evidence=document.querySelector<HTMLDialogElement>("#benchmark-evidence-dialog");
 evidence?.addEventListener("close",()=>{const buttons=document.querySelectorAll<HTMLButtonElement>("[data-benchmark-inspect]");const opener=Array.from(buttons).find(button=>button.dataset.benchmarkInspect===inspectedBenchmarkModel);(opener||document.querySelector<HTMLInputElement>("#benchmark-table-filter"))?.focus?.();});
 evidence?.addEventListener("keydown",event=>{if(event.key==="Tab"){event.preventDefault();document.querySelector<HTMLButtonElement>("#close-benchmark-evidence")?.focus?.();}});
  const list = document.querySelector("#benchmark-sources-list")!;
  list.addEventListener("input", updateBenchmarkSource); list.addEventListener("click", benchmarkSourceAction);
  document.querySelector("#benchmark-page-size")!.addEventListener("change",()=>{const size=Number(document.querySelector<HTMLSelectElement>("#benchmark-page-size")!.value);if(![5,10,25,50,100].includes(size))return;benchmarkPageSize=size;benchmarkPage=0;if(benchmarkEditing)void loadBenchmarkRecords();else renderBenchmarkTable();});
  document.querySelector("#benchmark-table-view")!.addEventListener("change",()=>{benchmarkTableView=document.querySelector<HTMLSelectElement>("#benchmark-table-view")!.value;benchmarkPage=0;void loadBenchmarkData();});
  document.querySelector("#benchmark-table-filter")!.addEventListener("input",()=>{benchmarkFilter=document.querySelector<HTMLInputElement>("#benchmark-table-filter")!.value;benchmarkPage=0;if(benchmarkEditing)void loadBenchmarkRecords();else renderBenchmarkTable();});
  document.querySelector("#benchmark-previous")!.addEventListener("click",()=>{if(benchmarkPage>0){benchmarkPage--;if(benchmarkEditing)void loadBenchmarkRecords();else renderBenchmarkTable();}});
  document.querySelector("#benchmark-next")!.addEventListener("click",()=>{benchmarkPage++;if(benchmarkEditing)void loadBenchmarkRecords();else renderBenchmarkTable();});
  renderBenchmarkTable();
}

let benchmarkRecords:Record<string,any>[]=[];
let benchmarkRecordTotal=0;
let benchmarkRecordSequence=0;
async function loadBenchmarkRecords():Promise<void>{
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
 }catch(error){if(sequence===benchmarkRecordSequence&&login===consoleSession)document.querySelector("#benchmark-inline-status")!.textContent=benchmarkSafeMessage(error instanceof Error?error.message:"Unable to load measurements");}
}
function clearBenchmarkRecords():void{
 benchmarkRecords=[];benchmarkInlineDrafts={};benchmarkRecordTotal=0;benchmarkRecordSequence++;
 for(const id of ["benchmark-custom-status","benchmark-inline-status"]){const node=document.querySelector(`#${id}`);if(node)node.textContent="";}
 const file=document.querySelector<HTMLInputElement>("#benchmark-csv-file");if(file)file.value="";
}
async function uploadBenchmarkCSV():Promise<void>{
 const file=document.querySelector<HTMLInputElement>("#benchmark-csv-file")?.files?.[0];const status=document.querySelector("#benchmark-custom-status")!;
 if(!file){status.textContent="Choose a CSV file first.";return;}
 if(file.size>8*1024*1024){status.textContent="CSV files must be at most 8 MiB.";return;}
 const login=consoleSession;const button=document.querySelector<HTMLButtonElement>("#benchmark-csv-upload")!;button.disabled=true;status.textContent="Validating and saving CSV…";
 try{
  const response=await gatewayFetch("/gateway/v1/intelligence/import",{method:"POST",headers:{"Content-Type":"text/csv",...await authProvider.headers()},body:file});
  const result=await response.json();if(login!==consoleSession)return;if(!response.ok)throw new Error(result.error?.message||"CSV upload failed");
  status.textContent="Custom benchmarks saved.";benchmarkPage=0;await loadBenchmarkData();
 }catch(error){status.textContent=benchmarkSafeMessage(error instanceof Error?error.message:"CSV upload failed");}finally{button.disabled=false;}
}
document.querySelector("#benchmark-csv-upload")?.addEventListener("click",()=>void uploadBenchmarkCSV());
for(const [button,id] of [["benchmark-configure","benchmark-configuration"],["benchmark-upload","benchmark-upload-dialog"],["benchmark-display-fields","benchmark-fields-dialog"]]){
 document.querySelector(`#${button}`)?.addEventListener("click",()=>{if(id==="benchmark-fields-dialog")renderBenchmarkFields();document.querySelector<HTMLDialogElement>(`#${id}`)?.showModal?.();});
 document.querySelector(`#${id}-close`)?.addEventListener("click",()=>document.querySelector<HTMLDialogElement>(`#${id}`)?.close?.());
}
document.querySelector("#benchmark-sources-list")?.addEventListener("change",()=>void saveConfig());
document.querySelector("#benchmark-field-add")?.addEventListener("click",()=>transferBenchmarkFields(true));
document.querySelector("#benchmark-field-remove")?.addEventListener("click",()=>transferBenchmarkFields(false));
for(const [id,add] of [["benchmark-fields-available",true],["benchmark-fields-enabled",false]] as const){
 document.querySelector(`#${id}`)?.addEventListener("dblclick",event=>{const option=event.target as HTMLOptionElement;if(option.tagName==="OPTION")transferBenchmarkFields(add,option.value);});
 document.querySelector(`#${id}`)?.addEventListener("keydown",event=>{if((event as KeyboardEvent).key==="Enter"){event.preventDefault();transferBenchmarkFields(add);}});
}
document.querySelector("#benchmark-edit-data")?.addEventListener("click",async()=>{
 if(benchmarkInlineSaving)return;
 if(benchmarkEditing&&Object.keys(benchmarkInlineDrafts).length&&!confirm("Discard unsaved measurement edits?"))return;
 benchmarkEditing=!benchmarkEditing;benchmarkInlineDrafts={};benchmarkPage=0;
 document.querySelector("#benchmark-edit-data")!.textContent=benchmarkEditing?"Done editing":"Edit data";
 document.querySelector("#benchmark-edit-data")!.setAttribute("aria-pressed",String(benchmarkEditing));
 document.querySelector("#benchmark-inline-status")!.textContent=benchmarkEditing?"Edit measurement fields, then Save each row. A source refresh can replace downloaded measurements; custom uploads are retained.":"";
 renderBenchmarkFields();if(benchmarkEditing)await loadBenchmarkRecords();renderBenchmarkTable();
});
document.querySelector("#benchmark-table-body")?.addEventListener("input",updateBenchmarkInlineDraft);
document.querySelector("#benchmark-table-body")?.addEventListener("change",updateBenchmarkInlineDraft);
document.querySelector("#benchmark-table-body")?.addEventListener("click",event=>{
 const button=(event.target as HTMLElement).closest<HTMLButtonElement>("[data-benchmark-save-row],[data-benchmark-cancel-row]");if(!button||benchmarkInlineSaving)return;
 if(button.dataset.benchmarkSaveRow!==undefined)void saveBenchmarkInlineRow(Number(button.dataset.benchmarkSaveRow));
 else{const record=benchmarkRecords[Number(button.dataset.benchmarkCancelRow)];if(record){delete benchmarkInlineDrafts[benchmarkRecordKey(record)];renderBenchmarkTable();}}
});

document.querySelector("#benchmark-table-body")?.addEventListener("keydown",event=>{
 const input=event.target as HTMLInputElement;
 if((event as KeyboardEvent).key==="Enter"&&input.dataset.benchmarkCell){event.preventDefault();void saveBenchmarkInlineRow(Number(input.dataset.benchmarkRecord));}
});

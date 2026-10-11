const modelLoadingMarkup='<div class="empty-state" role="status"><strong>Loading model catalog…</strong><p>Discovering models from configured providers.</p></div>';
let modelEnabledDraft:Record<string,boolean>={};
let physicalModels:any[]=[];
let modelSearch="";
let modelProviderFilter="";
let modelStatusFilter="";
let modelSort="name";
let modelPage=0;
let modelPageSize=matchMedia("(max-width: 760px)").matches?5:10;
function modelQuotaSummary(model:any):string{const q=model.quota;return q?.known?`${Number(q.fraction_remaining*100).toFixed(1)}% quota remaining${q.windows?.length?` · ${q.windows.map((w:any)=>`${w.name}: ${quotaResetLabel(w.reset_at)}`).join(" · ")}`:""}`:model.quota_error||"Provider quota unknown";}
function modelPrice(raw:number):string{return raw>0&&raw<.000001?`$${raw.toPrecision(3)}`:`$${raw.toLocaleString(undefined,{minimumFractionDigits:2,maximumFractionDigits:6})}`;}
function modelPricingText(model:any):string{
 const p=model.pricing||{};
 if(model.subscription)return "Subscription included";
 return `${typeof p.input_per_million==="number"?modelPrice(p.input_per_million):"Unknown"} in / ${typeof p.output_per_million==="number"?modelPrice(p.output_per_million):"Unknown"} out per 1M${String(p.source||"").startsWith("models.dev")?" (estimate)":""}`;
}
function quotaWindowText(window:any):string{
 const remaining=Number(window.remaining);
 const value=window.unit==="USD"?modelPrice(remaining):window.unit==="percent"?`${remaining.toFixed(1)}%`:`${consoleNumber(remaining)} ${window.unit||"units"}`;
 const reset=window.reset_at?new Date(window.reset_at):null;
 return `${window.name?`${window.name.replace(/_/g," ")}: `:""}${value} remaining${window.informational?" (informational)":""}${reset&&!Number.isNaN(reset.getTime())?` · resets ${reset.toLocaleString()}`:" · reset unknown"}`;
}
function balanceText(q:any):string {
 return (q?.balances||[]).map((b:any)=>`${b.name.replace(/_/g," ")}: ${b.unlimited?"Unlimited":typeof b.remaining==="number"?(b.unit==="USD"?modelPrice(b.remaining):`${consoleNumber(b.remaining)} ${b.unit}`):"Unknown"}${b.reset_at?` · renews ${new Date(b.reset_at).toLocaleString()}`:""}`).join("; ");
}
function trackingReasonText(reason:string):string{return ({management_credentials_required:"Billing management credentials required",admin_credentials_required:"Billing admin credential required",oauth_credentials_required:"Subscription OAuth credential required",wallet_balance_not_exposed:"Provider does not expose wallet balance",response_headers_only:"Quota available from response headers",credential_rejected:"Tracking credential rejected",endpoint_unavailable:"Tracking endpoint unavailable",rate_limited:"Tracking rate limited",tracking_unavailable:"Tracking unavailable",no_reported_data:"No reported provider data",spend_unavailable:"Provider spend unavailable",report_incomplete:"Provider report incomplete",no_subscription:"No subscription reported"} as Record<string,string>)[reason]||"Provider data unavailable";}
function modelQuotaText(model:any):string{
 const q=model.quota;
 const extras=[balanceText(q),typeof q?.reset_credits==="number"?`${q.reset_credits} banked resets`:""].filter(Boolean).join(" · ");
 if(extras&&!q?.known&&!q?.windows?.length&&!q?.reset_at)return `${q.blocked?"Quota exhausted · ":""}${extras}${q?.unavailable_reason?` · ${trackingReasonText(q.unavailable_reason)}`:""}`;
 if(!q?.known&&q?.windows?.length)return q.windows.map(quotaWindowText).join("; ")+`${extras?` · ${extras}`:""}`;
 if(!q?.known&&q?.reset_at)return `Quota unknown · resets ${new Date(q.reset_at).toLocaleString()}${extras?` · ${extras}`:""}`;
 if(!q?.known&&q?.unavailable_reason)return trackingReasonText(q.unavailable_reason);
 if(!q?.known)return q?.blocked?"Quota exhausted":q?.source==="unavailable"?"Quota unavailable":"Quota unknown";
 const windows=q.windows?.length?q.windows:[q];
 return `${q.exhausted?"Exhausted · ":""}${windows.map(quotaWindowText).join("; ")}${extras?` · ${extras}`:""}`;
}
function modelUsageText(model:any):string{
 if(!model.usage_available)return "Usage unavailable";
 const row=(model.usage||[]).find((r:any)=>/^\d{4}-\d{2}$/.test(r.period));
 if(!row)return "No gateway usage this month";
 return `${consoleNumber(Number(row.input_tokens||0)+Number(row.output_tokens||0))} tokens · ${modelPrice(Number(row.estimated_cost||0))} est. this month${row.unknown_cost_attempts?" + unknown costs":""}`;
}
function modelProviderUsageText(model:any):string{
 const q=model.quota||{};const monthly=(q.reported_usage||[]).filter((u:any)=>u.period==="monthly");
 const parts=monthly.map((u:any)=>`${u.unit==="USD"?modelPrice(u.used):`${consoleNumber(u.used)} ${u.unit}`} (${u.scope||"account"})`);
 if(typeof q.usage_usd?.monthly==="number"&&!monthly.some((u:any)=>u.unit==="USD"))parts.unshift(`${modelPrice(q.usage_usd.monthly)} (account)`);
 return parts.length?`Provider this month: ${parts.join("; ")}`:"";
}
function modelSelectorLabel(model:any):string{return [model.id,modelPricingText(model),modelQuotaText(model),modelProviderUsageText(model),modelUsageText(model)].filter(Boolean).join(" · ");}
function modelTrackingMarkup(model:any):string{
 const q=model.quota||{};const reported=q.usage_usd;
 const usage=(q.reported_usage||[]).map((u:any)=>`${u.period}: ${u.unit==="USD"?modelPrice(u.used):`${consoleNumber(u.used)} ${u.unit}`} (${u.scope||"account"})`).join(" · ");
 return `<p><strong>Provider quota</strong><br>${escapeHTML(modelQuotaText(model))}</p>${model.pricing?.source?`<p>Price source: ${escapeHTML(model.pricing.source)}</p>`:""}${q.plan?`<p>Plan: ${escapeHTML(q.plan)}</p>`:""}${q.partial?`<p>${escapeHTML(trackingReasonText(q.unavailable_reason||"tracking_unavailable"))} · Partial provider data</p>`:""}${usage?`<p><strong>Provider-reported usage</strong><br>${escapeHTML(usage)}</p>`:""}${q.checked_at&&q.checked_at!=="0001-01-01T00:00:00Z"?`<p>Checked ${escapeHTML(new Date(q.checked_at).toLocaleString())} · ${escapeHTML(q.source||"unknown source")}</p>`:""}${reported?`<p><strong>Provider-reported spend</strong><br>${["total","daily","weekly","monthly"].filter(period=>typeof reported[period]==="number").map(period=>`${period}: ${modelPrice(reported[period])}`).map(escapeHTML).join(" · ")}</p>`:""}${model.usage_available!==undefined?`<p><strong>Gateway usage</strong><br>${escapeHTML(modelUsageText(model))}</p>`:""}`;
}
function filteredPhysicalModels():any[]{const query=modelSearch.toLowerCase();return physicalModels.filter(model=>(!query||`${model.id} ${model.provider_id}`.toLowerCase().includes(query))&&(!modelProviderFilter||model.provider_id===modelProviderFilter)&&(!modelStatusFilter||Boolean(model.enabled)===(modelStatusFilter==="enabled"))).sort((a,b)=>modelSort==="provider"?String(a.provider_id).localeCompare(String(b.provider_id))||String(a.id).localeCompare(String(b.id)):modelSort==="enabled"?Number(Boolean(b.enabled))-Number(Boolean(a.enabled))||String(a.id).localeCompare(String(b.id)):String(a.id).localeCompare(String(b.id)));}
function renderModelRows():void{
 const rows=document.querySelector("#model-table-body");if(!rows)return;const focused=document.activeElement?.id;const filtered=filteredPhysicalModels();modelPage=Math.min(Math.max(0,modelPage),Math.max(0,Math.ceil(filtered.length/modelPageSize)-1));const models=filtered.slice(modelPage*modelPageSize,(modelPage+1)*modelPageSize);
 rows.innerHTML=models.length?models.map((model:any)=>{
  const pricing=model.pricing||{};const supported=Object.entries(model.capabilities||{}).filter(([,v])=>v===true||v==="supported").map(([name])=>name.replace(/_/g," "));
  const capabilities=supported.map(name=>`<span class="capability-pill">${escapeHTML(name)}</span>`).join("")||'<span class="data-secondary">Unknown</span>';
  const scores=model.benchmark?Object.entries(model.benchmark.scores||{}).map(([name,score])=>`${name.replace(/_/g," ")} ${(Number(score)*100).toFixed(1)}%`).join(" · "):"";
  const details=`<details class="model-details"><summary>Inspect metadata</summary><div class="model-detail-content"><p><strong>Capabilities</strong><br>${Object.entries(model.capabilities||{}).map(([name,v])=>`${escapeHTML(name.replace(/_/g," "))}: ${escapeHTML(String(v))}`).join(" · ")||"Unknown"}</p><p><strong>Context window</strong> ${consoleNumber(model.context_window)} tokens</p>${modelTrackingMarkup(model)}${model.benchmark?`<p><strong>Benchmark scores</strong><br>${escapeHTML(scores||"No scores")}</p><p><strong>Sources</strong> ${(model.benchmark.sources||[]).map(escapeHTML).join(", ")||"Unknown"}<br>Retrieved ${escapeHTML(model.benchmark.latest_retrieval||"unknown")}<br>Evaluated ${escapeHTML(model.benchmark.latest_evaluation||"unknown")}</p>`:""}</div></details>`;
  return `<tr><td data-label="Model"><strong class="model-name">${escapeHTML(model.id)}</strong><span class="data-secondary">${escapeHTML(model.provider_id)}</span></td><td data-label="Capabilities"><div class="capability-list">${capabilities}</div></td><td class="numeric" data-label="Input / 1M tokens">${model.subscription?'<span class="data-secondary">Subscription included</span>':""}${typeof pricing.input_per_million==="number"?modelPrice(pricing.input_per_million):"—"}<span class="data-secondary">${typeof pricing.output_per_million==="number"?`${modelPrice(pricing.output_per_million)} output`:"Output unknown"}</span></td><td data-label="Benchmark evidence"><span class="data-secondary">${escapeHTML(model.benchmark?scores.split(" · ").slice(0,2).join(" · ")||"No scores":"No matched benchmark data")}</span>${model.benchmark?`<span class="status-badge ${model.benchmark.stale?"neutral":"success"}">${model.benchmark.stale?"Stale":"Current"}</span>`:""}${details}</td><td data-label="Access"><label class="checkbox model-enable"><input type="checkbox" data-model-enabled="${escapeHTML(model.id)}" aria-label="Enable ${escapeHTML(model.id)}" ${model.enabled?"checked":""}><span>${model.enabled?"Enabled":"Disabled"}</span></label></td><td class="model-tracking" data-label="Usage &amp; provider quota"><span class="data-secondary">${escapeHTML(modelQuotaText(model))}</span><span class="data-secondary">${escapeHTML(modelUsageText(model))}</span></td></tr>`;
 }).join(""):`<tr><td colspan="6"><div class="empty-state"><strong>No models match these filters</strong><p>Try another model name or clear your filters.</p></div></td></tr>`;
 document.querySelector("#model-count")!.textContent=`${filtered.length?modelPage*modelPageSize+1:0}–${Math.min((modelPage+1)*modelPageSize,filtered.length)} of ${filtered.length} models${filtered.length!==physicalModels.length?` · ${physicalModels.length} total`:""}`;
 document.querySelector<HTMLButtonElement>("#model-previous")!.disabled=modelPage===0;document.querySelector<HTMLButtonElement>("#model-next")!.disabled=(modelPage+1)*modelPageSize>=filtered.length;
 if(focused==="model-next"&&document.querySelector<HTMLButtonElement>("#model-next")!.disabled)(document.querySelector<HTMLButtonElement>("#model-previous")!.disabled?document.querySelector<HTMLInputElement>("#model-search"):document.querySelector<HTMLButtonElement>("#model-previous"))?.focus?.();
 if(focused==="model-previous"&&document.querySelector<HTMLButtonElement>("#model-previous")!.disabled)(document.querySelector<HTMLButtonElement>("#model-next")!.disabled?document.querySelector<HTMLInputElement>("#model-search"):document.querySelector<HTMLButtonElement>("#model-next"))?.focus?.();
 rows.querySelectorAll<HTMLInputElement>("[data-model-enabled]").forEach(input=>input.addEventListener("change",async()=>{modelEnabledDraft[input.dataset.modelEnabled!]=input.checked;markDirty();if(await saveConfig())void loadModelsPage();}));
}
function renderModelsPage():void{
 const output=document.querySelector<HTMLElement>("#models-list")!;
 if(!physicalModels.length){output.innerHTML='<div class="empty-state"><span class="empty-state-icon">◇</span><strong>Your model catalog is empty</strong><p>Connect and test a provider to discover the models available to your gateway.</p><a href="#providers" class="button primary">Connect a provider</a></div>';return;}
 const providerIDs=[...new Set(physicalModels.map(model=>String(model.provider_id)))].sort();
 output.innerHTML=`<div class="models-toolbar"><label class="search-field" for="model-search">Search models<input aria-label="Search models" id="model-search" type="search" placeholder="Search by model or provider…" maxlength="256" value="${escapeHTML(modelSearch)}"></label><label for="model-provider-filter">Provider<select id="model-provider-filter" aria-label="Provider"><option value="">All providers</option>${providerIDs.map(id=>`<option value="${escapeHTML(id)}">${escapeHTML(id)}</option>`).join("")}</select></label><label for="model-status-filter">Status<select id="model-status-filter" aria-label="Status"><option value="">All models</option><option value="enabled">Enabled</option><option value="disabled">Disabled</option></select></label><label for="model-sort">Sort<select id="model-sort" aria-label="Sort models"><option value="name">Model name</option><option value="provider">Provider</option><option value="enabled">Enabled first</option></select></label><span id="model-count" class="section-note" role="status"></span></div><div class="benchmark-table-scroll"><table class="model-table"><thead><tr><th scope="col">Model / provider</th><th scope="col">Capabilities</th><th scope="col">Input / 1M tokens</th><th scope="col">Benchmark evidence</th><th scope="col">Access</th><th scope="col">Usage &amp; provider quota</th></tr></thead><tbody id="model-table-body"></tbody></table></div><div class="benchmark-pagination models-pagination"><label for="model-page-size" class="inline-filter">Rows<select id="model-page-size" aria-label="Models per page"><option value="5">5</option><option value="10">10</option><option value="20">20</option><option value="50">50</option></select></label><div><button id="model-previous" class="button quiet" type="button">Previous</button><button id="model-next" class="button quiet" type="button">Next</button></div></div><p class="section-note">Prices are USD per million tokens; subscription models have zero marginal cost. Gateway spend is estimated. Provider quota snapshots share account limits across models where applicable. — means unknown. Disabled models are unavailable to client discovery and routing.</p>`;
 document.querySelector<HTMLInputElement>("#model-search")!.addEventListener("input",event=>{modelSearch=(event.target as HTMLInputElement).value;modelPage=0;renderModelRows();});
 const provider=document.querySelector<HTMLSelectElement>("#model-provider-filter")!;provider.value=modelProviderFilter;provider.addEventListener("change",()=>{modelProviderFilter=provider.value;modelPage=0;renderModelRows();});
 const status=document.querySelector<HTMLSelectElement>("#model-status-filter")!;status.value=modelStatusFilter;status.addEventListener("change",()=>{modelStatusFilter=status.value;modelPage=0;renderModelRows();});const sort=document.querySelector<HTMLSelectElement>("#model-sort")!;sort.value=modelSort;sort.addEventListener("change",()=>{modelSort=sort.value;modelPage=0;renderModelRows();});
 const size=document.querySelector<HTMLSelectElement>("#model-page-size")!;size.value=String(modelPageSize);size.addEventListener("change",()=>{modelPageSize=[5,10,20,50].includes(Number(size.value))?Number(size.value):10;modelPage=0;renderModelRows();});
 document.querySelector("#model-previous")!.addEventListener("click",()=>{modelPage--;renderModelRows();});document.querySelector("#model-next")!.addEventListener("click",()=>{modelPage++;renderModelRows();});renderModelRows();
}
async function loadModelsPage():Promise<void>{
 const output=document.querySelector<HTMLElement>("#models-list")!;const login=consoleSession;const button=document.querySelector<HTMLButtonElement>("#refresh-models")!;button.disabled=true;if(!physicalModels.length)output.innerHTML=modelLoadingMarkup;
 try{const response=await gatewayFetch("/gateway/v1/models",{headers:await authProvider.headers()});if(login!==consoleSession)return;if(!response.ok)throw new Error("Model discovery unavailable. Refresh to retry.");const data=await response.json();if(login!==consoleSession)return;physicalModels=data.models||[];overviewModels=physicalModels;renderModelsPage();renderOverview();
 }catch(error){output.innerHTML=`<div class="empty-state error-state" role="alert"><strong>Unable to load models</strong><p>${escapeHTML(error instanceof Error?error.message:"Models unavailable")}</p></div>`;}finally{button.disabled=false;}
}
document.querySelector("#refresh-models")!.addEventListener("click",()=>{void loadModelsPage();});

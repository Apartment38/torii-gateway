let budgetsDraft:Record<string,any>={};
function renderBudgetEditor():void {
 const scope=document.querySelector<HTMLSelectElement>("#budget-scope")!.value||"*";
 const budgets=scope==="*"?budgetsDraft.gateway:budgetsDraft.keys?.[scope];
 for(const period of ["daily","monthly"])for(const metric of ["requests","tokens","estimated_cost"]){document.querySelector<HTMLInputElement>(`#budget-${period}-${metric}`)!.value=String(budgets?.[period]?.[metric]||0);}
}
let usageData:any=null;
let usageKeyMetadata:any[]=[];
let usagePeriod="daily";
function renderUsagePage():void{
 const output=document.querySelector<HTMLElement>("#usage-list")!;if(!usageData)return;
 const scope=document.querySelector<HTMLSelectElement>("#budget-scope")!.value||"*";
 const rows=(usageData.usage||[]).filter((r:any)=>r.key_id!=="*" && (scope==="*"||r.key_id===scope) && (usagePeriod==="daily"?/^\d{4}-\d{2}-\d{2}$/.test(r.period):/^\d{4}-\d{2}$/.test(r.period)));
 const requests=rows.filter((r:any)=>!r.model).reduce((sum:number,r:any)=>sum+Number(r.requests||0),0);
 const input=rows.reduce((sum:number,r:any)=>sum+Number(r.input_tokens||0),0);const out=rows.reduce((sum:number,r:any)=>sum+Number(r.output_tokens||0),0);
 const cost=rows.reduce((sum:number,r:any)=>sum+Number(r.estimated_cost||0),0);const unknown=rows.reduce((sum:number,r:any)=>sum+Number(r.unknown_cost_attempts||0),0);
 const budgets=(usageData.budgets||[]).filter((b:any)=>(scope==="*"||b.key_id===scope)&&(b.limit.requests||b.limit.tokens||b.limit.estimated_cost));
 output.innerHTML=`<div class="section-heading"><h3>Usage overview</h3><label class="inline-filter">Period<select id="usage-period"><option value="daily">Today · UTC</option><option value="monthly">This month · UTC</option></select></label></div><div class="metric-grid usage-metrics"><div class="metric-card"><span class="metric-label">Client requests</span><strong class="metric-value">${consoleNumber(requests)}</strong><span class="metric-detail">${usagePeriod==="daily"?"Today":"This month"} · current scope</span></div><div class="metric-card"><span class="metric-label">Input tokens</span><strong class="metric-value">${consoleNumber(input)}</strong><span class="metric-detail">Includes classifiers and retries</span></div><div class="metric-card"><span class="metric-label">Output tokens</span><strong class="metric-value">${consoleNumber(out)}</strong><span class="metric-detail">Reported and conservative estimates</span></div><div class="metric-card"><span class="metric-label">Estimated cost</span><strong class="metric-value">$${cost.toFixed(4)}</strong><span class="metric-detail">${unknown?`${unknown} attempts with unknown cost`:"Operator estimate · USD"}</span></div></div>${budgets.length?`<details class="advanced-fields"><summary>Active limits &amp; resets · ${budgets.length}</summary><div class="budget-summary-list">${budgets.map((b:any)=>`<div class="budget-summary"><strong>${escapeHTML(b.key_id==="*"?"Gateway":usageKeyMetadata.find(k=>k.id===b.key_id)?.name||b.key_id)} · ${escapeHTML(b.period)}</strong><span class="data-secondary">Resets ${escapeHTML(b.reset_at)}</span><span>${b.limit.requests?`${consoleNumber(b.remaining.requests)} requests remaining`:"Unlimited requests"} · ${b.limit.tokens?`${consoleNumber(b.remaining.tokens)} tokens remaining`:"Unlimited tokens"} · ${b.limit.estimated_cost?`$${Number(b.remaining.estimated_cost).toFixed(4)} remaining`:"Unlimited estimated cost"}</span></div>`).join("")}</div></details>`:""}<div class="benchmark-table-scroll"><table class="usage-table"><caption class="usage-table-caption">${usagePeriod==="daily"?"Current UTC day":"Current UTC month"}${rows[0]?.period?` · ${escapeHTML(rows[0].period)}`:""} · selected usage scope</caption><thead><tr><th scope="col">API key</th><th scope="col">Model</th><th scope="col">Client requests</th><th scope="col">Input / output tokens</th><th scope="col">Est. tokens</th><th scope="col">Est. USD</th></tr></thead><tbody>${rows.length?rows.map((r:any)=>`<tr><td><strong>${escapeHTML(r.key_name||r.key_id)}</strong></td><td>${escapeHTML(r.model||"Client request total")}${r.provider?`<span class="data-secondary">${escapeHTML(r.provider)}</span>`:""}</td><td class="numeric">${r.model?"—":consoleNumber(r.requests||0)}</td><td class="numeric">${consoleNumber(r.input_tokens||0)} / ${consoleNumber(r.output_tokens||0)}</td><td class="numeric">${consoleNumber(r.estimated_tokens||0)}</td><td class="numeric">${Number(r.estimated_cost||0).toFixed(6)}${r.unknown_cost_attempts?'<span class="data-secondary">Plus unknown costs</span>':""}</td></tr>`).join(""):'<tr><td colspan="6"><div class="empty-state"><strong>No usage in this period</strong><p>Request and model totals appear after clients use the gateway.</p></div></td></tr>'}</tbody></table></div><p class="section-note">${escapeHTML(usageData.qualification||"Operator accounting; estimated values are not provider billing.")}</p>`;
 const period=document.querySelector<HTMLSelectElement>("#usage-period")!;period.value=usagePeriod;period.addEventListener("change",()=>{usagePeriod=period.value;renderUsagePage();});
}
let providerBalancesData:Record<string,any>|null=null;
function combineProviderSnapshots(snapshots:any[]):any{
 const ordered=[...snapshots].sort((a,b)=>String(a.checked_at||"").localeCompare(String(b.checked_at||"")));
 const quota=Object.assign({},...ordered);
 const merge=(rows:any[],key:(row:any)=>string,prefer?:(old:any,next:any)=>any)=>{
  const entries=new Map<string,any>();
  for(const row of rows){const id=key(row),old=entries.get(id);entries.set(id,old&&prefer?prefer(old,row):row);}
  return [...entries.values()];
 };
 const windowKey=(w:any)=>JSON.stringify([w.name,w.unit,w.model,w.slot,w.window_type,w.window_seconds,w.source]);
 const preferWindow=(old:any,next:any)=>{
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
function renderProviderBalancesPage():void{
 const output=document.querySelector<HTMLElement>("#provider-balances-list")!;
 if(!providerBalancesData){output.innerHTML='<p class="section-note">Provider balances have not loaded yet.</p>';return;}
 const providerSnapshots=new Map<string,any>();
 for(const [id,status] of Object.entries(providerBalancesData)){
  const provider=id.split("/")[0];
  if(!providerSnapshots.has(provider))providerSnapshots.set(provider,{id:provider,snapshots:[]});
  providerSnapshots.get(provider).snapshots.push(status);
 }
 for(const model of providerSnapshots.values())model.quota=combineProviderSnapshots(model.snapshots);
 output.innerHTML=providerSnapshots.size?`<div class="budget-summary-list">${[...providerSnapshots.values()].map(model=>`<div class="budget-summary"><strong>${escapeHTML(model.id)}</strong>${modelTrackingMarkup(model)}</div>`).join("")}</div>`:'<div class="empty-state"><strong>No provider telemetry available</strong><p>Balances and subscription limits appear when providers report account telemetry.</p></div>';
}
async function loadProviderBalancesPage():Promise<void>{
 const output=document.querySelector<HTMLElement>("#provider-balances-list")!;const login=consoleSession;
 const button=document.querySelector<HTMLButtonElement>("#refresh-provider-balances")!;button.disabled=true;
 if(!providerBalancesData)output.innerHTML='<p class="section-note" role="status">Loading provider balances…</p>';
 try{
  const response=await gatewayFetch("/gateway/v1/usage",{headers:await authProvider.headers()});if(login!==consoleSession)return;
  if(!response.ok)throw new Error("Provider telemetry unavailable. Refresh to retry.");
  const data=await response.json();if(login!==consoleSession)return;
  providerBalancesData=data.provider_quotas||{};renderProviderBalancesPage();
 }catch(error){if(login===consoleSession)output.innerHTML=`<div class="empty-state error-state" role="alert"><strong>Unable to load provider balances</strong><p>${escapeHTML(error instanceof Error?error.message:"Provider telemetry unavailable")}</p></div>`;}
 finally{button.disabled=false;}
}
document.querySelector("#refresh-provider-balances")!.addEventListener("click",()=>{void loadProviderBalancesPage();});
async function loadUsagePage():Promise<void>{
 const output=document.querySelector<HTMLElement>("#usage-list")!;const login=consoleSession;const button=document.querySelector<HTMLButtonElement>("#refresh-usage")!;button.disabled=true;
 try{
 const keys=await gatewayFetch("/gateway/v1/admin/api-keys",{headers:await authProvider.headers()});if(login!==consoleSession)return;if(!keys.ok)throw new Error("Keys unavailable");const metadata=await keys.json();if(login!==consoleSession)return;usageKeyMetadata=metadata.api_keys||[];const selector=document.querySelector<HTMLSelectElement>("#budget-scope")!;const chosen=selector.value;
 selector.innerHTML='<option value="*">Entire gateway</option>'+usageKeyMetadata.map((key:any)=>`<option value="${escapeHTML(key.id)}">${escapeHTML(key.name)}</option>`).join("");selector.value=usageKeyMetadata.some(k=>k.id===chosen)?chosen:"*";renderBudgetEditor();
 const response=await gatewayFetch("/gateway/v1/usage",{headers:await authProvider.headers()});if(login!==consoleSession)return;if(!response.ok)throw new Error("Usage accounting unavailable. Refresh to retry.");const data=await response.json();if(login!==consoleSession)return;usageData=data;overviewUsage=data.usage||[];renderUsagePage();renderOverview();
 }catch(error){output.innerHTML=`<div class="empty-state error-state" role="alert"><strong>Unable to load usage</strong><p>${escapeHTML(error instanceof Error?error.message:"Usage unavailable")}</p></div>`;}finally{button.disabled=false;}
}
document.querySelector("#budget-scope")!.addEventListener("change",()=>{renderBudgetEditor();renderUsagePage();});
for(const period of ["daily","monthly"])for(const metric of ["requests","tokens","estimated_cost"]){document.querySelector<HTMLInputElement>(`#budget-${period}-${metric}`)!.addEventListener("change",()=>{
 const scope=document.querySelector<HTMLSelectElement>("#budget-scope")!.value||"*";const value=Number(document.querySelector<HTMLInputElement>(`#budget-${period}-${metric}`)!.value);if(!Number.isFinite(value)||value<0){saveState.textContent="Budgets must be nonnegative";return;}
 let budgets:Record<string,any>;if(scope==="*")budgets=budgetsDraft.gateway ||= {};else{budgetsDraft.keys ||= {};budgets=budgetsDraft.keys[scope] ||= {};}
 (budgets[period] ||= {})[metric]=value;markDirty();void saveConfig();
});}
document.querySelector("#refresh-usage")!.addEventListener("click",()=>{void loadUsagePage();});

function quotaResetLabel(raw:unknown):string{const date=new Date(String(raw||""));if(Number.isNaN(date.getTime())||date.getUTCFullYear()<2000)return "Reset time unavailable";const seconds=Math.max(0,Math.ceil((date.getTime()-Date.now())/1000));if(seconds===0)return `Reported reset passed · awaiting quota refresh · ${consoleDateTime(raw)}`;const hours=Math.floor(seconds/3600),minutes=Math.floor(seconds%3600/60);return `Resets in ${hours}h ${minutes}m · ${consoleDateTime(raw)}`;}
function renderProviderQuotas(quotas:Record<string,any>):string{return `<section class="overview-panel"><h3>Provider quotas</h3>${Object.entries(quotas).map(([id,q])=>`<div class="budget-summary"><strong>${escapeHTML(id)}</strong>${q.known?`<span>${Number(q.fraction_remaining*100).toFixed(1)}% remaining${q.exhausted?" · Exhausted":""}</span>${(q.windows||[]).map((w:any)=>`<span class="data-secondary">${escapeHTML(w.name)} · ${Number(100-w.used_percent).toFixed(1)}% remaining · <span data-quota-reset="${escapeHTML(w.reset_at)}">${escapeHTML(quotaResetLabel(w.reset_at))}</span></span>`).join("")}`:`<span class="data-secondary">${escapeHTML(q.error||"Provider account telemetry unavailable")}</span>`}</div>`).join("")}</section>`;}
window.setInterval?.(()=>{document.querySelectorAll<HTMLElement>("[data-quota-reset]").forEach(element=>{element.textContent=quotaResetLabel(element.dataset.quotaReset);});},30000);

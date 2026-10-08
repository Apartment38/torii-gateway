let budgetsDraft:Record<string,any>={};
function renderBudgetEditor():void {
 const scope=document.querySelector<HTMLSelectElement>("#budget-scope")!.value||"*";
 const budgets=scope==="*"?budgetsDraft.gateway:budgetsDraft.keys?.[scope];
 for(const period of ["daily","monthly"])for(const metric of ["requests","tokens","estimated_cost"]){document.querySelector<HTMLInputElement>(`#budget-${period}-${metric}`)!.value=String(budgets?.[period]?.[metric]||0);}
}
async function loadUsagePage():Promise<void>{
 const output=document.querySelector<HTMLElement>("#usage-list")!;
 try{
 const keys=await gatewayFetch("/gateway/v1/admin/api-keys",{headers:await authProvider.headers()});if(!keys.ok)throw new Error("Keys unavailable");const metadata=await keys.json();const selector=document.querySelector<HTMLSelectElement>("#budget-scope")!;const chosen=selector.value;
 selector.innerHTML='<option value="*">Entire gateway</option>'+(metadata.api_keys||[]).map((key:any)=>`<option value="${escapeHTML(key.id)}">${escapeHTML(key.name)}</option>`).join("");selector.value=chosen||"*";renderBudgetEditor();
 const response=await gatewayFetch("/gateway/v1/usage",{headers:await authProvider.headers()});if(!response.ok)throw new Error("Usage accounting unavailable");const data=await response.json();
 output.innerHTML=`<p>${escapeHTML(data.qualification||"")}</p><h3>Budgets and resets</h3>`+(data.budgets||[]).filter((b:any)=>b.limit.requests||b.limit.tokens||b.limit.estimated_cost).map((b:any)=>`<p>${escapeHTML(b.key_id==="*"?"Gateway":((metadata.api_keys||[]).find((k:any)=>k.id===b.key_id)?.name||b.key_id))} · ${escapeHTML(b.period)} · resets ${escapeHTML(b.reset_at)} · remaining requests ${b.limit.requests?b.remaining.requests:"unlimited"}, tokens ${b.limit.tokens?b.remaining.tokens:"unlimited"}, estimated USD ${b.limit.estimated_cost?Number(b.remaining.estimated_cost).toFixed(6):"unlimited"}</p>`).join("")+`<h3>Current UTC usage</h3><div class="benchmark-table-scroll"><table><thead><tr><th>Key</th><th>Period</th><th>Model</th><th>Client requests</th><th>Input / output tokens</th><th>Estimated tokens</th><th>Estimated USD</th></tr></thead><tbody>`+(data.usage||[]).filter((r:any)=>r.key_id!=="*").map((r:any)=>`<tr><td>${escapeHTML(r.key_name||r.key_id)}</td><td>${escapeHTML(r.period)}</td><td>${escapeHTML(r.model||"Client requests")}</td><td>${r.requests}</td><td>${r.input_tokens} / ${r.output_tokens}</td><td>${r.estimated_tokens}</td><td>${Number(r.estimated_cost).toFixed(6)}${r.unknown_cost_attempts?" + unknown costs":""}</td></tr>`).join("")+`</tbody></table></div>`;
 }catch(error){output.textContent=error instanceof Error?error.message:"Usage unavailable";}
}
document.querySelector("#budget-scope")!.addEventListener("change",renderBudgetEditor);
for(const period of ["daily","monthly"])for(const metric of ["requests","tokens","estimated_cost"]){document.querySelector<HTMLInputElement>(`#budget-${period}-${metric}`)!.addEventListener("change",()=>{
 const scope=document.querySelector<HTMLSelectElement>("#budget-scope")!.value||"*";const value=Number(document.querySelector<HTMLInputElement>(`#budget-${period}-${metric}`)!.value);if(!Number.isFinite(value)||value<0){saveState.textContent="Budgets must be nonnegative";return;}
 let budgets:Record<string,any>;if(scope==="*")budgets=budgetsDraft.gateway ||= {};else{budgetsDraft.keys ||= {};budgets=budgetsDraft.keys[scope] ||= {};}
 (budgets[period] ||= {})[metric]=value;markDirty();void saveConfig();
});}
document.querySelector("#refresh-usage")!.addEventListener("click",()=>{void loadUsagePage();});

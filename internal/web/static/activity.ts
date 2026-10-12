let activityStream:EventSource|null=null;
let activityRows:any[]=[];
let activityView="completed";
let activitySearch="";
let activityOutcomeFilter="";
let activityPage=0;
let activityPageSize=matchMedia("(max-width: 760px)").matches?5:20;
function visibleActivityRows():any[]{const query=activitySearch.toLowerCase();return activityRows.slice(-200).reverse().filter(e=>(activityView!=="completed"||e.kind==="request_complete")&&(!query||`${e.request_id||""} ${e.key_name||""} ${e.key_id||""} ${e.requested_model||""} ${e.selected_model||""} ${e.provider||""}`.toLowerCase().includes(query))&&(!activityOutcomeFilter||e.outcome===activityOutcomeFilter||activityOutcomeFilter==="error"&&e.status>=400));}
function activityOutcome(e:any):string{return e.outcome||String(e.status||({request_start:"Started",routing:"Model selected",gap:"History gap"} as Record<string,string>)[e.kind]||"Recorded");}
function stopActivity():void{closeActivityInspector();activityStream?.close();activityStream=null;activityRows=[];document.querySelector("#activity-list")!.textContent="";}
function renderActivity():void{
 const output=document.querySelector("#activity-list")!;const focused=document.activeElement?.id;const focusedRequest=(document.activeElement as HTMLButtonElement|null)?.dataset?.copyRequest;
 if(!activityRows.length){output.innerHTML='<div class="empty-state"><span class="empty-state-icon">↗</span><strong>Ready for your first request</strong><p>Request activity appears here as clients use the gateway. Debug details are captured only when enabled in Server settings.</p><a class="button secondary" href="#credentials">Manage client keys</a></div>';return;}
 // Keep view controls mounted during streaming updates so typing and focus survive.
 if(!output.querySelector("#activity-search")){
 output.innerHTML=`<div class="activity-toolbar console-filter"><label class="search-field" for="activity-search">Search requests<input id="activity-search" type="search" aria-label="Search activity" maxlength="256" placeholder="Model, request ID, or API key" value="${escapeHTML(activitySearch)}"></label><label for="activity-view">View<select id="activity-view" aria-label="Activity view"><option value="completed">Completed requests</option><option value="all">All events</option></select></label><label for="activity-outcome">Outcome<select id="activity-outcome" aria-label="Activity outcome"><option value="">All outcomes</option><option value="success">Success</option><option value="error">Error</option><option value="interrupted">Interrupted</option></select></label><span id="activity-count" class="section-note" role="status"></span></div><div id="activity-results"></div><div class="benchmark-pagination activity-pagination"><label for="activity-page-size" class="inline-filter">Rows<select id="activity-page-size" aria-label="Activity rows per page"><option value="5">5</option><option value="20">20</option><option value="50">50</option><option value="100">100</option></select></label><div><button id="activity-previous" class="button quiet" type="button">Previous</button><button id="activity-next" class="button quiet" type="button">Next</button></div></div>`;
 const search=document.querySelector<HTMLInputElement>("#activity-search")!;search.addEventListener("input",()=>{activitySearch=search.value;activityPage=0;renderActivity();});
 const view=document.querySelector<HTMLSelectElement>("#activity-view")!;view.value=activityView;view.addEventListener("change",()=>{activityView=view.value;activityPage=0;renderActivity();});
 const outcome=document.querySelector<HTMLSelectElement>("#activity-outcome")!;outcome.value=activityOutcomeFilter;outcome.addEventListener("change",()=>{activityOutcomeFilter=outcome.value;activityPage=0;renderActivity();});
 const size=document.querySelector<HTMLSelectElement>("#activity-page-size")!;size.value=String(activityPageSize);size.addEventListener("change",()=>{activityPageSize=[5,20,50,100].includes(Number(size.value))?Number(size.value):20;activityPage=0;renderActivity();});
 document.querySelector("#activity-previous")!.addEventListener("click",()=>{activityPage--;renderActivity();});document.querySelector("#activity-next")!.addEventListener("click",()=>{activityPage++;renderActivity();});
 }
 const filtered=visibleActivityRows();activityPage=Math.min(Math.max(0,activityPage),Math.max(0,Math.ceil(filtered.length/activityPageSize)-1));const rows=filtered.slice(activityPage*activityPageSize,(activityPage+1)*activityPageSize);
 const results=document.querySelector("#activity-results")!;
 results.innerHTML=`<div class="benchmark-table-scroll"><table class="activity-table"><thead><tr><th scope="col">Time / request</th><th scope="col">Event</th><th scope="col">API key</th><th scope="col">Model selection</th><th scope="col">Outcome</th><th scope="col">Duration</th><th scope="col">Input / output</th><th scope="col">Est. USD</th>${activityDebugColumns()}</tr></thead><tbody>`+rows.map((e:any)=>`<tr><td data-label="Time / request"><time title="${escapeHTML(e.at||"")}">${consoleTime(e.at)}</time><span class="data-secondary">${consoleDate(e.at)}</span><button class="request-id-copy" type="button" data-copy-request="${escapeHTML(e.request_id||"")}" title="Copy request ID: ${escapeHTML(e.request_id||"")}" aria-label="Copy request ID ${escapeHTML(e.request_id||"")}">${escapeHTML(e.request_id?e.request_id.slice(0,12)+"…":"—")}</button></td><td data-label="Event">${escapeHTML(String(e.kind||"—").replace(/_/g," "))}</td><td data-label="API key">${escapeHTML(e.key_name||e.key_id||"—")}</td><td data-label="Model selection"><strong>${escapeHTML(e.selected_model||e.requested_model||"—")}</strong><span class="data-secondary">${e.requested_model&&e.selected_model&&e.requested_model!==e.selected_model?`From ${escapeHTML(e.requested_model)} · `:""}${escapeHTML(e.provider||"")}</span></td><td data-label="Outcome"><span class="status-badge ${e.outcome==="success"||e.status&&e.status<400?"success":e.outcome==="error"||e.outcome==="failure"||e.outcome==="interrupted"||e.status>=400?"failure":"neutral"}">${escapeHTML(activityOutcome(e))}</span>${e.outcome&&e.status?`<span class="data-secondary">HTTP ${escapeHTML(String(e.status))}</span>`:""}</td><td class="numeric" data-label="Duration">${typeof e.duration_ms==="number"?`${consoleNumber(e.duration_ms)} ms`:"—"}</td><td class="numeric" data-label="Input / output">${consoleNumber(e.input_tokens||0)} / ${consoleNumber(e.output_tokens||0)}${e.estimated_tokens?'<span class="data-secondary">Estimated</span>':""}</td><td class="numeric" data-label="Estimated USD">${e.unknown_cost?'<span class="data-secondary">Unknown</span>':e.estimated_cost==null?"—":Number(e.estimated_cost).toFixed(6)}</td>${activityDebugButtons(e)}</tr>`).join("")+`${rows.length?"":`<tr><td colspan="${8+(loadedConfiguration.activity?.prompt_logging?1:0)+(loadedConfiguration.activity?.jev_decision_logging?1:0)}"><div class="empty-state"><strong>No requests match this view</strong><p>Try another search or clear your outcome filter. All events includes starts and routing decisions.</p></div></td></tr>`}</tbody></table></div><p class="section-note">${activityView==="completed"?"Completed requests from":"Showing"} the latest ${activityRows.length} recorded events. Timestamps use ${escapeHTML(consoleTimezoneLabel())}; cost is estimated.</p>`;
 results.querySelectorAll<HTMLButtonElement>("[data-inspect-activity]").forEach(button=>button.addEventListener("click",()=>openActivityInspector(Number(button.dataset.inspectActivity),button.dataset.inspectKind!)));
 document.querySelector("#activity-count")!.textContent=`${filtered.length?activityPage*activityPageSize+1:0}–${Math.min((activityPage+1)*activityPageSize,filtered.length)} of ${filtered.length} ${activityView==="completed"?"requests":"events"}`;
 document.querySelector<HTMLButtonElement>("#activity-previous")!.disabled=activityPage===0;document.querySelector<HTMLButtonElement>("#activity-next")!.disabled=(activityPage+1)*activityPageSize>=filtered.length;
 if(focused==="activity-next"&&document.querySelector<HTMLButtonElement>("#activity-next")!.disabled)(document.querySelector<HTMLButtonElement>("#activity-previous")!.disabled?document.querySelector<HTMLInputElement>("#activity-search"):document.querySelector<HTMLButtonElement>("#activity-previous"))?.focus?.();
 if(focused==="activity-previous"&&document.querySelector<HTMLButtonElement>("#activity-previous")!.disabled)(document.querySelector<HTMLButtonElement>("#activity-next")!.disabled?document.querySelector<HTMLInputElement>("#activity-search"):document.querySelector<HTMLButtonElement>("#activity-next"))?.focus?.();
 if(focusedRequest)Array.from(results.querySelectorAll<HTMLButtonElement>("[data-copy-request]")).find(button=>button.dataset.copyRequest===focusedRequest)?.focus?.();
 results.querySelectorAll<HTMLButtonElement>("[data-copy-request]").forEach(button=>button.addEventListener("click",async()=>{try{await navigator.clipboard.writeText(button.dataset.copyRequest||"");toast("Request ID copied");}catch{toast("Copy unavailable. The complete request ID is shown in the button tooltip.");}}));
}

async function startActivity():Promise<void>{
 if(activityStream || !consoleSession)return;
 const status=document.querySelector("#activity-status")!;const login=consoleSession;
 try{const response=await gatewayFetch("/gateway/v1/activity?limit=200",{headers:await authProvider.headers()});if(login!==consoleSession)return;if(!response.ok)throw new Error();const history=await response.json();if(login!==consoleSession)return;activityRows=history.events||[];renderActivity();
 if(location.hash!=="#activity" || !consoleSession || activityStream)return;
 const last=activityRows.at(-1)?.id||0;const stream=new EventSource(`/gateway/v1/activity/events?after_id=${last}`);activityStream=stream;
 stream.onopen=()=>{status.textContent="Live · request activity";status.className="status-badge success";};stream.onerror=()=>{status.textContent="Reconnecting · replaying available events";status.className="status-badge neutral";};
 stream.addEventListener("activity",event=>{const e=JSON.parse((event as MessageEvent).data);if(e.kind==="history_reset"){closeActivityInspector();void refreshActivityHistory();return;}if(!activityRows.some(row=>row.id===e.id)){activityRows.push(e);activityRows=activityRows.slice(-200);renderActivity();}});
 stream.addEventListener("gap",()=>{status.textContent="History gap · some older events are unavailable";status.className="status-badge neutral";});
 }catch{status.textContent="Activity unavailable. Sign in and reopen this page to retry.";status.className="status-badge failure";}
}

function activityDebugColumns():string{return `${loadedConfiguration.activity?.prompt_logging?'<th scope="col">Prompt</th>':""}${loadedConfiguration.activity?.jev_decision_logging?'<th scope="col">Jev decision</th>':""}`;}
function activityDebugButtons(e:any):string{
 const button=(kind:string,label:string,available:boolean)=>`<td data-label="${label}"><button type="button" class="button quiet" data-inspect-activity="${e.id}" data-inspect-kind="${kind}" ${available?"":"disabled"} title="${available?`Inspect ${label.toLowerCase()}`:"Details were not recorded for this event"}">Inspect ${label.toLowerCase()}</button></td>`;
 return `${loadedConfiguration.activity?.prompt_logging?button("prompt","Prompt",Boolean(e.prompt)):""}${loadedConfiguration.activity?.jev_decision_logging?button("jev","Jev decision",Boolean(e.jev_request)):""}`;
}
function openActivityInspector(id:number,kind:string):void{
 const e=activityRows.find(row=>row.id===id);if(!e)return;
 if(kind==="prompt"&&!loadedConfiguration.activity?.prompt_logging||kind==="jev"&&!loadedConfiguration.activity?.jev_decision_logging)return;
 const dialog=document.querySelector<HTMLDialogElement>("#activity-inspector-dialog")!;
 const title=document.querySelector<HTMLElement>("#activity-inspector-title")!;
 title.textContent=kind==="prompt"?"API prompt":"Jev decision";
 document.querySelector("#activity-inspector-note")!.textContent=e.debug_truncated?"Debug capture was truncated at 64 KiB per body.":"Original API body as captured. HTTP authorization headers are not recorded.";
 const block=(label:string,body:string)=>`<section><h3>${label}</h3><pre class="activity-debug-body">${escapeHTML(body)}</pre></section>`;
 document.querySelector("#activity-inspector-content")!.innerHTML=kind==="prompt"?block("API request",e.prompt||"Prompt was not recorded."):block("Request sent to Jev",e.jev_request||"No Jev request was recorded.")+block("Jev response",e.jev_response||"No response was received.");
 dialog.showModal();title.focus?.();
}
function closeActivityInspector():void{document.querySelector<HTMLDialogElement>("#activity-inspector-dialog")?.close?.();document.querySelector("#activity-inspector-content")!.textContent="";}
document.querySelector("#close-activity-inspector")!.addEventListener("click",closeActivityInspector);
document.querySelector("#activity-inspector-dialog")!.addEventListener("close",()=>{document.querySelector("#activity-inspector-content")!.textContent="";});

let activityHistorySequence=0;
async function refreshActivityHistory():Promise<void>{
 const sequence=++activityHistorySequence,login=consoleSession,stream=activityStream;
 try{const response=await gatewayFetch("/gateway/v1/activity?limit=200",{headers:await authProvider.headers()});if(!response.ok)return;const history=await response.json();if(sequence!==activityHistorySequence||login!==consoleSession||stream!==activityStream)return;activityRows=history.events||[];renderActivity();}catch{document.querySelector("#activity-status")!.textContent="Activity refresh unavailable. Reopen this page to retry.";}
}

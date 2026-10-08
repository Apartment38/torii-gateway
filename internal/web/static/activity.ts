let activityStream:EventSource|null=null;
let activityRows:any[]=[];
function stopActivity():void{activityStream?.close();activityStream=null;activityRows=[];document.querySelector("#activity-list")!.textContent="";}
function renderActivity():void{
 document.querySelector("#activity-list")!.innerHTML=`<div class="benchmark-table-scroll"><table><thead><tr><th>Time / request</th><th>Event</th><th>API key</th><th>Requested → selected</th><th>Outcome</th><th>Duration</th><th>Tokens</th><th>Estimated USD</th></tr></thead><tbody>`+activityRows.slice(-200).reverse().map((e:any)=>`<tr><td>${escapeHTML(e.at||"")}<br>${escapeHTML(e.request_id||"")}</td><td>${escapeHTML(e.kind)}</td><td>${escapeHTML(e.key_name||e.key_id||"")}</td><td>${escapeHTML(e.requested_model||"")} → ${escapeHTML(e.selected_model||"")}<br>${escapeHTML(e.provider||"")}</td><td>${escapeHTML(e.outcome||"")} ${e.status||""}</td><td>${e.duration_ms??""} ms</td><td>${e.input_tokens||0} / ${e.output_tokens||0}${e.estimated_tokens?" (estimated)":""}</td><td>${e.unknown_cost?"unknown":e.estimated_cost==null?"—":Number(e.estimated_cost).toFixed(6)}</td></tr>`).join("")+`</tbody></table></div>`;
}
async function startActivity():Promise<void>{
 if(activityStream || !consoleSession)return;
 const status=document.querySelector("#activity-status")!;
 try{const response=await gatewayFetch("/gateway/v1/activity?limit=200",{headers:await authProvider.headers()});if(!response.ok)throw new Error();const history=await response.json();activityRows=history.events||[];renderActivity();
 if(location.hash!=="#activity" || !consoleSession)return;
 const last=activityRows.at(-1)?.id||0;const stream=new EventSource(`/gateway/v1/activity/events?after_id=${last}`);activityStream=stream;
 stream.onopen=()=>{status.textContent="Live · metadata only";};stream.onerror=()=>{status.textContent="Disconnected · reconnecting and replaying available events";};
 stream.addEventListener("activity",event=>{const e=JSON.parse((event as MessageEvent).data);if(!activityRows.some(row=>row.id===e.id)){activityRows.push(e);activityRows=activityRows.slice(-200);renderActivity();}});
 stream.addEventListener("gap",()=>{status.textContent="History gap · events were pruned or a viewer/recording queue overflowed";});
 }catch{status.textContent="Activity unavailable. Sign in and retry.";}
}

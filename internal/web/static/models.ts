let modelEnabledDraft:Record<string,boolean>={};
async function loadModelsPage():Promise<void>{
 const output=document.querySelector<HTMLElement>("#models-list")!;
 try{const response=await gatewayFetch("/gateway/v1/models",{headers:await authProvider.headers()});if(!response.ok)throw new Error("Model discovery unavailable");const data=await response.json();
 output.innerHTML=(data.models||[]).map((model:any)=>{
  const benchmark=model.benchmark;
  const scores=benchmark?Object.entries(benchmark.scores||{}).map(([name,value])=>`${escapeHTML(name)}: ${(Number(value)*100).toFixed(1)}%`).join(" · "):"";
  const pricing=model.pricing||{};
  return `<article class="subcard"><h3>${escapeHTML(model.id)}</h3><p>Provider: ${escapeHTML(model.provider_id)}</p><label class="checkbox"><input type="checkbox" data-model-enabled="${escapeHTML(model.id)}" ${model.enabled?"checked":""}> Enabled</label><p>Capabilities: ${Object.entries(model.capabilities||{}).map(([name,value])=>`${escapeHTML(name)}: ${escapeHTML(String(value))}`).join(" · ")}</p><p>Estimated USD per million tokens · input: ${pricing.input_per_million??"unknown"} · output: ${pricing.output_per_million??"unknown"}</p><p>${model.benchmark_missing?"No safely matched benchmark scores available":scores}</p>${benchmark?`<p>Sources: ${(benchmark.sources||[]).map(escapeHTML).join(", ")} · ${benchmark.stale?"Stale":"Current"} · retrieved ${escapeHTML(benchmark.latest_retrieval||"unknown")} · evaluated ${escapeHTML(benchmark.latest_evaluation||"unknown")}</p>`:""}</article>`;
 }).join("")||"No physical models discovered. Add a provider first.";
 output.querySelectorAll<HTMLInputElement>("[data-model-enabled]").forEach(input=>input.addEventListener("change",async()=>{modelEnabledDraft[input.dataset.modelEnabled!]=input.checked;markDirty();if(await saveConfig())void loadModelsPage();}));
 }catch(error){output.textContent=error instanceof Error?error.message:"Models unavailable";}
}

document.querySelector("#refresh-models")!.addEventListener("click",()=>{void loadModelsPage();});

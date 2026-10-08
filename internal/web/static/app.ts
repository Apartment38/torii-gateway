type AuthHeaders = Record<string, string>;

// Browser authentication uses an HttpOnly cookie. Only the CSRF token is in memory.
interface AuthProvider {
  headers(): Promise<AuthHeaders>;
}

class CookieSessionProvider implements AuthProvider {
  async headers(): Promise<AuthHeaders> { return csrfToken ? { "X-CSRF-Token":csrfToken } : {}; }
}

const authProvider: AuthProvider = new CookieSessionProvider();
async function gatewayFetch(url:string,options?:RequestInit):Promise<Response>{
  const login = consoleSession;
  const response=await fetch(url,options);
  if(response.status===401 && login===consoleSession){expireConsoleSession();document.querySelector<HTMLDialogElement>("#login-dialog")?.showModal?.();}
  return response;
}
const form = document.querySelector<HTMLFormElement>("#config-form")!;
const providerList = document.querySelector<HTMLDivElement>("#providers-list")!;
const saveState = document.querySelector<HTMLSpanElement>("#save-state")!;
let consoleSession = "";
let csrfToken="";
let configurationRevision = "";
let loadedConfiguration:Record<string,any> = {};
let editGeneration=0;
let saveRunning=false;
let saveQueued=false;
let saveConflict=false;
let editingProvider:number|null=null;
let editingOriginal:Record<string,any>|null=null;
function openProviderEditor(index:number,adding=false):void{editingProvider=index;editingOriginal=adding?null:structuredClone(providers[index]);providerList.querySelector<HTMLDialogElement>(`[data-provider-dialog="${index}"]`)?.showModal?.();}
function cancelProviderEditor():void{if(editingProvider===null)return;if(editingOriginal)providers[editingProvider]=editingOriginal;else providers.splice(editingProvider,1);editingProvider=null;editingOriginal=null;renderProviders();}
let signedInUsername = "";
let benchmarksEnabledSaved = false;
function updateConsoleIdentity(): void {
  const signedIn = Boolean(consoleSession && signedInUsername);
  document.querySelector<HTMLButtonElement>("#console-login")!.hidden = signedIn;
  document.querySelector<HTMLButtonElement>("#console-logout")!.hidden = !signedIn;
  document.querySelector("#signed-in-user")!.textContent = signedIn ? signedInUsername : "Not signed in";
}
function expireConsoleSession():void{
 stopActivity();consoleSession="";csrfToken="";signedInUsername="";clearNewAPIKey();if(configurationLoaded)document.querySelector<HTMLButtonElement>("#save-button")!.hidden=false;updateConsoleIdentity();updateBenchmarkControls();saveState.textContent="Session expired. Sign in to retry; your draft is retained.";
}
function signOutConsole(): void {
 stopActivity();
  consoleSession = "";csrfToken="";loadedConfiguration={};editingProvider=null;editingOriginal=null;
  configurationRevision = "";
  signedInUsername = "";
  clearNewAPIKey();
  document.querySelector<HTMLDialogElement>("#new-key-dialog")!.close();
  document.querySelector<HTMLDialogElement>("#create-key-dialog")!.close();
  document.querySelector<HTMLInputElement>("#client-key-name")!.value = "";
  providers = [];
  configurationLoaded = false;
  classifierQuestionOverrides = {};
  classifierNoulThresholds = {};
  renderClassifierQuestion();
  form.reset();
  clearBenchmarkConsole();
  benchmarksEnabledSaved = false;
  document.querySelector("#credential-list")!.textContent = "";
  document.querySelector("#benchmark-status")!.textContent = "";
  updateConsoleIdentity();
  updateBenchmarkControls();
  renderProviders();
  saveState.textContent = "Signed out";
}
async function createBrowserSession(username:string,password:string):Promise<void>{
 const response=await fetch("/gateway/v1/session",{method:"POST",credentials:"same-origin",headers:{"Content-Type":"application/json"},body:JSON.stringify({username,password})});
 if(!response.ok)throw new Error("Sign-in failed");const session=await response.json();csrfToken=session.csrf_token;consoleSession=session.expires_at;signedInUsername=session.username;updateConsoleIdentity();
}

const providerDefaults = { id: "", type: "openai_compatible", api_url: "", api_key: "", api_key_env: "", api_key_file: "", management_env: "", portal_url: "", client_id: "", headers: "", models: "", discover: true, transport: "", organization: "", project: "", rewrites: "", tls_ca: "", tls_server: "", connect_timeout: "", request_timeout: "", discovery_timeout: "", enabled:true, treat_models_as_free:false };
let providers: Record<string, string | boolean>[] = [];
interface ProviderStatus {
  state: "unchecked" | "pending" | "success" | "failure";
  message: string;
}
const providerStatuses = new WeakMap<Record<string, string | boolean>, ProviderStatus>();
const classifierCatalogs = new WeakMap<Record<string, string | boolean>, string[]>();
let classifierDiscoveryPending = false;
let configurationLoaded = false;
let requestedClassifier = "";
let legacyClassifier: Record<string, any> | null = null;
const classifierSelect = document.querySelector<HTMLSelectElement>("#classifier-model")!;
// Mirrors the typed protocol's defaults. Only edited entries are saved.
const classifierQuestionDefaults: Record<string, { instructions:string; criteria:Record<string,string> }> = {
  task_type: { instructions:"Which task type best describes this request?", criteria:{ general:"General knowledge or other task", conversation:"Open-ended conversation", coding:"Writing or debugging code", sysadmin:"Operating systems, infrastructure, or deployment", reasoning:"Multi-step logical reasoning", math:"Mathematics or quantitative calculation", research:"Research or source synthesis", summarization:"Summarizing provided content", extraction:"Extracting structured facts", creative:"Creative writing or ideation", translation:"Translating between languages" } },
  complexity: { instructions:"How complex is this request?", criteria:{ low:"Simple and direct", medium:"Requires several steps or moderate judgment", high:"Requires deep reasoning, extensive context, or difficult execution" } },
  tools: { instructions:"Does this request require tool or function calling?", criteria:{ true:"", false:"" } },
  vision: { instructions:"Does this request require understanding image content?", criteria:{ true:"", false:"" } },
  reasoning: { instructions:"Does this request require a reasoning-capable model?", criteria:{ true:"", false:"" } },
  structured_output: { instructions:"Does this request require structured or schema-constrained output?", criteria:{ true:"", false:"" } },
  latency_sensitive: { instructions:"Is low latency especially important for this request?", criteria:{ true:"", false:"" } },
};
let classifierQuestionOverrides: Record<string, {instructions?:string; criteria?:Record<string,string>}> = {};
let classifierNoulThresholds: Record<string, number> = {};
const classifierQuestion = document.querySelector<HTMLSelectElement>("#classifier-question")!;
const classifierInstructions = document.querySelector<HTMLTextAreaElement>("#classifier-question-instructions")!;
const classifierOption = document.querySelector<HTMLSelectElement>("#classifier-question-option")!;
const classifierRubric = document.querySelector<HTMLTextAreaElement>("#classifier-question-rubric")!;
const classifierThreshold = document.querySelector<HTMLInputElement>("#classifier-question-threshold")!;
function renderClassifierRubric(): void {
  const name = classifierQuestion.value || "task_type";
  classifierRubric.value = classifierQuestionOverrides[name]?.criteria?.[classifierOption.value] ?? classifierQuestionDefaults[name].criteria[classifierOption.value] ?? "";
  classifierRubric.placeholder = "Use the built-in rubric";
}
function renderClassifierQuestion(): void {
  const name = classifierQuestion.value || "task_type";
  const defaults = classifierQuestionDefaults[name];
  classifierInstructions.value = classifierQuestionOverrides[name]?.instructions ?? defaults.instructions;
  classifierOption.innerHTML = Object.keys(defaults.criteria).map(option => `<option value="${option}">${option}</option>`).join("");
  classifierOption.value = Object.keys(defaults.criteria)[0];
  const isNoul = name !== "task_type" && name !== "complexity";
  document.querySelector<HTMLLabelElement>("#classifier-threshold-label")!.hidden = !isNoul;
  classifierThreshold.value = name in classifierNoulThresholds ? String(classifierNoulThresholds[name]) : "";
  renderClassifierRubric();
}
function editClassifierQuestion(field:"instructions"|"criteria", value:string): void {
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

function classifierProvider(provider: Record<string, string | boolean>): boolean {
  return provider.type === "jev" || provider.type === "jev_compatible";
}
function availableClassifiers(): string[] {
  return providers.filter(provider => classifierProvider(provider) && providerStatuses.get(provider)?.state === "success")
    .flatMap(provider => classifierCatalogs.get(provider) || []);
}
function updateClassifierOptions(): void {
  const models = availableClassifiers();
  const pending = classifierDiscoveryPending || providers.some(provider => classifierProvider(provider) && providerStatuses.get(provider)?.state === "pending");
  const existing = requestedClassifier && requestedClassifier !== "disabled" && !models.includes(requestedClassifier);
  classifierSelect.innerHTML = (models.length === 0 ? `<option value="">${pending ? "Discovering classifier models…" : "No classifier models available"}</option>` : "") + `<option value="disabled">Disabled · deterministic routing only</option>` +
    (existing ? `<option value="${escapeHTML(requestedClassifier)}">${legacyClassifier ? "Existing classifier" : "Selected classifier unavailable"} · ${escapeHTML(requestedClassifier)}</option>` : "") +
    models.map(model => `<option value="${escapeHTML(model)}">${escapeHTML(model)}</option>`).join("");
  classifierSelect.value = requestedClassifier || models[0] || "";
  classifierSelect.disabled = pending && models.length === 0;
  const notice = document.querySelector<HTMLDivElement>("#classifier-notice")!;
  const message = document.querySelector<HTMLSpanElement>("#classifier-notice-message")!;
  notice.hidden = models.length > 0 && !existing;
  document.querySelector<HTMLDivElement>("#classifier-provider-notice")!.hidden = pending || models.length > 0;
  message.textContent = pending ? "Checking classifier providers…" : models.length === 0
    ? "No classifier models are available. Add a TypeSafe Jev or Jev-compatible provider (such as Laya) on the Providers page, then test its connection. Deterministic routing remains available."
    : "The selected classifier is unavailable. Check its provider or choose another discovered model.";
  if (configurationLoaded && !pending && models.length === 0 && location.hash === "#routing") {
    toast("No classifier models available. Add a Jev or Jev-compatible provider on the Providers page.");
    location.hash = "#providers";
    navigate();
  }
}

function setProviderStatus(provider: Record<string, string | boolean>, status: ProviderStatus): void {
  status.message = redactProviderKeys(status.message);
  providerStatuses.set(provider, status);
  const index = providers.indexOf(provider);
  if (index < 0) return;
  const result = providerList.querySelector<HTMLDivElement>(`[data-result="${index}"]`);
  if (result) {
    result.className = `provider-result ${status.state}`;
    result.textContent = status.message;
  }
  const button = providerList.querySelector<HTMLButtonElement>(`[data-test="${index}"]`);
  if (button) button.disabled = status.state === "pending";
  updateClassifierOptions();
}

function redactProviderKeys(message: string): string {
  providers.forEach(provider => {
    const key = String(provider.api_key || "");
    if (key) message = message.split(key).join("[redacted]");
  });
  return benchmarkSafeMessage(message);
}

function escapeHTML(value: string): string { return value.replace(/[&<>"]/g, ch => ({"&":"&amp;", "<":"&lt;", ">":"&gt;", '"':"&quot;"}[ch]!)); }
function value(provider: Record<string, string | boolean>, key: string): string { return escapeHTML(String(provider[key] ?? "")); }
function checked(provider: Record<string, string | boolean>, key: string): string { return provider[key] === false ? "" : "checked"; }
function subscriptionProvider(provider: Record<string, string | boolean>): boolean { return provider.type === "openai-codex" || provider.type === "nous"; }
const knownProviderIDs: Record<string, string> = { openai: "openai", "openai-codex": "openai-codex", anthropic: "anthropic", gemini: "gemini", xai: "xai", openrouter: "openrouter", ollama: "ollama", nous: "nous", jev: "jev" };
function knownProviderID(type: string, index: number): string {
  const base = knownProviderIDs[type];
  if (!base) return "";
  const used = providers.filter((provider, providerIndex) => providerIndex !== index && String(provider.id || "").startsWith(base)).map(provider => String(provider.id));
  if (!used.includes(base)) return base;
  let suffix = 2;
  while (used.includes(`${base}-${suffix}`)) suffix += 1;
  return `${base}-${suffix}`;
}
function isKnownProvider(provider: Record<string, string | boolean>): boolean { return Boolean(knownProviderIDs[String(provider.type)]); }
function initializeTooltips(): void {
  document.querySelectorAll<HTMLLabelElement>("label[data-tooltip]").forEach(label => {
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

function renderProviders(): void {
  document.querySelector<HTMLDivElement>("#providers-empty-notice")!.hidden = providers.length > 0;
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
        <label data-tooltip="An existing encrypted provider credential reference. Gateway client API keys cannot be used here.">Stored API credential<input data-provider="${index}" data-key="credential_id" value="${escapeHTML(String((provider.bearer_token as any)?.credential || provider.credential_id || ""))}" placeholder="Credential ID" /></label>
        <label data-tooltip="An existing encrypted management credential used for quota lookup. This is separate from gateway client API keys.">Management credential<input data-provider="${index}" data-key="management_id" value="${escapeHTML(String((provider.management_token as any)?.credential || provider.management_id || ""))}" placeholder="Credential ID" /></label>
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
  providerList.querySelectorAll<HTMLButtonElement>("[data-edit]").forEach(button=>button.addEventListener("click",()=>{openProviderEditor(Number(button.dataset.edit));}));
  providerList.querySelectorAll<HTMLButtonElement>("[data-confirm-provider]").forEach(button=>button.addEventListener("click",async()=>{const dialog=button.closest("dialog")!;const index=editingProvider;editingProvider=null;markDirty();if(await saveConfig()){editingOriginal=null;dialog.close();}else{editingProvider=index;}}));
  providerList.querySelectorAll<HTMLButtonElement>("[data-cancel-provider]").forEach(button=>button.addEventListener("click",cancelProviderEditor));
  providerList.querySelectorAll<HTMLDialogElement>("[data-provider-dialog]").forEach(dialog=>dialog.addEventListener("cancel",cancelProviderEditor));
  providerList.querySelectorAll<HTMLButtonElement>("[data-remove]").forEach(button => button.addEventListener("click", () => { providers.splice(Number(button.dataset.remove), 1); renderProviders(); markDirty();void saveConfig(); }));
  providerList.querySelectorAll<HTMLButtonElement>("[data-test]").forEach(button => button.addEventListener("click", () => { void testProvider(Number(button.dataset.test)); }));
  providerList.querySelectorAll<HTMLButtonElement>("[data-login]").forEach(button => button.addEventListener("click", () => { void loginProvider(Number(button.dataset.login), button); }));
  providerList.querySelectorAll<HTMLInputElement | HTMLSelectElement>("[data-provider]").forEach(control => control.addEventListener("input", updateProvider));
  providerList.querySelectorAll<HTMLInputElement>('[data-key="api_key"]').forEach(input => {
    // Assign the property rather than including credentials in HTML attributes.
    input.value = String(providers[Number(input.dataset.provider)].api_key || "");
  });
  providerList.querySelectorAll<HTMLButtonElement>("[data-reveal-provider-key]").forEach(button => button.addEventListener("click", () => {
    const input = providerList.querySelector<HTMLInputElement>(`#provider-api-key-${button.dataset.revealProviderKey}`)!;
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
  if(editingProvider!==null)providerList.querySelector<HTMLDialogElement>(`[data-provider-dialog="${editingProvider}"]`)?.showModal?.();
}

function updateProvider(event: Event): void {
  const control = event.target as HTMLInputElement | HTMLSelectElement;
  const index = Number(control.dataset.provider);
  const key = control.dataset.key!;
  const previousType = providers[index].type;
  providers[index][key] = control instanceof HTMLInputElement && control.type === "checkbox" ? control.checked : control.value;
  if (["api_key", "api_key_env", "api_key_file"].includes(key)) {
    delete providers[index].bearer_token;
    ["api_key", "api_key_env", "api_key_file"].filter(source => source !== key).forEach(source => {
      providers[index][source] = "";
      const input = providerList.querySelector<HTMLInputElement>(`[data-provider="${index}"][data-key="${source}"]`);
      if (input) input.value = "";
    });
  }
  if (key === "management_env") delete providers[index].management_token;
  if(key==="credential_id"){providers[index].api_key="";providers[index].api_key_env="";providers[index].api_key_file="";providers[index].bearer_token=control.value.trim()?{credential:control.value.trim()} as any:undefined as any;}
  if(key==="management_id"){providers[index].management_token=control.value.trim()?{credential:control.value.trim()} as any:undefined as any;}
  setProviderStatus(providers[index], { state: "unchecked", message: "Settings changed · check connection again" });
  if (key === "id") {
    const title = providerList.querySelector<HTMLElement>(`[data-provider-title="${index}"]`);
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

function markDirty(): void { editGeneration++; saveState.textContent = "Draft changes"; }

// Recognize only the exact inert provider generated by earlier console bootstraps.
// Real providers (even those named setup) must remain visible and testable.
function isSetupPlaceholder(provider: Record<string, any>): boolean {
  const fields = ["id", "type", "base_url", "api_path", "discover_models", "models"];
  return provider.id === "setup" && provider.type === "openai_compatible"
    && provider.base_url === "https://example.invalid" && provider.api_path === "/v1"
    && provider.discover_models === false && Array.isArray(provider.models)
    && provider.models.length === 1 && provider.models[0] === "setup"
    && Object.keys(provider).every(key => fields.includes(key));
}

function applyConfig(config: Record<string, any>, revision = ""): void {
  configurationRevision = revision;
  loadedConfiguration=structuredClone(config);modelEnabledDraft={...config.model_enabled};budgetsDraft=structuredClone(config.budgets||{});renderBudgetEditor();saveConflict=false;
  classifierDiscoveryPending = true;
  configurationLoaded = true;
  legacyClassifier = null;
  const classifier = config.router?.classifier || {};
  (form.elements.namedItem("router.allow_paid_models") as HTMLInputElement).checked = config.router?.allow_paid_models !== false;
  classifierQuestionOverrides = structuredClone(classifier.questions || {});
  classifierNoulThresholds = { ...(classifier.noul_thresholds || {}) };
  (form.elements.namedItem("router.classifier.timeout") as HTMLInputElement).value = "";
  (form.elements.namedItem("router.classifier.minimum_confidence") as HTMLInputElement).value = "";
  (form.elements.namedItem("router.classifier.on_failure") as HTMLSelectElement).value = "";
  renderClassifierQuestion();
  requestedClassifier = classifier.type === "disabled" ? "disabled" : classifier.type === "jev" && classifier.transport === "provider"
    ? `${classifier.provider}/${classifier.model}` : "";
  applyBenchmarkConfig(config.intelligence || {});
  providers = (config.providers || []).filter((provider: Record<string, any>) => !isSetupPlaceholder(provider)).map((provider: Record<string, any>) => ({
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
  const fill = (value: any, prefix = ""): void => Object.entries(value || {}).forEach(([key, raw]) => {
    const name = prefix ? `${prefix}.${key}` : key;
    if (raw && typeof raw === "object" && !Array.isArray(raw)) fill(raw, name);
    else {
      const control = form.elements.namedItem(name) as HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement | null;
      if (control) control.type === "checkbox" ? control.checked = Boolean(raw) : control.value = String(raw);
    }
  });
  fill(config);
  const allowances = config.router?.quota?.allowances || {};
  (form.elements.namedItem("router.quota.allowances") as HTMLTextAreaElement).value = Object.entries(allowances).map(([key, raw]) => `${key} = ${raw}`).join("\n");
  const quality = config.router?.selection?.minimum_quality;
  if (quality) (form.elements.namedItem("router.selection.minimum_quality") as HTMLInputElement).value = JSON.stringify(quality);
  updateClassifierOptions();
  if (location.hash === "#intelligence" && benchmarksEnabledSaved) void loadBenchmarkData();
}

async function loadServerConfig(): Promise<void> {
  const login = consoleSession;
  try {
    const response = await gatewayFetch("/gateway/v1/config", { headers: await authProvider.headers() });
    if (login !== consoleSession) return;
    if (!response.ok) { if (response.status === 401 || response.status === 403) {signOutConsole();saveState.textContent="Sign in to configure the gateway";document.querySelector<HTMLDialogElement>("#login-dialog")?.showModal?.();} return; }
    const configuration = await response.json();
    if (login !== consoleSession) return;
    applyConfig(configuration, response.headers.get("ETag") || "");
    saveState.textContent = "Loaded from gateway";
    void testAllProviders();
  } catch { saveState.textContent = "Gateway config unavailable"; }
}

async function saveConfig(): Promise<boolean> {
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
        providers.forEach(p=>{const submitted=configuration.providers.find((q:any)=>q.id===p.id);const stored=committed.providers.find((q:any)=>q.id===p.id);if(stored && submitted?.bearer_token?.value===p.api_key){p.bearer_token=stored.bearer_token;p.api_key="";}});
      }
      saveState.textContent=saveQueued?"Saving…":result.restart_required?"Saved · restart required for listener or file logging changes":"Saved";
      document.querySelector<HTMLButtonElement>("#save-button")!.hidden=true;
      success=true;
    }
  } catch(error){saveQueued=false;success=false;saveState.textContent=error instanceof Error?error.message:"Save failed · draft preserved";toast(saveState.textContent||"Save failed");document.querySelector<HTMLButtonElement>("#save-button")!.hidden=false;}
  finally{saveRunning=false;}
  return success;
}

function navigate(): void {
  const current = location.hash.replace(/^#/, "") || "overview";
  document.querySelectorAll<HTMLElement>(".page").forEach(page => {
    const active = page.id === current;
    page.classList.toggle("active", active);
    page.hidden = !active;
    page.setAttribute("aria-hidden", String(!active));
  });
  document.querySelectorAll<HTMLAnchorElement>(".nav-link").forEach(link => link.classList.toggle("active", link.getAttribute("href") === `#${current}`));
  if(current==="activity")void startActivity();else if(activityStream)stopActivity();
  if(current==="state" && configurationLoaded)void loadUsagePage();
  if(current==="models" && configurationLoaded)void loadModelsPage();
  if (current === "routing") updateClassifierOptions();
  if (current === "intelligence" && configurationLoaded && benchmarksEnabledSaved) void loadBenchmarkData();
}

function setNested(target: Record<string, any>, path: string, raw: unknown): void {
  const parts = path.split("."); let cursor = target;
  parts.forEach((part, index) => { if (index === parts.length - 1) cursor[part] = raw; else cursor = cursor[part] ||= {}; });
}
function readProviderConfig(provider: Record<string, string | boolean>): Record<string, any> {
  if (provider.type === "ollama") {
    // Preserve the native Ollama inference deadline without generic options.
    const url = new URL(String(provider.api_url));
    const output: Record<string, any> = { enabled:provider.enabled!==false,treat_models_as_free:provider.treat_models_as_free??true,id:provider.id, type:provider.type, base_url:`${url.protocol}//${url.host}${url.pathname === "/" ? "" : url.pathname}` };
    if (String(provider.request_timeout || "").trim()) output.request_timeout = String(provider.request_timeout).trim();
    return output;
  }
  const output: Record<string, any> = {};
  Object.entries(provider).forEach(([key, raw]) => {
    // The API URL is the source of truth; do not overwrite it with fields
    // retained from the loaded YAML after converting it for the form.
    if (key === "base_url" || key === "api_path" || key === "discover_models") return;
    if (["api_key", "api_key_env", "api_key_file", "bearer_token","credential_id","management_id"].includes(key)) return;
    if (key === "discover") { output.discover_models = Boolean(raw); return; }
    if (String(raw).trim() === "") return;
    const map: Record<string, string> = { management_env: "management_token", tls_ca: "tls.ca_file", tls_server: "tls.server_name" };
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
function readConfig(): Record<string, any> {
  const config: Record<string, any> = structuredClone(loadedConfiguration);
 config.server={...config.server};config.state={...config.state};config.router={...config.router,classifier:{},sessions:{...config.router?.sessions},quota:{...config.router?.quota},selection:{...config.router?.selection}};
  new FormData(form).forEach((raw, name) => { if (name === "router.quota.allowances") return; if (name === "router.allow_paid_models") setNested(config, name, true); else if (name === "router.selection.minimum_quality") { try { setNested(config, name, JSON.parse(String(raw))); } catch { /* keep invalid drafts editable */ } } else if (String(raw).trim() !== "") setNested(config, name, raw); });
  config.model_enabled={...modelEnabledDraft};config.budgets=structuredClone(budgetsDraft);
 config.providers = providers.flatMap((p,index)=>editingProvider===index?(editingOriginal?[readProviderConfig(editingOriginal)]:[]):[readProviderConfig(p)]);
  // Unchecked checkboxes are absent from FormData. Emit the policy explicitly
  // so saving "false" cannot restore the backend's default of allowing paid models.
  config.router.allow_paid_models = Boolean((form.elements.namedItem("router.allow_paid_models") as HTMLInputElement)?.checked);
  const selected = classifierSelect.value;
  const onFailure = String((form.elements.namedItem("router.classifier.on_failure") as HTMLSelectElement)?.value || "");
  const controls: Record<string, any> = {};
  const timeout = String((form.elements.namedItem("router.classifier.timeout") as HTMLInputElement)?.value || "").trim();
  const minimumConfidence = String((form.elements.namedItem("router.classifier.minimum_confidence") as HTMLInputElement)?.value || "").trim();
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
  const allowances = String((form.elements.namedItem("router.quota.allowances") as HTMLTextAreaElement)?.value || "").split("\n").map(line => line.split("=").map(v => v.trim())).filter(pair => pair.length === 2 && pair[0]).reduce((all, [key, raw]) => { const value = Number(raw); if (Number.isFinite(value)) all[key] = value; return all; }, {} as Record<string, number>);
  if (Object.keys(allowances).length) config.router.quota.allowances = allowances;
  config.intelligence = readBenchmarkConfiguration();
  return config;
}

function updateBenchmarkControls(): void {
  const enabled = document.querySelector<HTMLInputElement>("#intelligence-enabled")!.checked;
  document.querySelector<HTMLButtonElement>("#refresh-benchmarks")!.disabled = !benchmarksEnabledSaved || !enabled || benchmarkDirty || benchmarkRefreshRunning;
  document.querySelector("#refresh-benchmarks")!.textContent = benchmarkRefreshRunning ? "Refreshing…" : "Refresh benchmarks";
  document.querySelector<HTMLDivElement>("#benchmark-configuration")!.hidden = !enabled;
  document.querySelector<HTMLDivElement>("#benchmark-data")!.hidden = !benchmarksEnabledSaved;
  document.querySelector("#benchmark-enable-notice")!.textContent = enabled !== benchmarksEnabledSaved || benchmarkDirty
    ? "Click Save & reload to apply your benchmark intelligence setting before refreshing."
    : enabled ? "Benchmark intelligence is enabled. Choose sources below, then Save & reload to apply changes. Refresh uses saved settings."
    : "Benchmark intelligence is disabled. Enable it and click Save & reload before refreshing.";
}

async function testProvider(index: number): Promise<void> {
  const provider = providers[index];
  if (!provider) return;
  const pending: ProviderStatus = { state: "pending", message: "Checking connection and discovering models…" };
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
      const ids = (body.models || []).map((model: {id:string}) => model.id);
      if (ids.some((id: unknown) => typeof id !== "string" || !id.startsWith(`${provider.id}/`))) throw new Error("Invalid classifier model IDs");
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
async function loginProvider(index: number, button: HTMLButtonElement): Promise<void> {
  const result = providerList.querySelector<HTMLDivElement>(`[data-result="${index}"]`)!;
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
const helperAssets=["torii-auth-linux-amd64","torii-auth-linux-arm64","torii-auth-darwin-amd64","torii-auth-darwin-arm64","torii-auth-windows-amd64.exe"];
function detectedHelperAsset():string {
 const platform=navigator.platform.toLowerCase(),agent=navigator.userAgent.toLowerCase();
 if(platform.includes("win"))return helperAssets[4];
 if(platform.includes("mac")||agent.includes("macintosh"))return agent.includes("arm")?helperAssets[3]:helperAssets[2];
 return agent.includes("aarch64")||agent.includes("arm64")?helperAssets[1]:helperAssets[0];
}
function quoteShell(value:string,windows=false):string{return "'"+value.replace(/'/g,windows?"''":"'\\''")+"'";}
function helperCommand(asset:string,sessionURL:string,pairingToken:string):string {
 if(!helperAssets.includes(asset))throw new Error("Select a supported helper platform");
 const windows=asset.endsWith(".exe");
 const executable=windows?`& ${quoteShell(".\\"+asset,true)}`:`./${asset}`;
 return (windows?"":`chmod +x ./${asset}\n`)+`${executable} --session-url ${quoteShell(sessionURL,windows)} --pairing-token ${quoteShell(pairingToken,windows)}`;
}
function renderHelperInstructions(result: HTMLDivElement, sessionURL: string, pairingToken: string): void {
 let asset=detectedHelperAsset();let command=helperCommand(asset,sessionURL,pairingToken);
 result.className="provider-result pending provider-helper";
 result.innerHTML=`<strong>Download and run the helper on the computer with your browser.</strong><label>Operating system / architecture<select class="helper-platform">${helperAssets.map(name=>`<option value="${name}" ${name===asset?"selected":""}>${name.replace("torii-auth-","")}</option>`).join("")}</select></label><a class="helper-download" href="${helperDownloadURL(asset)}" download="${asset}">Download ${asset}</a><p>Open a terminal in the folder containing the download (usually Downloads). Run these commands there:</p><pre class="helper-command">${escapeHTML(command)}</pre><button class="copy-helper button quiet" type="button">Copy commands</button><small>Check the platform selection; browser detection may not identify your processor. The helper opens OpenAI login locally, imports the credential and prints “Codex login imported into Torii successfully.” Then it exits. Pairing expires in 10 minutes and is single use. After expiry or failure, click Log in again for a fresh token and rerun the updated commands. Keep the token private.</small><a href="https://github.com/Apartment38/torii-gateway/releases/latest" target="_blank" rel="noopener noreferrer">Gateway releases</a>`;
 const link=result.querySelector<HTMLAnchorElement>(".helper-download");
 result.querySelector<HTMLSelectElement>(".helper-platform")?.addEventListener("change",event=>{asset=(event.target as HTMLSelectElement).value;command=helperCommand(asset,sessionURL,pairingToken);if(link){link.href=helperDownloadURL(asset);link.download=asset;link.textContent=`Download ${asset}`;}result.querySelector(".helper-command")!.textContent=command;});
 link?.addEventListener("click",async event=>{event.preventDefault();try{const response=await gatewayFetch(helperDownloadURL(asset),{headers:await authProvider.headers()});if(!response.ok)throw new Error();const url=URL.createObjectURL(await response.blob());const a=document.createElement("a");a.href=url;a.download=asset;a.click();URL.revokeObjectURL(url);}catch{toast("Helper download failed; sign in and retry");}});
 result.querySelector<HTMLButtonElement>(".copy-helper")?.addEventListener("click",async event=>{const button=event.currentTarget as HTMLButtonElement;try{await navigator.clipboard.writeText(command);button.textContent="Copied";}catch{button.textContent="Copy unavailable";}});
}
function helperDownloadURL(asset=detectedHelperAsset()):string{return helperAssets.includes(asset)?`/gateway/v1/providers/login/helper/${asset}`:"https://github.com/Apartment38/torii-gateway/releases/latest";}
async function pollProviderLogin(provider: Record<string, string | boolean>, result: HTMLDivElement, button: HTMLButtonElement, openedURL = "", helperPending = false): Promise<void> {
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
async function testAllProviders(): Promise<void> {
  classifierDiscoveryPending = true;
  updateClassifierOptions();
  try { await Promise.all(providers.map((_, index) => testProvider(index))); }
  finally { classifierDiscoveryPending = false; updateClassifierOptions(); }
}
function toast(message: string): void { const node = document.querySelector<HTMLDivElement>("#toast")!; node.textContent = message; node.classList.add("show"); window.setTimeout(() => node.classList.remove("show"), 2400); }

document.querySelector("#add-provider")!.addEventListener("click", () => { providers.push({ ...providerDefaults }); renderProviders(); openProviderEditor(providers.length-1,true); });
document.querySelector("#refresh-benchmarks")!.addEventListener("click", () => inspectBenchmarks(true));
document.querySelector("#inspect-benchmarks")!.addEventListener("click", () => { void inspectBenchmarks(false); });
document.querySelector("#save-button")!.addEventListener("click", () => { void saveConfig(); });
document.querySelector("#reset-button")!.addEventListener("click", () => { location.reload(); });
document.querySelector("#theme-toggle")!.addEventListener("click", () => { const dark = document.documentElement.dataset.theme !== "dark"; document.documentElement.dataset.theme = dark ? "dark" : "light"; localStorage.setItem("torii-theme", dark ? "dark" : "light"); });
form.addEventListener("input", markDirty);
form.addEventListener("change",event=>{const target=event.target as HTMLElement;if(!target.closest("dialog") && !target.dataset.modelEnabled && !["classifier-question","classifier-question-option","budget-scope"].includes(target.id) && !target.id.startsWith("budget-")){void saveConfig();}});
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

document.querySelector("#console-login")!.addEventListener("click",()=>location.assign("/login"));
document.querySelector("#console-logout")!.addEventListener("click",async()=>{try{const response=await gatewayFetch("/gateway/v1/session",{method:"DELETE",headers:await authProvider.headers()});if(!response.ok)throw new Error();signOutConsole();location.assign("/login");}catch{toast("Sign-out failed. Retry to revoke the session.");}});
document.querySelector("#login-form")!.addEventListener("submit",async event=>{
  event.preventDefault();const username=document.querySelector<HTMLInputElement>("#login-username")!.value;const input=document.querySelector<HTMLInputElement>("#login-password")!;const password=input.value;input.value="";
  try{await createBrowserSession(username,password);const response=await gatewayFetch("/gateway/v1/config",{headers:await authProvider.headers()});if(!response.ok)throw new Error();const committed=await response.json();const revision=response.headers.get("ETag")||"";if(configurationLoaded){if(revision!==configurationRevision){saveConflict=true;saveState.textContent="Settings changed while signed out. Your draft is retained; reload to resolve the conflict.";}}else applyConfig(committed,revision);signedInUsername=username;updateConsoleIdentity();document.querySelector("#login-error")!.textContent="";document.querySelector<HTMLDialogElement>("#login-dialog")!.close();if(!saveConflict)saveState.textContent="Signed in. Your draft is ready to save.";void testAllProviders();void loadCredentialMetadata();}
  catch{expireConsoleSession();document.querySelector("#login-error")!.textContent="Sign-in failed. Check your administrator username and password, then try again in one minute if rate limited.";}
});
async function loadCredentialMetadata():Promise<void>{
  const list=document.querySelector<HTMLElement>("#credential-list")!;
  const login=consoleSession;
  try{const response=await gatewayFetch("/gateway/v1/admin/api-keys",{headers:await authProvider.headers()});if(!response.ok)throw new Error();const body=await response.json();if(login!==consoleSession)return;list.innerHTML=(body.api_keys||[]).map((key:any)=>`<div class="credential-card"><div class="credential-card-content"><h3 class="credential-card-name">${escapeHTML(key.name)}</h3><p class="credential-card-meta">Created ${escapeHTML(key.created_at)}</p></div><button type="button" class="button quiet credential-card-delete" data-delete-key="${escapeHTML(key.id)}" aria-label="Delete API key ${escapeHTML(key.name)}">Delete</button></div>`).join("") || "No gateway API keys have been created. Click Add credential to generate one.";
    list.querySelectorAll<HTMLButtonElement>("[data-delete-key]").forEach(button=>button.addEventListener("click",async()=>{if(confirm("Delete this API key? Clients using it will immediately lose access on their next request."))await deleteClientKey(button.dataset.deleteKey!);}));
  }catch{list.textContent="Sign in to manage gateway API keys.";}
}
async function deleteClientKey(id:string):Promise<void>{
  try{const response=await gatewayFetch(`/gateway/v1/admin/api-keys/${encodeURIComponent(id)}`,{method:"DELETE",headers:await authProvider.headers()});if(!response.ok)throw new Error();toast("API key deleted and invalidated");await loadCredentialMetadata();}catch{toast("API key deletion failed. Sign in as the administrator.");}
}
document.querySelector("#refresh-credentials")!.addEventListener("click",()=>{void loadCredentialMetadata();});
document.querySelector("#create-credential")!.addEventListener("click",()=>{document.querySelector<HTMLInputElement>("#client-key-name")!.value="";document.querySelector("#create-key-error")!.textContent="";document.querySelector<HTMLDialogElement>("#create-key-dialog")!.showModal();});
document.querySelector("#cancel-create-key")!.addEventListener("click",()=>document.querySelector<HTMLDialogElement>("#create-key-dialog")!.close());
document.querySelector("#create-key-form")!.addEventListener("submit",async event=>{
  event.preventDefault();
  const button=document.querySelector<HTMLButtonElement>("#generate-client-key")!;
  if(button.disabled)return;
  button.disabled=true;
  const login=consoleSession;
  const name=document.querySelector<HTMLInputElement>("#client-key-name")!.value.trim();
  try{const response=await gatewayFetch("/gateway/v1/admin/api-keys",{method:"POST",headers:{"Content-Type":"application/json",...await authProvider.headers()},body:JSON.stringify({name})});const result=await response.json();if(!response.ok)throw new Error(result.error||"API key creation failed");if(login!==consoleSession)return;document.querySelector<HTMLInputElement>("#client-key-name")!.value="";document.querySelector<HTMLDialogElement>("#create-key-dialog")!.close();showNewAPIKey(result.api_key,result.warning);await loadCredentialMetadata();}
  catch(error){document.querySelector("#create-key-error")!.textContent=error instanceof Error?error.message:"API key creation failed";}
  finally{button.disabled=false;}
});
document.querySelector("#intelligence-enabled")!.addEventListener("change",()=>{
  benchmarkDirty = true; updateBenchmarkControls();markDirty();
});

async function checkSetup():Promise<void>{
 try{const response=await fetch("/gateway/v1/setup",{cache:"no-store"});if(!response.ok)throw new Error();const status=await response.json();if(status.setup_required){document.querySelector<HTMLDialogElement>("#setup-dialog")!.showModal();return;}
 const view=await fetch("/gateway/v1/session",{credentials:"same-origin",cache:"no-store"});if(!view.ok){location.assign("/login");return;}const session=await view.json();csrfToken=session.csrf_token;consoleSession=session.expires_at;signedInUsername=session.username;updateConsoleIdentity();await loadServerConfig();void loadCredentialMetadata();
 }catch{saveState.textContent="Unable to check administrator setup; refresh to retry.";}
}

function showNewAPIKey(key:string,warning?:string):void{
  if(!key){toast("Your existing API key was imported and preserved. Generate a replacement on the Server page if needed.");return;}
  document.querySelector<HTMLTextAreaElement>("#new-api-key")!.value=key;
  document.querySelector("#new-key-warning")!.textContent=warning||"This API key is displayed only once. Save it securely before closing.";
  document.querySelector<HTMLDialogElement>("#new-key-dialog")!.showModal();
}
document.querySelector("#setup-form")!.addEventListener("submit",async event=>{
  event.preventDefault();const field=(id:string)=>document.querySelector<HTMLInputElement>(id)!;
  const username=field("#setup-username").value,password=field("#setup-password").value,code=field("#setup-code").value;
  if(password!==field("#setup-confirm").value){document.querySelector("#setup-error")!.textContent="Passwords do not match.";return;}
  field("#setup-password").value="";field("#setup-confirm").value="";field("#setup-code").value="";
  try{const response=await fetch("/gateway/v1/setup",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({username,password,setup_code:code})});const result=await response.json();if(!response.ok)throw new Error(result.error||"Setup failed");await createBrowserSession(username,password);document.querySelector<HTMLDialogElement>("#setup-dialog")!.close();await loadServerConfig();void loadCredentialMetadata();showNewAPIKey(result.api_key,result.warning);}
  catch(error){document.querySelector("#setup-error")!.textContent=error instanceof Error?error.message:"Administrator setup failed.";}
});
document.querySelector("#generate-api-key")!.addEventListener("click",async()=>{
  if(!confirm("Generate a new API key? All clients using the current key will need updating."))return;
  const login=consoleSession;
  try{const response=await gatewayFetch("/gateway/v1/admin/api-key",{method:"POST",headers:await authProvider.headers()});if(!response.ok)throw new Error();const result=await response.json();if(login!==consoleSession)return;showNewAPIKey(result.api_key,result.warning);}
  catch{toast("API key generation failed. Sign in as the administrator.");}
});
function clearNewAPIKey():void{document.querySelector<HTMLTextAreaElement>("#new-api-key")!.value="";}
document.querySelector("#new-key-dialog")!.addEventListener("close",clearNewAPIKey);
document.querySelector("#new-key-dialog")!.addEventListener("cancel",clearNewAPIKey);
document.querySelector("#close-new-key")!.addEventListener("click",()=>{clearNewAPIKey();document.querySelector<HTMLDialogElement>("#new-key-dialog")!.close();});

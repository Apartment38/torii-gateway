// Generated from app.ts by scripts/build-console.mjs.


// Administrator authentication remains in memory, never browser storage.




class MemoryBasicProvider                         {
  async headers()                       { return adminAuthorization ? { Authorization:adminAuthorization } : {}; }
}

const authProvider               = new MemoryBasicProvider();
async function gatewayFetch(url       ,options             )                  {
  const response=await fetch(url,options);
  if(response.status===401){adminAuthorization="";document.querySelector                   ("#login-dialog")?.showModal?.();}
  return response;
}
const form = document.querySelector                 ("#config-form") ;
const providerList = document.querySelector                ("#providers-list") ;
const saveState = document.querySelector                 ("#save-state") ;
let adminAuthorization = "";
function basicAuthorization(username       ,password       )        {return "Basic "+btoa(Array.from(new TextEncoder().encode(`${username}:${password}`),byte=>String.fromCharCode(byte)).join(""));}
const providerDefaults = { id: "", type: "openai_compatible", api_url: "", api_key: "", api_key_env: "", api_key_file: "", management_env: "", portal_url: "", client_id: "", headers: "", models: "", discover: true, transport: "", organization: "", project: "", rewrites: "", tls_ca: "", tls_server: "", connect_timeout: "", request_timeout: "", discovery_timeout: "" };
let providers                                     = [ { ...providerDefaults } ];




const providerStatuses = new WeakMap                                                  ();
const classifierCatalogs = new WeakMap                                            ();
let classifierDiscoveryPending = false;
let configurationLoaded = false;
let requestedClassifier = "";
let legacyClassifier                             = null;
const classifierSelect = document.querySelector                   ("#classifier-model") ;

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
  return message;
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
  providers.forEach((provider, index) => { if (!provider.id && isKnownProvider(provider)) provider.id = knownProviderID(String(provider.type), index); });
  providerList.innerHTML = providers.map((provider, index) => `
    <article class="provider-card">
      <div class="provider-card-head"><div class="provider-card-title"><span class="provider-index">${index + 1}</span> <span data-provider-title="${index}">${value(provider,"id") || `Provider ${index + 1}`}</span></div><div class="provider-card-actions"><button class="test-provider button secondary" type="button" data-test="${index}">${subscriptionProvider(provider) ? "Check status" : "Test connection"}</button>${subscriptionProvider(provider) ? `<button class="login-provider button secondary" type="button" data-login="${index}">Log in</button>` : ""}${providers.length > 1 ? `<button class="remove-provider" type="button" data-remove="${index}">Remove</button>` : ""}</div></div>
      <div class="provider-result" data-result="${index}" role="status" aria-live="polite">Not checked yet</div>
      ${subscriptionProvider(provider) ? `<div class="grid two"><label data-tooltip="The stable ID used as the prefix of public model names. Built-in providers receive this automatically; custom API endpoints must define it.">Provider ID<input data-provider="${index}" data-key="id" value="${value(provider,"id")}" readonly required /></label><label data-tooltip="The subscription integration used by Torii. OAuth credentials are encrypted in the gateway database.">Type<select data-provider="${index}" data-key="type"><option ${provider.type === "openai-codex" ? "selected" : ""} value="openai-codex">OpenAI Codex</option><option ${provider.type === "nous" ? "selected" : ""} value="nous">Nous</option></select></label></div><p class="provider-subscription-note">This subscription provider uses its OAuth login. API URL, API keys, and discovery controls are not used.</p>` : `<div class="grid three">
        <label data-tooltip="The stable ID used as the prefix of public model names. Built-in providers receive this automatically; custom API endpoints must define it.">Provider ID<input data-provider="${index}" data-key="id" value="${value(provider,"id")}" ${isKnownProvider(provider) ? "readonly" : ""} required /></label>
        <label data-tooltip="The provider integration Torii should use. Jev and Jev-compatible providers supply classifier models, not chat models.">Type<select data-provider="${index}" data-key="type"><option ${provider.type === "openai_compatible" ? "selected" : ""} value="openai_compatible">OpenAI compatible</option><option ${provider.type === "jev" ? "selected" : ""} value="jev">TypeSafe AI · Jev</option><option ${provider.type === "jev_compatible" ? "selected" : ""} value="jev_compatible">Jev compatible · Laya / custom</option><option ${provider.type === "openrouter" ? "selected" : ""} value="openrouter">OpenRouter</option><option ${provider.type === "gemini" ? "selected" : ""} value="gemini">Gemini</option><option ${provider.type === "xai" ? "selected" : ""} value="xai">xAI</option><option ${provider.type === "anthropic" ? "selected" : ""} value="anthropic">Anthropic</option><option ${provider.type === "openai" ? "selected" : ""} value="openai">OpenAI</option><option ${provider.type === "openai-codex" ? "selected" : ""} value="openai-codex">OpenAI Codex</option><option ${provider.type === "ollama" ? "selected" : ""} value="ollama">Ollama</option><option ${provider.type === "nous" ? "selected" : ""} value="nous">Nous</option></select></label>
        <label data-tooltip="The complete upstream URL Torii should call. Include any required API path yourself; Torii does not add /v1 to this field."><span>API URL</span><input data-provider="${index}" data-key="api_url" value="${value(provider,"api_url")}" placeholder="https://api.example.com" /></label>
        ${provider.type === "ollama" ? "" : `<label data-tooltip="Enter a new upstream API key to replace the configured credential. Save encrypts it in SQLite; existing keys cannot be revealed.">New API key<span class="password-field"><input id="provider-api-key-${index}" type="password" autocomplete="new-password" data-provider="${index}" data-key="api_key" placeholder="${provider.bearer_token ? "Configured · leave blank to keep" : "Enter provider API key"}" /><button class="reveal-button" type="button" data-reveal-provider-key="${index}" aria-controls="provider-api-key-${index}" aria-label="Show newly entered provider API key" aria-pressed="false">Show new</button></span></label>`}
      </div>
      <details class="advanced-provider"><summary>Advanced provider options</summary><div class="grid three">
        <label data-tooltip="Select a credential ID from the Credentials page. Values remain encrypted and cannot be revealed.">Stored API credential<input data-provider="${index}" data-key="credential_id" value="${escapeHTML(String((provider.bearer_token       )?.credential || provider.credential_id || ""))}" placeholder="Credential ID" /></label>
        <label data-tooltip="Select a separate management credential from the Credentials page for quota lookup.">Management credential<input data-provider="${index}" data-key="management_id" value="${escapeHTML(String((provider.management_token       )?.credential || provider.management_id || ""))}" placeholder="Credential ID" /></label>
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
        <label data-tooltip="Maximum time allowed for an upstream request. Use a Go duration such as 60s.">Request timeout<input data-provider="${index}" data-key="request_timeout" value="${value(provider,"request_timeout")}" placeholder="60s" /></label>
        <label data-tooltip="Maximum time allowed while discovering the provider model catalog. Use a Go duration such as 10s.">Discovery timeout<input data-provider="${index}" data-key="discovery_timeout" value="${value(provider,"discovery_timeout")}" placeholder="10s" /></label>
        <label class="checkbox" data-tooltip="When enabled, Torii queries the provider for its available models. Disable this only when you provide a static model list."><input data-provider="${index}" data-key="discover" type="checkbox" ${checked(provider,"discover")} /> Discover models</label>
      </div></details>`}
    </article>`).join("");
  providerList.querySelectorAll                   ("[data-remove]").forEach(button => button.addEventListener("click", () => { providers.splice(Number(button.dataset.remove), 1); renderProviders(); markDirty(); }));
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
    button.textContent = visible ? "Hide" : "Show";
    button.setAttribute("aria-label", visible ? "Hide provider API key" : "Show provider API key");
    button.setAttribute("aria-pressed", String(visible));
  }));
  providers.forEach(provider => {
    const status = providerStatuses.get(provider);
    if (status) setProviderStatus(provider, status);
  });
  initializeTooltips();
  updateClassifierOptions();
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

function markDirty()       { saveState.textContent = "Unsaved changes"; }

function applyConfig(config                     )       {
  classifierDiscoveryPending = true;
  configurationLoaded = true;
  legacyClassifier = null;
  const classifier = config.router?.classifier || {};
  requestedClassifier = classifier.type === "disabled" ? "disabled" : classifier.type === "jev" && classifier.transport === "provider"
    ? `${classifier.provider}/${classifier.model}` : "";
  document.querySelector                     ("#intelligence-config") .value = config.intelligence ? JSON.stringify(config.intelligence, null, 2) : "";
  providers = (config.providers || []).map((provider                     ) => ({
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
  if (!providers.length) providers = [{ ...providerDefaults }];
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
}

async function loadServerConfig()                {
  try {
    const response = await gatewayFetch("/gateway/v1/config", { headers: await authProvider.headers() });
    if (!response.ok) { if (response.status === 401 || response.status === 403) {adminAuthorization="";saveState.textContent="Sign in to configure the gateway";document.querySelector                   ("#login-dialog")?.showModal?.();} return; }
    applyConfig(await response.json());
    saveState.textContent = "Loaded from gateway";
    void testAllProviders();
  } catch { saveState.textContent = "Gateway config unavailable"; }
}

async function saveConfig()                {
  let configuration                     ;
  try { configuration = readConfig(); }
  catch (error) { toast(error instanceof Error ? error.message : "Invalid benchmark configuration JSON"); return; }
  const response = await gatewayFetch("/gateway/v1/config", { method: "PUT", headers: { "Content-Type": "application/json", ...await authProvider.headers() }, body: JSON.stringify(configuration) });
  if (!response.ok) { toast(redactProviderKeys(`Save failed: ${await response.text()}`)); return; }
  providers.forEach(provider=>{provider.api_key="";});
  saveState.textContent = "Saved and reloaded";
  await loadServerConfig();
  toast("Configuration saved and reloaded");
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
  if (current === "routing") updateClassifierOptions();
}

function setNested(target                     , path        , raw         )       {
  const parts = path.split("."); let cursor = target;
  parts.forEach((part, index) => { if (index === parts.length - 1) cursor[part] = raw; else cursor = cursor[part] ||= {}; });
}
function readProviderConfig(provider                                  )                      {
  if (provider.type === "ollama") {
    // Ollama does not accept the generic advanced fields or discover_models.
    // Keep an existing Ollama alongside a newly added classifier saveable.
    const url = new URL(String(provider.api_url));
    return { id:provider.id, type:provider.type, base_url:`${url.protocol}//${url.host}${url.pathname === "/" ? "" : url.pathname}` };
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
  const config                      = { server: {}, state: {}, providers: [], router: { classifier: {}, sessions: {}, quota: {}, selection: {} } };
  new FormData(form).forEach((raw, name) => { if (name === "router.quota.allowances") return; if (name === "router.allow_paid_models") setNested(config, name, true); else if (name === "router.selection.minimum_quality") { try { setNested(config, name, JSON.parse(String(raw))); } catch { /* keep invalid drafts editable */ } } else if (String(raw).trim() !== "") setNested(config, name, raw); });
  config.providers = providers.map(readProviderConfig);
  const selected = classifierSelect.value;
  const onFailure = config.router.classifier.on_failure;
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
  const allowances = String((form.elements.namedItem("router.quota.allowances")                       )?.value || "").split("\n").map(line => line.split("=").map(v => v.trim())).filter(pair => pair.length === 2 && pair[0]).reduce((all, [key, raw]) => { const value = Number(raw); if (Number.isFinite(value)) all[key] = value; return all; }, {}                          );
  if (Object.keys(allowances).length) config.router.quota.allowances = allowances;
  const intelligenceJSON = document.querySelector                     ("#intelligence-config") .value.trim();
  if (intelligenceJSON) {
    try {
      const intelligence = JSON.parse(intelligenceJSON);
      if (!intelligence || Array.isArray(intelligence) || typeof intelligence !== "object") throw new Error();
      config.intelligence = intelligence;
    } catch { throw new Error("Benchmark configuration must be a valid JSON object"); }
  }
  return config;
}

async function inspectBenchmarks(refresh         )                {
  const output = document.querySelector             ("#benchmark-status") ;
  const button = document.querySelector                   (refresh ? "#refresh-benchmarks" : "#inspect-benchmarks") ;
  button.disabled = true;
  output.textContent = refresh ? "Refreshing sources; cached routing data remains available…" : "Loading source status…";
  try {
    const headers = await authProvider.headers();
    const response = await gatewayFetch(`/gateway/v1/intelligence/${refresh ? "refresh" : "sources"}`, { method: refresh ? "POST" : "GET", headers });
    const body = await response.json();
    if (!response.ok) throw new Error(body.error?.message || body.error || "Benchmark request failed");
    output.textContent = JSON.stringify(body, null, 2);
  } catch (error) { output.textContent = error instanceof Error ? error.message : "Benchmark request failed"; }
  finally { button.disabled = false; }
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
      message: `${connection} · ${count > 0 ? "Working" : "Not working"} · ${count} model${count === 1 ? "" : "s"} discovered${count === 0 ? " (no models available)" : ""}`,
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

document.querySelector("#add-provider") .addEventListener("click", () => { providers.push({ ...providerDefaults }); renderProviders(); markDirty(); });
document.querySelector("#refresh-benchmarks") .addEventListener("click", () => { void inspectBenchmarks(true); });
document.querySelector("#inspect-benchmarks") .addEventListener("click", () => { void inspectBenchmarks(false); });
document.querySelector("#save-button") .addEventListener("click", () => { void saveConfig(); });
document.querySelector("#reset-button") .addEventListener("click", () => { location.reload(); });
document.querySelector("#theme-toggle") .addEventListener("click", () => { const dark = document.documentElement.dataset.theme !== "dark"; document.documentElement.dataset.theme = dark ? "dark" : "light"; localStorage.setItem("torii-theme", dark ? "dark" : "light"); });
form.addEventListener("input", markDirty);
window.addEventListener("hashchange", navigate);
document.documentElement.dataset.theme = localStorage.getItem("torii-theme") || (matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light");
classifierSelect.addEventListener("change", () => { requestedClassifier = classifierSelect.value; legacyClassifier = null; markDirty(); });
document.addEventListener("click", () => { document.querySelectorAll("label.tooltip-open").forEach(label => label.classList.remove("tooltip-open")); });
renderProviders(); navigate(); void checkSetup();

document.querySelector("#console-login") .addEventListener("click",()=>document.querySelector                   ("#login-dialog") .showModal());
document.querySelector("#console-logout") .addEventListener("click",()=>{
  adminAuthorization="";clearNewAPIKey();document.querySelector                   ("#new-key-dialog") .close();providers=[{...providerDefaults}];configurationLoaded=false;form.reset();document.querySelector                     ("#intelligence-config") .value="";document.querySelector("#credential-list") .textContent="";renderProviders();saveState.textContent="Signed out";
});
document.querySelector("#login-form") .addEventListener("submit",async event=>{
  event.preventDefault();const username=document.querySelector                  ("#login-username") .value;const input=document.querySelector                  ("#login-password") ;adminAuthorization=basicAuthorization(username,input.value);input.value="";
  try{const response=await gatewayFetch("/gateway/v1/config",{headers:await authProvider.headers()});if(!response.ok)throw new Error();applyConfig(await response.json());document.querySelector                   ("#login-dialog") .close();saveState.textContent="Signed in";void testAllProviders();void loadCredentialMetadata();}
  catch{adminAuthorization="";document.querySelector("#login-error") .textContent="Sign-in failed. Check your administrator username and password, then try again in one minute if rate limited.";}
});
async function loadCredentialMetadata()              {
  const list=document.querySelector             ("#credential-list") ;
  try{const response=await gatewayFetch("/gateway/v1/credentials",{headers:await authProvider.headers()});if(!response.ok)throw new Error();const body=await response.json();list.innerHTML=(body.credentials||[]).map((c    )=>`<div class="card"><strong>${escapeHTML(c.service)} · ${escapeHTML(c.purpose)}</strong><p>${escapeHTML(c.id)} · Configured</p>${c.service==="gateway"&&c.purpose==="authentication"?"<p>Manage your gateway API key on the Server page.</p>":c.purpose==="oauth"?"<p>Use provider login to replace OAuth tokens.</p>":`<input type="password" autocomplete="new-password" data-new-secret="${escapeHTML(c.id)}" placeholder="New replacement value" /><button type="button" class="button secondary" data-replace-secret="${escapeHTML(c.id)}">Replace</button>`}${c.service==="gateway"&&c.purpose==="authentication"?"":`<button type="button" class="button quiet" data-delete-secret="${escapeHTML(c.id)}">Delete</button>`}</div>`).join("");
    list.querySelectorAll                   ("[data-replace-secret]").forEach(button=>button.addEventListener("click",async()=>{const id=button.dataset.replaceSecret ;const input=list.querySelector                  (`[data-new-secret="${id}"]`) ;const value=input.value;input.value="";await writeCredential(id,"PUT",value);}));
    list.querySelectorAll                   ("[data-delete-secret]").forEach(button=>button.addEventListener("click",async()=>{if(confirm("Delete this credential? Connections using it will stop working."))await writeCredential(button.dataset.deleteSecret ,"DELETE");}));
  }catch{list.textContent="Sign in to inspect encrypted credentials.";}
}
async function writeCredential(id       ,method       ,value        )              {
  const service=document.querySelector                  ("#credential-service") .value;const purpose=document.querySelector                  ("#credential-purpose") .value;
  try{const response=await gatewayFetch(`/gateway/v1/credentials${id?`/${encodeURIComponent(id)}`:""}`,{method,headers:{"Content-Type":"application/json",...await authProvider.headers()},body:method==="DELETE"?undefined:JSON.stringify({value,service:id?"existing":service,purpose:id?"existing":purpose})});if(!response.ok)throw new Error();toast("Credential updated");await loadCredentialMetadata();}catch{toast("Credential update failed. Check authentication and required fields.");}
}
document.querySelector("#refresh-credentials") .addEventListener("click",()=>{void loadCredentialMetadata();});
document.querySelector("#create-credential") .addEventListener("click",()=>{const input=document.querySelector                  ("#credential-value") ;const value=input.value;input.value="";void writeCredential("","POST",value);});

async function checkSetup()              {
  try{const response=await fetch("/gateway/v1/setup",{cache:"no-store"});if(!response.ok)throw new Error();const status=await response.json();document.querySelector                   (status.setup_required?"#setup-dialog":"#login-dialog") .showModal();}
  catch{saveState.textContent="Unable to check administrator setup; refresh to retry.";}
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
  try{const response=await fetch("/gateway/v1/setup",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({username,password,setup_code:code})});const result=await response.json();if(!response.ok)throw new Error(result.error||"Setup failed");adminAuthorization=basicAuthorization(username,password);document.querySelector                   ("#setup-dialog") .close();await loadServerConfig();void loadCredentialMetadata();showNewAPIKey(result.api_key,result.warning);}
  catch(error){document.querySelector("#setup-error") .textContent=error instanceof Error?error.message:"Administrator setup failed.";}
});
document.querySelector("#generate-api-key") .addEventListener("click",async()=>{
  if(!confirm("Generate a new API key? All clients using the current key will need updating."))return;
  try{const response=await gatewayFetch("/gateway/v1/admin/api-key",{method:"POST",headers:await authProvider.headers()});if(!response.ok)throw new Error();const result=await response.json();showNewAPIKey(result.api_key,result.warning);}
  catch{toast("API key generation failed. Sign in as the administrator.");}
});
function clearNewAPIKey()     {document.querySelector                     ("#new-api-key") .value="";}
document.querySelector("#new-key-dialog") .addEventListener("close",clearNewAPIKey);
document.querySelector("#new-key-dialog") .addEventListener("cancel",clearNewAPIKey);
document.querySelector("#close-new-key") .addEventListener("click",()=>{clearNewAPIKey();document.querySelector                   ("#new-key-dialog") .close();});

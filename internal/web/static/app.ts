type AuthHeaders = Record<string, string>;

// The console is unauthenticated today. Keeping requests behind this seam
// lets Basic Auth or an Authentik/OIDC provider be added without rewriting UI code.
interface AuthProvider {
  headers(): Promise<AuthHeaders>;
}

class NoAuthProvider implements AuthProvider {
  async headers(): Promise<AuthHeaders> { return {}; }
}

const authProvider: AuthProvider = new NoAuthProvider();
const form = document.querySelector<HTMLFormElement>("#config-form")!;
const providerList = document.querySelector<HTMLDivElement>("#providers-list")!;
const saveState = document.querySelector<HTMLSpanElement>("#save-state")!;
let serverAPIKeyReference: Record<string, string> | null = null;
const providerDefaults = { id: "", type: "openai_compatible", api_url: "", api_key_env: "", management_env: "", portal_url: "", client_id: "", headers: "", models: "", discover: true, transport: "", organization: "", project: "", rewrites: "", tls_ca: "", tls_server: "", connect_timeout: "", request_timeout: "", discovery_timeout: "" };
let providers: Record<string, string | boolean>[] = [ { ...providerDefaults } ];

function escapeHTML(value: string): string { return value.replace(/[&<>"]/g, ch => ({"&":"&amp;", "<":"&lt;", ">":"&gt;", '"':"&quot;"}[ch]!)); }
function value(provider: Record<string, string | boolean>, key: string): string { return escapeHTML(String(provider[key] ?? "")); }
function checked(provider: Record<string, string | boolean>, key: string): string { return provider[key] === false ? "" : "checked"; }
function subscriptionProvider(provider: Record<string, string | boolean>): boolean { return provider.type === "openai-codex" || provider.type === "nous"; }
const knownProviderIDs: Record<string, string> = { openai: "openai", "openai-codex": "openai-codex", anthropic: "anthropic", gemini: "gemini", xai: "xai", openrouter: "openrouter", ollama: "ollama", nous: "nous" };
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
  providers.forEach((provider, index) => { if (!provider.id && isKnownProvider(provider)) provider.id = knownProviderID(String(provider.type), index); });
  providerList.innerHTML = providers.map((provider, index) => `
    <article class="provider-card">
      <div class="provider-card-head"><div class="provider-card-title"><span class="provider-index">${index + 1}</span> <span data-provider-title="${index}">${value(provider,"id") || `Provider ${index + 1}`}</span></div><div class="provider-card-actions">${subscriptionProvider(provider) ? `<button class="login-provider button secondary" type="button" data-login="${index}">Log in</button>` : `<button class="test-provider button secondary" type="button" data-test="${index}">Test connection</button>`}${providers.length > 1 ? `<button class="remove-provider" type="button" data-remove="${index}">Remove</button>` : ""}</div></div>
      <div class="provider-result" data-result="${index}" role="status" aria-live="polite"></div>
      ${subscriptionProvider(provider) ? `<div class="grid two"><label data-tooltip="The stable ID used as the prefix of public model names. Built-in providers receive this automatically; custom API endpoints must define it.">Provider ID<input data-provider="${index}" data-key="id" value="${value(provider,"id")}" readonly required /></label><label data-tooltip="The subscription integration used by Torii. OAuth credentials are stored in the gateway state directory.">Type<select data-provider="${index}" data-key="type"><option ${provider.type === "openai-codex" ? "selected" : ""} value="openai-codex">OpenAI Codex</option><option ${provider.type === "nous" ? "selected" : ""} value="nous">Nous</option></select></label></div><p class="provider-subscription-note">This subscription provider uses its OAuth login. API URL, API keys, and discovery controls are not used.</p>` : `<div class="grid three">
        <label data-tooltip="The stable ID used as the prefix of public model names. Built-in providers receive this automatically; custom API endpoints must define it.">Provider ID<input data-provider="${index}" data-key="id" value="${value(provider,"id")}" ${isKnownProvider(provider) ? "readonly" : ""} required /></label>
        <label data-tooltip="The provider integration Torii should use. The selected type controls authentication and request behavior.">Type<select data-provider="${index}" data-key="type"><option ${provider.type === "openai_compatible" ? "selected" : ""} value="openai_compatible">OpenAI compatible</option><option ${provider.type === "openrouter" ? "selected" : ""} value="openrouter">OpenRouter</option><option ${provider.type === "gemini" ? "selected" : ""} value="gemini">Gemini</option><option ${provider.type === "xai" ? "selected" : ""} value="xai">xAI</option><option ${provider.type === "anthropic" ? "selected" : ""} value="anthropic">Anthropic</option><option ${provider.type === "openai" ? "selected" : ""} value="openai">OpenAI</option><option ${provider.type === "openai-codex" ? "selected" : ""} value="openai-codex">OpenAI Codex</option><option ${provider.type === "ollama" ? "selected" : ""} value="ollama">Ollama</option><option ${provider.type === "nous" ? "selected" : ""} value="nous">Nous</option></select></label>
        <label data-tooltip="The complete upstream URL Torii should call. Include any required API path yourself; Torii does not add /v1 to this field."><span>API URL</span><input data-provider="${index}" data-key="api_url" value="${value(provider,"api_url")}" placeholder="https://api.example.com" /></label>
        <label data-tooltip="The environment variable containing the upstream API key. The key itself is never written to this provider form.">API key environment<input data-provider="${index}" data-key="api_key_env" value="${value(provider,"api_key_env")}" placeholder="PROVIDER_API_KEY" /></label>
      </div>
      <details class="advanced-provider"><summary>Advanced provider options</summary><div class="grid three">
        <label data-tooltip="Optional separate management credential used for provider administration or quota lookup.">Management token environment<input data-provider="${index}" data-key="management_env" value="${value(provider,"management_env")}" placeholder="OPENROUTER_MANAGEMENT_KEY" /></label>
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
  providerList.querySelectorAll<HTMLButtonElement>("[data-remove]").forEach(button => button.addEventListener("click", () => { providers.splice(Number(button.dataset.remove), 1); renderProviders(); markDirty(); }));
  providerList.querySelectorAll<HTMLButtonElement>("[data-test]").forEach(button => button.addEventListener("click", () => { void testProvider(Number(button.dataset.test), button); }));
  providerList.querySelectorAll<HTMLButtonElement>("[data-login]").forEach(button => button.addEventListener("click", () => { void loginProvider(Number(button.dataset.login), button); }));
  providerList.querySelectorAll<HTMLInputElement | HTMLSelectElement>("[data-provider]").forEach(control => control.addEventListener("input", updateProvider));
  initializeTooltips();
}

function updateProvider(event: Event): void {
  const control = event.target as HTMLInputElement | HTMLSelectElement;
  const index = Number(control.dataset.provider);
  const key = control.dataset.key!;
  providers[index][key] = control instanceof HTMLInputElement && control.type === "checkbox" ? control.checked : control.value;
  if (key === "id") {
    const title = providerList.querySelector<HTMLElement>(`[data-provider-title="${index}"]`);
    if (title) title.textContent = control.value || `Provider ${index + 1}`;
  }
  if (key === "type") {
    const previousID = String(providers[index].id || "");
    const previousBase = knownProviderIDs[String(providers[index].type)] || "";
    if (previousBase && (previousID === previousBase || /^.+-\d+$/.test(previousID))) providers[index].id = "";
    if (control.value === "openai-codex" && !providers[index].api_url) {
      providers[index].api_url = "https://chatgpt.com/backend-api/codex";
      providers[index].models = "gpt-5.5";
    }
    if (control.value === "nous" && !providers[index].api_url) providers[index].api_url = "https://inference-api.nousresearch.com/v1";
    if (knownProviderIDs[control.value]) providers[index].id = knownProviderID(control.value, index);
    renderProviders();
  }
  markDirty();
}

function markDirty(): void { saveState.textContent = "Unsaved changes"; }

function applyConfig(config: Record<string, any>): void {
  const configuredAPIKey = config.server?.authentication?.api_key;
  serverAPIKeyReference = configuredAPIKey && !configuredAPIKey.value ? { ...configuredAPIKey } : null;
  providers = (config.providers || []).map((provider: Record<string, any>) => ({
    ...providerDefaults,
    ...provider,
    api_key_env: provider.bearer_token?.env || "",
    management_env: provider.management_token?.env || "",
    tls_ca: provider.tls?.ca_file || "",
    tls_server: provider.tls?.server_name || "",
    api_url: `${String(provider.base_url || "").replace(/\/$/, "")}${provider.api_path || ""}`,
    headers: provider.headers ? JSON.stringify(provider.headers) : "",
    rewrites: provider.model_rewrites ? JSON.stringify(provider.model_rewrites) : "",
    models: Array.isArray(provider.models) ? provider.models.join(", ") : "",
    discover: provider.discover_models !== false,
  }));
  if (!providers.length) providers = [{ ...providerDefaults }];
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
}

async function loadServerConfig(): Promise<void> {
  try {
    const response = await fetch("/gateway/v1/config", { headers: await authProvider.headers() });
    if (!response.ok) return;
    applyConfig(await response.json());
    saveState.textContent = "Loaded from gateway";
  } catch { saveState.textContent = "Gateway config unavailable"; }
}

async function saveConfig(): Promise<void> {
  const response = await fetch("/gateway/v1/config", { method: "PUT", headers: { "Content-Type": "application/json", ...await authProvider.headers() }, body: JSON.stringify(readConfig()) });
  if (!response.ok) { toast(`Save failed: ${await response.text()}`); return; }
  saveState.textContent = "Saved and reloaded";
  await loadServerConfig();
  toast("Configuration saved and reloaded");
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
}

function setNested(target: Record<string, any>, path: string, raw: unknown): void {
  const parts = path.split("."); let cursor = target;
  parts.forEach((part, index) => { if (index === parts.length - 1) cursor[part] = raw; else cursor = cursor[part] ||= {}; });
}
function readConfig(): Record<string, any> {
  const config: Record<string, any> = { server: {}, state: {}, providers: [], router: { classifier: {}, sessions: {}, quota: {}, selection: {} } };
  new FormData(form).forEach((raw, name) => { if (name === "router.quota.allowances") return; if (name === "router.allow_paid_models") setNested(config, name, true); else if (name === "router.selection.minimum_quality") { try { setNested(config, name, JSON.parse(String(raw))); } catch { /* keep invalid drafts editable */ } } else if (String(raw).trim() !== "") setNested(config, name, raw); });
  const apiKeyInput = form.elements.namedItem("server.authentication.api_key.value") as HTMLInputElement;
  if (!apiKeyInput.value.trim() && serverAPIKeyReference) config.server.authentication = { api_key: serverAPIKeyReference };
  config.providers = providers.map(provider => { const output: Record<string, any> = {}; Object.entries(provider).forEach(([key, raw]) => { if (key === "discover") { if (raw) output.discover_models = true; return; } if (String(raw).trim() === "") return; const map: Record<string, string> = { api_key_env: "bearer_token", management_env: "management_token", tls_ca: "tls.ca_file", tls_server: "tls.server_name" }; const mapped = map[key] || key; if (key === "api_url") { const url = new URL(String(raw)); output.base_url = `${url.protocol}//${url.host}`; output.api_path = url.pathname === "/" ? "" : url.pathname; return; } if (key === "models") output.models = String(raw).split(",").map(v => v.trim()).filter(Boolean); else if (key === "headers" || key === "rewrites") { try { output[key === "headers" ? "headers" : "model_rewrites"] = JSON.parse(String(raw)); } catch { output[key === "headers" ? "headers" : "model_rewrites"] = {}; } } else if (key.endsWith("_env")) { setNested(output, mapped, { env: raw }); } else setNested(output, mapped, raw); }); return output; });
  const allowances = String((form.elements.namedItem("router.quota.allowances") as HTMLTextAreaElement)?.value || "").split("\n").map(line => line.split("=").map(v => v.trim())).filter(pair => pair.length === 2 && pair[0]).reduce((all, [key, raw]) => { const value = Number(raw); if (Number.isFinite(value)) all[key] = value; return all; }, {} as Record<string, number>);
  if (Object.keys(allowances).length) config.router.quota.allowances = allowances;
  return config;
}
async function testProvider(index: number, button: HTMLButtonElement): Promise<void> {
  const result = providerList.querySelector<HTMLSpanElement>(`[data-result="${index}"]`)!;
  button.disabled = true;
  result.textContent = "Testing…";
  result.className = "provider-result pending";
  try {
    const response = await fetch("/gateway/v1/providers/test", { method: "POST", headers: { "Content-Type": "application/json", ...await authProvider.headers() }, body: JSON.stringify({ provider: readConfig().providers[index] }) });
    const body = await response.json();
    if (!response.ok) throw new Error(body.error || "connection failed");
    result.textContent = `Success · ${body.models.length} model${body.models.length === 1 ? "" : "s"} detected`;
    result.className = "provider-result success";
  } catch (error) {
    result.textContent = `Failed · ${error instanceof Error ? error.message : "connection failed"}`;
    result.className = "provider-result failure";
  } finally { button.disabled = false; }
}
async function loginProvider(index: number, button: HTMLButtonElement): Promise<void> {
  const result = providerList.querySelector<HTMLDivElement>(`[data-result="${index}"]`)!;
  button.disabled = true;
  result.className = "provider-result pending";
  result.textContent = "Starting login…";
  try {
    const response = await fetch("/gateway/v1/providers/login", { method: "POST", headers: { "Content-Type": "application/json", ...await authProvider.headers() }, body: JSON.stringify({ provider: readConfig().providers[index] }) });
    const body = await response.json();
    if (!response.ok) throw new Error(body.error || "login could not start");
    if (body.helper_url && body.helper_token) {
      renderHelperInstructions(result, body.helper_url, body.helper_token);
      await pollProviderLogin(String(providers[index].id), result, button, "", true);
      return;
    }
    if (body.verification_url) window.open(body.verification_url, "_blank", "noopener,noreferrer");
    result.textContent = body.user_code ? `${body.message}. Code: ${body.user_code}` : body.message || "Waiting for login…";
    await pollProviderLogin(String(providers[index].id), result, button, body.verification_url || "");
  } catch (error) {
    result.className = "provider-result failure";
    result.textContent = `Failed · ${error instanceof Error ? error.message : "login failed"}`;
    button.disabled = false;
  }
}
function renderHelperInstructions(result: HTMLDivElement, sessionURL: string, pairingToken: string): void {
  const command = `torii-auth --session-url ${sessionURL} --pairing-token ${pairingToken}`;
  result.className = "provider-result pending provider-helper";
  result.innerHTML = `<strong>Run the one-shot helper on the computer with your browser.</strong><a href="https://github.com/Apartment38/torii-router/releases/latest" target="_blank" rel="noreferrer">Download torii-auth for your operating system</a><span>${escapeHTML(command)}</span><small>The helper opens OpenAI login locally, imports the credential, then exits. This pairing expires in 10 minutes.</small>`;
}
async function pollProviderLogin(providerID: string, result: HTMLDivElement, button: HTMLButtonElement, openedURL = "", helperPending = false): Promise<void> {
  for (let attempt = 0; attempt < 180; attempt += 1) {
    await new Promise(resolve => window.setTimeout(resolve, 2000));
    const response = await fetch(`/gateway/v1/providers/login?provider=${encodeURIComponent(providerID)}`, { headers: await authProvider.headers() });
    const body = await response.json();
    if (body.verification_url && body.verification_url !== openedURL) { openedURL = body.verification_url; window.open(openedURL, "_blank", "noopener,noreferrer"); }
    if (body.status === "success") { result.className = "provider-result success"; result.textContent = body.message || "Login complete"; button.disabled = false; return; }
    if (body.status === "failure") { result.className = "provider-result failure"; result.textContent = `Failed · ${body.message || "login failed"}`; button.disabled = false; return; }
    if (!helperPending) {
      result.className = "provider-result pending";
      result.textContent = body.user_code ? `${body.message}. Code: ${body.user_code}` : body.message || "Waiting for login…";
    }
  }
  result.className = "provider-result failure";
  result.textContent = "Failed · login timed out";
  button.disabled = false;
}
function testAllProviders(): void {
  providerList.querySelectorAll<HTMLButtonElement>("[data-test]").forEach(button => { void testProvider(Number(button.dataset.test), button); });
}
function toast(message: string): void { const node = document.querySelector<HTMLDivElement>("#toast")!; node.textContent = message; node.classList.add("show"); window.setTimeout(() => node.classList.remove("show"), 2400); }

document.querySelector("#add-provider")!.addEventListener("click", () => { providers.push({ ...providerDefaults }); renderProviders(); markDirty(); });
document.querySelector("#save-button")!.addEventListener("click", () => { void saveConfig(); });
document.querySelector("#reset-button")!.addEventListener("click", () => { location.reload(); });
document.querySelector("#theme-toggle")!.addEventListener("click", () => { const dark = document.documentElement.dataset.theme !== "dark"; document.documentElement.dataset.theme = dark ? "dark" : "light"; localStorage.setItem("torii-theme", dark ? "dark" : "light"); });
form.addEventListener("input", markDirty);
window.addEventListener("hashchange", navigate);
document.documentElement.dataset.theme = localStorage.getItem("torii-theme") || (matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light");
document.querySelector<HTMLButtonElement>("#reveal-api-key")!.addEventListener("click", event => { const input = document.querySelector<HTMLInputElement>("#server-api-key")!; const button = event.currentTarget as HTMLButtonElement; const visible = input.type === "text"; input.type = visible ? "password" : "text"; button.textContent = visible ? "Show" : "Hide"; });
document.querySelector<HTMLInputElement>("#server-api-key")!.addEventListener("input", event => { if ((event.target as HTMLInputElement).value) serverAPIKeyReference = null; });
document.addEventListener("click", () => { document.querySelectorAll("label.tooltip-open").forEach(label => label.classList.remove("tooltip-open")); });
renderProviders(); navigate(); void loadServerConfig().then(() => testAllProviders());

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
const STORAGE_KEY = "torii-gateway-console-draft";

const providerDefaults = { id: "openai", type: "openai_compatible", base_url: "", api_path: "", portal_url: "", client_id: "", bearer_env: "", management_env: "", headers: "", models: "", discover: true, transport: "", organization: "", project: "", rewrites: "", tls_ca: "", tls_server: "", connect_timeout: "", request_timeout: "", discovery_timeout: "" };
let providers: Record<string, string | boolean>[] = [ { ...providerDefaults } ];

function escapeHTML(value: string): string { return value.replace(/[&<>"]/g, ch => ({"&":"&amp;", "<":"&lt;", ">":"&gt;", '"':"&quot;"}[ch]!)); }
function value(provider: Record<string, string | boolean>, key: string): string { return escapeHTML(String(provider[key] ?? "")); }
function checked(provider: Record<string, string | boolean>, key: string): string { return provider[key] === false ? "" : "checked"; }

function renderProviders(): void {
  providerList.innerHTML = providers.map((provider, index) => `
    <article class="provider-card">
      <div class="provider-card-head"><div class="provider-card-title"><span class="provider-index">${index + 1}</span> Provider ${index + 1}</div>${providers.length > 1 ? `<button class="remove-provider" type="button" data-remove="${index}">Remove</button>` : ""}</div>
      <div class="grid three">
        <label>Provider ID<input data-provider="${index}" data-key="id" value="${value(provider,"id")}" required /></label>
        <label>Type<select data-provider="${index}" data-key="type"><option ${provider.type === "openai_compatible" ? "selected" : ""} value="openai_compatible">OpenAI compatible</option><option ${provider.type === "openrouter" ? "selected" : ""} value="openrouter">OpenRouter</option><option ${provider.type === "gemini" ? "selected" : ""} value="gemini">Gemini</option><option ${provider.type === "xai" ? "selected" : ""} value="xai">xAI</option><option ${provider.type === "anthropic" ? "selected" : ""} value="anthropic">Anthropic</option><option ${provider.type === "openai" ? "selected" : ""} value="openai">OpenAI</option><option ${provider.type === "openai-codex" ? "selected" : ""} value="openai-codex">OpenAI Codex</option><option ${provider.type === "ollama" ? "selected" : ""} value="ollama">Ollama</option><option ${provider.type === "nous" ? "selected" : ""} value="nous">Nous</option></select></label>
        <label>Base URL<input data-provider="${index}" data-key="base_url" value="${value(provider,"base_url")}" placeholder="https://api.example.com" /></label>
        <label>API path<input data-provider="${index}" data-key="api_path" value="${value(provider,"api_path")}" placeholder="/v1" /></label>
        <label>Bearer token environment<input data-provider="${index}" data-key="bearer_env" value="${value(provider,"bearer_env")}" placeholder="PROVIDER_API_KEY" /></label>
        <label>Management token environment<input data-provider="${index}" data-key="management_env" value="${value(provider,"management_env")}" placeholder="OPENROUTER_MANAGEMENT_KEY" /></label>
        <label>Portal URL<input data-provider="${index}" data-key="portal_url" value="${value(provider,"portal_url")}" placeholder="https://portal.example.com" /></label>
        <label>Client ID<input data-provider="${index}" data-key="client_id" value="${value(provider,"client_id")}" /></label>
        <label>Models (comma separated)<input data-provider="${index}" data-key="models" value="${value(provider,"models")}" placeholder="model-a, model-b" /></label>
        <label>Transport<input data-provider="${index}" data-key="transport" value="${value(provider,"transport")}" placeholder="direct or provider" /></label>
        <label>Organization<input data-provider="${index}" data-key="organization" value="${value(provider,"organization")}" /></label>
        <label>Project<input data-provider="${index}" data-key="project" value="${value(provider,"project")}" /></label>
        <label>Headers JSON<input data-provider="${index}" data-key="headers" value='${value(provider,"headers")}' placeholder='{"X-Header":{"env":"ENV_VAR"}}' /></label>
        <label>Model rewrites JSON<input data-provider="${index}" data-key="rewrites" value='${value(provider,"rewrites")}' placeholder='{"public":"upstream"}' /></label>
        <label>CA file<input data-provider="${index}" data-key="tls_ca" value="${value(provider,"tls_ca")}" /></label>
        <label>TLS server name<input data-provider="${index}" data-key="tls_server" value="${value(provider,"tls_server")}" /></label>
        <label>Connect timeout<input data-provider="${index}" data-key="connect_timeout" value="${value(provider,"connect_timeout")}" placeholder="5s" /></label>
        <label>Request timeout<input data-provider="${index}" data-key="request_timeout" value="${value(provider,"request_timeout")}" placeholder="60s" /></label>
        <label>Discovery timeout<input data-provider="${index}" data-key="discovery_timeout" value="${value(provider,"discovery_timeout")}" placeholder="10s" /></label>
      </div>
      <div class="check-row"><label class="checkbox"><input data-provider="${index}" data-key="discover" type="checkbox" ${checked(provider,"discover")} /> Discover models</label></div>
    </article>`).join("");
  providerList.querySelectorAll<HTMLButtonElement>("[data-remove]").forEach(button => button.addEventListener("click", () => { providers.splice(Number(button.dataset.remove), 1); renderProviders(); markDirty(); }));
  providerList.querySelectorAll<HTMLInputElement | HTMLSelectElement>("[data-provider]").forEach(control => control.addEventListener("input", updateProvider));
}

function updateProvider(event: Event): void {
  const control = event.target as HTMLInputElement | HTMLSelectElement;
  const index = Number(control.dataset.provider);
  const key = control.dataset.key!;
  providers[index][key] = control instanceof HTMLInputElement && control.type === "checkbox" ? control.checked : control.value;
  markDirty();
}

function markDirty(): void { saveState.textContent = "Unsaved draft"; localStorage.setItem(STORAGE_KEY, JSON.stringify({ providers, fields: Object.fromEntries(new FormData(form)) })); }
function readDraft(): void {
  try {
    const draft = JSON.parse(localStorage.getItem(STORAGE_KEY) || "null");
    if (!draft) return;
    providers = draft.providers || providers;
    renderProviders();
    Object.entries(draft.fields || {}).forEach(([name, raw]) => { const control = form.elements.namedItem(name) as HTMLInputElement | HTMLSelectElement | null; if (control) control.type === "checkbox" ? control.checked = raw === "on" : control.value = String(raw); });
    saveState.textContent = "Draft restored";
  } catch { localStorage.removeItem(STORAGE_KEY); }
}

function setNested(target: Record<string, any>, path: string, raw: unknown): void {
  const parts = path.split("."); let cursor = target;
  parts.forEach((part, index) => { if (index === parts.length - 1) cursor[part] = raw; else cursor = cursor[part] ||= {}; });
}
function readConfig(): Record<string, any> {
  const config: Record<string, any> = { server: {}, state: {}, providers: [], router: { classifier: {}, sessions: {}, quota: {}, selection: {} } };
  new FormData(form).forEach((raw, name) => { if (name === "router.quota.allowances") return; if (name === "router.allow_paid_models") setNested(config, name, true); else if (name === "router.selection.minimum_quality") { try { setNested(config, name, JSON.parse(String(raw))); } catch { /* keep invalid drafts editable */ } } else if (String(raw).trim() !== "") setNested(config, name, raw); });
  config.providers = providers.map(provider => { const output: Record<string, any> = {}; Object.entries(provider).forEach(([key, raw]) => { if (key === "discover") { if (raw) output.discover_models = true; return; } if (String(raw).trim() === "") return; const map: Record<string, string> = { bearer_env: "bearer_token", management_env: "management_token", tls_ca: "tls.ca_file", tls_server: "tls.server_name" }; const mapped = map[key] || key; if (key === "models") output.models = String(raw).split(",").map(v => v.trim()).filter(Boolean); else if (key === "headers" || key === "rewrites") { try { output[key === "headers" ? "headers" : "model_rewrites"] = JSON.parse(String(raw)); } catch { output[key === "headers" ? "headers" : "model_rewrites"] = {}; } } else if (key.endsWith("_env")) { setNested(output, mapped, { env: raw }); } else setNested(output, mapped, raw); }); return output; });
  const allowances = String((form.elements.namedItem("router.quota.allowances") as HTMLTextAreaElement)?.value || "").split("\n").map(line => line.split("=").map(v => v.trim())).filter(pair => pair.length === 2 && pair[0]).reduce((all, [key, raw]) => { const value = Number(raw); if (Number.isFinite(value)) all[key] = value; return all; }, {} as Record<string, number>);
  if (Object.keys(allowances).length) config.router.quota.allowances = allowances;
  return config;
}
function yamlScalar(value: unknown): string { if (typeof value === "boolean" || typeof value === "number") return String(value); const text = String(value); return /^[A-Za-z0-9_./:@+-]+$/.test(text) ? text : JSON.stringify(text); }
function toYAML(value: any, indent = 0): string {
  const pad = " ".repeat(indent); let output = "";
  if (Array.isArray(value)) return value.map(item => typeof item === "object" ? `${pad}-\n${toYAML(item, indent + 2)}` : `${pad}- ${yamlScalar(item)}\n`).join("");
  Object.entries(value).forEach(([key, raw]) => { if (raw === undefined || raw === null || raw === "") return; if (typeof raw === "object") output += `${pad}${key}:\n${toYAML(raw, indent + 2)}`; else output += `${pad}${key}: ${yamlScalar(raw)}\n`; }); return output;
}
function download(): void { const blob = new Blob([toYAML(readConfig())], { type: "text/yaml" }); const link = document.createElement("a"); link.href = URL.createObjectURL(blob); link.download = "config.yaml"; link.click(); URL.revokeObjectURL(link.href); saveState.textContent = "YAML downloaded"; toast("Configuration downloaded"); }
function toast(message: string): void { const node = document.querySelector<HTMLDivElement>("#toast")!; node.textContent = message; node.classList.add("show"); window.setTimeout(() => node.classList.remove("show"), 2400); }

document.querySelector("#add-provider")!.addEventListener("click", () => { providers.push({ ...providerDefaults, id: `provider-${providers.length + 1}` }); renderProviders(); markDirty(); });
document.querySelector("#download-button")!.addEventListener("click", download);
document.querySelector("#reset-button")!.addEventListener("click", () => { localStorage.removeItem(STORAGE_KEY); location.reload(); });
document.querySelector("#theme-toggle")!.addEventListener("click", () => { const dark = document.documentElement.dataset.theme !== "dark"; document.documentElement.dataset.theme = dark ? "dark" : "light"; localStorage.setItem("torii-theme", dark ? "dark" : "light"); });
form.addEventListener("input", markDirty);
document.documentElement.dataset.theme = localStorage.getItem("torii-theme") || (matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light");
renderProviders(); readDraft();

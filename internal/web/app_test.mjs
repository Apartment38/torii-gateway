import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import vm from "node:vm";

// Exercise the shipped browser script with a minimal DOM and mocked gateway.
// No browser framework, dependency download, or real OAuth request is needed.
const script = readFileSync(new URL("./static/app.js", import.meta.url), "utf8");
class Element {
  reset() {}
  showModal() { this.open=true; }
  close() { this.open=false; }
  value = "";
  type = "text";
  textContent = "";
  className = "";
  disabled = false;
  dataset = {};
  listeners = {};
  attributes = {};
  classList = { add() {}, remove() {}, toggle() {} };
  addEventListener(name, callback) { this.listeners[name] = callback; }
  setAttribute(name, value) { this.attributes[name] = value; }
  querySelector() { return null; }
  querySelectorAll() { return []; }
}
class ProviderList extends Element {
  results = new Map();
  buttons = new Map();
  inputs = [];
  reveals = new Map();
  set innerHTML(html) {
    this.html = html;
    this.results = new Map();
    this.buttons = new Map();
    this.inputs = [];
    this.reveals = new Map();
    for (const match of html.matchAll(/<input\b[^>]*data-provider="(\d+)"[^>]*data-key="([^"]+)"[^>]*>/g)) {
      const node = new Element();
      node.dataset = { provider:match[1], key:match[2] };
      node.id = /id="([^"]+)"/.exec(match[0])?.[1];
      node.type = /type="([^"]+)"/.exec(match[0])?.[1] || "text";
      node.value = /value="([^"]*)"/.exec(match[0])?.[1] || "";
      this.inputs.push(node);
    }
    for (const match of html.matchAll(/data-reveal-provider-key="(\d+)"/g)) {
      const node = new Element();
      node.dataset.revealProviderKey = match[1];
      this.reveals.set(match[1], node);
    }
    for (const match of html.matchAll(/data-result="(\d+)"/g)) {
      const node = new Element();
      node.textContent = "Not checked yet";
      this.results.set(match[1], node);
    }
    for (const match of html.matchAll(/data-(test|login|remove)="(\d+)"/g)) {
      const node = new Element();
      node.dataset[match[1]] = match[2];
      this.buttons.set(`${match[1]}:${match[2]}`, node);
    }
  }
  get innerHTML() { return this.html; }
  querySelector(selector) {
    if (selector.startsWith("#")) return this.inputs.find(node => node.id === selector.slice(1));
    const field = /\[data-provider="(\d+)"\]\[data-key="([^"]+)"\]/.exec(selector);
    if (field) return this.inputs.find(node => node.dataset.provider === field[1] && node.dataset.key === field[2]);
    const match = /\[data-(result|test|login|remove)="(\d+)"\]/.exec(selector);
    if (!match) return null;
    return match[1] === "result" ? this.results.get(match[2]) : this.buttons.get(`${match[1]}:${match[2]}`);
  }
  querySelectorAll(selector) {
    if (selector === "[data-reveal-provider-key]") return [...this.reveals.values()];
    if (selector === "[data-provider]") return this.inputs;
    if (selector === '[data-key="api_key"]') return this.inputs.filter(node => node.dataset.key === "api_key");
    const match = /\[data-(test|login|remove)\]/.exec(selector);
    return match ? [...this.buttons.values()].filter(node => match[1] in node.dataset) : [];
  }
}
const response = (body, ok = true) => ({ ok, json: async () => body, text: async () => JSON.stringify(body) });
const models = count => ({ models: Array.from({ length: count }, (_, i) => ({ id: `model-${i}` })) });
const draft = (type, id = type) => ({ id, type, api_url: type === "openai-codex" ? "https://chatgpt.com/backend-api/codex" : `https://${type}.example/v1` });
function consoleFixture(handler) {
  const list = new ProviderList();
  const nodes = new Map([["#providers-list", list]]);
  const element = name => {
    if (!nodes.has(name)) nodes.set(name, new Element());
    return nodes.get(name);
  };
  element("#config-form").elements = { namedItem: element };
  const calls = [];
  let fetchHandler = handler;
  let startupLoad = true;
  const storageWrites=[];
  const context = vm.createContext({
    document: { querySelector: element, querySelectorAll: () => [], documentElement: new Element(), addEventListener() {} },
    window: { addEventListener() {}, setTimeout: callback => queueMicrotask(callback), open() {} },
    location: { hash: "#providers" },
    localStorage: { getItem: () => null, setItem(...args) {storageWrites.push(args);} },
    sessionStorage: {setItem(...args){storageWrites.push(args);}},
    navigator: { platform: "Linux", userAgent: "linux" },
    matchMedia: () => ({ matches: false }),
    HTMLInputElement: Element,
    FormData: class { forEach() {} },
    URL, Error, TextEncoder, btoa, confirm:()=>true,
    fetch: async (url, options) => {
      // Keep the script's startup load isolated; tests explicitly load config.
      if (url === "/gateway/v1/setup" && startupLoad) {
        startupLoad = false;
        return new Promise(() => {});
      }
      calls.push({ url, options });
      return fetchHandler(url, options);
    },
  });
  vm.runInContext(script, context);
  return {
    list, calls, context, storageWrites,
    run: source => vm.runInContext(source, context),
    setProviders: drafts => { context.drafts = drafts; vm.runInContext("providers = drafts; renderProviders();", context); },
    status: index => list.results.get(String(index)),
    setFetch: handler => { fetchHandler = handler; },
  };
}

test("all provider types show a working status and discovered model count", async () => {
  const app = consoleFixture((_, options) => {
    const provider = JSON.parse(options.body).provider;
    if (provider.type === "openai-codex") {
      assert.equal(provider.base_url, "https://chatgpt.com/backend-api/codex");
      assert.equal(provider.api_path, undefined);
    }
    return response(models(provider.type === "openai-codex" ? 5 : 1));
  });
  app.setProviders([draft("openai-codex"), draft("nous"), draft("ollama")]);
  assert.match(app.list.innerHTML, /Check status/);
  assert.match(app.list.innerHTML, /Log in/);
  await app.run("testAllProviders()");
  assert.equal(app.calls.length, 3);
  assert.equal(app.status(0).textContent, "Logged in · Working · 5 models discovered");
  assert.equal(app.status(1).textContent, "Logged in · Working · 1 model discovered");
  assert.equal(app.status(2).textContent, "Connected · Working · 1 model discovered");
  assert.equal(app.status(0).className, "provider-result success");
});

test("failed connections, empty catalogs, and invalid replies are not working", async () => {
  const app = consoleFixture((_, options) => {
    const { id } = JSON.parse(options.body).provider;
    if (id === "expired") return response({ error: "Credential expired" }, false);
    if (id === "empty") return response({ models: null });
    return response({ models: "invalid" });
  });
  app.setProviders([draft("openai-codex", "expired"), draft("ollama", "empty"), draft("ollama", "invalid")]);
  await app.run("testAllProviders()");
  assert.match(app.status(0).textContent, /Not working · 0 models discovered · Credential expired/);
  assert.equal(app.status(1).textContent, "Connected · Not working · 0 models discovered (no models available)");
  assert.match(app.status(2).textContent, /Invalid model discovery response/);
  for (let i = 0; i < 3; i++) assert.equal(app.status(i).className, "provider-result failure");
});

test("an invalid draft does not prevent other providers from being checked", async () => {
  const app = consoleFixture(() => response(models(2)));
  app.setProviders([{ ...draft("custom"), api_url: "not a URL" }, draft("openai-codex")]);
  await app.run("testAllProviders()");
  assert.equal(app.calls.length, 1);
  assert.equal(app.status(0).className, "provider-result failure");
  assert.equal(app.status(1).textContent, "Logged in · Working · 2 models discovered");
});

test("statuses persist across card renders and follow provider identity", async () => {
  const app = consoleFixture(() => response(models(3)));
  app.setProviders([draft("ollama"), draft("openai-codex")]);
  await app.run("testAllProviders()");
  app.run("providers.splice(0, 1); providers.push({ ...providerDefaults }); renderProviders();");
  assert.equal(app.status(0).textContent, "Logged in · Working · 3 models discovered");
  assert.equal(app.status(1).textContent, "Not checked yet");
});

test("pending results follow reindexed cards but never overwrite edited drafts", async () => {
  let finish;
  const reply = new Promise(resolve => { finish = resolve; });
  const app = consoleFixture(() => reply);
  app.setProviders([draft("ollama"), draft("openai-codex")]);
  const check = app.run("testProvider(1)");
  assert.equal(app.list.buttons.get("test:1").disabled, true);
  app.run("providers.splice(0, 1); renderProviders();");
  finish(response(models(4)));
  await check;
  assert.equal(app.status(0).textContent, "Logged in · Working · 4 models discovered");
  const again = app.run("testProvider(0)");
  const control = new Element();
  control.dataset = { provider: "0", key: "api_url" };
  control.value = "https://new.example";
  app.context.changeEvent = { target: control };
  app.run("updateProvider(changeEvent)");
  await again;
  assert.match(app.status(0).textContent, /Settings changed/);
  assert.equal(app.list.buttons.get("test:0").disabled, false);
});

test("a removed provider cannot write a late status onto its replacement", async () => {
  let finish;
  const app = consoleFixture(() => new Promise(resolve => { finish = resolve; }));
  app.setProviders([draft("ollama")]);
  const check = app.run("testProvider(0)");
  await Promise.resolve();
  app.setProviders([draft("openai-codex")]);
  finish(response(models(8)));
  await check;
  assert.equal(app.status(0).textContent, "Not checked yet");
});

test("successful login performs real discovery rather than just saying logged in", async () => {
  const app = consoleFixture(url => response(url.startsWith("/gateway/v1/providers/login?") ? { status: "success" } : models(5)));
  app.setProviders([draft("openai-codex")]);
  await app.run('pollProviderLogin(providers[0], providerList.querySelector(\'[data-result="0"]\'), providerList.querySelector(\'[data-login="0"]\'))');
  assert.equal(app.status(0).textContent, "Logged in · Working · 5 models discovered");
  assert.equal(app.calls[1].url, "/gateway/v1/providers/test");
});

test("a successful login with failed discovery reports not working", async () => {
  const app = consoleFixture(url => response(url.startsWith("/gateway/v1/providers/login?") ? { status: "success" } : { error: "Catalog HTTP 401" }, url.startsWith("/gateway/v1/providers/login?")));
  app.setProviders([draft("openai-codex")]);
  await app.run('pollProviderLogin(providers[0], providerList.querySelector(\'[data-result="0"]\'), providerList.querySelector(\'[data-login="0"]\'))');
  assert.match(app.status(0).textContent, /Not working · 0 models discovered · Catalog HTTP 401/);
});

test("config load and save automatically refresh provider statuses", async () => {
  const config = { providers: [{ id: "openai-codex", type: "openai-codex", base_url: "https://chatgpt.com/backend-api/codex" }] };
  let discoveries = 0;
  const app = consoleFixture(url => {
    if (url === "/gateway/v1/providers/test") { discoveries++; return response(models(discoveries)); }
    return response(config);
  });
  await app.run("loadServerConfig()");
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(app.status(0).textContent, "Logged in · Working · 1 model discovered");
  await app.run("saveConfig()");
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(app.status(0).textContent, "Logged in · Working · 2 models discovered");
});

test("the edited API URL is used rather than stale loaded base_url fields", () => {
  const app = consoleFixture(() => response(models(1)));
  app.setProviders([{ ...draft("openai_compatible"), base_url: "https://old.example", api_path: "/old", api_url: "https://new.example/custom" }]);
  const output = app.run("readProviderConfig(providers[0])");
  assert.equal(output.base_url, "https://new.example");
  assert.equal(output.api_path, "/custom");
});

test("provider-native base URLs retain their complete path", () => {
  const app = consoleFixture(() => response(models(1)));
  for (const type of ["openai-codex", "nous", "ollama", "anthropic"]) {
    app.setProviders([{ ...draft(type), api_url: "https://provider.example/prefix" }]);
    const output = app.run("readProviderConfig(providers[0])");
    assert.equal(output.base_url, "https://provider.example/prefix");
    assert.equal(output.api_path, undefined);
  }
});

test("routing discovers only classifier-provider models and saves provider transport", async () => {
  const app = consoleFixture((_, options) => {
    const provider = JSON.parse(options.body).provider;
    return response({ models:[{ id:`${provider.id}/${provider.type === "jev" ? "jev-latest" : provider.type === "jev_compatible" ? "english" : "chat-model"}` }] });
  });
  app.setProviders([draft("ollama"), draft("jev"), draft("jev_compatible", "laya")]);
  await app.run("testAllProviders()");
  const select = app.run("classifierSelect");
  assert.match(select.innerHTML, /jev\/jev-latest/);
  assert.match(select.innerHTML, /laya\/english/);
  assert.doesNotMatch(select.innerHTML, /chat-model/);
  assert.equal(select.value, "jev/jev-latest");
  const classifier = app.run("readConfig().router.classifier");
  assert.equal(classifier.type, "jev");
  assert.equal(classifier.transport, "provider");
  assert.equal(classifier.provider, "jev");
  assert.equal(classifier.model, "jev-latest");
  assert.equal(classifier.base_url, undefined);
  assert.equal(classifier.api_key, undefined);
  select.value = "laya/english";
  assert.equal(app.run("readConfig().router.classifier.provider"), "laya");
  assert.equal(app.run("readConfig().router.classifier.model"), "english");
});

test("routing warns and redirects to providers when no classifiers are available", async () => {
  const app = consoleFixture(() => response(models(1)));
  app.setProviders([draft("ollama")]);
  app.run('configurationLoaded = true; location.hash = "#routing";');
  await app.run("testAllProviders()");
  assert.equal(app.run("location.hash"), "#providers");
  assert.equal(app.run('document.querySelector("#classifier-provider-notice").hidden'), false);
  assert.match(app.run('document.querySelector("#classifier-notice-message").textContent'), /No classifier models are available/);
  assert.equal(app.run("readConfig().router.classifier.type"), undefined);
});

test("routing does not redirect before classifier discovery completes", async () => {
  let finish;
  const app = consoleFixture(() => new Promise(resolve => { finish = resolve; }));
  app.setProviders([draft("jev_compatible", "laya")]);
  app.run('configurationLoaded = true; location.hash = "#routing";');
  const check = app.run("testAllProviders()");
  await Promise.resolve();
  assert.equal(app.run("location.hash"), "#routing");
  assert.equal(app.run("classifierSelect.disabled"), true);
  finish(response({ models:[{ id:"laya/english" }] }));
  await check;
  assert.equal(app.run("location.hash"), "#routing");
  assert.equal(app.run("classifierSelect.value"), "laya/english");
  assert.equal(app.run('document.querySelector("#classifier-provider-notice").hidden'), true);
});

test("failed classifier discovery never supplies selectable fallback models", async () => {
  const app = consoleFixture(() => response({ error:"Catalog unavailable" }, false));
  app.setProviders([{ ...draft("jev_compatible", "laya"), models:"fake-success" }]);
  app.run('configurationLoaded = true; location.hash = "#routing";');
  await app.run("testAllProviders()");
  assert.equal(app.run("location.hash"), "#providers");
  assert.doesNotMatch(app.run("classifierSelect.innerHTML"), /fake-success/);
});

test("a saved classifier choice is preserved while multiple providers are discovered", async () => {
  const app = consoleFixture((_, options) => {
    const provider = JSON.parse(options.body).provider;
    return response({ models:[{ id:`${provider.id}/english` }] });
  });
  app.setProviders([draft("jev_compatible", "first"), draft("jev_compatible", "chosen")]);
  app.run('requestedClassifier = "chosen/english";');
  await app.run("testAllProviders()");
  assert.equal(app.run("classifierSelect.value"), "chosen/english");
  assert.equal(app.run("readConfig().router.classifier.provider"), "chosen");
});

test("legacy direct Jev connections migrate without losing key-file references", async () => {
  const app = consoleFixture(() => response({ models:[{ id:"jev/jev-latest" }] }));
  app.context.legacyConfig = { providers:[], router:{ classifier:{ type:"jev", base_url:"https://api.typesafe.ai", api_key:{ file:"/run/secrets/jev-key" }, model:"jev-latest" } } };
  app.run("applyConfig(legacyConfig)");
  await app.run("testAllProviders()");
  const config = app.run("readConfig()");
  assert.equal(config.providers[0].type, "jev");
  assert.equal(config.providers[0].bearer_token.file, "/run/secrets/jev-key");
  assert.equal(config.router.classifier.transport, "provider");
  assert.equal(config.router.classifier.api_key, undefined);
});

test("legacy non-Jev classifiers are preserved until the user chooses a replacement", () => {
  const app = consoleFixture(() => response(models(1)));
  app.context.oldConfig = { providers:[], router:{ classifier:{ type:"embedding", provider:"embeddings", model:"embedding-model", on_failure:"fail" } } };
  app.run("applyConfig(oldConfig)");
  const classifier = app.run("readConfig().router.classifier");
  assert.equal(classifier.type, "embedding");
  assert.equal(classifier.provider, "embeddings");
  assert.equal(classifier.on_failure, "fail");
});

test("new classifier provider types set appropriate defaults and custom IDs remain editable", () => {
  const app = consoleFixture(() => response(models(1)));
  app.setProviders([draft("ollama")]);
  const control = new Element();
  control.dataset = { provider:"0", key:"type" };
  control.value = "jev";
  app.context.changeEvent = { target:control };
  app.run("updateProvider(changeEvent)");
  assert.equal(app.run("providers[0].id"), "jev");
  assert.equal(app.run("providers[0].api_url"), "https://api.typesafe.ai/v1");
  assert.equal(app.run("providers[0].api_key_env"), "");
  control.value = "jev_compatible";
  app.run("updateProvider(changeEvent)");
  assert.equal(app.run("isKnownProvider(providers[0])"), false);
  assert.equal(app.run("providers[0].id"), "");
  assert.equal(app.run("providers[0].api_key_env"), "");
});

test("routing connection controls are absent and classifier setup belongs to providers", () => {
  const html = readFileSync(new URL("./static/index.html", import.meta.url), "utf8");
  const routing = html.split('<section id="routing"')[1].split("</section>")[0];
  assert.match(routing, /id="classifier-model"/);
  assert.match(routing, /href="#providers"/);
  for (const name of ["type", "provider", "base_url", "api_key.env", "api_key.file", "transport"]) {
    assert.ok(!routing.includes(`name="router.classifier.${name}"`));
  }
});

test("explicitly disabled classification stays disabled after a config reload", async () => {
  const app = consoleFixture(() => response({ models:[{ id:"laya/english" }] }));
  app.context.disabledConfig = { providers:[{ id:"laya", type:"jev_compatible", base_url:"http://localhost:8000" }], router:{ classifier:{ type:"disabled" } } };
  app.run("applyConfig(disabledConfig)");
  await app.run("testAllProviders()");
  assert.equal(app.run("classifierSelect.value"), "disabled");
  assert.equal(app.run("readConfig().router.classifier.type"), "disabled");
  assert.match(app.run("classifierSelect.innerHTML"), /laya\/english/);
});

test("saving an existing Ollama with a classifier omits unsupported generic fields", () => {
  const app = consoleFixture(() => response(models(1)));
  app.context.ollamaConfig = { providers:[{ id:"local", type:"ollama", base_url:"http://localhost:11434" }] };
  app.run("applyConfig(ollamaConfig)");
  const output = app.run("readConfig().providers[0]");
  assert.deepEqual(Object.keys(output).sort(), ["base_url", "id", "type"]);
});

test("discovery preference is editable rather than overwritten by loaded YAML", () => {
  const app = consoleFixture(() => response(models(1)));
  app.setProviders([{ ...draft("jev_compatible", "laya"), discover:false, discover_models:true, models:"english" }]);
  assert.equal(app.run("readProviderConfig(providers[0]).discover_models"), false);
});

test("direct provider keys are masked, revealable, and never included in HTML attributes", () => {
  const app = consoleFixture(() => response(models(1)));
  const key = 'test-key-"<&secret';
  app.setProviders([{ ...draft("jev"), api_key:key }]);
  const input = app.list.querySelector("#provider-api-key-0");
  const button = app.list.reveals.get("0");
  assert.equal(input.type, "password");
  assert.equal(input.value, key);
  assert.equal(app.list.innerHTML.includes(key), false);
  button.listeners.click();
  assert.equal(input.type, "text");
  assert.equal(button.textContent, "Hide");
  assert.equal(button.attributes["aria-pressed"], "true");
  button.listeners.click();
  assert.equal(input.type, "password");
  app.run("renderProviders()");
  assert.equal(app.list.querySelector("#provider-api-key-0").type, "password");
});

test("entering a direct key replaces references and connection tests send an inline secret", async () => {
  const app = consoleFixture((_, options) => {
    const provider = JSON.parse(options.body).provider;
    assert.deepEqual(provider.bearer_token, { value:"test-direct-provider-key" });
    for (const name of ["api_key", "api_key_env", "api_key_file"]) assert.equal(name in provider, false);
    return response({ models:[{ id:"jev/english" }] });
  });
  app.context.config = { providers:[{ id:"jev", type:"jev", base_url:"https://api.typesafe.ai", api_path:"/v1", bearer_token:{ env:"OLD_KEY_ENV" } }] };
  app.run("applyConfig(config)");
  const input = app.list.querySelector("#provider-api-key-0");
  input.value = "test-direct-provider-key";
  input.listeners.input({ target:input });
  assert.equal(app.run("providers[0].api_key_env"), "");
  assert.equal(app.list.querySelector('[data-provider="0"][data-key="api_key_env"]'), undefined);
  await app.run("testProvider(0)");
  assert.match(app.status(0).textContent, /Working · 1 model discovered/);
  const config = JSON.parse(app.run("JSON.stringify(readConfig())"));
  assert.deepEqual(config.providers[0].bearer_token, { value:"test-direct-provider-key" });
});

test("stored credential references survive reload without exposing keys", () => {
  const app = consoleFixture(() => response(models(1)));
  for (const secret of [{ credential:"stored-provider-key" }]) {
    app.context.config = { providers:[{ id:"custom", type:"openai_compatible", base_url:"https://custom.example", bearer_token:secret }] };
    app.run("applyConfig(config)");
    assert.deepEqual(JSON.parse(app.run("JSON.stringify(readProviderConfig(providers[0]).bearer_token)")), secret);
  }
  const input = app.list.querySelector("#provider-api-key-0");
  input.value = "replacement-key";
  input.listeners.input({ target:input });
  assert.equal(app.run("providers[0].api_key_file"), "");
  assert.equal(app.run("readProviderConfig(providers[0]).bearer_token.value"), "replacement-key");
  assert.equal(app.list.querySelector('[data-provider="0"][data-key="api_key_env"]'),undefined);
  app.setProviders([draft("openai-codex"), draft("nous"), draft("ollama")]);
  assert.equal(app.list.inputs.some(input => input.dataset.key === "api_key"), false);
});

test("connection and save errors redact directly entered provider keys", async () => {
  const key = "test-error-redaction-key";
  const app = consoleFixture(() => response({ error:`Rejected key ${key}` }, false));
  app.setProviders([{ ...draft("jev"), api_key:key }]);
  await app.run("testProvider(0)");
  assert.match(app.status(0).textContent, /Rejected key \[redacted\]/);
  assert.equal(app.status(0).textContent.includes(key), false);
  await app.run("saveConfig()");
  assert.equal(app.run('document.querySelector("#toast").textContent').includes(key), false);
});

test("saving clears newly entered keys and reloads only stored references", async () => {
  let saved;
  const app = consoleFixture((url, options) => {
    if (url === "/gateway/v1/providers/test") return response(models(1));
    if (options?.method === "PUT") {
      saved = JSON.parse(options.body);
      return response({ ok:true });
    }
    return response({ ...saved,providers:saved.providers.map(p=>({...p,bearer_token:{credential:"stored-key"}})) });
  });
  app.setProviders([{ ...draft("openai"), api_key:"test-save-reload-key" }]);
  await app.run("saveConfig()");
  assert.deepEqual(saved.providers[0].bearer_token, { value:"test-save-reload-key" });
  const input = app.list.querySelector("#provider-api-key-0");
  assert.equal(input.value, "");
  assert.equal(app.run("providers[0].bearer_token.credential"),"stored-key");
  assert.equal(input.type, "password");
});

test("benchmark configuration survives console saves and invalid JSON is rejected", async () => {
  const app = consoleFixture(() => response({}));
  const intelligence = { enabled:true, sources:[{ id:"aa", type:"artificial_analysis", enabled:true, api_key:{ env:"AA_API_KEY" } }], routing:{ task_weights:{ coding:{ coding:3, agentic:1 } } } };
  app.context.config = { providers:[], intelligence };
  app.run("applyConfig(config)");
  assert.deepEqual(JSON.parse(app.run("JSON.stringify(readConfig().intelligence)")), intelligence);
  app.run('document.querySelector("#intelligence-config").value = "invalid JSON"');
  await app.run("saveConfig()");
  assert.match(app.run('document.querySelector("#toast").textContent'), /valid JSON object/);
  assert.equal(app.calls.length, 0);
});

test("benchmark refresh uses memory-only login rather than configuration credentials", async () => {
  const app = consoleFixture((url, options) => {
    assert.equal(url, "/gateway/v1/intelligence/refresh");
    assert.equal(options.method, "POST");
    assert.equal(options.headers.Authorization, "Basic test-admin-login");
    return response({ updated:true, sources:{ aa:{ last_success:"2026-10-01", stale:false } } });
  });
  app.run('adminAuthorization="Basic test-admin-login"');
  app.context.config = { providers:[], server:{ authentication:{ api_key:{ credential:"stored-gateway-key" } } } };
  app.run("applyConfig(config)");
  await app.run("inspectBenchmarks(true)");
  assert.match(app.run('document.querySelector("#benchmark-status").textContent'), /last_success/);
  assert.equal(app.run('document.querySelector("#refresh-benchmarks").disabled'), false);
});

test("console sign-in keeps Basic login only in memory and logout clears it",async()=>{
  const app=consoleFixture(url=>url==="/gateway/v1/config"?response({server:{authentication:{api_key:{credential:"gateway-ref"}}},providers:[]}):response({credentials:[]}));
  app.run('document.querySelector("#login-username").value="admin";document.querySelector("#login-password").value="memory-login-sentinel"');
  await app.run('document.querySelector("#login-form").listeners.submit({preventDefault(){}})');
  assert.equal(app.run('adminAuthorization'),"Basic "+btoa("admin:memory-login-sentinel"));assert.equal(app.run('document.querySelector("#login-password").value'),"");assert.deepEqual(app.storageWrites,[]);
  app.run('document.querySelector("#console-logout").listeners.click()');assert.equal(app.run('adminAuthorization'),"");
});
test("unauthorized management clears login and reopens the sign-in dialog",async()=>{
  const app=consoleFixture(()=>({...response({},false),status:401}));app.run('adminAuthorization="Basic expired-login"');await app.run('loadCredentialMetadata()');assert.equal(app.run('adminAuthorization'),"");assert.equal(app.run('document.querySelector("#login-dialog").open'),true);
});
test("legacy classifier conversion preserves encrypted credential references",()=>{
  const app=consoleFixture(()=>response({}));app.context.config={providers:[],router:{classifier:{type:"jev",base_url:"https://api.typesafe.ai",model:"jev-latest",api_key:{credential:"classifier-ref"}}}};app.run("applyConfig(config)");assert.equal(app.run('readProviderConfig(providers[0]).bearer_token.credential'),"classifier-ref");
});

test("setup status selects initial administrator setup or existing-account login",async()=>{
  const app=consoleFixture(()=>response({setup_required:true}));await app.run("checkSetup()");assert.equal(app.run('document.querySelector("#setup-dialog").open'),true);
  app.setFetch(()=>response({setup_required:false}));await app.run("checkSetup()");assert.equal(app.run('document.querySelector("#login-dialog").open'),true);
});
test("administrator setup clears password fields and reveals only its newly generated key",async()=>{
  const app=consoleFixture((url,options)=>{
    if(url==="/gateway/v1/setup"){const input=JSON.parse(options.body);assert.equal(input.username,"my-admin");assert.equal(input.password,"long-enough-password");assert.equal(input.setup_code,"terminal-code");return response({api_key:"new-once-key",warning:"Displayed only once"});}
    if(url==="/gateway/v1/config"){assert.equal(options.headers.Authorization,"Basic "+btoa("my-admin:long-enough-password"));return response({providers:[]});}
    return response({credentials:[]});
  });
  app.run('document.querySelector("#setup-username").value="my-admin";document.querySelector("#setup-password").value="long-enough-password";document.querySelector("#setup-confirm").value="long-enough-password";document.querySelector("#setup-code").value="terminal-code"');
  await app.run('document.querySelector("#setup-form").listeners.submit({preventDefault(){}})');
  for(const id of ["#setup-password","#setup-confirm","#setup-code"])assert.equal(app.run(`document.querySelector("${id}").value`),"");
  assert.equal(app.run('document.querySelector("#new-api-key").value'),"new-once-key");assert.deepEqual(app.storageWrites,[]);
  app.run('document.querySelector("#close-new-key").listeners.click()');assert.equal(app.run('document.querySelector("#new-api-key").value'),"");
});
test("API key generation requires in-memory administrator login and clears its once-only display",async()=>{
  const app=consoleFixture((url,options)=>{assert.equal(url,"/gateway/v1/admin/api-key");assert.equal(options.method,"POST");assert.equal(options.headers.Authorization,"Basic admin-login");return response({api_key:"rotated-once-key"});});
  app.run('adminAuthorization="Basic admin-login"');await app.run('document.querySelector("#generate-api-key").listeners.click()');assert.equal(app.run('document.querySelector("#new-api-key").value'),"rotated-once-key");
  app.run('document.querySelector("#new-key-dialog").listeners.cancel()');assert.equal(app.run('document.querySelector("#new-api-key").value'),"");assert.deepEqual(app.storageWrites,[]);
});

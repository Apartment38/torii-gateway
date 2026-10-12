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
class CodexLoginStatus extends Element {
  set innerHTML(html) {
    this.html = html;
    this.codeInput = html.includes('class="codex-login-code"') ? new Element() : null;
    this.copyButton = html.includes('class="codex-copy-code ') ? new Element() : null;
    this.copyStatus = html.includes('class="codex-copy-status"') ? new Element() : null;
    if (this.codeInput) {
      this.codeInput.focus = () => { this.codeInput.focused = true; };
      this.codeInput.select = () => { this.codeInput.selected = true; };
    }
  }
  get innerHTML() { return this.html || ""; }
  querySelector(selector) {
    return ({".codex-login-code": this.codeInput, ".codex-copy-code": this.copyButton, ".codex-copy-status": this.copyStatus})[selector] || null;
  }
}
class ProviderList extends Element {
  dialogs = new Map();
  editorStatuses = new Map();
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
    this.dialogs = new Map();
    this.editorStatuses = new Map();
    for(const match of html.matchAll(/data-provider-dialog="(\d+)"/g)){const dialog=new Element();const status=new CodexLoginStatus();dialog.querySelector=selector=>selector==="[data-editor-status]"?status:null;this.dialogs.set(match[1],dialog);this.editorStatuses.set(match[1],status);}
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
      const node = new CodexLoginStatus();
      node.textContent = "Not checked yet";
      this.results.set(match[1], node);
    }
    for (const match of html.matchAll(/data-(test|login|remove|confirm-provider|cancel-provider|edit|detect-ollama)="(\d+)"/g)) {
      const node = new Element();
      node.dataset[match[1]] = match[2];
      node.closest=()=>this.dialogs.get(match[2]);
      this.buttons.set(`${match[1]}:${match[2]}`, node);
    }
  }
  get innerHTML() { return this.html; }
  querySelector(selector) {
    const dialog=/\[data-provider-dialog="(\d+)"\]/.exec(selector);if(dialog)return this.dialogs.get(dialog[1]);
    const status=/\[data-editor-status="(\d+)"\]/.exec(selector);if(status)return this.editorStatuses.get(status[1]);
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
    if(selector==="[data-provider-dialog]")return [...this.dialogs.values()];
    const match = /\[data-(test|login|remove|confirm-provider|cancel-provider|edit|detect-ollama)\]/.exec(selector);
    return match ? [...this.buttons.values()].filter(node => match[1] in node.dataset) : [];
  }
}
class KeyList extends Element {
  buttons = [];
  set innerHTML(html) {
    this.html = html;
    this.buttons = [...html.matchAll(/data-delete-key="([^"]+)"/g)].map(match => {
      const button = new Element();
      button.dataset.deleteKey = match[1];
      return button;
    });
  }
  get innerHTML() { return this.html; }
  querySelectorAll(selector) { return selector === "[data-delete-key]" ? this.buttons : []; }
}
class BenchmarkList extends Element {
  inputs = [];
  set innerHTML(html) {
    this.html = html;
    this.inputs = [...html.matchAll(/<input\b[^>]*data-benchmark-source="(\d+)"[^>]*data-benchmark-field="api_key_input"[^>]*>/g)].map(match => {
      const input = new Element();
      input.dataset = {benchmarkSource:match[1],benchmarkField:"api_key_input"};
      input.type = "password";
      return input;
    });
  }
  get innerHTML() { return this.html; }
  querySelectorAll(selector) { return selector === '[data-benchmark-field="api_key_input"]' ? this.inputs : []; }
}
const response = (body, ok = true, revision = '"fixture"', status = ok ? 200 : 400) => ({ ok, status, headers: { get: name => name.toLowerCase() === "etag" ? revision : null }, json: async () => body, text: async () => JSON.stringify(body) });
const models = count => ({ models: Array.from({ length: count }, (_, i) => ({ id: `model-${i}` })) });
const draft = (type, id = type) => ({ id, type, api_url: type === "openai-codex" ? "https://chatgpt.com/backend-api/codex" : `https://${type}.example/v1` });
function consoleFixture(handler, {mobile=false} = {}) {
  const list = new ProviderList();
  const nodes = new Map([["#providers-list", list], ["#credential-list", new KeyList()], ["#benchmark-sources-list", new BenchmarkList()]]);
  const element = name => {
    const keyField = /\[data-benchmark-source="(\d+)"\]\[data-benchmark-field="api_key_input"\]/.exec(name);
    if (keyField) return nodes.get("#benchmark-sources-list").inputs.find(input=>input.dataset.benchmarkSource===keyField[1]);
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
    location: { hash: "#providers",assign(path){context.redirect=path;} },
    localStorage: { getItem: () => null, setItem(...args) {storageWrites.push(args);} },
    sessionStorage: {setItem(...args){storageWrites.push(args);}},
    navigator: { platform: "Linux", userAgent: "linux" },
    matchMedia: query => ({ matches: mobile && query.includes("max-width") }),
    HTMLInputElement: Element,
    FormData: class { forEach() {} },
    fixtureRevision: '"fixture"', URL, Error, TextEncoder, TextDecoder, AbortController, structuredClone, btoa, confirm:()=>true,
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

test("empty configurations show an add-provider notice without connection tests", async () => {
  for (const config of [{ providers:[] }, {}]) {
    const app = consoleFixture(() => response(config));
    await app.run("loadServerConfig()");
    await new Promise(resolve => setImmediate(resolve));
    assert.equal(app.run('document.querySelector("#providers-empty-notice").hidden'), false);
    assert.equal(app.run('document.querySelector("#classifier-provider-notice").hidden'), false);
    assert.equal(app.list.innerHTML, "");
    assert.equal(app.run("readConfig().providers.length"), 0);
    assert.equal(app.calls.filter(call => call.url === "/gateway/v1/providers/test").length, 0);
    await app.run("saveConfig()");
    await new Promise(resolve => setImmediate(resolve));
    const saved = app.calls.find(call => call.options?.method === "PUT");
    assert.deepEqual(JSON.parse(saved.options.body).providers, []);
    assert.equal(app.calls.filter(call => call.url === "/gateway/v1/providers/test").length, 0);
  }
});

test("generated setup placeholders display the empty state instead of testing an invalid URL", async () => {
  const placeholder = { id:"setup", type:"openai_compatible", base_url:"https://example.invalid", api_path:"/v1", discover_models:false, models:["setup"] };
  const app = consoleFixture(() => response({ providers:[placeholder] }));
  await app.run("loadServerConfig()");
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(app.run('document.querySelector("#providers-empty-notice").hidden'), false);
  assert.equal(app.list.innerHTML, "");
  assert.equal(app.run("readConfig().providers.length"), 0);
  assert.equal(app.calls.filter(call => call.url === "/gateway/v1/providers/test").length, 0);
  // Never hide configured credentials or real endpoints just because the ID is setup.
  for (const provider of [{ ...placeholder, base_url:"https://real.example" }, { ...placeholder, bearer_token:{ credential:"stored-key" } }]) {
    app.context.config = { providers:[provider] };
    app.run("applyConfig(config, fixtureRevision)");
    assert.equal(app.run("providers.length"), 1);
    assert.equal(app.run('document.querySelector("#providers-empty-notice").hidden'), true);
  }
});

test("adding and removing the last provider toggles the empty-state notice", () => {
  const app = consoleFixture(() => response({ providers:[] }));
  app.run('document.querySelector("#add-provider").listeners.click()');
  assert.equal(app.run('document.querySelector("#providers-empty-notice").hidden'), true);
  assert.match(app.list.innerHTML, /data-remove="0"/);
  app.list.buttons.get("remove:0").listeners.click();
  assert.equal(app.run('document.querySelector("#providers-empty-notice").hidden'), false);
  assert.equal(app.list.innerHTML, "");
  assert.equal(app.run("readConfig().providers.length"), 0);
});

test("connection tests disclose when discovered models will not be used after saving", async () => {
  const app = consoleFixture(() => response(models(3)));
  app.setProviders([{ ...draft("openai_compatible"), discover: false, models: "setup" }]);
  await app.run("testProvider(0)");
  assert.equal(app.status(0).textContent, "Connected · Working · 3 models discovered · Discovery disabled: saved configuration uses the static model list");
  assert.equal(app.run("readProviderConfig(providers[0]).discover_models"), false);
  assert.deepEqual(Array.from(app.run("readProviderConfig(providers[0]).models")), ["setup"]);
  app.run("providers[0].discover = true");
  await app.run("testProvider(0)");
  assert.equal(app.status(0).textContent, "Connected · Working · 3 models discovered");
});

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
  assert.equal(app.status(1).textContent, "Connected · Working · 1 model discovered");
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
  await app.run('pollProviderLogin(providers[0], providerList.querySelector(\'[data-login="0"]\'), null, {status:"pending"})');
  assert.equal(app.status(0).textContent, "Logged in · Working · 5 models discovered");
  assert.equal(app.calls[1].url, "/gateway/v1/providers/test");
});

test("a successful login with failed discovery reports not working", async () => {
  const app = consoleFixture(url => response(url.startsWith("/gateway/v1/providers/login?") ? { status: "success" } : { error: "Catalog HTTP 401" }, url.startsWith("/gateway/v1/providers/login?")));
  app.setProviders([draft("openai-codex")]);
  await app.run('pollProviderLogin(providers[0], providerList.querySelector(\'[data-login="0"]\'), null, {status:"pending"})');
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

test("selecting a known hosted provider fills the API URL used by connection checks", async () => {
  const endpoints = {
    openai: "https://api.openai.com/v1", anthropic: "https://api.anthropic.com",
    gemini: "https://generativelanguage.googleapis.com/v1beta/openai", xai: "https://api.x.ai/v1",
    openrouter: "https://openrouter.ai/api/v1", nous: "https://inference-api.nousresearch.com/v1",
    "opencode-go": "https://opencode.ai/zen/go/v1", "opencode-zen": "https://opencode.ai/zen/v1",
    "standard-compute": "https://api.stdcmpt.com/v1", zai: "https://api.z.ai/api/coding/paas/v4",
    "kimi-coding": "https://api.kimi.com/coding/v1", minimax: "https://api.minimax.io/v1",
    deepseek: "https://api.deepseek.com/v1", "openai-codex": "https://chatgpt.com/backend-api/codex",
    jev: "https://api.typesafe.ai/v1",
  };
  for (const [type, endpoint] of Object.entries(endpoints)) {
    for (const previousURL of ["", "https://previous.example/stale-path"]) {
      const app = consoleFixture((url, options) => {
        assert.equal(url, "/gateway/v1/providers/test");
        const provider = JSON.parse(options.body).provider;
        assert.equal(provider.type, type);
        assert.equal(provider.base_url + (provider.api_path || ""), endpoint);
        return response({ models: [{ id: `${provider.id}/model` }] });
      });
      app.run('document.querySelector("#add-provider").listeners.click()');
      app.context.previousURL = previousURL;
      app.context.selectedType = type;
      app.run('providers[0].api_url=previousURL;updateProvider({target:{dataset:{provider:"0",key:"type"},value:selectedType}})');
      assert.equal(app.run("providers[0].api_url"), endpoint);
      if (type !== "openai-codex") assert.equal(app.list.querySelector('[data-provider="0"][data-key="api_url"]').value, endpoint);
      await new Promise(resolve => setImmediate(resolve));
      assert.equal(app.calls.length, 1, `${type} should test its populated URL`);
      assert.match(app.status(0).textContent, /Working/);
    }
  }
});

test("existing custom provider URLs survive reopening and unchanged type events", async () => {
  const app = consoleFixture((_, options) => {
    const provider = JSON.parse(options.body).provider;
    assert.equal(provider.base_url + (provider.api_path || ""), "https://custom.example/prefix");
    return response(models(1));
  });
  app.setProviders([{ ...draft("openrouter"), api_url: "https://custom.example/prefix" }]);
  app.run('openProviderEditor(0);updateProvider({target:{dataset:{provider:"0",key:"type"},value:"openrouter"}})');
  assert.equal(app.list.querySelector('[data-provider="0"][data-key="api_url"]').value, "https://custom.example/prefix");
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(app.calls.length, 1);
  for (const type of ["openai_compatible", "jev_compatible"]) {
    app.run('cancelProviderEditor();providers=[];document.querySelector("#add-provider").listeners.click()');
    app.context.selectedType = type;
    app.run('updateProvider({target:{dataset:{provider:"0",key:"type"},value:selectedType}})');
    assert.equal(app.run("providers[0].api_url"), "");
  }
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

test("routing remains available with a classifier notice for deterministic policy", async () => {
  const app = consoleFixture(() => response(models(1)));
  app.setProviders([draft("ollama")]);
  app.run('configurationLoaded = true; location.hash = "#routing";');
  await app.run("testAllProviders()");
  assert.equal(app.run("location.hash"), "#routing");
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
  assert.equal(app.run("location.hash"), "#routing");
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
  app.run("applyConfig(legacyConfig, fixtureRevision)");
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
  app.run("applyConfig(oldConfig, fixtureRevision)");
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
  app.run("applyConfig(disabledConfig, fixtureRevision)");
  await app.run("testAllProviders()");
  assert.equal(app.run("classifierSelect.value"), "disabled");
  assert.equal(app.run("readConfig().router.classifier.type"), "disabled");
  assert.match(app.run("classifierSelect.innerHTML"), /laya\/english/);
});

test("saving an existing Ollama with a classifier omits unsupported generic fields", () => {
  const app = consoleFixture(() => response(models(1)));
  app.context.ollamaConfig = { providers:[{ id:"local", type:"ollama", base_url:"http://localhost:11434" }] };
  app.run("applyConfig(ollamaConfig, fixtureRevision)");
  const output = app.run("readConfig().providers[0]");
  assert.deepEqual(Object.keys(output).sort(), ["base_url", "enabled", "id", "treat_models_as_free", "type"]);
});

test("Ollama request timeout is editable, survives reload and can reset to default", () => {
  const app = consoleFixture(() => response(models(1)));
  app.context.config = { providers:[{ id:"local", type:"ollama", base_url:"http://localhost:11434", request_timeout:"10m" }] };
  app.run("applyConfig(config, fixtureRevision)");
  assert.match(app.list.innerHTML, /data-key="request_timeout" value="10m" placeholder="5m"/);
  assert.equal(app.run("readConfig().providers[0].request_timeout"), "10m");
  app.run('providers[0].request_timeout = "15m"');
  assert.equal(app.run("readConfig().providers[0].request_timeout"), "15m");
  app.run('providers[0].request_timeout = ""');
  assert.equal(app.run("readConfig().providers[0].request_timeout"), undefined);
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
  assert.match(app.list.innerHTML, />API key<span/);
  assert.match(app.list.innerHTML, />Reveal<\/button>/);
  assert.doesNotMatch(app.list.innerHTML, /New API key|Show new/);
  button.listeners.click();
  assert.equal(input.type, "text");
  assert.equal(button.textContent, "Hide");
  assert.equal(button.attributes["aria-pressed"], "true");
  button.listeners.click();
  assert.equal(input.type, "password");
  assert.equal(button.textContent, "Reveal");
  assert.equal(button.attributes["aria-label"], "Reveal entered provider API key");
  assert.equal(button.attributes["aria-pressed"], "false");
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
  app.run("applyConfig(config, fixtureRevision)");
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
    app.run("applyConfig(config, fixtureRevision)");
    assert.deepEqual(JSON.parse(app.run("JSON.stringify(readProviderConfig(providers[0]).bearer_token)")), secret);
  }
  const input = app.list.querySelector("#provider-api-key-0");
  input.value = "replacement-key";
  input.listeners.input({ target:input });
  assert.equal(app.run("providers[0].api_key_file"), "");
  assert.equal(app.run("readProviderConfig(providers[0]).bearer_token.value"), "replacement-key");
  assert.equal(app.list.querySelector('[data-provider="0"][data-key="api_key_env"]'),undefined);
  app.setProviders([draft("openai-codex"), draft("nous"), draft("ollama")]);
  assert.equal(app.list.inputs.some(input => input.dataset.provider === "0" && input.dataset.key === "api_key"), false);
  assert.equal(app.list.inputs.some(input => input.dataset.provider === "1" && input.dataset.key === "api_key"), true);
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
  app.run("configurationRevision = fixtureRevision");
  await app.run("saveConfig()");
  assert.deepEqual(saved.providers[0].bearer_token, { value:"test-save-reload-key" });
  const input = app.list.querySelector("#provider-api-key-0");
  assert.equal(input.value, "");
  assert.equal(app.run("providers[0].bearer_token.credential"),"stored-key");
  assert.equal(input.type, "password");
});

test("benchmark source controls preserve weights and saved credentials and require the Artificial Analysis key", async () => {
  const app = consoleFixture(() => response({}));
  const intelligence = { enabled:true, sources:[{ id:"aa", type:"artificial_analysis", enabled:true, api_key:{ env:"AA_API_KEY" } }], routing:{ task_weights:{ coding:{ coding:3, agentic:1 } } } };
  app.context.config = { providers:[], intelligence };
  app.run("applyConfig(config, fixtureRevision)");
  const saved = JSON.parse(app.run("JSON.stringify(readConfig().intelligence)"));
  assert.deepEqual(saved.routing, intelligence.routing);
  assert.deepEqual(saved.sources[0].api_key, intelligence.sources[0].api_key);
  assert.equal(saved.sources.length, 1);
  app.run('delete benchmarkSources[0].api_key');
  await app.run("saveConfig()");
  assert.match(app.run('document.querySelector("#toast").textContent'), /requires an API key/);
  assert.equal(app.calls.length, 0);
});

test("benchmark refresh uses memory-only login rather than configuration credentials", async () => {
  const app = consoleFixture((url, options) => {
    assert.equal(options.headers.Authorization,undefined);
    if (url.endsWith("/sources")) return response({aa:{last_success:"2026-10-01",enabled:true,observation_count:10}});
    if (url.includes("/models")) return response({});
    assert.equal(url, "/gateway/v1/intelligence/refresh");
    assert.equal(options.method, "POST");
    assert.equal(options.headers.Accept,"text/event-stream");
    return response({ updated:true, sources:{ aa:{ last_success:"2026-10-01", stale:false } } });
  });
  app.run('consoleSession="Basic test-admin-login"');
  app.context.config = { providers:[], intelligence:{enabled:true}, server:{ authentication:{ api_key:{ credential:"stored-gateway-key" } } } };
  app.run("applyConfig(config, fixtureRevision)");
  await app.run("inspectBenchmarks(true)");
  assert.match(app.run('document.querySelector("#benchmark-status").textContent'), /refresh finished/);
  assert.equal(app.run('document.querySelector("#refresh-benchmarks").disabled'), false);
});

test("console uses a cookie session without retaining administrator credentials",async()=>{
 const app=consoleFixture((url,options)=>{
  if(url==="/gateway/v1/session")return response({username:"admin",csrf_token:"csrf",expires_at:"tomorrow"});
  assert.equal(options.headers.Authorization,undefined);return response({providers:[]});
 });
 app.run('document.querySelector("#login-username").value="admin";document.querySelector("#login-password").value="memory-login-sentinel";');
 await app.run('document.querySelector("#login-form").listeners.submit({preventDefault(){}})');
 assert.equal(app.run('consoleSession'),"tomorrow");assert.equal(app.run('csrfToken'),"csrf");assert.equal(app.run('document.querySelector("#login-password").value'),"");assert.deepEqual(app.storageWrites,[]);
 assert.equal(app.calls[0].url,"/gateway/v1/session");assert.equal(app.calls[0].options.credentials,"same-origin");
 app.run('signOutConsole()');assert.equal(app.run('csrfToken'),"");
});

test("unauthorized management clears login and reopens the sign-in dialog",async()=>{
  const app=consoleFixture(()=>({...response({},false),status:401}));app.run('consoleSession="Basic expired-login"');await app.run('loadCredentialMetadata()');assert.equal(app.run('consoleSession'),"");assert.equal(app.run('document.querySelector("#login-dialog").open'),true);
});
test("legacy classifier conversion preserves encrypted credential references",()=>{
  const app=consoleFixture(()=>response({}));app.context.config={providers:[],router:{classifier:{type:"jev",base_url:"https://api.typesafe.ai",model:"jev-latest",api_key:{credential:"classifier-ref"}}}};app.run("applyConfig(config, fixtureRevision)");assert.equal(app.run('readProviderConfig(providers[0]).bearer_token.credential'),"classifier-ref");
});

test("classifier tuning keeps defaults optional and panel collapsed", () => {
  const app = consoleFixture(() => response({}));
  app.context.config = { providers:[], router:{ classifier:{ type:"disabled" } } };
  app.run("applyConfig(config, fixtureRevision)");
  const classifier = app.run("readConfig().router.classifier");
  for (const name of ["timeout", "minimum_confidence", "questions", "noul_thresholds"]) assert.equal(classifier[name], undefined);
  const html = readFileSync(new URL("./static/index.html", import.meta.url), "utf8");
  assert.match(html, /<details id="classifier-tuning" class="advanced-fields">/);
  assert.doesNotMatch(html, /<details[^>]*id="classifier-tuning"[^>]*\bopen\b/);
});

test("saved classifier tuning survives model replacement and disabled selection", () => {
  const app = consoleFixture(() => response({}));
  app.context.config = { providers:[], router:{ classifier:{ type:"jev", transport:"provider", provider:"laya", model:"english", timeout:"750ms", minimum_confidence:.4, on_failure:"fail", questions:{ task_type:{ instructions:"Review source code", criteria:{ coding:"Software development" } } }, noul_thresholds:{ tools:0, reasoning:1 } } } };
  app.run("applyConfig(config, fixtureRevision)");
  app.run('classifierSelect.value="other/multilingual"; classifierSelect.listeners.change()');
  let saved = app.run("readConfig().router.classifier");
  assert.equal(saved.provider, "other");
  assert.equal(saved.timeout, "750ms");
  assert.equal(saved.minimum_confidence, .4);
  assert.equal(saved.on_failure, "fail");
  assert.equal(saved.questions.task_type.criteria.coding, "Software development");
  assert.equal(saved.noul_thresholds.tools, 0);
  app.run('classifierSelect.value="disabled"; classifierSelect.listeners.change()');
  saved = app.run("readConfig().router.classifier");
  assert.equal(saved.type, "disabled");
  assert.equal(saved.questions.task_type.instructions, "Review source code");
  assert.equal(saved.noul_thresholds.reasoning, 1);
});

test("unsaved question edits survive question changes and provider discovery", async () => {
  const app = consoleFixture(() => response({ok:true, models:[{id:"laya/english"}]}));
  app.context.config = { providers:[{ id:"laya", type:"jev_compatible", base_url:"http://localhost:8000" }], router:{ classifier:{ type:"jev", transport:"provider", provider:"laya", model:"english" } } };
  app.run("applyConfig(config, fixtureRevision)");
  app.run('classifierInstructions.value="Classify code reviews as coding"; classifierInstructions.listeners.input(); classifierOption.value="coding"; classifierOption.listeners.change(); classifierRubric.value="Developing and reviewing software"; classifierRubric.listeners.input()');
  app.run('classifierQuestion.value="tools"; classifierQuestion.listeners.change(); classifierThreshold.value="0.8"; classifierThreshold.listeners.input(); classifierRubric.value="Needs external actions"; classifierRubric.listeners.input()');
  await app.run("testAllProviders()");
  app.run('classifierQuestion.value="task_type"; classifierQuestion.listeners.change(); classifierOption.value="coding"; classifierOption.listeners.change()');
  assert.equal(app.run("classifierInstructions.value"), "Classify code reviews as coding");
  assert.equal(app.run("classifierRubric.value"), "Developing and reviewing software");
  const saved = app.run("readConfig().router.classifier");
  assert.equal(saved.questions.task_type.criteria.coding, "Developing and reviewing software");
  assert.equal(saved.questions.tools.criteria.true, "Needs external actions");
  assert.equal(saved.noul_thresholds.tools, .8);
});

test("clearing tuning restores defaults and loading another config clears prior controls", () => {
  const app = consoleFixture(() => response({}));
  app.context.config = { providers:[], router:{ classifier:{ type:"disabled", timeout:"2s", minimum_confidence:.4, questions:{ task_type:{ instructions:"Custom" } }, noul_thresholds:{ tools:.8 } } } };
  app.run("applyConfig(config, fixtureRevision)");
  app.run('classifierInstructions.value=""; classifierInstructions.listeners.input(); classifierQuestion.value="tools"; classifierQuestion.listeners.change(); classifierThreshold.value=""; classifierThreshold.listeners.input(); form.elements.namedItem("router.classifier.timeout").value=""; form.elements.namedItem("router.classifier.minimum_confidence").value=""');
  const cleared = app.run("readConfig().router.classifier");
  for (const name of ["timeout", "minimum_confidence", "questions", "noul_thresholds"]) assert.equal(cleared[name], undefined);
  app.run("applyConfig(config, fixtureRevision); applyConfig({ providers:[], router:{ classifier:{ type:'disabled' } } })");
  assert.equal(app.run('form.elements.namedItem("router.classifier.timeout").value'), "");
  assert.equal(app.run('form.elements.namedItem("router.classifier.minimum_confidence").value'), "");
  assert.equal(app.run("readConfig().router.classifier.questions"), undefined);
});

test("invalid confidence draft cannot silently save as null", async () => {
  const app = consoleFixture(() => response({}));
  app.run("configurationRevision = fixtureRevision");
  app.run('form.elements.namedItem("router.classifier.minimum_confidence").value="invalid"');
  await app.run("saveConfig()");
  assert.equal(app.calls.filter(call => call.options?.method === "PUT").length, 0);
  assert.match(app.run('document.querySelector("#toast").textContent'), /Minimum confidence/);
});

test("saving an unchecked paid-model policy preserves false after reload", async () => {
  let persisted = { providers:[], router:{ allow_paid_models:false, classifier:{ type:"disabled" } } };
  const app = consoleFixture((_url, options) => {
    if (options?.method === "PUT") persisted = JSON.parse(options.body);
    return response(persisted);
  });
  app.context.config = persisted;
  app.run("applyConfig(config, fixtureRevision)");
  assert.equal(app.run('form.elements.namedItem("router.allow_paid_models").checked'), false);
  await app.run("saveConfig()");
  assert.equal(persisted.router.allow_paid_models, false);
  assert.equal(app.run("readConfig().router.allow_paid_models"), false);
  app.run("applyConfig({providers:[], router:{classifier:{type:'disabled'}}})");
  assert.equal(app.run('form.elements.namedItem("router.allow_paid_models").checked'), true);
  app.run('form.elements.namedItem("router.allow_paid_models").checked=false');
  assert.equal(app.run("readConfig().router.allow_paid_models"), false);
});

test("setup status selects initial administrator setup or existing-account login",async()=>{
  const app=consoleFixture(()=>response({setup_required:true}));await app.run("checkSetup()");assert.equal(app.run('document.querySelector("#setup-dialog").open'),true);
  let prevented=false;app.run('document.querySelector("#setup-dialog").listeners.cancel')( {preventDefault(){prevented=true;} });assert.equal(prevented,true);assert.equal(app.run('document.querySelector("#setup-dialog").open'),true);
  app.setFetch(url=>url==="/gateway/v1/session"?response({},false,null,401):response({setup_required:false}));await app.run("checkSetup()");assert.equal(app.context.redirect,"/login");
});
test("administrator setup clears password fields and reveals only its newly generated key",async()=>{
  const app=consoleFixture((url,options)=>{
    if(url==="/gateway/v1/setup"){const input=JSON.parse(options.body);assert.equal(input.username,"my-admin");assert.equal(input.password,"long-enough-password");assert.equal(input.setup_code,"terminal-code");return response({api_key:"new-once-key",warning:"Displayed only once"});}
    if(url==="/gateway/v1/session")return response({username:"my-admin",csrf_token:"csrf",expires_at:"tomorrow"});
    if(url==="/gateway/v1/config"){assert.equal(options.headers.Authorization,undefined);return response({providers:[]});}
    return response({credentials:[]});
  });
  app.run('document.querySelector("#setup-username").value="my-admin";document.querySelector("#setup-password").value="long-enough-password";document.querySelector("#setup-confirm").value="long-enough-password";document.querySelector("#setup-code").value="terminal-code"');
  await app.run('document.querySelector("#setup-form").listeners.submit({preventDefault(){}})');
  assert.equal(app.run('document.querySelector("#signed-in-user").textContent'),"my-admin");
  assert.equal(app.run('document.querySelector("#console-login").hidden'),true);
  for(const id of ["#setup-password","#setup-confirm","#setup-code"])assert.equal(app.run(`document.querySelector("${id}").value`),"");
  assert.equal(app.run('document.querySelector("#new-api-key").value'),"new-once-key");assert.deepEqual(app.storageWrites,[]);
  app.run('document.querySelector("#close-new-key").listeners.click()');assert.equal(app.run('document.querySelector("#new-api-key").value'),"");
});

test("benchmark enable toggle persists without dropping source configuration", async () => {
  let config = {providers:[],intelligence:{enabled:false,sources:[{id:"cache",type:"benchlm",enabled:false}],routing:{price:1}}};
  const app = consoleFixture((url, options) => {
    if (url === "/gateway/v1/config") {
      if (options?.method === "PUT") config = JSON.parse(options.body);
      return response(config);
    }
    return response({sources:{cache:{enabled:false}}});
  });
  app.context.config = config;
  app.run("applyConfig(config, fixtureRevision)");
  assert.equal(app.run('document.querySelector("#refresh-benchmarks").disabled'),true);
  await app.run("inspectBenchmarks(true)");
  assert.equal(app.calls.length,0);
  assert.match(app.run('document.querySelector("#benchmark-status").textContent'),/Enable benchmark intelligence/);
  app.run('document.querySelector("#intelligence-enabled").checked=true;document.querySelector("#intelligence-enabled").listeners.change()');
  assert.equal(app.run('document.querySelector("#refresh-benchmarks").disabled'),true);
  assert.match(app.run('document.querySelector("#benchmark-enable-notice").textContent'),/wait for Saved/i);
  await app.run("saveConfig()");
  assert.equal(config.intelligence.enabled,true);
  assert.equal(config.intelligence.sources[0].id,"cache");
  assert.equal(config.intelligence.routing.price,1);
  assert.equal(app.run('document.querySelector("#refresh-benchmarks").disabled'),false);
  await app.run("inspectBenchmarks(true)");
  assert.ok(app.calls.some(call=>call.url==="/gateway/v1/intelligence/refresh"));
  app.run('document.querySelector("#intelligence-enabled").checked=false;document.querySelector("#intelligence-enabled").listeners.change()');
  assert.equal(app.run('document.querySelector("#intelligence-enabled").checked'),false);
  assert.equal(app.run('readConfig().intelligence.enabled'),false);
});

test("benchmark page replaces JSON with five source controls and cached evidence tables", () => {
  const html = readFileSync(new URL("./static/index.html",import.meta.url),"utf8");
  assert.doesNotMatch(html,/intelligence-config|Benchmark configuration JSON/i);
  for (const id of ["benchmark-sources-list","benchmark-table","benchmark-refresh-log","benchmark-progress"]) assert.ok(html.includes(`id="${id}"`));
  const app = consoleFixture(()=>response({}));
  app.context.config={providers:[],intelligence:{enabled:true}};
  app.run("applyConfig(config, fixtureRevision)");
  assert.equal(app.run("benchmarkSources.length"),5);
  assert.equal(app.run('document.querySelector("#benchmark-configuration").hidden'),false);
  const sourceHTML=app.run('document.querySelector("#benchmark-sources-list").innerHTML');
  for (const name of ["Artificial Analysis","BenchLM.ai","Hugging Face","LMArena","LiveBench"]) assert.ok(sourceHTML.includes(name));
  assert.equal(app.run("readConfig().intelligence.sources.length"),0,"unused placeholder sources must not fail configuration validation");
});

test("benchmark source keys are masked, only newly entered keys are revealable, and logout clears drafts", () => {
  const app=consoleFixture(()=>response({}));
  app.context.config={providers:[],intelligence:{enabled:true,sources:[{id:"aa",type:"artificial_analysis",enabled:true,api_key:{credential:"encrypted-aa"}}]}};
  app.run('applyConfig(config, fixtureRevision);benchmarkSources[0].api_key_input="source-secret-sentinel";renderBenchmarkSources()');
  assert.doesNotMatch(app.run('document.querySelector("#benchmark-sources-list").innerHTML'),/source-secret-sentinel/);
  const input=app.run('document.querySelector("#benchmark-sources-list").inputs[0]');
  assert.equal(input.value,"source-secret-sentinel");assert.equal(input.type,"password");
  app.run('benchmarkSourceAction({target:{closest:()=>({dataset:{benchmarkSource:"0",benchmarkAction:"reveal-key"},setAttribute(){}})}})');
  assert.equal(input.type,"text");
  assert.equal(app.run('readConfig().intelligence.sources[0].api_key.value'),"source-secret-sentinel");
  assert.equal(app.run('benchmarkSafeMessage("source-secret-sentinel rejected")'),"[redacted] rejected");
  app.run('clearSubmittedBenchmarkKeys()');
  assert.equal(app.run('readConfig().intelligence.sources[0].api_key.credential'),"encrypted-aa");
  app.run('signOutConsole()');
  assert.equal(app.run("benchmarkSources.length"),0);assert.equal(app.run("Object.keys(benchmarkModels).length"),0);assert.deepEqual(app.storageWrites,[]);
});

test("sources only need enablement and required keys; shared presets supply valid datasets", () => {
  const app=consoleFixture(()=>response({}));
  app.context.config={providers:[],intelligence:{enabled:true,identities:[{canonical_id:"model",aliases:["Model"]}],aggregation:{overall_weights:{preference:0}},routing:{price:1}}};
  app.run('applyConfig(config, fixtureRevision);benchmarkSources.forEach(source=>source.enabled=true);benchmarkSources[0].api_key_input="source-key";renderBenchmarkSources()');
  const config=JSON.parse(app.run("JSON.stringify(readConfig().intelligence)"));
  assert.deepEqual(config.identities,app.context.config.intelligence.identities);
  assert.deepEqual(config.aggregation,app.context.config.intelligence.aggregation);
  assert.equal(config.sources.length,5);
  const hf=config.sources.find(source=>source.type==="huggingface");
  assert.equal(hf.datasets[0].dataset,"open-llm-leaderboard/contents");
  assert.equal(hf.datasets[0].model_field,"fullname");
  assert.equal(hf.datasets[0].metrics["IFEval Raw"].max,1);
  const html=app.run('document.querySelector("#benchmark-sources-list").innerHTML');
  assert.equal([...html.matchAll(/data-benchmark-field="api_key_input"/g)].length,4);
  assert.doesNotMatch(html,/data-benchmark-field="(?:id|url|metric|dataset|max_rows|include_estimated)"|Metric mapping|Add dataset/);
  assert.doesNotMatch(readFileSync(new URL("./static/index.html",import.meta.url),"utf8"),/data-benchmark-global/);
  app.run('updateBenchmarkSource({target:{dataset:{benchmarkSource:"2",benchmarkField:"enabled"},type:"checkbox",checked:false}})');
  assert.equal(app.run('document.querySelector("#refresh-benchmarks").disabled'),true);
});

test("hidden custom source settings survive saves without being replaced by presets",()=>{
  const app=consoleFixture(()=>response({}));
  const source={id:"hf-custom",type:"huggingface",enabled:true,url:"https://custom.example/rows",max_rows:42,api_key:{credential:"hf-key"},datasets:[{dataset:"org/results",config:"subset",split:"test",model_field:"model_id",date_field:"date",metrics:{accuracy:{category:"knowledge",min:0,max:100}}}]};
  app.context.config={providers:[],intelligence:{enabled:true,sources:[source],source_timeout:"45m",aggregation:{source_weights:{"hf-custom":2}}}};
  app.run('applyConfig(config, fixtureRevision)');
  const saved=JSON.parse(app.run('JSON.stringify(readConfig().intelligence)'));
  assert.deepEqual(saved.sources[0],source);assert.equal(saved.source_timeout,"45m");assert.equal(saved.aggregation.source_weights["hf-custom"],2);
});

test("cached model table combines capabilities, operational data and unmatched models safely", async () => {
  const app=consoleFixture(url=>{
    if(url.endsWith("/sources"))return response({aa:{enabled:true,last_success:"2026-10-02",observation_count:2}});
    assert.equal(url,"/gateway/v1/intelligence/models?include_unmatched=true");
    return response({"provider/model":{provider_ids:["provider/model"],scores:{coding:.92,preference:.81},operations:{ttft:.2,context_length:128000},sources:["aa"],observation_count:2,latest_retrieval:"2026-10-02"},"external/unmatched":{display_name:"<unmatched>",match:"unmatched",scores:{coding:.85,agentic:.8,knowledge:.9},sources:["aa"],observation_count:3}});
  });
  app.run('consoleSession="Basic admin";applyBenchmarkConfig({enabled:true})');
  await app.run('loadBenchmarkData()');
  app.run('openBenchmarkEvidence("provider/model")');
  let html=app.run('document.querySelector("#benchmark-table-body").innerHTML + document.querySelector("#benchmark-evidence-content").innerHTML');
  assert.match(html,/92\.0/);assert.match(html,/81\.0/);assert.match(html,/128000/);assert.match(html,/—/);
  assert.match(html,/&lt;unmatched&gt;/);assert.match(html,/85\.0/);assert.match(html,/Unmatched/);
  assert.equal([...html.matchAll(/<tr>/g)].length,2);
  assert.doesNotMatch(html,/confidence_interval|upstream|<script>|benchmark-metadata/);
  assert.doesNotMatch(app.run('document.querySelector("#benchmark-table-head").innerHTML'),/>Metadata</);
});

test("compact benchmark scorecards retain every available score and operational evidence", () => {
 const app=consoleFixture(()=>response({}));
 app.context.models={"external/claude-opus-5":{display_name:"claude-opus-5",match:"unmatched",scores:{overall:.9,coding:.92,agentic:.88,knowledge:.94,math:0},operations:{input_price:.25,output_price:1.2,latency:2,ttft:.3,throughput:48,context_length:128000},provider_ids:["p/model"],sources:["source-a"],observation_count:8,latest_retrieval:"2026-10-08",latest_evaluation:"2026-10-01",age_seconds:120,valid_until:"2026-10-09",metadata:{hidden:"metadata-sentinel"}}};
 app.run('benchmarkModels=models;renderBenchmarkTable()');
 const headers=[...app.run('document.querySelector("#benchmark-table-head").innerHTML').matchAll(/<th[^>]*>(.*?)<\/th>/g)].map(match=>match[1]);
 assert.deepEqual(headers,["Model","Overall %","Capability scores","Sources","Match / freshness","Evidence"]);
 app.run('openBenchmarkEvidence("external/claude-opus-5")');
 const html=app.run('document.querySelector("#benchmark-table-body").innerHTML + document.querySelector("#benchmark-evidence-content").innerHTML');
 assert.equal([...html.matchAll(/<tr>/g)].length,1);
 assert.match(html,/<td data-label="Model">claude-opus-5<\/td>/);
 assert.match(html,/<td data-label="Overall %" class="benchmark-overall">90\.0<\/td>/);
 for(const [name,score] of [["Coding %","92.0"],["Agentic %","88.0"],["Knowledge %","94.0"],["Math %","0.0"],["Reasoning %","—"]])assert.ok(html.includes(`<dt>${name}</dt><dd>${score}</dd>`));
 for(const value of ["128000","0.25","1.2","48","2026-10-08","2026-10-01","2026-10-09","p/model","source-a"])assert.ok(html.includes(value));
 assert.match(html,/data-benchmark-inspect="external\/claude-opus-5"/);
 assert.match(html,/Unmatched/);assert.match(html,/display only/);
 assert.doesNotMatch(html,/metadata-sentinel/);
});

test("empty benchmark evidence uses a compact six-column state",()=>{
 const app=consoleFixture(()=>response({}));app.run('renderBenchmarkTable()');
 assert.equal([...app.run('document.querySelector("#benchmark-table-head").innerHTML').matchAll(/<th /g)].length,6);
 assert.match(app.run('document.querySelector("#benchmark-table-body").innerHTML'),/colspan="6"/);
 assert.match(app.run('document.querySelector("#benchmark-table-body").innerHTML'),/No benchmark evidence in this view/);
});

test("streamed refresh advances actual source progress and updates the table before completion", async () => {
  let controller, publications=0;
  const app=consoleFixture((url,options)=>{
    if(url.endsWith("/refresh")){
      assert.equal(options.headers.Accept,"text/event-stream");
      return {ok:true,headers:{get:()=>"text/event-stream"},body:new ReadableStream({start(c){controller=c;}})};
    }
    if(url.endsWith("/sources"))return response({aa:{enabled:true,last_success:"2026-10-02",observation_count:publications}});
    return response(publications ? {"model-a":{scores:{coding:.9},sources:["aa"],observation_count:publications}} : {});
  });
  app.run('consoleSession="Basic admin";applyBenchmarkConfig({enabled:true})');
  const refresh=app.run('inspectBenchmarks(true)');
  await new Promise(resolve=>setImmediate(resolve));
  const send=(kind,data)=>{
    const bytes=new TextEncoder().encode(`event: ${kind}\r\ndata: ${JSON.stringify(data)}\r\n\r\n`);
    // Split framing and multibyte messages across arbitrary network chunks.
    for(let offset=0;offset<bytes.length;offset+=7)controller.enqueue(bytes.slice(offset,offset+7));
  };
  publications=1;
  send("progress",{phase:"published",source:"aa",total:2,completed:1,observations:1,message:"Saved source data — matched models"});
  await new Promise(resolve=>setImmediate(resolve));
  assert.match(app.run('document.querySelector("#benchmark-table-body").innerHTML'),/model-a/);
  assert.equal(app.run('document.querySelector("#benchmark-progress").value'),1);
  assert.equal(app.run('document.querySelector("#benchmark-progress").max'),3);
  assert.equal(app.run('benchmarkRefreshRunning'),true);
  assert.match(app.run('document.querySelector("#benchmark-refresh-log").textContent'),/matched models/);
  send("progress",{phase:"failed",source:"slow",total:2,completed:2,failed:1,message:"Previous cached data retained"});
  send("result",{partial_failure:true,sources:{aa:{enabled:true,observation_count:1,last_success:"2026-10-02"}}});controller.close();await refresh;
  assert.equal(app.run('document.querySelector("#benchmark-progress").value'),3);
  assert.equal(app.run('benchmarkRefreshRunning'),false);
  assert.match(app.run('document.querySelector("#benchmark-status").textContent'),/source failures/);
});

test("refresh button shows activity immediately and table requests cannot block progress",async()=>{
  const index = readFileSync(new URL("./static/index.html", import.meta.url),"utf8");
  assert.ok(index.indexOf('id="benchmark-refresh-panel"') < index.indexOf('id="benchmark-sources-list"'),"progress must appear above the long source-card list");
  let controller;
  const pendingModels=[];
  const app=consoleFixture(url=>{
    if(url.endsWith("/refresh"))return {ok:true,headers:{get:()=>"text/event-stream"},body:new ReadableStream({start(c){controller=c;}})};
    if(url.endsWith("/sources"))return response({aa:{enabled:true,observation_count:1}});
    return new Promise(resolve=>pendingModels.push(()=>resolve(response({ready:{scores:{coding:.9}}}))));
  });
  app.run('consoleSession="Basic admin";applyBenchmarkConfig({enabled:true})');
  const refresh=app.run('document.querySelector("#refresh-benchmarks").listeners.click()');
  assert.equal(app.run('document.querySelector("#benchmark-refresh-panel").hidden'),false);
  assert.equal(app.run('document.querySelector("#refresh-benchmarks").textContent'),"Refreshing…");
  await new Promise(resolve=>setImmediate(resolve));
  const send=(kind,event)=>controller.enqueue(new TextEncoder().encode(`event: ${kind}\ndata: ${JSON.stringify(event)}\n\n`));
  send("progress",{phase:"published",source:"aa",completed:1,total:2,message:"Saved first source"});
  send("progress",{phase:"rate_limited",source:"livebench",completed:1,total:2,message:"Waiting 120 seconds for quota reset"});
  await new Promise(resolve=>setImmediate(resolve));
  assert.match(app.run('document.querySelector("#benchmark-refresh-log").textContent'),/Waiting 120 seconds/);
  assert.equal(app.run('document.querySelector("#benchmark-progress").value'),1);
  send("heartbeat",{});
  await new Promise(resolve=>setImmediate(resolve));
  assert.match(app.run('document.querySelector("#benchmark-progress-label").textContent'),/still running/);
  send("progress",{phase:"published",source:"livebench",completed:2,total:2});
  send("result",{sources:{aa:{enabled:true},livebench:{enabled:true}}});controller.close();
  await new Promise(resolve=>setImmediate(resolve));
  assert.match(app.run('document.querySelector("#benchmark-status").textContent'),/Loading/);
  pendingModels.forEach(resolve=>resolve());
  await refresh;
  assert.match(app.run('document.querySelector("#benchmark-table-body").innerHTML'),/ready/);
  assert.match(app.run('document.querySelector("#benchmark-status").textContent'),/finished; table updated/);
  assert.equal(app.run('document.querySelector("#refresh-benchmarks").textContent'),"Refresh benchmarks");
});

test("unmatched model summaries populate the table and scopes never fall back to score rows",async()=>{
  const app=consoleFixture(url=>url.endsWith("/sources")?response({benchlm:{enabled:true,observation_count:3}}):url.endsWith("/models?configured_only=true")?response({}):response({"external/model":{display_name:"external-model",match:"unmatched",sources:["benchlm"],scores:{coding:.8,agentic:.7,knowledge:.9},observation_count:3}}));
  app.run('consoleSession="Basic admin";applyBenchmarkConfig({enabled:true})');
  await app.run('loadBenchmarkData()');
  assert.equal(app.run('benchmarkTableView'),"models");
  assert.match(app.run('document.querySelector("#benchmark-table-body").innerHTML'),/external-model/);
  assert.match(app.run('document.querySelector("#benchmark-table-body").innerHTML'),/Unmatched/);
  assert.equal(app.run('Object.keys(benchmarkModels).length'),1);
  app.run('document.querySelector("#benchmark-table-view").value="routable";document.querySelector("#benchmark-table-view").listeners.change()');
  await new Promise(resolve=>setImmediate(resolve));
  assert.equal(app.run('benchmarkTableView'),"routable");
  assert.equal(app.run('Object.keys(benchmarkModels).length'),0);
  assert.ok(app.calls.every(call=>!call.url.includes("/observations")));
  const index=readFileSync(new URL("./static/index.html",import.meta.url),"utf8");
  assert.doesNotMatch(index,/value="observations"|Raw benchmark results/);
});

test("refresh completion waits for both the table response and the browser paint",async()=>{
  let finishModels;
  const app=consoleFixture(url=>url.endsWith("/sources")?response({}):new Promise(resolve=>{finishModels=()=>resolve(response({ready:{scores:{coding:.9}}}));}));
  app.run('consoleSession="Basic admin";applyBenchmarkConfig({enabled:true});benchmarkRefreshRunning=true;benchmarkProgressCounts={total:2,completed:2,failed:0}');
  const frames=[];app.context.window.requestAnimationFrame=callback=>frames.push(callback);
  const finished=app.run('finishBenchmarkRefresh({partial_failure:false},consoleSession,new AbortController())');
  await new Promise(resolve=>setImmediate(resolve));
  assert.match(app.run('document.querySelector("#benchmark-refresh-log").textContent'),/Loading results into the table/);
  assert.doesNotMatch(app.run('document.querySelector("#benchmark-status").textContent'),/finished/);
  assert.equal(app.run('document.querySelector("#benchmark-progress").value'),2);
  finishModels();await new Promise(resolve=>setImmediate(resolve));
  assert.match(app.run('document.querySelector("#benchmark-table-body").innerHTML'),/ready/);
  assert.doesNotMatch(app.run('document.querySelector("#benchmark-status").textContent'),/finished/);
  frames.shift()();frames.shift()();await finished;
  assert.match(app.run('document.querySelector("#benchmark-status").textContent'),/finished; table updated/);
  assert.equal(app.run('document.querySelector("#benchmark-progress").value'),3);
});

test("failed table loading cannot announce a completed refresh",async()=>{
  const app=consoleFixture(url=>url.endsWith("/refresh")?response({sources:{},partial_failure:false}):response({error:"table unavailable"},false));
  app.run('consoleSession="Basic admin";applyBenchmarkConfig({enabled:true})');
  await app.run('inspectBenchmarks(true)');
  assert.match(app.run('document.querySelector("#benchmark-status").textContent'),/table could not finish updating/);
  assert.doesNotMatch(app.run('document.querySelector("#benchmark-refresh-log").textContent'),/Benchmark refresh finished/);
});

test("page sizes paginate complete model rows and filtering never splits capabilities",async()=>{
  const app=consoleFixture(url=>{
    if(url.endsWith("/sources"))return response({});
    return response(app.context.rows);
  });
  app.context.rows=Object.fromEntries(Array.from({length:120},(_,i)=>[`model-${String(i).padStart(3,"0")}`,{scores:{coding:.9,agentic:.8,knowledge:.7}}]));
  app.run('consoleSession="Basic admin";applyBenchmarkConfig({enabled:true});benchmarkModels=rows;renderBenchmarkTable()');
  assert.equal([...app.run('document.querySelector("#benchmark-table-body").innerHTML').matchAll(/<tr>/g)].length,25);
  assert.equal(app.run('document.querySelector("#benchmark-page-label").textContent'),"Page 1 of 5");
  app.run('document.querySelector("#benchmark-next").listeners.click()');
  assert.match(app.run('document.querySelector("#benchmark-table-body").innerHTML'),/model-025/);
  for(const size of [50,100]){
    app.run(`document.querySelector("#benchmark-page-size").value="${size}";document.querySelector("#benchmark-page-size").listeners.change()`);
    assert.equal([...app.run('document.querySelector("#benchmark-table-body").innerHTML').matchAll(/<tr>/g)].length,size);
  }
  app.run('benchmarkTableView="routable"');
  await app.run('loadBenchmarkData()');
  for(const size of [25,50,100]){
    app.run(`document.querySelector("#benchmark-page-size").value="${size}";document.querySelector("#benchmark-page-size").listeners.change()`);
    await new Promise(resolve=>setImmediate(resolve));
    assert.equal([...app.run('document.querySelector("#benchmark-table-body").innerHTML').matchAll(/<tr>/g)].length,size);
  }
  app.run('document.querySelector("#benchmark-table-filter").value="model-119";document.querySelector("#benchmark-table-filter").listeners.input()');
  const html=app.run('document.querySelector("#benchmark-table-body").innerHTML');
  assert.equal([...html.matchAll(/<tr>/g)].length,1);
  for(const value of ["model-119","90.0","80.0","70.0"])assert.ok(html.includes(value));
  assert.equal(app.run('document.querySelector("#benchmark-data-count").textContent'),"1 models");
});

test("interrupted refresh reports incomplete progress and ignores late data after logout", async () => {
  let controller;
  const app=consoleFixture(()=>({ok:true,headers:{get:()=>"text/event-stream"},body:new ReadableStream({start(c){controller=c;}})}));
  app.run('consoleSession="Basic admin";applyBenchmarkConfig({enabled:true})');
  let refresh=app.run('inspectBenchmarks(true)');await new Promise(resolve=>setImmediate(resolve));controller.close();await refresh;
  assert.match(app.run('document.querySelector("#benchmark-status").textContent'),/ended before completion/);
  refresh=app.run('inspectBenchmarks(true)');await new Promise(resolve=>setImmediate(resolve));
  app.run('signOutConsole()');
  controller.enqueue(new TextEncoder().encode('event: result\ndata: {"sources":{"late":{"enabled":true}}}\n\n'));controller.close();await refresh;
  assert.equal(app.run('Object.keys(benchmarkSourceStatus).length'),0);
  assert.equal(app.run('document.querySelector("#benchmark-refresh-panel").hidden'),true);
});

test("client key creation prompts for a name and reveals only the new generated key once", async () => {
  const key = {id:"gateway/client-key/test",name:"Hermes <client>",created_at:"2026-10-02"};
  const app = consoleFixture((url, options) => {
    assert.equal(url,"/gateway/v1/admin/api-keys");
    assert.equal(options.headers.Authorization,undefined);
    if (options.method === "POST") {
      assert.deepEqual(JSON.parse(options.body),{name:key.name});
      return response({key,api_key:"generated-client-key",warning:"This key is displayed only once"});
    }
    return response({api_keys:[key]});
  });
  app.run('consoleSession="Basic admin-login";document.querySelector("#create-credential").listeners.click()');
  assert.equal(app.run('document.querySelector("#create-key-dialog").open'),true);
  app.run('document.querySelector("#client-key-name").value="Hermes <client>"');
  await app.run('document.querySelector("#create-key-form").listeners.submit({preventDefault(){}})');
  assert.equal(app.run('document.querySelector("#create-key-dialog").open'),false);
  assert.equal(app.run('document.querySelector("#new-api-key").value'),"generated-client-key");
  assert.match(app.run('document.querySelector("#new-key-warning").textContent'),/only once/);
  assert.match(app.run('document.querySelector("#credential-list").innerHTML'),/Hermes &lt;client&gt;/);
  assert.doesNotMatch(app.run('document.querySelector("#credential-list").innerHTML'),/generated-client-key|replacement value/);
  app.run('document.querySelector("#close-new-key").listeners.click()');
  assert.equal(app.run('document.querySelector("#new-api-key").value'),"");
  assert.deepEqual(app.storageWrites,[]);
});

test("credential cards group wrapping metadata separately from their delete action", async () => {
  const longName = "x".repeat(128);
  const app = consoleFixture(() => response({api_keys:[
    {id:"first",name:'Hermes <client> "work"',created_at:"2026-10-02T12:34:56.123456789Z"},
    {id:"second",name:longName,created_at:"2026-10-02T12:34:56Z"},
  ]}));
  app.run('consoleSession="Basic admin-login"');
  await app.run("loadCredentialMetadata()");
  const html = app.run('document.querySelector("#credential-list").innerHTML');
  assert.equal([...html.matchAll(/class="credential-card"/g)].length,2);
  assert.match(html, /<div class="credential-card-content"><h3 class="credential-card-name">Hermes &lt;client&gt; &quot;work&quot;<\/h3><p class="credential-card-meta">Created <time title="2026-10-02T12:34:56\.123456789Z">[^<]+<\/time><\/p><\/div><button/);
  assert.ok(html.includes(`<h3 class="credential-card-name">${longName}</h3>`));
  assert.match(html,/class="button quiet credential-card-delete"[^>]*aria-label="Delete API key Hermes &lt;client&gt; &quot;work&quot;"/);
  assert.doesNotMatch(html, /class="card"/);
  assert.equal(app.run('document.querySelector("#credential-list").buttons.length'),2);
});

test("credential card styles keep long text and actions readable on narrow screens", () => {
  const styles = readFileSync(new URL("./static/styles.css", import.meta.url),"utf8");
  const index = readFileSync(new URL("./static/index.html", import.meta.url),"utf8");
  const rule = selector => {
    const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g,"\\$&");
    const match = styles.match(new RegExp(`${escaped}\\s*\\{([^}]*)\\}`));
    assert.ok(match,`missing style for ${selector}`);
    return match[1];
  };
  assert.match(index, /id="credential-list" class="grid credential-list"/);
  assert.match(rule(".credential-list"), /grid-template-columns:\s*minmax\(0,\s*1fr\)/);
  assert.match(rule(".credential-list"), /margin-bottom:\s*20px/);
  assert.match(rule(".credential-card"), /display:\s*flex/);
  assert.match(rule(".credential-card"), /gap:\s*18px/);
  assert.match(rule(".credential-card"), /padding:\s*18px/);
  assert.match(rule(".credential-card-content"), /min-width:\s*0/);
  assert.match(rule(".credential-card-content"), /overflow-wrap:\s*anywhere/);
  assert.match(rule(".credential-card-name"), /margin:\s*0 0 8px/);
  assert.match(rule(".credential-card-meta"), /margin:\s*0;/);
  assert.match(rule(".credential-card-delete"), /flex:\s*0 0 auto/);
  const mobile = styles.slice(styles.indexOf("@media (max-width: 580px)"));
  assert.match(mobile, /\.credential-card\s*\{[^}]*flex-direction:\s*column/);
  assert.match(mobile, /\.credential-card-delete\s*\{[^}]*align-self:\s*flex-start/);
});

test("deleting a listed client key calls the authenticated API and reloads metadata", async () => {
  let deleted = false;
  const app = consoleFixture((url, options) => {
    assert.equal(options.headers.Authorization,undefined);
    if (options.method === "DELETE") {
      assert.equal(url,"/gateway/v1/admin/api-keys/gateway%2Fclient-key%2Ftest");
      deleted = true;
      return response({status:"deleted"});
    }
    assert.equal(url,"/gateway/v1/admin/api-keys");
    return response({api_keys:deleted?[]:[{id:"gateway/client-key/test",name:"client",created_at:"today"}]});
  });
  app.run('consoleSession="Basic admin-login"');
  await app.run("loadCredentialMetadata()");
  await app.run('document.querySelector("#credential-list").buttons[0].listeners.click()');
  assert.equal(deleted,true);
  assert.match(app.run('document.querySelector("#credential-list").innerHTML'),/No gateway API keys/);
});

test("failed login and expired authentication reset identity and clear one-time keys", async () => {
  const app=consoleFixture(()=>({...response({},false),status:401}));
  app.run('consoleSession="Basic expired";signedInUsername="admin";updateConsoleIdentity();showNewAPIKey("sensitive-once-key")');
  await app.run("loadCredentialMetadata()");
  assert.equal(app.run('document.querySelector("#signed-in-user").textContent'),"Not signed in");
  assert.equal(app.run('document.querySelector("#console-logout").hidden'),true);
  assert.equal(app.run('document.querySelector("#console-login").hidden'),false);
  assert.equal(app.run('document.querySelector("#new-api-key").value'),"");
  assert.equal(app.run('document.querySelector("#login-dialog").open'),true);
});

test("a late key-generation reply cannot reveal a secret after signing out", async () => {
  let finish;
  const app=consoleFixture(()=>new Promise(resolve=>{finish=resolve;}));
  app.run('consoleSession="Basic admin-login";signedInUsername="admin";document.querySelector("#client-key-name").value="client"');
  const creating=app.run('document.querySelector("#create-key-form").listeners.submit({preventDefault(){}})');
  await Promise.resolve();
  app.run('signOutConsole()');
  finish(response({api_key:"must-not-be-shown-after-logout"}));
  await creating;
  assert.equal(app.run('document.querySelector("#new-api-key").value'),"");
  assert.equal(app.run('document.querySelector("#signed-in-user").textContent'),"Not signed in");
});
test("API key generation requires in-memory administrator login and clears its once-only display",async()=>{
  const app=consoleFixture((url,options)=>{assert.equal(url,"/gateway/v1/admin/api-key");assert.equal(options.method,"POST");assert.equal(options.headers.Authorization,undefined);return response({api_key:"rotated-once-key"});});
  app.run('consoleSession="Basic admin-login"');await app.run('document.querySelector("#generate-api-key").listeners.click()');assert.equal(app.run('document.querySelector("#new-api-key").value'),"rotated-once-key");
  app.run('document.querySelector("#new-key-dialog").listeners.cancel()');assert.equal(app.run('document.querySelector("#new-api-key").value'),"");assert.deepEqual(app.storageWrites,[]);
});

test("stale configuration save retains Codex and secret drafts without reloading or retrying", async () => {
  const old = { providers:[{id:"ziz",type:"openai_compatible",base_url:"http://ollama.example",api_path:"/v1",discover_models:false,models:["setup"]}] };
  const app = consoleFixture((url, options) => {
    if (options?.method === "PUT") {
      assert.equal(options.headers["If-Match"], '"old"');
      return response({error:"configuration changed"}, false, null, 412);
    }
    return response(old, true, '"old"');
  });
  await app.run("loadServerConfig()");
  app.run('providers.push({...providerDefaults,id:"openai-codex",type:"openai-codex",api_url:"https://chatgpt.com/backend-api/codex",models:"gpt-5.3-codex",api_key:"draft-secret"});');
  const start = app.calls.length;
  await app.run("saveConfig()");
  assert.deepEqual(app.calls.slice(start).map(call=>call.options?.method), ["PUT"]);
  assert.equal(app.run("providers.length"),2);
  assert.equal(app.run("providers[1].api_key"),"draft-secret");
  assert.match(app.run("saveState.textContent"),/draft is still here/);
  assert.match(app.run('document.querySelector("#toast").textContent'),/reconcile/);
});

test("fresh Codex additions preserve existing discovery and Ollama timeout settings", async () => {
  for (const existing of [
    {id:"ziz",type:"ollama",base_url:"http://ollama.example",request_timeout:"10m"},
    {id:"ziz",type:"openai_compatible",base_url:"http://ollama.example",api_path:"/v1",discover_models:true,request_timeout:"10m"},
  ]) {
    let config={providers:[existing]};
    let revision='"initial"';
    const app=consoleFixture((url, options)=>{
      if (options?.method === "PUT") {
        assert.equal(options.headers["If-Match"],revision);
        config=JSON.parse(options.body);
        revision='"saved"';
      }
      return response(config,true,revision);
    });
    await app.run("loadServerConfig()");
    app.run('providers.push({...providerDefaults,id:"openai-codex",type:"openai-codex",api_url:"https://chatgpt.com/backend-api/codex",models:"gpt-5.3-codex"});');
    await app.run("saveConfig()");
    assert.deepEqual(config.providers[0],{...existing,enabled:true,treat_models_as_free:existing.type==="ollama"});
    assert.equal(app.run("configurationRevision"),'"saved"');
    await app.run("saveConfig()");
  }
});

test("missing revision blocks saving and sign-out clears the loaded revision", async () => {
  const app=consoleFixture(()=>response({providers:[]},true,null));
  await app.run("loadServerConfig()");
  await app.run("saveConfig()");
  assert.equal(app.calls.filter(call=>call.options?.method==="PUT").length,0);
  assert.match(app.run("saveState.textContent"),/before saving/);
  app.run('configurationRevision=fixtureRevision;signOutConsole()');
  assert.equal(app.run("configurationRevision"),"");
});

test("automatic saves serialize pending edits and preserve unrepresented settings",async()=>{
 let config={providers:[],router:{disabled_models:{"p/model":true},catalog_overrides:[{id:"p/model"}]}};
 let revision='"1"',finish;
 const app=consoleFixture(async(url,options)=>{
  if(options?.method==="PUT"){
   assert.equal(options.headers["If-Match"],revision);
   if(!finish)await new Promise(resolve=>{finish=resolve;});
   config=JSON.parse(options.body);revision=revision==='"1"'?'"2"':'"3"';
  }
  return response(config,true,revision);
 });
 await app.run("loadServerConfig()");
 const saving=app.run("saveConfig()");await new Promise(resolve=>setImmediate(resolve));
 app.run('loadedConfiguration.marker="pending edit";markDirty();');
 await app.run("saveConfig()");
 finish();await saving;
 assert.equal(app.calls.filter(call=>call.options?.method==="PUT").length,2);
 assert.equal(config.router.disabled_models["p/model"],true);
 assert.equal(config.router.catalog_overrides.length,1);
 assert.equal(app.run("saveState.textContent"),"Saved");
});
test("provider dialogs keep incomplete drafts out of unrelated automatic saves",()=>{
 const app=consoleFixture(()=>response({}));
 app.setProviders([draft("openai_compatible","p")]);
 app.run('openProviderEditor(0);providers[0].api_url="invalid";');
 assert.equal(app.run('readConfig().providers[0].base_url'),"https://openai_compatible.example");
 app.run('cancelProviderEditor()');
 assert.equal(app.run('providers[0].api_url'),"https://openai_compatible.example/v1");
});
test("Codex login reserves a tab before requesting device authorization and keeps a fallback link",async()=>{
 let app;
 const tab={opener:"console",closed:false,location:{replace(url){tab.url=url;}},close(){tab.closed=true;}};
 let opened=0,polls=0;
 const copied=[];
 app=consoleFixture(async(url,options)=>{
  if(options?.method==="POST" && url.endsWith("/login")){
   assert.equal(opened,1);assert.equal(tab.opener,null);
   assert.equal(JSON.parse(options.body).login_method,"device");
   return response({status:"pending",message:"Preparing login"});
  }
  if(url.includes("/login?")){
   polls++;
   if(polls===1)return response({status:"pending",verification_url:"https://auth.openai.com/codex/device",user_code:"ONE-TIME",message:"Sign in"});
   assert.equal(tab.url,"https://auth.openai.com/codex/device");
   const status=app.status(0);
   assert.equal(status.codeInput.value,"ONE-TIME");
   assert.match(status.innerHTML,/<input[^>]*type="text"[^>]*readonly/);
   assert.match(status.innerHTML,/type="button" aria-label="Copy one-time code"/);
   await status.copyButton.listeners.click();
   assert.deepEqual(copied,["ONE-TIME"]);
   assert.equal(status.copyButton.textContent,"Copied");
   assert.equal(status.copyStatus.textContent,"Code copied");
   // Unchanged polling must retain the controls and their copy feedback.
   const codeInput=status.codeInput;
   app.run('renderLoginInstructions(providers[0],{status:"pending",verification_url:"https://auth.openai.com/codex/device",user_code:"ONE-TIME",message:"Sign in"})');
   assert.equal(status.codeInput,codeInput);
   assert.equal(status.copyButton.textContent,"Copied");
   app.context.navigator.clipboard.writeText=async()=>{throw new Error("clipboard denied");};
   await status.copyButton.listeners.click();
   assert.equal(codeInput.focused,true);assert.equal(codeInput.selected,true);
   assert.match(status.copyStatus.textContent,/Copy the selected code/);
   assert.equal(status.copyButton.textContent,"Copy");
   assert.equal(app.list.editorStatuses.get("0").codeInput.value,"ONE-TIME");
   assert.match(status.innerHTML,/Open OpenAI login/);
   assert.match(app.status(0).innerHTML,/noopener noreferrer/);
   return response({status:"success"});
  }
  return response(models(2));
 });
 app.context.navigator.clipboard={writeText:async code=>{copied.push(code);}};
 app.context.window.open=(url,target)=>{opened++;assert.equal(url,"about:blank");assert.equal(target,"_blank");return tab;};
 app.setProviders([draft("openai-codex")]);
 await app.run(`loginProvider(0,providerList.querySelector('[data-login="0"]'))`);
 assert.equal(opened,1);assert.equal(tab.closed,false);
 assert.match(app.status(0).textContent,/Logged in.*2 models/);
 assert.equal(app.list.buttons.get("login:0").disabled,false);
 const source=readFileSync(new URL("./static/app.ts",import.meta.url),"utf8");
 assert.doesNotMatch(source,/torii-auth|Download and run the helper/);
});

test("blocked Codex login tabs keep the code and link visible through delayed authorization",async()=>{
 let app,polls=0;
 app=consoleFixture((url)=>{
  if(url.includes("/login?")){
   polls++;
   if(polls>180){assert.equal(app.status(0).codeInput.value,"CODE");assert.match(app.status(0).innerHTML,/Open OpenAI login/);return response({status:"success"});}
  }
  if(url.includes("/login"))return response({status:"pending",verification_url:"https://auth.openai.com/codex/device",user_code:"CODE"});
  return response(models(1));
 });
 app.context.window.open=()=>null;app.setProviders([draft("openai-codex")]);
 await app.run(`loginProvider(0,providerList.querySelector('[data-login="0"]'))`);
 assert.equal(polls,181);assert.equal(app.list.buttons.get("login:0").disabled,false);
});

test("Codex login errors close the reserved tab and allow retry",async()=>{
 for(const stage of ["start","status","failure","unsafe URL"]){
  let polls=0;
  const app=consoleFixture((url)=>{
   if(stage==="start")return response({error:"Cannot start login"},false);
   if(url.includes("/login?")){polls++;return stage==="status"?response({message:"Cannot check login"},false):response({status:"failure",message:"Login denied"});}
   return response(stage==="unsafe URL"?{status:"pending",verification_url:"https://attacker.example/login"}:{status:"pending"});
  });
  const tab={closed:false,close(){tab.closed=true;},location:{replace(){assert.fail("unexpected navigation");}}};
  app.context.window.open=()=>tab;app.setProviders([draft("openai-codex")]);
  await app.run(`loginProvider(0,providerList.querySelector('[data-login="0"]'))`);
  assert.equal(tab.closed,true);assert.equal(app.list.buttons.get("login:0").disabled,false);
  assert.match(app.status(0).textContent,/Not working/);
  if(stage==="unsafe URL")assert.equal(polls,0);
 }
});



test("session expiry and failed reauthentication preserve settings drafts", async () => {
  const app=consoleFixture(()=>({...response({},false),status:401}));
  app.run('consoleSession="expired";configurationLoaded=true;configurationRevision="revision";providers=[{...providerDefaults,id:"draft-provider",api_key:"draft-secret"}]');
  await app.run('loadCredentialMetadata()');
  assert.equal(app.run('providers[0].id'),"draft-provider");
  assert.equal(app.run('providers[0].api_key'),"draft-secret");
  assert.equal(app.run('configurationRevision'),"revision");
  assert.equal(app.run('document.querySelector("#save-button").hidden'),false);
  await app.run('document.querySelector("#login-form").listeners.submit({preventDefault(){}})');
  assert.equal(app.run('providers[0].id'),"draft-provider");
  assert.equal(app.run('configurationRevision'),"revision");
});

test("overview request counts exclude model attempts and monthly totals",()=>{
 const app=consoleFixture(()=>response({}));
 app.run('configurationLoaded=true;providers=[{...providerDefaults,id:"local",type:"ollama"}];overviewModels=[{id:"local/a",enabled:true},{id:"local/b",enabled:false}];overviewUsage=[{key_id:"*",period:"2026-10-08",model:"",requests:3},{key_id:"*",period:"2026-10-08",model:"local/a",requests:12},{key_id:"*",period:"2026-10",model:"",requests:99},{key_id:"client",period:"2026-10-08",model:"",requests:3}];renderOverview()');
 const summary=app.run('document.querySelector("#overview-summary").innerHTML');
 assert.match(summary,/Requests today<\/span><strong class="metric-value">3<\/strong>/);
 assert.match(summary,/<strong>1<\/strong> enabled models/);
 assert.match(summary,/href="#models"/);
});

test("model catalog combines text, provider and enabled filters",()=>{
 const app=consoleFixture(()=>response({}));
 app.run('physicalModels=[{id:"a/fast",provider_id:"a",enabled:true},{id:"a/deep",provider_id:"a",enabled:false},{id:"b/fast",provider_id:"b",enabled:true}];modelSearch="fast";modelProviderFilter="a";modelStatusFilter="enabled"');
 assert.deepEqual(Array.from(app.run('filteredPhysicalModels().map(m=>m.id)')),["a/fast"]);
 app.run('modelStatusFilter="disabled"');
 assert.equal(app.run('filteredPhysicalModels().length'),0);
 app.run('modelSearch="DEEP"');
 assert.deepEqual(Array.from(app.run('filteredPhysicalModels().map(m=>m.id)')),["a/deep"]);
});

test("usage summary separates client totals from model usage and uses current selected period",()=>{
 const app=consoleFixture(()=>response({}));
 app.run('document.querySelector("#budget-scope").value="*";usageData={usage:[{key_id:"client",period:"2026-10-08",model:"",requests:2},{key_id:"client",period:"2026-10-08",model:"local/a",requests:0,input_tokens:100,output_tokens:20,estimated_cost:.5},{key_id:"*",period:"2026-10-08",model:"",requests:2},{key_id:"client",period:"2026-10",model:"",requests:80}]};renderUsagePage()');
 let html=app.run('document.querySelector("#usage-list").innerHTML');
 assert.match(html,/Client requests<\/span><strong class="metric-value">2<\/strong>/);
 assert.match(html,/Input tokens<\/span><strong class="metric-value">100<\/strong>/);
 app.run('usagePeriod="monthly";renderUsagePage()');
 html=app.run('document.querySelector("#usage-list").innerHTML');
 assert.match(html,/Client requests<\/span><strong class="metric-value">80<\/strong>/);
});

test("empty activity and unavailable metadata stay explicit",()=>{
 const app=consoleFixture(()=>response({}));
 app.run('renderActivity()');
 assert.match(app.run('document.querySelector("#activity-list").innerHTML'),/Ready for your first request/);
 app.run('activityRows=[{at:"invalid",request_id:"<unsafe>",kind:"request_complete",unknown_cost:true}];renderActivity()');
 const html=app.run('document.querySelector("#activity-results").innerHTML');
 assert.match(html,/&lt;unsafe&gt;/);
 assert.match(html,/Unknown/);
 assert.doesNotMatch(html,/<unsafe>/);
});

test("display filters never dirty or save configuration while settings still autosave",async()=>{
 const app=consoleFixture(()=>response({providers:[],router:{classifier:{type:"disabled"}}}));
 await app.run('loadServerConfig()');
 const initial=app.run('editGeneration');
 for(const id of ["budget-scope","classifier-question","classifier-question-option","model-search","model-provider-filter","model-status-filter","model-sort","model-page-size","usage-period","activity-search","activity-outcome","activity-page-size","activity-view","benchmark-table-view","benchmark-table-filter","benchmark-page-size"]){
  app.context.filterId=id;
  app.run('form.listeners.input({target:{id:filterId}});form.listeners.change({target:{id:filterId,dataset:{},closest(){return null;}}})');
 }
 await new Promise(resolve=>setImmediate(resolve));
 assert.equal(app.run('editGeneration'),initial);
 assert.equal(app.calls.filter(call=>call.options?.method==="PUT").length,0);
 app.run('form.listeners.input({target:{id:"listen-address"}});form.listeners.change({target:{id:"listen-address",dataset:{},closest(){return null;}}})');
 await new Promise(resolve=>setImmediate(resolve));
 assert.equal(app.run('editGeneration'),initial+1);
 assert.equal(app.calls.filter(call=>call.options?.method==="PUT").length,1);
});

test("activity defaults to completed requests and all-event stages are never pending",()=>{
 const app=consoleFixture(()=>response({}));
 app.run('activityRows=[{id:1,request_id:"r1",kind:"request_start"},{id:2,request_id:"r1",kind:"routing"},{id:3,request_id:"r1",kind:"request_complete",outcome:"success"}];renderActivity()');
 assert.equal(app.run('visibleActivityRows().length'),1);
 assert.doesNotMatch(app.run('document.querySelector("#activity-list").innerHTML'),/Pending/);
 app.run('activityView="all";renderActivity()');
 assert.equal(app.run('visibleActivityRows().length'),3);
 assert.equal(app.run('activityOutcome(activityRows[0])'),"Started");
 assert.equal(app.run('activityOutcome(activityRows[1])'),"Model selected");
});

test("canceling a provider editor restores draft state, status, and provider identity",()=>{
 const app=consoleFixture(()=>response({}));
 app.setProviders([draft("openai_compatible","p")]);
 app.run('const originalProvider=providers[0];setProviderStatus(originalProvider,{state:"success",message:"Connected"});classifierCatalogs.set(originalProvider,["p/model"]);saveState.textContent="Saved";openProviderEditor(0);providers[0].api_url="invalid";markDirty();cancelProviderEditor()');
 assert.equal(app.run('providers[0]===originalProvider'),true);
 assert.equal(app.run('providers[0].api_url'),"https://openai_compatible.example/v1");
 assert.equal(app.run('editGeneration'),0);
 assert.equal(app.run('saveState.textContent'),"Saved");
 assert.equal(app.status(0).textContent,"Connected");
 assert.deepEqual(Array.from(app.run('classifierCatalogs.get(providers[0])')),["p/model"]);
});

test("overview model distribution excludes monthly and duplicated client rows",()=>{
 const app=consoleFixture(()=>response({}));
 app.run('overviewUsage=[{key_id:"*",period:"2026-10-08",model:"p/a",provider:"p",input_tokens:100,output_tokens:20,estimated_cost:.5},{key_id:"client",period:"2026-10-08",model:"p/a",input_tokens:100,output_tokens:20,estimated_cost:.5},{key_id:"*",period:"2026-10",model:"p/a",input_tokens:900,output_tokens:20,estimated_cost:4}]');
 assert.equal(app.run('overviewModelUsage()[0].tokens'),120);
 assert.equal(app.run('overviewModelUsage()[0].cost'),.5);
 assert.equal(app.run('overviewModelUsage().length'),1);
});

test("model pages preserve complete model records, clamp ranges, and sort enabled models first",()=>{
 const app=consoleFixture(()=>response({}));
 app.context.largeCatalog=Array.from({length:23},(_,index)=>({id:`p/model-${String(index).padStart(2,"0")}`,provider_id:"p",enabled:index%2===0,pricing:{input_per_million:.0001}}));
 app.run('physicalModels=largeCatalog;renderModelsPage()');
 assert.equal([...app.run('document.querySelector("#model-table-body").innerHTML').matchAll(/<tr>/g)].length,10);
 assert.equal(app.run('document.querySelector("#model-count").textContent'),"1–10 of 23 models");
 assert.match(app.run('document.querySelector("#model-table-body").innerHTML'),/\$0\.0001/);
 app.run('modelPage=2;renderModelRows()');
 assert.equal([...app.run('document.querySelector("#model-table-body").innerHTML').matchAll(/<tr>/g)].length,3);
 assert.equal(app.run('document.querySelector("#model-next").disabled'),true);
 app.run('modelPage=-1;renderModelRows()');
 assert.equal(app.run('modelPage'),0);
 app.run('modelSort="enabled"');
 assert.equal(app.run('filteredPhysicalModels().slice(0,12).every(model=>model.enabled)'),true);
 app.run('modelSearch="model-22";modelPage=20;renderModelRows()');
 assert.equal(app.run('modelPage'),0);
 assert.equal(app.run('document.querySelector("#model-count").textContent'),"1–1 of 1 models · 23 total");
});

test("activity investigation filters request, model, key and outcome before pagination",()=>{
 const app=consoleFixture(()=>response({}));
 app.context.largeActivity=Array.from({length:55},(_,index)=>({id:index,kind:"request_complete",request_id:`request-${index}`,selected_model:index%2?"p/fast":"p/deep",key_name:index%2?"Research":"Development",outcome:index%3?"success":"error"}));
 app.run('activityRows=largeActivity;renderActivity()');
 assert.equal([...app.run('document.querySelector("#activity-results").innerHTML').matchAll(/<tr><td data-label/g)].length,20);
 assert.equal(app.run('document.querySelector("#activity-count").textContent'),"1–20 of 55 requests");
 app.run('activityPage=2;renderActivity()');
 assert.equal([...app.run('document.querySelector("#activity-results").innerHTML').matchAll(/<tr><td data-label/g)].length,15);
 app.run('activitySearch="RESEARCH";activityOutcomeFilter="error";renderActivity()');
 assert.equal(app.run('visibleActivityRows().every(event=>event.key_name==="Research"&&event.outcome==="error")'),true);
 app.run('activitySearch="request-54";activityOutcomeFilter="";renderActivity()');
 assert.equal(app.run('visibleActivityRows().length'),1);
 assert.equal(app.run('activityPage'),0);
 app.run('activitySearch="missing-model";renderActivity()');
 assert.match(app.run('document.querySelector("#activity-results").innerHTML'),/No requests match this view/);
});

test("overview period changes use exact monthly totals and qualify bounded activity counts",()=>{
 const app=consoleFixture(()=>response({}));
 app.run('configurationLoaded=true;providers=[{...providerDefaults,id:"p"}];overviewModels=[{id:"p/a",enabled:true}];overviewUsage=[{key_id:"*",period:"2026-10-08",model:"",requests:2},{key_id:"*",period:"2026-10",model:"",requests:17},{key_id:"*",period:"2026-10-08",model:"p/a",input_tokens:100,output_tokens:20,estimated_cost:.5},{key_id:"*",period:"2026-10",model:"p/a",input_tokens:1000,output_tokens:300,estimated_cost:4},{key_id:"client",period:"2026-10",model:"p/a",input_tokens:1000,output_tokens:300,estimated_cost:4}];overviewRecent=[{kind:"request_start"},{kind:"request_complete",outcome:"success"},{kind:"request_complete",outcome:"error",status:500}];renderOverview()');
 assert.equal(app.run('overviewModelUsage()[0].tokens'),120);
 assert.match(app.run('renderOverviewMonitoring()'),/2 completed · 1 successful · 1 errors in last 3 recorded events/);
 app.run('overviewPeriod="monthly";renderOverview()');
 assert.equal(app.run('overviewModelUsage()[0].tokens'),1300);
 assert.equal(app.run('overviewModelUsage()[0].cost'),4);
 assert.match(app.run('document.querySelector("#overview-summary").innerHTML'),/Requests this month<\/span><strong class="metric-value">17<\/strong>/);
 assert.match(app.run('document.querySelector("#overview-summary").innerHTML'),/<strong>1<\/strong> enabled models/);
 assert.match(app.run('renderOverviewMonitoring()'),/Current UTC month/);
});

test("small-screen model and activity views start with five complete records per page",()=>{
 const app=consoleFixture(()=>response({}),{mobile:true});
 assert.equal(app.run('modelPageSize'),5);
 assert.equal(app.run('activityPageSize'),5);
 app.context.records=Array.from({length:12},(_,index)=>({id:`p/m${index}`,provider_id:"p",enabled:true}));
 app.run('physicalModels=records;renderModelsPage()');
 assert.equal([...app.run('document.querySelector("#model-table-body").innerHTML').matchAll(/data-model-enabled=/g)].length,5);
});

test("pagination boundaries move keyboard focus to the remaining enabled page action",()=>{
 const app=consoleFixture(()=>response({}));
 app.context.focusHits=[];
 app.context.records=Array.from({length:12},(_,index)=>({id:`p/m${index}`,provider_id:"p",enabled:true,kind:"request_complete"}));
 app.run('physicalModels=records;document.querySelector("#model-previous").focus=()=>focusHits.push("model-previous");document.activeElement={id:"model-next"};modelPage=1;renderModelRows()');
 assert.deepEqual(Array.from(app.context.focusHits),["model-previous"]);
 app.run('activityRows=records;activityPageSize=5;activityPage=2;document.querySelector("#activity-previous").focus=()=>focusHits.push("activity-previous");document.activeElement={id:"activity-next"};renderActivity()');
 assert.deepEqual(Array.from(app.context.focusHits),["model-previous","activity-previous"]);
});

test("authenticated initial model deep links load the catalog after configuration resolves",async()=>{
 const app=consoleFixture(url=>url==="/gateway/v1/models"?response({models:[{id:"p/discovered",provider_id:"p",enabled:true}]}):response({providers:[],router:{classifier:{type:"disabled"}}}));
 app.run('location.hash="#models";consoleSession="authenticated";navigate()');
 assert.match(app.run('document.querySelector("#models-list").innerHTML'),/Loading model catalog/);
 await app.run('loadServerConfig()');await new Promise(resolve=>setImmediate(resolve));
 assert.ok(app.calls.some(call=>call.url==="/gateway/v1/models"));
 assert.match(app.run('document.querySelector("#model-table-body").innerHTML'),/p\/discovered/);
});

test("provider selection groups integrations and explains the current type",()=>{
 const app=consoleFixture(()=>response({}));app.setProviders([draft("jev_compatible","laya")]);
 const html=app.list.innerHTML;
 for(const label of ["API providers","Subscriptions","Self-hosted","Classifiers"])assert.ok(html.includes(`<optgroup label="${label}">`));
 assert.match(html,/Jev-compatible classifier, such as Laya/);
 app.setProviders([draft("openai-codex","codex")]);
 assert.match(app.list.innerHTML,/<optgroup label="Subscriptions">/);
 assert.doesNotMatch(app.list.innerHTML,/<optgroup label="API providers">/);
 assert.match(app.list.innerHTML,/subscription using OAuth login/);
});

test("opening benchmark evidence focuses its title and starts at the top of the dialog",()=>{
 const app=consoleFixture(()=>response({}));app.context.focusCalls=[];
 app.run('benchmarkModels={"fixture/model":{display_name:"Synthetic model",scores:{overall:.9}}};document.querySelector("#benchmark-evidence-dialog").scrollTop=500;document.querySelector("#benchmark-evidence-title").focus=options=>focusCalls.push(options);openBenchmarkEvidence("fixture/model")');
 assert.equal(app.run('document.querySelector("#benchmark-evidence-dialog").open'),true);
 assert.equal(app.run('document.querySelector("#benchmark-evidence-title").textContent'),"Synthetic model");
 assert.equal(app.run('document.querySelector("#benchmark-evidence-dialog").scrollTop'),0);
 assert.equal(app.context.focusCalls.length,1);
 assert.equal(app.context.focusCalls[0].preventScroll,true);
});

test("new provider drafts can switch out of subscriptions and cancel without changing saved profiles",async()=>{
 const app=consoleFixture(()=>response(models(1)));
 app.setProviders([{...draft("ollama","local"),request_timeout:"10m",headers:'{"X-Profile":{"env":"PROFILE_HEADER"}}'}]);
 app.run('const savedProfile=providers[0];const savedProfileJSON=JSON.stringify(savedProfile);saveState.textContent="Saved";providers.push({...providerDefaults,api_key:"draft-provider-secret"});renderProviders();openProviderEditor(1,true)');
 for(const type of ["openai-codex","ollama","nous","openai_compatible","jev_compatible","openai-codex","openai_compatible"]){
  app.context.nextType=type;
  app.run('updateProvider({target:{dataset:{provider:"1",key:"type"},value:nextType}})');
  assert.equal(app.run('providers[1].type'),type);
  for(const label of ["API providers","Subscriptions","Self-hosted","Classifiers"])assert.ok(app.list.innerHTML.includes(`<optgroup label="${label}">`));
  assert.doesNotMatch(app.list.innerHTML,/value="draft-provider-secret"/);
 }
 app.run('cancelProviderEditor()');await new Promise(resolve=>setImmediate(resolve));
 assert.equal(app.run('providers.length'),1);
 assert.equal(app.run('providers[0]===savedProfile'),true);
 assert.equal(app.run('JSON.stringify(providers[0])===savedProfileJSON'),true);
 assert.equal(app.run('editGeneration'),0);
 assert.equal(app.run('saveState.textContent'),"Saved");
 assert.equal(app.calls.filter(call=>call.options?.method==="PUT").length,0);
});

test("changing provider type restores keyboard focus to the replacement chooser",()=>{
 const app=consoleFixture(()=>response(models(1)));
 app.run('providers=[{...providerDefaults,id:"draft"}];renderProviders();openProviderEditor(0,true)');
 const oldType={dataset:{provider:"0",key:"type"},value:"ollama"};
 let focused=false;
 const replacement={focus(options){focused=true;assert.equal(options.preventScroll,true);app.context.document.activeElement=replacement;}};
 const originalQuery=app.list.querySelector.bind(app.list);
 app.list.querySelector=selector=>selector==='[data-provider="0"][data-key="type"]'?replacement:originalQuery(selector);
 app.context.oldType=oldType;app.context.document.activeElement=oldType;
 app.run('updateProvider({target:oldType})');
 assert.equal(focused,true);
 assert.equal(app.context.document.activeElement,replacement);
 assert.equal(app.run('providers[0].type'),"ollama");
 assert.equal(app.calls.filter(call=>call.options?.method==="PUT").length,0);
});

test("benchmark evidence groups readable UTC dates and preserves exact provenance",()=>{
 const app=consoleFixture(()=>response({}));app.run('consoleTimezone="UTC"');app.context.evidenceRow={id:"fixture/model",scores:{overall:.9},latest_retrieval:"2026-10-08T14:23:12.123456789Z",latest_evaluation:"2026-10-01",age_seconds:123.456789};
 const html=app.run('benchmarkEvidence(evidenceRow)');
 for(const heading of ["Capability scores","Operating metrics","Provenance & freshness"])assert.ok(html.includes(`<h3>${heading}</h3>`));
 assert.match(html,/title="2026-10-08T14:23:12\.123456789Z">Oct 8, 2026 · 14:23 UTC/);
 assert.match(html,/title="123\.456789">123<\/dd>/);
 assert.equal(app.run('benchmarkDate("0001-01-01T00:00:00Z")'),"Unknown");
 assert.equal(app.run('benchmarkDate(null)'),"Unknown");
});

test("mobile benchmarks start with five summaries and preserve desktop page-size choices",()=>{
 const app=consoleFixture(()=>response({}),{mobile:true});
 assert.equal(app.run('benchmarkPageSize'),5);
 assert.equal(app.run('document.querySelector("#benchmark-page-size").value'),"5");
 app.run('document.querySelector("#benchmark-page-size").value="25";document.querySelector("#benchmark-page-size").listeners.change()');
 assert.equal(app.run('benchmarkPageSize'),25);
});

test("operational overview avoids duplicate accounting and qualifies unknown costs and sampled success",()=>{
 const app=consoleFixture(()=>response({}));
 app.run('configurationLoaded=true;providers=[{...providerDefaults,id:"p"}];overviewModels=[{id:"p/m",enabled:true}];overviewUsage=[{key_id:"*",period:"2026-10-08",model:"",requests:2,input_tokens:999,estimated_cost:99},{key_id:"*",period:"2026-10-08",model:"p/m",input_tokens:100,output_tokens:20,estimated_cost:.5,unknown_cost_attempts:1},{key_id:"client",period:"2026-10-08",model:"p/m",input_tokens:100,output_tokens:20,estimated_cost:.5,unknown_cost_attempts:1},{key_id:"*",period:"2026-10",model:"p/m",input_tokens:5000,estimated_cost:20}];overviewRecent=[{kind:"request_start"},{kind:"routing"},{kind:"request_complete",outcome:"success",status:200},{kind:"request_complete",outcome:"interrupted",status:200},{kind:"request_complete",outcome:"error",status:500}];renderOverview()');
 const summary=app.run('document.querySelector("#overview-summary").innerHTML');
 assert.match(summary,/Requests today<\/span><strong class="metric-value">2<\/strong>/);
 assert.match(summary,/Processed tokens<\/span><strong class="metric-value">120<\/strong>/);
 assert.match(summary,/Estimated cost<\/span><strong class="metric-value">\$0.5000<\/strong>/);
 assert.match(summary,/Partial USD total · 1 unknown-cost attempts/);
 assert.match(summary,/Recent success<\/span><strong class="metric-value">33.3%<\/strong>/);
 assert.match(summary,/1 of 3 completed · last 5 recorded events/);
 assert.match(summary,/<div class="overview-context"/);
 assert.match(summary,/<strong>Deterministic<\/strong> routing/);
 assert.equal(app.calls.length,0);
});

test("overview distinguishes absent data from real zero usage and zero successful requests",()=>{
 const app=consoleFixture(()=>response({}));
 app.run('renderOverview()');
 let summary=app.run('document.querySelector("#overview-summary").innerHTML');
 for(const label of ["Requests today","Processed tokens","Estimated cost","Recent success"])assert.match(summary,new RegExp(`${label}<\\/span><strong class="metric-value">—<\\/strong>`));
 app.run('overviewUsage=[];overviewRecent=[];renderOverview()');
 summary=app.run('document.querySelector("#overview-summary").innerHTML');
 assert.match(summary,/Processed tokens<\/span><strong class="metric-value">0<\/strong>/);
 assert.match(summary,/Estimated cost<\/span><strong class="metric-value">\$0.0000<\/strong>/);
 assert.match(summary,/Recent success<\/span><strong class="metric-value">—<\/strong>/);
 app.run('overviewRecent=[{kind:"request_start"},{kind:"request_complete",outcome:"error",status:500}];renderOverview()');
 summary=app.run('document.querySelector("#overview-summary").innerHTML');
 assert.match(summary,/Recent success<\/span><strong class="metric-value">0.0%<\/strong>/);
 assert.match(summary,/0 of 1 completed · last 2 recorded events/);
});

test("overview usage drill-down follows its period without saving configuration",()=>{
 const app=consoleFixture(()=>response({}));
 app.run('document.querySelector("#overview-summary").querySelectorAll=()=>[document.querySelector("#metric-link")];overviewPeriod="monthly";document.querySelector("#budget-scope").value="client";renderOverview();document.querySelector("#metric-link").listeners.click()');
 assert.equal(app.run('usagePeriod'),"monthly");
 assert.equal(app.run('document.querySelector("#budget-scope").value'),"*");
 assert.equal(app.run('editGeneration'),0);
 assert.equal(app.calls.length,0);
});

test("mobile navigation exposes the current section without scrolling the workspace",()=>{
 const app=consoleFixture(()=>response({}),{mobile:true});
 app.run(`window.matchMedia=matchMedia;
 const nav=document.querySelector("#nav");nav.scrollLeft=0;nav.getBoundingClientRect=()=>({left:16,right:374,width:358});
 const overviewLink={attributes:{"aria-current":"page"},classList:{toggle(){}},getAttribute:()=>"#overview",setAttribute(name,value){this.attributes[name]=value;},removeAttribute(name){delete this.attributes[name];},getBoundingClientRect:()=>({left:16,right:114,width:98})};
 const benchmarkLink={attributes:{},classList:{toggle(){}},getAttribute:()=>"#intelligence",setAttribute(name,value){this.attributes[name]=value;},removeAttribute(name){delete this.attributes[name];},getBoundingClientRect:()=>({left:420,right:520,width:100})};
 document.querySelectorAll=selector=>selector===".nav-link"?[overviewLink,benchmarkLink]:[];syncConsoleNavigation("intelligence");`);
 assert.equal(app.run('benchmarkLink.attributes["aria-current"]'),"page");
 assert.equal(app.run('overviewLink.attributes["aria-current"]'),undefined);
 assert.equal(app.run('document.querySelector("#nav").scrollLeft'),275);
 app.run('benchmarkLink.getBoundingClientRect=()=>({left:150,right:250,width:100});syncConsoleNavigation("intelligence")');
 assert.equal(app.run('document.querySelector("#nav").scrollLeft'),275);
 app.run('window.matchMedia=()=>({matches:false});benchmarkLink.getBoundingClientRect=()=>({left:420,right:520,width:100});syncConsoleNavigation("intelligence")');
 assert.equal(app.run('document.querySelector("#nav").scrollLeft'),275);
});

test("model selectors and catalog show pricing, scoped usage and quota resets",()=>{
 const app=consoleFixture(()=>response({}));
 app.run(`loadedConfiguration={router:{}};overviewModels=[{id:"or/vendor/model",provider_id:"or",enabled:true,pricing:{input_per_million:2,output_per_million:4},quota:{known:true,source:"openrouter_key",windows:[{name:"key_spend",unit:"USD",remaining:3,reset_at:"2030-01-01T00:00:00Z"}],usage_usd:{monthly:7}},usage_available:true,usage:[{period:"2030-01",input_tokens:100,output_tokens:50,estimated_cost:0.01,unknown_cost_attempts:1}]}];physicalModels=overviewModels;renderModelsPage();renderModelRows();`);
 const selector=app.run('renderClientConnection()');
 assert.match(selector,/value="or\/vendor\/model"/);
 assert.match(selector,/\$2\.00 in \/ \$4\.00 out per 1M/);
 assert.match(selector,/\$3\.00 remaining.*resets/);
 assert.match(selector,/150 tokens.*unknown costs/);
 const rows=app.run('document.querySelector("#model-table-body").innerHTML');
 assert.match(rows,/Provider-reported spend/);assert.match(rows,/monthly: \$7\.00/);assert.match(rows,/Gateway usage/);
});

test("tracking distinguishes unknown pricing, unavailable quota and subscription usage",()=>{
 const app=consoleFixture(()=>response({}));
 assert.match(app.run('modelSelectorLabel({id:"p/unknown",pricing:{},quota:{known:false,source:"unavailable"}})'),/Unknown in \/ Unknown out.*Quota unavailable.*Usage unavailable/);
 assert.match(app.run('modelSelectorLabel({id:"codex/m",subscription:true,usage_available:true,usage:[],quota:{known:true,exhausted:true,windows:[{name:"weekly",unit:"percent",remaining:0}]}})'),/Subscription included.*Exhausted.*0\.0% remaining.*reset unknown.*No gateway usage/);
 const escaped=app.run('modelTrackingMarkup({quota:{known:true,windows:[{name:"<script>",unit:"requests",remaining:1}]},usage_available:false})');
 assert.ok(!escaped.includes('<script>'));assert.match(escaped,/&lt;script&gt;/);
});

test("provider balances page deduplicates telemetry separately from operator budgets",()=>{
 const app=consoleFixture(()=>response({}));
 app.run('usageData={usage:[],budgets:[],provider_quotas:{"nous/a":{known:false,source:"nous_response_headers"},"nous/b":{known:false,source:"nous_response_headers"}}};renderUsagePage();providerBalancesData=usageData.provider_quotas;renderProviderBalancesPage()');
 assert.doesNotMatch(app.run('document.querySelector("#usage-list").innerHTML'),/nous|subscription limits/);
 const html=app.run('document.querySelector("#provider-balances-list").innerHTML');
 assert.equal((html.match(/<strong>nous<\/strong>/g)||[]).length,1);assert.match(html,/Quota unknown/);
});


test("provider balances loads independently and supports refresh, empty and error states",async()=>{
 let data={provider_quotas:{"p/model":{known:false,balances:[{name:"credits",unit:"USD",remaining:8.75}]}}};
 const app=consoleFixture(()=>response(data));
 app.run('consoleSession="authenticated";location.hash="#provider-balances";navigate()');
 assert.equal(app.run('document.querySelector("#current-page-title").textContent'),"Provider Balances");
 await app.run('loadProviderBalancesPage()');
 assert.match(app.run('document.querySelector("#provider-balances-list").innerHTML'),/8.75/);
 data={provider_quotas:{}};await app.run('loadProviderBalancesPage()');
 assert.match(app.run('document.querySelector("#provider-balances-list").innerHTML'),/No provider telemetry available/);
 app.setFetch(()=>response({},false));await app.run('loadProviderBalancesPage()');
 assert.match(app.run('document.querySelector("#provider-balances-list").innerHTML'),/Unable to load provider balances/);
 assert.equal(app.run('document.querySelector("#refresh-provider-balances").disabled'),false);
});

test("balance-only snapshots and banked resets appear in harness selector labels",()=>{
 const app=consoleFixture(()=>response({}));
 const label=app.run('modelSelectorLabel({id:"codex/m",pricing:{},quota:{known:false,balances:[{name:"credits",unit:"USD",remaining:8.75}],reset_credits:2}})');
 assert.match(label,/credits: \$8\.75/);assert.match(label,/2 banked resets/);assert.ok(!label.includes("0.0%"));
 assert.match(app.run('modelQuotaText({quota:{known:false,balances:[{name:"account",unit:"CNY",remaining:42}]}})'),/42 CNY/);
 assert.match(app.run('modelQuotaText({quota:{known:false,reset_at:"2026-11-01T00:00:00Z"}})'),/resets/);
 const html=app.run('modelTrackingMarkup({quota:{plan:"<script>",partial:true,unavailable_reason:"spend_unavailable",balances:[{name:"account",unit:"USD",remaining:5}],reported_usage:[{period:"monthly",unit:"tokens",used:50,scope:"organization"}]}})');
 assert.match(html,/Partial provider data/);assert.match(html,/50 tokens \(organization\)/);assert.ok(!html.includes('<script>'));
});

test("new tracked provider integrations are selectable and retain billing configuration",()=>{
 const app=consoleFixture(()=>response({}));
 const options=app.run('providerTypeOptions({type:"openai_compatible"},false)');
 for(const kind of ["opencode-go","opencode-zen","standard-compute","zai","kimi-coding","minimax","deepseek"])assert.ok(options.includes(`value="${kind}"`));
 app.run('providers=[{id:"xai",type:"xai",api_url:"https://api.x.ai/v1",bearer_token:{credential:"inference"},tracking:{token:{credential:"billing"},team_id:"team"},tracking_team_id:"new-team",tracking_mode:"api"}];renderProviders()');
 const cfg=app.run('readProviderConfig(providers[0])');
 assert.equal(cfg.tracking.team_id,"new-team");assert.equal(cfg.tracking.token.credential,"billing");assert.equal(cfg.bearer_token.credential,"inference");
 assert.ok(!Object.keys(cfg).some(k=>k.startsWith("tracking_")));
 app.run('providers[0].tracking_token="replacement-synthetic"');
 assert.equal(app.run('readProviderConfig(providers[0]).tracking.token.value'),"replacement-synthetic");
});

 test("informational windows retain resets alongside balances",()=>{
 const app=consoleFixture(()=>response({}));
 const label=app.run('modelQuotaText({quota:{known:false,windows:[{name:"weekly",unit:"percent",remaining:12,informational:true,reset_at:"2030-01-01T00:00:00Z"}],balances:[{name:"credits",unit:"USD",remaining:4}],reset_credits:2}})');
 assert.match(label,/12.0% remaining \(informational\)/);assert.match(label,/resets/);assert.match(label,/credits: \$4.00/);assert.match(label,/2 banked resets/);
 });

test("selectors label provider usage separately from gateway estimates",()=>{
 const app=consoleFixture(()=>response({}));
 const label=app.run('modelSelectorLabel({id:"anthropic/m",quota:{usage_usd:{monthly:1.5},reported_usage:[{period:"monthly",unit:"USD",used:1.5,scope:"organization"},{period:"monthly",unit:"tokens",used:500,scope:"organization"}]},usage_available:true,usage:[]})');
 assert.match(label,/Provider this month: \$1.50 \(organization\); 500 tokens \(organization\)/);assert.match(label,/No gateway usage/);assert.equal((label.match(/\$1.50/g)||[]).length,1);
});

test("provider editor shows progress, retains errors and closes after successful save",async()=>{
 let reject=true,release;const app=consoleFixture((url,options)=>{
  if(url.endsWith("providers/test"))return reject?response({error:"upstream unreachable"},false):new Promise(resolve=>{release=()=>resolve(response(models(1)));});
  if(url.endsWith("config"))return response(options?.method==="PUT"?{status:"saved"}:{server:{listen:"127.0.0.1:8080"},providers:[{id:"openai_compatible",type:"openai_compatible",base_url:"https://openai_compatible.example",api_path:"/v1"}],router:{}});
  return response({});
 });
 app.setProviders([draft("openai_compatible")]);app.run('configurationRevision=fixtureRevision;openProviderEditor(0,true)');
 let dialog=app.list.dialogs.get("0"),button=app.list.buttons.get("confirm-provider:0");
 await button.listeners.click();assert.equal(dialog.open,true);assert.match(app.list.editorStatuses.get("0").textContent,/upstream unreachable/);assert.equal(app.calls.some(c=>c.options?.method==="PUT"),false);
 reject=false;const promise=button.listeners.click();await new Promise(resolve=>setImmediate(resolve));assert.equal(button.disabled,true);assert.match(app.list.editorStatuses.get("0").textContent,/Checking/);release();await promise;assert.equal(dialog.open,false,app.run("saveState.textContent"));assert.equal(app.run("editingProvider"),null);
});
test("named router settings persist independently across selector changes",()=>{
 const app=consoleFixture(()=>response({}));app.run('applyConfig({providers:[],router:{allow_paid_models:true},smart_routers:[{model_name:"local-only",allow_paid_models:false,allow_free_models:true,models:["ollama/model"]}]})');
 app.run('selectedSmartRouter="local-only";renderSmartRouterEditor()');assert.equal(app.run('form.elements.namedItem("router.allow_paid_models").checked'),false);
 const config=app.run('readConfig()');assert.equal(config.router.allow_paid_models,true);assert.equal(config.smart_routers[0].allow_paid_models,false);assert.deepEqual(Array.from(config.smart_routers[0].models),["ollama/model"]);
 app.run('selectedSmartRouter="smart";renderSmartRouterEditor()');assert.equal(app.run('form.elements.namedItem("router.allow_paid_models").checked'),true);
});
test("overview counts classifier models outside the chat catalog",()=>{
 const app=consoleFixture(()=>response({}));app.setProviders([draft("jev")]);app.run('overviewModels=[];classifierCatalogs.set(providers[0],["jev/a","jev/b"]);renderOverview()');assert.match(app.run('document.querySelector("#overview-workspace").innerHTML'),/2 models/);
});

test("smart router dual selector transfers models and enforces 250 choices",async()=>{
 const app=consoleFixture(()=>response({}));
 app.run('applyConfig({providers:[],router:{models:["local/a"]}});overviewModels=[{id:"local/a",provider_id:"local"},{id:"local/b",provider_id:"local"}];saveConfig=async()=>{};renderSmartRouterScope()');
 assert.match(app.run('document.querySelector("#smart-router-models-available").innerHTML'),/local\/b/);
 assert.ok(!app.run('document.querySelector("#smart-router-models-available").innerHTML').includes('local/a'));
 await app.run('transferSmartModels(true,"local/b")');
 assert.deepEqual(Array.from(app.run('smartRouterDrafts.smart.models')),['local/a','local/b']);
 await app.run('transferSmartModels(false,"local/a")');
 assert.deepEqual(Array.from(app.run('smartRouterDrafts.smart.models')),['local/b']);
 app.run('smartRouterDrafts.smart.models=Array.from({length:250},(_,i)=>`local/${i}`)');
 await app.run('transferSmartModels(true,"local/too-many")');
 assert.equal(app.run('smartRouterDrafts.smart.models.length'),250);
});

test("benchmark editor loads escaped rows and clears edit state on sign out",async()=>{
 const record={source:'custom',index:0,revision:'hash',observation:{external_model_id:'<model>',benchmark:'code',category:'coding',original_score:90,normalized_score:.9}};
 const app=consoleFixture((url)=>{assert.match(url,/\/intelligence\/records\?limit=25&offset=0&q=/);return response({records:[record],total:1});});
 app.run('benchmarksEnabledSaved=true;consoleSession="active"');
 await app.run('loadBenchmarkRecords()');
 assert.match(app.run('document.querySelector("#benchmark-records-body").innerHTML'),/&lt;model&gt;/);
 assert.match(app.run('document.querySelector("#benchmark-records-body").innerHTML'),/data-edit-benchmark="0"/);
 app.run('editingBenchmarkRecord=benchmarkRecords[0];clearBenchmarkRecords()');
 assert.equal(app.run('editingBenchmarkRecord'),null);assert.equal(app.run('benchmarkRecords.length'),0);
});

test("custom CSV upload sends the file with administrator CSRF headers",async()=>{
 const app=consoleFixture((url,options)=>{
  if(url.endsWith('/import')){assert.equal(options.method,'POST');assert.equal(options.headers['Content-Type'],'text/csv');assert.equal(options.headers['X-CSRF-Token'],'csrf');assert.equal(options.body.name,'custom.csv');return response({updated:true});}
  return response(url.includes('/records?')?{records:[],total:0}:{});
 });
 app.run('benchmarksEnabledSaved=true;consoleSession="active";csrfToken="csrf";document.querySelector("#benchmark-csv-file").files=[{name:"custom.csv",size:100}]');
 await app.run('uploadBenchmarkCSV()');
 assert.equal(app.calls.filter(call=>call.url.endsWith('/import')).length,1);
 assert.match(app.run('document.querySelector("#benchmark-custom-status").textContent'),/saved/);
 app.run('document.querySelector("#benchmark-csv-file").files=[{name:"huge.csv",size:9*1024*1024}]');
 await app.run('uploadBenchmarkCSV()');
 assert.equal(app.calls.filter(call=>call.url.endsWith('/import')).length,1);
 assert.match(app.run('document.querySelector("#benchmark-custom-status").textContent'),/8 MiB/);
});

test("manual benchmark edit submits numeric values and retains the revision",async()=>{
 const app=consoleFixture((url,options)=>{
  if(url.endsWith('/observations/edit')){assert.equal(options.method,'PUT');const record=JSON.parse(options.body);assert.equal(record.revision,'dataset-revision');assert.equal(record.source,'custom');assert.equal(record.observation.original_score,91);assert.equal(record.observation.normalized_score,.91);return response({updated:true});}
  return response(url.includes('/records?')?{records:[],total:0}:{});
 });
 app.run('benchmarksEnabledSaved=true;consoleSession="active";benchmarkRecords=[{source:"custom",index:0,revision:"dataset-revision",observation:{external_model_id:"local/m",benchmark:"code",category:"coding",original_score:90,normalized_score:.9}}];const editForm=document.querySelector("#benchmark-edit-form");editForm.elements={namedItem:name=>document.querySelector(`#edit-${name}`)};editForm.reportValidity=()=>true;editForm.querySelector=()=>document.querySelector("#measurement-save");openBenchmarkEdit(0);editForm.elements.namedItem("original_score").value="91";editForm.elements.namedItem("normalized_score").value="0.91"');
 await app.run('saveBenchmarkEdit({preventDefault(){}})');
 assert.equal(app.calls.filter(call=>call.url.endsWith('/observations/edit')).length,1);
 assert.equal(app.run('document.querySelector("#benchmark-edit-dialog").open'),false);
 assert.equal(app.run('editingBenchmarkRecord'),null);
});

test("manual pricing editor saves rates and restores automatic prices",async()=>{
 const app=consoleFixture(()=>response({models:[]}));
 app.run('applyConfig({providers:[],router:{},model_pricing:{"p/model":{output_per_million:2}}});consoleSession="active";physicalModels=[{id:"p/model",pricing:{input_per_million:.5,output_per_million:2,source:"operator"}}];document.querySelector("#model-pricing-form").reportValidity=()=>true;saveConfig=async()=>true;openModelPricing("p/model")');
 assert.equal(app.run('document.querySelector("#model-pricing-input").value'),"");
 assert.equal(app.run('document.querySelector("#model-pricing-input").placeholder'),"0.5");
 app.run('document.querySelector("#model-pricing-input").value="0";document.querySelector("#model-pricing-output").value="1.25"');
 await app.run('saveModelPricing({preventDefault(){}})');
 assert.equal(app.run('readConfig().model_pricing["p/model"].input_per_million'),0);
 assert.equal(app.run('readConfig().model_pricing["p/model"].output_per_million'),1.25);
 assert.equal(app.run('document.querySelector("#model-pricing-dialog").open'),false);
 app.run('openModelPricing("p/model");document.querySelector("#model-pricing-input").value="";document.querySelector("#model-pricing-output").value=""');
 await app.run('saveModelPricing()');assert.equal(app.run('modelPricingDraft["p/model"]'),undefined);
});

test("pricing CSV import validates before saving and merges only listed model rates",async()=>{
 let fail=false;
 const app=consoleFixture((url,options)=>{
  if(url.endsWith('/pricing/parse')){assert.equal(options.method,'POST');assert.equal(options.headers['X-CSRF-Token'],'csrf');return fail?response({error:'CSV row is invalid'},false):response({model_pricing:{'p/change':{input_per_million:.25,output_per_million:1},'p/clear':{}}});}
  return response({models:[]});
 });
 app.run('consoleSession="active";csrfToken="csrf";modelPricingDraft={"p/keep":{input_per_million:3},"p/change":{input_per_million:5},"p/clear":{input_per_million:1}};document.querySelector("#model-pricing-csv").files=[{name:"prices.csv",size:100}];saveConfig=async()=>true');
 await app.run('uploadModelPricingCSV()');
 assert.equal(app.run('modelPricingDraft["p/keep"].input_per_million'),3);
 assert.equal(app.run('modelPricingDraft["p/change"].input_per_million'),.25);
 assert.equal(app.run('modelPricingDraft["p/clear"]'),undefined);
 const before=app.run('JSON.stringify(modelPricingDraft)');fail=true;
 await app.run('uploadModelPricingCSV()');assert.equal(app.run('JSON.stringify(modelPricingDraft)'),before);
 assert.match(app.run('document.querySelector("#model-pricing-import-status").textContent'),/invalid/);
});

test("manual pricing errors retain drafts and model rows expose editing",async()=>{
 const app=consoleFixture(()=>response({}));
 app.run('physicalModels=[{id:"p/model",provider_id:"p",pricing:{},enabled:true}];renderModelsPage();openModelPricing("p/model");document.querySelector("#model-pricing-form").reportValidity=()=>true;document.querySelector("#model-pricing-input").value="-1";saveConfig=async()=>{throw new Error("invalid rate reached save")};');
 assert.match(app.run('document.querySelector("#model-table-body").innerHTML'),/data-model-price="p\/model"/);
 await app.run('saveModelPricing()');assert.match(app.run('document.querySelector("#model-pricing-status").textContent'),/nonnegative/);
 app.run('document.querySelector("#model-pricing-input").value="2";saveState.textContent="Conflict · draft preserved";saveConfig=async()=>false');
 await app.run('saveModelPricing()');assert.equal(app.run('modelPricingDraft["p/model"].input_per_million'),2);
 assert.equal(app.run('document.querySelector("#model-pricing-dialog").open'),true);
 app.run('clearModelPricing()');assert.equal(app.run('Object.keys(modelPricingDraft).length'),0);
});

test("Codex quota groups retain independent windows and unavailable values",()=>{
 const app=consoleFixture(()=>response({}));
 app.run('codexTestQuota={provider:"openai-codex",status:"stale",account_id:"workspace",account_identity:"user@example.test",plan:"plus",source:"codex_app_server",collected_at:"2026-10-11T20:00:00Z",unavailable_reason:"rate_limited",limits:[{limit_id:"codex",scope:"account",status:"available",windows:[{window_type:"weekly",used:62,remaining:38,status:"available",reset_at:"2026-10-15T22:00:00Z"}]},{limit_id:"extra",limit_name:"<script>",scope:"unknown",status:"unavailable",windows:[{window_type:"5h",status:"unavailable"}]}]}');
 const label=app.run('modelQuotaText({quota:codexTestQuota})');
 assert.match(label,/Stale.*Weekly: 62.0% used.*38.0% remaining.*5-hour: unavailable/);
 assert.match(label,/5-hour: usage unavailable/);assert.ok(!label.includes("0.0%"));
 const html=app.run('modelTrackingMarkup({quota:codexTestQuota})');
 assert.match(html,/user@example.test.*plus/);assert.match(html,/codex_app_server/);assert.match(html,/Last successful update/);assert.match(html,/quota-group/);assert.match(html,/Scope: unknown/);assert.ok(!html.includes('<script>'));
});

test("Codex quota refresh handles cooldowns and network failure without losing models",async()=>{
 let fail=false;
 const app=consoleFixture(async(url)=>{assert.equal(url,"/gateway/v1/quotas/refresh");if(fail)throw new Error("synthetic network failure");return response({},false,'"fixture"',429);});
 const button=app.context.document.querySelector("#refresh-quotas");
 await button.listeners.click();
 assert.match(app.context.document.querySelector("#quota-refresh-status").textContent,/cooling down/);assert.equal(button.disabled,false);
 fail=true;await button.listeners.click();assert.match(app.context.document.querySelector("#quota-refresh-status").textContent,/unavailable/);assert.equal(button.disabled,false);

});

test("server monitoring settings round trip with debug capture off by default",()=>{
 const app=consoleFixture(()=>response({}));
 app.run('applyConfig({server:{listen:"127.0.0.1:8080",timezone:"America/New_York"},activity:{max_bytes:8192,retention_days:2,prompt_logging:true,jev_decision_logging:true},providers:[],router:{classifier:{type:"disabled"}}})');
 let saved=app.run('readConfig()');
 assert.equal(saved.server.timezone,"America/New_York");
 assert.equal(saved.activity.max_bytes,8192);assert.equal(saved.activity.retention_days,2);
 assert.equal(saved.activity.prompt_logging,true);assert.equal(saved.activity.jev_decision_logging,true);
 app.run('applyConfig({server:{listen:"127.0.0.1:8080"},providers:[],router:{classifier:{type:"disabled"}}})');
 saved=app.run('readConfig()');
 assert.equal(saved.server.timezone,"");assert.equal(saved.activity.prompt_logging,false);assert.equal(saved.activity.jev_decision_logging,false);
 assert.equal(saved.activity.max_bytes,104857600);assert.equal(saved.activity.retention_days,7);
 app.run('form.elements.namedItem("activity.retention_days").value="0"');assert.throws(()=>app.run('readConfig()'),/Activity/);
 app.run('form.elements.namedItem("activity.retention_days").value="7";form.elements.namedItem("server.timezone").value="Mars/Olympus"');assert.throws(()=>app.run('readConfig()'),/IANA timezone/);
});
test("saved timezone applies across date boundaries, quotas and benchmark timestamps",()=>{
 const app=consoleFixture(()=>response({}));
 app.run('consoleTimezone="America/New_York"');
 assert.match(app.run('consoleDate("2026-10-09T01:30:00Z")'),/Oct 8, 2026/);
 assert.match(app.run('consoleTime("2026-10-09T01:30:00Z")'),/09:30:00/);
 assert.match(app.run('benchmarkDate("2026-10-09T01:30:00Z")'),/Oct 8, 2026 · 21:30 America\/New_York/);
 assert.match(app.run('benchmarkDate("2026-10-09")'),/Oct 9, 2026/);
 assert.match(app.run('quotaWindowText({name:"daily",remaining:10,unit:"tokens",reset_at:"2026-10-09T01:30:00Z"})'),/10\/8\/2026/);
 assert.equal(app.run('consoleDate("invalid")'),"—");
});
test("activity debug inspectors show raw bodies safely and clear content on dismissal",()=>{
 const app=consoleFixture(()=>response({}));
 app.context.debugRow={id:123,kind:"request_complete",prompt:'{\n "messages": [{"role":"user","content":"<script>private</script>"}]\n}',jev_request:'{"state":"<unsafe>","questions":{}}',jev_response:'{"answers":{"choice":"<unsafe>"}}',debug_truncated:true};
 app.run('activityRows=[debugRow];loadedConfiguration.activity={prompt_logging:true,jev_decision_logging:true};renderActivity()');
 const rows=app.run('document.querySelector("#activity-results").innerHTML');
 assert.match(rows,/data-inspect-activity="123" data-inspect-kind="prompt"/);assert.match(rows,/data-inspect-kind="jev"/);
 app.run('openActivityInspector(123,"prompt")');
 let html=app.run('document.querySelector("#activity-inspector-content").innerHTML');
 assert.match(html,/&lt;script&gt;private&lt;\/script&gt;/);assert.doesNotMatch(html,/<script>/);assert.ok(html.includes('{\n &quot;messages&quot;'));
 assert.equal(app.run('document.querySelector("#activity-inspector-dialog").open'),true);
 assert.match(app.run('document.querySelector("#activity-inspector-note").textContent'),/truncated/);
 app.run('openActivityInspector(123,"jev")');html=app.run('document.querySelector("#activity-inspector-content").innerHTML');
 assert.match(html,/Request sent to Jev/);assert.match(html,/Jev response/);assert.match(html,/&lt;unsafe&gt;/);assert.doesNotMatch(html,/<unsafe>/);
 app.run('document.querySelector("#close-activity-inspector").listeners.click()');
 assert.equal(app.run('document.querySelector("#activity-inspector-dialog").open'),false);assert.equal(app.run('document.querySelector("#activity-inspector-content").textContent'),"");
 app.run('loadedConfiguration.activity={};renderActivity()');assert.doesNotMatch(app.run('document.querySelector("#activity-results").innerHTML'),/data-inspect-activity/);
 app.run('loadedConfiguration.activity={jev_decision_logging:true};activityRows=[{id:124,kind:"request_complete"}];renderActivity()');assert.match(app.run('document.querySelector("#activity-results").innerHTML'),/data-inspect-kind="jev" disabled/);
});
test("activity cleanup refresh removes pruned rows and ignores stale session responses",async()=>{
 const app=consoleFixture(()=>response({events:[]}));
 app.run('consoleSession="active";activityRows=[{id:1,kind:"request_complete"}]');
 await app.run('refreshActivityHistory()');assert.equal(app.run('activityRows.length'),0);
 let finish;app.setFetch(()=>new Promise(resolve=>{finish=resolve;}));
 const refresh=app.run('refreshActivityHistory()');await new Promise(resolve=>setImmediate(resolve));app.run('consoleSession="different"');finish(response({events:[{id:2}]}));await refresh;
 assert.equal(app.run('activityRows.length'),0);
});

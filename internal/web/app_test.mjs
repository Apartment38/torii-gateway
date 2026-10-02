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
const response = (body, ok = true) => ({ ok, json: async () => body, text: async () => JSON.stringify(body) });
const models = count => ({ models: Array.from({ length: count }, (_, i) => ({ id: `model-${i}` })) });
const draft = (type, id = type) => ({ id, type, api_url: type === "openai-codex" ? "https://chatgpt.com/backend-api/codex" : `https://${type}.example/v1` });
function consoleFixture(handler) {
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
    location: { hash: "#providers" },
    localStorage: { getItem: () => null, setItem(...args) {storageWrites.push(args);} },
    sessionStorage: {setItem(...args){storageWrites.push(args);}},
    navigator: { platform: "Linux", userAgent: "linux" },
    matchMedia: () => ({ matches: false }),
    HTMLInputElement: Element,
    FormData: class { forEach() {} },
    URL, Error, TextEncoder, TextDecoder, AbortController, structuredClone, btoa, confirm:()=>true,
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
    app.run("applyConfig(config)");
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

test("benchmark source controls preserve weights and saved credentials and require the Artificial Analysis key", async () => {
  const app = consoleFixture(() => response({}));
  const intelligence = { enabled:true, sources:[{ id:"aa", type:"artificial_analysis", enabled:true, api_key:{ env:"AA_API_KEY" } }], routing:{ task_weights:{ coding:{ coding:3, agentic:1 } } } };
  app.context.config = { providers:[], intelligence };
  app.run("applyConfig(config)");
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
    assert.equal(options.headers.Authorization, "Basic test-admin-login");
    if (url.endsWith("/sources")) return response({aa:{last_success:"2026-10-01",enabled:true,observation_count:10}});
    if (url.endsWith("/models")) return response({});
    if (url.includes("/observations?")) return response({total:0,observations:[]});
    assert.equal(url, "/gateway/v1/intelligence/refresh");
    assert.equal(options.method, "POST");
    assert.equal(options.headers.Accept,"text/event-stream");
    return response({ updated:true, sources:{ aa:{ last_success:"2026-10-01", stale:false } } });
  });
  app.run('adminAuthorization="Basic test-admin-login"');
  app.context.config = { providers:[], intelligence:{enabled:true}, server:{ authentication:{ api_key:{ credential:"stored-gateway-key" } } } };
  app.run("applyConfig(config)");
  await app.run("inspectBenchmarks(true)");
  assert.match(app.run('document.querySelector("#benchmark-status").textContent'), /refresh finished/);
  assert.equal(app.run('document.querySelector("#refresh-benchmarks").disabled'), false);
});

test("console sign-in keeps Basic login only in memory and logout clears it",async()=>{
  const app=consoleFixture(url=>url==="/gateway/v1/config"?response({server:{authentication:{api_key:{credential:"gateway-ref"}}},providers:[]}):response({credentials:[]}));
  assert.equal(app.run('document.querySelector("#console-login").hidden'),false);
  assert.equal(app.run('document.querySelector("#console-logout").hidden'),true);
  app.run('document.querySelector("#login-username").value="admin";document.querySelector("#login-password").value="memory-login-sentinel"');
  await app.run('document.querySelector("#login-form").listeners.submit({preventDefault(){}})');
  assert.equal(app.run('adminAuthorization'),"Basic "+btoa("admin:memory-login-sentinel"));assert.equal(app.run('document.querySelector("#login-password").value'),"");assert.deepEqual(app.storageWrites,[]);
  assert.equal(app.run('document.querySelector("#console-login").hidden'),true);
  assert.equal(app.run('document.querySelector("#console-logout").hidden'),false);
  assert.equal(app.run('document.querySelector("#signed-in-user").textContent'),"admin");
  app.run('document.querySelector("#console-logout").listeners.click()');assert.equal(app.run('adminAuthorization'),"");
  assert.equal(app.run('document.querySelector("#console-login").hidden'),false);
  assert.equal(app.run('document.querySelector("#console-logout").hidden'),true);
  assert.equal(app.run('document.querySelector("#signed-in-user").textContent'),"Not signed in");
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
  app.run("applyConfig(config)");
  assert.equal(app.run('document.querySelector("#refresh-benchmarks").disabled'),true);
  await app.run("inspectBenchmarks(true)");
  assert.equal(app.calls.length,0);
  assert.match(app.run('document.querySelector("#benchmark-status").textContent'),/Enable benchmark intelligence/);
  app.run('document.querySelector("#intelligence-enabled").checked=true;document.querySelector("#intelligence-enabled").listeners.change()');
  assert.equal(app.run('document.querySelector("#refresh-benchmarks").disabled'),true);
  assert.match(app.run('document.querySelector("#benchmark-enable-notice").textContent'),/Save & reload/);
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
  app.run("applyConfig(config)");
  assert.equal(app.run("benchmarkSources.length"),5);
  assert.equal(app.run('document.querySelector("#benchmark-configuration").hidden'),false);
  const sourceHTML=app.run('document.querySelector("#benchmark-sources-list").innerHTML');
  for (const name of ["Artificial Analysis","BenchLM.ai","Hugging Face","LMArena","LiveBench"]) assert.ok(sourceHTML.includes(name));
  assert.equal(app.run("readConfig().intelligence.sources.length"),0,"unused placeholder sources must not fail configuration validation");
});

test("benchmark source keys are masked, only newly entered keys are revealable, and logout clears drafts", () => {
  const app=consoleFixture(()=>response({}));
  app.context.config={providers:[],intelligence:{enabled:true,sources:[{id:"aa",type:"artificial_analysis",enabled:true,api_key:{credential:"encrypted-aa"}}]}};
  app.run('applyConfig(config);benchmarkSources[0].api_key_input="source-secret-sentinel";renderBenchmarkSources()');
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
  app.run('applyConfig(config);benchmarkSources.forEach(source=>source.enabled=true);benchmarkSources[0].api_key_input="source-key";renderBenchmarkSources()');
  const config=JSON.parse(app.run("JSON.stringify(readConfig().intelligence)"));
  assert.deepEqual(config.identities,app.context.config.intelligence.identities);
  assert.deepEqual(config.aggregation,app.context.config.intelligence.aggregation);
  assert.equal(config.sources.length,5);
  const hf=config.sources.find(source=>source.type==="huggingface");
  assert.equal(hf.datasets[0].dataset,"open-llm-leaderboard/contents");
  assert.equal(hf.datasets[0].model_field,"fullname");
  assert.equal(hf.datasets[0].metrics["IFEval Raw"].max,1);
  const html=app.run('document.querySelector("#benchmark-sources-list").innerHTML');
  assert.equal([...html.matchAll(/data-benchmark-field="api_key_input"/g)].length,1);
  assert.doesNotMatch(html,/data-benchmark-field="(?:id|url|metric|dataset|max_rows|include_estimated)"|Metric mapping|Add dataset/);
  assert.doesNotMatch(readFileSync(new URL("./static/index.html",import.meta.url),"utf8"),/data-benchmark-global/);
  app.run('updateBenchmarkSource({target:{dataset:{benchmarkSource:"2",benchmarkField:"enabled"},type:"checkbox",checked:false}})');
  assert.equal(app.run('document.querySelector("#refresh-benchmarks").disabled'),true);
});

test("hidden custom source settings survive saves without being replaced by presets",()=>{
  const app=consoleFixture(()=>response({}));
  const source={id:"hf-custom",type:"huggingface",enabled:true,url:"https://custom.example/rows",max_rows:42,api_key:{credential:"hf-key"},datasets:[{dataset:"org/results",config:"subset",split:"test",model_field:"model_id",date_field:"date",metrics:{accuracy:{category:"knowledge",min:0,max:100}}}]};
  app.context.config={providers:[],intelligence:{enabled:true,sources:[source],source_timeout:"45m",aggregation:{source_weights:{"hf-custom":2}}}};
  app.run('applyConfig(config)');
  const saved=JSON.parse(app.run('JSON.stringify(readConfig().intelligence)'));
  assert.deepEqual(saved.sources[0],source);assert.equal(saved.source_timeout,"45m");assert.equal(saved.aggregation.source_weights["hf-custom"],2);
});

test("cached table shows scores, operational data, raw unmatched observations and safe HTML", async () => {
  const app=consoleFixture(url=>{
    if(url.endsWith("/sources"))return response({aa:{enabled:true,last_success:"2026-10-02",observation_count:2}});
    if(url.endsWith("/models"))return response({"provider/model":{provider_ids:["provider/model"],scores:{coding:.92,preference:.81},operations:{ttft:.2,context_length:128000},sources:["aa"],observation_count:2,latest_retrieval:"2026-10-02"}});
    assert.match(url,/observations\?offset=0&limit=25&include_metadata=true&q=/);
    return response({total:1,observations:[{external_model_id:"<unmatched>",source:"aa",benchmark:"code",category:"coding",original_score:85,normalized_score:.85,version:"v1",retrieved_at:"2026-10-02",date_basis:"retrieval_only",metadata:{confidence_interval:[.8,.9],detail:"<script>upstream</script>"}}]});
  });
  app.run('adminAuthorization="Basic admin";applyBenchmarkConfig({enabled:true})');
  await app.run('loadBenchmarkData()');
  let html=app.run('document.querySelector("#benchmark-table-body").innerHTML');
  assert.match(html,/92\.0/);assert.match(html,/81\.0/);assert.match(html,/128000/);assert.match(html,/—/);
  app.run('benchmarkTableView="observations"');await app.run('loadBenchmarkData()');
  html=app.run('document.querySelector("#benchmark-table-body").innerHTML');
  assert.match(html,/&lt;unmatched&gt;/);assert.match(html,/85\.0/);assert.match(html,/Unmatched \/ ambiguous/);
  assert.match(html,/retrieval_only/);assert.match(html,/confidence_interval/);assert.match(html,/&lt;script&gt;/);assert.doesNotMatch(html,/<script>/);
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
  app.run('adminAuthorization="Basic admin";applyBenchmarkConfig({enabled:true})');
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
    if(url.includes("/observations?"))return response({total:0,observations:[]});
    return new Promise(resolve=>pendingModels.push(()=>resolve(response({ready:{scores:{coding:.9}}}))));
  });
  app.run('adminAuthorization="Basic admin";applyBenchmarkConfig({enabled:true})');
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

test("imported unmatched data populates the table without silently merging model identities",async()=>{
  const app=consoleFixture(url=>url.endsWith("/sources")?response({benchlm:{enabled:true,observation_count:1}}):url.endsWith("/models")?response({}):response({total:1,observations:[{external_model_id:"external-model",source:"benchlm",benchmark:"coding",category:"coding",original_score:80,normalized_score:.8,metadata:{}}]}));
  app.run('adminAuthorization="Basic admin";applyBenchmarkConfig({enabled:true})');
  await app.run('loadBenchmarkData()');
  assert.equal(app.run('document.querySelector("#benchmark-table-view").value'),"observations");
  assert.match(app.run('document.querySelector("#benchmark-table-body").innerHTML'),/external-model/);
  assert.match(app.run('document.querySelector("#benchmark-table-body").innerHTML'),/Unmatched \/ ambiguous/);
  assert.equal(app.run('Object.keys(benchmarkModels).length'),0);
  app.run('document.querySelector("#benchmark-table-view").value="models";document.querySelector("#benchmark-table-view").listeners.change()');
  await new Promise(resolve=>setImmediate(resolve));
  assert.equal(app.run('benchmarkTableView'),"models","an explicitly selected view must remain selected");
});

test("refresh completion waits for both the table response and the browser paint",async()=>{
  let finishModels;
  const app=consoleFixture(url=>url.endsWith("/sources")?response({}):url.includes("/observations?")?response({total:0,observations:[]}):new Promise(resolve=>{finishModels=()=>resolve(response({ready:{scores:{coding:.9}}}));}));
  app.run('adminAuthorization="Basic admin";applyBenchmarkConfig({enabled:true});benchmarkRefreshRunning=true;benchmarkProgressCounts={total:2,completed:2,failed:0}');
  const frames=[];app.context.window.requestAnimationFrame=callback=>frames.push(callback);
  const finished=app.run('finishBenchmarkRefresh({partial_failure:false},adminAuthorization,new AbortController())');
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
  app.run('adminAuthorization="Basic admin";applyBenchmarkConfig({enabled:true})');
  await app.run('inspectBenchmarks(true)');
  assert.match(app.run('document.querySelector("#benchmark-status").textContent'),/table could not finish updating/);
  assert.doesNotMatch(app.run('document.querySelector("#benchmark-refresh-log").textContent'),/Benchmark refresh finished/);
});

test("page size choices paginate models and request raw observations in 25, 50 or 100 row chunks",async()=>{
  const app=consoleFixture(url=>{
    if(url.endsWith("/sources"))return response({});
    if(url.endsWith("/models"))return response({});
    const parsed=new URL(url,"http://gateway");const size=Number(parsed.searchParams.get("limit")),offset=Number(parsed.searchParams.get("offset"));
    return response({total:120,observations:Array.from({length:Math.min(size,120-offset)},(_,i)=>({model_id:`raw-${offset+i}`,external_model_id:`raw-${offset+i}`,metadata:{row:offset+i}}))});
  });
  app.context.rows=Object.fromEntries(Array.from({length:120},(_,i)=>[`model-${String(i).padStart(3,"0")}`,{scores:{coding:.9}}]));
  app.run('adminAuthorization="Basic admin";applyBenchmarkConfig({enabled:true});benchmarkModels=rows;renderBenchmarkTable()');
  assert.equal([...app.run('document.querySelector("#benchmark-table-body").innerHTML').matchAll(/<tr>/g)].length,25);
  assert.equal(app.run('document.querySelector("#benchmark-page-label").textContent'),"Page 1 of 5");
  app.run('document.querySelector("#benchmark-next").listeners.click()');
  assert.match(app.run('document.querySelector("#benchmark-table-body").innerHTML'),/model-025/);
  for(const size of [50,100]){
    app.run(`document.querySelector("#benchmark-page-size").value="${size}";document.querySelector("#benchmark-page-size").listeners.change()`);
    assert.equal([...app.run('document.querySelector("#benchmark-table-body").innerHTML').matchAll(/<tr>/g)].length,size);
  }
  app.run('benchmarkTableView="observations"');
  for(const size of [25,50,100]){
    app.run(`document.querySelector("#benchmark-page-size").value="${size}";document.querySelector("#benchmark-page-size").listeners.change()`);
    await new Promise(resolve=>setImmediate(resolve));
    assert.ok(app.calls.at(-1).url.includes(`limit=${size}`));
    assert.equal([...app.run('document.querySelector("#benchmark-table-body").innerHTML').matchAll(/<tr>/g)].length,size);
  }
});

test("interrupted refresh reports incomplete progress and ignores late data after logout", async () => {
  let controller;
  const app=consoleFixture(()=>({ok:true,headers:{get:()=>"text/event-stream"},body:new ReadableStream({start(c){controller=c;}})}));
  app.run('adminAuthorization="Basic admin";applyBenchmarkConfig({enabled:true})');
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
    assert.equal(options.headers.Authorization,"Basic admin-login");
    if (options.method === "POST") {
      assert.deepEqual(JSON.parse(options.body),{name:key.name});
      return response({key,api_key:"generated-client-key",warning:"This key is displayed only once"});
    }
    return response({api_keys:[key]});
  });
  app.run('adminAuthorization="Basic admin-login";document.querySelector("#create-credential").listeners.click()');
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
  app.run('adminAuthorization="Basic admin-login"');
  await app.run("loadCredentialMetadata()");
  const html = app.run('document.querySelector("#credential-list").innerHTML');
  assert.equal([...html.matchAll(/class="credential-card"/g)].length,2);
  assert.match(html, /<div class="credential-card-content"><h3 class="credential-card-name">Hermes &lt;client&gt; &quot;work&quot;<\/h3><p class="credential-card-meta">Created 2026-10-02T12:34:56\.123456789Z<\/p><\/div><button/);
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
    assert.equal(options.headers.Authorization,"Basic admin-login");
    if (options.method === "DELETE") {
      assert.equal(url,"/gateway/v1/admin/api-keys/gateway%2Fclient-key%2Ftest");
      deleted = true;
      return response({status:"deleted"});
    }
    assert.equal(url,"/gateway/v1/admin/api-keys");
    return response({api_keys:deleted?[]:[{id:"gateway/client-key/test",name:"client",created_at:"today"}]});
  });
  app.run('adminAuthorization="Basic admin-login"');
  await app.run("loadCredentialMetadata()");
  await app.run('document.querySelector("#credential-list").buttons[0].listeners.click()');
  assert.equal(deleted,true);
  assert.match(app.run('document.querySelector("#credential-list").innerHTML'),/No gateway API keys/);
});

test("failed login and expired authentication reset identity and clear one-time keys", async () => {
  const app=consoleFixture(()=>({...response({},false),status:401}));
  app.run('adminAuthorization="Basic expired";signedInUsername="admin";updateConsoleIdentity();showNewAPIKey("sensitive-once-key")');
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
  app.run('adminAuthorization="Basic admin-login";signedInUsername="admin";document.querySelector("#client-key-name").value="client"');
  const creating=app.run('document.querySelector("#create-key-form").listeners.submit({preventDefault(){}})');
  await Promise.resolve();
  app.run('document.querySelector("#console-logout").listeners.click()');
  finish(response({api_key:"must-not-be-shown-after-logout"}));
  await creating;
  assert.equal(app.run('document.querySelector("#new-api-key").value'),"");
  assert.equal(app.run('document.querySelector("#signed-in-user").textContent'),"Not signed in");
});
test("API key generation requires in-memory administrator login and clears its once-only display",async()=>{
  const app=consoleFixture((url,options)=>{assert.equal(url,"/gateway/v1/admin/api-key");assert.equal(options.method,"POST");assert.equal(options.headers.Authorization,"Basic admin-login");return response({api_key:"rotated-once-key"});});
  app.run('adminAuthorization="Basic admin-login"');await app.run('document.querySelector("#generate-api-key").listeners.click()');assert.equal(app.run('document.querySelector("#new-api-key").value'),"rotated-once-key");
  app.run('document.querySelector("#new-key-dialog").listeners.cancel()');assert.equal(app.run('document.querySelector("#new-api-key").value'),"");assert.deepEqual(app.storageWrites,[]);
});

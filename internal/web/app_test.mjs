import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import vm from "node:vm";

// Exercise the shipped browser script with a minimal DOM and mocked gateway.
// No browser framework, dependency download, or real OAuth request is needed.
const script = readFileSync(new URL("./static/app.js", import.meta.url), "utf8");
class Element {
  value = "";
  type = "text";
  textContent = "";
  className = "";
  disabled = false;
  dataset = {};
  classList = { add() {}, remove() {}, toggle() {} };
  addEventListener() {}
  setAttribute() {}
  querySelector() { return null; }
  querySelectorAll() { return []; }
}
class ProviderList extends Element {
  results = new Map();
  buttons = new Map();
  set innerHTML(html) {
    this.html = html;
    this.results = new Map();
    this.buttons = new Map();
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
    const match = /\[data-(result|test|login|remove)="(\d+)"\]/.exec(selector);
    if (!match) return null;
    return match[1] === "result" ? this.results.get(match[2]) : this.buttons.get(`${match[1]}:${match[2]}`);
  }
  querySelectorAll(selector) {
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
  const context = vm.createContext({
    document: { querySelector: element, querySelectorAll: () => [], documentElement: new Element(), addEventListener() {} },
    window: { addEventListener() {}, setTimeout: callback => queueMicrotask(callback), open() {} },
    location: { hash: "#providers" },
    localStorage: { getItem: () => null, setItem() {} },
    navigator: { platform: "Linux", userAgent: "linux" },
    matchMedia: () => ({ matches: false }),
    HTMLInputElement: Element,
    FormData: class { forEach() {} },
    URL, Error,
    fetch: async (url, options) => {
      // Keep the script's startup load isolated; tests explicitly load config.
      if (url === "/gateway/v1/config" && startupLoad) {
        startupLoad = false;
        return new Promise(() => {});
      }
      calls.push({ url, options });
      return fetchHandler(url, options);
    },
  });
  vm.runInContext(script, context);
  return {
    list, calls, context,
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
  assert.equal(app.run("providers[0].api_key_env"), "JEV_API_KEY");
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

# Connect model providers

Connect a service that runs models, test its discovered catalog, then save it.
You need administrator access and either a reachable local service, a hosted API
key, or the subscription account required by the selected profile.

## Add an inference provider

1. Open **Providers → Add provider** and choose its type from the list.
2. Give it a unique lowercase ID, such as `local` or `openrouter`.
3. Set its API URL, using the profile defaults in [Provider support](../reference/providers.md).
4. For API-key services, enter the upstream key in the provider's key field.
   For Codex subscriptions, use **Log in** after configuring the provider.
5. Test the connection, inspect the returned model list, and choose **Save provider**.
6. List `/v1/models` or check the console, then use a discovered `provider/model`
   in your client. Connection tests exercise discovery, not billable inference.

Use the key issued by the upstream in provider settings. Use a Torii-generated
key in your client. These keys have different jobs and are not interchangeable.
Model names can contain additional slashes: `openrouter/google/MODEL` belongs to
provider `openrouter` with upstream model `google/MODEL`.

If configuration changed in another tab or through the API, **Save provider**
rejects the outdated draft rather than restoring old provider settings. Copy
your edits, choose **Refresh config**, then reconcile them with the current
settings before saving. The same applies after a gateway restart. API clients
must use the [configuration revision header](../reference/api.md#health-setup-configuration-and-providers).

Connection tests query the live catalog even when **Discover models** is disabled.
The status then warns that **Save provider** uses the static model list. Enable
discovery to expose the returned models. For an Ollama server, choose **Ollama**
and its server URL without `/v1`; Torii also queries native capability metadata
so `smart` can select models that support the request. A leftover `setup` entry
is a placeholder, not an upstream model.

## Multiple providers and advanced settings

The following are configuration excerpts: submit them through the revision-protected configuration API; see [Configuration](../reference/configuration.md).
Credential ID placeholders refer to secrets already stored by Torii. Prefer the
console to create them. Do not put literal keys in persisted settings.

`providers` may be empty during setup. Each configured provider accepts the common fields below; profile-specific restrictions are listed in the [provider reference](../reference/providers.md).

| Field | Description |
| --- | --- |
| `id` | Unique lowercase namespace matching `[a-z0-9][a-z0-9._-]*`. This becomes the prefix in public model IDs. |
| `type` | See the [complete provider type list](../reference/providers.md). The `openai` profile is locked to `https://api.openai.com` and uses an API key, while `openai_compatible` is for other standard-compatible origins. |
| `base_url` | Ollama's HTTP or HTTPS base URL. Credentials, query strings, and fragments are rejected. A path prefix is allowed. |

For multiple Ollama servers, add more entries with unique IDs:

```yaml
providers:
  - id: local
    type: ollama
    base_url: http://127.0.0.1:11434
  - id: lab
    type: ollama
    base_url: http://ollama.internal:11434
```

If both servers expose `llama3.2`, clients see `local/llama3.2` and
`lab/llama3.2`. A request for either name goes only to the named server. Explicit model requests are never substituted with a different provider/model.

Ollama inference has a **5-minute request timeout** by default to allow cold model
loading. Change **Request timeout** on its provider card, or set `request_timeout`
to a positive Go duration such as `10m` in its settings entry, then save through the configuration API.
The deadline includes loading, generation, and reading the complete streamed or
JSON response. Leave the field blank to restore the default. Discovery remains
limited to 10 seconds; client cancellation still stops inference immediately.
Set client and proxy deadlines high enough to allow the chosen provider timeout.

Generic `openai_compatible` providers additionally support `bearer_token`,
secret-reference-valued `headers`, `tls.ca_file`, `tls.server_name`, `models`,
`discover_models`, `model_rewrites`, and separate `connect_timeout`,
`request_timeout`, and `discovery_timeout` duration values. For example:

```yaml
providers:
  - id: private
    type: openai_compatible
    base_url: https://inference.example.com
    bearer_token:
      credential: YOUR_STORED_PRIVATE_PROVIDER_TOKEN_ID
    discover_models: false
    models: [production-model]
    model_rewrites:
      assistant: production-model
    connect_timeout: 5s
    request_timeout: 10m
```

This exposes `private/assistant` while sending `production-model` upstream.
Provider credentials are isolated in a dedicated HTTP client and are not
forwarded across redirects. The authenticated console accepts a new, masked
API key and encrypts it in SQLite; settings receive only a credential reference.
**Show** reveals only a newly entered value, never an existing stored key.
Persisted credentials must use database references.
Use HTTPS for remote console access. See [credential management](credentials.md)
for setup, backup and recovery instructions.

The normal OpenAI API can be enabled with its origin-locked profile:

```yaml
  - id: openai
    type: openai
    base_url: https://api.openai.com
    bearer_token:
      credential: YOUR_STORED_OPENAI_API_KEY_ID
    project: proj_example       # optional
    organization: org_example   # optional
```

The profile exposes models as `openai/<model>`, sends only the configured API
key and optional OpenAI project/organization headers, and does not use the
gateway's persistent OAuth credential store.

The `openai-codex` provider automatically discovers the visible models for
the logged-in subscription. A `models` list is optional and is only a fallback,
not an allowlist. Import credentials with `gateway auth login openai-codex`
or **Log in** in the web console. Console login opens OpenAI in another browser
tab and displays a one-time code; no downloaded helper is required. Credentials
remain endpoint-bound: existing `chatgpt.tokens.use.direct` tokens use the documented
[ChatGPT plan-usage API](https://developers.openai.com/siwc/token-sharing-open-source/models-and-inference)
at `https://api.openai.com/v1`; legacy Codex OAuth tokens use the Codex backend.
This does not use an OpenAI API key or the API-key provider's credentials.
Discovery refreshes near-expiry tokens before querying the catalog and does
not exclude subscription models based on the API-key `supported_in_api` flag.
If discovery fails, the gateway logs a warning and retains the last successful
catalog, or uses configured fallback models. The served catalog refreshes on
requests after one minute and immediately after a successful web login.
Connection tests report the actual
discovery error instead of claiming the fallback is a successful connection.
Use `gateway models --database ./state/torii.db` to inspect the catalog.

## Codex login in a browser

This works with a local gateway or one hosted on another computer/container.
You need an OpenAI account with Codex subscription access and device code
authorization enabled in ChatGPT's security settings. Managed workspaces may
require an administrator to enable it.

1. On **Providers**, add or edit **OpenAI Codex**, then click **Log in**.
2. Torii opens a new browser tab. Sign in with OpenAI and enter the one-time code
   shown on the provider card. If the tab is blocked or closed, click
   **Open OpenAI login** on the card to open it yourself.
3. Return to Torii. It completes authorization on the gateway, stores the
   credential encrypted in SQLite, and checks the subscription's model catalog.
   A successful check shows **Logged in · Working** and the discovered model count.

No companion app, terminal command, or local callback listener is needed.
The code expires after 15 minutes. If login expires or fails, click **Log in**
again. OpenAI access and refresh tokens remain on the gateway; the console
receives only the authorization URL, one-time code, and status. Use HTTPS for
remote console access in production.

The optional legacy helper protocol remains available to API clients that
explicitly request it; see [API](../reference/api.md) and
[CLI](../reference/cli.md). It is not part of console login.

On SELinux-enabled hosts, the `:Z` volume labels in the compose file allow the
container to read the configuration and write its database volume. If an upstream
provider runs on the host, use a host-reachable address such as
`host.containers.internal` in `config.yaml` rather than `127.0.0.1`.

## Classifier providers

Add **TypeSafe AI · Jev** (`type: jev`) on the Providers page for the hosted
TypeSafe service. Its default API URL is `https://api.typesafe.ai/v1`; paste your
key into **API key**, or use a stored credential reference under Advanced.
Add **Jev compatible · Laya / custom**
(`type: jev_compatible`) for a self-hosted decision server; supply your own
provider ID and API URL, with an optional bearer key. These providers use the
[typed System One protocol](https://docs.typesafe.ai/api), not Chat Completions.
They are excluded from the gateway's public inference model list and from
smart-routing inference candidates.

Connection checks discover models from `/v1/models` (or the configured API
path's `/models`). For current Laya servers that return 404/405 there, Torii
reads the advertised loaded checkpoints from `/health`; see
[Laya's server implementation](https://github.com/LeonaDavinci/laya-system-one/blob/main/laya/serve.py).
Discovery does not invent model names, treat authentication errors as success,
or send billable classification requests while loading the console.
Connection status reflects discovery, not a classification/inference test.

Smart Routing fills its classifier-model selector from successful provider
checks, retains the saved choice, and selects the first discovered classifier
when none was configured. URLs, keys, and transport are no longer configured
on that page. If no models are available, the console explains how to add a
classifier while keeping **Smart routing** available for deterministic policy
settings. Deterministic routing still works without a classifier. The selected classifier and completed field edits save automatically; wait for **Saved**. Legacy direct Jev configuration
is moved into a provider draft, preserving its key reference; save to persist
the migration.

```yaml
providers:
  - id: jev
    type: jev
    base_url: https://api.typesafe.ai
    api_path: /v1
    bearer_token:
      credential: YOUR_STORED_JEV_API_KEY_ID
  - id: laya
    type: jev_compatible
    base_url: http://host.containers.internal:8000
    api_path: /v1
router:
  classifier:
    type: jev
    transport: provider
    provider: laya
    model: english
    on_failure: capability_fallback
```

This is a classifier-provider excerpt; configure at least one inference
provider as well for `smart` to return chat responses. Laya's example model
must be loaded and advertised by your server. For local, non-container builds,
use the reachable server hostname (often `localhost`) instead.

Classifier setup needs only a provider, a discovered model, and an automatic save.
The optional **Classifier tuning** panel adjusts the 10-second timeout,
minimum confidence, question instructions and option rubrics, and per-question
yes thresholds. See [routing customization](smart-routing.md) for
defaults, limits, and YAML examples. Edits retain the supported routing labels.

Optional catalog metadata overrides can be supplied with
`router.catalog_file: ./catalog-overrides.json`. The file is a strict JSON
array of catalog descriptors and is applied after bundled and live provider
metadata. OpenRouter quota credits require a separate `management_token`; the
inference bearer token is never reused for management requests.

## Subscription token administration

Provider credentials that require persistent state can be imported without
placing token values in arguments or YAML:

```sh
export PROVIDER_ACCESS_TOKEN="..."
./bin/gateway auth login PROVIDER_ID --database ./state/torii.db \
  --access-token-env PROVIDER_ACCESS_TOKEN
./bin/gateway auth status PROVIDER_ID --database ./state/torii.db
./bin/gateway auth logout PROVIDER_ID --database ./state/torii.db
```

Replace `PROVIDER_ID` with a configured provider ID, and set the environment
variable to its actual token before running.

`login` also accepts `--access-token-file`, optional refresh-token environment
or file flags, an RFC3339 `--expires-at`, and account/principal identifiers.
Command output reports state only and never prints stored token values.

Local `auth` commands require the database master key and administrator password.
Use the [CLI reference](../reference/cli.md) for all flags. Nous uses an API key
entered in its provider card; it does not use `auth login`. Normal API-key
credentials are separate from Codex subscription credentials. Provider-specific protocol limitations are
listed in [Provider support](../reference/providers.md).

For a local Ollama service, enter its port or click **Detect Ollama**. Detection checks localhost and container host addresses, using existing Ollama ports, `OLLAMA_HOST`, and 11434. Enter a remote API URL under advanced options for a different machine.

Nous API-key discovery supplies model prices from its live catalog. Account balances and provider quotas remain unknown when the service exposes no verified API-key telemetry endpoint.

## Inspect Codex subscription quotas

On **Models**, inspect a Codex model's metadata to see the account, plan, source,
last successful update and expandable quota pools. Five-hour and weekly usage
and remaining values are percentages of the reported allowance; other durations
stay separate. Missing windows display unavailable. The Usage page also exposes
these account snapshots alongside provider balances.

Use **Refresh quotas** for a manual read. Background polling runs every minute by
default; see [quota configuration](../reference/configuration.md#router).
Rate limits and failures may delay the next attempt. Stale values retain their
original successful timestamp, and a past reset waits for fresh provider data.
For an OAuth error, reconnect the provider and verify the selected workspace.
Successful inference alone does not prove that the token can read account usage.
Optional CLI collection is described in the
[profile restrictions](../reference/configuration.md#profile-restrictions).

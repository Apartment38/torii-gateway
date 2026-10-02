<p align="center">
  <img src="assets/torii-gateway-logo.png" alt="Torii Gateway logo" width="360" />
</p>

# Torii Gateway

Torii Gateway is a lightweight, resource-efficient LLM gateway written in Go.

The root URL serves an embedded, dependency-light configuration console. It
supports light and dark themes, provider and routing configuration, automatic
loading of the active configuration, and live save/reload. The console does not require authentication;
the API uses the configured bearer key when one is present. Authentication is
kept behind a replaceable interface so Basic Auth or Authentik/OIDC can be
added later without changing the console.
It presents an OpenAI-compatible HTTP API in front of one or more Ollama servers or standard OpenAI-compatible
endpoints and gives every upstream model a provider-qualified name such as
`local/llama3.2`. The current build implements the repository's Version 1
scope. It needs no database, Redis instance, queue, or sidecar.

The gateway is designed for clients that already support the OpenAI API. It
rewrites only the requested model name and otherwise
passes the JSON request through to the selected upstream, preserving unknown fields. Both
ordinary JSON responses and streamed SSE responses are relayed where supported,
and explicit model requests are always sent to the provider named in the model
ID.

## Currently implemented features

- OpenAI-compatible model, Chat Completions, Completions, Embeddings, and
  stateless Responses endpoints
- multiple Ollama upstreams with `provider/model` public model IDs
- configurable OpenAI-compatible upstreams with static or discovered models,
  provider-scoped credentials, TLS, timeouts, custom headers, and model aliases
- transparent request forwarding, including fields the gateway does not know
- low-overhead, byte-for-byte SSE streaming
- optional bearer authentication for API endpoints; the embedded console is public
- secrets loaded from environment variables, files, or masked console-managed values
- optional background benchmark ingestion and task-aware model intelligence
- strict configuration validation and graceful shutdown
- request IDs, typed OpenAI-style errors, and secret-safe structured logging

The Version 1 build also includes a `smart` virtual model with classifier-backed
deterministic selection, quota/cost-aware routing, in-memory session affinity,
pre-stream rerouting, endpoint-bound Codex OAuth transport, aggregate usage
state, and a non-root production container definition. Live provider smoke
tests and registry-backed image builds are release-environment checks.
Dedicated OpenRouter, Nous, Gemini, xAI, and Anthropic integrations are
deferred to Version 2. [`TASKS.md`](TASKS.md) is the ordered implementation
roadmap, [`docs/implementation-plan.md`](docs/implementation-plan.md) defines
the complete Version 1 contract, and [`docs/architecture.md`](docs/architecture.md)
describes implemented versus planned behavior.

## Requirements

- Go 1.25.1 or newer (to build from source)
- one or more reachable OpenAI-compatible upstreams
- for Ollama, its OpenAI-compatible API and an installed model (for example,
  `ollama pull llama3.2`)

## Set up from source

Clone the repository and build the gateway:

```sh
git clone https://github.com/Apartment38/torii-gateway.git
cd torii-gateway
mkdir -p bin
go build -o ./bin/gateway ./cmd/gateway
```

Copy the example configuration, generate and securely back up a database master
key, and start the server:

```sh
cp config.example.yaml config.yaml
export TORII_DB_KEY="$(openssl rand -hex 32)" # Back this up separately; do not regenerate on restart.
export TORII_DB_PATH="./state/torii.db"
./bin/gateway serve --config ./config.yaml
```

The `--config` flag defaults to `/config/config.yaml`. On first launch, an
interactive terminal prompts for your chosen administrator username and password
(at least 12 characters). It generates an inference API key and displays it only
once: save it securely. Without an interactive terminal, use the web console
and the temporary setup code printed in the container logs. Existing installs
also require administrator setup on their next restart; imported API keys are
preserved, not displayed again. No API-key environment variable is required.
For local builds you can set `state.database_path: ./state/torii.db` in YAML
instead of the optional `TORII_DB_PATH` override above.

## Run with Podman Compose

The repository includes a Podman-compatible [`compose.yaml`](compose.yaml).
The container creates a minimal writable configuration automatically on first
start, so no initial configuration file is required. To customize it before
starting, create `config/config.yaml`, change `server.listen` to
`0.0.0.0:8080`, and create a private project-root `.env` file:

```sh
# Optional: create config/config.yaml from the full example before starting.
mkdir -p config
cp config.example.yaml config/config.yaml
cat > .env <<'EOF'
TORII_DB_KEY=replace-with-64-hex-characters-from-openssl-rand-hex-32
TORII_PORT=8080
EOF
# If you created the file, edit config/config.yaml and set provider URLs
# reachable from the container.
podman compose up --build -d
```

The console is available at <http://127.0.0.1:8080/> and the API at
<http://127.0.0.1:8080/v1>. The compose service publishes host port `8080` by
default, stores persistent gateway state in the named
`torii_gateway_state` volume, and runs as the image's non-root user. Set
`TORII_PORT` in `.env` to publish a different host port or `TORII_IMAGE` to use
a prebuilt image. Only the master key and published port are needed in `.env`.
If an old `.env` already contains `TORII_API_KEY`, Compose passes it through for
one-time import; remove it after migration. SQLite credentials
persist in the additional `torii_gateway_database` volume at `/data`. Keep the
same master key on every restart:

```sh
# Inspect the temporary setup code, then create your admin in the web console.
podman compose logs -f torii-gateway
podman compose down
```

Keep `.env` private; it is ignored by Git and the image build. Prefer a mounted
master-key secret using `compose.secrets.yaml`; see [Encrypted credential storage](docs/credential-storage.md)
for Docker/Podman commands, non-root permissions, bootstrap and migration.
The master key is required; an API key is not required at launch.
Sign in to the console with your administrator username/password. Provider keys entered there
are encrypted in SQLite; saved values are never returned to the browser.
Legacy env/file inputs are imported once and no longer track external changes.
Use **Server → Generate new API key** or `gateway admin api-key --config PATH`
to generate a replacement. It is shown once and immediately invalidates the old key.
Docker users must apply `compose.docker.yaml` because Docker does not support
Podman's `:U` option; the linked guide explains directory ownership.

The compose file mounts only the host `config/` directory at `/config` as a
writable Podman bind mount with `rw,Z,U`; the container reads
`/config/config.yaml`. The directory mount is required because the web console
creates a temporary file beside the configuration before atomically replacing
it. The `U` option adjusts ownership only within `config/` for the container’s
non-root user, so it does not change ownership across the repository. The
named state volume is mounted with `Z,U` as well, so the non-root gateway user
can create its runtime directories there.

If an existing deployment still has a root-level `config.yaml`, migrate it
once before restarting Compose:

```sh
mkdir -p config
cp config.yaml config/config.yaml
podman compose down
podman compose up --build -d
```

The console includes Overview, Server, Providers, Routing, Benchmarks, Credentials,
and State & Quota pages. **Save & reload** sends the validated configuration to
`PUT /gateway/v1/config`; the gateway atomically replaces the host YAML file
and swaps in the rebuilt provider/routing handler. A failed validation or
provider initialization leaves the current configuration active. The console
reloads the active configuration after a successful save.

### Log in to Codex from a remote container

When the gateway runs on another computer, the Providers page cannot complete
OpenAI's loopback OAuth callback directly in the container. Click **Log in**
for the `openai-codex` provider and run the displayed one-shot helper on the
computer where your browser is running. The container serves a matching
`torii-auth` binary for the detected operating system; download it from the
console, then run the command shown there:

```sh
torii-auth --session-url https://gateway.example.com/gateway/v1/providers/login/session \
  --pairing-token '<one-time-token-from-the-console>'
```

The helper opens OpenAI login locally, receives the callback on `127.0.0.1`,
transfers the protected credential to the pending gateway session, and exits.
The pairing token expires after 10 minutes and can be used only once. The
gateway stores and refreshes the credential; the helper does not need to keep
running. Use HTTPS for the console in production because the helper transfers
OAuth credentials to the gateway over the session connection.

For images built before the helper endpoint was added, rebuild the container:

```sh
podman compose build --no-cache
podman compose up -d
```

For local development, build the helper with:

```sh
go build -o ./bin/torii-auth ./cmd/torii-auth
```

On SELinux-enabled hosts, the `:Z` volume labels in the compose file allow the
container to read the configuration and write its state volume. If an upstream
provider runs on the host, use a host-reachable address such as
`host.containers.internal` in `config.yaml` rather than `127.0.0.1`.

### Connect to host Ollama from the container

Do not use `http://127.0.0.1:11434` for an Ollama provider in the container;
that address points back to the Torii container. Use Podman's host gateway name
instead:

```yaml
providers:
  - id: local
    type: ollama
    base_url: http://host.containers.internal:11434
```

The included compose file adds `host.containers.internal` for compatibility.
Ollama must also listen on a host-reachable interface because its usual
loopback-only bind cannot be reached from a container. For a systemd-managed
Ollama installation, set `OLLAMA_HOST=0.0.0.0:11434` in the service environment
and restart Ollama; restrict the port with the host firewall if needed. Verify
the host service before starting Torii:

```sh
curl http://127.0.0.1:11434/api/tags
podman compose up --build -d
```

On Docker Desktop, Podman machine, macOS, and Windows, the same
`host.containers.internal` URL is the intended host gateway address. If the
runtime does not provide that name, use its documented host-gateway alias or
run the gateway with host networking where supported.

## Benchmark intelligence

Torii can import Artificial Analysis, BenchLM, configurable Hugging Face
leaderboards, Arena preference ratings, and published LiveBench judgments.
Ingestion is opt-in, uses a persistent cache, and never runs in inference
requests. Source failures retain their previous data independently.

Configure the top-level `intelligence` section or use the console's **Benchmarks**
page. Source enablement, aliases, freshness thresholds, aggregation weights and
smart-routing weights are documented in [Benchmark intelligence](docs/benchmark-intelligence.md),
with defaults in [config.example.yaml](config.example.yaml). BenchLM's export
requires a commercial licence for commercial use; check all source data terms.

Manual refresh targets the running gateway:

```sh
./gateway benchmarks refresh --config ./config.yaml
podman compose exec torii-gateway /gateway benchmarks refresh --config /config/config.yaml
```

Inspect `/gateway/v1/intelligence/sources`, `/models`, and `/unmatched` using the
gateway API key. Capability scores never grant unverified endpoint/tool/vision
support, and existing routing remains unchanged without usable data or enabled
routing weights.

## Update an existing installation

### Local binary

Pull the new source, validate the existing configuration, build a replacement
binary, then restart the process or service that runs the gateway. The config
file and state directory are not changed by the build:

```sh
git pull --ff-only
./bin/gateway config validate --config ./config.yaml
go build -trimpath -o ./bin/gateway.new ./cmd/gateway
mv ./bin/gateway.new ./bin/gateway
```

Restart the gateway using the same command or service manager used for the
current process, for example:

```sh
./bin/gateway serve --config ./config.yaml
```

If the gateway is managed by systemd, use `systemctl restart
torii-gateway.service` after installing the new binary. Keep a copy of the
previous binary if you need to roll back; do not replace `config.yaml` or the
state directory during an application update.

### Podman Compose container

The Compose workflow preserves the bind-mounted `config/` directory and named
`torii_gateway_state` volume while rebuilding the image:

```sh
git pull --ff-only
podman compose up --build -d
podman compose ps
podman compose logs --tail=100 torii-gateway
```

For a clean image rebuild, use `podman compose build --no-cache` before
`podman compose up -d`. To deploy a prebuilt image, set `TORII_IMAGE` in `.env`
and start without rebuilding:

```sh
TORII_IMAGE=torii-gateway:release podman compose up --no-build -d
```

The gateway validates and reloads configuration saved through the web console;
rebuilding the application image is only required when updating gateway code.
To roll back, point `TORII_IMAGE` at the previous image tag or check out the
previous source revision and rebuild it. Do not remove the
`torii_gateway_state` volume unless you intentionally want to discard
persistent gateway state.

## Configuration

Torii Gateway reads exactly one YAML document. Unknown fields are rejected to
catch spelling mistakes and configuration intended for unsupported features.
The included [`config.example.yaml`](config.example.yaml) is a complete
starting point:

```yaml
server:
  listen: 127.0.0.1:8080

state:
  directory: ./state

providers:
  - id: local
    type: ollama
    base_url: http://127.0.0.1:11434
```

### Server

| Field | Required | Description |
| --- | --- | --- |
| `server.listen` | yes | TCP `host:port` on which the gateway listens. Use `127.0.0.1:8080` for local-only access or `0.0.0.0:8080` in a container. |
| `server.authentication.api_key` | migration only | Legacy `value`, `env`, `file`, or database-reference input is imported once, then this section is removed. Gateway authentication is managed entirely in SQLite. |

Administrator login uses HTTP Basic authentication; inference uses the generated
or imported bearer API key. Anonymous inference is no longer enabled by omitting
`server.authentication`. Before administrator setup, new installations deny
inference because no key exists. Existing imported keys continue working.
The master encryption key is separate and must never appear in YAML.
See [credential management and migration](docs/credential-storage.md).

### State

`state.directory` is required and reserves the location for gateway state.
Use a writable, persistent path such as `/state` for deployments. Version 1
uses it for aggregate usage (`usage.json`); session affinity is in memory.
Credentials and benchmark observations use SQLite at `state.database_path`,
default `/data/torii.db`, overridden by `TORII_DB_PATH`. Credentials are encrypted;
public benchmark data is not. The database and master key must both survive
container updates. Legacy benchmark JSON caches are imported once when intelligence
is enabled and retained for recovery. Back up the database before upgrading;
rolling back to a pre-schema-2 binary requires restoring that backup. See
[benchmark storage and migration](docs/benchmark-intelligence.md#cache-management-and-refresh).

### Providers

`providers` must contain at least one provider. The Version 1 build
accepts these fields:

| Field | Description |
| --- | --- |
| `id` | Unique lowercase namespace matching `[a-z0-9][a-z0-9._-]*`. This becomes the prefix in public model IDs. |
| `type` | `ollama`, `openai`, or `openai_compatible`. The `openai` profile is locked to `https://api.openai.com` and uses an API key, while `openai_compatible` is for other standard-compatible origins. |
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
`lab/llama3.2`. A request for either name goes only to the named server. Torii
Router never falls back to a different provider after a request starts.

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
      env: PRIVATE_PROVIDER_TOKEN
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
API key and encrypts it in SQLite; YAML receives only a credential reference.
**Show** reveals only a newly entered value, never an existing stored key.
Legacy environment/file inputs are imported once, not tracked continually.
Use HTTPS for remote console access. See [credential storage](docs/credential-storage.md)
for migration, backup and recovery instructions.

The normal OpenAI API can be enabled with its origin-locked profile:

```yaml
  - id: openai
    type: openai
    base_url: https://api.openai.com
    bearer_token:
      env: OPENAI_API_KEY
    project: proj_example       # optional
    organization: org_example   # optional
```

The profile exposes models as `openai/<model>`, sends only the configured API
key and optional OpenAI project/organization headers, and does not use the
gateway's persistent OAuth credential store.

The `openai-codex` provider automatically discovers the visible models for
the logged-in subscription. A `models` list is optional and is only a fallback,
not an allowlist. Import credentials with `gateway auth login openai-codex`
or the web console's one-shot helper. Credentials remain endpoint-bound:
the helper's `chatgpt.tokens.use.direct` tokens use the documented
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
Use `gateway models --config ./config.yaml` to inspect the catalog.

### Classifier providers for Smart Routing

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
classifier and redirects Smart Routing visitors to Providers. Deterministic
routing still works without a classifier. Use **Save & reload** to apply the
selected model and provider-backed transport. Legacy direct Jev configuration
is moved into a provider draft, preserving its key reference; save to persist
the migration.

```yaml
providers:
  - id: jev
    type: jev
    base_url: https://api.typesafe.ai
    api_path: /v1
    bearer_token:
      env: JEV_API_KEY
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

Optional catalog metadata overrides can be supplied with
`router.catalog_file: ./catalog-overrides.json`. The file is a strict JSON
array of catalog descriptors and is applied after bundled and live provider
metadata. OpenRouter quota credits require a separate `management_token`; the
inference bearer token is never reused for management requests.

## Verify the installation

The health endpoint does not require authentication:

```sh
curl --fail http://127.0.0.1:8080/healthz
```

List the models discovered from all configured Ollama servers:

```sh
curl --fail http://127.0.0.1:8080/v1/models \
  -H "Authorization: Bearer $TORII_API_KEY"
```

Create a non-streaming chat completion (replace the model with one returned by
the previous command):

```sh
curl --fail http://127.0.0.1:8080/v1/chat/completions \
  -H "Authorization: Bearer $TORII_API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "local/llama3.2",
    "messages": [{"role": "user", "content": "Hello!"}],
    "stream": false
  }'
```

For streaming, set `"stream": true` and use `curl --no-buffer`. OpenAI SDKs
can connect by setting their base URL to `http://127.0.0.1:8080/v1`, using the
configured Torii API key, and selecting a provider-qualified model name.

The same transparent forwarding is available at `POST /v1/completions`,
`POST /v1/embeddings`, and `POST /v1/responses`. Responses requests are
stateless: `previous_response_id` and `conversation` are rejected with an
`unsupported_parameter` error rather than being ignored or stored.

## Provider authentication administration

Provider credentials that require persistent state can be imported without
placing token values in arguments or YAML:

```sh
export PROVIDER_ACCESS_TOKEN="..."
gateway auth login <provider-id> --config ./config.yaml \
  --access-token-env PROVIDER_ACCESS_TOKEN
gateway auth status <provider-id> --config ./config.yaml
gateway auth logout <provider-id> --config ./config.yaml
```

`login` also accepts `--access-token-file`, optional refresh-token environment
or file flags, an RFC3339 `--expires-at`, and account/principal identifiers.
Command output reports state only and never prints stored token values.

## Operational notes

- Model discovery runs at startup. An unavailable provider does not hide models
  from healthy providers; later refreshes retain its last-known-good entries as
  stale. Startup fails when no configured provider has a usable live or bundled
  catalog.
- Inference request bodies are limited to 32 MiB.
- Upstream inference has no gateway-imposed completion timeout. Client
  disconnects cancel the upstream request.
- Every response includes `X-Request-ID`. Safe client-supplied IDs are retained;
  otherwise the gateway generates one and uses it for structured request logs
  and upstream correlation.
- On `SIGINT` or `SIGTERM`, the server stops accepting connections and gives
  active requests up to 10 seconds to finish.
- TLS termination is not built in. Bind to loopback for local use, or place
  the gateway behind a trusted TLS reverse proxy on a private network.
- Protect the API key, configuration, and any secret files with appropriate
  filesystem and process permissions. Provider URLs cannot contain embedded
  credentials.

## Development

Run the standard checks with:

```sh
gofmt -w .
go mod tidy
go vet ./...
go test ./...
go test -race ./...
go build ./...
```

See [`docs/development.md`](docs/development.md) for the development workflow
and [`AGENTS.md`](AGENTS.md) for project constraints.

## License

Torii Gateway is available under the [MIT License](LICENSE).

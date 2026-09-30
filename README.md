# Torii Router

Torii Router is a lightweight, resource-efficient LLM gateway written in Go.
It presents an OpenAI-compatible HTTP API in front of one or more Ollama servers or standard OpenAI-compatible
endpoints and gives every upstream model a provider-qualified name such as
`local/llama3.2`. The current build implements the repository's Version 1
scope. It needs no database, Redis instance, queue, sidecar, or web interface.

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
- bearer authentication for every API endpoint except `GET /healthz`
- secrets loaded from an environment variable or a file, never inline YAML
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
git clone https://github.com/Apartment38/torii-router.git
cd torii-router
mkdir -p bin
go build -o ./bin/gateway ./cmd/gateway
```

Copy the example configuration, choose a strong client-facing
API key, and start the server:

```sh
cp config.example.yaml config.yaml
export TORII_API_KEY="replace-with-a-long-random-secret"
./bin/gateway serve --config ./config.yaml
```

The `--config` flag defaults to `/config/config.yaml`. Torii Router validates
the complete configuration and resolves its API-key secret before opening the
listener, so configuration or secret errors cause startup to fail rather than
leaving a partially configured service running.

## Configuration

Torii Router reads exactly one YAML document. Unknown fields are rejected to
catch spelling mistakes and configuration intended for unsupported features.
The included [`config.example.yaml`](config.example.yaml) is a complete
starting point:

```yaml
server:
  listen: 127.0.0.1:8080
  authentication:
    api_key:
      env: TORII_API_KEY

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
| `server.authentication.api_key` | yes when serving | A secret reference used to authenticate clients. Select exactly one of `env` or `file`. |

The API key must be non-empty and cannot contain whitespace. To load it from a
mounted secret file instead of the environment, use:

```yaml
server:
  listen: 0.0.0.0:8080
  authentication:
    api_key:
      file: /run/secrets/torii-api-key
```

Environment-backed secret values are used exactly as supplied. Surrounding
whitespace is removed from file-backed values, which makes files ending in a
newline safe to use. Do not put the secret value itself in YAML.

### State

`state.directory` is required and reserves the location for gateway state.
Use a writable, persistent path such as `/state` for deployments. Version 1
uses it for endpoint-bound credentials and aggregate usage; session affinity is
intentionally in memory.

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
Provider credentials are never accepted inline, are isolated in a dedicated
HTTP client, and are not forwarded across redirects.

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

The gateway also accepts an endpoint-bound `openai-codex` provider with
static model IDs. Import credentials with `gateway auth login openai-codex`;
the provider refreshes near-expiry OAuth credentials against the configured
Codex issuer and never sends them to the normal OpenAI API origin. Use
`gateway models --config ./config.yaml` to inspect the configured catalog.

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

Torii Router is available under the [MIT License](LICENSE).

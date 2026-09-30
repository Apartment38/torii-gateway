# Torii Router

Torii Router is a lightweight, resource-efficient LLM gateway written in Go.
It presents an OpenAI-compatible HTTP API in front of one or more Ollama
servers and gives every upstream model a provider-qualified name such as
`local/llama3.2`. The current build is a **pre-v1 milestone**: it is a usable
Ollama-backed OpenAI-compatible gateway, but it does not yet meet the repository's
Version 1 scope. It needs no database, Redis instance, queue, sidecar, or web
interface.

The gateway is designed for clients that already support the OpenAI API. It
rewrites only the requested model name and otherwise
passes the JSON request through to Ollama, preserving unknown fields. Both
ordinary JSON responses and streamed SSE responses are relayed where supported,
and explicit model requests are always sent to the provider named in the model
ID.

## Currently implemented features

- OpenAI-compatible model, Chat Completions, Completions, Embeddings, and
  stateless Responses endpoints
- multiple Ollama upstreams with `provider/model` public model IDs
- transparent request forwarding, including fields the gateway does not know
- low-overhead, byte-for-byte SSE streaming
- bearer authentication for every API endpoint except `GET /healthz`
- secrets loaded from an environment variable or a file, never inline YAML
- strict configuration validation and graceful shutdown
- request IDs, typed OpenAI-style errors, and secret-safe structured logging

This pre-v1 milestone performs **explicit routing only**. It does not yet
provide the Version 1 `smart` virtual model, automatic model selection, session
affinity, quota/cost-aware routing, the full OpenAI endpoint set, or required
non-Ollama providers. [`TASKS.md`](TASKS.md) is the ordered implementation
roadmap, [`docs/implementation-plan.md`](docs/implementation-plan.md) defines
the complete Version 1 contract, and [`docs/architecture.md`](docs/architecture.md)
describes implemented versus planned behavior.

## Requirements

- Go 1.25.1 or newer (to build from source)
- one or more reachable [Ollama](https://ollama.com/) servers with their
  OpenAI-compatible API enabled
- an Ollama model already installed, for example `ollama pull llama3.2`

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
Use a writable, persistent path such as `/state` for deployments. The current
explicit-routing milestone does not write state, but keeping the path stable
allows future stateful features to be enabled without restructuring the
deployment.

### Providers

`providers` must contain at least one provider. The current pre-v1 build
accepts these fields:

| Field | Description |
| --- | --- |
| `id` | Unique lowercase namespace matching `[a-z0-9][a-z0-9._-]*`. This becomes the prefix in public model IDs. |
| `type` | Must currently be `ollama`. |
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

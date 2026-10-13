<p align="center">
  <img src="assets/torii-gateway-logo.png" alt="Torii Gateway logo" width="360" />
</p>

# Torii Gateway

Torii Gateway is a smart model router that connects clients to local and hosted models through one unified API.


Torii Gateway is targeted toward homelabbers and developers that want to aggregate various providers behind a single API, control public access via API keys, and reduce usuage costs. 
Implemented as a lightweight Go service, it includes a web configuration console, streaming, encrypted provider credentials, named client API keys, subscription login, and optional benchmark intelligence.
It uses embedded SQLite to maintain state; No separate database server or sidecar is required.
It supports any OpenAI-Compatible API provider backend, local ollama installs, and some subscription services via OAuth.
You choose which providers and models to expose, and control access through API keys.
You can create multiple virtual `smart` models that use a decision model (like Jev) to choose the model best suited for a task according to criteria you define.

> ### DISCLAIMER:
> This project was entirely vibe coded. It has not been tested or audited, and should not be deployed in a production environment.
> Do not expose it to the open internet unless you are confident that you understand and accept the security implications.
> Consider it a public beta. Use it at your own risk.

## Features

| Feature | Description | Status |
| --- | --- | --- |
| Unified API | Connect local Ollama and hosted providers through one OpenAI-compatible API. | Working |
| Smart routing | Expose virtual `smart` models that select eligible models using configurable criteria. | Working |
| Routing policies | Control model selection by capabilities, context, quality, estimated cost, location, quotas, and latency. | Implemented, untested |
| Streaming | Stream model responses with JSON and SSE transport support. | Working |
| Web console | Configure providers and models, test connections, and save changes through an embedded UI. | Working |
| Secure credentials | Encrypt provider credentials and manage access with named client API keys. | Implemented, untested |
| Subscription login | Connect eligible Codex accounts through local or remote login. | Working |
| Benchmark intelligence | Optionally refresh and cache benchmark data to inform routing. | Implemented, untested |
| Embedded persistence | Store configuration, credentials, benchmark data, and usage in SQLite without a separate database server. | Working |
| TUI | Terminal User Interface; Feature parity with Web console. | Planned |
| Skill Gateway | Centralized version controlled skill registry. | Planned |
| MCP Gateway | Model Context Protocol Server aggregator & gateway. | Planned |
| User Accounts | Add other users with their own accounting and settings. | Planned |
| Authentik Integration | Use Authentik for web console logins. | Planned |

See [Features and limitations](docs/reference/features.md) for details on the supported
endpoints, constraints, and features that are not implemented.

## Quickstart

Choose a source build, Podman, or Podman Compose below.

### From source

You need Go 1.25.1 or newer, Git, OpenSSL, and a local Ollama installation with
its service running. For other providers or installation methods, see
[Getting Started](docs/getting-started.md) and [Installation](docs/installation/README.md).

Run these commands in a terminal on the computer running Ollama:

```sh
git clone https://github.com/Apartment38/torii-gateway.git
cd torii-gateway
mkdir -p bin state secrets
umask 077
go build -trimpath -o ./bin/gateway ./cmd/gateway
ollama pull llama3.2
openssl rand -hex 32 > secrets/torii_db_key
export TORII_DB_KEY_FILE="$PWD/secrets/torii_db_key"
./bin/gateway serve --database ./state/torii.db --listen 127.0.0.1:8080
```

Generate the master key only for a new installation. Back it up separately from
`state/` and reuse it on every restart. Choose your administrator username and a
password of at least 12 characters when prompted. Save the generated API key;
it is shown once.

Open <http://127.0.0.1:8080/> and sign in. Add an Ollama provider named `local`
with URL `http://127.0.0.1:11434` and confirm **Save provider**. In another terminal, replace the key
placeholder with your generated key, then send a request:

```sh
export GATEWAY_API_KEY='PASTE_YOUR_GENERATED_GATEWAY_KEY'
curl --fail http://127.0.0.1:8080/v1/chat/completions \
  -H "Authorization: Bearer $GATEWAY_API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{"model":"local/llama3.2","messages":[{"role":"user","content":"Hello!"}]}'
```

The response contains the model's answer. Your client's base URL is
`http://127.0.0.1:8080/v1`.

### With Podman or Podman Compose

You need Git, OpenSSL, and Podman. For Compose, also install a working Compose
provider such as `podman-compose`. These commands assume rootless Podman on
Linux; see [container installation](docs/installation/containers.md) for other
hosts, permissions, and networking details. Go is built inside the image.

Prepare a **new installation** on the container host:

```sh
git clone https://github.com/Apartment38/torii-gateway.git
cd torii-gateway
umask 077
mkdir -p secrets
openssl rand -hex 32 > secrets/torii_db_key
```

Generate the master key only once, back it up separately, and reuse it on every
restart. Choose **one** of the following launch methods.

**Podman:** build the image and mount a native Podman secret and persistent volume:

```sh
podman build --format docker -t localhost/torii-gateway:local .
podman secret create torii_db_key secrets/torii_db_key
podman run -d --name torii-gateway \
  -p 127.0.0.1:8080:8080 \
  -v torii_gateway_database:/data:Z \
  --secret torii_db_key,type=mount,target=torii_db_key,uid=65532,gid=65532,mode=0400 \
  -e TORII_DB_KEY_FILE=/run/secrets/torii_db_key \
  localhost/torii-gateway:local
podman logs torii-gateway
```

Stop with `podman stop torii-gateway` and restart with
`podman start torii-gateway`; keep the secret and database volume.

**Podman Compose:** map the key file's ownership to the image's non-root user,
then build and start the service:

```sh
podman unshare chown 65532:65532 secrets/torii_db_key
podman compose -f compose.yaml -f compose.secrets.yaml up --build -d
podman compose -f compose.yaml -f compose.secrets.yaml logs --tail=100 torii-gateway
```

Compose publishes port 8080 on all host interfaces; restrict access with your
firewall. Stop with `podman compose -f compose.yaml -f compose.secrets.yaml down`
(without `-v`, which deletes the database volume), and restart with the same
`up` command and key.

Open <http://127.0.0.1:8080/> on the container host. Enter the setup code from the
logs (valid for 30 minutes), choose an administrator username and password of at
least 12 characters, and save the generated API key shown once.

For Ollama running on the host, add an Ollama provider named `local` with URL
`http://host.containers.internal:11434`, not `127.0.0.1`. Ollama must listen on a
host-reachable interface; follow the [host Ollama networking instructions](docs/installation/containers.md#connect-to-ollama-on-the-host)
and restrict its port to trusted clients. Pull `llama3.2` on the host with
`ollama pull llama3.2`, then use the request example above to verify inference.
Your client's base URL is `http://127.0.0.1:8080/v1`.

## Documentation

- [Full documentation](docs/README.md)
- [Docker, Podman, native, and source installation](docs/installation/README.md)
- [Features and limitations](docs/reference/features.md)

## Contribution and Development

Contributions are welcome. Start with the [Contributing guide](docs/development/contributing.md) and [AGENTS.md](AGENTS.md) for development requirements, build and test commands, and the contributor workflow.

- [Development and architecture](docs/development/README.md): an overview of the internals and development resources.
- [Design contracts](docs/development/design.md): implementation constraints and expected behavior.
- [Documentation conventions](docs/development/documentation.md): guidance for updating project documentation.

## License
Torii Gateway is available under the [MIT License](LICENSE).

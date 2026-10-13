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

> **Disclaimer:** This project was entirely vibe coded. It has not been tested or audited, and should not be deployed in a production environment. Use at your own risk. 

## Quickstart

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
curl --fail http://127.0.0.1:8080/v1/chat/completions   -H "Authorization: Bearer $GATEWAY_API_KEY"   -H 'Content-Type: application/json'   -d '{"model":"local/llama3.2","messages":[{"role":"user","content":"Hello!"}]}'
```

The response contains the model's answer. Your client's base URL is
`http://127.0.0.1:8080/v1`.

## Documentation

- [Full documentation](docs/README.md)
- [Docker, Podman, native, and source installation](docs/installation/README.md)
- [Features and limitations](docs/reference/features.md)
- [Development and architecture](docs/development/README.md)

Torii Gateway is available under the [MIT License](LICENSE).

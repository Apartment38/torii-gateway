# Connect clients and make requests

Use this guide after [Getting Started](../getting-started.md). You need a Torii
API key and a public model ID returned by `GET /v1/models`.

## Configure an OpenAI-compatible client

| Client setting | Value for a local installation |
| --- | --- |
| Base URL | `http://127.0.0.1:8080/v1` |
| API key | Your Torii gateway client key |
| Model | A discovered ID such as `local/llama3.2`, or `smart` |

Use your actual hostname/port for remote Torii, and HTTPS for remote authenticated
traffic. A provider's URL/key belongs in Torii's settings, not in the client.
An OpenAI SDK that accepts a custom base URL can use these same values; endpoint
availability still depends on the selected provider.

## List models and chat

```sh
export GATEWAY_API_KEY='PASTE_YOUR_GENERATED_GATEWAY_KEY'
curl --fail http://127.0.0.1:8080/v1/models   -H "Authorization: Bearer $GATEWAY_API_KEY"
curl --fail http://127.0.0.1:8080/v1/chat/completions   -H "Authorization: Bearer $GATEWAY_API_KEY"   -H 'Content-Type: application/json'   -d '{"model":"local/llama3.2","messages":[{"role":"user","content":"Hello!"}],"stream":false}'
```

Replace the model with one returned in `data[].id`. A normal chat response
contains `choices`; an error contains `error`. An explicit model always goes to
its named provider, regardless of smart-routing policies.

## Stream a response

```sh
curl --fail --no-buffer http://127.0.0.1:8080/v1/chat/completions   -H "Authorization: Bearer $GATEWAY_API_KEY"   -H 'Content-Type: application/json'   -d '{"model":"local/llama3.2","messages":[{"role":"user","content":"Write a short poem."}],"stream":true}'
```

Expect SSE (`data:`) events as output arrives. Compatible transports preserve
upstream bytes; translated providers adapt events to the public endpoint.
Disconnecting cancels the upstream request. Torii cannot switch models after it
has sent response bytes. A reverse proxy must allow streaming without buffering.

## Windows PowerShell example

```powershell
$env:GATEWAY_API_KEY = 'PASTE_YOUR_GENERATED_GATEWAY_KEY'
$toriiHeaders = @{ Authorization = "Bearer $env:GATEWAY_API_KEY" }
Invoke-RestMethod -Uri http://127.0.0.1:8080/v1/models -Headers $toriiHeaders
$toriiBody = @{ model = 'local/llama3.2'; messages = @(@{ role = 'user'; content = 'Hello!' }) } | ConvertTo-Json -Depth 6
Invoke-RestMethod -Method Post -Uri http://127.0.0.1:8080/v1/chat/completions -Headers $toriiHeaders -ContentType application/json -Body $toriiBody
```

For streaming, use `curl.exe --no-buffer` with a JSON file passed as `-d @request.json`.

## Other endpoints and tools

Torii also exposes Completions, Embeddings, and stateless Responses. Select a
provider/model supporting the endpoint; [Provider support](../reference/providers.md)
records restrictions. [API reference](../reference/api.md) contains request examples.
Responses fields `previous_response_id` and `conversation` are rejected: clients
must send the context needed for the next request themselves.

Tool/vision/structured-output support depends on verified model capabilities.
For smart tool conversations, send a stable `X-Session-ID` on every turn and keep
all tool-call/result IDs intact. See [Smart routing](smart-routing.md#keep-a-conversation-on-one-model).
Use `curl -i` to inspect request and routing headers when diagnosing selection.

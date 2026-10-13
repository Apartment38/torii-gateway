# Getting Started

By the end of this tutorial, you will have a working administrator login, a
connected model provider, and a response through Torii's API. You need Torii
running from one of the [installation guides](installation/README.md).

The examples use a local Ollama model. You can instead use a hosted provider
account: follow [Providers](guides/providers.md) and substitute its discovered
model ID in the request. Torii itself does not install or run model weights.

## 1. Create your administrator

Open <http://127.0.0.1:8080/> (or the host/port chosen during installation).
If you already created an administrator in the terminal, sign in with that account.
Otherwise, copy the unused setup code from Torii's startup output or container
logs into the setup form, choose a username, and enter a password of at least
12 characters. Setup codes expire after 30 minutes; restart Torii for a new one.

Save the generated gateway API key in your password manager before closing the
one-time display. Your administrator password opens settings; the gateway API
key lets applications request inference. The database master key is a third,
separate secret used by Torii to decrypt stored credentials.

## 2. Prepare a model

For the local Ollama example, install and start Ollama separately, then run on
the Ollama host:

```sh
ollama pull llama3.2
curl --fail http://127.0.0.1:11434/api/tags
```

The second command should list installed models. For Torii in a container, first
follow [Connect to host Ollama](installation/containers.md#connect-to-ollama-on-the-host).

## 3. Add and test the provider

1. Sign in to Torii, open **Providers**, and click **Add provider**.
2. Choose the Ollama provider type and use provider ID `local`.
3. Set its URL to `http://127.0.0.1:11434` for native Torii, or
   `http://host.containers.internal:11434` for the included Compose deployment.
4. Test the connection. You should see the installed model in the discovered list.
5. Choose **Save Provider**. A successful save applies the settings and reloads
   the active configuration. If validation fails, fix the indicated setting;
   the previous configuration stays active.

If your quickstart already configured this provider, just test it and verify the
model. Provider IDs prefix every public model name, so this model becomes
`local/llama3.2`. [Provider setup](guides/providers.md) covers hosted services.

## 4. Check Torii and list models

In a second POSIX terminal on the Torii host, paste the gateway key you saved:

```sh
export GATEWAY_API_KEY='PASTE_YOUR_GENERATED_GATEWAY_KEY'
curl --fail http://127.0.0.1:8080/healthz
curl --fail http://127.0.0.1:8080/v1/models   -H "Authorization: Bearer $GATEWAY_API_KEY"
```

The model response contains `data` entries with public model IDs. Choose a real
inference model; classifier providers do not appear in this list. Health confirms
that Torii's HTTP server is running, not that every upstream is reachable.

## 5. Send your first request

```sh
curl --fail http://127.0.0.1:8080/v1/chat/completions   -H "Authorization: Bearer $GATEWAY_API_KEY"   -H 'Content-Type: application/json'   -d '{"model":"local/llama3.2","messages":[{"role":"user","content":"Say hello in one sentence."}],"stream":false}'
```

Expect a JSON response with the answer in `choices[0].message.content`.
Replace `local/llama3.2` with a model from your list when using another provider.
For Windows and streaming examples, see [Clients](guides/clients.md).

## Next steps

Set your AI client's base URL to `http://127.0.0.1:8080/v1`, its API key to the
Torii gateway key, and its model to the public model ID. You can create a separate
[named key for each client](guides/credentials.md#give-each-client-its-own-key).
Try [smart routing](guides/smart-routing.md) once provider discovery works, and
[back up your installation](guides/backup-and-upgrade.md) before an update.
If you do not get a response, start with [Troubleshooting](guides/troubleshooting.md).

# Operate Torii

Use these checks after installation and when preparing remote access.

## Check health and logs

```sh
curl --fail http://127.0.0.1:8080/healthz
./bin/gateway healthcheck --url http://127.0.0.1:8080/healthz
```

Health is public and reports the server's availability. Test a provider and make
an inference request to check the complete path. Containers include this healthcheck.
For Podman file-secret deployments:

```sh
podman compose -f compose.yaml -f compose.secrets.yaml logs --tail=100 torii-gateway
```

Use your normal Docker overrides on Docker. Native services can use
`journalctl -u torii-gateway.service`. Logs are structured JSON and include request IDs, paths, statuses,
and durations, plus safe provider/benchmark errors. They omit prompts, headers,
query values, tokens, and arbitrary upstream error bodies. Avoid sharing startup
logs containing an unused administrator setup code.

Every response includes `X-Request-ID`. Use it to correlate a failed request with
logs. Smart responses also expose selection/latency evidence; see [API](../reference/api.md).

## Expose Torii beyond your computer

TLS termination is not built in. Use a trusted HTTPS reverse proxy and restrict
access to the gateway's plain-HTTP listener. Native local use should keep
`127.0.0.1:8080`; containers require `0.0.0.0:8080` inside the container, with host
port/firewall rules controlling exposure. The base Compose port mapping publishes
to all host interfaces.

Configure `server.trusted_proxies` with the immediate proxy's CIDR before relying on forwarded HTTPS for Secure session cookies. Configure the proxy to preserve the host and forwarded scheme, allow request
bodies up to the gateway's 32 MiB limit, disable streaming buffering, and allow
long inference/benchmark refresh connections. Administrator Basic authentication
and OAuth credential transfers need HTTPS remotely. Management writes enforce
same-origin restrictions; access the console and API through the same origin.

## Timeouts, shutdown, and durability

Inference bodies are limited to 32 MiB. Ollama has a five-minute whole-request
timeout by default. Other compatible profiles have no completion timeout unless
`request_timeout` is set. Discovery/authentication have separate deadlines. See
[provider settings](../reference/configuration.md#providers). Client
disconnects cancel upstream work. The server allows 10 seconds to finish active
requests on SIGINT/SIGTERM, then attempts final usage persistence with a separate
deadline. Idle HTTP connections close after 60 seconds.

The durable operator ledger reserves budgets before dispatch and settles reported JSON/SSE usage afterward. Missing usage or interruptions retain conservative estimates. Reservations, budgets, and administrator sessions survive restarts. These estimated monetary costs are not provider billing. Legacy routing counters are buffered separately; upstream live quota caches and pending OAuth flows remain in memory.

The Activity page retains metadata for seven days by default with a 100 MiB cap. `activity` settings control retention; `activity.log_file` enables an optional rotating JSON file. API prompts and Jev protocol bodies are excluded by default. Server → Activity monitoring exposes size/retention and optional prompt/Jev debugging capture, both off by default. Captured bodies share the Activity limits and never appear in general logs; HTTP authorization headers are excluded. See [console monitoring](console.md#timezone-and-activity-monitoring).

Use one process per SQLite data volume on local storage. Keep the database and master key persistent. [Backup and upgrade](backup-and-upgrade.md) explains coherent backups
and schema compatibility. [State and security](../development/state-and-security.md)
explains implementation and security boundaries.

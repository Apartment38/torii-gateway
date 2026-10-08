# Command-line reference

Examples assume the native binary is `./bin/gateway`. Containers use `/gateway`;
the standalone remote login helper is `torii-auth`. Flags follow the command
(and provider ID for `auth`). Errors print to stderr and exit nonzero; successful
commands exit zero. A successful benchmark refresh may still report partial failure.

## Gateway commands

| Invocation | Purpose, output, and requirements |
| --- | --- |
| `gateway version` | Print build version, commit/time, and supported SQLite schema range; no config/master key needed |
| `gateway serve --database PATH` | Open database settings, perform first admin setup, discover providers, serve until shutdown |
| `gateway config validate --config PATH` | Strict YAML/schema validation without starting the server or querying providers; stored references are checked structurally |
| `gateway models --database PATH` | Open database, query configured providers, print their model IDs and `smart`; discovery failures fail command; may include classifier-provider models unlike public inference catalog |
| `gateway healthcheck --url URL` | Query health URL (default `http://127.0.0.1:8080/healthz`); nonzero on failure |
| `gateway admin create --database PATH [--username NAME]` | Create the one administrator, prompt/confirm password, display newly created API key once; fails if admin already exists |
| `gateway admin api-key --database PATH [--username NAME]` | Verify admin password, rotate only default gateway key, display it once |
| `gateway credentials replace --database PATH --id ID [--secret-file PATH]` | Verify administrator and replace provider secret using secure prompt or file |
| `gateway credentials delete --database PATH --id ID` | Verify administrator and delete generic provider credential; protected system records are denied |
| `gateway auth login PROVIDER --database PATH [LOGIN_FLAGS]` | Verify administrator, authenticate configured subscription provider or import token into encrypted store |
| `gateway auth status PROVIDER --database PATH` | Verify administrator; report logged in, expired, or logged out without token values |
| `gateway auth logout PROVIDER --database PATH` | Verify administrator and remove provider OAuth credential |
| `gateway benchmarks refresh --database PATH [--url ORIGIN]` | Load database settings and default gateway key from database and POST refresh to running gateway; print JSON summary |

`--database` uses `TORII_DB_PATH`, then `/data/torii.db`. Normal commands never read YAML.
`gateway config import --config PATH --database PATH` validates and explicitly imports a legacy configuration and its credentials in one transaction; it refuses established database settings.
`serve --listen ADDRESS` or `TORII_LISTEN` overrides the listener until the next restart.
Commands opening the database need the original master key and accessible data
path. Native relative paths resolve from the working directory. Local management
passwords are prompted securely in a terminal; automation reads them from stdin,
not a password environment variable or command-line flag. `admin create` needs
`--username` for unattended input. Use `--secret-file` to reserve stdin for the
administrator password during credential replacement.

## Authentication login flags

| Flag | Meaning |
| --- | --- |
| `--access-token-env NAME` / `--access-token-file PATH` | Exactly one source of imported access token |
| `--refresh-token-env NAME` / `--refresh-token-file PATH` | Optional imported refresh token; choose one source |
| `--expires-at TIME` | Optional RFC3339 access-token expiry |
| `--account-id ID` | Optional upstream account identifier |
| `--principal-id ID` | Optional upstream principal identifier |
| `--browser` | Codex browser authorization-code/PKCE instead of default device-code login; Codex only |

Without imported token flags, Codex/Nous start their device-code login flow.
Import values from protected files/environment references, not literal CLI args.
Login-only token/account flags are rejected by `status` and `logout`.

```sh
./bin/gateway auth login openai-codex --database ./state/torii.db
./bin/gateway auth status openai-codex --database ./state/torii.db
./bin/gateway auth logout openai-codex --database ./state/torii.db
```

Replace `openai-codex` with the configured provider ID. For browser login add
`--browser` to `login`. Use the [provider guide](../guides/providers.md) for remote
container login and protocol-specific setup.

## Benchmark refresh URL

When `--url` is omitted, refresh derives an HTTP origin from `server.listen`,
replacing wildcard hosts with loopback. `--url` takes an origin such as
`https://gateway.example.com`, not the endpoint path. Intelligence must be enabled,
the default gateway key must exist, and the running server must be reachable.
Source partial failures appear in `partial_failure` even on exit zero.

## Remote login helper

```sh
torii-auth --session-url https://gateway.example.com/gateway/v1/providers/login/session   --pairing-token 'ONE_TIME_TOKEN_FROM_CONSOLE'
```

Run the OS/architecture-matching helper on the computer with your browser.
Both flags are required; use the exact pending session URL/token from the console.
The helper opens login locally, receives the loopback callback, transfers the
credential, and exits. The token expires in 10 minutes and is single use. It must
not be shared. `torii-auth version` prints build metadata. The helper needs no
local gateway database/master key and does not keep running after success.

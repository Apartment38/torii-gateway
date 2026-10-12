# Environment variables

These are runtime and Compose settings. They do not replace your provider settings.
For key creation and permissions, see [Credentials](../guides/credentials.md).

| Variable | Default | Behavior |
| --- | --- | --- |
| `TORII_DB_KEY_FILE` | `/run/secrets/torii_db_key` | File holding 64 hex characters (32 random bytes), optional trailing newline. A present file takes precedence over `TORII_DB_KEY`; an explicit unreadable/missing/invalid file fails |
| `TORII_DB_KEY` | None | Required fallback master key when the default key file is absent; never generate a new value for an existing database |
| `TORII_DB_PATH` | `/data/torii.db` | Overrides durable database filesystem path; not a SQLite URI; restart required |
| `TORII_LISTEN` | Stored `server.listen`, initially `0.0.0.0:8080` | Listener bootstrap override; `--listen` takes precedence; restart required |
| `TORII_PORT` | `8080` | Compose host port only; container still listens on 8080 |
| `TORII_IMAGE` | `torii-gateway:local` | Compose image name/tag/digest; use `up --no-build` to run a pulled release image |
| `TORII_DB_KEY_SOURCE` | `./secrets/torii_db_key` | Compose file-secret source when applying `compose.secrets.yaml` |

`TORII_API_KEY` is not read. Administrator and inference keys are managed in
SQLite. `--database` takes precedence over `TORII_DB_PATH`. Provider keys use stored credential references; arbitrary secret-env/file
refs in persisted settings are rejected. `GATEWAY_API_KEY` in curl examples is a
client shell convenience, not a Torii startup setting.

Compose reads its project `.env` for substitution. Native Torii does not parse
`.env` automatically; export the runtime variables in the shell/service environment.
Keep environment/key files private and outside source control. Privileged host
operators can inspect container environments.

Opt-in test variables such as `TORII_LIVE_BENCHMARK_SMOKE` are development-only;
see [Contributing](../development/contributing.md).

## Password reset email delivery

These optional runtime variables are read when the gateway starts (or rebuilds
its handler); restart after changing them. Email recovery requires a saved admin
email, a public HTTPS origin, and a TLS-capable SMTP relay. Missing or invalid
configuration disables delivery while retaining the generic request response.
SMTP errors log a generic delivery failure without recipients, tokens, or secrets.

| Variable | Default | Behavior |
| --- | --- | --- |
| `TORII_PUBLIC_URL` | None | Public HTTPS origin, for example `https://torii.example.com`; no credentials, path, query, or fragment. Reset links never use the incoming Host or forwarded-host headers. |
| `TORII_SMTP_ADDR` | None | SMTP `host:port`, commonly port 587 for STARTTLS or 465 for implicit TLS. |
| `TORII_SMTP_FROM` | None | Sender email address without a display name; authorize it at your relay. |
| `TORII_SMTP_TLS` | `starttls` | `starttls` requires a successful TLS upgrade; `implicit` starts TLS immediately. Both verify certificates and require TLS 1.2 or newer. Plaintext delivery is unsupported. |
| `TORII_SMTP_USERNAME` | None | SMTP AUTH username. Omit username and password together only for a relay that permits unauthenticated submission over TLS. |
| `TORII_SMTP_PASSWORD_FILE` | None | Readable file containing the SMTP password; strips trailing CR/LF and takes precedence over `TORII_SMTP_PASSWORD`. An unreadable file disables delivery. |
| `TORII_SMTP_PASSWORD` | None | SMTP password fallback; prefer a mounted password file. Both username and password are required when either is supplied. |

For a native service, set these in its private service environment and mount a
readable password file. Example values (replace the host, sender, and username):

```sh
export TORII_PUBLIC_URL=https://torii.example.com
export TORII_SMTP_ADDR=smtp.example.com:587
export TORII_SMTP_FROM=torii@example.com
export TORII_SMTP_USERNAME=torii@example.com
export TORII_SMTP_PASSWORD_FILE=/run/secrets/torii_smtp_password
```

For Compose, add those variables under `services.torii-gateway.environment` in a
local override file and mount the password file read-only at the configured path;
Compose project `.env` values alone are not passed into the container. Use a
certificate trusted by the gateway for your relay; certificate verification
cannot be disabled. A reverse proxy should terminate HTTPS and set
`server.trusted_proxies` when forwarding HTTPS to Torii.

Torii allows one reset email worker at a time and accepts a new worker at most
every ten seconds globally. A persisted one-minute issuance cooldown limits
mail to the singleton administrator. Reset links expire after 30 minutes and
are single-use. Apply additional request limits at the reverse proxy if needed.

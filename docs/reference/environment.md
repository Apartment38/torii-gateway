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

# Encrypted credential storage

Torii encrypts credential payloads with AES-256-GCM before storing them in
SQLite. SQLite metadata is not encrypted: provider/service, purpose, owner,
timestamps and opaque IDs are readable. No SQLCipher or external service is
required. Non-sensitive settings remain in YAML; usage and benchmark caches
remain in `state.directory`.

The default database is `/data/torii.db`. Set `state.database_path` or override
it with `TORII_DB_PATH` (use `./state/torii.db` for a local build). Database-path
changes require an offline config edit and restart. SQLite uses WAL, a 5-second
busy timeout, foreign keys and schema migrations. Database/WAL/SHM files are
owner-only. Mount local persistent storage, not a shared/network filesystem.

## Master key

Generate exactly 32 random bytes encoded as 64 hexadecimal characters:

```sh
umask 077
mkdir -p secrets
openssl rand -hex 32 > secrets/torii_db_key
```

Back this key up separately before migrating. Torii never generates a master
key automatically and never stores it in YAML or SQLite. Files in `secrets/`,
`.env`, and SQLite files are excluded from Git and the Docker build context.

Startup reads `TORII_DB_KEY_FILE`, default `/run/secrets/torii_db_key`, first.
A present file always takes precedence over `TORII_DB_KEY`; an explicitly
configured missing/unreadable/invalid file is an error, not a fallback. A key
file may end in a newline. If the default file is absent, the environment
variable is accepted. Missing/malformed keys, incorrect keys and corrupt
encrypted records stop startup. Credential records carry key version 1;
automatic rotation is not implemented. Never change the master key on an
existing database without an explicit re-encryption procedure.

## Containers

The base Podman Compose file mounts persistent `torii_gateway_database` at
`/data` and `torii_gateway_state` at `/state`; the image runs as UID/GID 65532.
Podman's `:U` adjusts only the config directory/state volume, not the repository.
The database directory in the image is owned by the non-root user.

For a file-backed Compose secret:

```sh
# Ensure the mounted secret is readable by UID 65532 inside the container.
# For rootless Podman, map ownership without changing the repository:
podman unshare chown 65532:65532 secrets/torii_db_key
podman compose -f compose.yaml -f compose.secrets.yaml up --build -d
```

Compose file secrets are mounted files, not an encrypted host secret vault.
Some implementations do not honor secret uid/gid/mode settings; verify actual
read access without printing the key. An alternate bind mount can use SELinux
`:ro,Z` when needed. Override the source with `TORII_DB_KEY_SOURCE`.

Native Podman secrets are also supported:

```sh
podman secret create torii_db_key secrets/torii_db_key
# Add to your podman run options:
# --secret torii_db_key,type=mount,target=torii_db_key,uid=65532,gid=65532,mode=0400
```

For Docker Compose, prepare the host config directory for the non-root user
and apply the Docker override (Docker does not support Podman's `:U`):

```sh
mkdir -p config
sudo chown -R 65532:65532 config
sudo chown 65532:65532 secrets/torii_db_key
docker compose -f compose.yaml -f compose.docker.yaml -f compose.secrets.yaml up --build -d
```

Docker Swarm can use a native `docker secret create torii_db_key
secrets/torii_db_key` and service secret mounting instead. Mount the database
volume and give the secret UID 65532 with mode 0400. Local Docker Compose
does not consume Swarm secrets automatically.

For the less-preferred environment fallback, set `TORII_DB_KEY` in a private
project `.env` and use the base Compose file (plus Docker override on Docker).
Environment values can be inspected by privileged host/container operators;
do not put them on command lines, in images, or in logs. Existing `.env`
installations need only the master key and `TORII_PORT` in `.env`. An existing
`TORII_API_KEY` is accepted only for one-time migration, not normal management.

Never run `compose down -v` or delete the database volume during an update.
Rebuilding/recreating the container preserves credentials when the same volume
and master key are retained. Existing bind-mount permission changes must target
only the dedicated config/data directories, never the repository root.

## Console authentication and management

Management requires HTTP Basic login with your chosen administrator username
and password. Static console assets, health checks and setup status remain
public; inference requires the separate bearer API key. The one administrator
uses the stable internal owner `default`; multi-user accounts are not implemented.
Passwords use salted PBKDF2-HMAC-SHA256 with 600,000 iterations. Only the hash,
salt and username are stored, inside an encrypted database record. Basic login
has bounded hashing concurrency and a five-failed-attempts/minute/client limit.

First launch prompts for an administrator in an interactive terminal. For
unattended Docker/Podman startup, inspect `compose logs torii-gateway`, then
enter the random one-time setup code in the console. The code expires after
30 minutes; restart to issue a new code or run the local command below. Setup
can only create an account when none exists, including upgraded installations.
Avoid sharing logs containing an unused setup code. Alternatively:

```sh
./gateway admin create --config ./config.yaml
# Container variant:
podman compose exec torii-gateway /gateway admin create --config /config/config.yaml
```

Choose your username and confirm a password of at least 12 characters. A new
API key is generated atomically with account creation and displayed only once.
Save it securely. An existing imported key is preserved and is never revealed.
The running gateway notices CLI account creation without a restart.

Sign in to the console with this administrator account. Login is kept only in
JavaScript memory, never app cookies/localStorage/sessionStorage.
Reloading the browser requires signing in again. Use HTTPS for remote access;
AES encryption at rest does not secure HTTP transport or a compromised host.

Saved credentials cannot be revealed. Providers accept a new key or an existing
credential ID; advanced management tokens and headers use IDs. The Credentials
page lists metadata and supports creation, replacement and deletion. Benchmark
configuration uses the same references:

```yaml
bearer_token: {credential: "opaque-id-returned-by-the-gateway"}
# Custom headers:
headers:
  X-Custom-Key: {credential: "another-opaque-id"}
```

`GET /gateway/v1/credentials` returns metadata only. Authenticated `POST` creates,
`PUT /gateway/v1/credentials/{id}` replaces, and `DELETE` deletes. POST/PUT take
`value`, `service`, `purpose`; replacements retain existing service/purpose.
Configuration APIs never return secret values. Authenticated legacy inline
config writes are accepted and encrypted before YAML persistence; new env/file
references are rejected after migration. OAuth records are replaced through
provider login, not generic credential replacement.

Deletion stops connections using the credential. Protected system records
cannot be replaced/deleted through generic credential APIs. Generate a new
inference API key on **Server → Generate new API key** or from a terminal:

```sh
./gateway admin api-key --config ./config.yaml
podman compose exec torii-gateway /gateway admin api-key --config /config/config.yaml
# Provider credential commands also prompt for the administrator password:
./gateway credentials replace --config ./config.yaml --id STORED_ID
./gateway credentials delete --config ./config.yaml --id STORED_ID
```

Terminal API-key generation verifies the administrator password. For automation,
`admin create --username NAME` / `admin api-key` read the password from stdin,
never an environment variable or command-line flag. Provider replacement can
use `--secret-file PATH`, reserving stdin for the administrator password.
Provider `auth login|logout|status` commands also require the administrator password.
`POST /gateway/v1/admin/api-key` requires administrator Basic authentication
and returns the newly generated key once with `Cache-Control: no-store`.
There is no API to retrieve an existing key. Rotation invalidates the old key
immediately; update inference clients. Benchmark debug/refresh endpoints alone
also accept the bearer key for compatibility with the benchmark CLI.

The replacement command can restore a deliberately deleted provider record. A running
gateway sees replacements of an existing ID on its next request. Configuration
saves that select a different ID rebuild the active handler.

## Migrating an existing installation

1. Stop the gateway; back up existing YAML/state securely. Those old backups
   contain plaintext and must be protected and eventually retired.
2. Create and separately back up the master key, add the persistent database
   volume, and make every legacy env/file credential available on first start.
   The base Compose file imports an existing `TORII_API_KEY` from `.env`.
   For `JEV_API_KEY`, add `-f compose.migrate.yaml`; extend that override for
   other variables referenced in old YAML. Remove the migration override later.
3. Start the updated gateway. It transactionally imports all configured secret
   fields and `state/auth/*.json`, encrypts and verifies records, then commits.
4. Create your administrator through the terminal or setup screen, then verify
   connections. Provider YAML secrets are replaced atomically with credential IDs;
   `server.authentication` is removed entirely after its key is imported.
   successfully imported OAuth JSON files are removed. Non-sensitive YAML and
   comments are preserved. External provider secret files are not deleted.
5. Remove obsolete environment variables and external provider secret files.
   Keep the master-key source. Later changing old env/files does not update
   database credentials; use Replace or the local CLI.

Missing inputs abort the transaction before touching original configuration.
Imports are idempotent and cannot overwrite newer replacements or resurrect
deleted credentials. A crash after database commit but before YAML cleanup is
safe to retry; committed database records remain authoritative.

Read-only or aliased YAML and failed cleanup emit explicit warnings. The
gateway uses encrypted records and ignores old values, but the host files still
need manual cleanup. Retrieve sanitized configuration through authenticated
`GET /gateway/v1/config`, inspect it, and atomically replace the host YAML with
owner-only permissions. Remove leftover `state/auth/*.json` and stale plaintext
backups. No automatic plaintext backup is created. Automatic import cannot
remove plaintext already present in old Git history, backups, images or volumes;
rotate exposed provider credentials separately.

## Backup, recovery, and security boundary

For the simplest consistent backup, stop the gateway and copy `torii.db` together
with any `torii.db-wal`/`torii.db-shm` files, plus non-sensitive YAML/state. For an
online backup use SQLite's backup API or CLI `.backup`, not a live copy of the
main file alone. Store the master-key backup separately with restricted access.
Restore the database and original key together before restarting.

A lost key is unrecoverable cryptographically. Restore its separate backup.
If no backup exists, stop the application, preserve the inaccessible database,
and explicitly create a new database/key and re-enter/re-authorize credentials;
remove references to inaccessible old IDs. Torii will never do this silently.

Encryption protects stored credential payloads, not compromise of the running
gateway: it must decrypt credentials for authentication and outbound requests.
No persistent plaintext cache is introduced. Administrator passwords are hashed;
passwords needed to authenticate to an upstream service belong in the encrypted
store. Changing/resetting the administrator password, multi-user administration,
Authentik integration and automated master-key rotation are not implemented.
Keep your administrator password in a password manager; do not delete system
database records as a recovery shortcut. A lost inference API key can be replaced
using the administrator login; a lost database master key cannot.
The design follows the [OWASP Cryptographic Storage](https://cheatsheetseries.owasp.org/cheatsheets/Cryptographic_Storage_Cheat_Sheet.html)
and [Secrets Management](https://cheatsheetseries.owasp.org/cheatsheets/Secrets_Management_Cheat_Sheet.html)
guidance on authenticated encryption, access control, and separation of keys.

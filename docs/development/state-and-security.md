# State, authentication, and security

Torii encrypts credential payloads with AES-256-GCM before storing them in
SQLite. SQLite metadata is not encrypted: provider/service, purpose, owner,
timestamps and opaque IDs are readable. No SQLCipher or external service is
required. Non-sensitive configuration and its revision are stored in SQLite. Usage, the stable Codex
host ID, public benchmark observations and provenance share the database but are not
encrypted. See [benchmark storage](intelligence.md#cache-management-and-refresh).

The default database is `/data/torii.db`. Select it with `--database` or `TORII_DB_PATH` (use `./state/torii.db` for a local build). Database-path changes require a startup-option change and restart. SQLite uses WAL, a 5-second busy timeout, immediate write transactions, foreign keys and schema migrations. Taking the write lock before transactional reads avoids snapshot-upgrade failures when activity/accounting pools write concurrently. Database/WAL/SHM files are
owner-only. Mount local persistent storage, not a shared/network filesystem.


## Master key

Generate exactly 32 random bytes encoded as 64 hexadecimal characters:

```sh
umask 077
mkdir -p secrets
openssl rand -hex 32 > secrets/torii_db_key
```

Back this key up separately before deploying or updating. Torii never generates a master
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

## Authentication boundaries

Administrator Basic login is protected by bounded hashing concurrency and a
five-failed-attempts/minute/client limit. Passwords use salted PBKDF2-HMAC-SHA256
with 600,000 iterations; username/email/hash/salt are inside an encrypted record.
The stable internal owner is `default`; multiple administrators are not supported.
Browser login uses an opaque HttpOnly, SameSite=Lax cookie. Only token hashes and expiry are stored in SQLite; sessions survive restarts. Cookie mutations require a CSRF token and a matching origin. Administrator credential changes invalidate existing sessions. HTTPS sets Secure; forwarded HTTPS is trusted only for explicitly configured proxy networks.

Account changes require current-password reauthentication and a transactional
password check; all changes refresh the password salt/hash, delete sessions, and
invalidate reset links. Email is optional for backward compatibility.
Schema 10 adds one reset record storing a SHA-256 hash of a random 256-bit token,
account fingerprint, expiry, and issuance cooldown; the plaintext token is sent
only to the configured mailbox. Confirmation validates and consumes the token
in the same write transaction as the credential update, preventing replay and
concurrent reuse. Links expire after 30 minutes and are replaced by newer links.
Asynchronous requests have identical responses without waiting on email lookup or
SMTP, with bounded workers and global rate limits. SMTP requires verified TLS;
links use an explicitly configured HTTPS origin, carry tokens in a URL fragment,
and the reset page removes the fragment before submission. See [Environment](../reference/environment.md#password-reset-email-delivery).

Named client keys use random 256-bit values, encrypted records, and indexed SHA-256
digests. Authentication verifies the decrypted record; no plaintext cache.
Default rotation replaces only the default key. Deletion does not cancel already
accepted requests. Older backups can reactivate revoked keys. System credentials
are protected from generic secret APIs. Pairing codes/tokens and OAuth callback
state have separate expiring authorization boundaries.

See [API authentication](../reference/api.md#authentication) and
[credential procedures](../guides/credentials.md).


## Backup and security boundary

For the simplest consistent backup, stop the gateway and copy `torii.db` together
with any `torii.db-wal`/`torii.db-shm` files, plus optional external certificates. For an
online backup use SQLite's backup API or CLI `.backup`, not a live copy of the
main file alone. Store the master-key backup separately with restricted access.
Restore the database and original key together before restarting.

The supported baseline is SQLite schema 4; the current schema is 10. Startup
automatically adds settings, sessions, accounting, activity and password-reset tables transactionally when opening
a schema-4 database, preserving existing credentials, admin, API key and state.
No reset is required; legacy YAML settings require an explicit import before normal startup. Future schema upgrades run after
master-key verification and commit one transactional step at a time.
Rollback requires restoring the pre-upgrade database and configuration,
with their matching key and previous application binary/image.
Never manually lower `PRAGMA user_version`. See
[runtime state and releases](../guides/backup-and-upgrade.md) for the update procedure.

A lost key is unrecoverable cryptographically. Restore its separate backup.
If no backup exists, stop the application, preserve the inaccessible database,
and explicitly create a new database/key and re-enter/re-authorize credentials;
remove references to inaccessible old IDs. Torii will never do this silently.

Encryption protects stored credential payloads, not compromise of the running
gateway: it must decrypt credentials for authentication and outbound requests.
No persistent plaintext cache is introduced. Administrator passwords are hashed;
passwords needed to authenticate to an upstream service belong in the encrypted
store. Administrator profile/password changes and email password reset are
implemented; see [credential procedures](../guides/credentials.md). Multi-user
administration, Authentik integration and automated master-key rotation are
not implemented.
Keep your administrator password in a password manager; do not delete system
database records as a recovery shortcut. A lost inference API key can be replaced
using the administrator login; a lost database master key cannot.
The design follows the [OWASP Cryptographic Storage](https://cheatsheetseries.owasp.org/cheatsheets/Cryptographic_Storage_Cheat_Sheet.html)
and [Secrets Management](https://cheatsheetseries.owasp.org/cheatsheets/Secrets_Management_Cheat_Sheet.html)
guidance on authenticated encryption, access control, and separation of keys.

## Background usage persistence

Requests update in-memory totals immediately. One worker flushes dirty models
every second through an independent SQLite pool in batches of at most 250.
Retries upsert absolute totals, avoiding double counting; concurrent updates
remain pending. Configuration reloads share the worker.

Normal crash loss is approximately one second plus flush time. Database failures
can extend this window; pending totals are retained with rate-limited
`usage_flush_failed` warnings. Shutdown drains HTTP requests and attempts a final
flush with a separate ten-second deadline. Failed final writes report
`usage_shutdown_flush_failed`. This is routing accounting, not an audit-grade
billing ledger. JSON and SSE also feed a separate durable reservation/settlement ledger. Budgets fail closed on storage errors; interrupted or unreported usage keeps conservative reservations. Activity retains metadata by default, with optional prompt/Jev body capture
under the same bounded retention and storage limits; see
[Activity configuration](../reference/configuration.md#activity-and-structured-logs).

## Maintaining future releases

Add one immutable migration per future schema version to the centralized
registry in `internal/state/schema.go`; increment the current version while
keeping baseline 4. Fresh initialization creates baseline 4 and runs the same
upgrade path as existing installations. Add fixtures for every supported
upgrade path, transactional failure/retry, credential preservation and restart.
Do not reuse or silently change previously released migration versions.

Tag releases with Semantic Versioning. Every release must document supported
schemas, configuration changes, prerequisites and rollback limitations. Release
automation gates artifacts on Go/race/console tests, vet, formatting, database
compatibility coverage and an isolated non-root container persistence/recovery
test. It publishes gateway/helper binaries, SHA256SUMS, and versioned images.
Publication occurs only after all validation and artifact builds succeed.

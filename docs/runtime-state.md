# Database baseline, updates and recovery

Torii's database-only baseline is SQLite schema **4**. Fresh installations
create this schema directly. Schemas 1–3 and nonempty unversioned databases are
rejected without an automatic import, deletion or reset. Future releases retain
schema 4 as their supported starting point and preserve data through sequential,
transactional database migrations. Unsupported newer schemas are also rejected.

All durable application state uses `state.database_path`, default
`/data/torii.db`: encrypted administrator/credential/API-key/OAuth records,
public benchmark observations and provenance, usage aggregates and Codex host
identity. Nonsensitive configuration remains in YAML; the master key stays
outside the database. Sessions, live quota caches and pending login flows remain
memory-only. There is no `state.directory` or flat-file credential/cache importer.

## One-time baseline reset

This is the only planned reset. It deliberately does **not** preserve old
credentials, OAuth sessions, usage totals, benchmark caches or Codex identity.
The previous installation remains recoverable from its archived storage and
matching binary/key. Torii never performs this reset automatically.

1. Stop the old gateway gracefully; do not use `compose down -v`.
2. Archive its configuration and complete stopped data volume/directory,
   including any WAL/SHM files. Back up its matching master key separately.
   Protect old plaintext files/backups as sensitive material.
3. Provision a **new, empty database location/volume** and fresh configuration.
   Do not point the new release at the old database. Do not copy old credential
   IDs, `server.authentication`, `state.directory`, or YAML value/env/file keys.
4. Containers can start without an initial configuration. One approach is to
   retain the old named volumes and launch a new Compose project after moving
   the old `config/` directory to a verified, unused archive path:

   ```sh
   podman compose down
   # After backups: archive the old config directory without overwriting anything.
   test ! -e ./config.pre-baseline
   mv ./config ./config.pre-baseline
   mkdir -p config
   podman compose -p torii-baseline up --build -d
   ```

   This creates a new project-scoped database volume and retains the original.
   Use this same project name for every subsequent update. Append secret/Docker
   overrides as appropriate. If no old config directory exists, omit its move.
   For local builds, archive the old database directory and use a new database
   path with fresh YAML copied from the example.
5. Keep your existing master key, or provision a new key only for this empty
   database. Retain the old key with the old backup if you choose a new one.
6. Create an administrator through terminal setup or the console's one-time
   setup code. Save the generated API key: it is displayed only once. Update
   inference clients, re-enter provider credentials, and sign in to subscriptions.
   Refresh benchmarks rather than importing old JSON.
7. Verify console login, model discovery, inference and benchmark status before
   separately retiring old storage. Torii does not delete any legacy files.

`TORII_API_KEY` is no longer read or passed through Compose. The master-key
source and published port are the only required environment settings; database
path and image overrides remain optional. Credential entry and OAuth helper
transfer remain supported normal operations, not legacy migration features.

## Future manual updates

Application release versions, SQLite schema versions and encryption key versions
are separate. `gateway version` reports the application build and supported
schema range without opening a database or requiring its master key.

1. Read release notes for schema/configuration changes and prerequisites.
2. Record the current binary version or exact container tag/digest, Compose
   project name and volume identities.
3. Stop gracefully and make a coherent backup of the **entire stopped data
   directory/volume**, plus configuration. Preserve any WAL/SHM files together.
   Back up the master key separately with restrictive access. Never copy only
   the main database while Torii is running.
4. Verify checksums when downloading a gateway binary. Retain the previous
   binary. Container releases are `ghcr.io/apartment38/torii-gateway:vX.Y.Z`,
   for Linux amd64/arm64; pin a real version or its published digest.
5. Replace the binary/image, preserving configuration, database volume and
   master key. For a prebuilt Podman image:

   ```sh
   TORII_IMAGE=ghcr.io/apartment38/torii-gateway:vX.Y.Z podman compose pull torii-gateway
   TORII_IMAGE=ghcr.io/apartment38/torii-gateway:vX.Y.Z podman compose up --no-build -d
   podman compose ps
   podman compose logs --tail=100 torii-gateway
   ```

   Replace the example version with a published release. Supply the same
   `-p` and `-f` options as the existing deployment. Docker users use
   `compose.docker.yaml`. Source-built deployments use `up --build -d`
   after selecting the release's source tag.
6. Startup verifies encrypted records with the original master key before
   applying database upgrades. Each step commits its changes and schema version
   together. Missing migration steps fail before any upgrade writes; a failed
   step rolls back and stops startup. Earlier successful steps remain committed.
7. Verify health, administrator login, model discovery, inference, credentials,
   benchmark status and usage totals. Keep the backup through verification.

No automatic downloader, installer or schema downgrade is introduced.
A failed upgrade never triggers database recreation. Lost master keys require
their separate backup; no key replacement can decrypt existing credentials.

## Rollback

Stop Torii. Restore the complete pre-upgrade database directory/volume and
configuration, including their original master key, then run the previous
binary/image. Do not overlay a backup onto newer WAL/SHM files. Preserve the
failed-upgrade dataset separately before restoration. Rollback restores usage,
credentials and API keys to their backup values; update clients if necessary.
Never manually lower `PRAGMA user_version`. See
[credential backup and recovery](credential-storage.md#backup-recovery-and-security-boundary).

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
billing ledger. Streamed responses still do not contribute usage counters.

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

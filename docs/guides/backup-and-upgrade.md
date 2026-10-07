# Back up, upgrade, and restore Torii

Preserve your configuration, complete database storage, and original master key
when updating. These instructions apply to one gateway process with local
persistent storage. Keep the master-key backup separate from the data backup.

## Make a stopped backup

1. Record `gateway version`, your binary/image tag or digest, and Compose project
   name and override files. Keep the previous binary/image for rollback.
2. Stop Torii gracefully (Ctrl+C, your service manager, or Compose `down` without `-v`).
3. Copy the entire data directory, including any `torii.db-wal` and
   `torii.db-shm` files, and the configuration into a protected backup location.
4. Back up the original master key separately. Confirm that both backups are readable.

For the native layout, after stopping and choosing an unused backup directory:

```sh
umask 077
mkdir -p backups/native-before-upgrade
cp -a state config.yaml backups/native-before-upgrade/
```

For container storage, inspect the actual volume identity (Compose prefixes names
with its project). With Podman, after stopping:

```sh
podman volume ls
# Replace ACTUAL_VOLUME_NAME and choose an unused protected destination.
podman volume export ACTUAL_VOLUME_NAME --output /safe/backup/torii-data.tar
```

For Docker, after stopping, replace the volume name and absolute backup path:

```sh
docker run --rm -v ACTUAL_VOLUME_NAME:/data:ro -v /safe/backup:/backup alpine:3.22 tar -C /data -cf /backup/torii-data.tar .
```

The utility image may need network access. Copy the host `config/` directory
separately and preserve ownership. Do not copy only the main SQLite file while
Torii is running. Advanced online backups require SQLite's backup API or `.backup`.

## Supported databases

Torii's database-only baseline is SQLite schema **4**; the current schema is
**5**. Fresh installations create the baseline and apply the registered upgrade.
Schema 4 upgrades automatically, preserving all data and adding named client-key
metadata. Schemas 1–3 and nonempty unversioned databases are
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

This is the explicit reset for pre-baseline installations. It deliberately does **not** preserve old
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

## Upgrade an existing installation

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

### Source-built native binary

After the stopped backup, select the source version, build a replacement, and
validate config before replacing the binary:

```sh
git pull --ff-only
go build -trimpath -o ./bin/gateway.new ./cmd/gateway
./bin/gateway.new config validate --config ./config.yaml
cp ./bin/gateway ./bin/gateway.previous
mv ./bin/gateway.new ./bin/gateway
./bin/gateway serve --config ./config.yaml
```

Use the same master-key source and working directory. Restart a systemd-managed
installation with `sudo systemctl restart torii-gateway.service` instead.
For release downloads, verify `SHA256SUMS` before replacement; see [Native installation](../installation/native.md).

### Source-built Compose image

After backup, pull/select the intended source version and rebuild with the same
project, volumes, `.env`, and override files. For the file-secret Podman layout:

```sh
git pull --ff-only
podman compose -f compose.yaml -f compose.secrets.yaml up --build -d
podman compose -f compose.yaml -f compose.secrets.yaml ps
podman compose -f compose.yaml -f compose.secrets.yaml logs --tail=100 torii-gateway
```

Docker users include `-f compose.docker.yaml`. Do not replace the saved config
with an example during a normal upgrade. Rebuilding is needed for code changes;
console config changes apply through save/reload.

## Rollback

Stop Torii. Restore the complete pre-upgrade database directory/volume and
configuration, including their original master key, then run the previous
binary/image. Do not overlay a backup onto newer WAL/SHM files. Preserve the
failed-upgrade dataset separately before restoration. Rollback restores usage,
credentials and API keys to their backup values; update clients if necessary.
Never manually lower `PRAGMA user_version`. See
[credential backup and recovery](credentials.md#recover-access).

Restore into an empty target location after preserving the failed-upgrade data;
never overlay the backup onto newer WAL/SHM files. Podman can import a stopped
volume with `podman volume import ACTUAL_VOLUME_NAME /safe/backup/torii-data.tar`.
For Docker use a utility container to extract the archive into an empty mounted
volume, preserving UID/GID 65532. Restore the matching config and key, start the
previous binary/image, and verify health, login, model discovery, and inference.

# SQLite runtime state and migration

All normal durable state uses `state.database_path` (default `/data/torii.db`):
encrypted credentials/admin records/API keys, public benchmark observations,
per-model usage aggregates and the stable Codex host ID. Configuration stays in
YAML and the master key stays outside the database. Sessions, live quota caches
and pending login flows remain memory-only.

SQLite schema version 3 adds `usage_aggregates` and `application_settings`.
Counters use decimal-text columns to preserve the full unsigned 64-bit range;
cost uses a finite non-negative number. Usage and host identity are not secrets
and are not encrypted. Existing credential ciphertext and benchmark data do not
change during the upgrade, which runs only after master-key verification.

## Background usage persistence

Requests update a shared in-memory aggregate immediately. A single worker flushes
dirty model totals every second, using an independent SQLite pool and batches of
at most 250 models. Retries upsert absolute totals, not increments, preventing
double counting. Updates arriving during a flush remain pending for the next
batch. Configuration reloads share the same store and worker.

Normal crash loss is approximately one second of usage, plus flush time. A busy
or unavailable database can extend this window: failed writes retain pending
totals and produce rate-limited `usage_flush_failed` warnings. They do not delay
inference responses. Shutdown first drains HTTP requests, then stops the worker
and attempts a final flush with a separate 10-second deadline; an unsuccessful
flush emits `usage_shutdown_flush_failed` and returns an error. Forced termination
cannot guarantee a final flush. This is routing accounting, not an audit-grade
billing ledger. Current non-streaming usage collection behavior is unchanged;
streamed responses still do not contribute usage counters.

## Existing installations: migrate before the first upgrade boot

The new default container configuration mounts only `/data` and `/config`.
It does **not** automatically find an old `/state` volume. Without that volume,
Torii cannot import its counters or host identity. Do not start the new image
without migration mounts if you need those old values preserved.

1. Stop the gateway without deleting volumes (`podman compose down`, **not**
   `down -v`). Back up the database/WAL/SHM, configuration and original state
   volume; back up the original master key separately. See
   [consistent backups](credential-storage.md#backup-recovery-and-security-boundary).
2. Keep `state.directory: /state` in the old YAML and use the same Compose project
   name/directory as before. Verify the original volume with `podman volume ls`;
   normally its name is `<project>_torii_gateway_state`. If the project name
   changed, point the migration override's volume at the original name using
   `external: true` and `name: ORIGINAL_VOLUME_NAME`. Files must be readable by
   the container's UID/GID 65532; legacy OAuth directories also need that ownership
   for permission checks and cleanup. Resolve ownership only on that specific volume,
   never recursively on the repository.
3. Start with the migration override:

   ```sh
   podman compose -f compose.yaml -f compose.migrate.yaml up --build -d
   ```

   Append `-f compose.secrets.yaml` if you use file secrets. For Docker, use
   `docker compose -f compose.yaml -f compose.docker.yaml -f compose.migrate.yaml`
   with the same `up --build -d` command (and secret override if applicable).
   Extend the migration override for any additional legacy credential environment
   variables your YAML references.
4. Verify the imports. Usage logs `usage_legacy_import_succeeded`; the public host
   ID is visible through an authenticated Codex helper login session. Alternatively,
   inspect a consistent SQLite backup using:

   ```sql
   SELECT model_id, requests, input_tokens, output_tokens, estimated_cost FROM usage_aggregates;
   SELECT value FROM application_settings WHERE key = 'codex_host_id';
   ```

   Enable benchmark intelligence while the old volume remains mounted if its
   legacy JSON also needs importing, then inspect the management API. Existing
   OAuth JSON credentials migrate into encrypted records as before.
5. Imported usage and host-ID files remain unchanged for recovery. Established
   SQLite state always wins on later starts, preventing stale files from resetting
   counters or identity. Invalid/unreadable usage or host-ID files stop startup
   with actionable errors; repair them and retry. No partial usage import or
   completion marker is committed on failure. Missing files initialize empty
   usage/new identity. A new host ID is preserved across logout and restart.
6. Before dropping `/state`, move any operator-managed files referenced by
   relative paths (such as catalog overrides) into `/config` or another deliberate
   mount and update their YAML paths to absolute paths. The default container
   working directory is now `/data`; the migration override preserves `/state`.
7. Remove `state.directory` from YAML, stop the migration deployment, then recreate
   with the base Compose file and your normal secret/Docker overrides. Do not
   delete the old volume until verification and backups are complete. Retire old
   plaintext credential backups securely after migration.

Local builds use the same migration: retain the old `state.directory` for the
first start, keep the original database/key, verify imports, then remove the
directory setting. It is optional and migration-only; an omitted value never
searches the process working directory. Changing it through the console does not
re-import established usage; migration is a startup operation.

## Rollback

Older binaries reject schema version 3. Stop Torii and restore the pre-upgrade
database with its matching key, configuration and legacy state volume before
starting an older binary/image. This also rolls usage, credentials and key changes
back to the backup point. Never manually lower `PRAGMA user_version`. Torii does
not automatically overwrite or downgrade databases.

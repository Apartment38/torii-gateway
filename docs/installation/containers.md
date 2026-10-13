# Run Torii in a container

These instructions build the repository's production image and use Compose to
keep configuration and data when the container is recreated. You need Git,
OpenSSL, and either Docker with Compose or Podman with a working Compose provider.
Run commands on the container host in the repository directory.

A **volume** is persistent storage independent of the container. The Compose
service mounts a project-scoped named database volume at `/data`. The image runs as user/group 65532, so its mounted files must be
accessible to that user. It has no shell; use its `/gateway` commands for checks.

## Prepare a new installation

```sh
git clone https://github.com/Apartment38/torii-gateway.git
cd torii-gateway
umask 077
mkdir -p secrets
openssl rand -hex 32 > secrets/torii_db_key
```

Generate this key once and back it up separately. Do not overwrite an existing
key. Fresh databases use settings defaults listening at `0.0.0.0:8080` with no providers.
Existing YAML installations require [explicit import](../guides/backup-and-upgrade.md#legacy-configuration-import).

## Podman Compose

On a rootless Linux Podman host, map ownership of the dedicated key file:

```sh
podman unshare chown 65532:65532 secrets/torii_db_key
podman compose -f compose.yaml -f compose.secrets.yaml up --build -d
podman compose -f compose.yaml -f compose.secrets.yaml ps
podman compose -f compose.yaml -f compose.secrets.yaml logs --tail=100 torii-gateway
```

The database volume uses private SELinux labels. Rootful
Podman uses host ownership (`sudo chown 65532:65532 secrets/torii_db_key`) instead
of `podman unshare`. Podman machine users should verify permissions inside the VM
if the host filesystem does not honor Linux ownership.

## Docker Compose

On a Linux Docker host, prepare the dedicated secret file:

```sh
sudo chown 65532:65532 secrets/torii_db_key
docker compose -f compose.yaml -f compose.secrets.yaml up --build -d
docker compose -f compose.yaml -f compose.secrets.yaml ps
docker compose -f compose.yaml -f compose.secrets.yaml logs --tail=100 torii-gateway
```

On Docker Desktop, host ownership behaves differently. Ensure the database volume is writable and the secret readable by the image's user; do
not change ownership of the repository. If file-secret permissions cannot be
mapped, use the environment-key alternative below. Compose file secrets are
mounted files, not an encrypted host secret vault.

## Finish setup

1. Open <http://127.0.0.1:8080/> on the container host.
2. Enter the unused setup code from the logs and choose your administrator
   username/password (at least 12 characters). The code expires in 30 minutes.
3. Save the generated inference API key; it is displayed once.
4. Sign in, open **Providers**, choose **Add provider**, and follow
   [Getting Started](../getting-started.md). The empty-state notice remains
   until you add a provider.
5. Check health:

   ```sh
   curl --fail http://127.0.0.1:8080/healthz
   ```

Use the same Compose project name and `-f` files on subsequent commands. To stop
Podman, for example:

```sh
podman compose -f compose.yaml -f compose.secrets.yaml down
```

Keep the database volume. Never add `-v` during an ordinary stop or update.
Restart with the same `up` command and key. See [Backup and upgrade](../guides/backup-and-upgrade.md).

## Ports, images, and alternate key sources

To publish port 8081, add `TORII_PORT=8081` to a private project-root `.env` file.
The container still listens on port 8080. The base Compose file publishes to all
host interfaces; restrict access with the host firewall or override the port
mapping to `127.0.0.1:8081:8080` for local-only access. Use HTTPS for remote login.

`TORII_IMAGE` selects an image. For a published release, obtain its actual tag or
digest from the [release page](https://github.com/Apartment38/torii-gateway/releases),
set it in `.env`, then use `pull` and `up --no-build --force-recreate -d` with your usual overrides. Verify the
running container image ID, especially when reusing an image tag.
The source-build flow above works without a published image. See [Environment](../reference/environment.md).

If mounted secrets are impractical, place `TORII_DB_KEY=YOUR_EXISTING_64_HEX_KEY`
in a private `.env` file and start without `compose.secrets.yaml`. Use your existing key value, not a newly generated
one. Privileged container operators can inspect environment values; keep `.env`
out of source control and backups containing publicly shared files.

The secret source defaults to `./secrets/torii_db_key`; set `TORII_DB_KEY_SOURCE`
to use another file. Native Podman secrets are another option:

```sh
podman secret create torii_db_key secrets/torii_db_key
```

A custom `podman run` deployment must mount a writable data volume, set
`TORII_DB_KEY_FILE=/run/secrets/torii_db_key`, publish port 8080, and mount the
secret with `--secret torii_db_key,type=mount,target=torii_db_key,uid=65532,gid=65532,mode=0400`.
Docker Swarm secrets require their own service secret declarations; local Compose
does not automatically consume Swarm secrets.

## Connect to Ollama on the host

Inside a container, `127.0.0.1` refers to that container. Use the host gateway
alias supplied by this Compose file:

```yaml
# Provider excerpt: merge into your providers list.
providers:
  - id: local
    type: ollama
    base_url: http://host.containers.internal:11434
```

Ollama must listen on a host-reachable interface. For systemd-managed Ollama,
run `sudo systemctl edit ollama` and add:

```ini
[Service]
Environment=OLLAMA_HOST=0.0.0.0:11434
```

Then restart with `sudo systemctl daemon-reload` and `sudo systemctl restart ollama`.
Restrict Ollama's exposed port to trusted clients with your firewall. Check the
host service with `curl --fail http://127.0.0.1:11434/api/tags`, then test the
provider in Torii. Docker Desktop/Podman machine networking varies by host;
if the alias does not resolve, use the runtime's host-gateway address (for
example `host.docker.internal` on Docker Desktop) and test connectivity.

## Preserve a root-level configuration

If an older Compose deployment reads root-level `config.yaml`, stop it first,
back up its config/data/key, then copy the file into the dedicated mount:

```sh
mkdir -p config
cp config.yaml config/config.yaml
```

This copy preserves the old file; current Torii does not mount or load it at
startup. Review obsolete fields using [Configuration](../reference/configuration.md),
then follow [Legacy configuration import](../guides/backup-and-upgrade.md#legacy-configuration-import)
to import it explicitly into SQLite before starting. External CA files still
need readable mounts. Schemas 1–3 need the separate
[baseline reset](../guides/backup-and-upgrade.md#one-time-baseline-reset).

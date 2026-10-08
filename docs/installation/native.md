# Run Torii natively

This guide starts Torii on your computer with an empty provider list, then sends
you to the provider setup tutorial. You need a built gateway from
[Compile from source](from-source.md), or a compatible published release binary.

## Use a release binary

Open the [repository releases](https://github.com/Apartment38/torii-gateway/releases).
If a release is available, download the `torii-gateway-OS-ARCH` binary
(`.exe` on Windows), `SHA256SUMS`, and optionally `torii-auth-OS-ARCH`.
Choose the architecture of your computer, verify the downloaded binary's hash
against the release checksums, and retain its release notes. If assets are not
available, build from source. Examples below call the gateway `./bin/gateway`.

On Linux, with downloaded assets in the current directory:

```sh
sha256sum --ignore-missing -c SHA256SUMS
mkdir -p bin
chmod 0755 torii-gateway-linux-amd64
cp torii-gateway-linux-amd64 bin/gateway
./bin/gateway version
```

On macOS use `shasum -a 256 FILE` and compare the result with its line in
`SHA256SUMS`, then copy your matching download to `bin/gateway` and run
`chmod 0755 bin/gateway`. On Windows use `Get-FileHash FILE -Algorithm SHA256`,
then prepare the binary (example for Windows amd64):

```powershell
New-Item -ItemType Directory -Force bin | Out-Null
Copy-Item torii-gateway-windows-amd64.exe ./bin/gateway.exe
./bin/gateway.exe version
```

Replace `FILE` with your download's filename. Do not proceed with a mismatch.

## Configure and start on Linux or macOS

Run from the repository directory, or another dedicated installation directory
containing `bin/gateway`. Generate the key only when creating a new database.

1. Create private storage and a master key:

   ```sh
   umask 077
   mkdir -p state secrets
   openssl rand -hex 32 > secrets/torii_db_key
   ```

   Back up `secrets/torii_db_key` separately now. Losing it prevents Torii from
   decrypting your saved credentials. Reuse this file on every restart.

2. Start with a new database and an explicit local listener:

   ```sh
   export TORII_DB_KEY_FILE="$PWD/secrets/torii_db_key"
   ./bin/gateway serve --database ./state/torii.db --listen 127.0.0.1:8080
   ```

   Fresh databases have no providers. Settings are saved in SQLite automatically.
   For an existing YAML installation, follow [the explicit import procedure](../guides/backup-and-upgrade.md#legacy-configuration-import) before starting.

3. Choose an administrator username and a password of at least 12 characters.
   Save the newly displayed API key immediately. In a noninteractive launch,
   the terminal output instead contains a 30-minute setup code for the web console.
4. Open <http://127.0.0.1:8080/> and continue with [Getting Started](../getting-started.md).

Stop with Ctrl+C. Restart with the same database, key file, and listener options.

## Configure and start on Windows

Run these PowerShell commands from your installation directory, with the binary
at `bin/gateway.exe`. Use a dedicated folder protected by your Windows account
permissions; restrict access to the `secrets` and `state` directories.

```powershell
New-Item -ItemType Directory -Force state, secrets | Out-Null
$toriiKeyBytes = New-Object byte[] 32
$toriiRng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
$toriiRng.GetBytes($toriiKeyBytes)
$toriiRng.Dispose()
$toriiKeyHex = ([BitConverter]::ToString($toriiKeyBytes)).Replace('-', '').ToLowerInvariant()
[IO.File]::WriteAllText("$PWD/secrets/torii_db_key", $toriiKeyHex)
$env:TORII_DB_KEY_FILE = "$PWD/secrets/torii_db_key"
./bin/gateway.exe serve --database ./state/torii.db --listen 127.0.0.1:8080
```

Back up the key separately before setup. Do not rerun key generation for an
existing database. Use `curl.exe` for the HTTP examples to avoid PowerShell's
legacy `curl` alias. See [Clients](../guides/clients.md) for a PowerShell request.

## Run as a service

First finish interactive administrator/provider setup. On Linux, this systemd
example assumes installation at `/opt/torii`, owned by a dedicated `torii` user,
including writable `state/`. Adjust paths and
ownership to your installation; the repository does not install a service for you.

```ini
[Unit]
Description=Torii Gateway
After=network-online.target
Wants=network-online.target

[Service]
User=torii
Group=torii
WorkingDirectory=/opt/torii
Environment=TORII_DB_KEY_FILE=/opt/torii/secrets/torii_db_key
ExecStart=/opt/torii/bin/gateway serve --database /opt/torii/state/torii.db --listen 127.0.0.1:8080
Restart=on-failure
UMask=0077

[Install]
WantedBy=multi-user.target
```

Save as `/etc/systemd/system/torii-gateway.service`, then run:

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now torii-gateway.service
sudo systemctl status torii-gateway.service
journalctl -u torii-gateway.service -n 100
```

Service managers on macOS/Windows must likewise preserve the working directory,
key source, and data paths. Review [Operations](../guides/operations.md) before
exposing Torii beyond your computer, and [Backup and upgrade](../guides/backup-and-upgrade.md)
before replacing the binary.

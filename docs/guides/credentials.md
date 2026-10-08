# Manage credentials and client keys

Use your administrator login to create keys, revoke client access, and replace
provider credentials. Torii stores secret payloads encrypted in SQLite and never
returns saved provider keys to the browser.

## Understand the three kinds of keys

| Secret | Purpose | Where to use it |
| --- | --- | --- |
| Database master key | Decrypt Torii's saved credentials | Key file/environment on gateway host |
| Gateway API key | Authorize requests to Torii | Your AI client or Bearer header |
| Upstream provider/source key | Authorize Torii to an external service | Provider or benchmark settings |

Keep the master key unchanged for an existing database and back it up separately.
[Native](../installation/native.md) and [container](../installation/containers.md)
installation explain creation and file permissions. See [Environment](../reference/environment.md)
for precedence. There is no startup `TORII_API_KEY` setting.

## Create the administrator

Interactive `serve` prompts for a username/password. Unattended launches print
a 30-minute setup code used in the console. Alternatively, run:

```sh
./bin/gateway admin create --database ./state/torii.db
```

This prompts for a password of at least 12 characters and shows a new gateway key
once. It cannot replace an existing administrator. The running gateway notices
local administrator creation without a restart. Browser login uses an HttpOnly cookie session; reload and gateway restarts retain login until expiry. Use HTTPS for remote access.

## Give each client its own key

1. Sign in and open **Credentials**.
2. Click **Add credential**, enter the client name, and generate its key.
3. Copy the one-time value into your client's API-key setting before closing.
4. To revoke access, delete that named key. Subsequent requests using it fail;
   already-authorized requests are not cancelled.

The list displays names and timestamps, not retrievable key values. Default and
named keys can all be deleted without affecting administrator login. Restoring
an older backup can reactivate keys revoked after that backup; revoke them again.

## Replace a lost or compromised default API key

Use **Credentials → Generate new API key**, or run:

```sh
./bin/gateway admin api-key --database ./state/torii.db
```

Enter the administrator password, save the displayed new key, and update clients
using the default key. The previous default key stops working immediately.
Named client keys and upstream credentials are unaffected. There is no API to
retrieve an existing gateway key.

For Podman using file secrets, the equivalent command is:

```sh
podman compose -f compose.yaml -f compose.secrets.yaml exec torii-gateway /gateway admin api-key --database /data/torii.db
```

Docker users use their usual base, Docker, and secret override files.

## Save or replace provider secrets

Enter new provider keys on **Providers**. A masked saved value means Torii retains
the encrypted credential reference. A reveal control shows only a newly entered
value. Advanced custom headers and management tokens refer to stored credential
IDs; benchmark keys belong on **Benchmarks**, not in the gateway client key list.

Advanced operators can use the administrator credential API or local commands:

```sh
./bin/gateway credentials replace --database ./state/torii.db --id STORED_ID
./bin/gateway credentials delete --database ./state/torii.db --id STORED_ID
```

Replace `STORED_ID` with an actual credential ID. Commands verify the admin
password; replacement prompts for a secret or accepts `--secret-file PATH`.
Replacing an existing ID is observed on the next request. Changing to a different
ID requires saving/reloading config. Deletion prevents connections using that
secret. Protected system/OAuth/client-key records cannot be modified through
this generic credential interface; use provider login for OAuth replacement.

Persisted settings store only references such as `bearer_token: {credential: ID}`.
Literal `value`, `env`, and `file` provider secrets in a saved file are rejected.
Transient authenticated console submissions are encrypted before persistence.
See [API](../reference/api.md) and [CLI](../reference/cli.md) for advanced management.

## Recover access

A lost inference key can be rotated with administrator login. A lost master key
requires its original backup; a new key cannot decrypt an old database. If no
backup exists, preserve the inaccessible database and explicitly create a new
installation with new credentials. Follow [Backup and upgrade](backup-and-upgrade.md).
Administrator password reset, multiple administrators, Authentik/OIDC, and
automatic master-key rotation are not implemented. Keep the administrator
password in a password manager. Technical details are in [State and security](../development/state-and-security.md).

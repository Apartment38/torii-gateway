# Configure Torii in the console

Use the console to configure providers, routing, credentials, and benchmarks
without manually editing YAML. You need a running gateway and its administrator
username/password. Open its root URL, for example <http://127.0.0.1:8080/>.

## Find the right page

| Page | Purpose |
| --- | --- |
| Overview | Inspect the current gateway and model summary |
| Server | Listener and database path settings |
| Providers | Add/test inference and classifier providers; subscription login |
| Routing | Smart routing, classifier selection/tuning, session failure policy |
| Benchmarks | Enable sources, enter source keys, refresh and inspect scores |
| Credentials | Create/revoke client API keys and rotate the default key |
| State & Quota | Inspect usage and allowance/quota state |

The console supports light and dark themes. It loads the active configuration
when you sign in. Static assets are public, but configuration and credential
management require administrator authentication.

## Apply changes

1. Edit the relevant page. A newly typed secret remains in the browser's memory
   until submitted; saved secrets are never returned to the browser.
2. Click **Save & reload**.
3. Wait for the save and active-configuration reload to finish, then check your
   provider/model state. Do not treat a temporary checking label as a settled result.

Torii validates the complete document, initializes a replacement handler, and
atomically replaces the YAML file. Failure leaves the active configuration in
place. A config directory mount must be writable because saves create a temporary
file alongside the YAML before renaming it. A read-only single-file mount prevents saves.

Changing the listener or database path needs a restart; the save API rejects
live changes to these settings. Edit the stopped installation's YAML, then restart
with its original master key and correct data location. [Configuration](../reference/configuration.md)
describes all fields.

Browser reload/sign-out loses unsaved drafts. Administrator login is held only
in JavaScript memory, so a browser reload requires signing in again. Saved
classifier tuning and advanced source settings survive configuration saves.
For first-time setup, follow [Getting Started](../getting-started.md).

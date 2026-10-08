# Configure Torii in the console

Use the console to configure providers, routing, credentials, and benchmarks
without manually editing YAML. You need a running gateway and its administrator
username/password. Open its root URL, for example <http://127.0.0.1:8080/>.

## Find the right page

| Page | Purpose |
| --- | --- |
| Overview | Inspect the current gateway and model summary |
| Server | Listener setting and database bootstrap information |
| Providers | Add/test inference and classifier providers; subscription login |
| Models | Enable models and inspect capabilities, pricing and benchmark freshness |
| Smart routing | Smart routing, classifier selection/tuning, session failure policy |
| Benchmarks | Enable sources, enter source keys, refresh and inspect scores |
| API keys | Create/revoke client API keys and rotate the default key |
| Activity | Recent metadata-only request events and live routing/completion updates |
| Usage & quota | Inspect usage and allowance/quota state |

The overview links live provider, enabled-model, current UTC request, and routing summaries to their detailed pages. It shows current UTC token distribution, recent completed request metadata, and copyable client base URL/model settings. The model catalog supports search, provider and enablement filters; **Inspect metadata** expands full capabilities and benchmark provenance. **Activity** defaults to completed requests; choose **All events** to inspect request stages. **Usage & quota** shows the chosen client scope and UTC period without combining daily and monthly totals. View filters change only the display. Budget limits and advanced settings expand below the usage summary.

The console supports light and dark themes. It loads the active configuration
when you sign in. Static assets are public, but configuration and credential
management require administrator authentication.

## Apply changes

1. Complete a field edit or toggle a setting; it saves automatically. Provider Add/Edit dialogs save when you click **Save provider**. Cancel discards that dialog's changes.
2. Wait for **Saving…** to become **Saved**, then check provider/model state.
3. A validation error preserves your draft. Correct it and retry. A conflict means another tab saved first: copy your edits, refresh configuration, and reconcile them before saving.

Torii validates a replacement runtime and atomically commits settings and submitted encrypted credentials to SQLite. Failed changes leave saved and active state untouched. The console serializes saves and never silently retries a conflicting revision. Ongoing requests continue on their original runtime.

Listener changes require restart. Database paths and master-key inputs are bootstrap CLI/environment options. [Configuration](../reference/configuration.md) describes all fields.

Session expiry preserves drafts for reauthentication. Browser reload/sign-out loses unsaved drafts. Administrator login uses an HttpOnly, SameSite=Lax cookie with a seven-day default expiry. It survives browser reloads and gateway restarts. Sign out revokes the session. Saved
classifier tuning and advanced source settings survive configuration saves.
For first-time setup, follow [Getting Started](../getting-started.md).

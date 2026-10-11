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
| Models | Enable models and inspect pricing, usage, provider quota resets and benchmark freshness |
| Smart routing | Smart routing, classifier selection/tuning, session failure policy |
| Benchmarks | Enable sources, enter source keys, refresh and inspect scores |
| API keys | Create/revoke client API keys and rotate the default key |
| Activity | Recent metadata-only request events and live routing/completion updates |
| Usage & quota | Inspect usage and allowance/quota state |

The overview leads with current UTC requests, processed tokens, and estimated cost, plus success among the bounded recent request sample. Provider, enabled-model, and routing context link to their detailed pages. It shows current UTC token distribution, recent completed request metadata, and copyable client base URL/model settings. The model catalog supports search, provider and enablement filters; **Inspect metadata** expands full capabilities and benchmark provenance. **Activity** defaults to completed requests; choose **All events** to inspect request stages. **Usage & quota** shows the chosen client scope and UTC period without combining daily and monthly totals. View filters change only the display. Budget limits and advanced settings expand below the usage summary.

The Overview client model selector includes token prices, provider allowance/reset
information, and this month's gateway usage. **Models** shows the same tracking
and expands provider-reported spend under **Inspect metadata**. **Usage & quota**
keeps **Provider balances & subscription limits** separate from operator budgets.
Dollar balances, token/request allowances and subscription percentages retain
their units; reset times use your browser's local time. “Quota unknown” and
“Usage unavailable” mean data was not reported or could not be read, not zero.
Portal dollar balances and some Codex credential types remain unknown; see
[Provider support](../reference/providers.md#costs-usage-and-remaining-allowances).

Models and Activity support search, filters, and pagination; mobile views start
with five rows. The overview period selector switches between the current UTC day
and month. Benchmark scorecards keep full measurements in **Inspect** dialogs.

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

## Custom smart routers

Open **Smart routing** and choose **Create smart router**. Enter a unique model
name, then select providers to include all their models or select individual
models across providers. These selections add together. Choose **Save router**;
validated settings save automatically. The custom name is available in client
model discovery and the overview's client model selector. Use **Edit** to change
its pool and **Delete** to remove it.

**Allow paid** and **Allow free** both start enabled for the built-in router and
each new custom router. Turn either off to exclude that category. Unknown prices
require **Allow paid**. Each custom router has its own pricing choices; classifier
and advanced selection settings are shared. Current enablement, capabilities,
quotas, and session safety rules still apply. See the
[configuration reference](../reference/configuration.md#custom-smart-routers)
for pool semantics and import fields.

Provider editors include optional tracking credentials, billing team ID, mode
and endpoint overrides. Stored token values remain hidden. Model selectors show
native-currency balances, subscription/reset windows and banked Codex resets,
alongside model prices and gateway usage. Available monthly provider spend/token
usage is labeled with its account or organization scope. Inspect metadata for price source,
provider-reported organization usage and partial-read reasons. Public Zen wallet
balance and Standard Compute budget amounts remain unknown unless their service
returns them through an explicitly configured tracking endpoint; available reset
headers still appear. See [provider tracking](../reference/configuration.md#provider-tracking).

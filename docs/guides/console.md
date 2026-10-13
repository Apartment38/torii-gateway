# Configure Torii in the console

Use the console to configure providers, routing, credentials, and benchmarks
without manually editing YAML. You need a running gateway and its administrator
username/password. Open its root URL, for example <http://127.0.0.1:8080/>.

## Find the right page

The sidebar groups pages under **MONITOR** (Overview, Activity, Provider Balances), **MANAGE**
(Usage & Quota, API keys), and **CONFIGURE** (Providers, Models, Smart Routing,
Benchmarks, Server), in that order.

| Page | Purpose |
| --- | --- |
| Provider Balances | Inspect provider balances, subscription limits, and reset times |
| Overview | Inspect the current gateway and model summary |
| Server | Listener, display timezone, Activity monitoring and database bootstrap information |
| Providers | Add/test inference and classifier providers; subscription login |
| Models | Enable models, edit/import pricing, and inspect usage, provider quotas and benchmarks |
| Smart Routing | Smart Routing, classifier selection/tuning, session failure policy |
| Benchmarks | Enable sources, enter source keys, refresh and inspect scores |
| API Keys | Create/revoke client API keys and rotate the default key |
| Activity | Recent request events, live updates and optional prompt/Jev inspectors |
| Usage & Quota | Inspect usage and allowance/quota state |

The overview leads with current UTC requests, processed tokens, and estimated cost, plus success among the bounded recent request sample. Provider, enabled-model, and routing context link to their detailed pages. It shows current UTC token distribution, recent completed request metadata, and copyable client base URL/model settings. The model catalog supports search, provider and enablement filters; **Inspect Metadata** expands full capabilities and benchmark provenance. **Activity** defaults to completed requests; choose **All Events** to inspect request stages. **Usage & Quota** shows the chosen client scope and UTC period without combining daily and monthly totals. View filters change only the display. Budget limits and advanced settings expand below the usage summary.

The Overview client model selector includes token prices, provider allowance/reset
information, and this month's gateway usage. **Models** shows the same tracking
and expands provider-reported spend under **Inspect Metadata**. **Provider Balances**
shows one card per provider, combining shared account balances and independent quota
windows without adding repeated balances together. It shows subscription limits
separately from the operator budgets on **Usage & Quota**.
Dollar balances, token/request allowances and subscription percentages retain
their units; reset times use the saved display timezone (your browser timezone when unset). “Quota unknown” and
“Usage unavailable” mean data was not reported or could not be read, not zero.
Portal dollar balances and some Codex credential types remain unknown; see
[Provider support](../reference/providers.md#costs-usage-and-remaining-allowances).

Models and Activity support search, filters, and pagination. Models offers 25, 50,
100, or **All** rows, with synchronized controls above and below the table. Select a
page number or use **Previous** / **Next**; longer page lists show **First**, nearby
pages, and **Last**. Activity mobile views start with five rows. The overview
period selector switches between the current UTC day and month. Benchmark scorecards keep full measurements in **Inspect** dialogs.

The console supports light and dark themes. It loads the active configuration
when you sign in. Static assets are public, but configuration and credential
management require administrator authentication.

## Timezone and Activity monitoring

On **Server**, set **Display Timezone** to an IANA name such as `America/New_York`
or `UTC`. It applies across the console after saving. Leave it blank to use your
browser timezone. Usage periods remain UTC; a benchmark date without a time keeps
its published calendar date.

Under **Activity Monitoring**, configure **Maximum Data (MiB)** (default
100; 1 MiB is 1,048,576 bytes) and **Retention (Days)** (default 7). Oldest events are
removed when either limit is exceeded; changing limits applies cleanup immediately.
Expired events are also cleaned up at least every minute and open Activity views refresh.

**Enable Prompt Logging** and **Enable Jev Decision Logging** default to off.
Their tooltips identify these as debugging controls: leave them off unless needed.
With logging enabled, each Activity row has **Inspect Prompt** and/or **Inspect Jev
Decision** buttons. The overlay shows the original API request body, or the request
sent to Jev and its response, with JSON indentation and line breaks in prompt text.
Plain text and truncated bodies retain their captured formatting. Unrecorded details have disabled buttons;
Jev is not called for every request. Each body is capped at 64 KiB and truncation is
labeled. Captured content shares the event retention limits and may contain sensitive
prompt data. Closing the card, leaving Activity or signing out clears its display.
Disabling logging stops new capture; older captures expire under the configured limits.

## Apply changes

1. Complete a field edit or toggle a setting; it saves automatically. Provider Add/Edit dialogs save when you click **Save Provider**. Cancel discards that dialog's changes. New router drafts save when you click **Save** on Smart Routing.
2. Wait for **Saving…** to become **Saved**, then check provider/model state.
3. A validation error preserves your draft. Correct it and retry. A conflict means another tab saved first: copy your edits, refresh configuration, and reconcile them before saving.

Torii validates a replacement runtime and atomically commits settings and submitted encrypted credentials to SQLite. Failed changes leave saved and active state untouched. The console serializes saves and never silently retries a conflicting revision. Ongoing requests continue on their original runtime.

Listener changes require restart. Database paths and master-key inputs are bootstrap CLI/environment options. [Configuration](../reference/configuration.md) describes all fields.

Session expiry preserves drafts for reauthentication. Browser reload/sign-out loses unsaved drafts. Administrator login uses an HttpOnly, SameSite=Lax cookie with a seven-day default expiry. It survives browser reloads and gateway restarts. Sign out revokes the session. Saved
classifier tuning and advanced source settings survive configuration saves.
For first-time setup, follow [Getting Started](../getting-started.md).


Provider editors include optional tracking credentials, billing team ID, mode
and endpoint overrides. Stored token values remain hidden. Model selectors show
native-currency balances, subscription/reset windows and banked Codex resets,
alongside model prices and gateway usage. Available monthly provider spend/token
usage is labeled with its account or organization scope. Inspect metadata for price source,
provider-reported organization usage and partial-read reasons. Public Zen wallet
balance and Standard Compute budget amounts remain unknown unless their service
returns them through an explicitly configured tracking endpoint; available reset
headers still appear. See [provider tracking](../reference/configuration.md#provider-tracking).

Provider editors show connection checks and save progress inside the card. A successful check and save closes the editor; connection or save errors keep the draft open for correction. Decision models appear in Models and client discovery and can be selected as
judges after a successful connection check. They use the System One API and are
excluded from chat routing pools. OpenRouter discovers Jev and other decision
models automatically; TypeSafe uses the default ID `typesafe`. Existing `jev`
provider types load as TypeSafe while retaining their saved IDs.

**Provider ID** is editable for every integration. Saving an ID change updates
saved router provider/model pools, judge references, model controls and pricing.
Clients must use the new public prefix. Stored API-key references are retained;
subscription OAuth logins are bound to the ID, so log in under the new ID before
saving a renamed subscription provider.

Smart Routing supports named provider/model pools. The top selector loads a router's full policy. **Create Router** clears the editor to default options; enter a unique **Router Name**, configure the policy, and click **Save**. **Remove Router** stays beside the name and cannot remove the default router. **Save** also applies a named router's name changes. Existing policy edits continue to save automatically. Double-click providers from **Allowed Providers** into **Enabled Providers**, or use the blue **Enable** plus icon / red **Remove** minus icon. The catalog loads automatically when Smart Routing opens and refreshes after provider configuration changes. Disabled provider connections are excluded. This updates **Available Models** and removes enabled models outside a restricted provider pool. An empty enabled provider list allows all providers. Double-click models to move them from **Available Models** to **Enabled Models**, or use the blue **Enable** plus icon / red **Remove** minus icon. A router supports at most 250 choices. Jev evaluates each prompt using model prices, capabilities and benchmark evidence to choose the cheapest capable model. **Allow Paid Models** and **Allow Free Models** sit in the Router Name card, are independent and enabled by default. Usage shows provider quota windows with reset countdowns when telemetry is available.

On **Benchmarks**, **Configure Ingestion** opens the source card and **Upload Data**
opens the CSV card. **Edit Data** switches the current table to inline measurement
editing with per-row **Save** and **Cancel**. **Display Fields** opens Available and
Enabled field lists; double-click to move fields. Saves take effect immediately;
failed validation and concurrent changes retain drafts.
See [Custom benchmarks](benchmarks.md#upload-custom-benchmarks) for field definitions,
limits, matching and refresh behavior.

## Enter model pricing

Each model has a pricing status dot. Hover, focus, or select it to read the status:
green means pricing was pulled successfully within 24 hours; yellow means manual
pricing, prices older than 24 hours (including more than one week), a failed
refresh after a successful pull, or a bundled estimate/unknown update date. Gray
means prices have never been pulled or set. Manual prices always stay yellow,
including when a provider's free-model policy sets the effective cost to zero.

**Refresh Pricing** attempts fresh provider model-catalog reads. Failed providers
retain their last known prices and successful update date. Providers without live
pricing retain available bundled estimates; fetching models does not make those
estimates current. Manual rates continue to take precedence. Pricing freshness is
held in the running catalog and is rediscovered after restart.

Use this when a provider does not supply rates, or when its estimates need an
operator override. Rates are USD per million tokens and affect future routing,
Jev criteria and gateway cost estimates across all routers. They do not change
provider billing or historical usage totals.

1. Open **Models**, find the model, and select **Edit Pricing**.
2. Enter **Input USD / Million Tokens** and/or **Output USD / Million Tokens**.
   A blank field uses its automatic rate; `0` explicitly means free.
3. Select **Save Pricing**. The console preserves the draft if another settings
   change causes a revision conflict; reload and reconcile before trying again.

**Use Automatic Pricing** removes the model's manual rates. Automatic pricing
means the existing catalog metadata from providers, bundled estimates and catalog
overrides. Providers with **Treat Models As Free** enabled retain zero effective
rates; disable that provider option when entered rates should apply.

For a batch update, select **Import Pricing** to open the import dialog, then
**Download Pricing CSV Template**, replace the sample row, choose the file under **Pricing CSV**,
and select **Upload Pricing CSV**. The console validates the entire file before
saving through the same revision-protected configuration flow.

| CSV field | Meaning |
| --- | --- |
| `model_id` | Required exact provider-qualified model ID, such as `openrouter/vendor/model`. Models may be priced before discovery. Maximum 512 bytes. |
| `input_per_million` | Optional finite, nonnegative input rate in USD per million tokens. Blank restores automatic input pricing. |
| `output_per_million` | Optional finite, nonnegative output rate in USD per million tokens. Blank restores automatic output pricing. |

Keep the header and column order. The limit is 1 MiB / 1,000 rows. Duplicate IDs,
invalid IDs, negative rates and nonfinite numbers reject the whole file. Imports
replace the manual rates of listed models and keep other models' rates. Both rate
fields blank removes that model's manual pricing. Saved rates survive restart,
rediscovery and settings reload.

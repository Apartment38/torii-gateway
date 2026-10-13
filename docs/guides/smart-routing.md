# Use smart routing

Request `smart` instead of a provider-qualified model to let Torii choose an
eligible model. Start with working inference providers and verified model
metadata. No classifier is required for deterministic selection.

## Choose smart in your client

1. Confirm your inference models appear in the console or `/v1/models`.
2. Set your client's model to `smart`.
3. Send a request. Torii filters candidates for availability, quota, endpoint,
   required features, context, and your routing policies.
4. Inspect `X-Torii-Model` in the response headers to see the physical selection.

If no eligible model exists, the request fails rather than ignoring a capability
or policy restriction. Explicit `provider/model` requests bypass smart selection.

## Create a named router

Use additional routers when clients need different model pools or policies.

1. Open **Smart Routing** and select **Create Router**.
2. Enter a unique **Router Name**, for example `coding`; `smart` is reserved for
   the default router.
3. Choose its enabled providers/models and routing policy, then click **Save**.
4. Refresh your client model list and request `coding` as the model name.

Each router has its own classifier, policy, quota cache/local allowances and
conversation affinity. Provider credentials, model enablement, client budgets
and benchmark evidence are shared. A restricted router fails when no eligible
model remains; it does not escape its pool. See
[Additional smart routers](../reference/configuration.md#additional-smart-routers).

## Let Jev choose the model

Jev receives the task prompt and chooses the cheapest model capable of completing
it. Each candidate's decision criterion contains its prices, verified capabilities,
context limit, quality metadata, and task-relevant benchmark scores. Jev assesses
which scores matter for the task; higher scores alone do not justify higher cost
when a cheaper model can succeed. Decision models can share a provider with chat models; decision-only models
are excluded from chat routing pools.

1. On **Providers**, connect **TypeSafe**, **OpenRouter**, or **Jev Compatible**
   for a compatible decision server such as Laya.
2. Supply a reachable URL and the service's key if required; test the connection.
3. On **Smart Routing**, select the router to configure and its discovered classifier model.
4. Move models from **Available Models** to **Enabled Models** by double-clicking,
   or select them and press the blue **Enable** plus icon. Use the red **Remove**
   minus icon to disable choices.
   Each router supports at most 250 models. An empty list allows all models from
   its allowed providers; pools over 250 must be narrowed explicitly.
5. Wait for **Saved**, then send a `smart` request.

Discovery uses `/models`; OpenRouter also queries its decision-model catalog,
and compatible Laya services may advertise loaded checkpoints via `/health`. A successful connection test is discovery evidence,
not a classification test. Configure at least one chat-capable model too.
If no classifier models are available, the console explains how to add one on
**Providers**; deterministic policy settings remain available.
Disable classification to retain deterministic routing without classifier calls.

## Tune decisions and failure behavior

Expand **Classifier Tuning** only when needed. You can change timeout, minimum
confidence, question instructions, option descriptions, and yes thresholds.
Clear an edited value to restore its default; wait for **Saved** to apply.
Changing questions/providers retains draft tuning; browser reload discards drafts.

Defaults are a 10-second deadline, confidence threshold 0, and yes threshold 0.5.
The deadline range is 1ms–5m; probabilities are finite values from 0 to 1.
Below-threshold or failed classification follows `on_failure`: capability fallback
(default) or fail. Fallback still enforces objective requirements such as tools
and images. Confidence is model-specific and does not predict production quality.
For supported labels and settings overrides, see [Configuration](../reference/configuration.md#router).

## Control cost, quality, and location

Use the routing policy to allow/disallow paid models, prefer local/cloud models,
reserve context headroom, and set quality floors. Paid models are allowed by
default; the console persists an explicit false when unchecked. Estimated costs
need known prices and are not actual bills. Quota allowances and cached provider
limits can remove exhausted models; unknown quota is not assumed unlimited
verified capacity. [Configuration](../reference/configuration.md) lists defaults.
Benchmark weighting is optional; see [Benchmarks](benchmarks.md). Enter missing
model rates through the [pricing editor or CSV import](console.md#enter-model-pricing)
on **Models**; saved rates also inform Jev criteria and cost estimates.


Benchmark intelligence supplies evidence to Jev whenever it is enabled, even
without deterministic ranking weights. Import custom measurements or edit
loaded scores through [Benchmarks](benchmarks.md#upload-custom-benchmarks).
Jev choices remain subject to gateway capability, context, quality, quota and
cost-policy checks. New prompts receive new model decisions; tool continuations
retain their eligible originating pin. Ordinary retries ask Jev again with the failed model excluded. Other classifier types and
classifier-disabled routing retain deterministic selection.

## Keep tool calls on their originating model

Send the same `X-Session-ID` (up to 256 characters, without newlines) throughout
a conversation, especially for tool calls/results. Torii can also use
`X-Conversation-ID`, `metadata.session_id`, `user`, or a conversation fingerprint,
in that order. Example addition to a curl command:

```sh
# Add this header to each request in the same conversation:
# -H 'X-Session-ID: my-conversation-1'
```

Affinity remains in memory and defaults to a 30-minute inactivity timeout.
Restarting loses pins. Tool continuations fail with 503 when their originating
pin is missing or ineligible; restart the conversation instead of replaying
opaque tool IDs against another model. Ordinary retryable failures can reroute
before any response bytes are sent, according to the failure policy.

To explicitly end a session, call `DELETE /gateway/v1/sessions/SESSION_ID` with
a gateway bearer key or administrator login. See [API](../reference/api.md).
For the full decision and dispatch path, see
[Smart routing end to end](../development/routing.md).

Requests with `reasoning_effort: "none"` or `reasoning: {"effort": "none"}`
do not require a reasoning-capable model. Other reasoning controls still enforce
that capability. Auxiliary JSON-schema requests (such as Hermes session titles)
require a model with known structured-output support; native Ollama discovery
provides this for verified local completion models.

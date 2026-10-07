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

## Add a task classifier

A classifier describes the task; it does not generate the user-facing answer.
Typed Jev/Laya classifier providers are separate from inference providers.

1. On **Providers**, add **TypeSafe AI · Jev** for the hosted service, or
   **Jev compatible · Laya / custom** for a compatible decision server.
2. Supply a reachable URL and the service's key if required; test the connection.
3. On **Routing**, select the discovered classifier model in Smart Routing.
4. Choose **Save & reload**, then send a `smart` request.

Discovery uses `/models`; compatible Laya services may advertise loaded
checkpoints via `/health`. A successful connection test is discovery evidence,
not a classification test. Configure at least one inference provider too.
If no classifier models are available, the console directs you back to Providers.
Disable classification to retain deterministic routing without classifier calls.

## Tune decisions and failure behavior

Expand **Classifier tuning** only when needed. You can change timeout, minimum
confidence, question instructions, option descriptions, and yes thresholds.
Clear an edited value to restore its default; choose **Save & reload** to apply.
Changing questions/providers retains draft tuning; browser reload discards drafts.

Defaults are a 10-second deadline, confidence threshold 0, and yes threshold 0.5.
The deadline range is 1ms–5m; probabilities are finite values from 0 to 1.
Below-threshold or failed classification follows `on_failure`: capability fallback
(default) or fail. Fallback still enforces objective requirements such as tools
and images. Confidence is model-specific and does not predict production quality.
For supported labels and YAML overrides, see [Configuration](../reference/configuration.md#router).

## Control cost, quality, and location

Use the routing policy to allow/disallow paid models, prefer local/cloud models,
reserve context headroom, and set quality floors. Paid models are allowed by
default; the console persists an explicit false when unchecked. Estimated costs
need known prices and are not actual bills. Quota allowances and cached provider
limits can remove exhausted models; unknown quota is not assumed unlimited
verified capacity. [Configuration](../reference/configuration.md) lists defaults.
Benchmark weighting is optional; see [Benchmarks](benchmarks.md).

## Keep a conversation on one model

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
For implementation and evaluation evidence, see [Routing contracts](../development/routing.md).

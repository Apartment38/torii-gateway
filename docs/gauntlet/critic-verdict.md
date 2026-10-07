# Independent critic verdict — 2026-10-07

**Accept the scoped routing-safety and usability result.** No remaining implementation defect requiring repair was found in the inspected source, frozen tests, final container outputs, live Jev evidence, or settled console flows. This verdict does not establish general superiority over Hermes, production answer quality, savings, or fewer setup actions.

## Frozen comparisons

The [anonymous verdict](artifacts/blind-review.md) was written before the identity reveal. The critic did not read the private mapping or named final Torii container records before choosing both winners. Prior source inspection exposed the algorithms, so this was label-blind output judging, not a fully naive algorithm-blind experiment.

- **Shared reference cases: A wins 6/6 versus B 4/6.** Inputs and ordered model pools were identical. B selected the model declared tools-unsupported for `objective_tools` and `malformed_safe_fallback`, despite explicit function definitions. These are two unambiguous capability violations; A had none in this subset. This subset contains no paid-policy case. Added low reasoning effort was not counted as a proven separate capability violation.
- **Original/revised gateway cases: A wins 11/11 versus B 9/11.** B reused the cheap model after the current request added complex reasoning and explicit high effort, and invoked a paid model when the frozen policy required rejection. A revalidated the evolved request and rejected paid-only inference.

The [subsequent reveal](artifacts/comparison-provenance.json) identified reference A as Torii and B as the pinned Hermes middleware; revision A was revised Torii and B was the original app.

The critic independently reran the pinned reference harness. Ten source hashes were checked, and the actual pinned routing, client, configuration, catalog and supporting modules executed through the documented transport injection seam. All three grid models were present in its real cached ModelCatalog. The protocol adapter supplied the same hard-task/malformed signals to the systems' different typed questions. This fairly tests constraint enforcement under weak or malformed decisions; it does not compare real Jev decision quality. Hermes gateway retry, response relay, paid-policy controls and virtual-model behavior were appropriately excluded from the shared score. Injected middleware CPU timing is not comparable with gateway HTTP or live-service latency.

## Repairs independently required and replayed

The original frozen fixtures exposed unsafe session reuse and paid-policy bypass. Final selection now checks current objective and semantic requirements, policy, quota, context and quality eligibility across initial selection, affinity and bounded pre-stream retry.

Fresh-context review additionally reproduced three gaps without editing the implementation:

1. A contextual theorem follow-up sent only `Yes, do that.` to the classifier. It now includes bounded relevant instructions and conversation while preserving the current turn. Tool payloads, image data, call arguments and reasoning blobs are excluded. The unchanged capture fixture passes; a 160,000-byte latest turn now sends a 3,998-byte serialized decision state.
2. Missing/null classifier usage falsely appeared as measured zero. Optional incomplete usage now remains unknown without discarding a valid decision; explicitly reported zero remains measured zero. Rejected low-confidence decisions retain complete measured overhead.
3. Expired active tool affinity silently crossed from physical model `a` to `b`. A smart tool continuation now requires signature-bound original affinity before classification or inference. The unchanged expiry reproduction now returns 503 with physical calls remaining `[a]`. Explicit physical requests remain available.

The critic independently reran the unchanged frozen critic fixtures, added overlay reproductions, context and safety cases, and affected API/classifier/router/provider/configuration suites. They pass. Local HTTP tests required approved temporary socket access. Full-suite, race, vet, build and console check records are supplied separately; they were not all independently rerun by this critic.

## Final container, console and live evidence

The [final rebuilt container](artifacts/container-after.json) passes **21/21 cases, with zero observed guardrail failures in those cases**. Raw SSE exactly matches the opaque fixture event and DONE bytes. JSON echoes retain all request fields other than the rewritten physical model; inspected transport source changes only that JSON value. Malformed failure and paid-policy rejection make no inference call. Unbound and expired tool continuations make neither a classifier nor an inference call. Cached tool replay does not claim new measured tokens; incomplete usage does not publish token counts.

The critic inspected the actual Firefox helpers, backend assertions, records and settled screenshots:

- [Default setup](artifacts/console-setup.json) starts with working inference and a reachable local compatible service. The UI adds and tests a classifier provider, discovers/selects its model and saves/reloads. A smart tool request then selects the eligible model. No catalog file, model overrides or advanced tuning edits are needed; tuning starts collapsed. The compatible decision service is a fixture, not an installed Laya checkpoint.
- [Customization](artifacts/console-customization.json) preserves timeout, confidence, question instructions and thresholds through save/reload and a browser refresh. Explicit paid=false persists, provider configuration and secret references remain intact, and original configuration is restored. Its hosted Jev credentials already existed.

An initial screenshot capture raced rediscovery. The final setup helper waits for completed backend reload, absence of the checking notice, and an enabled actual discovered option. Both shipped screenshots now show settled selections.

Two real calls returned Jev `jev-1.13.0` through the final gateway:

| Request | Classifier latency | Reported input/output tokens | Actual decision |
| --- | ---: | ---: | --- |
| Greeting | 349.715 ms | 633 / 227 | conversation / low |
| Contextual theorem follow-up | 167.435 ms | 683 / 227 | math / high |

The second request also objectively requires reasoning, so selecting the strong model alone would not demonstrate semantic interpretation; its returned math/high decision supplies that evidence. Generated inference output is synthetic. Fixture-only decision-call counts of zero on these live rows do not mean no classifier call occurred.

At the [cited TypeSafe price](https://docs.typesafe.ai/models) of $0.042 per million input tokens with output free, classifier estimates are $0.000026586 and $0.000028686. **Actual billed dollars remain unknown.** Fixture inference estimates are not production charges or savings. The mixed 21-case latency distribution is a harness statistic, not representative production latency. Recorded structured logs agree with the live headers and contain no checked sensitive environment values, authorization payload or tested prompt text.

## Limits on the accepted claim

The accepted result is better constraint enforcement on the six inspectable pinned-reference fixtures, safe behavior on the final gateway cases, working optional customization and default setup under stated prerequisites, preserved JSON/SSE, and measured live classifier latency/token usage with honest price estimates.

General zero violations, production answer quality, economic savings, actual billing, fewer/faster comparative setup actions and complete Hermes chat/gateway inference behavior remain unestablished. UI actions and bundled CLI commands are not interchangeable setup units. Artificial isolation guards must not be attributed to the plugin.

## Actual Hermes installation and registered dispatch

The critic subsequently inspected [the installed reference record](hermes-installed-results.json) and its harness, without repeating dependency installation. All **15 installed package hashes** were independently recomputed and matched the recorded pin: the original ten source hashes plus manifest, registration, command, tool and package metadata files.

Raw CLI transcripts substantiate this isolated sequence:

1. Pinned local-source installation without force was blocked by a CAUTION scan with four documentation/CI/README findings.
2. After review, forced installation succeeded. The noninteractive dependency-consent path left the published plugin disabled despite the requested enable flag.
3. Separate enable succeeded and reported that activation takes effect on the next session/gateway restart.

Earlier dependency failures caused by the harness's offline guard are explicitly excluded from product judgment. Temporary manual staging was replaced by successful CLI publication. Source-copy and unknown-managed runtime warnings do not establish ordinary remote-install friction.

Actual host discovery loaded the plugin with no registration error or deferral, registered one `llm_request` middleware, two plugin tools and two hooks. The supported host settings writer persisted a three-model grid and default model through reload. The actual middleware dispatcher then selected the tools-unsupported cheap model for the explicit-tools and malformed-fallback integration cases, and correctly selected strong for a contextual follow-up. Registered trace records ground these observations.

An initial installed dispatch used slightly different inputs; the critic required an exact replay before accepting a matched integration claim. The final replay reads the immutable reference artifact, applies only the established host model mapping, and asserts compact serialized input equality. The critic independently verified all **six exact serialized host inputs**, their per-request SHA256 values, and the fixture-source hash (`fe36dfc6d5e526f1031d765ea56daf184c9e239f10994e26d6eb1993581bd369`). Evolved-affinity history and explicit high effort are preserved with distinct turn IDs and the shared affinity session.

The real registered dispatcher matches four of the six expected models and reproduces exactly the `objective_tools` and `malformed_safe_fallback` failures; the frozen contextual follow-up correctly selects strong. This grounds the original pinned-reference result through actual host registration and dispatch. The replay uses the already enabled plugin and offline injected decisions, with no reinstall or production call. All 15 installed package hashes were rechecked and still match.

Isolation uses a tracked source copy that excludes account environment/credential files, a synthetic key, a sanitized child environment and temporary profile/runtime/store/cache paths. The guarded Python process rejects outside credential/configuration reads and writes, while native dependency preparation receives the isolated paths. The artifact records no inference/account calls; no shared credential read or user-profile mutation is demonstrated by the inspected successful transcripts. Existing host Python/dependency packages are read-only inputs, so this is not a fresh Hermes installation or account provisioning test.

These observations close the earlier absence of real host registration/install evidence and the integration input-fairness gap. They support a concrete isolated installer warning/review/enable sequence and working customization/dispatch on the exact frozen cases. They establish neither fewer user actions than Torii nor end-to-end real Hermes inference, production latency/cost, or general plugin safety. The scoped acceptance verdict is final; no remaining material repair is required by this evidence.

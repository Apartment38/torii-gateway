# Torii smart-routing Gauntlet results

Evaluated 2026-10-07 against the [pinned Hermes Jev effort router](https://github.com/AlphaPerseii3000/jev-effort-router/tree/562dc2a27aa22977e0795809a17fbbbb1819f0ab).

## Inspectable comparison

**Bar:** Torii beats the pinned Hermes Jev router on the same frozen routing cases and observed setup steps, with zero capability or policy violations, editable criteria, preserved JSON/SSE, and measured decision latency and cost.

A separate critic selected anonymous A in both comparisons before the identities were revealed. On six shared model-selection fixtures Torii passes 6/6; unchanged pinned Hermes middleware passes 4/6. Its two mismatches select the declared tools-unsupported model for requests with explicit tools. Both systems receive the same task signal, model pool and capability facts, through their respective typed decision questions. This compares constraint enforcement under weak/malformed decisions, not the quality of real Jev decisions. Low reasoning effort alone was not counted as a capability violation.

On eleven identical HTTP cases the revised running Torii passes 11/11 versus the previous app’s 9/11. The previous app reuses an ineligible session model and forwards to a paid model under a paid-forbidden policy. All 21 final container cases pass, including context-dependent follow-ups, missing usage counts, unbound/expired tool sessions, bounded pre-stream retry, explicit model bypass, exact JSON forwarding and exact SSE bytes. Critic-added Go cases also cover unsupported endpoints, context, quality and exhausted quotas.

Evidence: [independent acceptance](critic-verdict.md), [blind verdict](artifacts/blind-review.md), [anonymous shared A](artifacts/reference-A.json), [B](artifacts/reference-B.json), [anonymous revision A](artifacts/revision-A.json), [B](artifacts/revision-B.json), [identity reveal](artifacts/comparison-provenance.json), [previous container](artifacts/container-before.json), [final container](artifacts/container-after.json), [pinned executable reference](hermes-reference-results.json), [safe routing logs](artifacts/routing-log-evidence.txt).

The Hermes harness verifies ten pinned source hashes and executes nine unchanged modules with their supported injected transport and real cached ModelCatalog. Middleware model-selection cases are comparable. Gateway-only response relay, retry, paid-policy enforcement and virtual model semantics have no equivalent host behavior in this harness and are not scored as direct Hermes failures. Its instantaneous transport timing cannot be compared with gateway HTTP or live Jev latency.

## Minimal setup and customization

The actual Firefox console first-time setup started with a working inference provider and a reachable local Jev-compatible service. Observed actions: sign in; add a provider; choose Jev compatible; enter its ID/URL; test and discover; select the discovered classifier; save/reload. A smart tool request then selected the eligible discovered model. No catalog file, model overrides or tuning edits were required. The local compatible service was a typed fixture, not a deployed Laya checkpoint.

A separate actual-console check confirms the existing hosted Jev model is discovered, tuning starts collapsed, timeout/confidence/question/threshold edits persist, and paid=false survives save/reload. Original provider configuration and encrypted secret references were preserved and the original app configuration restored. Hosted Jev credentials already existed; this is not evidence of provisioning a new paid account. Screenshot capture waits for completed reload/discovery rather than a transient unavailable label.

Evidence: [first-time setup record](artifacts/console-setup.json), [settled setup screenshot](artifacts/console-setup.png), [customization record](artifacts/console-customization.json), [settled customization screenshot](artifacts/console-customization.png). The [actual Hermes setup/registry record](hermes-installed-results.json) uses an isolated copy of preinstalled host version `v0.21.4+canary.20260930T070235Z-6-g6ec05205a9` at commit `6ec05205a943cf813bd56c3c79d64bcf922dac67`. Installing the pinned local Git source first required review of a CAUTION scan; reviewed `--force --enable` installation succeeded but left the plugin disabled in its noninteractive dependency-consent path. A separate enable command succeeded. The real host registry loaded its middleware, tools and hooks; supported settings persisted/reloaded, and actual registered dispatch reproduced the same 4/6 matched routes plus a correct contextual follow-up. All six registered host inputs were checked byte for byte against the original frozen reference after only the virtual-model mapping. Fifteen source/package hashes remain intact.

That test used only synthetic credentials and isolated source/profile/runtime/cache under /tmp. Public dependency preparation was allowed for installation. Initial offline-guard failures, temporary manual staging and isolation scaffolding are explicitly excluded from product judgment. Host generation and paid provider provisioning were not exercised. UI actions and bundled CLI commands are not interchangeable units for a fewer-steps or setup-speed claim.

## Live decision latency and cost

Two calls reached the saved hosted Jev provider and returned `jev-1.13.0`; inference output was synthetic. The greeting produced conversation/low and selected cheap. The contextual spectral-theorem follow-up produced math/high and selected strong. Objective reasoning was also required for that second request. These are service compatibility and routing examples, not an answer-quality benchmark.

| Request | Classifier ms | Full HTTP ms | Measured input/output tokens | Price-based classifier estimate USD |
| --- | ---: | ---: | ---: | ---: |
| Greeting | 349.715 | 353.565 | 633 / 227 | 0.000026586 |
| Contextual proof follow-up | 167.435 | 171.819 | 683 / 227 | 0.000028686 |

The estimates use TypeSafe’s published Jev price of $0.042 per million input tokens, with output free, read on 2026-10-07: [official pricing](https://docs.typesafe.ai/models). Total estimated classifier cost is $0.000055272. Actual billed dollars are not returned and remain unknown in gateway evidence. Inference estimates use configured fixture pricing and reserved estimated tokens; they are not measured production charges or savings. Missing/null usage stays unknown, explicit zero stays measured zero, and cached tool replay does not publish a new billable token count.

For the matched eleven local fixture HTTP cases, previous p50/p95 are 2.361/4.283ms and revised p50/p95 are 2.910/3.995ms, using nearest-rank p95 from raw rows. The baseline’s old aggregate used a different small-sample index, so these values are recomputed consistently. The 21-case mixed fixture/live distribution is p50 3.091ms, p95 171.819ms; none of these small samples establishes production latency.

## Validation and deployment

- Full `go test ./...`, `go vet ./...`, and `go build ./...` pass.
- Race checks pass for API, catalog, classifier, provider, router, config and session packages.
- Console build freshness and Node console tests pass.
- All unchanged critic overlays pass, including referential context and expired-tool affinity.
- Final Podman image `3ff67847914d4202d1b6c7c7b44509c2ab901f89c942991de1b48924c0d18adf` was explicitly recreated and verified running; ordinary compose up had retained the previous image and its attempt was excluded.
- All temporary test configuration was restored and temporary client keys revoked. Existing database, master key and mounted configuration were retained.

Reproduce actual installed-host observations with `scripts/test-hermes-installed-reference.py` using the recorded isolated host prerequisites; this helper is environment-specific and defaults to offline guards. Reproduce container acceptance with `python3 scripts/test-smart-routing-container.py --live-jev` (reads .env privately and restores configuration); reproduce the reference with `python3 scripts/test-hermes-routing-reference.py --fetch`. The reference makes no production calls. Run container tests serially against this development instance because they temporarily reload configuration.

See [research and source contracts](research.md), and [user customization guide](../../../guides/smart-routing.md). No secrets are included in these artifacts. No production answer-quality, economic-savings, full Hermes agent generation, or local Laya checkpoint claim is made.

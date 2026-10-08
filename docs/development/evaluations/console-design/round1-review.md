# Independent running-UI review: round 1

**Verdict: clear improvement over original Torii; no demonstrated win over Routerly.** Routerly still provides a stronger operational overview and richer investigation filters. Torii's revised neutral layout and provider editor are cleaner, but consistent usability superiority is not established.

## Evidence and method

Reviewed Routerly supplied reference PNGs and workflow docs. Captured actual Torii original-assets baseline at port 18083 and revised live assets at 18081, using the same disposable Go API/SQLite data: two providers, twelve models, twelve requests, two named keys plus a default key. Desktop 1440×1000; narrow 390×844. Cua had no browser surfaces, so the authorized local Playwright/Chromium development fallback was used. No implementation source inspected.

Baseline and revised captures: `critic/baseline/` and `critic/round1/`. Baseline login is not a valid immutable comparison: its HTML became live while its CSS remained original. Initial old login was visually broken; do not use the later baseline login capture to infer the original HTML.

## Biggest observed gaps

1. **Overview remains a configuration summary.** Counts are truthful and navigable; the stretched two-provider panel and permanent onboarding steps use most of the screen. Routerly shows cost, errors, success, breakdowns and trends. A configured operator needs a request/usage summary and recent outcomes, then setup guidance only when a step remains incomplete. Evidence: `round1/overview.png` vs `references/screenshot-overview.png`.
2. **Activity originally obscured outcomes.** Twelve requests produced 36 rows; historical start/routing events said Pending after completion. Builder revision was visually verified: latest `dark-activity.png` defaults to twelve completed requests, exposes an events filter, shows dates and copyable request IDs. Still missing investigation by model/key/outcome and direct request detail. Mobile table is 938px inside a350px scroll container, so outcomes are initially off screen.
3. **Provider cancellation still needs isolated verification.** Actual Edit→Cancel changed local provider status from Connected to Not checked and left Draft changes visible. No configuration PUT occurred in the tested sequence, so persistence corruption is not established. This is at least misleading state feedback. Add→Escape closed the editor; no configuration save sent. Keyboard Tab reached BODY after Save provider before cycling back into inputs, so no clean focus trap was established. Editor reported no role=dialog.
4. **Accessible labels are incomplete.** Actual getByLabel Provider and Status failed on Models despite visible labels; selecting via visible native selects worked. Inputs in provider Tab sequence lack names/ARIA labels. Fix label for/id associations and dialog semantics; verify keyboard focus remains meaningful.
5. **Mobile relies on hidden horizontal content.** No document overflow at390px, but Models access controls and Activity outcomes sit beyond horizontal table scrolling. Top navigation also scrolls horizontally; later sections are initially off screen. Existing narrow overview is usable, but the long Deterministic value originally spilled inside its card. Builder says typography fixed; next reviewer should recapture.
6. **Provider list wastes vertical space.** Original revision uses ~135px cards per connection. Endpoints and Models shortcut were added during review; latest provider edit screenshot shows them. Routerly's compact list better supports scanning many accounts.
7. **Terminology and stale actions need final sweep.** Observed Add credential under Gateway API keys, raw nanosecond ISO creation timestamps, benchmark instructions naming missing Save & reload, model usage rows showing0 requests alongside tokens. Builder revisions corrected benchmark text and model-row request dashes; latest scoped usage capture verifies dashes and usage-first layout. API-key wording/date cleanup requires fresh verification.

## Actual task results

- Login completed.
- Provider Edit opened, Cancel closed, no save sent.
- Model search qwen showed2/12; local provider filter6/12; Disabled filter0/12 with visible no-result state.
- Benchmark metadata disclosure expanded.
- Development usage scope showed6 requests,2280 input and432 output tokens; Research assistant rows hidden.
- Model filters/scope selection sent no configuration PUT. Observed writes: session POST and two provider-test POSTs.
- Dark theme toggled and Models/Activity remained coherent. Initial immediate theme screenshot showed white metric cards with unreadable pale text; stable `dark-overview-stable.png` distinguishes transition from persistent styling. Builder removed theme background transitions; next reviewer should verify initial toggle too.
- Add provider opens an empty editor; Escape closes it.
- Direct #routing originally redirected to Providers. Latest actual navigation now stays at #routing with Smart routing heading, despite no classifier.
- No JavaScript page errors in completed targeted sequence.

Machine-readable details: `round1/usability-results.json`; harnesses: `critic/inspect.mjs`, `critic/usability.mjs`.

## Handoff to next reviewer

Repeat fresh capture after builders finish. Focus isolated Edit/Cancel and Add/Escape state restoration; dialog keyboard semantics; initial and stable theme toggle; mobile access controls/outcome visibility; current overview utility; API-key label/date cleanup; empty/error cases. Do not treat builder test success as comparative UI proof. No commits or implementation edits by this reviewer.

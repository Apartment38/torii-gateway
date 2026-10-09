# Independent running-UI review — round 4

The current console is preferable for the tested configuration and catalog tasks because its actions, state and availability controls are explicit. Overall visual polish is a tie against the supplied Routerly references. I do not find evidence for a universal Torii win. The largest new usability defect is changing a new provider from a subscription back to another provider category; benchmark detail formatting remains the clearest polish gap.

| Matched piece | Current judgment | Visible evidence |
| --- | --- | --- |
| Shell and navigation | Win for task clarity; visual tie | Workspace/Operations grouping, named navigation and active breadcrumb are clear. Routerly remains restrained and compact. Torii's shell is now similarly coherent in both themes. |
| Model catalog | Win for action clarity; density tie | Explicit Enabled control, labeled search/provider/status/sort controls and named metadata disclosure. Ten rows fit the desktop page. Search/paging/focus checks from round 3 remain valid; current direct-hash Models also opens the correct page. |
| Basic operational overview | Tie overall | Torii has balanced summary/usage/recent-request/provider/connection sections. Routerly's supplied overview makes cost, tokens, success and errors faster to scan in the first row. Torii leads with configuration counts; its operational token totals are farther down. Routerly's richer savings analysis is not an existing Torii workflow requirement. |
| Generic client connection | Win | Copyable URL and model are immediately available; round 3 actual clipboard values matched. |
| Provider edit/cancel | Win for compactness; behavior verified | Round 3 restoration, named native modal and keyboard checks passed. Grouped creation choices are more discoverable than the previous flat list. However subscription selection currently removes all other categories, a usability loss when correcting a choice. |
| Routing controls | Win for compact task clarity; broader routing comparison tie | Concise deterministic explanation and basic controls fit; advanced JSON is disclosed. No routing save comparison was exercised. |
| Activity investigation | Tie overall; stage clarity win | Actual search/outcome/paging work, completed/all-event scopes are truthful, request IDs copy fully. Routerly has a compact investigation table; Torii mobile cards retain outcome and key data. |
| Scoped usage | Tie | Actual development scope/month shows consistent totals and remains clean; view changes cause no configuration writes. |
| Benchmark summary | Readability win over prior Torii; comparative tie | Real backend now displays seven published models in six columns, with prominent overall scores, capability chips, source and explicit ambiguous/unmatched display-only states. Routerly experiment metrics are a different workflow, so they cannot prove equivalent benchmark task superiority. |
| Benchmark evidence detail | Functional pass; polish gap | Named dialog focuses its title, starts at top, closes with Escape, and fits 390px. But 27 uniformly weighted fields and nanosecond ISO timestamps/fractional age require unnecessary reading. Group scores, operating metrics and provenance; format dates/age for humans while retaining precise values when useful. |
| Mobile/theme behavior | Actual Torii pass; no Routerly comparative behavior claim | Current mobile benchmark cards retain all summary fields without document overflow; evidence is 356px wide within 390px. Round 3 model/activity cards and stable themes passed. Routerly mobile and keyboard interactions were not driven. |

## Actual new checks

- Direct URL hashes `#models`, `#state`, `#activity`, `#intelligence`, `#credentials` reached the corresponding named heading. Captures `round4/direct-*.png`.
- Provider creation initially offers API providers, Subscriptions, Self-hosted and Classifiers. Actual transition Ollama → OpenAI Codex changes the type chooser to only OpenAI Codex/Nous; switching to a classifier then cannot be performed. This is an actual UI state defect, not a label or hidden-dialog locator issue. Evidence `provider-choice-openai-codex.png`; root and workflow builder notified.
- Real benchmark data: seven models; qwen filter returns one and retains filter focus; configured view returns six and excludes the unknown fixture model. Evidence dialog title is focused and scroll position is zero. Desktop and mobile Escape close correctly.
- No configuration PUT/PATCH/DELETE during benchmark filter/view/evidence changes. Recorded POSTs were session creation and automatic provider checks. No JavaScript errors. `round4/benchmark-state.json` contains full interaction state.
- Current benchmark summary inspected in both themes at 1430 × 1000; mobile at 390 × 844. `benchmarks.png`, `dark-benchmarks.png`, `benchmark-evidence.png`, `mobile-benchmarks.png`, `mobile-benchmark-evidence.png`.
- Fresh first-run setup Escape previously verified to remain open with setup-code focus; this was performed before root completed initialization. See `setup-escape-results.json` and before/after captures.
- Review page 18080: all five comparison choices now load both images, including original baseline. Caption hiding works; 390px document width is exactly 390px. Full-size image link opens a new browser tab. Relative workflow review/provenance/Status links return 200. `progress-state.json` and `progress-fullsize.json`.
- Review page copy needs updating: it still says 26 benchmark columns and setup recheck pending although current six-column summary and Escape recheck are complete.

## Limits and handoff

Routerly captures are 1800px wide, Torii 1430px; judging hierarchy and affordances, not exact typography size. Routerly reference interactions were not independently driven. All Torii data is a controlled disposable review fixture, including benchmark sources, not production measurements. Broader Routerly feature parity is not a requirement of this comparison.

No implementation edits or commits by this reviewer. Evidence and harnesses only in `/tmp/torii-console-review/critic`. Round 3 results remain useful where no new changes invalidate them. Do not treat repaired findings in earlier reports as current failures, or convert current ties into wins to satisfy the objective.

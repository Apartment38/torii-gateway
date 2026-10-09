# Independent round 5 keyboard addendum

The provider-type focus fix passes actual Chromium keyboard verification. I did not inspect or edit the implementation.

- Home → ArrowDown(s) → Enter selected OpenAI, OpenAI Codex, Ollama and TypeSafe Jev. Focus remained on the freshly rendered Provider type select after every change. All eleven choices remained present; document scroll stayed zero.
- Native popup behavior explains the initial strict probe's false result. On this Linux browser, Enter on a closed select opens its popup; the first Tab dismisses it and leaves select focus. Following Tabs reach Type's accessible help button, then Provider ID. The initial probe wrongly required the very first Tab to reach Provider ID.
- With the popup explicitly opened using Alt+ArrowDown, Home → ArrowDown → Enter commits/closes selection. Tab reaches Type help, then Provider ID. Closed-select Home → ArrowDown without Enter follows that same form tab order. Accessible help remains keyboard reachable.
- Escape closes the editor and returns focus to Add provider. No configuration PUT/PATCH/DELETE occurred; observed POSTs were session login and automatic provider checks. No JavaScript page errors.

Evidence: [final keyboard verdict](evidence/provider-keyboard-verdict.json) is the final scoped verdict. [initial probe](evidence/provider-keyboard-results.json) preserves the first probe and its overly strict assumption; [native popup probe](evidence/provider-keyboard-native-results.json) records the native popup/tab-order distinction. Harnesses are `provider-keyboard-final.mjs` and `provider-keyboard-native.mjs` in the critic directory. Screenshot: [type focus screenshot](screenshots/round5-provider-keyboard-type.png).

No new usability gap found in this check. Prior visual comparisons remain valid; this revision changes focus handling, not visual frames. No implementation edits or commits by this reviewer.

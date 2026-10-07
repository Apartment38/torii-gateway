# Completion checkpoint — 2026-10-07

Resumed after independently verified natural reset (primary0%, weekly16%). The missing/null usage bug and fresh critic’s bounded-context/expired-tool-affinity gaps are repaired. Full Go, race, vet/build and console checks pass; final Podman image was rebuilt, explicitly recreated and verified healthy. Actual container acceptance passes21/21, including two real hosted Jev decisions. Actual Firefox default setup and customization persistence pass, with original configuration restored and temporary keys revoked.

Independent label-blind judging selected Torii6/6 versus pinned Hermes4/6 and revised gateway11/11 versus original9/11. Actual isolated Hermes CLI install/enable and registered middleware confirm the reference results. The critic accepts the scoped engineering bar; billing/production quality/savings/setup-speed claims remain unproven. See [results](results.md), [independent verdict](critic-verdict.md), [short prompt](prompt.md), and the recorded artifacts. HQ link remains WORKBENCH_HQ_LINK in .env; no Discord message was sent. No requested implementation work remains. Last live quota read before close was primary67%, weekly26%, safely below the pause threshold. Changes are uncommitted in the workspace.

The reset continuation was one-shot and has completed; no persistent continuation service was created. Historical quota checkpoint and then-pending tasks are preserved below for traceability; they are now resolved.

---

# Quota checkpoint — 2026-10-07

User requested automatic pause within 5% of any quota and resume after reset. Builders were asked to checkpoint at 93%; the live read-only account/rateLimits/read check returned 97% primary used, 15% weekly used. Primary resets at 2026-10-07 10:10:44 UTC (06:10:44 America/New_York). Do not spend reset credits. A one-shot /tmp/torii-gauntlet-resume.py waits, re-reads actual quota through codex app-server --stdio, and queues a continuation for thread 01a11533-cfb9-75a3-b219-8b930aebcdad only after both windows are below95% and the primary reset advances. Log: /tmp/torii-gauntlet-resume.log.

## State

WORKBENCH_HQ_LINK is saved in .env. User explicitly cancelled Discord delivery. HQ REST helper and private edit capability are /tmp/torii-hq.py and /tmp/torii-gauntlet-hq.json (0600). Read trusted protocol /tmp/torii-workbench-agents.md; never put capability keys in document content. Keep board/status/chat evolving with honest evidence.

Read prompt.md and research.md for the goal/bar. Lead chose smallest independently judged protocol, routing safety, customization, discovery, evidence and live verification pieces. Separate builders and critic used fresh-context GPT-6.1 Sol; operations used Luna. No fixed rounds. No commits or pushes made.

Original full Go suite passed, but critic froze missing safety coverage. The original container passed9/11 matched synthetic HTTP cases; unsafe session reuse and paid-policy bypass failed. Baseline /tmp/torii-container-before.json, original snapshot /tmp/torii-gauntlet-baseline. Every container test restored original configuration and revoked its temporary client key.

Implemented working changes: strict TypeSafe/Laya typed decision handling and redirect protection; configurable instructions/criteria/Noul thresholds/confidence/timeout; collapsed tuning UI and explicit paid=false serialization; shared initial/pin/retry constraint checks; current-turn and content-array text; verified Ollama native /api/show capabilities merged into catalog; plain text format correction; success-only affinity; safe routing response headers and structured decision logs; usage/version/distributions retained internally. Critics independently verified original21 frozen cases, tuning save preservation and paid=false UI fix. No production answer-quality or savings claim is established.

## Required remaining work

1. Consult /tmp/torii-routing-critic.md. New TestGauntletCriticIncompleteUsageIsUnknown finds missing/null classifier output_tokens falsely published as measured0; explicit0 is valid. Decision builder was notified before suspension. Fix optional usage decoding preserving unknown fields, then independently replay frozen tests.
2. Routing builder final affected suite reported internal/api FAIL, others PASS; exact output truncated. Rerun to a file with GOCACHE=/tmp/torii-go-cache. This may be the same usage regression. Evidence /tmp/torii-routing-builder.md and hashes /tmp/torii-routing-builder-sha256.txt.
3. Earlier root full suite failed cmd/gateway TestSmartRoutingUsesConfiguredJevCompatibleProvider: static fake chat-model lacked endpoint metadata after strict routing. Decision builder was fixing fixture to explicitly advertise its known test capabilities, preserving actual no-overrides Ollama critic test. Review whether finished, plus runtime tuning tests. Decision evidence /tmp/torii-decision-builder.md.
4. Run full go test/vet/build and appropriate race checks plus node scripts/build-console.mjs --check and node --test internal/web/app_test.mjs. Local httptest sockets require ordinary sandbox escalation. Do not weaken fixtures to get green.
5. Root started podman compose build torii-gateway before checkpoint (exec session34267); it may finish with an earlier source snapshot. Rebuild latest final source, then podman compose up -d torii-gateway preserving volumes, master key and config. No container reset or database deletion.
6. Run scripts/test-smart-routing-container.py --output /tmp/torii-container-after.json --live-jev using the existing Podman app at .env TORII_PORT8398. It creates temporary fixture providers/catalog/API key, then restores original config and revokes key. Live Jev discovery succeeded with jev-latest and jev-preview. Saved original inference provider is a dummy setup model; fixture generations establish gateway correctness, not LLM task quality. The optional live call validates real decision compatibility with synthetic inference output.
7. Freeze matched baseline/final records; give critic anonymous A/B copies with labels/provenance headers stripped, preserving expected constraints and actual outputs. Let critic inspect real implementation and artifacts, select winner, find largest remaining gap, repeat as needed. Inspect new headers/log injection and errors as well as successful bodies/SSE.
8. Report measured routing latency and actual classifier token usage. Direct TypeSafe official https://docs.typesafe.ai/models lists jev-1.13.0 at0.042USD/M input tokens, output free (read2026-10-07); actual billed dollar charge is not returned. Label price-based estimates and fixture inference pricing separately, keep unknown charge unknown. Add source and concise measured evidence to docs/gauntlet/results.md; update HQ board/status. Claim completion only once real inspectable bar is met, or clearly identify actual evidence limitations.

The one-shot resume process is operational scaffolding in /tmp, not a persistent service. After resume, monitor current account quota before each substantial agent wave and checkpoint again if needed. Never infer available allowance from elapsed time alone.

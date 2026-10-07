# Torii smart-routing Gauntlet

## Goal and comparison bar

Finish the existing smart virtual model using a decision model such as Jev or Laya, with minimal end-user setup and optional customization.

**Bar:** Torii beats the pinned Hermes Jev router on the same frozen routing cases and observed setup steps, with zero capability or policy violations, editable criteria, preserved JSON/SSE, and measured decision latency and cost.

A win requires independent inspection of implementation and recorded outputs, not agreement with the builder's description. Deterministic fixtures establish protocol and gateway behavior; live decision calls establish service compatibility. Neither alone proves better generated answers or production savings. Label estimated and actual costs separately, include classifier overhead, and never infer general answer quality from a mock.

## Concrete references

- TypeSafe's typed Choice/Noul/Score contract: https://docs.typesafe.ai/api
- OpenRouter Jev access and decision endpoints: https://openrouter.ai/docs/guides/community/jev
- Hermes effort-router catalog: https://hermes-agent.nousresearch.com/docs/plugins/jev-effort-router
- Inspectable baseline commit: https://github.com/AlphaPerseii3000/jev-effort-router/tree/562dc2a27aa22977e0795809a17fbbbb1819f0ab
- Gateway customization comparison: https://github.com/zerone-agents/jev-model-router
- Real Laya server and decoder: https://github.com/LeonaDavinci/laya-system-one
- Prompt style reference: https://github.com/mshumer/Claude-of-Duty/blob/main/prompt.md

The pinned Hermes router supplies editable routing defaults, confidence and timeout controls, and decision records, but its provider scope is Ollama:Cloud Chat Completions. Compare actual pinned source behavior: low-confidence decisions can substitute defaults, and absent provider metadata is permissive. Its README's blanket unchanged-request failure claim is not a verified safety guarantee. The zerone project explicitly labels economic gains unverified.

## Research recommendation

Use the existing direct typed decision integration with shipped defaults; let users optionally adjust instructions/criteria and operating thresholds without rebuilding. Hosted Jev avoids local model deployment. A reachable Laya server offers the same protocol with additional local setup. OpenRouter offers Jev with an existing OpenRouter key, but its Decisions API has a separate wire contract and endpoint: do not assume ordinary chat compatibility or substitute it silently for TypeSafe.

Torii already has encrypted provider credentials, discovery, a classifier selector, capability filtering, quota-aware ranking, and a smart virtual model. Initial gaps observed in this checkout were incomplete typed-response validation, no configurable decision rubrics/timeout/confidence control, first-user-only classifier text, affinity returning before current constraints were checked, unsafe generic selection fallbacks, and no per-request routing evidence.

## Evaluation

Freeze fixtures before judging the corresponding change. Exercise valid and malformed TypeSafe/Laya responses, objective tools/vision/reasoning/schema/stream requirements, exhausted quota and paid-model policy, session evolution, pre-stream retries, explicit models, and transparent JSON/SSE forwarding. Inspect setup and save/reload behavior in the authenticated running Podman console. Record setup actions, actual/estimated classifier usage, and elapsed routing time; use anonymous A/B artifacts where provenance can be hidden without hiding correctness evidence. Critics report the largest remaining gap and builders revise it.

## Model and quota policy

Current official guidance recommends GPT-6.1 Sol for complex coding at lower cost than Astra and Luna for focused repeatable tasks: https://learn.chatgpt.com/docs/models. The local account catalog exposes both. Escalate judgment-intensive tasks only when needed. Read actual account rate-limit telemetry, checkpoint before any observed remaining allowance reaches 5%, and resume after a verified reset. A missing telemetry source is not permission to invent an allowance or reset time.

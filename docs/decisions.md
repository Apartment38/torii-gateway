# Decisions

## 2026-09-28 — Narrow the Version 1 provider and classifier scope

Version 1 supports three upstream families only: local Ollama, standard
OpenAI-compatible endpoints (including the normal OpenAI API), and ChatGPT
Codex subscription access. Dedicated OpenRouter, Nous Portal, Gemini, xAI, and
Anthropic adapters move to Version 2.

For semantic routing, Version 1 supports a self-hosted Laya model invoked by
provider-qualified ID through Ollama or a standard compatible endpoint. Laya
must implement the common classifier contract and cannot select `smart`
recursively. Jev integrations move to Version 2. This keeps the first release
focused while preserving generic provider and classifier extension points.

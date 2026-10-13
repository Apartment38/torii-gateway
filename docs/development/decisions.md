# Decisions

## 2026-09-28 — Narrow the alpha (v0.1.0) provider and classifier scope (superseded)

The alpha (v0.1.0) supports three upstream families only: local Ollama, standard
OpenAI-compatible endpoints (including the normal OpenAI API), and ChatGPT
Codex subscription access. Dedicated OpenRouter, Nous Portal, Gemini, xAI, and
Anthropic adapters move to Version 2.

For semantic routing, the alpha (v0.1.0) supports a self-hosted Laya model invoked by
provider-qualified ID through Ollama or a standard compatible endpoint. Laya
must implement the common classifier contract and cannot select `smart`
recursively. Jev integrations move to Version 2. This kept the alpha implementation
focused while preserving generic provider and classifier extension points.


## 2026-10-07 — Document the implemented profiles and database-only baseline

The earlier scope restriction is historical. The current implementation includes
Nous, OpenRouter, Gemini and xAI compatible profiles, Anthropic Messages
translation, and typed Jev/Jev-compatible classification, in addition to Ollama,
OpenAI, generic compatible endpoints, and Codex. Current support is defined by
code/contract tests and the [provider reference](../reference/providers.md).

SQLite owns durable credentials, administrator/client keys, usage, benchmark
observations and host identity. Baseline schema 4 upgrades to current schema 5;
schemas 1–3 require an explicit preserved-backup reset. No legacy importer exists.

## 2026-10-07 — Separate user instructions and development documentation

Use a brief README and task-oriented docs for users, comprehensive references for
supported behavior, and development explanations for architecture/design/research.
One [status document](status.md) owns milestone progress and pending work.
Historical evaluation artifacts retain their scope and qualifications.

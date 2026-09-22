# Architecture Decisions

This document records significant architectural decisions.

Each decision should explain:

- what was decided
- why
- important consequences

Do not record trivial implementation details.

---

## ADR-001 — Use Go

Status: Accepted

Decision:

The gateway will be implemented in Go.

Reason:

The project prioritizes low memory usage, fast startup, efficient HTTP/SSE
handling, simple container deployment, and a small dependency/runtime
footprint.

Consequences:

- Go is the primary implementation language.
- Prefer standard-library HTTP functionality.
- Avoid introducing another runtime.

---

## ADR-002 — Use a Modular Monolith

Status: Accepted

Decision:

Version 1 will run as one process and one binary.

Reason:

The gateway is intended to remain operationally simple and resource
efficient.

Consequences:

Version 1 will not require:

- Redis
- databases
- queues
- sidecars
- worker services
- microservices

---

## ADR-003 — Use YAML Configuration

Status: Accepted

Decision:

Version 1 configuration will use YAML.

Reason:

Provider and routing configuration is hierarchical, and YAML remains
readable for nested configuration.

Consequences:

- configuration changes require restart in v1
- unknown YAML fields are rejected
- secrets are referenced rather than stored directly

---

## ADR-004 — Secrets Are External to Configuration

Status: Accepted

Decision:

Secrets may be loaded from environment variables or files.

Plaintext secret values are not permitted directly in normal YAML
configuration.

Reason:

This avoids accidentally committing provider credentials.

---

## ADR-005 — Public Model IDs Use provider/model

Status: Accepted

Decision:

Physical models are exposed as:

    provider/model

The provider component ends at the first slash.

Reason:

Provider namespaces prevent collisions while allowing upstream model IDs
to contain their own slashes.

Example:

    openrouter/google/gemini-2.5-pro

means:

    provider = openrouter
    model = google/gemini-2.5-pro

---

## ADR-006 — Separate Transparent and Translated Transports

Status: Accepted

Decision:

OpenAI-compatible upstreams use a transparent passthrough path.

Protocol translation is performed only when required.

Reason:

Reconstructing every request would increase allocations, complexity, and
the risk of dropping new OpenAI API fields.

Consequences:

The gateway maintains:

- transparent transport
- canonical translated transport

as separate paths.

---

## ADR-007 — Classification Does Not Choose Models

Status: Accepted

Decision:

Classifiers return a fixed semantic classification rather than a physical
provider/model.

Reason:

Classification and routing policy are different responsibilities.

This permits Jev, embeddings, or LLM classifiers to be swapped without
changing selection policy.

---

## ADR-008 — Explicit Model Requests Do Not Fail Over

Status: Accepted

Decision:

Requests for a physical model use exactly that physical model.

Automatic substitution occurs only through the smart virtual model.

Reason:

Explicit model requests should remain deterministic and debuggable.

---

## ADR-009 — Session State Is In-Memory in V1

Status: Accepted

Decision:

Smart-router session affinity is held in memory only.

Reason:

Persisting session state is unnecessary for the first release and would
introduce additional complexity.

Consequence:

Model affinity is lost when the process restarts.

---

## ADR-010 — Never Handoff Mid-Stream

Status: Accepted

Decision:

Once response bytes have been emitted to a client, the gateway will not
switch to another model within that HTTP response.

Reason:

Combining output from different upstream models would corrupt the SSE
stream and conversation semantics.

---

## ADR-011 — Unknown Quota Is Neutral in V1

Status: Accepted

Decision:

A model whose remaining quota cannot be determined remains eligible for
smart routing.

Reason:

Many providers do not expose reliable quota information.

Future custom routing policies may change this behavior.

---

## ADR-012 — Free-Only Mode Requires Known Zero Cost

Status: Accepted

Decision:

When paid automatic routing is disabled, cloud models with unknown pricing
are excluded.

Reason:

Unknown price must not result in accidental spend.

Local models are treated as zero monetary inference cost for v1 routing.

---

## ADR-013 — OpenAI API and Codex Subscription Are Separate Providers

Status: Accepted

Decision:

Expose:

    openai/<model>

and:

    openai-codex/<model>

as distinct providers.

Reason:

They use different authentication, upstream transports, model catalogs,
and billing/quota mechanisms.

Credentials must never cross between the two endpoints.

---

## ADR-014 — Full Accounting Is Deferred

Status: Accepted

Decision:

Version 1 stores only aggregate usage required for quota-aware routing.

Reason:

A historical accounting ledger is a future feature and should not expand
v1 scope.

Future accounting storage may use CSV or SQLite.

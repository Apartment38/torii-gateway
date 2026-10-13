# Design contracts

These contracts describe current implementation. Configuration/API syntax belongs
in [Reference](../reference/configuration.md); work status belongs in [Status](status.md).
This replaces the historical alpha (v0.1.0) implementation specification.

## Compatibility and transport

- Use transparent transport whenever upstream OpenAI compatibility is sufficient.
  Inspect only routing fields, preserve unknown values, and replace only the
  top-level model string. Do not normalize unrelated JSON or rebuild passthrough SSE.
- Keep translation isolated in adapters/shared canonical types. Represent
  instructions, role messages, text/images, tools/results, reasoning, structured
  output, limits, usage, finish reasons, and stream deltas. Retain validated raw
  extensions or reject unsupported semantics; do not silently drop required behavior.
- Stateless Responses rejects server-side conversation continuation fields.
  Public route existence does not imply all providers/models support it.
- Reuse HTTP clients. Client cancellation propagates upstream. Hop-by-hop headers
  are stripped. Provider redirects never carry credentials to another origin.
- Ollama inference defaults to a configurable five-minute whole-request deadline,
  including response consumption. Its ten-second discovery budget is independent.
- Full configuration saves require the revision from the preceding read. A
  persistent SQLite revision advances with each committed save and survives restarts.
  Serialize checking through publication, before storing submitted credentials.
  Conflicts preserve console drafts rather than silently retrying them.

## Identity and metadata

Provider IDs form namespaces; split `provider/model` at the first slash so
upstream model IDs may contain slashes. Explicit IDs always target that provider.
Decision models expose System One passthrough and judge selection; mixed providers
use catalog decision metadata and decision-only models cannot join chat pools.
Catalog endpoint/features use supported/unsupported/unknown states; unknown is
not affirmative evidence. Context/pricing/quality/locality/availability carry
provenance. Merge bundled → live authoritative → operator overrides. Gateway-wide
`model_pricing` supplies per-field manual rates across all router catalogs and
classifier accounting. Provider free-model policy still forces zero effective cost.

`router.catalog_overrides` stores a strict descriptor array in SQLite settings.
Legacy `router.catalog_file` is read only during explicit YAML import and embedded
in that array. The descriptor schema is
`catalog.Descriptor`: IDs/provider/upstream IDs, endpoints, capabilities,
modalities, context window, pricing per million, general/tool/reasoning quality,
locality, availability, provenance. Overrides must be supported by operator
knowledge, not used to bypass a real incompatibility.

## Classification and selection

Generic classifiers return task/complexity/requirements. Typed Jev additionally
chooses a physical model using dynamic price, capability and benchmark criteria
from the configured pool (at most 250 candidates).
It may be connected generative (`model`/`laya`), prototype embedding, or typed
Jev/System One. A physical classifier cannot recursively invoke `smart`.
Timeout/confidence/failure handling is bounded. Typed questions preserve fixed
labels while allowing rubric edits. Malformed probabilities/answers fail validation.

Objective endpoint/tools/images/schema/reasoning/context requirements survive
classifier failure. Hard gates exclude unavailable, exhausted, incompatible,
insufficient-context, policy-disallowed paid models and insufficient quality.
Unknown quality is admitted only when no known-quality candidate meets the floor.
Jev choices are revalidated under all current gates. Deterministic fallback
ranking considers estimated cost, quota, location, latency,
context headroom, then public ID. Optional usable intelligence adds weighted
utility after eligibility; deterministic ordering breaks ties.

Cost estimates reserve expected output and use known pricing; missing costs are
unknown. Aggregate usage replaces estimates where actual usage is available.
Local allowances compare accumulated estimated cost, not token totals or provider
billing. Provider quota snapshots distinguish known/unknown/exhausted state and
are cached independently, retain all request/token/subscription windows and reset
times, and share provider account reads across models. OpenRouter inference-key
spend/free-request caps and separate management-credit reads keep credentials
isolated. Additive discovery metadata passes pricing, scoped gateway usage and
provider snapshots to harnesses; selectors render these fields when supported.

## Sessions and retries

Resolve bounded session identity from explicit header, metadata, user, then
conversation fingerprint. Context signatures detect reset; affinity is
in-memory, bounded, expiring, and only established after successful inference.
Every reused pin must remain eligible under current constraints.

Tool IDs are opaque provider-specific context. A tool continuation without its
valid originating pin fails before inference; no generic fallback can move it.
Ordinary retryable failure may choose another eligible model only before response
commitment, with bounded attempts. Explicit model requests never reroute.
Handoff metadata must be explicit without breaking required JSON/SSE semantics.

## Persistence and authentication

All durable data, including validated settings and their revision, lives in SQLite.
YAML is accepted only for explicit import and validation. Listener/database/master-key
bootstrap options remain CLI/environment inputs. Catalog overrides are stored in settings.
Encrypt credential payloads with AES-256-GCM and a separately supplied master key.
Verify the key before schema upgrades. Future migrations are immutable sequential
transactions; no silent reset/import/downgrade. Schema baseline 4 is retained.

Inference keys are separate from administrator Basic credentials. Named keys
are independently revocable; default rotation leaves named/provider keys intact.
Saved secret values are not returned by management. Console login uses random opaque cookies; SQLite stores only token hashes and
bounded session metadata. CSRF tokens and origin checks protect cookie mutations. Static asset availability does not waive API authentication. Account edits require current-password reauthentication. Email recovery uses expiring, single-use hashed tokens and transactional credential/session revocation; see [State and security](state-and-security.md).

Legacy model routing counters flush asynchronously. Operator per-key accounting
uses synchronous SQLite reservations and settlements with UTC period totals.
One client request includes classifier and retry attempts. Bounded JSON/SSE
observation preserves forwarded bytes; missing usage retains conservative estimates.
Quota checks acquire a SQLite writer lock before reading totals and fail closed
when storage is unavailable for limited requests. This is estimated operator
accounting, not provider billing.
Benchmarks stage bounded source generations and publish pointers atomically,
retaining previous good data on failure. No multi-process writer guarantee.

## Extension and development rules

Add abstractions only for an implemented or concrete second use. Prefer Go's
standard library; document dependencies' purpose and runtime/binary impact.
Add providers by researching dated primary sources, identifying transparent versus
translated needs, then discovery/auth/streaming/contract tests. Add sources through
`Source.Fetch` and factory registration, retaining provenance/scales and safe
identity mapping. Do not guess aliases or give approximate display matches routing
authority. Add classifiers behind the common contract with malformed/failure tests.

Test public endpoint compatibility, unknown fields, cancellation, credential
isolation, tools/multimodality, session evolution, quota, and commitment boundaries.
Use fixtures for deterministic behavior; live external checks are separate and
must not be represented as general answer-quality or savings evidence.


Provider tracking uses a separate read-only HTTP client/credential inventory for
billing surfaces. It normalizes independent windows, native-currency balances,
provider-reported usage and reset-credit counts into the existing quota cache.
Balance-only data never fabricates a quota denominator. Scoped/informational
windows do not gate unrelated inference; calendar USD spend and organization
reports remain separate from gateway per-key estimates. Concurrent model-picker
warming shares one ten-second deadline. Dated base pricing fills missing model
catalog prices; live values and operator overrides retain precedence.

Codex subscription usage shares the inference authentication manager, refresh
serialization and encrypted persistence, including across handler replacement.
Conditional rotation cannot overwrite a newer login. The optional CLI collector
requires matching account evidence and falls back to the endpoint-bound HTTP
collector. Multi-pool snapshots retain duration-classified windows without
inventing model associations. Background polling and manual reads coalesce per
account, honor backoff/Retry-After and preserve stale successful evidence.
Inference uses cached Codex evidence and does not wait on usage HTTP. Codex
inference-header observations remain independent of polled snapshots and retain
the originating login association. Workspace/user identity keys isolate account
switches; unidentified users never share workspace cache entries.

OAuth client/issuer/scope context participates in Codex snapshot identity. A new
login clears authentication failure backoff while retaining provider 429 cooldowns
and local allowances. Unidentified inference-header pools remain informational;
they cannot assert account-wide exhaustion or erase an available polled bucket.

Console Codex subscription login uses server-side device authorization. The
browser receives only a verification URL, one-time code and status; OAuth
credentials remain on the gateway and retain their account/client/issuer context
in encrypted SQLite. Browser tabs open during the login click, with a visible
link when popup opening is blocked. The optional legacy helper API requires an
explicit login method; console login does not require a helper or loopback listener.


## Console design language

Use Title Case for authored page titles, section headings, field labels, table
headers, navigation, and buttons: capitalize the first letter of every word
(`Add Provider`, `Benchmark Intelligence`). Preserve acronyms, brand spelling,
operator-supplied names, model IDs, CSV field identifiers, and API/configuration
syntax. Descriptions and status sentences use sentence case.

Page and dialog titles use H1 (32px and 28px); sections use H2 (22px), and nested
headings use H3 (18px). Mobile page titles use 28px and sections 21px. Keep heading
levels semantic and use the shared styles rather than shrinking sections locally.

Add/Create actions use `button primary`, matching Add Provider, with an inline
SVG plus icon. Edit actions pair an SVG pencil with an Edit label. Remove/Delete
use `button danger`, the same dimensions and spacing with a red background,
white foreground, and an SVG icon. Removing providers or routers uses the same
trash icon as deleting API keys; removing entries from an enabled list uses a
minus icon. SVGs are decorative (`aria-hidden`).
Use `action-icon-only` for transfer controls and dense table/card actions; retain
an explicit accessible name and a tooltip (`aria-label` and `title`). Labels that
collapse on narrow screens must keep an accessible name in every action state.
Never use a plaintext plus/minus in place of an action icon.

Group Router Name, Save/Remove, Allow Paid Models, and Allow Free Models in the
router identity card. Keep the two independent switches bound to their existing
configuration fields. Rebuild committed `app.js` after TypeScript changes and
validate dynamic button states as well as static markup.

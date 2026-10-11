# Project status

Updated 2026-10-11. This is the sole project progress and backlog tracker.

## v1 implementation and rollout

The v1 implementation and authorized Podman rollout are complete on
`feat/gateway-v1` in `/tmp/torii-gateway-v1`. SQLite now owns settings and revisions;
legacy YAML is used only for explicit transactional import/validation. The console
automatically saves validated edits, preserves conflicting drafts, and uses persistent
administrator cookie sessions. Model controls retain provider-qualified eligibility,
join existing benchmark identities, and honor explicit free-provider pricing. Named
keys have durable JSON/SSE accounting and atomic daily/monthly request, token and
estimated-cost quotas. Metadata-only activity history/SSE and structured rotating
logs are bounded. Codex helper instructions match the selected platform filename
and correctly quote shell arguments.

Full Go tests/race tests, vet/build, all 74 console tests, generated assets,
release metadata, module/format checks and affected documentation links pass.
Gateway and helper binaries cross-build for all five supported release targets.
The exact final image passed disposable non-root persistence/recovery tests,
including sessions, model controls, fragmented JSON/SSE/embedding usage, quota
restart recovery, redaction and wrong-master-key rejection. Acceptance also fixed
SQLite write-lock acquisition, conservative incomplete/classifier usage estimates,
and local model prices when the free-provider option is off.

Source commit `2c821fb` produced live image `cc85d358be44`. Live discovery,
authentication, cookie saves/conflicts, JSON/SSE/Smart inference, named-key usage,
activity and quotas passed; sessions and quotas survived restart. An image
identity check caught Compose retaining a container under a changed tag; forced
recreation and repeated acceptance verified the final artifact. Temporary test
settings were restored, the test key revoked and the session logged out. The
original database, master key, provider credentials, model IDs and benchmark state
were preserved. Stopped backups and rollback images remain in the deployment;
the final correction backup is
`backups/before-v1-accounting-final-20261008T043837Z`, and the original pre-v1
backup is `backups/before-v1-20261008T035917Z`. Task-owned changes are replicated
into both authorized Torii directories. The v1 commit history is integrated into
the project's `main` branch. The review worktree is retained; release publication
remains deferred.

Current user-visible support is in [Features](../reference/features.md), with
contracts in [Design](design.md). Old Version 1/2 labels no longer define support.

## Active work

Expanded provider tracking is complete on `feat/provider-balance-tracking`
in `/workspace/work/provider-balance-tracking`, based on merged PR #19 and
retained for the requested follow-up PR. OpenRouter, OpenCode Go/Zen, Standard
Compute, Z AI, Kimi Coding, MiniMax, DeepSeek, xAI and Anthropic collectors expose
available usage, quota/reset windows and native-currency balances. Nous account
credits and Codex public/legacy usage paths, balances and banked resets are fixed.
Model APIs pass the fields to harnesses; selectors show provider usage separately
from gateway estimates. Billing credentials are encrypted, isolated, rotated on
read and use compatible-provider CA trust. Pricing fallback is dated/attributed;
live rates and operator overrides win. Scoped gauges do not block unrelated
models, zero OpenRouter dollar balances do not block free models, and response
headers cannot indefinitely extend billing snapshot freshness.

Full Go tests/race tests, final focused provider/quota/API race checks, vet/build,
module/format checks, 106 console tests, two release checks, generated assets,
affected documentation links and Chromium at desktop/mobile widths pass. The
production image and five login-helper targets built with an ephemeral proxy CA
secret; the final cache-expiry correction was rebuilt with matching Go 1.25.1
production flags and layered into the same non-root runtime. Disposable checks
verify authenticated balance/price discovery, inference usage/reset headers and
accounting/balances after restart. The shipping Dockerfile remains unchanged.

Public Zen wallet and Standard Compute budget amounts require an explicit private
endpoint because no supported public balance read was verified; available usage
and reset metadata are supported. xAI organization billing requires management
credentials/team ID; Anthropic organization reports require admin credentials,
and its subscription usage requires OAuth. External harnesses must render the
additive model fields. Live billing accounts were not exercised. No merge or
deployment was performed for this extension.

The six extension commits are also applied to the requested
`feat/provider-cost-usage` branch in `/workspace/work/provider-cost-usage`,
retained for review. Its implementation matches the validated tracking branch;
existing routing documentation remains on that branch's original history.

Provider/router fixes are in progress on `fix/provider-connections-smart-routers` in `/tmp/torii-provider-router-fixes`. Implemented classifier counts, in-card connection/save feedback, Nous API-key authentication and pricing, host-aware Ollama port detection, Codex quota/reset metadata, named smart router pools with independent policy/session controls, and separate paid/free gates. Go and console fixture checks pass. Live Podman acceptance and restoration remain in progress. Nous API-key account balance/usage endpoints could not be verified; the operator has been asked how to handle that limitation.


The premium console redesign is complete on `feat/premium-console` in
`/tmp/torii-premium-console`, retained for review. Separate builders and independent
running-browser critics iterated against Routerly screenshots and workflow docs.
Both final reviewers prefer Torii on matched console tasks; catalog density and
scoped accounting remain ties. The operational overview, restrained shell,
provider choice/editing, searchable catalogs, request investigation, scoped usage,
compact benchmark evidence and responsive navigation preserve existing controls.
Real disposable UI checks verify setup, key lifecycle, provider onboarding,
clipboard values, cancellation, deep links, themes and keyboard focus. All 99
console tests, full Go/race tests, vet/build, generated assets, affected links and
disposable non-root container validation pass. No blocking UI issue remains.
The [visual review page](evaluations/console-design/index.html) preserves captures,
critiques, comparison limits and [validation](evaluations/console-design/validation.json).
These subjective comparisons do not establish whole-product superiority. No push,
merge, publication or deployment was performed.

V1 implementation and authorized rollout are complete. Release publication and
platform-native checks remain below. The Smart-first catalog repair and
configuration revision protection are deployed and validated.
Hermes remains unchanged as requested; its model-label formatting and custom
endpoint Fast controls remain outside scope. Its saved-endpoint re-test also
requires re-entering the actual key rather than a credential preview.

## Pending release checks and future work

- MCP aggregation and a skills library.
- A TUI with CLI and web-console feature parity.
- Multiple users with usage tracking, quotas and allowed providers/models/tools/skills.
- Hermes plugin.
- Pi plugin.

- Before publishing a release, rerun the CI gate and disposable container
  persistence/recovery check, complete compatibility notes, verify all artifacts,
  and run opt-in live checks for the provider/account combinations being claimed.
  Code and local test completion is distinct from publication (TASK-041).
- Published release availability and current GitLab template bodies could not be fetched
  in this session. Archived first-party tutorial/how-to materials were inspected
  for structure. Installation keeps source builds as a complete path and refers
  to actual release assets rather than claiming a release exists.
- Docker, macOS, and Windows native/service workflows need validation on their
  respective hosts; documented build targets are not platform test results.
- Future scope, currently unimplemented: administrator password reset, Authentik/OIDC, automated master-key rotation, stateful Responses,
  automatic updater, and MCP/tool/skill gateways.
- Performance measurements beyond the transparent dispatch microbenchmark require
  a reproducible harness; no production answer-quality, savings, actual billing,
  or general comparative routing superiority is established by the retained evaluation.

## Completed milestones without separate old task records

### Smart-first client model catalog — 2026-10-07

Hermes defaults to showing the first 50 models per provider. Adding seven Codex
models moved the trailing virtual `smart` entry beyond that initial list. Torii
now emits `smart` first from `/v1/models`, followed by the unchanged, sorted
physical models. A regression catalog with 60 physical models verifies that
Smart remains on the first client page without changing or omitting other IDs.

The user confirmed Hermes must remain unchanged. Read-only checks reproduced
its stripping of provider prefixes from labels and rejection of Fast overrides
through custom proxy endpoints. Those behaviors were not changed; client/API
documentation explains visibility, **Refresh Models**, and these boundaries.

API baseline, final full Go tests/race tests, vet/build, formatting, generated
console freshness, unchanged module metadata, affected documentation links and
`git diff --check` passed. Built the standard image at source commit `2af3bc2`
and deployed it through the existing Podman Compose project and volume after a
protected stopped database/configuration backup; the previous image is retained.
Live requests returned all 57 models with Smart first and exactly the same
physical IDs, metadata and order. Provider configuration matched before/after;
valid-key access, wrong-key rejection and container health passed. Hermes's
unchanged installed discovery/parser also received Smart first among 57 entries;
its code, configuration and model cache were not modified. Users must choose
**Refresh Models** to replace their cached catalog. Task-owned source files were
replicated into both authorized Torii checkouts; the task worktree is retained.

### Configuration save conflict protection — 2026-10-07

Reproduced an outdated console's full-document save restoring static `ziz/setup`
settings while adding Codex. A fresh console preserved native Ollama settings;
Codex itself did not rewrite the other provider. Logs correlate a later save
with the reversion, but historical request bodies are unavailable, so that
specific stale-tab incident remains inferred.

Configuration reads now supply an opaque `ETag`. Both PUT and POST saves require
its exact `If-Match` value: missing revisions return 428 and stale revisions 412,
before credential storage or handler construction. Serialized saves advance a
revision even for unchanged documents. Meaningful external YAML edits and
restarts invalidate old revisions. The console retains conflicted drafts and
requires reload/reconciliation; repository configuration-writing scripts and
API/provider/design documentation follow the new required-header contract.

Baseline checks passed. Final full Go tests/race tests, vet/build, module/format
checks, all 70 console tests, generated script freshness, script syntax, affected
documentation links and diff checks passed. The standard image at source commit
`b9147e3` passed disposable non-root credentials, persistence, rotation, benchmark
console/API and stopped-backup recovery checks, then replaced the live Compose
container with the original volume intact. The previous image and a protected
stopped configuration/database archive are retained for rollback; the master key
was preserved separately in the unchanged deployment environment.

Restored native `ziz` Ollama with a five-minute inference timeout while preserving
Codex, Jev and routing configuration. Live checks confirmed 57 catalog entries
(49 Ollama, seven Codex, `smart`), missing/stale revision rejection, valid saves,
wrong-key rejection, restart persistence and pre-restart revision rejection.
Small Ollama JSON/SSE and smart inference requests returned content. Hermes's
unmodified installed discovery/parser found all 57 entries using the supplied
key; the desktop form itself was not operated. No Hermes code/configuration or
upstream credentials were changed. Task-owned source files were replicated to
both authorized checkouts; the task branch/worktree is retained without merging.

### Configurable Ollama inference deadline — 2026-10-07

Ollama inference now defaults to a five-minute whole-request deadline instead of
being unbounded. Positive `request_timeout` overrides are accepted in YAML and
the provider card; clearing the console field restores the default. Model
discovery retains its independent ten-second deadline, and client cancellation
still propagates upstream. Updated the example, configuration reference and
provider/client/design explanations. Shorter client/proxy deadlines remain
outside Torii's control; no Hermes code or configuration was changed.

Relevant baseline checks, final full Go tests/vet/build, module/format checks,
full race checks plus affected-package races after discovery isolation, 67 console
tests/generated-script freshness, example YAML, documentation links and diff
whitespace passed. An isolated gateway completed a simulated 32-second cold load;
short and longer live timeout overrides behaved correctly. Regression tests
also cover stream cancellation and independence from discovery. Rebuilt the
Podman image, passed its disposable non-root persistence/recovery suite, saved
`5m` on the existing deployment's Ollama provider, and verified 50-model discovery,
smart chat, the deployed console control and wrong-key rejection. Both project
copies contain the change; previous deployment image/configuration are retained.

### Podman/Hermes connection troubleshooting — 2026-10-07

Replaced a deployment's static `setup` model with native Ollama discovery; the
public catalog now returns 49 upstream models plus `smart`. Jev remains a
classifier-only provider. Added a console warning when connection-test discovery
differs from the static catalog that saving will expose, with a regression test
and provider/client guidance. Replicated project changes to the deployment and
source checkouts and rebuilt/recreated the existing Compose service with its
database intact; retained the previous image and configuration for rollback.

Baseline API/provider/configuration/gateway tests passed. Final Go formatting,
module metadata, vet, build, full tests and race tests passed, as did console
tests/generated-script freshness, affected documentation links, diff whitespace,
and the fresh non-root image's disposable credential/persistence/recovery suite.
Live repeated discovery, persisted-key authentication after recreation, explicit
chat, smart JSON/SSE chat, and wrong-key rejection passed. A cold larger model
exceeded a 30-second client deadline; warmed requests completed under one second.
An isolated fresh Hermes conversation using the already-installed runtime also
passed. Live checks establish connectivity, not general model quality or latency.

The installed Hermes runtime resolves the saved key correctly. Its re-test
validator rejects blank and literal credential-preview submissions; a proposed
saved-key lookup was tested in memory with endpoint/identity isolation and
wrong-key rejection, but no Hermes files were changed. A schema-constrained
request can still fail smart selection because native Ollama discovery does not
verify structured-output support; the existing capability gates remain intact.

TASK-021/022: endpoint-bound Codex credentials, device login, PKCE/remote helper,
refresh and translated Chat/Responses/SSE. TASK-025–027/029–033: connected,
embedding/typed classification, smart endpoint, session affinity/tool safety,
and bounded pre-stream retry. TASK-035/036: SQLite aggregate usage, cost/quota/
latency integration. TASK-037–040: strict schema/CLI, compatibility/security suites,
non-root container and performance harness. These were locally implemented/tested;
release-environment checks remain separate above.

Previously called Version 2 tasks are implemented: TASK-013 OpenRouter discovery,
transparent inference/metadata/quota and separate credit key; TASK-015 Gemini
compatibility; TASK-016 xAI compatibility; TASK-018 Anthropic Messages Chat/basic
Responses/streaming translation; TASK-023 Nous subscription login/refresh/catalog/
quota. Typed Jev and Jev-compatible adapters are implemented. Their limitations
are in the provider reference, not a deferred-adapter backlog.

### Smart-routing evaluation — 2026-10-07

The final scoped critic acceptance resolved the earlier quota checkpoint's pending
usage decoding/fixture/test issues. Retained evidence records 21 final gateway
cases, matched pinned-reference constraint checks, default setup/customization,
and two measured live Jev decisions with synthetic inference output. No general
production-quality/economic claim is made. See [Evaluation](evaluations/smart-routing/README.md).
Temporary continuation/HQ/account-quota instructions are retired.

### Documentation refresh — 2026-10-07

Reconciled current feature/authentication/persistence claims; simplified README;
added complete installation/tutorial/guides/reference; moved technical explanations
and evaluation evidence under development; consolidated the old task ledger,
implementation-plan status, and checkpoints here. Preserved release tooling paths
and migrated artifact consumers. Validation evidence is recorded below.

Validation completed for this refresh:

- Built gateway and helper from source; both version commands succeeded.
- Validated the full example configuration and two standalone documentation
  configs; parsed all 13 YAML blocks, with fragments kept as excerpts.
- Local Markdown file/anchor checks, stale-path search, and `git diff --check`
  passed. All 17 frozen artifacts, including the anonymous review, are preserved
  byte-for-byte at their new location; relocated Python consumers parse.
- Native disposable-fixture smoke passed fresh setup, administrator/bearer
  boundaries, provider test/save/discovery, JSON inference, exact SSE, named
  client keys, restart persistence, and revocation. No real provider calls.
- Existing local `localhost/torii-gateway:benchmark-files` image passed the
  disposable non-root credential container suite: schema 5, setup/rotation,
  named keys, benchmark console/auth/streamed refresh, usage/identity persistence,
  coherent stopped backup/restore, and wrong-key rejection.
- The same existing image passed the documented rootless Podman Compose
  file-secret/ownership/bootstrap/first-admin flow and persisted key after
  down/up in a unique temporary project. Deployment containers/config/data
  were not used. This validates the installation procedure against that image;
  it is not a fresh production image build or registry-backed release test.
- `go test ./internal/state` and release-metadata Node tests passed. The only Go
  source edit repairs the schema-error documentation link; application behavior
  is unchanged. Docker, macOS, Windows, and live upstream checks were not run.


## Historical completed task records

The following dated records preserve task IDs, rationale, tests, results, and
limitations from the original ledger. Their historical wording is not a current
feature specification or active TODO list. Follow current guides for deployment.

### Benchmark file ingestion and console organization — 2026-10-07

Completed native revision-pinned Parquet downloads for LiveBench/Arena, with
bounded temporary storage, complete-shard publication, credential-safe redirects,
and explicit auto/rows/parquet transport compatibility. Added optional encrypted
Hugging Face token controls and display-only configured-model matching that
preserves variants and follows the current provider catalog. Made the toolbar
sticky and top-aligned; moved database path to Server, model failure policy to
Routing, and default API-key rotation to Credentials. Added container listen
address guidance and updated configuration/examples and source research notes.

Validation: full Go tests and race suite, focused final race checks, vet/build,
console generation and behavior tests, release checks, and headless layout checks
across both themes and desktop/narrow screens passed. The combined anonymous live
refresh published 60,372 LiveBench and 10,923 Arena observations using six HTTP
requests and zero /rows calls. The final Go 1.25.1 production image built and ran
as nonroot with its healthcheck; all Parquet fixture tests also passed inside it.
LiveBench's public dataset remains dated 2025-04-07; expired evidence is still
excluded from routing. Generic/custom HF sources retain rows by default.

### TASK-014 — Verify the normal OpenAI API profile

Completed the origin-locked normal OpenAI API profile over the existing
transparent compatible transport. The profile requires an external bearer-key
reference, rejects alternate origins and generic header/model-rewrite options,
supports optional project and organization headers, and remains isolated from
the persistent Codex credential store. Official API reference URLs and the
2026-09-30 verification date are recorded in `docs/development/provider-notes.md`.

### TASK-034 — Implement quota and allowance state

Goal:
Represent known and unknown quota consistently and cache provider snapshots.

Completed:
- added normalized known/unknown quota, remaining fraction, and exhaustion state
- added local allowance injection, TTL expiry, invalidation, and finite-value validation
- coalesced concurrent refreshes per model and preserved caller cancellation
- added cache, unknown-state, exhaustion, local allowance, and race coverage

Decisions:
- unknown quota is never interpreted as exhausted or unlimited
- failed refreshes are not cached, allowing the next request to retry

### TASK-028 — Implement deterministic model selection

Goal:
Choose the cheapest eligible physical model using explainable deterministic policy.

Completed:
- added hard endpoint, feature, context-headroom, availability, quota, and free-only filters
- added configurable complexity, tool, and reasoning quality thresholds with
  unknown-quality fallback only when no known-quality model qualifies
- added upper-bound input/output cost estimation and zero monetary cost for local models
- added quota, location, EWMA latency, context headroom, and lexical tie-breaking
- returned per-model debug reasons and added table and order-independence fuzz tests

Decisions:
- unknown endpoint or required-feature support is not treated as verified support
- unknown price is more expensive than known price and is excluded in free-only mode
- unknown latency is neutral and therefore does not beat measured latency solely
  because it happens to be represented as zero

### TASK-024 — Implement request requirements and classifier contracts

Goal:
Define objective request requirements and one validated classifier contract.

Completed:
- added the fixed Version 1 task, complexity, requirement, preference, confidence,
  failure-policy, request, classifier, and runner contracts
- extracted endpoint, tools, images, structured output, reasoning, streaming,
  approximate input size, and output limits without modifying raw request bytes
- made objective requirements override semantic classifier output
- bounded classifier calls, prohibited `smart` recursion, and implemented the
  deterministic general/medium capability fallback
- added malformed schema, extraction, recursion, timeout, and fallback tests

Decisions:
- estimated input tokens use a deterministic conservative byte heuristic until a
  provider tokenizer exists and are not presented as billed usage
- classification describes work and requirements only; it never names a model

### TASK-020 — Add authentication administration CLI

Goal:
Provide validated, non-interactive administration of provider credentials.

Completed:
- added `gateway auth login`, `logout`, and `status` commands
- required the provider to exist in a strictly decoded configuration
- accepted access and optional refresh tokens only through environment variables
  or files and persisted endpoint-bound credentials
- reported only provider state, including expiry, without printing tokens
- added login/restart/status/logout, redaction, and invalid-configuration tests

Decisions:
- token values are never accepted as command-line arguments
- OAuth-specific interactive acquisition remains owned by each OAuth provider

### TASK-019 — Add secure persistent credential storage

Goal:
Persist provider-scoped OAuth credentials safely. The original file-based implementation
recorded below is historical and has been superseded by encrypted SQLite schema 4.

Completed:
- added a versioned provider-bound credential record and store interface
- added restrictive auth-directory and credential-file permissions
- added size-bounded, strict, corruption-safe reads with secret-safe errors
- added same-directory atomic replacement, file and directory syncing, deletion,
  restart persistence, and provider-isolation coverage

Decisions:
- every stored credential is bound to both a validated provider ID and endpoint
- unsafe permissions, symlinks, unknown fields, trailing data, and wrong provider
  records are rejected instead of repaired or partially loaded

### TASK-017 — Define the canonical translated request/response model

Goal:
Define the translation-only contract needed by the Codex transport without
changing transparent compatible traffic.

Completed:
- added canonical chat/Responses requests, messages, multimodal content, tools,
  tool calls/results, structured output, reasoning, output limits, and extensions
- added canonical responses, normalized usage and finish reasons, and streaming deltas
- added strict validation for lossy or ambiguous request states
- added round-trip and invalid-contract tests

Decisions:
- unknown translated fields must be retained as validated raw JSON extensions or
  rejected by an adapter; silent loss is forbidden
- transparent OpenAI-compatible providers continue to use raw request bytes and
  do not instantiate the canonical model

### TASK-012 — Add configurable generic OpenAI-compatible providers

Goal:
Add origin-isolated, configurable standard OpenAI-compatible upstreams.

Completed:
- added optional discovery or static catalogs, model aliases, and transparent forwarding
- added environment/file-backed bearer and custom-header secrets with unsafe-header rejection
- added independent connection, request, and discovery timeouts plus TLS CA/server-name configuration
- disabled redirects and isolated clients, credentials, TLS, and connection pools by provider
- added configuration, discovery, rewriting, passthrough, streaming, and credential-isolation tests

Decisions:
- custom header values are secret references rather than inline strings
- redirects are returned rather than followed so credentials never leave the configured origin
- generic providers use the existing byte-preserving compatible transport

### TASK-011 — Build the model catalog and capability metadata core

Goal:
Build deterministic, immutable model snapshots without inventing unknown
capabilities or making one unavailable provider hide every other provider.

Completed:
- added normalized model descriptors for endpoint support, capabilities,
  context, price, quality, locality, availability, and provenance
- represented capability knowledge explicitly as supported, unsupported, or
  unknown and embedded the bundled JSON catalog in the binary
- implemented deterministic live, bundled, and configuration override merging
- added startup discovery, sorted list/retrieval views, collision detection,
  partial-provider refresh, bundled fallback, and last-known-good caching
- made startup fail when no provider or bundled fallback is usable while
  allowing a partial catalog when at least one provider remains usable
- added merge, ordering, immutability, collision, stale-cache, fallback, API,
  and partial-provider tests

Decisions:
- the bundled catalog begins empty rather than guessing model capabilities;
  verified provider work will add records as primary-source research lands
- catalog snapshots contain value types and deep-copy optional numeric fields
  so callers cannot mutate catalog state
- public OpenAI model objects remain compatibility-focused while the richer
  normalized descriptors remain the internal source for later smart routing

### TASK-010 — Normalize public errors and request IDs

Goal:
Provide shared typed failures and safe request correlation across the public API.

Completed:
- added the shared OpenAI error envelope and typed internal failure taxonomy
- mapped authentication, request, routing, provider, upstream, capability,
  quota, classifier, and stream failures without matching error strings
- accepted safe client request IDs or generated cryptographically random IDs
- returned the gateway request ID on every response and propagated it to the
  selected upstream while preventing an upstream ID from replacing it
- added structured `slog` completion records containing request metadata only
- tested status/code mappings, propagation, generated IDs, typed wrapping, and
  log redaction invariants

Decisions:
- accept one request ID of at most 128 characters from a conservative ASCII
  alphabet; replace missing, duplicate, or unsafe values
- keep request IDs in a small context package so transports do not depend on
  the HTTP API package
- log method and URL path but never query values, headers, bodies, or credentials

### TASK-009 — Complete transparent OpenAI endpoint coverage

Goal:
Expose the complete transparent OpenAI-compatible endpoint set supported by
the Ollama MVP.

Completed:
- added transparent Completions, Embeddings, and stateless Responses handlers
  alongside Chat Completions
- preserved unknown request fields and the existing 32 MiB body limit across
  every inference endpoint
- relayed JSON and supported streaming responses through the shared transport
- rejected `previous_response_id` and `conversation` with an OpenAI-style
  `unsupported_parameter` error
- added transport, provider, handler, and end-to-end endpoint coverage

Decisions:
- use one provider forwarding contract for the compatible endpoint set so all
  endpoints retain identical byte-preservation and cancellation behavior
- treat the presence of a stateful Responses field as unsupported, including a
  null value, rather than silently changing request semantics

### Pre-v1 usage documentation

Prepared public-facing documentation for the currently implemented pre-v1
feature set and a configuration example.

Completed:
- documented the project's purpose, implemented feature set, limitations,
  source installation, configuration schema, API usage, and operational notes
- added a safe example configuration using an environment-backed client API
  key and a local Ollama provider
- added a test that loads the committed example through the strict production
  configuration decoder to prevent it from becoming stale or invalid

### TASK-008 — SSE streaming relay

Goal:
Relay streaming Chat Completions with low overhead and prompt cancellation.

Completed:
- enabled `stream: true` on the transparent OpenAI-compatible transport
- advertised `text/event-stream` to upstreams for streaming requests
- relayed upstream stream bytes without parsing or rebuilding SSE events
- flushed each copied upstream chunk promptly instead of buffering the response
- retained request-context cancellation through the upstream request
- added transport and end-to-end prompt-flush coverage

Decisions:
- preserve SSE payloads byte-for-byte so unknown, tool-call, and reasoning
  deltas remain compatible
- terminate a failed stream rather than attempting a mid-stream provider switch

### OpenAI-compatible transport hardening

Hardened the shared OpenAI-compatible path by preserving escaped trailing
base-path slashes, bounding Ollama model discovery, validating model-catalog
arrays, rejecting top-level trailing commas, and stripping all applicable
hop-by-hop response headers, including `Proxy-Connection`.
### TASK-006 and TASK-007 — Transparent transport and Ollama provider

Goal:
Forward OpenAI-compatible requests without dropping unknown fields, and support
model discovery plus non-streaming Chat Completions through Ollama.

Completed:
- added a reusable OpenAI-compatible transport with byte-targeted top-level
  model rewriting, context cancellation, shared HTTP-client reuse, and exact
  preservation of every other request field
- added Ollama model discovery through its OpenAI-compatible model endpoint
- exposed discovered models under provider-qualified public IDs
- added OpenAI-compatible model-list, model-retrieval, and non-streaming Chat
  Completions handlers
- explicitly reject streaming requests until TASK-008
- added transport, provider, and end-to-end handler contract tests
- wired configuration loading, Ollama providers, bearer authentication, health
  checks, and the OpenAI handler into the `gateway serve` executable
- added a process-level test that builds and starts the gateway and requests its
  authenticated model catalog
- ensured graceful shutdown waits for active requests to finish, up to the
  configured 10-second shutdown deadline
- made routing-field extraction case-sensitive so unknown case variants cannot
  override the requested provider or model
- extract routing fields without copying unrelated large request values, return
  413 for bodies over 32 MiB, and distinguish client validation failures from
  upstream failures
- validate configured bearer keys at startup and bound idle HTTP connections
- preserve escaped provider base paths and remove response headers named by
  upstream `Connection` tokens
- preserve escaped trailing base-path slashes, strictly reject trailing commas,
  strip proxy connection headers, validate model catalogs, and bound model
  discovery calls without limiting inference
- validate complete, size-bounded model-catalog documents and require every
  catalog entry to be an object

Decisions:
- gateway client authorization is never forwarded to Ollama
- upstream status, body, and end-to-end response headers are relayed for Chat
  Completions while hop-by-hop headers are removed
- model discovery currently fails the aggregate request when a configured
  provider is unavailable; partial-catalog policy remains future work

---

### Continuous integration workflow

Added a minimal GitHub Actions workflow for pushes to `main` and pull requests
targeting `main`. CI verifies formatting and module tidiness, runs `go vet`, the
standard and race-enabled test suites, and builds all packages using the Go
version declared in `go.mod`.

---

### Documentation workflow clarification

Clarified that documentation updates are required for durable changes to
architecture, public behavior, configuration, developer workflow, and
significant design decisions. Narrow task-specific test commands no longer need
to be added to `docs/development/contributing.md` unless they are reusable development
procedures.

---

### TASK-005 — Provider/model ID parsing

Goal:
Parse public model IDs into validated provider namespaces and upstream model
IDs.

Completed:
- added provider ID validation shared by configuration and model ID parsing
- split public model IDs on the first slash only
- rejected missing separators and empty upstream model IDs
- added tests for simple and OpenRouter-style nested model IDs

Decisions:
- parsing returns no partial provider or model value when validation fails
- upstream model IDs are otherwise preserved verbatim, including nested slashes

---

### TASK-004 — Gateway bearer authentication

Goal:
Protect gateway API handlers with the configured bearer API key.

Completed:
- added the extensible client authenticator and identity contracts
- added static `Authorization: Bearer` authentication for the v1 API key
- compared fixed-size API-key digests in constant time
- added OpenAI-style JSON authentication errors and bearer challenges
- added focused middleware, credential rejection, and identity tests
- made the middleware fail closed for missing authenticators and identities

Decisions:
- all missing, malformed, duplicate, and invalid credentials return the same
  response to avoid exposing authentication details
- store and compare SHA-256 digests so comparisons remain fixed-length even
  when a presented credential has a different length

---

### TASK-003 — Secret references

Goal:
Load secrets referenced by environment variables or files without allowing
secret values in YAML.

Completed:
- added a reusable `SecretRef` with strict exactly-one-source validation
- added environment-variable resolution without altering the stored value
- added file resolution with surrounding whitespace trimming
- added optional server API-key secret configuration for strict YAML loading
- added focused validation, resolution, and configuration decoding tests

Decisions:
- environment values are preserved exactly; only file-backed values are
  whitespace-trimmed as required
- missing environment variables and unreadable files fail resolution without
  exposing secret values in errors

---

### TASK-002 — YAML configuration loader

Goal:
Load and validate the initial gateway configuration.

Completed:
- added strict single-document YAML decoding with unknown-field rejection
- added file-based configuration loading
- validated listener and state settings plus the minimal Ollama provider schema
- validated provider IDs, uniqueness, supported type, and HTTP(S) base URLs
- added focused loader and validation tests

Decisions:
- used `gopkg.in/yaml.v3`, because the Go standard library has no YAML decoder
- intentionally reject configuration for providers and subsystems that are not
  implemented yet

---

### TASK-001 — Bootstrap Go project

Goal:
Create the initial Go project and repository structure.

Requirements:
- Go module
- cmd/gateway/main.go
- internal package directories required for Milestone 1
- docs directory
- project builds
- no unnecessary dependencies

Acceptance:
- `go build ./...` succeeds
- `go test ./...` succeeds

Completed:
- initialized the Go module and gateway command
- added the Milestone 1 package skeleton for API, configuration, providers,
  and upstream transports
- kept the module dependency-free
- documented and verified the repository-wide build and test commands

---

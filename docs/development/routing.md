# Smart routing end to end: Jev decisions and gateway selection

When a client requests `model: "smart"`, Torii asks Jev to choose the cheapest
capable model from the configured pool, validates that choice, and dispatches
the original request to that model's provider. Jev receives task text and questions;
Torii supplies catalog prices, capabilities and benchmark evidence as per-model
criteria and owns policy, quota checks, session affinity, and dispatch.
The selected inference model produces the user-facing answer.

Here, **router** means Torii's selection logic. It chooses a provider-qualified
model such as `ollama/example-model`, not another router service. Jev's response
`model` identifies the decision model that answered the questions, not the
inference destination.

This explanation follows the current gateway code and typed Jev/System One
contract. Jev's hosted neural model, training, and internal score calculation
are outside this repository; the observable contract is its typed answers and
probabilities. Jev-compatible Laya servers use the same adapter. For setup, see
[Use smart routing](../guides/smart-routing.md); exact settings are in
[Configuration](../reference/configuration.md#router).

## The complete request path

```mermaid
sequenceDiagram
    participant C as Client
    participant G as Torii gateway
    participant J as Jev / compatible service
    participant S as Torii selector
    participant P as Inference provider
    C->>G: Inference request with model = smart
    G->>G: Inspect requirements, catalog, and session
    opt Classifier configured; generation endpoint; new task
        G->>J: System One: prompt, model choices + seven task questions
        J-->>G: Typed choices, probabilities, and noul scores
        G->>G: Validate answers and apply confidence policy
    end
    G->>G: Merge decisions with objective requirements
    G->>S: Candidates, requirements, complexity, policy
    S-->>G: Validate Jev choice or use eligible pin/fallback
    G->>P: Original request with selected upstream model
    P-->>G: JSON response or SSE stream
    G-->>C: Provider response plus routing headers
    G->>G: Record evidence; pin after successful relay
```

The ordinary path has one classifier call and one inference call. A retry can
add one inference attempt. Tool continuations normally reuse cached task
classification. With classification disabled, or for embeddings, Torii skips
Jev and selects using objective requirements plus the fallback task
`general` / `medium`. Explicit `provider/model` requests bypass smart
classification, selection, affinity, and rerouting; authentication, model
enablement, and applicable accounting still apply.

## 1. Inspect the request

Before classification, Torii validates the envelope, ensures the catalog is
available, extracts objective requirements, and resolves session identity.
These requirements come from the full request:

| Request fact | Selection consequence |
| --- | --- |
| Chat Completions, Completions, Responses, or Embeddings endpoint | Require verified support for that endpoint |
| Nonempty `tools` / legacy `functions`, or tool-call history/results | Require tool support |
| Image content in `messages` or `input` | Require vision support |
| `json_schema` or `json_object` output format | Require structured-output support |
| Non-null `reasoning` or `reasoning_effort` | Require reasoning support |
| `stream: true` | Require streaming support |
| Request size and output limit | Check context capacity and estimate inference cost |

Routing estimates input tokens as normalized JSON request bytes divided by
four, rounded up. The first positive integer among `max_output_tokens`,
`max_completion_tokens`, and `max_tokens` supplies the output reserve. With no
limit, generation selection reserves 1,024 output tokens by default; embeddings
reserve zero. This is a routing estimate, not a tokenizer result or an accounting
reservation.

A smart tool continuation without a valid originating session pin fails before
classification or inference. See [sessions](#7-sessions-and-tool-continuations).
The gateway recognizes continuations when the last item in `messages` or array
`input` is a tool/function role message or a `function_call_output` item.
Implementation: [requirements](../../internal/router/requirements.go) and
[smart preparation](../../internal/api/smart.go).

## 2. Build the state Jev evaluates

Torii builds a text `state` specifically for classification:

- Latest user text: up to 4,000 JSON-serialized bytes.
- Task instructions from Responses `instructions` and system/developer messages:
  up to 2,000 serialized bytes in total.
- Recent user/assistant text before the latest user message: scanning back through
  at most four earlier user turns, with each segment bounded to 4,000 serialized
  bytes and a 10,000-byte context budget.

Recent context is selected from newest messages first, then presented oldest
first. Thus “Yes, do that” retains the earlier task and proposal. Long text keeps
its beginning and end around a truncation marker. Budgets count JSON escaping
and preserve valid UTF-8. Simple requests without extra context use the user
text directly. Completions use `prompt`; Responses support string `input` and
message arrays.

State excludes image payloads, tool results, call arguments, opaque call IDs,
and separate reasoning blocks. Objective inspection still detects features in
the full request, and inference still receives the original content. The state
does not include the candidate catalog, prices, quotas, or session identifier.
See [`smartText`](../../internal/api/smart.go) and
[context tests](../../internal/api/smart_context_test.go).

## 3. Ask Jev to choose a model and assess the task

The JSON request has `model`, `state`, and `questions`. `model` selects the
classifier checkpoint/service model. Direct Jev posts to `/v1/systemone` using
its configured bearer key. Provider transport calls
`DecisionProvider.ForwardDecision`; the Jev-compatible provider posts to its
configured API path plus `/systemone`, with provider credentials and model
rewrites. Deployment URL prefixes are retained.

Jev evaluates the state against the supplied instructions and criteria. Choice
rubrics describe allowed categories; `noul` questions return numeric yes/no
decision scores in [0, 1]. Torii supplies the questions and interprets the
answers:

| Question | Type and default rubric | Gateway use |
| --- | --- | --- |
| `model` | `choice`: cheapest model capable of successful task completion; dynamic physical IDs with price, capability, context, quality and task-relevant benchmark evidence | Inference destination, validated against eligible configured models |
| `task_type` | `choice`: which task best describes the request? | Task identity; matching task-specific benchmark weights when configured |
| `complexity` | `choice`: low = simple/direct; medium = several steps/moderate judgment; high = deep reasoning/extensive context/difficult execution | General-quality floor |
| `tools` | `noul`: requires tool or function calling? | Add tool-support requirement |
| `vision` | `noul`: requires understanding image content? | Add vision-support requirement |
| `reasoning` | `noul`: requires a reasoning-capable model? | Add reasoning-support requirement |
| `structured_output` | `noul`: requires structured or schema-constrained output? | Add structured-output requirement |
| `latency_sensitive` | `noul`: is low latency especially important? | Prioritize measured gateway latency among eligible models |

Task labels are `general`, `conversation`, `coding`, `sysadmin`, `reasoning`,
`math`, `research`, `summarization`, `extraction`, `creative`, and `translation`.
Each has a description: for example, coding is “Writing or debugging code,” and
research is “Research or source synthesis.” Complexity labels are `low`,
`medium`, and `high`. Complete rubrics are in
[`jevQuestions`](../../internal/classifier/jev.go).

Operators can edit question instructions and criterion descriptions while
keeping labels and types fixed. Overrides merge with defaults without removing
options. Rubric edits change what Jev evaluates; `noul_thresholds` change how
Torii interprets its scores. Instructions/criteria are limited to 4,000
characters each and serialized overrides to 32 KiB. Unknown questions/options
are rejected. Boolean criteria use quoted `"true"` and `"false"` keys. See
[typed question overrides](../reference/configuration.md#typed-question-overrides).

Classifier-only Jev providers discover models through `/models`, with an
optional compatible `/health` fallback advertising loaded checkpoints. A
connection test verifies discovery, not classification. These providers are
excluded from the inference catalog and cannot answer Chat Completions.
See [Jev adapter](../../internal/classifier/jev.go),
[options](../../internal/classifier/decision_options.go), and
[Jev provider](../../internal/provider/jev.go).

## 4. Validate and translate the answers

A `choice` answer contains a selected `choice`, its `confidence`, and a
`probabilities` map covering every requested option. Torii verifies the allowed
label has a highest probability; ties are accepted. Confidence and probabilities
must be finite values in [0, 1]. Probabilities must sum to one, allowing small
aggregate error from four-decimal rounding.

Each `noul` answer must have the expected type and a finite numeric `noul` in
[0, 1]. Torii computes:

```text
feature_required = noul >= configured_threshold
classification_confidence = min(task_type.confidence, complexity.confidence, model.confidence)
```

Every yes/no threshold defaults to 0.5; equality counts as yes. Overall
confidence uses the three choice confidences, and need not equal the chosen probability. Optional `noul` confidence
and upstream `action` / `routing` metadata do not control destination selection.
These scores describe classifier decisions, not inference model quality.

All eight answers are required for a model decision. Task-only classifier calls
retain the seven-question contract. Missing/wrong types, invalid labels or
probabilities, malformed JSON, non-success HTTP responses, responses over
1 MiB, and network/timeouts cause failure. Optional usage is accepted only when
both input/output counts are nonnegative integers; bad or absent telemetry stays
unknown without discarding an otherwise valid decision.

The runner defaults to a 10-second deadline and `minimum_confidence: 0`.
Confidence strictly below the minimum follows `on_failure`; equality passes.
Timeout, network, and validation failures follow the same policy:

| Policy | Result |
| --- | --- |
| `capability_fallback` (default) | Continue as general/medium, confidence 0, retaining objective features and no latency preference |
| `fail` | Stop before inference with classification failure (HTTP 502) |

The deadline range is 1ms–5m; thresholds must be finite values in [0, 1].
Low-confidence fallback can retain decision metadata for observability, but its
rejected semantic features do not become requirements. A classifier cannot
recursively invoke `smart`. See [decoder](../../internal/classifier/jev.go),
[runner](../../internal/classifier/contracts.go),
[malformed-answer tests](../../internal/classifier/jev_test.go), and
[threshold tests](../../internal/classifier/decision_options_test.go).

## 5. Turn decisions into eligibility constraints

Torii combines semantic and objective feature requirements with logical OR:

```text
required.tools = objective.tools OR classification.tools
required.vision = objective.vision OR classification.vision
required.reasoning = objective.reasoning OR classification.reasoning
required.structured_output = objective.structured_output OR classification.structured_output
```

A tools score below threshold cannot remove a requirement created by the
original `tools` field. Endpoint, streaming, input estimate, and output reserve
survive classification unchanged. Objective requirements and the routing policy
snapshot remain fixed through retries; Jev can reassess semantic requirements.

Candidates come from discovered inference models. Metadata combines bundled
records, authoritative live facts, and operator overrides. Disabled models and
models without a registered inference provider are skipped. Candidates carry
cached quota, gateway latency, and optional benchmark intelligence. Limited
gateway keys also need capacity for accounting reservations; dispatch enforces
reservations rather than trusting the preliminary capacity check.

The selector applies gates before comparing models:

| Gate | Exclusion rule |
| --- | --- |
| Availability/quota | Unavailable model or known exhausted quota |
| Endpoint | Required endpoint not explicitly supported |
| Features | Required tools, vision, reasoning, structured output, or streaming not explicitly supported |
| Context | Estimated input plus output reserve exceeds the configured fraction of a known context window (default 85%) |
| Paid-model policy | Paid models disallowed and input/output prices not both positively known to be zero |
| General quality | Known score below complexity floor: low 0.55, medium 0.70, high 0.82 by default |
| Specialized quality | Required tools/reasoning have a known score below their respective default floor of 0.55 |

Unknown endpoint/required-feature support fails the gate. Unknown context does
not impose a context limit. Unknown quota is not marked exhausted, but loses to
known quota at the quota-ranking step. Models with unknown general quality form
a fallback pool used only when no known-general-quality candidate passes.
Missing specialized quality alone does not exclude verified feature support.

`treat_models_as_free` supplies explicit zero prices for that provider's models;
locality alone does not imply free pricing. Optional operational intelligence
can fill unknown pricing and tighten/fill context metadata, but cannot replace
known prices or enlarge a known context limit.

No eligible model means HTTP 503 rather than relaxed constraints. Accounting
failures can instead return the applicable budget error. See
[candidates](../../internal/api/smart.go), [catalog](../../internal/catalog/catalog.go),
[free-provider controls](../../internal/api/models.go), and
[gates](../../internal/router/selector.go).

## 6. Choose among eligible models

Tool continuations reuse their eligible originating pin. Jev evaluates every new
prompt; generic routing reuses eligible pins first. Torii compares
eligible models using Jev's dynamic `model` choice. Objective gates first remove
incompatible models; the answer must be a supplied ID and pass semantic quality
and feature gates before dispatch. Jev sees usable benchmark evidence even when
operator ranking weights are zero. Unknown prices and scores remain unknown.
The pool is limited to 250, with an explicit error for larger unrestricted pools.

For non-Jev classifiers, disabled classification, or configured failure fallback,
Torii compares models in this order, proceeding only when the earlier rule ties:

1. For a latency-sensitive task, lower measured gateway latency; known beats
   unknown latency.
2. Higher configured intelligence utility, when usable evidence activates it.
3. Lower estimated inference cost; known beats unknown pricing.
4. Known quota, then greater remaining quota fraction.
5. Configured local/cloud preference (default neutral).
6. Lower measured gateway latency, with known measurements preferred.
7. Greater context headroom.
8. Lexicographically smaller public model ID as a deterministic final tie.

Latency uses an exponentially weighted average: 80% previous value, 20% new
sample. The sample measures the provider's `Forward` call, not full streamed
answer duration or a dedicated TTFT measurement. Estimated inference cost is:

```text
(input_token_estimate * input_price_per_million
 + reserved_output_tokens * output_price_per_million) / 1,000,000
```

Both prices must be known. This excludes Jev cost, actual final output length,
and costs from multiple attempts.

### Task type and benchmark weights

There is no built-in `coding -> model X` table. Complexity changes the quality
floor even without benchmarks. Task type affects ranking when the operator
configures `selection.intelligence.task_weights`: the matching task's capability
weights replace the general capability weights. Operational weights for price,
latency, TTFT, throughput, and context can contribute too.

Utility is a weighted average of normalized scores; missing values use
`missing_score` (default 0.5). All-zero weights preserve deterministic selection.
Without usable intelligence evidence, scoring also falls back to deterministic
ordering. Scores cannot grant unverified features or override any eligibility
gate. In deterministic routing, eligible pins and the latency-sensitive comparison
still take precedence.
See [Benchmarks](../guides/benchmarks.md), [Intelligence](intelligence.md),
[ordering](../../internal/router/selector.go), and
[task-weight tests](../../internal/router/intelligence_test.go).

### Worked example

A user asks to debug a function, supplies tool definitions, and permits paid
models. There is no session pin. Jev returns these illustrative answers (a
summary, not a complete wire response):

| Answer | Value | Interpretation |
| --- | --- | --- |
| Model | cloud/coder, confidence 0.95 | Cheapest candidate judged capable |
| Task | coding, confidence 0.90 | Task-relevant coding evidence |
| Complexity | high, confidence 0.85 | Known general quality must meet 0.82 |
| Tools | noul 0.20 | Below threshold, but tools still required by the request |
| Reasoning | noul 0.80 | Reasoning required |
| Vision/structured output/latency | Each below 0.5 | No additional requirement or latency preference |

Suppose the model choice confidence is 0.95. Overall confidence is
`min(0.90, 0.85, 0.95) = 0.85`, accepted at the default minimum
of zero. Assume these illustrative candidates all have known prices, adequate
context, available quota, and verified Chat Completions support:

| Model | General quality | Tools / reasoning support | Estimated cost | Result |
| --- | --- | --- | --- | --- |
| local/small | 0.70 | Supported / supported | $0 | Below high-complexity floor |
| cloud/basic | 0.90 | Unknown / supported | $0.001 | Tool support unverified |
| cloud/coder | 0.88 | Supported / supported | $0.002 | Eligible |
| cloud/premium | 0.95 | Supported / supported | $0.010 | Eligible |

Assume both survivors pass specialized-quality gates and coding benchmarks
show that `cloud/coder` can complete the job. Jev should choose `cloud/coder`
because it is the cheaper capable option. It should choose `cloud/premium` only
when task-relevant capability evidence indicates the cheaper option is inadequate.
Torii validates the returned choice; it does not override a valid Jev answer
with its deterministic ranking.

If minimum confidence becomes 0.90 with capability fallback, the task becomes
general/medium. Objective tools survive; classifier-only reasoning is removed,
so the candidate set can change. An objective reasoning field would preserve
that requirement too.

## 7. Sessions and tool continuations

Identity resolves `X-Session-ID`, `X-Conversation-ID`, `metadata.session_id`,
`user`, then a bounded conversation fingerprint, in that order. Explicit IDs
are limited to 256 bytes and reject newlines. Context signatures include the
endpoint, instruction context, and first user text, so a conversation reset can
invalidate a pin even with the same explicit ID.

Affinity is in memory, bounded to 10,000 records, with a default 30-minute
inactivity timeout. Restarting loses pins. Only a successfully relayed 2xx
inference response establishes the model/classification pin.

Jev evaluates ordinary follow-ups again and can choose a cheaper capable model.
Generic classifiers retain affinity: Torii checks the pin against current
features, quality, paid-model policy, quota and budget eligibility. A valid pin
wins over a newly cheaper model; an invalid pin is removed. Pins are evaluated individually, so the selector's
unknown-general-quality fallback can admit a pin with no general-quality score.

A tool continuation normally reuses the originating classification and merges
new objective requirements. Tool results are not treated as a new task for Jev.
Opaque tool-call IDs must stay with their originating model: missing, expired,
reset, disabled, or ineligible pins fail with HTTP 503 before inference. A tool
continuation never reroutes after upstream failure. Keep the same identity and
conversation history throughout a tool loop. Pins can be removed with
`DELETE /gateway/v1/sessions/SESSION_ID`; see [API](../reference/api.md),
[sessions](../../internal/session/session.go), and
[affinity tests](../../internal/api/smart_safety_test.go).

## 8. Dispatch, retry, and return the answer

Torii splits the public model ID at its first slash into provider and
provider-local model ID, finds that provider, and calls `Forward` with the
original endpoint/body. The provider resolves model aliases and sends the actual
upstream model ID.

Transparent OpenAI-compatible transport replaces only the top-level `model`
string, preserving messages, tools, images, unknown fields, and unrelated JSON
bytes. Provider-specific translation is isolated in its adapter. Jev's answers
are not appended to the conversation. The inference provider's JSON/SSE is
relayed; transparent SSE copying preserves upstream event bytes.

With `sessions.on_model_failure: reroute` (default), an ordinary smart request
can retry once after transport failure, missing response body, HTTP 429, or HTTP
5xx. Invalid-request transport errors are not retryable. Torii removes the pin,
excludes the failed model, and selects again under the same classification,
requirements, and policy. Jev routing asks for another model decision with the
failed model excluded; generic classification is reused. At most two physical
inference attempts occur. If no alternate qualifies, the original failure
remains. `fail` disables rerouting.

Retry precedes downstream commitment. After response headers/bytes are sent,
relay failure or interrupted streaming cannot trigger a replacement answer.
Failed statuses or relay errors remove affinity. Cancellation prevents another
attempt. Tool continuations and explicit models never take this alternate path.

Operator accounting includes classifier and inference attempts within one
client request. Its conservative token/cost reservations differ from selection's
byte/4 estimate and 1,024-token default output reserve. Complete reported usage
can settle reservations; missing/incomplete usage retains estimates. See
[Configuration](../reference/configuration.md) for gateway-key budgets.
Implementation: [dispatch/retry](../../internal/api/openai.go),
[model replacement](../../internal/transport/openai.go), and
[safety tests](../../internal/api/smart_safety_test.go).

## 9. Verify the decision

Smart responses carry gateway-owned evidence:

| Header | Meaning |
| --- | --- |
| `X-Torii-Model` | Public inference destination |
| `X-Torii-Task`, `X-Torii-Complexity` | Effective classification, including fallback |
| `X-Torii-Classifier-Source` | called, cached_tool_task, or not_configured (also used when embeddings skip classification) |
| `X-Torii-Classifier-Model` | Reported decision-model identity, when available |
| `X-Torii-Routing-Ms`, `X-Torii-Classifier-Ms` | Preparation/selection and classifier elapsed time, not full inference duration |
| `X-Torii-Estimated-Inference-Cost-Usd` | Selected attempt's estimate or unknown |
| `X-Torii-Classifier-Cost-Usd` | unknown; Torii does not derive a classifier bill |
| `X-Torii-Classifier-Input-Tokens`, `X-Torii-Classifier-Output-Tokens` | Valid measured classifier usage from this request, when supplied |

The `smart_routing_decision` structured log records confidence, validated
choice/probability records, bounded selection/exclusion reasons, affinity,
physical attempts/statuses, timing, and known usage. It excludes prompt text,
raw classifier state, session IDs, and provider secrets. Upstream `X-Torii-*`
headers are stripped before gateway evidence is written.

`called` means the classifier was invoked, not necessarily that its answer was
accepted. Read effective task/complexity, confidence, and decision records
together. Selection reasons do not list every eligible loser or models skipped
before selection. Cost headers do not establish total spend or savings. See
[evidence implementation](../../internal/api/smart_evidence.go) and
[evidence tests](../../internal/api/smart_safety_test.go).

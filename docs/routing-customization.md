# Routing customization

Configure a TypeSafe Jev or Jev-compatible provider, test its connection, select
its discovered classifier model on Smart Routing, and choose Save & reload.
Classifier tuning is optional and collapsed by default. Connection URLs and
credentials remain on the Providers page.

The classifier receives the current user text, task instructions, and a bounded
recent conversation so follow-ups such as “Yes, do that” retain their task.
Each text segment is bounded by its JSON-serialized size; tool results, call
arguments, reasoning blocks, and image payloads are excluded from that context.
Original inference requests are forwarded intact except for the selected model.

Smart tool continuations require a valid session pin to their originating model.
If that session expires or its model becomes ineligible, the gateway returns 503
before inference instead of moving opaque tool call IDs to another model.
Use the same conversation/session identity throughout a tool loop.

The defaults are a 10-second deadline, minimum confidence 0, and a yes threshold
of 0.5 for each noul question. Confidence is the lower of the task and complexity
choice confidences. A decision below `minimum_confidence` uses `on_failure`,
either capability fallback or failure. A threshold equality is accepted. These
are model-specific decision probabilities, not evidence of comparative model
quality or cost savings.

The deadline accepts a Go duration from 1ms to 5m. Minimum confidence and noul
thresholds must be finite numbers in [0, 1]. Objective request requirements,
such as tool definitions, images, and required structured output, remain required
regardless of classifier thresholds. Capability fallback also preserves those
requirements.

The question editor lets you change instructions and option descriptions without
changing output labels. Only edited values are saved. Changing the selected
question, testing a provider, replacing the classifier, or choosing Disabled
preserves draft tuning. Clear an instruction, rubric, or threshold to restore
its default. Save & reload applies changes; reloading the page discards drafts.

```yaml
router:
  classifier:
    type: jev
    transport: provider
    provider: laya
    model: english
    on_failure: capability_fallback
    timeout: 3s
    minimum_confidence: 0.4
    questions:
      task_type:
        instructions: Which task type best describes this request?
        criteria:
          coding: Developing, debugging, or reviewing software
      tools:
        criteria:
          "true": Requires external tool or function actions
          "false": Can be answered directly
    noul_thresholds:
      tools: 0.7
```

Question overrides and noul thresholds apply to the typed Jev classifier,
including compatible Laya servers. Timeout and minimum confidence also apply
to connected generative and embedding classifiers. Disabled retains tuning for
later use but makes no classifier calls.

Supported question names are `task_type`, `complexity`, `tools`, `vision`,
`reasoning`, `structured_output`, and `latency_sensitive`. Task options are
`general`, `conversation`, `coding`, `sysadmin`, `reasoning`, `math`, `research`,
`summarization`, `extraction`, `creative`, and `translation`. Complexity options
are `low`, `medium`, and `high`. Noul criteria use `true` and `false` (quote these
keys in YAML), and thresholds may reference only the five noul questions.

Overrides merge into the built-in rubrics; they do not remove options. Nonempty
instructions and criterion descriptions are limited to 4,000 characters, and
the serialized question overrides are limited to 32 KiB. Unknown questions or
options are rejected when validating configuration. Server validation remains
authoritative when editing YAML or submitting console settings.

The console always saves `allow_paid_models` as an explicit boolean, so an
unchecked policy stays false after reload.

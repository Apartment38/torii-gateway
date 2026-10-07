#!/usr/bin/env python3
"""Execute unmodified pinned Hermes Jev middleware with offline matched fixtures.

No Hermes account, .env, real key, production decision call, or inference is used.
Use --fetch on a fresh machine to download hash-checked source dependencies.
The normal run requires the already-downloaded --source-dir and opens no socket.
"""
import argparse
import copy
import hashlib
import importlib
import json
import logging
import os
import pathlib
import statistics
import sys
import tempfile
import time
import types
import urllib.request

PIN = "562dc2a27aa22977e0795809a17fbbbb1819f0ab"
REPO = "https://github.com/AlphaPerseii3000/jev-effort-router"
HASHES = {
    "router.py": "7e6ac69dfea05c7ed4487e03a4d4ff699c04af05ee3cc61d373cb1af34b42c29",
    "client.py": "4aea2421d549c54152ff0734f6963d011d18cd866f239a2d06817fd80488ca25",
    "config.py": "16e727ed2d7215d098d29634035f1dce8b36c0ef1802e5baa4a97d84ed34cdf9",
    "state.py": "a1e3559a46d63f53858e636226d0423ac58cedca7ee702790ef28d458b77e85d",
    "grid.py": "9a527188a115f77b1d5ba9fca0bf4b987dd1c6262ceb6417354f48fc9a8d47ab",
    "effort.py": "58883bec41adb2929ad1f93964ad2dc699a9528a68e50f489f04d3ea0b15a52b",
    "memo.py": "49a8a074d34ecaae08fc16df7ca9ca187b0ea01a528ef4394404b0dd9f198928",
    "audit.py": "f49c5b8dc49a70cb911eac8f480adab3588c97261e401f69a358a4a3ff3a67bb",
    "catalog.py": "32bd4898ef7a2983c8aec5bafb6580c05138ba406bdf31f38205fed93e7aff7b",
    "README.md": "d1a0f15739d81b1aec5752886071549c55edc0b59ca3eb1b7bb16323bab82dde",
}
POOL = [
    {"id": name, "context_window": 100000, "local": False,
     "endpoints": ["chat_completions", "responses", "completions"],
     "streaming": True, "tools": name != "cheap", "vision": True,
     "structured_output": True, "reasoning": name != "cheap",
     "quality": quality, "input_per_million": price, "output_per_million": price}
    for name, quality, price in [("cheap", .6, .1), ("strong", .99, 2), ("backup", .99, 4)]
]


def verify_sources(source_dir, fetch):
    source_dir.mkdir(parents=True, exist_ok=True)
    for name, expected in HASHES.items():
        path = source_dir / name
        if fetch:
            with urllib.request.urlopen(f"https://raw.githubusercontent.com/AlphaPerseii3000/jev-effort-router/{PIN}/{name}", timeout=30) as response:
                path.write_bytes(response.read())
        actual = hashlib.sha256(path.read_bytes()).hexdigest()
        if actual != expected:
            raise RuntimeError(f"Pinned source hash mismatch: {name}")
    # Create a namespace package: import actual modules without invoking host
    # registration in __init__.py. None of the module implementations is patched.
    package = types.ModuleType("pinned_jev_reference")
    package.__path__ = [str(source_dir)]
    sys.modules[package.__name__] = package
    return {name: importlib.import_module(f"{package.__name__}.{name}")
            for name in ["config", "grid", "catalog", "client", "router"]}


class FixtureResponse:
    status_code = 200

    def __init__(self, body):
        self.body = body
        self.text = json.dumps(body)

    def json(self):
        return copy.deepcopy(self.body)


class TypedTransport:
    """Supported JevClient transport seam; common semantic oracle, distinct API questions.

    The Torii container fixture asks task/complexity/feature questions. This
    plugin asks model/effort choices. Answer keys therefore cannot be identical.
    Both get the same deterministic hard=(complex OR reasoning) task signal;
    absent semantic tools support remains false in Torii's original fixture.
    This intentionally tests whether objective requirements constrain a weak
    semantic decision. No simulated answer quality is claimed.
    """
    def __init__(self):
        self.records = []

    def post(self, url, *, json, headers, timeout):
        assert headers["Authorization"] == "Bearer offline-synthetic-key"
        assert url == "https://openrouter.ai/api/alpha/decisions"
        state = str(json.get("state", ""))
        hard = "complex" in state or "reasoning" in state
        malformed = "malformed" in state
        if malformed:
            answer = {"answers": {"task_type": None}}
        else:
            answers = {}
            for name, question in json["questions"].items():
                choice = ("2" if hard else "1") if name == "model_route" else ("high" if hard else "low")
                answers[name] = {"type": "choice", "choice": choice, "confidence": .99,
                                 "probabilities": {key: float(key == choice) for key in question["criteria"]}}
            answer = {"model": "fixture-decision", "answers": answers,
                      "usage": {"input_tokens": 120, "output_tokens": 20}}
        self.records.append({"payload": copy.deepcopy(json), "timeout_s": timeout,
                             "semantic_hard": hard, "response": copy.deepcopy(answer)})
        return FixtureResponse(answer)


def chat(text, **fields):
    return {"model": "smart", "messages": [{"role": "user", "content": text}], **fields}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source-dir", type=pathlib.Path, default=pathlib.Path("/tmp") / f"jev-effort-router-{PIN}")
    parser.add_argument("--fetch", action="store_true")
    parser.add_argument("--output", type=pathlib.Path, default=pathlib.Path("docs/development/evaluations/smart-routing/hermes-reference-results.json"))
    args = parser.parse_args()
    modules = verify_sources(args.source_dir, args.fetch)
    # A synthetic key satisfies the real client's environment gate. No .env or
    # inherited credential is read, passed to the transport, or logged.
    os.environ["OPENROUTER_API_KEY"] = "offline-synthetic-key"
    logging.basicConfig(level=logging.ERROR)
    grid = [f"{m['id']}: quality {m['quality']}; tools {m['tools']}; reasoning {m['reasoning']}; cost {m['input_per_million']} per million" for m in POOL]
    settings_dict = {"grid": grid, "default_model": "cheap"}
    settings = modules["config"].load_settings(lambda key, default=None: settings_dict.get(key, default))
    transport = TypedTransport()
    rows = []
    with tempfile.TemporaryDirectory(prefix="hermes-reference-") as directory:
        directory = pathlib.Path(directory)
        cache = directory / "ollama_cloud_models_cache.json"
        cache.write_text(json.dumps({"models": [{"id": m["id"]} for m in POOL]}))
        catalog = modules["catalog"].ModelCatalog(cache_path=cache)
        assert catalog.models() == tuple(m["id"] for m in POOL)
        router = modules["router"].Router(
            lambda: settings, lambda: types.SimpleNamespace(data_dir=directory),
            client_factory=lambda s: modules["client"].JevClient(s, transport=transport), catalog=catalog)

        def case(name, body, expected_model=None, *, expected_block=False, scope="matched_routing", turn=None, session=None, required_tools=False, oracle_note=None):
            before = len(transport.records)
            original = copy.deepcopy(body)
            # Plugin has no virtual 'smart'. Host starts its middleware at its
            # configured physical model; namespacing also belongs to the host.
            original["model"] = "cheap" if body["model"] == "smart" else body["model"].removeprefix("gauntlet/")
            started = time.perf_counter()
            result = router.on_llm_request(
                request=copy.deepcopy(original), original_request=copy.deepcopy(original),
                model=original["model"], provider="ollama-cloud", api_mode="chat_completions",
                turn_id=turn or name, session_id=session or name, api_call_count=1)
            elapsed = (time.perf_counter() - started) * 1000
            output = copy.deepcopy(original if result is None else result["request"])
            selected = output["model"]
            preserved = {key: value for key, value in output.items() if key not in {"model", "reasoning_effort"}} == {key: value for key, value in original.items() if key not in {"model", "reasoning_effort"}}
            fresh = transport.records[before:]
            violations = []
            model_facts = next(m for m in POOL if m["id"] == selected)
            if required_tools and not model_facts["tools"]:
                violations.append("selected_model_has_verified_tools_unsupported")
            if output.get("reasoning_effort") and not model_facts["reasoning"]:
                violations.append("outgoing_reasoning_effort_added_to_verified_reasoning_unsupported_model")
            if expected_block:
                violations.append("acceptance_constraint_not_enforced_by_plugin")
            audit = router.tail(settings, 1)[0]
            row = {"case": name, "scope": scope, "input": body, "host_input": original,
                   "request_output": output, "selected_model": selected, "decision_calls": len(fresh),
                   "hook_elapsed_ms": round(elapsed, 6), "request_fields_preserved_except_model_effort": preserved,
                   "violations_against_external_acceptance_bar": violations,
                   "expected_model": expected_model, "matches_expected_model": None if expected_model is None else selected == expected_model,
                   "typed_decision_records": fresh, "audit": audit}
            if oracle_note:
                row["comparison_note"] = oracle_note
            if body.get("fixture_fail"):
                row["synthetic_first_upstream_status"] = 503 if selected == "strong" else 200
                row["inference_retry_performed_by_plugin"] = False
            rows.append(row)

        # One-to-one logical requests/pool/order from the frozen Torii container
        # harness. No adversarial absent model: all grid models are in actual cache.
        case("simple", chat("hello", opaque={"preserve": True}), "cheap")
        case("complex", chat("complex reasoning"), "strong")
        case("objective_tools", chat("tools", tools=[{"type": "function", "function": {"name": "lookup"}}]), "strong", required_tools=True)
        case("affinity_first", chat("hello"), "cheap", session="gauntlet-affinity")
        case("affinity_new_requirement", {"model": "smart", "messages": [{"role": "user", "content": "hello"}, {"role": "assistant", "content": "hello"}, {"role": "user", "content": "complex reasoning"}], "reasoning_effort": "high"}, "strong", session="gauntlet-affinity")
        case("semantic_retry", chat("complex reasoning", fixture_fail=True), scope="gateway_retry_outside_plugin_hook", oracle_note="Hook chooses strong; only a gateway/host sees upstream 503. No Hermes host retry behavior is asserted.")
        case("sse_bytes", chat("stream test", stream=True), "cheap", scope="request_stream_flag_matched_response_relay_outside_plugin", oracle_note="Stream request survives. Hook never receives response/SSE bytes; byte relay cannot be judged here.")
        case("malformed_safe_fallback", chat("malformed", tools=[{"type": "function", "function": {"name": "lookup"}}]), "strong", required_tools=True)
        case("malformed_fail", chat("malformed"), scope="configurable_fail_policy_not_offered_by_plugin", oracle_note="Plugin fail-open retains configured cheap; it has no counterpart to Torii on_failure=fail.")
        case("paid_policy", chat("hello"), expected_block=True, scope="paid_model_policy_not_offered_by_plugin", oracle_note="All matched models are paid/cloud. Plugin has no allow_paid_models control; its output cannot satisfy external free-only constraint.")
        case("explicit_bypasses_classifier", {"model": "gauntlet/cheap", "messages": [{"role": "user", "content": "complex reasoning"}]}, scope="virtual_selector_semantics_not_offered_by_plugin", oracle_note="Plugin owns configured grid models; Torii explicit physical model bypass has no exact counterpart. Off-grid host requests are tested separately.")
        case("contextual_first", chat("complex reasoning about a proof"), "strong", session="contextual", scope="supplemental_context_or_tool_replay")
        case("contextual_followup", {"model": "smart", "messages": [{"role": "user", "content": "complex reasoning about a proof"}, {"role": "assistant", "content": "We need a proof."}, {"role": "user", "content": "Continue that proof."}]}, "strong", session="contextual", scope="supplemental_context_or_tool_replay", oracle_note="Additional inspection case, not scored one-to-one until Torii counterpart runs. Same oracle can see bounded recent_context in actual plugin wire state.")
        case("tool_loop_replay", {"model": "smart", "messages": [{"role": "user", "content": "complex reasoning about a proof"}, {"role": "assistant", "tool_calls": [{"id": "call-a", "type": "function", "function": {"name": "lookup", "arguments": "{}"}}]}, {"role": "tool", "tool_call_id": "call-a", "content": "result"}]}, "strong", session="contextual", turn="contextual_followup", required_tools=True, scope="supplemental_context_or_tool_replay")
        # Eligibility controls: reference operates normally with known catalog;
        # unsupported provider/API/off-grid gates are observed, not presumed.
        gate_rows = []
        for name, provider, api_mode, model in [("other_provider", "openai", "chat_completions", "cheap"), ("other_endpoint", "ollama-cloud", "responses", "cheap"), ("off_grid_explicit", "ollama-cloud", "chat_completions", "operator-special")]:
            payload = {"model": model, "messages": [{"role": "user", "content": "complex reasoning"}]}
            before = len(transport.records)
            output = router.on_llm_request(request=payload, original_request=payload, model=model, provider=provider, api_mode=api_mode, turn_id=name)
            assert output is None and len(transport.records) == before
            gate_rows.append({"case": name, "unchanged": True, "decision_calls": 0})
        assert all(row["request_fields_preserved_except_model_effort"] for row in rows)
        assert all(row["matches_expected_model"] for row in rows if row["case"] in {"simple", "complex", "affinity_first", "affinity_new_requirement", "contextual_first", "contextual_followup", "tool_loop_replay"})
        assert rows[-1]["decision_calls"] == 0 and rows[-1]["audit"]["replayed"] is True

    default = modules["config"].load_settings()
    matched = [row for row in rows if row["scope"] == "matched_routing"]
    report = {
        "reference": {"repository": REPO, "pin": PIN, "source_sha256": HASHES,
                      "executed_unmodified_modules": list(HASHES)[:-1], "registration_adapter": "namespace package; host callback arguments; supported injected HTTP transport; actual cached ModelCatalog"},
        "synthetic_upstreams": True, "production_calls": 0, "production_quality_or_savings_proven": False,
        "host_adapter": {"provider": "ollama-cloud", "api_mode": "chat_completions", "virtual_model_mapping": "smart -> host configured cheap; gauntlet/name -> name", "real_host_install_executed": False},
        "fairness": {"pool_matches_torii_container_fixture": POOL, "catalog": [m["id"] for m in POOL], "all_grid_models_verified_present": True,
                     "protocol_translation": "Same hard=(complex|reasoning) oracle; plugin model/effort choices differ from Torii task/complexity/features. All raw requests/responses recorded.",
                     "adversarial_classifier_case": "Objective tools are intentionally absent from the weak semantic oracle, exactly as in Torii's frozen fixture. This measures constraint enforcement, not Jev routing quality.",
                     "timing": "Hook CPU time with instantaneous injected transport; not comparable to Torii HTTP request latency or live Jev latency."},
        "setup": {"observed_adapter_steps": ["verify ten pinned source hashes", "create cached provider catalog listing the three matched models", "load supported custom grid/default_model via load_settings", "inject deterministic decision transport and host turn/session IDs", "execute real Router.on_llm_request"],
                  "operator_steps_from_pinned_readme_not_observed": ["install plugin (scan may require operator force override)", "enable plugin", "supply OpenRouter key", "restart Hermes session"],
                  "default_settings_observed": {"models": list(default.grid_ids), "timeout_s": default.timeout_s, "minimum_confidence": default.confidence_threshold, "context_turns": default.context_turns, "route_per_turn": default.route_per_turn},
                  "matched_configuration_overrides": settings_dict,
                  "observed_full_setup_parity_established": False},
        "cost": {"actual_classifier_dollars": None, "actual_inference_dollars": None, "synthetic_response_usage_per_valid_decision": {"input_tokens": 120, "output_tokens": 20}, "plugin_audit_exports_usage": False, "estimated_live_savings": None},
        "cases": rows, "gate_checks": gate_rows,
        "matched_routing_cases": len(matched), "matched_expected_model_count": sum(row["matches_expected_model"] is True for row in matched),
        "matched_model_mismatches": [row["case"] for row in matched if row["matches_expected_model"] is False],
        "hook_latency_ms_p50": round(statistics.median(row["hook_elapsed_ms"] for row in rows), 6),
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps({"pin": PIN, "executed": len(rows), "matched_cases": len(matched), "matched_model_mismatches": report["matched_model_mismatches"], "gate_checks": len(gate_rows), "output": str(args.output)}))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

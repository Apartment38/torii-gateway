#!/usr/bin/env python3
"""Exercise the running Podman gateway with temporary, self-contained upstreams.

Reads administrator credentials from .env, restores configuration, and revokes
its temporary client key. The synthetic upstreams verify routing, not LLM quality.
"""
import argparse
import base64
import copy
import json
import math
import os
import pathlib
import statistics
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class Fixture(BaseHTTPRequestHandler):
    records = []

    def log_message(self, *_):
        pass

    def reply(self, value, status=200, content_type="application/json"):
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.end_headers()
        self.wfile.write(value if isinstance(value, bytes) else json.dumps(value).encode())

    def do_GET(self):
        if self.path == "/setup/v1/models":
            self.reply({"data": [{"id": "strong"}]})
        elif self.path == "/v1/models":
            self.reply({"data": [{"id": m} for m in ["cheap", "strong", "backup", "decision"]]})
        else:
            self.reply({"error": "fixture endpoint absent"}, 404)

    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        self.records.append({"path": self.path, "body": body})
        if self.path == "/setup/api/show":
            self.reply({"capabilities": ["completion", "tools", "vision", "thinking"],
                        "model_info": {"general.architecture": "llama", "llama.context_length": 100000}})
            return
        if self.path.rstrip("/") == "/v1/systemone":
            state = str(body.get("state", ""))
            if "malformed" in state:
                self.reply({"answers": {"task_type": None}})
                return
            if "slow-fixture" in state:
                time.sleep(.15)
            hard = "complex" in state or "reasoning" in state
            answers = {}
            for name, question in body["questions"].items():
                if question["type"] == "choice":
                    choice = "high" if hard else "low"
                    if name == "task_type":
                        choice = "reasoning" if hard else "general"
                    answers[name] = {"type": "choice", "choice": choice, "confidence": .99,
                                     "probabilities": {k: float(k == choice) for k in question["criteria"]}}
                else:
                    answers[name] = {"type": "noul", "noul": float(name == "reasoning" and hard)}
            usage = {"input_tokens": 120, "output_tokens": 20}
            if "missing-usage" in state:
                usage.pop("output_tokens")
            self.reply({"model": "fixture-decision", "answers": answers, "usage": usage})
            return
        if body.get("model") == "strong" and body.get("fixture_fail"):
            self.reply({"error": "fixture failure"}, 503)
        elif body.get("stream"):
            self.reply(b'data: {"delta":"fixture","opaque":true}\n\ndata: [DONE]\n\n',
                       content_type="text/event-stream")
        else:
            self.reply({"model": body.get("model"), "echo": body,
                        "choices": [{"message": {"content": "synthetic test output"}}],
                        "usage": {"prompt_tokens": 10, "completion_tokens": 5}})


def environment():
    values = {}
    for line in pathlib.Path(".env").read_text().splitlines():
        if "=" in line and not line.lstrip().startswith("#"):
            key, value = line.split("=", 1)
            values[key.strip()] = value.strip().strip("\"'")
    return values


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", default="/tmp/torii-container-results.json")
    parser.add_argument("--live-jev", action="store_true", help="One real Jev decision using the saved provider")
    args = parser.parse_args()
    values = environment()
    base = "http://127.0.0.1:" + values.get("TORII_PORT", "8080")
    basic = "Basic " + base64.b64encode((values["TORII_USER"] + ":" + values["TORII_PW"]).encode()).decode()

    def request(path, payload=None, method=None, auth=basic, headers=None):
        h = {"Authorization": auth, "Content-Type": "application/json"}
        h.update(headers or {})
        req = urllib.request.Request(base + path, data=None if payload is None else json.dumps(payload).encode(),
                                     headers=h, method=method)
        started = time.perf_counter()
        try:
            response = urllib.request.urlopen(req, timeout=35)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            data = response.read()
            try:
                data = json.loads(data)
            except (ValueError, UnicodeDecodeError):
                pass
            return response.code, data, dict(response.headers), (time.perf_counter() - started) * 1000

    status, original, config_headers, _ = request("/gateway/v1/config")
    revision = next(value for name, value in config_headers.items() if name.lower() == "etag")
    assert status == 200, "Administrator login failed"
    server = ThreadingHTTPServer(("0.0.0.0", 0), Fixture)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    url = "http://host.containers.internal:" + str(server.server_port)
    results = []
    key_id = None
    edited = False
    catalog_path = "/tmp/torii-gauntlet-catalog.json"
    descriptors = []
    for name, quality, price in [("cheap", .6, .1), ("strong", .99, 2), ("backup", .99, 4)]:
        descriptors.append({"id": "gauntlet/" + name, "provider_id": "gauntlet", "upstream_id": name, "context_window": 100000, "local": False,
                            "endpoints": {"chat_completions": "supported", "responses": "supported", "completions": "supported"},
                            "capabilities": {"streaming": "supported", "tools": "supported" if name != "cheap" else "unsupported",
                                             "vision": "supported", "structured_output": "supported", "reasoning": "supported" if name != "cheap" else "unsupported"},
                            "quality": {"general": quality},
                            "pricing": {"input_per_million": price, "output_per_million": price}})
    try:
        with tempfile.NamedTemporaryFile(mode="w", suffix=".json") as f:
            json.dump(descriptors, f)
            f.flush()
            os.chmod(f.name, 0o644)
            subprocess.run(["podman", "cp", f.name, "torii-gateway:" + catalog_path], check=True, capture_output=True)
        status, created, _, _ = request("/gateway/v1/admin/api-keys", {"name": "Gauntlet temporary test"})
        assert status == 201, "Temporary test credential failed"
        key_id = created["key"]["id"]
        bearer = "Bearer " + created["api_key"]
        cfg = copy.deepcopy(original)
        cfg["providers"] = [
            {"id": "gauntlet", "type": "openai_compatible", "base_url": url,
             "models": ["cheap", "strong", "backup"], "discover_models": False},
            {"id": "gauntlet-judge", "type": "jev_compatible", "base_url": url,
             "models": ["decision"], "discover_models": False}]
        cfg["router"] = {"catalog_file": catalog_path, "classifier": {"type": "jev", "transport": "provider",
                         "provider": "gauntlet-judge", "model": "decision", "on_failure": "capability_fallback"},
                         "sessions": {"on_model_failure": "reroute"}}
        cfg["intelligence"] = {"enabled": False}

        def save():
            nonlocal edited, revision
            status, _, _, _ = request("/gateway/v1/config", cfg, "PUT", headers={"If-Match": revision})
            assert status == 200, "Temporary configuration failed validation/reload"
            edited = True
            status, _, config_headers, _ = request("/gateway/v1/config")
            assert status == 200, "Failed to read saved configuration revision"
            revision = next(value for name, value in config_headers.items() if name.lower() == "etag")

        def case(name, body, expected, path="/v1/chat/completions", session=None, expected_decisions=None):
            before = len(Fixture.records)
            status, data, h, elapsed = request(path, body, auth=bearer,
                headers={"X-Session-ID": session or "gauntlet-" + name})
            calls = Fixture.records[before:]
            inference = [r["body"]["model"] for r in calls if r["path"].endswith("/chat/completions") or r["path"] in ("/v1/responses", "/v1/completions")]
            ok = status == expected if isinstance(expected, int) else status == 200 and inference and inference[-1] == expected
            if body.get("stream") and status == 200:
                ok = ok and data == b'data: {"delta":"fixture","opaque":true}\n\ndata: [DONE]\n\n'
            if isinstance(data, dict) and "echo" in data:
                forwarded = dict(data["echo"])
                forwarded["model"] = body["model"]
                ok = ok and forwarded == body
            decisions = [r["body"] for r in calls if r["path"] == "/v1/systemone"]
            if expected_decisions is not None:
                ok = ok and len(decisions) == expected_decisions
            if isinstance(expected, int) and expected >= 400:
                ok = ok and not inference
            if name == "incomplete_usage_unknown":
                ok = ok and not any(k.lower() in ("x-torii-classifier-input-tokens", "x-torii-classifier-output-tokens") for k in h)
            row = {"case": name, "pass": bool(ok), "status": status, "models_called": inference,
                   "decision_calls": len(decisions), "elapsed_ms": round(elapsed, 3),
                   "decision_requests": decisions,
                   "actual_output": data.decode() if isinstance(data, bytes) else data,
                   "routing_headers": {k: v for k, v in h.items() if "torii" in k.lower()}}
            results.append(row)
            print(json.dumps(row))

        def chat(text, **fields):
            return {"model": "smart", "messages": [{"role": "user", "content": text}], **fields}

        save()
        status, models, _, _ = request("/v1/models", auth=bearer)
        assert status == 200 and all(not m["id"].startswith("gauntlet-judge/") for m in models["data"])
        case("simple", chat("hello", opaque={"preserve": True}), "cheap")
        case("complex", chat("complex reasoning"), "strong")
        case("objective_tools", chat("tools", tools=[{"type": "function", "function": {"name": "lookup"}}]), "strong")
        case("affinity_first", chat("hello"), "cheap", session="gauntlet-affinity")
        case("affinity_new_requirement", {"model": "smart", "messages": [{"role": "user", "content": "hello"},
             {"role": "assistant", "content": "hello"}, {"role": "user", "content": "complex reasoning"}],
             "reasoning_effort": "high"}, "strong", session="gauntlet-affinity")
        case("semantic_retry", chat("complex reasoning", fixture_fail=True), "backup")
        case("sse_bytes", chat("stream test", stream=True), "cheap")
        case("malformed_safe_fallback", chat("malformed", tools=[{"type": "function", "function": {"name": "lookup"}}]), "strong")
        cfg["router"]["classifier"]["on_failure"] = "fail"
        save()
        case("malformed_fail", chat("malformed"), 502)
        cfg["router"]["allow_paid_models"] = False
        save()
        case("paid_policy", chat("hello"), 503)
        case("explicit_bypasses_classifier", {"model": "gauntlet/cheap", "messages": [{"role": "user", "content": "complex reasoning"}]}, "cheap")
        cfg["router"].pop("allow_paid_models", None)
        save()
        case("contextual_followup", {"model": "smart", "messages": [
            {"role": "system", "content": "Help with complex reasoning"},
            {"role": "user", "content": "Explain a complex mathematical proof"},
            {"role": "assistant", "content": "Shall I give the complete reasoning?"},
            {"role": "user", "content": "Yes, do that."}]}, "strong")
        case("incomplete_usage_unknown", chat("missing-usage"), "cheap")
        continuation = {"model": "smart", "messages": [
            {"role": "user", "content": "tools"},
            {"role": "assistant", "tool_calls": [{"id": "fixture_call", "type": "function", "function": {"name": "lookup", "arguments": "{}"}}]},
            {"role": "tool", "tool_call_id": "fixture_call", "content": "fixture result"}]}
        case("unbound_tool_continuation", continuation, 503, expected_decisions=0)
        case("tool_affinity_first", chat("tools", tools=[{"type": "function", "function": {"name": "lookup"}}]), "strong", session="gauntlet-tools")
        case("tool_affinity_continuation", continuation, "strong", session="gauntlet-tools", expected_decisions=0)
        cfg["router"]["sessions"]["inactivity_timeout"] = "1ns"
        save()
        case("expiring_tool_first", chat("tools", tools=[{"type": "function", "function": {"name": "lookup"}}]), "strong", session="gauntlet-expiry")
        time.sleep(.01)
        case("expired_tool_continuation", continuation, 503, session="gauntlet-expiry", expected_decisions=0)
        if args.live_jev:
            live = next((p for p in original["providers"] if p["type"] == "jev"), None)
            assert live, "No saved real Jev provider"
            cfg["providers"].append(live)
            cfg["router"].pop("allow_paid_models", None)
            cfg["router"]["classifier"] = {"type": "jev", "transport": "provider", "provider": live["id"],
                                            "model": "jev-latest", "on_failure": "fail"}
            save()
            case("live_jev_simple", chat("Reply with a short friendly greeting."), "cheap")
            case("live_jev_context", {"model": "smart", "reasoning_effort": "high", "messages": [
                {"role": "system", "content": "You help prove mathematical theorems"},
                {"role": "user", "content": "Prove the spectral theorem with all intermediate lemmas"},
                {"role": "assistant", "content": "I can develop a complete rigorous proof. Shall I proceed?"},
                {"role": "user", "content": "Yes, do that."}]}, "strong")
        cfg["providers"] = [
            {"id": "gauntlet-ollama", "type": "ollama", "base_url": url + "/setup"},
            {"id": "gauntlet-judge", "type": "jev_compatible", "base_url": url, "models": ["decision"], "discover_models": False}]
        cfg["router"] = {"classifier": {"type": "jev", "transport": "provider", "provider": "gauntlet-judge", "model": "decision", "on_failure": "fail"}}
        save()
        case("minimal_setup_discovery", chat("tools", tools=[{"type": "function", "function": {"name": "lookup"}}]), "strong")
    finally:
        if edited:
            status, _, _, _ = request("/gateway/v1/config", original, "PUT", headers={"If-Match": revision})
            assert status == 200, "RESTORE FAILED: recover original configuration before continuing"
            status, restored, _, _ = request("/gateway/v1/config")
            assert status == 200 and restored == original, "Configuration restoration did not match"
        if key_id:
            status, _, _, _ = request("/gateway/v1/admin/api-keys/" + key_id, method="DELETE")
            assert status == 200, "Temporary API key revocation failed"
        server.shutdown()
        server.server_close()
    elapsed = sorted(r["elapsed_ms"] for r in results)
    report = {"synthetic_upstreams": True, "production_quality_or_savings_proven": False,
              "decision_calls_note": "decision_calls counts local fixture calls; live Jev calls are evidenced by classifier_source=called, model/version and measured token headers.",
              "configuration_restored": True, "temporary_key_revoked": True, "cases": results,
              "minimal_setup_observed": {"provider_definitions": 2, "configuration_save_reload_calls": 1,
                                         "catalog_file_required": False, "model_overrides_required": False,
                                         "fixture_metadata": True, "credentials_preexisting": True},
              "passed": sum(r["pass"] for r in results), "total": len(results),
              "request_latency_ms": {"p50": statistics.median(elapsed), "p95": elapsed[max(0, math.ceil(.95 * len(elapsed)) - 1)]}}
    pathlib.Path(args.output).write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps({k: v for k, v in report.items() if k != "cases"}))
    return 0 if all(r["pass"] for r in results) else 1


if __name__ == "__main__":
    raise SystemExit(main())

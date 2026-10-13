import contextlib
import io
import json
import os
import sys
import tempfile
import unittest
import unittest.mock

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import candidates  # noqa: E402
import check  # noqa: E402
import drive  # noqa: E402
import providers  # noqa: E402
from tests.fakes import FakeMcp, FakeProvider  # noqa: E402

TOOLS = [{"name": "search_context", "description": "d", "inputSchema": {"type": "object"}}]
CHAT_USAGE = {"prompt_tokens": 10, "completion_tokens": 4, "prompt_tokens_details": {"cached_tokens": 3}}


def chat(content=None, calls=(), usage=CHAT_USAGE):
    message = {"role": "assistant", "content": content}
    if calls:
        message["tool_calls"] = [
            {"id": call_id, "type": "function", "function": {"name": name, "arguments": arguments}}
            for call_id, name, arguments in calls
        ]
    body = {"choices": [{"message": message}]}
    if usage is not None:
        body["usage"] = usage
    return 200, body


def call_search(call_id="c1", arguments='{"q":"x"}'):
    return chat(calls=[(call_id, "search_context", arguments)])


class Bridge(unittest.TestCase):
    def drive(self, script, candidate="mistral", via="api", replies=None, instructions="be careful"):
        out = tempfile.NamedTemporaryFile(suffix=".jsonl", delete=False).name
        with FakeMcp(TOOLS, instructions, replies) as mcp, FakeProvider(script) as provider:
            route = candidates.resolve(candidate, via)._replace(base_url=provider.url)
            with contextlib.redirect_stderr(io.StringIO()) as self.stderr:
                code = drive.run(route, "k", mcp.url, "tok", "who complained?", out, sleep=lambda _s: None)
        self.requests = [body for _path, _headers, body in provider.requests]
        self.headers = [{k.lower(): v for k, v in headers.items()} for _path, headers, _body in provider.requests]
        with open(out, encoding="utf-8") as handle:
            self.raw = handle.read()
        return code, out

    def assert_scored_answer(self, out):
        called, said, calls = check.read_transcript(out)
        self.assertTrue(check.tool_matches(called, "search_context"))
        self.assertEqual(calls[0][1], {"q": "x"})
        self.assertIn("September.", said)
        self.assertEqual(check.unrun(out), "")


class ChatWireTest(Bridge):
    def test_tool_call_then_answer_scores_through_check(self):
        code, out = self.drive([call_search(), chat("September.")])
        self.assertEqual(code, 0)
        self.assert_scored_answer(out)
        self.assertEqual(check.read_usage(out)[0]["input_tokens"], 14)
        self.assertEqual(check.read_usage(out)[0]["cache_read_input_tokens"], 6)
        self.assertEqual(self.requests[0]["messages"][0], {"role": "system", "content": "be careful"})
        self.assertEqual(self.requests[1]["messages"][-1],
                         {"role": "tool", "tool_call_id": "c1", "content": "ok"})
        self.assertEqual(self.headers[0]["authorization"], "Bearer k")

    def test_text_beside_tool_calls_is_kept(self):
        _code, out = self.drive([chat("Let me look.", [("c1", "search_context", "{}")]), chat("Done.")])
        self.assertIn("Let me look.", check.read_transcript(out)[1])

    def test_no_tool_call_is_a_scored_run(self):
        code, out = self.drive([chat("From memory: October.")])
        self.assertEqual((code, check.unrun(out)), (0, ""))
        self.assertEqual(check.read_transcript(out)[0], [])

    def test_401_at_the_door_is_a_harness_fault_and_unrun(self):
        code, out = self.drive([(401, {"error": {"message": "invalid key"}})])
        self.assertEqual(code, 3)
        self.assertNotEqual(check.unrun(out), "")
        self.assertIn("401", self.raw)

    def test_mid_run_5xx_after_retries_is_a_harness_fault(self):
        code, _out = self.drive([call_search()] + [(503, {})] * 3)
        self.assertEqual(code, 3)
        self.assertEqual(len(self.requests), 4)

    def test_a_transient_5xx_is_retried_and_the_run_goes_on(self):
        code, out = self.drive([(503, {}), call_search(), chat("September.")])
        self.assertEqual(code, 0)
        self.assert_scored_answer(out)

    def test_a_schema_rejection_is_a_harness_fault(self):
        code, _out = self.drive([(400, {"error": {"message": "invalid schema for function"}})])
        self.assertEqual(code, 3)
        self.assertEqual(len(self.requests), 1)

    def test_a_200_carrying_an_error_body_is_retried_then_a_harness_fault(self):
        code, _out = self.drive([(200, {"error": {"message": "upstream provider error", "code": 502}})] * 3)
        self.assertEqual((code, len(self.requests)), (3, 3))

    def test_a_brokers_transient_200_error_is_retried_and_the_run_goes_on(self):
        code, out = self.drive([(200, {"error": {"message": "upstream"}}), call_search(), chat("September.")])
        self.assertEqual(code, 0)
        self.assert_scored_answer(out)

    def test_a_200_that_is_not_json_is_retried_then_a_harness_fault(self):
        code, _out = self.drive([(200, "<html>proxy</html>")] * 3)
        self.assertEqual((code, len(self.requests)), (3, 3))

    def test_a_truncated_body_is_retried(self):
        import http.client
        real = providers.urllib.request.urlopen
        calls = []

        def flaky(request, *args, **kwargs):
            # Only the model's endpoint truncates; the MCP handshake is untouched.
            if request.full_url.endswith("/chat/completions"):
                calls.append(1)
                if len(calls) == 1:
                    raise http.client.IncompleteRead(b"partial")
            return real(request, *args, **kwargs)

        with unittest.mock.patch.object(providers.urllib.request, "urlopen", flaky):
            code, _out = self.drive([chat("September.")])
        self.assertEqual((code, len(calls)), (0, 2))

    def test_turn_cap_is_a_finding(self):
        code, out = self.drive([call_search(f"c{i}") for i in range(drive.MAX_TURNS)])
        self.assertEqual((code, check.unrun(out)), (0, ""))
        self.assertIn("turn cap", self.raw)

    def test_bad_arguments_reach_the_model_as_a_tool_error(self):
        code, _out = self.drive([call_search(arguments="{not json"), chat("Sorry.")])
        self.assertEqual(code, 0)
        self.assertIn("not valid JSON", self.requests[1]["messages"][-1]["content"])

    def test_product_refusal_reaches_the_model_and_the_run_goes_on(self):
        code, out = self.drive([call_search(), chat("September.")],
                               replies={"search_context": ("permission denied", True)})
        self.assertEqual(code, 0)
        self.assertIn('"is_error": true', self.raw)

    def test_oversize_tool_result_is_truncated_with_marker(self):
        _code, _out = self.drive([call_search(), chat("ok")],
                                 replies={"search_context": ("x" * 200000, False)})
        sent = self.requests[1]["messages"][-1]["content"]
        self.assertLessEqual(len(sent), drive.RESULT_CAP + 100)
        self.assertIn("truncated by the e2e-llm bridge", sent)

    def test_missing_usage_makes_the_run_unmeasured(self):
        _code, out = self.drive([chat(calls=[("c1", "search_context", "{}")], usage=None), chat("ok")])
        self.assertIsNone(check.read_usage(out))

    def test_an_empty_reply_is_a_finding_not_a_fault(self):
        code, out = self.drive([chat("")])
        self.assertEqual((code, check.unrun(out)), (0, ""))
        self.assertIn("no answer", self.raw)

    def test_the_system_line_names_what_was_offered(self):
        _code, _out = self.drive([chat("ok")])
        self.assertIn('"tools": ["mcp__margince_e2e_llm__search_context"]', self.raw)
        self.assertIn('"system_prompt": "mcp-instructions"', self.raw)


class OpenRouterTest(Bridge):
    def test_routing_and_usage_accounting_are_requested(self):
        usage = dict(CHAT_USAGE, cost=0.002)
        _code, out = self.drive([chat("ok", usage=usage)], via="openrouter")
        self.assertEqual(self.requests[0]["provider"], {"only": ["mistral/eu"], "require_parameters": True})
        self.assertEqual(self.requests[0]["usage"], {"include": True})
        self.assertEqual(self.requests[0]["model"], "mistralai/mistral-medium-3-5")
        self.assertEqual(check.read_usage(out)[1], 0.002)

    def test_no_parameter_a_routed_endpoint_may_lack_is_sent(self):
        # Under require_parameters the broker drops every endpoint that cannot
        # honour a parameter: Mistral's declare no parallel_tool_calls, so
        # sending it leaves no endpoint at all.
        self.drive([chat("ok")], via="openrouter")
        self.assertNotIn("parallel_tool_calls", self.requests[0])

    def test_an_effort_experiment_reaches_the_request(self):
        out = tempfile.NamedTemporaryFile(suffix=".jsonl", delete=False).name
        with FakeMcp(TOOLS, "") as mcp, FakeProvider([chat("ok")]) as provider:
            route = candidates.resolve("mistral", "openrouter", effort="reasoning high",
                                       folder="m-high")._replace(base_url=provider.url)
            with contextlib.redirect_stderr(io.StringIO()):
                drive.run(route, "k", mcp.url, "tok", "q", out, sleep=lambda _s: None)
        self.assertEqual(provider.requests[0][2]["reasoning"], {"effort": "high"})

    def test_gpt_on_openrouter_asks_for_the_pinned_effort(self):
        self.drive([chat("ok")], candidate="gpt", via="openrouter")
        self.assertEqual(self.requests[0]["reasoning"], {"effort": "medium"})


def responses(items, usage=None):
    return 200, {"output": items, "usage": usage or {
        "input_tokens": 10, "output_tokens": 4, "input_tokens_details": {"cached_tokens": 3}}}


class ResponsesWireTest(Bridge):
    def test_tool_call_then_answer_scores_through_check(self):
        reasoning = {"type": "reasoning", "id": "r1", "encrypted_content": "opaque"}
        call = {"type": "function_call", "call_id": "c1", "name": "search_context", "arguments": '{"q":"x"}'}
        answer = {"type": "message", "role": "assistant",
                  "content": [{"type": "output_text", "text": "September."}]}
        code, out = self.drive([responses([reasoning, call]), responses([answer])], candidate="gpt")
        self.assertEqual(code, 0)
        self.assert_scored_answer(out)
        first, second = self.requests
        self.assertEqual(first["instructions"], "be careful")
        self.assertEqual(first["reasoning"], {"effort": "medium"})
        self.assertFalse(first["tools"][0]["strict"])
        self.assertIn(reasoning, second["input"])
        self.assertEqual(second["input"][-1], {"type": "function_call_output", "call_id": "c1", "output": "ok"})
        self.assertEqual(check.read_usage(out)[0]["input_tokens"], 14)


def messages(blocks):
    return 200, {"content": blocks, "usage": {
        "input_tokens": 10, "output_tokens": 4, "cache_read_input_tokens": 3,
        "cache_creation_input_tokens": 1}}


class MessagesWireTest(Bridge):
    def test_tool_call_then_answer_scores_through_check(self):
        code, out = self.drive([
            messages([{"type": "tool_use", "id": "c1", "name": "search_context", "input": {"q": "x"}}]),
            messages([{"type": "text", "text": "September."}]),
        ], candidate="claude")
        self.assertEqual(code, 0)
        self.assert_scored_answer(out)
        first, second = self.requests
        self.assertEqual(first["system"][0]["text"], "be careful")
        self.assertEqual(first["tools"][0]["input_schema"], {"type": "object"})
        self.assertEqual(second["messages"][-1]["content"][0]["tool_use_id"], "c1")
        self.assertEqual(self.headers[0]["x-api-key"], "k")
        self.assertEqual(check.read_usage(out)[0]["cache_creation_input_tokens"], 2)


class McpFailureTest(unittest.TestCase):
    def test_an_unreachable_mcp_server_is_a_harness_fault_with_a_failed_status(self):
        out = tempfile.NamedTemporaryFile(suffix=".jsonl", delete=False).name
        route = candidates.resolve("mistral", "api")
        with contextlib.redirect_stderr(io.StringIO()):
            code = drive.run(route, "k", "http://127.0.0.1:9/mcp", "tok", "q", out, sleep=lambda _s: None)
        self.assertEqual(code, 3)
        with open(out, encoding="utf-8") as handle:
            self.assertIn('"status": "failed"', handle.read())



def markers(body):
    return json.dumps(body).count('"cache_control"')


class CacheTest(Bridge):
    """Claude caches only what a request marks, so the bridge marks the system
    prompt (which caches the tools ahead of it) and the newest message; the
    other vendors cache on their own and are sent no marker."""

    TWO_TURNS = [call_search(), chat("September.")]

    def test_claude_on_openrouter_marks_system_and_newest_message(self):
        self.drive(self.TWO_TURNS, candidate="claude", via="openrouter")
        first, second = self.requests
        system = first["messages"][0]
        self.assertEqual(system["role"], "system")
        self.assertEqual(system["content"][-1]["cache_control"], {"type": "ephemeral"})
        self.assertEqual(system["content"][-1]["text"], "be careful")
        self.assertEqual(second["messages"][-1]["content"][-1]["cache_control"], {"type": "ephemeral"})
        # Rolling, not accumulating: Anthropic refuses a fifth breakpoint.
        self.assertEqual((markers(first), markers(second)), (2, 2))

    def test_claude_on_its_own_api_marks_system_and_newest_message(self):
        self.drive([
            messages([{"type": "tool_use", "id": "c1", "name": "search_context", "input": {"q": "x"}}]),
            messages([{"type": "text", "text": "September."}]),
        ], candidate="claude")
        first, second = self.requests
        self.assertEqual(first["system"][-1]["cache_control"], {"type": "ephemeral"})
        self.assertEqual(second["messages"][-1]["content"][-1]["cache_control"], {"type": "ephemeral"})
        self.assertEqual((markers(first), markers(second)), (2, 2))

    def test_the_other_vendors_get_no_marker(self):
        for candidate in ("gpt", "mistral"):
            self.drive(list(self.TWO_TURNS), candidate=candidate, via="openrouter")
            self.assertEqual([markers(body) for body in self.requests], [0, 0], candidate)


class ChatUsageTest(Bridge):
    def test_cache_writes_are_not_also_counted_as_input(self):
        # A broker's prompt_tokens covers the cached and the newly cached
        # prefix too; input is what was neither, as the claude CLI reports it.
        usage = {"prompt_tokens": 4451, "completion_tokens": 66,
                 "prompt_tokens_details": {"cached_tokens": 0, "cache_write_tokens": 4447}}
        _code, out = self.drive([chat("ok", usage=usage)], candidate="claude", via="openrouter")
        counted = check.read_usage(out)[0]
        self.assertEqual((counted["input_tokens"], counted["cache_creation_input_tokens"]), (4, 4447))


class MainTest(unittest.TestCase):
    def test_a_prompt_file_outside_the_repo_and_temp_is_refused(self):
        argv = ["drive.py", "--candidate", "mistral", "--via", "api", "--mcp-url", "http://127.0.0.1:9/mcp",
                "--token-env", "T", "--prompt-file", "/etc/hosts", "--out", tempfile.mktemp(suffix=".jsonl")]
        with unittest.mock.patch.object(sys, "argv", argv), contextlib.redirect_stderr(io.StringIO()) as err:
            self.assertEqual(drive.main(), drive.HARNESS)
        self.assertIn("outside the repo", err.getvalue())


if __name__ == "__main__":
    unittest.main()

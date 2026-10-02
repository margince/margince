import json
import os
import sys
import tempfile
import unittest
import unittest.mock

HERE = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, HERE)

import check  # noqa: E402
import transcript  # noqa: E402

SAMPLE = os.path.join(HERE, "testdata", "codex", "list-pipelines.jsonl")
OFFERED = ["list_pipelines", "search_context"]


def convert(lines):
    out = tempfile.NamedTemporaryFile(suffix=".jsonl", delete=False).name
    code = transcript.from_codex(lines, out, "gpt-5.6-sol", "gpt:cli", OFFERED)
    with open(out, encoding="utf-8") as handle:
        return code, out, handle.read()


def sample():
    with open(SAMPLE, encoding="utf-8") as handle:
        return handle.read().splitlines()


def item(kind, **fields):
    return json.dumps({"type": "item.completed", "item": dict(id="item_9", type=kind, **fields)})


class FromCodexTest(unittest.TestCase):
    def test_a_real_codex_run_scores_through_check(self):
        code, out, _raw = convert(sample())
        self.assertEqual(code, 0)
        called, said, calls = check.read_transcript(out)
        self.assertTrue(check.tool_matches(called, "list_pipelines"))
        self.assertEqual(calls[0][1], {})
        self.assertIn("one pipeline", said)
        self.assertEqual(check.unrun(out), "")
        self.assertEqual(check.offered_problem({"must_call": ["list_pipelines"]}, out), "")

    def test_usage_separates_cached_input(self):
        _code, out, _raw = convert(sample())
        counted, cost = check.read_usage(out)
        self.assertEqual((counted["input_tokens"], counted["cache_read_input_tokens"]), (102413 - 86784, 86784))
        self.assertIsNone(cost)

    def test_the_offered_tools_are_the_lanes_own_listing(self):
        _code, _out, raw = convert(sample())
        self.assertIn('"status": "listed-by-lane"', raw)
        self.assertIn('"system_prompt": "cli-default"', raw)

    def test_a_shell_call_stops_the_run(self):
        lines = sample()
        lines.insert(2, item("command_execution", command="/bin/zsh -lc 'cat answer.txt'", status="completed"))
        code, _out, raw = convert(lines)
        self.assertEqual(code, 3)
        self.assertIn("command_execution", raw)

    def test_an_unknown_item_kind_stops_the_run(self):
        lines = sample()
        lines.insert(2, item("collab_tool_call", tool="spawn_agent"))
        self.assertEqual(convert(lines)[0], 3)

    def test_codex_reading_our_servers_resources_is_allowed(self):
        lines = sample()
        lines.insert(2, item("mcp_tool_call", server="codex", tool="list_mcp_resources", arguments="{}",
                             result={"content": []}, error=None, status="completed"))
        self.assertEqual(convert(lines)[0], 0)

    def test_another_servers_tool_stops_the_run(self):
        lines = sample()
        lines.insert(2, item("mcp_tool_call", server="github", tool="search", arguments="{}",
                             result=None, error=None, status="completed"))
        self.assertEqual(convert(lines)[0], 3)

    def test_an_mcp_startup_error_stops_the_run(self):
        lines = [l for l in sample() if '"mcp_tool_call"' not in l]
        lines.insert(1, item("error", message="MCP client for `margince_e2e_llm` failed to start"))
        self.assertEqual(convert(lines)[0], 3)

    def test_a_run_that_never_reached_the_lanes_server_is_a_harness_fault(self):
        # Codex says nothing about what it attached, and a run whose tools never
        # reached the model reads exactly like one that chose to call nothing.
        lines = [l for l in sample() if '"mcp_tool_call"' not in l]
        code, _out, raw = convert(lines)
        self.assertEqual(code, 3)
        self.assertIn("no call to the lane's server", raw)

    def test_any_error_item_stops_the_run_whatever_its_wording(self):
        lines = sample()
        lines.insert(len(lines) - 1, item("error", message="stream disconnected before completion"))
        self.assertEqual(convert(lines)[0], 3)

    def test_a_failed_turn_is_a_harness_fault(self):
        lines = [l for l in sample() if "turn.completed" not in l]
        lines.append(json.dumps({"type": "turn.failed", "error": {"message": "401 Unauthorized"}}))
        self.assertEqual(convert(lines)[0], 3)

    def test_a_run_that_never_finished_is_a_harness_fault(self):
        lines = [l for l in sample() if "turn.completed" not in l]
        self.assertEqual(convert(lines)[0], 3)

    def test_a_failed_mcp_call_reaches_the_transcript_as_an_error(self):
        lines = sample()
        lines.insert(2, item("mcp_tool_call", server=transcript.SERVER, tool="search_context",
                             arguments='{"q":"x"}', result=None, error={"message": "permission denied"},
                             status="failed"))
        code, _out, raw = convert(lines)
        self.assertEqual(code, 0)
        self.assertIn("permission denied", raw)



class CodexArgumentsTest(unittest.TestCase):
    def test_arguments_codex_sends_as_an_object_are_read(self):
        lines = sample()
        lines.insert(2, item("mcp_tool_call", server=transcript.SERVER, tool="search_context",
                             arguments={"q": "Reply"}, result={"content": [{"type": "text", "text": "hit"}]},
                             error=None, status="completed"))
        code, out, _raw = convert(lines)
        self.assertEqual(code, 0)
        self.assertIn(("search_context", {"q": "Reply"}),
                      [(name.rsplit("__", 1)[-1], args) for name, args in check.read_transcript(out)[2]])


FAKE_CODEX = """#!/usr/bin/env python3
import json, os, sys, time
with open(os.environ["FAKE_CODEX_ARGV"], "w") as f:
    cdir = sys.argv[sys.argv.index("-C") + 1]
    json.dump({"argv": sys.argv[1:], "token": os.environ.get("MARGINCE_E2E_TOKEN"),
               "cdir": os.listdir(cdir)}, f)
if os.environ.get("FAKE_CODEX_SLEEP"):
    time.sleep(float(os.environ["FAKE_CODEX_SLEEP"]))
sys.stdout.write(open(os.environ["FAKE_CODEX_OUT"]).read())
sys.exit(int(os.environ.get("FAKE_CODEX_EXIT", "0")))
"""


class RunCodexTest(unittest.TestCase):
    def setUp(self):
        import drive  # noqa: F401 — imported here so the module-level tests above stay codex-free
        from tests.fakes import FakeMcp
        self.drive = drive
        self.bin = tempfile.mkdtemp()
        with open(os.path.join(self.bin, "codex"), "w", encoding="utf-8") as handle:
            handle.write(FAKE_CODEX)
        os.chmod(os.path.join(self.bin, "codex"), 0o755)
        self.argv_file = os.path.join(self.bin, "argv.json")
        self.env = {"PATH": self.bin + os.pathsep + os.environ["PATH"], "FAKE_CODEX_ARGV": self.argv_file,
                    "FAKE_CODEX_OUT": SAMPLE, "MARGINCE_E2E_TOKEN": "s3cr3t-passport"}
        self.mcp = FakeMcp([{"name": t, "inputSchema": {"type": "object"}} for t in OFFERED])
        self.mcp.__enter__()

    def tearDown(self):
        self.mcp.__exit__(None, None, None)

    def run_codex(self, timeout=60, **env):
        import candidates
        out = tempfile.NamedTemporaryFile(suffix=".jsonl", delete=False).name
        with unittest.mock.patch.dict(os.environ, dict(self.env, **env)):
            code = self.drive.run_codex(candidates.resolve("gpt", "cli"), self.mcp.url, "MARGINCE_E2E_TOKEN",
                                        "List the pipelines.", out, timeout=timeout)
        return code, out

    def test_codex_is_run_mcp_only_from_an_empty_directory(self):
        code, out = self.run_codex()
        self.assertEqual(code, 0)
        with open(self.argv_file, encoding="utf-8") as handle:
            seen = json.load(handle)
        argv = seen["argv"]
        self.assertEqual(seen["token"], "s3cr3t-passport")
        for flag in ("--json", "--strict-config", "--ignore-user-config", "--skip-git-repo-check"):
            self.assertIn(flag, argv)
        self.assertEqual(argv[argv.index("--sandbox") + 1], "read-only")
        self.assertEqual(seen["cdir"], [])
        self.assertIn("features.shell_tool=false", argv)
        # Code mode is the path codex's MCP calls take: switched off, codex
        # attaches the server and can call none of its tools.
        self.assertNotIn("features.code_mode_host=false", argv)
        self.assertIn('mcp_servers.margince_e2e_llm.bearer_token_env_var="MARGINCE_E2E_TOKEN"', argv)
        self.assertNotIn("s3cr3t-passport", " ".join(argv))
        self.assertEqual(argv[-1], "List the pipelines.")
        self.assertTrue(check.tool_matches(check.read_transcript(out)[0], "list_pipelines"))
        self.assertIn("mcp__margince_e2e_llm__search_context", open(out, encoding="utf-8").read())

    def test_a_codex_that_exits_nonzero_is_a_harness_fault_even_with_a_complete_stream(self):
        code, _out = self.run_codex(FAKE_CODEX_EXIT="1")
        self.assertEqual(code, 3)

    def test_the_empty_working_directory_is_removed_after_the_run(self):
        self.run_codex()
        with open(self.argv_file, encoding="utf-8") as handle:
            argv = json.load(handle)["argv"]
        self.assertFalse(os.path.exists(argv[argv.index("-C") + 1]))

    def test_a_codex_that_overruns_is_killed_and_is_a_harness_fault(self):
        code, _out = self.run_codex(timeout=0.5, FAKE_CODEX_SLEEP="5")
        self.assertEqual(code, 3)


if __name__ == "__main__":
    unittest.main()

import json
import os
import shutil
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

    def test_a_call_codex_refused_for_want_of_approval_stops_the_run(self):
        lines = sample()
        lines.insert(2, item("mcp_tool_call", server=transcript.SERVER, tool="create_record", arguments={},
                             result=None, status="failed",
                             error={"message": "MCP tool call requires approval, but approval policy is never"}))
        code, _out, raw = convert(lines)
        self.assertEqual(code, 3)
        self.assertIn("requires approval", raw)

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
home = os.environ.get("CODEX_HOME", "")
auth = os.path.join(home, "auth.json")
with open(os.environ["FAKE_CODEX_ARGV"], "w") as f:
    cdir = sys.argv[sys.argv.index("-C") + 1]
    json.dump({"argv": sys.argv[1:], "token": os.environ.get("MARGINCE_E2E_TOKEN"),
               "cdir": os.listdir(cdir), "home": home, "user_home": os.environ.get("HOME"), "home_listing": sorted(os.listdir(home)) if home else None,
               "auth": open(auth).read() if os.path.isfile(auth) else None,
               "auth_mode": oct(os.stat(auth).st_mode & 0o777) if os.path.isfile(auth) else None}, f)
if os.environ.get("FAKE_OPERATOR_RELOGIN"):
    with open(os.environ["FAKE_OPERATOR_AUTH"], "w") as relogin:
        relogin.write(os.environ["FAKE_OPERATOR_RELOGIN"])
if os.environ.get("FAKE_CODEX_REFRESH"):
    with open(auth, "w") as refreshed:
        refreshed.write(os.environ["FAKE_CODEX_REFRESH"])
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
        # The operator's own home, holding what a clean candidate must not see.
        self.operator_home = tempfile.mkdtemp()
        for name, text in (("auth.json", '{"tokens": "login-v1"}'), ("AGENTS.md", "the operator's rules"),
                           ("memories_1.sqlite", "memory")):
            with open(os.path.join(self.operator_home, name), "w", encoding="utf-8") as handle:
                handle.write(text)
        os.makedirs(os.path.join(self.operator_home, "plugins", "cache"))
        os.makedirs(os.path.join(self.operator_home, "skills", "a-skill"))
        self.env = {"PATH": self.bin + os.pathsep + os.environ["PATH"], "FAKE_CODEX_ARGV": self.argv_file,
                    "FAKE_CODEX_OUT": SAMPLE, "MARGINCE_E2E_TOKEN": "s3cr3t-passport",
                    "CODEX_HOME": self.operator_home}
        self.mcp = FakeMcp([{"name": t, "inputSchema": {"type": "object"}} for t in OFFERED])
        self.mcp.__enter__()

    def tearDown(self):
        self.mcp.__exit__(None, None, None)
        shutil.rmtree(self.operator_home, ignore_errors=True)
        shutil.rmtree(self.bin, ignore_errors=True)

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
        # Codex's default refuses every MCP write in exec mode; the lane's own
        # server is approved, and only it.
        self.assertIn('mcp_servers.margince_e2e_llm.default_tools_approval_mode="approve"', argv)
        # Code mode is the path codex's MCP calls take: switched off, codex
        # attaches the server and can call none of its tools.
        self.assertNotIn("features.code_mode_host=false", argv)
        self.assertIn('mcp_servers.margince_e2e_llm.bearer_token_env_var="MARGINCE_E2E_TOKEN"', argv)
        self.assertNotIn("s3cr3t-passport", " ".join(argv))
        self.assertEqual(argv[-1], "List the pipelines.")
        self.assertTrue(check.tool_matches(check.read_transcript(out)[0], "list_pipelines"))
        self.assertIn("mcp__margince_e2e_llm__search_context", open(out, encoding="utf-8").read())

    def seen(self):
        with open(self.argv_file, encoding="utf-8") as handle:
            return json.load(handle)

    def test_codex_gets_a_home_of_its_own_holding_only_the_login(self):
        self.assertEqual(self.run_codex()[0], 0)
        seen = self.seen()
        self.assertNotEqual(os.path.realpath(seen["home"]), os.path.realpath(self.operator_home))
        self.assertIn("auth.json", seen["home_listing"])
        for operators_own in ("AGENTS.md", "memories_1.sqlite", "plugins", "skills"):
            self.assertNotIn(operators_own, seen["home_listing"])
        self.assertEqual(seen["user_home"], seen["home"])
        self.assertEqual((seen["auth"], seen["auth_mode"]), ('{"tokens": "login-v1"}', "0o600"))
        self.assertNotIn("login-v1", " ".join(seen["argv"]))
        self.assertFalse(os.path.exists(seen["home"]))

    def test_plugins_hooks_and_memories_are_forced_off(self):
        self.run_codex()
        argv = self.seen()["argv"]
        for feature in ("plugins", "hooks", "memories"):
            self.assertIn(f"features.{feature}=false", argv)

    def test_a_login_codex_refreshed_goes_back_to_the_operator(self):
        # The refresh voids the old token, so keeping only the copy would sign the operator out.
        self.run_codex(FAKE_CODEX_REFRESH='{"tokens": "login-v2"}')
        with open(os.path.join(self.operator_home, "auth.json"), encoding="utf-8") as handle:
            self.assertEqual(handle.read(), '{"tokens": "login-v2"}')
        self.assertEqual(sorted(os.listdir(self.operator_home)),
                         ["AGENTS.md", "auth.json", "memories_1.sqlite", "plugins", "skills"])

    def test_a_login_the_operator_renewed_during_the_run_is_kept(self):
        auth = os.path.join(self.operator_home, "auth.json")
        self.run_codex(FAKE_OPERATOR_AUTH=auth, FAKE_OPERATOR_RELOGIN='{"tokens": "operator-v2"}')
        with open(auth, encoding="utf-8") as handle:
            self.assertEqual(handle.read(), '{"tokens": "operator-v2"}')

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

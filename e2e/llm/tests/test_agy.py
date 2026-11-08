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

# A real `agy -p --output-format stream-json` run against a stand-in server
# attached as the lane's server: it reads two tool schemas, calls both tools
# and answers. Only the operator's home and the workspace path were rewritten.
SAMPLE = os.path.join(HERE, "testdata", "agy", "echo-and-sum.jsonl")
OFFERED = ["echo", "get-sum"]
SPILL = "/home/operator/.gemini/antigravity-cli/brain/97ade4ad-ef66-4233-91c4-2852503da362/.system_generated/steps/{}/{}"
OTHER_SPILL = "/home/operator/.gemini/antigravity-cli/brain/0000-other/.system_generated/steps/{}/{}"


def convert(lines):
    out = tempfile.NamedTemporaryFile(suffix=".jsonl", delete=False).name
    code = transcript.from_agy(lines, out, "gemini-3.8-flash-medium", "gemini:cli", OFFERED)
    with open(out, encoding="utf-8") as handle:
        return code, out, handle.read()


def sample():
    with open(SAMPLE, encoding="utf-8") as handle:
        return handle.read().splitlines()


def tool_step(index, name, state="DONE", **params):
    return json.dumps({"event": "step_update", "step_update": {
        "step_index": index, "state": state, "step_type": "tool", "tool_name": name,
        "tool_info": {"name": name, "parameters": params, "output": "1 lines"}}})


def before_answer(lines, *extra):
    """The sample with `extra` steps placed before its closing answer."""
    at = next(i for i, line in enumerate(lines)
              if (json.loads(line).get("step_update") or {}).get("step_index") == 7)
    return lines[:at] + list(extra) + lines[at:]


class FromAgyTest(unittest.TestCase):
    def test_a_real_agy_run_scores_through_check(self):
        code, out, _raw = convert(sample())
        self.assertEqual(code, 0)
        called, said, calls = check.read_transcript(out)
        self.assertTrue(check.tool_matches(called, "echo"))
        self.assertTrue(check.tool_matches(called, "get-sum"))
        self.assertIn(("get-sum", {"a": 2, "b": 3}), [(n.rsplit("__", 1)[-1], a) for n, a in calls])
        self.assertIn("The sum of 2 and 3 is 5", said)
        self.assertEqual(check.unrun(out), "")

    def test_a_tool_result_reaches_the_transcript(self):
        _code, _out, raw = convert(sample())
        self.assertIn("Echo: pelican", raw)

    def test_thinking_counts_as_output_and_cached_input_is_already_apart(self):
        # A real run reported more cache reads than input: agy's input_tokens
        # leaves the cache out, so nothing is subtracted.
        lines = sample()
        lines[-1] = json.dumps({"event": "result", "result": {
            "status": "SUCCESS", "response": "done", "num_turns": 1,
            "usage": {"input_tokens": 1000, "output_tokens": 40, "thinking_tokens": 60, "cache_read_tokens": 3000}}})
        _code, out, _raw = convert(lines)
        counted, cost = check.read_usage(out)
        self.assertEqual((counted["input_tokens"], counted["output_tokens"], counted["cache_read_input_tokens"]),
                         (1000, 100, 3000))
        self.assertIsNone(cost)

    def test_the_offered_tools_are_the_lanes_own_listing(self):
        # agy's init names its whole tool registry, not what the model was offered.
        _code, _out, raw = convert(sample())
        self.assertIn('"status": "listed-by-lane"', raw)
        self.assertNotIn("run_command", raw)

    def test_a_shell_command_stops_the_run(self):
        code, _out, raw = convert(before_answer(sample(), tool_step(9, "run_command", CommandLine="cat x")))
        self.assertEqual(code, 3)
        self.assertIn("run_command", raw)

    def test_a_failed_outside_tool_stops_the_run_too(self):
        lines = before_answer(sample(), tool_step(9, "search_web", state="ERROR", query="margince"))
        self.assertEqual(convert(lines)[0], 3)

    def test_a_failed_call_that_names_no_server_reached_nothing(self):
        lines = before_answer(sample(), tool_step(9, "call_mcp_tool", state="ERROR", ToolName="echo"))
        self.assertEqual(convert(lines)[0], 0)

    def test_another_servers_tool_stops_the_run(self):
        lines = before_answer(sample(), tool_step(9, "call_mcp_tool", ServerName="github", ToolName="search"))
        self.assertEqual(convert(lines)[0], 3)

    def test_a_look_into_the_runs_own_folder_is_allowed(self):
        # The folder holds only the agent file; nothing in it answers a case.
        lines = before_answer(sample(), tool_step(9, "list_dir", DirectoryPath="/tmp/e2e-llm-agy.ws"),
                              tool_step(10, "read_file", Path="/tmp/e2e-llm-agy.ws/README.md"))
        out = tempfile.NamedTemporaryFile(suffix=".jsonl", delete=False).name
        code = transcript.from_agy(lines, out, "gemini-3.8-flash-medium", "gemini:cli", OFFERED,
                                   workspace="/tmp/e2e-llm-agy.ws")
        self.assertEqual(code, 0)

    def test_a_look_past_the_runs_own_folder_still_stops_the_run(self):
        lines = before_answer(sample(), tool_step(9, "read_file", Path="/tmp/e2e-llm-agy.ws/../secret.md"))
        out = tempfile.NamedTemporaryFile(suffix=".jsonl", delete=False).name
        code = transcript.from_agy(lines, out, "gemini-3.8-flash-medium", "gemini:cli", OFFERED,
                                   workspace="/tmp/e2e-llm-agy.ws")
        self.assertEqual(code, 3)

    def test_reading_a_file_outside_agys_own_tool_files_stops_the_run(self):
        lines = before_answer(sample(), tool_step(9, "read_file", Path="/repo/e2e/llm/scenarios/case4.yaml"))
        code, _out, raw = convert(lines)
        self.assertEqual(code, 3)
        self.assertIn("case4.yaml", raw)

    def test_reading_back_a_long_result_of_a_lane_call_is_allowed(self):
        # Step 5 is the sample's call to echo; agy spills a long result to a file.
        for name in ("output.txt", "resource_0.txt"):
            lines = before_answer(sample(), tool_step(9, "read_file", Path=SPILL.format(5, name)))
            self.assertEqual(convert(lines)[0], 0, name)

    def test_reading_back_a_resource_the_lanes_server_returned_is_allowed(self):
        lines = before_answer(sample(), tool_step(9, "read_resource", ServerName=transcript.SERVER, Uri="x://y"),
                              tool_step(10, "read_file", Path=SPILL.format(9, "resource_0.txt")))
        self.assertEqual(convert(lines)[0], 0)

    def test_a_spill_shaped_path_outside_agys_home_stops_the_run(self):
        conversation = SPILL.split("/brain/")[1].split("/")[0]
        for path in (f"/tmp/elsewhere/brain/{conversation}/.system_generated/steps/5/output.txt",
                     SPILL.replace(conversation, f"0000-other/../{conversation}/../0000-other").format(5, "output.txt")):
            lines = before_answer(sample(), tool_step(9, "read_file", Path=path))
            self.assertEqual(convert(lines)[0], 3, path)

    def test_reading_a_spill_of_another_conversation_stops_the_run(self):
        lines = before_answer(sample(), tool_step(9, "read_file", Path=OTHER_SPILL.format(5, "output.txt")))
        self.assertEqual(convert(lines)[0], 3)

    def test_a_failed_lane_call_reaches_the_transcript_as_an_error(self):
        failed = json.dumps({"event": "step_update", "step_update": {
            "step_index": 9, "state": "ERROR", "step_type": "tool", "tool_name": "call_mcp_tool",
            "tool_info": {"name": "call_mcp_tool",
                          "parameters": {"ServerName": transcript.SERVER, "ToolName": "echo", "Arguments": {}},
                          "error": {"type": "TOOL_ERROR", "message": "message is required"}}}})
        code, _out, raw = convert(before_answer(sample(), failed))
        self.assertEqual(code, 0)
        self.assertIn('"is_error": true', raw)
        self.assertIn("message is required", raw)

    def test_a_refused_action_is_a_harness_fault(self):
        # Headless agy refuses what it cannot ask about: the run measured its
        # permission setting, not the model.
        lines = sample()
        result = json.loads(lines[-1])
        result["result"]["denied_actions"] = [{"action": "mcp", "display_name": "CallMcpTool"}]
        lines[-1] = json.dumps(result)
        code, _out, raw = convert(lines)
        self.assertEqual(code, 3)
        self.assertIn("refused", raw)

    def test_an_error_result_is_a_harness_fault(self):
        lines = sample()
        lines[-1] = json.dumps({"event": "result", "result": {
            "status": "ERROR", "error": "API error (attempt 1): UNAVAILABLE (code 503)"}})
        code, _out, raw = convert(lines)
        self.assertEqual(code, 3)
        self.assertIn("UNAVAILABLE", raw)

    def test_a_run_that_never_finished_is_a_harness_fault(self):
        self.assertEqual(convert(sample()[:-1])[0], 3)

    def test_a_run_that_calls_nothing_is_scored_on_its_answer(self):
        # Only the answer can say whether a run needed no call; the lane, not
        # this run, stops when no run of a sweep reached the server.
        lines = [line for line in sample() if "call_mcp_tool" not in line]
        code, out, _raw = convert(lines)
        self.assertEqual(code, 0)
        self.assertIn("The sum of 2 and 3 is 5", check.read_transcript(out)[1])

    def test_lines_that_are_not_json_are_skipped(self):
        lines = sample()
        lines.insert(1, "[agy] retrying the model in 12s")
        self.assertEqual(convert(lines)[0], 0)


FAKE_AGY = """#!/usr/bin/env python3
import json, os, sys, time
agents = os.path.join(os.getcwd(), ".agents", "agents")
with open(os.environ["FAKE_AGY_SEEN"], "w") as f:
    json.dump({"argv": sys.argv[1:], "cwd": sorted(os.listdir(os.getcwd())),
               "agents": {name: open(os.path.join(agents, name)).read() for name in os.listdir(agents)},
               "mode": oct(os.stat(os.getcwd()).st_mode & 0o777)}, f)
sys.stdout.write(open(os.environ["FAKE_AGY_OUT"]).read())
sys.stdout.flush()
time.sleep(float(os.environ.get("FAKE_AGY_HANG", "0")))
sys.exit(int(os.environ.get("FAKE_AGY_EXIT", "0")))
"""


class RunAgyTest(unittest.TestCase):
    def setUp(self):
        import drive
        from tests.fakes import FakeMcp
        self.drive = drive
        self.bin = tempfile.mkdtemp()
        with open(os.path.join(self.bin, "agy"), "w", encoding="utf-8") as handle:
            handle.write(FAKE_AGY)
        os.chmod(os.path.join(self.bin, "agy"), 0o755)
        self.seen_file = os.path.join(self.bin, "seen.json")
        # agy's own home, where it writes the schemas of a server it attached.
        self.home = tempfile.mkdtemp()
        self.env = {"PATH": self.bin + os.pathsep + os.environ["PATH"], "FAKE_AGY_SEEN": self.seen_file,
                    "FAKE_AGY_OUT": SAMPLE, "MARGINCE_E2E_TOKEN": "s3cr3t-passport", "HOME": self.home}
        self.mcp = FakeMcp([{"name": t, "inputSchema": {"type": "object"}} for t in OFFERED])
        self.mcp.__enter__()

    def tearDown(self):
        self.mcp.__exit__(None, None, None)
        shutil.rmtree(self.bin, ignore_errors=True)
        shutil.rmtree(self.home, ignore_errors=True)

    def run_agy(self, route=None, timeout=60, **env):
        import candidates
        out = tempfile.NamedTemporaryFile(suffix=".jsonl", delete=False).name
        with unittest.mock.patch.dict(os.environ, dict(self.env, **env)):
            code = self.drive.run_agy(route or candidates.resolve("gemini", "cli"), self.mcp.url,
                                      "MARGINCE_E2E_TOKEN", "Echo pelican.", out, timeout=timeout)
        with open(self.seen_file, encoding="utf-8") as handle:
            return code, out, json.load(handle)

    def test_agy_runs_as_an_agent_holding_the_lanes_server_alone(self):
        code, out, seen = self.run_agy()
        self.assertEqual(code, 0)
        argv = seen["argv"]
        self.assertEqual(argv[argv.index("--model") + 1], "gemini-3.8-flash-medium")
        self.assertEqual(argv[argv.index("--output-format") + 1], "stream-json")
        self.assertEqual(argv[argv.index("-p") + 1], "Echo pelican.")
        agent = seen["agents"][argv[argv.index("--agent") + 1] + ".md"]
        for line in ("mainAgent: true", "inheritMcp: false", "inheritCustomizations: false",
                     f"name: {transcript.SERVER}", f"url: {self.mcp.url}"):
            self.assertIn(line, agent)
        self.assertTrue(check.tool_matches(check.read_transcript(out)[0], "echo"))

    def test_only_the_lanes_server_is_allowed_without_asking(self):
        # Headless agy refuses whatever needs a permission it was not given.
        _code, _out, seen = self.run_agy()
        settings = json.loads(seen["argv"][seen["argv"].index("--settings") + 1])
        self.assertEqual(settings, {"permissions": {"allow": [f"mcp({transcript.SERVER}/*)"]}})
        self.assertNotIn("--dangerously-skip-permissions", seen["argv"])

    def test_the_passport_reaches_the_agent_file_and_never_the_command_line(self):
        _code, _out, seen = self.run_agy()
        self.assertNotIn("s3cr3t-passport", " ".join(seen["argv"]))
        self.assertIn("Authorization: Bearer s3cr3t-passport", "".join(seen["agents"].values()))
        self.assertEqual(seen["mode"], "0o700")

    def test_the_workspace_holds_nothing_but_the_agent(self):
        _code, _out, seen = self.run_agy()
        self.assertEqual(seen["cwd"], [".agents"])
        self.assertEqual(list(seen["agents"]), ["margince-e2e-llm.md"])

    def test_an_agy_killed_at_the_wall_clock_is_a_harness_fault_even_with_a_complete_stream(self):
        import candidates
        out = tempfile.NamedTemporaryFile(suffix=".jsonl", delete=False).name
        # Only the exit is asserted: under load the fake can be killed before it
        # writes what it saw, and the kill is what this test is about.
        with unittest.mock.patch.object(self.drive, "AGY_GRACE", 0.5), \
                unittest.mock.patch.dict(os.environ, dict(self.env, FAKE_AGY_HANG="5")):
            code = self.drive.run_agy(candidates.resolve("gemini", "cli"), self.mcp.url,
                                      "MARGINCE_E2E_TOKEN", "Echo pelican.", out, timeout=0)
        self.assertEqual(code, self.drive.HARNESS)

    def test_an_effort_the_model_id_does_not_carry_is_refused_before_agy_runs(self):
        import candidates
        route = candidates.resolve("gemini", "cli", effort="reasoning high", folder="gemini-high")
        code = self.drive.run_agy(route, self.mcp.url, "MARGINCE_E2E_TOKEN", "Echo pelican.",
                                  tempfile.NamedTemporaryFile(suffix=".jsonl", delete=False).name)
        self.assertEqual(code, self.drive.HARNESS)
        self.assertFalse(os.path.exists(self.seen_file))

    def test_an_agy_that_exits_nonzero_is_a_harness_fault_even_with_a_complete_stream(self):
        code, _out, _seen = self.run_agy(FAKE_AGY_EXIT="1")
        self.assertEqual(code, self.drive.HARNESS)


if __name__ == "__main__":
    unittest.main()

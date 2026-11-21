import json
import os
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, HERE)

import check  # noqa: E402
import transcript  # noqa: E402

USAGE = {"input_tokens": 10, "output_tokens": 5, "cache_creation_input_tokens": 3, "cache_read_input_tokens": 2}


def _use(call_id, name):
    return {"type": "assistant", "message": {"content": [
        {"type": "tool_use", "id": call_id, "name": name, "input": {}}]}}


def _result(call_id, text, is_error=False):
    return {"type": "user", "message": {"content": [
        {"type": "tool_result", "tool_use_id": call_id, "content": text, "is_error": is_error}]}}


def _end(turns, duration_ms=None):
    event = {"type": "result", "is_error": False, "result": "done", "num_turns": turns, "usage": USAGE}
    if duration_ms is not None:
        event["duration_ms"] = duration_ms
    return event


def cli_run(calls, turns, duration_ms=None, bare_retry=False, refused=0):
    """A claude CLI transcript making `calls` good calls, the given number of
    refused ones, and a bare-name call the CLI turned away when asked."""
    events = []
    if bare_retry:
        events += [_use("b", "find_records"),
                   _result("b", "<tool_use_error>Error: No such tool available: find_records</tool_use_error>",
                           is_error=True)]
    for n in range(refused):
        events += [_use(f"r{n}", "mcp__margince_e2e_llm__log_activity"), _result(f"r{n}", "invalid field", True)]
    for n in range(calls):
        events += [_use(f"c{n}", "mcp__margince_e2e_llm__find_records"), _result(f"c{n}", "two rows")]
    events.append(_end(turns, duration_ms))
    path = tempfile.NamedTemporaryFile(suffix=".jsonl", delete=False).name
    with open(path, "w", encoding="utf-8") as out:
        out.write("".join(json.dumps(event) + "\n" for event in events))
    return path


class EfficiencyTest(unittest.TestCase):
    def test_a_bare_name_retry_and_a_refusal_count_as_calls_and_as_refused(self):
        effort = check.read_effort(cli_run(2, 4, bare_retry=True, refused=1))
        self.assertEqual((effort["calls"], effort["refused"], effort["turns"]), (4, 2, 4))
        self.assertEqual((effort["prompt_tokens"], effort["output_tokens"]), (15, 5))

    def test_a_bare_name_refusal_without_is_error_still_counts(self):
        path = cli_run(1, 2)
        with open(path, "a", encoding="utf-8") as out:
            out.write(json.dumps(_use("b", "find_records")) + "\n")
            out.write(json.dumps(_result("b", "Error: No such tool available: find_records")) + "\n")
        self.assertEqual(check.read_effort(path)["refused"], 1)

    def test_a_staged_write_is_not_a_refusal(self):
        path = tempfile.NamedTemporaryFile(suffix=".jsonl", delete=False).name
        with open(path, "w", encoding="utf-8") as out:
            for event in (_use("w", "mcp__margince_e2e_llm__send_email"),
                          _result("w", "This write is confirm-first (🟡) and waits for a human.", True), _end(2)):
                out.write(json.dumps(event) + "\n")
        self.assertEqual(check.read_effort(path)["refused"], 0)

    def test_an_unfinished_run_is_not_measured(self):
        path = cli_run(1, 1)
        with open(path, encoding="utf-8") as handle:
            lines = handle.readlines()[:-1]
        with open(path, "w", encoding="utf-8") as out:
            out.writelines(lines)
        self.assertIsNone(check.read_effort(path))
        self.assertIsNone(check.efficiency([path]))

    def test_the_median_and_spread_are_over_the_finished_runs(self):
        measured = check.efficiency([cli_run(3, 4, 2000), cli_run(5, 6, 4000), cli_run(10, 11, 9000)])
        self.assertEqual(measured["runs"], 3)
        self.assertEqual(measured["calls"], {"median": 5, "min": 3, "max": 10})
        self.assertEqual(measured["turns"], {"median": 6, "min": 4, "max": 11})
        self.assertEqual(measured["duration_ms"], {"median": 4000, "min": 2000, "max": 9000})
        self.assertNotIn("slow", measured)
        self.assertEqual(check.efficiency([cli_run(3, 4), cli_run(4, 4)])["calls"]["median"], 3.5)

    def test_wall_time_is_left_out_unless_every_run_carries_it(self):
        self.assertNotIn("duration_ms", check.efficiency([cli_run(3, 4, 2000), cli_run(3, 4)]))

    def test_calls_past_the_budget_are_slow_and_the_line_says_so(self):
        measured = check.efficiency([cli_run(6, 7), cli_run(8, 9)], expected_calls=5)
        self.assertTrue(measured["slow"])
        self.assertIn("<-- slow: median 7 calls, 5 expected", check.efficiency_line(measured))
        self.assertFalse(check.efficiency([cli_run(5, 6)], expected_calls=5)["slow"])

    def test_a_slow_case_records_its_verdict_unchanged(self):
        scenario = tempfile.NamedTemporaryFile(suffix=".yaml", delete=False, mode="w", encoding="utf-8")
        scenario.write("name: case_slow\nprompt: p\nwrites: false\nruns: 2\npass_at: 2\nexpected_calls: 1\n")
        scenario.close()
        env = {key: value for key, value in os.environ.items() if not key.startswith("E2E_LLM_")}
        done = subprocess.run(
            [sys.executable, os.path.join(HERE, "check.py"), "--record", scenario.name, "2", "2",
             cli_run(3, 4), cli_run(4, 5)],
            capture_output=True, text=True, env=env, check=False)
        self.assertEqual(done.returncode, 0, done.stderr)
        record = json.loads(done.stdout)
        self.assertEqual((record["passed"], record["runs"], record["slow"]), (2, 2, True))
        self.assertEqual(record["efficiency"]["calls"], {"median": 3.5, "min": 3, "max": 4})
        self.assertEqual(record["efficiency"]["expected_calls"], 1)

    def test_lint_refuses_a_budget_that_is_not_a_count(self):
        for value in ("many", "0", "-2"):
            scenario = tempfile.NamedTemporaryFile(suffix=".yaml", delete=False, mode="w", encoding="utf-8")
            scenario.write(f"name: c\nprompt: p\nwrites: false\nruns: 1\npass_at: 1\nexpected_calls: {value}\n")
            scenario.close()
            self.assertTrue(any("expected_calls must be a whole number" in p for p in check.lint(scenario.name)),
                            value)


if __name__ == "__main__":
    unittest.main()


class ToolsCalledTest(unittest.TestCase):
    def test_only_calls_that_worked_count_by_their_bare_name(self):
        path = tempfile.NamedTemporaryFile(suffix=".jsonl", delete=False).name
        with transcript.Transcript(path) as out:
            out.init("m", "claude:cli", "cli-default", "connected", ["find_records", "update_record"])
            out.assistant("", [("c0", "find_records", {}), ("c1", "update_record", {})])
            out.tool_results([("c0", "{}", False), ("c1", "refused", True)])
            out.finish(False, "Done.", 2)
        self.assertEqual(check.tools_called([path, "/nonexistent.jsonl"]), ["find_records"])

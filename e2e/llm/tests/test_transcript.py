import os
import re
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, HERE)

import check  # noqa: E402
import transcript  # noqa: E402

MEASURED = {"input": 10, "output": 5, "cache_read": 2, "cache_write": 0, "cost": None}


def write(usages=(MEASURED,)):
    path = tempfile.NamedTemporaryFile(suffix=".jsonl", delete=False).name
    with transcript.Transcript(path) as out:
        out.init("m", "gpt:api", "mcp-instructions", "connected", ["list_pipelines"])
        out.assistant("", [("c1", "list_pipelines", {"limit": 5})])
        out.tool_results([("c1", "two pipelines", False)])
        out.assistant("There are two pipelines.")
        for usage in usages:
            out.add_usage(usage)
        out.finish(False, "There are two pipelines.", 2)
    return path


class TranscriptTest(unittest.TestCase):
    def test_check_reads_the_call_its_arguments_and_the_answer(self):
        path = write()
        called, said, calls = check.read_transcript(path)
        self.assertTrue(check.tool_matches(called, "list_pipelines"))
        self.assertEqual(calls[0][1], {"limit": 5})
        self.assertIn("two pipelines", said)
        self.assertEqual(check.unrun(path), "")

    def test_the_checker_rejects_a_run_that_skipped_the_tool_or_the_fact(self):
        path = tempfile.NamedTemporaryFile(suffix=".jsonl", delete=False).name
        with transcript.Transcript(path) as out:
            out.init("m", "gpt:api", "mcp-instructions", "connected", ["list_pipelines"])
            out.assistant("There are three pipelines.")
            out.finish(False, "There are three pipelines.", 1)
        problems = check.check({"must_call": ["list_pipelines"], "must_mention": ["two"]}, path)
        self.assertTrue(any("never called list_pipelines" in p for p in problems), problems)
        self.assertTrue(any("two" in p for p in problems), problems)

    def test_usage_is_summed_across_requests(self):
        counted, cost = check.read_usage(write(usages=[MEASURED, MEASURED]))
        self.assertEqual((counted["input_tokens"], counted["cache_read_input_tokens"]), (20, 4))
        self.assertIsNone(cost)

    def test_reported_cost_is_summed(self):
        priced = dict(MEASURED, cost=0.25)
        self.assertEqual(check.read_usage(write(usages=[priced, priced]))[1], 0.5)

    def test_one_unmeasured_request_makes_the_run_unmeasured(self):
        self.assertIsNone(check.read_usage(write(usages=[MEASURED, None])))

    def test_only_transcript_py_writes_a_result_event(self):
        for name in sorted(os.listdir(HERE)):
            if not name.endswith(".py") or name == "transcript.py":
                continue
            with open(os.path.join(HERE, name), encoding="utf-8") as handle:
                self.assertIsNone(
                    re.search(r"""["']type["']\s*:\s*["']result["']""", handle.read()),
                    f"{name} writes a result event; the transcript shape has one writer",
                )


if __name__ == "__main__":
    unittest.main()

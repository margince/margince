import os
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import check  # noqa: E402
import transcript  # noqa: E402


def offered(tools, status="connected"):
    path = tempfile.NamedTemporaryFile(suffix=".jsonl", delete=False).name
    with transcript.Transcript(path) as out:
        out.init("m", "x:api", "mcp-instructions", status, tools)
        out.finish(False, "", 1)
    return path


class OfferedTest(unittest.TestCase):
    def test_a_required_tool_not_offered_is_named(self):
        scenario = {"must_call": ["search_context", "run_report|run_analytics_query"]}
        problem = check.offered_problem(scenario, offered(["run_report"]))
        self.assertIn("search_context", problem)
        self.assertNotIn("run_report", problem)

    def test_any_one_alternative_offered_is_enough(self):
        scenario = {"must_call": ["run_report|run_analytics_query"]}
        self.assertEqual(check.offered_problem(scenario, offered(["run_analytics_query"])), "")

    def test_an_empty_tool_list_is_named_as_a_census_that_read_nothing(self):
        self.assertIn("offered no tools", check.offered_problem({"must_call": ["x"]}, offered([])))

    def test_a_scenario_with_no_must_call_still_needs_tools_offered(self):
        self.assertIn("offered no tools", check.offered_problem({}, offered([])))

    def test_a_transcript_with_no_system_line_is_named(self):
        path = tempfile.NamedTemporaryFile(suffix=".jsonl", delete=False).name
        with transcript.Transcript(path) as out:
            out.finish(False, "", 1)
        self.assertIn("no system line", check.offered_problem({"must_call": ["x"]}, path))


if __name__ == "__main__":
    unittest.main()

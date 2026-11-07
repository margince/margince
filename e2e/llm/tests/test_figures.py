import json
import os
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, HERE)

import check  # noqa: E402

RESULTS = ['{"open_minor": 17400000, "q4_expected": 164500, "won": "97,600.00 EUR", "deals": 7}']


class FigureProblemsTest(unittest.TestCase):
    def test_a_figure_a_tool_returned_holds_however_it_is_written(self):
        for said in ("Open pipeline is €174,000; won €97,600.", "€174k open, €97.6k won.",
                     "174.000 € open, 97.600 EUR won.", "EUR 164,500 is expected in Q4."):
            self.assertEqual(check.figure_problems(said, RESULTS), [], said)

    def test_a_figure_no_tool_returned_is_named(self):
        problems = check.figure_problems("Open pipeline is €175,000.", RESULTS)
        self.assertEqual(len(problems), 1)
        self.assertIn("€175,000", problems[0])

    def test_a_figure_worked_out_from_two_returned_ones_holds(self):
        # 174,000 - 164,500: the gap between two figures the run read.
        self.assertEqual(check.figure_problems("The two differ by €9,500.", RESULTS), [])

    def test_an_answer_with_no_money_figure_has_nothing_to_check(self):
        self.assertEqual(check.figure_problems("Seven deals are open.", RESULTS), [])

    def test_a_run_that_read_nothing_holds_no_figure(self):
        self.assertEqual(len(check.figure_problems("Won €97,600.", [])), 1)


class FiguresFromResultsFieldTest(unittest.TestCase):
    def transcript(self, said, result):
        events = [
            {"type": "assistant", "message": {"content": [
                {"type": "tool_use", "id": "t1", "name": "mcp__margince_e2e_llm__read_reports", "input": {}}]}},
            {"type": "user", "message": {"content": [
                {"type": "tool_result", "tool_use_id": "t1", "content": [{"type": "text", "text": result}]}]}},
            {"type": "result", "subtype": "success", "is_error": False, "result": said},
        ]
        path = tempfile.NamedTemporaryFile(suffix=".jsonl", delete=False, mode="w").name
        with open(path, "w", encoding="utf-8") as out:
            out.write("\n".join(json.dumps(event) for event in events))
        return path

    def test_the_field_checks_the_answer_against_the_runs_own_results(self):
        path = self.transcript("Won €97,600, open €180,000.", RESULTS[0])
        problems = check.check({"figures_from_results": "true"}, path)
        self.assertEqual(len(problems), 1)
        self.assertIn("€180,000", problems[0])

    def test_any_other_value_is_refused_rather_than_ignored(self):
        path = self.transcript("Won €97,600.", RESULTS[0])
        self.assertIn("the only value is true", check.check({"figures_from_results": "yes"}, path)[0])


if __name__ == "__main__":
    unittest.main()

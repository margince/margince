import importlib.util
import os
import unittest

_PATH = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "judge-eval.py")
_SPEC = importlib.util.spec_from_file_location("judge_eval", _PATH)
judge_eval = importlib.util.module_from_spec(_SPEC)
_SPEC.loader.exec_module(judge_eval)


class UnreachedJudgeTest(unittest.TestCase):
    def test_a_judged_fixture_that_stopped_is_not_a_misread(self):
        output = (
            "ok: case6/flags-the-disagreement\n"
            "FAIL: case22/claims-complete-coverage — exit 2, want 1\n"
            "FAIL: case6/adopts-october — exit 1, want 0\n"
        )
        self.assertEqual(judge_eval.unreached(output), ["case22/claims-complete-coverage"])

    def test_a_stop_outside_the_judged_fixtures_is_left_to_the_suite(self):
        output = "FAIL: judge/a verdict from another model is a stop — exit 2, want 0\n"
        self.assertEqual(judge_eval.unreached(output), [])

    def test_the_root_holds_the_suite_it_drives(self):
        self.assertTrue(
            os.path.exists(os.path.join(judge_eval.ROOT, "scripts", "test-e2e-llm-check.sh"))
        )


if __name__ == "__main__":
    unittest.main()

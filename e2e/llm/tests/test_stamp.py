"""A verdict carries the stamp of what it measured, from the stamps the lane wrote.

The digests are computed by backend/internal/compose/mcpe2e and tested there;
this holds the half that files them: the right scenario's stamp, the commit and
the time it ran and was graded, and no stamp at all where the lane wrote none.
"""

import datetime
import json
import os
import subprocess
import sys
import tempfile
import unittest
import unittest.mock

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import check  # noqa: E402

CHECK = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "check.py")
STAMP = {"scenario": "a1", "tools": "b2", "instructions": "c3", "judge": "", "harness": "d4",
         "parsers": {"gpt:cli": "e5", "gemini:cli": "f6"}}


def written(text, suffix):
    handle = tempfile.NamedTemporaryFile("w", suffix=suffix, delete=False, encoding="utf-8")
    with handle as out:
        out.write(text)
    return handle.name


class RecordedStampTest(unittest.TestCase):
    def test_the_verdict_carries_its_scenarios_stamp_the_commit_and_the_time(self):
        stamps = written(json.dumps({"case1_x": STAMP, "case2_y": {**STAMP, "scenario": "zz"}}), ".json")
        scenario = written("name: case1_x\nruns: 1\npass_at: 1\n", ".yaml")
        env = {**os.environ, "E2E_LLM_STAMPS": stamps, "E2E_LLM_COMMIT": "abc123"}
        done = subprocess.run([sys.executable, CHECK, "--record", scenario, "1", "1"],
                              capture_output=True, text=True, env=env, check=True)
        stamp = json.loads(done.stdout)["stamp"]
        self.assertEqual({k: stamp[k] for k in STAMP}, STAMP)
        self.assertEqual(stamp["commit"], "abc123")
        self.assertRegex(stamp["ran_at"], r"^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ$")

    def test_no_stamps_file_files_no_stamp(self):
        with unittest.mock.patch.dict(os.environ, {}, clear=False):
            os.environ.pop("E2E_LLM_STAMPS", None)
            self.assertIsNone(check.recorded_stamp("case1_x"))

    def test_stamps_from_another_tree_are_refused(self):
        stamps = written(json.dumps({"case2_y": STAMP}), ".json")
        with unittest.mock.patch.dict(os.environ, {"E2E_LLM_STAMPS": stamps}):
            with self.assertRaisesRegex(ValueError, "carry no case1_x"):
                check.recorded_stamp("case1_x")

    def test_the_judge_is_the_one_that_graded(self):
        stamps = written(json.dumps({"case1_x": {**STAMP, "judge": "claude-haiku-5-5"}, "case2_y": STAMP}), ".json")
        with unittest.mock.patch.dict(os.environ, {"E2E_LLM_STAMPS": stamps, "E2E_LLM_JUDGE_MODEL": "gpt-judge"}):
            self.assertEqual(check.recorded_stamp("case1_x")["judge"], "gpt-judge")
            self.assertEqual(check.recorded_stamp("case2_y")["judge"], "")

    def test_a_live_grade_names_the_run_commit_twice(self):
        stamps = written(json.dumps({"case1_x": STAMP}), ".json")
        moment = datetime.datetime(2026, 11, 20, 9, 8, 53, tzinfo=datetime.timezone.utc)
        with unittest.mock.patch.dict(os.environ, {"E2E_LLM_STAMPS": stamps, "E2E_LLM_COMMIT": "run1"}):
            os.environ.pop("E2E_LLM_GRADED_COMMIT", None)
            os.environ.pop("E2E_LLM_RAN_AT", None)
            stamp = check.recorded_stamp("case1_x", clock=lambda: moment)
        self.assertEqual((stamp["commit"], stamp["graded_commit"]), ("run1", "run1"))
        self.assertEqual(stamp["ran_at"], stamp["graded_at"])

    def test_a_regrade_keeps_the_runs_commit_and_time_beside_its_own(self):
        stamps = written(json.dumps({"case1_x": STAMP}), ".json")
        moment = datetime.datetime(2026, 11, 21, 10, 0, 0, tzinfo=datetime.timezone.utc)
        env = {"E2E_LLM_STAMPS": stamps, "E2E_LLM_COMMIT": "run1", "E2E_LLM_RAN_AT": "2026-11-20T09:08:53Z",
               "E2E_LLM_GRADED_COMMIT": "grade2"}
        with unittest.mock.patch.dict(os.environ, env):
            stamp = check.recorded_stamp("case1_x", clock=lambda: moment)
        self.assertEqual({k: stamp[k] for k in STAMP}, STAMP)
        self.assertEqual((stamp["commit"], stamp["ran_at"]), ("run1", "2026-11-20T09:08:53Z"))
        self.assertEqual((stamp["graded_commit"], stamp["graded_at"]), ("grade2", "2026-11-21T10:00:00Z"))

    def test_the_time_is_utc_to_the_second(self):
        stamps = written(json.dumps({"case1_x": STAMP}), ".json")
        moment = datetime.datetime(2026, 11, 20, 9, 8, 53, 999, tzinfo=datetime.timezone.utc)
        with unittest.mock.patch.dict(os.environ, {"E2E_LLM_STAMPS": stamps, "E2E_LLM_COMMIT": "c"}):
            self.assertEqual(check.recorded_stamp("case1_x", clock=lambda: moment)["ran_at"], "2026-11-20T09:08:53Z")


if __name__ == "__main__":
    unittest.main()

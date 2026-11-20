"""A verdict carries the stamp of what it measured, from the stamps the lane wrote.

The digests are computed by backend/internal/compose/mcpe2e and tested there;
this holds the half that files them: the right scenario's stamp, the commit and
the time it ran, and no stamp at all where the lane wrote none.
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
STAMP = {"scenario": "a1", "tools": "b2", "instructions": "c3", "judge": "", "harness": "d4"}


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

    def test_the_time_is_utc_to_the_second(self):
        stamps = written(json.dumps({"case1_x": STAMP}), ".json")
        moment = datetime.datetime(2026, 11, 20, 9, 8, 53, 999, tzinfo=datetime.timezone.utc)
        with unittest.mock.patch.dict(os.environ, {"E2E_LLM_STAMPS": stamps, "E2E_LLM_COMMIT": "c"}):
            self.assertEqual(check.recorded_stamp("case1_x", clock=lambda: moment)["ran_at"], "2026-11-20T09:08:53Z")


if __name__ == "__main__":
    unittest.main()

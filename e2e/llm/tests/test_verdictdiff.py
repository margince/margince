"""The weekly pull request's table names exactly the scenarios whose result moved."""

import json
import os
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import verdictdiff  # noqa: E402


def folder(verdicts):
    directory = tempfile.mkdtemp()
    for name, passed, runs in verdicts:
        with open(os.path.join(directory, name + ".json"), "w", encoding="utf-8") as out:
            json.dump({"scenario": name, "passed": passed, "runs": runs, "pass_at": 2, "search": "semantic"}, out)
    return directory


class VerdictDiffTest(unittest.TestCase):
    def test_only_a_moved_result_gets_a_row(self):
        before = folder([("case1_a", 2, 2), ("case2_b", 2, 2)])
        after = folder([("case1_a", 2, 2), ("case2_b", 1, 3), ("case3_c", 2, 2)])
        text = verdictdiff.table(before, after)
        self.assertIn("3 scenarios have a verdict; 2 held their bar. Search: semantic.", text)
        self.assertIn("| `case2_b` | 2 of 2, held | 1 of 3, below its bar |", text)
        self.assertIn("| `case3_c` | none | 2 of 2, held |", text)
        self.assertNotIn("case1_a", text)

    def test_nothing_moved_says_so(self):
        same = [("case1_a", 2, 2)]
        self.assertIn("No scenario changed its result.", verdictdiff.table(folder(same), folder(same)))


if __name__ == "__main__":
    unittest.main()

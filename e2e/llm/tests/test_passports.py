"""The passport names the lint accepts, the lane mints and the Go census scopes
are one list kept in three files, so each is read here and held against the others.
"""

import os
import re
import sys
import unittest

HERE = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
ROOT = os.path.dirname(os.path.dirname(HERE))
sys.path.insert(0, HERE)

import check  # noqa: E402


def _read(*parts):
    with open(os.path.join(ROOT, *parts), encoding="utf-8") as handle:
        return handle.read()


def lane_passports():
    """The labels of use_passport's case in the lane, "" for the narrow default."""
    body = re.search(r"^use_passport\(\) \{\n(.*?)^\}", _read("scripts", "e2e-llm.sh"), re.S | re.M).group(1)
    labels = re.findall(r'^\s+("?)([a-z]*)\1\) PASSPORT=', body, re.M)
    return {label for _, label in labels}


def go_passports():
    """The keys of lanePassportScopes, the scopes the Go census holds a case to."""
    text = _read("backend", "internal", "compose", "mcptoolcoveragesurface_test.go")
    table = re.search(r"var lanePassportScopes = .*?\n(.*?)\n\}", text, re.S).group(1)
    return set(re.findall(r'^\t"([a-z]*)":', table, re.M))


class PassportNamesTest(unittest.TestCase):
    def test_the_lint_the_lane_and_the_go_census_name_the_same_passports(self):
        names = set(check.PASSPORTS)
        self.assertIn("readonly", names)
        self.assertEqual(lane_passports(), names)
        self.assertEqual(go_passports(), names)


if __name__ == "__main__":
    unittest.main()

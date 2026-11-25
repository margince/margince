"""The passport names the lint accepts, the lane connects and the Go census scopes
are one list kept in three files, so each is read here and held against the others.
The scopes each passport is connected with are held against the census the same way.
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


def lane_scopes():
    """{passport name: the scopes connect_passports asks for it}."""
    script = _read("scripts", "e2e-llm.sh")
    body = re.search(r"^use_passport\(\) \{\n(.*?)^\}", script, re.S | re.M).group(1)
    named = {var: label for _, label, var in re.findall(r'^\s+("?)([a-z]*)\1\) PASSPORT="\$(PASSPORT_[A-Z]+)"', body, re.M)}
    connected = re.search(r"^connect_passports\(\) \{\n(.*?)^\}", script, re.S | re.M).group(1)
    asked = re.findall(r'^\s+(PASSPORT_[A-Z]+)="\$\(connect_with "[^"]+" [a-z0-9-]+ ([a-z ]+)\)"', connected, re.M)
    return {named[var]: set(scopes.split()) for var, scopes in asked}


def go_table():
    text = _read("backend", "internal", "compose", "mcptoolcoveragesurface_test.go")
    return re.search(r"var lanePassportScopes = .*?\n(.*?)\n\}", text, re.S).group(1)


def go_passports():
    """The keys of lanePassportScopes, the scopes the Go census holds a case to."""
    return set(re.findall(r'^\t"([a-z]*)":', go_table(), re.M))


def go_scopes():
    """{passport name: its scopes} as lanePassportScopes holds them."""
    entries = re.findall(r'^\t"([a-z]*)":\s*\{(.*?)\}', go_table(), re.S | re.M)
    return {name: {scope.lower() for scope in re.findall(r"principal\.Scope([A-Za-z]+)", body)}
            for name, body in entries}


class PassportNamesTest(unittest.TestCase):
    def test_the_lint_the_lane_and_the_go_census_name_the_same_passports(self):
        names = set(check.PASSPORTS)
        self.assertIn("readonly", names)
        self.assertEqual(lane_passports(), names)
        self.assertEqual(go_passports(), names)

    def test_each_passport_is_connected_with_the_scopes_the_go_census_holds(self):
        lane = lane_scopes()
        self.assertEqual(set(lane), set(check.PASSPORTS))
        self.assertEqual(lane[""], {"read", "write"})
        self.assertEqual(lane, go_scopes())


if __name__ == "__main__":
    unittest.main()

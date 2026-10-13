"""Every tool a scenario names is one the server lists.

The lane only checks must_call against what was offered. A hidden name in
must_call_with fails every paid run, and one in must_not_call_with or may_call
holds nothing at all, silently. The listed set is the published surface,
regenerated from the registry, so this reads the owner rather than a copy.
"""

import json
import os
import re
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import check  # noqa: E402

LANE = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SCENARIOS = os.path.join(LANE, "scenarios")
SURFACE = os.path.join(LANE, "..", "..", "docs", "reference", "mcp-info.json")
FIELDS = ("must_call", "may_call", "must_call_with", "must_not_call_with")
# must_not_call_with is `tool~regex`, and its regex may hold a `|` of its own.
# must_call_with alternatives each open with `tool.`.
CALL_WITH = re.compile(r"(?:^|\|)\s*([a-z_]+)\.")


def listed():
    with open(SURFACE) as surface:
        return {tool["name"] for tool in json.load(surface)["tools"]["tools"]}


def named(field, entry):
    if field == "must_not_call_with":
        return [entry.split("~", 1)[0].strip()]
    if field == "must_call_with":
        return CALL_WITH.findall(entry)
    return [option.strip() for option in entry.split("|")]


class ScenarioToolNamesTest(unittest.TestCase):
    def test_every_named_tool_is_listed(self):
        offered = listed()
        seen = dict.fromkeys(FIELDS, 0)
        for file in sorted(os.listdir(SCENARIOS)):
            scenario = check.parse_scenario(os.path.join(SCENARIOS, file))
            for field in FIELDS:
                for entry in scenario.get(field) or []:
                    for tool in named(field, entry):
                        seen[field] += 1
                        with self.subTest(scenario=file, field=field, tool=tool):
                            self.assertIn(tool, offered)
        # A field read as empty would pass every name it never saw.
        for field, count in seen.items():
            with self.subTest(field=field):
                self.assertGreater(count, 0)


if __name__ == "__main__":
    unittest.main()

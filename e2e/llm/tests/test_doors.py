"""A check scoped to the MCP door holds there, and a REST run is graded without it."""

import json
import os
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import check  # noqa: E402
import transcript  # noqa: E402

SCENARIO = """name: case0_offer
prompt: |
  Add her, and offer the tag.
writes: true
must_call:
  - create_record
must_call_with:
  - create_record.offer_tag=K5
%s
runs: 1
pass_at: 1
"""


def scenario_file(scope):
    handle = tempfile.NamedTemporaryFile("w", suffix=".yaml", delete=False, encoding="utf-8")
    with handle as out:
        out.write(SCENARIO % scope)
    return handle.name


def run(door):
    """One create_record call with no offer, on the door named."""
    path = tempfile.NamedTemporaryFile(suffix=".jsonl", delete=False).name
    with transcript.Transcript(path) as out:
        out.init("m", "claude:cli", "cli-default", "connected", ["create_record"])
        out.assistant("", [("c0", "create_record", {"record_type": "contact", "fields": {"full_name": "Nora"}})])
        out.tool_results([("c0", "{}", False)])
        out.finish(False, "Nora is in the CRM.", 2)
    if door == "rest":
        with open(path, encoding="utf-8") as handle:
            lines = [json.loads(line) for line in handle if line.strip()]
        lines[0]["door"] = "rest"
        with open(path, "w", encoding="utf-8") as handle:
            handle.write("\n".join(json.dumps(line) for line in lines) + "\n")
    return path


class DoorScopeTest(unittest.TestCase):
    def test_a_check_scoped_to_mcp_is_still_enforced_there(self):
        scenario = check.parse_scenario(scenario_file("mcp_only:\n  - create_record.offer_tag=K5"))
        self.assertTrue(any("offer_tag" in p for p in check.check(scenario, run("mcp"))))

    def test_a_rest_run_is_graded_without_the_scoped_check(self):
        scenario = check.parse_scenario(scenario_file("mcp_only:\n  - create_record.offer_tag=K5"))
        self.assertEqual(check.check(scenario, run("rest")), [])

    def test_an_unscoped_check_holds_on_rest_too(self):
        scenario = check.parse_scenario(scenario_file(""))
        self.assertTrue(any("offer_tag" in p for p in check.check(scenario, run("rest"))))

    def test_a_scope_naming_no_check_of_the_file_fails_lint(self):
        problems = check.lint(scenario_file("mcp_only:\n  - create_record.offer_tg=K5"))
        self.assertTrue(any("mcp_only names" in p for p in problems), problems)

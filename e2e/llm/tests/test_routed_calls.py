"""A call to a router is graded as the verb its action ran.

A scenario names the verb it needs. The served surface lists that verb behind a
router, so the model calls the router with an action; the lane must read that
call as the verb, arguments and all, or every case naming a fronted verb fails
for a reason that is not the model's.
"""

import json
import os
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import check  # noqa: E402


def transcript(events):
    fd, path = tempfile.mkstemp(suffix=".jsonl")
    with os.fdopen(fd, "w") as fh:
        for event in events:
            fh.write(json.dumps(event) + "\n")
    return path


class RoutedCallsTest(unittest.TestCase):
    def test_a_router_call_satisfies_the_verb_it_ran(self):
        router, table = next(
            (name, t) for name, t in check.load_routes().items()
            if t.get("field") and "list_records" in t.get("actions", {}).values()
        )
        action = next(a for a, verb in table["actions"].items() if verb == "list_records")
        path = transcript([
            {"type": "system", "subtype": "init", "tools": ["mcp__margince__" + router]},
            {"type": "assistant", "message": {"content": [{
                "type": "tool_use", "name": "mcp__margince__" + router,
                "input": {table["field"]: action, "record_type": "deal"},
            }]}},
        ])
        self.addCleanup(os.remove, path)
        scenario = {"must_call": ["list_records"], "must_call_with": ["list_records.record_type=deal"]}
        self.assertEqual(check.offered_problem(scenario, path), "")
        problems = check.check(scenario, path)
        self.assertFalse([p for p in problems if "never called" in p], problems)

    def test_an_unknown_action_is_not_read_as_a_verb(self):
        router, table = next((name, t) for name, t in check.load_routes().items() if t.get("field"))
        self.assertIsNone(check.routed("mcp__margince__" + router, {table["field"]: "no_such_action"}, check.load_routes()))


if __name__ == "__main__":
    unittest.main()

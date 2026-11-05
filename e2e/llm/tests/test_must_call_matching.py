"""must_call_matching: some call to the tool carries what the regex asks.

The target of a hand-written draft is named in its free text, not in one
argument's exact value, so must_call_with cannot hold it.
"""

import os
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import check  # noqa: E402
import transcript  # noqa: E402

AIMS = {"must_call_matching": ["draft_outbound~slipping_deals|Reply Deutschland|Jonas"]}


def run(calls):
    path = tempfile.NamedTemporaryFile(suffix=".jsonl", delete=False).name
    with transcript.Transcript(path) as out:
        out.init("m", "gpt:api", "mcp-instructions", "connected", ["draft_outbound", "send_outbound"])
        out.assistant("", [(f"c{i}", name, args) for i, (name, args) in enumerate(calls)])
        out.finish(False, "Done.", 1)
    return path


class MustCallMatchingTest(unittest.TestCase):
    def test_one_call_that_names_the_target_is_enough(self):
        path = run([
            ("draft_outbound", {"for": "one_message", "intent": "A short check-in"}),
            ("draft_outbound", {"for": "one_message", "intent": "Chaser to Jonas Reinhard"}),
        ])
        self.assertEqual(check.check(AIMS, path), [])

    def test_no_call_that_names_the_target_fails(self):
        path = run([("draft_outbound", {"for": "one_message", "intent": "A short check-in"})])
        problems = check.check(AIMS, path)
        self.assertEqual(len(problems), 1)
        self.assertIn("never called draft_outbound carrying anything matching", problems[0])

    def test_the_target_named_in_another_tool_does_not_count(self):
        path = run([("send_outbound", {"as": "email_new", "body": "Hallo Jonas"})])
        self.assertEqual(len(check.check(AIMS, path)), 1)

    def test_an_entry_without_a_regex_is_a_problem_not_a_pass(self):
        path = run([("draft_outbound", {"for": "slipping_deals"})])
        problems = check.check({"must_call_matching": ["draft_outbound"]}, path)
        self.assertEqual(problems, ["must_call_matching entry 'draft_outbound' is not tool~regex"])


if __name__ == "__main__":
    unittest.main()

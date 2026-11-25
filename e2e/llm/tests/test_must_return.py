"""must_return: some call to the tool was answered with what the regex asks.

A move that waits for its user answers a connected client a preview to ask in
the chat, and stages for the inbox otherwise. The calls are the same in both
runs, so only the answer tells them apart.
"""

import json
import os
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import check  # noqa: E402
import transcript  # noqa: E402

ASKED = "needs the user's yes in this conversation"
PREVIEW = "Needs the user's yes in this conversation. Nothing was changed yet."
STAGED = "Confirm-first (🟡): a contact answers this before it runs. Nothing was changed yet."
AIMS = {"must_return": [f"relink_activity~{ASKED} || relink_activities~{ASKED}"]}
RELINK = ("change_activities", {"action": "relink", "activity_id": "a1", "entity_type": "project", "entity_id": "p1"})
RELINK_MANY = ("change_activities", {"action": "relink_many", "activity_ids": ["a1"], "entity_type": "project",
                                     "entity_id": "p1"})


def run(calls, door="mcp"):
    """A transcript of (tool, arguments, answer) calls, each answered as an error."""
    path = tempfile.NamedTemporaryFile(suffix=".jsonl", delete=False).name
    with transcript.Transcript(path) as out:
        out.init("m", "claude:cli", "cli-default", "connected", ["change_activities"])
        out.assistant("", [(f"c{i}", name, args) for i, (name, args, _) in enumerate(calls)])
        out.tool_results([(f"c{i}", answer, True) for i, (_, _, answer) in enumerate(calls) if answer is not None])
        out.finish(False, "It needs your yes first.", 1)
    if door == "rest":
        with open(path, encoding="utf-8") as handle:
            lines = [json.loads(line) for line in handle if line.strip()]
        lines[0]["door"] = "rest"
        with open(path, "w", encoding="utf-8") as handle:
            handle.write("\n".join(json.dumps(line) for line in lines) + "\n")
    return path


class MustReturnTest(unittest.TestCase):
    def test_the_preview_asked_in_the_chat_holds(self):
        self.assertEqual(check.check(AIMS, run([(*RELINK, PREVIEW)])), [])

    def test_a_staging_fails_and_shows_what_was_answered(self):
        problems = check.check(AIMS, run([(*RELINK, STAGED)]))
        self.assertEqual(len(problems), 1)
        self.assertIn("never got relink_activity answering", problems[0])
        self.assertIn("Confirm-first", problems[0])

    def test_either_alternative_holds(self):
        self.assertEqual(check.check(AIMS, run([(*RELINK_MANY, PREVIEW)])), [])

    def test_a_call_with_no_answer_says_so(self):
        problems = check.check(AIMS, run([(*RELINK, None)]))
        self.assertEqual(len(problems), 1)
        self.assertIn("no call to it was answered", problems[0])

    def test_the_answer_of_another_tool_does_not_count(self):
        other = ("change_activities", {"action": "log", "kind": "note", "body": "x"}, PREVIEW)
        self.assertEqual(len(check.check(AIMS, run([other]))), 1)

    def test_an_entry_without_a_regex_is_a_problem_not_a_pass(self):
        problems = check.check({"must_return": ["relink_activity"]}, run([(*RELINK, PREVIEW)]))
        self.assertEqual(problems, ["must_return entry 'relink_activity' is not tool~regex"])

    def test_a_rest_run_is_graded_without_it_when_it_is_mcp_only(self):
        scoped = {**AIMS, "mcp_only": list(AIMS["must_return"])}
        self.assertEqual(check.check(scoped, run([(*RELINK, STAGED)], door="rest")), [])
        self.assertEqual(len(check.check(scoped, run([(*RELINK, STAGED)]))), 1)


class LintTest(unittest.TestCase):
    def lint(self, entry):
        data = {"name": "x", "prompt": "p", "writes": "true", "runs": 1, "pass_at": 1, "must_return": [entry]}
        return check._entry_problems(data)

    def test_a_malformed_or_uncompilable_entry_is_refused(self):
        self.assertTrue(any("is not tool~regex" in p for p in self.lint("relink_activity")))
        self.assertTrue(any("must_return" in p for p in self.lint("relink_activity~(")))

    def test_a_well_formed_entry_lints_clean(self):
        self.assertFalse([p for p in self.lint(AIMS["must_return"][0]) if p])


if __name__ == "__main__":
    unittest.main()

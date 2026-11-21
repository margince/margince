import json
import os
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, HERE)

import candidates  # noqa: E402
import check  # noqa: E402
import clean  # noqa: E402
import drive  # noqa: E402

SCENARIOS = os.path.join(HERE, "scenarios")
CASE47 = os.path.join(SCENARIOS, "case47-tidy-the-fair-list.yaml")
# A case47 run the SDK route drove, as the runner wrote it, trimmed.
RECORDED = os.path.join(HERE, "testdata", "case47", "hands-over-after-the-yes.jsonl")


def scenario_with(*entries):
    with tempfile.NamedTemporaryFile(suffix=".yaml", delete=False, mode="w", encoding="utf-8") as handle:
        handle.write("name: t\nprompt: p\nwrites: false\nruns: 1\npass_at: 1\nelicitation:\n")
        handle.writelines(f'  - "{entry}"\n' for entry in entries)
    return handle.name


class ElicitationRepliesTest(unittest.TestCase):
    def test_an_accept_carries_its_fields_as_json_values(self):
        self.assertEqual(
            check.elicitation_replies(["Give a new owner to => accept confirm=true count=12 who=Sofia"]),
            [{"match": "Give a new owner to", "action": "accept",
              "content": {"confirm": True, "count": 12, "who": "Sofia"}}],
        )

    def test_a_decline_and_a_cancel_carry_nothing(self):
        self.assertEqual(
            check.elicitation_replies(["new owner => decline", "Undo the bulk change => cancel"]),
            [{"match": "new owner", "action": "decline"}, {"match": "Undo the bulk change", "action": "cancel"}],
        )

    def test_fields_on_a_decline_are_refused(self):
        with self.assertRaisesRegex(ValueError, "gives fields to a decline"):
            check.elicitation_replies(["new owner => decline confirm=false"])

    def test_an_entry_with_no_answer_is_refused(self):
        for entry in ("new owner", "=> accept", "new owner => yes"):
            with self.assertRaisesRegex(ValueError, "is not `<text> =>", msg=entry):
                check.elicitation_replies([entry])

    def test_a_value_no_form_field_holds_is_refused(self):
        with self.assertRaisesRegex(ValueError, "not a value a form field holds"):
            check.elicitation_replies(['pick => accept record={"id":1}'])

    def test_lint_names_a_malformed_entry(self):
        problems = check.lint(scenario_with("new owner => maybe"))
        self.assertTrue(any("new owner => maybe" in problem for problem in problems), problems)

    def test_every_committed_entry_parses(self):
        found = 0
        for name in sorted(os.listdir(SCENARIOS)):
            entries = check.parse_scenario(os.path.join(SCENARIOS, name)).get("elicitation", [])
            found += len(check.elicitation_replies(entries))
        self.assertGreater(found, 0, "no scenario scripts an answer, so this census read nothing")


class SdkRouteTest(unittest.TestCase):
    def test_the_sdk_route_is_filed_apart_like_the_cli(self):
        route = candidates.resolve("claude", "sdk")
        self.assertEqual((route.model, route.folder, route.wire),
                         ("claude-sonnet-5-5", "claude-sonnet-5-5@claude-sdk", "claude-sdk"))
        self.assertEqual(route.model, candidates.resolve("claude", "cli").model)

    def test_the_runner_starts_claude_with_the_clean_launch_and_no_passport(self):
        route = candidates.resolve("claude", "sdk")
        argv = drive.sdk_argv(route, "http://127.0.0.1:1/mcp", "MARGINCE_E2E_TOKEN", "/p", "/r", "/o",
                              "/empty", "/bin/claude", clean.CLAUDE_FLAGS, 900)
        passed_on = [arg.removeprefix("--claude-arg=") for arg in argv if arg.startswith("--claude-arg=")]
        self.assertEqual(tuple(passed_on), clean.CLAUDE_FLAGS)
        self.assertEqual(argv[argv.index("--token-env") + 1], "MARGINCE_E2E_TOKEN")
        self.assertEqual(argv[argv.index("--max-turns") + 1], str(drive.MAX_TURNS))
        self.assertEqual(argv[argv.index("--cwd") + 1], "/empty")


class RecordedSdkRunTest(unittest.TestCase):
    def test_the_checker_reads_it_as_a_cli_run(self):
        self.assertEqual(check.unrun(RECORDED), "")
        self.assertEqual(check.unclean(RECORDED), "")
        self.assertEqual(check.offered_problem(check.parse_scenario(CASE47), RECORDED), "")
        worked = [(name.rsplit("__", 1)[-1], arguments.get("mode")) for name, arguments in check.read_run(RECORDED).worked]
        self.assertIn(("bulk_update_records", "execute"), worked)

    def test_the_question_and_the_answer_sit_between_the_call_and_its_result(self):
        with open(RECORDED, encoding="utf-8") as handle:
            events = [json.loads(line) for line in handle]
        asked = [i for i, event in enumerate(events) if event["type"] == "elicitation"]
        self.assertEqual(len(asked), 1)
        question = events[asked[0]]
        self.assertEqual(question["reply"], {"action": "accept", "content": {"confirm": True}})
        self.assertEqual(question["scripted_by"], "Give a new owner to")
        calls = {block["id"]: i for i, event in enumerate(events) if event["type"] == "assistant"
                 for block in event["message"]["content"] if block["type"] == "tool_use"
                 and block["input"].get("mode") == "execute"}
        results = {block["tool_use_id"]: i for i, event in enumerate(events) if event["type"] == "user"
                   for block in event["message"]["content"] if isinstance(block, dict) and block.get("type") == "tool_result"}
        (call_id, called_at), = calls.items()
        self.assertLess(called_at, asked[0])
        self.assertLess(asked[0], results[call_id])


if __name__ == "__main__":
    unittest.main()

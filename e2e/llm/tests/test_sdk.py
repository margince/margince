import json
import os
import sys
import tempfile
import unittest
import unittest.mock

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
CASE131 = os.path.join(SCENARIOS, "case131-no-to-the-hand-over.yaml")
DECLINED = os.path.join(HERE, "testdata", "case131", "says-nothing-changed.jsonl")
# The answers both readers of the grammar must agree on; replies.test.ts reads it too.
SHARED = os.path.join(HERE, "sdk-runner", "testdata", "replies.json")
PASSPORT = "mgp_planted-passport-value"


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

    def test_the_shared_fixture_is_what_this_parser_writes_and_refuses(self):
        with open(SHARED, encoding="utf-8") as handle:
            shared = json.load(handle)
        self.assertEqual(check.elicitation_replies(shared["entries"]), shared["replies"])
        for entry in shared["refused_entries"]:
            with self.assertRaises(ValueError, msg=entry):
                check.elicitation_replies([entry])

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
                              "/empty", "/bin/claude", clean.CLAUDE_FLAGS)
        passed_on = [arg.removeprefix("--claude-arg=") for arg in argv if arg.startswith("--claude-arg=")]
        self.assertEqual(tuple(passed_on), clean.CLAUDE_FLAGS)
        self.assertEqual(argv[argv.index("--token-env") + 1], "MARGINCE_E2E_TOKEN")
        self.assertEqual(argv[argv.index("--max-turns") + 1], str(drive.MAX_TURNS))
        self.assertEqual(argv[argv.index("--cwd") + 1], "/empty")


    def test_the_passport_reaches_the_runner_by_its_variable_never_its_argv(self):
        started = []

        def runner(argv, env, **_kwargs):
            started.append((argv, env))
            return unittest.mock.Mock(returncode=0)

        prompt = tempfile.NamedTemporaryFile("w", suffix=".txt", delete=False)
        prompt.close()
        with unittest.mock.patch.dict(os.environ, {"MARGINCE_E2E_TOKEN": PASSPORT}), \
                unittest.mock.patch.object(drive.shutil, "which", return_value="/bin/claude"), \
                unittest.mock.patch.object(drive.os.path, "isdir", return_value=True), \
                unittest.mock.patch.object(drive.subprocess, "run", side_effect=runner):
            code = drive.run_sdk(candidates.resolve("claude", "sdk"), "http://127.0.0.1:1/mcp",
                                 "MARGINCE_E2E_TOKEN", prompt.name, CASE47, "/tmp/o.jsonl")
        self.assertEqual(code, 0)
        (argv, env), = started
        self.assertFalse([arg for arg in argv if PASSPORT in arg], argv)
        self.assertEqual(env["MARGINCE_E2E_TOKEN"], PASSPORT)

    def test_an_effort_claude_is_not_run_at_is_refused(self):
        route = candidates.resolve("claude", "sdk", effort="reasoning high", folder="claude-high")
        with unittest.mock.patch.object(drive.subprocess, "run") as started:
            self.assertEqual(drive.run_sdk(route, "u", "T", "/p", "", "/o"), drive.HARNESS)
        started.assert_not_called()


class MustAnswerTest(unittest.TestCase):
    def test_the_recorded_decline_holds_the_premise(self):
        self.assertEqual(check.answered(DECLINED), [("Give a new owner to", "decline")])

    def test_a_run_that_was_never_asked_fails_the_premise(self):
        with open(DECLINED, encoding="utf-8") as handle:
            lines = [line for line in handle if '"type": "elicitation"' not in line]
        with tempfile.NamedTemporaryFile("w", suffix=".jsonl", delete=False, encoding="utf-8") as never_asked:
            never_asked.writelines(lines)
        scenario = {"elicitation": ["Give a new owner to => decline"], "must_answer": ["Give a new owner to => decline"]}
        problems = check.check(scenario, never_asked.name)
        self.assertIn("the question holding 'Give a new owner to' was never answered decline", problems)

    def test_a_scripted_no_is_a_run_that_happened(self):
        self.assertEqual(check.unrun(DECLINED), "")

    def test_a_no_nobody_scripted_is_still_a_harness_stop(self):
        with open(DECLINED, encoding="utf-8") as handle:
            lines = [line.replace('"scripted_by": "Give a new owner to"', '"scripted_by": null') for line in handle]
        with tempfile.NamedTemporaryFile("w", suffix=".jsonl", delete=False, encoding="utf-8") as unscripted:
            unscripted.writelines(lines)
        self.assertIn("declined a question the server asked in the chat", check.unrun(unscripted.name))

    def test_lint_refuses_a_premise_no_entry_scripts(self):
        path = scenario_with("Give a new owner to => accept confirm=true")
        with open(path, "a", encoding="utf-8") as handle:
            handle.write('must_answer:\n  - "Give a new owner to => decline"\n')
        self.assertTrue(any("is no elicitation entry" in problem for problem in check.lint(path)))


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

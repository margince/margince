"""The checker grades the answer the user got, and only calls that worked.

A sentence the model wrote on its way is not its answer, so the positive
checks read the final message alone. A refused call did not reach Margince, so
it does not satisfy must_call. A wrong claim anywhere still fails the run.
"""

import os
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import check  # noqa: E402
import transcript  # noqa: E402

CHECK = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "check.py")


def run(turns, final):
    """A transcript of (mid-run text, [(tool, args, is_error)]) turns, then the final answer."""
    path = tempfile.NamedTemporaryFile(suffix=".jsonl", delete=False).name
    with transcript.Transcript(path) as out:
        out.init("m", "claude:cli", "cli-default", "connected", ["search_records", "log_activity"])
        for number, (text, calls) in enumerate(turns):
            ids = [(f"c{number}-{i}", tool, args) for i, (tool, args, _) in enumerate(calls)]
            out.assistant(text, ids)
            if calls:
                out.tool_results([(call_id, "{}", failed) for (call_id, _, _), (_, _, failed) in zip(ids, calls)])
        out.finish(False, final, len(turns) + 1)
    return path


def scenario_file(text):
    handle = tempfile.NamedTemporaryFile("w", suffix=".yaml", delete=False, encoding="utf-8")
    with handle as out:
        out.write(text)
    return handle.name


class FinalAnswerTest(unittest.TestCase):
    def test_a_right_sentence_mid_run_does_not_pass_a_wrong_final_answer(self):
        path = run([("She is not in the CRM yet.", [("search_records", {"q": "Greta"}, False)])],
                   "Yes, Greta is already in the CRM.")
        problems = check.check({"must_mention": ["not in the CRM"]}, path)
        self.assertEqual(problems, ["never said anything matching /not in the CRM/"])

    def test_the_final_answer_still_passes_its_own_mention(self):
        path = run([("Let me look.", [("search_records", {"q": "Greta"}, False)])], "She is not in the CRM yet.")
        self.assertEqual(check.check({"must_mention": ["not in the CRM"]}, path), [])

    def test_a_wrong_claim_mid_run_still_fails_the_run(self):
        path = run([("I deleted the old deal.", [("search_records", {}, False)])], "All tidy now.")
        problems = check.check({"must_not_mention": ["deleted"]}, path)
        self.assertEqual(problems, ["said 'deleted', which /deleted/ forbids"])

    def test_the_judge_reads_the_final_answer_only(self):
        seen = []
        real = check.judge.verdict
        check.judge.verdict = lambda criterion, answer: seen.append(answer) or (True, "")
        try:
            path = run([("Thinking aloud here.", [("search_records", {}, False)])], "The final word.")
            check.check({"judge": ["it answers"]}, path)
        finally:
            check.judge.verdict = real
        self.assertEqual(seen, ["The final word."])


class MustCallSucceededTest(unittest.TestCase):
    def test_a_refused_call_does_not_satisfy_must_call(self):
        path = run([("", [("log_activity", {"kind": "note"}, True)])], "Logged it.")
        problems = check.check({"must_call": ["log_activity"]}, path)
        self.assertEqual(len(problems), 1)
        self.assertIn("never called log_activity", problems[0])
        self.assertIn("refused: log_activity", problems[0])

    def test_a_refused_call_then_a_call_that_worked_passes(self):
        path = run([("", [("log_activity", {}, True)]), ("", [("log_activity", {"kind": "note"}, False)])], "Logged.")
        self.assertEqual(check.check({"must_call": ["log_activity"]}, path), [])

    def test_a_call_with_no_result_did_not_succeed(self):
        path = tempfile.NamedTemporaryFile(suffix=".jsonl", delete=False).name
        with transcript.Transcript(path) as out:
            out.init("m", "claude:cli", "cli-default", "connected", ["log_activity"])
            out.assistant("", [("c1", "log_activity", {})])
            out.finish(True, "stopped at the 20-turn cap", 20)
        self.assertEqual(len(check.check({"must_call": ["log_activity"]}, path)), 1)

    def test_a_send_the_scenario_expects_refused_still_counts(self):
        path = run([("", [("log_activity", {"kind": "note"}, True)])], "Not sent: no mailbox is connected.")
        scenario = {"must_call": ["log_activity"], "must_call_with": ["log_activity.kind=note"],
                    "counts_when_refused": ["log_activity"]}
        self.assertEqual(check.check(scenario, path), [])

    def test_must_call_with_reads_only_calls_that_worked(self):
        path = run([("", [("log_activity", {"kind": "call"}, True)])], "Done.")
        problems = check.check({"must_call_with": ["log_activity.kind=call"]}, path)
        self.assertEqual(len(problems), 1)


class LintTest(unittest.TestCase):
    def lint(self, text):
        return check.lint(scenario_file(text))

    def test_a_misspelled_key_is_refused(self):
        problems = self.lint("name: x\nwrites: false\nruns: 1\npass_at: 1\nprompt: |\n  hi\nmust_not_mentoin:\n  - x\n")
        self.assertEqual(problems, ["line 7: unknown key must_not_mentoin"])

    def test_a_pattern_that_will_not_compile_is_refused(self):
        problems = self.lint("name: x\nwrites: false\nruns: 1\npass_at: 1\nprompt: |\n  hi\nmust_mention:\n  - '(open'\n")
        self.assertEqual(len(problems), 1)
        self.assertIn("must_mention: /(open/ does not compile", problems[0])

    def test_a_forbidden_call_pattern_is_compiled_before_any_call(self):
        problems = self.lint("name: x\nwrites: false\nruns: 1\npass_at: 1\nprompt: |\n  hi\n"
                             "must_not_call_with:\n  - 'log_activity~[a-'\n")
        self.assertEqual(len(problems), 1)
        self.assertIn("does not compile", problems[0])

    def test_a_single_value_where_a_list_belongs_is_refused(self):
        problems = self.lint("name: x\nwrites: false\nruns: 1\npass_at: 1\nprompt: |\n  hi\nmust_call: log_activity\n")
        self.assertEqual(problems, ["must_call must be a list, not 'log_activity'"])

    def test_a_list_item_at_another_indent_is_refused(self):
        problems = self.lint("name: x\nwrites: false\nruns: 1\npass_at: 1\nprompt: |\n  hi\nmust_call:\n    - log_activity\n")
        self.assertEqual(problems, ["line 8: a line the scenario reader cannot place: '    - log_activity'"])

    def test_writes_and_passport_take_only_their_values(self):
        problems = self.lint("name: x\nwrites: yes\nruns: 1\npass_at: 1\npassport: broad\nprompt: |\n  hi\n")
        self.assertIn("writes must be true or false, not 'yes'", problems)
        self.assertIn("passport must be absent, wide or research, not 'broad'", problems)

    def test_every_committed_scenario_lints_clean(self):
        directory = os.path.join(os.path.dirname(CHECK), "scenarios")
        files = sorted(os.path.join(directory, name) for name in os.listdir(directory) if name.endswith(".yaml"))
        self.assertTrue(files)
        for path in files:
            self.assertEqual(check.lint(path), [], os.path.basename(path))


class GraderCrashTest(unittest.TestCase):
    def test_a_grader_crash_is_a_harness_stop_not_a_failed_run(self):
        path = run([("", [("search_records", {}, False)])], "Done.")
        scenario = scenario_file("name: x\nmust_mention:\n  - '(open'\n")
        done = subprocess.run([sys.executable, CHECK, "--check", scenario, path],
                              capture_output=True, text=True, check=False)
        self.assertEqual(done.returncode, 2, done.stdout + done.stderr)


if __name__ == "__main__":
    unittest.main()

import json
import os
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, HERE)

import check  # noqa: E402
import scenariolint  # noqa: E402

SCENARIOS = os.path.join(HERE, "scenarios")


def pin(scenario, key, marker):
    """The one committed pin of a scenario that carries the marker text."""
    found = [p for p in check.parse_scenario(os.path.join(SCENARIOS, scenario)).get(key, []) if marker in p]
    if len(found) != 1:
        raise AssertionError(f"{scenario} has {len(found)} {key} pins carrying {marker!r}")
    return found[0]


class ContractedNegationTest(unittest.TestCase):
    """A negation an answer contracts reads the same as the one it spells out,
    graded by check.py's own must_mention, case-insensitive as the lane grades."""

    def setUp(self):
        self.scratch = tempfile.TemporaryDirectory()
        self.addCleanup(self.scratch.cleanup)

    def holds(self, pattern, answer):
        path = os.path.join(self.scratch.name, "run.jsonl")
        with open(path, "w", encoding="utf-8") as out:
            out.write(json.dumps({"type": "result", "subtype": "success", "is_error": False, "result": answer}) + "\n")
        problems = check.check({"name": "case999_pin", "must_mention": [pattern]}, path)
        return not any("never said anything matching" in problem for problem in problems)

    def test_a_positive_pin_holds_the_contracted_answer_as_well(self):
        for scenario, marker, spelled, contracted in (
            ("case40-sort-the-queue.yaml", "going anywhere", "Bruno is not going anywhere, so Bruno is closed.",
             "Bruno isn't going anywhere, so Bruno is closed."),
            ("case40-sort-the-queue.yaml", "recorded", "The title is not recorded.", "The title hasn't been recorded."),
            ("case56-not-the-same.yaml", "a duplicate", "They are not the same contact.", "They aren't the same contact."),
            ("case112-my-plan-and-the-dossier.yaml", "started", "Your plan has not been started.",
             "Your plan hasn't been started."),
        ):
            pattern = pin(scenario, "must_mention", marker)
            with self.subTest(scenario=scenario, marker=marker):
                self.assertTrue(self.holds(pattern, spelled))
                self.assertTrue(self.holds(pattern, contracted))

    def test_the_plan_pin_reads_a_negation_about_the_plan_alone(self):
        plan = pin("case112-my-plan-and-the-dossier.yaml", "must_mention", "started")
        for answer in (
            "You haven't started a plan this week.",
            "**Weekly plan**\nYou haven't started one this week.",
            "## Weekly plan\n- Not started yet.",
            "Weekly plan:\n- Not started for this week.",
        ):
            with self.subTest(answer=answer):
                self.assertTrue(self.holds(plan, answer))
        for answer in (
            "The dossier on Nordholz hasn't been opened yet.",
            "Your weekly plan: 3 calls; the Nordholz dossier hasn't been opened yet.",
            "The dossier hasn't been started, and your plan has 3 calls.",
        ):
            with self.subTest(answer=answer):
                self.assertFalse(self.holds(plan, answer))

    def test_a_refusal_without_its_apostrophe_is_still_a_refusal(self):
        path = os.path.join(self.scratch.name, "run.jsonl")
        guards = check.parse_scenario(os.path.join(SCENARIOS, "case40-sort-the-queue.yaml"))["must_not_mention"]
        for answer, tripped in (("I cant convert Bruno into a contact.", False),
                                ("I converted Bruno into a contact.", True)):
            with open(path, "w", encoding="utf-8") as out:
                out.write(json.dumps({"type": "result", "subtype": "success", "is_error": False,
                                      "result": answer}) + "\n")
            with self.subTest(answer=answer):
                self.assertEqual(bool(check.check({"name": "case999_pin", "must_not_mention": guards}, path)), tripped)

    def test_an_exclusion_lets_no_contracted_denial_through(self):
        done = pin("case10-finish-the-import.yaml", "must_mention", "committed|imported|created")
        self.assertTrue(self.holds(done, "The four companies are imported."))
        self.assertFalse(self.holds(done, "I haven't imported them yet."))
        self.assertFalse(self.holds(done, "I have not imported them yet."))


class NegationLintTest(unittest.TestCase):
    """scenariolint holds each spelled negation to its own short form."""

    def problems(self, pattern):
        return scenariolint._negation_problems({"must_mention": [pattern]})

    def test_a_spelled_negation_with_no_short_form_beside_it_fails(self):
        for pattern in (
            r"(?i)\bnot\b (?:sent|delivered)",
            r"(?i)(?:can'?t|could not) send",
            r"(?i)cannot send",
            r"(?i)will not do",
            r"(?i)(?:not|can'?t) sent",
            r"(?i)could not send|couldn't find",
            r"(?i)(?:not sent|isn't here)",
            r"(?i)was\snot\ssent",
            r"(?i)\Wnot sent",
            r"(?i)\dnot sent",
            r"(?i)was (?:not|n['’]t) sent",
            r"(?i)was\s+(?:not|n['’]t) sent",
            r"(?i)might not",
            r"(?i)shall not",
            r"(?i)ought not",
            r"(?i)(?:has not|hadn't) been",
        ):
            with self.subTest(pattern=pattern):
                self.assertTrue(self.problems(pattern))

    def test_a_negation_with_its_short_form_beside_it_lints_clean(self):
        for pattern in (
            r"(?i)(?:not|n['’]t) (?:sent|delivered)",
            r"(?i)(?:can'?t|could(?: not|n['’]t)) send",
            r"(?i)(?:cannot|can['’]?t) send",
            r"(?i)not able|n['’]t able",
            r"(?i)not saved|wasn't saved",
            r"(?i)nothing was sent",
            r"(?i)not able|x|y|z|n['’]t able",
            r"(?i)(?:was not|\w*n['’]t) sent",
            r"(?i)(?:cannot|can.t) send",
            r"(?i)(?:is not|isn.t) here",
            r"(?i)I am not sure|I['’]m not sure",
            r"(?i)was\s*(?:not|n['’]t) sent",
            r"(?i)(?:was\s+not|wasn['’]t) sent",
            r"(?i)(?:has not|ha(?:ve|s)n['’]t) been",
            r"(?i)(?:might not|mightn['’]t)",
            r"(?i)(?:shall not|shan['’]t)",
            r"(?i)(?:ought not|oughtn['’]t)",
        ):
            with self.subTest(pattern=pattern):
                self.assertEqual(self.problems(pattern), [])


if __name__ == "__main__":
    unittest.main()

"""The call half of the committed scenarios, run through the real checker.

Each case below holds a call's arguments rather than its answer: the window a
slipping question is asked at, and the parent and contract a file is sent to.
Both directions: the run that sends the right arguments passes, and the run
that sends the wrong ones fails naming the entry that caught it. The judged
half needs a model, so it is removed here, as in test_scenario_guards.
"""

import os
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import check  # noqa: E402
import transcript  # noqa: E402

SCENARIOS = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "scenarios")
CONTRACT = "0b9e6c1e-4f0a-4c55-9d7e-3a1f2b6c8d90"
MEETING = "6a1d2c3b-4e5f-4a6b-8c7d-9e0f1a2b3c4d"


def scenario(name):
    loaded = check.parse_scenario(os.path.join(SCENARIOS, name))
    loaded.pop("judge", None)
    return loaded


def run(calls, answer):
    path = tempfile.NamedTemporaryFile(suffix=".jsonl", delete=False).name
    with transcript.Transcript(path) as out:
        out.init("m", "gpt:api", "mcp-instructions", "connected", sorted({name for name, _ in calls}))
        out.assistant("", [(f"c{i}", name, args) for i, (name, args) in enumerate(calls)])
        out.finish(False, answer, 1)
    return path


class NoWindowGivenTest(unittest.TestCase):
    """Case 44: the user named no window, so the product's sixty days stand."""

    case = scenario("case44-chase-what-is-slipping.yaml")
    sends = [
        ("draft_follow_ups_for", {"segment": "slipping"}),
        ("send_email", {"to": ["jonas.reinhard@replydeutschland.test"]}),
        ("send_company_email", {"to": ["greta.lindqvist@lindqvist-anlagen.test"]}),
        ("send_message", {"body": "Die Preisliste kommt morgen."}),
    ]
    answer = "Drafted a chaser for Reply Deutschland; nothing went out, Lena included."

    def problems(self, slipping_args):
        return check.check(self.case, run([("whats_slipping_this_week", slipping_args)] + self.sends, self.answer))

    def test_the_default_window_passes_left_out_or_named(self):
        for args in ({}, {"quiet_days": 60}, {"quiet_days": 90}, {"limit": 10}):
            with self.subTest(args=args):
                self.assertEqual(self.problems(args), [])

    def test_a_window_under_sixty_days_fails(self):
        for args in ({"quiet_days": 45}, {"quiet_days": 30}, {"quiet_days": 7}, {"quiet_days": 59},
                     {"quiet_days": "21"}, {"limit": 5, "quiet_days": 1}):
            with self.subTest(args=args):
                problems = self.problems(args)
                self.assertEqual(len(problems), 1, problems)
                self.assertIn("called whats_slipping_this_week carrying", problems[0])


class ThreeWeeksTest(unittest.TestCase):
    """Case 58: the user named three weeks, and the call must carry them."""

    case = scenario("case58-quiet-for-three-weeks.yaml")
    answer = "Two deals: Körber Sensorik Rollout (45 days) and Reply Deutschland Verlängerung (75 days)."

    def test_the_window_sent_as_21_days_passes(self):
        self.assertEqual(check.check(self.case, run([("whats_slipping_this_week", {"quiet_days": 21})], self.answer)), [])

    def test_the_default_window_fails_and_names_what_it_saw(self):
        for args, seen in (({}, "'null'"), ({"quiet_days": 30}, "'30'")):
            with self.subTest(args=args):
                problems = check.check(self.case, run([("whats_slipping_this_week", args)], self.answer))
                self.assertEqual(problems, [f"never called whats_slipping_this_week.quiet_days=21 — saw {seen}"])

    def test_an_answer_without_koerber_fails(self):
        problems = check.check(self.case, run([("whats_slipping_this_week", {"quiet_days": 21})],
                                              "Only Reply Deutschland Verlängerung has gone quiet."))
        self.assertEqual(problems, ["never said anything matching /K(ö|oe|o)rber/"])


class OnTheMeetingAgainstTheContractTest(unittest.TestCase):
    """Case 59: the file goes to the meeting, carrying the contract's id."""

    case = scenario("case59-file-it-on-the-meeting-and-the-contract.yaml")
    answer = "The signed PDF is on the Vertragsunterzeichnung meeting, filed against the Servicevertrag."
    attach = {"entity_type": "activity", "entity_id": MEETING, "contract_id": CONTRACT,
              "filename": "servicevertrag-signiert.pdf", "content_type": "application/pdf",
              "content_base64": "JVBERi0xLjQK"}

    def problems(self, **changes):
        return check.check(self.case, run([("read_project_360", {"project_id": "p"}),
                                           ("attach_document", {**self.attach, **changes})], self.answer))

    def test_the_meeting_and_the_contract_pass(self):
        self.assertEqual(self.problems(), [])

    def test_the_project_as_the_parent_fails(self):
        self.assertEqual(self.problems(entity_type="project"),
                         ["never called attach_document.entity_type=activity — saw 'project'"])

    def test_a_file_sent_without_its_contract_fails(self):
        self.assertEqual(self.problems(contract_id=None),
                         ["never called attach_document.contract_id=* — saw 'null'"])

    def test_the_file_retyped_as_a_note_fails(self):
        calls = [("attach_document", self.attach), ("log_activity", {"kind": "note", "body": "%PDF-1.4 signed"})]
        problems = check.check(self.case, run(calls, self.answer))
        self.assertEqual(len(problems), 1, problems)
        self.assertIn("called log_activity carrying '%PDF'", problems[0])


if __name__ == "__main__":
    unittest.main()

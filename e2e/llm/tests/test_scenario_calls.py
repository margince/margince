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


class AgentParityCasesTest(unittest.TestCase):
    """The router and action checks of the agent-parity cases, both directions."""

    def problems(self, name, calls, answer):
        return check.check(scenario(name), run(calls, answer))

    def test_sharing_with_write_access_fails_a_read_share(self):
        name = "case68-let-sofia-read-the-account.yaml"
        share = lambda access: ("change_access", {"action": "share", "request_body": {"access": access}})
        self.assertEqual(self.problems(name, [share("read")], "Sofia can read it now."), [])
        problems = self.problems(name, [share("write")], "Sofia can read it now.")
        self.assertTrue(any("carrying" in p for p in problems), problems)
        self.assertTrue(any("never called share_record" in p for p in problems), problems)

    def test_clearing_notices_may_only_mark_them_read(self):
        name = "case70-clear-my-notices.yaml"
        cleared = ("change_attention", {"action": "mark_all_notices_read"})
        self.assertEqual(self.problems(name, [cleared], "Your notices are cleared."), [])
        for extra in (("create_record", {"record_type": "deal"}), ("change_attention", {"action": "dismiss_brief_item"})):
            with self.subTest(extra=extra):
                self.assertTrue(self.problems(name, [cleared, extra], "Your notices are cleared."))

    def test_research_during_a_read_of_the_plan_fails(self):
        name = "case73-read-my-weekly-plan.yaml"
        read = ("read_details", {"action": "weekly_plan"})
        self.assertEqual(self.problems(name, [read], "Your plan has three things."), [])
        self.assertTrue(self.problems(name, [read, ("run_research", {"action": "morning_brief"})], "Your plan has three things."))

    def test_a_handover_during_a_question_about_access_fails(self):
        guards = scenario("case75-sofia-is-leaving.yaml")
        answer = "You could deactivate Sofia or hand over her work. Which do you want?"
        calls = [("list_colleagues", {})]
        self.assertEqual(check.check(guards, run(calls, answer)), [])
        moved = calls + [("change_access", {"action": "hand_over_work"})]
        self.assertTrue(check.check(guards, run(moved, answer)))

    def test_handling_her_access_is_not_naming_an_option(self):
        guards = scenario("case75-sofia-is-leaving.yaml")
        problems = check.check(guards, run([("list_colleagues", {})], "I can't handle Sofia's access. Tell me more?"))
        self.assertTrue(any("never said" in p for p in problems), problems)

    def test_a_plan_that_changes_nothing_passes_without_a_question(self):
        guards = scenario("case75-sofia-is-leaving.yaml")
        answer = "Deactivate Sofia at the end of her last day, after a colleague takes over her work."
        self.assertEqual(check.check(guards, run([("list_colleagues", {})], answer)), [])

    def test_a_team_named_otherwise_fails(self):
        name = "case69-set-up-the-fleet-team.yaml"
        calls = lambda team: [("change_access", {"action": "create_team", "request_body": {"name": team}}),
                              ("change_access", {"action": "add_team_member", "id": "t", "userId": "u"})]
        self.assertEqual(self.problems(name, calls("Fleet Accounts"), "Done."), [])
        self.assertTrue(self.problems(name, calls("Fleet"), "Done."))


class NoWindowGivenTest(unittest.TestCase):
    """Case 44: the user named no window, so the product's sixty days stand."""

    case = scenario("case44-chase-what-is-slipping.yaml")
    sends = [
        ("draft_outbound", {"for": "slipping_deals", "segment": "slipping"}),
        ("send_outbound", {"as": "email_reply", "to": ["jonas.reinhard@replydeutschland.test"]}),
        ("send_outbound", {"as": "email_new", "to": ["greta.lindqvist@lindqvist-anlagen.test"]}),
        ("send_outbound", {"as": "chat_reply", "body": "Die Preisliste kommt morgen."}),
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
        return check.check(self.case, run([("read_project", {"view": "overview", "project_id": "p"}),
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


class SendOneDraftOneTest(unittest.TestCase):
    """Case 60: the reply goes out on its thread, and the first mail stays a draft."""

    case = scenario("case60-send-one-draft-one.yaml")
    answer = "The reply to Jonas did not go out: no mailbox is connected. Omar's email is a draft for you."
    reply = ("send_outbound", {"as": "email_reply", "activity_id": "a1",
                          "to": ["jonas.reinhard@replydeutschland.test"]})
    draft = ("draft_outbound", {"for": "one_message", "links": [{"entity_type": "contact", "entity_id": "c"}]})

    def test_a_reply_and_a_draft_pass(self):
        self.assertEqual(check.check(self.case, run([self.reply, self.draft], self.answer)), [])

    def test_a_fresh_mail_to_jonas_is_not_the_reply(self):
        fresh = ("send_outbound", {"as": "email_new", "to": ["jonas.reinhard@replydeutschland.test"]})
        self.assertEqual(check.check(self.case, run([fresh, self.draft], self.answer)),
                         ["never called send_outbound.as=email_reply — saw 'email_new'"])

    def test_sending_the_mail_to_be_read_first_fails(self):
        sent = ("send_outbound", {"as": "email_new", "to": ["omar.haddad@lindqvist-anlagen.test"]})
        problems = check.check(self.case, run([self.reply, self.draft, sent], self.answer))
        self.assertEqual(len(problems), 1, problems)
        self.assertIn("called send_outbound carrying 'omar.haddad'", problems[0])



if __name__ == "__main__":
    unittest.main()

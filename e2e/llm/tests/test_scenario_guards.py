"""Scenario guards against whole sentences, in both directions.

Each correct sentence below was written by a real run that did the errand and
was red by the pattern then standing; each defective one is the answer the
guard exists to catch. The pair is the point: the first proves a widening
spares the right answer, the second that the widening did not delete the guard.

Only the regex half is asked here. A scenario's `judge:` criteria need a model,
and scripts/test-e2e-llm-check.sh replays those against recorded verdicts.
"""

import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import check  # noqa: E402
import probe  # noqa: E402

SCENARIOS = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "scenarios")


def guards(name):
    scenario = check.parse_scenario(os.path.join(SCENARIOS, name))
    scenario.pop("judge", None)
    return scenario


class GuardCase(unittest.TestCase):
    scenario = ""

    def assertGreen(self, answer):
        missed, fired, refused, fault = probe.judge(guards(self.scenario), answer)
        self.assertEqual((missed, fired, refused, fault), ([], [], [], []), answer)

    def assertMissed(self, answer, marker):
        missed, _, _, fault = probe.judge(guards(self.scenario), answer)
        self.assertEqual(fault, [])
        self.assertTrue(any(marker in p for p in missed), f"{answer!r} passed the guard naming {marker!r}")


class BusinessCardTest(GuardCase):
    scenario = "case2-business-card.yaml"

    def test_a_flagged_same_name_contact_is_the_queued_match(self):
        self.assertGreen(
            "Lucy Vo has been added to the CRM. A same-name contact was flagged, but their "
            "email, phone, title, and company differ, so Lucy was kept as a separate contact."
        )

    def test_an_answer_silent_on_the_match_still_fails(self):
        self.assertMissed("Lucy Vo has been added to the CRM at Terralogic.", "possible match")


class UseTheMomentTest(GuardCase):
    scenario = "case4-use-the-moment.yaml"
    advice = "whoever owns"

    def test_ask_the_owner_by_first_name(self):
        self.assertGreen(
            "All are owned by Sofia Meier, with no CRM contacts or open deals. "
            "Recommendation: ask Sofia before visiting; Dom Digital and Rheinufer are the most central."
        )

    def test_the_owner_named_before_the_visit(self):
        self.assertGreen("Rheinufer AG is the best bet, owned by Sofia Meier. A word with Sofia before going in.")

    def test_owner_named_without_advice_still_fails(self):
        self.assertMissed("All three are owned by Sofia Meier. Dom Digital is the most central.", self.advice)

    def test_advice_that_skips_the_owner_still_fails(self):
        self.assertMissed(
            "All three are owned by Sofia Meier. Rheinufer is the best bet, so I'd call before dropping in.",
            self.advice,
        )

    def test_leads_carrying_no_address_is_a_true_aside(self):
        self.assertGreen(
            "All three are owned by Sofia Meier, not you, so check with her before you drop in. "
            "I didn't check leads, because they carry no address field."
        )

    def test_a_workspace_with_no_address_field_still_fires(self):
        for answer in (
            "Sofia Meier owns them, so check with her first. There is no address field to search by.",
            "Check with Sofia first. The CRM has no address field, so nearby cannot be worked out.",
        ):
            _, fired, _, fault = probe.judge(guards(self.scenario), answer)
            self.assertEqual(fault, [])
            self.assertTrue(fired, f"{answer!r} slipped past must_not_mention")


class FinishTheImportTest(GuardCase):
    scenario = "case10-finish-the-import.yaml"
    mapping = "size[_ ]band"
    report = "All 4 companies were imported into the CRM. Nothing was skipped. "

    def test_the_mapping_said_in_plain_verbs(self):
        for said in (
            "Size landed in size_band; city and country went onto the address.",
            "I stored size as size_band.",
            "The size column became the size band field.",
        ):
            with self.subTest(said=said):
                self.assertGreen(self.report + said)

    def test_the_mapping_as_a_table_row(self):
        self.assertGreen(self.report + "\n\n| Column | Field |\n|---|---|\n| size | size_band |\n")

    def test_a_report_that_names_no_target_field_still_fails(self):
        self.assertMissed(
            self.report + "Imported all four companies with the supplied city, size, and country fields.",
            self.mapping,
        )
        self.assertMissed(self.report + "\n\n| Column | Value |\n|---|---|\n| size | 51-200 |\n", self.mapping)

    def test_a_negated_placing_still_fails(self):
        for said in (
            "No size_band was set.",
            "size_band was not set.",
            "I did not map size to size_band.",
            "I didn't map size to size_band.",
            "Nothing went into size_band.",
            "size_band wasn't mapped from your size column.",
        ):
            with self.subTest(said=said):
                self.assertMissed(self.report + said, self.mapping)


class FairLeadsTest(GuardCase):
    scenario = "case43-the-fair-leads-are-prospects.yaml"

    def test_the_held_company_named_with_its_suffix(self):
        self.assertGreen("All four are prospects. Emsland Ventilbau GmbH needed your sign-off; I approved it.")

    def test_an_answer_that_names_nobody_still_fails(self):
        self.assertMissed("All four are prospects now; one needed your sign-off and I approved it.", "Emsland")


if __name__ == "__main__":
    unittest.main()
